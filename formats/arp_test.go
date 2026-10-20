package formats

import (
	"math"
	"slices"
	"testing"
)

// arpSine is a mono sample of a sine wave at freq, about a second long.
func arpSine(freq float64, frames int) Sample {
	s := make([]int16, frames)
	for i := range s {
		s[i] = int16(12000 * math.Sin(2*math.Pi*freq*float64(i)/SampleRate))
	}
	return Sample{Channels: 1, Samples: s}
}

func TestChordNotes(t *testing.T) {
	if got := ChordNotes(0, -1, 0, 0); !slices.Equal(got, []int{0, 4, 7}) {
		t.Errorf("C major = %v", got)
	}
	if got := ChordNotes(9, -1, 4, 0); !slices.Equal(got, []int{9, 0, 4, 7}) {
		t.Errorf("A min7 = %v", got)
	}
	// A minor scale: degree 1 triad is A C E, degree 1 seventh chord adds G.
	if got := ChordNotes(9, 1, 0, 1); !slices.Equal(got, []int{9, 0, 4}) {
		t.Errorf("Am triad on degree 1 = %v", got)
	}
	if got := ChordNotes(9, 1, 0, 8); !slices.Equal(got, []int{9, 0, 4, 7}) {
		t.Errorf("Am7 on degree 1 = %v", got)
	}
	if got := ChordNotes(9, 1, 0, 0); !slices.Equal(got, []int{9, 11, 0, 2, 4, 5, 7}) {
		t.Errorf("A minor scale = %v", got)
	}
	if got := ChordNotes(0, 0, 0, 5); !slices.Equal(got, []int{7, 11, 2}) {
		t.Errorf("triad on the fifth of C major (G B D) = %v", got)
	}
}

func TestOffsetsFollowTheSampleKey(t *testing.T) {
	a := DefaultArp()
	a.Octaves = 1
	// A sample in C: C E G are 0, 4, 7 semitones from its own pitch.
	if got := a.Offsets(); !slices.Equal(got, []int{0, 4, 7}) {
		t.Errorf("sample in C: %v", got)
	}
	// A sample in F sounding a C chord: C is the F five semitones down, E four above that, G seven.
	a.Key = 5
	if got := a.Offsets(); !slices.Equal(got, []int{-5, -1, 2}) {
		t.Errorf("sample in F: %v", got)
	}
	a.Octaves, a.Shift = 2, 1
	if len(a.Offsets()) == 0 {
		t.Error("shifted: no notes")
	}
	for _, o := range a.Offsets() {
		if o < -24 || o > 24 {
			t.Errorf("offset %d out of range", o)
		}
	}
}

func TestArpSequences(t *testing.T) {
	a := DefaultArp()
	a.Octaves = 1
	steps := func(dir int, n int) []int {
		a.Dir = dir
		var out []int
		for _, s := range a.sequence(n) {
			out = append(out, s[0])
		}
		return out
	}
	if got := steps(ArpUp, 4); !slices.Equal(got, []int{0, 4, 7, 0}) {
		t.Errorf("up %v", got)
	}
	if got := steps(ArpDown, 4); !slices.Equal(got, []int{7, 4, 0, 7}) {
		t.Errorf("down %v", got)
	}
	if got := steps(ArpUpDown, 6); !slices.Equal(got, []int{0, 4, 7, 4, 0, 4}) {
		t.Errorf("up-down %v", got)
	}
	if got := steps(ArpPedal, 4); !slices.Equal(got, []int{0, 4, 0, 7}) {
		t.Errorf("pedal %v", got)
	}
	if got := steps(ArpThirds, 6); !slices.Equal(got, []int{0, 7, 4, 0, 7, 4}) {
		t.Errorf("thirds %v", got)
	}
	a.Dir = ArpStabs
	if s := a.sequence(2); len(s[0]) != 3 {
		t.Errorf("stabs play the whole chord: %v", s)
	}
	a.Dir, a.Seed = ArpRandom, 7
	if !slices.Equal(steps(ArpRandom, 8), steps(ArpRandom, 8)) {
		t.Error("random with a seed must repeat")
	}
}

func TestRenderArpLengthAndLevel(t *testing.T) {
	src := arpSine(220, SampleRate/2)
	a := DefaultArp()
	got := RenderArp(src, a)
	// One bar of sixteenths at 120 BPM is two seconds.
	if want := 2 * SampleRate; math.Abs(float64(got.Frames()-want)) > 1 {
		t.Errorf("one bar at 120 BPM = %d frames, want %d", got.Frames(), want)
	}
	a.Bars, a.BPM, a.Rate = 2, 90, 0
	if got := RenderArp(src, a); math.Abs(float64(got.Frames())-8*SampleRate*60/90) > 1 {
		t.Errorf("2 bars at 90 BPM: %d frames", got.Frames())
	}
	a = DefaultArp()
	a.Dir, a.Octaves = ArpStabs, 3
	loud := RenderArp(arpSine(220, 20000), a)
	peak := 0
	for _, v := range loud.Samples {
		peak = max(peak, int(math.Abs(float64(v))))
	}
	if peak > math.MaxInt16 {
		t.Errorf("clipped: %d", peak)
	}
	if peak == 0 {
		t.Error("silent")
	}
}

func TestRenderChordHoldsTheLongestNote(t *testing.T) {
	src := arpSine(220, 24000)
	a := DefaultArp()
	a.Dir, a.Octaves = ArpChord, 1
	got := RenderArp(src, a)
	if got.Frames() != src.Frames() {
		t.Errorf("chord of 0, 4, 7 semitones is %d frames; the unpitched sample has %d", got.Frames(), src.Frames())
	}
	// The chord has the three pitches: the strongest bins of its spectrum are at 220 Hz and the two notes above it.
	if got := RenderArp(Sample{}, a); got.Frames() != 0 {
		t.Error("an empty sample makes an empty chord")
	}
	a.Notes = nil
	if got := RenderArp(src, a); got.Frames() != 0 {
		t.Error("no notes make nothing")
	}
}

func TestArpLoopsSeamlessly(t *testing.T) {
	// A long note near the end wraps to the start: the first frames are not silent even though the first step has a short gate.
	src := arpSine(110, SampleRate)
	a := DefaultArp()
	a.Gate, a.Rate, a.Bars = 4, 3, 1 // open gate
	got := RenderArp(src, a)
	if got.Frames() == 0 {
		t.Fatal("empty")
	}
	tail := got.Samples[got.Frames()-1]
	head := got.Samples[0]
	if math.Abs(float64(tail)-float64(head)) > 20000 {
		t.Errorf("a jump from %d to %d at the loop point", tail, head)
	}
}
