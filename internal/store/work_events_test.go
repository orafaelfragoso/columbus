package store

import "testing"

func TestWorkEventsAppendAndRead(t *testing.T) {
	db := openTemp(t)
	epicID := seedEpic(t, db, "tracked")

	err := db.WithTx(func(tx *Tx) error {
		if e := tx.AppendWorkEvent("epic", epicID, "todo", "", "t0"); e != nil {
			return e
		}
		if e := tx.AppendWorkEvent("epic", epicID, "in_progress", "started", "t1"); e != nil {
			return e
		}
		// comment-only note: status is NULL.
		return tx.AppendWorkEvent("epic", epicID, "", "a progress note", "t2")
	})
	if err != nil {
		t.Fatalf("append events: %v", err)
	}

	events, err := db.WorkEvents("epic", epicID)
	if err != nil {
		t.Fatalf("WorkEvents: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}
	// Chronological (id ascending).
	if events[0].NewStatus != "todo" || events[0].Comment != "" {
		t.Fatalf("event[0] = %+v", events[0])
	}
	if events[1].NewStatus != "in_progress" || events[1].Comment != "started" {
		t.Fatalf("event[1] = %+v", events[1])
	}
	if events[2].NewStatus != "" || events[2].Comment != "a progress note" {
		t.Fatalf("event[2] = %+v (status should be NULL/empty)", events[2])
	}
}

func TestSetStatusDenormalizesOntoRow(t *testing.T) {
	db := openTemp(t)
	epicID := seedEpic(t, db, "movable")
	taskEpic := seedEpic(t, db, "with-task")

	var taskID int64
	if err := db.WithTx(func(tx *Tx) error {
		var e error
		if taskID, e = tx.NextTaskSeq(); e != nil {
			return e
		}
		return tx.InsertTask(taskID, taskEpic, "child", "", "todo", "t0", "t0")
	}); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	if err := db.WithTx(func(tx *Tx) error {
		if e := tx.SetEpicStatus(epicID, "blocked", "t1"); e != nil {
			return e
		}
		return tx.SetTaskStatus(taskID, "done", "t1")
	}); err != nil {
		t.Fatalf("set status: %v", err)
	}

	epic, _, _ := db.EpicFull(epicID)
	if epic.Status != "blocked" {
		t.Fatalf("epic status = %q, want blocked", epic.Status)
	}
	task, _, _ := db.TaskFull(taskID)
	if task.Status != "done" {
		t.Fatalf("task status = %q, want done", task.Status)
	}
}
