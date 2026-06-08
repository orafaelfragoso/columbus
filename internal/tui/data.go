// Package tui implements `columbus view`: a local, full-screen dashboard over the
// project's indexed state, embeddings, memory, and structured work (epics,
// stories and tasks). It is a read-mostly projection — the same data the
// JSON/LLM commands expose, rendered for humans. All state is derived from a
// Snapshot loaded through a Source port, which keeps the rendering layer
// testable without a terminal.
package tui

import (
	"sort"
	"strings"
)

// statusLabel renders a stored status/kind for display ("in_progress" → "IN
// PROGRESS"). Shared by the data layer (detail markdown) and the renderer.
func statusLabel(s string) string { return strings.ToUpper(strings.ReplaceAll(s, "_", " ")) }

// Snapshot is an immutable view of everything the dashboard renders. It is
// produced by a Source (the store adapter in production, a fake in tests).
type Snapshot struct {
	Branch        string
	Head          string
	Dirty         bool
	LastIndexedAt string

	Files      int
	Symbols    int
	Embeddings int
	Edges      int
	Memories   int
	MemCounts  map[string]int

	Epics   []EpicRow
	Stories []StoryRow
	Tasks   []TaskRow // every task, across all stories/epics
	Mems    []MemRow
	Hubs    []HubRow
}

// EpicRow is an epic summary plus its task roll-up (progress is derived, not
// stored — an epic has no percentage field).
type EpicRow struct {
	ID     int64
	IDStr  string
	Title  string
	Status string
	Done   int
	Total  int
}

// StoryRow is a story summary with its parent epic.
type StoryRow struct {
	ID     int64
	EpicID int64
	IDStr  string
	Title  string
	Status string
}

// TaskRow is a task summary with its parent story and denormalized epic.
type TaskRow struct {
	ID      int64
	EpicID  int64
	StoryID int64
	IDStr   string
	Title   string
	Status  string
}

// MemRow is a durable-memory summary.
type MemRow struct {
	ID    string
	Kind  string
	Title string
}

// HubRow is a file and its import in-degree.
type HubRow struct {
	Path string
	In   int
}

// Source loads a Snapshot. Implemented by StoreSource in production.
type Source interface {
	Load() (Snapshot, error)
}

// SearchHit is one result of a global search across code, memory and work. It
// is produced by the search function wired via WithSearch.
type SearchHit struct {
	Grain   string // "symbol" | "file" | "memory" | "epic" | "story" | "task"
	ID      string
	Title   string
	Where   string // path:line for code, id for memory/epic/story/task
	Score   float64
	Snippet string
}

// DetailSource optionally renders a rich markdown detail for one work or memory
// item (kind is "epic", "story", "task" or "memory"). StoreSource implements it
// by fetching full bodies, tags, refs and history; the model falls back to a
// Snapshot summary otherwise.
type DetailSource interface {
	Detail(kind string, id int64) (string, error)
}

// Progress is the fraction of an epic's tasks that are done (0 when it has no
// tasks). It is the bar the work view draws.
func (e EpicRow) Progress() float64 {
	if e.Total <= 0 {
		return 0
	}
	return float64(e.Done) / float64(e.Total)
}

// open reports whether a status counts as still-open work.
func open(status string) bool {
	return status != "done" && status != "cancelled"
}

// EpicsActive counts epics that are neither done nor cancelled.
func (s Snapshot) EpicsActive() int {
	n := 0
	for _, e := range s.Epics {
		if open(e.Status) {
			n++
		}
	}
	return n
}

// StoriesOpen counts stories that are neither done nor cancelled.
func (s Snapshot) StoriesOpen() int {
	n := 0
	for _, st := range s.Stories {
		if open(st.Status) {
			n++
		}
	}
	return n
}

// TasksOpen counts tasks that are neither done nor cancelled.
func (s Snapshot) TasksOpen() int {
	n := 0
	for _, t := range s.Tasks {
		if open(t.Status) {
			n++
		}
	}
	return n
}

// TasksForEpic returns the tasks belonging to the given epic, in id order.
func (s Snapshot) TasksForEpic(epicID int64) []TaskRow {
	var out []TaskRow
	for _, t := range s.Tasks {
		if t.EpicID == epicID {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// TasksForStory returns the tasks belonging to the given story, in id order.
func (s Snapshot) TasksForStory(storyID int64) []TaskRow {
	var out []TaskRow
	for _, t := range s.Tasks {
		if t.StoryID == storyID {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
