// Package perform is what the live view works from: the MIDI the unit sends (pad hits, controllers, the clock), kept as a
// state that a screen can draw. It has no screen of its own, so every platform shows it the same way.
package perform

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/moahbeatlab/404tui/midiin"
)

const (
	monLines = 200 // messages kept for the list
	// monHot is how long a pad hit shows as a hit when no note off comes.
	monHot = 250 * time.Millisecond
	// monMoving is how long a controller counts as moving after its last value.
	monMoving = 8 * time.Second
)

type Line struct {
	At   time.Time
	Text string
}

type CC struct {
	Chan, Num, Val int
	At             time.Time
	First          int // the order controllers were first seen in
}

type Monitor struct {
	lines  []Line
	counts map[midiin.Kind]int
	hits   map[int]time.Time // bank*16 + pad (0 to 15) -> when it was last hit
	vel    map[int]int       // ... and how hard
	offs   map[int]time.Time // ... and when it was let go
	held   map[int]bool
	ccs    map[[2]int]*CC
	BeatAt time.Time
	BPM    float64
	seen   int
	// padAt is when any pad of the unit was last heard.
	PadAt time.Time
	// played holds the pads (bank*16 + pad) heard since the sequencer started: what a pattern plays.
	played map[int]bool
	// playedAt is the clock tick each of them was last heard on: a pad that has not been heard for a
	// long time belongs to a pattern that played before.
	playedAt map[int]int
	// hitLog are the pads heard lately (bank*16 + pad) with the clock tick of each.
	hitLog []hitAt
}

func New() *Monitor {
	return &Monitor{
		counts: map[midiin.Kind]int{},
		hits:   map[int]time.Time{},
		vel:    map[int]int{},
		offs:   map[int]time.Time{},
		held:   map[int]bool{},
		ccs:    map[[2]int]*CC{},
	}
}

// padOfEvent is the bank (0 to 9) and pad (0 to 15) a note stands for, when it is one of the unit's.
func PadOfEvent(e midiin.Event) (bank, pad int, ok bool) {
	if (e.Kind != midiin.NoteOn && e.Kind != midiin.NoteOff) || e.Channel < 1 || e.Channel > 10 {
		return 0, 0, false
	}
	p, ok := PadOfNote(e.Data1)
	return e.Channel - 1, p - 1, ok
}

// describeMIDI says a message in words.
func Describe(e midiin.Event) string {
	switch e.Kind {
	case midiin.NoteOn, midiin.NoteOff:
		if bank, pad, ok := PadOfEvent(e); ok {
			s := fmt.Sprintf("%-8s bank %c pad %-2d", e.Kind, 'A'+bank, pad+1)
			if e.Kind == midiin.NoteOn {
				s += fmt.Sprintf(" vel %d", e.Data2)
			}
			return s
		}
		return fmt.Sprintf("%-8s ch %-2d note %d vel %d", e.Kind, e.Channel, e.Data1, e.Data2)
	case midiin.CC:
		return fmt.Sprintf("cc       ch %-2d %d = %d", e.Channel, e.Data1, e.Data2)
	case midiin.ProgramChange:
		return fmt.Sprintf("program  ch %-2d %d", e.Channel, e.Data1)
	case midiin.PitchBend:
		return fmt.Sprintf("bend     ch %-2d %d", e.Channel, e.Data1)
	case midiin.Pressure:
		return fmt.Sprintf("pressure ch %-2d key %d = %d", e.Channel, e.Data1, e.Data2)
	case midiin.Sysex:
		n := min(len(e.Raw), 12)
		return fmt.Sprintf("sysex    %d bytes % X", len(e.Raw), e.Raw[:n])
	}
	return e.String()
}

