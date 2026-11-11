package core

import (
	"encoding/json"
	"fmt"
	"image/color"
	"sort"
	"time"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/lab"
)

// stepTicks is one sixteenth note.
const stepTicks = formats.PPQ / 4

var velSteps = []int{127, 100, 70, 40}

// doc reads the pattern in front; ok is false when it is empty (it does not exist yet).
func (a *App) doc() (lab.PatternDoc, bool) {
	var d lab.PatternDoc
	b, err := a.lab.Pattern(a.pat)
	if err != nil || json.Unmarshal(b, &d) != nil {
		return lab.PatternDoc{Bars: 4, Length: 4 * 4 * formats.PPQ, Beats: 4}, false
	}
	return d, true
}

func (a *App) steps() int {
	d, _ := a.doc()
	return d.Bars * d.Beats * 4
}

// rowPad is the pad of the row under the cursor.
func (a *App) rowPad() int {
	if a.row < 0 || a.row >= len(a.rows) {
		return a.padN()
	}
	return a.rows[a.row]
}

const maxRows = 16

// autoRows works out the rows of the step grid: only the tracks in use, the pads that have notes in the pattern, with the marked
// pads (the keys of an arp) and the tracks the user added; for an empty pattern the pad that was in front, to start with. The pad
// the cursor is on stays while the cursor is on it, even when its last note has been taken off. In pad order.
func (a *App) autoRows() {
	keep := -1
	if a.rowFocus >= 0 {
		keep, a.rowFocus = a.rowFocus, -1
	} else if a.rowsSet && a.row >= 0 && a.row < len(a.rows) {
		keep = a.rows[a.row]
	}
	set := map[int]bool{}
	d, _ := a.doc()
	for _, n := range d.Notes {
		set[n.Pad] = true
	}
	for _, p := range a.marks {
		set[p] = true
	}
	if keep >= 0 {
		set[keep] = true
	}
	var extras []int // a track that was added stays while it has notes or the cursor is on it
	for _, p := range a.rowExtra {
		if set[p] {
			extras = append(extras, p)
		}
	}
	a.rowExtra = extras
	for _, p := range extras {
		set[p] = true
	}
	if len(set) == 0 {
		set[a.padN()] = true // nothing yet: the pad that was in front
	}
	picks := make([]int, 0, len(set))
	for p := range set {
		picks = append(picks, p)
	}
	sort.Ints(picks)
	a.rows = picks
	a.rowsSet = true
	target := keep
	if target < 0 {
		target = a.padN()
	}
	a.row = 0
	for i, p := range picks {
		if p == target {
			a.row = i
		}
	}
	if a.row < a.rowTop {
		a.rowTop = a.row
	}
	if a.row >= a.rowTop+maxRows {
		a.rowTop = a.row - maxRows + 1
	}
	a.rowTop = max(0, min(a.rowTop, len(picks)-maxRows))
}

// addTrack puts a pad among the rows (it has no notes yet) and the cursor on it.
func (a *App) addTrack(p int) {
	for _, q := range a.rowExtra {
		if q == p {
			a.rowFocus = p
			a.autoRows()
			return
		}
	}
	a.rowExtra = append(a.rowExtra, p)
	a.rowFocus = p
	a.autoRows()
	a.say(fmt.Sprintf("a track for %s", a.label(p)), false)
}

// notesIn are the notes of a pad that start in a step: one, or several for a split (ratcheted) step.
func notesIn(d lab.PatternDoc, pad, step int) []lab.Note {
	var out []lab.Note
	for _, n := range d.Notes {
		if n.Pad == pad && int(n.Tick) >= step*stepTicks && int(n.Tick) < (step+1)*stepTicks {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Tick < out[j].Tick })
	return out
}

// noteAt finds the note of a pad that starts in a step.
func noteAt(d lab.PatternDoc, pad, step int) (lab.Note, bool) {
	for _, n := range d.Notes {
		if n.Pad == pad && int(n.Tick) >= step*stepTicks && int(n.Tick) < (step+1)*stepTicks {
			return n, true
		}
	}
	return lab.Note{}, false
}

func (a *App) seqKey(k Key, repeat int) {
	if a.aHeld {
		a.noteEditKey(k, repeat)
		return
	}
	if a.patStrip {
		a.stripKey(k)
		return
	}
	jump := 1 + repeat/4 // a held key runs faster
	switch k {
	case Left: // the steps run across the screen
		a.step = max(a.step-jump, 0)
	case Right:
		a.step = min(a.step+jump, a.steps()-1)
	case Up: // the pads run down it
		a.row = max(a.row-1, 0)
		a.autoRows() // the track left drops out when it has no notes
	case Down:
		a.row = min(a.row+1, len(a.rows)-1)
		a.autoRows()
	case A:
		a.holdNote()
	case X:
		a.togglePlay()
	case B: // the settings of the note under the cursor (the tools of the pattern: Select+B); also while the pattern plays
		a.openNotePanel()
	}
}

// holdNote is A going down in the grid. On an empty cell it puts a note there (and A held goes on to shape it); on a note it
// waits: if A is let go with nothing edited the note goes, if the arrows are used it is edited.
func (a *App) holdNote() {
	pad := a.rowPad()
	a.aHeld, a.aDelete, a.aPad = true, false, pad
	a.aSession++
	d, _ := a.doc()
	if n, ok := noteAt(d, pad, a.step); ok {
		a.aTick, a.aDelete = n.Tick, true
		return
	}
	a.toggle(pad, a.step) // a new note, a step long
	a.aTick = uint32(a.step * stepTicks)
}

