package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/midiin"
)

// The unit sends MIDI over USB: a note for each pad hit, its clock and transport. The same input
// takes messages that control the app: on the control channel a note (or a controller) switches the
// live mode, the recording, and the cue of the Perform view.

// WithMIDI gives the model a MIDI input to listen to; without one nothing is listened to.
func (m Model) WithMIDI(s midiin.Source) Model {
	m.midiSrc = s
	return m
}

// midiLink is an input that is open: its name, how to stop it, and where its messages come out.
type midiLink struct {
	name string
	stop func()
	ch   chan midiin.Event
}

// Defaults.
const (
	defaultMidiPort = "404"
	defaultMidiChan = 16
)

var defaultMidiMap = [4]string{"note 60", "note 61", "note 62", "note 63"}

// midiActions names what the four messages do, in the order of State.MidiMap.
var midiActions = [4]string{"live mode", "recording", "next becomes now", "clear next"}

func (m Model) midiPort() string {
	if m.mem.MidiPort != "" {
		return m.mem.MidiPort
	}
	return defaultMidiPort
}

// midiChan is the control channel, 0 for none.
func (m Model) midiChan() int {
	switch s := strings.ToLower(strings.TrimSpace(m.mem.MidiChan)); s {
	case "":
		return defaultMidiChan
	case "off":
		return 0
	default:
		if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= 16 {
			return n
		}
	}
	return defaultMidiChan
}

func (m Model) midiMapText(i int) string {
	if m.mem.MidiMap[i] != "" {
		return m.mem.MidiMap[i]
	}
	return defaultMidiMap[i]
}

// parseMidiMap reads "note 60", "cc 20" or just "60" (a note); false for "off" or what is not one.
func parseMidiMap(s string) (kind midiin.Kind, n int, ok bool) {
	f := strings.Fields(strings.ToLower(s))
	if len(f) == 1 {
		f = []string{"note", f[0]}
	}
	if len(f) != 2 {
		return 0, 0, false
	}
	n, err := strconv.Atoi(f[1])
	if err != nil || n < 0 || n > 127 {
		return 0, 0, false
	}
	switch f[0] {
	case "note":
		return midiin.NoteOn, n, true
	case "cc":
		return midiin.CC, n, true
	}
	return 0, 0, false
}

// startMIDI opens the input, when there is a source to open and none is open.
func (m Model) startMIDI() tea.Cmd {
	src, part := m.midiSrc, m.midiPort()
	if src == nil || m.midi != nil {
		return nil
	}
	return func() tea.Msg {
		link := &midiLink{ch: make(chan midiin.Event, 512)}
		stop, name, err := src.Open(part, func(e midiin.Event) {
			e.At = time.Now() // when it arrived, not when the program gets to it
			select {
			case link.ch <- e:
			default: // nobody is reading: a clock is not worth waiting for
			}
		})
		if err != nil {
			return midiOpenedMsg{err: err}
		}
		link.name, link.stop = name, stop
		return midiOpenedMsg{link: link}
	}
}

// waitMIDI hands over the next message of an input.
func waitMIDI(l *midiLink) tea.Cmd {
	return func() tea.Msg { return midiMsg{link: l, ev: <-l.ch} }
}

func (m Model) midiOpened(msg midiOpenedMsg) (tea.Model, tea.Cmd) {
	m.midiOpening = false
	if msg.err != nil {
		if msg.err.Error() != m.midiErr {
			m.log.Debug("midi: not open", "err", msg.err)
		}
		m.midiErr = msg.err.Error()
		return m, nil
	}
	m.log.Info("midi: listening", "input", msg.link.name)
	m.midi, m.midiErr = msg.link, ""
	return m, waitMIDI(msg.link)
}

// restartMIDI closes the input and opens it again, after the port was changed.
func (m Model) restartMIDI() (Model, tea.Cmd) {
	if m.midi != nil && m.midi.stop != nil {
		m.midi.stop()
	}
	m.midi, m.midiOpening = nil, true
	return m, m.startMIDI()
}

// clockState counts the MIDI clock: 24 ticks make a quarter note.
type clockState struct {
	ticks   int
	running bool
	at      time.Time
}

// clockText says where the clock is, "BAR 5 · BEAT 2", and the loop of the pattern that plays when
// its length is known; empty when no clock has come lately.
func (m Model) clockText(barsOfPattern int) string {
	c := m.clock
	if c == nil || time.Since(c.at) > 2*time.Second {
		return ""
	}
	beats := c.ticks / 24
	text := fmt.Sprintf("BAR %d · BEAT %d", beats/4+1, beats%4+1)
	if barsOfPattern > 0 {
		text += fmt.Sprintf("  ·  LOOP %d", beats/4/barsOfPattern+1)
	}
	if !c.running {
		text += "  (stopped)"
	}
	return text
}

