package memory

import "testing"

func TestExportRoundTripReassignsAndDedupes(t *testing.T) {
	src, _ := newManager(t, nil)
	src.Add(AddParams{Kind: "decision", Title: "A", Body: "alpha", Tags: []string{"x"}})
	src.Add(AddParams{Kind: "pattern", Title: "B", Body: "beta"})

	doc, err := src.Export("", "")
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(doc.Memories) != 2 || doc.SchemaVersion != ExportSchemaVersion {
		t.Fatalf("export doc = %+v", doc)
	}

	// Import into a fresh project (reassign ids).
	dst, _ := newManager(t, nil)
	res, err := dst.Import(doc, false)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if res.Imported != 2 || res.Skipped != 0 {
		t.Errorf("first import = %+v, want 2 imported", res)
	}

	// Re-import the SAME doc: content-hash dedupe skips everything.
	res2, err := dst.Import(doc, false)
	if err != nil {
		t.Fatalf("second Import: %v", err)
	}
	if res2.Imported != 0 || res2.Skipped != 2 {
		t.Errorf("idempotent re-import = %+v, want 0 imported / 2 skipped", res2)
	}

	list, _ := dst.List("", "")
	if list.Total != 2 {
		t.Errorf("destination total = %d, want 2 (no duplicates)", list.Total)
	}
}

func TestImportReassignsIDsByDefault(t *testing.T) {
	src, _ := newManager(t, nil)
	src.Add(AddParams{Kind: "decision", Title: "only"})
	doc, _ := src.Export("", "")

	dst, _ := newManager(t, nil)
	// Pre-existing memory in destination occupies mem_001.
	dst.Add(AddParams{Kind: "pattern", Title: "preexisting"})

	if _, err := dst.Import(doc, false); err != nil {
		t.Fatalf("Import: %v", err)
	}
	list, _ := dst.List("", "")
	if list.Total != 2 {
		t.Fatalf("total = %d, want 2", list.Total)
	}
	// Imported record must have a fresh, non-colliding id (mem_002).
	ids := map[string]bool{}
	for _, m := range list.Memories {
		if ids[m.ID] {
			t.Fatalf("duplicate id %s after reassign", m.ID)
		}
		ids[m.ID] = true
	}
}

func TestImportPreserveIDsIntoEmptyStore(t *testing.T) {
	src, _ := newManager(t, nil)
	src.Add(AddParams{Kind: "decision", Title: "one"})
	src.Add(AddParams{Kind: "decision", Title: "two"})
	doc, _ := src.Export("", "")

	dst, _ := newManager(t, nil)
	res, err := dst.Import(doc, true)
	if err != nil {
		t.Fatalf("preserve import: %v", err)
	}
	if res.Imported != 2 {
		t.Errorf("imported = %d, want 2", res.Imported)
	}
	full, ok, _ := dst.DB.MemoryFull(1)
	if !ok || full.Title != "one" {
		t.Errorf("mem_001 should be restored as 'one', got ok=%v %q", ok, full.Title)
	}
	// Counter must advance past restored ids so the next add gets mem_003.
	r, _ := dst.Add(AddParams{Kind: "pattern", Title: "next"})
	if r.ID != "mem_003" {
		t.Errorf("next id = %s, want mem_003", r.ID)
	}
}

func TestImportPreserveIDsCollisionErrors(t *testing.T) {
	src, _ := newManager(t, nil)
	src.Add(AddParams{Kind: "decision", Title: "one"})
	doc, _ := src.Export("", "")

	dst, _ := newManager(t, nil)
	dst.Add(AddParams{Kind: "pattern", Title: "occupies mem_001"})

	_, err := dst.Import(doc, true)
	if err == nil {
		t.Fatal("expected collision error with --preserve-ids")
	}
}
