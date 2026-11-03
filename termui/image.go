package termui

import (
	"image"
	"image/color"
	"image/draw"
	"strings"

	"golang.org/x/image/font/inconsolata"
	"golang.org/x/image/math/fixed"
)

// Image draws cells the way a terminal shows them: each cell 8x16 pixels, letters in a monospace face, the block
// characters as their shapes and braille as dots. It is for screenshots of what the terminal shows.
func Image(cells [][]Cell) *image.RGBA {
	rows := len(cells)
	cols := 0
	if rows > 0 {
		cols = len(cells[0])
	}
	img := image.NewRGBA(image.Rect(0, 0, cols*cellW, rows*cellH))
	shapeOf := map[rune]mask{}
	for _, s := range shapes {
		shapeOf[s.r] = s.m
	}
	for cy, row := range cells {
		for cx, c := range row {
			x0, y0 := cx*cellW, cy*cellH
			draw.Draw(img, image.Rect(x0, y0, x0+cellW, y0+cellH), image.NewUniform(c.Bg), image.Point{}, draw.Src)
			switch {
			case c.R == ' ' || c.R == 0:
			case c.R >= 0x2800 && c.R <= 0x28FF: // braille: a dot for each bit
				bit := [2][4]uint{{0, 1, 2, 6}, {3, 4, 5, 7}}
				for col := 0; col < 2; col++ {
					for r := 0; r < 4; r++ {
						if (c.R-0x2800)>>bit[col][r]&1 == 1 {
							draw.Draw(img, image.Rect(x0+col*4+1, y0+r*4+1, x0+col*4+3, y0+r*4+3), image.NewUniform(c.Fg), image.Point{}, draw.Src)
						}
					}
				}
			case strings.ContainsRune("│─━╭╮╰╯╎●┆", c.R):
				drawLine(img, x0, y0, c)
			default:
				if m, ok := shapeOf[c.R]; ok {
					for y := 0; y < cellH; y++ {
						for x := 0; x < cellW; x++ {
							if m.get(x, y) {
								img.SetRGBA(x0+x, y0+y, c.Fg)
							}
						}
					}
					continue
				}
				face := inconsolata.Regular8x16
				if c.Bold {
					face = inconsolata.Bold8x16
				}
				dot := fixed.P(x0, y0+face.Ascent)
				dr, mask, maskp, _, ok := face.Glyph(dot, c.R)
				if ok {
					draw.DrawMask(img, dr, image.NewUniform(c.Fg), image.Point{}, mask, maskp, draw.Over)
				}
			}
		}
	}
	return img
}

var _ color.RGBA

// drawLine draws the line and corner characters the screens use: a thin line through the middle of the cell, a heavier one,
// rounded corners as the two lines that meet, a dot.
func drawLine(img *image.RGBA, x0, y0 int, c Cell) {
	px := func(x, y int) { img.SetRGBA(x0+x, y0+y, c.Fg) }
	vert := func(from, to, w int) {
		for y := from; y < to; y++ {
			for d := 0; d < w; d++ {
				px(4-w/2+d, y)
			}
		}
	}
	horiz := func(from, to, w int) {
		for x := from; x < to; x++ {
			for d := 0; d < w; d++ {
				px(x, 8-w/2+d)
			}
		}
	}
	switch c.R {
	case '│', '┆':
		vert(0, cellH, 1)
	case '╎':
		for y := 2; y < cellH; y += 4 {
			vert(y, y+2, 1)
		}
	case '─':
		horiz(0, cellW, 1)
	case '━':
		horiz(0, cellW, 3)
	case '╭':
		horiz(4, cellW, 1)
		vert(8, cellH, 1)
	case '╮':
		horiz(0, 5, 1)
		vert(8, cellH, 1)
	case '╰':
		horiz(4, cellW, 1)
		vert(0, 9, 1)
	case '╯':
		horiz(0, 5, 1)
		vert(0, 9, 1)
	case '●':
		for y := 5; y < 11; y++ {
			for x := 1; x < 7; x++ {
				if (x-3)*(x-3)+(y-8)*(y-8) <= 9 {
					px(x, y)
				}
			}
		}
	}
}
