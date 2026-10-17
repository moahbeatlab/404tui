package tui

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/midiin"
	"github.com/moahbeatlab/404tui/perflog"
	"github.com/moahbeatlab/404tui/proto"
)

// The monitor keeps what the MIDI input said lately: the Perform view shows the pads that are hit
// and the controllers that move, Config lists the last messages, and a recording writes them to the
// event log.

const (
	monLines = 200 // messages kept for the list
	// monHot is how long a pad hit shows as a hit when no note off comes.
	monHot = 250 * time.Millisecond
	// monMoving is how long a controller counts as moving after its last value.
	monMoving = 8 * time.Second
)

type monLine struct {
	at   time.Time
	text string
}

type ccState struct {
	ch, cc, val int
	at          time.Time
	first       int // the order controllers were first seen in
}

type monitor struct {
	lines  []monLine
	counts map[midiin.Kind]int
	hits   map[int]time.Time // bank*16 + pad (0 to 15) -> when it was last hit
	vel    map[int]int       // ... and how hard
	offs   map[int]time.Time // ... and when it was let go
	held   map[int]bool
	ccs    map[[2]int]*ccState
	beatAt time.Time
	bpm    float64
	seen   int
	// padAt is when any pad of the unit was last heard.
	padAt time.Time
	// played holds the pads (bank*16 + pad) heard since the sequencer started: what a pattern plays.
	played map[int]bool
	// playedAt is the clock tick each of them was last heard on: a pad that has not been heard for a
	// long time belongs to a pattern that played before.
	playedAt map[int]int
}

func newMonitor() *monitor {
	return &monitor{
		counts: map[midiin.Kind]int{},
		hits:   map[int]time.Time{},
		vel:    map[int]int{},
		offs:   map[int]time.Time{},
		held:   map[int]bool{},
		ccs:    map[[2]int]*ccState{},
	}
}

// padOfEvent is the bank (0 to 9) and pad (0 to 15) a note stands for, when it is one of the unit's.
func padOfEvent(e midiin.Event) (bank, pad int, ok bool) {
	if (e.Kind != midiin.NoteOn && e.Kind != midiin.NoteOff) || e.Channel < 1 || e.Channel > 10 {
		return 0, 0, false
	}
	p, ok := padOfNote(e.Data1)
	return e.Channel - 1, p - 1, ok
}

