package memory

import (
	"testing"

	"github.com/orafaelfragoso/columbus/internal/store"
)

// seedEpicWithTask seeds one epic (with a memory ref + events) and one child
// task directly through the store, returning their numeric ids.
func seedEpicWithTask(t *testing.T, m *Manager, memRef string) (epicID, taskID int64) {
	t.Helper()
	err := m.DB.WithTx(func(tx *store.Tx) error {
		var e error
		if epicID, e = tx.NextEpicSeq(); e != nil {
			return e
		}
		if e = tx.InsertEpic(epicID, "Ship search", "epic body", "in_progress", "t0", "t1"); e != nil {
			return e
		}
		if e = tx.AddWorkTag("epic", epicID, "infra"); e != nil {
			return e
		}
		if e = tx.AddWorkRef("epic", epicID, "memory", memRef); e != nil {
			return e
		}
		if e = tx.AppendWorkEvent("epic", epicID, "todo", "", "t0"); e != nil {
			return e
		}
		if e = tx.AppendWorkEvent("epic", epicID, "in_progress", "kicked off", "t1"); e != nil {
			return e
		}
		if taskID, e = tx.NextTaskSeq(); e != nil {
			return e
		}
		if e = tx.InsertTask(taskID, epicID, "child task", "", "todo", "t0", "t0"); e != nil {
			return e
		}
		return tx.AppendWorkEvent("task", taskID, "todo", "", "t0")
	})
	if err != nil {
		t.Fatalf("seedEpicWithTask: %v", err)
	}
	return epicID, taskID
}

func TestExportCarriesEpicsAndTasks(t *testing.T) {
	m, _ := newManager(t, nil)
	mem, _ := m.Add(AddParams{Kind: "decision", Title: "use WAL"})
	seedEpicWithTask(t, m, mem.ID)

	doc, err := m.Export("", "")
	if err != nil {
		t.Fatal(err)
	}
	if doc.SchemaVersion != ExportSchemaVersion {
		t.Fatalf("schema version = %d", doc.SchemaVersion)
	}
	if len(doc.Epics) != 1 || len(doc.Tasks) != 1 || len(doc.Memories) != 1 {
		t.Fatalf("doc = %d epics, %d tasks, %d memories", len(doc.Epics), len(doc.Tasks), len(doc.Memories))
	}
	epic := doc.Epics[0]
	if epic.ID != "epic_001" || epic.Status != "in_progress" || len(epic.Events) != 2 {
		t.Fatalf("epic = %+v", epic)
	}
	if len(epic.Refs) != 1 || epic.Refs[0].TargetType != "memory" || epic.Refs[0].TargetRef != "mem_001" {
		t.Fatalf("epic refs = %+v", epic.Refs)
	}
	if doc.Tasks[0].Epic != "epic_001" {
		t.Fatalf("task epic = %q", doc.Tasks[0].Epic)
	}
}

func TestImportPreserveIDsRoundTripsKnowledge(t *testing.T) {
	src, _ := newManager(t, nil)
	mem, _ := src.Add(AddParams{Kind: "decision", Title: "use WAL"})
	seedEpicWithTask(t, src, mem.ID)
	doc, _ := src.Export("", "")

	dst, _ := newManager(t, nil)
	res, err := dst.Import(doc, true)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.Imported == 0 {
		t.Fatalf("nothing imported: %+v", res)
	}

	out, _ := dst.Export("", "")
	if len(out.Epics) != 1 || out.Epics[0].ID != "epic_001" {
		t.Fatalf("epics not preserved: %+v", out.Epics)
	}
	if len(out.Tasks) != 1 || out.Tasks[0].ID != "task_001" || out.Tasks[0].Epic != "epic_001" {
		t.Fatalf("tasks not preserved: %+v", out.Tasks)
	}
	// The id counters advanced so a subsequent add does not collide.
	e2, err := dst.Export("", "")
	if err != nil || len(e2.Epics) != 1 {
		t.Fatalf("re-export: %v", err)
	}
}

