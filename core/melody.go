package core

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/lab"
)

// Melody (the NOTES tools of SEQ): notes at pitches for the pad of the track, a sample or the pad that plays a synth, in the key of
// the bank: a bass line, a lead, an arp, stabs or held pad chords, in a style. It is written as a motif, not as notes at random: one
// bar is made (its rhythm from the style, its notes from the scale, the chord tones on the strong steps) and the bars go A A' A B,
// each over its chord of the progression, the last one coming home. Ghost notes and accents are settings of their own, and the
// range is the octaves the notes stay in (a pattern note goes 12 semitones up or down from the pad's own pitch, not more).

// The kinds of role: how the notes are made, and what is done to them (ghost notes, the grid of the note values).
const (
	kBass = iota
	kLead
	kArp
	kChords
	kHeld
)

// melRole is a role: its name, its kind and the range "for the role" (semitones from the pad's own pitch).
type melRole struct {
	name   string
	kind   int
	lo, hi int
}

var melRoles = []melRole{
	{"bass", kBass, -7, 7},
	{"lead", kLead, -2, 12},
	{"arp", kArp, -5, 12},
	{"stabs", kChords, -5, 10},
	{"pad (held chords)", kHeld, -7, 9},
	{"sub bass (long roots)", kBass, -12, 0},
	{"octave bass (low, high)", kBass, -12, 12},
	{"acid line (16ths, octaves, slides)", kBass, -12, 12},
	{"pluck (short offbeat hook)", kLead, 0, 12},
	{"call and response", kLead, -2, 12},
	{"piano house (broken chords)", kChords, -5, 10},
	{"drone (the tonic held)", kHeld, -12, 0},
	{"dub chord (deep echo)", kChords, -5, 10},
	{"parallel stabs (one shape moved)", kChords, -5, 10},
	{"rolling bass (root, fifth, octave)", kBass, -7, 12},
	{"bass pattern", kBass, -7, 12}, // the bass type's patterns (melBassPats): offbeat 8ths, rolling 8ths, gallop...
}

// melRates are the note values a melody can be written in (ticks; 0 the style's own rhythm), the triplets last.
var melRates = []struct {
	name  string
	ticks int
}{
	{"the style's", 0}, {"1/2", 960}, {"1/4", 480}, {"1/8", 240}, {"1/16", 120}, {"1/32", 60},
	{"1/4 triplets", 320}, {"1/8 triplets", 160}, {"1/16 triplets", 80},
}

var melStyles = []string{"house", "deep house", "tech house", "techno", "trance", "progressive", "dub techno", "acid house"}

// melHarm is a loop of chords and the bars each lasts (no chords: the key's one chord).
type melHarm struct {
	steps []progStep
	per   int
}

// melStyleHarmony is what a style plays when Chords, Chord length and Chord notes are "the style's": loops in minor, one of which
// each roll of the dice takes (a loop twice in the list is taken twice as often), the loop of a major key, and the notes of the
// chords (an index of melColors). From tutorials and the counted studies there are (GiantSteps keys and tempos, 100 uplifting
// trance loops: two bars a chord, five or six chords) and the loops Hooktheory's TheoryTab has of trance, progressive and house
// artists (the anthem loop most, i v and i VI often, i iv seldom): two-chord shuttles or 4-bar loops a chord a bar in house and deep house,
// one chord in tech house, techno, dub techno and acid, the anthem loops in trance and progressive.
var melStyleHarmony = []struct {
	minor []melHarm
	major melHarm
	color int
}{
	{[]melHarm{{[]progStep{{0, 'm'}, {10, 'M'}}, 1}, {[]progStep{{0, 'm'}, {5, 'm'}}, 1}, {[]progStep{{5, 'm'}, {8, 'M'}, {0, 'm'}, {10, 'M'}}, 1},
		{[]progStep{{0, 'm'}, {8, 'M'}}, 1}, {[]progStep{{8, 'M'}, {10, 'M'}, {0, 'm'}, {0, 'm'}}, 1}, {[]progStep{{0, 'm'}, {10, 'M'}}, 1}},
		melHarm{[]progStep{{0, 'M'}, {5, 'M'}}, 1}, 2}, // house: i VII (twice), i iv, iv VI i VII, i VI, VI VII i; I IV; sevenths
	{[]melHarm{{[]progStep{{0, 'm'}, {5, 'm'}}, 1}, {[]progStep{{0, 'm'}, {5, 'm'}}, 1}, {[]progStep{{0, 'm'}, {8, 'M'}, {5, 'm'}, {5, 'm'}}, 1}, {[]progStep{{0, 'm'}, {5, 'm'}, {10, 'M'}, {10, 'M'}}, 1},
		{[]progStep{{0, 'm'}, {8, 'M'}}, 1}, {[]progStep{{0, 'm'}, {8, 'M'}, {3, 'M'}, {10, 'M'}}, 1}},
		melHarm{[]progStep{{0, 'M'}, {5, 'M'}}, 1}, 3}, // deep house: i iv (twice), i VI iv iv, i iv VII VII, and as pop deep house i VI, i VI III VII; ninths
	{[]melHarm{{nil, 1}, {nil, 1}, {[]progStep{{0, 'm'}, {8, 'M'}, {10, 'M'}, {10, 'M'}}, 1}, {[]progStep{{0, 'm'}, {5, 'm'}, {7, 'm'}, {7, 'm'}}, 1}},
		melHarm{nil, 1}, 2}, // tech house: one chord mostly, else i VI VII, i iv v
	{[]melHarm{{nil, 1}, {nil, 1}, {[]progStep{{0, 'm'}, {10, 'M'}, {8, 'M'}, {10, 'M'}}, 2}, {[]progStep{{0, 'm'}, {8, 'M'}, {3, 'M'}, {10, 'M'}}, 4}},
		melHarm{nil, 1}, 1}, // techno: one chord mostly; melodic techno i VII VI VII, i VI III VII over 2 and 4 bars a chord
	{[]melHarm{{[]progStep{{0, 'm'}, {8, 'M'}, {3, 'M'}, {10, 'M'}}, 2}, {[]progStep{{0, 'm'}, {10, 'M'}, {8, 'M'}, {10, 'M'}}, 2}, {[]progStep{{0, 'm'}, {10, 'M'}, {8, 'M'}, {7, 'D'}}, 2}, {[]progStep{{0, 'm'}, {8, 'M'}, {3, 'M'}, {10, 'M'}}, 2},
		{[]progStep{{8, 'M'}, {10, 'M'}, {0, 'm'}, {0, 'm'}}, 2}, {[]progStep{{0, 'm'}, {8, 'M'}, {3, 'M'}, {10, 'M'}}, 2}, {[]progStep{{0, 'm'}, {8, 'M'}, {10, 'M'}, {10, 'M'}}, 2}, {[]progStep{{0, 'm'}, {7, 'm'}, {8, 'M'}, {3, 'M'}}, 2}},
		melHarm{[]progStep{{0, 'M'}, {7, 'D'}, {9, 'm'}, {5, 'M'}}, 2}, 1}, // trance: i VI III VII (three times), i VII VI VII, i VII VI V, VI VII i, i VI VII, i v VI III; two bars a chord
	{[]melHarm{{[]progStep{{0, 'm'}, {8, 'M'}, {3, 'M'}, {10, 'M'}}, 2}, {[]progStep{{0, 'm'}, {3, 'M'}, {10, 'M'}, {8, 'M'}}, 2}, {[]progStep{{0, 'm'}, {8, 'M'}, {10, 'M'}, {10, 'M'}}, 1},
		{[]progStep{{0, 'm'}, {8, 'M'}, {3, 'M'}, {10, 'M'}}, 2}, {[]progStep{{0, 'm'}, {7, 'm'}, {8, 'M'}, {3, 'M'}}, 2}},
		melHarm{[]progStep{{0, 'M'}, {7, 'D'}, {9, 'm'}, {5, 'M'}}, 2}, 4}, // progressive: i VI III VII (twice), i III VII VI, i VI VII, i v VI III; sus2
	{[]melHarm{{nil, 1}, {[]progStep{{0, 'm'}, {5, 'm'}}, 2}, {[]progStep{{0, 'm'}, {5, 's'}}, 2}},
		melHarm{nil, 1}, 5}, // dub techno: one chord, i7 iv7, i7 IVsus2, a chord two bars; open
	{[]melHarm{{nil, 1}}, melHarm{nil, 1}, 1}, // acid house: one chord
}

// melColors are the notes of a chord: the style's, or a voicing.
var melColors = []string{"the style's", "triads", "sevenths", "ninths", "sus2", "dub (root, fifth, seventh, the third up)", "seventh at the bottom (Kerri Chandler)"}

// melChordLens are the bars a chord of the progression lasts (0: the style's).
var melChordLens = []int{0, 1, 2, 4}

// melSwings are how late every second 16th comes (MPC swing, %; 0: the role's, a rolling bass 57, the rest straight).
var melSwings = []int{0, 50, 54, 57, 60}

