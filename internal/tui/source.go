package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rafaelfragoso/columbus/internal/memory"
	"github.com/rafaelfragoso/columbus/internal/store"
	"github.com/rafaelfragoso/columbus/internal/work"
)

const (
	maxMemRows = 12
	maxHubRows = 12
)

// StoreSource loads a Snapshot from the project store and memory manager. It is
// a pure read: it never mutates and never re-resolves the working tree.
type StoreSource struct {
	DB     *store.DB
	Memory *memory.Manager
	Branch string // resolved by the caller (git); blank is tolerated
}

// Load reads index metadata, epics/tasks (with a derived task roll-up),
// memories, and graph hubs into an immutable Snapshot.
func (s *StoreSource) Load() (Snapshot, error) {
	meta, err := s.DB.Meta().Get()
	if err != nil {
		return Snapshot{}, err
	}

	epics, err := s.DB.ListEpics("", "")
	if err != nil {
		return Snapshot{}, err
	}
	tasks, err := s.DB.ListTasks(0, "", "")
	if err != nil {
		return Snapshot{}, err
	}

	done, total := map[int64]int{}, map[int64]int{}
	taskRows := make([]TaskRow, 0, len(tasks))
	for _, t := range tasks {
		total[t.EpicID]++
		if t.Status == "done" {
			done[t.EpicID]++
		}
		taskRows = append(taskRows, TaskRow{
			ID: t.ID, EpicID: t.EpicID, IDStr: work.FormatTaskID(t.ID),
			Title: t.Title, Status: t.Status,
		})
	}
	epicRows := make([]EpicRow, 0, len(epics))
	for _, e := range epics {
		epicRows = append(epicRows, EpicRow{
			ID: e.ID, IDStr: work.FormatEpicID(e.ID), Title: e.Title, Status: e.Status,
			Done: done[e.ID], Total: total[e.ID],
		})
	}

	var memRows []MemRow
	var memCounts map[string]int
	memTotal := 0
	if s.Memory != nil {
		list, err := s.Memory.List("", "")
		if err != nil {
			return Snapshot{}, err
		}
		memCounts, memTotal = list.Counts, list.Total
		for i, m := range list.Memories {
			if i >= maxMemRows {
				break
			}
			memRows = append(memRows, MemRow{ID: m.ID, Kind: m.Kind, Title: m.Title})
		}
	}

	depEdges, err := s.DB.AllDepEdges()
	if err != nil {
		return Snapshot{}, err
	}
	testLinks, err := s.DB.AllTestLinks()
	if err != nil {
		return Snapshot{}, err
	}
	inDeg := map[string]int{}
	for _, e := range depEdges {
		inDeg[e.To]++
	}
	hubs := topHubs(inDeg)
	if len(hubs) == 0 {
		// Package-based languages (notably Go) import package paths, not single
		// files, so the indexer can't resolve them to file ids and dep_edges is
		// empty. Fall back to raw import specifiers so the graph still surfaces
		// the project's most-depended-on modules.
		imports, err := s.DB.AllImports()
		if err != nil {
			return Snapshot{}, err
		}
		hubs = topHubs(specifierInDegree(imports))
	}

	return Snapshot{
		Branch: s.Branch, Head: meta.IndexedHead, Dirty: meta.Dirty, LastIndexedAt: meta.LastIndexedAt,
		Files: meta.FilesCount, Symbols: meta.SymbolsCount, Edges: len(depEdges) + len(testLinks),
		Memories: memTotal, MemCounts: memCounts,
		Epics: epicRows, Tasks: taskRows, Mems: memRows, Hubs: hubs,
	}, nil
}

// topHubs turns an in-degree map into the highest-degree HubRows, ordered by
// in-degree then path, capped at maxHubRows.
func topHubs(inDeg map[string]int) []HubRow {
	hubs := make([]HubRow, 0, len(inDeg))
	for path, in := range inDeg {
		hubs = append(hubs, HubRow{Path: path, In: in})
	}
	sort.Slice(hubs, func(i, j int) bool {
		if hubs[i].In != hubs[j].In {
			return hubs[i].In > hubs[j].In
		}
		return hubs[i].Path < hubs[j].Path
	})
	if len(hubs) > maxHubRows {
		hubs = hubs[:maxHubRows]
	}
	return hubs
}

// specifierInDegree counts how many files import each module specifier. Stdlib
// imports (a bare or slash-only path whose first segment has no dot, e.g. "fmt"
// or "os/signal") are excluded as graph noise; if that leaves nothing, every
// specifier is counted so the panel is never needlessly empty.
func specifierInDegree(imports []store.ImportRow) map[string]int {
	inDeg := map[string]int{}
	for _, im := range imports {
		if isModuleSpecifier(im.Specifier) {
			inDeg[im.Specifier]++
		}
	}
	if len(inDeg) == 0 {
		for _, im := range imports {
			inDeg[im.Specifier]++
		}
	}
	return inDeg
}

