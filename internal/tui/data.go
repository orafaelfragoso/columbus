// Package tui implements `columbus view`: a local, full-screen dashboard over
// the project's indexed state, embeddings and durable memory. It is a
// read-mostly projection — the same data the JSON/LLM commands expose,
// rendered for humans. All state is derived from a Snapshot loaded through a
// Source port, which keeps the rendering layer testable without a terminal.
package tui

import "strings"

// kindLabel renders a stored memory kind for display ("adr" → "ADR"). Shared
// by the data layer (detail markdown) and the renderer.
func kindLabel(s string) string { return strings.ToUpper(strings.ReplaceAll(s, "_", " ")) }

// Snapshot is an immutable view of everything the dashboard renders. It is
// produced by a Source (the store adapter in production, a fake in tests).
type Snapshot struct {
	Branch string
	Head   string
	Dirty  bool

	Files      int
	Symbols    int
	Embeddings int
	Memories   int
	MemCounts  map[string]int

	Mems []MemRow
}

// MemRow is a durable-memory summary.
type MemRow struct {
	ID    string
	Kind  string
	Title string
	Tags  []string
}

// Source loads a Snapshot. Implemented by StoreSource in production.
type Source interface {
	Load() (Snapshot, error)
}

// SearchHit is one result of a global search across code and memory. It is
// produced by the search function wired via WithSearch.
type SearchHit struct {
	Grain   string // "symbol" | "file" | "memory"
	ID      string
	Title   string
	Where   string // path:line for code, id for memory
	Score   float64
	Snippet string
}

// DetailSource optionally renders a rich markdown detail for one memory
// (kind is "memory"). StoreSource implements it by fetching the full body,
// tags and links; the model falls back to a Snapshot summary otherwise.
type DetailSource interface {
	Detail(kind string, id int64) (string, error)
}
