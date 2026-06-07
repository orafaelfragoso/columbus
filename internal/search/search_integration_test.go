package search

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/orafaelfragoso/columbus/internal/clock"
	"github.com/orafaelfragoso/columbus/internal/config"
	"github.com/orafaelfragoso/columbus/internal/extract"
	"github.com/orafaelfragoso/columbus/internal/index"
	"github.com/orafaelfragoso/columbus/internal/store"
)

// buildIndexedStore creates a git repo, indexes it, and returns an open store.
func buildIndexedStore(t *testing.T) *store.DB {
	t.Helper()
	work := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "t@e.com"}, {"config", "user.name", "T"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = work
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v\n%s", err, out)
		}
	}
	files := map[string]string{
		"server.go":      "package srv\n\n// Server handles requests.\ntype Server struct{}\n\nfunc NewServer() *Server { return &Server{} }\n",
		"server_test.go": "package srv\n\nfunc TestServer(t *testing.T) {}\n",
		"lib.ts":         "export function helper() {}\n",
		"client.ts":      "import { helper } from './lib';\n\nexport function connect() {}\n",
	}
	for rel, content := range files {
		if err := os.WriteFile(filepath.Join(work, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = work
	cmd.Run()
	cmd = exec.Command("git", "commit", "-q", "-m", "x")
	cmd.Dir = work
	cmd.Run()

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
	return db
}

func TestSearchFindsSymbolByName(t *testing.T) {
	db := buildIndexedStore(t)
	e := &Engine{DB: db}
	res, err := e.Search(Query{Text: "NewServer", Kind: KindCode, Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.Total == 0 {
		t.Fatal("expected results for NewServer")
	}
	top := res.Hits[0]
	if top.Name != "NewServer" {
		t.Errorf("top hit = %q, want NewServer", top.Name)
	}
	if top.Score <= 0 {
		t.Errorf("score = %v, want > 0", top.Score)
	}
}

func TestSearchRanksImplOverTest(t *testing.T) {
	db := buildIndexedStore(t)
	e := &Engine{DB: db}
	res, err := e.Search(Query{Text: "Server", Kind: KindCode, Limit: 20})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// The Server type (impl) should rank above the TestServer function (test).
	var implRank, testRank = -1, -1
	for i, h := range res.Hits {
		if h.Name == "Server" && implRank == -1 {
			implRank = i
		}
		if h.Name == "TestServer" {
			testRank = i
		}
	}
	if implRank == -1 {
		t.Fatal("Server symbol not found")
	}
	if testRank != -1 && implRank > testRank {
		t.Errorf("impl Server (rank %d) should outrank TestServer (rank %d)", implRank, testRank)
	}
}

func TestSearchGraphEnrichment(t *testing.T) {
	db := buildIndexedStore(t)
	e := &Engine{DB: db}
	res, err := e.Search(Query{Text: "lib", Kind: KindCode, Limit: 20, Graph: true})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// lib.ts is imported by client.ts (relative import) -> imported_by edge.
	found := false
	for _, h := range res.Hits {
		if h.Path == "lib.ts" {
			for _, ib := range h.Graph.ImportedBy {
				if ib == "client.ts" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Errorf("expected lib.ts imported_by client.ts in graph enrichment, hits=%+v", res.Hits)
	}
}

func TestSearchEmptyQueryIsUsageError(t *testing.T) {
	db := buildIndexedStore(t)
	e := &Engine{DB: db}
	_, err := e.Search(Query{Text: "   ", Kind: KindCode})
	if err == nil {
		t.Fatal("expected usage error for empty query")
	}
}

func TestSearchIndexMissing(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "empty.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	e := &Engine{DB: db}
	_, err = e.Search(Query{Text: "anything", Kind: KindCode})
	if err == nil {
		t.Fatal("expected INDEX_MISSING")
	}
}