// melForms are how the bars of a motif follow each other: A A' A B (the end changes), A A B B, or the same bar.
var melForms = []string{"A A' A B", "A A B B", "A A A A (the same bar)"}

// melEchoes are the steps between a dub chord's echoes: 3/16 (the dotted 8th), 5/16, 7/16.
var melEchoes = []int{3, 5, 7}

var melModes = []struct {
	name  string
	steps []int
	minor bool
}{
	{"minor", []int{0, 2, 3, 5, 7, 8, 10}, true},
	{"major", []int{0, 2, 4, 5, 7, 9, 11}, false},
	{"dorian", []int{0, 2, 3, 5, 7, 9, 10}, true},
	{"phrygian", []int{0, 1, 3, 5, 7, 8, 10}, true},
	{"harmonic minor", []int{0, 2, 3, 5, 7, 8, 11}, true},
	{"minor pentatonic", []int{0, 3, 5, 7, 10}, true},
}

// melOctaves are the octaves the notes go from and to: the role's own range, or an octave counted from the pad's own pitch (0: the
// octave from it up; on the synth the one from C3, a bass's from C2). A pad plays a sample one octave up and down at most.
var melOctaves = []string{"the type's", "-3", "-2", "-1", "0", "+1", "+2", "+3"}

// melOctave is the octave of an index of melOctaves (not the first).
func melOctave(i int) int { return i - 4 }

var melDensities = []string{"sparse", "medium", "busy"}

// melBars are the lengths a melody can have, in bars; 0 is the pattern's.
var melBars = []int{0, 1, 2, 4, 8}
var melGhosts = []string{"none", "a few", "many"}
var melAccents = []string{"none", "on the offbeats", "on the beats", "here and there"}

// melodySet is the dialog's settings, kept between visits; key is -1 until the bank's key is read.
type melodySet struct {
	key, mode, role, style, density, ghost, accent int
	prog                                           int // 0 the style's, 1 the key's one chord, else progressions[prog-2]
	chordLen, color                                int // indexes of melChordLens and melColors: 0 the style's
	swing, form, echo                              int // indexes of melSwings, melForms and melEchoes
	pick                                           int // the roll of the dice: which of the style's loops
	octLo, octHi                                   int // indexes of melOctaves: 0 the role's
	bars                                           int // an index of melBars: 0 the pattern's length
	rate                                           int // an index of melRates: 0 the style's rhythm
	out                                            int // where the melody plays, an index of synthOutNames (0: the pad); one for every role
	typ                                            int // an index of melTypes; role is its engine (melEngine)
	// the types' own settings, each kept while another type is chosen
	bassPat, approach   int // bass: melBassPats, melApproaches
	phrase, leaps       int // lead: melPhrases, melLeaps
	arpDir, arpSpan     int // arp: melArpDirs, melArpSpans
	arpGate             int // arp: melArpGates
	hook, place         int // pluck: melHooks, melPlaces
	stabRhythm, padMove int // stabs: melStabRhythms; pad: melPadMoves
	droneNote           int // drone: melDroneNotes
	// the 303: mel303Steps, mel303Pitches, mel303OffKey, mel303Slides, mel303Octaves, mel303Accents
	s3Steps, s3Pitches, s3OffKey, s3Slides, s3Octs, s3Accs int
	init                                                   bool
}

func (a *App) melodyMenu() {
	pad := a.rowPad()
	st := &a.mel
	bank := (a.pat - 1) / 16
	key, minor, why := a.bankKey(bank)
	if !st.init {
		*st = melodySet{density: 1, ghost: 1, accent: 1, init: true} // MIDI out off: the pad's sample plays the melody
		if !minor {
			st.mode = 1
		}
		melTypeOf(st, melRoleGuess(a.info.Pads[pad].Name))
	}
	st.key = key
	if why != "" && !minor && st.mode == 0 {
		st.mode = 1
	}
	keys := make([]string, 12)
	for i, n := range sharpNames {
		keys[i] = n
		if i == key && why != "" {
			keys[i] = n + "  (the bank's)"
		}
	}
	modes := make([]string, len(melModes))
	for i, m := range melModes {
		modes[i] = m.name
	}
	rates := make([]string, len(melRates))
	for i, r := range melRates {
		rates[i] = r.name
	}
	progs := []string{"the style's", "one chord (the key's)"}
	for _, p := range progressions {
		progs = append(progs, fmt.Sprintf("%s  %s", p.name, p.roman))
	}
	padNote, _ := a.sampleNote(pad)
	patBars := 0
	if d, ok := a.doc(); ok {
		patBars = max(int(d.Length)/(16*stepTicks), 1)
	}
	lengths := []string{fmt.Sprintf("the pattern's (%s)", count(max(patBars, 1), "bar"))}
	for _, b := range melBars[1:] {
		l := count(b, "bar")
		switch {
		case patBars > 0 && b < patBars:
			l += fmt.Sprintf(", repeated over the %d", patBars)
		case patBars > 0 && b > patBars:
			l += fmt.Sprintf(" (the synth; the pattern keeps %d)", patBars)
		}
		lengths = append(lengths, l)
	}
	settings := []menuItem{
		val("Type", melTypes, st.typ, func(v int) { st.typ = v }),
		{label: "THE TYPE'S OWN", header: true},
		val("Pattern", melBassPats, st.bassPat, func(v int) { st.bassPat = v }),
		val("Approach", melApproaches, st.approach, func(v int) { st.approach = v }),
		val("Phrase", melPhrases, st.phrase, func(v int) { st.phrase = v }),
		val("Leaps", melLeaps, st.leaps, func(v int) { st.leaps = v }),
		val("Direction", melArpDirs, st.arpDir, func(v int) { st.arpDir = v }),
		val("Span", melArpSpans, st.arpSpan, func(v int) { st.arpSpan = v }),
		val("Gate", melArpGates, st.arpGate, func(v int) { st.arpGate = v }),
		val("Hook", melHooks, st.hook, func(v int) { st.hook = v }),
		val("Placement", melPlaces, st.place, func(v int) { st.place = v }),
		val("Rhythm", melStabRhythms, st.stabRhythm, func(v int) { st.stabRhythm = v }),
		val("Echo (dub chord)", []string{"3/16 (dotted 8th)", "5/16", "7/16"}, st.echo, func(v int) { st.echo = v }),
		val("Movement", melPadMoves, st.padMove, func(v int) { st.padMove = v }),
		val("Drone note", melDroneNotes, st.droneNote, func(v int) { st.droneNote = v }),
		val("Steps", mel303StepNames, st.s3Steps, func(v int) { st.s3Steps = v }),
		val("Pitches", mel303Pitches, st.s3Pitches, func(v int) { st.s3Pitches = v }),
		val("Off-key notes", mel303OffKey, st.s3OffKey, func(v int) { st.s3OffKey = v }),
		val("Slides", mel303Slides, st.s3Slides, func(v int) { st.s3Slides = v }),
		val("Octave jumps", mel303Octaves, st.s3Octs, func(v int) { st.s3Octs = v }),
		val("Accent steps", mel303Accents, st.s3Accs, func(v int) { st.s3Accs = v }),
		{label: "FOR EVERY TYPE", header: true},
		val("Length", lengths, st.bars, func(v int) { st.bars = v }),
		val("Style", melStyles, st.style, func(v int) { st.style = v }),
		val("Notes", rates, st.rate, func(v int) { st.rate = v }),
		val("Key", keys, st.key, func(v int) { st.key = v }),
		val("Scale", modes, st.mode, func(v int) { st.mode = v }),
		{label: "Chords", vals: progs, val: st.prog, onChange: func(v int) { st.prog = v }, run: func(int) { a.melProgList(a.menu) }},
		val("Chord length", []string{"the style's", "1 bar", "2 bars", "4 bars"}, st.chordLen, func(v int) { st.chordLen = v }),
		val("Chord notes", melColors, st.color, func(v int) { st.color = v }),
		val("Form", melForms, st.form, func(v int) { st.form = v }),
		val("Swing", []string{"the type's", "none (straight)", "54%", "57%", "60%"}, st.swing, func(v int) { st.swing = v }),
		val("Octave from", melOctaves, st.octLo, func(v int) { st.octLo = v }),
		val("Octave to", melOctaves, st.octHi, func(v int) { st.octHi = v }),
		val("Density", melDensities, st.density, func(v int) { st.density = v }),
		val("Ghost notes", melGhosts, st.ghost, func(v int) { st.ghost = v }),
		val("Accents", melAccents, st.accent, func(v int) { st.accent = v }),
	}
	online := a.live != nil
	if online { // the synth behind the unit: only while the unit is edited live, and only when asked
		settings = append(settings, val("MIDI out", synthOutNames, st.out, func(v int) { st.out = v }))
	}
	var dlg *menu
	refresh := func() { // the type's own settings, and only the settings that apply, are shown
		st.role = melEngine(*st)
		if dlg == nil {
			return
		}
		head, own, inOwn := -1, 0, false
		for i := range dlg.items {
			it := &dlg.items[i]
			if it.header {
				inOwn = strings.HasSuffix(it.label, "'S OWN")
				if inOwn {
					head = i
				}
				continue
			}
			it.hidden = !melOwn(it.label, *st)
			if inOwn && !it.hidden {
				own++
			}
		}
		if head >= 0 {
			dlg.items[head].label = strings.ToUpper("the "+melTypes[st.typ]) + "'S OWN"
			dlg.items[head].hidden = own == 0
		}
	}
	for i := range settings {
		if orig := settings[i].onChange; orig != nil {
			settings[i].onChange = func(v int) { orig(v); refresh() }
		}
	}
	st.role = melEngine(*st)
	a.toolNoPlay = true
	defer func() { a.toolNoPlay = false }()
	a.toolMenu(fmt.Sprintf("melody for %s (sample in %s)", a.label(pad), sharpNames[padNote]), settings, "Write the melody", func() {
		d, err := a.ensurePattern()
		if err != nil {
			a.say(err.Error(), true)
			return
		}
		bars := max(int(d.Length)/(16*stepTicks), 1)
		mb := bars
		if st.bars > 0 {
			mb = melBars[st.bars]
		}
		pn, _ := a.sampleNote(pad)
		st.pick = int(a.tt.seed & 0xffff) // the style's loop of this roll
		line := melodyLine(pad, mb, *st, pn, a.tt.seed)
		notes := padPitches(line)
		what := fmt.Sprintf("%s %s in %s %s, %s", melStyles[st.style], melTypeName(*st), sharpNames[st.key], melModes[st.mode].name, count(mb, "bar"))
		o := st.out
		if lo, hi := melWindow(*st); (o == 0 || !online) && (lo < -formats.PitchRange || hi > formats.PitchRange) {
			what += " (a pad goes an octave up and down at most: the notes further out are moved in)"
		}
		a.finishTrack(pad, fitBars(notes, mb, bars), what)
		if o > 0 && online {
			k := ((st.key-pn)%12 + 12) % 12
			if k > 6 {
				k -= 12
			}
			if a.synthPlay(pad, line, mb*16*stepTicks, 10+o, synthTonic(st.role, st.key), k) {
				a.say(what+" "+synthSaid(10+o, a.midiOutName), false)
			}
		} else if a.synth != nil {
			a.synthStop()
		}
	})
	dlg = a.menu
	refresh()
	if m := a.menu; m != nil { // the synth stops with the dialog; Space plays the pattern on the computer, or stops it
		m.onX = func() {
			a.togglePlay()
			a.synthSync()
		}
		m.xHintOf = func() string {
			if a.seqOn {
				return "stop"
			}
			return "play here"
		}
		m.foot, m.footH = a.melodyFoot(m), 1+melodyHelpRows+3 // what the setting at the cursor does, then NOW
		orig := m.onClose
		m.onClose = func() {
			a.synthStop()
			if orig != nil {
				orig()
			}
		}
	}
	said := "no key found in the bank's names or sounds: C minor, change it in Key"
	if why != "" {
		said = fmt.Sprintf("the bank is in %s %s (%s)", sharpNames[key], map[bool]string{true: "minor", false: "major"}[minor], why)
	}
	if a.synth != nil {
		said += "; playing " + synthSaid(a.synth.ch, a.midiOutName)
	}
	a.say(said, false)
}

