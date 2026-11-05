package core

import (
	"errors"
	"strings"
	"time"

	"github.com/moahbeatlab/404tui/midiin"
	"github.com/moahbeatlab/404tui/perflog"
	"github.com/moahbeatlab/404tui/perform"
)

// MIDI: the unit's MIDI is listened to when the platform has it (a computer's build with MIDI; the handheld has none yet).
// The messages go to the monitor (what the PERF screen shows), the clock counts, pads hit in turn are the pad codes, and a
// message on the control channel does what CONFIG says (live mode, recording, the next pattern).

type midiLink struct {
	ch   chan midiin.Event
	stop func()
	name string
}

// padCode keeps the pads hit in the last moments.
type padCode struct{ hits []codeHit }

type codeHit struct {
	pad int
	at  time.Time
}

const codeWindow = 4 * time.Second

// feed notes a hit of a pad (1 to 16) and says which of the codes it completed, or -1. The hits of a completed code are
// forgotten, so that it has to be played again to count again.
func (c *padCode) feed(pad int, now time.Time, codes ...[]int) int {
	c.hits = append(c.hits, codeHit{pad: pad, at: now})
	keep := c.hits[:0]
	for _, h := range c.hits {
		if now.Sub(h.at) <= codeWindow {
			keep = append(keep, h)
		}
	}
	c.hits = keep
	for i, code := range codes {
		if len(code) == 0 || len(c.hits) < len(code) {
			continue
		}
		tail := c.hits[len(c.hits)-len(code):]
		match := true
		for k, p := range code {
			if tail[k].pad != p {
				match = false
				break
			}
		}
		if match {
			c.hits = nil
			return i
		}
	}
	return -1
}

// midiTick opens the input when there is none (the unit may be plugged in later), and takes in what has come.
func (a *App) midiTick() {
	if a.plat.MIDI == nil {
		return
	}
	if a.midi == nil && time.Since(a.midiTry) > 3*time.Second {
		a.midiTry = time.Now()
		link := &midiLink{ch: make(chan midiin.Event, 512)}
		stop, name, err := a.plat.MIDI.Open(a.midiPort(), func(e midiin.Event) {
			e.At = time.Now()
			select {
			case link.ch <- e:
			default: // nobody is reading: a clock is not worth waiting for
			}
		})
		if errors.Is(err, midiin.ErrUnavailable) { // this build has no MIDI: no use to try again
			a.plat.MIDI = nil
			return
		}
		if err != nil {
			a.midiErr = err.Error()
		} else {
			link.stop, link.name = stop, name
			a.midi, a.midiErr = link, ""
		}
	}
	a.ctlTick()
	if a.midi == nil {
		return
	}
	for i := 0; i < 256; i++ {
		select {
		case e := <-a.midi.ch:
			a.midiEvent(e)
		default:
			return
		}
	}
}

// The controller: a Faderfox EC4 (or any controller) has a MIDI input of its own, beside the unit's. Its pattern group (a channel of its own,
// 16 by default) says which pattern is played: the knob of E1 to E10 (a turn of its controller number 60 to 69) chooses the bank A to J, and a push
// (the note 60 to 75) is the pattern 1 to 16 of that bank. The controller sends it to the unit as well (a Program Change, by its own routing),
// so this is heard here to know the pattern and to write it into the event log; nothing is sent to the unit from here.

// ctlTick opens the controller's input when there is one (it may be plugged in later) and takes in what has come.
func (a *App) ctlTick() {
	if a.plat.MIDI == nil || a.ctlPort() == "" {
		return
	}
	if a.ctl == nil && time.Since(a.ctlTry) > 3*time.Second {
		a.ctlTry = time.Now()
		link := &midiLink{ch: make(chan midiin.Event, 128)}
		stop, name, err := a.plat.MIDI.Open(a.ctlPort(), func(e midiin.Event) {
			e.At = time.Now()
			select {
			case link.ch <- e:
			default:
			}
		})
		if err != nil {
			a.ctlErr = err.Error()
		} else {
			link.stop, link.name = stop, name
			a.ctl, a.ctlErr = link, ""
		}
	}
	if a.ctl == nil {
		return
	}
	for i := 0; i < 128; i++ {
		select {
		case e := <-a.ctl.ch:
			a.ctlEvent(e)
		default:
			return
		}
	}
}

// restartCtl closes the controller's input so that it is opened again with the setting as it is now.
func (a *App) restartCtl() {
	if a.ctl != nil && a.ctl.stop != nil {
		a.ctl.stop()
	}
	a.ctl, a.ctlTry = nil, time.Time{}
}

