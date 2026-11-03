// Package termui shows the shared screens (package handheld) in a terminal and plays them from the keyboard: the same
// screens, the same layout, the same menus as on the handheld, on any size of window, drawn with the characters of a
// terminal (flat tiles, an amber accent, rounded dialogs, braille waveforms).
package terminal

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/moahbeatlab/404tui/core"
)

// Cell is a character with its colours.
type Cell = core.Cell

// Profile is how many colours the terminal has.
type Profile int

const (
	TrueColor Profile = iota
	ANSI256
)

// Glyphs says which characters may be used: all of them, or only those of an old console font (no braille, no eighths).
type Glyphs struct{ Full bool }

// FullGlyphs and BasicGlyphs are the two sets.
var (
	FullGlyphs  = Glyphs{Full: true}
	BasicGlyphs = Glyphs{}
)

// Cells is the screen as rows of cells, with the characters an old console font lacks replaced when the set is basic.
func Cells(s core.TextScreen, gl Glyphs) [][]Cell {
	out := make([][]Cell, s.Rows)
	for y := 0; y < s.Rows; y++ {
		out[y] = make([]Cell, s.Cols)
		for x := 0; x < s.Cols; x++ {
			c := s.Cells[y*s.Cols+x]
			if !gl.Full {
				c.R = basic(c.R)
			}
			out[y][x] = c
		}
	}
	return out
}

// basic is a character of the basic set that looks most like r.
func basic(r rune) rune {
	switch {
	case r >= 0x2800 && r <= 0x28ff: // braille: the dots of its upper two rows and of its lower two
		d := r - 0x2800
		up := int(d&1 + d>>1&1 + d>>3&1 + d>>4&1)
		dn := int(d>>2&1 + d>>5&1 + d>>6&1 + d>>7&1)
		switch {
		case up >= 2 && dn >= 2:
			return '█'
		case up >= 2:
			return '▀'
		case dn >= 2:
			return '▄'
		}
		return ' '
	case strings.ContainsRune("▁▂▃", r):
		return ' '
	case strings.ContainsRune("▅▆▇", r):
		return '▄'
	case strings.ContainsRune("▏▎▍", r):
		return ' '
	case strings.ContainsRune("▋▊▉", r):
		return '▌'
	case r == '╭' || r == '╮' || r == '╰' || r == '╯':
		return '+'
	case r == '━' || r == '─':
		return '-'
	case r == '│' || r == '╎':
		return '|'
	case r == '●':
		return 'o'
	}
	return r
}

// Render is the screen as text with colour codes, a line for each row (no newline after the last).
func Render(s core.TextScreen, p Profile, gl Glyphs) string { return Draw(Cells(s, gl), p) }

// Draw writes cells out with the colour codes of the profile.
func Draw(cells [][]Cell, p Profile) string {
	var sb strings.Builder
	for y, row := range cells {
		if y > 0 {
			sb.WriteByte('\n')
		}
		var fg, bg color.RGBA
		bold, first := false, true
		for _, c := range row {
			if c.R == ' ' || c.R == '█' {
				// the foreground does not show: keep the colours as they are when only the ground differs
				if c.R == ' ' {
					c.Fg = c.Bg
				} else {
					c.Bg = c.Fg
				}
			}
			if first || c.Bold != bold {
				if c.Bold {
					sb.WriteString("\x1b[1m")
				} else if !first {
					sb.WriteString("\x1b[22m")
				}
				bold = c.Bold
			}
			if first || c.Bg != bg {
				sb.WriteString(sgr(48, c.Bg, p))
				bg = c.Bg
			}
			if first || c.Fg != fg {
				sb.WriteString(sgr(38, c.Fg, p))
				fg = c.Fg
			}
			first = false
			sb.WriteRune(c.R)
		}
		sb.WriteString("\x1b[0m")
	}
	return sb.String()
}

func sgr(base int, c color.RGBA, p Profile) string {
	if p == TrueColor {
		return fmt.Sprintf("\x1b[%d;2;%d;%d;%dm", base, c.R, c.G, c.B)
	}
	return fmt.Sprintf("\x1b[%d;5;%dm", base, xterm256(c))
}

// xterm256 is the nearest colour of the 256-colour palette: the 6x6x6 cube or the grey ramp.
func xterm256(c color.RGBA) int {
	lvl := func(v uint8) int {
		switch {
		case v < 48:
			return 0
		case v < 115:
			return 1
		}
		return (int(v) - 35) / 40
	}
	r, g, b := lvl(c.R), lvl(c.G), lvl(c.B)
	cube := 16 + 36*r + 6*g + b
	steps := []int{0, 95, 135, 175, 215, 255}
	cr, cg, cb := steps[r], steps[g], steps[b]
	grey := (int(c.R) + int(c.G) + int(c.B)) / 3
	gi := min(max((grey-8+5)/10, 0), 23)
	gv := 8 + 10*gi
	dc := (int(c.R)-cr)*(int(c.R)-cr) + (int(c.G)-cg)*(int(c.G)-cg) + (int(c.B)-cb)*(int(c.B)-cb)
	dg := (int(c.R)-gv)*(int(c.R)-gv) + (int(c.G)-gv)*(int(c.G)-gv) + (int(c.B)-gv)*(int(c.B)-gv)
	if dg < dc {
		return 232 + gi
	}
	return cube
}
