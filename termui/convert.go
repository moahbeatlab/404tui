// Package termui shows the handheld's screens (package handheld) in a terminal and plays them from the keyboard: the
// same screens, the same layout, on any size of window. A frame is drawn as pixels by the app; here the letters stay
// letters and everything else is turned into block, half-block and braille characters.
package termui

import (
	"fmt"
	"image"
	"image/color"
	"math/bits"
	"strings"

	"github.com/moahbeatlab/404tui/handheld"
)

// Cell size in the app's pixels.
const (
	cellW, cellH = handheld.CellW, handheld.CellH
	npix         = cellW * cellH
)

// Profile is how many colours the terminal has.
type Profile int

const (
	TrueColor Profile = iota
	ANSI256
)

// Glyphs says which characters may be used for what is not a letter.
type Glyphs struct {
	Braille  bool // dots, for waveforms and thin lines
	Eighths  bool // bars in eighths of a cell, for meters and frames
	Quadrant bool // quarter blocks
}

// FullGlyphs is every character below; BasicGlyphs only space, the full block and the half blocks, for a font that has
// little.
var (
	FullGlyphs  = Glyphs{Braille: true, Eighths: true, Quadrant: true}
	BasicGlyphs = Glyphs{}
)

// mask is the pixels of a cell that a character covers, a bit for each of the 8x16.
type mask [2]uint64

func (m *mask) set(x, y int) { i := y*cellW + x; m[i/64] |= 1 << (i % 64) }
func (m mask) get(x, y int) bool {
	i := y*cellW + x
	return m[i/64]>>(i%64)&1 == 1
}

func (m mask) xor(o mask) int { return bits.OnesCount64(m[0]^o[0]) + bits.OnesCount64(m[1]^o[1]) }

type shape struct {
	r    rune
	m    mask
	kind int // 0 plain, 1 eighths, 2 quadrants
}

var shapes []shape

func rectMask(x0, y0, x1, y1 int) (m mask) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			m.set(x, y)
		}
	}
	return
}

func orMask(a, b mask) mask { return mask{a[0] | b[0], a[1] | b[1]} }

func init() {
	add := func(r rune, m mask, kind int) { shapes = append(shapes, shape{r, m, kind}) }
	add(' ', mask{}, 0)
	add('█', rectMask(0, 0, 8, 16), 0)
	add('▀', rectMask(0, 0, 8, 8), 0)
	add('▄', rectMask(0, 8, 8, 16), 0)
	add('▌', rectMask(0, 0, 4, 16), 0)
	add('▐', rectMask(4, 0, 8, 16), 0)
	// lower bars in eighths of the height (2 px), left bars in eighths of the width (1 px)
	for i, r := range []rune("▁▂▃▅▆▇") {
		k := []int{1, 2, 3, 5, 6, 7}[i]
		add(r, rectMask(0, 16-2*k, 8, 16), 1)
	}
	for i, r := range []rune("▏▎▍▋▊▉") {
		k := []int{1, 2, 3, 5, 6, 7}[i]
		add(r, rectMask(0, 0, k, 16), 1)
	}
	add('▔', rectMask(0, 0, 8, 2), 1)
	add('▕', rectMask(7, 0, 8, 16), 1)
	// quarters: upper left, upper right, lower left, lower right
	ul, ur, ll, lr := rectMask(0, 0, 4, 8), rectMask(4, 0, 8, 8), rectMask(0, 8, 4, 16), rectMask(4, 8, 8, 16)
	add('▘', ul, 2)
	add('▝', ur, 2)
	add('▖', ll, 2)
	add('▗', lr, 2)
	add('▚', orMask(ul, lr), 2)
	add('▞', orMask(ur, ll), 2)
	add('▙', orMask(orMask(ul, ll), lr), 2)
	add('▛', orMask(orMask(ul, ur), ll), 2)
	add('▜', orMask(orMask(ul, ur), lr), 2)
	add('▟', orMask(orMask(ur, ll), lr), 2)
}