func (a *App) ctlStatus() string {
	switch {
	case a.plat.MIDI == nil:
		return "no MIDI in this build"
	case a.ctlPort() == "":
		return "off"
	case a.ctl != nil:
		return "listening to " + a.ctl.name
	case a.ctlErr != "":
		return "not listening: " + a.ctlErr
	}
	return "looking for the controller..."
}

// ctlEvent takes in one message of the controller.
func (a *App) ctlEvent(e midiin.Event) {
	pc := a.patChan()
	if pc == 0 || e.Channel != pc {
		return
	}
	switch {
	case e.Kind == midiin.CC && e.Data1 >= 60 && e.Data1 <= 69:
		if bank := e.Data1 - 60; bank != a.ctlBank {
			a.ctlBank = bank
			a.say("controller: bank "+string(rune('A'+bank)), false)
		}
	case e.Kind == midiin.NoteOn && e.Data2 > 0 && e.Data1 >= 60 && e.Data1 <= 75:
		a.ctlPattern(a.ctlBank*16+e.Data1-60+1, e.At)
	}
}

// ctlPattern notes that the controller played the pattern of this slot (1 to 160): it is the pattern that plays, and the event log has it.
func (a *App) ctlPattern(slot int, at time.Time) {
	a.perfNow, a.perfNext = slot, 0
	a.say("controller: pattern "+a.slotLabel(slot), false)
	if l := a.evlog; l != nil {
		if at.IsZero() {
			at = time.Now()
		}
		l.last = [2]int{slot, 0}
		l.log.Add(at, "pattern", perflog.P("now", a.slotLabel(slot)), perflog.P("next", ""), perflog.P("by", "controller"))
		l.eventCount++
	}
}

// restartMIDI closes the input so that it is opened again with the setting as it is now.
func (a *App) restartMIDI() {
	if a.midi != nil && a.midi.stop != nil {
		a.midi.stop()
	}
	a.midi, a.midiTry = nil, time.Time{}
}

func (a *App) midiStatus() string {
	switch {
	case a.plat.MIDI == nil:
		return "no MIDI in this build"
	case a.midi != nil:
		return "listening to " + a.midi.name
	case a.midiErr != "":
		return "not listening: " + a.midiErr
	}
	return "looking for the MIDI input..."
}

// midiEvent takes in one message.
func (a *App) midiEvent(e midiin.Event) {
	a.mon.Feed(&a.clk, e)
	a.logMIDI(e)
	switch e.Kind {
	case midiin.Clock, midiin.Start, midiin.Continue, midiin.Stop:
		if e.Kind == midiin.Start {
			a.mon.Reset()
		}
		return
	}
	ctl := a.midiChan()
	// A message on the control channel: a note on, or a controller turned up.
	if ctl != 0 && e.Channel == ctl {
		for i := range midiActions {
			kind, n, ok := parseMidiMap(a.midiMapText(i))
			if !ok || kind != e.Kind || e.Data1 != n {
				continue
			}
			if e.Kind == midiin.CC && e.Data2 < 64 {
				continue
			}
			a.midiAction(i)
			return
		}
		return
	}
	// A pad hit on the unit, in a bank's channel.
	if e.Kind == midiin.NoteOn && e.Channel >= 1 && e.Channel <= 10 {
		if pad, ok := perform.PadOfNote(e.Data1); ok {
			if a.clk.Running && time.Since(a.clk.At) < time.Second {
				// the sequencer runs: these notes are what the pattern plays, not pads hit by hand
				a.mon.Heard((e.Channel-1)*16+pad-1, a.clk.Ticks)
				return
			}
			a.codeHit(pad)
		}
	}
}

// midiAction does what a message of the control channel stands for: 0 live mode, 1 recording, 2 the next pattern becomes the one
// that plays, 3 clear the next.
func (a *App) midiAction(i int) {
	switch i {
	case 0:
		a.toggleLive()
	case 1:
		a.ToggleRecording()
	case 2:
		a.perfSwap()
	case 3:
		a.perfNext = 0
		a.say("no next pattern", false)
	}
}

// codeHit feeds a pad hit to the pad codes and does what the one it completes says.
func (a *App) codeHit(pad int) {
	switch a.codes.feed(pad, time.Now(), a.liveCode(), a.recCode()) {
	case 0:
		a.toggleLive()
	case 1:
		a.ToggleRecording()
	}
}

// toggleLive goes to the PERF screen, or back to where it came from.
func (a *App) toggleLive() {
	if Terminal { // the PERF screen is not in the program of a computer
		return
	}
	if a.scr == Perform && a.liveBack != Perform {
		a.go_(a.liveBack)
		a.say("live mode off", false)
		return
	}
	a.liveBack = a.scr
	if a.liveBack == Perform {
		a.liveBack = Pads
	}
	a.go_(Perform)
	a.say("live mode on: "+strings.TrimSpace(codeText(a.liveCode()))+" again leaves it", false)
}
