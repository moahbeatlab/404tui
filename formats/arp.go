package formats

import (
	"math"
	"math/rand"
)

// Chords and arps made from one sample: the sample is pitched the way a sampler's chromatic mode does it (by changing
// the speed, see Pitched), once per note, and the notes are mixed into a chord or laid out in time as an arp. The result is
// a new sample for a pad. The notes are pitch classes (0 = C ... 11 = B) and sit relative to the key the sample is tuned to,
// so a sample in F that is given a C chord sounds a C chord.

// Arp patterns. ArpChord is one chord held for as long as the sample is; the rest play a step each, at the rate.
const (
	ArpChord = iota
	ArpUp
	ArpDown
	ArpUpDown
	ArpDownUp
	ArpRandom
	ArpOrder
	ArpGliss
	ArpThirds
	ArpPedal
	ArpStabs
)

// ArpDirs names the patterns, in the order of the constants.
var ArpDirs = []string{"CHORD", "UP", "DOWN", "UP-DN", "DN-UP", "RANDOM", "ORDER", "GLISS", "THIRDS", "PEDAL", "STABS"}

// ArpDirNotes says in a few words what each pattern plays.
var ArpDirNotes = []string{"one chord, held", "low to high", "high to low", "up and back", "down and back", "random", "as you entered the notes",
	"a chromatic run", "1 3 2 4 3 5 ...", "the root between the notes", "the whole chord on every step"}

// ArpRate is how many steps a beat has.
type ArpRate struct {
	Name    string
	PerBeat int
}

// ArpRates are the rates of the arp.
var ArpRates = []ArpRate{{"1/4", 1}, {"1/8", 2}, {"1/8T", 3}, {"1/16", 4}, {"1/16T", 6}, {"1/32", 8}}

// ArpGates are the length of a note as a part of its step, in percent; 0 lets the note ring to its end.
var ArpGates = []int{25, 50, 75, 100, 0}

// ArpSwings delay every second step by this part of a step.
var ArpSwings = []struct {
	Name string
	Part float64
}{{"OFF", 0}, {"LIGHT", 1.0 / 6}, {"TRIPLET", 1.0 / 3}}

// ArpScales are the scales the chords can be built on.
var ArpScales = []struct {
	Name string
	Iv   []int
}{
	{"MAJ", []int{0, 2, 4, 5, 7, 9, 11}}, {"MIN", []int{0, 2, 3, 5, 7, 8, 10}}, {"MIN-P", []int{0, 3, 5, 7, 10}},
	{"MAJ-P", []int{0, 2, 4, 7, 9}}, {"DOR", []int{0, 2, 3, 5, 7, 9, 10}}, {"MIXO", []int{0, 2, 4, 5, 7, 9, 10}},
	{"BLUES", []int{0, 3, 5, 6, 7, 10}},
}

// ArpChords are the chord types: the semitones above the root.
var ArpChords = []struct {
	Name string
	Iv   []int
}{
	{"MAJ", []int{0, 4, 7}}, {"MIN", []int{0, 3, 7}}, {"7", []int{0, 4, 7, 10}}, {"MAJ7", []int{0, 4, 7, 11}}, {"MIN7", []int{0, 3, 7, 10}},
	{"SUS2", []int{0, 2, 7}}, {"SUS4", []int{0, 5, 7}}, {"DIM", []int{0, 3, 6}}, {"AUG", []int{0, 4, 8}}, {"5", []int{0, 7}},
	{"6", []int{0, 4, 7, 9}}, {"9", []int{0, 4, 7, 10, 14}},
}

// PitchNames name the pitch classes.
var PitchNames = []string{"C", "Db", "D", "Eb", "E", "F", "Gb", "G", "Ab", "A", "Bb", "B"}

