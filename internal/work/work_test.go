package work

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/rafaelfragoso/columbus/internal/clock"
	"github.com/rafaelfragoso/columbus/internal/contract"
	"github.com/rafaelfragoso/columbus/internal/store"
)

func newManager(t *testing.T) *Manager {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "c.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &Manager{DB: db, Clock: clock.Fixed{T: time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)}}
}

func assertCode(t *testing.T, err error, want contract.Code) {
	t.Helper()
	var ce *contract.Error
	if !errors.As(err, &ce) || ce.Code != want {
		t.Fatalf("err = %v, want code %s", err, want)
	}
}

func TestEpicAddAssignsIDsStatusAndInitialEvent(t *testing.T) {
	m := newManager(t)
	r1, err := m.EpicAdd(EpicAddParams{Title: "first", Tags: []string{"a", "a"}})
	if err != nil {
		t.Fatal(err)
	}
	r2, _ := m.EpicAdd(EpicAddParams{Title: "second"})
	if r1.ID != "epic_001" || r2.ID != "epic_002" {
		t.Fatalf("ids = %s, %s", r1.ID, r2.ID)
	}
	if r1.Status != "todo" {
		t.Fatalf("status = %q, want todo", r1.Status)
	}
	if len(r1.Tags) != 1 {
		t.Fatalf("tags deduped = %v", r1.Tags)
	}
	events, _ := m.DB.WorkEvents("epic", 1)
	if len(events) != 1 || events[0].NewStatus != "todo" {
		t.Fatalf("initial event = %+v", events)
	}
}

func TestEpicAddRequiresTitle(t *testing.T) {
	m := newManager(t)
	_, err := m.EpicAdd(EpicAddParams{})
	assertCode(t, err, contract.CodeUsage)
}

func TestTaskAddRequiresExistingEpic(t *testing.T) {
	m := newManager(t)
	_, err := m.TaskAdd(TaskAddParams{Epic: "epic_999", Title: "x"})
	assertCode(t, err, contract.CodeNotFound)
}

func TestTaskAddUnderEpic(t *testing.T) {
	m := newManager(t)
	e, _ := m.EpicAdd(EpicAddParams{Title: "parent"})
	r, err := m.TaskAdd(TaskAddParams{Epic: e.ID, Title: "child"})
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != "task_001" || r.Epic != "epic_001" || r.Status != "todo" {
		t.Fatalf("task = %+v", r)
	}
}

func TestStatusAppendsEventAndDenormalizes(t *testing.T) {
	m := newManager(t)
	e, _ := m.EpicAdd(EpicAddParams{Title: "x"})
	r, err := m.EpicStatus(e.ID, "in_progress", "kicked off")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "in_progress" {
		t.Fatalf("status = %q", r.Status)
	}
	events, _ := m.DB.WorkEvents("epic", 1)
	if len(events) != 2 || events[1].NewStatus != "in_progress" || events[1].Comment != "kicked off" {
		t.Fatalf("events = %+v", events)
	}
}

func TestStatusRejectsUnknownValue(t *testing.T) {
	m := newManager(t)
	e, _ := m.EpicAdd(EpicAddParams{Title: "x"})
	_, err := m.EpicStatus(e.ID, "shipping", "")
	assertCode(t, err, contract.CodeUsage)
}

func TestStatusAnyToAnyAllowed(t *testing.T) {
	m := newManager(t)
	e, _ := m.EpicAdd(EpicAddParams{Title: "x"})
	if _, err := m.EpicStatus(e.ID, "done", ""); err != nil {
		t.Fatal(err)
	}
	// No transition order: done -> todo is permitted (data, not orchestration).
	if _, err := m.EpicStatus(e.ID, "todo", ""); err != nil {
		t.Fatalf("any->any must be allowed: %v", err)
	}
}

func TestCommentAppendsNoteWithoutStatus(t *testing.T) {
	m := newManager(t)
	e, _ := m.EpicAdd(EpicAddParams{Title: "x"})
	if _, err := m.EpicComment(e.ID, "a progress note"); err != nil {
		t.Fatal(err)
	}
	events, _ := m.DB.WorkEvents("epic", 1)
	last := events[len(events)-1]
	if last.NewStatus != "" || last.Comment != "a progress note" {
		t.Fatalf("comment event = %+v", last)
	}
}

