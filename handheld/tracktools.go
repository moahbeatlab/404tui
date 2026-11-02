package handheld

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"sort"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/lab"
	"github.com/moahbeatlab/404tui/proto"
)

// Track tools (from the SEQ menu): what is done with the notes of one pad in the pattern, its track. They are the tools
// of a MIDI FX chain (Loop Cartographer's EUCLID, CHANCE, RANDOMIZE, ECHO, LOOP STRETCH, DISPATCH), made for a sample
// pad: they read the track, make a new one, and write it back. Each can be tried and undone (Y+L2).

// trackState is the settings of the track tools, kept between visits.
type trackState struct {
	euSteps, euHits, euRot, euRate, euGate, euVel int
	chance, chanceLoop                            int
	rndAmount, rndShape, rndBias                  int
	ecRate, ecRepeats, ecDecay, ecGate            int
	cycle                                         int
	dispatch                                      int
	copyTo                                        int
	euDecay                                       int
	fillPat, fillGate, fillDecay, fillVel         int
	shapeGate, shapeDecay                         int
	seed                                          int64
	init                                          bool
}

func (a *App) trackDefaults() *trackState {
	t := &a.tt
	if !t.init {
		*t = trackState{euSteps: 16, euHits: 5, euRate: 3, euGate: 1, euVel: 1, chance: 70, ecRate: 1, ecRepeats: 3, ecDecay: 1, ecGate: 1, cycle: 12, fillGate: 2, fillVel: 1, shapeGate: 2, init: true}
	}
	return t
}

