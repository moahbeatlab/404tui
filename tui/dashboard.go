package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/moahbeatlab/404tui/version"
)

// A font is the letters of the logo as rows of pixels; each pixel is drawn px cells wide, and
// gap cells are left between two letters. A cell is about twice as tall as it is wide, so two
// cells make a pixel a square.
type logoFont struct {
	glyphs map[rune][]string
	px     int
	gap    int
}

var (
	// bigFont is seven rows of five pixels, drawn two cells wide: thick, and wide.
	bigFont = logoFont{px: 2, gap: 4, glyphs: map[rune][]string{
		'4': {"█   █", "█   █", "█   █", "█████", "    █", "    █", "    █"},
		'0': {"█████", "█   █", "█   █", "█   █", "█   █", "█   █", "█████"},
		't': {"  █  ", "  █  ", "█████", "  █  ", "  █  ", "  █  ", "   ██"},
		'u': {"█   █", "█   █", "█   █", "█   █", "█   █", "█   █", " ███ "},
		'i': {"  █  ", "     ", "  █  ", "  █  ", "  █  ", "  █  ", "  █  "},
	}}
	// mediumFont is five rows of four pixels, two cells wide, for a narrower screen.
	mediumFont = logoFont{px: 2, gap: 3, glyphs: map[rune][]string{
		'4': {"█  █", "█  █", "████", "   █", "   █"},
		'0': {"████", "█  █", "█  █", "█  █", "████"},
		't': {" █  ", "████", " █  ", " █  ", " ███"},
		'u': {"█  █", "█  █", "█  █", "█  █", " ██ "},
		'i': {" █ ", "   ", " █ ", " █ ", " █ "},
	}}
	// smallFont is the same, one cell wide.
	smallFont = logoFont{px: 1, gap: 2, glyphs: mediumFont.glyphs}
)

// font is the biggest one the screen has room for, or false when it has room for none.
func (m Model) font() (logoFont, bool) {
	width := func(f logoFont) int {
		w := 0
		for _, r := range "404tui" {
			w += len([]rune(f.glyphs[r][0]))*f.px + f.gap
		}
		return w - f.gap
	}
	for _, f := range []logoFont{bigFont, mediumFont, smallFont} {
		if width(f)+4 <= m.width {
			return f, true
		}
	}
	return logoFont{}, false
}

// logoRows draws "404tui" in blocks: "404" in the accent and "tui" in bright text, both bold, with a
// light column that sweeps along it as the frames go by.
func (m Model) logoRows(f logoFont) []string {
	st := m.st
	word := []rune("404tui")
	rows := len(f.glyphs['4'])
	var total int
	for _, r := range word {
		total += len([]rune(f.glyphs[r][0]))*f.px + f.gap
	}
	sweep := (m.frame * 2) % (total + 16) // runs past the end, then starts over
	out := make([]string, rows)
	for row := 0; row < rows; row++ {
		var b strings.Builder
		col := 0
		for i, r := range word {
			for _, c := range f.glyphs[r][row] {
				style := st.accent
				if i >= 3 {
					style = lipgloss.NewStyle().Foreground(st.th.text).Bold(true)
				}
				for k := 0; k < f.px; k++ {
					if d := col - sweep + 8; d >= 0 && d < 3 {
						style = lipgloss.NewStyle().Foreground(st.th.hot).Bold(true)
					}
					if c == '█' {
						b.WriteString(style.Render("█"))
					} else {
						b.WriteString(" ")
					}
					col++
				}
			}
			b.WriteString(strings.Repeat(" ", f.gap))
			col += f.gap
		}
		out[row] = strings.TrimRight(b.String(), " ")
	}
	return out
}

// padGlow draws the 16 pads laid out as on the unit, 1 at the top left and 16 at the bottom
// right, each with its number: a light runs over them in the order a groove is played, and leaves
// a fading trail in the colours of the buses. The tiles are two rows tall, three with room.
func (m Model) padGlow() []string {
	st := m.st
	rowsHigh := 2
	if m.height >= 48 {
		rowsHigh = 3
	}
	order := []int{0, 1, 2, 3, 7, 6, 5, 4, 8, 9, 10, 11, 15, 14, 13, 12}
	head := (m.frame / 2) % len(order)
	age := make([]int, 16)
	for step, pad := range order {
		age[pad] = (head - step + len(order)) % len(order)
	}
	tile := func(pad, line int) string {
		colour, fg := st.th.tile, st.th.dim
		switch a := age[pad]; {
		case a == 0:
			colour, fg = st.th.accent, st.th.cursorFg
		case a == 1:
			colour, fg = st.th.bus2, st.th.cursorFg
		case a == 2:
			colour, fg = st.th.bus1, st.th.cursorFg
		case a < 6:
			colour, fg = st.th.tileEmpty, st.th.text
		}
		text := strings.Repeat(" ", 10)
		if line == 0 {
			text = fmt.Sprintf(" %-8d ", pad+1)
		}
		return lipgloss.NewStyle().Background(colour).Foreground(fg).Bold(true).Render(text)
	}
	var rows []string
	for r := 0; r < 4; r++ {
		for line := 0; line < rowsHigh; line++ {
			var b strings.Builder
			for c := 0; c < 4; c++ {
				b.WriteString(tile(r*4+c, line))
				if c < 3 {
					b.WriteString("  ")
				}
			}
			rows = append(rows, b.String())
		}
		if r < 3 {
			rows = append(rows, "")
		}
	}
	return rows
}

// dashboard is the screen shown while there is no unit to show yet, or while it is read: the name
// of the tool, a light running over the pads, and what is going on under them.
func (m Model) dashboard(status []string) string {
	st := m.st
	var lines []string
	if f, ok := m.font(); ok {
		lines = append(lines, m.logoRows(f)...)
	} else {
		lines = append(lines, st.accent.Render("404")+st.text.Render("tui"))
	}
	by := st.dim.Render("by ") + st.accent.Render(strings.Join(strings.Split("moahbeatlab", ""), " "))
	lines = append(lines, "", by, st.faint.Render("sample · slice · sequence  ·  SP-404MKII  ·  v"+version.Version))
	if m.height >= 40 {
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
