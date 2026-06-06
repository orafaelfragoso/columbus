package store

import "testing"

func TestWorkRefsRoundTrip(t *testing.T) {
	db := openTemp(t)
	epicID := seedEpic(t, db, "with refs")

	err := db.WithTx(func(tx *Tx) error {
		if e := tx.AddWorkRef("epic", epicID, "file", "internal/a.go"); e != nil {
			return e
		}
		if e := tx.AddWorkRef("epic", epicID, "symbol", "Foo"); e != nil {
			return e
		}
		// idempotent: a duplicate is ignored.
		return tx.AddWorkRef("epic", epicID, "file", "internal/a.go")
	})
	if err != nil {
		t.Fatalf("add refs: %v", err)
	}

	full, _, _ := db.EpicFull(epicID)
	if len(full.Refs) != 2 {
		t.Fatalf("refs = %+v, want 2 (deduped)", full.Refs)
	}

	if err := db.WithTx(func(tx *Tx) error {
		return tx.RemoveWorkRef("epic", epicID, "symbol", "Foo")
	}); err != nil {
		t.Fatalf("remove ref: %v", err)
	}
	full, _, _ = db.EpicFull(epicID)
	if len(full.Refs) != 1 || full.Refs[0].TargetType != "file" {
		t.Fatalf("after remove = %+v", full.Refs)
	}
}

func TestWorkForTarget(t *testing.T) {
	db := openTemp(t)
	epicID := seedEpic(t, db, "epic ref")

	var taskID int64
	err := db.WithTx(func(tx *Tx) error {
		var e error
		if taskID, e = tx.NextTaskSeq(); e != nil {
			return e
		}
		if e = tx.InsertTask(taskID, epicID, "task ref", "", "in_progress", "t0", "t0"); e != nil {
			return e
		}
		if e = tx.AddWorkRef("epic", epicID, "file", "internal/a.go"); e != nil {
			return e
		}
		if e = tx.AddWorkRef("task", taskID, "file", "internal/a.go"); e != nil {
			return e
		}
		// A different target must not show up.
		return tx.AddWorkRef("task", taskID, "symbol", "Foo")
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	owners, err := db.WorkForTarget("file", "internal/a.go")
	if err != nil {
		t.Fatalf("WorkForTarget: %v", err)
	}
	if len(owners) != 2 {
		t.Fatalf("owners = %+v, want 2", owners)
	}
	// Ordered epic before task.
	if owners[0].OwnerType != "epic" || owners[0].Title != "epic ref" {
		t.Fatalf("owners[0] = %+v", owners[0])
	}
	if owners[1].OwnerType != "task" || owners[1].Status != "in_progress" {
		t.Fatalf("owners[1] = %+v", owners[1])
	}
}

func TestHasFilesUnderDir(t *testing.T) {
	db := openTemp(t)
	if err := db.WithTx(func(tx *Tx) error {
		_, e := tx.PutFile(FileRecord{Path: "internal/store/db.go", Language: "go", Role: "impl", BlobOID: "x"}, nil, nil, nil, nil)
		return e
	}); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	for _, dir := range []string{"internal", "internal/store"} {
		ok, err := db.HasFilesUnderDir(dir)
		if err != nil || !ok {
			t.Fatalf("HasFilesUnderDir(%q) = %v, %v; want true", dir, ok, err)
		}
	}
	// A trailing slash is tolerated.
	if ok, _ := db.HasFilesUnderDir("internal/"); !ok {
		t.Fatal("HasFilesUnderDir tolerates a trailing slash")
	}
	// A non-existent / partial-segment directory does not match.
	if ok, _ := db.HasFilesUnderDir("internal/sto"); ok {
		t.Fatal("HasFilesUnderDir must match whole path segments, not prefixes")
	}
	if ok, _ := db.HasFilesUnderDir("cmd"); ok {
		t.Fatal("HasFilesUnderDir(cmd) should be false")
	}
}

func TestWorkFTSRoundTrip(t *testing.T) {
	db := openTemp(t)
	epicID := seedEpic(t, db, "Ship search")
	if err := db.WithTx(func(tx *Tx) error {
		return tx.ReindexWorkFTS("epic", epicID, "Ship search", "body text", "fts", "a useful comment")
	}); err != nil {
		t.Fatalf("ReindexWorkFTS: %v", err)
	}

	owners, err := db.SearchWorkFTS(`"search"*`, 10)
	if err != nil || len(owners) != 1 || owners[0].OwnerType != "epic" || owners[0].OwnerID != epicID {
		t.Fatalf("SearchWorkFTS(title) = %+v, %v", owners, err)
	}
	// Comments feed the index too.
	if owners, _ := db.SearchWorkFTS(`"comment"*`, 10); len(owners) != 1 {
		t.Fatalf("SearchWorkFTS(comment) = %+v, want 1", owners)
	}
	// Re-deleting via reindex removes stale terms.
	if err := db.WithTx(func(tx *Tx) error { return tx.DeleteWorkFTS("epic", epicID) }); err != nil {
		t.Fatalf("DeleteWorkFTS: %v", err)
	}
	if owners, _ := db.SearchWorkFTS(`"search"*`, 10); len(owners) != 0 {
		t.Fatalf("after delete = %+v, want empty", owners)
	}
}
