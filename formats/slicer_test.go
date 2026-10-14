package formats

import (
	"math"
	"slices"
	"testing"
)

// ramp is a mono sample whose value says where in it a frame is, so a cut can be checked.
func ramp(frames int) Sample {
	s := Sample{Channels: 1, Samples: make([]int16, frames)}
	for i := range s.Samples {
		s.Samples[i] = int16(i%30000 + 1)
	}
	return s
}

func TestDivideCoversSample(t *testing.T) {
	s := NewSlicing(48000, 8)
	if len(s.Slices) != 8 || !slices.Equal(s.Order, []int{0, 1, 2, 3, 4, 5, 6, 7}) {
		t.Fatalf("slices %d, order %v", len(s.Slices), s.Order)
	}
	total := 0
	for i := range s.Slices {
		total += s.Length(i)
	}
	if total != 48000 || s.Slices[0].Start != 0 {
		t.Fatalf("slices cover %d frames from %d", total, s.Slices[0].Start)
	}
	if got := NewSlicing(100, 16); len(got.Slices) != 1 {
		t.Errorf("a sample too short for slices got %d", len(got.Slices))
	}
}

func TestMoveStartStopsAtNeighbours(t *testing.T) {
	s := NewSlicing(4000, 4) // starts 0 1000 2000 3000
	if got := s.MoveStart(1, 300); got != 1300 {
		t.Fatalf("moved to %d", got)
	}
	if s.End(0) != 1300 || s.Length(1) != 700 {
		t.Errorf("the slice before did not follow: ends %d, next is %d long", s.End(0), s.Length(1))
	}
	if got := s.MoveStart(1, 5000); got != 2000-MinSliceFrames {
		t.Errorf("pushed onto the next slice: %d", got)
	}
	if got := s.MoveStart(1, -5000); got != MinSliceFrames {
		t.Errorf("pushed onto the one before: %d", got)
	}
	// The first slice can leave out what is before it.
	s = NewSlicing(4000, 4)
	if got := s.MoveStart(0, 100); got != 100 {
		t.Errorf("first start %d", got)
	}
	if got := s.MoveStart(0, -500); got != 0 {
		t.Errorf("first start went before the sample: %d", got)
	}
}

func TestSplitKeepsOrderSounding(t *testing.T) {
	s := NewSlicing(4000, 4)
	s.Order = []int{3, 1, 1, 0}
	s.Slices[1].Reverse = true
	n := s.Split(1500)
	if n != 2 || len(s.Slices) != 5 || s.Slices[2].Start != 1500 {
		t.Fatalf("split gave %d, slices %+v", n, s.Slices)
	}
	if !s.Slices[2].Reverse {
		t.Error("the new slice lost the settings of the one it was cut from")
	}
	// Slice 3 is now 4; the cut piece plays right after each play of its parent.
	if want := []int{4, 1, 2, 1, 2, 0}; !slices.Equal(s.Order, want) {
		t.Errorf("order %v, want %v", s.Order, want)
	}
	if s.Split(1500+10) != -1 || s.Split(0) != -1 {
		t.Error("a cut leaving a piece that is too short was made")
	}
}

func TestRemove(t *testing.T) {
	s := NewSlicing(4000, 4)
	s.Order = []int{3, 1, 2, 0}
	if !s.Remove(1) {
		t.Fatal("not removed")
	}
	if len(s.Slices) != 3 || s.End(0) != 2000 {
		t.Errorf("the slice before did not grow: %+v", s.Slices)
	}
	if want := []int{2, 1, 0}; !slices.Equal(s.Order, want) {
		t.Errorf("order %v, want %v", s.Order, want)
	}
	one := NewSlicing(100, 1)
	if one.Remove(0) {
		t.Error("the last slice was removed")
	}
}

func TestSetStartsAndSliceAt(t *testing.T) {
	s := &Slicing{Frames: 10000}
	s.SetStarts([]int{5000, 100, 5010, 20000, -4, 2500})
	var starts []int
	for _, sl := range s.Slices {
		starts = append(starts, sl.Start)
	}
	if want := []int{0, 100, 2500, 5000}; !slices.Equal(starts, want) {
		t.Fatalf("starts %v, want %v", starts, want)
	}
	for frame, want := range map[int]int{0: 0, 99: 0, 100: 1, 4999: 2, 5000: 3, 9999: 3} {
		if got := s.SliceAt(frame); got != want {
			t.Errorf("SliceAt(%d) = %d, want %d", frame, got, want)
		}
	}
}

func TestValidateMendsAFile(t *testing.T) {
	s := &Slicing{Frames: 1000, Order: []int{9, -1},
		Slices: []Slice{{Start: 500, Chance: 900, Gain: -4, Pitch: 99}, {Start: 5000}, {Start: 0}, {Start: 3}}}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(s.Slices) != 2 || s.Slices[0].Start != 0 || s.Slices[1].Start != 500 {
		t.Fatalf("slices %+v", s.Slices)
	}
	if got := s.Slices[1]; got.Chance != 100 || got.Gain != 0 || got.Pitch != 24 {
		t.Errorf("not clamped: %+v", got)
	}
	if !slices.Equal(s.Order, []int{0, 1}) {
		t.Errorf("order %v", s.Order)
	}
	if (&Slicing{}).Validate() == nil {
		t.Error("a slicing with no frames is fine")
	}
}