func TestImportReassignDropsUnmappableMemoryRef(t *testing.T) {
	dst, _ := newManager(t, nil)
	// Reassign import of an epic whose memory ref points at a memory NOT in the
	// document: the bare numeric id would mis-target a local memory, so the ref
	// is dropped rather than silently written.
	doc := ExportDoc{
		SchemaVersion: ExportSchemaVersion,
		Epics: []ExportEpic{{
			ID: "epic_003", Title: "orphan-ref epic", Status: "todo",
			Refs:   []ExportRef{{TargetType: "memory", TargetRef: "mem_099"}, {TargetType: "symbol", TargetRef: "Foo"}},
			Events: []ExportEvent{{Status: "todo", CreatedAt: "t0"}},
		}},
	}
	if _, err := dst.Import(doc, false); err != nil {
		t.Fatalf("reassign import: %v", err)
	}
	out, _ := dst.Export("", "")
	if len(out.Epics) != 1 {
		t.Fatalf("epics = %+v", out.Epics)
	}
	for _, r := range out.Epics[0].Refs {
		if r.TargetType == "memory" {
			t.Fatalf("unmappable memory ref should be dropped, got %+v", r)
		}
	}
	// The non-memory (symbol) ref survives untouched.
	if len(out.Epics[0].Refs) != 1 || out.Epics[0].Refs[0].TargetType != "symbol" {
		t.Fatalf("symbol ref should survive: %+v", out.Epics[0].Refs)
	}
}

func TestImportPreserveIDsEpicCollisionErrors(t *testing.T) {
	dst, _ := newManager(t, nil)
	seedEpicWithTask(t, dst, "mem_001") // occupies epic_001 / task_001

	doc := ExportDoc{
		SchemaVersion: ExportSchemaVersion,
		Epics:         []ExportEpic{{ID: "epic_001", Title: "dup", Status: "todo"}},
	}
	if _, err := dst.Import(doc, true); err == nil {
		t.Fatal("expected a collision error importing epic_001 with --preserve-ids")
	}
}

func TestImportReassignRemapsCrossEntityRefs(t *testing.T) {
	dst, _ := newManager(t, nil)
	// Pre-occupy mem_001 / epic_001 so reassign must allocate fresh ids.
	dst.Add(AddParams{Kind: "pattern", Title: "existing"})
	seedEpicWithTask(t, dst, "mem_001")

	doc := ExportDoc{
		SchemaVersion: ExportSchemaVersion,
		Memories:      []ExportRecord{{ID: "mem_005", Kind: "decision", Title: "imported memory"}},
		Epics: []ExportEpic{{
			ID: "epic_003", Title: "imported epic", Status: "todo",
			Refs:   []ExportRef{{TargetType: "memory", TargetRef: "mem_005"}},
			Events: []ExportEvent{{Status: "todo", CreatedAt: "t0"}},
		}},
		Tasks: []ExportTask{{
			ID: "task_002", Epic: "epic_003", Title: "imported task", Status: "todo",
			Events: []ExportEvent{{Status: "todo", CreatedAt: "t0"}},
		}},
	}

	if _, err := dst.Import(doc, false); err != nil {
		t.Fatalf("reassign import: %v", err)
	}

	out, _ := dst.Export("", "")
	// New ids: memory mem_002, epic epic_002, task task_002 (allocated fresh).
	var imported *ExportEpic
	for i := range out.Epics {
		if out.Epics[i].Title == "imported epic" {
			imported = &out.Epics[i]
		}
	}
	if imported == nil {
		t.Fatalf("imported epic missing: %+v", out.Epics)
	}
	if len(imported.Refs) != 1 || imported.Refs[0].TargetType != "memory" {
		t.Fatalf("imported epic refs = %+v", imported.Refs)
	}
	// The memory ref must point at the remapped (new) memory id, not mem_005.
	if imported.Refs[0].TargetRef == "mem_005" {
		t.Fatalf("cross-entity ref was not remapped: %s", imported.Refs[0].TargetRef)
	}
	newMemID := imported.Refs[0].TargetRef
	var found bool
	for _, mm := range out.Memories {
		if mm.ID == newMemID && mm.Title == "imported memory" {
			found = true
		}
	}
	if !found {
		t.Fatalf("remapped memory ref %s does not point at the imported memory: %+v", newMemID, out.Memories)
	}
	// The imported task must be re-parented to the remapped epic id.
	for _, ta := range out.Tasks {
		if ta.Title == "imported task" && ta.Epic != imported.ID {
			t.Fatalf("task epic = %q, want remapped %q", ta.Epic, imported.ID)
		}
	}
}
