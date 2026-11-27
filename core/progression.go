package core

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/moahbeatlab/404tui/formats"
)

// Chord progressions of the chord tool: the common ones of EDM (house, trance, progressive, future bass, deep house), in the key of the
// tool's Root (the minor ones with Root as the minor tonic). They are heard one chord a bar, and bounced at once onto a run of free pads,
// one chord a pad, so they can be played and sequenced straight away.

// progStep is one chord of a progression: semitones above the tonic and its quality (M major, m minor, d diminished, D the dominant:
// a major triad, a 7, 9 or 11 in the richer colours).
type progStep struct {
	semi int
	q    byte
}

// progressions: a name to find it again by, the feeling it is known for, the Roman numerals (small: minor) and the chords.
var progressions = []struct {
	name, mood, roman string
	steps             []progStep
}{
	{"Epic anthem", "euphoric, uplifting, a bit sad", "i VI III VII", []progStep{{0, 'm'}, {8, 'M'}, {3, 'M'}, {10, 'M'}}}, // the anthem: Am F C G
	{"Rise and fall", "driving, restless", "i VII VI VII", []progStep{{0, 'm'}, {10, 'M'}, {8, 'M'}, {10, 'M'}}},           // Am G F G
	{"Three-step climb", "hopeful, building up", "i VI VII", []progStep{{0, 'm'}, {8, 'M'}, {10, 'M'}}},                    // Am F G
	{"Trance minor", "dramatic, longing", "i iv VI V", []progStep{{0, 'm'}, {5, 'm'}, {8, 'M'}, {7, 'D'}}},                 // trance, the major V of harmonic minor: Am Dm F E
	{"Melodic house", "wistful, warm", "i iv VII III", []progStep{{0, 'm'}, {5, 'm'}, {10, 'M'}, {3, 'M'}}},                // Am Dm G C
	{"Dark progressive", "dark, brooding", "i VI iv v", []progStep{{0, 'm'}, {8, 'M'}, {5, 'm'}, {7, 'm'}}},                // Am F Dm Em
	{"Big room minor", "heroic, triumphant", "i III VII VI", []progStep{{0, 'm'}, {3, 'M'}, {10, 'M'}, {8, 'M'}}},          // Am C G F
	{"Deep two-chord", "hypnotic, moody", "i iv", []progStep{{0, 'm'}, {5, 'm'}}},                                          // deep house, two chords: Am Dm
	{"Pop anthem", "happy, euphoric", "I V vi IV", []progStep{{0, 'M'}, {7, 'D'}, {9, 'm'}, {5, 'M'}}},                     // pop and big room: C G Am F
	{"Fifties", "nostalgic, sweet", "I vi IV V", []progStep{{0, 'M'}, {9, 'm'}, {5, 'M'}, {7, 'D'}}},                       // C Am F G
	{"Royal road", "bittersweet, emotional", "IV V iii vi", []progStep{{5, 'M'}, {7, 'D'}, {4, 'm'}, {9, 'm'}}},            // future bass, "royal road": F G Em Am
	{"Heartfelt", "sad yet hopeful", "vi IV I V", []progStep{{9, 'm'}, {5, 'M'}, {0, 'M'}, {7, 'D'}}},                      // Am F C G in C major
	{"Deep house jazz", "smooth, sophisticated", "ii V I vi", []progStep{{2, 'm'}, {7, 'D'}, {0, 'M'}, {9, 'm'}}},          // deep house with sevenths: Dm7 G7 Cmaj7 Am7
	{"Tension and release", "powerful, resolving", "vi IV V I", []progStep{{9, 'm'}, {5, 'M'}, {7, 'D'}, {0, 'M'}}},        // festival and big room: Am F G C
	{"Wide open", "uplifting, open", "IV I vi V", []progStep{{5, 'M'}, {0, 'M'}, {9, 'm'}, {7, 'D'}}},                      // F C Am G
	{"Sunset chill", "chilled, laid-back", "i VII v VI", []progStep{{0, 'm'}, {10, 'M'}, {7, 'm'}, {8, 'M'}}},              // Am G Em F
	{"Dorian groove", "funky, soulful", "i IV", []progStep{{0, 'm'}, {5, 'M'}}},                                            // deep and tech house: Am D
	{"Phrygian menace", "menacing, tense", "i bII", []progStep{{0, 'm'}, {1, 'M'}}},                                        // dark techno: Am Bb
	{"Hypnotic pulse", "cold, hypnotic", "i v", []progStep{{0, 'm'}, {7, 'm'}}},                                            // techno: Am Em
	{"Andalusian descent", "dramatic, fateful", "i VII VI V", []progStep{{0, 'm'}, {10, 'M'}, {8, 'M'}, {7, 'D'}}},         // Am G F E
	{"Harmonic noir", "tense, cinematic", "i ii° V i", []progStep{{0, 'm'}, {2, 'd'}, {7, 'D'}, {0, 'm'}}},                 // Am Bdim E Am
	{"Gospel lift", "joyful, soulful", "I IV", []progStep{{0, 'M'}, {5, 'M'}}},                                             // classic house piano: C F
	{"Mixolydian anthem", "bright, rocking", "I bVII IV I", []progStep{{0, 'M'}, {10, 'M'}, {5, 'M'}, {0, 'M'}}},           // C Bb F C
	{"Neo soul turn", "smooth, romantic", "IV III vi v I", []progStep{{5, 'M'}, {4, 'D'}, {9, 'm'}, {7, 'm'}, {0, 'D'}}},   // future bass and lo-fi: Fmaj7 E7 Am7 Gm7 C7
	// the emotional EDM progressions of the MC-101 notes (brainfck.org/t/roland-mc-101, "Chords progressions")
	{"Euphoric lift", "uplifting, joyful, euphoric", "v VI VII i", []progStep{{7, 'm'}, {8, 'M'}, {10, 'M'}, {0, 'm'}}}, // D#m: A#m B C# D#m
	{"Deep and serious", "deep, serious", "VI VII i VII", []progStep{{8, 'M'}, {10, 'M'}, {0, 'm'}, {10, 'M'}}},         // Bm: G A Bm A
	{"Sad descent", "sad", "i VII iv VI", []progStep{{0, 'm'}, {10, 'M'}, {5, 'm'}, {8, 'M'}}},                          // Gm: Gm F Cm Eb
	{"Feel good", "feel good, euphoric", "VI III VII i", []progStep{{8, 'M'}, {3, 'M'}, {10, 'M'}, {0, 'm'}}},           // Bm: G D A Bm
	{"Happy-sad", "feel good but kinda sad", "VI i VII v", []progStep{{8, 'M'}, {0, 'm'}, {10, 'M'}, {7, 'm'}}},         // G#m: E G#m F# D#m
	{"Pump up", "pumping, driving", "i i VI iv", []progStep{{0, 'm'}, {0, 'm'}, {8, 'M'}, {5, 'm'}}},                    // Gm: Gm Gm Eb Cm
	{"Pump up, happy", "pumping, kinda happy", "i i III VI", []progStep{{0, 'm'}, {0, 'm'}, {3, 'M'}, {8, 'M'}}},        // Gm: Gm Gm Bb Eb
	{"Chill", "chilled, relaxed", "VI i VII i", []progStep{{8, 'M'}, {0, 'm'}, {10, 'M'}, {0, 'm'}}},                    // C#m: A C#m B C#m
	{"Happy", "happy", "IV V I I", []progStep{{5, 'M'}, {7, 'D'}, {0, 'M'}, {0, 'M'}}},                                  // C: F G C C
}

