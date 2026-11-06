package formats

import "fmt"

// A note of a pattern has a pitch byte (byte 3 of its record). 0 is a plain note, played at the pad's own pitch. A note played from the
// unit's chromatic keyboard has 0x8D plus how many semitones it is above the pad's pitch, from 0x81 (12 below) to 0x99 (12 above): the
// notes of a pattern played over A5 hold 0x81 0x87 0x8D 0x93 0x98 0x99 for -12 -6 0 +6 +11 +12, and the pads of the live bank hold 0x8D.

// PitchCentre is the pitch byte of a chromatic note at the pad's own pitch.
const PitchCentre = 0x8D

// PitchRange is how far a note can be moved, up or down, in semitones.
const PitchRange = 12

// PitchSemitones is how many semitones a note with that pitch byte is above the pad's pitch (0 for a plain note).
func PitchSemitones(b uint8) int {
	if b == 0 {
		return 0
	}
	return min(max(int(b)-PitchCentre, -PitchRange), PitchRange)
}

// PitchByte is the pitch byte of a note that many semitones above the pad's pitch; 0 for no change, which is a plain note.
func PitchByte(semitones int) uint8 {
	if semitones == 0 {
		return 0
	}
	return uint8(PitchCentre + min(max(semitones, -PitchRange), PitchRange))
}

// PitchText says it for a screen: "plain", or "+12 st".
func PitchText(b uint8) string {
	if b == 0 {
		return "plain"
	}
	return fmt.Sprintf("%+d st", PitchSemitones(b))
}