// describeMIDI says a message in words.
func describeMIDI(e midiin.Event) string {
	switch e.Kind {
	case midiin.NoteOn, midiin.NoteOff:
		if bank, pad, ok := padOfEvent(e); ok {
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
func (mo *monitor) add(e midiin.Event) {
	mo.counts[e.Kind]++
	if e.Kind == midiin.Clock {
		return
	}
	at := e.At
	if at.IsZero() {
		at = time.Now()
	}
	if bank, pad, ok := padOfEvent(e); ok {
		key := bank*16 + pad
		if e.Kind == midiin.NoteOn {
			mo.hits[key], mo.held[key], mo.vel[key] = at, true, e.Data2
			mo.padAt = at
		} else {
			delete(mo.held, key)
			mo.offs[key] = at
		}
	}
	if e.Kind == midiin.CC {
		k := [2]int{e.Channel, e.Data1}
		if mo.ccs[k] == nil {
			mo.seen++
			mo.ccs[k] = &ccState{ch: e.Channel, cc: e.Data1, first: mo.seen}
		}
		mo.ccs[k].val, mo.ccs[k].at = e.Data2, at
	}
	mo.lines = append(mo.lines, monLine{at: at, text: describeMIDI(e)})
	if len(mo.lines) > monLines {
		mo.lines = mo.lines[len(mo.lines)-monLines:]
	}
}

// tick takes in the clock tick that made the count ticks; on a beat the tempo is worked out.
func (mo *monitor) tick(at time.Time, ticks int) {
	if (ticks-1)%24 != 0 {
		return
	}
	if !mo.beatAt.IsZero() {
		if dt := at.Sub(mo.beatAt).Seconds(); dt > 0.1 && dt < 3 {
			bpm := 60 / dt
			if mo.bpm == 0 {
				mo.bpm = bpm
			} else {
				mo.bpm = mo.bpm*0.5 + bpm*0.5
			}
		}
	}
	mo.beatAt = at
}

// monFade is how long the glow of a pad takes to die away once it is let go.
const monFade = 700 * time.Millisecond

// glow is how brightly a pad shows, 0 to 1: a hit lights it by how hard it was hit, it stays so while
// held and fades after.
func (mo *monitor) glow(bank, pad int, now time.Time) float64 {
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
func (mo *monitor) hot(bank, pad int, now time.Time) bool {
	return mo.glow(bank, pad, now) > 0
}

// moving lists the controllers that moved lately, the newest first.
func (mo *monitor) moving(now time.Time) []*ccState {
	if mo == nil {
		return nil
	}
	var out []*ccState
	for _, c := range mo.ccs {
		if now.Sub(c.at) < monMoving {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].at.Equal(out[j].at) {
			return out[i].at.After(out[j].at)
		}
		return out[i].cc < out[j].cc
	})
	return out
}

// last is the newest n messages as text, the newest last.
func (mo *monitor) last(n int) []monLine {
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
func (mo *monitor) summary() string {
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
	if mo.bpm > 0 {
		text += fmt.Sprintf(" · clock ≈ %.1f BPM", mo.bpm)
	}
	return text
}

// logMIDI writes a message to the event log of the recording that runs.
func (m Model) logMIDI(e midiin.Event) {
	if m.perfLog == nil {
		return
	}
	at := e.At
	if at.IsZero() {
		at = time.Now()
	}
	switch e.Kind {
	case midiin.Clock:
		if m.clock != nil && (m.clock.ticks-1)%24 == 0 {
			m.perfLog.Add(at, "beat", perflog.P("n", (m.clock.ticks-1)/24), perflog.P("bpm", m.mon.bpm))
		}
	case midiin.Start, midiin.Stop, midiin.Continue:
		m.perfLog.Add(at, e.Kind.String())
	case midiin.NoteOn, midiin.NoteOff:
		f := []perflog.Pair{perflog.P("ch", e.Channel), perflog.P("n", e.Data1), perflog.P("v", e.Data2)}
		if bank, pad, ok := padOfEvent(e); ok {
			f = append(f, perflog.P("bank", string(rune('A'+bank))), perflog.P("pad", pad+1))
		}
		kind := "on"
		if e.Kind == midiin.NoteOff {
			kind = "off"
		}
		m.perfLog.Add(at, kind, f...)
	case midiin.CC:
		m.perfLog.Add(at, "cc", perflog.P("ch", e.Channel), perflog.P("n", e.Data1), perflog.P("v", e.Data2))
	case midiin.Sysex:
		m.perfLog.Add(at, "sysex", perflog.P("hex", fmt.Sprintf("% X", e.Raw[:min(len(e.Raw), 64)])), perflog.P("len", len(e.Raw)))
	default:
		m.perfLog.Add(at, e.Kind.String(), perflog.P("ch", e.Channel), perflog.P("n", e.Data1), perflog.P("v", e.Data2))
	}
}

// startPerfLog opens the event log next to a recording that began.
func (m Model) startPerfLog(audio string, started time.Time) (*perflog.Log, string) {
	l, err := perflog.Create(perflog.PathFor(audio), audio, started)
	if err != nil {
		return nil, "no event log: " + err.Error()
	}
	return l, ""
}

// monitorLines is the Config block of what the MIDI input said: counts, then the last messages.
func (m Model) monitorLines() []string {
	st := m.st
	head := st.bold.Render("MIDI monitor") + st.dim.Render("   c  clears")
	if m.midi == nil {
		return []string{head, st.dim.Render("nothing while no input is open")}
	}
	lines := []string{head}
	if sum := m.mon.summary(); sum != "" {
		lines = append(lines, st.text.Render(sum))
	} else {
		lines = append(lines, st.dim.Render("nothing has come yet: hit a pad or turn a knob on the unit"))
	}
	for _, l := range m.mon.last(6) {
		lines = append(lines, st.dim.Render(l.at.Format("15:04:05.000")+"  ")+st.text.Render(l.text))
	}
	return lines
}

// logCue writes the cue (the pattern that plays and the next) to the event log.
func (m Model) logCue() {
	m.perfLog.Add(time.Now(), "pattern", perflog.P("now", slotText(m.perfNow)), perflog.P("next", slotText(m.perfNext)))
}

// controls lists every controller seen, in the order it was first seen.
func (mo *monitor) controls() []*ccState {
	if mo == nil {
		return nil
	}
	out := make([]*ccState, 0, len(mo.ccs))
	for _, c := range mo.ccs {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].first < out[j].first })
	return out
}

// unitMuted says whether a track of the pattern that plays looks muted on the unit: the unit does not
// say, but a pad that has notes in the pattern and has not been heard on MIDI for a whole round of
// the pattern, while other pads have been, is silent. It needs the clock running for a round first,
// and says false when nothing of the unit's playing comes over MIDI at all.
func (m Model) unitMuted(d *app.PatternDetail, pad proto.PadIndex, now time.Time) bool {
	c, mo := m.clock, m.mon
	if mo == nil || c == nil || !c.running || time.Since(c.at) > 2*time.Second || d == nil || d.Length == 0 {
		return false
	}
	bpm := mo.bpm
	if bpm <= 0 {
		bpm = m.bankTempo(int(pad.Bank()))
	}
	if bpm <= 0 {
		return false
	}
	round := time.Duration(float64(d.Length) / formats.PPQ * 60 / bpm * float64(time.Second))
	if round < time.Second {
		return false
	}
	// A round has gone by since the loop began.
	if float64(c.ticks-m.perfSync)/24*60/bpm < round.Seconds() {
		return false
	}
	// Pads are being heard.
	if now.Sub(mo.padAt) > round {
		return false
	}
	has := false
	for _, r := range d.Grid.Rows {
		if r.Pad == pad && slices.Contains(r.Hits, true) {
			has = true
		}
	}
	if !has {
		return false
	}
	key := int(pad.Bank())*16 + int(pad.Pad())
	last, ok := mo.hits[key]
	return !ok || now.Sub(last) > round+time.Second
}

// padMuted says whether a pad is muted: in the mix on this computer, or silent on the unit in the
// pattern that plays (see unitMuted).
func (m Model) padMuted(pad proto.PadIndex) bool {
	if m.muted[pad] {
		return true
	}
	if m.perfNow == nil {
		return false
	}
	d, _ := m.perfTracks(m.perfNow)
	return m.unitMuted(d, pad, time.Now())
}

// heard notes that a pad of a pattern played, on the clock tick it did, and forgets the pads that
// were last heard more than eight bars ago: they are from a pattern that has been left.
func (mo *monitor) heard(key, tick int) {
	if mo.played == nil {
		mo.played = map[int]bool{}
	}
	if mo.playedAt == nil {
		mo.playedAt = map[int]int{}
	}
	mo.played[key], mo.playedAt[key] = true, tick
	for k, at := range mo.playedAt {
		if at > tick || tick-at > 8*ticksPerBar {
			delete(mo.played, k)
			delete(mo.playedAt, k)
		}
	}
}

// heardSince says whether a pad was heard within the last ticks of the clock.
func (mo *monitor) heardSince(key, tick, ticks int) bool {
	at, ok := mo.playedAt[key]
	return ok && at <= tick && tick-at <= ticks
}

// heardWithin is the pads heard within the last ticks of the clock.
func (mo *monitor) heardWithin(tick, ticks int) map[int]bool {
	out := map[int]bool{}
	for k, at := range mo.playedAt {
		if at <= tick && tick-at <= ticks {
			out[k] = true
		}
	}
	return out
}