func take(q *Sequencer, n int) []int {
	var out []int
	for i := 0; i < n; i++ {
		out = append(out, q.Next().Slice)
	}
	return out
}

func TestOrderMode(t *testing.T) {
	s := NewSlicing(4000, 4)
	s.Order = []int{2, 0, 0, 3}
	if got := take(NewSequencer(s), 9); !slices.Equal(got, []int{2, 0, 0, 3, 2, 0, 0, 3, 2}) {
		t.Errorf("played %v", got)
	}
}

func TestOrderChanceMakesRests(t *testing.T) {
	s := NewSlicing(4000, 2)
	s.Slices[1].Chance = 0
	q := NewSequencer(s)
	for i := 0; i < 20; i++ {
		step := q.Next()
		if step.Rest != (step.Slice == 1) {
			t.Fatalf("step %d: %+v", i, step)
		}
	}
	s.Slices[1].Chance = 50
	rests := 0
	q = NewSequencer(s)
	for i := 0; i < 2000; i++ {
		if step := q.Next(); step.Slice == 1 && step.Rest {
			rests++
		}
	}
	if rests < 400 || rests > 600 { // 1000 plays of slice 1 at 50 %
		t.Errorf("%d rests of 1000", rests)
	}
}

func TestRandomIsRepeatableAndWeighted(t *testing.T) {
	s := NewSlicing(4000, 4)
	s.Mode, s.Seed = ModeRandom, 7
	s.Slices[2].Chance = 0
	a, b := take(NewSequencer(s), 200), take(NewSequencer(s), 200)
	if !slices.Equal(a, b) {
		t.Error("the same seed gave different picks")
	}
	if slices.Contains(a, 2) {
		t.Error("a slice with chance 0 was picked")
	}
	s.Seed = 8
	if slices.Equal(a, take(NewSequencer(s), 200)) {
		t.Error("another seed gave the same picks")
	}
	// Weights: slice 0 five times as likely as 1 and 3 (100 of 140).
	s.Slices[0].Chance, s.Slices[1].Chance, s.Slices[3].Chance = 100, 20, 20
	count := map[int]int{}
	for _, i := range take(NewSequencer(s), 6000) {
		count[i]++
	}
	if count[0] < 4000 || count[0] > 4600 {
		t.Errorf("slice 0 picked %d of 6000 (about 4286 expected): %v", count[0], count)
	}
}

func TestNoRepeat(t *testing.T) {
	s := NewSlicing(4000, 3)
	s.Mode = ModeNoRepeat
	got := take(NewSequencer(s), 500)
	for i := 1; i < len(got); i++ {
		if got[i] == got[i-1] {
			t.Fatalf("slice %d twice in a row at %d", got[i], i)
		}
	}
	// With a single slice there is nothing else to play.
	one := NewSlicing(4000, 1)
	one.Mode = ModeNoRepeat
	if got := take(NewSequencer(one), 3); !slices.Equal(got, []int{0, 0, 0}) {
		t.Errorf("one slice played %v", got)
	}
}

func TestShuffleEveryRound(t *testing.T) {
	s := NewSlicing(8000, 8)
	s.Mode, s.Seed = ModeShuffle, 3
	s.Slices[5].Chance = 0
	got := take(NewSequencer(s), 7*20)
	for r := 0; r < 20; r++ {
		round := slices.Clone(got[r*7 : (r+1)*7])
		slices.Sort(round)
		if !slices.Equal(round, []int{0, 1, 2, 3, 4, 6, 7}) {
			t.Fatalf("round %d played %v", r, round)
		}
		if r > 0 && got[r*7] == got[r*7-1] {
			t.Errorf("round %d started on the slice the last one ended on", r)
		}
	}
}

func TestSliceAudio(t *testing.T) {
	src := ramp(4000)
	s := NewSlicing(4000, 4)
	plain := SliceAudio(src, s, 1)
	if plain.Frames() != 1000 {
		t.Fatalf("%d frames", plain.Frames())
	}
	// Faded at the ends, untouched in the middle.
	if plain.Samples[0] != 0 || plain.Samples[500] != src.Samples[1500] {
		t.Errorf("ends %d, middle %d (want %d)", plain.Samples[0], plain.Samples[500], src.Samples[1500])
	}
	s.Slices[1].Reverse = true
	rev := SliceAudio(src, s, 1)
	if rev.Samples[500] != src.Samples[1000+(999-500)] {
		t.Errorf("reversed middle %d", rev.Samples[500])
	}
	s.Slices[1].Reverse, s.Slices[1].Gain = false, 50
	if half := SliceAudio(src, s, 1); half.Samples[500] != int16(math.Round(float64(src.Samples[1500])/2)) {
		t.Errorf("half gain %d", half.Samples[500])
	}
	s.Slices[1].Gain, s.Slices[1].Pitch = 100, 12
	if up := SliceAudio(src, s, 1); up.Frames() != 500 {
		t.Errorf("an octave up is %d frames, want 500", up.Frames())
	}
}

