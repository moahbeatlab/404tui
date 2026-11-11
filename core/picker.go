package core

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/moahbeatlab/404tui/lab"
)

// The pattern picker (Y+B in SEQ): patterns are in banks A to J of 16, like the pads. L1 and R1 (or Left and Right at
// the edge of the grid) change the bank, the arrows choose the pattern in it, A opens it. Patterns with notes show how
// long they are and how many notes they have.
type picker struct {
	bank, cur int // the bank 0..9, the cursor 0..15
	info      [16]struct {
		used        bool
		bars, notes int
		name        string
		secs        float64
	}
	// a picker of pads (the same dialog everywhere a pad is chosen): the title, whether the pad must have a sample, and what
	// to do with the pad chosen
	pads   bool
	title  string
	needed bool
	onPick func(pad int)
	// onCancel is what B does besides closing it: a picker that came out of a menu goes back to it
	onCancel func()
	replace  bool // the pads with a sample are taken over (a bounce), they are marked so
}

// pickPad opens the pad picker: a bank at a time, the pads as they are on the PADS screen. start is the pad the cursor
// begins on. needSample says the pad must have a sample (a track for it); else the pad may be empty, or have a sample that is
// then replaced.
func (a *App) pickPad(title string, start int, needSample bool, onPick func(pad int)) {
	start = min(max(start, 0), 159)
	a.picker = &picker{bank: start / 16, cur: start % 16, pads: true, title: title, needed: needSample, onPick: onPick, replace: !needSample}
	a.pickerLoad()
}

// pickFreePad is pickPad on the first empty pad after from.
func (a *App) pickFreePad(title string, from int, onPick func(pad int)) {
	start := from
	if f := a.freeRun(1, from); f >= 0 {
		start = f
	}
	a.pickPad(title, start, false, onPick)
}

// pickPattern opens the pattern picker to choose a slot (1..160) for something: copy, move.
func (a *App) pickPattern(title string, start int, onPick func(slot int)) {
	a.picker = &picker{bank: (start - 1) / 16, cur: (start - 1) % 16, title: title, onPick: onPick}
	a.pickerLoad()
}

func (a *App) openPicker() {
	p := &picker{bank: (a.pat - 1) / 16, cur: (a.pat - 1) % 16}
	a.picker = p
	a.pickerLoad()
}

// pickerLoad reads what the 16 patterns of the bank in front hold.
func (a *App) pickerLoad() {
	p := a.picker
	for i := range p.info {
		if p.pads {
			pd := a.info.Pads[p.bank*16+i]
			p.info[i].used, p.info[i].name, p.info[i].secs = pd.File, pd.Name, float64(pd.Frames)/48000
			continue
		}
		p.info[i].used, p.info[i].bars, p.info[i].notes = false, 0, 0
		b, err := a.lab.Pattern(p.bank*16 + i + 1)
		var d lab.PatternDoc
		if err != nil || json.Unmarshal(b, &d) != nil {
			continue
		}
		p.info[i].used, p.info[i].bars, p.info[i].notes = true, d.Bars, len(d.Notes)
	}
}

func (a *App) pickerKey(k Key) {
	p := a.picker
	bank := func(d int) {
		p.bank = (p.bank + 10 + d) % 10
		a.pickerLoad()
	}
	switch k {
	case Up:
		p.cur = (p.cur + 12) % 16
	case Down:
		p.cur = (p.cur + 4) % 16
	case Left:
		if p.cur%4 == 0 { // the edge: the previous bank, from its right side
			bank(-1)
			p.cur += 3
		} else {
			p.cur--
		}
	case Right:
		if p.cur%4 == 3 {
			bank(1)
			p.cur -= 3
		} else {
			p.cur++
		}
	case L1:
		bank(-1)
	case R1:
		bank(1)
	case B:
		a.picker = nil
		if p.onCancel != nil {
			p.onCancel()
		}
	case X: // Space: hear the sample of the pad in front
		if p.pads {
			if n := p.bank*16 + p.cur; a.info.Pads[n].File {
				a.playPad(n)
			} else {
				a.say(a.label(n)+" has no sample", true)
			}
		}
	case A:
		if p.pads {
			n := p.bank*16 + p.cur
			if p.needed && !a.info.Pads[n].File {
				a.say(a.label(n)+" has no sample: choose a pad with one", true)
				return
			}
			a.picker = nil
			p.onPick(n)
			return
		}
		n := p.bank*16 + p.cur + 1
		a.picker = nil
		if p.onPick != nil {
			p.onPick(n)
			return
		}
		a.openPattern(n)
		a.say(fmt.Sprintf("pattern %s", a.label(n-1)), false)
	}
}

