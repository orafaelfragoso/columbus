package memory

import (
	"strings"

	"github.com/orafaelfragoso/columbus/internal/contract"
	"github.com/orafaelfragoso/columbus/internal/store"
)

// AddParams are the inputs to Add.
type AddParams struct {
	Kind     string
	Title    string
	Body     string
	Evidence []EvidenceSpec
	Links    []LinkSpec
	Tags     []string
}

// Add creates a new memory and returns the stored record with any warnings
// (unresolved links, missing evidence files).
func (m *Manager) Add(p AddParams) (MemoryResult, error) {
	if !validKind(p.Kind) {
		return MemoryResult{}, &contract.Error{Code: contract.CodeInvalidKind,
			Message: "unknown memory kind: " + p.Kind, Hint: "one of: " + strings.Join(Kinds, ", ")}
	}
	if strings.TrimSpace(p.Title) == "" {
		return MemoryResult{}, contract.Errorf(contract.CodeUsage, "memory --title is required")
	}

	var warnings []string
	for _, l := range p.Links {
		if w := m.resolveLinkWarning(l); w != "" {
			warnings = append(warnings, w)
		}
	}

	now := m.now()
	var newID int64
	err := m.DB.WithTx(func(tx *store.Tx) error {
		id, err := tx.NextMemSeq()
		if err != nil {
			return err
		}
		newID = id
		if err := tx.InsertMemory(id, p.Kind, p.Title, p.Body, now, now); err != nil {
			return err
		}
		for _, tag := range dedupe(p.Tags) {
			if err := tx.AddTag(id, tag); err != nil {
				return err
			}
		}
		for _, ev := range p.Evidence {
			if err := tx.AddEvidence(id, ev.Path, ev.Start, ev.End, m.blobOIDOf(ev.Path)); err != nil {
				return err
			}
		}
		for _, l := range p.Links {
			if err := tx.AddLink(id, l.Type, l.Ref); err != nil {
				return err
			}
		}
		return tx.ReindexMemoryFTS(id, p.Title, p.Body, dedupe(p.Tags))
	})
	if err != nil {
		return MemoryResult{}, err
	}
	return m.load(newID, warnings)
}

// EditParams are partial changes; nil pointers mean "unchanged".
type EditParams struct {
	Title          *string
	Body           *string
	Kind           *string
	AddTags        []string
	RemoveTags     []string
	AddEvidence    []EvidenceSpec
	RemoveEvidence []EvidenceSpec
	AddLinks       []LinkSpec
	RemoveLinks    []LinkSpec
}

func (p EditParams) empty() bool {
	return p.Title == nil && p.Body == nil && p.Kind == nil &&
		len(p.AddTags) == 0 && len(p.RemoveTags) == 0 &&
		len(p.AddEvidence) == 0 && len(p.RemoveEvidence) == 0 &&
		len(p.AddLinks) == 0 && len(p.RemoveLinks) == 0
}

