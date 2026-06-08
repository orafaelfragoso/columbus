package store

import "testing"

// seedEpic creates an epic with a fresh id and returns it.
func seedEpic(t *testing.T, db *DB, title string) int64 {
	t.Helper()
	var id int64
	err := db.WithTx(func(tx *Tx) error {
		var e error
		if id, e = tx.NextEpicSeq(); e != nil {
			return e
		}
		return tx.InsertEpic(id, title, "", "todo", "t0", "t0")
	})
	if err != nil {
		t.Fatalf("seedEpic: %v", err)
	}
	return id
}

// seedStory creates a story under epicID with a fresh id and returns it.
func seedStory(t *testing.T, db *DB, epicID int64, title string) int64 {
	t.Helper()
	var id int64
	err := db.WithTx(func(tx *Tx) error {
		var e error
		if id, e = tx.NextStorySeq(); e != nil {
			return e
		}
		return tx.InsertStory(id, epicID, title, "", "todo", "t0", "t0")
	})
	if err != nil {
		t.Fatalf("seedStory: %v", err)
	}
	return id
}

func TestTaskRepoRoundTrip(t *testing.T) {
	db := openTemp(t)
	epicID := seedEpic(t, db, "parent epic")
	storyID := seedStory(t, db, epicID, "parent story")

	var taskID int64
	err := db.WithTx(func(tx *Tx) error {
		var e error
		if taskID, e = tx.NextTaskSeq(); e != nil {
			return e
		}
		if e = tx.InsertTask(taskID, epicID, storyID, "wire cli", "the cli task", "todo", "t0", "t0"); e != nil {
			return e
		}
		return tx.AddWorkTag("task", taskID, "cli")
	})
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}

	full, ok, err := db.TaskFull(taskID)
	if err != nil || !ok {
		t.Fatalf("TaskFull ok=%v err=%v", ok, err)
	}
	if full.EpicID != epicID || full.Title != "wire cli" || len(full.Tags) != 1 {
		t.Fatalf("TaskFull = %+v", full)
	}

	list, err := db.ListTasks(epicID, 0, "todo", "cli")
	if err != nil || len(list) != 1 || list[0].ID != taskID {
		t.Fatalf("ListTasks = %v, %v", list, err)
	}
	if got, _ := db.ListTasks(0, 0, "done", ""); len(got) != 0 {
		t.Fatalf("ListTasks(done) = %v, want empty", got)
	}

	exists, err := db.TaskExists(taskID)
	if err != nil || !exists {
		t.Fatalf("TaskExists = %v, %v", exists, err)
	}
	ids, err := db.AllTaskIDs()
	if err != nil || len(ids) != 1 || ids[0] != taskID {
		t.Fatalf("AllTaskIDs = %v, %v", ids, err)
	}
}

func TestSetTaskSeqAtLeast(t *testing.T) {
	db := openTemp(t)
	if err := db.WithTx(func(tx *Tx) error { return tx.SetTaskSeqAtLeast(7) }); err != nil {
		t.Fatalf("SetTaskSeqAtLeast: %v", err)
	}
	var n int64
	if err := db.WithTx(func(tx *Tx) error {
		var e error
		n, e = tx.NextTaskSeq()
		return e
	}); err != nil {
		t.Fatalf("NextTaskSeq: %v", err)
	}
	if n != 8 {
		t.Fatalf("NextTaskSeq after SetTaskSeqAtLeast(7) = %d, want 8", n)
	}
}

func TestInsertTaskRequiresExistingEpic(t *testing.T) {
	db := openTemp(t)
	// The NOT NULL FK with foreign_keys=on rejects a task whose epic is absent.
	err := db.WithTx(func(tx *Tx) error {
		return tx.InsertTask(1, 999, 1, "orphan", "", "todo", "t0", "t0")
	})
	if err == nil {
		t.Fatal("InsertTask with missing epic should fail the FK constraint")
	}
}

func TestReparentTask(t *testing.T) {
	db := openTemp(t)
	epicA := seedEpic(t, db, "epic A")
	epicB := seedEpic(t, db, "epic B")
	storyA := seedStory(t, db, epicA, "story A")
	storyB := seedStory(t, db, epicB, "story B")

	var taskID int64
	if err := db.WithTx(func(tx *Tx) error {
		var e error
		if taskID, e = tx.NextTaskSeq(); e != nil {
			return e
		}
		return tx.InsertTask(taskID, epicA, storyA, "movable", "", "todo", "t0", "t0")
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := db.WithTx(func(tx *Tx) error {
		return tx.ReparentTask(taskID, epicB, storyB, "t1")
	}); err != nil {
		t.Fatalf("ReparentTask: %v", err)
	}
	full, _, _ := db.TaskFull(taskID)
	if full.EpicID != epicB {
		t.Fatalf("after reparent epic_id = %d, want %d", full.EpicID, epicB)
	}
	if list, _ := db.ListTasks(epicA, 0, "", ""); len(list) != 0 {
		t.Fatalf("epic A still has tasks: %v", list)
	}
}

func TestDeleteEpicCascadesTasksAndAssociations(t *testing.T) {
	db := openTemp(t)
	epicID := seedEpic(t, db, "doomed")
	storyID := seedStory(t, db, epicID, "doomed story")

	var taskID int64
	if err := db.WithTx(func(tx *Tx) error {
		var e error
		if taskID, e = tx.NextTaskSeq(); e != nil {
			return e
		}
		if e = tx.InsertTask(taskID, epicID, storyID, "child", "", "todo", "t0", "t0"); e != nil {
			return e
		}
		return tx.AddWorkTag("task", taskID, "doomed-tag")
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := db.WithTx(func(tx *Tx) error { return tx.DeleteEpic(epicID) }); err != nil {
		t.Fatalf("DeleteEpic: %v", err)
	}
	if ok, _ := db.TaskExists(taskID); ok {
		t.Fatal("child task should be gone after epic cascade")
	}
	var tagCount int
	if err := db.SQL().QueryRow(`SELECT COUNT(*) FROM work_tags WHERE owner_type='task' AND owner_id=?`, taskID).Scan(&tagCount); err != nil {
		t.Fatalf("count tags: %v", err)
	}
	if tagCount != 0 {
		t.Fatalf("task tags survived cascade: %d", tagCount)
	}
}

func TestDeleteTask(t *testing.T) {
	db := openTemp(t)
	epicID := seedEpic(t, db, "keeper")
	storyID := seedStory(t, db, epicID, "keeper story")
	var taskID int64
	if err := db.WithTx(func(tx *Tx) error {
		var e error
		if taskID, e = tx.NextTaskSeq(); e != nil {
			return e
		}
		if e = tx.InsertTask(taskID, epicID, storyID, "removable", "", "todo", "t0", "t0"); e != nil {
			return e
		}
		return tx.AddWorkTag("task", taskID, "x")
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.WithTx(func(tx *Tx) error { return tx.DeleteTask(taskID) }); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if ok, _ := db.TaskExists(taskID); ok {
		t.Fatal("task should be gone after DeleteTask")
	}
	if ok, _ := db.EpicExists(epicID); !ok {
		t.Fatal("parent epic must survive a task delete")
	}
}
