package tui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/orafaelfragoso/columbus/internal/clock"
	"github.com/orafaelfragoso/columbus/internal/memory"
	"github.com/orafaelfragoso/columbus/internal/store"
)

// newSeededSource opens a fresh store, seeds one memory with an embedded
// vector, and returns a StoreSource over it.
func newSeededSource(t *testing.T) *StoreSource {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "c.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	clk := clock.Fixed{T: time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)}
	mm := &memory.Manager{DB: db, Clock: clk, WorkDir: t.TempDir()}

	if _, err := mm.Add(memory.AddParams{Kind: "adr", Title: "use WAL", Body: "single-writer journal", Tags: []string{"db"}}); err != nil {
		t.Fatalf("memory.Add: %v", err)
	}
	vec := make([]float32, 256)
	vec[0] = 1
	if err := db.UpsertVector("memory", 1, "test-model", "sha", vec); err != nil {
		t.Fatalf("UpsertVector: %v", err)
	}
	return &StoreSource{DB: db, Memory: mm, Branch: "main"}
}

func TestStoreSourceLoadMapsMemoryAndEmbeddings(t *testing.T) {
	snap, err := newSeededSource(t).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if snap.Memories != 1 || len(snap.Mems) != 1 || snap.Mems[0].Kind != "adr" {
		t.Fatalf("memory = %d %+v", snap.Memories, snap.Mems)
	}
	if snap.MemCounts["adr"] != 1 {
		t.Fatalf("MemCounts = %+v, want adr:1", snap.MemCounts)
	}
	if snap.Embeddings != 1 {
		t.Fatalf("Embeddings = %d, want 1", snap.Embeddings)
	}
	if snap.Branch != "main" {
		t.Fatalf("branch = %q, want main", snap.Branch)
	}
}

func TestStoreSourceDetailRendersMemoryMarkdown(t *testing.T) {
	src := newSeededSource(t)

	md, err := src.Detail("memory", 1)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	for _, want := range []string{"# use WAL", "ADR", "`mem_001`", "single-writer journal", "db"} {
		if !strings.Contains(md, want) {
			t.Fatalf("memory detail missing %q:\n%s", want, md)
		}
	}

	if md, err := src.Detail("memory", 999); err != nil || md != "" {
		t.Fatalf("unknown memory detail = %q, err=%v; want empty/no-error", md, err)
	}
	// Work kinds are gone: unknown kinds resolve to nothing, not an error.
	if md, err := src.Detail("epic", 1); err != nil || md != "" {
		t.Fatalf("epic detail = %q, err=%v; want empty/no-error", md, err)
	}
}