// Edit applies partial changes to a memory.
func (m *Manager) Edit(idStr string, p EditParams) (MemoryResult, error) {
	id, err := ParseID(idStr)
	if err != nil {
		return MemoryResult{}, err
	}
	if p.empty() {
		return MemoryResult{}, contract.Errorf(contract.CodeUsage, "edit requires at least one change")
	}
	if p.Kind != nil && !validKind(*p.Kind) {
		return MemoryResult{}, &contract.Error{Code: contract.CodeInvalidKind, Message: "unknown memory kind: " + *p.Kind}
	}
	cur, ok, err := m.DB.MemoryFull(id)
	if err != nil {
		return MemoryResult{}, err
	}
	if !ok {
		return MemoryResult{}, notFound(idStr)
	}

	var warnings []string
	for _, l := range p.AddLinks {
		if w := m.resolveLinkWarning(l); w != "" {
			warnings = append(warnings, w)
		}
	}

	kind, title, body := cur.Kind, cur.Title, cur.Body
	if p.Kind != nil {
		kind = *p.Kind
	}
	if p.Title != nil {
		title = *p.Title
	}
	if p.Body != nil {
		body = *p.Body
	}

	err = m.DB.WithTx(func(tx *store.Tx) error {
		if err := tx.UpdateMemory(id, kind, title, body, m.now()); err != nil {
			return err
		}
		for _, t := range p.RemoveTags {
			if err := tx.RemoveTag(id, t); err != nil {
				return err
			}
		}
		for _, t := range p.AddTags {
			if err := tx.AddTag(id, t); err != nil {
				return err
			}
		}
		for _, ev := range p.RemoveEvidence {
			if err := tx.RemoveEvidence(id, ev.Path, ev.Start, ev.End); err != nil {
				return err
			}
		}
		for _, ev := range p.AddEvidence {
			if err := tx.AddEvidence(id, ev.Path, ev.Start, ev.End, m.blobOIDOf(ev.Path)); err != nil {
				return err
			}
		}
		for _, l := range p.RemoveLinks {
			if err := tx.RemoveLink(id, l.Type, l.Ref); err != nil {
				return err
			}
		}
		for _, l := range p.AddLinks {
			if err := tx.AddLink(id, l.Type, l.Ref); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return MemoryResult{}, err
	}

	// Rebuild FTS from the now-current tags.
	updated, _, err := m.DB.MemoryFull(id)
	if err != nil {
		return MemoryResult{}, err
	}
	if err := m.DB.WithTx(func(tx *store.Tx) error {
		return tx.ReindexMemoryFTS(id, updated.Title, updated.Body, updated.Tags)
	}); err != nil {
		return MemoryResult{}, err
	}
	return m.load(id, warnings)
}

// Link adds links to an existing memory.
func (m *Manager) Link(idStr string, links []LinkSpec) (MemoryResult, error) {
	id, err := ParseID(idStr)
	if err != nil {
		return MemoryResult{}, err
	}
	if len(links) == 0 {
		return MemoryResult{}, contract.Errorf(contract.CodeUsage, "link requires at least one --link")
	}
	if ok, err := m.DB.MemoryExists(id); err != nil {
		return MemoryResult{}, err
	} else if !ok {
		return MemoryResult{}, notFound(idStr)
	}
	// Resolve link warnings before the transaction: DB reads cannot run inside
	// WithTx (the writer holds the single connection).
	var warnings []string
	for _, l := range links {
		if w := m.resolveLinkWarning(l); w != "" {
			warnings = append(warnings, w)
		}
	}
	err = m.DB.WithTx(func(tx *store.Tx) error {
		for _, l := range links {
			if err := tx.AddLink(id, l.Type, l.Ref); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return MemoryResult{}, err
	}
	return m.load(id, warnings)
}

// Remove hard-deletes a memory (no interactive prompt; agent-friendly).
func (m *Manager) Remove(idStr string) (RemoveResult, error) {
	id, err := ParseID(idStr)
	if err != nil {
		return RemoveResult{}, err
	}
	ok, err := m.DB.MemoryExists(id)
	if err != nil {
		return RemoveResult{}, err
	}
	if !ok {
		return RemoveResult{}, notFound(idStr)
	}
	if err := m.DB.WithTx(func(tx *store.Tx) error { return tx.DeleteMemory(id) }); err != nil {
		return RemoveResult{}, err
	}
	return RemoveResult{ID: FormatID(id), Removed: true}, nil
}

// List returns memory summaries filtered by optional kind and tag, plus counts.
func (m *Manager) List(kind, tag string) (ListResult, error) {
	if kind != "" && !validKind(kind) {
		return ListResult{}, &contract.Error{Code: contract.CodeInvalidKind, Message: "unknown memory kind: " + kind}
	}
	briefs, err := m.DB.ListMemories(kind, tag)
	if err != nil {
		return ListResult{}, err
	}
	res := ListResult{Kind: kind, Tag: tag, Counts: map[string]int{}}
	for _, b := range briefs {
		res.Memories = append(res.Memories, MemoryRef{ID: FormatID(b.ID), Kind: b.Kind, Title: b.Title})
		res.Counts[b.Kind]++
	}
	res.Total = len(res.Memories)
	return res, nil
}

// Search runs a pure FTS5 query over memory title/body/tags.
func (m *Manager) Search(query string, limit int) (ListResult, error) {
	if strings.TrimSpace(query) == "" {
		return ListResult{}, contract.Errorf(contract.CodeUsage, "memory search requires a query")
	}
	if limit <= 0 {
		limit = 20
	}
	match := ftsMatch(query)
	ids, err := m.DB.SearchMemoryFTS(match, limit)
	if err != nil {
		return ListResult{}, err
	}
	res := ListResult{Counts: map[string]int{}}
	for _, id := range ids {
		b, ok, err := m.DB.MemoryBriefByID(id)
		if err != nil {
			return ListResult{}, err
		}
		if !ok {
			continue
		}
		res.Memories = append(res.Memories, MemoryRef{ID: FormatID(b.ID), Kind: b.Kind, Title: b.Title})
		res.Counts[b.Kind]++
	}
	res.Total = len(res.Memories)
	return res, nil
}

// ftsMatch builds a permissive prefix-OR MATCH expression from a query.
func ftsMatch(query string) string {
	var parts []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			parts = append(parts, `"`+strings.ToLower(cur.String())+`"*`)
			cur.Reset()
		}
	}
	for _, r := range query {
		if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return strings.Join(parts, " OR ")
}

// load reads a memory back into a typed result.
func (m *Manager) load(id int64, warnings []string) (MemoryResult, error) {
	full, ok, err := m.DB.MemoryFull(id)
	if err != nil {
		return MemoryResult{}, err
	}
	if !ok {
		return MemoryResult{}, notFound(FormatID(id))
	}
	r := resultFrom(full)
	r.Warnings = warnings
	return r, nil
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