// noteEditKey is a key pressed while A is held on a note: Up and Down change the velocity, Left and Right the length, L1 and
// R1 move the note earlier or later. A key held down goes faster.
func (a *App) noteEditKey(k Key, repeat int) {
	d, ok := a.doc()
	if !ok {
		return
	}
	var n lab.Note
	found := false
	for _, x := range d.Notes {
		if x.Pad == a.aPad && x.Tick == a.aTick {
			n, found = x, true
		}
	}
	if !found {
		return
	}
	acc := accel(repeat)
	vel, ln, move := n.Vel, n.Len, 0
	switch k {
	case Up:
		vel += acc
	case Down:
		vel -= acc
	case Right:
		ln += 15 * acc
	case Left:
		ln -= 15 * acc
	case R1:
		move = 5 * acc
	case L1:
		move = -5 * acc
	default:
		return
	}
	a.aDelete = false // it is edited now: letting go keeps it
	vel = min(max(vel, 1), 127)
	ln = min(max(ln, 15), max(int(d.Length)-int(n.Tick), 15))
	move = min(max(move, -int(n.Tick)), int(d.Length)-1-int(n.Tick))
	args, _ := json.Marshal(lab.PatternArgs{Notes: []lab.PadAt{{Pad: n.Pad, Tick: n.Tick}}, Vel: vel, Len: ln, Move: move})
	if err := a.lab.PatternOp(a.pat, "edit", args); err != nil {
		a.say(err.Error(), true)
		return
	}
	a.aTick = uint32(int(n.Tick) + move)
	a.step = int(a.aTick) / stepTicks
	a.refresh()
	a.replay()
	if s, ok := a.noteSample(a.aPad, vel, ln, n.Pitch); ok && k != L1 && k != R1 {
		a.play(s)
	}
	a.say(fmt.Sprintf("%s: velocity %d, length %.2f steps, %+d ticks", a.label(a.aPad), vel, float64(ln)/stepTicks, int(a.aTick)-int(a.aTick)/stepTicks*stepTicks), false)
}

func (a *App) seqShifted(k Key) {
	switch k {
	case Up:
		a.setPattern(a.pat + 1)
	case Down:
		a.setPattern(a.pat - 1)
	case A:
		a.toggleFreeze()
	case B:
		a.openPicker()
	case X:
		a.addTrackPick()
	}
}

// addTrackPick asks for a pad (the dialog the patterns are chosen in), starting on the next pad with a sample that is not a row yet.
func (a *App) addTrackPick() {
	shown := map[int]bool{}
	for _, p := range a.rows {
		shown[p] = true
	}
	start := a.rowPad()
	for off := 1; off < 160; off++ {
		p := (a.rowPad() + off) % 160
		if a.info.Pads[p].File && !shown[p] {
			start = p
			break
		}
	}
	a.pickPad("Add a track: choose a pad", start, true, a.addTrackNote)
}

// addTrackNote makes a track of the pad and puts a note on its first step (1.1), so the track is there to be played and shaped.
func (a *App) addTrackNote(p int) {
	a.addTrack(p)
	if d, ok := a.doc(); !ok || !hasNote(d, p, 0) {
		a.toggle(p, 0)
	}
	a.step = 0
	a.refresh()
	a.say(fmt.Sprintf("a track for %s, with a note on 1.1", a.label(p)), false)
}

func hasNote(d lab.PatternDoc, pad, step int) bool {
	_, ok := noteAt(d, pad, step)
	return ok
}

func (a *App) setPattern(n int) {
	a.rowExtra = nil // the tracks added belong to the pattern they were added to
	a.pat = (n+159)%160 + 1
	a.step = min(a.step, a.steps()-1)
	a.autoRows()
	if a.seqOn {
		a.replay()
	}
}

func (a *App) toggle(pad, step int) {
	d, ok := a.doc()
	if !ok {
		if err := a.lab.NewPattern(a.pat, 4); err != nil {
			a.say(err.Error(), true)
			return
		}
		d, _ = a.doc()
	}
	if n, ok := noteAt(d, pad, step); ok {
		args, _ := json.Marshal(lab.PatternArgs{Notes: []lab.PadAt{{Pad: n.Pad, Tick: n.Tick}}, Delete: true})
		if err := a.lab.PatternOp(a.pat, "edit", args); err != nil {
			a.say(err.Error(), true)
		}
	} else {
		args, _ := json.Marshal(lab.PatternArgs{Pad: pad, Tick: uint32(step * stepTicks), Vel: 100, Len: stepTicks})
		if err := a.lab.PatternOp(a.pat, "insert", args); err != nil {
			a.say(err.Error(), true)
		}
	}
	a.refresh()
	a.replay()
}

// setRow puts notes on the steps of a pad that are in the mask of each bar, taking the others off.
func (a *App) setRow(pad int, mask []int) {
	d, ok := a.doc()
	if !ok {
		if err := a.lab.NewPattern(a.pat, 4); err != nil {
			a.say(err.Error(), true)
			return
		}
		d, _ = a.doc()
	}
	args, _ := json.Marshal(lab.PatternArgs{Pad: pad})
	a.lab.PatternOp(a.pat, "clearPad", args)
	for bar := 0; bar < d.Bars; bar++ {
		for _, s := range mask {
			args, _ := json.Marshal(lab.PatternArgs{Pad: pad, Tick: uint32((bar*16 + s) * stepTicks), Vel: 100, Len: stepTicks})
			a.lab.PatternOp(a.pat, "insert", args)
		}
	}
	a.refresh()
	a.replay()
}

func euclid(hits int) []int {
	var m []int
	for i := 0; i < hits; i++ {
		m = append(m, i*16/hits)
	}
	return m
}

var rowPresets = []struct {
	name string
	mask []int
}{
	{"every beat", []int{0, 4, 8, 12}},
	{"backbeat 2 4", []int{4, 12}},
	{"off-beats", []int{2, 6, 10, 14}},
	{"eighths", []int{0, 2, 4, 6, 8, 10, 12, 14}},
	{"sixteenths", []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}},
	{"boom-bap kick", []int{0, 7, 10}},
	{"3+3+2", []int{0, 3, 6, 8, 11, 14}},
}

