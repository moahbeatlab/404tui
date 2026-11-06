package core

import (
	"sort"
	"sync"
	"time"

	"github.com/moahbeatlab/404tui/midiin"
	"github.com/moahbeatlab/404tui/perflog"
	"github.com/moahbeatlab/404tui/proto"
)

// The controller's pad group (the numbers 76 to 91 on the controller channel, beside the pattern group at 60 to 75): it works on the pads of the
// bank the controller has chosen. A **push** (Note On) of E1 to E16 mutes the pad 1 to 16 of that bank, or brings it back. A **turn** of the knob
// isolates it: every other pad of the bank is muted, so that this one is heard alone. A turn of another knob moves the isolation; the same knob
// turned again after a pause ends it. The unit does not know a mute from outside: the pad's level is set to 0 (and put back), like the old PERFORM mute.

const (
	ctlPadFirst = 76 // E1; the sixteenth is 91
	ctlPadLast  = 91
	isoPause    = 700 * time.Millisecond // a turn after this long is a new gesture
)

// ctlPads is what the pad group has done to the unit.
type ctlPads struct {
	mu    sync.Mutex
	muted map[int]bool  // pads muted by a push
	iso   int           // the pad that is isolated, or -1
	was   map[int]int32 // pad -> the level it had before it was set to 0
	last  time.Time     // the last turn of the isolating knob
	// sync runs a piece of work on the unit; the program does it in the background, a test at once
	sync func(func())
}

func (p *ctlPads) init() {
	if p.muted == nil {
		p.muted, p.was, p.iso = map[int]bool{}, map[int]int32{}, -1
	}
}

// ctlPadEvent takes a message of the pad group in (note or controller number 76 to 91 on the controller channel).
func (a *App) ctlPadEvent(e midiin.Event) {
	if !a.ctlPadsOn() || e.Data1 < ctlPadFirst || e.Data1 > ctlPadLast {
		return
	}
	pad := a.ctlBank*16 + e.Data1 - ctlPadFirst
	p := &a.pads
	p.mu.Lock()
	p.init()
	switch {
	case e.Kind == midiin.NoteOn && e.Data2 > 0:
		p.muted[pad] = !p.muted[pad]
		if !p.muted[pad] {
			delete(p.muted, pad)
		}
	case e.Kind == midiin.CC:
		at := e.At
		if at.IsZero() {
			at = time.Now()
		}
		switch {
		case p.iso != pad:
			p.iso = pad
		case at.Sub(p.last) > isoPause:
			p.iso = -1
		}
		p.last = at
	default:
		p.mu.Unlock()
		return
	}
	want := p.wanted()
	p.mu.Unlock()
	a.padsApply(want, e.At, false)
}

// wanted is the set of pads that are to be silent: the muted ones, and with an isolation every other pad of its bank.
func (p *ctlPads) wanted() map[int]bool {
	want := map[int]bool{}
	for pad := range p.muted {
		want[pad] = true
	}
	if p.iso >= 0 {
		b := p.iso / 16
		for i := b * 16; i < b*16+16; i++ {
			if i != p.iso {
				want[i] = true
			}
		}
	}
	return want
}

// padsApply makes the unit match: pads in want go to level 0 (remembering theirs), the others that were set to 0 get their level back.
func (a *App) padsApply(want map[int]bool, at time.Time, wait bool) {
	dev := a.deviceNow()
	p := &a.pads
	if dev == nil {
		a.say("controller: no unit to mute on", true)
		return
	}
	work := func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		var changed []int
		for pad := range want {
			if _, done := p.was[pad]; done {
				continue
			}
			idx, ok := proto.NewPadIndex(uint16(pad))
			if !ok {
				continue
			}
			blk, err := dev.PadBlock(idx)
			if err != nil {
				a.say("controller: pad "+a.label(pad)+": "+err.Error(), true)
				continue
			}
			level, _ := blk.Param(0x69)
			p.was[pad] = level
			if err := dev.SetParam(0x69, proto.TargetPad(idx), 0); err != nil {
				delete(p.was, pad)
				a.say("controller: pad "+a.label(pad)+": "+err.Error(), true)
				continue
			}
			changed = append(changed, pad)
		}
		var back []int
		for pad, level := range p.was {
			if want[pad] {
				continue
			}
			idx, ok := proto.NewPadIndex(uint16(pad))
			if ok {
				if err := dev.SetParam(0x69, proto.TargetPad(idx), level); err != nil {
					a.say("controller: pad "+a.label(pad)+": "+err.Error(), true)
					continue
				}
			}
			delete(p.was, pad)
			back = append(back, pad)
		}
		sort.Ints(changed)
		sort.Ints(back)
		a.padsLog(at, changed, true)
		a.padsLog(at, back, false)
		switch {
		case p.iso >= 0:
			a.say("controller: pad "+a.label(p.iso)+" alone", false)
		case len(changed) > 0:
			a.say("controller: pad "+a.label(changed[0])+" muted", false)
		case len(back) > 0:
			a.say("controller: pad "+a.label(back[0])+" is back", false)
		}
	}
	switch {
	case p.sync != nil:
		p.sync(work)
	case wait:
		work()
	default:
		go work()
	}
}

// padsLog writes a mute into the event log of a recording (the same entry the old PERFORM made).
func (a *App) padsLog(at time.Time, pads []int, on bool) {
	l := a.evlog
	if l == nil {
		return
	}
	if at.IsZero() {
		at = time.Now()
	}
	for _, pad := range pads {
		l.log.Add(at, "mute", perflog.P("bank", string(rune('A'+pad/16))), perflog.P("pad", pad%16+1), perflog.P("on", on))
		l.eventCount++
	}
}

// padsRestore puts every level back (the program is left, the controller is switched off, or the bank is another).
func (a *App) padsRestore() {
	p := &a.pads
	p.mu.Lock()
	p.init()
	p.muted, p.iso = map[int]bool{}, -1
	p.mu.Unlock()
	p.mu.Lock()
	any := len(p.was) > 0
	p.mu.Unlock()
	if any {
		a.padsApply(map[int]bool{}, time.Time{}, true)
	}
}

// deviceSession is what the pad group needs of the unit's session.
type deviceSession interface {
	PadBlock(proto.PadIndex) (*proto.PadBlock, error)
	SetParam(id byte, target proto.Target, value int32) error
}

// deviceNow is the unit's session, if there is one.
func (a *App) deviceNow() deviceSession {
	if l, ok := a.link.(DeviceLink); ok && l.Dev != nil {
		return l.Dev
	}
	return nil
}

func (a *App) ctlPadsOn() bool { return a.set.MidiCtlPads != "off" }
