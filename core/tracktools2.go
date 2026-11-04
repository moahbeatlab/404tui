package core

import (
	"fmt"
	"math"
	"math/rand"
	"sort"

	"github.com/moahbeatlab/404tui/lab"
)

// More track tools: Ratchet (the unit's SUBSTEP: a step split into 2, 3 or 4 parts, each part a note as long as the part),
// Evolve (each bar a small mutation of the one before), Accent and ghosts (the hi-hat's life), Nudge (a track pushed or pulled
// by ticks, which is also the groove: a snare that sits late).

type trackState2 struct {
	raWhich, raParts, raPlays, raRamp  int
	raPitch                            int // the pitch each part of a ratchet is above the one before (an index of pitchSteps)
	evChange, evWhat, evReset, evScope int
	acAccent, acVel, acOthers          int
	acGhosts, acGhostVel, acGhostPct   int
	nudge                              int
	init                               bool
}

func (a *App) tools2() *trackState2 {
	t := &a.tt2
	if !t.init {
		*t = trackState2{raParts: 0, evChange: 1, evReset: 0, acVel: 1, acOthers: 3, acGhosts: 1, acGhostVel: 1, acGhostPct: 1, nudge: 8, init: true}
	}
	return t
}

// ---- Ratchet

var (
	ratchetWhich = []string{"every note", "every 2nd note", "every 3rd note", "every 4th note", "the last note of each bar", "25% of the notes", "50% of the notes", "75% of the notes"}
	ratchetParts = []int{2, 3, 4}
	ratchetPlays = []string{"all parts", "skip the first", "skip the last", "every other part"}
	ratchetRamps = []string{"flat", "louder", "quieter", "accent the first"}
)

func (a *App) ratchetMenu() {
	t := a.tools2()
	pad := a.rowPad()
	a.toolMenu(fmt.Sprintf("ratchet %s", a.label(pad)), []menuItem{
		val("Notes", ratchetWhich, t.raWhich, func(v int) { t.raWhich = v }),
		val("Split into", names(ratchetParts, func(v int) string { return fmt.Sprintf("%d parts", v) }), t.raParts, func(v int) { t.raParts = v }),
		val("Parts that play", ratchetPlays, t.raPlays, func(v int) { t.raPlays = v }),
		val("Velocity", ratchetRamps, t.raRamp, func(v int) { t.raRamp = v }),
		val("Pitch of each part", names(pitchSteps, stepText), t.raPitch, func(v int) { t.raPitch = v }),
	}, "Ratchet them", func() {
		notes := a.trackNotes(pad)
		if len(notes) == 0 {
			a.say("this track has no notes", true)
			return
		}
		out := ratchetNotesPitch(notes, t.raWhich, ratchetParts[t.raParts], t.raPlays, t.raRamp, pitchSteps[t.raPitch], a.tt.seed)
		a.finishTrack(pad, out, fmt.Sprintf("ratchet x%d", ratchetParts[t.raParts]))
	})
}

// ratchetNotes splits the chosen notes: a note on a step becomes parts notes on the parts of that step, each as long as a part,
// as the unit's SUBSTEP lays them out. The others stay as they are.
func ratchetNotes(notes []lab.Note, which, parts, plays, ramp int, seed int64) []lab.Note {
	return ratchetNotesPitch(notes, which, parts, plays, ramp, 0, seed)
}