// seqMenu is the pattern tools: the pattern as a whole, its tempo, timing and length, and the track (the pad of the row in front).
func (a *App) seqMenu() {
	pad := a.rowPad()
	m := &menu{title: fmt.Sprintf("pattern %s  row %s", a.label(a.pat-1), a.label(pad))}
	run := func(label string, f func()) {
		m.items = append(m.items, menuItem{label: label, run: func(int) { f() }})
	}
	set := func(label string, vals []string, cur int, change func(v int)) {
		m.items = append(m.items, menuItem{label: label, vals: vals, val: cur, keepOpen: true, onChange: change})
	}

	m.tabHead("PATTERN")
	run("Choose a pattern (bank, then pattern)...", a.openPicker)
	run("New pattern from a template...", a.templatesMenu)
	run(variationLabel, a.variationMenu)
	set("Loop only the bar in focus (Y+A)", []string{"off", "on"}, b2i(a.freeze), func(v int) {
		if (v == 1) != a.freeze {
			a.toggleFreeze()
		}
	})
	run("New pattern from the audible tracks", a.newFromAudible)
	run("Bounce the tracks as you hear them to a pad", func() { a.bounce(a.audible, "mix") })
	run("Bounce only this track to a pad", func() { a.bounce(func(p int) bool { return p == pad }, a.label(pad)) })
	run("Unmute and unsolo every track", func() {
		delete(a.trk, a.pat)
		a.say("every track plays", false)
		a.replay()
	})
	run("Copy to the next pattern", func() {
		to := a.pat%160 + 1
		if err := a.lab.CopyPattern(a.pat, to); err != nil {
			a.say(err.Error(), true)
			return
		}
		a.pat = to
		a.refresh()
		a.autoRows()
		a.say("copied to "+a.label(to-1), false)
	})
	run("Copy to a slot...", func() {
		from := a.pat
		a.pickPattern("copy to which pattern?", a.pat%160+1, func(to int) {
			if to == from {
				a.say("that is this pattern", true)
				return
			}
			pre := a.lab.Snapshot()
			if err := a.lab.CopyPattern(from, to); err != nil {
				a.say(err.Error(), true)
				return
			}
			a.commitUndo(pre, A, 0)
			a.refresh()
			a.say(fmt.Sprintf("%s copied to %s", a.label(from-1), a.label(to-1)), false)
		})
	})
	run("Move to a slot...", func() {
		from := a.pat
		a.pickPattern("move to which pattern?", a.pat%160+1, func(to int) {
			if to == from {
				a.say("that is this pattern", true)
				return
			}
			pre := a.lab.Snapshot()
			if err := a.lab.CopyPattern(from, to); err != nil {
				a.say(err.Error(), true)
				return
			}
			a.lab.DeletePattern(from)
			a.commitUndo(pre, A, 0)
			a.pat = to
			a.refresh()
			a.autoRows()
			a.say(fmt.Sprintf("%s moved to %s", a.label(from-1), a.label(to-1)), false)
		})
	})
	run("Use the pads of another bank...", a.retargetMenu)
	run("Delete the pattern", func() {
		a.lab.DeletePattern(a.pat)
		a.refresh()
		a.replay()
	})

	m.head("TEMPO")
	bpms := make([]string, 0, 181)
	for v := 60; v <= 240; v++ {
		bpms = append(bpms, fmt.Sprintf("%d BPM", v))
	}
	bank := (a.pat - 1) / 16
	what := fmt.Sprintf("Tempo of bank %c", 'A'+bank)
	if a.info.UsesProjectTempo {
		what = "Tempo of the project"
	}
	set(what, bpms, min(max(int(a.tempo()+0.5)-60, 0), len(bpms)-1), func(v int) {
		var err error
		if a.info.UsesProjectTempo {
			err = a.lab.SetTempo(float64(v + 60))
		} else {
			err = a.lab.SetBankTempo(bank, float64(v+60))
		}
		if err != nil {
			a.say(err.Error(), true)
			return
		}
		a.refresh()
		a.replay()
	})
	sel := 0
	if a.info.UsesProjectTempo {
		sel = 1
	}
	set("Tempo select", []string{"each bank's own", "the project's"}, sel, func(v int) {
		a.lab.SetUsesProjectTempo(v == 1)
		a.refresh()
		a.replay()
		a.say(fmt.Sprintf("tempo: %s, %s BPM", map[bool]string{true: "the project's", false: "bank " + string(rune('A'+bank))}[v == 1], bpmText(a.tempo())), false)
	})

	scope := 0
	if a.scopeRow {
		scope = 1
	}
	m.tabHead("TIMING")
	set("Applies to", []string{"every pad", "only " + a.label(pad)}, scope, func(v int) { a.scopeRow = v == 1 })
	target := func() int { // the pad the timing tools act on: -1 for every pad
		if a.scopeRow {
			return pad
		}
		return -1
	}
	m.items = append(m.items, menuItem{label: "Swing", vals: []string{"54%", "58%", "62%", "66%", "70%"}, val: 2, keepOpen: true, run: func(v int) {
		args, _ := json.Marshal(lab.PatternArgs{Pad: target(), Grid: stepTicks, Amount: float64([]int{54, 58, 62, 66, 70}[v])})
		a.opSay("swing", args)
	}})
	m.items = append(m.items, menuItem{label: "Humanize", vals: []string{"light", "medium", "loose"}, val: 1, keepOpen: true, run: func(v int) {
		args, _ := json.Marshal(lab.PatternArgs{Pad: target(), Timing: uint32([]int{6, 12, 24}[v]), Velocity: []int{6, 12, 24}[v], Seed: int64(a.step + 1)})
		a.opSay("humanize", args)
	}})
	m.items = append(m.items, menuItem{label: "Quantize to 1/16", keepOpen: true, run: func(int) {
		args, _ := json.Marshal(lab.PatternArgs{Pad: target(), Grid: stepTicks, Strength: 1})
		a.opSay("quantize", args)
	}})
	m.items = append(m.items, menuItem{label: "Shift", vals: []string{"1 step <", "1 step >"}, val: 1, keepOpen: true, run: func(v int) {
		args, _ := json.Marshal(lab.PatternArgs{Pad: target(), Grid: stepTicks, Steps: []int{-1, 1}[v]})
		a.opSay("shift", args)
	}})
	m.items = append(m.items, menuItem{label: "Reverse", keepOpen: true, run: func(int) {
		args, _ := json.Marshal(lab.PatternArgs{Pad: target()})
		a.opSay("reverse", args)
	}})

	m.head("LENGTH")
	m.items = append(m.items, menuItem{label: "Double", keepOpen: true, run: func(int) { a.opSay("double", []byte(`{"pad":-1}`)); a.step = min(a.step, a.steps()-1) }})
	m.items = append(m.items, menuItem{label: "Halve", keepOpen: true, run: func(int) { a.opSay("halve", []byte(`{"pad":-1}`)); a.step = min(a.step, a.steps()-1) }})

	m.tabHead("NOTES")
	m.head("WRITE")
	run("Fill...", a.fillMenu)
	run("Euclid...", a.euclidMenu)
	run("Evolve over the bars...", a.evolveMenu)
	m.head("CHANGE THE NOTES")
	run("Ratchet (sub-steps)...", a.ratchetMenu)
	run("Accents and ghosts...", a.accentMenu)
	run("LFO on the notes (velocity, length, timing)...", a.lfoMenu)
	run("ENV: each note shapes the next (velocity, length, timing)...", a.envMenu)
	run("Echo...", a.echoMenu)
	run("Chance...", a.chanceMenu)
	run("Randomize velocity...", a.randomizeMenu)
	run("Shape (gate, decay)...", a.shapeMenu)
	run("Cycle...", a.cycleMenu)
	run("Nudge (push, pull)...", a.nudgeMenu)
	run("Pitch (transpose, random, runs)...", a.pitchMenu)
	run("Invert", a.invert)
	m.tabHead("TRACKS")
	run("Dispatch over the marked pads...", a.dispatchMenu)
	run("Copy to a pad...", a.copyTrackMenu)
	run("Swap with a pad...", a.swapTrackPick)
	run("Clear", func() {
		args, _ := json.Marshal(lab.PatternArgs{Pad: pad})
		a.lab.PatternOp(a.pat, "clearPad", args)
		a.refresh()
		a.replay()
	})
	m.items = append(m.items, menuItem{label: "Add a track (choose a pad)...", run: func(int) { a.addTrackPick() }})
	m.tabify()
	a.menu = m
}

