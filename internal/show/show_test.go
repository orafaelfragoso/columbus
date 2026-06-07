package show

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/orafaelfragoso/columbus/internal/clock"
	"github.com/orafaelfragoso/columbus/internal/config"
	"github.com/orafaelfragoso/columbus/internal/contract"
	"github.com/orafaelfragoso/columbus/internal/extract"
	"github.com/orafaelfragoso/columbus/internal/index"
	"github.com/orafaelfragoso/columbus/internal/store"
)

func buildShower(t *testing.T, files map[string]string) *Shower {
	t.Helper()
	work := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "t@e.com"}, {"config", "user.name", "T"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = work
		cmd.Run()
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
	return &Shower{DB: db, WorkDir: work, Registry: reg}
}

func assertCode(t *testing.T, err error, want contract.Code) {
	t.Helper()
	var ce *contract.Error
	if !errors.As(err, &ce) || ce.Code != want {
		t.Fatalf("err = %v, want code %s", err, want)
	}
}

func TestShowSymbolShowsAllMatchesWithLiveRange(t *testing.T) {
	s := buildShower(t, map[string]string{
		"a.go": "package a\n\nfunc Handler() {}\n",
		"b.go": "package b\n\nfunc Handler() {}\n",
	})
	res, err := s.Symbol("Handler", "", 0)
	if err != nil {
		t.Fatalf("Symbol: %v", err)
	}
	if res.Total != 2 || len(res.Blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(res.Blocks))
	}
	for _, b := range res.Blocks {
		if b.StartLine != 3 {
			t.Errorf("%s start line = %d, want 3", b.Path, b.StartLine)
		}
	}
}

func TestShowSymbolInFilter(t *testing.T) {
	s := buildShower(t, map[string]string{
		"a.go": "package a\n\nfunc Handler() {}\n",
		"b.go": "package b\n\nfunc Handler() {}\n",
	})
	res, err := s.Symbol("Handler", "a.go", 0)
	if err != nil {
		t.Fatalf("Symbol: %v", err)
	}
	if len(res.Blocks) != 1 || res.Blocks[0].Path != "a.go" {
		t.Errorf("expected only a.go, got %+v", res.Blocks)
	}
}

func TestShowSymbolNotFoundSuggests(t *testing.T) {
	s := buildShower(t, map[string]string{"a.go": "package a\n\nfunc Handler() {}\n"})
	_, err := s.Symbol("Handlerr", "", 0)
	assertCode(t, err, contract.CodeNotFound)
	var ce *contract.Error
	errors.As(err, &ce)
	if ce.Hint == "" {
		t.Error("expected a did-you-mean hint")
	}
}

func TestShowFileOutline(t *testing.T) {
	s := buildShower(t, map[string]string{
		"svc.go": "package svc\n\ntype S struct{}\n\nfunc (s *S) Do() {}\n\nfunc New() *S { return nil }\n",
	})
	res, err := s.File("svc.go", 3)
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	if len(res.Outline) < 3 {
		t.Errorf("outline = %d entries, want >= 3", len(res.Outline))
	}
	if res.Language != "go" {
		t.Errorf("language = %q, want go", res.Language)
	}
}

func TestShowFileNotFoundSuggests(t *testing.T) {
	s := buildShower(t, map[string]string{"internal/svc.go": "package svc\n"})
	_, err := s.File("svc.go", 3)
	assertCode(t, err, contract.CodeNotFound)
}

func TestShowMemoryNotFound(t *testing.T) {
	s := buildShower(t, map[string]string{"a.go": "package a\n"})
	_, err := s.Memory("mem_001")
	assertCode(t, err, contract.CodeNotFound)
}

func TestParseMemoryID(t *testing.T) {
	if n, err := ParseMemoryID("mem_042"); err != nil || n != 42 {
		t.Errorf("ParseMemoryID(mem_042) = %d, %v", n, err)
	}
	if _, err := ParseMemoryID("garbage"); err == nil {
		t.Error("expected error for garbage id")
	}
}
