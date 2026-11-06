package formats

import "testing"

func TestPitchByteOfChromaticNotes(t *testing.T) {
	// the bytes of the notes the user played over A5 on the unit: -12 -6 0 +6 +11 +12
	for b, want := range map[uint8]int{0x81: -12, 0x87: -6, 0x8D: 0, 0x93: 6, 0x98: 11, 0x99: 12, 0: 0} {
		if got := PitchSemitones(b); got != want {
			t.Errorf("%#x is %d semitones, got %d", b, want, got)
		}
	}
	for st := -12; st <= 12; st++ {
		if got := PitchSemitones(PitchByte(st)); got != st {
			t.Errorf("%d goes to the byte and back as %d", st, got)
		}
	}
	if PitchByte(0) != 0 || PitchByte(40) != 0x99 || PitchByte(-40) != 0x81 {
		t.Error("no change is a plain note; the range is kept")
	}
	if PitchText(0) != "plain" || PitchText(0x99) != "+12 st" || PitchText(0x81) != "-12 st" {
		t.Error("the text")
	}
}
