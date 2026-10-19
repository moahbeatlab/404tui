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
	// sel is the snapshot whose values the page shows (1 to 4), the last one held or tapped.
	sel int
	// edit: what is changed on the unit while a snapshot is held is kept in it when it is let go.
	edit bool
	// skip are the buses a held snapshot or a morph does not touch, because what they have is not known (see punchKnown).
	skip [4]bool
	// morph: a tap on a snapshot does not hold it but starts the morph (see morph.go).
	morph bool
	// active is the snapshot the morph goes to (1 to 4, 0 for none), mix how far it is (0 to 127), from the state
	// the buses were in when it began and sent what was last sent to each bus.
	active, mix int
	from, sent  PunchSnap
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
	return m.punchSend(bus, p, nil)
}

// punchSend is punchGo, but with last (what the bus was sent before) it sends only what differs, and notes what it sent.
func (m Model) punchSend(bus int, p PunchBus, last *PunchBus) Model {
	sc := m.scenes()
	ch := byte(bus)
	f := m.fx[bus]
	if p.Num >= 0 && (last == nil || last.Num != p.Num) {
		m.sceneSend(sc, []byte{0xB0 | ch, ccFxNumber, byte(p.Num)})
		f.num, f.numSeen = p.Num, true
		if last != nil {
			last.Num = p.Num
		}
	}
	for k, v := range p.Knobs {
		if v >= 0 && (last == nil || last.Knobs[k] != v) {
			m.sceneSend(sc, []byte{0xB0 | ch, byte(punchCCs[k]), byte(clamp(v, 0, 127))})
			if last != nil {
				last.Knobs[k] = v
			}
		}
	}
	if p.On >= 0 && (last == nil || last.On != p.On) {
		v := byte(0)
		if p.On == 1 {
			v = 127
		}
		m.sceneSend(sc, []byte{0xB0 | ch, ccFxSwitch, v})
		f.on, f.onSeen = p.On == 1, true
		if last != nil {
			last.On = p.On
		}
	}
	m.fx[bus] = f
	return m
}