// add takes a message in. The clock is only counted (see tick); everything else is listed.
func (mo *Monitor) Add(e midiin.Event) {
	mo.counts[e.Kind]++
	if e.Kind == midiin.Clock {
		return
	}
	at := e.At
	if at.IsZero() {
		at = time.Now()
	}
	if bank, pad, ok := PadOfEvent(e); ok {
		key := bank*16 + pad
		if e.Kind == midiin.NoteOn {
			mo.hits[key], mo.held[key], mo.vel[key] = at, true, e.Data2
			mo.PadAt = at
		} else {
			delete(mo.held, key)
			mo.offs[key] = at
		}
	}
	if e.Kind == midiin.CC {
		k := [2]int{e.Channel, e.Data1}
		if mo.ccs[k] == nil {
			mo.seen++
			mo.ccs[k] = &CC{Chan: e.Channel, Num: e.Data1, First: mo.seen}
		}
		mo.ccs[k].Val, mo.ccs[k].At = e.Data2, at
	}
	mo.lines = append(mo.lines, Line{At: at, Text: Describe(e)})
	if len(mo.lines) > monLines {
		mo.lines = mo.lines[len(mo.lines)-monLines:]
	}
}

// tick takes in the clock tick that made the count ticks; on a beat the tempo is worked out.
func (mo *Monitor) Tick(at time.Time, ticks int) {
	if (ticks-1)%24 != 0 {
		return
	}
	if !mo.BeatAt.IsZero() {
		if dt := at.Sub(mo.BeatAt).Seconds(); dt > 0.1 && dt < 3 {
			bpm := 60 / dt
			if mo.BPM == 0 {
				mo.BPM = bpm
			} else {
				mo.BPM = mo.BPM*0.5 + bpm*0.5
			}
		}
	}
	mo.BeatAt = at
}

// monFade is how long the glow of a pad takes to die away once it is let go.
const monFade = 700 * time.Millisecond

// glow is how brightly a pad shows, 0 to 1: a hit lights it by how hard it was hit, it stays so while
// held and fades after.
func (mo *Monitor) Glow(bank, pad int, now time.Time) float64 {
	if mo == nil {
		return 0
	}
	key := bank*16 + pad
	hit, ok := mo.hits[key]
	if !ok {
		return 0
	}
	level := 0.45 + 0.55*float64(mo.vel[key])/127
	if mo.held[key] && now.Sub(hit) < 5*time.Second {
		return level
	}
	since := hit
	if off := mo.offs[key]; off.After(since) {
		since = off
	}
	// A message with no note off still fades, from the hit.
	if age := now.Sub(since); age < monFade {
		return level * (1 - float64(age)/float64(monFade))
	}
	return 0
}

// hot says whether a pad of a bank is lit by a hit.
func (mo *Monitor) Hot(bank, pad int, now time.Time) bool {
	return mo.Glow(bank, pad, now) > 0
}

// moving lists the controllers that moved lately, the newest first.
func (mo *Monitor) Moving(now time.Time) []*CC {
	if mo == nil {
		return nil
	}
	var out []*CC
	for _, c := range mo.ccs {
		if now.Sub(c.At) < monMoving {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].At.Equal(out[j].At) {
			return out[i].At.After(out[j].At)
		}
		return out[i].Num < out[j].Num
	})
	return out
}

// last is the newest n messages as text, the newest last.
func (mo *Monitor) Last(n int) []Line {
	if mo == nil || len(mo.lines) == 0 {
		return nil
	}
	l := mo.lines
	if len(l) > n {
		l = l[len(l)-n:]
	}
	return l
}

// summary counts the messages by kind, for Config.
func (mo *Monitor) Summary() string {
	if mo == nil {
		return ""
	}
	var parts []string
	for k := midiin.NoteOn; k <= midiin.Sysex; k++ {
		if n := mo.counts[k]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", k, n))
		}
	}
	text := strings.Join(parts, " · ")
	if mo.BPM > 0 {
		text += fmt.Sprintf(" · clock ≈ %.1f BPM", mo.BPM)
	}
	return text
}

// controls lists every controller seen, in the order it was first seen.
func (mo *Monitor) Controls() []*CC {
	if mo == nil {
		return nil
	}
	out := make([]*CC, 0, len(mo.ccs))
	for _, c := range mo.ccs {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].First < out[j].First })
	return out
}