func (a *App) opSay(op string, args []byte) {
	if err := a.lab.PatternOp(a.pat, op, args); err != nil {
		a.say(err.Error(), true)
		return
	}
	a.refresh()
	a.say(op, false)
	a.replay()
}

// render mixes the pattern into one stereo sample: every note starts the sample of its pad at its tick.
func (a *App) render() (formats.Sample, bool) { return a.renderOf(a.audible, a.freeze) }

// renderOf is render for the pads the filter lets through; frozen is the bar in focus, else the whole pattern.
// pitchedID is the sample played that many semitones up or down (tape style, as the unit's chromatic keys do), made once and kept.
func (a *App) pitchedID(id, semitones int) int {
	if semitones == 0 {
		return id
	}
	key := [2]int{id, semitones}
	if pid, ok := a.pitchMade[key]; ok {
		return pid
	}
	pid, err := a.lab.Pitched(id, semitones)
	if err != nil {
		return id
	}
	if a.pitchMade == nil {
		a.pitchMade = map[[2]int]int{}
	}
	a.pitchMade[key] = pid
	return pid
}

func (a *App) renderOf(play func(pad int) bool, frozen bool) (formats.Sample, bool) {
	d, ok := a.doc()
	if !ok {
		return formats.Sample{}, false
	}
	bpm := a.tempo()
	perTick := 60 / bpm / formats.PPQ * formats.SampleRate
	length := int(d.Length)
	lo, hi := 0, length
	if frozen { // only the bar in focus
		span := a.viewSpan() // the bar, or the two bars, in view
		lo = min(a.step/span*span*stepTicks, max(length-stepTicks, 0))
		hi = min(lo+span*stepTicks, length)
		a.frozenPage = a.step / span
	}
	n := int(float64(hi-lo) * perTick)
	mix := make([]int32, n*2)
	for _, nt := range d.Notes {
		if int(nt.Tick) < lo || int(nt.Tick) >= hi || !play(nt.Pad) {
			continue
		}
		id, ok := a.padSample(nt.Pad)
		if over, auditioned := a.aud[nt.Pad]; auditioned { // a file heard in place of the pad's sample
			id, ok = over, true
		}
		if !ok {
			continue
		}
		id = a.pitchedID(id, formats.PitchSemitones(uint8(nt.Pitch))) // a chromatic note is played up or down
		s, err := a.lab.Sample(id)
		if err != nil {
			continue
		}
		at := int(float64(int(nt.Tick)-lo) * perTick)
		g := int32(nt.Vel)
		for f := 0; f < s.Frames() && at+f < n; f++ {
			l := int32(s.Samples[f*s.Channels])
			r := int32(s.Samples[f*s.Channels+s.Channels-1])
			mix[(at+f)*2] += l * g / 127
			mix[(at+f)*2+1] += r * g / 127
		}
	}
	out := make([]int16, len(mix))
	for i, v := range mix {
		out[i] = int16(max(-32767, min(32767, v)))
	}
	return formats.Sample{Channels: 2, Samples: out}, true
}

