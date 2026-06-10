package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// layout holds the computed rectangle sizes for the current window.
type layout struct {
	memW, bodyH      int
	detailW, detailH int
}

func (m Model) dims() layout {
	w, h := m.w, m.h
	// Sections stack flush (no inter-section blank rows), so the body panel gets
	// everything left after the header, cards and footer.
	rem := h - 2 - cardHeight - lipgloss.Height(m.footerView())
	if rem < 8 {
		rem = 8
	}
	return layout{
		memW: max(12, w), bodyH: rem,
		// Clamp the overlay/detail box so a very narrow or short terminal can't
		// drive a zero/negative box width (which would break the compositor math).
		detailW: max(24, min(w-12, 96)), detailH: max(6, h*7/10),
	}
}

func (m Model) View() tea.View {
	if m.w == 0 || m.h == 0 {
		return altView("loading…")
	}
	dash := lipgloss.JoinVertical(lipgloss.Left,
		m.headerView(),
		m.cardsView(),
		m.bodyView(),
		m.footerView(),
	)

	switch {
	case m.confirmQuit:
		return altView(m.overlay(dash, m.quitBox()))
	case m.searchActive:
		return altView(m.overlay(dash, m.searchBox()))
	case m.showResults:
		return altView(m.overlay(dash, m.resultsBox()))
	case m.showDetail:
		return altView(m.overlay(dash, m.detailBox()))
	}
	return altView(dash)
}

// altView wraps content in a full-screen (alternate-buffer) tea.View.
func altView(content string) tea.View {
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// overlay composites a floating box centered over the dashboard background using
// lipgloss's Canvas/Layer compositor, so the dashboard stays visible behind the
// modal instead of being replaced by it.
func (m Model) overlay(bg, box string) string {
	bw, bh := lipgloss.Width(box), lipgloss.Height(box)
	x := max(0, (m.w-bw)/2)
	y := max(0, (m.h-bh)/2)
	comp := lipgloss.NewCompositor(
		lipgloss.NewLayer(bg).Z(0),
		lipgloss.NewLayer(box).X(x).Y(y).Z(1),
	)
	cv := lipgloss.NewCanvas(m.w, m.h)
	cv.Compose(comp)
	return cv.Render()
}

// box renders a rounded, violet-bordered modal box sized to hold text of the
// given inner width.
func box(title, body string, innerW int) string {
	// v2 Width is the total box width (border + padding included): innerW text +
	// 2 padding + 2 border.
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(cViolet).
		Padding(0, 1).Width(innerW + 4).
		Render(st(cBright, true).Render(title) + "\n\n" + body)
}

func (m Model) searchBox() string {
	d := m.dims()
	body := st(cMuted, false).Render("Semantic search across code and memory") + "\n\n" +
		m.searchInput.View() + "\n\n" +
		st(cMuted, false).Render("enter search · esc cancel")
	return box("Search", body, d.detailW)
}

func (m Model) resultsBox() string {
	title := fmt.Sprintf("Search — %d results", len(m.results.Items()))
	body := m.results.View() + "\n" + st(cMuted, false).Render("↑/↓ select · enter open · esc back")
	return box(title, body, m.dims().detailW)
}

func (m Model) detailBox() string {
	body := m.detail.View() + "\n\n" + st(cMuted, false).Render("esc back · ↑/↓ scroll")
	return box(m.detailTitle, body, m.dims().detailW)
}

func (m Model) quitBox() string {
	body := st(cText, false).Render("Quit Columbus?") + "\n\n" +
		st(cMuted, false).Render("y confirm · n cancel · esc cancel")
	return box("Confirm Quit", body, min(40, m.dims().detailW))
}

func (m Model) headerView() string {
	s := m.snap
	left := st(cViolet, true).Render("✦ Columbus")

	var status string
	switch {
	case m.reindexing:
		status = m.spin.View() + st(cViolet, false).Render(" reindexing…")
	case m.searching:
		status = m.spin.View() + st(cViolet, false).Render(" searching…")
	case m.loading:
		status = m.spin.View() + st(cMuted, false).Render(" loading…")
	case m.err != nil:
		status = st(cRed, false).Render("● error: " + m.err.Error())
	default:
		dirty, dc := "clean", cGreen
		if s.Dirty {
			dirty, dc = "dirty", cYellow
		}
		head := s.Head
		if head == "" {
			head = "(unindexed)"
		}
		status = st(dc, false).Render("● ") +
			st(cMuted, false).Render("indexed ") + st(cText, false).Render(head) +
			st(cMuted, false).Render(" · ") + st(dc, false).Render(dirty) +
			st(cMuted, false).Render(" · ") + st(cText, false).Render(comma(s.Files)+" files") +
			st(cMuted, false).Render(" · ") + st(cText, false).Render(comma(s.Symbols)+" symbols")
	}
	right := status
	if s.Branch != "" {
		branch := st(cMuted, false).Render("branch:") + st(cText, false).Render(s.Branch)
		if right != "" {
			right = branch + st(cMuted, false).Render(" · ") + right
		} else {
			right = branch
		}
	}
	return spread(m.w, left, right) + "\n" + st(cBorder, false).Render(strings.Repeat("─", m.w))
}

func (m Model) cardsView() string {
	s := m.snap
	const n = 4
	wds := splitEven(max(n, m.w-(n-1)*hGap), n)
	cards := []string{
		card(wds[0], cBlue, "FILES INDEXED", comma(s.Files), "tree-sitter parsed"),
		card(wds[1], cGreen, "SYMBOLS", comma(s.Symbols), "defs + refs"),
		card(wds[2], cPink, "EMBEDDINGS", comma(s.Embeddings), "semantic vectors"),
		card(wds[3], cViolet, "MEMORIES", comma(s.Memories), memSub(s.MemCounts)),
	}
	return joinH(cards...)
}

func (m Model) bodyView() string {
	d := m.dims()
	body := m.memoryTableView(d.memW-4, max(1, d.bodyH-5))
	return panel(d.memW, d.bodyH, "MEMORY",
		fmt.Sprintf("%d entries", m.snap.Memories), body, true)
}

func (m Model) memoryTableView(innerW, rows int) string {
	if innerW < 1 {
		innerW = 1
	}
	cols := memColumns(innerW)
	hasTags := len(cols) == 3
	kindW, titleW := cols[0].Width, cols[1].Width
	tagsW := 0
	header := cell(cols[0].Title, kindW, cMuted) + cell(cols[1].Title, titleW, cMuted)
	if hasTags {
		tagsW = cols[2].Width
		header += cell(cols[2].Title, tagsW, cMuted)
	}
	lines := []string{
		header,
		st(cBorder, false).Render(strings.Repeat("─", innerW)),
	}

	visibleRows := max(0, rows-len(lines))
	cursor := clampCursor(m.mem.Cursor(), len(m.snap.Mems))
	start := 0
	if visibleRows > 0 && cursor >= visibleRows {
		start = cursor - visibleRows + 1
	}
	end := min(len(m.snap.Mems), start+visibleRows)
	for i := start; i < end; i++ {
		mr := m.snap.Mems[i]
		kind := truncate(strings.ToUpper(mr.Kind), kindW)
		title := truncate(mr.Title, titleW)
		tags := truncate(strings.Join(mr.Tags, ","), tagsW)
		if i == cursor {
			row := padR(kind, kindW) + padR(title, titleW)
			if hasTags {
				row += padR(tags, tagsW)
			}
			lines = append(lines, selectionStyle().Render(row))
			continue
		}
		row := cell(kind, kindW, kindColor(mr.Kind)) + cell(title, titleW, cText)
		if hasTags {
			row += cell(tags, tagsW, cMuted)
		}
		lines = append(lines, row)
	}
	for len(lines) < rows {
		lines = append(lines, strings.Repeat(" ", innerW))
	}
	return strings.Join(lines, "\n")
}

func (m Model) footerView() string {
	var km help.KeyMap = m.keys
	switch {
	case m.confirmQuit:
		km = staticHelp{quitHelpKeys()}
	case m.searchActive:
		km = staticHelp{searchHelpKeys()}
	case m.showResults:
		km = staticHelp{resultsHelpKeys()}
	case m.showDetail:
		km = staticHelp{detailHelpKeys()}
	}
	return lipgloss.NewStyle().Padding(0, 1).Render(m.help.View(km))
}

func memSub(counts map[string]int) string {
	if len(counts) == 0 {
		return "durable memory"
	}
	parts := make([]string, 0, len(counts))
	for _, k := range []string{"adr", "plan", "documentation"} {
		if n := counts[k]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, k))
		}
	}
	if len(parts) == 0 {
		return "durable memory"
	}
	return strings.Join(parts, " · ")
}

