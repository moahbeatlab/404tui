package termui

import (
	"image"
	"image/color"
	"image/draw"

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
