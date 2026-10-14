package formats

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"slices"
	"sort"
)

// Slicing cuts a sample into slices and says how they are played back: in an order you set, or
// at random. It is the whole model of the slicer, with no I/O; everything is in sample frames
// of the source, so it can be saved and played again on the same sample.
//
// A slice runs from its Start to the Start of the next one, the last to the end of the sample,
// so the slices always cover the sample from the first one's start and never overlap. The
// slices are kept in the order they lie in the sample; the order they are played in is Order.
type Slicing struct {
	// Frames is the length of the sample that is sliced.
	Frames int     `json:"frames"`
	Slices []Slice `json:"slices"`
	// Order is the slices to play, by index, one after the other and round again.
	Order []int    `json:"order"`
	Mode  PlayMode `json:"mode"`
	// Seed makes random modes repeatable: the same seed picks the same slices.
	Seed int64 `json:"seed"`
	// Division and BPM put the steps on a grid: with a Division of 16, every step lasts a
	// sixteenth note at BPM, whatever the slice's length; a longer slice is cut off, a shorter
	// one leaves silence. Division 0 plays the slices at their own length, one after the other.
	Division int     `json:"division,omitempty"`
	BPM      float64 `json:"bpm,omitempty"`
}

// Divisions are the grids a slicing can be put on, as notes to the bar: 4 is a quarter note.
var Divisions = []int{0, 4, 8, 16}

// StepFrames is how long a step lasts on the grid, 0 when the slices play at their own
// length.
func (s *Slicing) StepFrames() int {
	if s.Division <= 0 || s.BPM <= 0 {
		return 0
	}
	return max(int(math.Round(float64(SampleRate)*60/s.BPM*4/float64(s.Division))), 1)
}

// Slice is where one slice starts and how it sounds.
type Slice struct {
	Start int `json:"start"`
	// Chance is 0..100. In Order mode it is the chance that the slice plays when its turn
	// comes (otherwise the time is left silent); in the random modes it is the weight the
	// slice is picked with, so 0 never plays.
	Chance int `json:"chance"`
	// Gain is a percentage: 100 as it is.
	Gain    int  `json:"gain"`
	Pitch   int  `json:"pitch"` // semitones
	Reverse bool `json:"reverse,omitempty"`
}

// PlayMode is how the next slice is picked.
type PlayMode int

const (
	// ModeOrder plays the slices of Order, over and over.
	ModeOrder PlayMode = iota
	// ModeRandom picks any slice, weighted by its Chance.
	ModeRandom
	// ModeNoRepeat is ModeRandom without the same slice twice in a row.
	ModeNoRepeat
	// ModeShuffle plays every slice once, in a new random order each round.
	ModeShuffle
)

var modeNames = [...]string{"order", "random", "no repeat", "shuffle"}

func (m PlayMode) String() string {
	if m < 0 || int(m) >= len(modeNames) {
		return fmt.Sprint(int(m))
	}
	return modeNames[m]
}

// Next is the following mode, round to the first.
func (m PlayMode) Next() PlayMode { return (m + 1) % PlayMode(len(modeNames)) }

// MinSliceFrames is the shortest a slice can be made, so that a start point cannot be pushed
// onto the next one: about 1.3 ms.
const MinSliceFrames = 64

// DefaultSlice is a slice that plays as it is.
func DefaultSlice(start int) Slice { return Slice{Start: start, Chance: 100, Gain: 100} }

// ErrNoSlices is a slicing with nothing to play.
var ErrNoSlices = errors.New("no slices")

// NewSlicing cuts a sample of the given length into n equal slices, played in sequence.
func NewSlicing(frames, n int) *Slicing {
	s := &Slicing{Frames: frames}
	s.Divide(n)
	return s
}

// Divide replaces the slices by n equal ones and the order by their sequence.
func (s *Slicing) Divide(n int) {
	n = max(1, min(n, max(s.Frames/MinSliceFrames, 1)))
	s.Slices = s.Slices[:0]
	for i := 0; i < n; i++ {
		s.Slices = append(s.Slices, DefaultSlice(i*s.Frames/n))
	}
	s.ResetOrder()
}