// ---- table columns/rows ----

func colW(n int) int {
	if n < 6 {
		return 6
	}
	return n
}

const (
	memKindW = 10
	memTagsW = 16
	// memTagsMinInner is the smallest inner width that still leaves a readable
	// Title once Kind and Tags are subtracted; below it the Tags column is
	// dropped so the table degrades to Kind + Title on narrow panes.
	memTagsMinInner = memKindW + memTagsW + 12
)

// memHasTags reports whether the Tags column fits at the given inner width.
func memHasTags(inner int) bool { return inner >= memTagsMinInner }

func memColumns(inner int) []table.Column {
	cols := []table.Column{
		{Title: "Kind", Width: memKindW},
	}
	if memHasTags(inner) {
		cols = append(cols,
			table.Column{Title: "Title", Width: colW(inner - memKindW - memTagsW)},
			table.Column{Title: "Tags", Width: memTagsW},
		)
		return cols
	}
	return append(cols, table.Column{Title: "Title", Width: colW(inner - memKindW)})
}

func memTableRows(mems []MemRow, inner int) []table.Row {
	hasTags := memHasTags(inner)
	titleW := colW(inner - memKindW)
	if hasTags {
		titleW = colW(inner - memKindW - memTagsW)
	}
	rows := make([]table.Row, 0, len(mems))
	for _, mr := range mems {
		row := table.Row{
			st(kindColor(mr.Kind), false).Render(strings.ToUpper(mr.Kind)),
			truncate(mr.Title, titleW),
		}
		if hasTags {
			row = append(row, truncate(strings.Join(mr.Tags, ","), memTagsW))
		}
		rows = append(rows, row)
	}
	return rows
}

// ---- detail markdown (rendered by glamour) ----

func memMarkdown(mr MemRow) string {
	return fmt.Sprintf("# %s\n\n**Kind:** %s  ·  **ID:** `%s`\n",
		mr.Title, kindLabel(mr.Kind), mr.ID)
}

func grainColor(grain string) color.Color {
	switch grain {
	case "memory":
		return cViolet
	case "symbol":
		return cBlue
	case "file":
		return cGreen
	default:
		return cMuted
	}
}
