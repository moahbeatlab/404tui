package tui

import (
	"fmt"
	"strings"

	"github.com/moahbeatlab/404tui/midiin"
)

// Morph: a snapshot of Punch is not held but started, and then a slider or an encoder moves the buses from the state
// they were in to the snapshot's, 0 to 127. Where the effect is the same the knobs move smoothly; an effect that
// changes, or a bus switched on or off, changes halfway (at 64), as do knobs that were not known at the start.
// Back at 0 everything is as it was. Nothing waits for the clock.

// morphChan is the channel of the EC4's third group: encoders 1 to 4 belong to snapshots 1 to 4. A push (note
// 60 to 63) starts the morph to that snapshot, or stops it when it is the one that runs; a turn (CC 60 to 63,
// relative) starts it if it does not run and moves it.
const morphChan = 14

// morphStep is how far one click of an encoder goes (of 127).
const morphStep = 2

// morphBus is the state of a bus at mix (0 to 127) of the way from a to b.
func morphBus(a, b PunchBus, mix int) PunchBus {
	out := a
	half := mix >= 64
	same := a.Num < 0 || b.Num < 0 || a.Num == b.Num
	if b.Num >= 0 && half {
		out.Num = b.Num
	}
	if b.On >= 0 && half {
		out.On = b.On
	}
	for k := range out.Knobs {
		x, y := a.Knobs[k], b.Knobs[k]
		switch {
		case y < 0:
		case x < 0 || !same:
			if half {
				out.Knobs[k] = y
			}
		default:
			out.Knobs[k] = x + int(float64(y-x)*float64(mix)/127+0.5*sign(y-x))
		}
	}
	return out
}

func sign(v int) float64 {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

// morphStart makes a snapshot the one the morph goes to, from the state the buses are in now; the one that runs
// is stopped (the buses stay where they are).
func (m Model) morphStart(slot int) Model {
	sc := m.scenes()
	m.sc = sc
	run := &sc.punch
	if run.active == slot+1 {
		run.active = 0
		m.message = fmt.Sprintf("MORPH %d stopped where it is", slot+1)
		return m
	}
	if !punchSet(m.mem.Punch[slot]) {
		m.message = fmt.Sprintf("PUNCH %d is empty: tap SAVE, then it", slot+1)
		return m
	}
	run.active, run.mix = slot+1, 0
	run.from = m.lfoBase(m.capturePunch())
	run.sent = run.from
	run.skip = [4]bool{}
	var unknown []string
	for b, p := range m.mem.Punch[slot] {
		if p.Set && !m.punchBusy(b) && !m.punchKnown(b, p) {
			run.skip[b] = true
			unknown = append(unknown, fmt.Sprint(b+1))
		}
	}
	m.message = fmt.Sprintf("MORPH to %d: move the slider or the encoder", slot+1)
	if len(unknown) > 0 {
		m.message = fmt.Sprintf("MORPH %d: BUS %s left alone, the app does not know what it has: tap its chip at the top and tell it", slot+1, strings.Join(unknown, ", "))
	}
	return m
}

// morphSet moves the morph to mix (0 to 127) and sends what that changes.
func (m Model) morphSet(mix int) Model {
	sc := m.scenes()
	m.sc = sc
	run := &sc.punch
	if run.active == 0 {
		return m
	}
	run.mix = clamp(mix, 0, 127)
	for b, to := range m.mem.Punch[run.active-1] {
		if !to.Set || m.punchBusy(b) || run.skip[b] {
			continue
		}
		last := run.sent[b]
		m = m.punchSend(b, morphBus(run.from[b], to, run.mix), &last)
		run.sent[b] = last
	}
	return m
}

// morphEvent takes what the EC4's morph group sends.
func (m Model) morphEvent(e midiin.Event) (Model, bool) {
	switch e.Kind {
	case midiin.NoteOn:
		if i := int(e.Data1) - ec4FirstNote; i >= 0 && i < 4 && e.Data2 > 0 {
			return m.morphStart(i), true
		}
	case midiin.CC:
		i := int(e.Data1) - ec4FirstCC
		if i < 0 || i > 3 || e.Data2 == 64 {
			return m, false
		}
		m.sc = m.scenes()
		if m.sc.punch.active != i+1 {
			if m = m.morphStart(i); m.sc.punch.active != i+1 {
				return m, true
			}
		}
		return m.morphSet(m.sc.punch.mix + (int(e.Data2)-64)*morphStep), true
	}
	return m, false
}