type looper interface{ Loop(formats.Sample) error }
type replacer interface {
	ReplaceLoop(formats.Sample) error
}

func (a *App) togglePlay() {
	if a.seqOn {
		a.stop()
		a.seqOn = false
		return
	}
	a.startLoop()
}

func (a *App) startLoop() {
	a.playPat = a.pat // the pattern that plays is the one in view when it starts; the view may go on to others
	s, ok := a.render()
	if !ok {
		a.say("the pattern is empty: A puts a note", true)
		return
	}
	lp, isLoop := a.Audio.(looper)
	if a.Audio == nil || !isLoop {
		a.say("no audio output", true)
		return
	}
	if err := lp.Loop(s); err != nil {
		a.say(err.Error(), true)
		return
	}
	a.seqOn, a.seqStart = true, a.clock()
}

// replay puts an edit into the loop that plays, without a gap.
func (a *App) replay() {
	if !a.seqOn {
		return
	}
	if a.playPat != 0 && a.playPat != a.pat { // another pattern is in view: what plays is not touched (Space stops it, Space plays this one)
		return
	}
	s, ok := a.render()
	if !ok { // nothing left to play (the last note was taken away): the loop ends
		a.stop()
		return
	}
	if r, ok := a.Audio.(replacer); ok {
		r.ReplaceLoop(s)
	}
}

func (a *App) clock() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

// playStep is the step the loop is at, -1 when it is not playing.
func (a *App) playStep() int {
	if !a.seqOn || a.playPat != 0 && a.playPat != a.pat { // nothing plays, or another pattern does: no playhead in this one
		return -1
	}
	bpm := a.tempo()
	stepDur := time.Duration(float64(time.Minute) / bpm / 4)
	n := int(a.clock().Sub(a.seqStart) / stepDur)
	if a.freeze {
		span := a.viewSpan()
		return a.frozenPage*span + n%max(min(span, a.steps()-a.frozenPage*span), 1)
	}
	return n % a.steps()
}

// toggleFreeze loops only the bar in focus, or the whole pattern again.
func (a *App) toggleFreeze() {
	a.freeze = !a.freeze
	if a.freeze {
		span := a.viewSpan()
		if span > 16 {
			a.say(fmt.Sprintf("bars %d-%d are frozen: X loops only the bars in view", a.step/span*span/16+1, a.step/span*span/16+span/16), false)
		} else {
			a.say(fmt.Sprintf("bar %d is frozen: X loops only this bar, L1/R1 move the loop", a.step/16+1), false)
		}
	} else {
		a.say("the whole pattern plays again", false)
	}
	if a.seqOn {
		a.replay()
	}
}

// freezeTick moves a playing frozen loop to the bar that came into focus.
func (a *App) freezeTick() {
	if a.freeze && a.seqOn && a.step/a.viewSpan() != a.frozenPage {
		a.replay()
	}
}

func lerp(a, b color.RGBA, t float64) color.RGBA {
	f := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
	return color.RGBA{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B), 255}
}

// seqRowCells is the height of a track's row in the step grid: a row of cells, or two when there are few tracks.
func (a *App) seqRowCells() int {
	shown := min(len(a.rows)-a.rowTop, maxRows)
	room := Rows - 2 - 3
	return min(max(room/max(shown, 1), 1), 2)
}

// SeqStrip is p of a keyboard on SEQ: the arrows choose the pattern in the 4 by 4 tiles at the bottom (the bank's patterns), and the
// pattern changes as they move; p, A or B go back to the grid.
func (a *App) SeqStrip() {
	if a.scr != Seq || !Terminal || a.Modal() || a.prompt != nil || a.aHeld {
		return
	}
	a.CancelShift()
	a.patStrip = !a.patStrip
	if a.patStrip {
		a.say("patterns: the arrows choose, [ ] the bank, Space plays, p or A goes back", false)
	}
}

// StripFocus says whether the arrows are on the row of pattern boxes.
func (a *App) StripFocus() bool { return a.patStrip && a.scr == Seq }

func (a *App) stripKey(k Key) {
	switch k {
	case Left:
		a.setPattern(a.pat - 1)
	case Right:
		a.setPattern(a.pat + 1)
	case L1, R1: // the bank, as [ and ]
		a.SeqBank(map[Key]int{L1: -1, R1: 1}[k])
	case X:
		a.togglePlay()
	case A, B:
		a.patStrip = false
	case Up: // the tile above (the bank is a ring, as in the pattern dialog)
		a.setPattern((a.pat-1)/16*16 + ((a.pat-1)%16+12)%16 + 1)
	case Down:
		a.setPattern((a.pat-1)/16*16 + ((a.pat-1)%16+4)%16 + 1)
	}
}

// drawPatternStrip is the 16 patterns of the bank as a grid of tiles, 4 by 4 like the pads on PADS but a line high: the label, the bars
// and the notes. The one in front is lit (white while the arrows are on the grid), the ones with notes are bright, empty ones dim.
func (a *App) drawPatternStrip(g Gfx, x0, y0, avail int) {
	bank := (a.pat - 1) / 16
	for i := 0; i < 16; i++ {
		col, row := i%4, i/4
		x := x0 + col*avail/4
		w := x0 + (col+1)*avail/4 - x - 1
		y := y0 + row
		slot := bank*16 + i + 1
		var d lab.PatternDoc
		used := false
		if b, err := a.lab.Pattern(slot); err == nil {
			used = json.Unmarshal(b, &d) == nil && len(d.Notes) > 0
		}
		bg, fg := cPanel, cDim
		switch {
		case slot == a.pat && a.patStrip:
			bg, fg = cWhite, cBg
		case slot == a.pat:
			bg, fg = cAmber, cBg
		case used:
			bg, fg = cLine, cText
		}
		g.Fill(x, y, w, 1, bg)
		txt := fmt.Sprintf("%-3s empty", a.label(slot-1))
		if used {
			txt = fmt.Sprintf("%-3s %d bars %3d notes", a.label(slot-1), d.Bars, len(d.Notes))
			if w < 22 {
				txt = fmt.Sprintf("%-3s %3d notes", a.label(slot-1), len(d.Notes))
			}
		}
		g.Text(x+1, y, fit(txt, max(w-1, 0)), fg, slot == a.pat)
	}
}

