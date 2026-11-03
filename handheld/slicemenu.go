//go:build !windows

package handheld

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

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

// sliceMenu is the settings of the slicer (Y+B): the slice in front first, then how the order plays.
func (a *App) sliceMenu() {
	if a.sl == nil {
		return
	}
	sel := a.sliceAt(a.at)
	m := &menu{title: fmt.Sprintf("slice settings  %d slices", len(a.sl.Slices))}
	set := func(label string, vals []string, cur int, change func(v int)) {
		m.items = append(m.items, menuItem{label: label, vals: vals, val: cur, keepOpen: true, onChange: change})
	}
	if sel >= 0 {
		m.tabHead(fmt.Sprintf("SLICE %d", sel+1))
		s := &a.sl.Slices[sel]
		audition := func() { a.playSlice() }
		set("Pitch", names(pitchVals, signed), near(pitchVals, s.Pitch), func(v int) { s.Pitch = pitchVals[v]; audition() })
		set("Gain", names(gainVals, pct), near(gainVals, s.Gain), func(v int) { s.Gain = gainVals[v]; audition() })
		set("Reverse", []string{"off", "on"}, b2i(s.Reverse), func(v int) { s.Reverse = v == 1; audition() })
		set("Fade in", names(formats.EnvSteps, ms), near(formats.EnvSteps, s.Attack), func(v int) { s.Attack = formats.EnvSteps[v]; audition() })
		set("Fade out", names(formats.EnvSteps, ms), near(formats.EnvSteps, s.Release), func(v int) { s.Release = formats.EnvSteps[v]; audition() })
		set("Roll", names(formats.Rolls, func(v int) string {
			if v == 0 {
				return "off"
			}
			return fmt.Sprintf("%d hits", v)
		}), near(formats.Rolls, s.Roll), func(v int) { s.Roll = formats.Rolls[v]; audition() })
		set("Roll louder", names(loudVals, signed), near(loudVals, s.RollLoud), func(v int) { s.RollLoud = loudVals[v]; audition() })
		set("Roll pitch", names(pitchVals, signed), near(pitchVals, s.RollPitch), func(v int) { s.RollPitch = pitchVals[v]; audition() })
		set("Chance", names(chanceVals, pct), near(chanceVals, s.Chance), func(v int) { s.Chance = chanceVals[v] })
		m.items = append(m.items, menuItem{label: "Reset this slice", run: func(int) {
			s.Gain, s.Pitch, s.Reverse, s.Roll, s.RollLoud, s.RollPitch, s.Attack, s.Release, s.Chance = 100, 0, false, 0, 0, 0, 0, 0, 100
			a.say(fmt.Sprintf("slice %d is as it was cut", sel+1), false)
		}})
	}
	m.tabHead("PLAY")
	set("Play order", modeNames, int(a.sl.Mode), func(v int) { a.sl.Mode = formats.PlayMode(v) })
	bpm := a.tempoOf(a.spad / 16)
	set(fmt.Sprintf("Grid, at %s BPM of bank %c", bpmText(bpm), 'A'+rune(a.spad/16)), gridNames, near(formats.Divisions, a.sl.Division), func(v int) { a.sl.Division = formats.Divisions[v] })
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
		menuItem{label: "Tools (Select+B)...", run: func(int) { a.sliceToolsMenu() }},
	)
	m.tabify()
	a.menu = m
}