// Arp is what to make: the notes and how to play them.
type Arp struct {
	// Key is the pitch class the sample is tuned to.
	Key int
	// Root is the lowest note of the chord, as a pitch class.
	Root int
	// Notes are the pitch classes, in the order they were entered.
	Notes []int
	// Dir is the pattern, one of the Arp constants.
	Dir int
	// Octaves is how many octaves the notes are stacked over, 1 to 3; Shift moves them up or down by octaves.
	Octaves, Shift int
	// Rate, Gate and Swing index ArpRates, ArpGates and ArpSwings.
	Rate, Gate, Swing int
	// BPM and Bars give the length of an arp: Bars bars of four beats.
	BPM  float64
	Bars int
	// Seed makes the RANDOM pattern the same each time.
	Seed int64
}

// DefaultArp is a C major chord arp over two octaves, at the sixteenths.
func DefaultArp() Arp {
	return Arp{Notes: ChordNotes(0, -1, 0, 0), Dir: ArpUp, Octaves: 2, Rate: 3, Gate: 1, BPM: 120, Bars: 1}
}

// ChordNotes gives the pitch classes of a chord. With scale < 0 it is the chord type (an index of ArpChords) on the root.
// With a scale it is built on the scale: degree 0 is the whole scale, 1 to 7 the triad on that degree, 8 to 14 the seventh
// chord on degree 1 to 7.
func ChordNotes(root, scale, chord, degree int) []int {
	var out []int
	add := func(p int) {
		p = ((p % 12) + 12) % 12
		for _, q := range out {
			if q == p {
				return
			}
		}
		out = append(out, p)
	}
	if scale < 0 || scale >= len(ArpScales) {
		for _, i := range ArpChords[min(max(chord, 0), len(ArpChords)-1)].Iv {
			add(root + i)
		}
		return out
	}
	iv := ArpScales[scale].Iv
	at := func(d int) int { return root + iv[d%len(iv)] + 12*(d/len(iv)) }
	if degree <= 0 {
		for _, i := range iv {
			add(root + i)
		}
		return out
	}
	k := (degree - 1) % 7
	picks := []int{k, k + 2, k + 4}
	if degree > 7 {
		picks = append(picks, k+6)
	}
	for _, d := range picks {
		add(at(d))
	}
	return out
}

// Offsets are the notes as semitones from the sample's own pitch, in the order they were entered, stacked over the octaves,
// and folded back to two octaves either way of the sample (a note further away would be a click or a whole minute).
func (a Arp) Offsets() []int {
	// The lowest note sits within half an octave of the sample's own pitch.
	d0 := ((a.Root-a.Key+18)%12+12)%12 - 6
	var out []int
	seen := map[int]bool{}
	for o := 0; o < max(a.Octaves, 1); o++ {
		for _, pc := range a.Notes {
			d := d0 + ((pc-a.Root)%12+12)%12 + 12*o + 12*a.Shift
			for d > 24 {
				d -= 12
			}
			for d < -24 {
				d += 12
			}
			if !seen[d] {
				seen[d] = true
				out = append(out, d)
			}
		}
	}
	return out
}

// sequence is what each step plays: a list of semitones, so a step can be a chord.
func (a Arp) sequence(steps int) [][]int {
	ord := a.Offsets()
	asc := append([]int(nil), ord...)
	sortInts(asc)
	desc := make([]int, len(asc))
	for i, v := range asc {
		desc[len(asc)-1-i] = v
	}
	bounce := func(l []int) []int {
		out := append([]int(nil), l...)
		for i := len(l) - 2; i >= 1; i-- {
			out = append(out, l[i])
		}
		return out
	}
	var cycle []int
	switch a.Dir {
	case ArpDown:
		cycle = desc
	case ArpUpDown:
		cycle = bounce(asc)
	case ArpDownUp:
		cycle = bounce(desc)
	case ArpOrder:
		cycle = ord
	case ArpGliss:
		if len(asc) < 2 {
			cycle = asc
			break
		}
		var run []int
		for n := asc[0]; n <= asc[len(asc)-1] && len(run) < 49; n++ {
			run = append(run, n)
		}
		cycle = bounce(run)
	case ArpThirds:
		if len(asc) < 3 {
			cycle = asc
			break
		}
		for i := range asc {
			cycle = append(cycle, asc[i], asc[(i+2)%len(asc)])
		}
	case ArpPedal:
		if len(asc) < 2 {
			cycle = asc
			break
		}
		for _, n := range asc[1:] {
			cycle = append(cycle, asc[0], n)
		}
	default:
		cycle = asc
	}
	out := make([][]int, steps)
	if len(cycle) == 0 {
		return out
	}
	rng := rand.New(rand.NewSource(a.Seed))
	for i := range out {
		switch a.Dir {
		case ArpStabs:
			out[i] = asc
		case ArpRandom:
			out[i] = []int{asc[rng.Intn(len(asc))]}
		default:
			out[i] = []int{cycle[i%len(cycle)]}
		}
	}
	return out
}