// melRoleGuess is the role a pad's name points to (a bass, a lead, a pad...), else the lead.
func melRoleGuess(name string) int {
	n := strings.ToLower(name)
	has := func(ws ...string) bool {
		for _, w := range ws {
			if strings.Contains(n, w) {
				return true
			}
		}
		return false
	}
	switch {
	case has("acid", "303"):
		return 7
	case has("sub"):
		return 5
	case has("bass", "808"):
		return 0
	case has("arp"):
		return 2
	case has("pluck"):
		return 8
	case has("piano", "keys", "organ", "m1"):
		return 10
	case has("stab", "chord", "key"):
		return 3
	case has("drone"):
		return 11
	case has("dub", "echo"):
		return 12
	case has("pad", "string", "atmo"):
		return 4
	}
	return 1
}

// bankKey is the key of a bank: the note its names say most (G#, Gs, Am, Cmin), else the note its tonal pads sound in; minor unless
// the names say major. why is where it came from, empty when nothing was found.
func (a *App) bankKey(bank int) (pc int, minor bool, why string) {
	var votes [12]int
	majors, minors, named := 0, 0, 0
	for p := bank * 16; p < bank*16+16; p++ {
		pd := a.info.Pads[p]
		if !pd.File {
			continue
		}
		if n, mode, ok := keyFromName(pd.Name); ok {
			votes[n]++
			named++
			switch mode {
			case 1:
				majors++
			case 0:
				minors++
			}
		}
	}
	best := func() int {
		b := 0
		for i := range votes {
			if votes[i] > votes[b] {
				b = i
			}
		}
		return b
	}
	if named > 0 {
		b := best()
		return b, majors <= minors, fmt.Sprintf("%s in the names of %s", sharpNames[b], count(votes[b], "pad"))
	}
	sounded := 0
	for p := bank * 16; p < bank*16+16; p++ {
		pd := a.info.Pads[p]
		if !pd.File || melRoleGuess(pd.Name) == 1 && !tonalName(pd.Name) { // (a name that says no role: only when it sounds tonal)
			continue
		}
		if id, ok := a.padSample(p); ok {
			if s, err := a.lab.Sample(id); err == nil {
				if n, ok := noteFromSound(s); ok {
					votes[n]++
					sounded++
				}
			}
		}
	}
	if sounded > 0 {
		return best(), true, fmt.Sprintf("the sound of %s", count(sounded, "tonal pad"))
	}
	return 0, true, ""
}

// tonalName is a name of a sound with a note (a lead, a synth...), not a drum.
func tonalName(name string) bool {
	n := strings.ToLower(name)
	for _, w := range []string{"lead", "synth", "pluck", "bell", "vox", "vocal", "melod", "hook", "loop"} {
		if strings.Contains(n, w) {
			return true
		}
	}
	return false
}

// melodyNotes writes the track: bars of the pattern, the settings, the note the pad's sample is in, and the roll of the dice.
// melodyNotes is the melody as notes of the pad: a pitch further than an octave from the pad's own is moved into that octave (a
// pad plays its sample one octave up and down at most).
func melodyNotes(pad, bars int, st melodySet, padNote int, seed int64) []lab.Note {
	return padPitches(melodyLine(pad, bars, st, padNote, seed))
}

// padPitches is the line for the pad: each pitch (in semitones) within an octave of the pad's, as the pattern's pitch byte.
func padPitches(line []lab.Note) []lab.Note {
	out := make([]lab.Note, len(line))
	for i, n := range line {
		for n.Pitch > formats.PitchRange {
			n.Pitch -= 12
		}
		for n.Pitch < -formats.PitchRange {
			n.Pitch += 12
		}
		n.Pitch = int(formats.PitchByte(n.Pitch))
		out[i] = n
	}
	return out
}

