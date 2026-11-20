package core

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/lab"
	"github.com/moahbeatlab/404tui/midiin"
)

// Recording a track from a synth (SEQ → t → PATTERN, "Record a track from the MC-101"): the track in front is armed, the pattern loops
// (on the computer, or the unit plays it and its MIDI clock is followed), and the notes played on the synth (an MC-101 on the computer's
// USB, its own MIDI port) on one channel are caught where they fall in the loop, every pass adding to the take (overdub). Disarm writes
// the pattern again to the next empty slot with the take on the armed pad, each note's pitch counted from the pad's sample.

const defaultLivePort = "mc-101"

// liveSnaps are the grids the starts of the notes are moved to: their names and ticks (0: as played).
var liveSnaps = []struct {
	name  string
	ticks int
}{{"1/16", 120}, {"1/32", 60}, {"1/8 triplets", 160}, {"off: as played", 0}}

// liveOctaves are the octaves a pad's sample can be said to sound in (Roland's names: C4 is MIDI note 60).
var liveOctaves = []int{1, 2, 3, 4, 5, 6}

// liveTake is the notes caught, in ticks of the loop.
type liveTake struct {
	length int // the loop, in ticks
	snap   int // the grid, in ticks; 0 none
	held   map[int]liveHeld
	notes  []lab.Note // Pitch holds the MIDI note until the take is written
	passes map[int64]bool
}

type liveHeld struct {
	at  float64 // ticks from the start of the first round
	vel int
}

func newLiveTake(length, snap int) *liveTake {
	return &liveTake{length: max(length, 1), snap: snap, held: map[int]liveHeld{}, passes: map[int64]bool{}}
}

// feed takes in a note on or off that came at ticks (from the start of a round of the loop; it may be past the round's end).
func (t *liveTake) feed(e midiin.Event, ticks float64) {
	switch {
	case e.Kind == midiin.NoteOn && e.Data2 > 0:
		t.held[e.Data1] = liveHeld{at: ticks, vel: e.Data2}
		t.passes[int64(ticks)/int64(t.length)] = true
	case e.Kind == midiin.NoteOff || e.Kind == midiin.NoteOn:
		h, ok := t.held[e.Data1]
		if !ok {
			return
		}
		delete(t.held, e.Data1)
		t.add(e.Data1, h, ticks)
	}
}

// add writes a note: its start on the grid and in the loop, its length as held (at least a little, at most the loop).
func (t *liveTake) add(note int, h liveHeld, end float64) {
	at := int64(h.at + 0.5)
	if t.snap > 0 {
		at = (int64(h.at) + int64(t.snap)/2) / int64(t.snap) * int64(t.snap)
	}
	tick := int((at%int64(t.length) + int64(t.length)) % int64(t.length))
	ln := int(end - h.at + 0.5)
	if ln <= 0 { // across a round of the unit's clock: the round began again
		ln += t.length
	}
	ln = min(max(ln, 30), t.length)
	n := lab.Note{Tick: uint32(tick), Vel: h.vel, Len: ln, Pitch: note}
	for i, o := range t.notes { // the same note on the same step again: the last pass wins
		if o.Tick == n.Tick && o.Pitch == n.Pitch {
			t.notes[i] = n
			return
		}
	}
	t.notes = append(t.notes, n)
}

// finish ends the notes still held at ticks.
func (t *liveTake) finish(ticks float64) {
	for note, h := range t.held {
		t.add(note, h, ticks)
	}
	t.held = map[int]liveHeld{}
}

// padNotes are the take's notes for pad: each pitch in semitones from ref (the MIDI note the pad's sample sounds at), an octave in
// when it is further than a pad goes; folded counts those.
func (t *liveTake) padNotes(pad, ref int) (out []lab.Note, folded int) {
	for _, n := range t.notes {
		s := n.Pitch - ref
		f := false
		for s > formats.PitchRange {
			s -= 12
			f = true
		}
		for s < -formats.PitchRange {
			s += 12
			f = true
		}
		if f {
			folded++
		}
		n.Pad, n.Pitch = pad, int(formats.PitchByte(s))
		out = append(out, n)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Tick < out[j].Tick })
	return out, folded
}

// liveRec is the recording dialog's state while it is open.
type liveRec struct {
	pad    int
	ch     chan midiin.Event
	stop   func()
	port   string // the input taken, or why there is none
	ok     bool
	armed  bool
	take   *liveTake
	length int
}

func (a *App) livePort() string {
	if a.set.LivePort != "" {
		return a.set.LivePort
	}
	return defaultLivePort
}

func (a *App) liveChan() int {
	if a.set.LiveChan >= 1 && a.set.LiveChan <= 16 {
		return a.set.LiveChan
	}
	return 12
}

