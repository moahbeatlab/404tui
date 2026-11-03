package handheld

import (
	"fmt"
	"image/color"
	"strings"
)

// Cell is a character of the terminal screen with its colours.
type Cell struct {
	R      rune
	Fg, Bg color.RGBA
	Bold   bool
}

// The terminal's colours are those of the terminal program's "Hardware" theme: a dark ground, flat tiles, an amber accent.
var (
	tBg      = xterm(233)
	tPanel   = xterm(234)
	tTile    = xterm(236)
	tTileHi  = xterm(238)
	tLine    = xterm(240)
	tDim     = xterm(245)
	tText    = xterm(252)
	tAccent  = xterm(214)
	tAccentD = xterm(94) // the selected part of a waveform, a playing pad
	tDark    = xterm(232) // text on the accent
	tWhite   = xterm(231)
	tGood    = xterm(114)
	tBad     = xterm(203)
)

// xterm is a colour of the 256-colour palette as red, green, blue.
func xterm(n int) color.RGBA {
	switch {
	case n >= 232:
		v := uint8(8 + 10*(n-232))
		return color.RGBA{v, v, v, 255}
	case n >= 16:
		n -= 16
		lv := []uint8{0, 95, 135, 175, 215, 255}
		return color.RGBA{lv[n/36], lv[n/6%6], lv[n%6], 255}
	}
	return color.RGBA{128, 128, 128, 255}
}

// terminalColours says which of the terminal's colours stands for each colour of the palette the screens use.
var terminalColours = map[color.RGBA]color.RGBA{
	cBg: tBg, cPanel: tPanel, cPanel2: tTile, cLine: tLine, cDim: tDim, cText: tText,
	cAmber: tAccent, cAmberD: tAccentD, cWhite: tWhite, cGood: tGood, cBad: tBad,
}

func tcol(c color.RGBA) color.RGBA {
	if t, ok := terminalColours[c]; ok {
		return t
	}
	return c
}

// textGfx draws on a grid of characters.
type textGfx struct {
	cols, rows int
	cells      []Cell
	// clash says where a letter was written over a different one: two things in the same place (the tests look at it).
	clash []string
}

func newTextGfx() *textGfx {
	t := &textGfx{cols: Cols, rows: Rows, cells: make([]Cell, Cols*Rows)}
	for i := range t.cells {
		t.cells[i] = Cell{R: ' ', Fg: tText, Bg: tBg}
	}
	return t
}

func (t *textGfx) at(col, row int) *Cell {
	if col < 0 || row < 0 || col >= t.cols || row >= t.rows {
		return nil
	}
	return &t.cells[row*t.cols+col]
}

func (t *textGfx) Fill(col, row, cols, rows int, c color.RGBA) {
	bg := tcol(c)
	for y := row; y < row+rows; y++ {
		for x := col; x < col+cols; x++ {
			if cell := t.at(x, y); cell != nil {
				*cell = Cell{R: ' ', Fg: tText, Bg: bg}
			}
		}
	}
}

func (t *textGfx) Text(col, row int, s string, c color.RGBA, bold bool) {
	fg := tcol(c)
	for i, r := range []rune(s) {
		cell := t.at(col+i, row)
		if cell == nil {
			continue
		}
		if r == ' ' {
			continue
		}
		if cell.R != ' ' && cell.R != r && cell.R < 0x2500 { // a letter over a letter, not over a rule or a bar
			t.clash = append(t.clash, fmt.Sprintf("%q over %q at column %d, row %d (%q...)", r, cell.R, col+i, row, strings.TrimSpace(s)))
		}
		f := fg
		if cell.Bg == tAccent && f != tDark { // text on the accent is dark
			f = tDark
		}
		cell.R, cell.Fg, cell.Bold = r, f, bold
	}
}

func (t *textGfx) HRule(col, row, cols int, c color.RGBA) {
	for x := col; x < col+cols; x++ {
		if cell := t.at(x, row); cell != nil && cell.R == ' ' {
			cell.R, cell.Fg = '─', tcol(c)
		}
	}
}

func (t *textGfx) VRule(col, row, rows int, c color.RGBA) {
	for y := row; y < row+rows; y++ {
		if cell := t.at(col, y); cell != nil && cell.R == ' ' {
			cell.R, cell.Fg = '│', tcol(c)
		}
	}
}