// Steps is how many steps an arp has.
func (a Arp) Steps() int {
	return max(a.Bars, 1) * 4 * ArpRates[min(max(a.Rate, 0), len(ArpRates)-1)].PerBeat
}

// RenderArp makes the chord or the arp from a sample. An arp loops: what rings on past the end wraps round to the start, so
// the sample can be a pad that loops. It is empty when there are no notes or the sample is.
func RenderArp(src Sample, a Arp) Sample {
	ch := max(src.Channels, 1)
	offs := a.Offsets()
	if len(offs) == 0 || src.Frames() < 2 {
		return Sample{}
	}
	pitched := map[int]Sample{}
	get := func(semi int) Sample {
		if p, ok := pitched[semi]; ok {
			return p
		}
		p := src.Pitched(semi)
		pitched[semi] = p
		return p
	}
	if a.Dir == ArpChord {
		length := 0
		for _, o := range offs {
			length = max(length, get(o).Frames())
		}
		mix := make([]float64, length*ch)
		for _, o := range offs {
			addInto(mix, get(o), 0, get(o).Frames(), ch, 0, false)
		}
		return limitMix(mix, ch)
	}
	per := ArpRates[min(max(a.Rate, 0), len(ArpRates)-1)].PerBeat
	bpm := a.BPM
	if bpm < 20 {
		bpm = 120
	}
	stepF := SampleRate * 60 / bpm / float64(per)
	steps := a.Steps()
	total := int(math.Round(float64(steps) * stepF))
	if total < 1 {
		return Sample{}
	}
	mix := make([]float64, total*ch)
	gate := ArpGates[min(max(a.Gate, 0), len(ArpGates)-1)]
	swing := ArpSwings[min(max(a.Swing, 0), len(ArpSwings)-1)].Part
	for i, notes := range a.sequence(steps) {
		start := float64(i) * stepF
		if i%2 == 1 {
			start += stepF * swing
		}
		for _, n := range notes {
			p := get(n)
			length := p.Frames()
			cut := false
			if gate > 0 {
				if g := int(stepF * float64(gate) / 100); g < length {
					length, cut = max(g, 16), true
				}
			}
			addInto(mix, p, int(math.Round(start)), length, ch, total, cut)
		}
	}
	return limitMix(mix, ch)
}

// addInto adds the first length frames of s to mix from frame at on. With wrap > 0 it goes round to the start at wrap
// frames; cut fades the end of the part (a note stopped by its gate).
func addInto(mix []float64, s Sample, at, length, ch, wrap int, cut bool) {
	fade := min(length, int(0.008*SampleRate))
	frames := len(mix) / ch
	for f := 0; f < length; f++ {
		pos := at + f
		if wrap > 0 {
			pos %= wrap
		}
		if pos >= frames {
			break
		}
		g := 1.0
		if cut && f >= length-fade {
			g = float64(length-f) / float64(fade)
		}
		for c := 0; c < ch; c++ {
			mix[pos*ch+c] += float64(s.Samples[f*ch+c]) * g
		}
	}
}

// limitMix turns a mix into a sample, turned down when the sum is louder than a sample can be.
func limitMix(mix []float64, ch int) Sample {
	peak := 0.0
	for _, v := range mix {
		peak = max(peak, math.Abs(v))
	}
	gain := 1.0
	if limit := 0.97 * math.MaxInt16; peak > limit {
		gain = limit / peak
	}
	out := make([]int16, len(mix))
	for i, v := range mix {
		out[i] = int16(math.Round(v * gain))
	}
	return Sample{Channels: ch, Samples: out}
}

func sortInts(a []int) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}
