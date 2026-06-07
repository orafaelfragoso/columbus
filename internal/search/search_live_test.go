package search

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/orafaelfragoso/columbus/internal/clock"
	"github.com/orafaelfragoso/columbus/internal/config"
	"github.com/orafaelfragoso/columbus/internal/extract"
	"github.com/orafaelfragoso/columbus/internal/grep"
	"github.com/orafaelfragoso/columbus/internal/index"
	"github.com/orafaelfragoso/columbus/internal/store"
)

// buildLiveEngine indexes a fixture and returns an engine with the live path on.
func buildLiveEngine(t *testing.T, files map[string]string) *Engine {
	t.Helper()
	work := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "t@e.com"}, {"config", "user.name", "T"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = work
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v\n%s", err, out)
		}
	}
	for rel, content := range files {
		full := filepath.Join(work, rel)
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	add := exec.Command("git", "add", "-A")
	add.Dir = work
	add.Run()

	reg, _ := extract.NewRegistry()
	db, err := store.Open(filepath.Join(t.TempDir(), "c.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ix := &index.Indexer{
		DB: db, Registry: reg, WorkDir: work,
		Clock:       clock.Fixed{T: time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)},
		MaxFileSize: config.DefaultMaxFileSize,
		Excludes:    config.Default().Indexing.Exclude,
	}
	if _, err := ix.Run(index.ModeFull); err != nil {
		t.Fatalf("index: %v", err)
	}
	return &Engine{DB: db, WorkDir: work, Registry: reg, Searcher: grep.New()}
}

func TestLiveResolvesLineRangeAndSnippet(t *testing.T) {
	e := buildLiveEngine(t, map[string]string{
		"svc.go": "package svc\n\n// header comment\nfunc Handler() string {\n\treturn \"ok\"\n}\n",
	})
	res, err := e.Search(Query{Text: "Handler", Kind: KindCode, Limit: 10, ContextLines: 0})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	var h *Hit
	for i := range res.Hits {
		if res.Hits[i].Name == "Handler" {
			h = &res.Hits[i]
		}
	}
	if h == nil {
		t.Fatal("Handler not found")
	}
	if h.StartLine != 4 {
		t.Errorf("StartLine = %d, want 4 (live-resolved)", h.StartLine)
	}
	if h.Snippet == "" || !strings.Contains(h.Snippet, "func Handler()") {
		t.Errorf("snippet missing body: %q", h.Snippet)
	}
}

func TestLiveContentMatchSurfacesSymbolWithoutNameMatch(t *testing.T) {
	// The query "frobnicate" appears only in a function BODY, not in any symbol
	// name. The metadata FTS won't match it; the live content path must surface
	// the enclosing function.
	e := buildLiveEngine(t, map[string]string{
		"work.go": "package work\n\nfunc Process() {\n\t// call frobnicate here\n\tx := 1\n}\n",
	})
	res, err := e.Search(Query{Text: "frobnicate", Kind: KindCode, Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	found := false
	for _, h := range res.Hits {
		if h.Name == "Process" {
			found = true
		}
	}
	if !found {
		t.Errorf("content match should surface Process, hits=%+v", res.Hits)
	}
}
