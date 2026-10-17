package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/moahbeatlab/404tui/midiin"
)

// Motion (Perform): the moves of C1 to C3 of a bus are recorded for some bars, beginning on a bar of the
// unit's clock, and played back in a loop by sending the same controllers to the unit. Each bus has
// four slots to keep a motion in. A tap on an empty slot records into it (it waits for the next bar and
// records the bars set for glides); a tap on a slot that holds a motion plays it in a loop from the next
// bar; a tap on the one that plays stops it. "clear" empties the slot that was tapped last.

const motionSlots = 4

const (
	motionIdle      = iota // nothing: the slot is empty, or holds a motion that is not playing
	motionArmed            // waiting for the bar to record from
	motionRecording        // recording
	motionWaiting          // waiting for the bar to play from
	motionPlaying          // playing in a loop
)

// motionEv is a move of a knob: the clock tick in the loop, the knob (0 to 2) and its value.
type motionEv struct{ tick, knob, val int }

// motion is a slot: what was recorded, and where it is.
type motion struct {
	state  int
	total  int // ticks in the loop
	pos    int // ticks since the loop (or the recording) began
	events []motionEv
}

func (mo motion) filled() bool { return len(mo.events) > 0 && mo.total > 0 }

// busy says whether a motion is going on for the bus: then its scenes leave the knobs alone.
func (bs *busScene) motionBusy() bool {
	for _, mo := range bs.mo {
		if mo.state != motionIdle {
			return true
		}
	}
	return false
}

// motionAct handles a tap on a slot of a bus.
func (m Model) motionAct(bus, slot int) Model {
	sc := m.scenes()
	bs := &sc.bus[bus]
	if slot < 0 || slot >= motionSlots {
		return m
	}
	bs.sel = slot
	mo := &bs.mo[slot]
	running := m.clock != nil && m.clock.running
	switch {
	case mo.state == motionPlaying || mo.state == motionWaiting:
		mo.state = motionIdle
		m.message = fmt.Sprintf("BUS %d: motion %d stopped", bus+1, slot+1)
	case mo.state == motionArmed || mo.state == motionRecording:
		*mo = motion{}
		m.message = fmt.Sprintf("BUS %d: the recording into slot %d was cancelled", bus+1, slot+1)
	case !running:
		m.message = "motion follows the unit's MIDI clock: start its sequencer"
	case mo.filled():
		bs.stopMotions()
		bs.glide = nil
		mo.state, mo.pos = motionWaiting, 0
		m.message = fmt.Sprintf("BUS %d: motion %d plays from the next bar", bus+1, slot+1)
	default:
		bs.stopMotions()
		bs.glide = nil
		*mo = motion{state: motionArmed, total: sc.bars * ticksPerBar}
		m.message = fmt.Sprintf("BUS %d: motion %d records %s from the next bar: turn C1 to C3 on the unit", bus+1, slot+1, plural(sc.bars, "bar"))
	}
	m.sc = sc
	return m
}

// stopMotions stops what plays or waits (a recording is kept as it is).
func (bs *busScene) stopMotions() {
	for i := range bs.mo {
		if st := bs.mo[i].state; st == motionPlaying || st == motionWaiting {
			bs.mo[i].state = motionIdle
		}
	}
}

// motionClear empties the slot of a bus that was tapped last.
func (m Model) motionClear(bus int) Model {
	sc := m.scenes()
	bs := &sc.bus[bus]
	bs.mo[bs.sel] = motion{}
	m.message = fmt.Sprintf("BUS %d: motion %d cleared", bus+1, bs.sel+1)
	m.sc = sc
	return m
}

// motionIn notes a knob that was moved on the unit while a motion is recorded.
func (m Model) motionIn(e midiin.Event) {
	if m.sc == nil || e.Kind != midiin.CC || e.Channel < 1 || e.Channel > 2 || e.Data1 < 16 || e.Data1 > 18 {
		return
	}
	bs := &m.sc.bus[e.Channel-1]
	mo := &bs.mo[bs.sel]
	if mo.state != motionRecording {
		return
	}
	mo.events = append(mo.events, motionEv{tick: mo.pos, knob: e.Data1 - 16, val: e.Data2})
}

// motionTick moves the motions on with the clock.
func (m Model) motionTick() {
	sc := m.sc
	if sc == nil || m.clock == nil {
		return
	}
	for bus := range 2 {
		bs := &sc.bus[bus]
		for slot := range bs.mo {
			mo := &bs.mo[slot]
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
					if !mo.filled() {
						*mo = motion{} // nothing was moved
						continue
					}
					mo.state, mo.pos = motionPlaying, 0
					m.motionPlay(sc, bus, slot)
					mo.pos = 1
				}
			case motionWaiting:
				if m.clock.ticks%ticksPerBar == 0 {
					mo.state, mo.pos = motionPlaying, 0
					m.motionPlay(sc, bus, slot)
					mo.pos = 1 % mo.total
				}
			case motionPlaying:
				m.motionPlay(sc, bus, slot)
				mo.pos = (mo.pos + 1) % mo.total
			}
		}
	}
}

