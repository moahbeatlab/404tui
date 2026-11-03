//go:build !windows

package handheld

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
	pads    bool
	title   string
	needed  bool
	onPick  func(pad int)
	replace bool // the pads with a sample are taken over (a bounce), they are marked so
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
		a.openPattern(n)
		a.say(fmt.Sprintf("pattern %s", a.label(n-1)), false)
	}
}

func (a *App) drawPicker(c *canvas) {
	p := a.picker
	x0, y0, w, h := 16, 36, contentW-32, 388
	c.rect(x0, y0, w, h, cPanel2)
	c.box(x0, y0, w, h, 2, cAmber)
	head := "CHOOSE A PATTERN"
	if p.pads {
		head = strings.ToUpper(p.title)
	}
	c.text(x0+14, y0+10, head, cAmber, true)
	c.text(x0+w-14-textW("L1 R1 bank"), y0+10, "L1 R1 bank", cDim, false)
	if p.pads && p.replace {
		c.text(x0+14, y0+h-24, "a pad with a sample is replaced (Y+L2 undoes)", cDim, false)
	}
	// the banks
	for b := 0; b < 10; b++ {
		bx := x0 + 14 + b*41
		col, fg := cPanel, cDim
		if b == p.bank {
			col, fg = cAmber, cBg
		}
		c.rect(bx, y0+34, 37, 26, col)
		c.text(bx+14, y0+39, string(rune('A'+b)), fg, true)
	}
	// the 16 patterns
	const tw, th, gap = 98, 66, 6
	for i := 0; i < 16; i++ {
		n := p.bank*16 + i + 1
		if p.pads {
			n-- // the pads count from 0 in the lists, a pattern from 1
		}
		tx, ty := x0+14+(i%4)*(tw+gap), y0+74+(i/4)*(th+gap)
		bg := cPanel
		if p.info[i].used {
			bg = cPanel
			c.rect(tx, ty, tw, th, cPanel)
			c.rect(tx, ty, 4, th, cAmber)
		} else {
			c.rect(tx, ty, tw, th, bg)
		}
		fg := cDim
		if p.info[i].used {
			fg = cText
		}
		if p.pads {
			c.text(tx+12, ty+6, a.label(n), fg, true)
			if p.info[i].used {
				c.text(tx+12, ty+26, short(p.info[i].name, 10), cText, false)
				c.text(tx+12, ty+42, fmt.Sprintf("%.2f s", p.info[i].secs), cDim, false)
			} else {
				c.text(tx+12, ty+30, "empty", cLine, false)
			}
			if i == p.cur {
				c.box(tx-2, ty-2, tw+4, th+4, 2, cAmber)
			}
			continue
		}
		c.text(tx+12, ty+6, a.label(n-1), fg, true)
		if p.info[i].used {
			c.text(tx+12, ty+26, fmt.Sprintf("%d bars", p.info[i].bars), cDim, false)
			c.text(tx+12, ty+42, fmt.Sprintf("%d notes", p.info[i].notes), cAmber, false)
		} else {
			c.text(tx+12, ty+30, "empty", cLine, false)
		}
		if n == a.pat {
			c.box(tx+1, ty+1, tw-2, th-2, 1, cWhite)
		}
		if i == p.cur {
			c.box(tx-2, ty-2, tw+4, th+4, 2, cAmber)
		}
	}
}