// brailleRune is the braille character with the dots on in d: d[col][row], 2 columns and 4 rows.
func brailleRune(d [2][4]bool) rune {
	// dot numbers: column 0 rows 0..2 are 1,2,3, column 1 rows 0..2 are 4,5,6, row 3 is 7 and 8
	bit := [2][4]uint{{0, 1, 2, 6}, {3, 4, 5, 7}}
	var v rune
	for c := 0; c < 2; c++ {
		for r := 0; r < 4; r++ {
			if d[c][r] {
				v |= 1 << bit[c][r]
			}
		}
	}
	return 0x2800 + v
}

func pack(c color.RGBA) uint32 { return uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B) }
func unpack(v uint32) color.RGBA {
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}
}

func dist(a, b uint32) int {
	dr, dg, db := int(a>>16&255)-int(b>>16&255), int(a>>8&255)-int(b>>8&255), int(a&255)-int(b&255)
	return dr*dr + dg*dg + db*db
}

// Cell is a character with its colours.
type Cell struct {
	R      rune
	Fg, Bg color.RGBA
	Bold   bool
}

// cellOf works out the character for the cell at (cx, cy): the letter when the app drew one, else the character that
// looks most like the pixels.
func cellOf(img *image.RGBA, cx, cy int, g handheld.Glyph, gl Glyphs) Cell {
	var px [npix]uint32
	type count struct {
		c uint32
		n int
	}
	var cs [12]count
	nc := 0
	for y := 0; y < cellH; y++ {
		o := (cy*cellH+y)*img.Stride + cx*cellW*4
		for x := 0; x < cellW; x++ {
			v := uint32(img.Pix[o])<<16 | uint32(img.Pix[o+1])<<8 | uint32(img.Pix[o+2])
			o += 4
			px[y*cellW+x] = v
			found := false
			for i := 0; i < nc; i++ {
				if cs[i].c == v {
					cs[i].n++
					found = true
					break
				}
			}
			if !found {
				if nc < len(cs) {
					cs[nc] = count{v, 1}
					nc++
				} else { // too many colours (a gradient): count it with the nearest
					best := 0
					for i := 1; i < nc; i++ {
						if dist(cs[i].c, v) < dist(cs[best].c, v) {
							best = i
						}
					}
					cs[best].n++
				}
			}
		}
	}
	bi := 0
	for i := 1; i < nc; i++ {
		if cs[i].n > cs[bi].n {
			bi = i
		}
	}
	bg := cs[bi].c
	if g.R != 0 {
		return Cell{R: g.R, Fg: g.Fg, Bg: unpack(bg), Bold: g.Bold}
	}
	if nc == 1 {
		return Cell{R: ' ', Fg: unpack(bg), Bg: unpack(bg)}
	}
	// the colour that differs most from the ground, by how many pixels it has
	fi, fs := -1, -1
	for i := 0; i < nc; i++ {
		if i == bi {
			continue
		}
		if s := cs[i].n * (1 + dist(cs[i].c, bg)/64); s > fs {
			fi, fs = i, s
		}
	}
	fg := cs[fi].c
	var m mask
	on := 0
	for y := 0; y < cellH; y++ {
		for x := 0; x < cellW; x++ {
			v := px[y*cellW+x]
			if dist(v, fg) < dist(v, bg) {
				m.set(x, y)
				on++
			}
		}
	}
	// the block character that covers the most of it (or its opposite, with the colours swapped)
	best, bestErr, bestInv := rune(' '), 1<<30, false
	for _, s := range shapes {
		if (s.kind == 1 && !gl.Eighths) || (s.kind == 2 && !gl.Quadrant) {
			continue
		}
		if e := s.m.xor(m); e < bestErr {
			best, bestErr, bestInv = s.r, e, false
		}
		inv := mask{^s.m[0], ^s.m[1]}
		if e := inv.xor(m); e < bestErr {
			best, bestErr, bestInv = s.r, e, true
		}
	}
	if gl.Eighths && bestErr > 10 {
		if cell, ok := skyline(m, fg, bg); ok {
			return cell
		}
	}
	if gl.Braille && bestErr > 10 {
		var d [2][4]bool
		any := false
		for c := 0; c < 2; c++ {
			for r := 0; r < 4; r++ {
				n := 0
				for y := 0; y < 4; y++ {
					for x := 0; x < 4; x++ {
						if m.get(c*4+x, r*4+y) {
							n++
						}
					}
				}
				d[c][r] = n >= 3
				any = any || d[c][r]
			}
		}
		if any {
			return Cell{R: brailleRune(d), Fg: unpack(fg), Bg: unpack(bg)}
		}
	}
	if bestInv {
		return Cell{R: best, Fg: unpack(bg), Bg: unpack(fg)}
	}
	return Cell{R: best, Fg: unpack(fg), Bg: unpack(bg)}
}

