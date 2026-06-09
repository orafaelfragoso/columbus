package index

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
	"github.com/orafaelfragoso/columbus/internal/store"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newIndexer builds an Indexer over a fresh git repo seeded with a few files.
func newIndexer(t *testing.T) (*Indexer, string) {
	t.Helper()
	work := t.TempDir()
	git(t, work, "init", "-q")
	git(t, work, "config", "user.email", "t@e.com")
	git(t, work, "config", "user.name", "T")

	write(t, work, "svc.go", "package svc\n\nfunc New() int { return 1 }\n\ntype Server struct{}\n")
	write(t, work, "util.py", "def helper():\n    pass\n")
	write(t, work, "README.md", "# Title\n")
	git(t, work, "add", "-A")
	git(t, work, "commit", "-q", "-m", "init")

	reg, err := extract.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(t.TempDir(), "columbus.sqlite")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	return &Indexer{
		DB:          db,
		Registry:    reg,
		WorkDir:     work,
		Clock:       clock.Fixed{T: time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)},
		MaxFileSize: config.DefaultMaxFileSize,
		Excludes:    config.Default().Indexing.Exclude,
	}, work
}

func TestIndexInitialFull(t *testing.T) {
	ix, _ := newIndexer(t)
	res, err := ix.Run(ModeFull)
	if err != nil {
		t.Fatalf("Run full: %v", err)
	}
	if res.Indexed != 3 {
		t.Errorf("indexed = %d, want 3", res.Indexed)
	}
	if res.Symbols < 3 {
		t.Errorf("symbols = %d, want >= 3 (New, Server, helper, Title)", res.Symbols)
	}
	if res.TotalFiles != 3 {
		t.Errorf("total files = %d, want 3", res.TotalFiles)
	}
}

func TestIndexIncrementalNoChange(t *testing.T) {
	ix, _ := newIndexer(t)
	if _, err := ix.Run(ModeIncremental); err != nil {
		t.Fatal(err)
	}
	res, err := ix.Run(ModeIncremental)
	if err != nil {
		t.Fatal(err)
	}
	if res.Indexed != 0 {
		t.Errorf("second run indexed = %d, want 0", res.Indexed)
	}
	if res.Unchanged != 3 {
		t.Errorf("unchanged = %d, want 3", res.Unchanged)
	}
}

func TestIndexIncrementalModify(t *testing.T) {
	ix, work := newIndexer(t)
	if _, err := ix.Run(ModeIncremental); err != nil {
		t.Fatal(err)
	}
	write(t, work, "svc.go", "package svc\n\nfunc New() int { return 2 }\n\nfunc Extra() {}\n")
	res, err := ix.Run(ModeIncremental)
	if err != nil {
		t.Fatal(err)
	}
	if res.Indexed != 1 {
		t.Errorf("indexed = %d, want 1", res.Indexed)
	}
	if res.Unchanged != 2 {
		t.Errorf("unchanged = %d, want 2", res.Unchanged)
	}
}

func TestIndexIncrementalAddUntracked(t *testing.T) {
	ix, work := newIndexer(t)
	if _, err := ix.Run(ModeIncremental); err != nil {
		t.Fatal(err)
	}
	write(t, work, "extra.ts", "export const x = 1;\n")
	res, err := ix.Run(ModeIncremental)
	if err != nil {
		t.Fatal(err)
	}
	if res.Indexed != 1 {
		t.Errorf("indexed = %d, want 1 (new untracked file)", res.Indexed)
	}
	if res.TotalFiles != 4 {
		t.Errorf("total = %d, want 4", res.TotalFiles)
	}
}

func TestIndexIncrementalDelete(t *testing.T) {
	ix, work := newIndexer(t)
	if _, err := ix.Run(ModeIncremental); err != nil {
		t.Fatal(err)
	}
	git(t, work, "rm", "-q", "util.py")
	res, err := ix.Run(ModeIncremental)
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 1 {
		t.Errorf("deleted = %d, want 1", res.Deleted)
	}
	if res.TotalFiles != 2 {
		t.Errorf("total = %d, want 2", res.TotalFiles)
	}
}

func TestIndexClean(t *testing.T) {
	ix, _ := newIndexer(t)
	if _, err := ix.Run(ModeFull); err != nil {
		t.Fatal(err)
	}
	res, err := ix.Run(ModeClean)
	if err != nil {
		t.Fatal(err)
	}
	if res.TotalFiles != 0 {
		t.Errorf("total after clean = %d, want 0", res.TotalFiles)
	}
	hashes, _ := ix.DB.FileHashes()
	if len(hashes) != 0 {
		t.Errorf("expected empty index after clean, got %d", len(hashes))
	}
}

func TestIndexStatusDoesNotWrite(t *testing.T) {
	ix, _ := newIndexer(t)
	res, err := ix.Run(ModeStatus)
	if err != nil {
		t.Fatal(err)
	}
	if !res.StatusOnly {
		t.Error("StatusOnly should be true")
	}
	if res.Indexed != 3 {
		t.Errorf("would-index = %d, want 3", res.Indexed)
	}
	// No write happened: the index is still empty.
	hashes, _ := ix.DB.FileHashes()
	if len(hashes) != 0 {
		t.Errorf("status must not write, found %d files", len(hashes))
	}
}

func TestIndexNonCodeFilesEmbedContent(t *testing.T) {
	ix, work := newIndexer(t)
	fe := &fakeEmbedder{}
	ix.Embedder = fe

	// A non-code text file whose content must reach the embedder...
	write(t, work, "deploy.yml", "steps:\n  - run: magic_pipeline_token_xyz\n")
	// ...a generated/lock file whose content must NOT...
	write(t, work, "go.sum", "example.com/m v1.0.0/go.mod h1:lockhashmustnotembed=\n")
	// ...and a binary blob that is skipped entirely.
	if err := os.WriteFile(filepath.Join(work, "blob.bin"), []byte{0, 1, 2, 0, 3}, 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := ix.Run(ModeFull)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.SkippedBinary != 1 {
		t.Errorf("skipped binary = %d, want 1", res.SkippedBinary)
	}

	seen := strings.Join(fe.seen, "\n")
	if !strings.Contains(seen, "magic_pipeline_token_xyz") {
		t.Error("non-code file content was not embedded")
	}
	if strings.Contains(seen, "lockhashmustnotembed") {
		t.Error("generated/lock file content must not be embedded")
	}
	// The lockfile still indexes as a file row (path-only embed); the binary does not.
	if got := vecCount(t, ix, fileOwner); got != res.TotalFiles {
		t.Errorf("file vectors = %d, want %d (one per indexed file)", got, res.TotalFiles)
	}
}

func TestIndexNonGit(t *testing.T) {
	work := t.TempDir()
	write(t, work, "main.go", "package main\n\nfunc main() {}\n")
	write(t, work, "node_modules/dep/index.js", "module.exports = 1;\n") // excluded
	reg, _ := extract.NewRegistry()
	db, _ := store.Open(filepath.Join(t.TempDir(), "c.sqlite"))
	t.Cleanup(func() { db.Close() })
	ix := &Indexer{
		DB: db, Registry: reg, WorkDir: work,
		Clock:       clock.Fixed{T: time.Now()},
		MaxFileSize: config.DefaultMaxFileSize,
		Excludes:    config.Default().Indexing.Exclude,
	}
	res, err := ix.Run(ModeFull)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Indexed != 1 {
		t.Errorf("indexed = %d, want 1 (node_modules excluded)", res.Indexed)
	}
}
