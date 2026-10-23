package lab

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"

	"github.com/moahbeatlab/404tui/formats"
)

// SlicerConsts are the choices the slicer's controls step through.
func SlicerConsts() ([]byte, error) {
	return json.Marshal(map[string]any{
		"swings": formats.Swings, "humanizes": formats.Humanizes, "rolls": formats.Rolls, "divisions": formats.Divisions,
		"envSteps": formats.EnvSteps, "modes": []string{"order", "random", "no repeat", "shuffle"},
		"rollLoudMax": formats.RollLoudMax, "rollLoudStep": formats.RollLoudStep, "rollPitchMax": formats.RollPitchMax,
		"minSlice": formats.MinSliceFrames, "rate": formats.SampleRate,
	})
}

// SlicerArgs are the arguments of a slicing operation; each operation reads what it needs.
type SlicerArgs struct {
	ID     int     `json:"id"`     // the sample (onsets, grid)
	N      int     `json:"n"`      // a number of slices
	Seed   int64   `json:"seed"`   //
	I      int     `json:"i"`      // a slice
	Frame  int     `json:"frame"`  // a frame
	Delta  int     `json:"delta"`  // a number of frames
	Limit  int     `json:"limit"`  // most onsets
	Sens   float64 `json:"sens"`   // onset sensitivity 0..1
	Step   float64 `json:"step"`   // grid step in frames
	Anchor int     `json:"anchor"` // frame of a grid line, -1 for none
	Starts []int   `json:"starts"` // where joined samples start
	Frames int     `json:"frames"` // length of the sample
}

