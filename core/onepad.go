package core

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/moahbeatlab/404tui/formats"
)

// The arp from one sample, as DOOM OS does it: one pad is the sound, the notes are its pitches in a key and scale. A chord is
// stacked on the scale (thirds, fifths, sevenths...) or taken from the chord types, the arp lays the notes out in time. The
// notes of the arp are pitched copies of the sample: heard as a loop, bounced to one pad, or put on free pads (the keys) and
// written into the pattern.

var (
	onePadStacks = []struct {
		name string
		deg  []int // scale degrees above the root, counted from 0
	}{
		{"1  root", []int{0}}, {"1-3  thirds", []int{0, 2}}, {"1-5  fifths", []int{0, 4}}, {"1-3-5  triad", []int{0, 2, 4}},
		{"1-2-5  sus2", []int{0, 1, 4}}, {"1-4-5  sus4", []int{0, 3, 4}}, {"1-3-5-6  sixth", []int{0, 2, 4, 5}},
		{"1-3-5-7  seventh", []int{0, 2, 4, 6}}, {"1-3-5-7-9  ninth", []int{0, 2, 4, 6, 8}}, {"every note of the scale", nil},
	}
	onePadOcts = []int{-3, -2, -1, 0, 1, 2, 3} // as DOOM OS: the notes repeated up to three octaves up or down
	// chordTypes are the chords of the chord tool (and of the arp on CHROMATIC): the semitones above the root.
	chordTypes = []struct {
		name string
		iv   []int
	}{
		{"major", []int{0, 4, 7}}, {"minor", []int{0, 3, 7}}, {"5  power", []int{0, 7}}, {"sus2", []int{0, 2, 7}}, {"sus4", []int{0, 5, 7}},
		{"dim", []int{0, 3, 6}}, {"aug", []int{0, 4, 8}}, {"6", []int{0, 4, 7, 9}}, {"m6", []int{0, 3, 7, 9}},
		{"7", []int{0, 4, 7, 10}}, {"maj7", []int{0, 4, 7, 11}}, {"m7", []int{0, 3, 7, 10}}, {"m(maj7)", []int{0, 3, 7, 11}},
		{"dim7", []int{0, 3, 6, 9}}, {"m7b5  half-dim", []int{0, 3, 6, 10}}, {"7sus4", []int{0, 5, 7, 10}}, {"add9", []int{0, 4, 7, 14}},
		{"m(add9)", []int{0, 3, 7, 14}}, {"6/9", []int{0, 4, 7, 9, 14}}, {"9", []int{0, 4, 7, 10, 14}}, {"maj9", []int{0, 4, 7, 11, 14}},
		{"m9", []int{0, 3, 7, 10, 14}}, {"7b9", []int{0, 4, 7, 10, 13}}, {"7#9", []int{0, 4, 7, 10, 15}}, {"11", []int{0, 4, 7, 10, 14, 17}},
		{"m11", []int{0, 3, 7, 10, 14, 17}}, {"13", []int{0, 4, 7, 10, 14, 21}}, {"maj13", []int{0, 4, 7, 11, 14, 21}},
	}
)

// onePad is the settings of the menu, kept between visits. The playing settings are a.arpp, as for the marked pads.
type onePad struct {
	key, scale, stack, chord, degree, oct int
	hold                                  bool // X (Space) repeats the chord or the arp until it is pressed again; off, it plays once
	progOn                                bool // the chord tool: the loop that plays is the progression
	prog, color                           int  // the chord tool: the progression (an index of progressions), triads (0) or sevenths (1)
	from                                  int  // the note the sample of the pad in front is in (0 C .. 11 B)
	init                                  bool
}

// onePadSemis is the pitches (semitones from the sample, low to high) of the settings.
func onePadSemis(o onePad) []int {
	var out []int
	add := func(s int) {
		for _, v := range out {
			if v == s {
				return
			}
		}
		out = append(out, s)
	}
	var base []int // the chord over the key, in semitones
	if o.scale == 0 {
		base = append(base, chordTypes[o.chord].iv...)
	} else {
		iv := formats.ArpScales[o.scale-1].Iv
		at := func(d int) int { return iv[d%len(iv)] + 12*(d/len(iv)) }
		deg := onePadStacks[o.stack].deg
		if deg == nil {
			for d := range iv {
				deg = append(deg, d)
			}
		}
		for _, d := range deg {
			base = append(base, at(o.degree+d))
		}
	}
	n := onePadOcts[o.oct]
	step := 1
	if n < 0 {
		step = -1
	}
	for oct := 0; oct != n+step; oct += step {
		for _, s := range base {
			if v := (o.key-o.from+12)%12 + s + 12*oct; v >= -36 && v <= 36 { // from the sample's own note up to the key
				add(v)
			}
		}
	}
	sort.Ints(out)
	if len(out) > maxMarks { // the keys are pads: the lowest ones are kept
		out = out[:maxMarks]
	}
	sort.Ints(out)
	return out
}

