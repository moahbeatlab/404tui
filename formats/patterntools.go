package formats

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"

	"github.com/moahbeatlab/404tui/proto"
)

// Pattern tools move notes around in time. The file is delta-encoded (byte 0 of every record
// is the ticks since the record before it), so a tool that shifts a note has to recompute the
// whole stream of deltas. These tools do that by reading the records into a list keyed on the
// absolute tick, letting a transform change the note ticks, then writing the delta stream back
// out — control events, note bytes (pad, pitch, velocities, length, flags), the end record and
// the trailer are all carried through untouched, so a file this package does not fully
// understand still round-trips outside the ticks a tool moved.

// patRecord is one 8-byte record read out of a pattern, with its absolute tick.
type patRecord struct {
	tick uint32
	// bytes is the record other than its delta (bytes 1..7); for a note the pad, pitch and
	// velocities, kept as they were.
	bytes [record - 1]byte
	note  bool
}

// readRecords reads every record of a pattern up to the end record into absolute-tick form.
// It returns the records (notes and controls, in file order), the end tick (pattern length),
// and the trailer bytes after the end record. A file whose length is not a multiple of 8, or
// that has no end record, is rejected so a tool never writes a malformed file.
func readRecords(b []byte) (recs []patRecord, length uint32, trailer []byte, err error) {
	if len(b) == 0 || len(b)%record != 0 {
		return nil, 0, nil, errors.New("invalid pattern file: length is not a multiple of 8")
	}
	var tick uint32
	for at := 0; at+record <= len(b); at += record {
		r := b[at : at+record]
		if r[1] == endRecord {
			length = tick
			if at+2*record <= len(b) {
				trailer = append([]byte(nil), b[at+record:at+2*record]...)
			}
			return recs, length, trailer, nil
		}
		isNote := r[1] >= noteFirst && r[1] <= noteLast
		isControl := r[1] == control
		if isNote || isControl {
			var rec patRecord
			rec.tick = tick
			copy(rec.bytes[:], r[1:])
			rec.note = isNote
			recs = append(recs, rec)
		}
		// Filler records (0x80) carry only time; their delta is folded into the next record's
		// absolute tick and they are not kept.
		tick += uint32(r[0])
	}
	return nil, 0, nil, errors.New("invalid pattern file: no end record")
}

// writeRecords serialises records back to a pattern file. The records are sorted by tick so
// deltas never go negative; a gap larger than 255 ticks is bridged with filler records. The
// end record sits at length and the trailer follows it.
//
// A record's delta byte is the time from that record to the next one (as ParsePattern reads
// it: it accumulates the delta after reading the record), so simultaneous notes carry delta 0
// and the gap after a run of notes is carried by the last of them or by filler.
func writeRecords(recs []patRecord, length uint32, trailer []byte) []byte {
	sort.SliceStable(recs, func(i, j int) bool { return recs[i].tick < recs[j].tick })
	for i := range recs {
		if recs[i].tick > length {
			recs[i].tick = length // never past the end
		}
	}
	out := make([]byte, 0, (len(recs)+int(length)/255+3)*record)
	// emitFiller carries a gap of time with filler records, each at most 255 ticks.
	emitFiller := func(gap uint32) {
		for gap > 0 {
			step := min(gap, 255)
			out = append(out, byte(step), 0x80, 0, 0, 0, 0, 0, 0)
			gap -= step
		}
	}
	var at uint32 // absolute tick reached so far
	for _, r := range recs {
		emitFiller(r.tick - at)
		at = r.tick
		out = append(out, 0) // delta 0: the time is carried by the filler before it
		out = append(out, r.bytes[:]...)
	}
	// Carry the remaining time to the end record with filler, then the end record.
	emitFiller(length - at)
	out = append(out, 0, endRecord, 0, 0, 0, 0, 0, 0)
	return append(out, trailer...)
}

// forEachNote calls f with the index and absolute tick of every note record, letting it return
// the new tick. Control records are left where they are. in, when not nil, limits f to the
// notes whose pad it accepts, so a tool can work on one track and leave the rest alone.
func mapNotes(recs []patRecord, in Scope, f func(i int, tick uint32) uint32) {
	for i := range recs {
		if recs[i].note && in.has(recs[i]) {
			recs[i].tick = f(i, recs[i].tick)
		}
	}
}

