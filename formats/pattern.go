package formats

import (
	"encoding/binary"
	"errors"

	"github.com/moahbeatlab/404tui/proto"
)

// PPQ is the ticks per quarter note of a pattern.
const PPQ = 480

const (
	record    = 8
	noteFirst = 0x2F
	noteLast  = 0x7F
	endRecord = 0x8C
	control   = 0x8E
	// A pad's note number is its position within a group of five banks' worth... of 80 pads.
	groupSize = 5 * 16
)

// Event is one thing a pattern does: a Note or a Control change.
type Event struct {
	Tick uint32
	// Note fields, when Control is false.
	Pad proto.PadIndex
	// Pitch is byte 3: 0 for a plain note, otherwise the chromatic pitch value.
	Pitch       uint8
	Velocity    uint8
	OffVelocity uint8
	// Length is the gate length in ticks.
	Length uint16
	// Flags are byte 2 bits other than the pad group (meaning unknown).
	Flags uint8

	// Control fields, when Control is true.
	Control    bool
	Channel    uint8
	Controller uint8
	Value      uint8
}

// Pattern is a parsed PTNnnnnn.BIN.
type Pattern struct {
	Events []Event
	// Length is the tick of the end record: the pattern length.
	Length uint32
	// Trailer is the 8 bytes after the end record, when present.
	Trailer []byte
}

// ParsePattern parses a PTNnnnnn.BIN file; the layout is in docs/re/04-patterns.md.
func ParsePattern(b []byte) (Pattern, error) {
	if len(b) == 0 || len(b)%record != 0 {
		return Pattern{}, errors.New("invalid pattern file: length is not a multiple of 8")
	}
	var p Pattern
	var tick uint32
	for at := 0; at+record <= len(b); at += record {
		r := b[at : at+record]
		switch {
		case r[1] >= noteFirst && r[1] <= noteLast:
			pad, ok := notePad(r)
			if !ok {
				return Pattern{}, errors.New("invalid pattern file: note on a pad beyond J16")
			}
			p.Events = append(p.Events, Event{
				Tick: tick, Pad: pad, Pitch: r[3], Velocity: min(r[4], 127), OffVelocity: r[5],
				Length: binary.LittleEndian.Uint16(r[6:]), Flags: r[2] & 0xF0,
			})
		case r[1] == control:
			p.Events = append(p.Events, Event{Tick: tick, Control: true, Channel: r[2] & 0x0F, Controller: r[5], Value: r[6]})
		case r[1] == endRecord:
			p.Length = tick
			if at+2*record <= len(b) {
				p.Trailer = append([]byte(nil), b[at+record:at+2*record]...)
			}
			return p, nil
		}
		tick += uint32(r[0])
	}
	p.Length = tick
	return p, nil
}

// notePad is the pad a note record plays.
func notePad(r []byte) (proto.PadIndex, bool) {
	index := uint16(r[2]&0x0F)*groupSize + uint16(r[1]-noteFirst)
	return proto.NewPadIndex(index)
}

// Slice is the part of the pattern from tick from up to tick to, as a pattern of its own: the
// events in it moved to start at 0, and that part's length. A note begun before from is left
// out, even if it sounds on into the part.
func (p Pattern) Slice(from, to uint32) Pattern {
	out := Pattern{Length: to - from, Trailer: p.Trailer}
	for _, e := range p.Events {
		if e.Tick >= from && e.Tick < to {
			e.Tick -= from
			out.Events = append(out.Events, e)
		}
	}
	return out
}

// BeatsPerBar is the beats per bar from the trailer (the denominator is always 4); 4 when
// the pattern has no trailer.
func (p Pattern) BeatsPerBar() int {
	if len(p.Trailer) < 5 {
		return 4
	}
	switch code := int(p.Trailer[4]); {
	case code >= 1 && code <= 3:
		return code
	case code >= 4 && code <= 6:
		return code + 1
	}
	return 4
}

// RetargetBank rewrites every note in b (a raw PTNnnnnn.BIN) that plays a pad in fromBank to
// play the pad in toBank at the same position within the bank, leaving every other byte
// (deltas, control events, filler records, the trailer) untouched. It returns the number
// of notes changed.
//
// It patches the two bytes that encode a note's pad in place rather than rebuilding the
// file from a parsed Pattern, so a pattern this package does not fully understand still
// round-trips byte for byte outside the notes retargeted.
func RetargetBank(b []byte, fromBank, toBank uint16) int {
	if len(b)%record != 0 {
		return 0
	}
	changed := 0
	for at := 0; at+record <= len(b); at += record {
		r := b[at : at+record]
		if r[1] < noteFirst || r[1] > noteLast {
			continue
		}
		pad, ok := notePad(r)
		if !ok || pad.Bank() != fromBank {
			continue
		}
		moved, ok := proto.PadFromBankPad(toBank, pad.Pad())
		if !ok {
			continue
		}
		r[1] = noteFirst + byte(moved.Index()%groupSize)
		r[2] = r[2]&0xF0 | byte(moved.Index()/groupSize)&0x0F
		changed++
	}
	return changed
}

// MaxStartBars is the longest pattern StartPattern builds.
const MaxStartBars = 8

// StartPattern builds a PTNnnnnn.BIN of bars bars of 4/4 in which every one of pads is
// struck on the first tick and held to the end: the loops of a bank started together.
//
// The records follow docs/re/04-patterns.md of SparkyMK2. Every pattern file seen so far
// was 4 bars long with 4 in the trailer's first byte, so writing bars there is a guess.
func StartPattern(pads []proto.PadIndex, bars int) ([]byte, error) {
	if len(pads) == 0 {
		return nil, errors.New("a pattern needs at least one pad")
	}
	if bars < 1 || bars > MaxStartBars {
		return nil, errors.New("a pattern is 1 to 8 bars long")
	}
	length := bars * 4 * PPQ
	b := make([]byte, 0, (len(pads)+length/255+3)*record)
	for _, p := range pads {
		b = append(b, 0, noteFirst+byte(p.Index()%groupSize), byte(p.Index()/groupSize), 0, 127, 64)
		b = binary.LittleEndian.AppendUint16(b, uint16(length))
	}
	// Filler records carry the time to the end, 255 ticks at most each.
	for left := length; left > 0; {
		step := min(left, 255)
		b = append(b, byte(step), 0x80, 0, 0, 0, 0, 0, 0)
		left -= step
	}
	b = append(b, 0, endRecord, 0, 0, 0, 0, 0, 0)
	return append(b, byte(bars), 0, 0, 0, 0, 0x80, 0x04, 0x01), nil
}

// PatternPath is the card path of a pattern slot's file in a project (1-based project).
func PatternPath(project uint8, slot proto.PadIndex) string {
	return projectDir(project) + "/PTN/PTN" + pad5(int(slot.Index())+1) + ".BIN"
}

func projectDir(project uint8) string {
	return "ROLAND/SP-404MKII/PROJECT_" + pad2(int(project))
}

func pad2(n int) string {
	return string([]byte{byte('0' + n/10%10), byte('0' + n%10)})
}

func pad5(n int) string {
	b := make([]byte, 5)
	for i := 4; i >= 0; i-- {
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b)
}
