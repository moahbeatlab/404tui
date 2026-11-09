package core

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/moahbeatlab/404tui/app"
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
	stopPoll   chan struct{}
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
	a.CastTo(l)
	l.Add(s.Started, "project", perflog.P("data", a.projectSnapshot(false)), perflog.P("source", "editor"))
	a.snapshotUnitProject()
	a.logUnitStatus()
	dbg.Logger.Info("events: log started", "file", l.Path)
}

// stopEventLog finishes the log.
func (a *App) stopEventLog() {
	e := a.evlog
	if e == nil {
		return
	}
	a.evlog = nil
	a.CastTo(nil)
	if e.stopPoll != nil {
		close(e.stopPoll)
	}
	e.log.Add(time.Now(), "end", perflog.P("events", e.eventCount))
	e.log.Close()
	dbg.Logger.Info("events: log finished", "file", e.log.Path, "events", e.eventCount)
}

// eventLogTick writes what is not a MIDI message: when the audio began, and the pattern that plays when it changes.
func (a *App) eventLogTick() {
	a.applyUnitSnapshot()
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
	// Info is the pad's settings as the panel names them (Bus, Mute grp, Mode, Gate, Loop, One shot, Level, Pan, Pitch, Attack ...).
	Info map[string]string `json:"info,omitempty"`
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
func (a *App) projectSnapshot(fromUnit bool) map[string]any {
	out := map[string]any{"name": a.ProjectName(), "tempo": a.info.Tempo, "bank_tempos": a.info.BankTempos, "project_tempo": a.info.UsesProjectTempo}
	var pads []snapPad
	for n, p := range a.info.Pads {
		if !p.File && !(fromUnit && p.Has) { // a project read from the unit has no sample files here: a pad with a sample is told by its block
			continue
		}
		sp := snapPad{N: n, Label: a.label(n), Name: p.Name, Frames: p.Frames}
		if v := a.padValue(n, 0x7A, 0); v == 1 || v == 2 {
			sp.Bus = int(v)
		}
		sp.Info = map[string]string{}
		for _, grp := range a.padInfoGroups(n) {
			for _, l := range grp {
				if !l.head && l.label != "" && l.value != "" {
					sp.Info[l.label] = l.value
				}
			}
		}
		if !fromUnit && !a.lab.IsRemote(n) { // a sample that is on the unit is not fetched for this
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

// unitSnap is the unit's open project, read in the background.
type unitSnap struct {
	dir     string
	project int
	err     error
}

// snapshotUnitProject reads the project the unit has open (a few small files, no samples) and writes it to the log as the project, in
// place of the editor's, when the two are not the same: what is played on the unit is not always what is in the editor.
func (a *App) snapshotUnitProject() {
	dev, ok := a.unitDevice()
	if !ok || a.evlog == nil {
		return
	}
	cur, err := dev.CurrentProject()
	if err != nil || (a.live != nil && a.live.project == int(cur)) {
		return
	}
	if a.evSnapCh == nil {
		a.evSnapCh = make(chan unitSnap, 1)
	}
	dir := filepath.Join(a.dataDir(), "eventlog-tmp")
	os.RemoveAll(dir)
	ch := a.evSnapCh
	go func() {
		st := time.Now()
		_, err := app.ExportProjectLight(app.NewRouter(dev), cur, dir, nil)
		dbg.Logger.Info("events: the unit's project was read for the log", "project", int(cur)+1, "ms", time.Since(st).Milliseconds(), "err", err)
		ch <- unitSnap{dir: dir, project: int(cur), err: err}
	}()
}

// applyUnitSnapshot writes the unit's project to the log (on the main loop: the lab and the info are swapped for a moment).
func (a *App) applyUnitSnapshot() {
	select {
	case u := <-a.evSnapCh:
		defer os.RemoveAll(u.dir)
		e := a.evlog
		if e == nil || u.err != nil {
			return
		}
		tmp := lab.New()
		tmp.Begin()
		filepath.Walk(u.dir, func(p string, fi os.FileInfo, err error) error {
			if err != nil || fi.IsDir() {
				return nil
			}
			if data, err := os.ReadFile(p); err == nil {
				rel, _ := filepath.Rel(u.dir, p)
				tmp.Add(rel, data)
			}
			return nil
		})
		if _, err := tmp.Finish(); err != nil {
			return
		}
		saveLab, saveInfo, savePcm := a.lab, a.info, a.pcm
		a.lab, a.pcm = tmp, map[int]int{}
		if b, err := tmp.Info(); err == nil {
			a.info = lab.Info{}
			json.Unmarshal(b, &a.info)
		}
		data := a.projectSnapshot(true)
		a.lab, a.info, a.pcm = saveLab, saveInfo, savePcm
		data["unit_project"] = u.project + 1
		e.log.Add(time.Now(), "project", perflog.P("data", data), perflog.P("source", "unit"))
	default:
	}
}

// logUnitStatus polls the unit's status four times a second while it records and writes what changes: the pad that is selected (a pad
// hit selects it), the pad that sounds, and the raw bytes of the status ("status", with the bytes that count the frames of a sounding
// pad left out), which may hold more than is understood of it, such as the pattern that plays.
func (a *App) logUnitStatus() {
	dev, ok := a.unitDevice()
	e := a.evlog
	if !ok || e == nil {
		return
	}
	e.stopPoll = make(chan struct{})
	stop, l := e.stopPoll, e.log
	go func() {
		t := time.NewTicker(250 * time.Millisecond)
		defer t.Stop()
		var lastHex string
		var lastSel, lastPlay = -1, false
		for {
			select {
			case <-stop:
				return
			case <-t.C:
			}
			raw, err := dev.RawStatus()
			if err != nil || len(raw) < 6 {
				continue
			}
			at := time.Now()
			masked := append([]byte(nil), raw...)
			for i := 8; i < 12 && i < len(masked); i++ {
				masked[i] = 0 // the frames played of a sounding pad change all the time
			}
			if hex := fmt.Sprintf("% x", masked); hex != lastHex {
				lastHex = hex
				l.Add(at, "status", perflog.P("hex", hex))
			}
			sel := int(raw[3])*16 + int(raw[5])
			if sel != lastSel {
				lastSel = sel
				l.Add(at, "select", perflog.P("bank", string(rune('A'+raw[3]))), perflog.P("pad", int(raw[5])+1))
			}
			playing := len(raw) >= 12 && binary.LittleEndian.Uint32(raw[8:12]) != 0xFFFFFFFF
			if playing != lastPlay {
				lastPlay = playing
				l.Add(at, "sound", perflog.P("on", playing), perflog.P("bank", string(rune('A'+raw[3]))), perflog.P("pad", int(raw[5])+1))
			}
		}
	}()
}
