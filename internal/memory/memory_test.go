package memory

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/rafaelfragoso/columbus/internal/clock"
	"github.com/rafaelfragoso/columbus/internal/config"
	"github.com/rafaelfragoso/columbus/internal/contract"
	"github.com/rafaelfragoso/columbus/internal/extract"
	"github.com/rafaelfragoso/columbus/internal/index"
	"github.com/rafaelfragoso/columbus/internal/store"
)

func newManager(t *testing.T, files map[string]string) (*Manager, string) {
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
		os.WriteFile(full, []byte(content), 0o644)
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
	ix.Run(index.ModeFull)
	return &Manager{DB: db, Clock: clock.Fixed{T: time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)}, WorkDir: work}, work
}

func assertCode(t *testing.T, err error, want contract.Code) {
	t.Helper()
	var ce *contract.Error
	if !errors.As(err, &ce) || ce.Code != want {
		t.Fatalf("err = %v, want code %s", err, want)
	}
}

func TestAddAssignsMonotonicIDs(t *testing.T) {
	m, _ := newManager(t, map[string]string{"a.go": "package a\n"})
	r1, err := m.Add(AddParams{Kind: "decision", Title: "first"})
	if err != nil {
		t.Fatal(err)
	}
	r2, _ := m.Add(AddParams{Kind: "pattern", Title: "second"})
	if r1.ID != "mem_001" || r2.ID != "mem_002" {
		t.Errorf("ids = %s, %s; want mem_001, mem_002", r1.ID, r2.ID)
	}
}

func TestAddRejectsUnknownKind(t *testing.T) {
	m, _ := newManager(t, nil)
	_, err := m.Add(AddParams{Kind: "nonsense", Title: "x"})
	assertCode(t, err, contract.CodeInvalidKind)
}

func TestAddRequiresTitle(t *testing.T) {
	m, _ := newManager(t, nil)
	_, err := m.Add(AddParams{Kind: "decision"})
	assertCode(t, err, contract.CodeUsage)
}

func TestRemovedIDsAreNotReused(t *testing.T) {
	m, _ := newManager(t, nil)
	r1, _ := m.Add(AddParams{Kind: "decision", Title: "first"})
	if _, err := m.Remove(r1.ID); err != nil {
		t.Fatal(err)
	}
	r2, _ := m.Add(AddParams{Kind: "decision", Title: "second"})
	if r2.ID == r1.ID {
		t.Errorf("id %s was reused after deletion", r2.ID)
	}
	if r2.ID != "mem_002" {
		t.Errorf("id = %s, want mem_002", r2.ID)
	}
}

func TestUnresolvedLinkIsWarningNotError(t *testing.T) {
	m, _ := newManager(t, map[string]string{"a.go": "package a\n"})
	r, err := m.Add(AddParams{Kind: "decision", Title: "x", Links: []LinkSpec{{Type: "file", Ref: "does/not/exist.go"}}})
	if err != nil {
		t.Fatalf("unresolved link must not error: %v", err)
	}
	if len(r.Warnings) == 0 {
		t.Error("expected a warning for unresolved link")
	}
	if len(r.Links) != 1 {
		t.Error("link should still be stored")
	}
}

func TestEditRequiresAChange(t *testing.T) {
	m, _ := newManager(t, nil)
	r, _ := m.Add(AddParams{Kind: "decision", Title: "x"})
	_, err := m.Edit(r.ID, EditParams{})
	assertCode(t, err, contract.CodeUsage)
}

func TestEditUpdatesTitleAndTags(t *testing.T) {
	m, _ := newManager(t, nil)
	r, _ := m.Add(AddParams{Kind: "decision", Title: "old", Tags: []string{"a"}})
	newTitle := "new"
	updated, err := m.Edit(r.ID, EditParams{Title: &newTitle, AddTags: []string{"b"}, RemoveTags: []string{"a"}})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "new" {
		t.Errorf("title = %q", updated.Title)
	}
	if len(updated.Tags) != 1 || updated.Tags[0] != "b" {
		t.Errorf("tags = %v, want [b]", updated.Tags)
	}
}

func TestListFilterAndCounts(t *testing.T) {
	m, _ := newManager(t, nil)
	m.Add(AddParams{Kind: "decision", Title: "d1"})
	m.Add(AddParams{Kind: "decision", Title: "d2"})
	m.Add(AddParams{Kind: "failure", Title: "f1"})

	all, _ := m.List("", "")
	if all.Total != 3 || all.Counts["decision"] != 2 || all.Counts["failure"] != 1 {
		t.Errorf("counts = %+v", all.Counts)
	}
	onlyFail, _ := m.List("failure", "")
	if onlyFail.Total != 1 {
		t.Errorf("failure filter total = %d", onlyFail.Total)
	}
}

func TestSearchFindsByBody(t *testing.T) {
	m, _ := newManager(t, nil)
	m.Add(AddParams{Kind: "decision", Title: "Use WAL", Body: "sqlite journal mode rationale"})
	res, err := m.Search("journal", 10)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 || res.Memories[0].Title != "Use WAL" {
		t.Errorf("search = %+v", res)
	}
}

func TestValidateDetectsDriftAndBroken(t *testing.T) {
	m, work := newManager(t, map[string]string{"a.go": "package a\n\nfunc F() {}\n"})
	// evidence on a real file (ok), and on a missing file (broken).
	_, err := m.Add(AddParams{Kind: "decision", Title: "x",
		Evidence: []EvidenceSpec{{Path: "a.go", Start: 1, End: 1}, {Path: "ghost.go", Start: 1, End: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	// Mutate a.go so its blob oid changes -> stale.
	os.WriteFile(filepath.Join(work, "a.go"), []byte("package a\n\nfunc F() { _ = 1 }\n"), 0o644)

	res, err := m.Validate()
	if err != nil {
		t.Fatal(err)
	}
	if res.Drifted < 2 {
		t.Errorf("expected >=2 drifted (stale a.go + broken ghost.go), got %d", res.Drifted)
	}
	if res.Healthy {
		t.Error("should not be healthy with drift")
	}
}

func TestParseEvidence(t *testing.T) {
	ev, err := ParseEvidence("internal/svc.go:10-20")
	if err != nil || ev.Path != "internal/svc.go" || ev.Start != 10 || ev.End != 20 {
		t.Errorf("parsed = %+v, err=%v", ev, err)
	}
	if _, err := ParseEvidence("nopath"); err == nil {
		t.Error("expected error for missing range")
	}
}

func TestParseLink(t *testing.T) {
	l, err := ParseLink("symbol:Server")
	if err != nil || l.Type != "symbol" || l.Ref != "Server" {
		t.Errorf("parsed = %+v, err=%v", l, err)
	}
	if _, err := ParseLink("bogus:x"); err == nil {
		t.Error("expected error for bad link type")
	}
}
