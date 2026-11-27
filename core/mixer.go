package core

import (
	"fmt"
)

// The mixer of SEQ (M on a computer): a strip for every track of the pattern, the pads it plays, with what the unit mixes a pad
// with: its level, its pan and its bus (an EQ on the unit is an effect on a bus). Mute and solo are those of the grid: they are
// heard on the computer and leave the pad alone. Every change is a change of the pad (undo, and the unit gets it when it is edited
// live); while the pattern plays it is heard at once.

var (
	mixLevel = padPages[0][0] // Level
	mixPan   = padPages[0][1] // Pan
	mixBus   = busSetting()
)

func busSetting() padSetting {
	for _, page := range padPages {
		for _, st := range page {
			if st.id == 0x7A {
				return st
			}
		}
	}
	panic("no bus setting")
}

// SeqMixer is M of a keyboard on SEQ: the mixer in place of the grid, or the grid again.
func (a *App) SeqMixer() {
	if a.scr != Seq || !Terminal || a.Modal() || a.prompt != nil || a.aHeld {
		return
	}
	a.CancelShift()
	a.patStrip = false
	a.mixer = !a.mixer
	if a.mixer {
		if len(a.rows) == 0 {
			a.mixer = false
			a.say("no tracks in this pattern", true)
			return
		}
		a.say("mixer: h l the pad, j k level, [ ] pan, b bus, d remove, M or Esc the grid", false)
		return
	}
	a.say("the grid", false)
}

// MixerOn says whether the mixer is in front.
func (a *App) MixerOn() bool { return a.mixer && a.scr == Seq }

func (a *App) mixerKey(k Key, repeat int) {
	if len(a.rows) == 0 && k != X && k != A { // a pattern without tracks (H J K L went to it): nothing to set
		return
	}
	pad := a.rowPad()
	step := int32(1 + repeat/4) // a held key runs faster
	nudge := func(st padSetting, d int32) {
		a.setSetting(pad, st, a.settingValue(pad, st)+d)
		a.replay() // a pattern that plays is heard with it at once
	}
	switch k {
	case Left:
		a.row = max(a.row-1, 0)
	case Right:
		a.row = min(a.row+1, len(a.rows)-1)
	case Up:
		nudge(mixLevel, step)
	case Down:
		nudge(mixLevel, -step)
	case L1:
		nudge(mixPan, -step)
	case R1:
		nudge(mixPan, step)
	case B: // the bus: off, BUS 1, BUS 2
		v := (a.settingValue(pad, mixBus) + 1) % 3
		a.setSetting(pad, mixBus, v)
		a.replay()
		a.say(a.label(pad)+" to "+settingText(mixBus, v), false)
	case X:
		a.togglePlay()
	case L2: // hear the pad as it is set
		a.playPad(pad)
	case R2: // mute, solo, normal
		a.SeqTrack(map[int]int{trackOn: trackMute, trackMute: trackSolo, trackSolo: trackSolo}[a.trackMode(pad)])
	case A:
		a.mixer = false
	}
}

// drawMixer draws the strips where the grid is: the pad, its name, a level fader as high as there is room for, the pan, the bus,
// mute and solo, and how many notes it has in the pattern.
func (a *App) drawMixer(g Gfx) {
	title := fmt.Sprintf("MIXER  PATTERN %s", a.label(a.pat-1))
	g.Text(2, 1, title, cAmber, true)
	d, _ := a.doc()
	g.Text(2+len(title)+2, 1, fmt.Sprintf("%d tracks · %s BPM", len(a.rows), bpmText(a.tempo())), cDim, false)
	notes := map[int]int{}
	for _, n := range d.Notes {
		notes[n.Pad]++
	}
	n := len(a.rows)
	if n == 0 {
		g.Text(2, 3, "no tracks", cDim, false)
		return
	}
	room := contentCols - 4
	sw := min(max(room/n, 11), 18) // a strip's width, the gap included
	fit1 := max(room/sw, 1)
	first := 0
	if n > fit1 { // more strips than fit: the window follows the cursor
		first = min(max(a.row-fit1/2, 0), n-fit1)
		g.Text(contentCols-14, 1, fmt.Sprintf("%d-%d of %d", first+1, first+fit1, n), cDim, false)
	}
	top, bottom := 3, Rows-4            // the rows the strips have
	fh := min(max(bottom-top-9, 3), 24) // the fader's height
	for i := first; i < min(n, first+fit1); i++ {
		pad := a.rows[i]
		x := 2 + (i-first)*sw
		w := sw - 2
		sel := i == a.row
		if sel {
			g.Fill(x-1, top, w+2, bottom-top+1, cPanel2)
		}
		fg := cText
		if sel {
			fg = cAmber
		}
		mode := a.trackMode(pad)
		g.Text(x, top, a.label(pad), fg, true)
		g.Text(x, top+1, fit(a.info.Pads[pad].Name, w), cDim, false)
		// the level: a fader up from the bottom, the number under it
		lv := a.settingValue(pad, mixLevel)
		col := cAmber
		if !sel {
			col = cAmberD
		}
		if mode == trackMute || a.anySolo() && mode != trackSolo {
			col = cLine
		}
		fx := x + w/4
		fw := max(w/2, 2)
		g.Fill(fx, top+3, fw, fh, cLine) // the track, filled from the bottom up to the level
		if up := (int(lv)*fh + 63) / 127; up > 0 {
			g.Fill(fx, top+3+fh-up, fw, up, col)
		}
		y := top + 3 + fh
		g.Text(x+(w-len(fmt.Sprint(lv)))/2, y, fmt.Sprint(lv), fg, sel)
		// the pan
		pv := a.settingValue(pad, mixPan)
		g.Fader(x, y+2, w, float64(pv-13)/102, 0.5, true)
		pt := mixPan.show(pv)
		g.Text(x+(w-len(pt))/2, y+3, pt, cText, false)
		// the bus
		bv := a.settingValue(pad, mixBus)
		bus := settingText(mixBus, bv)
		bc := cText
		if bv == 0 {
			bus, bc = "no bus", cDim
		}
		g.Text(x+(w-len(bus))/2, y+5, bus, bc, bv != 0)
		// mute and solo, the notes
		g.Badge(x+w/2-3, y+6, "M", mode == trackMute)
		g.Badge(x+w/2+1, y+6, "S", mode == trackSolo)
		nt := count(notes[pad], "note")
		g.Text(x+(w-len(nt))/2, y+7, fit(nt, w), cDim, false)
	}
}

// anySolo is whether a track of the pattern is solo.
func (a *App) anySolo() bool {
	for _, m := range a.trk[a.pat] {
		if m == trackSolo {
			return true
		}
	}
	return false
}