func TestRender(t *testing.T) {
	src := ramp(4000)
	s := NewSlicing(4000, 4)
	s.Order = []int{3, 0}
	s.Slices[0].Chance = 0
	out, placed := Render(src, s, NewSequencer(s), 4)
	if out.Frames() != 4000 || len(placed) != 4 {
		t.Fatalf("%d frames, %d placed", out.Frames(), len(placed))
	}
	if placed[1].At != 1000 || placed[1].Slice != 0 || !placed[1].Rest || placed[2].At != 2000 {
		t.Errorf("placed %+v", placed)
	}
	for _, v := range out.Samples[1000:2000] {
		if v != 0 {
			t.Fatal("a rest is not silent")
		}
	}
	if out.Samples[500] != src.Samples[3500] {
		t.Error("the first step is not slice 3")
	}
}

func TestGridStepsHaveTheLengthOfANote(t *testing.T) {
	src := ramp(48000)
	s := NewSlicing(48000, 4)   // slices of 12000 frames
	s.Division, s.BPM = 16, 120 // a sixteenth is 6000 frames
	if s.StepFrames() != 6000 {
		t.Fatalf("step %d", s.StepFrames())
	}
	out, placed := Render(src, s, NewSequencer(s), 4)
	if out.Frames() != 24000 || placed[1].At != 6000 || placed[1].Frames != 6000 || placed[1].Audio != 6000 {
		t.Fatalf("%d frames, placed %+v", out.Frames(), placed)
	}
	// A slice longer than the step is cut off with a fade; the next starts on the beat.
	if out.Samples[6000+3000] != SliceAudio(src, s, 1).Samples[3000] {
		t.Error("the middle of the step is not the slice")
	}
	if last := out.Samples[6000+5999]; last != 0 {
		t.Errorf("the cut was not faded out: %d", last)
	}
	// A slice shorter than the step leaves silence after it.
	s.Division = 4 // a quarter is 24000 frames
	out, placed = Render(src, s, NewSequencer(s), 1)
	if out.Frames() != 24000 || placed[0].Audio != 12000 {
		t.Fatalf("%d frames, audio %d", out.Frames(), placed[0].Audio)
	}
	for _, v := range out.Samples[12000:] {
		if v != 0 {
			t.Fatal("the rest of the step is not silent")
		}
	}
	s.Division = 0
	if s.StepFrames() != 0 {
		t.Error("no grid still has a step")
	}
	s.Division, s.BPM = 7, 9000
	_ = s.Validate()
	if s.Division != 0 || s.BPM != 300 {
		t.Errorf("division %d, bpm %g", s.Division, s.BPM)
	}
}

func TestPeaks(t *testing.T) {
	s := Sample{Channels: 2, Samples: []int16{0, 100, 0, -300, 50, 0, 0, 0}}
	if got := s.Peaks(0, 4, 2); !slices.Equal(got, []int{300, 50}) {
		t.Errorf("peaks %v", got)
	}
	if got := s.Peaks(3, 3, 4); !slices.Equal(got, []int{0, 0, 0, 0}) {
		t.Errorf("empty range %v", got)
	}
}

func TestZeroCrossing(t *testing.T) {
	s := Sample{Channels: 1, Samples: []int16{5, 4, 3, 2, 1, -1, -2, -3, -4, -5}}
	if got := s.ZeroCrossing(2, 5); got != 5 {
		t.Errorf("crossing at %d, want 5", got)
	}
	if got := s.ZeroCrossing(2, 1); got != 2 {
		t.Errorf("out of reach: %d", got)
	}
}

// bursts is silence with a decaying noise burst starting at each of the given frames.
func bursts(frames int, at ...int) Sample {
	s := Sample{Channels: 1, Samples: make([]int16, frames)}
	for _, start := range at {
		for i := 0; i < 6000 && start+i < frames; i++ {
			amp := 20000 * math.Exp(-float64(i)/1500)
			s.Samples[start+i] = int16(amp * math.Sin(float64(i)*0.9))
		}
	}
	return s
}

func TestTransients(t *testing.T) {
	want := []int{4800, 19200, 33600, 40000}
	src := bursts(48000, want...)
	got := src.Transients(16, 0.5)
	if len(got) != len(want) {
		t.Fatalf("found %v, want near %v", got, want)
	}
	for i, f := range got {
		if f > want[i] || want[i]-f > 1024 {
			t.Errorf("onset %d at %d, want just before %d", i, f, want[i])
		}
	}
	if got := src.Transients(2, 0.5); len(got) != 2 {
		t.Errorf("asked for 2, got %v", got)
	}
	if got := (Sample{Channels: 1, Samples: make([]int16, 48000)}).Transients(8, 1); len(got) != 0 {
		t.Errorf("silence has onsets: %v", got)
	}
}
