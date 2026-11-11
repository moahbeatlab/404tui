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
	var out []string
	for _, s := range progressions[p].steps {
		_, suf := progChord(s.q, color)
		out = append(out, formats.PitchNames[(key+s.semi)%12]+suf)
	}
	return out
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
