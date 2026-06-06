package memory

// Validate checks every memory's evidence anchors and links against the current
// tree/index. Drift is always reported as a warning, never a hard invalid: a
// memory stays usable even when its evidence drifted.
func (m *Manager) Validate() (ValidateResult, error) {
	ids, err := m.DB.AllMemoryIDs()
	if err != nil {
		return ValidateResult{}, err
	}
	res := ValidateResult{}
	for _, id := range ids {
		full, ok, err := m.DB.MemoryFull(id)
		if err != nil {
			return ValidateResult{}, err
		}
		if !ok {
			continue
		}
		entry := MemoryValidation{ID: FormatID(id), Kind: full.Kind, Title: full.Title}

		for _, ev := range full.Evidence {
			status := m.evidenceStatus(ev.Path, ev.BlobOIDAtCreation)
			entry.Evidence = append(entry.Evidence, EvidenceStatus{
				Path: ev.Path, LineStart: ev.LineStart, LineEnd: ev.LineEnd, Status: status,
			})
			if status != "ok" {
				entry.Warnings = append(entry.Warnings, "evidence "+ev.Path+" is "+status)
				res.Drifted++
			}
		}
		for _, l := range full.Links {
			resolved := m.linkResolves(l.TargetType, l.TargetRef)
			entry.Links = append(entry.Links, LinkStatus{
				TargetType: l.TargetType, TargetRef: l.TargetRef, Resolved: resolved,
			})
			if !resolved {
				entry.Warnings = append(entry.Warnings, "link "+l.TargetType+":"+l.TargetRef+" does not resolve")
				res.Unresolved++
			}
		}
		res.Memories = append(res.Memories, entry)
	}
	res.Total = len(res.Memories)
	res.Healthy = res.Drifted == 0 && res.Unresolved == 0
	return res, nil
}

// evidenceStatus compares stored evidence to the current working tree:
// "ok" (file blob unchanged), "stale" (file changed) or "broken" (file gone).
func (m *Manager) evidenceStatus(path, blobAtCreation string) string {
	current := m.blobOIDOf(path)
	if current == "" {
		return "broken"
	}
	if blobAtCreation != "" && current != blobAtCreation {
		return "stale"
	}
	return "ok"
}

func (m *Manager) linkResolves(targetType, ref string) bool {
	switch targetType {
	case "file":
		_, ok, _ := m.DB.FileByPath(ref)
		return ok
	case "symbol":
		rows, _ := m.DB.SymbolsByName(symbolName(ref))
		return len(rows) > 0
	}
	return false
}
