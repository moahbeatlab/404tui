package core

import "image/color"

// pixelGfx draws on a canvas of pixels: the handheld's screen, the web page and the screenshots.
type pixelGfx struct{ c *canvas }

func (p pixelGfx) Fill(col, row, cols, rows int, c color.RGBA) {
	p.c.rect(col*CellW, row*CellH, cols*CellW, rows*CellH, c)
}

func (p pixelGfx) Text(col, row int, s string, c color.RGBA, bold bool) {
	p.c.text(col*CellW, row*CellH, s, c, bold)
}

func (p pixelGfx) HRule(col, row, cols int, c color.RGBA) {
	p.c.rect(col*CellW, row*CellH, cols*CellW, 1, c)
}

func (p pixelGfx) VRule(col, row, rows int, c color.RGBA) {
	p.c.rect(col*CellW, row*CellH, 1, rows*CellH, c)
}

func (p pixelGfx) Frame(col, row, cols, rows int, c color.RGBA) {
	p.c.box(col*CellW, row*CellH, cols*CellW, rows*CellH, 2, c)
}

func (p pixelGfx) Highlight(col, row, cols, rows int) {
	p.c.rect(col*CellW, row*CellH, cols*CellW, rows*CellH, cAmberD)
	p.c.rect(col*CellW, row*CellH, 3, rows*CellH, cAmber)
}

func (p pixelGfx) Cursor(col, row, cols, rows int) {
	p.c.box(col*CellW, row*CellH, cols*CellW, rows*CellH, 2, cWhite)
}

func (p pixelGfx) Bar(col, row, cols int, frac float64, fg, track color.RGBA) {
	x, y, w := col*CellW, row*CellH, cols*CellW
	p.c.rect(x, y+8, w, 8, track)
	p.c.rect(x, y+8, int(frac*float64(w)), 8, fg)
}

func (p pixelGfx) Fader(col, row, cols int, frac, def float64, bipolar bool) {
	bx, y, bw := col*CellW, row*CellH, cols*CellW
	p.c.rect(bx, y+8, bw, 8, cPanel) // the bar stands on the lower half of the row
	if bipolar {                     // from the middle out
		mid, end := bx+int(def*float64(bw)), bx+int(frac*float64(bw))
		p.c.rect(min(mid, end), y+8, abs(end-mid)+1, 8, cAmber)
	} else {
		p.c.rect(bx, y+8, int(frac*float64(bw)), 8, cAmber)
	}
	p.c.rect(bx+int(def*float64(bw)), y, 2, CellH, cDim) // where the default is
	p.c.rect(min(bx+int(frac*float64(bw))-1, bx+bw-4), y, 4, CellH, cWhite)
}

func (p pixelGfx) BarsPerCell() int { return 4 }

func (p pixelGfx) Bars(col, row, cols, rows int, h []float64, mirrored bool, c color.RGBA) {
	x, y, w, hh := col*CellW, row*CellH, cols*CellW, rows*CellH
	mid := y + hh/2
	if mirrored {
		p.c.rect(x, mid, w, 1, cLine)
	}
	for i, hv := range h {
		if mirrored {
			v := int(hv * float64(hh/2))
			p.c.rect(x+2*i, mid-v, 2, 2*v+1, c)
		} else {
			v := int(hv * float64(hh))
			p.c.rect(x+2*i, y+hh-v, 2, v, c)
		}
	}
}

func (p pixelGfx) Mark(col, row, cols, rows int, x float64, c color.RGBA) {
	p.c.rect(col*CellW+int(x*float64(cols*CellW)), row*CellH, 1, rows*CellH, c)
}

func (p pixelGfx) Region(col, row, cols, rows int, x0, x1 float64) {
	w := float64(cols * CellW)
	if x1 > x0 {
		p.c.rect(col*CellW+int(x0*w), row*CellH, max(int((x1-x0)*w), 1), rows*CellH, cAmberD)
	}
}

func (p pixelGfx) Badge(col, row int, letter string, on bool) {
	x, y := col*CellW, row*CellH
	if on {
		p.c.rect(x, y, 14, 16, cAmber)
		p.c.text(x+3, y, letter, cBg, true)
		return
	}
	p.c.box(x, y, 14, 16, 1, cLine)
	p.c.text(x+3, y, letter, cLine, false)
}

func (p pixelGfx) Piano(col, row, cols int, lit map[int]bool) {
	x, y := col*CellW, row*CellH
	kw := (cols*CellW - 2*CellW) / 14
	top := y + 2
	x += CellW - 7
	p.c.rect(x+6, top, 14*kw+2, 40, cLine)
	white := []int{0, 2, 4, 5, 7, 9, 11}
	for i := 0; i < 14; i++ {
		s := white[i%7] + 12*(i/7)
		c := cText
		if lit[s] {
			c = cAmber
		}
		p.c.rect(x+7+i*kw, top+1, kw-2, 38, c)
	}
	blackAt := map[int]int{1: 0, 3: 1, 6: 3, 8: 4, 10: 5} // semitone -> white key to its left
	for o := 0; o < 2; o++ {
		for s, k := range blackAt {
			c := cBg
			if lit[s+12*o] {
				c = cAmberD
			}
			bx := x + 7 + (k+7*o)*kw + kw - 6
			p.c.rect(bx, top+1, 12, 23, c)
			if lit[s+12*o] {
				p.c.box(bx, top+1, 12, 23, 1, cAmber)
			}
		}
	}
}

func (p pixelGfx) StepCell(col, row, cols, rows int, bg color.RGBA) {
	p.c.rect(col*CellW, row*CellH, cols*CellW-2, rows*CellH-2, bg)
}

func (p pixelGfx) Note(col, row, cols, rows int, start, length, vel float64, c color.RGBA) {
	x, y, w, rh := col*CellW, row*CellH, cols*CellW-2, rows*CellH
	off := int(start * float64(w))
	nw := max(min(int(length*float64(w)), w-off), 2)
	h := max(int(float64(rh-2)*vel), 4) // as tall as it is loud
	p.c.rect(x+off, y+(rh-2)-h, nw, h, c)
}

func (p pixelGfx) Tie(col, row, cols, rows int, frac float64, c color.RGBA) {
	w := max(int(frac*float64(cols*CellW-2)), 2)
	p.c.rect(col*CellW, row*CellH+(rows*CellH-2)-4, w, 3, lerp(cAmberD, c, 0.7))
}

func (p pixelGfx) Logo(col, row int, s string, c color.RGBA) {
	for i, r := range s {
		for y, line := range logoFont[r] {
			for x, px := range line {
				if px == '1' {
					p.c.rect((col+i*7+x*2)*CellW, (row+y)*CellH, 2*CellW, CellH, c)
				}
			}
		}
	}
}
