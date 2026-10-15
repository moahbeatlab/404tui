package formats

import (
	"math"
	"math/rand"
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

func TestEdgesAreIndependent(t *testing.T) {
	s := NewSlicing(4000, 4) // 0-1000 1000-2000 2000-3000 3000-4000
	if got := s.MoveStart(1, 300); got != 1300 {
		t.Fatalf("moved to %d", got)
	}
	if s.Slices[0].End != 1000 || s.Slices[1].End != 2000 {
		t.Errorf("a neighbour followed: %+v", s.Slices[:2])
	}
	// A start can go past a neighbour's, so slices overlap, and leave a gap behind it.
	if got := s.MoveStart(1, -5000); got != 0 {
		t.Errorf("start went before the sample: %d", got)
	}
	if got := s.MoveEnd(0, -400); got != 600 {
		t.Errorf("end %d", got)
	}
	if s.Slices[1].Start != 0 || s.Length(0) != 600 {
		t.Errorf("slices %+v", s.Slices[:2])
	}
	// Stopped at the slice's own other edge.
	if got := s.MoveStart(2, 9000); got != 3000-MinSliceFrames {
		t.Errorf("start passed the end: %d", got)
	}
	if got := s.MoveEnd(2, -9000); got != s.Slices[2].Start+MinSliceFrames {
		t.Errorf("end passed the start: %d", got)
	}
	if got := s.MoveEnd(3, 9000); got != 4000 {
		t.Errorf("end went past the sample: %d", got)
	}
}

func TestSplitKeepsOrderSounding(t *testing.T) {
	s := NewSlicing(4000, 4)
	s.Order = []int{3, 1, 1, 0}
	s.Slices[1].Reverse = true
	n := s.Split(1, 1500)
	if n != 2 || len(s.Slices) != 5 || s.Slices[2].Start != 1500 || s.Slices[2].End != 2000 || s.Slices[1].End != 1500 {
		t.Fatalf("split gave %d, slices %+v", n, s.Slices)
	}
	if !s.Slices[2].Reverse {
		t.Error("the new slice lost the settings of the one it was cut from")
	}
	// Slice 3 is now 4; the cut piece plays right after each play of its parent.
	if want := []int{4, 1, 2, 1, 2, 0}; !slices.Equal(s.Order, want) {
		t.Errorf("order %v, want %v", s.Order, want)
	}
	if s.Split(1, 1500+10) != -1 || s.Split(1, 1000) != -1 || s.Split(1, 3500) != -1 || s.Split(9, 10) != -1 {
		t.Error("a cut leaving a piece that is too short, or outside the slice, was made")
	}
}

func TestRemove(t *testing.T) {
	s := NewSlicing(4000, 4)
	s.Order = []int{3, 1, 2, 0}
	if !s.Remove(1) {
		t.Fatal("not removed")
	}
	// Nothing grows over the gap.
	if len(s.Slices) != 3 || s.Slices[0].End != 1000 || s.Slices[1].Start != 2000 {
		t.Errorf("slices %+v", s.Slices)
	}
	if want := []int{2, 1, 0}; !slices.Equal(s.Order, want) {
		t.Errorf("order %v, want %v", s.Order, want)
	}
	one := NewSlicing(100, 1)
	if one.Remove(0) {
		t.Error("the last slice was removed")
	}
}

func TestSetStarts(t *testing.T) {
	s := &Slicing{Frames: 10000}
	s.SetStarts([]int{5000, 100, 5010, 20000, -4, 2500})
	var got [][2]int
	for _, sl := range s.Slices {
		got = append(got, [2]int{sl.Start, sl.End})
	}
	want := [][2]int{{0, 100}, {100, 2500}, {2500, 5000}, {5000, 10000}}
	if !slices.Equal(got, want) {
		t.Fatalf("slices %v, want %v", got, want)
	}
}

func TestValidateMendsAFile(t *testing.T) {
	s := &Slicing{Frames: 1000, Order: []int{9, -1, 3},
		Slices: []Slice{{Start: 100, End: 5000, Chance: 900, Gain: -4, Pitch: 99}, {Start: 5000, End: 6000}, {Start: 0, End: 3}, {Start: 200, End: 500}}}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	// The slice past the sample and the one that is too short are dropped; order follows.
	if len(s.Slices) != 2 || s.Slices[0].Start != 100 || s.Slices[0].End != 1000 || s.Slices[1].Start != 200 {
		t.Fatalf("slices %+v", s.Slices)
	}
	if got := s.Slices[0]; got.Chance != 100 || got.Gain != 0 || got.Pitch != 24 {
		t.Errorf("not clamped: %+v", got)
	}
	if !slices.Equal(s.Order, []int{1}) {
		t.Errorf("order %v", s.Order)
	}
	if (&Slicing{}).Validate() == nil {
		t.Error("a slicing with no frames is fine")
	}
}

func TestValidateReadsASlicingWithoutEnds(t *testing.T) {
	s := &Slicing{Frames: 3000, Order: []int{1, 0},
		Slices: []Slice{{Start: 1000, Chance: 100, Gain: 100}, {Start: 0, Chance: 100, Gain: 100}, {Start: 2000, Chance: 100, Gain: 100}}}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	for i, want := range [][2]int{{0, 1000}, {1000, 2000}, {2000, 3000}} {
		if got := [2]int{s.Slices[i].Start, s.Slices[i].End}; got != want {
			t.Errorf("slice %d is %v, want %v", i, got, want)
		}
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

func TestDivideRandomCoversTheSampleWithUnequalSlices(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	s := &Slicing{Frames: 48000}
	s.DivideRandom(8, rng)
	if len(s.Slices) != 8 || len(s.Order) != 8 {
		t.Fatalf("%d slices, order %v", len(s.Slices), s.Order)
	}
	at, shortest, longest := 0, 48000, 0
	for i, sl := range s.Slices {
		if sl.Start != at {
			t.Fatalf("slice %d starts at %d, the one before ended at %d", i, sl.Start, at)
		}
		at = sl.End
		shortest, longest = min(shortest, s.Length(i)), max(longest, s.Length(i))
	}
	if at != 48000 {
		t.Errorf("the slices end at %d", at)
	}
	if shortest < 48000/8/8 || longest < 2*shortest {
		t.Errorf("lengths run from %d to %d: not unequal enough, or too short", shortest, longest)
	}
	again := &Slicing{Frames: 48000}
	again.DivideRandom(8, rand.New(rand.NewSource(7)))
	if !slices.Equal(again.Slices, s.Slices) {
		t.Error("the same dice cut differently")
	}
}

func TestDivideInCutsEachPartOnItsOwn(t *testing.T) {
	s := &Slicing{Frames: 30000}
	s.DivideIn([]int{10000}, 6, nil)
	// 6 slices share the parts by length: 2 in the first part, 4 in the second.
	if len(s.Slices) != 6 {
		t.Fatalf("%d slices", len(s.Slices))
	}
	for i, sl := range s.Slices {
		if sl.Start < 10000 && sl.End > 10000 {
			t.Errorf("slice %d runs over the joint: %d-%d", i, sl.Start, sl.End)
		}
	}
	if s.Slices[1].End != 10000 || s.Slices[2].Start != 10000 {
		t.Errorf("the first part ends at %d, the second starts at %d", s.Slices[1].End, s.Slices[2].Start)
	}
}

func TestExtendAddsSlicesAfterTheOthers(t *testing.T) {
	s := NewSlicing(4000, 4)
	s.Order = []int{3, 1}
	s.Extend(6000, 2)
	if s.Frames != 6000 || len(s.Slices) != 6 || s.Slices[4].Start != 4000 || s.Slices[5].End != 6000 {
		t.Fatalf("frames %d, slices %+v", s.Frames, s.Slices)
	}
	if !slices.Equal(s.Order, []int{3, 1, 4, 5}) {
		t.Errorf("order %v", s.Order)
	}
}

func TestJoinSamplesMakesMonoStereoWhenMixed(t *testing.T) {
	mono := Sample{Channels: 1, Samples: []int16{1, 2}}
	stereo := Sample{Channels: 2, Samples: []int16{5, 6, 7, 8}}
	got := JoinSamples(mono, stereo)
	if got.Channels != 2 || !slices.Equal(got.Samples, []int16{1, 1, 2, 2, 5, 6, 7, 8}) {
		t.Errorf("%+v", got)
	}
	if one := JoinSamples(mono, mono); one.Channels != 1 || one.Frames() != 4 {
		t.Errorf("%+v", one)
	}
}

func TestSettleFindsTheNearestOnsetInReach(t *testing.T) {
	onsets := []int{1000, 5000, 9000}
	if got := Settle(onsets, 5600, 1000, 200); got != 5000 {
		t.Errorf("settled on %d", got)
	}
	if got := Settle(onsets, 4900, 1000, 200); got != 5000 {
		t.Errorf("settled on %d, an onset a little ahead", got)
	}
	if got := Settle(onsets, 7000, 1000, 200); got != 7000 {
		t.Errorf("settled on %d with nothing near", got)
	}
}

// flat is a sample of one loud value, so that what a render does to its length and its fades can
// be read off.
func flat(frames int) Sample {
	s := Sample{Channels: 1, Samples: make([]int16, frames)}
	for i := range s.Samples {
		s.Samples[i] = 10000
	}
	return s
}

func TestRollPlaysTheSliceStartSeveralTimes(t *testing.T) {
	s := NewSlicing(4000, 1)
	s.Slices[0].Roll = 4
	out, placed := Render(flat(4000), s, NewSequencer(s), 1)
	if out.Frames() != 4000 || placed[0].Roll != 4 {
		t.Fatalf("%d frames, roll %d", out.Frames(), placed[0].Roll)
	}
	// Four hits of 1000 frames: loud in the middle of each, down to nothing where one is cut
	// off for the next.
	for hit := 0; hit < 4; hit++ {
		if v := out.Samples[hit*1000+500]; v < 9000 {
			t.Errorf("hit %d is %d in the middle", hit, v)
		}
		if v := out.Samples[hit*1000+999]; absInt(int(v)) > 300 {
			t.Errorf("hit %d is still %d where the next starts", hit, v)
		}
	}
}

func TestSwingDelaysEverySecondStep(t *testing.T) {
	s := NewSlicing(48000, 4)
	s.Division, s.BPM, s.Swing = 16, 120, 66
	_, placed := Render(ramp(48000), s, NewSequencer(s), 4)
	grid := s.StepFrames()
	if placed[0].Frames <= grid || placed[1].Frames >= grid {
		t.Fatalf("steps last %d, %d, a straight one %d", placed[0].Frames, placed[1].Frames, grid)
	}
	if placed[0].Frames+placed[1].Frames != 2*grid || placed[2].At != 2*grid {
		t.Errorf("a pair is %d, the third starts at %d, want %d", placed[0].Frames+placed[1].Frames, placed[2].At, 2*grid)
	}
	// 66% puts the second step two thirds of the way through the pair.
	if want := int(float64(2*grid) * 0.66); absInt(placed[1].At-want) > 2 {
		t.Errorf("the second step starts at %d, want about %d", placed[1].At, want)
	}
	// With an odd number of steps the last one has no partner and stays straight, so a round
	// is as long as its steps.
	out, _ := Render(ramp(48000), s, NewSequencer(s), 3)
	if out.Frames() != 3*grid {
		t.Errorf("three steps are %d frames, want %d", out.Frames(), 3*grid)
	}
}

func TestHumanizeIsRepeatableAndOnlyMovesWhatItShould(t *testing.T) {
	s := NewSlicing(48000, 4)
	s.Division, s.BPM = 16, 120
	plain, _ := Render(flat(48000), s, NewSequencer(s), 8)
	s.Humanize = 100
	one, placed := Render(flat(48000), s, NewSequencer(s), 8)
	two, _ := Render(flat(48000), s, NewSequencer(s), 8)
	if !slices.Equal(one.Samples, two.Samples) {
		t.Error("the same seed humanized differently")
	}
	if slices.Equal(one.Samples, plain.Samples) || one.Frames() != plain.Frames() {
		t.Errorf("humanize changed nothing, or the length (%d, %d)", one.Frames(), plain.Frames())
	}
	late := false
	for _, p := range placed {
		late = late || p.Lead > 0
		if p.Lead >= p.Frames/2 {
			t.Errorf("a step is %d frames late of %d", p.Lead, p.Frames)
		}
	}
	if !late {
		t.Error("no step was late")
	}
}

func TestValidateKeepsTheNewSettingsInRange(t *testing.T) {
	s := NewSlicing(4000, 2)
	s.Swing, s.Humanize, s.Slices[0].Roll = 30, 400, 5
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if s.Swing != 0 || s.Humanize != 100 || s.Slices[0].Roll != 0 {
		t.Errorf("swing %d, humanize %d, roll %d", s.Swing, s.Humanize, s.Slices[0].Roll)
	}
}

func TestSliceEnvelopeFadesInAndOut(t *testing.T) {
	src := Sample{Channels: 1, Samples: make([]int16, 48000)}
	for i := range src.Samples {
		src.Samples[i] = 10000
	}
	s := NewSlicing(48000, 1)
	plain := SliceAudio(src, s, 0)
	s.Slices[0].Attack, s.Slices[0].Release = 100, 200 // ms
	got := SliceAudio(src, s, 0)
	if got.Frames() != plain.Frames() {
		t.Fatalf("the envelope changed the length: %d, %d", got.Frames(), plain.Frames())
	}
	a, r := 100*SampleRate/1000, 200*SampleRate/1000
	if got.Samples[0] != 0 || got.Samples[a/2] < 4000 || got.Samples[a/2] > 6000 {
		t.Errorf("attack: start %d, halfway %d", got.Samples[0], got.Samples[a/2])
	}
	if got.Samples[a+10] != 10000 {
		t.Errorf("after the attack the sample is %d, not at full level", got.Samples[a+10])
	}
	n := got.Frames()
	if got.Samples[n-1] != 0 || got.Samples[n-r/2] < 4000 || got.Samples[n-r/2] > 6000 {
		t.Errorf("release: end %d, halfway %d", got.Samples[n-1], got.Samples[n-r/2])
	}
	// Longer than the slice: it still fits, never louder than the source.
	s.Slices[0].Attack, s.Slices[0].Release = 2000, 2000
	for _, v := range SliceAudio(src, s, 0).Samples {
		if v > 10000 {
			t.Fatal("the envelope made it louder")
		}
	}
}

func TestStepEnvStaysInRange(t *testing.T) {
	if StepEnv(0, -1) != 0 || StepEnv(2000, 1) != 2000 {
		t.Error("steps leave the range")
	}
	if StepEnv(0, 1) != 2 || StepEnv(7, 1) != 10 || StepEnv(7, -1) != 2 {
		t.Errorf("steps: %d %d %d", StepEnv(0, 1), StepEnv(7, 1), StepEnv(7, -1))
	}
}

func TestDivideGridCutsOneSlicePerStep(t *testing.T) {
	edges := func(s *Slicing) []int {
		var out []int
		for _, x := range s.Slices {
			out = append(out, x.Start)
		}
		return out
	}
	// A second at 120 BPM: a 1/4 step is half a second, a 1/16 step is 6000 frames.
	s := NewSlicing(48000, 1)
	if !s.DivideGrid(nil, 24000, 0) || !slices.Equal(edges(s), []int{0, 24000}) {
		t.Errorf("quarters: %v", edges(s))
	}
	s.DivideGrid(nil, 6000, 0)
	if len(s.Slices) != 8 || s.Slices[7].End != 48000 || len(s.Order) != 8 {
		t.Errorf("sixteenths: %d slices, last ends %d, order %v", len(s.Slices), s.Slices[7].End, s.Order)
	}
	// The slices are one step long and touch.
	for i, x := range s.Slices {
		if x.End-x.Start != 6000 || (i > 0 && x.Start != s.Slices[i-1].End) {
			t.Errorf("slice %d is %d..%d", i, x.Start, x.End)
		}
	}
	// An anchor puts a grid line there: what is before it is a slice of its own.
	s.DivideGrid(nil, 6000, 1000)
	if got := edges(s); !slices.Equal(got, []int{0, 1000, 7000, 13000, 19000, 25000, 31000, 37000, 43000}) {
		t.Errorf("anchored: %v", got)
	}
	// A short lead-in is not a slice.
	s.DivideGrid(nil, 6000, 30)
	if got := edges(s); got[0] != 0 || got[1] != 6030 {
		t.Errorf("a lead-in of 30 frames: %v", got)
	}
	// Samples put together are cut each on their own.
	s.DivideGrid([]int{24000}, 6000, 0)
	if got := edges(s); !slices.Equal(got, []int{0, 6000, 12000, 18000, 24000, 30000, 36000, 42000}) {
		t.Errorf("two samples: %v", got)
	}
	// A step that does not divide: the leftover joins the last slice when it is too short.
	s.DivideGrid(nil, 7000, 0)
	if last := s.Slices[len(s.Slices)-1]; last.End != 48000 || s.Validate() != nil {
		t.Errorf("uneven: %v %v", edges(s), s.Validate())
	}
	// Too short a step does nothing.
	before := len(s.Slices)
	if s.DivideGrid(nil, 10, 0) || len(s.Slices) != before {
		t.Error("a step shorter than a slice changed the slicing")
	}
}

func TestRollShapeChangesTheHits(t *testing.T) {
	s := NewSlicing(4000, 1)
	s.Slices[0].Roll = 4
	even, _ := Render(flat(4000), s, NewSequencer(s), 1)
	s.Slices[0].RollLoud = -75 // 100% down to 25%
	decay, _ := Render(flat(4000), s, NewSequencer(s), 1)
	if even.Frames() != decay.Frames() {
		t.Fatalf("lengths %d and %d", even.Frames(), decay.Frames())
	}
	mid := func(x Sample, hit int) int { return int(x.Samples[hit*1000+500]) }
	if mid(even, 0) != mid(even, 3) {
		t.Errorf("even hits differ: %d and %d", mid(even, 0), mid(even, 3))
	}
	if mid(decay, 0) < 9000 || mid(decay, 3) > mid(decay, 0)/3 || mid(decay, 3) < 1000 {
		t.Errorf("decay hits %d, %d, %d, %d", mid(decay, 0), mid(decay, 1), mid(decay, 2), mid(decay, 3))
	}
	// A rise gets higher: the last hit is shorter than the slice, so it is cut sooner.
	s.Slices[0].RollLoud, s.Slices[0].RollPitch = 75, 12
	if rise, _ := Render(flat(4000), s, NewSequencer(s), 1); rise.Frames() != 4000 {
		t.Errorf("a rise changed the length to %d", rise.Frames())
	}
}
