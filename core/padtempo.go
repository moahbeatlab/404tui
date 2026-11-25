package core

import (
	"fmt"
	"math"

	"github.com/moahbeatlab/404tui/formats"
)

// The tempo a pad's sample says it is at (its BPM, what the unit's BPM sync stretches it by) is checked against the tempo heard in
// the sample, and against one its name states: the ones that differ are listed with both, and nothing changes until one is chosen.

// padTempo is what is known of the tempo of one pad.
type padTempo struct {
	pad   int
	unit  float64 // the pad's BPM setting, 0: none
	heard float64 // the tempo heard in the sample, 0: no steady beat (or too short)
	conf  float64
	named float64 // the tempo the name states, 0: none
	short bool    // too short to hear a tempo in
}

// want is the tempo the check proposes: the name's, else the heard one; 0 when there is none.
func (t padTempo) want() float64 {
	if t.named > 0 {
		return t.named
	}
	return t.heard
}

// sameTempo: within half a BPM, or the same at half or double (a 70 BPM loop heard at 140 is at 70).
func sameTempo(a, b float64) bool {
	for _, f := range []float64{1, 2, 0.5} {
		if math.Abs(a-b*f) < 0.5 {
			return true
		}
	}
	return false
}

// agrees says whether the pad's BPM is what the check would set.
func (t padTempo) agrees() bool {
	w := t.want()
	return w <= 0 || (t.unit > 0 && sameTempo(t.unit, w))
}

// minTempoFrames: a sample shorter than two bars at 120 has too few beats to hear a tempo in.
const minTempoFrames = 4 * formats.SampleRate

// padTempoOf finds the tempos of a pad.
func (a *App) padTempoOf(n int) padTempo {
	t := padTempo{pad: n, named: bpmOfName(a.info.Pads[n].Name)}
	if v, err := a.lab.PadParam(n, 0x6F); err == nil && v > 0 {
		t.unit = float64(v) / 100
	}
	id, ok := a.padSample(n)
	if !ok {
		return t
	}
	s, err := a.lab.Sample(id)
	if err != nil {
		return t
	}
	if s.Frames() < minTempoFrames {
		t.short = true
		return t
	}
	tp := formats.DetectTempo(s)
	t.heard, t.conf = tp.BPM, tp.Confidence
	return t
}

// tempoText is a tempo to two decimals, the way the unit shows a pad's BPM; "-" for none.
func tempoText(v float64) string {
	if v <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.2f", v)
}

// tempoRow is one pad of the check: its name and its BPM, then what was heard and what its name says.
func (a *App) tempoRow(t padTempo) string {
	l := fmt.Sprintf("%s %-12s %6s ", a.label(t.pad), fitRunes(a.info.Pads[t.pad].Name, 12), tempoText(t.unit))
	switch {
	case t.short:
		l += " too short"
	case t.heard > 0:
		l += fmt.Sprintf(" heard %s %.0f%%", tempoText(t.heard), t.conf*100)
	default:
		l += " no steady beat"
	}
	if t.named > 0 {
		l += " name " + bpmText(t.named)
	}
	return l
}

// changeRow is a pad whose BPM differs: its BPM, the one it gets, and why.
func (a *App) changeRow(t padTempo) string {
	why := fmt.Sprintf("heard %.0f%%", t.conf*100)
	if t.named > 0 {
		why = "its name"
	}
	return fmt.Sprintf("%s %-12s %6s -> %-6s %s", a.label(t.pad), fitRunes(a.info.Pads[t.pad].Name, 12), tempoText(t.unit), tempoText(t.want()), why)
}

// checkTempos hears the tempo of every pad of the bank with a sample (the marked pads, when some are marked) and shows the
// ones whose BPM differs, to be set one by one or all together.
func (a *App) checkTempos() {
	var pads []int
	scope := fmt.Sprintf("bank %c", 'A'+a.bank)
	if len(a.marks) > 0 {
		pads = a.targetsFor(a.padN())
		scope = fmt.Sprintf("the %d marked pads", len(pads))
	} else {
		for n := a.bank * 16; n < a.bank*16+16; n++ {
			if a.info.Pads[n].File {
				pads = append(pads, n)
			}
		}
	}
	if len(pads) == 0 {
		a.say("no pad with a sample in "+scope, false)
		return
	}
	var differ, agree []padTempo
	for _, n := range pads {
		t := a.padTempoOf(n)
		if t.agrees() {
			agree = append(agree, t)
		} else {
			differ = append(differ, t)
		}
	}
	m := &menu{title: fmt.Sprintf("tempos of %s: %d of %d differ", scope, len(differ), len(pads))}
	add := func(l string, run func(int)) { m.items = append(m.items, menuItem{label: l, run: run}) }
	if len(differ) > 0 {
		add(fmt.Sprintf("Set the %d that differ (the name's tempo, else the one heard)", len(differ)), func(int) { a.setTempos(differ) })
		add("No, leave them", func(int) {})
		m.head("DIFFER: A SETS THAT ONE")
		for _, t := range differ {
			t := t
			add(a.changeRow(t), func(int) { a.setTempos([]padTempo{t}) })
		}
	} else {
		add("Every BPM is right: close", func(int) {})
	}
	if len(agree) > 0 {
		m.head("RIGHT, OR NOTHING TO GO BY")
		for _, t := range agree {
			m.items = append(m.items, menuItem{label: a.tempoRow(t), run: func(int) {}})
		}
	}
	a.menu = m
}

// setTempos gives pads the tempo the check found for them (undone with the rest).
func (a *App) setTempos(ts []padTempo) {
	pre := a.lab.Snapshot()
	n := 0
	for _, t := range ts {
		if w := t.want(); w > 0 && a.lab.SetPadParam(t.pad, 0x6F, int32(math.Round(w*100))) == nil {
			n++
		}
	}
	a.commitUndo(pre, A, 0)
	a.refresh()
	if len(ts) == 1 && n == 1 {
		a.say(fmt.Sprintf("%s: BPM %s (was %s)", a.label(ts[0].pad), tempoText(ts[0].want()), tempoText(ts[0].unit)), false)
		return
	}
	a.say(fmt.Sprintf("the BPM of %d pads set", n), false)
}