// SetStarts replaces the slices by ones starting at the given frames (any order, duplicates
// and starts too close are dropped), and the order by their sequence. A start at 0 is added
// when none is, so the whole sample is covered.
func (s *Slicing) SetStarts(starts []int) {
	starts = append([]int{0}, starts...)
	sort.Ints(starts)
	s.Slices = s.Slices[:0]
	for _, f := range starts {
		if f < 0 || f >= s.Frames {
			continue
		}
		if n := len(s.Slices); n > 0 && f-s.Slices[n-1].Start < MinSliceFrames {
			continue
		}
		s.Slices = append(s.Slices, DefaultSlice(f))
	}
	s.ResetOrder()
}

// ResetOrder plays the slices in the order they lie in the sample.
func (s *Slicing) ResetOrder() {
	s.Order = s.Order[:0]
	for i := range s.Slices {
		s.Order = append(s.Order, i)
	}
}

// End is the frame after the last one of slice i.
func (s *Slicing) End(i int) int {
	if i+1 < len(s.Slices) {
		return s.Slices[i+1].Start
	}
	return s.Frames
}

// Length is how many frames slice i has.
func (s *Slicing) Length(i int) int { return s.End(i) - s.Slices[i].Start }

// SliceAt is the slice that holds a frame, -1 before the first one.
func (s *Slicing) SliceAt(frame int) int {
	return sort.Search(len(s.Slices), func(i int) bool { return s.Slices[i].Start > frame }) - 1
}

// MoveStart moves slice i's start by delta frames. It stops at the neighbours (leaving each at
// least MinSliceFrames) and reports where the start is now. Moving a start moves the end of
// the slice before it, as the slices butt against each other.
func (s *Slicing) MoveStart(i, delta int) int {
	return s.SetStart(i, s.Slices[i].Start+delta)
}

// SetStart puts slice i's start at a frame, held between its neighbours. The first slice may
// start anywhere before the second; what lies before it is left out.
func (s *Slicing) SetStart(i, frame int) int {
	lo := 0
	if i > 0 {
		lo = s.Slices[i-1].Start + MinSliceFrames
	}
	hi := s.End(i) - MinSliceFrames
	if hi < lo {
		return s.Slices[i].Start
	}
	s.Slices[i].Start = min(max(frame, lo), hi)
	return s.Slices[i].Start
}

// Split cuts the slice that holds a frame in two, the new slice starting there, with the
// settings of the one it was cut from. It plays right after the old one wherever that is in
// the order. It returns the new slice's index, or -1 when the cut would leave a piece that is
// too short.
func (s *Slicing) Split(frame int) int {
	i := s.SliceAt(frame)
	if i < 0 || frame-s.Slices[i].Start < MinSliceFrames || s.End(i)-frame < MinSliceFrames {
		return -1
	}
	n := s.Slices[i]
	n.Start = frame
	s.Slices = slices.Insert(s.Slices, i+1, n)
	var order []int
	for _, o := range s.Order {
		switch {
		case o > i:
			order = append(order, o+1)
		case o == i:
			order = append(order, o, i+1)
		default:
			order = append(order, o)
		}
	}
	s.Order = order
	return i + 1
}

// Remove takes slice i out: the one before it grows over its part of the sample (the first
// one leaves its part out), and it is dropped from the order. The last slice cannot be removed.
func (s *Slicing) Remove(i int) bool {
	if len(s.Slices) < 2 || i < 0 || i >= len(s.Slices) {
		return false
	}
	s.Slices = slices.Delete(s.Slices, i, i+1)
	var order []int
	for _, o := range s.Order {
		switch {
		case o == i:
		case o > i:
			order = append(order, o-1)
		default:
			order = append(order, o)
		}
	}
	if len(order) == 0 {
		for j := range s.Slices {
			order = append(order, j)
		}
	}
	s.Order = order
	return true
}

// Clone is a copy that shares nothing with the original.
func (s *Slicing) Clone() *Slicing {
	c := *s
	c.Slices = slices.Clone(s.Slices)
	c.Order = slices.Clone(s.Order)
	return &c
}

