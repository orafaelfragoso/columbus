package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
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

func runes(s string) tea.KeyPressMsg { return tea.KeyPressMsg{Code: []rune(s)[0], Text: s} }
func ktype(c rune) tea.KeyPressMsg   { return tea.KeyPressMsg{Code: c} }
func shiftTab() tea.KeyPressMsg      { return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift} }

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
	next, _ := m.Update(shiftTab())
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
	back, _ := nm.Update(ktype(tea.KeyEscape))
	if back.(Model).showDetail {
		t.Fatal("esc did not close the detail pane")
	}
}

func TestSearchKeyOpensForm(t *testing.T) {
	m := ready(t)
	next, _ := m.Update(runes("/"))
	if !next.(Model).searchActive {
		t.Fatal("/ did not open the search input")
	}
}

func TestEscCancelsSearchForm(t *testing.T) {
	m := ready(t)
	next, _ := m.Update(runes("/"))
	nm := next.(Model)
	if !nm.searchActive {
		t.Fatal("/ did not open the search input")
	}
	back, _ := nm.Update(ktype(tea.KeyEscape))
	if back.(Model).searchActive {
		t.Fatal("esc did not cancel the search input")
	}
}

func TestEnterInSearchInputStartsSearch(t *testing.T) {
	m := readyModel(t, New(fakeSource{sampleSnap()},
		WithSearch(func(string) ([]SearchHit, error) { return nil, nil })))

	m = mustUpdate(m, runes("/"))   // open the search input
	m = mustUpdate(m, runes("wal")) // type a query
	if got := m.searchInput.Value(); got != "wal" {
		t.Fatalf("typed value = %q, want wal (input not capturing keystrokes)", got)
	}

	next, cmd := m.Update(ktype(tea.KeyEnter))
	nm := next.(Model)
	if nm.searchActive {
		t.Fatal("enter did not close the search input")
	}
	if !nm.searching {
		t.Fatal("enter did not start a search")
	}
	if cmd == nil {
		t.Fatal("enter did not return a search command")
	}
}