// Frame is a rounded frame in the accent, as the terminal program's dialogs have.
func (t *textGfx) Frame(col, row, cols, rows int, c color.RGBA) {
	fg := tcol(c)
	put := func(x, y int, r rune) {
		if cell := t.at(x, y); cell != nil {
			cell.R, cell.Fg, cell.Bold = r, fg, false
		}
	}
	for x := col + 1; x < col+cols-1; x++ {
		put(x, row, '─')
		put(x, row+rows-1, '─')
	}
	for y := row + 1; y < row+rows-1; y++ {
		put(col, y, '│')
		put(col+cols-1, y, '│')
	}
	put(col, row, '╭')
	put(col+cols-1, row, '╮')
	put(col, row+rows-1, '╰')
	put(col+cols-1, row+rows-1, '╯')
}

// Highlight is the accent as the ground, with dark text: the selected row, as the terminal program shows its cursor.
func (t *textGfx) Highlight(col, row, cols, rows int) {
	t.Fill(col, row, cols, rows, cAmber)
}

// Cursor turns the tile into the accent with dark text and bars (what a waveform in it is drawn in).
func (t *textGfx) Cursor(col, row, cols, rows int) {
	for y := row; y < row+rows; y++ {
		for x := col; x < col+cols; x++ {
			if cell := t.at(x, y); cell != nil {
				cell.Bg, cell.Fg = tAccent, tDark
			}
		}
	}
}

var eighths = []rune(" ▏▎▍▌▋▊▉█")

func (t *textGfx) Bar(col, row, cols int, frac float64, fg, track color.RGBA) {
	n := min(max(frac, 0), 1) * float64(cols)
	for i := 0; i < cols; i++ {
		cell := t.at(col+i, row)
		if cell == nil {
			continue
		}
		f := min(max(n-float64(i), 0), 1)
		cell.R, cell.Fg, cell.Bg, cell.Bold = eighths[int(f*8+0.5)], tcol(fg), tcol(track), false
	}
}

// Fader is a heavy line in the accent over a thin line, the setting a dot on it, the default a small mark.
func (t *textGfx) Fader(col, row, cols int, frac, def float64, bipolar bool) {
	pos := min(int(frac*float64(cols)), cols-1)
	dpos := min(int(def*float64(cols)), cols-1)
	from, to := 0, pos
	if bipolar {
		from, to = min(dpos, pos), max(dpos, pos)
	}
	for i := 0; i < cols; i++ {
		cell := t.at(col+i, row)
		if cell == nil {
			continue
		}
		cell.R, cell.Fg, cell.Bold = '─', tLine, false
		if i >= from && i <= to {
			cell.R, cell.Fg = '━', tAccent
		}
		if i == dpos && i != pos {
			cell.R, cell.Fg = '╎', tDim
			if i >= from && i <= to {
				cell.Fg = tDark
				cell.Fg = tAccent
			}
		}
		if i == pos {
			cell.R, cell.Fg, cell.Bold = '●', tWhite, true
		}
	}
}

func (t *textGfx) BarsPerCell() int { return 2 }

// brailleBits are the bits of a braille character: [column][row] of its 2x4 dots.
var brailleBits = [2][4]uint{{0, 1, 2, 6}, {3, 4, 5, 7}}

// Bars draws values as braille dots: two columns of values to a cell, four dots high to a row.
func (t *textGfx) Bars(col, row, cols, rows int, h []float64, mirrored bool, c color.RGBA) {
	dots := rows * 4
	fg := tcol(c)
	for cx := 0; cx < cols; cx++ {
		for ry := 0; ry < rows; ry++ {
			var v rune
			for dc := 0; dc < 2; dc++ {
				i := cx*2 + dc
				if i >= len(h) {
					continue
				}
				for dr := 0; dr < 4; dr++ {
					y := ry*4 + dr // the dot row from the top
					var on bool
					if mirrored {
						ext := int(h[i]*float64(dots/2) + 0.5)
						mid := dots / 2
						on = y >= mid-max(ext, 1) && y < mid+max(ext, 1)
					} else {
						ext := int(h[i]*float64(dots) + 0.5)
						on = y >= dots-ext
					}
					if on {
						v |= 1 << brailleBits[dc][dr]
					}
				}
			}
			cell := t.at(col+cx, row+ry)
			if cell == nil || v == 0 {
				continue
			}
			cell.R, cell.Fg, cell.Bold = 0x2800+v, fg, false
		}
	}
}