// onePadKeyboard draws two octaves of a piano with the notes of the arp lit: the semitones from the sample, whose own note is from.
func onePadKeyboard(semis []int, from int) func(g Gfx, col, row, cols int) {
	return func(g Gfx, col, row, cols int) {
		on := map[int]bool{}
		names := ""
		seen := map[int]bool{}
		for _, s := range semis {
			v := s + from + 48 // a note below the sample (octaves down) is still on the keys
			on[v%24] = true
			if !seen[v%12] {
				seen[v%12] = true
				names += formats.PitchNames[v%12] + " "
			}
		}
		g.Piano(col, row, cols, on)
		g.Text(col+2, row+3, fit(names, cols-4), cAmber, true)
	}
}

// openOnePadMenu is the chord (chord) or the arp from pad n's one sample. The two share the notes (key, scale, chord, octaves): a chord
// found in one is there in the other. X (Space) plays it once; with Hold it repeats (the chord every bar, the arp as a loop) until X again.
func (a *App) openOnePadMenu(n int, chord bool) {
	a.arpPrintDefaults()
	st := &a.arpp
	o := a.notesOf(chord)
	what := "arp"
	if chord {
		what = "chord"
	}
	m := &menu{title: what + " from " + a.label(n), footH: 4}
	scales := []string{"CHROMATIC (chord types)"}
	for _, s := range formats.ArpScales {
		scales = append(scales, s.Name)
	}
	chords := arpNames(chordTypes, func(c struct {
		name string
		iv   []int
	}) string {
		return c.name
	})
	stacks := make([]string, len(onePadStacks))
	for i, s := range onePadStacks {
		stacks[i] = s.name
	}
	degrees := func() []string {
		l := 7
		if o.scale > 0 {
			l = len(formats.ArpScales[o.scale-1].Iv)
		}
		out := make([]string, l)
		for i := range out {
			out[i] = fmt.Sprintf("%d", i+1)
		}
		return out
	}
	var degI, chI, progI int
	redo := func() {
		m.foot = onePadKeyboard(onePadSemis(*o), o.from)
		if chord {
			if progI > 0 {
				m.at(progI).vals = progLabels(o.key, o.color == 1)
			}
			if a.arpOn {
				a.onePadReplay(n, chord)
			}
			return
		}
		if o.scale == 0 { // chromatic: the chord types, else the stacks of the scale
			m.at(chI).vals, m.at(chI).val = chords, o.chord
		} else {
			m.at(chI).vals, m.at(chI).val = stacks, o.stack
		}
		m.at(degI).vals = degrees()
		o.degree = min(o.degree, len(m.at(degI).vals)-1)
		m.at(degI).val = o.degree
		if a.arpOn {
			a.onePadReplay(n, chord)
		}
	}
	set := func(label string, vals []string, cur int, change func(v int)) int {
		m.items = append(m.items, menuItem{label: label, vals: vals, val: cur, keepOpen: true, onChange: func(v int) { change(v); redo() }})
		return len(m.items) - 1
	}
	if !chord {
		m.tabHead("NOTES")
	}
	from, how := a.sampleNote(n)
	o.from = from
	set("Sample is in", formats.PitchNames, o.from, func(v int) {
		o.from = v
		if a.sampleIn == nil {
			a.sampleIn = map[int]int{}
		}
		a.sampleIn[n] = v
	})
	if how != "" {
		a.say(fmt.Sprintf("the sample of %s is in %s (%s)", a.label(n), formats.PitchNames[from], how), false)
	}
	if chord {
		set("Root", formats.PitchNames, o.key, func(v int) { o.key = v })
		set("Chord", chords, o.chord, func(v int) { o.chord = v })
	} else {
		set("Key", formats.PitchNames, o.key, func(v int) { o.key = v })
		set("Scale", scales, o.scale, func(v int) { o.scale = v })
		degI = set("On degree (scales)", degrees(), o.degree, func(v int) { o.degree = v })
		chI = set("Chord", stacks, o.stack, func(v int) {
			if o.scale == 0 {
				o.chord = v
			} else {
				o.stack = v
			}
		})
	}
	set("Octaves", names(onePadOcts, func(v int) string { return fmt.Sprintf("%+d", v) }), o.oct, func(v int) { o.oct = v })
	holdVals := []string{"off: one pass", "on: loops"}
	if chord {
		holdVals = []string{"off: once", "on: every bar"}
	}
	m.items = append(m.items, menuItem{label: "Hold", vals: holdVals, val: btoi(o.hold), keepOpen: true,
		onChange: func(v int) {
			o.hold = v == 1
			if !o.hold && a.arpOn {
				a.stop()
			}
			m.xHint = onePadXHint(o.hold, what)
		}})
	if chord {
		m.head("MAKE")
		m.items = append(m.items, menuItem{label: "Bounce the chord to a new pad", run: func(int) { a.onePadBounce(n, true, m) }})
		m.head("PROGRESSION (IN THE ROOT'S KEY)")
		progI = set("Progression", progLabels(o.key, o.color == 1), o.prog, func(v int) { o.prog = v })
		set("Chords", progColors, o.color, func(v int) { o.color = v })
		m.items = append(m.items,
			menuItem{label: "Hear the progression (a chord a bar)", keepOpen: true, run: func(int) { a.progHear(n) }},
			menuItem{label: "Bounce the progression to free pads", run: func(int) { a.progBounce(n, m) }},
		)
	} else {
		m.tabHead("PLAY")
		set("Type", arpTypes, st.typ, func(v int) { st.typ = v; a.say(arpTypeNotes[v], false) })
		set("Rate", arpNames(formats.ArpRates, func(r formats.ArpRate) string { return r.Name }), st.rate, func(v int) { st.rate = v })
		set("Gate", names(printGates, func(v int) string { return fmt.Sprintf("%d%%", v) }), st.gate, func(v int) { st.gate = v })
		set("Velocity", names(printVels, func(v int) string { return fmt.Sprint(v) }), st.vel, func(v int) { st.vel = v })
		set("Length", names(printBars, func(v int) string { return fmt.Sprintf("%d bar", v) }), near(printBars, st.bars), func(v int) { st.bars = printBars[v] })
		m.tabHead("MAKE")
		m.items = append(m.items,
			menuItem{label: "Bounce the arp to a new pad", run: func(int) { a.onePadBounce(n, false, m) }},
			menuItem{label: "Print the arp to the pattern", run: func(int) { a.onePadPattern(n) }},
		)
	}
	m.onClose = func() {
		if a.arpOn {
			a.stop()
		}
	}
	m.onX = func() { a.onePadSpace(n, chord) }
	m.xHint = onePadXHint(o.hold, what)
	redo()
	if !chord {
		m.tabify()
	}
	a.menu = m
}