// Validate puts right what a file that was edited or came from another sample can get wrong:
// starts out of range or out of order, an order pointing at slices that are not there, a gain
// or a chance out of range.
func (s *Slicing) Validate() error {
	if s.Frames <= 0 {
		return errors.New("the sample has no frames")
	}
	slices.SortFunc(s.Slices, func(a, b Slice) int { return a.Start - b.Start })
	kept := s.Slices[:0]
	for _, sl := range s.Slices {
		if sl.Start < 0 || sl.Start >= s.Frames {
			continue
		}
		if n := len(kept); n > 0 && sl.Start-kept[n-1].Start < MinSliceFrames {
			continue
		}
		sl.Chance = min(max(sl.Chance, 0), 100)
		sl.Gain = min(max(sl.Gain, 0), 400)
		sl.Pitch = min(max(sl.Pitch, -24), 24)
		kept = append(kept, sl)
	}
	s.Slices = kept
	if len(s.Slices) == 0 {
		return ErrNoSlices
	}
	order := s.Order[:0]
	for _, o := range s.Order {
		if o >= 0 && o < len(s.Slices) {
			order = append(order, o)
		}
	}
	s.Order = order
	if len(s.Order) == 0 {
		s.ResetOrder()
	}
	if s.Mode < 0 || int(s.Mode) >= len(modeNames) {
		s.Mode = ModeOrder
	}
	if !slices.Contains(Divisions, s.Division) {
		s.Division = 0
	}
	if s.BPM != 0 {
		s.BPM = min(max(s.BPM, 40), 300)
	}
	return nil
}

// ---- picking ----

// Step is one slice to play: which, and whether it sounds (a step that does not is a rest as
// long as the slice would have been).
type Step struct {
	Slice int
	Rest  bool
}

// Sequencer picks the slices to play, one after the other, in the slicing's mode. It works on a
// copy of the slicing, so editing goes on without it; make a new one to hear an edit.
type Sequencer struct {
	s    *Slicing
	rng  *rand.Rand
	at   int
	last int
	bag  []int
}

// NewSequencer starts a sequence at the first step. A seed of 0 in the slicing is as good as
// any other, so a slicing plays the same every time.
func NewSequencer(s *Slicing) *Sequencer {
	return &Sequencer{s: s.Clone(), rng: rand.New(rand.NewSource(s.Seed)), last: -1}
}

// Next is the next step.
func (q *Sequencer) Next() Step {
	s := q.s
	switch s.Mode {
	case ModeRandom, ModeNoRepeat:
		i := q.weighted(s.Mode == ModeNoRepeat)
		q.last = i
		return Step{Slice: i}
	case ModeShuffle:
		if len(q.bag) == 0 {
			for i, sl := range s.Slices {
				if sl.Chance > 0 {
					q.bag = append(q.bag, i)
				}
			}
			if len(q.bag) == 0 {
				q.bag = append(q.bag, 0)
			}
			q.rng.Shuffle(len(q.bag), func(a, b int) { q.bag[a], q.bag[b] = q.bag[b], q.bag[a] })
			// A new round does not start with the slice the last one ended on.
			if len(q.bag) > 1 && q.bag[len(q.bag)-1] == q.last {
				q.bag[0], q.bag[len(q.bag)-1] = q.bag[len(q.bag)-1], q.bag[0]
			}
		}
		i := q.bag[len(q.bag)-1]
		q.bag = q.bag[:len(q.bag)-1]
		q.last = i
		return Step{Slice: i}
	}
	i := s.Order[q.at%len(s.Order)]
	q.at++
	q.last = i
	// The roll is made for every step, so that the picks after it do not depend on whether a
	// chance was 100.
	roll := q.rng.Intn(100)
	return Step{Slice: i, Rest: roll >= s.Slices[i].Chance}
}

