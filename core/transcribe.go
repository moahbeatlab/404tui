package core

import (
	"fmt"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/lab"
	"github.com/moahbeatlab/404tui/proto"
)

// A played melody in a key (PATTERN → GENERATE): a melody played on a pad in chromatic mode, against any sample, written again
// as a new pattern: moved from the key it was played in to another, the notes off that key's scale moved to the nearest in it,
// on the pad (the bass, the synth) that is to play it, its pitches counted from that pad's sample.

// transcribeSet is the tool's settings, kept between openings.
type transcribeSet struct {
	played, playedNote int // the key the melody was played in, the note of the played pad's sample
	into, mode         int // the key and the scale (melModes) to write it in
	keep               int // 0: the notes off the scale are moved into it, 1: kept as played
	to, toNote         int // the pad that plays it and its sample's note; to -1: the played pad
	init               bool
}

var transcribeKeep = []string{"moved to the nearest in it", "kept as played"}

// transcribeShift is how far (semitones, -6..5) every note moves: from the key played to the key wanted, and from the played
// pad's sample to the playing pad's, the octave kept as near as can be.
func transcribeShift(t transcribeSet) int {
	s := ((t.into-t.played+t.playedNote-t.toNote)%12 + 12) % 12
	if s > 5 {
		s -= 12
	}
	return s
}

// snapToScale is how far (semitones) to move a note interval semitones above the tonic to the nearest note of the scale: 0 when
// it is in it; a tie goes down.
func snapToScale(interval int, scale []int) int {
	c := (interval%12 + 12) % 12
	best := 12
	for _, st := range append(scale, 12) {
		for _, d := range []int{st - c, st - 12 - c} {
			if abs(d) < abs(best) || abs(d) == abs(best) && d < best {
				best = d
			}
		}
	}
	return best
}

// transcribePitch is the pitch byte of the note in the new pattern for a note of the played one (its byte b): whether it was
// moved into the scale and whether an octave (a pad goes 12 semitones up or down, not more).
func transcribePitch(b uint8, t transcribeSet) (out uint8, snapped, folded bool) {
	s := formats.PitchSemitones(b)
	r := s + transcribeShift(t)
	if t.keep == 0 {
		// the interval above the new tonic: the same as above the played one, the octave aside
		if d := snapToScale(t.playedNote+s-t.played, melModes[t.mode].steps); d != 0 {
			r += d
			snapped = true
		}
	}
	for r > formats.PitchRange {
		r -= 12
		folded = true
	}
	for r < -formats.PitchRange {
		r += 12
		folded = true
	}
	return formats.PitchByte(r), snapped, folded
}

// transcribeMenu opens the tool on the track in front: the pad the melody was played on.
func (a *App) transcribeMenu() {
	from := a.rowPad()
	d, ok := a.doc()
	var n int
	if ok {
		for _, nt := range d.Notes {
			if nt.Pad == from {
				n++
			}
		}
	}
	if n == 0 {
		a.say(fmt.Sprintf("%s plays nothing in this pattern: put the track of the melody in front", a.label(from)), true)
		return
	}
	t := &a.tr
	key, minor, _ := a.bankKey((a.pat - 1) / 16)
	if !t.init {
		*t = transcribeSet{played: key, into: key, to: -1, init: true}
		if !minor {
			t.mode = 1
		}
	}
	t.playedNote, _ = a.sampleNote(from)
	to := from
	if t.to >= 0 && t.to < 160 && a.info.Pads[t.to].File {
		to = t.to
	}
	t.to = to
	t.toNote, _ = a.sampleNote(to)
	modes := make([]string, len(melModes))
	for i, md := range melModes {
		modes[i] = md.name
	}
	padVal := func() []string {
		s := a.label(t.to) + "  " + a.info.Pads[t.to].Name
		if t.to == from {
			s = a.label(t.to) + " (the one played on)"
		}
		return []string{s}
	}
	set := func(label string, vals []string, cur int, f func(v int)) menuItem {
		return menuItem{label: label, vals: vals, val: cur, keepOpen: true, onChange: f}
	}
	m := &menu{title: fmt.Sprintf("the melody of %s (%d notes) in a key", a.label(from), n), parent: a.menu}
	m.items = []menuItem{
		{label: "PLAYED", header: true},
		set("In the key of", sharpNames, t.played, func(v int) { t.played = v }),
		set("Its sample is in", sharpNames, t.playedNote, func(v int) { t.playedNote = v }),
		{label: "WRITTEN", header: true},
		set("In the key of", sharpNames, t.into, func(v int) { t.into = v }),
		set("Scale", modes, t.mode, func(v int) { t.mode = v }),
		set("Notes off the scale", transcribeKeep, t.keep, func(v int) { t.keep = v }),
		{label: "On the pad", vals: padVal(), keepOpen: true},
		set("Its sample is in (the pad)", sharpNames, t.toNote, func(v int) { t.toNote = v }),
		{label: "Write the new pattern", run: func(int) { a.transcribeWrite(from) }},
	}
	for i := range m.items {
		if m.items[i].label == "On the pad" {
			m.items[i].run = func(int) {
				a.pickPad("which pad plays the melody?", t.to, true, func(p int) {
					t.to = p
					t.toNote, _ = a.sampleNote(p)
					for j := range m.items {
						switch m.items[j].label {
						case "On the pad":
							m.items[j].vals = padVal()
						case "Its sample is in (the pad)":
							m.items[j].val = t.toNote
						}
					}
				})
			}
		}
	}
	m.footH = melodyHelpRows + 2
	m.foot = func(g Gfx, x, y, w int) {
		g.HRule(x+1, y, w-2, cLine)
		g.Fill(x+1, y, 8, 1, cPanel2)
		g.Text(x+2, y, "ABOUT", cDim, true)
		lines := wrap(transcribeHelp(m.item().label), w-4)
		for i, l := range lines {
			if i >= melodyHelpRows {
				break
			}
			g.Text(x+2, y+1+i, l, cText, false)
		}
		g.Text(x+2, y+1+melodyHelpRows, fit(a.transcribeSays(from, d.Notes), w-4), cDim, false)
	}
	a.menu = m
}

