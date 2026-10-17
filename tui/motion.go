package tui

import (
	"fmt"

	"github.com/moahbeatlab/404tui/midiin"
)

// Motion (Perform): the moves of C1 to C3 of a bus are recorded for some bars, beginning on a bar of the
// unit's clock, and then played back in a loop by sending the same controllers to the unit. One button per
// bus: a tap arms it, the recording begins on the next bar and ends after the number of bars set for
// glides, the loop begins at once, and a tap stops it.

const (
	motionIdle = iota
	motionArmed
	motionRecording
	motionPlaying
)

// motionEv is a move of a knob: the clock tick in the loop, the knob (0 to 2) and its value.
type motionEv struct{ tick, knob, val int }

// motion is the state of one bus's motion.
type motion struct {
	state  int
	total  int // ticks in the loop
	pos    int // ticks since the loop (or the recording) began
	events []motionEv
}

// motionAct handles a tap on a motion button.
func (m Model) motionAct(bus int) Model {
	sc := m.scenes()
	mo := &sc.bus[bus].mo
	switch mo.state {
	case motionIdle:
		if m.clock == nil || !m.clock.running {
			m.message = "motion is recorded with the unit's MIDI clock: start its sequencer"
			break
		}
		sc.bus[bus].glide = nil
		*mo = motion{state: motionArmed, total: sc.bars * ticksPerBar}
		m.message = fmt.Sprintf("BUS %d: the recording of %s starts on the next bar: turn C1 to C3 on the unit", bus+1, plural(sc.bars, "bar"))
	default:
		mo.state = motionIdle
		m.message = fmt.Sprintf("BUS %d: motion stopped", bus+1)
	}
	m.sc = sc
	return m
}

// motionIn notes a knob that was moved on the unit while a motion is recorded.
func (m Model) motionIn(e midiin.Event) {
	if m.sc == nil || e.Kind != midiin.CC || e.Channel < 1 || e.Channel > 2 || e.Data1 < 16 || e.Data1 > 18 {
		return
	}
	mo := &m.sc.bus[e.Channel-1].mo
	if mo.state != motionRecording {
		return
	}
	mo.events = append(mo.events, motionEv{tick: mo.pos, knob: e.Data1 - 16, val: e.Data2})
}

// motionTick moves the motions on with the clock: an armed one waits for a bar, a recording counts, a
// loop sends what was recorded at this tick.
func (m Model) motionTick() {
	sc := m.sc
	if sc == nil || m.clock == nil {
		return
	}
	for bus := range 2 {
		mo := &sc.bus[bus].mo
		switch mo.state {
		case motionArmed:
			if m.clock.ticks%ticksPerBar != 0 {
				continue
			}
			mo.state, mo.pos = motionRecording, 0
			// The knobs as they are now are where the loop begins.
			for k := range 3 {
				if c := m.ccOf(bus+1, 16+k); c != nil {
					mo.events = append(mo.events, motionEv{tick: 0, knob: k, val: c.val})
				}
			}
		case motionRecording:
			mo.pos++
			if mo.pos >= mo.total {
				mo.state, mo.pos = motionPlaying, 0
				m.motionPlay(sc, bus)
				mo.pos = 1
			}
		case motionPlaying:
			m.motionPlay(sc, bus)
			mo.pos = (mo.pos + 1) % mo.total
		}
	}
}

// motionPlay sends the moves recorded at the tick the loop is at.
func (m Model) motionPlay(sc *sceneState, bus int) {
	mo := &sc.bus[bus].mo
	for _, ev := range mo.events {
		if ev.tick == mo.pos {
			m.sceneSend(sc, []byte{0xB0 | byte(bus), byte(16 + ev.knob), byte(clamp(ev.val, 0, 127))})
		}
	}
}

// motionButton is the label of a bus's motion button, and whether it is lit.
func (m Model) motionButton(bus int) (string, bool) {
	mo := m.scenes().bus[bus].mo
	switch mo.state {
	case motionArmed:
		return "waiting for the bar… cancel", true
	case motionRecording:
		return fmt.Sprintf("recording %d/%d bars", mo.pos/ticksPerBar+1, mo.total/ticksPerBar), true
	case motionPlaying:
		return fmt.Sprintf("▶ looping %s · stop", plural(mo.total/ticksPerBar, "bar")), true
	}
	return "● record motion", false
}

// motionLines are the MOTION panel: a button for each bus.
func (m Model) motionLines(width int) ([]string, []btn) {
	st := m.st
	b := newRowBuilder(st, m.btnH)
	half := max((width-1)/2, 10)
	for bus := range 2 {
		label, on := m.motionButton(bus)
		b.button(fmt.Sprintf("BUS %d  %s", bus+1, label), on, "motion", bus, half)
		if bus == 0 {
			b.gap(1)
		}
	}
	return b.rows(0)
}
