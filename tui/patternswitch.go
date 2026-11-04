package tui

import (
	"time"

	"github.com/moahbeatlab/404tui/midiin"
	"github.com/moahbeatlab/404tui/perflog"
	"github.com/moahbeatlab/404tui/proto"
)

// The pattern the unit plays is changed from here by a Program Change (the channel is the bank, the number the pattern in it). The
// unit says nothing when its pattern changes, so what is sent is the one record of it: every change is written to the event log with
// its time, and a replay shows the pattern that was asked for.
//
// A pattern is normally the NEXT one: it waits for the unit's loop to end (the unit sends a note on channel 13 at every loop start), and
// then it is sent. NOW sends it at once.

// loopMarker is the note the unit sends at every loop of its pattern.
func loopMarker(e midiin.Event) bool {
	return e.Kind == midiin.NoteOn && e.Channel == 13 && e.Data1 == 48
}

// patternCue asks for a pattern: at once, or at the next loop start (the unit sends the marker when it starts too, so a pattern asked
// for while it stands still is sent when it is started).
func (m Model) patternCue(slot proto.PadIndex, now bool) Model {
	sc := m.scenes()
	m.sc = sc
	if now {
		sc.pcNext = nil
		return m.patternSend(slot)
	}
	sc.pcNext = &slot
	m.perfNext = &slot
	m.log.Info("pattern: queued for the loop start", "pattern", slot.String())
	m.perfLog.Add(time.Now(), "pattern", perflog.P("now", slotText(m.perfNow)), perflog.P("next", slotText(m.perfNext)))
	return m
}

// patternLoopStart sends the pattern that waits, now that the loop starts.
func (m Model) patternLoopStart() Model {
	if m.sc == nil || m.sc.pcNext == nil {
		return m
	}
	slot := *m.sc.pcNext
	m.sc.pcNext = nil
	return m.patternSend(slot)
}

func (m Model) patternSend(slot proto.PadIndex) Model {
	sc := m.scenes()
	m.sc = sc
	m.sceneSend(sc, []byte{0xC0 | byte(slot.Bank()), byte(slot.Pad())})
	m.perfNow, m.perfNext, m.perfAuto = &slot, nil, false
	m.perfLog.Add(time.Now(), "pattern", perflog.P("now", slotText(m.perfNow)), perflog.P("next", slotText(m.perfNext)), perflog.P("sent", true))
	return m
}
