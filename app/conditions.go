package app

import (
	"fmt"
	"math/rand"
	"slices"
	"strings"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// Cond says when a note plays over the loops of a pattern, like the trig conditions of other
// sequencers. The unit's pattern files have no place for it, so it is kept beside the pattern
// and resolved when the pattern is played or bounced on this computer.
type Cond struct {
	// A and B make it play on loop A of every B loops (A is 1..B); B 0 is every loop.
	A, B int
	// Prob is the chance, in percent, that it plays when its loop comes; 0 and 100 are always.
	Prob int
}

// Plain is a condition that changes nothing: the note plays on every loop.
func (c Cond) Plain() bool { return c.B <= 1 && (c.Prob <= 0 || c.Prob >= 100) }

// Chancy says whether chance is in it, so that two bounces can differ.
func (c Cond) Chancy() bool { return c.Prob > 0 && c.Prob < 100 }

func (c Cond) String() string {
	switch {
	case c.Plain():
		return "every loop"
	case c.B > 1 && c.Chancy():
		return fmt.Sprintf("%d:%d · %d%%", c.A, c.B, c.Prob)
	case c.B > 1:
		return fmt.Sprintf("%d:%d", c.A, c.B)
	}
	return fmt.Sprintf("%d%%", c.Prob)
}

// Explain says in plain words when the note plays: "only on loop 1 of every 2 loops (...)".
func (c Cond) Explain() string {
	when := "on every loop"
	if c.B > 1 {
		var nth []string
		for k := 0; k < 3; k++ {
			nth = append(nth, ordinal(k*c.B+c.A))
		}
		when = fmt.Sprintf("only on loop %d of every %d loops (the %s ... time round)", c.A, c.B, strings.Join(nth, ", "))
	}
	if c.Chancy() {
		if c.B > 1 {
			return fmt.Sprintf("%s, and then only %d%% of the time", when, c.Prob)
		}
		return fmt.Sprintf("%d%% of the time, rolled again on every loop", c.Prob)
	}
	return when
}

func ordinal(n int) string {
	switch {
	case n%100 >= 11 && n%100 <= 13:
		return fmt.Sprintf("%dth", n)
	case n%10 == 1:
		return fmt.Sprintf("%dst", n)
	case n%10 == 2:
		return fmt.Sprintf("%dnd", n)
	case n%10 == 3:
		return fmt.Sprintf("%drd", n)
	}
	return fmt.Sprintf("%dth", n)
}

// Plays says whether the note plays on loop (counted from 0) when the dice have shown roll
// (0..99): the loop has to be the one asked for, and the roll under the chance.
func (c Cond) Plays(loop, roll int) bool {
	if c.B > 1 && loop%c.B != c.A-1 {
		return false
	}
	return !c.Chancy() || roll < c.Prob
}

// CondSteps are the lengths of a cycle that can be chosen, in loops.
var CondSteps = []int{2, 3, 4, 8}

// CondChances are the chances that can be chosen, in percent.
var CondChances = []int{90, 75, 50, 25, 10}

// CondCatalog is every condition that can be chosen, in the order c steps through it: always,
// then 1:2 2:2, 1:3 2:3 3:3 and so on, then the chances.
func CondCatalog() []Cond {
	out := []Cond{{}}
	for _, b := range CondSteps {
		for a := 1; a <= b; a++ {
			out = append(out, Cond{A: a, B: b})
		}
	}
	for _, p := range CondChances {
		out = append(out, Cond{Prob: p})
	}
	return out
}

// StepCond moves one place on (dir 1) or back (dir -1) through the catalog from c, round again
// at the ends.
func StepCond(c Cond, dir int) Cond {
	cat := CondCatalog()
	at := slices.Index(cat, c)
	if at < 0 {
		at = 0
	}
	return cat[((at+dir)%len(cat)+len(cat))%len(cat)]
}

// NoteKey names a note for its condition: the pad and the sixteenth the note starts in, so a
// small nudge or a swing keeps the condition with the note.
type NoteKey struct {
	Pad  proto.PadIndex
	Step int
}

// sixteenth is how many ticks a sixteenth note lasts.
const sixteenth = formats.PPQ / 4

// KeyOf is the key of the note an event is.
func KeyOf(e formats.Event) NoteKey { return NoteKey{Pad: e.Pad, Step: int(e.Tick) / sixteenth} }

// Conditions are the conditions of a pattern's notes; a note that is not in it plays always.
type Conditions map[NoteKey]Cond

// CycleLoops is after how many loops the pattern is the same again: the least common multiple
// of the cycles of the conditions in it, 1 with none.
func (cs Conditions) CycleLoops() int {
	loops := 1
	for _, c := range cs {
		if c.B > 1 {
			loops = lcm(loops, c.B)
		}
	}
	return loops
}

// Chancy says whether any condition has a chance in it, so that the cycle is one roll of many.
func (cs Conditions) Chancy() bool {
	for _, c := range cs {
		if c.Chancy() {
			return true
		}
	}
	return false
}

// Loops is how many loops a bounce or a loop that plays runs over: the cycle, and when a chance
// is in it at least rolls loops, so that the chances get a few loops to show in.
func (cs Conditions) Loops(rolls int) int {
	loops := cs.CycleLoops()
	if cs.Chancy() {
		loops = max(loops, rolls)
	}
	return loops
}

// PrunedTo is the conditions that are not plain, of the notes a pattern file has.
func (cs Conditions) PrunedTo(notes []formats.NoteRecord) Conditions {
	out := Conditions{}
	for _, n := range notes {
		key := NoteKey{Pad: n.Pad, Step: int(n.Tick) / sixteenth}
		if c, ok := cs[key]; ok && !c.Plain() {
			out[key] = c
		}
	}
	return out
}

// Pruned is the conditions that are not plain, of the notes the pattern has.
func (cs Conditions) Pruned(p formats.Pattern) Conditions {
	out := Conditions{}
	for _, e := range p.Events {
		if e.Control {
			continue
		}
		if c, ok := cs[KeyOf(e)]; ok && !c.Plain() {
			out[KeyOf(e)] = c
		}
	}
	return out
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func lcm(a, b int) int { return a / gcd(a, b) * b }

// Unroll is the pattern as it plays over loops loops, one after the other, each note where its
// condition lets it: the pattern is loops times as long, and a note that does not play on a
// loop is not in it. Chances come from seed, so a seed gives the same loops every time.
func Unroll(p formats.Pattern, cs Conditions, loops int, seed int64) formats.Pattern {
	loops = max(loops, 1)
	out := formats.Pattern{Length: p.Length * uint32(loops), Trailer: p.Trailer}
	rng := rand.New(rand.NewSource(seed))
	for loop := 0; loop < loops; loop++ {
		for _, e := range p.Events {
			if !e.Control {
				// Every note rolls, so that one note's condition does not change the dice of the next.
				roll := rng.Intn(100)
				if !cs[KeyOf(e)].Plays(loop, roll) {
					continue
				}
			}
			e.Tick += uint32(loop) * p.Length
			out.Events = append(out.Events, e)
		}
	}
	return out
}

// CycleOf says how long a pattern of length ticks (beats to the bar) is once its conditions
// have come round: in loops, bars and seconds at a tempo. rolls is how many loops a pattern with
// chances in it is rolled over when its cycle is shorter.
func CycleOf(length uint32, beats int, bpm float64, cs Conditions, rolls int) (loops, bars int, seconds float64) {
	loops = cs.Loops(rolls)
	barTicks := uint32(max(beats, 1)) * formats.PPQ
	bars = int((length*uint32(loops) + barTicks - 1) / barTicks)
	if bpm > 0 {
		seconds = float64(length) * float64(loops) / formats.PPQ * 60 / bpm
	}
	return loops, bars, seconds
}

// CycleInfo is CycleOf for a parsed pattern.
func CycleInfo(p formats.Pattern, cs Conditions, bpm float64, rolls int) (loops, bars int, seconds float64) {
	return CycleOf(p.Length, p.BeatsPerBar(), bpm, cs, rolls)
}
