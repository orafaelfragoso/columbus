package store

// Memory is a full memory record with its associations.
type Memory struct {
	ID        int64
	Kind      string
	Title     string
	Body      string
	CreatedAt string
	UpdatedAt string
	Tags      []string
	Evidence  []Evidence
	Links     []Link
}

// Evidence is a git-anchored reference attached to a memory.
type Evidence struct {
	Path              string
	LineStart         int
	LineEnd           int
	BlobOIDAtCreation string
}

// Link associates a memory with a file or symbol.
type Link struct {
	TargetType string // "file" | "symbol"
	TargetRef  string
}

// MemoryFull fetches a memory and all its associations by numeric id.
func (d *DB) MemoryFull(id int64) (Memory, bool, error) {
	var m Memory
	err := d.db.QueryRow(`SELECT id, kind, title, body, created_at, updated_at FROM memories WHERE id = ?`, id).
		Scan(&m.ID, &m.Kind, &m.Title, &m.Body, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		if isNoRows(err) {
			return Memory{}, false, nil
		}
		return Memory{}, false, storeErr(err)
	}

	tags, err := d.pathQuery(`SELECT tag FROM memory_tags WHERE memory_id = ? ORDER BY tag`, id)
	if err != nil {
		return Memory{}, false, err
	}
	m.Tags = tags

	evRows, err := d.db.Query(`SELECT path, line_start, line_end, blob_oid_at_creation
		FROM memory_evidence WHERE memory_id = ? ORDER BY id`, id)
	if err != nil {
		return Memory{}, false, storeErr(err)
	}
	defer evRows.Close()
	for evRows.Next() {
		var e Evidence
		if err := evRows.Scan(&e.Path, &e.LineStart, &e.LineEnd, &e.BlobOIDAtCreation); err != nil {
			return Memory{}, false, storeErr(err)
		}
		m.Evidence = append(m.Evidence, e)
	}

	lnRows, err := d.db.Query(`SELECT target_type, target_ref FROM memory_links WHERE memory_id = ? ORDER BY id`, id)
	if err != nil {
		return Memory{}, false, storeErr(err)
	}
	defer lnRows.Close()
	for lnRows.Next() {
		var l Link
		if err := lnRows.Scan(&l.TargetType, &l.TargetRef); err != nil {
			return Memory{}, false, storeErr(err)
		}
		m.Links = append(m.Links, l)
	}
	return m, true, nil
}
