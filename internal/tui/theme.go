package tui

import (
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// palette — a dark, violet-accented theme shared across the dashboard.
var (
	cBorder = lipgloss.Color("#2a2e3d")
	cText   = lipgloss.Color("#c8ccd8")
	cBright = lipgloss.Color("#eef0f6")
	cMuted  = lipgloss.Color("#5b6173")
	cTrack  = lipgloss.Color("#23283a")

	cViolet = lipgloss.Color("#a78bfa")
	cGreen  = lipgloss.Color("#4ade80")
	cYellow = lipgloss.Color("#fbbf24")
	cRed    = lipgloss.Color("#f87171")
	cBlue   = lipgloss.Color("#60a5fa")
	cCyan   = lipgloss.Color("#2dd4bf")
	cPink   = lipgloss.Color("#f472b6")
)

func st(c color.Color, bold bool) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(c).Bold(bold)
}

func selectionStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(cTrack).Background(cViolet).Bold(true)
}

func statusColor(s string) color.Color {
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

func kindColor(k string) color.Color {
	switch k {
	case "decision":
		return cViolet
	case "pattern":
		return cCyan
	case "failure":
		return cRed
	case "backlog":
		return cYellow
	case "command":
		return cBlue
	default: // glossary
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
	inner := w - 4 // box width (w-2) minus horizontal padding (1 each side)
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	if len(lines) > rows {
		lines = lines[:rows]
	}
	// Clamp every body line to the inner width so an over-wide table row can
	// never wrap and push past — i.e. "eat" — the panel's right border.
	for i, ln := range lines {
		if lipgloss.Width(ln) > inner {
			lines[i] = ansi.Truncate(ln, inner, "")
		}
	}
	titleColor := cBright
	border := cBorder
	if focused {
		titleColor, border = cViolet, cViolet
	}
	head := spread(w-4, st(titleColor, true).Render(title), st(cMuted, false).Render(meta))
	// In lipgloss v2 Width/Height are the TOTAL box size (border included), so we
	// pass the full w×h rather than subtracting the border as we did in v1.
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(border).
		Width(w).Height(h).Padding(0, 1).
		Render(head + "\n\n" + strings.Join(lines, "\n"))
}

func card(w int, accent color.Color, label, value, sub string) string {
	in := w - 4
	content := cell(label, in, cMuted) + "\n\n" +
		padR(st(accent, true).Render(truncate(value, in)), in) + "\n" +
		cell(sub, in, cMuted)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(cBorder).
		Width(w).Height(cardHeight).Padding(0, 1).
		Render(content)
}

// cardHeight is the total rendered height of a metric card (border + 4 lines).
const cardHeight = 6

// hGap is the number of blank columns between side-by-side boxes.
const hGap = 1

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

// ---- text/width helpers ----

func spread(width int, left, right string) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func cell(s string, w int, c color.Color) string {
	return padR(lipgloss.NewStyle().Foreground(c).Render(truncate(s, w)), w)
}

func padR(s string, w int) string {
	if gap := w - lipgloss.Width(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
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

func wrapText(s string, w int) []string {
	if w < 1 {
		return []string{""}
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}

	lines := make([]string, 0, len(words))
	line := ""
	for _, word := range words {
		for lipgloss.Width(word) > w {
			if line != "" {
				lines = append(lines, line)
				line = ""
			}
			var chunk string
			chunk, word = splitWord(word, w)
			lines = append(lines, chunk)
		}
		if line == "" {
			line = word
			continue
		}
		if lipgloss.Width(line)+1+lipgloss.Width(word) <= w {
			line += " " + word
			continue
		}
		lines = append(lines, line)
		line = word
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

func splitWord(s string, w int) (string, string) {
	runes := []rune(s)
	for i := 1; i <= len(runes); i++ {
		if lipgloss.Width(string(runes[:i])) > w {
			if i == 1 {
				return string(runes[:1]), string(runes[1:])
			}
			return string(runes[:i-1]), string(runes[i-1:])
		}
	}
	return s, ""
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