// lowerBlocks are the bars from the bottom in eighths of the height: 1..8.
var lowerBlocks = []rune("▁▂▃▄▅▆▇█")

// skyline is a waveform's cell: columns of colour that all start at the bottom of the cell (or all at the top) and go up
// (down) by different amounts. It is drawn as one bar as high as the strong columns, which is how a waveform reads in
// a terminal; thin lines and anything else are left to braille.
func skyline(m mask, fg, bg uint32) (Cell, bool) {
	var n [cellW]int
	cols, fromBottom, fromTop := 0, true, true
	for x := 0; x < cellW; x++ {
		top, bot, cnt := -1, -1, 0
		for y := 0; y < cellH; y++ {
			if m.get(x, y) {
				if top < 0 {
					top = y
				}
				bot = y
				cnt++
			}
		}
		if cnt == 0 {
			continue
		}
		if cnt != bot-top+1 { // not one run
			return Cell{}, false
		}
		cols++
		n[x] = cnt
		fromBottom = fromBottom && bot == cellH-1
		fromTop = fromTop && top == 0
	}
	if cols < 4 || (!fromBottom && !fromTop) {
		return Cell{}, false
	}
	// the height of the strong half of the columns
	var hs []int
	for _, v := range n {
		if v > 0 {
			hs = append(hs, v)
		}
	}
	for i := range hs {
		for j := i + 1; j < len(hs); j++ {
			if hs[j] > hs[i] {
				hs[i], hs[j] = hs[j], hs[i]
			}
		}
	}
	sum, k := 0, (len(hs)+1)/2
	for _, v := range hs[:k] {
		sum += v
	}
	eighths := min(max((sum/k+1)/2, 1), 8)
	if fromBottom {
		return Cell{R: lowerBlocks[eighths-1], Fg: unpack(fg), Bg: unpack(bg)}, true
	}
	if eighths == 8 {
		return Cell{R: '█', Fg: unpack(fg), Bg: unpack(fg)}, true
	}
	// from the top: the bar of what is left over, in the ground's colour on the bar's
	return Cell{R: lowerBlocks[8-eighths-1], Fg: unpack(bg), Bg: unpack(fg)}, true
}

// Cells turns a frame into its characters, row by row.
func Cells(f handheld.Frame, gl Glyphs) [][]Cell {
	out := make([][]Cell, f.Rows)
	for cy := 0; cy < f.Rows; cy++ {
		out[cy] = make([]Cell, f.Cols)
		for cx := 0; cx < f.Cols; cx++ {
			out[cy][cx] = cellOf(f.Base, cx, cy, f.Text[cy*f.Cols+cx], gl)
		}
	}
	return out
}

// Render is the frame as text with colour codes, a line for each row (no newline after the last).
func Render(f handheld.Frame, p Profile, gl Glyphs) string {
	return Draw(Cells(f, gl), p)
}

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
