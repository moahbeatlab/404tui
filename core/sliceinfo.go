package core

import (
	"fmt"
	"strings"
)

// The slicer's panel in the terminal: the sample, the slice under the cursor with all its settings, and the order it plays in.

// sliceHeader is the two lines over the groups: the pad and its sample, then the sample's length, the slices and the zoom.
func (a *App) sliceHeader(g Gfx, x, y, w int) {
	g.Text(x, y, fmt.Sprintf("%s %s", a.label(a.spad), fit(a.info.Pads[a.spad].Name, w-4)), cAmber, true)
	g.Text(x, y+1, fit(fmt.Sprintf("%.2f s  %d slices  zoom %dx", float64(a.sl.Frames)/48000, len(a.sl.Slices), 1<<a.zoom), w), cDim, false)
}

func (a *App) sliceInfoGroups() [][]infoLine {
	sl := a.sl
	sec := func(f int) string { return fmt.Sprintf("%.3f s", float64(f)/48000) }
	sel := a.sliceAt(a.at)
	if sel < 0 {
		return [][]infoLine{{{label: "SLICE", head: true}, {label: "", value: "none here: a cuts", off: true}}}
	}
	s := sl.Slices[sel]
	beats := float64(s.End-s.Start) / 48000 * a.tempoOf(a.spad/16) / 60
	slice := []infoLine{{label: "SLICE", head: true},
		{label: "Number", value: fmt.Sprintf("%d of %d", sel+1, len(sl.Slices))},
		{label: "Start", value: sec(s.Start)},
		{label: "End", value: sec(s.End)},
		{label: "Length", value: sec(s.End - s.Start)},
		{label: "Beats", value: fmt.Sprintf("%.2f", beats)}}
	onOff := func(l string, on bool) infoLine {
		if on {
			return infoLine{label: l, value: "on", on: true}
		}
		return infoLine{label: l, value: "off", off: true}
	}
	sound := []infoLine{{label: "SOUND", head: true},
		{label: "Pitch", value: fmt.Sprintf("%+d st", s.Pitch)},
		{label: "Gain", value: fmt.Sprintf("%d%%", s.Gain)},
		{label: "Chance", value: fmt.Sprintf("%d%%", s.Chance)},
		onOff("Reverse", s.Reverse)}
	env := []infoLine{{label: "ENVELOPE", head: true},
		{label: "Attack", value: fmt.Sprintf("%d ms", s.Attack)},
		{label: "Release", value: fmt.Sprintf("%d ms", s.Release)}}
	roll := []infoLine{{label: "ROLL", head: true}}
	if s.Roll > 0 {
		roll = append(roll, infoLine{label: "Hits", value: fmt.Sprint(s.Roll)},
			infoLine{label: "Loudness", value: fmt.Sprintf("%+d%%", s.RollLoud)},
			infoLine{label: "Pitch", value: fmt.Sprintf("%+d st", s.RollPitch)})
	} else {
		roll = append(roll, infoLine{label: "Hits", value: "off", off: true})
	}
	var at []string
	for i, v := range sl.Order {
		if v == sel {
			at = append(at, fmt.Sprint(i+1))
		}
	}
	where := "not in it"
	if len(at) > 0 {
		where = strings.Join(at, ", ")
	}
	order := []infoLine{{label: "ORDER", head: true},
		{label: "Mode", value: orderMode(sl)},
		{label: "Plays at", value: where},
		{label: "Steps", value: fmt.Sprint(a.steps2())}}
	if sl.Division > 0 {
		order = append(order, infoLine{label: "Grid", value: fmt.Sprintf("1/%d at %s", sl.Division, bpmText(sl.BPM))})
	} else {
		order = append(order, infoLine{label: "Grid", value: "off", off: true})
	}
	return [][]infoLine{slice, order, sound, env, roll}
}
