package core

import (
	"encoding/json"
	"time"

	"github.com/moahbeatlab/404tui/dbg"
	"github.com/moahbeatlab/404tui/lab"
	"github.com/moahbeatlab/404tui/midiin"
	"github.com/moahbeatlab/404tui/perflog"
	"github.com/moahbeatlab/404tui/perform"
	"github.com/moahbeatlab/404tui/record"
)

// The event log of a recording. While the unit's audio is recorded, what the unit does is written beside the WAV as name.events.jsonl, one
// JSON object a line with its time in seconds from the start of the recording: the pads that are hit and let go, the controllers (the
// knobs of the buses, their effect and switch), the clock with the tempo, start and stop, and the pattern that plays when it is known. The
// second line, "project", says what the project is: the pads (names, bus, a small picture of the sound) and the patterns with their notes,
// so that a replay can draw them without the project. "audio_start" says when the first frame of the WAV was heard, in seconds after
// the start of the log (the time of the events and of the audio is the same clock; this is the little that is between).
//
// A replay (sp404 replay) shows it with the audio: the pads lighting, the pattern with its playhead, the knobs moving, the level.

type eventLog struct {
	log        *perflog.Log
	rec        *record.Session
	audioDone  bool
	last       [2]int // the pattern that plays and the next one, as last written
	beatTicks  int
	startedAt  time.Time
	eventCount int
}

// startEventLog opens the log of a recording that began.
func (a *App) startEventLog(s *record.Session) {
	l, err := perflog.Create(perflog.PathFor(s.Path), s.Path, s.Started)
	if err != nil {
		a.say("no event log: "+err.Error(), true)
		return
	}
	a.evlog = &eventLog{log: l, rec: s, startedAt: s.Started}
	l.Add(s.Started, "project", perflog.P("data", a.projectSnapshot()))
	dbg.Logger.Info("events: log started", "file", l.Path)
}

// stopEventLog finishes the log.
func (a *App) stopEventLog() {
	e := a.evlog
	if e == nil {
		return
	}
	a.evlog = nil
	e.log.Add(time.Now(), "end", perflog.P("events", e.eventCount))
	e.log.Close()
	dbg.Logger.Info("events: log finished", "file", e.log.Path, "events", e.eventCount)
}

// eventLogTick writes what is not a MIDI message: when the audio began, and the pattern that plays when it changes.
func (a *App) eventLogTick() {
	e := a.evlog
	if e == nil {
		return
	}
	if !e.audioDone {
		if at, ok := e.rec.FirstFrame(); ok {
			e.audioDone = true
			e.log.Add(at, "audio_start", perflog.P("rate", 48000))
		} else if e.rec.Tool != "the program itself" { // a recorder program: its first frame is not known, the start is taken
			e.audioDone = true
		}
	}
	if now := [2]int{a.perfNow, a.perfNext}; now != e.last {
		e.last = now
		e.log.Add(time.Now(), "pattern", perflog.P("now", a.slotLabel(now[0])), perflog.P("next", a.slotLabel(now[1])))
	}
}

func (a *App) slotLabel(slot int) string {
	if slot < 1 || slot > 160 {
		return ""
	}
	return a.label(slot - 1)
}

