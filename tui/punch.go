package tui

import (
	"fmt"
	"strings"
)

// Punch: four snapshots of the effects of all four buses. Held, a snapshot puts every bus it holds into that
// state (the effect, its six knobs, on or off); let go, each bus goes back to what it was just before, whatever it
// was doing then. It is sent as MIDI controllers, as the scenes are, so it is as quick as they are. What the unit
// has not said (a knob never turned, an effect never switched) is neither saved nor given back.

// punchCCs are the six knobs of an effect: CTRL 1 to 3 and CTRL 4 to 6.
var punchCCs = [6]int{16, 17, 18, 80, 81, 82}

// punchRun is what is going on: the snapshot that is held and what the buses were before it, and whether
// the next tap on a snapshot saves into it. It lives in the scenes, which the copies of the model share.
type punchRun struct {
	held int // the snapshot held, 1 to 4; 0 for none
	was  PunchSnap
	arm  bool
}

// captureBus reads the effect of a bus as the unit last said it (or as this program last sent it).
func (m Model) captureBus(bus int) PunchBus {
	f := m.fx[bus]
	p := PunchBus{On: -1, Num: -1, Knobs: [6]int{-1, -1, -1, -1, -1, -1}}
	if f.onSeen {
		p.On = 0
		if f.on {
			p.On = 1
		}
	}
	if f.numSeen {
		p.Num = f.num
	}
	for k, cc := range punchCCs {
		if c := m.ccOf(bus+1, cc); c != nil {
			p.Knobs[k] = c.val
		}
	}
	p.Set = p.On >= 0 || p.Num >= 0
	for _, v := range p.Knobs {
		p.Set = p.Set || v >= 0
	}
	return p
}

func (m Model) capturePunch() PunchSnap {
	var s PunchSnap
	for b := range s {
		s[b] = m.captureBus(b)
	}
	return s
}

// punchGo sends a bus into a state: the effect first, then the knobs, then on or off.
func (m Model) punchGo(bus int, p PunchBus) Model {
	sc := m.scenes()
	ch := byte(bus)
	f := m.fx[bus]
	if p.Num >= 0 {
		m.sceneSend(sc, []byte{0xB0 | ch, ccFxNumber, byte(p.Num)})
		f.num, f.numSeen = p.Num, true
	}
	for k, v := range p.Knobs {
		if v >= 0 {
			m.sceneSend(sc, []byte{0xB0 | ch, byte(punchCCs[k]), byte(clamp(v, 0, 127))})
		}
	}
	if p.On >= 0 {
		v := byte(0)
		if p.On == 1 {
			v = 127
		}
		m.sceneSend(sc, []byte{0xB0 | ch, ccFxSwitch, v})
		f.on, f.onSeen = p.On == 1, true
	}
	m.fx[bus] = f
	return m
}

// punchBusy is whether a motion owns the knobs of a bus: it is left alone.
func (m Model) punchBusy(bus int) bool {
	sc := m.scenes()
	return bus < len(sc.bus) && sc.bus[bus].motionBusy()
}

// punchAct does what the page asks: "save" arms the saving (or disarms it), "down" holds a snapshot (or, with the
// arm on, saves the buses into it), "up" lets it go.
func (m Model) punchAct(act string, slot int) Model {
	sc := m.scenes()
	m.sc = sc
	run := &sc.punch
	switch act {
	case "save":
		run.arm = !run.arm
		if run.arm {
			m.message = "SAVE: tap a snapshot to store the effects of all four buses in it"
		} else {
			m.message = ""
		}
		return m
	case "down":
		if slot < 0 || slot > 3 {
			return m
		}
		if run.arm {
			run.arm = false
			m.mem.Punch[slot] = m.capturePunch()
			m.message = fmt.Sprintf("PUNCH %d saved: %s", slot+1, punchText(m.mem.Punch[slot]))
			return m
		}
		if run.held != 0 {
			// A second finger, or the lift of the first was lost: the same one lets go, another waits.
			if run.held == slot+1 {
				return m.punchUp()
			}
			return m
		}
		snap := m.mem.Punch[slot]
		if !punchSet(snap) {
			m.message = fmt.Sprintf("PUNCH %d is empty: tap SAVE, then it", slot+1)
			return m
		}
		run.held, run.was = slot+1, m.capturePunch()
		for b, p := range snap {
			if p.Set && !m.punchBusy(b) {
				m = m.punchGo(b, p)
			}
		}
		m.message = fmt.Sprintf("PUNCH %d held: let go to go back", slot+1)
	case "up":
		if run.held == slot+1 {
			return m.punchUp()
		}
	}
	return m
}

// punchUp puts the buses back to what they were before the snapshot that is held. A value that was not known then
// cannot be given back and stays as the snapshot had it.
func (m Model) punchUp() Model {
	run := &m.scenes().punch
	if run.held == 0 {
		return m
	}
	snap := m.mem.Punch[run.held-1]
	for b, p := range snap {
		if p.Set && !m.punchBusy(b) {
			m = m.punchGo(b, run.was[b])
		}
	}
	m.message = fmt.Sprintf("PUNCH %d let go", run.held)
	run.held = 0
	return m
}

func punchSet(s PunchSnap) bool {
	for _, p := range s {
		if p.Set {
			return true
		}
	}
	return false
}

// punchText says what a snapshot holds, for the message.
func punchText(s PunchSnap) string {
	var parts []string
	for b, p := range s {
		if !p.Set {
			continue
		}
		t := fmt.Sprintf("BUS %d", b+1)
		if p.Num >= 0 {
			t += " " + fxName(p.Num)
		}
		parts = append(parts, t)
	}
	if len(parts) == 0 {
		return "nothing known yet: turn a knob on the unit once"
	}
	return strings.Join(parts, ", ")
}

// webPunch is the snapshots for the page.
type webPunch struct {
	Arm   bool            `json:"arm"`
	Slots [4]webPunchSlot `json:"slots"`
}

type webPunchSlot struct {
	Set   bool    `json:"set"`
	Held  bool    `json:"held"`
	Buses [4]bool `json:"buses"`
}

func (m Model) webPunch() webPunch {
	run := m.scenes().punch
	w := webPunch{Arm: run.arm}
	for i, s := range m.mem.Punch {
		w.Slots[i] = webPunchSlot{Set: punchSet(s), Held: run.held == i+1}
		for b, p := range s {
			w.Slots[i].Buses[b] = p.Set
		}
	}
	return w
}