// sliceToolsMenu is what is done with the slices (Select+B): cutting them, changing them, keeping the result.
func (a *App) sliceToolsMenu() {
	if a.sl == nil {
		return
	}
	m := &menu{title: fmt.Sprintf("slice tools  %d slices", len(a.sl.Slices))}
	set := func(label string, vals []string, cur int, change func(v int)) {
		m.items = append(m.items, menuItem{label: label, vals: vals, val: cur, keepOpen: true, onChange: change})
	}
	m.tabHead("CUT")
	parts := []int{2, 4, 8, 16, 32}
	m.items = append(m.items, menuItem{label: "Cut in equal parts", vals: names(parts, func(v int) string { return fmt.Sprint(v) }), val: 2,
		run: func(v int) { a.equalParts(parts[v]) }})
	m.items = append(m.items, menuItem{label: "Cut at random places", vals: names(parts, func(v int) string { return fmt.Sprint(v) }), val: 2,
		run: func(v int) { a.randomParts(parts[v]) }})
	grids := []float64{1, 0.5, 0.25, 0.125}
	m.items = append(m.items, menuItem{label: fit(fmt.Sprintf("Cut on the grid at %.0f BPM", a.tempo()), 40), vals: []string{"1/4", "1/8", "1/16", "1/32"}, val: 1,
		run: func(v int) { a.gridParts(grids[v]) }})
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
	m.tabHead("ORDER")
	m.items = append(m.items,
		menuItem{label: "Mangle the slices (dice)...", run: func(int) { a.mangleMenu() }},
		menuItem{label: "Shuffle the order", run: func(int) { a.shuffleOrder() }},
		menuItem{label: "Play order back to 1, 2, 3...", run: func(int) {
			a.sl.ResetOrder()
			a.say("order: "+orderText(a.sl.Order), false)
		}},
	)
	m.tabHead("KEEP")
	m.items = append(m.items,
		menuItem{label: "Result to a pad (choose bank and pad)...", run: func(int) { a.resultToPad() }},
		menuItem{label: "Slices to pads (choose the first)...", run: func(int) { a.slicesToPadsPick(false) }},
		menuItem{label: "Slices to pads, random order...", run: func(int) { a.slicesToPadsPick(true) }},
		menuItem{label: "Settings: slice, order, grid (Y+B)...", run: func(int) { a.sliceMenu() }},
	)
	m.tabify()
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
	var meta lab.Rendered
	json.Unmarshal(mb, &meta)
	a.placed = meta.Placed
	s, err := a.lab.Sample(meta.ID)
	return s, meta.ID, err
}

// slicePlan is the steps the order plays as it is set now (the mode, the order, the seed and the length), with no sound made: the
// row under the waveform shows it whenever something in the menus changes.
func (a *App) slicePlan() []formats.Step {
	if a.sl == nil || len(a.sl.Order) == 0 || len(a.sl.Slices) == 0 {
		return nil
	}
	sl := a.sl.Clone()
	n := min(a.steps2(), 256)
	q := formats.NewSequencer(sl)
	out := make([]formats.Step, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, q.Next())
	}
	return out
}

// playingStep is the step of the order that sounds now (-1 when none does): the result that was played, by the time since.
func (a *App) playingStep() int {
	if len(a.placed) == 0 || a.playT0.IsZero() {
		return -1
	}
	frame := int(time.Since(a.playT0).Seconds() * formats.SampleRate)
	for i, p := range a.placed {
		if frame >= p.At && frame < p.At+p.Frames {
			return i
		}
	}
	return -1
}

func (a *App) playResult(newDice bool) {
	s, id, err := a.renderResult(newDice)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	a.lab.Free(id)
	a.play(s)
	a.playT0 = time.Now()
	a.say(fmt.Sprintf("%s, %d steps, %.1f s", modeNames[a.sl.Mode], a.steps2(), float64(s.Frames())/formats.SampleRate), false)
}

// resultToPad asks for the pad (bank, then pad) and puts the result of the order there.
func (a *App) resultToPad() {
	a.pickFreePad("Put the result on a pad", a.spad+1, func(n int) { a.resultTo(n) })
}

func (a *App) resultTo(n int) {
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

// cycleGrid is L2 on the slicer: the next step of the grid (off, 1/2 ... 1/16), and the order is heard in it.
func (a *App) cycleGrid() {
	if a.sl == nil {
		return
	}
	i := (near(formats.Divisions, a.sl.Division) + 1) % len(formats.Divisions)
	a.sl.Division = formats.Divisions[i]
	a.say("grid "+gridNames[i], false)
	a.playResult(false)
	a.say("grid "+gridNames[i]+", "+fmt.Sprintf("%d steps", a.steps2()), false)
}