// punchKnown is whether the app knows what a bus has well enough to give it back after a snapshot changed it: its
// effect when the snapshot sets one, whether it is on when the snapshot says so. The unit says these only when they change
// on it, so an effect loaded before the app listened is not known, and a bus like that is left alone: changing it
// would leave it in a state the app cannot undo.
func (m Model) punchKnown(bus int, p PunchBus) bool {
	f := m.fx[bus]
	return (p.Num < 0 || f.numSeen) && (p.On < 0 || f.onSeen)
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
	case "mode":
		// slot is the mode here: 0 hold, 1 morph, 2 edit.
		run.morph, run.edit = slot == 1, slot == 2
		switch slot {
		case 1:
			m.message = "MORPH: tap a snapshot, then the slider (or the encoder) moves to it; tap it again to stop"
		case 2:
			m.message = "EDIT: hold a snapshot and turn the knobs on the unit; let go to keep what you changed in it"
		default:
			m.message = "HOLD: hold a snapshot to punch it"
		}
		return m
	case "down":
		if slot < 0 || slot > 3 {
			return m
		}
		run.sel = slot + 1
		if run.morph && !run.arm {
			return m.morphStart(slot)
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
		run.held, run.was = slot+1, m.lfoBase(m.capturePunch())
		for b, w := range run.was {
			m.log.Info("punch: held", "slot", slot+1, "bus", b+1, "in snapshot", snap[b].Set, "was effect", w.Num, "was on", w.On, "was knobs", w.Knobs)
		}
		var unknown []string
		run.skip = [4]bool{}
		for b, p := range snap {
			if !p.Set || m.punchBusy(b) {
				continue
			}
			if !m.punchKnown(b, p) {
				run.skip[b] = true
				unknown = append(unknown, fmt.Sprint(b+1))
				continue
			}
			m = m.punchGo(b, p)
		}
		m.message = fmt.Sprintf("PUNCH %d held: let go to go back", slot+1)
		if len(unknown) > 0 {
			m.message = fmt.Sprintf("PUNCH %d: BUS %s left alone, the app does not know what it has: tap its chip at the top and tell it", slot+1, strings.Join(unknown, ", "))
		}
	case "up":
		if run.morph {
			return m
		}
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
	text := "let go"
	if run.edit {
		n := 0
		cur := m.capturePunch()
		for b := range snap {
			if merged, changed := punchMerge(snap[b], cur[b], run.was[b]); changed {
				snap[b], n = merged, n+1
			}
		}
		m.mem.Punch[run.held-1] = snap
		text = "let go, nothing changed"
		if n > 0 {
			text = fmt.Sprintf("let go, %s kept: %s", plural(n, "bus"), punchText(snap))
		}
	}
	for b, p := range snap {
		if (p.Set || run.edit) && !m.punchBusy(b) && !run.skip[b] {
			back := run.was[b]
			m.log.Info("punch: let go", "slot", run.held, "bus", b+1, "back to effect", back.Num, "on", back.On, "knobs", back.Knobs)
			m = m.punchGo(b, back)
		}
	}
	m.message = fmt.Sprintf("PUNCH %d %s", run.held, text)
	run.held = 0
	return m
}

// punchMerge takes what was changed on a bus while a snapshot was held into the snapshot: every value that is not
// what it was before the hold (was) and not what the snapshot had. A bus the snapshot did not hold joins it when
// something on it changed. changed says whether anything did.
func punchMerge(snap, cur, was PunchBus) (PunchBus, bool) {
	out, changed := snap, false
	if !snap.Set {
		out = PunchBus{On: -1, Num: -1, Knobs: [6]int{-1, -1, -1, -1, -1, -1}}
	}
	take := func(snapV, curV, wasV int, to *int) {
		if curV < 0 || curV == snapV && snap.Set {
			return
		}
		if !snap.Set && curV == wasV {
			return
		}
		*to, changed = curV, true
	}
	take(snap.Num, cur.Num, was.Num, &out.Num)
	take(snap.On, cur.On, was.On, &out.On)
	for k := range out.Knobs {
		take(snap.Knobs[k], cur.Knobs[k], was.Knobs[k], &out.Knobs[k])
	}
	if changed {
		out.Set = true
	}
	return out, changed
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
			t += " " + fxNameOf(b, p.Num)
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
	Arm  bool `json:"arm"`
	Edit bool `json:"edit"`
	Sel  int  `json:"sel"`
	// Bus is what the app knows of each bus's effect now (Num -1 and On -1 when the unit never said).
	Bus [4]webBusState `json:"bus"`
	// Snap are the values of the selected snapshot, Live what the buses are now (to see what was changed).
	Snap    [4]webBusVals   `json:"snap"`
	Live    [4]webBusVals   `json:"live"`
	Morph   bool            `json:"morph"`
	Active  int             `json:"active"`
	Mix     int             `json:"mix"`
	Slots   [4]webPunchSlot `json:"slots"`
	Presets []webPreset     `json:"presets"`
}

// webPreset is a preset to choose from: its name, what it does and the buses it sets.
type webPreset struct {
	Name  string  `json:"name"`
	Note  string  `json:"note"`
	Buses [4]bool `json:"buses"`
}

type webPunchSlot struct {
	Set   bool    `json:"set"`
	Held  bool    `json:"held"`
	Buses [4]bool `json:"buses"`
}

func (m Model) webPunch() webPunch {
	run := m.scenes().punch
	w := webPunch{Arm: run.arm, Morph: run.morph, Edit: run.edit, Active: run.active, Mix: run.mix, Presets: make([]webPreset, len(punchPresets))}
	w.Sel = run.sel
	for b := range w.Bus {
		f := m.fx[b]
		w.Bus[b] = webBusState{Num: -1, On: -1}
		if f.numSeen {
			w.Bus[b].Num, w.Bus[b].Name = f.num, fxNameOf(b, f.num)
		}
		if f.onSeen {
			w.Bus[b].On = 0
			if f.on {
				w.Bus[b].On = 1
			}
		}
	}
	if run.sel > 0 {
		live := m.capturePunch()
		for b := range live {
			w.Snap[b] = webVals(b, m.mem.Punch[run.sel-1][b])
			w.Live[b] = webVals(b, live[b])
		}
	}
	for i, p := range punchPresets {
		w.Presets[i] = webPreset{Name: p.name, Note: p.note}
		for b, f := range p.bus {
			w.Presets[i].Buses[b] = f.fx != ""
		}
	}
	for i, s := range m.mem.Punch {
		w.Slots[i] = webPunchSlot{Set: punchSet(s), Held: run.held == i+1}
		for b, p := range s {
			w.Slots[i].Buses[b] = p.Set
		}
	}
	return w
}

// punchPreset puts a preset into a snapshot. One that is held is let go first, so what it gave back is not lost.
func (m Model) punchPreset(preset, slot int) Model {
	if preset < 0 || preset >= len(punchPresets) || slot < 0 || slot > 3 {
		return m
	}
	m.sc = m.scenes()
	if m.sc.punch.held != 0 {
		m = m.punchUp()
	}
	m.mem.Punch[slot] = punchPresets[preset].snap()
	m.message = fmt.Sprintf("PUNCH %d: %s", slot+1, punchPresets[preset].name)
	return m
}

// webBusVals is the effect of a bus with its knobs as the unit says them, for the page.
type webBusVals struct {
	Set    bool      `json:"set"`
	Name   string    `json:"name"`
	Num    int       `json:"num"`
	On     int       `json:"on"`
	Params [6]string `json:"params"`
	Text   [6]string `json:"text"`
	Raw    [6]int    `json:"raw"`
}

func webVals(bus int, p PunchBus) webBusVals {
	if !p.Set {
		return webBusVals{}
	}
	v := webBusVals{Set: true, Num: p.Num, On: p.On, Raw: p.Knobs, Name: "?"}
	if p.Num >= 0 {
		v.Name = fxNameOf(bus, p.Num)
	}
	knobs := fxKnobs(bus, max(p.Num, 0))
	for k, kn := range knobs {
		if p.Num < 0 {
			kn = fxParam{name: fmt.Sprintf("CTRL %d", k+1), kind: "c", hi: 127}
		}
		if kn.name != "" {
			v.Params[k], v.Text[k] = kn.name, kn.valueText(p.Knobs[k])
		}
	}
	return v
}

// webBusState is what is known of a bus: the effect and whether it is on.
type webBusState struct {
	Num  int    `json:"num"`
	Name string `json:"name"`
	On   int    `json:"on"`
}

// fxDeclare tells the app what a bus has: the unit only says it when it changes, so what was loaded before the app
// listened is unknown, and a punch cannot put it back. "num" says the effect, "on" whether it is on (1 or 0). Nothing is sent.
func (m Model) fxDeclare(bus int, what string, n int) Model {
	if bus < 0 || bus > 3 {
		return m
	}
	f := m.fx[bus]
	switch what {
	case "num":
		if n < 0 || n >= len(fxTable(bus)) {
			return m
		}
		f.num, f.numSeen = n, true
		m.message = fmt.Sprintf("BUS %d: %s noted (nothing sent to the unit)", bus+1, fxNameOf(bus, n))
	case "on":
		f.on, f.onSeen = n == 1, true
		m.message = fmt.Sprintf("BUS %d: noted as %s (nothing sent to the unit)", bus+1, map[bool]string{true: "on", false: "off"}[n == 1])
	default:
		return m
	}
	m.fx[bus] = f
	return m
}