// melodyLine is the melody with its pitches in semitones from the pad's own pitch, as far as the octaves go (for the synth).
func melodyLine(pad, bars int, st melodySet, padNote int, seed int64) []lab.Note {
	rng := rand.New(rand.NewSource(seed*7919 + int64(pad)*131 + int64(st.role)))
	mode := melModes[st.mode]
	k := ((st.key-padNote)%12 + 12) % 12 // the tonic, in semitones from the pad's own pitch (-5..6)
	if k > 6 {
		k -= 12
	}
	inScale := map[int]bool{}
	for _, s := range mode.steps {
		inScale[s] = true
	}
	lo, hi := melWindow(st)
	var scale []int // the pitches of the scale in the range, low to high
	for p := lo; p <= hi; p++ {
		if inScale[((p-k)%12+12)%12] {
			scale = append(scale, p)
		}
	}
	chordOf := func(bar int) []int { // the pitch classes (from the tonic) of the bar's chord, root first
		step := melStep(st, bar)
		third := map[byte]int{'M': 4, 'D': 4, 'm': 3, 'd': 3, 's': 2}[step.q]
		fifth := 7
		if step.q == 'd' {
			fifth = 6
		}
		return []int{step.semi, (step.semi + third) % 12, (step.semi + fifth) % 12}
	}
	isTone := func(p int, chord []int) bool {
		for _, c := range chord {
			if ((p-k-c)%12+12)%12 == 0 {
				return true
			}
		}
		return false
	}
	nearest := func(p int, ok func(int) bool) int { // the nearest pitch of the range that ok takes
		for d := 0; d <= 48; d++ {
			for _, q := range []int{p - d, p + d} {
				if q >= lo && q <= hi && ok(q) {
					return q
				}
			}
		}
		return min(max(p, lo), hi)
	}
	rootNear := func(chord []int, near int) int {
		return nearest(near, func(q int) bool { return ((q-k-chord[0])%12+12)%12 == 0 })
	}
	toneNear := func(chord []int, near int) int { return nearest(near, func(q int) bool { return isTone(q, chord) }) }
	scaleIdx := func(p int) int { // the place of p in the scale, or of the pitch below it
		i := sort.SearchInts(scale, p)
		if i < len(scale) && scale[i] == p {
			return i
		}
		return max(i-1, 0)
	}
	color := melColor(st)
	fold := func(p int) int { // into the range, an octave at a time
		for p > hi {
			p -= 12
		}
		for p < lo {
			p += 12
		}
		return p
	}
	voiceOf := func(bar, near int) []int { // the bar's chord in the chosen notes, its root near near
		root := rootNear(chordOf(bar), near)
		var v []int
		for _, iv := range melVoicing(melStep(st, bar), color, inScale) {
			if p := fold(root + iv); !containsInt(v, p) {
				v = append(v, p)
			}
		}
		sort.Ints(v)
		return v
	}
	colorOf := func(bar int) []int { // the pitch classes (from the tonic) of the chord in its notes, for the arp
		step := melStep(st, bar)
		var pcs []int
		for _, iv := range melVoicing(step, color, inScale) {
			pcs = append(pcs, ((step.semi+iv)%12+12)%12)
		}
		return pcs
	}
	centre := (lo + hi) / 2
	type hit struct{ step, length, pitch, vel, ticks int } // ticks: the length in ticks, when set (an arp's gate)
	var out []hit
	add := func(bar int, h hit) { h.step += bar * 16; out = append(out, h) }

	switch st.role {
	case 0: // bass: the style's rhythm, the chord's root mostly, its octave and fifth, an approach into the next chord
		tmpl := melBassRhythm(st.style, st.density, rng)
		rootW := []float64{0.65, 0.5, 0.6, 0.7, 0.85, 0.7, 0.8, 0.5}[st.style]
		fn := make([]int, len(tmpl)) // 0 root, 1 octave, 2 fifth, 3 a scale step from the root
		roll := func(i int) {
			r := rng.Float64()
			switch {
			case r < rootW:
				fn[i] = 0
			case r < rootW+(1-rootW)*0.4:
				fn[i] = 1
			case r < rootW+(1-rootW)*0.75:
				fn[i] = 2
			default:
				fn[i] = 3
			}
		}
		for i := range tmpl {
			roll(i)
		}
		if st.style == 3 && rng.Intn(2) == 0 { // techno: the root on the beats, the fifth on the 8ths between
			tmpl, fn = nil, nil
			for s := 0; s < 16; s += 2 {
				tmpl = append(tmpl, [2]int{s, 1})
				fn = append(fn, map[bool]int{true: 0, false: 2}[s%4 == 0])
			}
		}
		for i, h := range tmpl { // house: an octave or a fifth on the last offbeat
			if st.style == 0 && h[0] == 14 && rng.Intn(2) == 0 {
				fn[i] = 1 + rng.Intn(2)
			}
		}
		fnB := append([]int(nil), fn...) // the B bars of A A B B: the second half again
		for i, h := range tmpl {
			if h[0] >= 8 {
				old := fn[i]
				roll(i)
				fnB[i], fn[i] = fn[i], old
			}
		}
		base := rootNear(chordOf(0), min(max(0, lo), hi)) // the bass sits on the root nearest the pad's own pitch
		for bar := 0; bar < bars; bar++ {
			f := append([]int(nil), fn...)
			switch form := bar % 4; {
			case st.form == 1 && form >= 2:
				f = append([]int(nil), fnB...)
			case st.form == 0 && (form == 1 || form == 3): // A' and B: the end of the bar changes
				for i := max(len(f)-1-form/2, 0); i < len(f); i++ {
					old := fn[i]
					roll(i)
					f[i], fn[i] = fn[i], old
				}
			}
			ch := chordOf(bar)
			root := rootNear(ch, base)
			for i, h := range tmpl {
				p := root
				switch f[i] {
				case 1:
					if root+12 <= hi {
						p = root + 12
					} else if root-12 >= lo {
						p = root - 12
					}
				case 2:
					p = nearest(root+7, func(q int) bool { return ((q-k-ch[2])%12+12)%12 == 0 })
				case 3:
					j := scaleIdx(root) + []int{-1, 1}[rng.Intn(2)]
					p = scale[min(max(j, 0), len(scale)-1)]
				}
				if i == len(tmpl)-1 && bar+1 < bars && melStep(st, bar+1) != melStep(st, bar) && (st.approach == 2 || st.approach == 0 && rng.Float64() < 0.5) { // into the next chord, a step away
					next := rootNear(chordOf(bar+1), root)
					j := scaleIdx(next) + []int{-1, 1}[rng.Intn(2)]
					p = scale[min(max(j, 0), len(scale)-1)]
				}
				add(bar, hit{h[0], h[1], p, 100, 0})
			}
		}
	case 1: // lead: a motif of one bar, walking the scale, the chord tones on the strong steps; A A' A B
		rhythm := melLeadRhythm(st.style, st.density, rng)
		walk := make([]int, len(rhythm)) // the motif as moves in the scale from its first note
		for i := 1; i < len(walk); i++ {
			moves := []int{-2, -1, -1, 0, 1, 1, 2, -3, 3}
			if st.leaps == 1 { // wide: 4ths, 5ths and octaves (in scale steps), now and then a step
				moves = []int{-4, -3, 3, 4, 7, -7, 1, -1, 4}
			}
			walk[i] = walk[i-1] + moves[rng.Intn(9)]
		}
		start := toneNear(chordOf(0), centre)
		off := make([]int, len(rhythm)) // where a repeated note steps off to (0: it stays), the same in every bar
		for i := range off {
			if rng.Intn(3) > 0 {
				off[i] = []int{-1, 1}[rng.Intn(2)]
			}
		}
		walkB := append([]int(nil), walk...) // the B bars of A A B B: the second half moves elsewhere
		for i := max(len(walkB)/2, 1); i < len(walkB); i++ {
			walkB[i] = walkB[i-1] + []int{-2, -1, 1, 2, 3}[rng.Intn(5)]
		}
		for bar := 0; bar < bars; bar++ {
			form := bar % 4
			r, w := rhythm, walk
			switch {
			case st.form == 1 && form >= 2:
				w = walkB
			case st.form == 0 && (form == 1 || form == 3): // the answer: the end changes
				w = append([]int(nil), walk...)
				from := len(w) - 1 - form/2
				for i := max(from, 1); i < len(w); i++ {
					w[i] = w[i-1] + []int{-2, -1, 1, 2}[rng.Intn(4)]
				}
			}
			ch := chordOf(bar)
			first := toneNear(ch, start)
			prev := -100
			for i, h := range r {
				j := min(max(scaleIdx(first)+w[i], 0), len(scale)-1)
				p := scale[j]
				if h[0]%4 == 0 || i == 0 { // a strong step: a chord tone
					p = toneNear(ch, p)
				}
				if p == prev && off[i] != 0 { // a line moves: the same note twice is rare
					j = min(max(scaleIdx(p)+off[i], 0), len(scale)-1)
					p = scale[j]
				}
				prev = p
				if bar == bars-1 && i == len(r)-1 && (bars%4 == 0 || bars < 4) { // the last note comes home
					p = rootNear(ch, p)
				}
				add(bar, hit{h[0], h[1], p, 96, 0})
			}
		}
	case 2: // arp: the chord's tones in a direction (up, or up and down, as the type's) over the range or a span of octaves
		rate := []int{2, 2, 1, 1, 1, 1, 2, 1}[st.style] // in steps: 8ths in house and dub techno, 16ths else
		if st.density == 0 {
			rate *= 2
		}
		dir := st.arpDir
		if updown := rng.Intn(2) == 0; dir == adType {
			dir = adUp
			if updown {
				dir = adUpDown
			}
		}
		gate := 0 // ticks; 0 the type's (2/3 of the step)
		if st.arpGate > 0 {
			gate = rate * stepTicks * []int{0, 25, 50, 75, 100}[st.arpGate] / 100
		}
		for bar := 0; bar < bars; bar++ {
			ch := chordOf(bar)
			cc := colorOf(bar)
			var tones []int
			if span := st.arpSpan; span > 0 {
				from := rootNear(ch, max(lo, centre-6*span))
				for p := from; p < from+12*span && p <= hi; p++ {
					if isTone(p, cc) {
						tones = append(tones, p)
					}
				}
			} else {
				for p := lo; p <= hi; p++ {
					if isTone(p, cc) && p >= centre-7 && p <= centre+9 {
						tones = append(tones, p)
					}
				}
			}
			if len(tones) == 0 {
				tones = []int{rootNear(ch, centre)}
			}
			seq := melArpSeq(dir, tones, scale, fold, rng)
			for s, i := 0, 0; s < 16; s, i = s+rate, i+1 {
				if dir == adChord {
					for _, p := range voiceOf(bar, tones[0]) {
						add(bar, hit{s, max(rate*2/3, 1), p, 92, gate})
					}
					continue
				}
				p := seq[i%len(seq)]
				if dir == adRandom {
					p = tones[rng.Intn(len(tones))]
				}
				add(bar, hit{s, max(rate*2/3, 1), p, 92, gate})
			}
		}
	case 5: // sub bass: the root held, a bar, half bars or a little push
		tmpl := [][][2]int{{{0, 16}}, {{0, 8}, {8, 8}}, {{0, 6}, {6, 2}, {8, 6}, {14, 2}}}[st.density]
		for bar := 0; bar < bars; bar++ {
			root := rootNear(chordOf(bar), lo+3)
			for _, h := range tmpl {
				add(bar, hit{h[0], h[1], root, 104, 0})
			}
		}
	case 6: // octave bass: the root low and high in turn, on 8ths (the offbeats when sparse, 16ths when busy)
		step := []int{4, 2, 1}[st.density]
		for bar := 0; bar < bars; bar++ {
			ch := chordOf(bar)
			low := rootNear(ch, lo+2)
			high := low + 12
			switch {
			case high <= hi:
			case low-12 >= lo:
				high, low = low, low-12
			default: // no octave fits in the range: the root and its fifth
				high = nearest(low+7, func(q int) bool { return ((q-k-ch[2])%12+12)%12 == 0 })
			}
			first := 0
			if st.density == 0 {
				first = 2
			}
			for s, i := first, 0; s < 16; s, i = s+step, i+1 {
				p := low
				if i%2 == 1 {
					p = high
				}
				add(bar, hit{s, 1, p, 100, 0})
			}
		}
	case 7: // 303: as the TB-303's sequencer has it, a row of pitches (each with its octave up or down, slide and accent) and a
		// row of time (Note, Tie, Rest), each looping on its own: a pitch is taken at each Note, so when the rows differ in length
		// the line turns over itself; the time row is Steps long, so a 12 or 7 against the 4/4 drifts and comes back
		line303(st, bars, rng, scale, scaleIdx, lo, hi, func(bar int) int { return rootNear(chordOf(bar), lo+5) }, func(bar int, s, l, p, v int) {
			add(bar, hit{s, l, p, v, 0})
		})
	case 8: // pluck: short notes on the offbeat 16ths, the chord's tones in a set order (a hook)
		cands := []int{2, 3, 6, 7, 10, 11, 14}
		if st.place == 1 { // anywhere: every 16th
			cands = []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
		}
		prob := []float64{0.35, 0.5, 0.7}[st.density]
		var steps []int
		if st.hook == 0 {
			for _, c := range cands {
				if rng.Float64() < prob {
					steps = append(steps, c)
				}
			}
		}
		want := 3
		if st.hook > 0 {
			want = st.hook + 2 // 3 to 7 notes
		}
		for len(steps) < want {
			c := cands[rng.Intn(len(cands))]
			if !containsInt(steps, c) {
				steps = append(steps, c)
			}
		}
		sort.Ints(steps)
		order := make([]int, len(steps))
		for i := range order {
			order[i] = rng.Intn(4)
		}
		for bar := 0; bar < bars; bar++ {
			ch := chordOf(bar)
			var tones []int
			for p := centre - 2; p <= hi && len(tones) < 4; p++ {
				if isTone(p, ch) && p >= lo {
					tones = append(tones, p)
				}
			}
			if len(tones) == 0 {
				tones = []int{toneNear(ch, centre)}
			}
			for i, s := range steps {
				o := order[i]
				if st.form == 0 && bar%4 == 3 && i == len(steps)-1 {
					o = 0 // the end of the phrase: its lowest tone
				}
				add(bar, hit{s, 1, tones[o%len(tones)], 96, 0})
			}
		}
	case 9: // call and response: a phrase in the first half of the bar, its answer in the second; the answer ends higher, then home
		rhythm := melLeadRhythm(st.style, st.density, rng)
		var call, resp [][2]int
		for _, h := range rhythm {
			if h[0] < 8 {
				call = append(call, [2]int{h[0], min(h[1], 8-h[0])})
			} else {
				resp = append(resp, h)
			}
		}
		if len(call) == 0 {
			call = [][2]int{{0, 2}, {2, 2}}
		}
		if len(resp) == 0 {
			resp = [][2]int{{8, 2}, {12, 2}}
		}
		cw := make([]int, len(call))
		for i := 1; i < len(cw); i++ {
			cw[i] = cw[i-1] + [][]int{{-2, -1, 1, 2}, {-4, -3, 3, 4}}[st.leaps][rng.Intn(4)]
		}
		for bar := 0; bar < bars; bar++ {
			ch := chordOf(bar)
			first := toneNear(ch, centre)
			for i, h := range call {
				p := scale[min(max(scaleIdx(first)+cw[i], 0), len(scale)-1)]
				if i == 0 {
					p = toneNear(ch, p)
				}
				add(bar, hit{h[0], h[1], p, 98, 0})
			}
			last := scaleIdx(first) + cw[len(cw)-1]
			for i, h := range resp {
				last += []int{-1, 1, 1, 2}[rng.Intn(4)]
				if last >= len(scale)-1 { // the top of the range: it turns
					last -= 3
				}
				p := scale[min(max(last, 0), len(scale)-1)]
				if i == len(resp)-1 { // the answer ends: on a chord tone, the root every other bar
					if bar%2 == 1 {
						p = rootNear(ch, p)
					} else {
						p = toneNear(ch, p)
					}
				}
				add(bar, hit{h[0], h[1], p, 90, 0})
			}
		}
	case 10: // piano house: the chord broken, the whole chord and its top two notes in turn, on the classic syncopation
		steps := [][]int{{3, 10}, {0, 3, 6, 10, 13}, {0, 3, 6, 8, 10, 13, 15}}[st.density]
		for bar := 0; bar < bars; bar++ {
			voice := voiceOf(bar, centre-3)
			for i, s := range steps {
				v := voice
				if i%2 == 1 { // the top two
					v = voice[1:]
				}
				for _, p := range v {
					add(bar, hit{s, 2, p, 94, 0})
				}
			}
		}
	case 11: // drone: the key's tonic held under everything (its fifth with it when busy)
		for bar := 0; bar < bars; bar++ {
			tonic := rootNear([]int{0}, lo+5)
			add(bar, hit{0, 16, tonic, 88, 0})
			if st.droneNote == 2 || st.droneNote == 0 && st.density == 2 {
				add(bar, hit{0, 16, nearest(tonic+7, func(q int) bool { return ((q-k-7)%12+12)%12 == 0 }), 80, 0})
			}
		}
	case 12: // dub chord: a seventh chord on an offbeat, its echoes a dotted 8th apart, each quieter (the delay of dub techno)
		stabs := [][]int{{2}, {2, 10}, {2, 7, 10}}[st.density]
		if rng.Intn(2) == 0 { // or a push: the "and" of 2 and 4
			stabs = [][]int{{6}, {6, 14}, {3, 6, 14}}[st.density]
		}
		for bar := 0; bar < bars; bar++ {
			voice := voiceOf(bar, centre-4)
			if color == 1 { // a dub chord has its seventh at least
				root := rootNear(chordOf(bar), centre-4)
				voice = nil
				for _, iv := range melVoicing(melStep(st, bar), 2, inScale) {
					voice = append(voice, fold(root+iv))
				}
				sort.Ints(voice)
			}
			for _, s0 := range stabs {
				for e, vel := range []int{92, 55, 33, 20} { // the stab, then its echoes
					s := s0 + e*melEchoes[st.echo] // 3 16ths (the dotted 8th, the classic dub delay), 5 or 7
					if bar*16+s >= bars*16 {
						break
					}
					for _, p := range voice {
						add(bar, hit{s, 1, p, vel, 0})
					}
				}
			}
		}
	case 13: // parallel stabs (Detroit, Kerri Chandler): one chord shape, moved as it is to other roots, even out of the key
		shape := voiceOf(0, centre-3)
		steps := [][]int{{0, 6, 10}, {0, 3, 6, 10, 13}, {0, 3, 6, 8, 10, 13, 15}}[st.density]
		moves := make([]int, len(steps)*2) // two bars of roots, in semitones from the bar's chord
		for i := range moves {
			if i > 0 && rng.Intn(3) > 0 {
				moves[i] = []int{-2, 3, 5, 7, -5, 2, -3}[rng.Intn(7)]
			}
		}
		for bar := 0; bar < bars; bar++ {
			off := rootNear(chordOf(bar), shape[0]) - rootNear(chordOf(0), shape[0])
			for i, s := range steps {
				m := moves[(bar%2)*len(steps)+i]
				if bar == bars-1 && i == len(steps)-1 && bars > 1 {
					m = 0 // the end comes home
				}
				d := off + m
				for _, c := range []int{off + m, off + m - 12, off + m + 12, off, off - 12, off + 12} { // the shape stays whole: an octave the other way, or home
					if shape[0]+c >= lo && shape[len(shape)-1]+c <= hi {
						d = c
						break
					}
				}
				for _, p := range shape {
					add(bar, hit{s, 1, min(max(p+d, lo), hi), 94, 0})
				}
			}
		}
	case 14: // rolling bass (Chris Stussy): root, fifth, octave in a 16th roll; bar 2 the root for the octave, bar 4 a run up home
		cell := [][2]int{{0, 0}, {4, 1}, {5, 0}, {6, 2}, {7, 0}, {8, 0}, {12, 1}, {13, 0}, {14, 2}, {15, 0}} // step, and 0 root 1 fifth 2 octave
		switch st.density {
		case 0:
			cell = [][2]int{{0, 0}, {4, 1}, {5, 0}, {6, 2}, {12, 1}, {13, 0}, {14, 2}}
		case 2:
			cell = append(cell, [2]int{10, 0}, [2]int{11, 1})
		}
		for bar := 0; bar < bars; bar++ {
			ch := chordOf(bar)
			root := rootNear(ch, lo+5)
			fifth := nearest(root+7, func(q int) bool { return ((q-k-ch[2])%12+12)%12 == 0 })
			oct := root + 12
			if oct > hi {
				oct = root
			}
			for _, c := range cell {
				s, p := c[0], root
				switch {
				case c[1] == 1:
					p = fifth
				case c[1] == 2 && bar%4 != 1:
					p = oct
				}
				if bar%4 == 3 && s >= 12 { // the run: up the scale from the fifth to the octave
					p = scale[min(scaleIdx(fifth)+(s-12), len(scale)-1)]
					if s == 15 {
						p = rootNear(ch, oct)
					}
				}
				vel := 74 + rng.Intn(8)
				switch {
				case s == 0:
					vel = 110
				case s == 6 || s == 14:
					vel = 90
				}
				add(bar, hit{s, 1, p, vel, 0})
			}
		}
	case 15: // the bass type's patterns: the classic bass lines of house, techno and trance, on the bar's root
		pat := st.bassPat
		if pat == bpStyle || pat == bpOctave || pat == bpSub || pat == bpRoll { // (those have engines of their own)
			pat = bpOffbeat
		}
		base := rootNear(chordOf(0), min(max(0, lo), hi))
		upFifth := rng.Intn(2) == 0 // the slide arc's goal: the fifth, else the fourth
		for bar := 0; bar < bars; bar++ {
			ch := chordOf(bar)
			root := rootNear(ch, base)
			oct := root + 12
			if oct > hi {
				oct = root
			}
			fifth := nearest(root+7, func(q int) bool { return ((q-k-ch[2])%12+12)%12 == 0 })
			below := scale[max(scaleIdx(root)-1, 0)]
			goal := fifth
			if !upFifth {
				goal = scale[min(scaleIdx(root)+3, len(scale)-1)]
			}
			var hs []hit
			h := func(s, l, p, v int) { hs = append(hs, hit{s, l, p, v, 0}) }
			switch pat {
			case bpOffbeat: // one short note on every "and" (the donk of 90s house and trance); busy: the last an octave up
				for s := 2; s < 16; s += 4 {
					p := root
					if s == 14 && st.density == 2 {
						p = oct
					}
					h(s, 1, p, 104)
				}
			case bpRolling8: // every 8th on one note, a step down on beat 3 (Fisher, Alan Fitzpatrick), the velocity alive
				for s := 0; s < 16; s += 2 {
					if s == 0 && st.density == 0 {
						continue
					}
					p := root
					if s == 8 || s == 10 {
						p = below
					}
					h(s, 1, p, 88+rng.Intn(16))
				}
				if st.density == 2 {
					h(15, 1, oct, 84)
				}
			case bpRolling16: // the 16ths around the kick: the beat left to it
				for s := 0; s < 16; s++ {
					if s%4 == 0 || st.density == 0 && s%4 == 1 {
						continue
					}
					p, v := root, 92
					if s%4 == 2 {
						v = 106
					}
					if st.density == 2 && s%4 == 3 && rng.Intn(3) == 0 {
						p = oct
					}
					h(s, 1, p, v)
				}
			case bpDouble: // pairs on the offbeats (the "and" and the 16th after it)
				for b := 0; b < 4; b++ {
					if st.density == 0 && b%2 == 0 {
						continue
					}
					p2 := root
					if st.density == 2 && b == 3 {
						p2 = oct
					}
					h(b*4+2, 1, root, 104)
					h(b*4+3, 1, p2, 90)
				}
			case bpGallop: // an 8th on the beat, then two 16ths an octave up
				for b := 0; b < 4; b++ {
					if st.density == 0 && b%2 == 1 {
						continue
					}
					h(b*4, 2, root, 104)
					h(b*4+2, 1, oct, 90)
					h(b*4+3, 1, oct, 84)
				}
			case bpStabSus: // a stab on the one, then the note held over beats 2 and 3
				h(0, 1, root, 112)
				p := root
				if st.density > 0 {
					p = fifth
				}
				h(4, 8, p, 92)
				if st.density == 2 {
					h(14, 1, oct, 96)
				}
			case bpSyncSus: // held from the "and" of 1, a short stab on the "and" of 2; again in the second half
				h(2, 4, root, 100)
				h(6, 1, root, 112)
				if st.density > 0 {
					h(10, 4, root, 100)
					h(14, 1, fifth, 112)
				}
			case bpSlideArc: // two bars on the root, the third slides up to the fifth (or fourth), the fourth slides home
				switch bar % 4 {
				case 2:
					h(2, 2, root, 100)
					h(6, 3, root, 100) // held into the next: a slide on a synth set to legato
					h(8, 4, goal, 104)
					h(12, 2, goal, 96)
					h(14, 2, goal, 96)
				case 3:
					h(2, 3, goal, 100) // and back
					h(4, 2, root, 104)
					h(6, 2, root, 96)
					h(10, 2, root, 96)
					h(14, 2, root, 96)
				default:
					for s := 2; s < 16; s += 4 {
						h(s, 2, root, 100)
					}
				}
			}
			// into the next chord, a step away: now and then (the pattern's), never, or always
			if n := len(hs); n > 0 && pat != bpSlideArc && bar+1 < bars && melStep(st, bar+1) != melStep(st, bar) &&
				(st.approach == 2 || st.approach == 0 && rng.Float64() < 0.5) {
				next := rootNear(chordOf(bar+1), root)
				j := scaleIdx(next) + []int{-1, 1}[rng.Intn(2)]
				hs[n-1].pitch = scale[min(max(j, 0), len(scale)-1)]
			}
			for _, x := range hs {
				add(bar, x)
			}
		}
	case 3, 4: // stabs: the chord on the style's offbeats; pad: the chord held over the bar
		tmpl := [][][2]int{{{0, 16}}, {{0, 8}, {8, 8}}, {{0, 4}, {4, 4}, {8, 4}, {12, 4}}}[st.padMove]
		if st.role == 3 {
			tmpl = melStabRhythm(st.style, st.density, rng)
		}
		for bar := 0; bar < bars; bar++ {
			voice := voiceOf(bar, centre-3)
			r := tmpl
			if st.form == 0 && bar%4 == 3 && st.role == 3 && len(tmpl) > 1 && rng.Intn(2) == 0 { // B: one stab less
				r = tmpl[:len(tmpl)-1]
			}
			for i, h := range r {
				vel := 92
				if st.role == 4 && st.padMove == 2 { // rising: soft to loud over the bar
					vel = 64 + i*20
				}
				for _, p := range voice {
					add(bar, hit{h[0], h[1], p, vel, 0})
				}
			}
		}
	}

	// ghost notes: quiet, short notes on free 16ths, at the pitch before them (bass, lead and arp)
	if st.ghost > 0 && melRoles[st.role].kind <= kArp && st.role != 5 && st.role != 14 { // (a chord, a drone, a sub bass, a roll do not ghost)
		taken := map[int]bool{}
		for _, h := range out {
			taken[h.step] = true
		}
		prob := []float64{0, 0.18, 0.4}[st.ghost]
		sort.Slice(out, func(i, j int) bool { return out[i].step < out[j].step })
		var ghosts []hit
		last := -1
		for s := 0; s < bars*16; s++ {
			if taken[s] {
				for _, h := range out {
					if h.step == s {
						last = h.pitch
					}
				}
				continue
			}
			if last < 0 || s%4 == 0 || rng.Float64() >= prob {
				continue
			}
			ghosts = append(ghosts, hit{s, 1, last, 34 + rng.Intn(14), 0})
		}
		out = append(out, ghosts...)
		// a line with no room left (a rolling bass): some of its own notes off the 8ths go quiet instead
		for i, want := 0, bars*[]int{0, 1, 3}[st.ghost]-len(ghosts); i < len(out) && want > 0; i++ {
			if out[i].step%2 == 1 && out[i].vel >= 60 && rng.Float64() < prob*2 {
				out[i].vel, out[i].length = 34+rng.Intn(14), 1
				want--
			}
		}
	}
	// accents
	for i := range out {
		h := &out[i]
		if h.vel < 60 || st.role == 7 || (st.role == 4 && st.padMove == 2) { // a ghost; a 303's accents are its own; a rising pad its own
			continue
		}
		s := h.step % 16
		acc := false
		switch st.accent {
		case 1:
			acc = s%4 == 2
		case 2:
			acc = s%4 == 0
		case 3:
			acc = rng.Intn(4) == 0
		}
		v := h.vel - 10 + rng.Intn(9) // a little life
		if acc {
			v = 120 + rng.Intn(8)
		} else if st.accent > 0 {
			v -= 8
		}
		h.vel = min(max(v, 1), 127)
	}
	notes := make([]lab.Note, 0, len(out))
	for _, h := range out {
		if h.step >= bars*16 {
			continue
		}
		ln := min(h.length, bars*16-h.step) * stepTicks
		cut := ln - stepTicks/8
		if h.ticks > 0 {
			cut = min(h.ticks, ln)
		}
		notes = append(notes, lab.Note{Tick: uint32(h.step * stepTicks), Pad: pad, Vel: h.vel, Len: max(cut, 1), Pitch: h.pitch})
	}
	sort.SliceStable(notes, func(i, j int) bool { return notes[i].Tick < notes[j].Tick })
	sw := melSwings[st.swing]
	if sw == 0 && st.role == 14 { // the role's: a rolling bass swings, the rest is straight
		sw = 57
	}
	if d := (sw - 50) * 2 * stepTicks / 100; d > 0 && melRates[st.rate].ticks == 0 { // every second 16th a little late
		for i := range notes {
			if notes[i].Tick%(2*stepTicks) == stepTicks {
				notes[i].Tick += uint32(d)
				notes[i].Len = max(notes[i].Len-d, 1)
			}
		}
	}
	return applyRate(notes, melRoles[st.role].kind, melRates[st.rate].ticks, bars*16*stepTicks, rng)
}