func (t *textGfx) Mark(col, row, cols, rows int, x float64, c color.RGBA) {
	cx := col + min(int(x*float64(cols)), cols-1)
	for y := row; y < row+rows; y++ {
		if cell := t.at(cx, y); cell != nil {
			cell.R, cell.Fg, cell.Bold = '│', tcol(c), false
			if cell.Bg == tAccent {
				cell.Fg = tDark
			}
		}
	}
}

func (t *textGfx) Region(col, row, cols, rows int, x0, x1 float64) {
	if x1 <= x0 {
		return
	}
	from, to := col+int(x0*float64(cols)), col+min(int(x1*float64(cols)+0.999), cols)
	t.Fill(from, row, max(to-from, 1), rows, cAmberD)
}

// Badge is the letter, in the accent when it is on, dim between brackets of colour when off.
func (t *textGfx) Badge(col, row int, letter string, on bool) {
	if on {
		t.Fill(col, row, 2, 1, cAmber)
		t.Text(col, row, letter, cBg, true)
		return
	}
	t.Text(col, row, letter, cLine, false)
}

func (t *textGfx) Piano(col, row, cols int, lit map[int]bool) {
	kw := max((cols-2)/14, 2)
	x0 := col + 1
	white := []int{0, 2, 4, 5, 7, 9, 11}
	for i := 0; i < 14; i++ {
		s := white[i%7] + 12*(i/7)
		c := tText
		if lit[s] {
			c = tAccent
		}
		for r := 0; r < 3; r++ {
			for k := 0; k < kw-1; k++ {
				if cell := t.at(x0+i*kw+k, row+r); cell != nil {
					cell.R, cell.Fg, cell.Bg = ' ', tText, c
				}
			}
		}
	}
	blackAt := map[int]int{1: 0, 3: 1, 6: 3, 8: 4, 10: 5}
	for o := 0; o < 2; o++ {
		for s, k := range blackAt {
			c := tBg
			if lit[s+12*o] {
				c = tAccentD
			}
			bx := x0 + (k+7*o)*kw + kw - 1
			for r := 0; r < 2; r++ {
				if cell := t.at(bx, row+r); cell != nil {
					cell.R, cell.Fg, cell.Bg = ' ', tText, c
				}
			}
		}
	}
}

func (t *textGfx) StepCell(col, row, cols, rows int, bg color.RGBA) {
	w := cols
	if cols >= 3 {
		w = cols - 1 // a column between steps
	}
	t.Fill(col, row, w, rows, bg)
}

// Note is bars in the cell's columns: as high as it is loud (eighths of the cell's height), as wide as it lasts.
func (t *textGfx) Note(col, row, cols, rows int, start, length, vel float64, c color.RGBA) {
	w := cols
	if cols >= 3 {
		w = cols - 1
	}
	x0 := min(int(start*float64(w)), w-1)
	x1 := max(min(int((start+length)*float64(w)+0.999), w), x0+1)
	total := int(vel*float64(rows*8) + 0.5)
	total = max(total, 1)
	for x := x0; x < x1; x++ {
		for r := 0; r < rows; r++ { // from the bottom row up
			e := min(max(total-r*8, 0), 8)
			if e == 0 {
				continue
			}
			if cell := t.at(col+x, row+rows-1-r); cell != nil {
				cell.R, cell.Fg, cell.Bold = []rune(" ▁▂▃▄▅▆▇█")[e], tcol(c), false
			}
		}
	}
}

func (t *textGfx) Tie(col, row, cols, rows int, frac float64, c color.RGBA) {
	n := max(int(frac*float64(cols)+0.5), 1)
	for x := 0; x < n && x < cols; x++ {
		if cell := t.at(col+x, row+rows-1); cell != nil && cell.R == ' ' {
			cell.R, cell.Fg, cell.Bold = '▂', tcol(c), false
		}
	}
}

// Logo is the name in whole blocks of the accent.
func (t *textGfx) Logo(col, row int, s string, c color.RGBA) {
	for i, r := range s {
		for y, line := range logoFont[r] {
			for x, px := range line {
				if px == '1' {
					t.Fill(col+i*7+x*2, row+y, 2, 1, c)
				}
			}
		}
	}
}