// ratchetNotesPitch is ratchetNotes with each part pitchStep semitones from the one before it (a rising or falling roll).
func ratchetNotesPitch(notes []lab.Note, which, parts, plays, ramp, pitchStep int, seed int64) []lab.Note {
	rng := rand.New(rand.NewSource(seed))
	sorted := append([]lab.Note(nil), notes...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Tick < sorted[j].Tick })
	lastOfBar := map[int]int{} // bar -> index of its last note
	for i, n := range sorted {
		lastOfBar[int(n.Tick)/(16*stepTicks)] = i
	}
	chosen := func(i int) bool {
		switch which {
		case 0:
			return true
		case 1, 2, 3:
			return i%(which+1) == 0
		case 4:
			return lastOfBar[int(sorted[i].Tick)/(16*stepTicks)] == i
		}
		return rng.Intn(100) < []int{25, 50, 75}[which-5]
	}
	part := stepTicks / parts
	var out []lab.Note
	for i, n := range sorted {
		if !chosen(i) {
			out = append(out, n)
			continue
		}
		start := int(n.Tick) / stepTicks * stepTicks
		for p := 0; p < parts; p++ {
			if (plays == 1 && p == 0) || (plays == 2 && p == parts-1) || (plays == 3 && p%2 == 1) {
				continue
			}
			f := float64(p) / float64(max(parts-1, 1))
			vel := float64(n.Vel)
			switch ramp {
			case 1:
				vel *= 0.6 + 0.4*f
			case 2:
				vel *= 1 - 0.4*f
			case 3:
				if p > 0 {
					vel *= 0.6
				}
			}
			out = append(out, shiftPitch(lab.Note{Tick: uint32(start + p*part), Pad: n.Pad, Vel: min(max(int(math.Round(vel)), 1), 127), Len: part, Pitch: n.Pitch}, p*pitchStep))
		}
	}
	return out
}

// ---- Evolve

var (
	evolveChanges = []int{10, 20, 30, 40, 60}
	evolveWhats   = []string{"moves, drops and adds", "moves only", "adds only", "drops only", "velocities only", "pitches only"}
	evolveResets  = []string{"never", "every 2 bars", "every 3 bars", "every 4 bars"}
)

func (a *App) evolveMenu() {
	t := a.tools2()
	pad := a.rowPad()
	a.toolMenu(fmt.Sprintf("evolve %s", a.label(pad)), []menuItem{
		val("Change", names(evolveChanges, func(v int) string { return fmt.Sprintf("%d%% a bar", v) }), t.evChange, func(v int) { t.evChange = v }),
		val("What", evolveWhats, t.evWhat, func(v int) { t.evWhat = v }),
		val("Back to the first bar", evolveResets, t.evReset, func(v int) { t.evReset = v }),
		val("Tracks", []string{"this one", "every one with notes"}, t.evScope, func(v int) { t.evScope = v }),
	}, "Evolve the bars", func() {
		d, err := a.ensurePattern()
		if err != nil {
			a.say(err.Error(), true)
			return
		}
		if d.Bars < 2 {
			a.say("a pattern of one bar has nothing to evolve into: double it first", true)
			return
		}
		pads := []int{pad}
		if t.evScope == 1 {
			pads = pads[:0]
			seen := map[int]bool{}
			for _, n := range d.Notes {
				if !seen[n.Pad] {
					seen[n.Pad] = true
					pads = append(pads, n.Pad)
				}
			}
			sort.Ints(pads)
		}
		total := 0
		for i, p := range pads {
			ns := a.trackNotes(p)
			if len(ns) == 0 {
				continue
			}
			out := evolveNotes(ns, int(d.Length), evolveChanges[t.evChange], t.evWhat, t.evReset, a.tt.seed+int64(i)*7919)
			if err := a.setTrack(p, out); err != nil {
				a.say(err.Error(), true)
				return
			}
			total += len(out)
		}
		a.refresh()
		a.autoRows()
		a.replay()
		a.say(fmt.Sprintf("evolved %d tracks over %d bars: %d notes", len(pads), d.Bars, total), false)
	})
}

// evolveNotes keeps the first bar and makes every later bar a mutation of the one before it (or, every reset bars, of the
// first again): change percent of the notes are moved a step, dropped or made louder or softer, and about as many new notes
// are added, according to what.
func evolveNotes(notes []lab.Note, length, change, what, reset int, seed int64) []lab.Note {
	rng := rand.New(rand.NewSource(seed))
	barT := 16 * stepTicks
	bars := (length + barT - 1) / barT
	// the first bar is the seed: the notes of the track in it (a track with nothing there starts from what bar 1 has)
	var first []lab.Note
	for _, n := range notes {
		if int(n.Tick) < barT {
			n.Tick = uint32(int(n.Tick))
			first = append(first, n)
		}
	}
	if len(first) == 0 {
		return notes
	}
	out := append([]lab.Note(nil), first...)
	prev := first
	for b := 1; b < bars; b++ {
		base := prev
		if reset > 0 && b%(reset+1) == 0 {
			base = first
		}
		cur := mutateBar(base, barT, change, what, rng)
		for _, n := range cur {
			m := n
			m.Tick = n.Tick + uint32(b*barT)
			if int(m.Tick) < length {
				out = append(out, m)
			}
		}
		prev = cur
	}
	return out
}