// padOfNote is the pad (1 to 16) a note the unit sends stands for: pad 13 is note 36 and pad 1 is
// 48, with the pads of a row in order.
func padOfNote(note int) (int, bool) {
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

// midiEvent acts on a message: the clock is counted, a pad of the unit is a hit for the pad codes,
// and a message on the control channel does what it is set to.
func (m Model) midiEvent(msg midiMsg) (tea.Model, tea.Cmd) {
	if msg.link != m.midi {
		return m, nil // an input that was closed
	}
	next := waitMIDI(msg.link)
	e := msg.ev
	if m.vc != nil {
		m.vc.keep = false
	}
	if m.clock == nil {
		m.clock = &clockState{}
	}
	if m.mon == nil {
		m.mon = newMonitor()
	}
	m.mon.add(e)
	if e.Kind != midiin.Clock {
		m.log.Debug("midi: message", "msg", describeMIDI(e), "raw", e.String())
	} else if (m.clock.ticks)%(24*16) == 0 {
		m.log.Debug("midi: clock", "ticks", m.clock.ticks, "bpm", m.mon.bpm)
	}
	switch e.Kind {
	case midiin.Clock:
		m.clock.ticks++
		m.clock.at = time.Now()
		m.clock.running = true
		m.mon.tick(e.At, m.clock.ticks)
		m.logMIDI(e)
		m = m.sceneTick()
		if m.vc != nil && m.clock.ticks%6 != 0 {
			m.vc.keep = true
		}
		return m, next
	case midiin.Start:
		m.clock.ticks, m.clock.running, m.clock.at = 0, true, time.Now()
		m.perfSync = 0
		m.mon.beatAt = time.Time{}
		m.mon.played, m.mon.playedAt = map[int]bool{}, map[int]int{}
		m.logMIDI(e)
		return m, next
	case midiin.Continue:
		m.clock.running, m.clock.at = true, time.Now()
		m.logMIDI(e)
		return m, next
	case midiin.Stop:
		m.clock.running = false
		m.logMIDI(e)
		return m, next
	}
	m.logMIDI(e)
	m = m.fxEvent(e)
	ctl := m.midiChan()
	// A message on the control channel: a note on, or a controller turned up.
	if ctl != 0 && e.Channel == ctl {
		for i := range midiActions {
			kind, n, ok := parseMidiMap(m.midiMapText(i))
			if !ok || kind != e.Kind || e.Data1 != n {
				continue
			}
			if e.Kind == midiin.CC && e.Data2 < 64 {
				continue
			}
			nm, cmd := m.midiAction(i)
			return nm, tea.Batch(cmd, next)
		}
		return m, next
	}
	// A pad hit on the unit, in a bank's channel: exactly the pad, even when it is hit twice.
	if e.Kind == midiin.NoteOn && e.Channel >= 1 && e.Channel <= 10 {
		if pad, ok := padOfNote(e.Data1); ok {
			m.midiPadAt = time.Now()
			if m.clock.running && time.Since(m.clock.at) < time.Second {
				// The sequencer runs: these notes are what the pattern plays (the unit sends them
				// over MIDI), not pads hit by hand. They find the pattern, not the pad codes.
				m.mon.heard((e.Channel-1)*16+pad-1, m.clock.ticks)
				nm := m.detectPattern()
				if nm.perfNow != m.perfNow {
					// A new pattern: its notes are read for the Perform view.
					return nm, tea.Batch(nm.wanted(), next)
				}
				return nm, next
			}
			nm, cmd := m.codeHit(pad)
			return nm, tea.Batch(cmd, next)
		}
	}
	return m, next
}

// midiAction does what one of the four messages is for.
func (m Model) midiAction(i int) (tea.Model, tea.Cmd) {
	switch i {
	case 0:
		nm, cmd := m.toggleLive()
		return nm, cmd
	case 1:
		return m.toggleRecording()
	case 2:
		return m.perfKey(" ")
	}
	return m.perfKey("x")
}

// midiFieldText is a part of the MIDI setting as it is typed: 0 the input, 1 the channel, 2 to 5
// the messages.
func (m Model) midiFieldText(i int) string {
	switch {
	case i == 0:
		return m.midiPort()
	case i == 1:
		if m.midiChan() == 0 {
			return "off"
		}
		return strconv.Itoa(m.midiChan())
	}
	return m.midiMapText(i - 2)
}

func midiPromptTitle(i int) string {
	switch i {
	case 0:
		return "MIDI input"
	case 1:
		return "MIDI control channel"
	}
	return "MIDI message for " + midiActions[i-2]
}

func midiPromptHint(i int) string {
	switch i {
	case 0:
		return "a part of the name of the input; the unit's is SP-404MKII, so 404  ·  "
	case 1:
		return "1 to 16, or off  ·  "
	}
	return "note 60 or cc 20 (a controller turned up past half), off for none  ·  "
}

// applyMidi keeps what was typed for a part of the MIDI setting.
func (m Model) applyMidi(field int, text string) (tea.Model, tea.Cmd) {
	text = strings.ToLower(text)
	switch {
	case field == 0:
		m.mem.MidiPort = text
		nm, cmd := m.restartMIDI()
		nm.message = fmt.Sprintf("listening for the MIDI input with %q in its name", nm.midiPort())
		return nm, cmd
	case field == 1:
		if n, err := strconv.Atoi(text); text != "off" && (err != nil || n < 1 || n > 16) {
			m.message = "a MIDI channel is 1 to 16 (or off)"
			return m, nil
		}
		m.mem.MidiChan = text
		m.message = "control channel " + m.midiFieldText(1)
		return m, nil
	}
	if text != "off" {
		if _, _, ok := parseMidiMap(text); !ok {
			m.message = "a message is note 60 or cc 20 (0 to 127), or off"
			return m, nil
		}
	}
	m.mem.MidiMap[field-2] = text
	m.message = fmt.Sprintf("%s: %s on channel %s", midiActions[field-2], m.midiMapText(field-2), m.midiFieldText(1))
	return m, nil
}

// midiLine says whether MIDI is being listened to, for the Config view.
func (m Model) midiLine() string {
	switch {
	case m.midiSrc == nil:
		return "No MIDI in this program."
	case m.midi != nil:
		return "Listening to " + m.midi.name + ". The unit's pads (channels 1 to 10) count for the pad codes; its clock counts bars in Perform."
	case m.midiErr != "":
		return "Not listening: " + m.midiErr
	}
	return "Looking for the input…"
}
