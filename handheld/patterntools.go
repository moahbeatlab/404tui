package handheld

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/lab"
)

// Pattern tools (from the SEQ menu): what is done with the notes of the pattern in front. The arp is the one of the
// unit's firmware (DOOM OS) made for the pads of the columns: the pads are the keys, the arp lays notes over them in time
// (type, rate, gate), and printing writes them into the pattern, where the pads' own notes are replaced.

var (
	arpTypes     = []string{"UP", "DOWN", "U/D", "U/D+", "RND", "ORD", "CHRD", "CONV", "DIV", "PNKY", "THMB"}
	arpTypeNotes = []string{"low to high", "high to low", "up and down", "up and down, ends twice", "random", "as the columns stand",
		"all keys at once", "outside in", "middle out", "up, the top key between", "up, the bottom key between"}
	printGates = []int{25, 50, 75, 100}
	printVels  = []int{127, 100, 80, 60}
	printBars  = []int{1, 2, 4, 8}
)

type arpPrint struct {
	typ, rate, gate, vel, bars int
	init                       bool
}

func (a *App) arpPrintDefaults() {
	if !a.arpp.init {
		a.arpp = arpPrint{typ: 0, rate: 3, gate: 1, vel: 1, bars: 2, init: true}
	}
}

// arpSequence is what each step plays: indices into the keys, one for a note, several for a chord. keys are in the order of
// the columns; the rng is for RND.
func arpSequence(n, typ, steps int, rng *rand.Rand) [][]int {
	if n == 0 {
		return nil
	}
	var cycle []int
	switch typ {
	case 0, 5: // UP, ORD: the keys in order (they stand in pad order across the columns)
		for i := 0; i < n; i++ {
			cycle = append(cycle, i)
		}
	case 1: // DOWN
		for i := n - 1; i >= 0; i-- {
			cycle = append(cycle, i)
		}
	case 2: // U/D: up and down, the ends once
		for i := 0; i < n; i++ {
			cycle = append(cycle, i)
		}
		for i := n - 2; i >= 1; i-- {
			cycle = append(cycle, i)
		}
	case 3: // U/D+: the highest and the lowest twice
		for i := 0; i < n; i++ {
			cycle = append(cycle, i)
		}
		for i := n - 1; i >= 0; i-- {
			cycle = append(cycle, i)
		}
	case 7: // CONV: from the outside in
		for lo, hi := 0, n-1; lo <= hi; lo, hi = lo+1, hi-1 {
			cycle = append(cycle, lo)
			if hi != lo {
				cycle = append(cycle, hi)
			}
		}
	case 8: // DIV: from the middle out
		mid := (n - 1) / 2
		cycle = append(cycle, mid)
		for d := 1; len(cycle) < n; d++ {
			if mid+d < n {
				cycle = append(cycle, mid+d)
			}
			if mid-d >= 0 {
				cycle = append(cycle, mid-d)
			}
		}
	case 9: // PNKY: up, the top key between each
		for i := 0; i < n-1; i++ {
			cycle = append(cycle, i, n-1)
		}
		if n == 1 {
			cycle = []int{0}
		}
	case 10: // THMB: up, the bottom key between each
		for i := 1; i < n; i++ {
			cycle = append(cycle, 0, i)
		}
		if n == 1 {
			cycle = []int{0}
		}
	}
	out := make([][]int, steps)
	for i := range out {
		switch typ {
		case 4: // RND
			out[i] = []int{rng.Intn(n)}
		case 6: // CHRD
			all := make([]int, n)
			for k := range all {
				all[k] = k
			}
			out[i] = all
		default:
			out[i] = []int{cycle[i%len(cycle)]}
		}
	}
	return out
}

// arpKeys are the pads of the columns that have a sample, in the order of the columns (UP puts them in pad order).
func (a *App) arpKeys(typ int) []int {
	var keys []int
	for _, p := range a.rows {
		if a.info.Pads[p].File {
			keys = append(keys, p)
		}
	}
	if typ != 5 { // everything but ORD goes by pitch, which the pads stand for in pad order
		sort.Ints(keys)
	}
	return keys
}

// printArp writes the arp over the pads of the columns into the pattern.
func (a *App) printArp() { a.printArpKeys(a.arpKeys(a.arpp.typ)) }

// printArpKeys writes the arp into the pattern: the notes of the key pads are replaced.
func (a *App) printArpKeys(keys []int) {
	st := a.arpp
	if len(keys) < 2 {
		a.say("the arp needs 2 or more pads with a sample (mark pads with Y+A, or Y+X changes a row's pad)", true)
		return
	}
	d, ok := a.doc()
	if !ok {
		bars := printBars[near(printBars, st.bars)]
		if err := a.lab.NewPattern(a.pat, bars); err != nil {
			a.say(err.Error(), true)
			return
		}
		d, _ = a.doc()
	}
	per := formats.ArpRates[st.rate].PerBeat
	stepT := formats.PPQ / per
	steps := int(d.Length) / stepT
	if steps < 1 {
		a.say("the pattern is too short for that rate", true)
		return
	}
	for _, p := range keys {
		args, _ := json.Marshal(lab.PatternArgs{Pad: p})
		a.lab.PatternOp(a.pat, "clearPad", args)
	}
	length := max(stepT*printGates[st.gate]/100, 1)
	vel := printVels[st.vel]
	n := 0
	for i, hits := range arpSequence(len(keys), st.typ, steps, rand.New(rand.NewSource(int64(a.pat)))) {
		for _, k := range hits {
			args, _ := json.Marshal(lab.PatternArgs{Pad: keys[k], Tick: uint32(i * stepT), Vel: vel, Len: length})
			if err := a.lab.PatternOp(a.pat, "insert", args); err != nil {
				a.say(err.Error(), true)
				return
			}
			n++
		}
	}
	a.step = 0
	a.refresh()
	a.replay()
	a.say(fmt.Sprintf("arp %s: %d notes on %d pads", arpTypes[st.typ], n, len(keys)), false)
}
