package handheld

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/lab"
)

// The slicer menu (Y+B in SLICE): how the slice under the cursor sounds (pitch, gain, reverse, fades, a roll), how the
// slices are played (in order, at random, with no repeat, shuffled; on a grid, with swing), and what to do with the
// result: play it, put it on a pad, put the slices on pads in a random order. The engine is the one of the computer
// slicer (formats.Slicing); a slice's settings are baked into its audio when it goes to a pad.

var (
	pitchVals  = seq(-12, 12, 1)
	gainVals   = []int{25, 50, 75, 100, 125, 150, 200}
	chanceVals = []int{0, 25, 50, 75, 100}
	loudVals   = seq(-100, 100, 25)
	stepsVals  = []int{0, 4, 8, 12, 16, 24, 32, 48, 64, 128}
	modeNames  = []string{"in order", "random", "no repeat", "shuffle"}
	gridNames  = []string{"off", "1/2", "1/4", "1/8", "1/16"}
)

func seq(from, to, step int) []int {
	var v []int
	for i := from; i <= to; i += step {
		v = append(v, i)
	}
	return v
}

// near is the index of the value of list closest to v.
func near(list []int, v int) int {
	best := 0
	for i, x := range list {
		if abs(x-v) < abs(list[best]-v) {
			best = i
		}
	}
	return best
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func names(list []int, f func(int) string) []string {
	out := make([]string, len(list))
	for i, v := range list {
		out[i] = f(v)
	}
	return out
}

func signed(v int) string { return fmt.Sprintf("%+d", v) }
func pct(v int) string {
	if v == 0 {
		return "off"
	}
	return fmt.Sprintf("%d%%", v)
}
func ms(v int) string { return fmt.Sprintf("%d ms", v) }

func (a *App) sliceMenu() {
	if a.sl == nil {
		return
	}
	sel := a.sliceAt(a.at)
	m := &menu{title: fmt.Sprintf("slicer  %d slices", len(a.sl.Slices))}
	set := func(label string, vals []string, cur int, change func(v int)) {
		m.items = append(m.items, menuItem{label: label, vals: vals, val: cur, keepOpen: true, onChange: change})
	}
	if sel >= 0 {
		s := &a.sl.Slices[sel]
		audition := func() { a.playSlice() }
		pre := fmt.Sprintf("Slice %d: ", sel+1)
		set(pre+"pitch", names(pitchVals, signed), near(pitchVals, s.Pitch), func(v int) { s.Pitch = pitchVals[v]; audition() })
		set(pre+"gain", names(gainVals, pct), near(gainVals, s.Gain), func(v int) { s.Gain = gainVals[v]; audition() })
		set(pre+"reverse", []string{"off", "on"}, b2i(s.Reverse), func(v int) { s.Reverse = v == 1; audition() })
		set(pre+"fade in", names(formats.EnvSteps, ms), near(formats.EnvSteps, s.Attack), func(v int) { s.Attack = formats.EnvSteps[v]; audition() })
		set(pre+"fade out", names(formats.EnvSteps, ms), near(formats.EnvSteps, s.Release), func(v int) { s.Release = formats.EnvSteps[v]; audition() })
		set(pre+"roll", names(formats.Rolls, func(v int) string {
			if v == 0 {
				return "off"
			}
			return fmt.Sprintf("%d hits", v)
		}), near(formats.Rolls, s.Roll), func(v int) { s.Roll = formats.Rolls[v]; audition() })
		set(pre+"roll louder", names(loudVals, signed), near(loudVals, s.RollLoud), func(v int) { s.RollLoud = loudVals[v]; audition() })
		set(pre+"roll pitch", names(pitchVals, signed), near(pitchVals, s.RollPitch), func(v int) { s.RollPitch = pitchVals[v]; audition() })
		set(pre+"chance", names(chanceVals, pct), near(chanceVals, s.Chance), func(v int) { s.Chance = chanceVals[v] })
		m.items = append(m.items, menuItem{label: "Reset this slice", run: func(int) {
			s.Gain, s.Pitch, s.Reverse, s.Roll, s.RollLoud, s.RollPitch, s.Attack, s.Release, s.Chance = 100, 0, false, 0, 0, 0, 0, 0, 100
			a.say(fmt.Sprintf("slice %d is as it was cut", sel+1), false)
		}})
	}
	parts := []int{2, 4, 8, 16, 32}
	m.items = append(m.items, menuItem{label: "Cut in equal parts", vals: names(parts, func(v int) string { return fmt.Sprint(v) }), val: 2,
		run: func(v int) { a.equalParts(parts[v]) }})
	limits := []int{4, 8, 16, 24, 32}
	set("Auto-cut (Y+A): up to", names(limits, func(v int) string { return fmt.Sprintf("%d slices", v) }), near(limits, a.autoLimit()),
		func(v int) { a.autoMax = limits[v] })
	sens := []float64{0.2, 0.35, 0.5, 0.7, 0.9}
	cur := 2
	for i, v := range sens {
		if v == a.autoSensitivity() {
			cur = i
		}
	}
	set("Auto-cut (Y+A): sensitivity", []string{"low", "less", "normal", "more", "high"}, cur, func(v int) { a.autoSens = sens[v] })
	set("Play order", modeNames, int(a.sl.Mode), func(v int) { a.sl.Mode = formats.PlayMode(v) })
	set("Grid", gridNames, near(formats.Divisions, a.sl.Division), func(v int) { a.sl.Division = formats.Divisions[v] })
	set("Swing", names(formats.Swings, pct), near(formats.Swings, a.sl.Swing), func(v int) { a.sl.Swing = formats.Swings[v] })
	set("Humanize", names(formats.Humanizes, pct), near(formats.Humanizes, a.sl.Humanize), func(v int) { a.sl.Humanize = formats.Humanizes[v] })
	set("Length", names(stepsVals, func(v int) string {
		if v == 0 {
			return fmt.Sprintf("auto, %d steps", lab.RoundSteps(a.sl))
		}
		return fmt.Sprintf("%d steps", v)
	}), near(stepsVals, a.sliceSteps), func(v int) { a.sliceSteps = stepsVals[v] })
	m.items = append(m.items,
		menuItem{label: "Play the whole order", keepOpen: true, run: func(int) { a.playResult(false) }},
		menuItem{label: "Play again, new dice", keepOpen: true, run: func(int) { a.playResult(true) }},
		menuItem{label: "Result to a free pad", run: func(int) { a.resultToPad() }},
		menuItem{label: "Shuffle the order (X to pads follows it)", run: func(int) { a.shuffleOrder() }},
		menuItem{label: "Slices to pads, random order", run: func(int) { a.slicesToPads(true) }},
	)
	a.menu = m
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// render plays the slicing as set (a grid takes the project's tempo) into a sample, which the caller frees.
func (a *App) renderResult(newDice bool) (formats.Sample, int, error) {
	if newDice {
		a.sl.Seed++
	}
	if a.sl.Division > 0 {
		a.sl.BPM = a.tempoOf(a.spad / 16)
	}
	sb, _ := json.Marshal(a.sl)
	mb, err := a.lab.RenderSteps(a.sid, sb, a.sliceSteps, 0)
	if err != nil {
		return formats.Sample{}, 0, err
	}
	var meta lab.Meta
	json.Unmarshal(mb, &meta)
	s, err := a.lab.Sample(meta.ID)
	return s, meta.ID, err
}

func (a *App) playResult(newDice bool) {
	s, id, err := a.renderResult(newDice)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	a.lab.Free(id)
	a.play(s)
	a.say(fmt.Sprintf("%s, %d steps, %.1f s", modeNames[a.sl.Mode], a.steps2(), float64(s.Frames())/formats.SampleRate), false)
}

func (a *App) resultToPad() {
	n := a.freeRun(1, a.spad+1)
	if n < 0 {
		a.say("no empty pad: clear one with Y+B on PADS", true)
		return
	}
	_, id, err := a.renderResult(false)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	name := fmt.Sprintf("%s_mix", fit(a.info.Pads[a.spad].Name, 12))
	err = a.lab.Assign(n, id, name)
	a.lab.Free(id)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	delete(a.pcm, n)
	a.refresh()
	a.say(fmt.Sprintf("the result is on %s", a.label(n)), false)
}

// shuffleOrder puts the order of the slices in a random order that stays (play order "in order" then plays it).
func (a *App) shuffleOrder() {
	a.sl.Seed++
	sb, _ := json.Marshal(a.sl)
	out, err := lab.ShuffleOrder(sb, a.sl.Seed)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	sl := &formats.Slicing{}
	if json.Unmarshal(out, sl) != nil {
		return
	}
	a.sl.Order = sl.Order
	a.sl.Mode = formats.ModeOrder
	a.say("order: "+orderText(a.sl.Order), false)
}

// steps2 is how many steps the result has: the length set in the menu, else one round.
func (a *App) steps2() int {
	if a.sliceSteps > 0 {
		return a.sliceSteps
	}
	return lab.RoundSteps(a.sl)
}

// orderText is the order as numbers from 1, "3 1 4 2".
func orderText(o []int) string {
	var sb strings.Builder
	for i, v := range o {
		if i > 0 {
			sb.WriteByte(' ')
		}
		fmt.Fprint(&sb, v+1)
	}
	return sb.String()
}