// heard notes that a pad of a pattern played, on the clock tick it did, and forgets the pads that
// were last heard more than eight bars ago: they are from a pattern that has been left.
func (mo *Monitor) Heard(key, tick int) {
	if mo.played == nil {
		mo.played = map[int]bool{}
	}
	if mo.playedAt == nil {
		mo.playedAt = map[int]int{}
	}
	mo.played[key], mo.playedAt[key] = true, tick
	// The hits are kept for a while, with their ticks: they show where the pattern that plays began.
	mo.hitLog = append(mo.hitLog, hitAt{key, tick})
	if len(mo.hitLog) > 600 {
		mo.hitLog = mo.hitLog[len(mo.hitLog)-400:]
	}
	for k, at := range mo.playedAt {
		if at > tick || tick-at > 8*TicksPerBar {
			delete(mo.played, k)
			delete(mo.playedAt, k)
		}
	}
}

// heardSince says whether a pad was heard within the last ticks of the clock.
func (mo *Monitor) HeardSince(key, tick, ticks int) bool {
	at, ok := mo.playedAt[key]
	return ok && at <= tick && tick-at <= ticks
}

// heardWithin is the pads heard within the last ticks of the clock.
func (mo *Monitor) HeardWithin(tick, ticks int) map[int]bool {
	out := map[int]bool{}
	for k, at := range mo.playedAt {
		if at <= tick && tick-at <= ticks {
			out[k] = true
		}
	}
	return out
}

// setCC notes a controller value that this program sent to the unit, so the faders show it as they
// would when the unit says it.
func (mo *Monitor) SetCC(ch, cc, val int, at time.Time) {
	k := [2]int{ch, cc}
	if mo.ccs[k] == nil {
		mo.seen++
		mo.ccs[k] = &CC{Chan: ch, Num: cc, First: mo.seen}
	}
	mo.ccs[k].Val, mo.ccs[k].At = val, at
}

// hitAt is a pad heard on a clock tick.
type hitAt struct{ key, tick int }

// TicksPerBar is the clock ticks of a bar of 4/4: 24 to a quarter note.
const TicksPerBar = 4 * 24

// PadOfNote is the pad (1 to 16) a note of the unit's pad channels stands for.
func PadOfNote(note int) (int, bool) {
	order := [16]int{12, 13, 14, 15, 8, 9, 10, 11, 4, 5, 6, 7, 0, 1, 2, 3}
	if note < 36 || note > 51 {
		return 0, false
	}
	for pad, off := range order {
		if off == note-36 {
			return pad + 1, true
		}
	}
	return 0, false
}

// Clock is the unit's MIDI clock as counted: ticks since it started, whether it runs, when the last tick came.
type Clock struct {
	Ticks   int
	Running bool
	At      time.Time
}

// Beat is the beat of the bar (0 to 3) and the bar (from 0) the clock is at.
func (c Clock) Beat() (bar, beat int) {
	t := max(c.Ticks-1, 0)
	return t / TicksPerBar, t / 24 % 4
}

// Feed takes in a message: the monitor notes it, the clock counts, and the tempo is worked out on the beats.
func (mo *Monitor) Feed(c *Clock, e midiin.Event) {
	mo.Add(e)
	at := e.At
	if at.IsZero() {
		at = time.Now()
	}
	switch e.Kind {
	case midiin.Start:
		c.Ticks, c.Running, c.At = 0, true, at
	case midiin.Continue:
		c.Running, c.At = true, at
	case midiin.Stop:
		c.Running = false
	case midiin.Clock:
		if !c.Running && c.Ticks == 0 { // a clock without a Start: counted from here
			c.Running = true
		}
		c.Ticks++
		c.At = at
		mo.Tick(at, c.Ticks)
	}
}

// Reset forgets the pads heard (a new start of the sequencer begins with nothing heard).
func (mo *Monitor) Reset() {
	mo.BeatAt = time.Time{}
	mo.played, mo.playedAt = map[int]bool{}, map[int]int{}
}

// CCValue is the last value of a controller on a channel, -1 when none has come.
func (mo *Monitor) CCValue(ch, num int) int {
	if c := mo.ccs[[2]int{ch, num}]; c != nil {
		return c.Val
	}
	return -1
}

// HitAt is when a pad (bank 0 to 9, pad 0 to 15) was last hit.
func (mo *Monitor) HitAt(bank, pad int) (time.Time, bool) {
	t, ok := mo.hits[bank*16+pad]
	return t, ok
}
