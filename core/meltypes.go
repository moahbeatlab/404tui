package core

import "math/rand"

// The Melody dialog's types: what is written (a bass, a 303 line, a lead...). Each type has settings of its own, shown under it
// while it is chosen and kept when another type is (back to the 303, its settings are as they were). A type and its settings
// come down to one of the engines of melodyLine (melRoles, by index: st.role).

var melTypes = []string{"bass", "303", "lead", "arp", "pluck", "stabs", "pad", "drone"}

const (
	tBass = iota
	t303
	tLead
	tArp
	tPluck
	tStabs
	tPad
	tDrone
)

// melBassPats are the bass type's patterns: the style's rhythm, then the classic bass lines of house and techno.
var melBassPats = []string{"the style's", "offbeat 8ths", "rolling 8ths", "rolling 16ths", "double offbeat", "gallop", "octave (low, high)",
	"sub (long roots)", "root, fifth, octave roll", "stab and sustain", "syncopated sustain", "slide arc"}

// bpOffbeat... are melBassPats by name, for the bass pattern engine.
const (
	bpStyle = iota
	bpOffbeat
	bpRolling8
	bpRolling16
	bpDouble
	bpGallop
	bpOctave
	bpSub
	bpRoll
	bpStabSus
	bpSyncSus
	bpSlideArc
)

var melApproaches = []string{"the pattern's (now and then)", "none", "always"}
var melPhrases = []string{"a motif (Form)", "call and response"}
var melLeaps = []string{"steps (the scale)", "wide (4ths, 5ths, octaves)"}

// melArpDirs are the arp's directions: every one of DOOM OS's (UP DOWN U/D U/D+ RND ORD CHRD CONV DIV PNKY THMB) and of the
// sample ARP's (UP-DN DN-UP THIRDS PEDAL GLISS STABS).
var melArpDirs = []string{"the type's (up, or up and down)", "up", "down", "up and down", "up and down, the ends twice", "down and up",
	"random", "order (root, fifth, octave, tenth)", "converge (outside in)", "diverge (inside out)", "pinky (the top between)",
	"thumb (the bottom between)", "thirds (1 3 2 4 3 5...)", "scale run (a gliss in the key)", "chord (all at once)"}

const (
	adType = iota
	adUp
	adDown
	adUpDown
	adUpDownPlus
	adDownUp
	adRandom
	adOrder
	adConverge
	adDiverge
	adPinky
	adThumb
	adThirds
	adScale
	adChord
)

var melArpSpans = []string{"the type's", "1 octave", "2 octaves", "3 octaves"}
var melArpGates = []string{"the type's (2/3)", "25%", "50%", "75%", "100% (legato)"}
var melHooks = []string{"the type's (3 or more)", "3 notes", "4 notes", "5 notes", "6 notes", "7 notes"}
var melPlaces = []string{"offbeats", "anywhere"}
var melStabRhythms = []string{"the style's", "piano house (M1)", "parallel shape (Detroit)", "dub chord (deep echo)"}
var melPadMoves = []string{"held", "struck again each half bar", "rising (soft to loud, 4 a bar)"}
var melDroneNotes = []string{"the type's (the fifth when busy)", "the root", "the root and the fifth"}

// melEngine is the engine (an index of melRoles) of the type and its settings.
func melEngine(st melodySet) int {
	switch st.typ {
	case tBass:
		switch st.bassPat {
		case bpStyle:
			return 0
		case bpOctave:
			return 6
		case bpSub:
			return 5
		case bpRoll:
			return 14
		}
		return 15
	case t303:
		return 7
	case tLead:
		return []int{1, 9}[st.phrase]
	case tArp:
		return 2
	case tPluck:
		return 8
	case tStabs:
		return []int{3, 10, 13, 12}[st.stabRhythm]
	case tPad:
		return 4
	}
	return 11
}

// melTypeOf sets the type and its settings that write with engine role (a guess from a pad's name).
func melTypeOf(st *melodySet, role int) {
	switch role {
	case 0, 5, 6, 14, 15:
		st.typ = tBass
		st.bassPat = map[int]int{0: bpStyle, 5: bpSub, 6: bpOctave, 14: bpRoll, 15: bpOffbeat}[role]
	case 7:
		st.typ = t303
	case 1, 9:
		st.typ, st.phrase = tLead, map[int]int{1: 0, 9: 1}[role]
	case 2:
		st.typ = tArp
	case 8:
		st.typ = tPluck
	case 3, 10, 13, 12:
		st.typ, st.stabRhythm = tStabs, map[int]int{3: 0, 10: 1, 13: 2, 12: 3}[role]
	case 4:
		st.typ = tPad
	default:
		st.typ = tDrone
	}
	st.role = melEngine(*st)
}

