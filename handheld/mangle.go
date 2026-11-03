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
	pitchRange              int // semitones of a random pitch
	pitchFix                int // an index of manglePitchFix: 0 is random in the range, else that many semitones
	rollFix                 int // 0: random hits, else that many
	which                   int // which slices the counts hit: an index of mangleWhich
	human, fade             int
	seed                    int64
}

var (
	// the slices a count hits: the dice, spread evenly over the slices, the first ones, the last ones
	mangleWhich      = []string{"at random (the dice)", "spread evenly", "the first ones", "the last ones"}
	manglePitchFix   = []int{0, -12, -7, -5, -3, -2, -1, 1, 2, 3, 5, 7, 12}
	mangleRollFix    = []int{0, 2, 3, 4, 6, 8}
	manglePitchRange = []int{2, 5, 12}
	mangleHumanVals  = []int{0, 25, 50, 100}
	mangleFadeVals   = []int{0, 10, 50, 200}
)

// mangleSlicing applies the dice to a copy of the slicing and returns it.
func mangleSlicing(base *formats.Slicing, m mangleSet) *formats.Slicing {
	sl := base.Clone()
	dice := rand.New(rand.NewSource(m.seed ^ 0x2545f4914f6cdd1d))
	n := len(sl.Slices)
	chosen := func(k, effect int) []bool {
		is := make([]bool, n)
		perm := dice.Perm(n) // rolled for every effect, so a choice of slices does not change the next roll
		k = min(max(k, 0), n)
		switch m.which {
		case 1: // spread evenly; each effect starts a slice later, so that they do not all fall on the same ones
			for i := 0; i < k; i++ {
				is[(i*n/k+effect)%n] = true
			}
		case 2:
			for i := 0; i < k; i++ {
				is[i] = true
			}
		case 3:
			for i := n - k; i < n; i++ {
				is[i] = true
			}
		default:
			for _, i := range perm[:k] {
				is[i] = true
			}
		}
		return is
	}
	drop, rev, pitched, roll := chosen(m.drop, 0), chosen(m.rev, 1), chosen(m.pitchN, 2), chosen(m.roll, 3)
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
			if f := mangleRollFix[min(max(m.rollFix, 0), len(mangleRollFix)-1)]; f > 0 {
				sc.Roll = f
			}
		}
		if pitched[i] {
			sc.Pitch = int(pitchDie*float64(2*m.pitchRange+1)) - m.pitchRange
			if f := manglePitchFix[min(max(m.pitchFix, 0), len(manglePitchFix)-1)]; f != 0 {
				sc.Pitch = f
			}
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
	counts := seq(0, n, 1) // exact numbers of slices, one by one (L1 R1 jump ten)
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
	set("Which slices", mangleWhich, a.mg.which, func(v int) { a.mg.which = v })
	cnt("Drop-outs (silent slices)", &a.mg.drop)
	cnt("Reversed slices", &a.mg.rev)
	cnt("Pitched slices", &a.mg.pitchN)
	set("  pitch by", names(manglePitchFix, func(v int) string {
		if v == 0 {
			return "random in the range"
		}
		return fmt.Sprintf("%+d semitones", v)
	}), a.mg.pitchFix, func(v int) { a.mg.pitchFix = v })
	set("  random range (+/-)", names(manglePitchRange, func(v int) string { return fmt.Sprintf("%d semitones", v) }), near(manglePitchRange, a.mg.pitchRange), func(v int) { a.mg.pitchRange = manglePitchRange[v] })
	cnt("Slices with a roll", &a.mg.roll)
	set("  roll hits", names(mangleRollFix, func(v int) string {
		if v == 0 {
			return "random"
		}
		return fmt.Sprintf("%d hits", v)
	}), a.mg.rollFix, func(v int) { a.mg.rollFix = v })
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
