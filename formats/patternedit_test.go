package formats_test

import (
	"bytes"
	"testing"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

func padOf(t *testing.T, name string) proto.PadIndex {
	t.Helper()
	p, err := proto.ParsePad(name)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestInsertSetAndRemoveANoteChangeOnlyThatNote(t *testing.T) {
	a1, a5 := padOf(t, "A1"), padOf(t, "A5")
	b, err := formats.StartPattern([]proto.PadIndex{a1, padOf(t, "H9")}, 2)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := formats.ParsePattern(b)
	// A note in the middle of a stretch of time: the stretch is split, nothing else moves.
	for _, tick := range []uint32{0, 1, 480, 700, 960, 3839} {
		edited, err := formats.InsertNote(b, tick, a5, 90, 120)
		if err != nil {
			t.Fatalf("tick %d: %v", tick, err)
		}
		notes, err := formats.NoteRecords(edited)
		if err != nil {
			t.Fatal(err)
		}
		var found *formats.NoteRecord
		for i := range notes {
			if notes[i].Pad == a5 {
				found = &notes[i]
			}
		}
		if found == nil || found.Tick != tick || found.Velocity != 90 || found.Length != 120 {
			t.Fatalf("tick %d: the note is %+v", tick, found)
		}
		after, err := formats.ParsePattern(edited)
		if err != nil || after.Length != before.Length || len(after.Events) != len(before.Events)+1 {
			t.Fatalf("tick %d: length %d events %d (was %d, %d), %v", tick, after.Length, len(after.Events), before.Length, len(before.Events), err)
		}
		if !bytes.Equal(after.Trailer, before.Trailer) {
			t.Errorf("tick %d: the trailer changed", tick)
		}
		// The old notes are where they were, with the velocity they had.
		for _, e := range before.Events {
			ok := false
			for _, n := range after.Events {
				ok = ok || n.Pad == e.Pad && n.Tick == e.Tick && n.Velocity == e.Velocity
			}
			if !ok {
				t.Errorf("tick %d: %v at %d is gone", tick, e.Pad, e.Tick)
			}
		}
		// Velocity: only that note.
		if err := formats.SetVelocity(edited, found.Offset, 200); err != nil {
			t.Fatal(err)
		}
		again, _ := formats.NoteRecords(edited)
		for _, n := range again {
			want := uint8(127)
			if n.Pad == a5 {
				want = 127 // 200 is held to 127
			}
			if n.Velocity != want {
				t.Errorf("velocity of %v is %d", n.Pad, n.Velocity)
			}
		}
		if err := formats.SetVelocity(edited, found.Offset, 40); err != nil {
			t.Fatal(err)
		}
		// Removing it gives the pattern it was, in every way that is heard.
		if err := formats.RemoveNote(edited, found.Offset); err != nil {
			t.Fatal(err)
		}
		gone, err := formats.ParsePattern(edited)
		if err != nil || len(gone.Events) != len(before.Events) || gone.Length != before.Length {
			t.Errorf("tick %d: after removing, %d events, length %d, %v", tick, len(gone.Events), gone.Length, err)
		}
	}
	if _, err := formats.InsertNote(b, 3840, a5, 90, 120); err == nil {
		t.Error("a note at the end of the pattern was accepted")
	}
	if err := formats.SetVelocity(b, 3, 10); err == nil {
		t.Error("an offset that is not a record was accepted")
	}
	if err := formats.RemoveNote(b, len(b)-8); err == nil {
		t.Error("something that is not a note was removed")
	}
}
