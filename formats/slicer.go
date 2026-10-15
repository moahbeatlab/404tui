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
// Each slice has its own Start and End, independent of the others: slices can leave gaps between
// them, overlap, or repeat a part of the sample. Slices are numbered as they were made; the order
// they are played in is Order.
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
	// Swing, on a grid, puts every second step late, as a drum machine's swing does: 50 is
	// straight, 66 is a triplet feel, 0 is off.
	Swing int `json:"swing,omitempty"`
	// Humanize is 0..100: how much each step's loudness and, on a grid, its timing wander.
	Humanize int `json:"humanize,omitempty"`
}

// Swings are the amounts of swing a slicing steps through, in percent: 0 is off.
var Swings = []int{0, 54, 58, 62, 66, 70}

// Humanizes are the amounts of humanize a slicing steps through.
var Humanizes = []int{0, 25, 50, 100}

// Rolls are the numbers of hits a slice's roll steps through: 0 is none.
var Rolls = []int{0, 2, 3, 4, 6, 8}

// RollShape is how the hits of a roll change from the first to the last: their loudness, in
// percent of the slice's gain, and their pitch, in semitones at the last hit (the ones between
// are on a straight line). A build-up roll rises in pitch while it swells.
type RollShape struct {
	Name      string
	From, To  int
	Semitones int
}

// RollShapes are the shapes a roll steps through; the first is every hit alike.
var RollShapes = []RollShape{
	{"even", 100, 100, 0},
	{"decay", 100, 25, 0},
	{"swell", 25, 100, 0},
	{"rise", 40, 100, 12},
	{"fall", 100, 40, -12},
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
	// Start is the first frame of the slice and End the frame after its last one.
	Start int `json:"start"`
	End   int `json:"end"`
	// Chance is 0..100. In Order mode it is the chance that the slice plays when its turn
	// comes (otherwise the time is left silent); in the random modes it is the weight the
	// slice is picked with, so 0 never plays.
	Chance int `json:"chance"`
	// Gain is a percentage: 100 as it is.
	Gain    int  `json:"gain"`
	Pitch   int  `json:"pitch"` // semitones
	Reverse bool `json:"reverse,omitempty"`
	// Roll plays the start of the slice that many times over the time the slice has: a drum
	// roll. 0 plays it once.
	Roll int `json:"roll,omitempty"`
	// RollShape is the index in RollShapes of how the hits of the roll change.
	RollShape int `json:"roll_shape,omitempty"`
	// Attack and Release are fades in and out of the slice, in milliseconds, baked into its
	// audio. 0 leaves only the short fade that keeps a cut from clicking.
	Attack  int `json:"attack,omitempty"`
	Release int `json:"release,omitempty"`
}

// EnvSteps are the lengths, in milliseconds, that an attack or a release steps through.
var EnvSteps = []int{0, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 2000}

