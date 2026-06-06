package store

import "testing"

func TestMigration0002BumpsSchema(t *testing.T) {
	db := openTemp(t)
	// 0002 adds the epic_seq/task_seq counters; their presence proves the
	// migration ran and is the floor for the epics/tasks feature.
	if _, err := db.SQL().Exec(`SELECT epic_seq, task_seq FROM index_meta WHERE id = 1`); err != nil {
		t.Fatalf("epic_seq/task_seq columns missing: %v", err)
	}
}

func TestEpicSeqIsMonotonic(t *testing.T) {
	db := openTemp(t)
	var seen []int64
	err := db.WithTx(func(tx *Tx) error {
		for range 3 {
			n, e := tx.NextEpicSeq()
			if e != nil {
				return e
			}
			seen = append(seen, n)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("NextEpicSeq: %v", err)
	}
	if seen[0] != 1 || seen[1] != 2 || seen[2] != 3 {
		t.Fatalf("epic seq = %v, want [1 2 3]", seen)
	}
}

func TestEpicRepoRoundTrip(t *testing.T) {
	db := openTemp(t)

	var id int64
	err := db.WithTx(func(tx *Tx) error {
		var e error
		if id, e = tx.NextEpicSeq(); e != nil {
			return e
		}
		if e = tx.InsertEpic(id, "Ship search", "the search epic", "todo", "t0", "t0"); e != nil {
			return e
		}
		return tx.AddWorkTag("epic", id, "search")
	})
	if err != nil {
		t.Fatalf("seed epic: %v", err)
	}

	full, ok, err := db.EpicFull(id)
	if err != nil || !ok {
		t.Fatalf("EpicFull ok=%v err=%v", ok, err)
	}
	if full.Title != "Ship search" || full.Status != "todo" || len(full.Tags) != 1 || full.Tags[0] != "search" {
		t.Fatalf("EpicFull = %+v", full)
	}

	list, err := db.ListEpics("todo", "search")
	if err != nil || len(list) != 1 || list[0].ID != id {
		t.Fatalf("ListEpics = %v, %v", list, err)
	}
	if got, _ := db.ListEpics("done", ""); len(got) != 0 {
		t.Fatalf("ListEpics(done) = %v, want empty", got)
	}

	ids, err := db.AllEpicIDs()
	if err != nil || len(ids) != 1 || ids[0] != id {
		t.Fatalf("AllEpicIDs = %v, %v", ids, err)
	}

	exists, err := db.EpicExists(id)
	if err != nil || !exists {
		t.Fatalf("EpicExists = %v, %v", exists, err)
	}

	if err := db.WithTx(func(tx *Tx) error {
		return tx.UpdateEpic(id, "Ship search v2", "updated body", "t1")
	}); err != nil {
		t.Fatalf("UpdateEpic: %v", err)
	}
	full, _, _ = db.EpicFull(id)
	if full.Title != "Ship search v2" || full.Body != "updated body" || full.UpdatedAt != "t1" {
		t.Fatalf("after UpdateEpic = %+v", full)
	}

	if err := db.WithTx(func(tx *Tx) error { return tx.DeleteEpic(id) }); err != nil {
		t.Fatalf("DeleteEpic: %v", err)
	}
	if ok, _ := db.EpicExists(id); ok {
		t.Fatal("epic should be gone after DeleteEpic")
	}
	if got, ok, _ := db.EpicFull(id); ok || got.ID != 0 {
		t.Fatalf("EpicFull after delete = %+v ok=%v, want zero/false", got, ok)
	}
}

func TestSetEpicSeqAtLeast(t *testing.T) {
	db := openTemp(t)
	if err := db.WithTx(func(tx *Tx) error { return tx.SetEpicSeqAtLeast(40) }); err != nil {
		t.Fatalf("SetEpicSeqAtLeast: %v", err)
	}
	var n int64
	if err := db.WithTx(func(tx *Tx) error {
		var e error
		n, e = tx.NextEpicSeq()
		return e
	}); err != nil {
		t.Fatalf("NextEpicSeq: %v", err)
	}
	if n != 41 {
		t.Fatalf("NextEpicSeq after SetEpicSeqAtLeast(40) = %d, want 41", n)
	}
}