// Scope is which notes a pattern tool works on: nil (the zero value) is the whole pattern, a
// pad predicate is one track. Swing, quantize, shift, reverse and humanize all take one, so a
// tool can move every track together or just the pads of the row you are on.
type Scope func(proto.PadIndex) bool

// WholePattern is the scope of every note.
var WholePattern Scope = nil

// OnlyPad is the scope of the notes on one pad.
func OnlyPad(pad proto.PadIndex) Scope {
	return func(p proto.PadIndex) bool { return p == pad }
}

// has reports whether the scope takes a note record's pad.
func (s Scope) has(r patRecord) bool {
	if s == nil {
		return true
	}
	pad, ok := notePad(append([]byte{0}, r.bytes[:]...))
	return ok && s(pad)
}

// clampTick keeps a tick inside [0, length].
func clampTick(t int64, length uint32) uint32 {
	if t < 0 {
		return 0
	}
	if t > int64(length) {
		return length
	}
	return uint32(t)
}

// Quantize snaps notes to the nearest grid step. grid is the step length in ticks (e.g. PPQ/4
// for sixteenth notes); strength 0..1 is how far toward the grid each note moves (1 is full
// snap). in scopes it to a track or the whole pattern. It returns the pattern quantized.
func Quantize(b []byte, grid uint32, strength float64, in Scope) ([]byte, error) {
	if grid == 0 {
		return nil, errors.New("the grid step must be at least one tick")
	}
	strength = clamp01(strength)
	recs, length, trailer, err := readRecords(b)
	if err != nil {
		return nil, err
	}
	mapNotes(recs, in, func(_ int, tick uint32) uint32 {
		nearest := int64(math.Round(float64(tick)/float64(grid))) * int64(grid)
		moved := float64(tick) + (float64(nearest)-float64(tick))*strength
		return clampTick(int64(math.Round(moved)), length)
	})
	return writeRecords(recs, length, trailer), nil
}

// Swing delays the off-beat grid steps, the MPC way: with grid as the swing resolution (PPQ/4
// for sixteenths, PPQ/2 for eighths), every note landing on an odd step is pushed later. amount
// is the swing percentage 50..75; 50 is straight, 66 is a triplet feel, 75 the hard limit. It
// returns the pattern swung.
func Swing(b []byte, grid uint32, amount float64, in Scope) ([]byte, error) {
	if grid == 0 {
		return nil, errors.New("the swing resolution must be at least one tick")
	}
	amount = math.Max(50, math.Min(75, amount))
	// At 50% an off-beat step sits halfway between on-beats; at amount% it sits amount% of the
	// way, so it is pushed by (amount-50)% of the step spacing (two grid steps).
	push := (amount - 50) / 100 * float64(2*grid)
	recs, length, trailer, err := readRecords(b)
	if err != nil {
		return nil, err
	}
	mapNotes(recs, in, func(_ int, tick uint32) uint32 {
		step := int64(math.Round(float64(tick) / float64(grid)))
		if step%2 == 0 {
			return tick // on the beat, unmoved
		}
		return clampTick(int64(math.Round(float64(tick)+push)), length)
	})
	return writeRecords(recs, length, trailer), nil
}

// Humanize nudges each note's timing and velocity by a small random amount, to loosen a stiff
// programmed feel. timing is the most a note moves in ticks and velocity the most its velocity
// changes; both are symmetric around zero. tighten names pads (kick and the like) that get half
// the timing jitter so the groove's anchor stays put. in scopes it to a track or the whole
// pattern. rng is the source of randomness so the result can be made repeatable in a test. It
// returns the pattern humanized.
func Humanize(b []byte, timing uint32, velocity uint8, tighten func(proto.PadIndex) bool, in Scope, rng *rand.Rand) ([]byte, error) {
	recs, length, trailer, err := readRecords(b)
	if err != nil {
		return nil, err
	}
	for i := range recs {
		if !recs[i].note || !in.has(recs[i]) {
			continue
		}
		spread := int64(timing)
		if tighten != nil {
			if pad, ok := notePad(append([]byte{0}, recs[i].bytes[:]...)); ok && tighten(pad) {
				spread /= 2
			}
		}
		if spread > 0 {
			jitter := rng.Int63n(2*spread+1) - spread
			recs[i].tick = clampTick(int64(recs[i].tick)+jitter, length)
		}
		if velocity > 0 {
			v := int(recs[i].bytes[3]) + rng.Intn(int(2*velocity)+1) - int(velocity)
			recs[i].bytes[3] = byte(min(max(v, 1), 127))
		}
	}
	return writeRecords(recs, length, trailer), nil
}