// mutateBar returns a changed copy of the notes of one bar (ticks inside the bar).
func mutateBar(notes []lab.Note, barT, change, what int, rng *rand.Rand) []lab.Note {
	steps := barT / stepTicks
	occupied := map[int]bool{}
	cur := append([]lab.Note(nil), notes...)
	for _, n := range cur {
		occupied[int(n.Tick)/stepTicks] = true
	}
	moves, drops, adds, wiggles := what == 0 || what == 1, what == 0 || what == 3, what == 0 || what == 2, what == 4
	pitches := what == 5 // the notes stay where they are and a part of them change their pitch
	var out []lab.Note
	var avg, count int
	for _, n := range cur {
		avg += n.Vel
		count++
		if rng.Intn(100) >= change {
			if wiggles {
				n.Vel = min(max(n.Vel+rng.Intn(31)-15, 1), 127)
			}
			out = append(out, n)
			continue
		}
		op := rng.Intn(2) // when both are on, a changed note is moved or dropped
		switch {
		case pitches:
			out = append(out, shiftPitch(n, rng.Intn(9)-4))
		case wiggles:
			n.Vel = min(max(n.Vel+rng.Intn(41)-20, 1), 127)
			out = append(out, n)
		case moves && (!drops || op == 0):
			s := int(n.Tick) / stepTicks
			to := s + []int{-1, 1}[rng.Intn(2)]
			if to >= 0 && to < steps && !occupied[to] {
				occupied[s], occupied[to] = false, true
				n.Tick = uint32(to*stepTicks + int(n.Tick)%stepTicks)
			}
			out = append(out, n)
		case drops:
			occupied[int(n.Tick)/stepTicks] = false // gone
		default:
			out = append(out, n)
		}
	}
	if adds && count > 0 {
		want := int(math.Round(float64(count) * float64(change) / 200)) // about half as many as the change
		if what == 2 {
			want = int(math.Round(float64(count) * float64(change) / 100))
		}
		for tries := 0; want > 0 && tries < 64; tries++ {
			s := rng.Intn(steps)
			if occupied[s] {
				continue
			}
			occupied[s] = true
			out = append(out, lab.Note{Tick: uint32(s * stepTicks), Pad: cur[0].Pad, Vel: min(max(avg/count*85/100, 1), 127), Len: cur[0].Len})
			want--
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Tick < out[j].Tick })
	return out
}

// ---- Accent and ghosts

var (
	accentWhere = []string{"none", "the beats (1 2 3 4)", "the off-beats (the ands)", "every 2nd note", "every 3rd note", "every 4th note", "the first of each bar"}
	accentVels  = []int{127, 115, 105}
	otherVels   = []int{0, 100, 90, 80, 70, 60} // 0 keeps what the others have
	ghostWhere  = []string{"none", "before each hit", "after each hit", "empty off-beat 16ths", "every empty step"}
	ghostVels   = []int{20, 30, 40, 50}
	ghostPcts   = []int{25, 50, 75, 100}
)

