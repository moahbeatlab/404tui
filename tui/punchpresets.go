package tui

import "math"

// Presets for the Punch snapshots: moments to hold, each setting the effect of one to four buses. The knob values
// are the raw controller values (0 to 127) of the effect's parameters as the unit lists them, worked out with the
// helpers below: pct for a knob that shows 0 to 100, span for one with another range, pick for a list of choices.

func pct(v float64) int                { return int(math.Round(v * 127 / 100)) }
func span(v, lo, hi float64) int       { return int(math.Round((v - lo) / (hi - lo) * 127)) }
func pick(i, n int) int                { return (2*i + 1) * 64 / n }
func balance(rightPercent float64) int { return pct(rightPercent) }

// fxSet is one bus of a preset: the effect by name (empty: the bus is left alone) and its six knobs.
type fxSet struct {
	fx string
	k  [6]int
}

type punchPreset struct {
	name, note string
	bus        [4]fxSet
}

var punchPresets = []punchPreset{
	{"RISER", "high-pass filter opens, big hall", [4]fxSet{
		{"Super Filter", [6]int{pct(75), pct(45), pick(2, 3), pct(0), pct(50), 0}}, // cutoff, resonance, HPF, depth, rate, sync off
		{},
		{"Reverb", [6]int{pick(3, 4), pct(85), pct(75), pick(13, 18), pick(14, 15), 0}}, // hall 2, time, level, low cut 315, high cut flat
		{},
	}},
	{"TAPE", "worn cassette and lo-fi", [4]fxSet{
		{"Cassette Sim", [6]int{pct(40), pct(35), span(40, 0, 60), pct(55), pct(70), 0}}, // tone, hiss, age, drive, wow/flutter
		{},
		{"Lo-fi", [6]int{pick(2, 6), pick(4, 9), span(-20, -100, 100), pick(10, 17), balance(60), pct(80)}},
		{},
	}},
	{"ECHO THROW", "long tape echo and a dotted delay", [4]fxSet{
		{"Tape Echo", [6]int{span(375, 10, 800), span(78, 0, 99), pct(100), pick(3, 7), pct(30), pct(30)}}, // time, feedback, level, S+M
		{},
		{"SX Delay", [6]int{pick(8, 16), span(70, 0, 99), balance(40), -1, -1, -1}}, // 1/8 dotted, feedback, balance
		{},
	}},
	{"STUTTER", "scatter, double speed", [4]fxSet{
		{"Scatter", [6]int{pick(4, 10), pick(9, 10), 127, pick(1, 2), -1, -1}}, // type 5, depth 100, scatter on, double
		{}, {}, {},
	}},
	{"TAPE STOP", "the stopper winds the sound down", [4]fxSet{
		{"Stopper", [6]int{pct(90), pick(3, 9), pct(30), pct(60), pct(60), -1}}, // depth, rate 1/2, resonance, filter mod, amp mod
		{}, {}, {},
	}},
	{"TELEPHONE", "band-pass and a little grit", [4]fxSet{
		{"Super Filter", [6]int{pct(60), pct(55), pick(1, 3), pct(0), pct(50), 0}}, // cutoff, resonance, BPF
		{},
		{"Overdrive", [6]int{pct(40), span(20, -100, 100), balance(70), pct(80), -1, -1}}, // drive, tone, balance, level
		{},
	}},
}

// snap is the preset as a snapshot: its buses on.
func (p punchPreset) snap() PunchSnap {
	var s PunchSnap
	for b, f := range p.bus {
		s[b] = PunchBus{On: -1, Num: -1, Knobs: [6]int{-1, -1, -1, -1, -1, -1}}
		if f.fx == "" {
			continue
		}
		s[b] = PunchBus{Set: true, On: 1, Num: fxNum(b, f.fx), Knobs: f.k}
	}
	return s
}