// liveRef is the MIDI note the pad's sample sounds at: its note in the octave set (C4 by default).
func (a *App) liveRef(pad int) int {
	pc, _ := a.sampleNote(pad)
	oct := 4
	if a.set.LiveOct >= 1 && a.set.LiveOct <= len(liveOctaves) {
		oct = liveOctaves[a.set.LiveOct-1]
	}
	return 12*(oct+1) + pc
}

// liveOpen listens to the synth's port (again, after the setting changed).
func (a *App) liveOpen() {
	r := a.srec
	if r == nil {
		return
	}
	if r.stop != nil {
		r.stop()
		r.stop = nil
	}
	r.ok = false
	if a.plat.MIDI == nil {
		r.port = "no MIDI in this build"
		return
	}
	ch := make(chan midiin.Event, 512)
	stop, name, err := a.plat.MIDI.Open(a.livePort(), func(e midiin.Event) {
		e.At = time.Now()
		select {
		case ch <- e:
		default:
		}
	})
	switch {
	case errors.Is(err, midiin.ErrUnavailable):
		r.port = "no MIDI in this build"
	case err != nil:
		r.port = "no input named like " + a.livePort() + ": plug the synth in (USB), then A on Port"
	default:
		r.ch, r.stop, r.port, r.ok = ch, stop, name, true
	}
}

func (a *App) liveClose() {
	if r := a.srec; r != nil && r.stop != nil {
		r.stop()
	}
	a.srec = nil
}

// liveTicks is where in the loop a moment is: ticks from the start of the round that plays; false when nothing plays.
func (a *App) liveTicks(at time.Time) (float64, bool) {
	start, _, perTick, ok := a.synthTiming(a.srec.length)
	if !ok || perTick <= 0 {
		return 0, false
	}
	return float64(at.Sub(start)) / perTick, true
}

// liveEvent takes in a message of the synth: a note on the channel, while armed and the loop plays.
func (a *App) liveEvent(e midiin.Event) {
	r := a.srec
	if r == nil || !r.armed || e.Channel != a.liveChan() || (e.Kind != midiin.NoteOn && e.Kind != midiin.NoteOff) {
		return
	}
	if e.At.IsZero() {
		e.At = a.clock()
	}
	if t, ok := a.liveTicks(e.At); ok {
		r.take.feed(e, t)
	}
}

// liveRecTick takes in what the synth sent.
func (a *App) liveRecTick() {
	r := a.srec
	if r == nil || r.ch == nil {
		return
	}
	for i := 0; i < 256; i++ {
		select {
		case e := <-r.ch:
			a.liveEvent(e)
		default:
			return
		}
	}
}

// liveRecMenu is the dialog: where to listen, the grid, the pad's octave, and arm / disarm.
func (a *App) liveRecMenu() {
	pad := a.rowPad()
	d, _ := a.doc()
	a.liveClose()
	a.srec = &liveRec{pad: pad, length: int(d.Length)}
	a.liveOpen()
	chans := make([]string, 16)
	for i := range chans {
		chans[i] = fmt.Sprint(i + 1)
	}
	snaps := make([]string, len(liveSnaps))
	for i, s := range liveSnaps {
		snaps[i] = s.name
	}
	pc, _ := a.sampleNote(pad)
	octs := make([]string, len(liveOctaves))
	for i, o := range liveOctaves {
		octs[i] = fmt.Sprintf("%s%d (MIDI %d)", sharpNames[pc], o, 12*(o+1)+pc)
	}
	octIdx := 3
	if a.set.LiveOct >= 1 && a.set.LiveOct <= len(liveOctaves) {
		octIdx = a.set.LiveOct - 1
	}
	var m *menu
	armLabel := func() string {
		if a.srec != nil && a.srec.armed {
			return "Disarm: write the new pattern"
		}
		return "Arm: record what is played"
	}
	m = &menu{title: "record " + a.label(pad) + " from a synth", parent: a.menu}
	m.items = []menuItem{
		{label: "LISTEN TO", header: true},
		{label: "Port (its name holds)", vals: []string{a.livePort()}, run: func(int) {
			a.askText("the synth's MIDI port", "a part of its name: mc-101, mc-707...", a.livePort(), func(s string) {
				a.set.LivePort = strings.ToLower(strings.TrimSpace(s))
				a.saveSettings()
				m.items[1].vals = []string{a.livePort()}
				a.liveOpen()
			})
		}},
		{label: "Channel", vals: chans, val: a.liveChan() - 1, keepOpen: true, onChange: func(v int) { a.set.LiveChan = v + 1; a.saveSettings() }},
		{label: "TAKE", header: true},
		{label: "Snap the starts to", vals: snaps, val: a.set.LiveSnap, keepOpen: true, onChange: func(v int) {
			a.set.LiveSnap = v
			a.saveSettings()
			if a.srec != nil && a.srec.take != nil {
				a.srec.take.snap = liveSnaps[v].ticks
			}
		}},
		{label: "The pad's sample sounds at", vals: octs, val: octIdx, keepOpen: true, onChange: func(v int) { a.set.LiveOct = v + 1; a.saveSettings() }},
		{label: armLabel(), keepOpen: true},
		{label: "Clear the take (stay armed)", keepOpen: true, run: func(int) {
			if r := a.srec; r != nil && r.take != nil {
				r.take = newLiveTake(r.length, liveSnaps[a.set.LiveSnap].ticks)
				a.say("the take is empty again", false)
			}
		}},
	}
	arm := len(m.items) - 2
	m.items[arm].run = func(int) {
		r := a.srec
		if r == nil {
			return
		}
		if !r.armed {
			r.armed = true
			r.take = newLiveTake(r.length, liveSnaps[a.set.LiveSnap].ticks)
			m.items[arm].label = armLabel()
			if _, _, _, ok := a.synthTiming(r.length); !ok {
				a.say("armed: Space plays the pattern (or start the unit), then play", false)
			} else {
				a.say("armed: play", false)
			}
			return
		}
		a.liveWrite()
		m.items[arm].label = armLabel()
	}
	m.onClose = func() {
		if r := a.srec; r != nil && r.armed && r.take != nil && len(r.take.notes) > 0 {
			a.say(fmt.Sprintf("the take (%d notes) is thrown away", len(r.take.notes)), false)
		}
		a.liveClose()
	}
	m.onX = func() { a.togglePlay() }
	m.xHintOf = func() string {
		if a.seqOn {
			return "stop"
		}
		return "play"
	}
	m.footH = 6
	m.foot = func(g Gfx, x, y, w int) {
		g.HRule(x+1, y, w-2, cLine)
		g.Fill(x+1, y, 7, 1, cPanel2)
		g.Text(x+2, y, "NOW", cDim, true)
		for i, l := range a.liveSays(w - 4) {
			c := cText
			if i == 0 && a.srec != nil && a.srec.armed {
				c = cBad
			}
			g.Text(x+2, y+1+i, l, c, false)
		}
	}
	a.menu = m
}

