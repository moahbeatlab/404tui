package core

import (
	"fmt"
	"math"
	"math/rand"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/lab"
)

// Pitch in the track tools. A note of a pattern has a pitch (see formats/notepitch.go): the tools that make notes can give them one,
// the tools that change notes can move it (an echo that climbs, a ratchet that rises, a random spread, an LFO or an envelope on the
// pitch), and the Pitch tool works on the pitches alone. Every tool keeps the pitches the notes already have.

var (
	// pitchSemis are the pitches a tool can set, -12 to +12 semitones; the middle one (0) is a plain note.
	pitchSemis = func() []int {
		out := make([]int, 0, 25)
		for i := -formats.PitchRange; i <= formats.PitchRange; i++ {
			out = append(out, i)
		}
		return out
	}()
	pitchMid = formats.PitchRange // the index of 0 in pitchSemis
	// pitchSteps are how far a repeat or a part is from the one before, in semitones.
	pitchSteps = []int{0, 1, 2, 3, 4, 5, 7, 12, -1, -2, -3, -4, -5, -7, -12}
	// pitchSpreads are how far a random pitch goes from the note's own, up and down.
	pitchSpreads = []int{0, 1, 2, 3, 5, 7, 12}
)

func semiText(v int) string {
	if v == 0 {
		return "plain"
	}
	return fmt.Sprintf("%+d st", v)
}

func stepText(v int) string {
	if v == 0 {
		return "no change"
	}
	return fmt.Sprintf("%+d st", v)
}

// shiftPitch moves a note's pitch by semitones, within the range of the unit.
func shiftPitch(n lab.Note, by int) lab.Note {
	n.Pitch = int(formats.PitchByte(formats.PitchSemitones(uint8(n.Pitch)) + by))
	return n
}

// setPitch gives a note that pitch (0 makes it a plain note).
func setPitch(n lab.Note, semitones int) lab.Note {
	n.Pitch = int(formats.PitchByte(semitones))
	return n
}

// withPitch gives every note that pitch.
func withPitch(notes []lab.Note, semitones int) []lab.Note {
	if semitones == 0 {
		return notes
	}
	for i := range notes {
		notes[i] = setPitch(notes[i], semitones)
	}
	return notes
}

// ---- the Pitch tool

var pitchModes = []string{"transpose by", "set every note to", "random within", "rising run, a step each note", "falling run, a step each note", "back to plain"}

// pitchNotes works on the pitches of the notes of a track in time order.
func pitchNotes(notes []lab.Note, mode, amount int, seed int64) []lab.Note {
	rng := rand.New(rand.NewSource(seed))
	out := make([]lab.Note, len(notes))
	step := max(int(math.Abs(float64(amount))), 1)
	for i, n := range notes {
		switch mode {
		case 0:
			n = shiftPitch(n, amount)
		case 1:
			n = setPitch(n, amount)
		case 2:
			n = setPitch(n, formats.PitchSemitones(uint8(n.Pitch))+rng.Intn(2*abs(amount)+1)-abs(amount))
		case 3:
			n = setPitch(n, (i*step)%(formats.PitchRange+1))
		case 4:
			n = setPitch(n, -(i*step)%(formats.PitchRange+1))
		default:
			n = setPitch(n, 0)
		}
		out[i] = n
	}
	return out
}

func (a *App) pitchMenu() {
	t := a.trackDefaults()
	pad := a.rowPad()
	a.toolMenu(fmt.Sprintf("pitch of %s", a.label(pad)), []menuItem{
		val("What to do", pitchModes, t.pitMode, func(v int) { t.pitMode = v }),
		val("Semitones", names(pitchSemis, func(v int) string { return fmt.Sprintf("%+d", v) }), t.pitAmount, func(v int) { t.pitAmount = v }),
	}, "Change the pitches", func() {
		notes := a.trackNotes(pad)
		if len(notes) == 0 {
			a.say("this track has no notes", true)
			return
		}
		a.finishTrack(pad, pitchNotes(notes, t.pitMode, pitchSemis[t.pitAmount], t.seed), "pitch: "+pitchModes[t.pitMode])
	})
}

// randomizePitch moves the pitch of every note by a random number of semitones, up to spread either way (0 leaves them alone).
func randomizePitch(notes []lab.Note, spread int, seed int64) []lab.Note {
	if spread == 0 {
		return notes
	}
	rng := rand.New(rand.NewSource(seed + 101))
	for i := range notes {
		notes[i] = shiftPitch(notes[i], rng.Intn(2*spread+1)-spread)
	}
	return notes
}
