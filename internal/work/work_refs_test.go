package work

import (
	"testing"

	"github.com/rafaelfragoso/columbus/internal/contract"
	"github.com/rafaelfragoso/columbus/internal/store"
)

// seedTargets writes one indexed file (with a symbol) and one memory so that
// file/dir/symbol/memory refs have something to resolve against.
func seedTargets(t *testing.T, m *Manager) {
	t.Helper()
	err := m.DB.WithTx(func(tx *store.Tx) error {
		if _, e := tx.PutFile(
			store.FileRecord{Path: "internal/a.go", Language: "go", Role: "impl", BlobOID: "x"},
			[]store.SymbolRecord{{Name: "Foo", Kind: "function", Exported: true}},
			nil, nil, nil,
		); e != nil {
			return e
		}
		id, e := tx.NextMemSeq()
		if e != nil {
			return e
		}
		return tx.InsertMemory(id, "decision", "a memory", "", "t0", "t0")
	})
	if err != nil {
		t.Fatalf("seedTargets: %v", err)
	}
}

func TestEpicRefResolvedTargetsNoWarning(t *testing.T) {
	m := newManager(t)
	seedTargets(t, m)
	e, _ := m.EpicAdd(EpicAddParams{Title: "x"})

	r, err := m.EpicRef(e.ID, []RefSpec{
		{Type: "file", Ref: "internal/a.go"},
		{Type: "dir", Ref: "internal"},
		{Type: "symbol", Ref: "Foo"},
		{Type: "memory", Ref: "mem_001"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Warnings) != 0 {
		t.Fatalf("resolved refs should not warn: %v", r.Warnings)
	}
	if len(r.Refs) != 4 {
		t.Fatalf("refs = %+v, want 4", r.Refs)
	}
}

func TestEpicRefUnresolvedIsWarningNotError(t *testing.T) {
	m := newManager(t)
	seedTargets(t, m)
	e, _ := m.EpicAdd(EpicAddParams{Title: "x"})

	r, err := m.EpicRef(e.ID, []RefSpec{{Type: "file", Ref: "ghost.go"}}, nil)
	if err != nil {
		t.Fatalf("unresolved ref must not error: %v", err)
	}
	if len(r.Warnings) == 0 {
		t.Fatal("expected a drift warning")
	}
	if len(r.Refs) != 1 {
		t.Fatal("ref should be stored anyway")
	}
}

func TestEpicRefRequiresAChange(t *testing.T) {
	m := newManager(t)
	e, _ := m.EpicAdd(EpicAddParams{Title: "x"})
	_, err := m.EpicRef(e.ID, nil, nil)
	assertCode(t, err, contract.CodeUsage)
}

func TestEpicRefRemove(t *testing.T) {
	m := newManager(t)
	seedTargets(t, m)
	e, _ := m.EpicAdd(EpicAddParams{Title: "x"})
	m.EpicRef(e.ID, []RefSpec{{Type: "file", Ref: "internal/a.go"}, {Type: "symbol", Ref: "Foo"}}, nil)

	r, err := m.EpicRef(e.ID, nil, []RefSpec{{Type: "symbol", Ref: "Foo"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Refs) != 1 || r.Refs[0].TargetType != "file" {
		t.Fatalf("after remove = %+v", r.Refs)
	}
}

func TestValidateReportsDrift(t *testing.T) {
	m := newManager(t)
	seedTargets(t, m)
	e, _ := m.EpicAdd(EpicAddParams{Title: "x"})
	m.EpicRef(e.ID, []RefSpec{
		{Type: "file", Ref: "internal/a.go"}, // resolves
		{Type: "file", Ref: "ghost.go"},      // drift
	}, nil)

	res, err := m.EpicValidate()
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 || res.Unresolved != 1 {
		t.Fatalf("validate = %+v", res)
	}
	if res.Healthy {
		t.Fatal("should not be healthy with an unresolved ref")
	}
}

func TestTaskRefAndValidate(t *testing.T) {
	m := newManager(t)
	seedTargets(t, m)
	e, _ := m.EpicAdd(EpicAddParams{Title: "x"})
	ta, _ := m.TaskAdd(TaskAddParams{Epic: e.ID, Title: "t"})

	if _, err := m.TaskRef(ta.ID, []RefSpec{{Type: "dir", Ref: "does/not/exist"}}, nil); err != nil {
		t.Fatal(err)
	}
	res, err := m.TaskValidate()
	if err != nil {
		t.Fatal(err)
	}
	if res.Unresolved != 1 {
		t.Fatalf("task validate unresolved = %d, want 1", res.Unresolved)
	}
}

func TestParseRef(t *testing.T) {
	r, err := ParseRef("memory:mem_007")
	if err != nil || r.Type != "memory" || r.Ref != "mem_007" {
		t.Fatalf("parsed = %+v err=%v", r, err)
	}
	if _, err := ParseRef("ticket:JIRA-1"); err == nil {
		t.Fatal("ParseRef should reject unknown target type")
	}
}