// Shift moves notes later (steps > 0) or earlier (steps < 0) by steps grid steps, keeping them
// inside the pattern. in scopes it to a track or the whole pattern. It returns the pattern
// shifted.
func Shift(b []byte, grid uint32, steps int, in Scope) ([]byte, error) {
	if grid == 0 {
		return nil, errors.New("the step must be at least one tick")
	}
	recs, length, trailer, err := readRecords(b)
	if err != nil {
		return nil, err
	}
	by := int64(steps) * int64(grid)
	mapNotes(recs, in, func(_ int, tick uint32) uint32 {
		return clampTick(int64(tick)+by, length)
	})
	return writeRecords(recs, length, trailer), nil
}

// MoveNote nudges the notes of pad at tick from by ticks (earlier when negative), keeping them
// inside the pattern; every other record, and every byte of the notes but their time, stays as
// it was. It is the hand-made counterpart of Shift, for one note. It returns the pattern with
// the note moved, or an error when pad has no note at from.
func MoveNote(b []byte, pad proto.PadIndex, from uint32, ticks int) ([]byte, error) {
	recs, length, trailer, err := readRecords(b)
	if err != nil {
		return nil, err
	}
	if length == 0 {
		return nil, errors.New("the pattern has no length to move a note in")
	}
	in, moved := OnlyPad(pad), false
	for i := range recs {
		if recs[i].note && recs[i].tick == from && in.has(recs[i]) {
			recs[i].tick = clampTick(int64(from)+int64(ticks), length-1)
			moved = true
		}
	}
	if !moved {
		return nil, errors.New("no note of that pad there to move")
	}
	return writeRecords(recs, length, trailer), nil
}

// Reverse mirrors the notes about the middle of the pattern: a note at tick t lands at
// length-t, so the pattern plays backwards while keeping each note's pad, velocity and length.
// in scopes it to a track or the whole pattern. It returns the reversed pattern.
func Reverse(b []byte, in Scope) ([]byte, error) {
	recs, length, trailer, err := readRecords(b)
	if err != nil {
		return nil, err
	}
	mapNotes(recs, in, func(_ int, tick uint32) uint32 {
		return clampTick(int64(length)-int64(tick), length)
	})
	return writeRecords(recs, length, trailer), nil
}

// MaxPatternBars is the longest pattern DoublePattern makes.
const MaxPatternBars = 64

// patternBars is how many whole bars a pattern of length ticks is, at the beats per bar of its
// trailer, and the ticks of one bar.
func patternBars(b []byte, length uint32) (bars int, barTicks uint32, err error) {
	p, err := ParsePattern(b)
	if err != nil {
		return 0, 0, err
	}
	barTicks = uint32(p.BeatsPerBar()) * PPQ
	if length == 0 || length%barTicks != 0 {
		return 0, 0, errors.New("the pattern is not a whole number of bars")
	}
	return int(length / barTicks), barTicks, nil
}

// withBars is the trailer with its bar count (byte 0) changed from bars to now. A trailer whose
// first byte is not the bar count the pattern has is left as it is, since then it is not one.
func withBars(trailer []byte, bars, now int) []byte {
	out := append([]byte(nil), trailer...)
	if len(out) > 0 && int(out[0]) == bars {
		out[0] = byte(now)
	}
	return out
}

// DoublePattern makes the pattern twice as long by playing it again after itself: every note
// and control is copied one pattern length later, so a 2-bar pattern becomes 4 bars whose second
// half can then be varied. It returns the doubled pattern.
func DoublePattern(b []byte) ([]byte, error) {
	recs, length, trailer, err := readRecords(b)
	if err != nil {
		return nil, err
	}
	bars, _, err := patternBars(b, length)
	if err != nil {
		return nil, err
	}
	if bars*2 > MaxPatternBars {
		return nil, fmt.Errorf("%d bars doubled is more than the %d a pattern can be", bars, MaxPatternBars)
	}
	for _, r := range recs[:len(recs):len(recs)] {
		r.tick += length
		recs = append(recs, r)
	}
	return writeRecords(recs, 2*length, withBars(trailer, bars, 2*bars)), nil
}

