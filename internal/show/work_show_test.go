package show

import (
	"testing"

	"github.com/rafaelfragoso/columbus/internal/contract"
	"github.com/rafaelfragoso/columbus/internal/store"
)

func TestShowEpicDetail(t *testing.T) {
	s := buildShower(t, map[string]string{"a.go": "package a\n\nfunc F() {}\n"})

	var epicID, taskID int64
	err := s.DB.WithTx(func(tx *store.Tx) error {
		var e error
		if epicID, e = tx.NextEpicSeq(); e != nil {
			return e
		}
		if e = tx.InsertEpic(epicID, "Ship search", "body", "in_progress", "t0", "t1"); e != nil {
			return e
		}
		if e = tx.AddWorkTag("epic", epicID, "search"); e != nil {
			return e
		}
		if e = tx.AddWorkRef("epic", epicID, "file", "a.go"); e != nil {
			return e
		}
		if e = tx.AddWorkRef("epic", epicID, "file", "ghost.go"); e != nil {
			return e
		}
		if e = tx.AppendWorkEvent("epic", epicID, "todo", "", "t0"); e != nil {
			return e
		}
		if e = tx.AppendWorkEvent("epic", epicID, "in_progress", "started", "t1"); e != nil {
			return e
		}
		if taskID, e = tx.NextTaskSeq(); e != nil {
			return e
		}
		return tx.InsertTask(taskID, epicID, "child task", "", "blocked", "t0", "t0")
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	view, err := s.Epic("epic_001")
	if err != nil {
		t.Fatalf("Epic: %v", err)
	}
	if view.ID != "epic_001" || view.Status != "in_progress" || view.Title != "Ship search" {
		t.Fatalf("view = %+v", view)
	}
	if len(view.Tags) != 1 || len(view.Events) != 2 {
		t.Fatalf("tags/events = %+v / %+v", view.Tags, view.Events)
	}
	// Inline drift: a.go resolves, ghost.go does not.
	var resolved, drifted int
	for _, r := range view.Refs {
		if r.Resolved {
			resolved++
		} else {
			drifted++
		}
	}
	if resolved != 1 || drifted != 1 {
		t.Fatalf("refs drift = %+v", view.Refs)
	}
	if len(view.Tasks) != 1 || view.Tasks[0].ID != "task_001" || view.Tasks[0].Status != "blocked" {
		t.Fatalf("child tasks = %+v", view.Tasks)
	}
}

func TestShowTaskDetail(t *testing.T) {
	s := buildShower(t, map[string]string{"a.go": "package a\n"})
	err := s.DB.WithTx(func(tx *store.Tx) error {
		eid, e := tx.NextEpicSeq()
		if e != nil {
			return e
		}
		if e = tx.InsertEpic(eid, "parent", "", "todo", "t0", "t0"); e != nil {
			return e
		}
		tid, e := tx.NextTaskSeq()
		if e != nil {
			return e
		}
		if e = tx.InsertTask(tid, eid, "do the thing", "", "todo", "t0", "t0"); e != nil {
			return e
		}
		return tx.AppendWorkEvent("task", tid, "todo", "", "t0")
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	view, err := s.Task("task_001")
	if err != nil {
		t.Fatalf("Task: %v", err)
	}
	if view.ID != "task_001" || view.Epic != "epic_001" || len(view.Events) != 1 {
		t.Fatalf("view = %+v", view)
	}
}

func TestShowEpicNotFound(t *testing.T) {
	s := buildShower(t, map[string]string{"a.go": "package a\n"})
	_, err := s.Epic("epic_404")
	assertCode(t, err, contract.CodeNotFound)
}

func TestShowTaskNotFound(t *testing.T) {
	s := buildShower(t, map[string]string{"a.go": "package a\n"})
	_, err := s.Task("task_404")
	assertCode(t, err, contract.CodeNotFound)
}