// notesOf is the settings of the chord tool or the arp tool, each kept between visits.
func (a *App) notesOf(chord bool) *onePad {
	o := &a.onepad
	def := onePad{stack: 3, oct: 4, init: true} // a triad over two octaves
	if chord {
		o = &a.chordp
		def = onePad{oct: 3, init: true} // a major chord, one octave
	}
	if !o.init {
		*o = def
	}
	return o
}

func onePadXHint(hold bool, what string) string {
	if hold {
		return "loop " + what + " / stop"
	}
	return what + " once"
}

// onePadSpace is X (Space) in the menu: with Hold the chord or the arp repeats until X again, else it plays once. A loop that plays
// is stopped either way.
func (a *App) onePadSpace(n int, chord bool) {
	if a.arpOn {
		a.stop()
		a.say("stopped", false)
		return
	}
	hold := a.notesOf(chord).hold
	s, err := a.onePadSound(n, chord, hold)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	if !hold {
		a.play(s)
		return
	}
	lp, ok := a.Audio.(looper)
	if a.Audio == nil || !ok {
		a.say("no audio output", true)
		return
	}
	a.stop()
	if err := lp.Loop(s); err != nil {
		a.say(err.Error(), true)
		return
	}
	a.arpOn = true
	a.notesOf(chord).progOn = false
	a.say("it loops: change a setting to hear it at once, Space stops it", false)
}

// onePadSound is what X plays: the chord (as a loop, one bar at the bank's tempo) or the arp.
func (a *App) onePadSound(n int, chord, loop bool) (formats.Sample, error) {
	if !chord {
		return a.onePadRender(n)
	}
	ss, err := a.onePadSamples(n, onePadSemis(*a.notesOf(true)))
	if err != nil {
		return formats.Sample{}, err
	}
	s := chordMix(ss)
	if loop {
		bar := int(4 * 60 / a.tempoOf(n/16) * formats.SampleRate)
		out := make([]int16, bar*2)
		copy(out, s.Samples)
		s.Samples = out
	}
	return s, nil
}

