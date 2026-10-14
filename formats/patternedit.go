package formats

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/moahbeatlab/404tui/proto"
)

// NoteRecord is a note of a pattern file with where it sits in the bytes, so that it can be
// changed in place.
type NoteRecord struct {
	// Offset is where the record starts in the file.
	Offset   int
	Tick     uint32
	Pad      proto.PadIndex
	Velocity uint8
	Length   uint16
}

// NoteRecords lists the notes of a pattern file with their offsets.
func NoteRecords(b []byte) ([]NoteRecord, error) {
	if len(b) == 0 || len(b)%record != 0 {
		return nil, errors.New("invalid pattern file: length is not a multiple of 8")
	}
	var notes []NoteRecord
	var tick uint32
	for at := 0; at+record <= len(b); at += record {
		r := b[at : at+record]
		switch {
		case r[1] >= noteFirst && r[1] <= noteLast:
			pad, ok := notePad(r)
			if !ok {
				return nil, errors.New("invalid pattern file: note on a pad beyond J16")
			}
			notes = append(notes, NoteRecord{Offset: at, Tick: tick, Pad: pad, Velocity: min(r[4], 127), Length: binary.LittleEndian.Uint16(r[6:])})
		case r[1] == endRecord:
			return notes, nil
		}
		tick += uint32(r[0])
	}
	return notes, nil
}

func noteAt(b []byte, offset int) ([]byte, error) {
	if offset < 0 || offset+record > len(b) || offset%record != 0 {
		return nil, fmt.Errorf("no record at byte %d", offset)
	}
	r := b[offset : offset+record]
	if r[1] < noteFirst || r[1] > noteLast {
		return nil, fmt.Errorf("the record at byte %d is not a note", offset)
	}
	return r, nil
}

// SetVelocity changes the velocity of the note at offset, 1 to 127, changing nothing else.
func SetVelocity(b []byte, offset int, velocity uint8) error {
	r, err := noteAt(b, offset)
	if err != nil {
		return err
	}
	r[4] = min(max(velocity, 1), 127)
	return nil
}

// RemoveNote takes the note at offset out of the pattern by making it a filler record, which
// keeps the time to the next record, so nothing after it moves.
func RemoveNote(b []byte, offset int) error {
	r, err := noteAt(b, offset)
	if err != nil {
		return err
	}
	delta := r[0]
	clear(r)
	r[0], r[1] = delta, 0x80
	return nil
}

// InsertNote returns the pattern with a note on pad at tick, the other records where they
// were: the record that spans the tick is split in two, and the note goes between them.
func InsertNote(b []byte, tick uint32, pad proto.PadIndex, velocity uint8, length uint16) ([]byte, error) {
	if len(b) == 0 || len(b)%record != 0 {
		return nil, errors.New("invalid pattern file: length is not a multiple of 8")
	}
	note := make([]byte, 0, record)
	note = append(note, 0, noteFirst+byte(pad.Index()%groupSize), byte(pad.Index()/groupSize), 0, min(max(velocity, 1), 127), 64)
	note = binary.LittleEndian.AppendUint16(note, length)

	var t uint32
	for at := 0; at+record <= len(b); at += record {
		r := b[at : at+record]
		if r[1] == endRecord {
			if tick >= t {
				return nil, errors.New("that is after the end of the pattern")
			}
			break
		}
		delta := uint32(r[0])
		switch {
		case t == tick:
			// Another record starts at this tick: the note goes before it, with no time of its own.
			return splice(b, at, note), nil
		case t < tick && tick < t+delta:
			// The record spans the tick: it gets the time up to it, the note the rest.
			out := append([]byte(nil), b...)
			out[at] = byte(tick - t)
			note[0] = byte(t + delta - tick)
			return splice(out, at+record, note), nil
		}
		t += delta
	}
	return nil, errors.New("no place for a note there")
}

func splice(b []byte, at int, rec []byte) []byte {
	out := make([]byte, 0, len(b)+record)
	out = append(out, b[:at]...)
	out = append(out, rec...)
	return append(out, b[at:]...)
}

// SetLength changes how long the note at offset lasts, in ticks, changing nothing else.
func SetLength(b []byte, offset int, length uint16) error {
	r, err := noteAt(b, offset)
	if err != nil {
		return err
	}
	binary.LittleEndian.PutUint16(r[6:], max(length, 1))
	return nil
}