// viewSpan is how many steps the grid shows: a bar, and on a wide terminal two or four bars side by side while their
// steps keep a width of 3 (z turns that off).
func (a *App) viewSpan() int {
	span := 16
	steps := a.steps()
	for Terminal && !a.seqOneBar && span < 64 && span < steps && (contentCols-6-1)/(span*2) >= 3 {
		span *= 2
	}
	return span
}

// drawSeq is the step grid, as on the unit: a row for each pad (up to 16), the 16 steps of one bar across, a lit block
// where a pad plays, as tall as its velocity. L1/R1 (or the cursor) turn the bar.
func (a *App) drawSeq(g Gfx) {
	d, ok := a.doc()
	title := fmt.Sprintf("PATTERN %s", a.label(a.pat-1))
	g.Text(2, 1, title, cAmber, true)
	sub := fmt.Sprintf("%d bars", d.Bars)
	if !ok {
		sub = "empty: A puts the first note"
	}
	subRoom := contentCols - 2 - 14 - 3*((a.steps()+a.viewSpan()-1)/a.viewSpan()) - (2 + len(title) + 2) // the bar label and the bars are right of it
	g.Text(2+len(title)+2, 1, fit(sub, max(subRoom, 0)), cDim, false)

	lx, x0 := 2, 6 // the pad labels, the first step
	steps := a.steps()
	span := a.viewSpan()
	avail := min(max(contentCols-x0-1, span*2), span*8) // the cells the steps share: all the width there is (up to 8 a step)
	page := a.step / span
	bars := (steps + span - 1) / span
	ph := a.playStep()
	const ry = 3
	shown := a.rows[a.rowTop:min(len(a.rows), a.rowTop+maxRows)]
	rh := a.seqRowCells()                         // few tracks: two rows each
	cw := min(max((contentCols-x0-1)/span, 2), 8) // the handheld: a step is a whole number of cells
	stepX := func(s int) int { return x0 + s*cw }
	if Terminal { // a step starts where its share of the width says: no cells are left over
		stepX = func(s int) int { return x0 + s*avail/span }
	}
	stepW := func(s int) int { return stepX(s+1) - stepX(s) }

	// the step numbers
	for s := 0; s < span && page*span+s < steps; s++ {
		st := page*span + s
		col := cDim
		if s%4 == 0 {
			col = cText
		}
		if ph == st {
			col = cGood
		}
		if st == a.step {
			col = cAmber
		}
		num := s%16 + 1
		if Terminal { // the steps are numbered through the pattern: bar 2 shows 17 to 32
			num = st + 1
		}
		g.Text(stepX(s)+(stepW(s)-len(fmt.Sprint(num)))/2, 2, fmt.Sprint(num), col, s%4 == 0)
	}
	// a band for the step the cursor is on and for the one that plays, over the rows
	for s := 0; s < span && page*span+s < steps; s++ {
		st := page*span + s
		switch {
		case ph == st:
			g.Fill(stepX(s), ry, stepW(s), rh*len(shown), cLine)
		case st == a.step && !Terminal:
			g.Fill(stepX(s), ry, stepW(s), rh*len(shown), cPanel2)
		}
	}
	cursor := [4]int{-1}
	for si, pad := range shown {
		i := a.rowTop + si // the index in all the rows
		y := ry + si*rh
		has := a.info.Pads[pad].File
		fg := cDim
		if has {
			fg = cText
		}
		bh := rh // the height of a track's boxes: in the terminal one line, with a blank line between tracks when there is room
		if Terminal {
			bh = 1
		}
		ly, lh := y, bh
		if i == a.row {
			if Terminal { // the label in the accent colour, no box
				fg = cAmber
			} else {
				g.Highlight(lx-1, ly, x0-lx+1, lh)
				fg = cWhite
			}
		}
		if a.trackMode(pad) == trackMute {
			fg = cDim
		}
		g.Text(lx, ly, a.label(pad), fg, i == a.row)
		switch a.trackMode(pad) {
		case trackMute:
			g.Text(lx+3, ly, "M", cBad, true)
		case trackSolo:
			g.Text(lx+3, ly, "S", cGood, true)
		}
		for s := 0; s < span && page*span+s < steps; s++ {
			st := page*span + s
			x := stepX(s)
			{
				bg := cPanel
				if (s/4)%2 == 1 {
					bg = cPanel2
				}
				if ph == st {
					bg = cLine
				}
				if Terminal && i == a.row && st == a.step && !a.aHeld { // the step under the cursor: its box is lit
					bg = cLine
				}
				g.StepCell(x, y, stepW(s), bh, bg)
				for _, n := range notesIn(d, pad, st) { // every note that starts in the step, where it starts: sub-steps and nudges show
					col := cAmber
					if ph == st {
						col = cGood
					}
					if a.aHeld && pad == a.aPad && n.Tick == a.aTick {
						col = cWhite // the note being edited
					} else if Terminal && i == a.row && st == a.step {
						col = cBlue // the note under the cursor, in its own colour
					}
					start := float64(int(n.Tick)-st*stepTicks) / stepTicks // how far into the step it starts
					length := float64(n.Len) / stepTicks
					if n.Len >= stepTicks { // a long note fills the rest of its step
						length = 1 - start
					}
					g.Note(x, y, stepW(s), bh, start, min(length, 1-start), float64(n.Vel)/127, col)
					// longer than a step: a tie runs on over the steps it lasts
					if n.Len > stepTicks && !Terminal { // (the terminal shows the length in the panel)
						end := int(n.Tick) + n.Len
						for t := s + 1; t < span && page*span+t < steps && (page*span+t)*stepTicks < end; t++ {
							frac := 1.0
							if rest := end - (page*span+t)*stepTicks; rest < stepTicks {
								frac = float64(rest) / stepTicks
							}
							g.Tie(stepX(t), y, stepW(t), bh, frac, col)
						}
					}
				}
			}
			if i == a.row && st == a.step {
				cursor = [4]int{x, y, stepW(s), bh}
			}
		}
	}
	if cursor[0] >= 0 {
		if !Terminal { // in the terminal the lit box and the note's colour show the cursor
			g.Cursor(cursor[0], cursor[1], cursor[2], cursor[3])
		}
	}
	if Terminal { // the banks, as on PADS: the one the pattern is in lit (the bank's 16 patterns are in the panel)
		a.drawBanks(g, (a.pat-1)/16)
	}
	// which bar this is, and where the loop is when it plays
	bx := contentCols - 2 - bars*3
	for b := 0; b < bars; b++ {
		col := cLine
		if b == page {
			col = cAmber
		}
		if ph >= 0 && ph/span == b {
			col = cGood
		}
		g.Fill(bx+b*3, 1, 2, 1, col)
	}
	barLbl := fmt.Sprintf("bar %d/%d", page+1, bars)
	if span > 16 {
		barLbl = fmt.Sprintf("bars %d-%d/%d", page*span/16+1, min((page+1)*span/16, (steps+15)/16), (steps+15)/16)
	}
	if a.freeze {
		barLbl += " FROZEN"
	}
	g.Text(bx-1-len(barLbl), 1, barLbl, cText, true)
}

