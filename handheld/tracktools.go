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
	seed                                          int64
	init                                          bool
}

func (a *App) trackDefaults() *trackState {
	t := &a.tt
	if !t.init {
		*t = trackState{euSteps: 16, euHits: 5, euRate: 3, euGate: 1, euVel: 1, chance: 70, ecRate: 1, ecRepeats: 3, ecDecay: 1, ecGate: 1, cycle: 12, init: true}
	}
	return t
}

var (
	euGates    = []int{25, 50, 75, 100}
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

// toolMenu builds a menu of settings with the apply action at the end; the menu stays open, so a setting can be changed and
// the tool applied again (Y+L2 undoes).
func (a *App) toolMenu(title string, settings []menuItem, applyLabel string, apply func()) {
	m := &menu{title: title}
	m.head("SETTINGS")
	for _, it := range settings {
		it.keepOpen = true
		m.items = append(m.items, it)
	}
	m.head("DO IT")
	m.items = append(m.items, menuItem{label: applyLabel, keepOpen: true, run: func(int) { apply() }})
	a.menu = m
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
	}, "Write it over the track", func() {
		d, err := a.ensurePattern()
		if err != nil {
			a.say(err.Error(), true)
			return
		}
		a.finishTrack(pad, euclidNotes(pad, int(d.Length), rateTicks(t.euRate), t.euSteps, t.euHits, t.euRot, euGates[t.euGate], printVels[t.euVel]),
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
		t.seed++
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
		t.seed++
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
		t.seed++
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
