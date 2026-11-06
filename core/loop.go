package core

import (
	"fmt"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// The LOOP screen is a bank of loops that play together. The loops come from wherever you like (the unit's own looper copied
// to a pad, the slicer, a file); this screen keeps the bank and its pattern in order: the pattern in which all 16 pads of the bank
// start on beat 1 and last the whole pattern, a pad's sample deleted (twice, to be sure), a pad remixed: cut in equal slices that
// are played in a random order, put on the next free pad.

var loopSlices = []int{4, 8, 16, 32}

func (a *App) loopPad() int { return a.loopBank*16 + a.loopCur }

func (a *App) loopKey(k Key, repeat int) {
	switch k {
	case Up:
		a.loopCur = (a.loopCur + 12) % 16
		a.loopDelAsk = -1
	case Down:
		a.loopCur = (a.loopCur + 4) % 16
		a.loopDelAsk = -1
	case Left:
		a.loopCur = a.loopCur/4*4 + (a.loopCur%4+3)%4
		a.loopDelAsk = -1
	case Right:
		a.loopCur = a.loopCur/4*4 + (a.loopCur%4+1)%4
		a.loopDelAsk = -1
	case L1:
		a.loopBank = (a.loopBank + 9) % 10
		a.set.LooperBank = a.loopBank
	case R1:
		a.loopBank = (a.loopBank + 1) % 10
		a.set.LooperBank = a.loopBank
	case A:
		a.loopMakePattern()
	case X:
		a.loopDelete()
	case B:
		a.go_(Pads)
	}
}

func (a *App) loopShifted(k Key) {
	switch k {
	case X:
		a.loopRemix()
	case Up:
		a.loopSlicesAt = (a.loopSlicesAt + 1) % len(loopSlices)
		a.say(fmt.Sprintf("a remix cuts %d slices", loopSlices[a.loopSlicesAt]), false)
	case Down:
		a.loopSlicesAt = (a.loopSlicesAt + len(loopSlices) - 1) % len(loopSlices)
		a.say(fmt.Sprintf("a remix cuts %d slices", loopSlices[a.loopSlicesAt]), false)
	}
}

// loopBars is the length of the pattern of the bank in bars: that of the pattern with all its pads, else of the loops (2 when
// neither says).
func (a *App) loopBars() int {
	if b, err := a.lab.PatternBytes(a.loopBank*16 + 1); err == nil && len(b) > 0 {
		if n := patternBars(b); n > 0 {
			return n
		}
	}
	return 2
}

// loopMakePattern writes the pattern of the bank: every pad that has a sample starts on beat 1 and lasts the whole pattern.
func (a *App) loopMakePattern() {
	var pads []proto.PadIndex
	for i := 0; i < 16; i++ {
		if a.info.Pads[a.loopBank*16+i].File {
			p, _ := proto.NewPadIndex(uint16(a.loopBank*16 + i))
			pads = append(pads, p)
		}
	}
	if len(pads) == 0 {
		a.say("no loops in this bank", true)
		return
	}
	b, err := formats.StartPattern(pads, a.loopBars())
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	slot := a.loopBank*16 + 1
	pre := a.lab.Snapshot()
	if err := a.lab.SetPatternBytes(slot, b); err != nil {
		a.say(err.Error(), true)
		return
	}
	a.commitUndo(pre, A, 0)
	a.refresh()
	a.say(fmt.Sprintf("pattern %s: %d loops start together, %d bars", a.label(slot-1), len(pads), a.loopBars()), false)
}

// loopDelete takes a pad's sample out: it asks once, the second X does it.
func (a *App) loopDelete() {
	n := a.loopPad()
	if !a.info.Pads[n].File {
		a.say(a.label(n)+" is empty", true)
		return
	}
	if a.loopDelAsk != n {
		a.loopDelAsk = n
		a.say("X again deletes "+a.label(n)+" "+a.info.Pads[n].Name, true)
		return
	}
	a.loopDelAsk = -1
	pre := a.lab.Snapshot()
	if err := a.lab.ClearPad(n); err != nil {
		a.say(err.Error(), true)
		return
	}
	delete(a.pcm, n)
	a.commitUndo(pre, X, 0)
	a.refresh()
	a.say(a.label(n)+" deleted (its note stays in the pattern)", false)
}

// loopRemix cuts the loop of the pad in front into equal slices, plays them in a random order and puts the result on the next free
// pad of the bank.
func (a *App) loopRemix() {
	n := a.loopPad()
	if !a.info.Pads[n].File {
		a.say(a.label(n)+" is empty", true)
		return
	}
	free := -1
	for i := 1; i <= 16; i++ {
		p := a.loopBank*16 + (a.loopCur+i)%16
		if !a.info.Pads[p].File {
			free = p
			break
		}
	}
	if free < 0 {
		a.say("no free pad in this bank for the remix", true)
		return
	}
	pre := a.lab.Snapshot()
	oldBank, oldCur := a.bank, a.cur
	a.bank, a.cur = n/16, n%16
	a.openSlicer()
	if a.sl == nil {
		a.bank, a.cur = oldBank, oldCur
		return
	}
	a.equalParts(loopSlices[a.loopSlicesAt])
	a.shuffleOrder()
	a.resultTo(free)
	a.bank, a.cur = oldBank, oldCur
	a.sl = nil
	a.commitUndo(pre, X, 0)
	a.refresh()
}

func (a *App) drawLoop(g Gfx) {
	g.Text(2, 1, fmt.Sprintf("LOOPER  bank %c", 'A'+a.loopBank), cAmber, true)
	slot := a.loopBank*16 + 1
	status := "no pattern: A makes it"
	if b, err := a.lab.PatternBytes(slot); err == nil && len(b) > 0 {
		status = fmt.Sprintf("pattern %s  %d bars", a.label(slot-1), a.loopBars())
	}
	g.Text(contentCols-2-len(status), 1, status, cDim, false)
	cols := contentCols - 2
	pw := cols / 4
	x0 := 1 + (cols-4*pw)/2
	ph := max((Rows-6)/4, 3)
	tw, th := pw-1, ph-1
	cursor := [4]int{-1}
	for i := 0; i < 16; i++ {
		n := a.loopBank*16 + i
		x, y := x0+(i%4)*pw, 2+(i/4)*ph
		p := a.info.Pads[n]
		bg := cPanel
		if p.File {
			bg = cPanel2
		}
		g.Fill(x, y, tw, th, bg)
		num := fmt.Sprint(i + 1)
		g.Text(x+tw-len(num)-1, y, num, cDim, false)
		if p.File {
			g.Fill(x, y, 1, th, cAmber)
			g.Text(x+2, y, short(p.Name, pw-6), cText, true)
			if id, ok := a.padSample(n); ok && th >= 3 {
				a.wave(g, id, 0, p.Frames, x+1, y+1, tw-2, (ph-2)/2*2, cAmber)
			}
		}
		if i == a.loopCur {
			cursor = [4]int{x, y, tw, th}
		}
	}
	if cursor[0] >= 0 {
		g.Cursor(cursor[0], cursor[1], cursor[2], cursor[3])
	}
	g.Text(2, Rows-4, fmt.Sprintf("a remix cuts %d slices (Y+^v)", loopSlices[a.loopSlicesAt]), cDim, false)
}
