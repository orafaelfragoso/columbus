package tui

import "testing"

func TestEpicProgressIsDerivedFromTaskRollup(t *testing.T) {
	cases := []struct {
		done, total int
		want        float64
	}{
		{3, 6, 0.5},
		{0, 0, 0},
		{4, 4, 1},
		{0, 5, 0},
	}
	for _, c := range cases {
		if got := (EpicRow{Done: c.done, Total: c.total}).Progress(); got != c.want {
			t.Fatalf("Progress(%d/%d) = %v, want %v", c.done, c.total, got, c.want)
		}
	}
}

func TestActiveAndOpenCountsExcludeDoneAndCancelled(t *testing.T) {
	s := Snapshot{
		Epics: []EpicRow{
			{Status: "in_progress"}, {Status: "done"}, {Status: "cancelled"}, {Status: "todo"},
		},
		Tasks: []TaskRow{{Status: "todo"}, {Status: "done"}, {Status: "blocked"}},
	}
	if got := s.EpicsActive(); got != 2 {
		t.Fatalf("EpicsActive = %d, want 2", got)
	}
	if got := s.TasksOpen(); got != 2 {
		t.Fatalf("TasksOpen = %d, want 2", got)
	}
}

func TestTasksForEpicFiltersAndSortsByID(t *testing.T) {
	s := Snapshot{Tasks: []TaskRow{
		{ID: 2, EpicID: 1}, {ID: 1, EpicID: 1}, {ID: 3, EpicID: 2},
	}}
	got := s.TasksForEpic(1)
	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 2 {
		t.Fatalf("TasksForEpic(1) = %+v, want ids [1 2]", got)
	}
	if len(s.TasksForEpic(99)) != 0 {
		t.Fatal("TasksForEpic(99) should be empty")
	}
}