// onePadSamples is the sample of pad n pitched to each of the semitones.
func (a *App) onePadSamples(n int, semis []int) ([]formats.Sample, error) {
	id, ok := a.padSample(n)
	if !ok {
		return nil, fmt.Errorf("this pad has no sample")
	}
	if len(semis) < 2 {
		return nil, fmt.Errorf("one note only: stack more notes or add octaves")
	}
	out := make([]formats.Sample, len(semis))
	for i, s := range semis {
		pid, err := a.lab.Pitched(id, s)
		if err != nil {
			return nil, err
		}
		out[i], err = a.lab.Sample(pid)
		a.lab.Free(pid)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (a *App) onePadRender(n int) (formats.Sample, error) {
	ss, err := a.onePadSamples(n, onePadSemis(a.onepad))
	if err != nil {
		return formats.Sample{}, err
	}
	return a.mixArp(ss, a.tempoOf(n/16))
}

// onePadReplay puts a changed setting into the loop that plays.
func (a *App) onePadReplay(n int, chord bool) {
	r, ok := a.Audio.(replacer)
	if !ok {
		return
	}
	s, err := a.onePadSound(n, chord, true)
	if chord && a.notesOf(true).progOn { // the progression loops, not the chord
		s, err = a.progSound(n)
	}
	if err == nil {
		r.ReplaceLoop(s)
	}
}

// chordMix plays the samples together, as long as the longest, with room for the sum.
func chordMix(ss []formats.Sample) formats.Sample {
	frames := 0
	for _, s := range ss {
		frames = max(frames, s.Frames())
	}
	mix := make([]int32, frames*2)
	for _, s := range ss {
		ch := max(s.Channels, 1)
		for f := 0; f < s.Frames(); f++ {
			mix[f*2] += int32(s.Samples[f*ch])
			mix[f*2+1] += int32(s.Samples[f*ch+ch-1])
		}
	}
	div := int32(max(1, len(ss)*6/10))
	out := make([]int16, len(mix))
	for i, v := range mix {
		out[i] = int16(max(-32767, min(32767, v/div)))
	}
	return formats.Sample{Channels: 2, Samples: out}
}

// onePadBounce keeps the arp, or with chord the notes played together, on the next free pad. Then (or when the pad dialog is left)
// the tool's menu m is back as it was, to make another one.
func (a *App) onePadBounce(n int, chord bool, m *menu) {
	var s formats.Sample
	var err error
	name := "arp"
	if chord {
		name = "chord"
		var ss []formats.Sample
		if ss, err = a.onePadSamples(n, onePadSemis(*a.notesOf(true))); err == nil {
			s = chordMix(ss)
		}
	} else {
		s, err = a.onePadRender(n)
	}
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	a.pickFreePad("Put it on a pad", n+1, func(to int) {
		defer func() { a.menu = m }()
		mb, err := a.lab.Decode(s.WAV(), "wav")
		var meta struct{ ID int }
		if err != nil || json.Unmarshal(mb, &meta) != nil {
			a.say("cannot keep the arp", true)
			return
		}
		err = a.assignPad(to, meta.ID, fmt.Sprintf("%s_%s", fit(a.info.Pads[n].Name, 9), name))
		a.lab.Free(meta.ID)
		if err != nil {
			a.say(err.Error(), true)
			return
		}
		delete(a.pcm, to)
		a.refresh()
		a.say(fmt.Sprintf("the %s is on %s: change it and bounce again for another", name, a.label(to)), false)
	})
	if a.picker != nil {
		a.picker.onCancel = func() { a.menu = m }
	}
}

// onePadPattern puts the pitched copies on free pads (low to high) and writes the arp over them into the pattern.
func (a *App) onePadPattern(n int) {
	semis := onePadSemis(a.onepad)
	id, ok := a.padSample(n)
	if !ok || len(semis) < 2 {
		a.say("the arp needs this pad's sample and 2 notes or more", true)
		return
	}
	first := a.freeRun(len(semis), n+1)
	if first < 0 {
		a.say(fmt.Sprintf("no room: %d empty pads in a row are needed", len(semis)), true)
		return
	}
	base := fit(a.info.Pads[n].Name, 9)
	var keys []int
	for i, s := range semis {
		pid, err := a.lab.Pitched(id, s)
		if err == nil {
			err = a.assignPad(first+i, pid, fmt.Sprintf("%s_%+d", base, s))
			a.lab.Free(pid)
		}
		if err != nil {
			a.say(err.Error(), true)
			return
		}
		delete(a.pcm, first+i)
		keys = append(keys, first+i)
	}
	a.refresh()
	a.marks = keys
	a.stop()
	a.menu = nil
	a.go_(Seq)
	a.printArpKeys(keys)
}