// logMIDI writes a message of the unit to the event log of the recording that runs.
func (a *App) logMIDI(e midiin.Event) {
	l := a.evlog
	if l == nil {
		return
	}
	at := e.At
	if at.IsZero() {
		at = time.Now()
	}
	switch e.Kind {
	case midiin.Clock:
		if a.clk.Running && (a.clk.Ticks-1)%24 == 0 {
			l.log.Add(at, "beat", perflog.P("n", (a.clk.Ticks-1)/24), perflog.P("bpm", a.mon.BPM))
			l.eventCount++
		}
	case midiin.Start, midiin.Stop, midiin.Continue:
		l.log.Add(at, e.Kind.String())
		l.eventCount++
	case midiin.NoteOn, midiin.NoteOff:
		f := []perflog.Pair{perflog.P("ch", e.Channel), perflog.P("n", e.Data1), perflog.P("v", e.Data2)}
		if pad, ok := perform.PadOfNote(e.Data1); ok && e.Channel >= 1 && e.Channel <= 10 {
			f = append(f, perflog.P("bank", string(rune('A'+e.Channel-1))), perflog.P("pad", pad))
		}
		kind := "on"
		if e.Kind == midiin.NoteOff || e.Kind == midiin.NoteOn && e.Data2 == 0 {
			kind = "off"
		}
		l.log.Add(at, kind, f...)
		l.eventCount++
	case midiin.CC:
		l.log.Add(at, "cc", perflog.P("ch", e.Channel), perflog.P("n", e.Data1), perflog.P("v", e.Data2))
		l.eventCount++
		switch e.Data1 { // the effect of a bus (83), its switch (19): written as what they mean too
		case 83:
			l.log.Add(at, "fx", perflog.P("bus", e.Channel), perflog.P("fx", e.Data2))
		case 19:
			l.log.Add(at, "fx", perflog.P("bus", e.Channel), perflog.P("on", e.Data2 >= 64))
		}
	default:
		l.log.Add(at, e.Kind.String(), perflog.P("ch", e.Channel), perflog.P("n", e.Data1), perflog.P("v", e.Data2))
		l.eventCount++
	}
}

// ---- the project, as the second line of the log

type snapPad struct {
	N      int    `json:"n"`
	Label  string `json:"label"`
	Name   string `json:"name"`
	Bus    int    `json:"bus,omitempty"`
	Frames int    `json:"frames"`
	Peaks  []int  `json:"peaks,omitempty"`
}

type snapPattern struct {
	Slot   int      `json:"slot"`
	Label  string   `json:"label"`
	Bars   int      `json:"bars"`
	Beats  int      `json:"beats"`
	Length uint32   `json:"length"`
	Notes  [][5]int `json:"notes"` // tick, pad (0..159), velocity, length, pitch byte
}

// projectSnapshot is what a replay needs to know of the project.
func (a *App) projectSnapshot() map[string]any {
	out := map[string]any{"name": a.ProjectName(), "tempo": a.info.Tempo, "bank_tempos": a.info.BankTempos, "project_tempo": a.info.UsesProjectTempo}
	var pads []snapPad
	for n, p := range a.info.Pads {
		if !p.File {
			continue
		}
		sp := snapPad{N: n, Label: a.label(n), Name: p.Name, Frames: p.Frames}
		if v := a.padValue(n, 0x7A, 0); v == 1 || v == 2 {
			sp.Bus = int(v)
		}
		if !a.lab.IsRemote(n) { // a sample that is on the unit is not fetched for this
			if id, ok := a.padSample(n); ok && p.Frames > 0 {
				if pk, err := a.lab.Peaks(id, 0, p.Frames, 32); err == nil {
					top := 1
					for _, v := range pk {
						top = max(top, v)
					}
					for _, v := range pk {
						sp.Peaks = append(sp.Peaks, v*100/top)
					}
				}
			}
		}
		pads = append(pads, sp)
	}
	out["pads"] = pads
	var pats []snapPattern
	for s := 1; s <= 160; s++ {
		b, err := a.lab.Pattern(s)
		if err != nil {
			continue
		}
		var d lab.PatternDoc
		if json.Unmarshal(b, &d) != nil || len(d.Notes) == 0 {
			continue
		}
		sp := snapPattern{Slot: s, Label: a.label(s - 1), Bars: d.Bars, Beats: d.Beats, Length: d.Length}
		for _, n := range d.Notes {
			sp.Notes = append(sp.Notes, [5]int{int(n.Tick), n.Pad, n.Vel, n.Len, n.Pitch})
		}
		pats = append(pats, sp)
	}
	out["patterns"] = pats
	return out
}
