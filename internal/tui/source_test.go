package tui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rafaelfragoso/columbus/internal/clock"
	"github.com/rafaelfragoso/columbus/internal/memory"
	"github.com/rafaelfragoso/columbus/internal/store"
	"github.com/rafaelfragoso/columbus/internal/work"
)

// newSeededSource opens a fresh store, seeds one epic with two tasks (one done)
// and one memory, and returns a StoreSource over it.
func newSeededSource(t *testing.T) *StoreSource {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "c.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	clk := clock.Fixed{T: time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)}
	wm := &work.Manager{DB: db, Clock: clk}
	mm := &memory.Manager{DB: db, Clock: clk, WorkDir: t.TempDir()}

	e, err := wm.EpicAdd(work.EpicAddParams{Title: "Indexing core"})
	if err != nil {
		t.Fatalf("EpicAdd: %v", err)
	}
	tk, err := wm.TaskAdd(work.TaskAddParams{Epic: e.ID, Title: "parse go"})
	if err != nil {
		t.Fatalf("TaskAdd: %v", err)
	}
	if _, err := wm.TaskAdd(work.TaskAddParams{Epic: e.ID, Title: "parse ts"}); err != nil {
		t.Fatalf("TaskAdd: %v", err)
	}
	if _, err := wm.TaskStatus(tk.ID, "done", ""); err != nil {
		t.Fatalf("TaskStatus: %v", err)
	}
	if _, err := mm.Add(memory.AddParams{Kind: "decision", Title: "use WAL"}); err != nil {
		t.Fatalf("memory.Add: %v", err)
	}
	return &StoreSource{DB: db, Memory: mm, Branch: "main"}
}

func TestStoreSourceLoadMapsWorkRollupAndMemory(t *testing.T) {
	snap, err := newSeededSource(t).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(snap.Epics) != 1 {
		t.Fatalf("epics = %d, want 1", len(snap.Epics))
	}
	e := snap.Epics[0]
	if e.IDStr != "epic_001" || e.Total != 2 || e.Done != 1 {
		t.Fatalf("epic roll-up = %+v, want epic_001 1/2", e)
	}
	if got := e.Progress(); got != 0.5 {
		t.Fatalf("derived progress = %v, want 0.5", got)
	}

	if len(snap.Tasks) != 2 {
		t.Fatalf("tasks = %d, want 2", len(snap.Tasks))
	}
	if snap.TasksOpen() != 1 {
		t.Fatalf("TasksOpen = %d, want 1", snap.TasksOpen())
	}
	if got := snap.TasksForEpic(e.ID); len(got) != 2 || got[0].IDStr != "task_001" {
		t.Fatalf("TasksForEpic = %+v", got)
	}

	if snap.Memories != 1 || len(snap.Mems) != 1 || snap.Mems[0].Kind != "decision" {
		t.Fatalf("memory = %d %+v", snap.Memories, snap.Mems)
	}
	if snap.Branch != "main" {
		t.Fatalf("branch = %q, want main", snap.Branch)
	}
}

func TestStoreSourceFallsBackToImportHubsWhenNoDepEdges(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "c.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Go-style imports resolve to package paths, not file ids, so dep_edges stays
	// empty. Three files import a shared internal package; one also imports stdlib.
	put := func(path string, imports []string) {
		err := db.WithTx(func(tx *store.Tx) error {
			_, e := tx.PutFile(store.FileRecord{Path: path, Language: "go"}, nil, imports, nil, nil)
			return e
		})
		if err != nil {
			t.Fatalf("PutFile %s: %v", path, err)
		}
	}
	const hub = "github.com/acme/proj/internal/store"
	put("a.go", []string{hub, "fmt"})
	put("b.go", []string{hub, "context"})
	put("c.go", []string{hub})

	snap, err := (&StoreSource{DB: db}).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(snap.Hubs) == 0 {
		t.Fatal("Hubs empty: import fallback did not populate the graph")
	}
	top := snap.Hubs[0]
	if top.Path != hub || top.In != 3 {
		t.Fatalf("top hub = %+v, want %s in-degree 3", top, hub)
	}
	// Stdlib specifiers (no dotted first segment) are excluded as graph noise.
	for _, h := range snap.Hubs {
		if h.Path == "fmt" || h.Path == "context" {
			t.Fatalf("stdlib specifier %q should be excluded from hubs", h.Path)
		}
	}
}

func TestStoreSourceDetailRendersEpicMarkdown(t *testing.T) {
	src := newSeededSource(t)

	md, err := src.Detail("epic", 1)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	for _, want := range []string{"# Indexing core", "## Tasks", "`task_001`", "## History"} {
		if !strings.Contains(md, want) {
			t.Fatalf("epic detail missing %q:\n%s", want, md)
		}
	}

	if md, err := src.Detail("epic", 999); err != nil || md != "" {
		t.Fatalf("unknown epic detail = %q, err=%v; want empty/no-error", md, err)
	}
}
