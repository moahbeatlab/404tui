package lab

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// Note is a note of a pattern.
type Note struct {
	Tick  uint32 `json:"tick"`
	Pad   int    `json:"pad"`
	Vel   int    `json:"vel"`
	Len   int    `json:"len"`
	Pitch int    `json:"pitch"` // 0 plain; anything else is a chromatic note
}

// PatternDoc is a pattern for the editor.
type PatternDoc struct {
	N        int    `json:"n"`
	Label    string `json:"label"`
	Length   uint32 `json:"length"`
	Bars     int    `json:"bars"`
	Beats    int    `json:"beats"`
	PPQ      int    `json:"ppq"`
	Notes    []Note `json:"notes"`
	Controls int    `json:"controls"`
}

func (l *Lab) pattern(n int) ([]byte, error) {
	if n < 1 || n > 160 {
		return nil, fmt.Errorf("no pattern %d", n)
	}
	b, ok := l.files[patFile(n)]
	if !ok {
		return nil, fmt.Errorf("pattern %s is empty", padLabel(n-1))
	}
	return b, nil
}

func padOf(n int) (proto.PadIndex, error) {
	p, ok := proto.NewPadIndex(uint16(n))
	if !ok {
		return proto.PadIndex{}, fmt.Errorf("no pad %d", n)
	}
	return p, nil
}

// Pattern reads pattern n (1..160) for editing.
func (l *Lab) Pattern(n int) ([]byte, error) {
	b, err := l.pattern(n)
	if err != nil {
		return nil, err
	}
	p, err := formats.ParsePattern(b)
	if err != nil {
		return nil, err
	}
	d := PatternDoc{N: n, Label: padLabel(n - 1), Length: p.Length, Beats: p.BeatsPerBar(), PPQ: formats.PPQ, Notes: []Note{}}
	d.Bars = int(p.Length) / (d.Beats * formats.PPQ)
	for _, e := range p.Events {
		if e.Control {
			d.Controls++
			continue
		}
		d.Notes = append(d.Notes, Note{e.Tick, int(e.Pad.Index()), int(e.Velocity), int(e.Length), int(e.Pitch)})
	}
	return json.Marshal(d)
}

// NewPattern makes pattern n an empty one of bars bars (replacing what was there).
func (l *Lab) NewPattern(n, bars int) error {
	if l.pad == nil {
		return errors.New("there is no project")
	}
	if n < 1 || n > 160 {
		return fmt.Errorf("no pattern %d", n)
	}
	b, err := formats.EmptyPattern(bars)
	if err != nil {
		return err
	}
	l.files[patFile(n)] = b
	return nil
}

// DeletePattern removes pattern n.
func (l *Lab) DeletePattern(n int) error {
	if n < 1 || n > 160 {
		return fmt.Errorf("no pattern %d", n)
	}
	delete(l.files, patFile(n))
	return nil
}

// CopyPattern copies pattern from onto to.
func (l *Lab) CopyPattern(from, to int) error {
	b, err := l.pattern(from)
	if err != nil {
		return err
	}
	if to < 1 || to > 160 {
		return fmt.Errorf("no pattern %d", to)
	}
	l.files[patFile(to)] = append([]byte(nil), b...)
	return nil
}

// PatternArgs are the arguments of a pattern operation.
type PatternArgs struct {
	Pad      int     `json:"pad"` // -1 or absent: every pad
	Grid     uint32  `json:"grid"`
	Strength float64 `json:"strength"`
	Amount   float64 `json:"amount"`
	Steps    int     `json:"steps"`
	Timing   uint32  `json:"timing"`
	Velocity int     `json:"velocity"`
	Seed     int64   `json:"seed"`
	Tick     uint32  `json:"tick"`
	Len      int     `json:"len"`
	Move     int     `json:"move"`
	Vel      int     `json:"vel"`
	Bars     int     `json:"bars"`
	Delete   bool    `json:"delete"`
	Notes    []PadAt `json:"notes"` // notes chosen by pad and start tick
	Tighten  []int   `json:"tighten"`
}

// PadAt names a note by its pad and tick.
type PadAt struct {
	Pad  int    `json:"pad"`
	Tick uint32 `json:"tick"`
}