// progColors are how rich the chords of a progression are. A major chord's eleventh is raised (#11): the plain one clashes with the
// third. The diminished chord stays half-diminished above triads.
var progColors = []string{"triads", "sevenths", "ninths", "elevenths"}

// progQualities are, for each colour, the chord type (a name in chordTypes) and the name suffix of each quality.
var progQualities = []map[byte][2]string{
	{'M': {"major", ""}, 'm': {"minor", "m"}, 'd': {"dim", "dim"}, 'D': {"major", ""}},
	{'M': {"maj7", "maj7"}, 'm': {"m7", "m7"}, 'd': {"m7b5  half-dim", "m7b5"}, 'D': {"7", "7"}},
	{'M': {"maj9", "maj9"}, 'm': {"m9", "m9"}, 'd': {"m7b5  half-dim", "m7b5"}, 'D': {"9", "9"}},
	{'M': {"maj9#11", "maj9#11"}, 'm': {"m11", "m11"}, 'd': {"m7b5  half-dim", "m7b5"}, 'D': {"11", "11"}},
}

// progChord is the chord type (an index of chordTypes) and the name suffix of a step in a colour.
func progChord(q byte, color int) (int, string) {
	qs := progQualities[min(max(color, 0), len(progQualities)-1)][q]
	name, suffix := qs[0], qs[1]
	for i, c := range chordTypes {
		if c.name == name {
			return i, suffix
		}
	}
	return 0, suffix
}