func (a *App) accentMenu() {
	t := a.tools2()
	pad := a.rowPad()
	a.toolMenu(fmt.Sprintf("accents and ghosts on %s", a.label(pad)), []menuItem{
		val("Accent", accentWhere, t.acAccent, func(v int) { t.acAccent = v }),
		val("Accent velocity", names(accentVels, func(v int) string { return fmt.Sprint(v) }), t.acVel, func(v int) { t.acVel = v }),
		val("The other notes", names(otherVels, func(v int) string {
			if v == 0 {
				return "as they are"
			}
			return fmt.Sprint(v)
		}), t.acOthers, func(v int) { t.acOthers = v }),
		val("Ghost notes", ghostWhere, t.acGhosts, func(v int) { t.acGhosts = v }),
		val("Ghost velocity", names(ghostVels, func(v int) string { return fmt.Sprint(v) }), t.acGhostVel, func(v int) { t.acGhostVel = v }),
		val("Ghost chance", names(ghostPcts, func(v int) string { return fmt.Sprintf("%d%%", v) }), t.acGhostPct, func(v int) { t.acGhostPct = v }),
	}, "Apply them", func() {
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
		out := accentGhostNotes(notes, pad, int(d.Length), t.acAccent, accentVels[t.acVel], otherVels[t.acOthers], t.acGhosts, ghostVels[t.acGhostVel], ghostPcts[t.acGhostPct], a.tt.seed)
		a.finishTrack(pad, out, "accents and ghosts")
	})
}

// accentGhostNotes sets the velocity of the accented notes and of the others, and adds soft ghost notes on empty steps.
func accentGhostNotes(notes []lab.Note, pad, length, accent, accVel, others, ghosts, ghostVel, ghostPct int, seed int64) []lab.Note {
	rng := rand.New(rand.NewSource(seed))
	sorted := append([]lab.Note(nil), notes...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Tick < sorted[j].Tick })
	firstOfBar := map[int]int{}
	for i, n := range sorted {
		b := int(n.Tick) / (16 * stepTicks)
		if _, ok := firstOfBar[b]; !ok {
			firstOfBar[b] = i
		}
	}
	occupied := map[int]bool{}
	for i := range sorted {
		n := &sorted[i]
		occupied[int(n.Tick)/stepTicks] = true
		hit := false
		switch accent {
		case 1:
			hit = int(n.Tick)%formatsBeat == 0
		case 2:
			hit = int(n.Tick)%formatsBeat == formatsBeat/2
		case 3, 4, 5:
			hit = i%(accent-1) == 0
		case 6:
			hit = firstOfBar[int(n.Tick)/(16*stepTicks)] == i
		}
		switch {
		case hit:
			n.Vel = accVel
		case accent != 0 && others > 0:
			n.Vel = others
		}
	}
	out := sorted
	if ghosts > 0 {
		var cands []int
		steps := length / stepTicks
		seen := map[int]bool{}
		for _, n := range sorted {
			s := int(n.Tick) / stepTicks
			switch ghosts {
			case 1:
				cands = append(cands, s-1)
			case 2:
				cands = append(cands, s+1)
			}
		}
		if ghosts >= 3 {
			for s := 0; s < steps; s++ {
				if ghosts == 4 || s%2 == 1 {
					cands = append(cands, s)
				}
			}
		}
		for _, s := range cands {
			if s < 0 || s >= steps || occupied[s] || seen[s] || rng.Intn(100) >= ghostPct {
				continue
			}
			seen[s] = true
			out = append(out, lab.Note{Tick: uint32(s * stepTicks), Pad: pad, Vel: ghostVel, Len: stepTicks / 2})
		}
	}
	return out
}

const formatsBeat = 4 * stepTicks // a beat, in ticks

// ---- Nudge

func (a *App) nudgeMenu() {
	t := a.tools2()
	pad := a.rowPad()
	ticks := seq(-40, 40, 5)
	a.toolMenu(fmt.Sprintf("nudge %s", a.label(pad)), []menuItem{
		val("Push (-) or pull (+)", names(ticks, func(v int) string { return fmt.Sprintf("%+d ticks", v) }), t.nudge, func(v int) { t.nudge = v }),
	}, "Move the notes", func() {
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
		a.finishTrack(pad, nudgeNotes(notes, int(d.Length), ticks[t.nudge]), fmt.Sprintf("nudge %+d ticks", ticks[t.nudge]))
	})
}

// nudgeNotes moves every note by ticks; a note moved off either end of the pattern stays on its end.
func nudgeNotes(notes []lab.Note, length, ticks int) []lab.Note {
	out := make([]lab.Note, len(notes))
	for i, n := range notes {
		n.Tick = uint32(min(max(int(n.Tick)+ticks, 0), length-1))
		out[i] = n
	}
	return out
}
