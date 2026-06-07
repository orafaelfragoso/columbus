package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
)

// layout holds the computed rectangle sizes for the current window.
type layout struct {
	epicsW, tasksW, midH int
	memW, graphW, botH   int
	detailW, detailH     int
}

// vGap is the number of blank rows between stacked sections.
const vGap = 3

func (m Model) dims() layout {
	w, h := m.w, m.h
	// header(2) + cards(6) + footer + 3 inter-section blank rows.
	rem := h - 2 - cardHeight - lipgloss.Height(m.footerView()) - vGap
	if rem < 8 {
		rem = 8
	}
	midH := rem * 56 / 100
	midAvail := w - hGap
	botAvail := w - hGap
	epicsW := midAvail * 42 / 100
	memW := botAvail * 52 / 100
	return layout{
		epicsW: epicsW, tasksW: midAvail - epicsW, midH: midH,
		memW: memW, graphW: botAvail - memW, botH: rem - midH,
		detailW: min(w-8, 100), detailH: h * 7 / 10,
	}
}

func (m Model) View() string {
	if m.w == 0 || m.h == 0 {
		return "loading…"
	}
	if m.search != nil {
		return modal(m.w, m.h, "Search", m.search.View())
	}
	if m.showResults {
		body := m.results.View() + "\n\n" + st(cMuted, false).Render("esc back · ↑/↓ scroll")
		title := fmt.Sprintf("Search — %d results", len(m.resultsHits))
		return modal(m.w, m.h, title, body)
	}
	if m.showDetail {
		body := m.detail.View() + "\n\n" + st(cMuted, false).Render("esc back · ↑/↓ scroll")
		return modal(m.w, m.h, m.detailTitle, body)
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		m.headerView(),
		"",
		m.cardsView(),
		"",
		m.midView(),
		"",
		m.botView(),
		m.footerView(),
	)
}

func (m Model) headerView() string {
	s := m.snap
	left := st(cViolet, true).Render("✦ Columbus")
	if s.Branch != "" {
		left += st(cMuted, false).Render("   branch:") + st(cText, false).Render(s.Branch)
	}

	var right string
	switch {
	case m.reindexing:
		right = m.spin.View() + st(cViolet, false).Render(" reindexing…")
	case m.loading:
		right = m.spin.View() + st(cMuted, false).Render(" loading…")
	case m.err != nil:
		right = st(cRed, false).Render("● error: " + m.err.Error())
	default:
		dirty, dc := "clean", cGreen
		if s.Dirty {
			dirty, dc = "dirty", cYellow
		}
		head := s.Head
		if head == "" {
			head = "(unindexed)"
		}
		right = st(dc, false).Render("● ") +
			st(cMuted, false).Render("indexed ") + st(cText, false).Render(head) +
			st(cMuted, false).Render(" · ") + st(dc, false).Render(dirty) +
			st(cMuted, false).Render(" · ") + st(cText, false).Render(comma(s.Files)+" files") +
			st(cMuted, false).Render(" · ") + st(cText, false).Render(comma(s.Symbols)+" symbols")
	}
	return spread(m.w, left, right) + "\n" + st(cBorder, false).Render(strings.Repeat("─", m.w))
}

func (m Model) cardsView() string {
	s := m.snap
	wds := splitEven(m.w-5*hGap, 6)
	cards := []string{
		card(wds[0], cBlue, "FILES INDEXED", comma(s.Files), "tree-sitter parsed"),
		card(wds[1], cGreen, "SYMBOLS", comma(s.Symbols), "defs + refs"),
		card(wds[2], cViolet, "MEMORIES", comma(s.Memories), memSub(s.MemCounts)),
		card(wds[3], cCyan, "EPICS", comma(len(s.Epics)), fmt.Sprintf("%d active", s.EpicsActive())),
		card(wds[4], cYellow, "TASKS", comma(len(s.Tasks)), fmt.Sprintf("%d open", s.TasksOpen())),
		card(wds[5], cPink, "GRAPH EDGES", comma(s.Edges), "imports + tests"),
	}
	return joinH(cards...)
}

