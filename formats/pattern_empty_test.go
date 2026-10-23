package formats

import (
	"slices"
	"testing"

	"github.com/moahbeatlab/404tui/proto"
)

func TestEmptyPatternMatchesStartPatternWithoutNotes(t *testing.T) {
	for _, bars := range []int{1, 2, 4, 8} {
		b, err := EmptyPattern(bars)
		if err != nil {
			t.Fatal(err)
		}
		p, err := ParsePattern(b)
		if err != nil {
			t.Fatal(err)
		}
		if p.Length != uint32(bars*4*PPQ) || p.BeatsPerBar() != 4 || len(p.Events) != 0 || p.Trailer[0] != byte(bars) {
			t.Errorf("%d bars: length %d, beats %d, %d events, trailer % x", bars, p.Length, p.BeatsPerBar(), len(p.Events), p.Trailer)
		}
		// With one note added the file is what StartPattern writes for that note.
		one, _ := StartPattern([]proto.PadIndex{mustPad("A1")}, bars)
		with, err := InsertNote(b, 0, mustPad("A1"), 127, uint16(bars*4*PPQ))
		if err != nil || !slices.Equal(one, with) {
			t.Errorf("%d bars: an inserted note differs from StartPattern (%v)", bars, err)
		}
	}
	for _, bars := range []int{0, MaxPatternBars + 1} {
		if _, err := EmptyPattern(bars); err == nil {
			t.Errorf("%d bars accepted", bars)
		}
	}
}
