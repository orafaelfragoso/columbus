package knowledge

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/orafaelfragoso/columbus/internal/clock"
	"github.com/orafaelfragoso/columbus/internal/contract"
	"github.com/orafaelfragoso/columbus/internal/store"
)

func newKnowledge(t *testing.T) *Manager {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "c.sqlite"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &Manager{DB: db, Clock: clock.Fixed{T: time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)}, WorkDir: t.TempDir()}
}

func assertCode(t *testing.T, err error, code contract.Code) {
	t.Helper()
	var ce *contract.Error
	if !errors.As(err, &ce) {
		t.Fatalf("want *contract.Error %s, got %v", code, err)
	}
	if ce.Code != code {
		t.Fatalf("code = %s, want %s", ce.Code, code)
	}
}

func TestAddWorkChain(t *testing.T) {
	m := newKnowledge(t)
	e, err := m.Add("epic", AddParams{Title: "Search"})
	if err != nil || e.Kind != "epic" || e.ID != "epic_001" || e.Status != "todo" {
		t.Fatalf("epic = %+v err=%v", e, err)
	}
	s, err := m.Add("story", AddParams{Parent: e.ID, Title: "Tokenizer"})
	if err != nil || s.Kind != "story" || s.Parent != "epic_001" {
		t.Fatalf("story = %+v err=%v", s, err)
	}
	ta, err := m.Add("task", AddParams{Parent: s.ID, Title: "Pad"})
	if err != nil || ta.Kind != "task" || ta.Parent != "story_001" {
		t.Fatalf("task = %+v err=%v", ta, err)
	}
}

func TestAddContextWithType(t *testing.T) {
	m := newKnowledge(t)
	c, err := m.Add("context", AddParams{Type: "decision", Title: "Use CLS pooling", Body: "token 0"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Kind != "context" || c.Type != "decision" || c.ID == "" {
		t.Fatalf("context = %+v", c)
	}
}

func TestUpdateTaskStatusAndComment(t *testing.T) {
	m := newKnowledge(t)
	e, _ := m.Add("epic", AddParams{Title: "E"})
	s, _ := m.Add("story", AddParams{Parent: e.ID, Title: "S"})
	ta, _ := m.Add("task", AddParams{Parent: s.ID, Title: "T"})

	got, err := m.Update("task", ta.ID, UpdateParams{Status: "in_progress", Comment: "started"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "in_progress" {
		t.Fatalf("status = %q, want in_progress", got.Status)
	}

	// An unknown status is rejected by the work engine.
	if _, err := m.Update("task", ta.ID, UpdateParams{Status: "shipping"}); err == nil {
		t.Fatal("unknown status should error")
	}
}

func TestUpdateEmptyIsUsage(t *testing.T) {
	m := newKnowledge(t)
	e, _ := m.Add("epic", AddParams{Title: "E"})
	_, err := m.Update("epic", e.ID, UpdateParams{})
	assertCode(t, err, contract.CodeUsage)
}

func TestRemoveRequiresForce(t *testing.T) {
	m := newKnowledge(t)
	e, _ := m.Add("epic", AddParams{Title: "E"})
	if _, err := m.Remove("epic", e.ID, false); err == nil {
		t.Fatal("remove without force should fail")
	}
	r, err := m.Remove("epic", e.ID, true)
	if err != nil || !r.Removed || r.Kind != "epic" {
		t.Fatalf("remove = %+v err=%v", r, err)
	}
}

func TestTagKindIsReadOnly(t *testing.T) {
	m := newKnowledge(t)
	if _, err := m.Add("tag", AddParams{Title: "x"}); err == nil {
		t.Fatal("adding a tag should be rejected")
	}
	assertCodeFromList(t, m)
}

func assertCodeFromList(t *testing.T, m *Manager) {
	t.Helper()
	// list tag aggregates distinct tags with counts.
	m.Add("epic", AddParams{Title: "E", Tags: []string{"alpha", "beta"}})
	m.Add("context", AddParams{Type: "decision", Title: "C", Tags: []string{"alpha"}})
	p, err := m.List("tag", ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	tl, ok := p.(TagListResult)
	if !ok {
		t.Fatalf("list tag returned %T, want TagListResult", p)
	}
	if tl.Total != 2 {
		t.Fatalf("distinct tags = %d, want 2", tl.Total)
	}
	counts := map[string]int{}
	for _, tc := range tl.Tags {
		counts[tc.Tag] = tc.Count
	}
	if counts["alpha"] != 2 || counts["beta"] != 1 {
		t.Fatalf("tag counts = %+v", counts)
	}
}

func TestListUnknownKind(t *testing.T) {
	m := newKnowledge(t)
	_, err := m.List("widget", ListFilter{})
	assertCode(t, err, contract.CodeInvalidKind)
}

func TestListWorkAndContext(t *testing.T) {
	m := newKnowledge(t)
	e, _ := m.Add("epic", AddParams{Title: "E"})
	m.Add("story", AddParams{Parent: e.ID, Title: "S1"})
	m.Add("story", AddParams{Parent: e.ID, Title: "S2"})
	m.Add("context", AddParams{Type: "pattern", Title: "P"})

	p, err := m.List("story", ListFilter{Parent: e.ID})
	if err != nil {
		t.Fatal(err)
	}
	lr := p.(ListResult)
	if lr.Total != 2 || lr.Kind != "story" {
		t.Fatalf("story list = %+v", lr)
	}

	p, _ = m.List("context", ListFilter{})
	if p.(ListResult).Total != 1 {
		t.Fatalf("context list total = %d, want 1", p.(ListResult).Total)
	}
}