var (
	euGates    = []int{25, 50, 75, 100}
	longGates  = []int{25, 50, 75, 100, 150, 200} // a note's length as a part of a step: over 100 it ties into the next
	decayPcts  = []int{0, 5, 10, 15, 20, 30, 40}  // the velocity lost on each hit of a bar, in percent
	chancePcts = []int{0, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	echoDecays = []int{85, 70, 55, 40, 25}
)

// trackNotes are the notes of the pad in the pattern in front, in time order.
func (a *App) trackNotes(pad int) []lab.Note {
	d, _ := a.doc()
	var out []lab.Note
	for _, n := range d.Notes {
		if n.Pad == pad {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Tick < out[j].Tick })
	return out
}

// ensurePattern makes the pattern in front exist (4 bars) so that a tool has something to write into.
func (a *App) ensurePattern() (lab.PatternDoc, error) {
	d, ok := a.doc()
	if ok {
		return d, nil
	}
	if err := a.lab.NewPattern(a.pat, 4); err != nil {
		return d, err
	}
	d, _ = a.doc()
	return d, nil
}

// setTrack replaces the notes of a pad in the pattern in front with these.
func (a *App) setTrack(pad int, notes []lab.Note) error {
	if _, err := a.ensurePattern(); err != nil {
		return err
	}
	if a.freeze { // a frozen bar: the tool changes that bar, every other bar of the track stays as it is
		lo := uint32(a.step / 16 * 16 * stepTicks)
		hi := lo + 16*stepTicks
		var out []lab.Note
		for _, n := range a.trackNotes(pad) {
			if n.Tick < lo || n.Tick >= hi {
				out = append(out, n)
			}
		}
		for _, n := range notes {
			if n.Tick >= lo && n.Tick < hi {
				out = append(out, n)
			}
		}
		notes = out
	}
	args, _ := json.Marshal(lab.PatternArgs{Pad: pad})
	if err := a.lab.PatternOp(a.pat, "clearPad", args); err != nil {
		return err
	}
	b, err := a.lab.PatternBytes(a.pat)
	if err != nil {
		return err
	}
	p, ok := proto.NewPadIndex(uint16(pad))
	if !ok {
		return fmt.Errorf("no pad %d", pad)
	}
	sort.SliceStable(notes, func(i, j int) bool { return notes[i].Tick < notes[j].Tick })
	for _, n := range notes {
		if nb, err := formats.InsertNote(b, n.Tick, p, uint8(min(max(n.Vel, 1), 127)), uint16(min(max(n.Len, 1), 65535))); err == nil {
			b = nb
		}
	}
	return a.lab.SetPatternBytes(a.pat, b)
}

// finishTrack writes a tool's result, refreshes, puts it into the loop that plays and says what happened.
func (a *App) finishTrack(pad int, notes []lab.Note, what string) {
	if err := a.setTrack(pad, notes); err != nil {
		a.say(err.Error(), true)
		return
	}
	a.refresh()
	a.autoRows()
	a.replay()
	a.say(fmt.Sprintf("%s on %s: %d notes", what, a.label(pad), len(notes)), false)
}

// randomTools are the tools with a roll of the dice in them: they get an "Another roll" item.
var randomTools = map[string]bool{"Thin the track out": true, "Change the velocities": true, "Ratchet them": true, "Evolve the bars": true,
	"Apply them": true, "Modulate them": true, "Share the notes out": true}

// toolMenu is a tool's dialog: the settings, and the tool at work while the dialog is open. Opening it, and every change of a
// setting, puts the pattern back as it was and applies the tool again, and the loop plays it, so the effect is heard at once.
// "Keep it" commits what is heard as one undo step (and the dialog goes on from there); B throws the trial away and puts the
// pattern back (and stops the loop if the dialog started it). The roll of the dice stays the same while the settings change.
func (a *App) toolMenu(title string, settings []menuItem, applyLabel string, apply func()) {
	base := a.lab.Snapshot()
	a.toolBase = base
	a.tt.seed++ // a new roll for each visit
	startedLoop := false
	preview := func() {
		a.restoreProject(base)
		apply()
	}
	m := &menu{title: title}
	m.head("SETTINGS")
	for _, it := range settings {
		it.keepOpen = true
		orig := it.onChange
		it.onChange = func(v int) {
			if orig != nil {
				orig(v)
			}
			preview()
		}
		m.items = append(m.items, it)
	}
	if randomTools[applyLabel] {
		m.items = append(m.items, menuItem{label: "Another roll of the dice", keepOpen: true, run: func(int) { a.tt.seed++; preview() }})
	}
	m.head("TRIAL")
	m.items = append(m.items, menuItem{label: "Keep it (B throws it away)", keepOpen: true, run: func(int) {
		if !a.lab.Changed(base) {
			a.say("nothing to keep: the pattern is as it was", false)
			return
		}
		label := a.msg
		if label == "" {
			label = title
		}
		a.undo = append(a.undo, undoEntry{base, fit(label, 40)})
		a.redo = nil
		a.trimUndo()
		base = a.lab.Snapshot()
		a.toolBase = base
		a.say("kept: "+label, false)
	}})
	m.onClose = func() {
		changed := a.lab.Changed(base)
		a.restoreProject(base)
		a.toolBase = nil
		if startedLoop && a.seqOn {
			a.stop()
		}
		if changed {
			a.say("the trial is thrown away", false)
		}
	}
	a.menu = m
	preview() // the tool at work with its settings as they are
	if !a.seqOn && a.lab.Changed(base) {
		a.startLoop()
		startedLoop = a.seqOn
	}
}

func val(label string, vals []string, cur int, set func(v int)) menuItem {
	return menuItem{label: label, vals: vals, val: cur, onChange: set}
}

func rateNames() []string {
	return arpNames(formats.ArpRates, func(r formats.ArpRate) string { return r.Name })
}

func rateTicks(rate int) int {
	return formats.PPQ / formats.ArpRates[min(max(rate, 0), len(formats.ArpRates)-1)].PerBeat
}

// ---- Euclid

func (a *App) euclidMenu() {
	t := a.trackDefaults()
	pad := a.rowPad()
	nums := func(lo, hi int) []string {
		var out []string
		for i := lo; i <= hi; i++ {
			out = append(out, fmt.Sprint(i))
		}
		return out
	}
	a.toolMenu(fmt.Sprintf("euclid on %s", a.label(pad)), []menuItem{
		val("Rate", rateNames(), t.euRate, func(v int) { t.euRate = v }),
		val("Steps", nums(1, 32), t.euSteps-1, func(v int) { t.euSteps = v + 1 }),
		val("Hits", nums(0, 32), t.euHits, func(v int) { t.euHits = v }),
		val("Rotate", nums(0, 31), t.euRot, func(v int) { t.euRot = v }),
		val("Gate", names(euGates, func(v int) string { return fmt.Sprintf("%d%%", v) }), t.euGate, func(v int) { t.euGate = v }),
		val("Velocity", names(printVels, func(v int) string { return fmt.Sprint(v) }), t.euVel, func(v int) { t.euVel = v }),
		val("Decay per hit", names(decayPcts, func(v int) string { return pct(v) }), t.euDecay, func(v int) { t.euDecay = v }),
	}, "Write it over the track", func() {
		d, err := a.ensurePattern()
		if err != nil {
			a.say(err.Error(), true)
			return
		}
		ns := euclidNotes(pad, int(d.Length), rateTicks(t.euRate), t.euSteps, t.euHits, t.euRot, euGates[t.euGate], printVels[t.euVel])
		a.finishTrack(pad, decayNotes(ns, decayPcts[t.euDecay]),
			fmt.Sprintf("euclid %d/%d", min(t.euHits, t.euSteps), t.euSteps))
	})
}

// euclidNotes spreads hits as evenly as it can over steps and repeats that cycle over the length of the pattern, a step being
// stepT ticks long.
func euclidNotes(pad, length, stepT, steps, hits, rot, gate, vel int) []lab.Note {
	hits = min(max(hits, 0), steps)
	on := make([]bool, steps)
	for i := 0; i < hits; i++ {
		on[(i*steps/hits+rot)%steps] = true
	}
	var out []lab.Note
	for j := 0; j*stepT < length; j++ {
		if on[j%steps] {
			out = append(out, lab.Note{Tick: uint32(j * stepT), Pad: pad, Vel: vel, Len: max(stepT*gate/100, 1)})
		}
	}
	return out
}

// ---- Chance

func (a *App) chanceMenu() {
	t := a.trackDefaults()
	pad := a.rowPad()
	loops := []string{"fresh every note"}
	for i := 1; i <= 16; i++ {
		loops = append(loops, fmt.Sprintf("repeats every %d", i))
	}
	a.toolMenu(fmt.Sprintf("chance on %s", a.label(pad)), []menuItem{
		val("Chance", names(chancePcts, func(v int) string { return fmt.Sprintf("%d%%", v) }), near(chancePcts, t.chance), func(v int) { t.chance = chancePcts[v] }),
		val("Choices", loops, t.chanceLoop, func(v int) { t.chanceLoop = v }),
	}, "Thin the track out", func() {
		notes := a.trackNotes(pad)
		if len(notes) == 0 {
			a.say("this track has no notes", true)
			return
		}
		a.finishTrack(pad, chanceNotes(notes, t.chance, t.chanceLoop, t.seed), fmt.Sprintf("chance %d%%", t.chance))
	})
}

// chanceNotes keeps each note with the chance (percent). With loop > 0 the choices for the first loop notes are repeated for
// the notes after them, so the thinned track repeats.
func chanceNotes(notes []lab.Note, chance, loop int, seed int64) []lab.Note {
	rng := rand.New(rand.NewSource(seed))
	var keep []bool
	var out []lab.Note
	for i, n := range notes {
		var k bool
		if loop > 0 && i >= loop {
			k = keep[i%loop]
		} else {
			k = rng.Intn(100) < chance
		}
		keep = append(keep, k)
		if k {
			out = append(out, n)
		}
	}
	return out
}

// ---- Randomize

func (a *App) randomizeMenu() {
	t := a.trackDefaults()
	pad := a.rowPad()
	amounts := []int{5, 10, 15, 20, 30, 40, 60}
	biases := seq(-30, 30, 10)
	a.toolMenu(fmt.Sprintf("randomize velocity of %s", a.label(pad)), []menuItem{
		val("Amount", names(amounts, func(v int) string { return fmt.Sprintf("+-%d", v) }), t.rndAmount, func(v int) { t.rndAmount = v }),
		val("Shape", []string{"even", "bell"}, t.rndShape, func(v int) { t.rndShape = v }),
		val("Bias", names(biases, signed), near(biases, t.rndBias), func(v int) { t.rndBias = biases[v] }),
	}, "Change the velocities", func() {
		notes := a.trackNotes(pad)
		if len(notes) == 0 {
			a.say("this track has no notes", true)
			return
		}
		a.finishTrack(pad, randomizeNotes(notes, amounts[t.rndAmount], t.rndShape == 1, t.rndBias, t.seed), "randomized")
	})
}

func randomizeNotes(notes []lab.Note, amount int, bell bool, bias int, seed int64) []lab.Note {
	rng := rand.New(rand.NewSource(seed))
	out := make([]lab.Note, len(notes))
	for i, n := range notes {
		var d float64
		if bell { // the sum of two draws is more often near the middle
			d = (rng.Float64() + rng.Float64() - 1) * float64(amount)
		} else {
			d = (rng.Float64()*2 - 1) * float64(amount)
		}
		n.Vel = min(max(n.Vel+int(math.Round(d))+bias, 1), 127)
		out[i] = n
	}
	return out
}

// ---- Echo

func (a *App) echoMenu() {
	t := a.trackDefaults()
	pad := a.rowPad()
	reps := make([]string, 16)
	for i := range reps {
		reps[i] = fmt.Sprint(i + 1)
	}
	a.toolMenu(fmt.Sprintf("echo on %s", a.label(pad)), []menuItem{
		val("Rate", rateNames(), t.ecRate, func(v int) { t.ecRate = v }),
		val("Repeats", reps, t.ecRepeats-1, func(v int) { t.ecRepeats = v + 1 }),
		val("Decay", names(echoDecays, func(v int) string { return fmt.Sprintf("x%d%%", v) }), t.ecDecay, func(v int) { t.ecDecay = v }),
		val("Gate", names(euGates, func(v int) string { return fmt.Sprintf("%d%%", v) }), t.ecGate, func(v int) { t.ecGate = v }),
	}, "Add the echoes", func() {
		d, err := a.ensurePattern()
		if err != nil {
			a.say(err.Error(), true)
			return
		}
		notes := a.trackNotes(pad)
		if len(notes) == 0 {
			a.say("this track has no notes", true)
			return
		}
		a.finishTrack(pad, echoNotes(notes, int(d.Length), rateTicks(t.ecRate), t.ecRepeats, echoDecays[t.ecDecay], euGates[t.ecGate]), "echo")
	})
}

// echoNotes repeats every note after it, repeats times, each a step later and quieter by decay percent, cut at the end.
func echoNotes(notes []lab.Note, length, stepT, repeats, decay, gate int) []lab.Note {
	out := append([]lab.Note(nil), notes...)
	for _, n := range notes {
		vel := float64(n.Vel)
		for r := 1; r <= repeats; r++ {
			vel = vel * float64(decay) / 100
			tick := int(n.Tick) + r*stepT
			if tick >= length || vel < 4 {
				break
			}
			out = append(out, lab.Note{Tick: uint32(tick), Pad: n.Pad, Vel: int(vel), Len: max(stepT*gate/100, 1)})
		}
	}
	return out
}

// ---- Cycle (loop stretch)

func (a *App) cycleMenu() {
	t := a.trackDefaults()
	pad := a.rowPad()
	lens := make([]string, 32)
	for i := range lens {
		lens[i] = fmt.Sprintf("%d steps", i+1)
	}
	a.toolMenu(fmt.Sprintf("cycle %s", a.label(pad)), []menuItem{
		val("Repeat the first", lens, t.cycle-1, func(v int) { t.cycle = v + 1 }),
	}, "Repeat them over the pattern", func() {
		d, err := a.ensurePattern()
		if err != nil {
			a.say(err.Error(), true)
			return
		}
		notes := a.trackNotes(pad)
		if len(notes) == 0 {
			a.say("this track has no notes", true)
			return
		}
		a.finishTrack(pad, cycleNotes(notes, int(d.Length), t.cycle*stepTicks), fmt.Sprintf("cycle of %d steps", t.cycle))
	})
}

// cycleNotes repeats the notes of the first span ticks over the length: the track against the others in a different length.
func cycleNotes(notes []lab.Note, length, span int) []lab.Note {
	var out []lab.Note
	for off := 0; off < length; off += span {
		for _, n := range notes {
			if int(n.Tick) < span && off+int(n.Tick) < length {
				m := n
				m.Tick = uint32(off + int(n.Tick))
				out = append(out, m)
			}
		}
	}
	return out
}

// ---- Invert

// invertNotes puts a note on every 16th without one and takes the others away.
func invertNotes(notes []lab.Note, pad, length, vel int) []lab.Note {
	has := map[int]bool{}
	for _, n := range notes {
		has[int(n.Tick)/stepTicks] = true
	}
	var out []lab.Note
	for s := 0; s*stepTicks < length; s++ {
		if !has[s] {
			out = append(out, lab.Note{Tick: uint32(s * stepTicks), Pad: pad, Vel: vel, Len: stepTicks})
		}
	}
	return out
}

func (a *App) invert() {
	pad := a.rowPad()
	d, err := a.ensurePattern()
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	a.finishTrack(pad, invertNotes(a.trackNotes(pad), pad, int(d.Length), 100), "inverted")
}

// ---- Dispatch: the notes of the track shared out over the marked pads

func (a *App) dispatchMenu() {
	t := a.trackDefaults()
	pad := a.rowPad()
	a.toolMenu(fmt.Sprintf("dispatch %s over the marked pads", a.label(pad)), []menuItem{
		val("Mode", []string{"in turn", "at random"}, t.dispatch, func(v int) { t.dispatch = v }),
	}, "Share the notes out", func() {
		keys := a.markedKeys()
		if len(keys) < 2 {
			a.say("mark 2 or more pads first (Y+A on PADS)", true)
			return
		}
		notes := a.trackNotes(pad)
		if len(notes) == 0 {
			a.say("this track has no notes", true)
			return
		}
		rng := rand.New(rand.NewSource(t.seed))
		byPad := map[int][]lab.Note{}
		for i, n := range notes {
			k := keys[i%len(keys)]
			if t.dispatch == 1 {
				k = keys[rng.Intn(len(keys))]
			}
			n.Pad = k
			byPad[k] = append(byPad[k], n)
		}
		inKeys := false
		for _, k := range keys {
			inKeys = inKeys || k == pad
		}
		if !inKeys { // the track's own notes have gone to the others
			a.setTrack(pad, nil)
		}
		for k, ns := range byPad {
			if err := a.setTrack(k, ns); err != nil {
				a.say(err.Error(), true)
				return
			}
		}
		a.refresh()
		a.autoRows()
		a.replay()
		a.say(fmt.Sprintf("%d notes shared over %d pads", len(notes), len(keys)), false)
	})
}

// ---- Copy

func (a *App) copyTrackMenu() {
	t := a.trackDefaults()
	pad := a.rowPad()
	padNames := make([]string, 160)
	for i := range padNames {
		padNames[i] = a.label(i)
	}
	if t.copyTo == pad {
		t.copyTo = (pad + 1) % 160
	}
	a.toolMenu(fmt.Sprintf("copy %s to a pad", a.label(pad)), []menuItem{
		val("To", padNames, t.copyTo, func(v int) { t.copyTo = v }),
	}, "Copy (its notes are replaced)", func() {
		notes := a.trackNotes(pad)
		if len(notes) == 0 {
			a.say("this track has no notes", true)
			return
		}
		out := make([]lab.Note, len(notes))
		for i, n := range notes {
			n.Pad = t.copyTo
			out[i] = n
		}
		a.finishTrack(t.copyTo, out, "copied")
	})
}

// ---- Fill

// fillPresets are rhythms over the 16 steps of a bar; the track is filled with them in every bar.
var fillPresets = []struct {
	name string
	mask []int
}{
	{"every beat", []int{0, 4, 8, 12}},
	{"beats 1 and 3", []int{0, 8}},
	{"backbeat (2 and 4)", []int{4, 12}},
	{"off-beats (the ands)", []int{2, 6, 10, 14}},
	{"eighths", []int{0, 2, 4, 6, 8, 10, 12, 14}},
	{"sixteenths", []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}},
	{"off-beat sixteenths", []int{1, 3, 5, 7, 9, 11, 13, 15}},
	{"boom-bap kick", []int{0, 7, 10}},
	{"tresillo 3+3+2", []int{0, 3, 6, 8, 11, 14}},
	{"son clave 3-2", []int{0, 3, 6, 10, 12}},
	{"rumba clave 3-2", []int{0, 3, 7, 10, 12}},
	{"bossa nova clave", []int{0, 3, 6, 10, 13}},
	{"every third step", []int{0, 3, 6, 9, 12, 15}},
	{"every fifth step", []int{0, 5, 10, 15}},
	{"first step only", []int{0}},
	{"first half of the bar", []int{0, 1, 2, 3, 4, 5, 6, 7}},
	{"second half of the bar", []int{8, 9, 10, 11, 12, 13, 14, 15}},
	{"last beat stutter", []int{12, 13, 14, 15}},
	{"pickup before the next bar", []int{0, 8, 14, 15}},
}