// motionPlay sends the moves recorded at the tick the loop is at.
func (m Model) motionPlay(sc *sceneState, bus, slot int) {
	mo := &sc.bus[bus].mo[slot]
	for _, ev := range mo.events {
		if ev.tick == mo.pos {
			m.sceneSend(sc, []byte{0xB0 | byte(bus), byte(16 + ev.knob), byte(clamp(ev.val, 0, 127))})
		}
	}
}

// motionSpec is what a slot's button shows.
func (m Model) motionSpec(bus, slot int) btnSpec {
	mo := m.scenes().bus[bus].mo[slot]
	sp := btnSpec{label: fmt.Sprint(slot + 1), detail: "empty", act: "motion", arg: bus*motionSlots + slot}
	switch mo.state {
	case motionArmed:
		sp.detail, sp.on = "next bar…", true
	case motionRecording:
		sp.detail, sp.on = fmt.Sprintf("REC %d/%d", mo.pos/ticksPerBar+1, mo.total/ticksPerBar), true
	case motionWaiting:
		sp.detail, sp.on = "next bar…", true
	case motionPlaying:
		sp.detail, sp.on = "▶ "+mo.sparkline(10), true
	default:
		if mo.filled() {
			sp.detail = fmt.Sprintf("%d bar%s  %s", mo.total/ticksPerBar, map[bool]string{true: "", false: "s"}[mo.total/ticksPerBar == 1], mo.sparkline(10))
		}
	}
	return sp
}

// motionLines are the MOTION panel: for each bus a row of its four slots and a clear button.
func (m Model) motionLines(width int) ([]string, []btn) {
	st := m.st
	var lines []string
	var btns []btn
	slotW := max((width-motionSlots-8)/(motionSlots+1), 8)
	// How long a motion is, in bars.
	sc := m.scenes()
	bb := newRowBuilder(st, m.btnH)
	bb.text(st.dim.Render("length: "))
	lenBtn := max((width-8-5-6)/5, 5)
	for _, n := range []int{1, 2, 3, 4, 8} {
		bb.button(fmt.Sprint(n), n == sc.bars, "bars", n, lenBtn)
		bb.gap(1)
	}
	bb.text(st.dim.Render("bars"))
	l0, b0 := bb.rows(0)
	lines = append(lines, l0...)
	btns = append(btns, b0...)
	for bus := range 2 {
		colour, _ := st.th.busColour(int32(bus + 1))
		head := fmt.Sprintf("BUS %d", bus+1)
		lines = append(lines, lipgloss.NewStyle().Foreground(colour).Bold(true).Render(head)+st.dim.Render("  motion: tap a slot to record, play or stop it"))
		// Two lines of text (the slot and what it holds) are centred in a band of even height.
		b := newRowBuilder(st, m.btnH+1)
		specs := make([]btnSpec, motionSlots)
		for slot := range specs {
			specs[slot] = m.motionSpec(bus, slot)
		}
		b.row(slotW, specs...)
		b.gap(1)
		b.row(slotW, btnSpec{label: "clear", detail: fmt.Sprintf("slot %d", m.scenes().bus[bus].sel+1), act: "mclear", arg: bus})
		l, bt := b.rows(len(lines))
		lines = append(lines, l...)
		btns = append(btns, bt...)
	}
	// Where what is sent goes: the last message, or why nothing can be sent.
	switch {
	case sc.outErr != "":
		lines = append(lines, st.warn.Render("cannot send to the unit: "+sc.outErr))
	case sc.last != "":
		lines = append(lines, st.dim.Render("last sent to the unit: "+sc.last))
	}
	if sc.outErr == "" {
		if _, ok := m.midiSrc.(midiin.Outputs); !ok {
			lines = append(lines, st.warn.Render("no MIDI output in this build"))
		}
	}
	return lines, btns
}

// sparkline draws the knob that moves the most as a small curve of n steps over the loop, with its
// number in front: "C2 ▁▃▆█▆▃▁".
func (mo motion) sparkline(n int) string {
	var best, bestRange int
	var curves [3][]int
	for k := range curves {
		curves[k] = make([]int, n)
		for i := range curves[k] {
			curves[k][i] = -1
		}
	}
	for _, ev := range mo.events {
		i := min(ev.tick*n/max(mo.total, 1), n-1)
		curves[ev.knob][i] = ev.val
	}
	for k, c := range curves {
		lo, hi := 128, -1
		for _, v := range c {
			if v >= 0 {
				lo, hi = min(lo, v), max(hi, v)
			}
		}
		if hi >= 0 && hi-lo >= bestRange {
			best, bestRange = k, hi-lo
		}
	}
	steps := []rune("▁▂▃▄▅▆▇█")
	out := make([]rune, n)
	last := 0
	for i, v := range curves[best] {
		if v < 0 {
			v = last // a step with no move holds the value before it
		}
		last = v
		out[i] = steps[clamp(v*len(steps)/128, 0, len(steps)-1)]
	}
	return fmt.Sprintf("C%d %s", best+1, string(out))
}
