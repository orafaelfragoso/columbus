package tui

import (
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
)

type viewTab int

const (
	tabMain viewTab = iota
	tabWork
	tabCount
)

type workKind int

const (
	workEpics workKind = iota
	workStories
	workTasks
	workKindCount
)

// snapshotMsg carries the result of an async Source.Load.
type snapshotMsg struct {
	snap Snapshot
	err  error
}

// tickMsg fires on the auto-refresh interval.
type tickMsg struct{}

// reindexMsg carries the result of an in-process reindex.
type reindexMsg struct{ err error }

// Option configures a Model at construction.
type Option func(*Model)

// WithRefreshInterval enables silent auto-refresh on the given interval. Zero
// (the default) disables it.
func WithRefreshInterval(d time.Duration) Option {
	return func(m *Model) { m.refreshEvery = d }
}

// WithReindex wires the `R` key to an in-process reindex. Nil (the default)
// makes `R` a no-op.
func WithReindex(fn func() error) Option {
	return func(m *Model) { m.reindex = fn }
}

// WithSearch wires the `/` key to a global search across code, memory and work.
// Nil (the default) makes `/` a no-op.
func WithSearch(fn func(string) ([]SearchHit, error)) Option {
	return func(m *Model) { m.searchFn = fn }
}

// searchResultMsg carries the result of an async global search.
type searchResultMsg struct {
	hits []SearchHit
	err  error
}

// hitItem adapts a SearchHit to the bubbles/list item interface so results are
// navigable (↑/↓ to select, enter to open the selected hit's detail).
type hitItem struct{ hit SearchHit }

func (h hitItem) Title() string {
	return st(grainColor(h.hit.Grain), true).Render(strings.ToUpper(h.hit.Grain)) + "  " + h.hit.Title
}
func (h hitItem) Description() string {
	desc := h.hit.Where
	if h.hit.Score > 0 {
		if desc != "" {
			desc += " · "
		}
		desc += "rank " + rankString(h.hit.Score)
	}
	return desc
}
func (h hitItem) FilterValue() string { return h.hit.Title }

type workItem struct {
	Kind    string
	ID      int64
	IDStr   string
	Title   string
	Status  string
	Meta    string
	Percent float64
}

// Model is the root dashboard model. All logic lives in Update and the pure
// rebuild/dims helpers; View is a thin renderer (so it stays testable).
type Model struct {
	src  Source
	snap Snapshot
	err  error

	w, h       int
	activeTab  viewTab
	workKind   workKind
	workCursor int
	loading    bool

	keys keyMap
	help help.Model
	spin spinner.Model

	mem table.Model

	detail      viewport.Model
	showDetail  bool
	detailTitle string
	// detailFromResults marks a detail view opened by drilling into a search
	// result, so Esc steps back to the results list instead of the dashboard.
	detailFromResults bool

	searchInput  textinput.Model
	searchActive bool

	searchFn    func(string) ([]SearchHit, error)
	searching   bool
	results     list.Model
	showResults bool
	confirmQuit bool

	refreshEvery time.Duration
	reindex      func() error
	reindexing   bool
}

// New builds a dashboard model over the given Source.
func New(src Source, opts ...Option) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = st(cViolet, false)

	ti := textinput.New()
	ti.Placeholder = "type a query…"
	ti.Prompt = "› "
	ti.CharLimit = 120
	ts := textinput.DefaultDarkStyles()
	ts.Focused.Prompt = st(cViolet, true)
	ts.Focused.Text = st(cBright, false)
	ts.Focused.Placeholder = st(cMuted, false)
	ti.SetStyles(ts)

	res := list.New(nil, resultsDelegate(), 0, 0)
	res.SetShowTitle(false)
	res.SetShowStatusBar(false)
	res.SetShowHelp(false)
	res.SetFilteringEnabled(false)
	res.SetShowPagination(true)

	m := Model{
		src:         src,
		keys:        defaultKeys(),
		help:        help.New(),
		spin:        sp,
		mem:         newTable(),
		detail:      viewport.New(),
		results:     res,
		searchInput: ti,
		loading:     true,
	}
	for _, o := range opts {
		o(&m)
	}
	return m
}

// resultsDelegate styles the search-results list with a clear selection accent.
func resultsDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.Foreground(cBright).BorderForeground(cViolet)
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.Foreground(cViolet).BorderForeground(cViolet)
	d.Styles.NormalTitle = d.Styles.NormalTitle.Foreground(cText)
	d.Styles.NormalDesc = d.Styles.NormalDesc.Foreground(cMuted)
	return d
}

