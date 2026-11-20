package core

import (
	"fmt"
	"strings"
)

// melodyHelpRows are the rows of the Melody dialog's ABOUT block: what the setting at the cursor does, in words.
const melodyHelpRows = 4

// melTypeHelp says what each type (melTypes, in order) writes.
var melTypeHelp = []string{
	"bass: a bass line on the chord roots; Pattern picks how it moves (the classic house, techno and trance lines).",
	"303: an acid line as the TB-303 plays it: 16ths, one to three notes, octave jumps, slides and accents.",
	"lead: a melody on the scale: a motif that repeats and answers itself, or a call and its response.",
	"arp: the chord's notes one after the other, in a Direction, over a Span of octaves.",
	"pluck: a short hook of the chord's notes, on the offbeats or anywhere in the bar.",
	"stabs: short chords: the style's rhythm, the M1 piano, one shape moved (Detroit), or a dub chord with its echoes.",
	"pad: the chords held, each as long as it lasts, or struck again.",
	"drone: the key's root held through the bars.",
}

var melBassPatHelp = []string{
	"the style's: the style's own bass rhythm, the root mostly, its octave and fifth.",
	"offbeat 8ths: one short note on every \"and\" (the donk of 90s house and trance); busy: the last an octave up.",
	"rolling 8ths: every 8th on one note, a step down on beat 3, the velocity alive (Fisher, Alan Fitzpatrick).",
	"rolling 16ths: the 16ths around the kick, the beat left to it; busy: octaves now and then.",
	"double offbeat: two notes on each offbeat, the \"and\" and the 16th after it: driving.",
	"gallop: an 8th on the beat, then two 16ths an octave up.",
	"octave: the root low and an octave up in turn (disco and house).",
	"sub: long low roots, one or two notes a bar, for under a kick.",
	"root, fifth, octave roll: a 16th roll, swung a little; the last bar runs up the scale home (Chris Stussy).",
	"stab and sustain: a stab on the one, then a note held over beats 2 and 3 (drops, peak time).",
	"syncopated sustain: held from the \"and\" of 1, a short stab on the \"and\" of 2, again in the second half.",
	"slide arc: two bars on the root, the third slides up to the fifth (or fourth), the fourth slides home. Set the synth to legato.",
}

var melStabHelp = []string{
	"the style's: the chord on the style's offbeats.",
	"piano house: the chord broken up in the M1 piano rhythm.",
	"parallel shape: one chord shape moved as it is to other roots, even out of the key (Detroit, Kerri Chandler).",
	"dub chord: a seventh chord on an offbeat, with its echoes quieter each time (see Echo).",
}

var melStyleHelp = []string{
	"house: 3+3+2 rhythms; two-chord loops or four chords a bar each; seventh chords.",
	"deep house: two-chord loops (i iv most) or four chords a bar each; ninth chords.",
	"tech house: one chord mostly, broken offbeat rhythms; seventh chords.",
	"techno: one chord mostly, sparse stabs and bass; melodic loops 2 or 4 bars a chord.",
	"trance: the anthem loops (i VI III VII...), two bars a chord; triads.",
	"progressive: the anthem loops, two bars a chord; sus2 chords.",
	"dub techno: one chord or i iv over two bars; open dub chords, a drone.",
	"acid house: one chord; lines in the 303 manner.",
}

var melModeHelp = []string{
	"minor: the dark, common choice for house and techno.",
	"major: bright, uplifting.",
	"dorian: minor with a raised 6th: soulful, deep house and jazz.",
	"phrygian: minor with a flat 2nd: dark, tense techno.",
	"harmonic minor: minor with a raised 7th: dramatic, trance leads.",
	"minor pentatonic: five notes only; nothing clashes.",
}

var melColorHelp = []string{
	"the style's: the chord notes the style uses (see Style).",
	"triads: three notes, root, third and fifth.",
	"sevenths: the 7th added: softer, house.",
	"ninths: the 7th and the 9th: lush, deep house.",
	"sus2: the 2nd in place of the third: open, neither minor nor major.",
	"dub: root, fifth, seventh and the third an octave up: the dub techno chord.",
	"seventh at the bottom: the 7th under the root (the Kerri Chandler voicing).",
}

var melFormHelp = []string{
	"A A' A B: the bar, again with a changed end, again, then a different bar to close the 4.",
	"A A B B: two bars of one idea, then two of another.",
	"A A A A: the same bar every time: a loop.",
}