// melOwn says whether the dialog's setting labeled label applies to the type and its settings (the rest are hidden).
func melOwn(label string, st melodySet) bool {
	e := melEngine(st)
	switch label {
	case "Pattern":
		return st.typ == tBass
	case "Approach":
		return st.typ == tBass && (e == 0 || e == 15)
	case "Phrase", "Leaps":
		return st.typ == tLead
	case "Direction", "Span", "Gate":
		return st.typ == tArp
	case "Hook", "Placement":
		return st.typ == tPluck
	case "Rhythm":
		return st.typ == tStabs
	case "Movement":
		return st.typ == tPad
	case "Drone note":
		return st.typ == tDrone
	case "Steps", "Pitches", "Off-key notes", "Slides", "Octave jumps", "Accent steps":
		return st.typ == t303
	case "Accents":
		return e != 7 // the 303 has its own
	case "Form":
		return e == 0 || e == 1 || e == 3 || e == 8
	case "Echo (dub chord)":
		return e == 12
	case "Ghost notes":
		return melHasGhosts(e)
	}
	return true
}

// melHasGhosts says whether the engine writes ghost notes (a chord, a drone, a sub bass, a roll do not).
func melHasGhosts(e int) bool {
	return melRoles[e].kind <= kArp && e != 5 && e != 14
}

// melTypeName is the type as a message says it: with its pattern, phrase or rhythm when it has one.
func melTypeName(st melodySet) string {
	switch {
	case st.typ == tBass && st.bassPat > 0:
		return "bass (" + melBassPats[st.bassPat] + ")"
	case st.typ == tLead && st.phrase > 0:
		return "lead (call and response)"
	case st.typ == tArp && st.arpDir > 0:
		return "arp (" + melArpDirs[st.arpDir] + ")"
	case st.typ == tStabs && st.stabRhythm > 0:
		return "stabs (" + melStabRhythms[st.stabRhythm] + ")"
	}
	return melTypes[st.typ]
}

// melArpSeq is the order an arp plays the tones (low to high) in: a step each, round again at the end. The random direction is
// rolled step by step by the caller; the chord plays them all at once.
func melArpSeq(dir int, t, scale []int, fold func(int) int, rng *rand.Rand) []int {
	n := len(t)
	rev := func(xs []int) []int {
		out := make([]int, len(xs))
		for i, x := range xs {
			out[len(xs)-1-i] = x
		}
		return out
	}
	if n < 2 {
		return t
	}
	var seq []int
	switch dir {
	case adDown:
		seq = rev(t)
	case adUpDown: // the ends once
		seq = append(append(seq, t...), rev(t[1:n-1])...)
	case adUpDownPlus: // the ends twice
		seq = append(append(seq, t...), rev(t)...)
	case adDownUp:
		seq = append(rev(t), t[1:n-1]...)
	case adOrder: // the spread chord: root, fifth, octave, tenth
		root, third, fifth := t[0], t[min(1, n-1)], t[min(2, n-1)]
		for _, p := range t {
			switch (p - root) % 12 {
			case 3, 4:
				third = p
			case 6, 7:
				fifth = p
			}
		}
		for _, p := range []int{root, fifth, root + 12, third + 12} {
			seq = append(seq, fold(p))
		}
	case adConverge: // outside in
		for i, j := 0, n-1; i <= j; i, j = i+1, j-1 {
			seq = append(seq, t[i])
			if i != j {
				seq = append(seq, t[j])
			}
		}
	case adDiverge: // inside out
		m := n / 2
		seq = append(seq, t[m])
		for d := 1; len(seq) < n; d++ {
			if m+d < n {
				seq = append(seq, t[m+d])
			}
			if m-d >= 0 {
				seq = append(seq, t[m-d])
			}
		}
	case adPinky: // the top between the others
		for _, p := range t[:n-1] {
			seq = append(seq, p, t[n-1])
		}
	case adThumb: // the bottom between the others
		for _, p := range t[1:] {
			seq = append(seq, t[0], p)
		}
	case adThirds: // 1 3 2 4 3 5: each tone, then the one two above it
		for i := 0; i+2 < n; i++ {
			seq = append(seq, t[i], t[i+2])
		}
		if len(seq) == 0 {
			seq = t
		}
	case adScale: // a run up the scale from the lowest tone to the highest
		for _, p := range scale {
			if p >= t[0] && p <= t[n-1] {
				seq = append(seq, p)
			}
		}
	default: // up (and random, chord: rolled or stacked by the caller)
		seq = t
	}
	if len(seq) == 0 {
		return t
	}
	return seq
}

var mel303Steps = []int{16, 15, 14, 12, 8, 7}
var mel303StepNames = []string{"16 (a bar)", "15", "14", "12 (in line every 3 bars)", "8 (half a bar)", "7"}
var mel303Pitches = []string{"the type's (1 to 3)", "3", "4", "5", "6", "8"}
var mel303OffKey = []string{"none", "some (b2, b5, the 7th)"}
var mel303Slides = []string{"the type's (rolled)", "none", "few", "some", "many"}
var mel303Octaves = []string{"the type's (up, rolled)", "none", "up", "down", "up and down"}
var mel303Accents = []string{"the type's (rolled)", "none", "few", "many", "every step"}