func TestEnterOnEmptyQueryDoesNotSearch(t *testing.T) {
	m := readyModel(t, New(fakeSource{sampleSnap()},
		WithSearch(func(string) ([]SearchHit, error) { return nil, nil })))
	m = mustUpdate(m, runes("/"))
	next, _ := m.Update(ktype(tea.KeyEnter))
	if nm := next.(Model); nm.searching {
		t.Fatal("enter on an empty query should not start a search")
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
	back, _ := nm.Update(ktype(tea.KeyEscape))
	if back.(Model).showResults {
		t.Fatal("esc did not close the results modal")
	}
}

func TestResultsAreNavigableAndOpenDetail(t *testing.T) {
	hits := []SearchHit{
		{Grain: "memory", ID: "mem_001", Title: "use WAL", Where: "mem_001"},
		{Grain: "symbol", Title: "NewServer", Where: "internal/api/server.go:42"},
	}
	m := readyModel(t, New(fakeSource{sampleSnap()},
		WithSearch(func(string) ([]SearchHit, error) { return hits, nil })))
	m = mustUpdate(m, searchResultMsg{hits: hits})
	if !m.showResults {
		t.Fatal("results modal did not open")
	}

	// The modal is composited OVER the dashboard: background chrome stays visible.
	out := m.View().Content
	if !strings.Contains(out, "Columbus") || !strings.Contains(out, "Search —") {
		t.Fatalf("results should overlay (not replace) the dashboard:\n%s", out)
	}

	// ↓ selects the second hit; enter opens its detail and closes the results.
	m = mustUpdate(m, ktype(tea.KeyDown), ktype(tea.KeyEnter))
	if m.showResults || !m.showDetail {
		t.Fatalf("enter on a result should open detail: results=%v detail=%v", m.showResults, m.showDetail)
	}
	if !strings.Contains(m.detailTitle, "NewServer") {
		t.Fatalf("navigation opened the wrong hit: detailTitle=%q", m.detailTitle)
	}
}

func TestEscFromResultDetailReturnsToResultsList(t *testing.T) {
	hits := []SearchHit{
		{Grain: "memory", ID: "mem_001", Title: "use WAL", Where: "mem_001"},
		{Grain: "symbol", Title: "NewServer", Where: "internal/api/server.go:42"},
	}
	m := readyModel(t, New(fakeSource{sampleSnap()},
		WithSearch(func(string) ([]SearchHit, error) { return hits, nil })))
	m = mustUpdate(m, searchResultMsg{hits: hits})

	// Drill into the second hit's detail.
	m = mustUpdate(m, ktype(tea.KeyDown), ktype(tea.KeyEnter))
	if !m.showDetail || m.showResults {
		t.Fatalf("precondition: detail open over closed results (detail=%v results=%v)", m.showDetail, m.showResults)
	}

	// Esc steps back to the (preserved) results list rather than the dashboard.
	m = mustUpdate(m, ktype(tea.KeyEscape))
	if !m.showResults || m.showDetail {
		t.Fatalf("esc should return to the results list (results=%v detail=%v)", m.showResults, m.showDetail)
	}
	if len(m.results.Items()) != 2 {
		t.Fatalf("results list should be preserved, got %d items", len(m.results.Items()))
	}

	// A second esc closes the results list back to the dashboard.
	m = mustUpdate(m, ktype(tea.KeyEscape))
	if m.showResults {
		t.Fatal("second esc should close the results modal back to the dashboard")
	}
}

func TestEscFromPaneDetailReturnsToDashboard(t *testing.T) {
	m := ready(t) // detail opened from a dashboard pane, not from results
	m = mustUpdate(m, ktype(tea.KeyEnter))
	if !m.showDetail {
		t.Fatal("precondition: enter opens pane detail")
	}
	m = mustUpdate(m, ktype(tea.KeyEscape))
	if m.showDetail || m.showResults {
		t.Fatalf("esc from a pane detail should close to the dashboard (detail=%v results=%v)", m.showDetail, m.showResults)
	}
}

func TestRefreshTickDoesNotCloseAnOpenModal(t *testing.T) {
	hits := []SearchHit{{Grain: "symbol", Title: "NewServer", Where: "a.go:1"}}
	m := readyModel(t, New(fakeSource{sampleSnap()},
		WithRefreshInterval(time.Millisecond),
		WithSearch(func(string) ([]SearchHit, error) { return hits, nil })))
	m = mustUpdate(m, searchResultMsg{hits: hits})

	// A background refresh tick must not reload (which would rebuild and wipe the
	// results out from under the user) while a modal is open.
	_, cmd := m.Update(tickMsg{})
	if cmd == nil {
		t.Fatal("tick should still reschedule itself")
	}
	if !m.showResults || len(m.results.Items()) != 1 {
		t.Fatal("a refresh tick wiped the open results modal")
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

func TestOnlyFocusedTableShowsSelectionHighlight(t *testing.T) {
	const selBgSeq = "48;2;36;29;61" // selBg (#241d3d) rendered as an ANSI background
	m := ready(t)                    // focus starts on epics
	if !strings.Contains(m.epics.View(), selBgSeq) {
		t.Fatal("focused epics table should highlight its selected row")
	}
	if strings.Contains(m.graph.View(), selBgSeq) {
		t.Fatalf("blurred graph table should not paint a selection highlight:\n%s", m.graph.View())
	}

	// Cycling focus moves the highlight; it must never leave two panes lit at once.
	m = mustUpdate(m, ktype(tea.KeyTab), ktype(tea.KeyTab), ktype(tea.KeyTab)) // →graph
	if !strings.Contains(m.graph.View(), selBgSeq) {
		t.Fatal("graph table should highlight its selection once focused")
	}
	if strings.Contains(m.epics.View(), selBgSeq) {
		t.Fatal("epics table should drop its highlight once blurred")
	}
}

func TestSearchInProgressShowsHeaderFeedback(t *testing.T) {
	m := readyModel(t, New(fakeSource{sampleSnap()},
		WithSearch(func(string) ([]SearchHit, error) { return nil, nil })))
	m = mustUpdate(m, runes("/"), runes("x"))
	next, _ := m.Update(ktype(tea.KeyEnter))
	nm := next.(Model)
	if !nm.searching {
		t.Fatal("precondition: enter should start a search")
	}
	if !strings.Contains(nm.View().Content, "searching") {
		t.Fatalf("header should signal an in-flight search:\n%s", nm.View().Content)
	}
}

func TestDimsAndViewSurviveTinyTerminals(t *testing.T) {
	m := New(fakeSource{sampleSnap()})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 12, Height: 5})
	next, _ = next.(Model).Update(snapshotMsg{snap: sampleSnap()})
	nm := next.(Model)

	d := nm.dims()
	if d.detailW < 1 || d.detailH < 1 {
		t.Fatalf("detail dims must stay positive on a tiny terminal: %+v", d)
	}
	// Opening the search overlay at this size must not panic.
	sm := mustUpdate(nm, runes("/"))
	_ = sm.View().Content
}

func TestFooterHelpIsContextual(t *testing.T) {
	m := ready(t)
	if !strings.Contains(m.footerView(), "next pane") {
		t.Fatal("dashboard footer should advertise pane navigation")
	}
	sm := mustUpdate(m, runes("/"))
	f := sm.footerView()
	if strings.Contains(f, "next pane") {
		t.Fatalf("search footer should not show dashboard pane keys:\n%s", f)
	}
	if !strings.Contains(f, "cancel") {
		t.Fatalf("search footer should show its own keys:\n%s", f)
	}
}

func TestStackedSectionsRenderFlush(t *testing.T) {
	m := ready(t)
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	for i := 1; i < len(lines)-1; i++ {
		if strings.TrimSpace(lines[i]) != "" {
			continue
		}
		prev := strings.TrimSpace(lines[i-1])
		next := strings.TrimSpace(lines[i+1])
		// A blank line wedged between one box's bottom border and the next box's
		// top border is exactly the inter-section gap we want gone.
		if strings.HasSuffix(prev, "╯") && strings.HasPrefix(next, "╭") {
			t.Fatalf("blank separator between stacked sections at line %d:\n%q\n%q\n%q", i, prev, lines[i], next)
		}
	}
}

func TestViewRendersDashboardAcrossSizes(t *testing.T) {
	for _, sz := range []tea.WindowSizeMsg{{Width: 120, Height: 30}, {Width: 168, Height: 44}, {Width: 220, Height: 55}} {
		m := New(fakeSource{sampleSnap()})
		next, _ := m.Update(sz)
		next, _ = next.(Model).Update(snapshotMsg{snap: sampleSnap()})
		out := next.(Model).View().Content
		if !strings.Contains(out, "Columbus") || !strings.Contains(out, "EPICS") {
			t.Fatalf("View at %dx%d missing expected chrome", sz.Width, sz.Height)
		}
	}
}
