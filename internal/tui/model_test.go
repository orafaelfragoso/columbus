package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type fakeSource struct{ snap Snapshot }

func (f fakeSource) Load() (Snapshot, error) { return f.snap, nil }

// fakeDetailSource also implements DetailSource, returning a fixed document.
type fakeDetailSource struct {
	snap   Snapshot
	detail string
}

func (f fakeDetailSource) Load() (Snapshot, error)              { return f.snap, nil }
func (f fakeDetailSource) Detail(string, int64) (string, error) { return f.detail, nil }

func sampleSnap() Snapshot {
	return Snapshot{
		Branch: "main", Head: "abc1234", Files: 214, Symbols: 1883, Edges: 642, Memories: 1,
		MemCounts: map[string]int{"decision": 1},
		Epics: []EpicRow{
			{ID: 1, IDStr: "epic_001", Title: "Indexing core", Status: "in_progress", Done: 1, Total: 2},
			{ID: 2, IDStr: "epic_002", Title: "Search ranking", Status: "todo", Done: 0, Total: 1},
		},
		Tasks: []TaskRow{
			{ID: 1, EpicID: 1, IDStr: "task_001", Title: "parse go", Status: "done"},
			{ID: 2, EpicID: 1, IDStr: "task_002", Title: "symbol graph", Status: "todo"},
			{ID: 3, EpicID: 2, IDStr: "task_003", Title: "rank cache", Status: "todo"},
		},
		Mems: []MemRow{{ID: "mem_001", Kind: "decision", Title: "use WAL"}},
		Hubs: []HubRow{{Path: "internal/store/store.go", In: 5}},
	}
}

func ready(t *testing.T) Model {
	t.Helper()
	return readyModel(t, New(fakeSource{sampleSnap()}))
}

func readyModel(t *testing.T, m Model) Model {
	t.Helper()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 168, Height: 44})
	next, _ = next.(Model).Update(snapshotMsg{snap: sampleSnap()})
	return next.(Model)
}