func newTable() table.Model {
	return table.New(table.WithStyles(tableStyles(false)))
}

// tableStyles builds the shared table look. Only the focused pane paints an
// active selection background; blurred panes show a muted, background-less
// cursor row so the violet border is the single, unambiguous focus cue.
func tableStyles(active bool) table.Styles {
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).BorderForeground(cBorder).BorderBottom(true).
		Padding(0, 0).Bold(false).Foreground(cMuted)
	s.Cell = s.Cell.Padding(0, 0).Foreground(cText)
	if active {
		s.Selected = selectionStyle()
	} else {
		s.Selected = lipgloss.NewStyle().Foreground(cMuted)
	}
	return s
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.load(), m.spin.Tick}
	if m.refreshEvery > 0 {
		cmds = append(cmds, m.tick())
	}
	return tea.Batch(cmds...)
}

func (m Model) load() tea.Cmd {
	return func() tea.Msg {
		snap, err := m.src.Load()
		return snapshotMsg{snap: snap, err: err}
	}
}

func (m Model) tick() tea.Cmd {
	return tea.Tick(m.refreshEvery, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m Model) runReindex() tea.Cmd {
	fn := m.reindex
	return func() tea.Msg { return reindexMsg{err: fn()} }
}

func (m Model) runSearch(query string) tea.Cmd {
	fn := m.searchFn
	return func() tea.Msg {
		hits, err := fn(query)
		return searchResultMsg{hits: hits, err: err}
	}
}

// modalOpen reports whether any overlay (search input, results, detail) is up.
// While one is open the background auto-refresh is suspended so it can't wipe
// the overlay's content out from under the user.
func (m Model) modalOpen() bool {
	return m.searchActive || m.showResults || m.showDetail || m.confirmQuit
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.help.SetWidth(msg.Width)
		m.rebuild()
		return m, nil

	case snapshotMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.snap, m.err = msg.snap, nil
		m.rebuild()
		return m, nil

	case spinner.TickMsg:
		if !m.loading && !m.reindexing && !m.searching {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case searchResultMsg:
		m.searching = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		items := make([]list.Item, 0, len(msg.hits))
		for _, h := range msg.hits {
			items = append(items, hitItem{hit: h})
		}
		m.results.SetItems(items)
		// Size the list to its content (capped), so a couple of hits don't render
		// a mostly-empty full-height modal.
		d := m.dims()
		const perItem = 3 // DefaultDelegate: title + description + spacing
		m.results.SetSize(d.detailW, min(d.detailH, max(perItem, len(items)*perItem+2)))
		m.results.Select(0)
		m.showResults = true
		return m, nil

	case tickMsg:
		cmds := []tea.Cmd{m.tick()} // always reschedule
		if !m.loading && !m.reindexing && !m.searching && !m.modalOpen() {
			cmds = append(cmds, m.load()) // silent reload (no spinner)
		}
		return m, tea.Batch(cmds...)

	case reindexMsg:
		m.reindexing = false
		if msg.err != nil {
			m.loading = false
			m.err = msg.err
			return m, nil
		}
		return m, m.load() // reload snapshot; snapshotMsg clears loading

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if isCtrlC(msg) {
		return m, tea.Quit
	}

	// The search input owns all keystrokes while active. Esc cancels; Enter runs
	// the query synchronously (no async form handshake to lose). Everything else
	// is forwarded to the text input so it accumulates the query.
	if m.searchActive {
		switch msg.Code {
		case tea.KeyEscape:
			m.searchActive = false
			m.searchInput.Blur()
			return m, nil
		case tea.KeyEnter:
			q := strings.TrimSpace(m.searchInput.Value())
			m.searchActive = false
			m.searchInput.Blur()
			if q != "" && m.searchFn != nil {
				m.searching = true
				return m, tea.Batch(m.runSearch(q), m.spin.Tick)
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	}

	if m.confirmQuit {
		switch {
		case msg.Code == tea.KeyEscape || msg.Text == "n" || msg.Text == "N":
			m.confirmQuit = false
			return m, nil
		case msg.Code == tea.KeyEnter || msg.Text == "y" || msg.Text == "Y":
			m.confirmQuit = false
			return m, tea.Quit
		default:
			return m, nil
		}
	}

	// Esc is handled next so it always backs out of a mode (results or detail).
	if msg.Code == tea.KeyEscape {
		switch {
		case m.showResults:
			m.showResults = false
		case m.showDetail:
			m.showDetail = false
			if m.detailFromResults {
				m.detailFromResults = false
				m.showResults = true
			}
		}
		return m, nil
	}

	switch {
	case key.Matches(msg, m.keys.Quit):
		m.confirmQuit = true
		return m, nil
	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
		m.rebuild()
		return m, nil
	case key.Matches(msg, m.keys.Refresh):
		if m.reindexing {
			return m, nil
		}
		m.loading = true
		return m, tea.Batch(m.load(), m.spin.Tick)
	case key.Matches(msg, m.keys.Reindex):
		if m.reindex == nil || m.reindexing || m.loading {
			return m, nil
		}
		m.reindexing = true
		return m, tea.Batch(m.runReindex(), m.spin.Tick)
	case key.Matches(msg, m.keys.Search):
		return m, m.openSearch()
	}

	// The results overlay is a navigable list: enter opens the selected hit,
	// everything else (↑/↓, paging) is forwarded to the list.
	if m.showResults {
		if msg.Code == tea.KeyEnter {
			m.openHitDetail()
			return m, nil
		}
		var cmd tea.Cmd
		m.results, cmd = m.results.Update(msg)
		return m, cmd
	}

	if m.showDetail {
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		return m, cmd
	}

	switch {
	case key.Matches(msg, m.keys.Tab):
		m.cycleTab(1)
		return m, nil
	case key.Matches(msg, m.keys.ShiftTab):
		m.cycleTab(-1)
		return m, nil
	case key.Matches(msg, m.keys.Enter):
		m.openDetail()
		return m, nil
	}

	var cmd tea.Cmd
	switch m.activeTab {
	case tabMain:
		m.mem, cmd = m.mem.Update(msg)
	case tabWork:
		switch {
		case key.Matches(msg, m.keys.WorkPrev):
			m.cycleWorkKind(-1)
		case key.Matches(msg, m.keys.WorkNext):
			m.cycleWorkKind(1)
		case key.Matches(msg, m.keys.Left):
			m.moveWorkColumn(-1)
		case key.Matches(msg, m.keys.Right):
			m.moveWorkColumn(1)
		case key.Matches(msg, m.keys.Up):
			m.moveWorkCursor(-1)
		case key.Matches(msg, m.keys.Down):
			m.moveWorkCursor(1)
		}
	}
	return m, cmd
}

func isCtrlC(msg tea.KeyPressMsg) bool {
	return msg.Code == 'c' && msg.Mod == tea.ModCtrl
}

func (m *Model) cycleTab(dir int) {
	m.activeTab = viewTab((int(m.activeTab) + dir + int(tabCount)) % int(tabCount))
	m.applyFocus()
}

func (m *Model) applyFocus() {
	m.mem.Blur()
	m.mem.SetStyles(tableStyles(false))
	if m.activeTab == tabMain {
		m.mem.Focus()
		m.mem.SetStyles(tableStyles(true))
	}
	m.workCursor = clampCursor(m.workCursor, len(m.workItems()))
}

func (m *Model) cycleWorkKind(dir int) {
	m.workKind = workKind((int(m.workKind) + dir + int(workKindCount)) % int(workKindCount))
	m.workCursor = clampCursor(0, len(m.workItems()))
}

func (m *Model) moveWorkCursor(dir int) {
	items := m.workItems()
	if len(items) == 0 {
		m.workCursor = 0
		return
	}

	m.workCursor = clampCursor(m.workCursor, len(items))
	status := items[m.workCursor].Status
	indices := make([]int, 0, len(items))
	current := 0
	for i, item := range items {
		if item.Status != status {
			continue
		}
		if i == m.workCursor {
			current = len(indices)
		}
		indices = append(indices, i)
	}
	if len(indices) == 0 {
		return
	}
	next := (current + dir + len(indices)) % len(indices)
	m.workCursor = indices[next]
}

func (m *Model) moveWorkColumn(dir int) {
	items := m.workItems()
	if len(items) == 0 {
		m.workCursor = 0
		return
	}

	m.workCursor = clampCursor(m.workCursor, len(items))
	statuses := kanbanStatuses()
	currentStatus := items[m.workCursor].Status
	currentStatusIdx := -1
	for i, status := range statuses {
		if status == currentStatus {
			currentStatusIdx = i
			break
		}
	}
	if currentStatusIdx < 0 {
		return
	}

	row := 0
	for i := 0; i < m.workCursor; i++ {
		if items[i].Status == currentStatus {
			row++
		}
	}

	for step := 1; step <= len(statuses); step++ {
		nextStatusIdx := (currentStatusIdx + dir*step) % len(statuses)
		if nextStatusIdx < 0 {
			nextStatusIdx += len(statuses)
		}
		indices := workIndicesByStatus(items, statuses[nextStatusIdx])
		if len(indices) == 0 {
			continue
		}
		m.workCursor = indices[min(row, len(indices)-1)]
		return
	}
}

func workIndicesByStatus(items []workItem, status string) []int {
	indices := make([]int, 0, len(items))
	for i, item := range items {
		if item.Status == status {
			indices = append(indices, i)
		}
	}
	return indices
}

func (m *Model) openSearch() tea.Cmd {
	m.searchInput.SetValue("")
	m.searchInput.SetWidth(max(20, min(60, m.w-14)))
	m.searchActive = true
	return m.searchInput.Focus()
}

// openHitDetail opens the rich detail for the currently selected search result.
func (m *Model) openHitDetail() {
	it, ok := m.results.SelectedItem().(hitItem)
	if !ok {
		return
	}
	h := it.hit
	var kind, md string
	var id int64
	switch h.Grain {
	case "memory":
		kind, id = "memory", idNum(h.ID)
		md = hitMarkdown(h)
	case "epic":
		kind, id = "epic", idNum(h.ID)
		md = hitMarkdown(h)
	case "story":
		kind, id = "story", idNum(h.ID)
		md = hitMarkdown(h)
	case "task":
		kind, id = "task", idNum(h.ID)
		md = hitMarkdown(h)
	default:
		md = hitMarkdown(h)
	}
	m.detailTitle = h.Title
	m.setDetail(kind, id, md)
	m.showResults = false
	m.showDetail = true
	m.detailFromResults = true
}

func hitMarkdown(h SearchHit) string {
	var b strings.Builder
	b.WriteString("# " + h.Title + "\n\n")
	b.WriteString("**" + strings.ToUpper(h.Grain) + "**")
	if h.Where != "" {
		b.WriteString(" · `" + h.Where + "`")
	}
	if h.Score > 0 {
		b.WriteString(" · rank " + rankString(h.Score))
	}
	b.WriteString("\n")
	if strings.TrimSpace(h.Snippet) != "" {
		b.WriteString("\n## Snippet\n\n```text\n")
		b.WriteString(strings.TrimRight(h.Snippet, "\n"))
		b.WriteString("\n```\n")
	}
	return b.String()
}

func rankString(score float64) string {
	return strconv.FormatFloat(score, 'f', 2, 64)
}

func (m *Model) openDetail() {
	var kind, md string
	var id int64
	switch m.activeTab {
	case tabMain:
		mr, ok := m.selectedMem()
		if !ok {
			return
		}
		kind, id = "memory", idNum(mr.ID)
		m.detailTitle = mr.ID + "  ·  " + mr.Title
		md = memMarkdown(mr)
	case tabWork:
		item, ok := m.selectedWorkItem()
		if !ok {
			return
		}
		kind, id = item.Kind, item.ID
		m.detailTitle = item.IDStr + "  ·  " + item.Title
		switch item.Kind {
		case "epic":
			if e, ok := m.epicByID(item.ID); ok {
				md = epicMarkdown(e, m.snap)
			}
		case "story":
			if st, ok := m.storyByID(item.ID); ok {
				md = storyMarkdown(st, m.snap)
			}
		case "task":
			if t, ok := m.taskByID(item.ID); ok {
				md = taskMarkdown(t, m.snap)
			}
		}
	}
	m.setDetail(kind, id, md)
	m.showDetail = true
	m.detailFromResults = false
}

// setDetail renders markdown (preferring a rich DetailSource document when the
// source supports the kind) into the detail viewport.
func (m *Model) setDetail(kind string, id int64, md string) {
	if ds, ok := m.src.(DetailSource); ok && kind != "" {
		if rich, err := ds.Detail(kind, id); err == nil && strings.TrimSpace(rich) != "" {
			md = rich
		}
	}
	out := md
	if r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(max(20, m.detail.Width())),
	); err == nil {
		if s, e := r.Render(md); e == nil {
			out = s
		}
	}
	m.detail.SetContent(out)
	m.detail.GotoTop()
}

func (m Model) selectedMem() (MemRow, bool) {
	i := m.mem.Cursor()
	if i < 0 || i >= len(m.snap.Mems) {
		return MemRow{}, false
	}
	return m.snap.Mems[i], true
}

func (m Model) selectedWorkItem() (workItem, bool) {
	items := m.workItems()
	if m.workCursor < 0 || m.workCursor >= len(items) {
		return workItem{}, false
	}
	return items[m.workCursor], true
}

func (m Model) workItems() []workItem {
	switch m.workKind {
	case workEpics:
		items := make([]workItem, 0, len(m.snap.Epics))
		for _, e := range m.snap.Epics {
			items = append(items, workItem{
				Kind: "epic", ID: e.ID, IDStr: e.IDStr, Title: e.Title, Status: e.Status,
				Meta:    strconv.Itoa(e.Done) + "/" + strconv.Itoa(e.Total) + " tasks",
				Percent: e.Progress(),
			})
		}
		return items
	case workStories:
		items := make([]workItem, 0, len(m.snap.Stories))
		for _, st := range m.snap.Stories {
			items = append(items, workItem{
				Kind: "story", ID: st.ID, IDStr: st.IDStr, Title: st.Title, Status: st.Status,
				Meta: m.epicLabel(st.EpicID),
			})
		}
		return items
	case workTasks:
		items := make([]workItem, 0, len(m.snap.Tasks))
		for _, t := range m.snap.Tasks {
			items = append(items, workItem{
				Kind: "task", ID: t.ID, IDStr: t.IDStr, Title: t.Title, Status: t.Status,
				Meta: m.storyLabel(t.StoryID),
			})
		}
		return items
	default:
		return nil
	}
}

func (m Model) epicByID(id int64) (EpicRow, bool) {
	for _, e := range m.snap.Epics {
		if e.ID == id {
			return e, true
		}
	}
	return EpicRow{}, false
}

func (m Model) storyByID(id int64) (StoryRow, bool) {
	for _, st := range m.snap.Stories {
		if st.ID == id {
			return st, true
		}
	}
	return StoryRow{}, false
}

func (m Model) taskByID(id int64) (TaskRow, bool) {
	for _, t := range m.snap.Tasks {
		if t.ID == id {
			return t, true
		}
	}
	return TaskRow{}, false
}

func (m Model) epicLabel(id int64) string {
	for _, e := range m.snap.Epics {
		if e.ID == id {
			return e.IDStr
		}
	}
	return "epic_" + padID(id)
}

func (m Model) storyLabel(id int64) string {
	for _, st := range m.snap.Stories {
		if st.ID == id {
			return st.IDStr
		}
	}
	return "story_" + padID(id)
}

func padID(id int64) string {
	if id < 0 {
		id = 0
	}
	if id < 10 {
		return "00" + strconv.FormatInt(id, 10)
	}
	if id < 100 {
		return "0" + strconv.FormatInt(id, 10)
	}
	return strconv.FormatInt(id, 10)
}

// idNum extracts the trailing numeric id from a prefixed id like "mem_012".
func idNum(s string) int64 {
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	n, _ := strconv.ParseInt(s[i:], 10, 64)
	return n
}

// rebuild recomputes layout-dependent component state. Safe to call before a
// size or snapshot has arrived (it no-ops until both width and height exist).
func (m *Model) rebuild() {
	if m.w == 0 || m.h == 0 {
		return
	}
	d := m.dims()

	m.mem.SetColumns(memColumns(d.memW - 4))
	m.mem.SetRows(memTableRows(m.snap.Mems, d.memW-4))
	m.mem.SetWidth(d.memW - 4)
	m.mem.SetHeight(max(1, d.bodyH-5))
	m.mem.SetCursor(clampCursor(m.mem.Cursor(), len(m.snap.Mems)))

	m.detail.SetWidth(d.detailW)
	m.detail.SetHeight(d.detailH)
	m.results.SetSize(d.detailW, d.detailH)

	m.applyFocus()
}

// clampCursor keeps a table cursor in [0, n-1]; bubbles' table can leave the
// cursor at -1 after SetRows on a previously-empty table, which would make the
// selection helpers report "nothing selected" even with rows present.
func clampCursor(cur, n int) int {
	if n == 0 {
		return 0
	}
	if cur < 0 {
		return 0
	}
	if cur >= n {
		return n - 1
	}
	return cur
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