func (m Model) midView() string {
	d := m.dims()
	epicsBox := panel(d.epicsW, d.midH, "EPICS",
		fmt.Sprintf("%d total", len(m.snap.Epics)), m.epics.View(), m.focus == focusEpics)

	title := "TASKS"
	if e, ok := m.selectedEpic(); ok {
		title = "TASKS — " + e.Title
	}
	tasksBox := panel(d.tasksW, d.midH, title,
		fmt.Sprintf("%d", len(m.curTasks)), m.tasks.View(), m.focus == focusTasks)
	return joinH(epicsBox, tasksBox)
}

func (m Model) botView() string {
	d := m.dims()
	memBox := panel(d.memW, d.botH, "MEMORY",
		fmt.Sprintf("%d entries", m.snap.Memories), m.mem.View(), m.focus == focusMemory)
	graphBox := panel(d.graphW, d.botH, "GRAPH — top imported files",
		"by in-degree", m.graph.View(), m.focus == focusGraph)
	return joinH(memBox, graphBox)
}

func (m Model) footerView() string {
	return lipgloss.NewStyle().Padding(0, 1).Render(m.help.View(m.keys))
}

func memSub(counts map[string]int) string {
	if len(counts) == 0 {
		return "durable knowledge"
	}
	parts := make([]string, 0, len(counts))
	for _, k := range []string{"decision", "pattern", "gotcha", "reference", "constraint"} {
		if n := counts[k]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, k))
		}
	}
	if len(parts) == 0 {
		return "durable knowledge"
	}
	return strings.Join(parts, " · ")
}

// ---- table columns/rows ----

func epicColumns(inner int) []table.Column {
	nameW := inner - 4 - 14 - 6 - 11
	if nameW < 6 {
		nameW = 6
	}
	return []table.Column{
		{Title: "#", Width: 4},
		{Title: "Epic", Width: nameW},
		{Title: "Status", Width: 14},
		{Title: "Tasks", Width: 6},
		{Title: "Progress", Width: 11},
	}
}

func epicTableRows(epics []EpicRow, inner int) []table.Row {
	nameW := inner - 4 - 14 - 6 - 11
	if nameW < 6 {
		nameW = 6
	}
	rows := make([]table.Row, 0, len(epics))
	for _, e := range epics {
		rows = append(rows, table.Row{
			strings.TrimPrefix(e.IDStr, "epic_"),
			truncate(e.Title, nameW),
			statusBadge(e.Status),
			fmt.Sprintf("%d/%d", e.Done, e.Total),
			bar(e.Progress(), 9, cBar),
		})
	}
	return rows
}

func taskColumns(inner int) []table.Column {
	nameW := inner - 5 - 14
	if nameW < 6 {
		nameW = 6
	}
	return []table.Column{
		{Title: "#", Width: 5},
		{Title: "Task", Width: nameW},
		{Title: "Status", Width: 14},
	}
}

func taskTableRows(tasks []TaskRow, inner int) []table.Row {
	nameW := inner - 5 - 14
	if nameW < 6 {
		nameW = 6
	}
	rows := make([]table.Row, 0, len(tasks))
	for _, t := range tasks {
		rows = append(rows, table.Row{
			strings.TrimPrefix(t.IDStr, "task_"),
			truncate(t.Title, nameW),
			statusBadge(t.Status),
		})
	}
	return rows
}

func colW(n int) int {
	if n < 6 {
		return 6
	}
	return n
}

func memColumns(inner int) []table.Column {
	kindW := 10
	return []table.Column{
		{Title: "Kind", Width: kindW},
		{Title: "Title", Width: colW(inner - kindW)},
	}
}