// ExtendPattern makes the pattern twice as long with nothing in the new half: the notes stay
// where they are and the second half is silent, for a loop that is twice as long as the
// pattern. It returns the extended pattern.
func ExtendPattern(b []byte) ([]byte, error) {
	recs, length, trailer, err := readRecords(b)
	if err != nil {
		return nil, err
	}
	bars, _, err := patternBars(b, length)
	if err != nil {
		return nil, err
	}
	if bars*2 > MaxPatternBars {
		return nil, fmt.Errorf("%d bars doubled is more than the %d a pattern can be", bars, MaxPatternBars)
	}
	return writeRecords(recs, 2*length, withBars(trailer, bars, 2*bars)), nil
}

// HalvePattern keeps the first half of the pattern: it is half as long, and what played in the
// second half is gone. A pattern of one bar, or an odd number of bars, cannot be halved into whole
// bars. It returns the halved pattern.
func HalvePattern(b []byte) ([]byte, error) {
	recs, length, trailer, err := readRecords(b)
	if err != nil {
		return nil, err
	}
	bars, _, err := patternBars(b, length)
	if err != nil {
		return nil, err
	}
	switch {
	case bars == 1:
		return nil, errors.New("one bar is as short as a pattern gets")
	case bars%2 == 1:
		return nil, fmt.Errorf("%d bars cannot be halved into whole bars", bars)
	}
	half := length / 2
	kept := recs[:0]
	for _, r := range recs {
		if r.tick < half {
			kept = append(kept, r)
		}
	}
	return writeRecords(kept, half, withBars(trailer, bars, bars/2)), nil
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

// NoteEdit is a change made to several notes at once: each field that is set is applied to every
// note chosen, and the rest of the note stays as it was.
type NoteEdit struct {
	// Velocity is added to each note's, and kept within 1..127; SetVelocity, when above 0, sets it.
	Velocity    int
	SetVelocity int
	// Length is added to each note's length in ticks, never under 1 tick nor past the end.
	Length int
	// Move shifts the notes in time by that many ticks, within the pattern.
	Move int
	// Snap puts each note on the nearest step of a grid of that many ticks.
	Snap uint32
	// Delete takes the notes out.
	Delete bool
}

// EditNotes applies edit to the notes of pad that start at one of ticks, and returns the pattern;
// everything else in it stays as it was. It says how many notes it changed.
func EditNotes(b []byte, pad proto.PadIndex, ticks map[uint32]bool, edit NoteEdit) ([]byte, int, error) {
	recs, length, trailer, err := readRecords(b)
	if err != nil {
		return nil, 0, err
	}
	in := OnlyPad(pad)
	changed := 0
	out := recs[:0:0]
	for _, r := range recs {
		if !r.note || !in.has(r) || !ticks[r.tick] {
			out = append(out, r)
			continue
		}
		changed++
		if edit.Delete {
			continue
		}
		// The bytes of a note after its delta: 3 is the velocity, 5 and 6 the length.
		if edit.SetVelocity > 0 {
			r.bytes[3] = byte(min(max(edit.SetVelocity, 1), 127))
		} else if edit.Velocity != 0 {
			r.bytes[3] = byte(min(max(int(min(r.bytes[3], 127))+edit.Velocity, 1), 127))
		}
		if edit.Length != 0 {
			n := int(r.bytes[5]) | int(r.bytes[6])<<8
			n = min(max(n+edit.Length, 1), 65535)
			if room := int(length) - int(r.tick); room >= 1 {
				n = min(n, room)
			}
			r.bytes[5], r.bytes[6] = byte(n), byte(n>>8)
		}
		tick := int64(r.tick)
		if edit.Snap > 0 {
			g := int64(edit.Snap)
			tick = (tick + g/2) / g * g
		}
		tick += int64(edit.Move)
		r.tick = clampTick(tick, max(length, 1)-1)
		out = append(out, r)
	}
	if changed == 0 {
		return nil, 0, errors.New("none of those notes is in the pattern any more")
	}
	return writeRecords(out, length, trailer), changed, nil
}