// StepEnv moves ms one place up or down EnvSteps, from the nearest place if it is between.
func StepEnv(ms, dir int) int {
	at := 0
	for i, v := range EnvSteps {
		if v <= ms {
			at = i
		}
	}
	return EnvSteps[min(max(at+dir, 0), len(EnvSteps)-1)]
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
func DefaultSlice(start, end int) Slice { return Slice{Start: start, End: end, Chance: 100, Gain: 100} }

// ErrNoSlices is a slicing with nothing to play.
var ErrNoSlices = errors.New("no slices")

// NewSlicing cuts a sample of the given length into n equal slices, played in sequence.
func NewSlicing(frames, n int) *Slicing {
	s := &Slicing{Frames: frames}
	s.Divide(n)
	return s
}

// Divide replaces the slices by n equal ones and the order by their sequence.
func (s *Slicing) Divide(n int) { s.DivideIn(nil, n, nil) }

// DivideRandom replaces the slices by n of unequal lengths, cut at random places, and the order
// by their sequence.
func (s *Slicing) DivideRandom(n int, rng *rand.Rand) { s.DivideIn(nil, n, rng) }

// DivideIn replaces the slices by about n new ones and the order by their sequence. The sample is
// made of parts that start at the given frames (a sample put together from several: none for one
// part), and every part is cut on its own, with its share of the n by its length and at least
// one. With a rng the cuts fall at random places and the slices differ in length; without one
// they are equal.
func (s *Slicing) DivideIn(starts []int, n int, rng *rand.Rand) {
	bounds := []int{0}
	for _, f := range starts {
		if f > bounds[len(bounds)-1] && f < s.Frames {
			bounds = append(bounds, f)
		}
	}
	bounds = append(bounds, s.Frames)
	s.Slices = s.Slices[:0]
	for i := 0; i+1 < len(bounds); i++ {
		from, to := bounds[i], bounds[i+1]
		k := max(int(math.Round(float64(n)*float64(to-from)/float64(max(s.Frames, 1)))), 1)
		s.Slices = append(s.Slices, divideRange(from, to, k, rng)...)
	}
	s.ResetOrder()
}

// DivideGrid cuts the sample on a tempo grid: an edge at every step frames, so each slice is one
// step long. Each part (as in DivideIn) has its grid lines from its own start, except the part
// that anchor lies in, where one of the lines is at the anchor: the downbeat, when the sample
// begins a little before it. What is left before the first line, if it is long enough to be a
// slice, is one. A step shorter than a slice can be leaves things as they are, and false.
func (s *Slicing) DivideGrid(starts []int, step float64, anchor int) bool {
	if !(step >= MinSliceFrames) || math.IsInf(step, 0) {
		return false
	}
	bounds := []int{0}
	for _, f := range starts {
		if f > bounds[len(bounds)-1] && f < s.Frames {
			bounds = append(bounds, f)
		}
	}
	bounds = append(bounds, s.Frames)
	s.Slices = s.Slices[:0]
	for i := 0; i+1 < len(bounds); i++ {
		from, to := bounds[i], bounds[i+1]
		base := float64(from)
		if anchor >= from && anchor < to {
			base = float64(anchor)
		}
		edges := []int{from}
		for k := -int(math.Floor((base - float64(from)) / step)); ; k++ {
			e := int(math.Round(base + float64(k)*step))
			if e >= to {
				break
			}
			if e-edges[len(edges)-1] >= MinSliceFrames {
				edges = append(edges, e)
			}
		}
		if len(edges) > 1 && to-edges[len(edges)-1] < MinSliceFrames {
			edges = edges[:len(edges)-1]
		}
		for j, e := range edges {
			end := to
			if j+1 < len(edges) {
				end = edges[j+1]
			}
			s.Slices = append(s.Slices, DefaultSlice(e, end))
		}
	}
	s.ResetOrder()
	return true
}

// divideRange cuts frames from..to in n slices, equal or, with a rng, of random lengths of which
// none is shorter than an eighth of an equal one.
func divideRange(from, to, n int, rng *rand.Rand) []Slice {
	frames := to - from
	n = max(1, min(n, max(frames/MinSliceFrames, 1)))
	out := make([]Slice, 0, n)
	if rng == nil {
		for i := 0; i < n; i++ {
			out = append(out, DefaultSlice(from+i*frames/n, from+(i+1)*frames/n))
		}
		return out
	}
	floor := min(max(frames/(n*8), MinSliceFrames), frames/n)
	weights := make([]float64, n)
	total := 0.0
	for i := range weights {
		weights[i] = 0.2 + math.Min(rng.ExpFloat64(), 5)
		total += weights[i]
	}
	spare := float64(frames - n*floor)
	start := from
	for i := 0; i < n; i++ {
		end := min(start+floor+int(spare*weights[i]/total), to)
		if i == n-1 {
			end = to
		}
		out = append(out, DefaultSlice(start, end))
		start = end
	}
	return out
}

// Extend makes the sample longer, by a part put on its end that is cut in n equal slices, played
// after the others.
func (s *Slicing) Extend(frames, n int) {
	from := s.Frames
	if frames <= from {
		return
	}
	s.Frames = frames
	for _, sl := range divideRange(from, frames, n, nil) {
		s.Order = append(s.Order, len(s.Slices))
		s.Slices = append(s.Slices, sl)
	}
}

// SetStarts replaces the slices by ones starting at the given frames, each running to the start
// of the next (any order; duplicates and starts too close are dropped), and the order by their
// sequence. A start at 0 is added when none is, so the whole sample is covered.
func (s *Slicing) SetStarts(starts []int) {
	starts = append([]int{0}, starts...)
	sort.Ints(starts)
	var kept []int
	for _, f := range starts {
		if f < 0 || f >= s.Frames {
			continue
		}
		if n := len(kept); n > 0 && f-kept[n-1] < MinSliceFrames {
			continue
		}
		kept = append(kept, f)
	}
	s.Slices = s.Slices[:0]
	for i, f := range kept {
		end := s.Frames
		if i+1 < len(kept) {
			end = kept[i+1]
		}
		s.Slices = append(s.Slices, DefaultSlice(f, end))
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
func (s *Slicing) End(i int) int { return s.Slices[i].End }

// Length is how many frames slice i has.
func (s *Slicing) Length(i int) int { return s.Slices[i].End - s.Slices[i].Start }

// SetStart puts slice i's start at a frame, at least MinSliceFrames before its end. Nothing else
// moves.
func (s *Slicing) SetStart(i, frame int) int {
	s.Slices[i].Start = min(max(frame, 0), s.Slices[i].End-MinSliceFrames)
	return s.Slices[i].Start
}

// SetEnd puts slice i's end at a frame, at least MinSliceFrames after its start.
func (s *Slicing) SetEnd(i, frame int) int {
	s.Slices[i].End = min(max(frame, s.Slices[i].Start+MinSliceFrames), s.Frames)
	return s.Slices[i].End
}

// MoveStart moves slice i's start by delta frames and reports where it is now.
func (s *Slicing) MoveStart(i, delta int) int { return s.SetStart(i, s.Slices[i].Start+delta) }

// MoveEnd moves slice i's end by delta frames and reports where it is now.
func (s *Slicing) MoveEnd(i, delta int) int { return s.SetEnd(i, s.Slices[i].End+delta) }

// Split cuts slice i in two at a frame: i now ends there and a new slice, with the same settings,
// runs from there to where i ended. The new one plays right after i wherever that is in the
// order. It returns the new slice's index, or -1 when the frame is not inside the slice or
// would leave a piece that is too short.
func (s *Slicing) Split(i, frame int) int {
	if i < 0 || i >= len(s.Slices) || frame-s.Slices[i].Start < MinSliceFrames || s.Slices[i].End-frame < MinSliceFrames {
		return -1
	}
	n := s.Slices[i]
	n.Start = frame
	s.Slices[i].End = frame
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

// Remove takes slice i out, and drops it from the order. The last slice cannot be removed.
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
// edges out of range, an order pointing at slices that are not there, a gain or a chance out of
// range. A file from before slices had an end of their own has none: each ends where the next
// starts.
func (s *Slicing) Validate() error {
	if s.Frames <= 0 {
		return errors.New("the sample has no frames")
	}
	legacy := slices.ContainsFunc(s.Slices, func(sl Slice) bool { return sl.End == 0 })
	if legacy {
		// The old slices were always in the order they lay in the sample.
		slices.SortFunc(s.Slices, func(a, b Slice) int { return a.Start - b.Start })
		for i := range s.Slices {
			s.Slices[i].End = s.Frames
			if i+1 < len(s.Slices) {
				s.Slices[i].End = s.Slices[i+1].Start
			}
		}
	}
	kept := s.Slices[:0]
	remap := map[int]int{}
	for i, sl := range s.Slices {
		sl.End = min(sl.End, s.Frames)
		if sl.Start < 0 || sl.End-sl.Start < MinSliceFrames {
			continue
		}
		sl.Chance = min(max(sl.Chance, 0), 100)
		sl.Gain = min(max(sl.Gain, 0), 400)
		sl.Pitch = min(max(sl.Pitch, -24), 24)
		sl.Attack = min(max(sl.Attack, 0), EnvSteps[len(EnvSteps)-1])
		sl.Release = min(max(sl.Release, 0), EnvSteps[len(EnvSteps)-1])
		if !slices.Contains(Rolls, sl.Roll) {
			sl.Roll = 0
		}
		if sl.RollShape < 0 || sl.RollShape >= len(RollShapes) {
			sl.RollShape = 0
		}
		remap[i] = len(kept)
		kept = append(kept, sl)
	}
	s.Slices = kept
	if len(s.Slices) == 0 {
		return ErrNoSlices
	}
	order := s.Order[:0]
	for _, o := range s.Order {
		if n, ok := remap[o]; ok {
			order = append(order, n)
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
	if s.Swing < 50 || s.Swing > 75 {
		s.Swing = 0
	}
	s.Humanize = min(max(s.Humanize, 0), 100)
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
	attack := max(fade, min(sl.Attack*SampleRate/1000, frames))
	release := max(fade, min(sl.Release*SampleRate/1000, frames))
	for f := 0; f < frames; f++ {
		g := float64(sl.Gain) / 100
		if f < attack {
			g *= float64(f) / float64(attack)
		}
		if left := frames - 1 - f; left < release {
			g *= float64(left) / float64(release)
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
// silence as long as the slice. On a grid, swing makes every second step late and humanize lets
// the timing and the loudness wander; both come from the slicing's seed, so a seed plays the
// same every time.
func Render(src Sample, s *Slicing, q *Sequencer, steps int) (Sample, []Placed) {
	out := Sample{Channels: src.Channels}
	var placed []Placed
	cache := map[int]Sample{}
	ch := max(src.Channels, 1)
	grid := s.StepFrames()
	human := rand.New(rand.NewSource(s.Seed ^ 0x5bd1e995))
	swing := 0
	if grid > 0 && s.Swing > 50 {
		swing = int(float64(grid) * float64(s.Swing-50) * 2 / 100)
	}
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
			switch {
			case n%2 == 1:
				slot -= swing
			case n < steps-1:
				// The step before a late one lasts longer. A last step with no partner stays
				// straight, so that the round has the length of its steps and the next starts
				// on the beat.
				slot += swing
			}
		}
		// The dice are rolled for every step, so that a setting does not change the others.
		lateness, loudness := human.Float64(), human.Float64()*2-1
		lead := 0
		if grid > 0 && s.Humanize > 0 {
			lead = min(int(lateness*float64(grid)*float64(s.Humanize)/100/8), slot/2)
		}
		gain := 1 + loudness*float64(s.Humanize)/100*0.25
		roll := s.Slices[step.Slice].Roll
		if roll > 1 && slot/roll < MinSliceFrames {
			roll = 0
		}
		if roll > 1 {
			lead = 0
		}
		window := slot - lead
		placed = append(placed, Placed{Step: step, At: out.Frames(), Frames: slot, Audio: min(audio.Frames(), window), Lead: lead, Roll: roll, Len: audio.Frames()})
		piece := make([]int16, slot*ch)
		if !step.Rest {
			if roll > 1 {
				shape := RollShapes[min(max(s.Slices[step.Slice].RollShape, 0), len(RollShapes)-1)]
				for r := 0; r < roll; r++ {
					hit := audio
					if shape.Semitones != 0 || shape.From != shape.To {
						hit = rollHit(audio, shape, float64(r)/float64(roll-1))
					}
					lay(piece[(r*slot/roll)*ch:((r+1)*slot/roll)*ch], hit.Samples, ch)
				}
			} else {
				lay(piece[lead*ch:], audio.Samples, ch)
			}
			if gain != 1 {
				for i, v := range piece {
					piece[i] = int16(min(max(math.Round(float64(v)*gain), math.MinInt16), math.MaxInt16))
				}
			}
		}
		out.Samples = append(out.Samples, piece...)
	}
	return out, placed
}

// rollHit is one hit of a roll at t (0 for the first, 1 for the last): the audio at the
// loudness and the pitch the shape has there.
func rollHit(audio Sample, shape RollShape, t float64) Sample {
	hit := audio
	if semis := int(math.Round(float64(shape.Semitones) * t)); semis != 0 {
		hit = hit.Pitched(semis)
	}
	gain := (float64(shape.From) + float64(shape.To-shape.From)*t) / 100
	if gain == 1 {
		return hit
	}
	out := Sample{Channels: hit.Channels, Samples: make([]int16, len(hit.Samples))}
	for i, v := range hit.Samples {
		out.Samples[i] = int16(math.Round(float64(v) * gain))
	}
	return out
}

// lay copies audio to the start of dst, which has the given channels; audio that is longer is
// cut off with a fade-out, so the cut does not click.
func lay(dst, audio []int16, ch int) {
	n := copy(dst, audio)
	if len(audio) > len(dst) {
		fadeOut(dst[:n], ch, min(declick, n/ch/2))
	}
}

// Placed is a step in a rendered sample.
type Placed struct {
	Step
	// At is where the step starts and Frames how long it lasts; Audio is how much of that the
	// slice sounds for (on a grid it can be less, or cut short), after Lead frames of silence
	// that humanize put in front. Len is the whole slice as it sounds, and Roll the number of
	// hits it is played in.
	At, Frames, Audio, Lead, Len, Roll int
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

// JoinSamples puts samples one after the other. A mono sample that comes with stereo ones is
// played on both sides.
func JoinSamples(parts ...Sample) Sample {
	ch := 1
	for _, p := range parts {
		ch = max(ch, p.Channels)
	}
	out := Sample{Channels: ch}
	for _, p := range parts {
		if max(p.Channels, 1) == ch {
			out.Samples = append(out.Samples, p.Samples...)
			continue
		}
		for _, v := range p.Samples {
			for c := 0; c < ch; c++ {
				out.Samples = append(out.Samples, v)
			}
		}
	}
	return out
}

// Settle moves a frame to the onset closest to it, looking up to before frames back and after
// frames ahead; with no onset that near it stays where it is. It is what makes a marker that was
// put down a little late, as a finger is, land on the hit.
func Settle(onsets []int, frame, before, after int) int {
	best, found := frame, false
	for _, o := range onsets {
		if o < frame-before || o > frame+after {
			continue
		}
		if !found || absInt(o-frame) < absInt(best-frame) {
			best, found = o, true
		}
	}
	return best
}