// weighted picks a slice with the odds its chances give. With nothing to pick from (every
// chance 0, or only the slice that just played) it plays the first slice that has one.
func (q *Sequencer) weighted(noRepeat bool) int {
	s := q.s
	total := 0
	weight := func(i int) int {
		if noRepeat && i == q.last && len(s.Slices) > 1 {
			return 0
		}
		return s.Slices[i].Chance
	}
	for i := range s.Slices {
		total += weight(i)
	}
	if total == 0 {
		for i := range s.Slices {
			if i != q.last || len(s.Slices) == 1 {
				return i
			}
		}
		return 0
	}
	roll := q.rng.Intn(total)
	for i := range s.Slices {
		if roll -= weight(i); roll < 0 {
			return i
		}
	}
	return 0
}

// ---- rendering ----

// declick is the fade at the ends of a slice, in frames: 1.5 ms, so that a cut in the middle of
// a wave does not click.
const declick = 72

// SliceAudio is slice i of a sample as it sounds: cut out, reversed, pitched, at its gain and
// with its ends faded.
func SliceAudio(src Sample, s *Slicing, i int) Sample {
	ch := max(src.Channels, 1)
	sl := s.Slices[i]
	from, to := min(sl.Start, src.Frames()), min(s.End(i), src.Frames())
	if to <= from {
		return Sample{Channels: src.Channels}
	}
	out := Sample{Channels: src.Channels, Samples: slices.Clone(src.Samples[from*ch : to*ch])}
	if sl.Reverse {
		for a, b := 0, out.Frames()-1; a < b; a, b = a+1, b-1 {
			for c := 0; c < ch; c++ {
				out.Samples[a*ch+c], out.Samples[b*ch+c] = out.Samples[b*ch+c], out.Samples[a*ch+c]
			}
		}
	}
	if sl.Pitch != 0 {
		out = out.Pitched(sl.Pitch)
	}
	frames := out.Frames()
	fade := min(declick, frames/2)
	for f := 0; f < frames; f++ {
		g := float64(sl.Gain) / 100
		if f < fade {
			g *= float64(f) / float64(fade)
		} else if left := frames - 1 - f; left < fade {
			g *= float64(left) / float64(fade)
		}
		if g == 1 {
			continue
		}
		for c := 0; c < ch; c++ {
			v := math.Round(float64(out.Samples[f*ch+c]) * g)
			out.Samples[f*ch+c] = int16(min(max(v, math.MinInt16), math.MaxInt16))
		}
	}
	return out
}

// Render joins the next steps slices of a sequence into one sample, and says where each one
// starts in it, in frames, and which it is, so a playhead can follow. The pieces of a rest are
// silence as long as the slice.
func Render(src Sample, s *Slicing, q *Sequencer, steps int) (Sample, []Placed) {
	out := Sample{Channels: src.Channels}
	var placed []Placed
	cache := map[int]Sample{}
	ch := max(src.Channels, 1)
	grid := s.StepFrames()
	for n := 0; n < steps; n++ {
		step := q.Next()
		audio, ok := cache[step.Slice]
		if !ok {
			audio = SliceAudio(src, s, step.Slice)
			cache[step.Slice] = audio
		}
		slot := audio.Frames()
		if grid > 0 {
			slot = grid
		}
		placed = append(placed, Placed{Step: step, At: out.Frames(), Frames: slot, Audio: min(audio.Frames(), slot)})
		piece := make([]int16, slot*ch)
		if !step.Rest {
			copy(piece, audio.Samples[:min(len(audio.Samples), len(piece))])
			if audio.Frames() > slot {
				// Cut off by the next step: fade out, so the cut does not click.
				fadeOut(piece, ch, min(declick, slot/2))
			}
		}
		out.Samples = append(out.Samples, piece...)
	}
	return out, placed
}

// Placed is a step in a rendered sample.
type Placed struct {
	Step
	// At is where the step starts and Frames how long it lasts; Audio is how much of that the
	// slice sounds for (on a grid it can be less, or cut short).
	At, Frames, Audio int
}

// fadeOut brings the last frames of interleaved audio down to nothing.
func fadeOut(samples []int16, ch, frames int) {
	n := len(samples) / ch
	for f := 0; f < frames && f < n; f++ {
		g := float64(f) / float64(frames)
		at := (n - 1 - f) * ch
		for c := 0; c < ch; c++ {
			samples[at+c] = int16(math.Round(float64(samples[at+c]) * g))
		}
	}
}

