package core

import (
	"fmt"
	"strings"
)

// melodyHelpRows are the rows of the Melody dialog's ABOUT block: what the setting at the cursor does, in words.
const melodyHelpRows = 4

// melRoleHelp says what each role (melRoles, in order) writes.
var melRoleHelp = []string{
	"bass: a bass line on the chord roots and fifths, in the style's bass rhythm.",
	"lead: a melody of a motif that repeats and answers itself, on the notes of the scale.",
	"arp: the notes of the chord one after the other, up and down.",
	"stabs: short chords in the style's stab rhythm.",
	"pad: the chords held, each as long as it lasts.",
	"sub bass: long low roots, one or two notes a bar, for under a kick.",
	"octave bass: the root low and an octave up in turn (disco and house).",
	"acid line: a 303 pattern: 12 to 16 of the 16 steps, one to three notes, octave jumps, slides and accents.",
	"pluck: a short hook on the offbeats.",
	"call and response: a phrase, then its answer.",
	"piano house: the chord broken up in the M1 piano rhythm.",
	"drone: the key's root held through the bars.",
	"dub chord: a seventh chord on an offbeat, with its echoes quieter each time (see Echo).",
	"parallel stabs: one chord shape moved as it is to other roots, even out of the key (Detroit, Kerri Chandler).",
	"rolling bass: root, fifth and octave in a 16th roll, swung a little; the last bar runs up the scale home.",
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
	case label == "Role":
		return "What is written. " + pick(melRoleHelp, val)
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
			return "The chords under the melody: one of the style's loops; another roll of the dice can take another. NOW shows each bar's chord."
		case 1:
			return "The chords under the melody: only the key's own chord, every bar."
		}
		return "The chords under the melody: this progression in roman numerals (i is the key's chord, iv the one on the 4th, VI on the 6th...). NOW shows each bar's chord."
	case label == "Chord length":
		return "How many bars each chord of the loop lasts. The style's: house a bar, trance and progressive two."
	case label == "Chord notes":
		return "Which notes make each chord. " + pick(melColorHelp, val)
	case label == "Form":
		return "How the bars of the motif follow each other. " + pick(melFormHelp, val)
	case label == "Swing":
		return "How late every second 16th comes (MPC swing; 50% is straight). The role's: a rolling bass 57%, the rest straight. Only with Notes on the style's."
	case strings.HasPrefix(label, "Echo"):
		if st.role != 12 {
			return "Only for the dub chord role: the gap between its echoes. Choose dub chord in Role to hear it."
		}
		return "The gap between the dub chord's echoes, in 16ths: 3/16 is the dotted 8th of the classic dub delay; 5/16 and 7/16 fall off the beat."
	case label == "Octave from":
		return "The lowest octave the notes may go to, from the pad's own pitch (0: from the sample's note up). The role's: its own range. A pad's sample goes an octave up and down at most."
	case label == "Octave to":
		return "The highest octave the notes may go to, from the pad's own pitch. The role's: its own range. A pad's sample goes an octave up and down at most; the synth plays them all."
	case label == "Density":
		return "How many notes: sparse leaves room for the drums, busy fills the bar."
	case label == "Ghost notes":
		if k := melRoles[st.role].kind; k > kArp || st.role == 5 || st.role == 14 {
			return "Quiet notes between the main ones. Not for chords, pads, drones, a sub or a rolling bass: this role has none."
		}
		return "Quiet notes between the main ones, for groove: none, a few, or many."
	case label == "Accents":
		return "Which notes are played louder: on the offbeats (house), on the beats, or here and there. An acid line's accents are its own."
	case label == "MIDI out":
		return "Where this role plays: the pad's sample, or a MIDI channel to the synth behind the unit (the MC-101), in time with the unit's clock. It stays as it is when other settings change; every change is sent at once."
	case label == "Another roll of the dice":
		return "A: the same settings, other notes (and another of the style's chord loops)."
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