// noteSample is the sample of a pad as a note plays it: as loud as its velocity, and cut where the note ends (its length, at
// the tempo of the pad's bank), with a short fade so the cut does not click.
func (a *App) noteSample(pad, vel, lenTicks, pitch int) (formats.Sample, bool) {
	id, ok := a.padSample(pad)
	if !ok {
		return formats.Sample{}, false
	}
	id = a.pitchedID(id, formats.PitchSemitones(uint8(pitch)))
	s, err := a.lab.Sample(id)
	if err != nil {
		return formats.Sample{}, false
	}
	ch := max(s.Channels, 1)
	frames := s.Frames()
	if lenTicks > 0 {
		perTick := 60 / a.tempoOf(pad/16) / formats.PPQ * formats.SampleRate
		frames = min(frames, max(int(float64(lenTicks)*perTick), 64))
	}
	fade := min(frames, int(0.008*formats.SampleRate))
	out := make([]int16, frames*ch)
	for f := 0; f < frames; f++ {
		g := int32(vel)
		if frames < s.Frames() && f >= frames-fade { // cut by the length: fade out
			g = g * int32(frames-f) / int32(fade)
		}
		for c := 0; c < ch; c++ {
			out[f*ch+c] = int16(int32(s.Samples[f*ch+c]) * g / 127)
		}
	}
	return formats.Sample{Channels: ch, Samples: out}, true
}

// playNote plays the note under the cursor, as it sounds in the pattern (a cell without a note plays its pad at velocity 100).
func (a *App) playNote() {
	pad := a.rowPad()
	d, _ := a.doc()
	vel, ln, pitch := 100, 0, 0
	if n, ok := noteAt(d, pad, a.step); ok {
		vel, ln, pitch = n.Vel, n.Len, n.Pitch
	}
	s, ok := a.noteSample(pad, vel, ln, pitch)
	if !ok {
		a.say(a.label(pad)+" has no sample", true)
		return
	}
	a.play(s)
}

// noteTiming says how far a note is from the 16th grid: "on the grid", or "5 ticks late (5.2 ms)". The short form is "5t late".
func (a *App) noteTiming(n lab.Note, pad int) string { return a.noteTimingForm(n, pad, false) }

func (a *App) noteTimingForm(n lab.Note, pad int, short bool) string {
	off := int(n.Tick) % stepTicks
	if off == 0 {
		return "on the grid"
	}
	late := off
	word := "late"
	if off > stepTicks/2 { // nearer the next step: it is early for that one
		late = off - stepTicks
		word = "early"
	}
	ms := float64(abs(late)) * 60000 / (a.tempoOf(pad/16) * formats.PPQ)
	if short {
		return fmt.Sprintf("%dt %s (%.1fms)", abs(late), word, ms)
	}
	return fmt.Sprintf("%d ticks %s (%.1f ms)", abs(late), word, ms)
}

// noteInfo is what is known of the note under the cursor, for the footer: where, how loud, how long, how far off the grid, and
// whether it is a chromatic note (a pitch byte other than 0) or a plain one.
func (a *App) noteInfo() string {
	d, ok := a.doc()
	if !ok {
		return ""
	}
	pad := a.rowPad()
	n, ok := noteAt(d, pad, a.step)
	if !ok {
		return ""
	}
	beat := int(n.Tick) % formats.PPQ
	kind := formats.PitchText(uint8(n.Pitch)) // "plain", or how many semitones it is up or down
	return fmt.Sprintf("%s %d.%d  vel %d  len %.2f st  %s  beat+%dt  %s", a.label(pad), int(n.Tick)/(16*stepTicks)+1,
		int(n.Tick)/stepTicks%16+1, n.Vel, float64(n.Len)/stepTicks, a.noteTimingForm(n, pad, true), beat, kind)
}

// ---- mute and solo (R2 on a row: normal, mute, solo). Kept for the session, per pattern; the project is not touched.

const (
	trackOn = iota
	trackMute
	trackSolo
)