func (a *App) drawPicker(g Gfx) {
	p := a.picker
	wc, hr := contentCols-2, Rows-3 // the dialog in cells; a blank row at the top and bottom for the frame
	x0, y0 := 1, 1
	g.Fill(x0, y0, wc, hr, cPanel2)
	g.Frame(x0, y0, wc, hr, cAmber)
	head := "CHOOSE A PATTERN"
	if p.title != "" {
		head = strings.ToUpper(p.title)
	}
	g.Text(x0+2, y0+1, fit(head, wc-16), cAmber, true)
	bk := tk("L1 R1 bank")
	g.Text(x0+wc-2-len(bk), y0+1, bk, cDim, false)
	if p.pads && p.replace {
		g.Text(x0+2, y0+hr-2, fit("a pad with a sample is replaced (Y+L2 undoes)", wc-4), cDim, false)
	}
	// the banks
	bwc := (wc - 4) / 10
	for b := 0; b < 10; b++ {
		bx := x0 + 2 + b*bwc
		col, fg := cPanel, cDim
		if b == p.bank {
			col, fg = cAmber, cBg
		}
		g.Fill(bx, y0+3, bwc-1, 1, col)
		g.Text(bx+(bwc-1)/2, y0+3, string(rune('A'+b)), fg, true)
	}
	// the 16 patterns
	pw := (wc - 4) / 4
	ph := max((hr-8)/4, 3) // the rows left under the banks, less the line at the foot
	tw, th := pw-1, ph-1
	cursor := [4]int{-1}
	for i := 0; i < 16; i++ {
		n := p.bank*16 + i + 1
		if p.pads {
			n-- // the pads count from 0 in the lists, a pattern from 1
		}
		tx, ty := x0+2+(i%4)*pw, y0+5+(i/4)*ph
		g.Fill(tx, ty, tw, th, cPanel)
		if p.info[i].used {
			g.Fill(tx, ty, 1, th, cAmber)
		}
		fg := cDim
		if p.info[i].used {
			fg = cText
		}
		if p.pads {
			g.Text(tx+2, ty, a.label(n), fg, true)
			if p.info[i].used {
				g.Text(tx+2, ty+1, short(p.info[i].name, pw-4), cText, false)
				if th >= 3 {
					g.Text(tx+2, ty+2, fmt.Sprintf("%.2f s", p.info[i].secs), cDim, false)
				}
			} else {
				g.Text(tx+2, ty+1, "empty", cLine, false)
			}
		} else {
			g.Text(tx+2, ty, a.label(n-1), fg, true)
			if p.info[i].used {
				g.Text(tx+2, ty+1, fmt.Sprintf("%d bars", p.info[i].bars), cDim, false)
				if th >= 3 {
					g.Text(tx+2, ty+2, fmt.Sprintf("%d notes", p.info[i].notes), cAmber, false)
				}
			} else {
				g.Text(tx+2, ty+1, "empty", cLine, false)
			}
			if n == a.pat {
				g.Text(tx+tw-2, ty, "<", cWhite, true)
			}
		}
		if i == p.cur {
			cursor = [4]int{tx, ty, tw, th}
		}
	}
	if cursor[0] >= 0 {
		g.Cursor(cursor[0], cursor[1], cursor[2], cursor[3])
	}
}