func memTableRows(mems []MemRow, inner int) []table.Row {
	kindW := 10
	titleW := colW(inner - kindW)
	rows := make([]table.Row, 0, len(mems))
	for _, mr := range mems {
		rows = append(rows, table.Row{
			st(kindColor(mr.Kind), false).Render(strings.ToUpper(mr.Kind)),
			truncate(mr.Title, titleW),
		})
	}
	return rows
}

func graphColumns(inner int) []table.Column {
	impW := 16
	return []table.Column{
		{Title: "File", Width: colW(inner - impW)},
		{Title: "Imports", Width: impW},
	}
}

func graphTableRows(hubs []HubRow, inner int) []table.Row {
	impW := 16
	fileW := colW(inner - impW)
	maxIn := 1
	for _, h := range hubs {
		if h.In > maxIn {
			maxIn = h.In
		}
	}
	rows := make([]table.Row, 0, len(hubs))
	for _, h := range hubs {
		imp := bar(float64(h.In)/float64(maxIn), 9, cPink) +
			padL(st(cMuted, false).Render(fmt.Sprintf("%d", h.In)), impW-9)
		rows = append(rows, table.Row{truncate(h.Path, fileW), imp})
	}
	return rows
}

// ---- detail markdown (rendered by glamour) ----

func memMarkdown(mr MemRow) string {
	return fmt.Sprintf("# %s\n\n**Kind:** %s  ·  **ID:** `%s`\n",
		mr.Title, statusLabel(mr.Kind), mr.ID)
}

func graphMarkdown(h HubRow) string {
	return fmt.Sprintf("# %s\n\n**Imported by:** %d files (in-degree)\n", h.Path, h.In)
}

func epicMarkdown(e EpicRow, s Snapshot) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", e.Title)
	fmt.Fprintf(&b, "- **ID:** `%s`\n- **Status:** %s\n- **Progress:** %d / %d tasks done\n\n",
		e.IDStr, statusLabel(e.Status), e.Done, e.Total)
	tasks := s.TasksForEpic(e.ID)
	if len(tasks) > 0 {
		b.WriteString("## Tasks\n\n")
		for _, t := range tasks {
			fmt.Fprintf(&b, "- `%s` %s — _%s_\n", t.IDStr, t.Title, statusLabel(t.Status))
		}
	}
	return b.String()
}

func taskMarkdown(t TaskRow, s Snapshot) string {
	epic := "—"
	for _, e := range s.Epics {
		if e.ID == t.EpicID {
			epic = e.IDStr + " · " + e.Title
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", t.Title)
	fmt.Fprintf(&b, "- **ID:** `%s`\n- **Status:** %s\n- **Epic:** %s\n",
		t.IDStr, statusLabel(t.Status), epic)
	return b.String()
}

// renderResults formats global search hits as a colored, aligned list.
func renderResults(hits []SearchHit, width int) string {
	if len(hits) == 0 {
		return st(cMuted, false).Render("no matches")
	}
	tagW := 8
	whereW := width / 3
	titleW := width - tagW - whereW - 2
	if titleW < 8 {
		titleW = 8
	}
	var b strings.Builder
	for _, h := range hits {
		tag := padR(st(grainColor(h.Grain), false).Render(strings.ToUpper(h.Grain)), tagW)
		b.WriteString(tag + " " + cell(h.Title, titleW, cText) + " " +
			st(cMuted, false).Render(truncate(h.Where, whereW)) + "\n")
	}
	return b.String()
}

func grainColor(grain string) lipgloss.Color {
	switch grain {
	case "memory":
		return cViolet
	case "epic":
		return cCyan
	case "task":
		return cYellow
	case "symbol":
		return cBlue
	case "file":
		return cGreen
	default:
		return cMuted
	}
}

func modal(w, h int, title, body string) string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(cViolet).Padding(0, 1).
		Width(min(w-4, 100)).
		Render(st(cBright, true).Render(title) + "\n\n" + body)
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box)
}
