package work

import (
	"testing"

	"github.com/orafaelfragoso/columbus/internal/contract"
)

func TestStoryAddRequiresExistingEpic(t *testing.T) {
	m := newManager(t)
	_, err := m.StoryAdd(StoryAddParams{Epic: "epic_999", Title: "x"})
	assertCode(t, err, contract.CodeNotFound)
}

func TestStoryAddRequiresTitle(t *testing.T) {
	m := newManager(t)
	e, _ := m.EpicAdd(EpicAddParams{Title: "parent"})
	_, err := m.StoryAdd(StoryAddParams{Epic: e.ID})
	assertCode(t, err, contract.CodeUsage)
}

func TestStoryAddUnderEpic(t *testing.T) {
	m := newManager(t)
	e, _ := m.EpicAdd(EpicAddParams{Title: "parent"})
	r, err := m.StoryAdd(StoryAddParams{Epic: e.ID, Title: "story"})
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != "story_001" || r.Epic != "epic_001" || r.Status != "todo" {
		t.Fatalf("story = %+v", r)
	}
}

// The full epic > story > task chain is enforced: a task needs a story, and a
// story needs an epic.
func TestEpicStoryTaskChainEnforced(t *testing.T) {
	m := newManager(t)
	e, _ := m.EpicAdd(EpicAddParams{Title: "E"})
	s, _ := m.StoryAdd(StoryAddParams{Epic: e.ID, Title: "S"})
	ta, err := m.TaskAdd(TaskAddParams{Story: s.ID, Title: "T"})
	if err != nil {
		t.Fatal(err)
	}
	if ta.Story != "story_001" || ta.Epic != "epic_001" {
		t.Fatalf("task chain = %+v", ta)
	}
	// A task without a valid story is rejected.
	if _, err := m.TaskAdd(TaskAddParams{Story: "story_999", Title: "orphan"}); err == nil {
		t.Fatal("task under missing story should fail")
	}
}

func TestStoryListFilterByEpic(t *testing.T) {
	m := newManager(t)
	a, _ := m.EpicAdd(EpicAddParams{Title: "A"})
	b, _ := m.EpicAdd(EpicAddParams{Title: "B"})
	m.StoryAdd(StoryAddParams{Epic: a.ID, Title: "a1"})
	m.StoryAdd(StoryAddParams{Epic: a.ID, Title: "a2"})
	m.StoryAdd(StoryAddParams{Epic: b.ID, Title: "b1"})

	all, _ := m.StoryList("", "", "")
	if all.Total != 3 {
		t.Fatalf("all stories = %d, want 3", all.Total)
	}
	underA, _ := m.StoryList(a.ID, "", "")
	if underA.Total != 2 {
		t.Fatalf("stories under A = %d, want 2", underA.Total)
	}
	for _, s := range underA.Stories {
		if s.Epic != "epic_001" {
			t.Errorf("story %s epic = %s, want epic_001", s.ID, s.Epic)
		}
	}
}

// Deleting an epic cascades its stories and their tasks.
func TestEpicDeleteCascadesStoriesAndTasks(t *testing.T) {
	m := newManager(t)
	e, _ := m.EpicAdd(EpicAddParams{Title: "doomed"})
	s, _ := m.StoryAdd(StoryAddParams{Epic: e.ID, Title: "story"})
	ta, _ := m.TaskAdd(TaskAddParams{Story: s.ID, Title: "child"})

	if _, err := m.EpicDelete(e.ID, true); err != nil {
		t.Fatal(err)
	}
	sid, _ := ParseStoryID(s.ID)
	if ok, _ := m.DB.StoryExists(sid); ok {
		t.Error("story should be gone after epic cascade")
	}
	tid, _ := ParseTaskID(ta.ID)
	if ok, _ := m.DB.TaskExists(tid); ok {
		t.Error("task should be gone after epic cascade")
	}
}

// Deleting a story cascades its tasks but leaves the parent epic.
func TestStoryDeleteCascadesTasksKeepsEpic(t *testing.T) {
	m := newManager(t)
	e, _ := m.EpicAdd(EpicAddParams{Title: "keeper"})
	s, _ := m.StoryAdd(StoryAddParams{Epic: e.ID, Title: "story"})
	ta, _ := m.TaskAdd(TaskAddParams{Story: s.ID, Title: "child"})

	if _, err := m.StoryDelete(s.ID, true); err != nil {
		t.Fatal(err)
	}
	tid, _ := ParseTaskID(ta.ID)
	if ok, _ := m.DB.TaskExists(tid); ok {
		t.Error("task should be gone after story cascade")
	}
	eid, _ := ParseEpicID(e.ID)
	if ok, _ := m.DB.EpicExists(eid); !ok {
		t.Error("parent epic must survive a story delete")
	}
}