// line303 writes a 303 line: root is each bar's root (the chords transpose the pattern, as the 303's track mode does), add puts a
// note (bar, step, length in steps, pitch, velocity). A slide holds the note into the next one (a synth set to legato glides);
// accents in a row climb, as the 303's accent sweep does.
func line303(st melodySet, bars int, rng *rand.Rand, scale []int, scaleIdx func(int) int, lo, hi int, root func(int) int, add func(bar, s, l, p, v int)) {
	type pitch struct{ deg, semi, oct, slide, acc int } // deg: scale steps from the root; semi: chromatic, off the key
	n := 1 + rng.Intn(3)
	if st.s3Pitches > 0 {
		n = []int{0, 3, 4, 5, 6, 8}[st.s3Pitches]
	}
	octP, slideP := 0.06+rng.Float64()*0.38, rng.Float64()*0.4
	if st.s3Octs == 1 {
		octP = 0
	} else if st.s3Octs > 1 {
		octP = 0.3
	}
	if st.s3Slides > 0 {
		slideP = []float64{0, 0, 0.12, 0.25, 0.45}[st.s3Slides]
	}
	allAcc := rng.Intn(4) == 0
	accP := 0.08 + rng.Float64()*0.15
	switch st.s3Accs {
	case 1:
		accP, allAcc = 0, false
	case 2:
		accP, allAcc = 0.12, false
	case 3:
		accP, allAcc = 0.3, false
	case 4:
		allAcc = true
	}
	if st.accent == 0 && st.s3Accs == 0 { // Accents none, the type's: none
		accP, allAcc = 0, false
	}
	degs := []int{0} // the root first, then the third, fifth, seventh, second... of the scale
	cands := []int{2, 4, 6, 1, 3, 5}
	for len(degs) < min(n, 1+len(cands)) {
		if c := cands[rng.Intn(len(cands))]; !containsInt(degs, c) {
			degs = append(degs, c)
		}
	}
	for len(degs) < n { // 8 pitches: some again
		degs = append(degs, degs[rng.Intn(len(degs))])
	}
	row := make([]pitch, n)
	for i := range row {
		row[i].deg = degs[i]
		if i > 0 && rng.Intn(2) == 0 { // the row is not in order
			j := 1 + rng.Intn(i)
			row[i].deg, row[j].deg = row[j].deg, row[i].deg
		}
	}
	for i := range row {
		p := &row[i]
		if st.s3OffKey == 1 && i > 0 && rng.Float64() < 0.3 { // off the key: the flat 2nd, the flat 5th, the major 7th
			p.deg, p.semi = 0, []int{1, 6, 11}[rng.Intn(3)]
		}
		if rng.Float64() < octP {
			p.oct = 1
			switch {
			case st.s3Octs == 3:
				p.oct = -1
			case st.s3Octs == 4 && rng.Intn(2) == 0:
				p.oct = -1
			}
		}
		if rng.Float64() < slideP {
			p.slide = 1
		}
		if allAcc || rng.Float64() < accP {
			p.acc = 1
		}
	}
	if st.accent > 0 || st.s3Accs > 1 { // an accent at least
		acc := false
		for _, p := range row {
			acc = acc || p.acc == 1
		}
		if !acc {
			row[0].acc = 1
		}
	}
	steps := mel303Steps[st.s3Steps]
	rest, tie := []float64{0.25, 0.15, 0}[st.density], []float64{0.08, 0.08, 0}[st.density]
	time := make([]byte, steps) // N a note, T a tie, R a rest
	for i := range time {
		r := rng.Float64()
		switch {
		case i == 0 || r >= rest+tie:
			time[i] = 'N'
		case r < rest:
			time[i] = 'R'
		default:
			time[i] = 'T'
		}
	}
	type on struct{ bar, s, l, p, v int }
	var last *on
	var out []*on
	pi, prevAcc := 0, false
	for g := 0; g < bars*16; g++ {
		bar, s := g/16, g%16
		switch time[g%steps] {
		case 'R':
			last, prevAcc = nil, false
		case 'T':
			if last != nil {
				last.l++
			}
		default:
			x := row[pi%len(row)]
			pi++
			r := root(bar)
			pp := r + x.semi
			if x.semi == 0 {
				pp = scale[min(max(scaleIdx(r)+x.deg, 0), len(scale)-1)]
			}
			if x.oct == 1 && pp+12 <= hi {
				pp += 12
			} else if x.oct == -1 && pp-12 >= lo {
				pp -= 12
			}
			pp = min(max(pp, lo), hi)
			v := 88
			if x.acc == 1 {
				v = 120
				if prevAcc { // the accent sweep: accents in a row climb
					v = 127
				}
			}
			prevAcc = x.acc == 1
			last = &on{bar, s, 1 + x.slide, pp, v}
			out = append(out, last)
		}
	}
	for _, o := range out {
		add(o.bar, o.s, o.l, o.p, o.v)
	}
}