// Slicing applies one operation to a slicing (JSON in, JSON out). The operations are the ones
// of formats.Slicing: new, divide, random, onsets, grid, split, remove, setStart, setEnd,
// moveStart, moveEnd, reset, validate, clone.
func (l *Lab) Slicing(op string, slicing []byte, args []byte) ([]byte, error) {
	var a SlicerArgs
	a.Anchor = -1
	if len(args) > 0 {
		if err := json.Unmarshal(args, &a); err != nil {
			return nil, err
		}
	}
	var s *formats.Slicing
	if op == "new" {
		if a.Frames < 2 {
			return nil, errors.New("no sample to slice")
		}
		s = formats.NewSlicing(a.Frames, max(a.N, 1))
	} else {
		s = &formats.Slicing{}
		if err := json.Unmarshal(slicing, s); err != nil {
			return nil, err
		}
	}
	switch op {
	case "new", "validate", "clone":
	case "divide":
		s.DivideIn(a.Starts, max(a.N, 1), nil)
	case "random":
		s.DivideIn(a.Starts, max(a.N, 1), rand.New(rand.NewSource(a.Seed)))
	case "onsets":
		smp, err := l.Sample(a.ID)
		if err != nil {
			return nil, err
		}
		starts := smp.Transients(max(a.Limit, 2), a.Sens)
		if len(starts) == 0 {
			return nil, errors.New("no onsets found: try a higher sensitivity")
		}
		s.SetStarts(starts)
	case "grid":
		if !s.DivideGrid(a.Starts, a.Step, a.Anchor) {
			return nil, errors.New("the grid steps are too short to cut in: use a longer step or a lower tempo")
		}
	case "split":
		if s.Split(a.I, a.Frame) < 0 {
			return nil, errors.New("cannot cut there")
		}
	case "remove":
		if !s.Remove(a.I) {
			return nil, errors.New("the last slice stays")
		}
	case "setStart":
		s.SetStart(a.I, a.Frame)
	case "setEnd":
		s.SetEnd(a.I, a.Frame)
	case "moveStart":
		s.MoveStart(a.I, a.Delta)
	case "moveEnd":
		s.MoveEnd(a.I, a.Delta)
	case "reset":
		s.ResetOrder()
	default:
		return nil, fmt.Errorf("unknown slicing operation %q", op)
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(s)
}

// Placed is a step of a rendered sample, for a playhead.
type Placed struct {
	Slice  int  `json:"slice"`
	Rest   bool `json:"rest"`
	At     int  `json:"at"`
	Frames int  `json:"frames"`
	Audio  int  `json:"audio"`
	Lead   int  `json:"lead"`
	Roll   int  `json:"roll"`
}

// Rendered is the result of Render.
type Rendered struct {
	Meta
	Placed []Placed `json:"placed"`
}

// RoundSteps is how many steps make one round of a slicing: the order's length, or one per slice
// for the random modes.
func RoundSteps(s *formats.Slicing) int {
	switch s.Mode {
	case formats.ModeRandom, formats.ModeNoRepeat:
		return len(s.Slices)
	case formats.ModeShuffle:
		n := 0
		for _, sl := range s.Slices {
			if sl.Chance > 0 {
				n++
			}
		}
		return max(n, 1)
	}
	return len(s.Order)
}

// Render plays a slicing for rounds rounds (0: one round) and keeps the result as a sample. round
// is added to the seed, so that the random modes give another stretch each time.
func (l *Lab) Render(id int, slicing []byte, rounds, round int) ([]byte, error) {
	src, err := l.Sample(id)
	if err != nil {
		return nil, err
	}
	s := &formats.Slicing{}
	if err := json.Unmarshal(slicing, s); err != nil {
		return nil, err
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	s.Seed += int64(round)
	steps := RoundSteps(s) * max(rounds, 1)
	if steps > 4096 {
		return nil, errors.New("too many steps")
	}
	out, placed := formats.Render(src, s, formats.NewSequencer(s), steps)
	if out.Frames() == 0 {
		return nil, errors.New("nothing to render: every slice is silent")
	}
	r := Rendered{Meta: l.meta(l.keep(out))}
	for _, p := range placed {
		r.Placed = append(r.Placed, Placed{p.Slice, p.Rest, p.At, p.Frames, p.Audio, p.Lead, p.Roll})
	}
	return json.Marshal(r)
}

// SliceAudio keeps slice i as it sounds (with its pitch, gain, reverse, envelope) as a sample.
func (l *Lab) SliceAudio(id int, slicing []byte, i int) ([]byte, error) {
	src, err := l.Sample(id)
	if err != nil {
		return nil, err
	}
	s := &formats.Slicing{}
	if err := json.Unmarshal(slicing, s); err != nil {
		return nil, err
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if i < 0 || i >= len(s.Slices) {
		return nil, fmt.Errorf("no slice %d", i)
	}
	out := formats.SliceAudio(src, s, i)
	if out.Frames() == 0 {
		return nil, errors.New("that slice is silent")
	}
	return json.Marshal(l.meta(l.keep(out)))
}

// ShuffleOrder puts the order in a random order from a seed.
func ShuffleOrder(slicing []byte, seed int64) ([]byte, error) {
	s := &formats.Slicing{}
	if err := json.Unmarshal(slicing, s); err != nil {
		return nil, err
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	q := formats.NewSequencer(&formats.Slicing{Frames: s.Frames, Slices: s.Slices, Order: s.Order, Mode: formats.ModeShuffle, Seed: seed})
	n := len(s.Order)
	out := make([]int, 0, n)
	for len(out) < n {
		out = append(out, q.Next().Slice)
	}
	s.Order = out
	return json.Marshal(s)
}

// StretchToBars stretches a sample, keeping its pitch, to a whole number of bars at bpm. The
// number of bars is the nearest one; it fails when that is more than 2x away.
func (l *Lab) StretchToBars(id int, bpm float64) ([]byte, error) {
	s, err := l.Sample(id)
	if err != nil {
		return nil, err
	}
	if bpm < 20 || bpm > 300 {
		return nil, errors.New("the tempo is 20 to 300 BPM")
	}
	barFrames := 240 / bpm * formats.SampleRate
	bars := float64(max(int(float64(s.Frames())/barFrames+0.5), 1))
	frames := int(bars*barFrames + 0.5)
	ratio := float64(frames) / float64(max(s.Frames(), 1))
	if ratio < 0.5 || ratio > 2 {
		return nil, fmt.Errorf("%.0f bars at %g BPM would stretch the sample %.2f times: too far to sound right", bars, bpm, ratio)
	}
	return json.Marshal(map[string]any{"bars": bars, "ratio": ratio, "meta": l.meta(l.keep(s.Stretched(frames)))})
}
