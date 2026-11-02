package handheld

import (
	"fmt"
	"math/rand"

	"github.com/moahbeatlab/404tui/formats"
)

// The dice of the web slicer's MANGLE page, for the slicer: how many of the slices drop out, play backwards, get a pitch
// (up or down by up to a range) or a roll, how far the loudness and timing wander, and the fades at the slice ends. The
// slices they hit are fixed by the roll, so one more slice keeps the ones it had. A change of any setting plays the result.

type mangleSet struct {
	drop, rev, pitchN, roll int
	pitchRange              int // semitones
	human, fade             int
	seed                    int64
}

var (
	manglePitchRange = []int{2, 5, 12}
	mangleHumanVals  = []int{0, 25, 50, 100}
	mangleFadeVals   = []int{0, 10, 50, 200}
)

// mangleSlicing applies the dice to a copy of the slicing and returns it.
func mangleSlicing(base *formats.Slicing, m mangleSet) *formats.Slicing {
	sl := base.Clone()
	dice := rand.New(rand.NewSource(m.seed ^ 0x2545f4914f6cdd1d))
	n := len(sl.Slices)
	chosen := func(k int) []bool {
		is := make([]bool, n)
		for _, i := range dice.Perm(n)[:min(max(k, 0), n)] {
			is[i] = true
		}
		return is
	}
	drop, rev, pitched, roll := chosen(m.drop), chosen(m.rev), chosen(m.pitchN), chosen(m.roll)
	for i := range sl.Slices {
		sc := &sl.Slices[i]
		rollPick, pitchDie := dice.Intn(len(formats.Rolls)-1), dice.Float64()
		sc.Chance = 100
		if drop[i] {
			sc.Chance = 0
		}
		sc.Reverse = rev[i]
		sc.Attack, sc.Release = m.fade, m.fade
		sc.Roll, sc.Pitch = 0, 0
		if roll[i] {
			sc.Roll = formats.Rolls[1+rollPick]
		}
		if pitched[i] {
			sc.Pitch = int(pitchDie*float64(2*m.pitchRange+1)) - m.pitchRange
		}
	}
	sl.Humanize = m.human
	return sl
}

// mangleMenu is the dialog: settings play the result at once, "Keep it" stays, B puts the slices back as they were.
func (a *App) mangleMenu() {
	if a.sl == nil {
		return
	}
	base := a.sl.Clone()
	n := len(base.Slices)
	if a.mg.seed == 0 {
		a.mg = mangleSet{pitchRange: 5, seed: 1}
	}
	counts := seq(0, n, 1)
	if n > 12 { // a long list: steps of a few
		counts = nil
		for i := 0; i <= n; i += max(n/8, 1) {
			counts = append(counts, i)
		}
	}
	apply := func() {
		keep := a.sl.Seed
		a.sl = mangleSlicing(base, a.mg)
		a.sl.Seed = keep
		a.playResult(false)
	}
	m := &menu{title: fmt.Sprintf("mangle  %d slices", n)}
	set := func(label string, vals []string, cur int, change func(v int)) {
		m.items = append(m.items, menuItem{label: label, vals: vals, val: cur, keepOpen: true, onChange: func(v int) { change(v); apply() }})
	}
	cnt := func(label string, p *int) {
		set(label, names(counts, func(v int) string { return fmt.Sprint(v) }), near(counts, *p), func(v int) { *p = counts[v] })
	}
	cnt("Drop-outs (silent slices)", &a.mg.drop)
	cnt("Reversed slices", &a.mg.rev)
	cnt("Pitched slices", &a.mg.pitchN)
	set("  pitch range (+/- semitones)", names(manglePitchRange, func(v int) string { return fmt.Sprint(v) }), near(manglePitchRange, a.mg.pitchRange), func(v int) { a.mg.pitchRange = manglePitchRange[v] })
	cnt("Slices with a roll", &a.mg.roll)
	set("Humanize (loudness, timing)", names(mangleHumanVals, pct), near(mangleHumanVals, a.mg.human), func(v int) { a.mg.human = mangleHumanVals[v] })
	set("Fades at the slice ends", names(mangleFadeVals, ms), near(mangleFadeVals, a.mg.fade), func(v int) { a.mg.fade = mangleFadeVals[v] })
	m.items = append(m.items,
		menuItem{label: "Roll the dice again", keepOpen: true, run: func(int) { a.mg.seed++; apply() }},
		menuItem{label: "Keep it (B puts the slices back)", run: func(int) {
			base = a.sl.Clone() // what is there stays; the menu closing must not undo it
			a.say("mangled: Y+B for the menu, A cuts on", false)
		}})
	m.onClose = func() { a.sl = base }
	a.menu = m
	apply()
}