func runes(s string) tea.KeyMsg      { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
func ktype(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }

func TestQuitKeyReturnsQuitCommand(t *testing.T) {
	m := ready(t)
	_, cmd := m.Update(runes("q"))
	if cmd == nil {
		t.Fatal("expected a command from q")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("q did not produce tea.Quit, got %T", cmd())
	}
}

func TestTabCyclesForwardThroughFourPanes(t *testing.T) {
	m := ready(t)
	want := []focus{focusTasks, focusMemory, focusGraph, focusEpics}
	for i, w := range want {
		next, _ := m.Update(ktype(tea.KeyTab))
		m = next.(Model)
		if m.focus != w {
			t.Fatalf("tab #%d focus = %v, want %v", i+1, m.focus, w)
		}
	}
}

func TestShiftTabCyclesBackward(t *testing.T) {
	m := ready(t) // starts on epics
	next, _ := m.Update(ktype(tea.KeyShiftTab))
	if f := next.(Model).focus; f != focusGraph {
		t.Fatalf("shift+tab from epics = %v, want graph (wrap back)", f)
	}
}

func TestEnterOpensDetailForMemoryAndGraph(t *testing.T) {
	m := ready(t)
	// Tab to memory, enter → detail titled with the memory id.
	m = mustUpdate(m, ktype(tea.KeyTab), ktype(tea.KeyTab)) // epics→tasks→memory
	if m.focus != focusMemory {
		t.Fatalf("focus = %v, want memory", m.focus)
	}
	md, _ := m.Update(ktype(tea.KeyEnter))
	if mm := md.(Model); !mm.showDetail || !strings.Contains(mm.detailTitle, "mem_001") {
		t.Fatalf("memory enter: showDetail=%v title=%q", mm.showDetail, mm.detailTitle)
	}

	// Tab to graph, enter → detail titled with the file path.
	m = ready(t)
	m = mustUpdate(m, ktype(tea.KeyTab), ktype(tea.KeyTab), ktype(tea.KeyTab)) // →graph
	if m.focus != focusGraph {
		t.Fatalf("focus = %v, want graph", m.focus)
	}
	gd, _ := m.Update(ktype(tea.KeyEnter))
	if gm := gd.(Model); !gm.showDetail || !strings.Contains(gm.detailTitle, "store.go") {
		t.Fatalf("graph enter: showDetail=%v title=%q", gm.showDetail, gm.detailTitle)
	}
}

func mustUpdate(m Model, msgs ...tea.Msg) Model {
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

func TestSnapshotPopulatesTasksForSelectedEpic(t *testing.T) {
	m := ready(t)
	// epic_001 is selected by default → its two tasks fill the task panel.
	if len(m.curTasks) != 2 {
		t.Fatalf("curTasks = %d, want 2 (epic_001's tasks)", len(m.curTasks))
	}
}

func TestMovingEpicCursorResyncsTaskPanel(t *testing.T) {
	m := ready(t)
	next, _ := m.Update(runes("j")) // down to epic_002
	nm := next.(Model)
	if e, _ := nm.selectedEpic(); e.IDStr != "epic_002" {
		t.Fatalf("after j, selected = %+v, want epic_002", e)
	}
	if len(nm.curTasks) != 1 {
		t.Fatalf("curTasks = %d, want 1 (epic_002's task)", len(nm.curTasks))
	}
}

func TestEnterOpensDetailForSelectedEpic(t *testing.T) {
	m := ready(t)
	next, _ := m.Update(ktype(tea.KeyEnter))
	nm := next.(Model)
	if !nm.showDetail {
		t.Fatal("enter did not open the detail pane")
	}
	if !strings.Contains(nm.detailTitle, "epic_001") {
		t.Fatalf("detail title = %q, want epic_001", nm.detailTitle)
	}
	// esc closes it.
	back, _ := nm.Update(ktype(tea.KeyEsc))
	if back.(Model).showDetail {
		t.Fatal("esc did not close the detail pane")
	}
}

func TestSearchKeyOpensForm(t *testing.T) {
	m := ready(t)
	next, _ := m.Update(runes("/"))
	if next.(Model).search == nil {
		t.Fatal("/ did not open the search form")
	}
}

func TestEscCancelsSearchForm(t *testing.T) {
	m := ready(t)
	next, _ := m.Update(runes("/"))
	nm := next.(Model)
	if nm.search == nil {
		t.Fatal("/ did not open the search form")
	}
	back, _ := nm.Update(ktype(tea.KeyEsc))
	if back.(Model).search != nil {
		t.Fatal("esc did not cancel the search form")
	}
}

func TestGlobalSearchInvokesSearchFnAndShowsResults(t *testing.T) {
	var gotQuery string
	hits := []SearchHit{
		{Grain: "memory", ID: "mem_001", Title: "use WAL", Where: "mem_001"},
		{Grain: "symbol", ID: "", Title: "NewServer", Where: "internal/api/server.go:42"},
	}
	fn := func(q string) ([]SearchHit, error) { gotQuery = q; return hits, nil }
	m := readyModel(t, New(fakeSource{sampleSnap()}, WithSearch(fn)))

	// runSearch executes the wired function and yields a searchResultMsg.
	msg := m.runSearch("server")()
	if gotQuery != "server" {
		t.Fatalf("searchFn got %q, want server", gotQuery)
	}
	sr, ok := msg.(searchResultMsg)
	if !ok || len(sr.hits) != 2 {
		t.Fatalf("runSearch produced %T (%+v)", msg, msg)
	}

	next, _ := m.Update(sr)
	nm := next.(Model)
	if !nm.showResults {
		t.Fatal("searchResultMsg did not open the results modal")
	}
	view := nm.results.View()
	if !strings.Contains(view, "NewServer") || !strings.Contains(view, "use WAL") {
		t.Fatalf("results modal missing hits:\n%s", view)
	}
	// esc closes the results modal.
	back, _ := nm.Update(ktype(tea.KeyEsc))
	if back.(Model).showResults {
		t.Fatal("esc did not close the results modal")
	}
}

func TestReindexKeyRunsAndClears(t *testing.T) {
	called := false
	m := readyModel(t, New(fakeSource{sampleSnap()}, WithReindex(func() error { called = true; return nil })))

	next, cmd := m.Update(runes("R"))
	nm := next.(Model)
	if !nm.reindexing {
		t.Fatal("R did not enter reindexing state")
	}
	if cmd == nil {
		t.Fatal("R did not return a reindex command")
	}
	// Execute the reindex command directly: it runs the closure and yields a
	// reindexMsg, which then clears the reindexing state.
	msg := nm.runReindex()()
	if !called {
		t.Fatal("reindex closure did not run")
	}
	rm, ok := msg.(reindexMsg)
	if !ok || rm.err != nil {
		t.Fatalf("runReindex produced %T (%+v), want reindexMsg{nil}", msg, msg)
	}
	done, _ := nm.Update(rm)
	if done.(Model).reindexing {
		t.Fatal("reindexMsg did not clear reindexing state")
	}
}

func TestReindexKeyIsNoOpWithoutReindexer(t *testing.T) {
	m := ready(t) // no WithReindex
	next, _ := m.Update(runes("R"))
	if next.(Model).reindexing {
		t.Fatal("R should be a no-op when no reindexer is wired")
	}
}

func TestReindexErrorSurfacesAndClears(t *testing.T) {
	m := readyModel(t, New(fakeSource{sampleSnap()}, WithReindex(func() error { return errors.New("boom") })))
	next, _ := m.Update(runes("R"))
	done, _ := next.(Model).Update(reindexMsg{err: errors.New("boom")})
	nm := done.(Model)
	if nm.reindexing {
		t.Fatal("reindexing should clear on error")
	}
	if nm.err == nil {
		t.Fatal("reindex error should surface on the model")
	}
}

func TestTickReschedulesAndReloads(t *testing.T) {
	m := readyModel(t, New(fakeSource{sampleSnap()}, WithRefreshInterval(time.Millisecond)))
	_, cmd := m.Update(tickMsg{})
	if cmd == nil {
		t.Fatal("tick should always return a command (at least a reschedule)")
	}
}

func TestEnterUsesDetailSourceWhenAvailable(t *testing.T) {
	src := fakeDetailSource{snap: sampleSnap(), detail: "# ZZTOKEN detail body\n\nrich content"}
	m := readyModel(t, New(src))
	next, _ := m.Update(ktype(tea.KeyEnter))
	nm := next.(Model)
	if !nm.showDetail {
		t.Fatal("enter did not open detail")
	}
	if !strings.Contains(nm.detail.View(), "ZZTOKEN") {
		t.Fatalf("detail did not render the DetailSource document:\n%s", nm.detail.View())
	}
}

func TestRefreshKeyEntersLoadingState(t *testing.T) {
	m := ready(t)
	next, cmd := m.Update(runes("r"))
	if !next.(Model).loading {
		t.Fatal("r did not enter loading state")
	}
	if cmd == nil {
		t.Fatal("r did not return a reload command")
	}
}

func TestViewRendersDashboardAcrossSizes(t *testing.T) {
	for _, sz := range []tea.WindowSizeMsg{{Width: 120, Height: 30}, {Width: 168, Height: 44}, {Width: 220, Height: 55}} {
		m := New(fakeSource{sampleSnap()})
		next, _ := m.Update(sz)
		next, _ = next.(Model).Update(snapshotMsg{snap: sampleSnap()})
		out := next.(Model).View()
		if !strings.Contains(out, "Columbus") || !strings.Contains(out, "EPICS") {
			t.Fatalf("View at %dx%d missing expected chrome", sz.Width, sz.Height)
		}
	}
}