func (a *App) trackMode(pad int) int { return a.trk[a.pat][pad] }

// audible is whether the pad's notes play: with any solo only the solos do, otherwise all but the muted ones.
func (a *App) audible(pad int) bool {
	m := a.trk[a.pat]
	if len(m) == 0 {
		return true
	}
	for _, v := range m {
		if v == trackSolo {
			return m[pad] == trackSolo
		}
	}
	return m[pad] != trackMute
}

// cycleTrack is R2 in the grid.
func (a *App) cycleTrack() {
	if len(a.rows) == 0 {
		return
	}
	pad := a.rowPad()
	if a.trk == nil {
		a.trk = map[int]map[int]int{}
	}
	if a.trk[a.pat] == nil {
		a.trk[a.pat] = map[int]int{}
	}
	m := (a.trk[a.pat][pad] + 1) % 3
	if m == trackOn {
		delete(a.trk[a.pat], pad)
	} else {
		a.trk[a.pat][pad] = m
	}
	a.say(a.label(pad)+[]string{" plays", " is muted", " is solo"}[m], false)
	a.replay()
}

// newFromAudible copies the pattern into the next empty one without the tracks that do not sound now.
func (a *App) newFromAudible() {
	d, ok := a.doc()
	if !ok {
		a.say("the pattern is empty", true)
		return
	}
	to := 0
	for off := 1; off < 160; off++ {
		n := (a.pat-1+off)%160 + 1
		if _, err := a.lab.Pattern(n); err != nil {
			to = n
			break
		}
	}
	if to == 0 {
		a.say("every pattern is used", true)
		return
	}
	if err := a.lab.CopyPattern(a.pat, to); err != nil {
		a.say(err.Error(), true)
		return
	}
	dropped := map[int]bool{}
	from := a.pat
	a.pat = to
	for _, nt := range d.Notes {
		if !a.audibleIn(from, nt.Pad) && !dropped[nt.Pad] {
			dropped[nt.Pad] = true
			args, _ := json.Marshal(lab.PatternArgs{Pad: nt.Pad})
			a.lab.PatternOp(to, "clearPad", args)
		}
	}
	a.refresh()
	a.autoRows()
	a.say(fmt.Sprintf("%s: %d tracks left out", a.label(to-1), len(dropped)), false)
}

func (a *App) audibleIn(pat, pad int) bool {
	save := a.pat
	a.pat = pat
	ok := a.audible(pad)
	a.pat = save
	return ok
}

// bounce renders the pattern (all of it, the tracks the filter lets through) into one sample on the next empty pad.
func (a *App) bounce(play func(int) bool, tag string) {
	a.pickFreePad("Bounce to a pad", a.rowPad()+1, func(to int) {
		s, ok := a.renderOf(play, false)
		if !ok {
			a.say("nothing to bounce", true)
			return
		}
		mb, err := a.lab.Decode(s.WAV(), "wav")
		if err != nil {
			a.say(err.Error(), true)
			return
		}
		var meta struct{ ID int }
		if json.Unmarshal(mb, &meta) != nil {
			a.say("cannot keep the bounce", true)
			return
		}
		err = a.assignPad(to, meta.ID, fit(fmt.Sprintf("bnc_%s_%s", a.label(a.pat-1), tag), 15))
		a.lab.Free(meta.ID)
		if err != nil {
			a.say(err.Error(), true)
			return
		}
		delete(a.pcm, to)
		a.refresh()
		a.say("bounced to "+a.label(to), false)
	})
}

// retargetMenu asks for a bank and plays the pattern's notes on the pads of that bank instead (the same pad numbers).
func (a *App) retargetMenu() {
	slot := a.pat
	m := &menu{title: "pattern " + a.label(slot-1) + ": use the pads of bank"}
	for b := 0; b < 10; b++ {
		b := b
		m.items = append(m.items, menuItem{label: fmt.Sprintf("Bank %c", 'A'+b), run: func(int) {
			pb, err := a.lab.PatternBytes(slot)
			if err != nil {
				a.say(err.Error(), true)
				return
			}
			from := 0
			if used := patternBanks(pb); len(used) > 0 {
				from = used[0]
			}
			pre := a.lab.Snapshot()
			n := formats.RetargetBank(pb, uint16(from), uint16(b))
			if n == 0 || from == b {
				a.say("no notes to move", true)
				return
			}
			if err := a.lab.SetPatternBytes(slot, pb); err != nil {
				a.say(err.Error(), true)
				return
			}
			a.commitUndo(pre, A, 0)
			a.refresh()
			a.autoRows()
			a.say(fmt.Sprintf("%d notes now on the pads of bank %c", n, 'A'+b), false)
		}})
	}
	a.menu = m
}

// patternBanks are the banks whose pads a pattern plays, the first is where the notes are now.
func patternBanks(b []byte) []int {
	p, err := formats.ParsePattern(b)
	if err != nil {
		return nil
	}
	seen := map[int]bool{}
	var out []int
	for _, n := range p.Events {
		if n.Control {
			continue
		}
		bank := int(n.Pad.Bank())
		if !seen[bank] {
			seen[bank] = true
			out = append(out, bank)
		}
	}
	return out
}

// patternBars is the length of a pattern file in bars.
func patternBars(b []byte) int {
	p, err := formats.ParsePattern(b)
	if err != nil || p.Length == 0 {
		return 0
	}
	return int((p.Length + 4*formats.PPQ - 1) / (4 * formats.PPQ))
}

// patternPadsOf are the pads (0..159) a pattern has notes for.
func patternPadsOf(b []byte) []int {
	p, err := formats.ParsePattern(b)
	if err != nil {
		return nil
	}
	var out []int
	for _, e := range p.Events {
		if !e.Control {
			out = append(out, int(e.Pad.Index()))
		}
	}
	return out
}