// PatternOp applies one operation to pattern n. The operations: quantize, swing, humanize,
// shift, reverse, double, extend, halve, insert, edit (velocity, length, move, snap, delete of
// chosen notes), clearPad.
func (l *Lab) PatternOp(n int, op string, args []byte) error {
	b, err := l.pattern(n)
	if err != nil {
		return err
	}
	a := PatternArgs{Pad: -1}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &a); err != nil {
			return err
		}
	}
	var scope formats.Scope
	if a.Pad >= 0 {
		p, err := padOf(a.Pad)
		if err != nil {
			return err
		}
		scope = formats.OnlyPad(p)
	}
	var out []byte
	switch op {
	case "quantize":
		out, err = formats.Quantize(b, a.Grid, a.Strength, scope)
	case "swing":
		out, err = formats.Swing(b, a.Grid, a.Amount, scope)
	case "humanize":
		tighten := map[uint16]bool{}
		for _, t := range a.Tighten {
			tighten[uint16(t)] = true
		}
		out, err = formats.Humanize(b, a.Timing, uint8(min(max(a.Velocity, 0), 64)),
			func(p proto.PadIndex) bool { return tighten[p.Index()] }, scope, rand.New(rand.NewSource(a.Seed)))
	case "shift":
		out, err = formats.Shift(b, a.Grid, a.Steps, scope)
	case "reverse":
		out, err = formats.Reverse(b, scope)
	case "double":
		out, err = formats.DoublePattern(b)
	case "extend":
		out, err = formats.ExtendPattern(b)
	case "halve":
		out, err = formats.HalvePattern(b)
	case "insert":
		p, e := padOf(a.Pad)
		if e != nil {
			return e
		}
		out, err = formats.InsertNote(b, a.Tick, p, uint8(min(max(a.Vel, 1), 127)), uint16(min(max(a.Len, 1), 65535)))
	case "edit":
		set := map[formats.PadTick]bool{}
		for _, x := range a.Notes {
			p, e := padOf(x.Pad)
			if e != nil {
				return e
			}
			set[formats.PadTick{Pad: p, Tick: x.Tick}] = true
		}
		out, _, err = formats.EditNoteSet(b, set, formats.NoteEdit{Velocity: a.Velocity, SetVelocity: a.Vel, Length: 0,
			SetLength: a.Len, Move: a.Move, Snap: a.Grid, Delete: a.Delete})
	case "clearPad":
		p, e := padOf(a.Pad)
		if e != nil {
			return e
		}
		pat, e := formats.ParsePattern(b)
		if e != nil {
			return e
		}
		set := map[formats.PadTick]bool{}
		for _, ev := range pat.Events {
			if !ev.Control && ev.Pad == p {
				set[formats.PadTick{Pad: p, Tick: ev.Tick}] = true
			}
		}
		if len(set) == 0 {
			return nil
		}
		out, _, err = formats.EditNoteSet(b, set, formats.NoteEdit{Delete: true})
	default:
		return fmt.Errorf("unknown pattern operation %q", op)
	}
	if err != nil {
		return err
	}
	if _, err := formats.ParsePattern(out); err != nil {
		return err
	}
	l.files[patFile(n)] = out
	return nil
}

// RhythmNames are the rhythms a progression can be played in.
var RhythmNames = []string{"hold", "quarters", "offbeat", "eighths", "charleston", "3+3+2", "stabs"}

// rhythms: where the chord sounds in a bar of 16 steps, and how long each hit lasts (in steps).
var rhythms = map[string][][2]int{
	"hold":       {{0, 16}},
	"quarters":   {{0, 3}, {4, 3}, {8, 3}, {12, 3}},
	"offbeat":    {{2, 2}, {6, 2}, {10, 2}, {14, 2}},
	"eighths":    {{0, 2}, {2, 2}, {4, 2}, {6, 2}, {8, 2}, {10, 2}, {12, 2}, {14, 2}},
	"charleston": {{0, 6}, {6, 3}},
	"3+3+2":      {{0, 3}, {3, 3}, {6, 3}, {9, 3}, {12, 2}},
	"stabs":      {{0, 1}, {3, 1}, {6, 1}, {10, 1}, {12, 1}},
}