// melodyHelp is what the item labeled label, set to val, does in the Melody dialog.
func melodyHelp(label string, val int, st melodySet) string {
	pick := func(list []string, i int) string {
		if i >= 0 && i < len(list) {
			return list[i]
		}
		return ""
	}
	switch {
	case label == "Type":
		return "What is written; its own settings are under it. " + pick(melTypeHelp, val)
	case label == "Pattern":
		return "How the bass moves. " + pick(melBassPatHelp, val)
	case label == "Approach":
		return "The last note of a bar before the chord changes: a step from the next root, leading into it. Now and then, never, or always."
	case label == "Phrase":
		if val == 1 {
			return "Call and response: a phrase in the first half of the bar, its answer in the second; the answer ends higher, then home."
		}
		return "A motif: one bar that repeats, its end changed as Form says."
	case label == "Leaps":
		return "How far the lead moves from note to note: steps of the scale, or wide leaps (4ths, 5ths, octaves) for a bigger hook."
	case label == "Direction":
		return "The order the arp plays the chord's notes, as DOOM OS's arp and the sample ARP have them: up, down, up and down (the ends once or twice), converge, diverge, pinky and thumb (the top or the bottom between), thirds, a scale run, the chord."
	case label == "Span":
		return "How many octaves the arp climbs: the type's (about an octave and a half), or 1 to 3. A pad's sample goes an octave up and down at most; the synth plays them all."
	case label == "Gate":
		return "How long each arp note is, as a part of its step: short and plucked, or 100% (legato, a slide on a synth set to it). Only with Notes on the style's."
	case label == "Hook":
		return "How many notes the pluck's hook has: the type's (three or more, as Density says), or 3 to 7."
	case label == "Placement":
		return "Where the pluck's notes go: on the offbeat 16ths (off the kick), or anywhere in the bar."
	case label == "Rhythm":
		return "How the stabs go. " + pick(melStabHelp, val)
	case label == "Movement":
		return "How the pad plays the chord: held, struck again each half bar, or four times a bar from soft to loud."
	case label == "Drone note":
		return "What the drone holds: the root (the fifth too when Density is busy), the root alone, or the root and the fifth."
	case label == "Length":
		return "How many bars the melody runs before it repeats. Shorter than the pattern: it repeats over it; longer: only the synth plays it all, the pattern keeps its length."
	case label == "Style":
		return "The rhythms and the chords the melody is written in. " + pick(melStyleHelp, val)
	case label == "Notes":
		if val == 0 {
			return "The note values: the style's own rhythm. Set 1/2 to 1/32 or triplets to write every note in one value (Swing then does nothing)."
		}
		return fmt.Sprintf("The note values: every note a %s (the style's rhythm is set aside; Swing does nothing).", melRates[val].name)
	case label == "Key":
		return "The key the notes and chords are in. \"the bank's\" is the key read from the pads' names (G#, Am, Cmin...) or from their sound."
	case label == "Scale":
		return "The notes allowed. " + pick(melModeHelp, val)
	case label == "Chords":
		switch val {
		case 0:
			return "The chords under the melody: one of the style's loops; another roll of the dice can take another. A (Enter): the list of every progression with its chords. NOW shows each bar's chord."
		case 1:
			return "The chords under the melody: only the key's own chord, every bar. A (Enter): the list of every progression."
		}
		return "The chords under the melody: this progression in roman numerals (i is the key's chord, iv the one on the 4th, VI on the 6th...). A (Enter): the list, with the chords and how each feels."
	case label == "Chord length":
		return "How many bars each chord of the loop lasts. The style's: house a bar, trance and progressive two."
	case label == "Chord notes":
		return "Which notes make each chord. " + pick(melColorHelp, val)
	case label == "Form":
		return "How the bars of the motif follow each other. " + pick(melFormHelp, val)
	case label == "Swing":
		return "How late every second 16th comes (MPC swing; 50% is straight). The type's: a root-fifth-octave roll 57%, the rest straight. Only with Notes on the style's."
	case strings.HasPrefix(label, "Echo"):
		return "The gap between the dub chord's echoes, in 16ths: 3/16 is the dotted 8th of the classic dub delay; 5/16 and 7/16 fall off the beat."
	case label == "Octave from":
		return "The lowest octave the notes may go to, from the pad's own pitch (0: from the sample's note up). The type's: its own range. A pad's sample goes an octave up and down at most."
	case label == "Octave to":
		return "The highest octave the notes may go to, from the pad's own pitch. The type's: its own range. A pad's sample goes an octave up and down at most; the synth plays them all."
	case label == "Density":
		return "How many notes: sparse leaves room for the drums, busy fills the bar."
	case label == "Ghost notes":
		if k := melRoles[st.role].kind; k > kArp || st.role == 5 || st.role == 14 {
			return "Quiet notes between the main ones. Not for chords, pads, drones, a sub or a roll."
		}
		return "Quiet notes between the main ones, for groove: none, a few, or many."
	case label == "Accents":
		return "Which notes are played louder: on the offbeats (house), on the beats, or here and there. An acid line's accents are its own."
	case label == "MIDI out":
		return "Where the melody plays: the pad's sample, or a MIDI channel to the synth behind the unit (the MC-101), in time with the unit's clock. It stays as it is when other settings change; every change is sent at once."
	case label == "Another roll of the dice":
		return "A (or n anywhere in the dialog on a keyboard): the same settings, other notes (and another of the style's chord loops)."
	case strings.HasPrefix(label, "Keep it"):
		return "A: keep the melody in the track. Closing the dialog without it throws the trial away."
	case strings.HasPrefix(label, "Write"):
		return "A: write the melody into the track as a trial: listen, roll again, keep it."
	}
	return "Up and down choose a setting, left and right change it; the trial is written again as it changes."
}

// melodyFoot draws what the setting at the cursor does, then NOW.
func (a *App) melodyFoot(m *menu) func(g Gfx, x, y, w int) {
	return func(g Gfx, x, y, w int) {
		g.HRule(x+1, y, w-2, cLine)
		g.Fill(x+1, y, 8, 1, cPanel2)
		g.Text(x+2, y, "ABOUT", cDim, true)
		it := m.item()
		lines := wrap(melodyHelp(it.label, it.val, a.mel), w-4)
		for i, l := range lines {
			if i == melodyHelpRows-1 && len(lines) > melodyHelpRows {
				l = fit(l+" "+lines[i+1], w-4) // fit ends it with ~
			}
			if i >= melodyHelpRows {
				break
			}
			g.Text(x+2, y+1+i, l, cText, false)
		}
		a.melodyNow(g, x, y+1+melodyHelpRows, w)
	}
}
