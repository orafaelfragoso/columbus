package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// palette — a dark, violet-accented theme shared across the dashboard.
var (
	cBorder = lipgloss.Color("#2a2e3d")
	cText   = lipgloss.Color("#c8ccd8")
	cBright = lipgloss.Color("#eef0f6")
	cMuted  = lipgloss.Color("#5b6173")
	cTrack  = lipgloss.Color("#23283a")

	cViolet = lipgloss.Color("#a78bfa")
	cBar    = lipgloss.Color("#8b5cf6")
	cGreen  = lipgloss.Color("#4ade80")
	cYellow = lipgloss.Color("#fbbf24")
	cRed    = lipgloss.Color("#f87171")
	cBlue   = lipgloss.Color("#60a5fa")
	cCyan   = lipgloss.Color("#2dd4bf")
	cPink   = lipgloss.Color("#f472b6")

	selBg = lipgloss.Color("#241d3d")
)

func st(c lipgloss.Color, bold bool) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(c).Bold(bold)
}

func statusColor(s string) lipgloss.Color {
	switch s {
	case "done":
		return cGreen
	case "in_progress":
		return cBlue
	case "blocked":
		return cRed
	case "todo":
		return cYellow
	default:
		return cMuted
	}
}

func statusBadge(s string) string {
	c := statusColor(s)
	return lipgloss.NewStyle().Foreground(c).Render("● " + statusLabel(s))
}

func kindColor(k string) lipgloss.Color {
	switch k {
	case "decision":
		return cViolet
	case "pattern":
		return cCyan
	case "gotcha":
		return cRed
	case "reference":
		return cBlue
	default:
		return cMuted
	}
}

// panel renders a rounded box (total w×h) with a title/meta header line, a blank
// spacer, and a pre-rendered body, clipping the body to the rows that fit.
func panel(w, h int, title, meta, body string, focused bool) string {
	rows := h - 2 - 2 // border (top+bottom), title line, spacer line
	if rows < 0 {
		rows = 0
	}
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	if len(lines) > rows {
		lines = lines[:rows]
	}
	titleColor := cBright
	border := cBorder
	if focused {
		titleColor, border = cViolet, cViolet
	}
	head := spread(w-4, st(titleColor, true).Render(title), st(cMuted, false).Render(meta))
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(border).
		Width(w-2).Height(h-2).Padding(0, 1).
		Render(head + "\n\n" + strings.Join(lines, "\n"))
}

func card(w int, accent lipgloss.Color, label, value, sub string) string {
	in := w - 4
	content := cell(label, in, cMuted) + "\n\n" +
		padR(st(accent, true).Render(truncate(value, in)), in) + "\n" +
		cell(sub, in, cMuted)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(cBorder).
		Width(w-2).Height(4).Padding(0, 1).
		Render(content)
}

// cardHeight is the total rendered height of a metric card (border + 4 lines).
const cardHeight = 6

// hGap is the number of blank columns between side-by-side boxes.
const hGap = 2

// joinH lays boxes out horizontally with hGap blank columns between them.
func joinH(boxes ...string) string {
	h := 0
	for _, b := range boxes {
		if hb := lipgloss.Height(b); hb > h {
			h = hb
		}
	}
	spacer := vspace(hGap, h)
	parts := make([]string, 0, len(boxes)*2-1)
	for i, b := range boxes {
		if i > 0 {
			parts = append(parts, spacer)
		}
		parts = append(parts, b)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func vspace(w, h int) string {
	line := strings.Repeat(" ", w)
	rows := make([]string, h)
	for i := range rows {
		rows[i] = line
	}
	return strings.Join(rows, "\n")
}

func bar(pct float64, width int, fg lipgloss.Color) string {
	if width < 1 {
		width = 1
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	fill := int(pct*float64(width) + 0.5)
	if fill > width {
		fill = width
	}
	return lipgloss.NewStyle().Foreground(fg).Render(strings.Repeat("█", fill)) +
		lipgloss.NewStyle().Foreground(cTrack).Render(strings.Repeat("█", width-fill))
}

// ---- text/width helpers ----

func spread(width int, left, right string) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func cell(s string, w int, c lipgloss.Color) string {
	return padR(lipgloss.NewStyle().Foreground(c).Render(truncate(s, w)), w)
}

func padR(s string, w int) string {
	if gap := w - lipgloss.Width(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

func padL(s string, w int) string {
	if gap := w - lipgloss.Width(s); gap > 0 {
		return strings.Repeat(" ", gap) + s
	}
	return s
}

func truncate(s string, w int) string {
	if w < 1 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > w {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

func comma(n int) string {
	s := strconv.Itoa(n)
	var out []byte
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	return string(out)
}

func splitEven(total, n int) []int {
	base, rem := total/n, total%n
	out := make([]int, n)
	for i := range out {
		out[i] = base
		if i < rem {
			out[i]++
		}
	}
	return out
}