func (a *App) fillMenu() {
	t := a.trackDefaults()
	pad := a.rowPad()
	pats := make([]string, len(fillPresets))
	for i, f := range fillPresets {
		pats[i] = f.name
	}
	a.toolMenu(fmt.Sprintf("fill %s", a.label(pad)), []menuItem{
		val("Pattern", pats, t.fillPat, func(v int) { t.fillPat = v }),
		val("Velocity", names(printVels, func(v int) string { return fmt.Sprint(v) }), t.fillVel, func(v int) { t.fillVel = v }),
		val("Gate", names(longGates, func(v int) string { return fmt.Sprintf("%d%%", v) }), t.fillGate, func(v int) { t.fillGate = v }),
		val("Decay per hit", names(decayPcts, func(v int) string { return pct(v) }), t.fillDecay, func(v int) { t.fillDecay = v }),
	}, "Fill the track", func() {
		d, err := a.ensurePattern()
		if err != nil {
			a.say(err.Error(), true)
			return
		}
		ns := fillNotes(pad, int(d.Length), fillPresets[t.fillPat].mask, printVels[t.fillVel], longGates[t.fillGate], decayPcts[t.fillDecay])
		a.finishTrack(pad, ns, fillPresets[t.fillPat].name)
	})
}

// fillNotes puts a note on the steps of the mask in every bar: gate percent of a step long, each hit of a bar decay percent
// quieter than the one before it.
func fillNotes(pad, length int, mask []int, vel, gate, decay int) []lab.Note {
	var out []lab.Note
	for bar := 0; bar*16*stepTicks < length; bar++ {
		for _, s := range mask {
			tick := (bar*16 + s) * stepTicks
			if tick >= length {
				continue
			}
			out = append(out, lab.Note{Tick: uint32(tick), Pad: pad, Vel: vel, Len: max(stepTicks*gate/100, 1)})
		}
	}
	return decayNotes(out, decay)
}

