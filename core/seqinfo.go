package core

import "fmt"

// The sequencer's panel in the terminal: the pad of the track, the note under the cursor and the pattern, as the pad's
// information is laid out.

func (a *App) seqInfoGroups() [][]infoLine {
	d, ok := a.doc()
	pad := a.rowPad()
	name := ""
	if pad < len(a.info.Pads) {
		name = a.info.Pads[pad].Name
	}
	mode := "plays"
	switch a.trackMode(pad) {
	case trackMute:
		mode = "muted"
	case trackSolo:
		mode = "solo"
	}
	notes := 0
	for _, n := range d.Notes {
		if n.Pad == pad {
			notes++
		}
	}
	track := []infoLine{{label: "TRACK", head: true},
		{label: "Pad", value: a.label(pad)},
		{label: "Sample", value: name},
		{label: "Mode", value: mode, on: mode == "solo", off: mode == "muted"},
		{label: "Notes", value: fmt.Sprint(notes)}}
	note := []infoLine{{label: "NOTE", head: true},
		{label: "Bar.step", value: fmt.Sprintf("%d.%d", a.step/16+1, a.step%16+1)}}
	if n, ok := noteAt(d, pad, a.step); ok {
		pitch := "plain"
		if n.Pitch != 0 {
			pitch = fmt.Sprintf("chromatic %d", n.Pitch)
		}
		note = append(note,
			infoLine{label: "Velocity", value: fmt.Sprint(n.Vel)},
			infoLine{label: "Length", value: fmt.Sprintf("%.2f steps", float64(n.Len)/stepTicks)},
			infoLine{label: "Timing", value: shortTiming(a.noteTiming(n, pad))},
			infoLine{label: "Pitch", value: pitch})
	} else {
		note = append(note, infoLine{label: "", value: "no note: A puts one", off: true})
	}
	pat := []infoLine{{label: "PATTERN", head: true},
		{label: "Slot", value: a.label(a.pat - 1)},
		{label: "Length", value: fmt.Sprintf("%d bars", d.Bars)},
		{label: "Tracks", value: fmt.Sprint(len(a.rows))},
		{label: "Tempo", value: fmt.Sprintf("%s BPM", bpmText(a.tempo()))}}
	if !ok {
		pat = []infoLine{{label: "PATTERN", head: true}, {label: "Slot", value: a.label(a.pat - 1)}, {label: "", value: "empty: A puts the first note", off: true}}
	}
	return [][]infoLine{note, pat, track}
}

// shortTiming is the first part of a timing: "on the grid", "24 ticks late".
func shortTiming(s string) string {
	for i := 0; i+1 < len(s); i++ {
		if s[i] == ' ' && s[i+1] == '(' {
			return s[:i]
		}
	}
	return s
}
