package core

import (
	"image"
	"image/color"
	"image/draw"

	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/inconsolata"
	"golang.org/x/image/math/fixed"
)

// The screen is a grid of cells of 8x16 pixels: 80x30 on the R36S (640x480). In a terminal it is as many cells as the
// window has, so the same screens are drawn on any size; SetSize says which. Every screen lays itself out in cells.
const (
	CellW  = 8
	CellH  = 16
	margin = 16
	// MinCols and MinRows are the smallest screen the layouts are made for: the handheld's.
	MinCols, MinRows = 80, 30
)

var (
	W, H       = MinCols * CellW, MinRows * CellH
	Cols, Rows = MinCols, MinRows
)

// SetSize sets the screen to cols x rows cells (at least 80x30) and works the layout out again.
func SetSize(cols, rows int) {
	Cols, Rows = max(cols, MinCols), max(rows, MinRows)
	W, H = Cols*CellW, Rows*CellH
	layoutScreen()
}

func rgb(v uint32) color.RGBA { return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255} }

// The palette: a dark ground, one amber (the moahbeatlab colour) for what is active, and two greys.
var (
	cBg     = rgb(0x0b0d10)
	cPanel  = rgb(0x14181d)
	cPanel2 = rgb(0x1c222a)
	cLine   = rgb(0x2c343e)
	cDim    = rgb(0x6f7a87)
	cText   = rgb(0xd9dfe6)
	cAmber  = rgb(0xffb000)
	cAmberD = rgb(0x5c4200)
	cWhite  = rgb(0xffffff)
	cGood   = rgb(0x5fd38d)
	cBad    = rgb(0xff6b5e)
	cBlue   = rgb(0x5fafff) // the second colour of the slices
)

// A canvas is the frame as pixels: what the handheld shows (and what a screenshot is).
type canvas struct {
	img *image.RGBA
}

func newCanvas() *canvas {
	c := &canvas{img: image.NewRGBA(image.Rect(0, 0, W, H))}
	c.rect(0, 0, W, H, cBg)
	return c
}

func (c *canvas) rect(x, y, w, h int, col color.RGBA) {
	r := image.Rect(x, y, x+w, y+h).Intersect(c.img.Bounds())
	draw.Draw(c.img, r, image.NewUniform(col), image.Point{}, draw.Src)
}

// box draws an outline t pixels thick.
func (c *canvas) box(x, y, w, h, t int, col color.RGBA) {
	c.rect(x, y, w, t, col)
	c.rect(x, y+h-t, w, t, col)
	c.rect(x, y, t, h, col)
	c.rect(x+w-t, y, t, h, col)
}

// text draws s in the 8x16 face at pixel x, y; bold for emphasis.
func (c *canvas) text(x, y int, s string, col color.RGBA, bold bool) {
	face := inconsolata.Regular8x16
	if bold {
		face = inconsolata.Bold8x16
	}
	src := image.NewUniform(col)
	dot := fixed.P(x, y+face.Ascent)
	for _, r := range s {
		dr, mask, maskp, adv, ok := face.Glyph(dot, r)
		if ok {
			draw.DrawMask(c.img, dr, src, image.Point{}, mask, maskp, draw.Over)
		}
		dot.X += adv
	}
}

// logoFont is the letters of the logo, 3 pixels wide and 5 high.
var logoFont = map[rune][5]string{
	'4': {"1.1", "1.1", "111", "..1", "..1"},
	'0': {"111", "1.1", "1.1", "1.1", "111"},
	't': {".1.", "111", ".1.", ".1.", ".11"},
	'u': {"1.1", "1.1", "1.1", "1.1", "111"},
	'i': {".1.", "...", ".1.", ".1.", ".1."},
}

func textW(s string) int { return len(s) * CellW }

// fit cuts s to n characters, ending in ~ when it was longer.
func fit(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "~"
}

var _ = basicfont.Face7x13

// short cuts s to n characters keeping its end, where a number tells slices apart: "break_96bpm_03" is "break_~03".
func short(s string, n int) string {
	if len(s) <= n || n < 6 {
		return fit(s, n)
	}
	return s[:n-3] + "~" + s[len(s)-2:]
}

// short2 cuts a path to n characters keeping its end: "~r/a/b/samples".
func short2(s string, n int) string {
	if len(s) <= n || n < 4 {
		return fit(s, n)
	}
	return "~" + s[len(s)-(n-1):]
}