// liveSays is the dialog's NOW: armed or not, the take, the loop, the input.
func (a *App) liveSays(w int) []string {
	r := a.srec
	if r == nil {
		return nil
	}
	var out []string
	if r.armed {
		n := len(r.take.notes)
		out = append(out, fit(fmt.Sprintf("● ARMED %s: %s on channel %d, %d passes", a.label(r.pad), count(n, "note"), a.liveChan(), len(r.take.passes)), w))
	} else {
		out = append(out, fit("not armed: A on Arm, then play on the synth", w))
	}
	if t, ok := a.liveTicks(a.clock()); ok {
		in := int(t) % max(r.length, 1)
		out = append(out, fit(fmt.Sprintf("the loop: bar %d.%d of %d", in/(16*stepTicks)+1, in%(16*stepTicks)/(4*stepTicks)+1, max(r.length/(16*stepTicks), 1)), w))
	} else {
		out = append(out, fit("the loop: stopped; Space plays the pattern, or start the unit", w))
	}
	if r.ok {
		out = append(out, fit("listening to "+r.port, w))
	} else {
		out = append(out, wrap(r.port, w)...)
	}
	out = append(out, fit("Esc throws the take away", w))
	return out
}

// liveWrite disarms and writes the pattern again to the next empty slot, the take on the armed pad (with the notes it had).
func (a *App) liveWrite() {
	r := a.srec
	r.armed = false
	if t, ok := a.liveTicks(a.clock()); ok {
		r.take.finish(t)
	}
	if len(r.take.notes) == 0 {
		a.say("disarmed: nothing was played on channel "+fmt.Sprint(a.liveChan()), true)
		return
	}
	notes, folded := r.take.padNotes(r.pad, a.liveRef(r.pad))
	to := a.freeSlot()
	if to == 0 {
		a.say("every pattern is used", true)
		return
	}
	pre := a.lab.Snapshot()
	from := a.pat
	if _, err := a.lab.Pattern(from); err == nil {
		if err := a.lab.CopyPattern(from, to); err != nil {
			a.say(err.Error(), true)
			return
		}
	}
	a.pat = to
	all := append(a.trackNotes(r.pad), notes...)
	if err := a.setTrack(r.pad, all); err != nil {
		a.pat = from
		a.say(err.Error(), true)
		return
	}
	a.commitUndo(pre, A, 0)
	a.refresh()
	a.autoRows()
	msg := fmt.Sprintf("%s: %s on %s, played on the synth", a.label(to-1), count(len(notes), "note"), a.label(r.pad))
	if folded > 0 {
		msg += fmt.Sprintf(" (%d an octave in: a pad goes an octave up and down)", folded)
	}
	a.say(msg, false)
	r.take = nil
}