// applyRate writes the notes in a note value of r ticks (0: as they are). An arp is laid out again at that rate, its order kept;
// the other lines are snapped onto the grid (a place keeps the notes of the first onset that falls on it, so a chord stays whole)
// and each note is held to the next or for its value; a value finer than a 16th adds doubles now and then (a roll, a pickup). Held
// notes stay as they are.
func applyRate(notes []lab.Note, kind, r, total int, rng *rand.Rand) []lab.Note {
	if r == 0 || kind == kHeld || len(notes) == 0 {
		return notes
	}
	barT := 16 * stepTicks
	if kind == kArp {
		var out []lab.Note
		for b := 0; b*barT < total; b++ {
			var seq []lab.Note
			for _, n := range notes {
				if int(n.Tick)/barT == b && n.Vel >= 60 {
					seq = append(seq, n)
				}
			}
			for i, t := 0, 0; len(seq) > 0 && t < barT && b*barT+t < total; i, t = i+1, t+r {
				n := seq[i%len(seq)]
				n.Tick, n.Len = uint32(b*barT+t), max(r*2/3, 1)
				out = append(out, n)
			}
		}
		return out
	}
	owner := map[int]uint32{}
	var out []lab.Note
	for _, n := range notes {
		t := (int(n.Tick) + r/2) / r * r
		if t >= total {
			continue
		}
		if o, ok := owner[t]; ok && o != n.Tick {
			continue
		}
		owner[t] = n.Tick
		n.Tick = uint32(t)
		out = append(out, n)
	}
	var onsets []int
	for t := range owner {
		onsets = append(onsets, t)
	}
	sort.Ints(onsets)
	next := func(t int) int {
		i := sort.SearchInts(onsets, t+1)
		if i < len(onsets) {
			return onsets[i]
		}
		return total
	}
	for i := range out {
		t := int(out[i].Tick)
		if r >= stepTicks { // a coarser value: the note fills it (or up to the next)
			out[i].Len = max(min(r, next(t)-t)-stepTicks/8, 1)
		} else {
			out[i].Len = max(min(out[i].Len, next(t)-t-5), 1)
		}
	}
	if r < stepTicks {
		var extra []lab.Note
		for _, n := range out {
			t := int(n.Tick) + r
			if n.Vel < 60 || rng.Intn(3) > 0 || t >= total || next(int(n.Tick)) <= t {
				continue
			}
			d := n
			d.Tick, d.Vel, d.Len = uint32(t), max(n.Vel-20, 30), max(r-5, 1)
			extra = append(extra, d)
		}
		out = append(out, extra...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Tick < out[j].Tick })
	return out
}