// isModuleSpecifier reports whether an import path looks like a third-party or
// module-local dependency (its first path segment contains a dot, as in a host
// name) rather than a standard-library import.
func isModuleSpecifier(spec string) bool {
	first := spec
	if i := strings.IndexByte(spec, '/'); i >= 0 {
		first = spec[:i]
	}
	return strings.Contains(first, ".")
}

// Detail renders a full markdown document for an epic or task: body, tags,
// references, child tasks (epics) and the append-only history. Returns "" when
// the id is unknown.
func (s *StoreSource) Detail(kind string, id int64) (string, error) {
	switch kind {
	case "epic":
		e, ok, err := s.DB.EpicFull(id)
		if err != nil || !ok {
			return "", err
		}
		events, err := s.DB.WorkEvents("epic", id)
		if err != nil {
			return "", err
		}
		tasks, err := s.DB.ListTasks(id, "", "")
		if err != nil {
			return "", err
		}
		return epicDetailMarkdown(e, tasks, events), nil
	case "task":
		t, ok, err := s.DB.TaskFull(id)
		if err != nil || !ok {
			return "", err
		}
		events, err := s.DB.WorkEvents("task", id)
		if err != nil {
			return "", err
		}
		return taskDetailMarkdown(t, events), nil
	case "memory":
		mm, ok, err := s.DB.MemoryFull(id)
		if err != nil || !ok {
			return "", err
		}
		return memoryDetailMarkdown(mm), nil
	}
	return "", nil
}

func memoryDetailMarkdown(mm store.Memory) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", mm.Title)
	fmt.Fprintf(&b, "**Kind:** %s  ·  **ID:** `%s`\n\n", statusLabel(mm.Kind), memory.FormatID(mm.ID))
	if mm.Body != "" {
		b.WriteString(mm.Body + "\n\n")
	}
	if len(mm.Tags) > 0 {
		fmt.Fprintf(&b, "**Tags:** %s\n", strings.Join(mm.Tags, ", "))
	}
	return b.String()
}

func epicDetailMarkdown(e store.Epic, tasks []store.TaskBrief, events []store.WorkEvent) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", e.Title)
	fmt.Fprintf(&b, "**Status:** %s  ·  **ID:** `%s`\n\n", statusLabel(e.Status), work.FormatEpicID(e.ID))
	if e.Body != "" {
		b.WriteString(e.Body + "\n\n")
	}
	if len(e.Tags) > 0 {
		fmt.Fprintf(&b, "**Tags:** %s\n\n", strings.Join(e.Tags, ", "))
	}
	writeRefs(&b, e.Refs)
	if len(tasks) > 0 {
		b.WriteString("## Tasks\n\n")
		for _, t := range tasks {
			fmt.Fprintf(&b, "- `%s` %s — _%s_\n", work.FormatTaskID(t.ID), t.Title, statusLabel(t.Status))
		}
		b.WriteString("\n")
	}
	writeHistory(&b, events)
	return b.String()
}

func taskDetailMarkdown(t store.Task, events []store.WorkEvent) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", t.Title)
	fmt.Fprintf(&b, "**Status:** %s  ·  **ID:** `%s`  ·  **Epic:** `%s`\n\n",
		statusLabel(t.Status), work.FormatTaskID(t.ID), work.FormatEpicID(t.EpicID))
	if t.Body != "" {
		b.WriteString(t.Body + "\n\n")
	}
	if len(t.Tags) > 0 {
		fmt.Fprintf(&b, "**Tags:** %s\n\n", strings.Join(t.Tags, ", "))
	}
	writeRefs(&b, t.Refs)
	writeHistory(&b, events)
	return b.String()
}

func writeRefs(b *strings.Builder, refs []store.WorkRef) {
	if len(refs) == 0 {
		return
	}
	b.WriteString("## References\n\n")
	for _, r := range refs {
		fmt.Fprintf(b, "- `%s` %s\n", r.TargetType, r.TargetRef)
	}
	b.WriteString("\n")
}

func writeHistory(b *strings.Builder, events []store.WorkEvent) {
	if len(events) == 0 {
		return
	}
	b.WriteString("## History\n\n")
	for _, ev := range events {
		switch {
		case ev.NewStatus != "" && ev.Comment != "":
			fmt.Fprintf(b, "- `%s` → **%s** — %s\n", ev.CreatedAt, statusLabel(ev.NewStatus), ev.Comment)
		case ev.NewStatus != "":
			fmt.Fprintf(b, "- `%s` → **%s**\n", ev.CreatedAt, statusLabel(ev.NewStatus))
		default:
			fmt.Fprintf(b, "- `%s` %s\n", ev.CreatedAt, ev.Comment)
		}
	}
}