// decayNotes makes each note of a bar decay percent quieter than the one before it, counting from the first of the bar.
func decayNotes(notes []lab.Note, decay int) []lab.Note {
	if decay <= 0 {
		return notes
	}
	sorted := append([]lab.Note(nil), notes...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Tick < sorted[j].Tick })
	bar, k := -1, 0
	for i := range sorted {
		b := int(sorted[i].Tick) / (16 * stepTicks)
		if b != bar {
			bar, k = b, 0
		}
		v := float64(sorted[i].Vel) * math.Pow(1-float64(decay)/100, float64(k))
		sorted[i].Vel = min(max(int(math.Round(v)), 1), 127)
		k++
	}
	return sorted
}

// ---- Shape: gate and decay on the notes the track has

func (a *App) shapeMenu() {
	t := a.trackDefaults()
	pad := a.rowPad()
	gates := append([]string{"to the next note"}, names(longGates, func(v int) string { return fmt.Sprintf("%d%% of a step", v) })...)
	a.toolMenu(fmt.Sprintf("shape the notes of %s", a.label(pad)), []menuItem{
		val("Gate", gates, t.shapeGate, func(v int) { t.shapeGate = v }),
		val("Decay per hit", names(decayPcts, func(v int) string { return pct(v) }), t.shapeDecay, func(v int) { t.shapeDecay = v }),
	}, "Shape them", func() {
		notes := a.trackNotes(pad)
		if len(notes) == 0 {
			a.say("this track has no notes", true)
			return
		}
		d, _ := a.doc()
		gate := 0
		if t.shapeGate > 0 {
			gate = longGates[t.shapeGate-1]
		}
		a.finishTrack(pad, decayNotes(gateNotes(notes, int(d.Length), gate), decayPcts[t.shapeDecay]), "shaped")
	})
}

// gateNotes sets the length of each note: gate percent of a step, or with gate 0 up to the next note (the last up to the end).
func gateNotes(notes []lab.Note, length, gate int) []lab.Note {
	out := append([]lab.Note(nil), notes...)
	sort.Slice(out, func(i, j int) bool { return out[i].Tick < out[j].Tick })
	for i := range out {
		if gate > 0 {
			out[i].Len = max(stepTicks*gate/100, 1)
			continue
		}
		next := length
		if i+1 < len(out) {
			next = int(out[i+1].Tick)
		}
		out[i].Len = max(next-int(out[i].Tick), 1)
	}
	return out
}
