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