// ---- waveform and cutting helpers ----

// Peaks is the loudest absolute value, 0..32768, of cols equal parts of the frames from..to of
// the sample, for drawing it.
func (s Sample) Peaks(from, to, cols int) []int {
	ch := max(s.Channels, 1)
	frames := s.Frames()
	from, to = max(from, 0), min(to, frames)
	out := make([]int, max(cols, 0))
	if to <= from || cols <= 0 {
		return out
	}
	for c := range out {
		lo := from + int(int64(c)*int64(to-from)/int64(cols))
		hi := from + int(int64(c+1)*int64(to-from)/int64(cols))
		hi = min(max(hi, lo+1), to)
		peak := 0
		for _, v := range s.Samples[lo*ch : hi*ch] {
			peak = max(peak, absInt(int(v)))
		}
		out[c] = peak
	}
	return out
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// ZeroCrossing is the frame nearest to a frame, within radius, where the sample (all channels
// summed) changes sign: a cut there does not click. With none in reach it is the frame itself.
func (s Sample) ZeroCrossing(frame, radius int) int {
	ch := max(s.Channels, 1)
	frames := s.Frames()
	at := func(f int) int {
		sum := 0
		for c := 0; c < ch; c++ {
			sum += int(s.Samples[f*ch+c])
		}
		return sum
	}
	if frame < 0 || frame >= frames {
		return frame
	}
	for d := 0; d <= radius; d++ {
		for _, f := range []int{frame - d, frame + d} {
			if f < 1 || f >= frames {
				continue
			}
			a, b := at(f-1), at(f)
			if (a <= 0 && b > 0) || (a >= 0 && b < 0) {
				return f
			}
		}
	}
	return frame
}

// Transients finds where sounds start in a sample, as the frames of up to limit onsets, the
// strongest ones when there are more. It looks for jumps in loudness, so it suits drums and
// plucked sounds better than pads. sensitivity is 0..1: higher finds more.
func (s Sample) Transients(limit int, sensitivity float64) []int {
	const hop = 256
	ch := s.Channels
	if ch < 1 {
		ch = 1
	}
	frames := s.Frames()
	n := frames / hop
	if n < 3 || limit < 1 {
		return nil
	}
	energy := make([]float64, n)
	for i := range energy {
		sum := 0.0
		for f := i * hop; f < (i+1)*hop; f++ {
			v := 0.0
			for c := 0; c < ch; c++ {
				v += float64(s.Samples[f*ch+c])
			}
			v /= float64(ch) * 32768
			sum += v * v
		}
		energy[i] = math.Sqrt(sum / hop)
	}
	// Onset strength: the rise in loudness, in dB, from the quietest of the hops just before.
	const floor = 1e-4
	strength := make([]float64, n)
	for i := 1; i < n; i++ {
		prev := energy[i-1]
		if i > 1 {
			prev = math.Min(prev, energy[i-2])
		}
		strength[i] = 20 * math.Log10((energy[i]+floor)/(prev+floor))
	}
	// A hop is an onset when it rises clearly and is the strongest of its neighbours.
	threshold := 3 + (1-math.Min(math.Max(sensitivity, 0), 1))*15
	minGap := 48000 / 16 / hop
	type onset struct {
		hop      int
		strength float64
	}
	var found []onset
	for i := 1; i < n; i++ {
		if strength[i] < threshold {
			continue
		}
		peak := true
		for j := max(i-minGap, 1); j <= min(i+minGap, n-1); j++ {
			if strength[j] > strength[i] || (strength[j] == strength[i] && j < i) {
				peak = false
				break
			}
		}
		if peak {
			found = append(found, onset{i, strength[i]})
		}
	}
	sort.Slice(found, func(a, b int) bool { return found[a].strength > found[b].strength })
	if len(found) > limit {
		found = found[:limit]
	}
	out := make([]int, 0, len(found))
	for _, o := range found {
		// The rise is seen a hop late: start a hop before, then at a zero crossing.
		out = append(out, s.ZeroCrossing(max(o.hop-1, 0)*hop, hop/2))
	}
	sort.Ints(out)
	return out
}