// ProgressionSpec is a chord progression to write as a pattern.
type ProgressionSpec struct {
	// Pads are the chord pads by degree 1..7 (index 0 is degree 1): where each chord of the kit sits.
	Pads []int `json:"pads"`
	// Degrees is the progression, one chord each.
	Degrees []int `json:"degrees"`
	// Bars is the length of the pattern; the chords share it equally (the progression repeats
	// when there are more chords than fit, and each chord lasts longer when there are fewer).
	Bars     int    `json:"bars"`
	Rhythm   string `json:"rhythm"`
	Velocity int    `json:"velocity"`
	Seed     int64  `json:"seed"`
	// Humanize is 0..100: how much the velocities and starts wander.
	Humanize int `json:"humanize"`
}

// WriteProgression writes pattern n: the chords of a progression, one after the other, on the
// pads of a chord kit.
func (l *Lab) WriteProgression(n int, spec []byte) error {
	var s ProgressionSpec
	if err := json.Unmarshal(spec, &s); err != nil {
		return err
	}
	if l.pad == nil {
		return errors.New("there is no project")
	}
	if len(s.Degrees) == 0 || len(s.Degrees) > 16 {
		return errors.New("a progression has 1 to 16 chords")
	}
	if s.Rhythm == "" {
		s.Rhythm = "hold"
	}
	hits, ok := rhythms[s.Rhythm]
	if !ok {
		return fmt.Errorf("unknown rhythm %q", s.Rhythm)
	}
	if s.Bars < 1 {
		s.Bars = len(s.Degrees)
	}
	if s.Velocity == 0 {
		s.Velocity = 100
	}
	b, err := formats.EmptyPattern(s.Bars)
	if err != nil {
		return err
	}
	total := uint32(s.Bars * 4 * formats.PPQ)
	per := total / uint32(len(s.Degrees))
	if s.Bars < len(s.Degrees) && total%uint32(len(s.Degrees)) != 0 {
		return errors.New("that many chords do not fit that many bars evenly")
	}
	rng := rand.New(rand.NewSource(s.Seed))
	for i, deg := range s.Degrees {
		d := (deg-1)%7 + 1
		if deg < 1 || d > len(s.Pads) {
			return fmt.Errorf("no chord pad for degree %d", deg)
		}
		pad, err := padOf(s.Pads[d-1])
		if err != nil {
			return err
		}
		start := uint32(i) * per
		// The rhythm fits one bar, or the chord when it is shorter; a longer chord repeats it each bar.
		chunk := min(per, uint32(4*formats.PPQ))
		step := chunk / 16
		if step == 0 {
			return errors.New("that is too many chords for the pattern")
		}
		for c := uint32(0); c*chunk < per; c++ {
			for _, h := range hits {
				tick := start + c*chunk + uint32(h[0])*step
				length := max(int(float64(h[1]*int(step))*0.92), 1)
				if s.Rhythm == "hold" {
					length = int(per)
				}
				vel := s.Velocity
				if s.Humanize > 0 && s.Rhythm != "hold" {
					tick += uint32(rng.Intn(max(int(step)*s.Humanize/400, 1)))
					vel += rng.Intn(2*s.Humanize/5+1) - s.Humanize/5
				}
				if tick >= total {
					continue
				}
				length = min(length, int(total-tick))
				b, err = formats.InsertNote(b, tick, pad, uint8(min(max(vel, 1), 127)), uint16(min(length, 65535)))
				if err != nil {
					return err
				}
			}
			if s.Rhythm == "hold" {
				break
			}
		}
	}
	if _, err := formats.ParsePattern(b); err != nil {
		return err
	}
	l.files[patFile(n)] = b
	return nil
}

// PatternBytes is the file of pattern n as it is (for undo); empty when there is none.
func (l *Lab) PatternBytes(n int) ([]byte, error) {
	if n < 1 || n > 160 {
		return nil, fmt.Errorf("no pattern %d", n)
	}
	return append([]byte(nil), l.files[patFile(n)]...), nil
}

// SetPatternBytes puts a pattern file back (from PatternBytes); an empty one removes the pattern.
func (l *Lab) SetPatternBytes(n int, b []byte) error {
	if n < 1 || n > 160 {
		return fmt.Errorf("no pattern %d", n)
	}
	if len(b) == 0 {
		delete(l.files, patFile(n))
		return nil
	}
	if _, err := formats.ParsePattern(b); err != nil {
		return err
	}
	l.files[patFile(n)] = append([]byte(nil), b...)
	return nil
}
