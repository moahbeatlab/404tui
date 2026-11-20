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
	case "Form":
		return e == 0 || e == 1 || e == 3 || e == 7 || e == 8
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