// progNames is the chords of progression p over the tonic key, as names (Am F C G).
func progNames(p, key, color int) []string {
	names := formats.PitchNames
	if sharpKey(p, key) {
		names = sharpNames
	}
	var out []string
	for _, s := range progressions[p].steps {
		_, suf := progChord(s.q, color)
		out = append(out, names[(key+s.semi)%12]+suf)
	}
	return out
}

// sharpNames are the pitch classes as a key with sharps writes them.
var sharpNames = []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}

// sharpKey says whether the progression's key is written with sharps (G, D, A, E, B, F# major and their minors): A major has F#m,
// not Gbm. A progression is minor when its tonic chord (the one on 0) is minor.
func sharpKey(p, key int) bool {
	major := key
	for _, s := range progressions[p].steps {
		if s.semi == 0 && s.q == 'm' {
			major = (key + 3) % 12 // the relative major
		}
	}
	switch major {
	case 7, 2, 9, 4, 11, 6:
		return true
	}
	return false
}

// progLabels are the progressions as the menu shows them, in the key.
func progLabels(key, color int) []string {
	out := make([]string, len(progressions))
	for i, p := range progressions {
		out[i] = p.name + ": " + strings.Join(progNames(i, key, color), " ")
	}
	return out
}

// progSemis is the pitches (semitones from the sample) of each chord of the progression. Each chord's root is kept within a fifth of
// the tonic, below or above, so the chords stay close together instead of climbing through the octave.
func progSemis(o onePad) [][]int {
	var out [][]int
	shift := (o.key - o.from + 12) % 12 // from the sample's note up to the tonic
	for _, s := range progressions[o.prog].steps {
		semi := s.semi
		if semi > 6 {
			semi -= 12
		}
		base := shift + semi
		c := o
		c.scale = 0
		c.chord, _ = progChord(s.q, o.color)
		c.key = ((base%12+12)%12 + o.from) % 12 // onePadSemis starts the chord at base mod 12
		delta := base - (base%12+12)%12
		semis := onePadSemis(c)
		for i := range semis {
			semis[i] += delta
		}
		out = append(out, semis)
	}
	return out
}

// progSound is the progression played one chord a bar at the bank's tempo.
func (a *App) progSound(n int) (formats.Sample, error) {
	bar := int(4 * 60 / a.tempoOf(n/16) * formats.SampleRate)
	var all []int16
	for _, semis := range progSemis(*a.notesOf(true)) {
		ss, err := a.onePadSamples(n, semis)
		if err != nil {
			return formats.Sample{}, err
		}
		c := chordMix(ss)
		part := make([]int16, bar*2)
		copy(part, c.Samples)
		all = append(all, part...)
	}
	return formats.Sample{Channels: 2, Samples: all}, nil
}

