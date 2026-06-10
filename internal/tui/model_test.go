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
		Branch: "main", Head: "abc1234", Files: 214, Symbols: 1883, Embeddings: 7, Memories: 2,
		MemCounts: map[string]int{"adr": 1, "plan": 1},
		Mems: []MemRow{
			{ID: "mem_001", Kind: "adr", Title: "use WAL"},
			{ID: "mem_002", Kind: "plan", Title: "ship master search"},
		},
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

func TestQuitKeyOpensConfirmationModal(t *testing.T) {
	m := ready(t)
	next, cmd := m.Update(runes("q"))
	nm := next.(Model)
	if cmd != nil {
		t.Fatal("q should open confirmation without quitting immediately")
	}
	if !nm.confirmQuit {
		t.Fatal("q did not open the quit confirmation")
	}
	if out := ansi.Strip(nm.View().Content); !strings.Contains(out, "Quit Columbus?") || !strings.Contains(out, "y confirm") {
		t.Fatalf("quit confirmation not rendered:\n%s", out)
	}
}

func TestQuitConfirmationCanBeCancelled(t *testing.T) {
	m := mustUpdate(ready(t), runes("q"))
	for _, key := range []tea.KeyPressMsg{runes("n"), ktype(tea.KeyEscape)} {
		next, cmd := m.Update(key)
		nm := next.(Model)
		if cmd != nil {
			t.Fatalf("%v should not quit", key)
		}
		if nm.confirmQuit {
			t.Fatalf("%v should close quit confirmation", key)
		}
		m = mustUpdate(ready(t), runes("q"))
	}
}

func TestQuitConfirmationAcceptsYAndEnter(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{runes("y"), ktype(tea.KeyEnter)} {
		m := mustUpdate(ready(t), runes("q"))
		next, cmd := m.Update(key)
		if next.(Model).confirmQuit {
			t.Fatalf("%v should close quit confirmation", key)
		}
		if cmd == nil {
			t.Fatalf("%v did not return a quit command", key)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("%v did not produce tea.Quit, got %T", key, cmd())
		}
	}
}

func TestCtrlCStillReturnsQuitCommand(t *testing.T) {
	m := ready(t)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("expected a command from ctrl+c")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("ctrl+c did not produce tea.Quit, got %T", cmd())
	}
}

func TestEnterOpensDetailForSelectedMemory(t *testing.T) {
	m := ready(t)
	md, _ := m.Update(ktype(tea.KeyEnter))
	if mm := md.(Model); !mm.showDetail || !strings.Contains(mm.detailTitle, "mem_001") {
		t.Fatalf("memory enter: showDetail=%v title=%q", mm.showDetail, mm.detailTitle)
	}
}

func TestDownMovesMemorySelection(t *testing.T) {
	m := ready(t)
	m = mustUpdate(m, ktype(tea.KeyDown))
	md, _ := m.Update(ktype(tea.KeyEnter))
	if mm := md.(Model); !mm.showDetail || !strings.Contains(mm.detailTitle, "mem_002") {
		t.Fatalf("after down, enter should open mem_002: showDetail=%v title=%q", mm.showDetail, mm.detailTitle)
	}
}

func mustUpdate(m Model, msgs ...tea.Msg) Model {
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
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

func TestSearchResultDetailShowsSnippetAndRank(t *testing.T) {
	hits := []SearchHit{{
		Grain: "symbol", Title: "NewServer", Where: "internal/api/server.go:42",
		Score: 0.92, Snippet: "func NewServer() *Server {\n\treturn &Server{}\n}",
	}}
	m := readyModel(t, New(fakeSource{sampleSnap()},
		WithSearch(func(string) ([]SearchHit, error) { return hits, nil })))
	m = mustUpdate(m, searchResultMsg{hits: hits}, ktype(tea.KeyEnter))

	if !m.showDetail {
		t.Fatal("enter on result should open detail")
	}
	detail := ansi.Strip(m.detail.View())
	for _, want := range []string{"rank 0.92", "func NewServer()", "return &Server{}"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("search result detail missing %q:\n%s", want, detail)
		}
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

func TestMainMemoryTableShowsSelectionHighlight(t *testing.T) {
	const borderBgSeq = "48;2;167;139;250" // active panel border (#a78bfa) rendered as an ANSI background
	m := ready(t)
	for _, line := range strings.Split(m.bodyView(), "\n") {
		if strings.Contains(line, "use WAL") {
			if !strings.Contains(line, borderBgSeq) {
				t.Fatalf("selected memory row should use the border color as its row background:\n%s", line)
			}
			return
		}
	}
	t.Fatalf("selected memory row not found:\n%s", m.bodyView())
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

func TestHeaderHasNoTabsAndShowsBranchOnRight(t *testing.T) {
	m := ready(t)

	header := m.headerView()
	firstLine := strings.Split(header, "\n")[0]
	clean := ansi.Strip(firstLine)
	for _, want := range []string{"Columbus", "branch:main"} {
		if !strings.Contains(clean, want) {
			t.Fatalf("header missing %q:\n%s", want, clean)
		}
	}
	for _, notWant := range []string{"MAIN", "WORK"} {
		if strings.Contains(clean, notWant) {
			t.Fatalf("header should not render tab %q:\n%s", notWant, clean)
		}
	}
}

func TestMainViewRendersMetricCardsAndFullMemoryPanel(t *testing.T) {
	m := ready(t)
	out := ansi.Strip(m.View().Content)
	for _, want := range []string{"FILES INDEXED", "SYMBOLS", "EMBEDDINGS", "7", "MEMORIES", "1 adr", "1 plan", "MEMORY"} {
		if !strings.Contains(out, want) {
			t.Fatalf("main view missing %q:\n%s", want, out)
		}
	}
	for _, notWant := range []string{"EPICS", "STORIES", "TASKS", "GRAPH EDGES"} {
		if strings.Contains(out, notWant) {
			t.Fatalf("main view should not render %q:\n%s", notWant, out)
		}
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
	if !strings.Contains(m.footerView(), "detail") {
		t.Fatal("dashboard footer should advertise the enter/detail key")
	}
	sm := mustUpdate(m, runes("/"))
	f := sm.footerView()
	if strings.Contains(f, "reindex") {
		t.Fatalf("search footer should not show dashboard keys:\n%s", f)
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
		if !strings.Contains(out, "Columbus") || !strings.Contains(out, "MEMORY") || !strings.Contains(out, "EMBEDDINGS") {
			t.Fatalf("View at %dx%d missing expected chrome", sz.Width, sz.Height)
		}
	}
}
