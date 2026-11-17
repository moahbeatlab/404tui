package handout

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"sort"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/moahbeatlab/404tui/formats"
)

// The step grids are pictures: an e-reader draws a picture the same everywhere, a table of coloured cells it may not. Grey levels are
// the ones an e-ink screen shows cleanly.
const (
	gridW   = 1320 // fits the 1404 pixels of an InkPad 3 with its margins
	labelW  = 360
	gapPx   = 6
	beatGap = 14
	headH   = 44
)

var (
	ink   = color.Gray{20}
	soft  = color.Gray{140}
	line  = color.Gray{190}
	dimTx = color.Gray{90}
	white = color.Gray{255}
)

// Accent is the velocity from which a note is drawn black; softer notes are grey.
const Accent = 100

var (
	fontOnce       sync.Once
	boldTT, monoTT *opentype.Font
	faceMu         sync.Mutex
	faces          = map[[2]int]font.Face{}
)

func face(mono bool, size int) font.Face {
	fontOnce.Do(func() {
		boldTT, _ = opentype.Parse(gobold.TTF)
		monoTT, _ = opentype.Parse(gomono.TTF)
	})
	faceMu.Lock()
	defer faceMu.Unlock()
	k := [2]int{map[bool]int{false: 0, true: 1}[mono], size}
	if f, ok := faces[k]; ok {
		return f
	}
	tt := boldTT
	if mono {
		tt = monoTT
	}
	f, _ := opentype.NewFace(tt, &opentype.FaceOptions{Size: float64(size), DPI: 72, Hinting: font.HintingFull})
	faces[k] = f
	return f
}

func text(img *image.Gray, x, y int, s string, f font.Face, c color.Gray) int {
	d := font.Drawer{Dst: img, Src: image.NewUniform(c), Face: f, Dot: fixed.P(x, y)}
	d.DrawString(s)
	return d.Dot.X.Round()
}

func width(s string, f font.Face) int { return font.MeasureString(f, s).Round() }

// fitText cuts s to w pixels, with an ellipsis when it had to.
func fitText(s string, f font.Face, w int) string {
	if width(s, f) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && width(string(r)+"…", f) > w {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

// cell is a step of a row: the loudest note that starts on it, or a note that started before and still sounds (held).
type cell struct {
	vel, semis int
	on, held   bool
	pitched    bool
}

// barCells sorts the notes of one bar into rows (the pads, in the order given) and steps (sixteenths). A note longer than a step is held
// over the steps after it, into the next bars too, up to the end of the pattern.
func barCells(pt Pattern, bar int, rows []int) [][]cell {
	steps := pt.Beats * 4
	stepTicks := uint32(formats.PPQ / 4)
	first, total := bar*steps, pt.Bars*steps
	row := map[int]int{}
	for i, pad := range rows {
		row[pad] = i
	}
	out := make([][]cell, len(rows))
	for i := range out {
		out[i] = make([]cell, steps)
	}
	for _, n := range pt.Notes {
		r, ok := row[n.Pad]
		if !ok {
			continue
		}
		start := min(int((n.Tick+stepTicks/2)/stepTicks), total-1) // late in the last step: it shows there
		end := min(int((n.Tick+uint32(max(n.Len, 1))+stepTicks-1)/stepTicks), total)
		vel := n.Vel
		for s := start + 1; s < end; s++ {
			if s >= first && s < first+steps {
				c := &out[r][s-first]
				if !c.on && (!c.held || vel > c.vel) {
					*c = cell{held: true, vel: vel}
				}
			}
		}
		if start < first || start >= first+steps {
			continue
		}
		c := &out[r][start-first]
		if !c.on || vel > c.vel {
			*c = cell{vel: vel, on: true, pitched: n.Pitch != 0, semis: formats.PitchSemitones(uint8(n.Pitch))}
		}
	}
	return out
}

// padsOf are the pads a pattern plays, in order.
func padsOf(pt Pattern) []int {
	seen := map[int]bool{}
	var out []int
	for _, n := range pt.Notes {
		if !seen[n.Pad] {
			seen[n.Pad] = true
			out = append(out, n.Pad)
		}
	}
	sort.Ints(out)
	return out
}

// BarPNG draws one bar of a pattern: the pads down the side, the sixteenths across, a black square for an accent, a grey one for a
// softer note, a bar through the steps a long note is held over, and the semitones on a note that is played chromatically.
func BarPNG(p Project, pt Pattern, bar int) ([]byte, error) {
	rows := padsOf(pt)
	cells := barCells(pt, bar, rows)
	steps := pt.Beats * 4
	cw := (gridW - labelW - (pt.Beats-1)*beatGap - (steps-1)*gapPx) / steps
	ch := min(cw, 64)
	h := headH + len(rows)*(ch+gapPx)
	img := image.NewGray(image.Rect(0, 0, gridW, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(white), image.Point{}, draw.Src)
	x0 := func(s int) int { return labelW + s*(cw+gapPx) + s/4*beatGap }
	num := face(true, 26)
	for b := 0; b < pt.Beats; b++ {
		text(img, x0(b*4)+4, headH-14, fmt.Sprint(b+1), num, dimTx)
	}
	lab := face(false, min(30, ch*55/100))
	padLab := face(true, min(24, ch*45/100))
	semi := face(false, min(24, cw*42/100))
	for r, pad := range rows {
		y := headH + r*(ch+gapPx)
		base := y + ch/2 + 10
		x := text(img, 0, base, p.Pads[pad].Label, padLab, dimTx)
		text(img, x+12, base, fitText(p.padName(pad), lab, labelW-(x+12)-16), lab, ink)
		for s := 0; s < steps; s++ {
			rc := image.Rect(x0(s), y, x0(s)+cw, y+ch)
			c := cells[r][s]
			fill := ink
			if c.vel < Accent {
				fill = soft
			}
			if !c.on {
				frame(img, rc, line, 2)
				if c.held { // the note before still sounds: a bar through the middle, joined to the step before
					left := rc.Min.X - gapPx
					if s%4 == 0 {
						left -= beatGap
					}
					if s == 0 {
						left = rc.Min.X
					}
					draw.Draw(img, image.Rect(left, y+ch*32/100, rc.Max.X, y+ch*68/100), image.NewUniform(fill), image.Point{}, draw.Src)
				}
				continue
			}
			draw.Draw(img, rc, image.NewUniform(fill), image.Point{}, draw.Src)
			if c.pitched {
				t := fmt.Sprintf("%+d", c.semis)
				if c.semis == 0 {
					t = "0"
				}
				text(img, rc.Min.X+(cw-width(t, semi))/2, rc.Min.Y+ch/2+8, t, semi, white)
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func frame(img *image.Gray, r image.Rectangle, c color.Gray, t int) {
	u := image.NewUniform(c)
	draw.Draw(img, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+t), u, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(r.Min.X, r.Max.Y-t, r.Max.X, r.Max.Y), u, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(r.Min.X, r.Min.Y, r.Min.X+t, r.Max.Y), u, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(r.Max.X-t, r.Min.Y, r.Max.X, r.Max.Y), u, image.Point{}, draw.Src)
}