// melWindow is the range of pitch (semitones from the pad's own pitch) for the settings: the role's, or from the low octave's
// bottom to the high octave's top (turned round when they are the wrong way round).
func melWindow(st melodySet) (int, int) {
	lo, hi := melRoles[st.role].lo, melRoles[st.role].hi
	if st.role == 2 && st.arpSpan > 0 { // an arp over 2 or 3 octaves needs the room
		hi = max(hi, lo+12*st.arpSpan+5)
	}
	if st.octLo > 0 {
		lo = melOctave(st.octLo) * 12
	}
	if st.octHi > 0 {
		hi = melOctave(st.octHi)*12 + 12
	}
	if st.octLo > 0 && st.octHi > 0 && lo >= hi {
		lo, hi = melOctave(st.octHi)*12, melOctave(st.octLo)*12+12
	}
	if lo >= hi {
		if st.octLo > 0 {
			hi = lo + 12
		} else {
			lo = hi - 12
		}
	}
	return lo, hi
}

// melBassRhythm is a bar of a bass line in a style: the steps and lengths (in steps).
func melBassRhythm(style, density int, rng *rand.Rand) [][2]int {
	var steps []int
	length := 1
	switch style {
	case 0: // house: the offbeat, or now and then the 3+3+2 cell (Underworld's "Pearl's Girl")
		if rng.Intn(4) == 0 {
			return [][2]int{{0, 3}, {6, 3}, {12, 2}}
		}
		steps, length = []int{2, 6, 10, 14}, 2
		if density == 2 {
			steps = append(steps, 3, 11)
		}
	case 1: // deep house: syncopated, longer
		steps, length = []int{0, 3, 6, 10, 13}, 2
		if density == 0 {
			steps = []int{0, 6, 10}
		}
	case 2, 4: // tech house and trance: rolling 16ths around the kick; tech house now and then the broken offbeat
		if style == 2 && rng.Intn(3) == 0 {
			return [][2]int{{2, 2}, {6, 2}, {9, 2}, {14, 2}}
		}
		for s := 0; s < 16; s++ {
			if s%4 != 0 {
				steps = append(steps, s)
			}
		}
	case 3, 5: // techno and progressive: offbeat pairs
		steps = []int{2, 3, 6, 7, 10, 11, 14, 15}
	case 6: // dub techno: the root held (a sub drone), or a few notes between the beats
		return append([][2]int(nil), [][][2]int{{{0, 16}}, {{0, 12}, {14, 2}}, {{3, 2}, {6, 3}, {11, 2}, {14, 2}}}[density]...)
	case 7: // acid house: 16ths, a few left out
		for s := 0; s < 16; s++ {
			steps = append(steps, s)
		}
	}
	keep := []float64{0.6, 0.85, 1}[density]
	var out [][2]int
	for _, s := range steps {
		if s%4 == 2 || rng.Float64() < keep { // the offbeat stays
			out = append(out, [2]int{s, length})
		}
	}
	if len(out) == 0 {
		out = [][2]int{{2, length}}
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}

// melLeadRhythm is a bar of a lead in a style: weighted steps (syncopated in house, running in trance), each note held to the next.
func melLeadRhythm(style, density int, rng *rand.Rand) [][2]int {
	w := func(s int) float64 {
		switch {
		case s%4 == 0:
			return []float64{0.4, 0.35, 0.35, 0.45, 0.75, 0.6, 0.25, 0.5}[style]
		case s%2 == 0:
			return []float64{0.65, 0.5, 0.6, 0.5, 0.8, 0.7, 0.3, 0.6}[style]
		default:
			return []float64{0.35, 0.25, 0.4, 0.35, 0.45, 0.35, 0.12, 0.5}[style]
		}
	}
	scale := []float64{0.7, 1, 1.35}[density]
	var steps []int
	for s := 0; s < 16; s++ {
		if rng.Float64() < w(s)*scale {
			steps = append(steps, s)
		}
	}
	for len(steps) < 3 { // a motif has three notes at least
		s := rng.Intn(16)
		if !containsInt(steps, s) {
			steps = append(steps, s)
		}
	}
	sort.Ints(steps)
	maxLen := []int{2, 4, 2, 2, 2, 3, 4, 1}[style]
	out := make([][2]int, len(steps))
	for i, s := range steps {
		next := 16
		if i+1 < len(steps) {
			next = steps[i+1]
		}
		out[i] = [2]int{s, max(min(next-s, maxLen), 1)}
	}
	return out
}

// melStabRhythm is a bar of chord stabs in a style.
func melStabRhythm(style, density int, rng *rand.Rand) [][2]int {
	var steps []int
	switch style {
	case 1: // deep house: the classic push
		steps = []int{3, 6, 10, 13}
	case 2: // tech house: few and dry
		steps = []int{2, 10}
		if density == 2 {
			steps = []int{2, 7, 10}
		}
	case 3: // techno: the "and" of 2 and 4, the push on the last 16th held over the bar, or every 3 16ths against the kick
		switch {
		case density == 0:
			return [][2]int{{6, 1}, {14, 1}}
		case density == 2 || rng.Intn(3) == 0:
			return [][2]int{{0, 1}, {3, 1}, {6, 1}, {9, 1}, {12, 1}, {15, 1}}
		case rng.Intn(2) == 0:
			return [][2]int{{15, 2}}
		}
		return [][2]int{{6, 1}, {14, 1}}
	case 6: // dub techno: the chord on the "and", seldom
		return append([][2]int(nil), [][][2]int{{{6, 1}}, {{6, 1}, {14, 1}}, {{2, 1}, {6, 1}, {10, 1}, {14, 1}}}[density]...)
	default: // house, trance, progressive: the offbeats
		steps = []int{2, 6, 10, 14}
	}
	if density == 0 && len(steps) > 2 {
		steps = []int{steps[0], steps[2]}
	}
	if density == 2 && rng.Intn(2) == 0 && !containsInt(steps, 15) {
		steps = append(steps, 15)
	}
	out := make([][2]int, len(steps))
	for i, s := range steps {
		out[i] = [2]int{s, 1}
	}
	return out
}

func containsInt(xs []int, x int) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// melodyNow is the foot of the melody dialog: the bars of the pattern with the one that plays lit and the beat, what the time
// follows, and whether the melody in the dialog is the one the synth plays yet.
func (a *App) melodyNow(g Gfx, x, y, w int) {
	g.HRule(x+1, y, w-2, cLine)
	g.Fill(x+1, y, 6, 1, cPanel2)
	g.Text(x+2, y, "NOW", cDim, true)
	d, ok := a.doc()
	if !ok {
		return
	}
	length := int(d.Length)
	if s := a.synth; s != nil { // the synth loops the melody's own length
		s.mu.Lock()
		if s.length > 0 {
			length = s.length
		}
		s.mu.Unlock()
	}
	bars := max(length/(16*stepTicks), 1)
	start, loop, perTick, playing := a.synthTiming(length)
	if a.synth == nil && !a.seqOn {
		playing = false
	}
	bar, beat := -1, -1
	now := time.Now()
	barOf := func(t time.Time) int {
		if loop <= 0 || perTick <= 0 {
			return 0
		}
		el := t.Sub(start) % loop
		if el < 0 {
			el += loop
		}
		return int(float64(el)/perTick) / (16 * stepTicks)
	}
	if playing && loop > 0 && !now.Before(start) {
		tick := int(float64(now.Sub(start)%loop) / perTick)
		bar, beat = tick/(16*stepTicks), tick/(4*stepTicks)%4
	}
	cx := x + 2
	shown := 0 // the bars and their chords that fit before the beats, each as wide as its name
	for b := 0; b < bars; b++ {
		label := fmt.Sprintf("%d %s", b+1, a.melChordName(b))
		if b > 0 && cx+len(label)+16 > x+w {
			break
		}
		if b == bar {
			g.Fill(cx, y+1, len(label)+1, 1, cAmber)
			g.Text(cx, y+1, label, cBg, true)
		} else {
			g.Text(cx, y+1, label, cDim, false)
		}
		cx += max(len(label)+2, 8)
		shown++
	}
	if bars > shown && bar >= shown {
		g.Text(cx, y+1, fmt.Sprintf("bar %d", bar+1), cAmber, true)
		cx += 7
	}
	cx++
	for i := 0; i < 4; i++ {
		g.Badge(cx+i*2, y+1, fmt.Sprint(i+1), i == beat)
	}
	from := "the pattern here"
	if a.synthClock() {
		from = "the unit's clock"
	}
	var line string
	col := cText
	switch {
	case a.synth != nil:
		s := a.synth
		s.mu.Lock()
		ch, gen, sentGen, sentAt := s.ch, s.gen, s.sentGen, s.sentAt
		s.mu.Unlock()
		switch {
		case !playing:
			line, col = fmt.Sprintf("MIDI ch %d: waiting: start the unit (or Space)", ch), cDim
		case sentGen == gen:
			line = fmt.Sprintf("MIDI ch %d, melody #%d on the synth since bar %d (%s)", ch, gen, barOf(sentAt)+1, from)
		default:
			line, col = fmt.Sprintf("MIDI ch %d, melody #%d: from its next note", ch, gen), cAmber
		}
	case playing:
		line = "on the pad, heard here"
	default:
		line, col = "waiting: Space plays it here", cDim
	}
	g.Text(x+2, y+2, fit(line, w-4), col, false)
}

// fitBars puts a melody of mb bars into a pattern of bars: repeated when it is shorter, its first bars when it is longer.
func fitBars(notes []lab.Note, mb, bars int) []lab.Note {
	barT := 16 * stepTicks
	var out []lab.Note
	for off := 0; off < bars; off += mb {
		for _, n := range notes {
			t := int(n.Tick) + off*barT
			if t >= bars*barT {
				continue
			}
			n.Tick = uint32(t)
			n.Len = min(n.Len, bars*barT-t)
			out = append(out, n)
		}
	}
	return out
}

// melChordName is the chord of a bar in the dialog's key and progression (G#m, E, Bdim).
func (a *App) melChordName(bar int) string {
	st := a.mel
	step := melStep(st, bar)
	in := map[int]bool{}
	for _, s := range melModes[st.mode].steps {
		in[s] = true
	}
	return sharpNames[(st.key+step.semi)%12] + melChordSuffix(step, melVoicing(step, melColor(st), in))
}

// melSteps is the progression of the settings and the bars a chord lasts (no chords: the key's one chord).
func melSteps(st melodySet) ([]progStep, int) {
	per := 1
	var steps []progStep
	switch {
	case st.prog == 1:
	case st.prog >= 2:
		steps = progressions[st.prog-2].steps
	default:
		h := melStyleHarmony[st.style]
		alt := h.major
		if melModes[st.mode].minor {
			alt = h.minor[st.pick%len(h.minor)]
		}
		steps, per = alt.steps, alt.per
	}
	if st.chordLen > 0 {
		per = melChordLens[st.chordLen]
	}
	return steps, per
}

// melStep is the chord of a bar: the progression's, each chord as many bars as Chord length says.
func melStep(st melodySet, bar int) progStep {
	steps, per := melSteps(st)
	if len(steps) == 0 {
		return progStep{0, map[bool]byte{true: 'm', false: 'M'}[melModes[st.mode].minor]}
	}
	return steps[(bar/max(per, 1))%len(steps)]
}

// melColor is the notes of the chords (an index of melColors, not 0).
func melColor(st melodySet) int {
	if st.color > 0 {
		return st.color
	}
	return melStyleHarmony[st.style].color
}

// melVoicing is a chord in a voicing, as semitones from its root: the third and fifth by its quality, the seventh and ninth from
// the scale when it has them (the seventh of the VII of a minor key is a minor one), else by the quality.
func melVoicing(step progStep, color int, inScale map[int]bool) []int {
	in := func(iv int) bool { return inScale[((step.semi+iv)%12+12)%12] }
	pick := func(want, alt int) int {
		if !in(want) && in(alt) {
			return alt
		}
		return want
	}
	third, fifth := 3, 7
	switch step.q {
	case 'M', 'D':
		third = 4
	case 's': // sus2
		third = pick(2, 1)
	}
	if step.q == 'd' {
		fifth = 6
	}
	sev := pick(10, 11)
	if step.q == 'M' {
		sev = pick(11, 10)
	}
	ninth := pick(14, 13)
	switch color {
	case 2:
		return []int{0, third, fifth, sev}
	case 3:
		return []int{0, third, sev, ninth}
	case 4:
		return []int{0, pick(2, 1), fifth}
	case 5:
		return []int{0, fifth, sev, third + 12}
	case 6:
		return []int{sev - 12, 0, third, fifth}
	}
	return []int{0, third, fifth}
}

// melChordSuffix names a voiced chord after its root: m, m7, maj7, 7, m9, sus2...
func melChordSuffix(step progStep, v []int) string {
	has := func(iv int) bool {
		for _, x := range v {
			if (x%12+12)%12 == iv {
				return true
			}
		}
		return false
	}
	minor := has(3) && !has(4)
	ninth := (has(2) || has(1)) && !has(7)
	switch {
	case !has(3) && !has(4) && has(2):
		return "sus2"
	case has(10) && ninth:
		return map[bool]string{true: "m9", false: "9"}[minor]
	case has(11) && ninth:
		return map[bool]string{true: "m(maj9)", false: "maj9"}[minor]
	case !has(3) && !has(4) && has(1):
		return "sus(b2)"
	case has(10) && step.q == 'd':
		return "m7b5"
	case has(10):
		return map[bool]string{true: "m7", false: "7"}[minor]
	case has(11):
		return map[bool]string{true: "m(maj7)", false: "maj7"}[minor]
	}
	return progQualities[0][step.q][1]
}
