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

// markerFresh says the unit has sent its loop marker lately: not every pattern or setting makes it send one, and then nothing waits for it.
func (m Model) markerFresh() bool {
	return m.sc != nil && !m.sc.lastMarker.IsZero() && time.Since(m.sc.lastMarker) < 15*time.Second
}

// patternCue asks for a pattern: at once, or at the next loop start (at once too when the unit sends no loop marker to wait for).
func (m Model) patternCue(slot proto.PadIndex, now bool) Model {
	sc := m.scenes()
	m.sc = sc
	if now || !m.markerFresh() {
		sc.pcNext = nil
		return m.patternSend(slot)
	}
	sc.pcNext, sc.pcAt = &slot, time.Now()
	m.perfNext = &slot
	m.log.Info("pattern: queued for the loop start", "pattern", slot.String())
	m.perfLog.Add(time.Now(), "pattern", perflog.P("now", slotText(m.perfNow)), perflog.P("next", slotText(m.perfNext)))
	return m
}

// patternRequested notes a pattern the Patterns page sent to the unit (which plays it from the end of its loop): it is logged with its
// time, and taken to be the one that plays from the next loop start (or from a few seconds on when the unit sends no marker).
func (m Model) patternRequested(slot proto.PadIndex) Model {
	sc := m.scenes()
	m.sc = sc
	sc.pcConfirm, sc.pcAt = &slot, time.Now()
	m.perfNext = &slot
	m.perfLog.Add(time.Now(), "pattern", perflog.P("now", slotText(m.perfNow)), perflog.P("next", slotText(m.perfNext)), perflog.P("sent", true))
	return m
}

// patternLoopStart is the loop starting: a pattern that waits is sent, and one that was sent now plays.
func (m Model) patternLoopStart() Model {
	sc := m.scenes()
	m.sc = sc
	sc.lastMarker = time.Now()
	if sc.pcNext != nil {
		slot := *sc.pcNext
		sc.pcNext = nil
		return m.patternSend(slot)
	}
	return m.patternTakeOver()
}

// patternTakeOver makes the pattern that was sent the one that plays.
func (m Model) patternTakeOver() Model {
	if m.sc == nil || m.sc.pcConfirm == nil {
		return m
	}
	slot := *m.sc.pcConfirm
	m.sc.pcConfirm = nil
	m.perfNow, m.perfNext, m.perfAuto = &slot, nil, false
	m.perfLog.Add(time.Now(), "pattern", perflog.P("now", slotText(m.perfNow)), perflog.P("next", slotText(m.perfNext)))
	return m
}

// patternWaitTick does not wait for ever for a loop marker that does not come.
func (m Model) patternWaitTick() Model {
	if m.sc == nil || time.Since(m.sc.pcAt) < 8*time.Second {
		return m
	}
	if m.sc.pcNext != nil {
		slot := *m.sc.pcNext
		m.sc.pcNext = nil
		return m.patternSend(slot)
	}
	return m.patternTakeOver()
}

func (m Model) patternSend(slot proto.PadIndex) Model {
	sc := m.scenes()
	m.sc = sc
	m.sceneSend(sc, []byte{0xC0 | byte(slot.Bank()), byte(slot.Pad())})
	sc.pcConfirm = nil
	m.perfNow, m.perfNext, m.perfAuto = &slot, nil, false
	m.perfLog.Add(time.Now(), "pattern", perflog.P("now", slotText(m.perfNow)), perflog.P("next", slotText(m.perfNext)), perflog.P("sent", true))
	return m
}