func TestCommentRequiresText(t *testing.T) {
	m := newManager(t)
	e, _ := m.EpicAdd(EpicAddParams{Title: "x"})
	_, err := m.EpicComment(e.ID, "   ")
	assertCode(t, err, contract.CodeUsage)
}

func TestEpicEditUpdatesTitleAndTags(t *testing.T) {
	m := newManager(t)
	e, _ := m.EpicAdd(EpicAddParams{Title: "old", Tags: []string{"a"}})
	newTitle := "new"
	r, err := m.EpicEdit(e.ID, EpicEditParams{Title: &newTitle, AddTags: []string{"b"}, RemoveTags: []string{"a"}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Title != "new" || len(r.Tags) != 1 || r.Tags[0] != "b" {
		t.Fatalf("edited = %+v", r)
	}
}

func TestEpicEditRequiresAChange(t *testing.T) {
	m := newManager(t)
	e, _ := m.EpicAdd(EpicAddParams{Title: "x"})
	_, err := m.EpicEdit(e.ID, EpicEditParams{})
	assertCode(t, err, contract.CodeUsage)
}

func TestTaskReparent(t *testing.T) {
	m := newManager(t)
	a, _ := m.EpicAdd(EpicAddParams{Title: "A"})
	b, _ := m.EpicAdd(EpicAddParams{Title: "B"})
	ta, _ := m.TaskAdd(TaskAddParams{Epic: a.ID, Title: "movable"})
	newEpic := b.ID
	r, err := m.TaskEdit(ta.ID, TaskEditParams{Epic: &newEpic})
	if err != nil {
		t.Fatal(err)
	}
	if r.Epic != "epic_002" {
		t.Fatalf("reparented epic = %q", r.Epic)
	}
}

func TestTaskReparentToMissingEpicFails(t *testing.T) {
	m := newManager(t)
	a, _ := m.EpicAdd(EpicAddParams{Title: "A"})
	ta, _ := m.TaskAdd(TaskAddParams{Epic: a.ID, Title: "x"})
	missing := "epic_999"
	_, err := m.TaskEdit(ta.ID, TaskEditParams{Epic: &missing})
	assertCode(t, err, contract.CodeNotFound)
}

func TestEpicDeleteRequiresForceAndCascades(t *testing.T) {
	m := newManager(t)
	e, _ := m.EpicAdd(EpicAddParams{Title: "doomed"})
	ta, _ := m.TaskAdd(TaskAddParams{Epic: e.ID, Title: "child"})

	_, err := m.EpicDelete(e.ID, false)
	assertCode(t, err, contract.CodeUsage)

	if _, err := m.EpicDelete(e.ID, true); err != nil {
		t.Fatal(err)
	}
	tid, _ := ParseTaskID(ta.ID)
	if ok, _ := m.DB.TaskExists(tid); ok {
		t.Fatal("child task should be gone after epic cascade")
	}
}

func TestEpicListFilterByStatus(t *testing.T) {
	m := newManager(t)
	a, _ := m.EpicAdd(EpicAddParams{Title: "a"})
	m.EpicAdd(EpicAddParams{Title: "b"})
	m.EpicStatus(a.ID, "done", "")

	all, _ := m.EpicList("", "")
	if all.Total != 2 {
		t.Fatalf("total = %d", all.Total)
	}
	done, _ := m.EpicList("done", "")
	if done.Total != 1 || done.Epics[0].ID != "epic_001" {
		t.Fatalf("done filter = %+v", done)
	}
}

func TestEpicSearchableByTitleAndComment(t *testing.T) {
	m := newManager(t)
	e, _ := m.EpicAdd(EpicAddParams{Title: "Ship search", Tags: []string{"infra"}})
	if _, err := m.EpicComment(e.ID, "investigate the tokenizer"); err != nil {
		t.Fatal(err)
	}

	for _, q := range []string{`"search"*`, `"tokenizer"*`, `"infra"*`} {
		owners, err := m.DB.SearchWorkFTS(q, 10)
		if err != nil || len(owners) != 1 || owners[0].OwnerType != "epic" {
			t.Fatalf("SearchWorkFTS(%s) = %+v, %v", q, owners, err)
		}
	}
}

func TestParseIDs(t *testing.T) {
	if _, err := ParseEpicID("epic_007"); err != nil {
		t.Fatalf("ParseEpicID: %v", err)
	}
	if _, err := ParseTaskID("nope"); err == nil {
		t.Fatal("ParseTaskID should reject non-task id")
	}
}
