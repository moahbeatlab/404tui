package core

import (
	"fmt"
	"image/color"
	"strings"
)

// The pad's information under the faders of the panel, as the classic program's pad box had it: the sample (length, file,
// tempo and bars, the patterns that use it), then the settings by group (playback, sound, routing). It takes the room
// there is: two columns on a wide panel, one column otherwise, cut off at the last row.

type infoLine struct {
	label, value string
	on, off      bool       // a switch: lit or faint
	col          color.RGBA // a colour of its own for the value (the bus), zero for none
	head         bool
}

func (a *App) padSettingText(pad int, label string) (string, bool, bool) {
	for _, page := range padPages {
		for _, st := range page {
			if st.label != label {
				continue
			}
			v := a.settingValue(pad, st)
			switch st.kind {
			case kSwitch, kFlag:
				if v != 0 {
					return "on", true, false
				}
				return "off", false, true
			case kChoice:
				if int(v-st.lo) < len(st.names) && v >= st.lo {
					return st.names[v-st.lo], false, false
				}
			}
			if st.show != nil {
				return st.show(v), false, false
			}
			return fmt.Sprint(v), false, false
		}
	}
	return "", false, false
}

// padUsedIn names the patterns that have notes on a pad ("B2 · C4"); read once per change of the project.
func (a *App) padUsedIn(pad int) string {
	if a.usedCache == nil {
		a.usedCache = map[int]string{}
	}
	if s, ok := a.usedCache[pad]; ok {
		return s
	}
	var names []string
	for _, pi := range a.info.Patterns {
		for _, p := range a.patternPads(pi.N) {
			if p == pad {
				names = append(names, pi.Label)
				break
			}
		}
	}
	s := "no pattern"
	if len(names) > 0 {
		s = strings.Join(names, " · ")
	}
	a.usedCache[pad] = s
	return s
}

var shortInfo = map[string]string{"Play mode": "Mode", "Gate": "Gate", "Loop": "Loop", "One shot": "One shot", "BPM sync": "BPM sync",
	"Level": "Level", "Pan": "Pan", "Attack": "Attack", "Hold": "Hold", "Release": "Release", "Pitch": "Pitch", "Fine pitch": "Fine", "Time stretch": "Stretch", "Vinyl": "Vinyl", "Bus": "Bus", "Mute group": "Mute grp"}

func (a *App) padInfoGroups(n int) [][]infoLine {
	p := a.info.Pads[n]
	var sample []infoLine
	sample = append(sample, infoLine{label: "SAMPLE", head: true})
	sample = append(sample, infoLine{label: "Size", value: fmt.Sprintf("%.1f KB", float64(p.Frames*2*int(max(p.Ch, 1)))/1024)})
	if bpm := a.padValue(n, 0x6F, 12000); bpm > 0 && p.Frames > 0 {
		sample = append(sample, infoLine{label: "Tempo", value: fmt.Sprintf("%d BPM", bpm/100)},
			infoLine{label: "Bars", value: fmt.Sprintf("%.1f", float64(p.Frames)/48000*float64(bpm)/100/60/4)})
	}
	sample = append(sample, infoLine{label: "Used in", value: a.padUsedIn(n)})
	group := func(title string, labels ...string) []infoLine {
		out := []infoLine{{label: title, head: true}}
		for _, l := range labels {
			v, on, off := a.padSettingText(n, l)
			il := infoLine{label: shortInfo[l], value: v, on: on, off: off}
			if l == "Bus" { // the colour the pad's strip has
				switch v {
				case "BUS 1":
					il.col = cBlue
				case "BUS 2":
					il.col = cBad
				}
			}
			out = append(out, il)
		}
		return out
	}
	return [][]infoLine{
		sample,
		group("PLAYBACK", "Play mode", "Gate", "Loop", "One shot", "BPM sync"),
		group("ROUTING", "Bus", "Mute group"),
		group("SOUND", "Level", "Pan", "Pitch", "Fine pitch", "Time stretch", "Vinyl"),
		group("ENVELOPE", "Attack", "Hold", "Release"),
	}
}

func (a *App) drawPadInfo(g Gfx, x, y0, w, y1, n int) {
	a.drawInfoGroups(g, x, y0, w, y1, a.padInfoGroups(n), 3)
}

// drawInfoGroups lays out groups of lines in the room: in two columns when the panel is wide (the first left groups
// on the left), else one under the other, cut off at the last row.
func (a *App) drawInfoGroups(g Gfx, x, y0, w, y1 int, groups [][]infoLine, left int) {
	draw := func(x, y, w int, lines []infoLine) int {
		for _, l := range lines {
			if y >= y1 {
				return y
			}
			if l.head {
				g.Text(x, y, l.label, cAmber, true)
				y++
				continue
			}
			lw := 9
			if l.label == "" { // a remark runs the whole width
				g.Text(x, y, fit(l.value, w), cDim, false)
				y++
				continue
			}
			g.Text(x, y, fit(l.label, lw), cDim, false)
			c := cText
			switch {
			case l.on:
				c = cGood
			case l.off:
				c = cDim
			case l.col != (color.RGBA{}):
				c = l.col
			}
			g.Text(x+lw, y, fit(l.value, w-lw), c, l.on)
			y++
		}
		return y
	}
	if w >= 40 {
		cw := w / 2
		y := y0
		for _, grp := range groups[:min(left, len(groups))] {
			y = draw(x, y, cw-1, grp) + 1
		}
		y = y0
		for _, grp := range groups[min(left, len(groups)):] {
			y = draw(x+cw, y, cw, grp) + 1
		}
		return
	}
	y := y0
	for _, grp := range groups {
		y = draw(x, y, w, grp) + 1
	}
}