// transcribeSays is what the settings do to the notes: how many are moved into the scale, how many an octave.
func (a *App) transcribeSays(from int, notes []lab.Note) string {
	var n, snapped, folded int
	for _, nt := range notes {
		if nt.Pad != from {
			continue
		}
		n++
		_, s, f := transcribePitch(uint8(nt.Pitch), a.tr)
		if s {
			snapped++
		}
		if f {
			folded++
		}
	}
	return fmt.Sprintf("%d notes %+d st, %d into the scale, %d an octave", n, transcribeShift(a.tr), snapped, folded)
}

func transcribeHelp(label string) string {
	switch label {
	case "In the key of":
		return "Played: the key you played the melody in (its tonic; the scale does not matter). Written: the key it is written in, its tonic where yours was."
	case "Its sample is in":
		return "The note of the sample you played the melody against (read from its name or sound; change it if it is wrong): the notes you heard are counted from it."
	case "Its sample is in (the pad)":
		return "The note of the sample of the pad that plays the melody: the new notes are counted from it, so a G# is a G# on any sample."
	case "Scale":
		return "The scale of the written key: the notes off it are moved to the nearest note in it (see the next setting)."
	case "Notes off the scale":
		return "A note you played that is not in the scale: moved to the nearest note in it (a tie goes down: a major third in minor becomes the minor third), or kept as you played it."
	case "On the pad":
		return "A: choose the pad that plays the melody in the new pattern (the bass, the synth): its sample's note is read."
	case "Write the new pattern":
		return "A: the pattern copied to the next empty slot, the melody's notes rewritten on the pad, the other tracks as they are. This pattern stays as it is."
	}
	return "Play a melody on a pad in chromatic mode, against any sample; here it is written again in a key."
}

// transcribeWrite copies the pattern to the next empty slot and writes the melody of pad from again there.
func (a *App) transcribeWrite(from int) {
	to := a.freeSlot()
	if to == 0 {
		a.say("every pattern is used", true)
		return
	}
	pre := a.lab.Snapshot()
	if err := a.lab.CopyPattern(a.pat, to); err != nil {
		a.say(err.Error(), true)
		return
	}
	b, err := a.lab.PatternBytes(to)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	fp, _ := proto.NewPadIndex(uint16(from))
	tp, _ := proto.NewPadIndex(uint16(a.tr.to))
	var snapped, folded int
	n := formats.RewritePadNotes(b, fp, tp, func(p uint8) uint8 {
		out, s, f := transcribePitch(p, a.tr)
		if s {
			snapped++
		}
		if f {
			folded++
		}
		return out
	})
	if err := a.lab.SetPatternBytes(to, b); err != nil {
		a.say(err.Error(), true)
		return
	}
	a.commitUndo(pre, A, 0)
	a.menu = nil
	a.pat = to
	a.refresh()
	a.autoRows()
	a.say(fmt.Sprintf("%s: %d notes on %s in %s %s (%d moved into the scale, %d an octave)", a.label(to-1), n, a.label(a.tr.to),
		sharpNames[a.tr.into], melModes[a.tr.mode].name, snapped, folded), false)
}

// freeSlot is the first empty pattern slot after the one in front, 0 when every one is used.
func (a *App) freeSlot() int {
	for off := 1; off < 160; off++ {
		n := (a.pat-1+off)%160 + 1
		if _, err := a.lab.Pattern(n); err != nil {
			return n
		}
	}
	return 0
}
