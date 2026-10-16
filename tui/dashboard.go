package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/moahbeatlab/404tui/version"
)

// glyphs are the letters of the logo, five rows of blocks each.
var glyphs = map[rune][5]string{
	'4': {"█  █", "█  █", "████", "   █", "   █"},
	'0': {"████", "█  █", "█  █", "█  █", "████"},
	't': {" █  ", "████", " █  ", " █  ", " ███"},
	'u': {"█  █", "█  █", "█  █", "█  █", " ██ "},
	'i': {" █ ", "   ", " █ ", " █ ", " █ "},
}

// logoRows draws "404tui" in blocks: "404" in the accent, "tui" in the text colour, with a bright
// column that sweeps along it as the frames go by.
func (m Model) logoRows() []string {
	st := m.st
	word := []rune("404tui")
	var width int
	for _, r := range word {
		width += len(glyphs[r][0]) + 2
	}
	sweep := (m.frame * 2) % (width + 14) // the bright column; it runs past the end and starts over
	rows := make([]string, 5)
	for row := 0; row < 5; row++ {
		var b strings.Builder
		col := 0
		for i, r := range word {
			for _, c := range glyphs[r][row] {
				style := st.accent
				if i >= 3 {
					style = st.text
				}
				if d := col - sweep + 7; d >= 0 && d < 3 {
					style = lipgloss.NewStyle().Foreground(st.th.text).Bold(true)
				}
				if c == '█' {
					b.WriteString(style.Render("█"))
				} else {
					b.WriteString(" ")
				}
				col++
			}
			b.WriteString("  ")
			col += 2
		}
		rows[row] = strings.TrimRight(b.String(), " ")
	}
	return rows
}

// padGlow draws the 16 pads as small tiles: a light runs over them in the order a groove is
// played, and leaves a fading trail. The colours are the buses'.
func (m Model) padGlow() []string {
	st := m.st
	order := []int{0, 1, 2, 3, 7, 6, 5, 4, 8, 9, 10, 11, 15, 14, 13, 12}
	head := (m.frame / 2) % len(order)
	age := make([]int, 16)
	for i := range age {
		age[i] = 99
	}
	for step, pad := range order {
		age[pad] = (head - step + len(order)) % len(order)
	}
	var rows []string
	for r := 0; r < 4; r++ {
		var top, bottom strings.Builder
		for c := 0; c < 4; c++ {
			a := age[r*4+c]
			colour := st.th.tile
			switch {
			case a == 0:
				colour = st.th.accent
			case a == 1:
				colour = st.th.bus2
			case a == 2:
				colour = st.th.bus1
			case a < 6:
				colour = st.th.tileEmpty
			}
			tile := lipgloss.NewStyle().Background(colour).Render("      ")
			top.WriteString(tile + " ")
			bottom.WriteString(tile + " ")
		}
		rows = append(rows, strings.TrimRight(top.String(), " "), strings.TrimRight(bottom.String(), " "))
	}
	return rows
}

// dashboard is the screen shown while there is no unit to show yet, or while it is read: the name
// of the tool, a light running over the pads, and what is going on under them.
func (m Model) dashboard(status []string) string {
	st := m.st
	var lines []string
	if m.width >= 46 {
		lines = append(lines, m.logoRows()...)
	} else {
		lines = append(lines, st.accent.Render("404")+st.text.Render("tui"))
	}
	by := st.dim.Render("by ") + st.accent.Render(strings.Join(strings.Split("moahbeatlab", ""), " "))
	lines = append(lines, "", by, st.faint.Render("sample · slice · sequence  ·  SP-404MKII  ·  v"+version.Version))
	if m.height >= 34 {
		lines = append(lines, "")
		lines = append(lines, m.padGlow()...)
	}
	lines = append(lines, "")
	lines = append(lines, status...)
	width := 0
	for _, l := range lines {
		width = max(width, ansi.StringWidth(l))
	}
	centred := make([]string, len(lines))
	for i, l := range lines {
		centred[i] = lipgloss.PlaceHorizontal(width, lipgloss.Center, l)
	}
	return strings.Join(centred, "\n")
}