// progHear plays the progression once, or with Hold loops it until it is asked again (or Space).
func (a *App) progHear(n int) {
	if a.arpOn {
		a.stop()
		if a.notesOf(true).progOn { // the chord looped: the progression takes its place
			a.say("stopped", false)
			return
		}
	}
	s, err := a.progSound(n)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	o := a.notesOf(true)
	names := strings.Join(progNames(o.prog, o.key, o.color), " ")
	if !o.hold {
		a.play(s)
		a.say(names+", a chord a bar", false)
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
	o.progOn = true
	a.say(names+" loops: change a setting to hear it at once, Space stops it", false)
}

// progBounce puts the chords of the progression on a run of empty pads, one chord a pad, from the pad chosen in the pad dialog; then
// the chord tool m is back.
func (a *App) progBounce(n int, m *menu) {
	o := *a.notesOf(true)
	chords := progSemis(o)
	names := progNames(o.prog, o.key, o.color)
	start := a.freeRun(len(chords), n+1)
	if start < 0 {
		a.say(fmt.Sprintf("no room: %d empty pads in a row are needed", len(chords)), true)
		a.menu = m
		return
	}
	a.pickPad(fmt.Sprintf("Put the %d chords from a pad", len(chords)), start, false, func(to int) {
		defer func() { a.menu = m }()
		for i := range chords {
			if p := to + i; p >= len(a.info.Pads) || a.info.Pads[p].File {
				a.say(fmt.Sprintf("%d empty pads in a row are needed from %s", len(chords), a.label(to)), true)
				return
			}
		}
		base := a.info.Pads[n].Name
		for i, semis := range chords {
			ss, err := a.onePadSamples(n, semis)
			if err != nil {
				a.say(err.Error(), true)
				return
			}
			mb, err := a.lab.Decode(chordMix(ss).WAV(), "wav")
			var meta struct{ ID int }
			if err != nil || json.Unmarshal(mb, &meta) != nil {
				a.say("cannot keep the chord", true)
				return
			}
			err = a.assignPad(to+i, meta.ID, fit(names[i]+"_"+base, 15)) // the chord first: Am7_Pad
			a.lab.Free(meta.ID)
			if err != nil {
				a.say(err.Error(), true)
				return
			}
			delete(a.pcm, to+i)
		}
		a.refresh()
		a.say(fmt.Sprintf("%s on %s..%s", strings.Join(names, " "), a.label(to), a.label(to+len(chords)-1)), false)
	})
	if a.picker != nil {
		a.picker.onCancel = func() { a.menu = m }
	}
}

// progMinor says whether progression p is in minor: its tonic chord (the one on 0) is minor.
func progMinor(p int) bool {
	for _, s := range progressions[p].steps {
		if s.semi == 0 {
			return s.q == 'm'
		}
	}
	return false
}

// progList is the box of every progression (A or Enter on the Progression row of the chord tool), the minor ones and the major ones,
// each with its chords in the key. The foot tells the feeling and the numerals of the one under the cursor; X (Space) hears it, A
// takes it (picked is told), B goes back to the chord tool as it was.
func (a *App) progList(n int, parent *menu, picked func()) {
	o := a.notesOf(true)
	l := &menu{title: "progressions in " + formats.PitchNames[o.key], parent: parent, footH: 3}
	at := map[int]int{} // menu row -> progression
	for _, minor := range []bool{true, false} {
		if minor {
			l.head("MINOR (ROOT IS THE MINOR KEY)")
		} else {
			l.head("MAJOR")
		}
		for i, p := range progressions {
			if progMinor(i) != minor {
				continue
			}
			i := i
			at[len(l.items)] = i
			if i == o.prog {
				l.cur = len(l.items)
			}
			l.items = append(l.items, menuItem{label: p.name, vals: []string{strings.Join(progNames(i, o.key, o.color), " ")}, keepOpen: true,
				run: func(int) {
					if a.arpOn {
						a.stop()
					}
					o.prog = i
					a.menu = parent
					picked()
					a.say(fmt.Sprintf("%s (%s): %s", p.name, p.mood, strings.Join(progNames(i, o.key, o.color), " ")), false)
				}})
		}
	}
	l.foot = func(g Gfx, col, row, cols int) {
		if i, ok := at[l.cur]; ok {
			p := progressions[i]
			g.Text(col+2, row+1, fit("Feels: "+p.mood, cols-4), cAmber, true)
			g.Text(col+2, row+2, fit(p.roman, cols-4), cDim, false)
		}
	}
	heard := -1 // the one that loops
	l.onX = func() {
		i, ok := at[l.cur]
		if !ok {
			return
		}
		if a.arpOn {
			a.stop()
			if heard == i { // Space on the one that loops stops it
				heard = -1
				return
			}
		}
		was := o.prog
		o.prog = i
		a.progHear(n)
		o.prog = was // only heard: A takes it
		heard = -1
		if a.arpOn {
			heard = i
		}
	}
	l.xHintOf = func() string { return onePadXHint(o.hold, "progression") }
	l.onClose = func() {
		if a.arpOn && o.progOn {
			a.stop()
		}
	}
	a.menu = l
}

// melProgList is the box of the Melody dialog's chords (A or Enter on its Chords row): the style's loops, the key's one chord, then
// every progression, the minor ones and the major ones, each with its chords in the melody's key. The foot tells the feeling and the
// numerals of the one under the cursor; A takes it (the trial is written again, a step of the dialog's undo), B goes back as it was.
func (a *App) melProgList(parent *menu) {
	row := -1
	for i, it := range parent.items {
		if it.label == "Chords" && !it.header {
			row = i
		}
	}
	if row < 0 {
		return
	}
	st := &a.mel
	l := &menu{title: "chords in " + sharpNames[st.key] + " " + melModes[st.mode].name, parent: parent, footH: 3}
	at := map[int]int{} // menu row -> the Chords value (0 the style's, 1 one chord, 2+ a progression)
	take := func(v int) {
		a.menu = parent
		parent.items[row].val = v
		if f := parent.items[row].onChange; f != nil {
			f(v)
		}
		a.say("chords: "+parent.items[row].vals[v], false)
	}
	add := func(v int, label, vals string) {
		at[len(l.items)] = v
		if v == st.prog {
			l.cur = len(l.items)
		}
		l.items = append(l.items, menuItem{label: label, vals: []string{vals}, keepOpen: true, run: func(int) { take(v) }})
	}
	l.head("THE STYLE'S")
	add(0, "the style's loop", melStyles[st.style]+": a roll of the dice picks one")
	add(1, "one chord", sharpNames[st.key]+", every bar")
	for _, minor := range []bool{true, false} {
		if minor {
			l.head("MINOR (KEY IS THE MINOR TONIC)")
		} else {
			l.head("MAJOR")
		}
		for i, p := range progressions {
			if progMinor(i) == minor {
				add(i+2, p.name, strings.Join(progNames(i, st.key, 0), " "))
			}
		}
	}
	l.foot = func(g Gfx, col, row, cols int) {
		v, ok := at[l.cur]
		if !ok {
			return
		}
		mood, roman := "the style's own loops (Style); NOW shows each bar's chord", "a roll of the dice can take another of them"
		switch {
		case v == 1:
			mood, roman = "still: only the key's own chord", "i (or I)"
		case v >= 2:
			mood, roman = "Feels: "+progressions[v-2].mood, progressions[v-2].roman+"   (A: take it, B: back)"
		}
		g.Text(col+2, row+1, fit(mood, cols-4), cAmber, true)
		g.Text(col+2, row+2, fit(roman, cols-4), cDim, false)
	}
	a.menu = l
}
