package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// The Patterns page of the web UI: the patterns of a bank, and a tap makes the unit play one next. The unit is told by a MIDI
// Program Change on the channel of the bank (1 for A to 10 for J) with the number of the pattern (0 to 15), as the unit's
// "PC Rx" setting wants it; the unit does not say it took it, so the page marks the last one sent.

// ptnBankNow is the bank the Patterns page shows: the one of the pattern that plays (found from the pads heard, or sent from
// here), which it follows when the pattern changes; the one chosen on the page stays until the pattern that plays changes; else
// the unit's bank.
func (m Model) ptnBankNow() int {
	if lp := m.lp; lp != nil {
		if m.perfNow != nil && (lp.ptSeen == nil || *lp.ptSeen != *m.perfNow) {
			now := *m.perfNow
			lp.ptSeen, lp.ptSet = &now, false
		}
		if lp.ptSet {
			return clamp(lp.ptBank, 0, proto.Banks-1)
		}
	}
	if m.perfNow != nil {
		return int(m.perfNow.Bank())
	}
	return m.perfBank()
}

// ptnCommand handles the buttons of the Patterns page.
func (m Model) ptnCommand(act string, i int) (tea.Model, tea.Cmd) {
	if m.snap == nil {
		return m, nil
	}
	lp := m.looper()
	bank := m.ptnBankNow()
	switch act {
	case "ptnbank", "ptnbankset":
		if act == "ptnbank" {
			i = bank + i
		}
		lp.ptSet, lp.ptBank = true, (i%proto.Banks+proto.Banks)%proto.Banks
		return m, m.wanted()
	case "ptn":
		rows := m.patterns[bank]
		if i < 0 || i > 15 || i >= len(rows) || !rows[i].Exists {
			return m, nil
		}
		if m.sc == nil {
			m.sc = m.scenes()
		}
		sc := m.sc
		sc.outErr = ""
		m.sceneSend(sc, []byte{0xC0 | byte(bank), byte(i)})
		if sc.out == nil {
			m.message = "the pattern was not sent: no MIDI output to the unit"
			if sc.outErr != "" {
				m.message += " (" + sc.outErr + ")"
			}
			return m, nil
		}
		slot := rows[i].Slot
		m.perfNext = &slot
		lp.ptFallback = m.ptnDue()
		m.logCue()
		m.message = fmt.Sprintf("pattern %s is sent: the unit plays it next (the unit needs PC Rx on)", slot)
		return m, m.wanted()
	}
	return m, nil
}

// ptnNotes are the places of the notes of a pattern in clock ticks within it, by pad (bank*16 + pad), and its length in clock ticks.
func (m Model) ptnNotes(slot proto.PadIndex) (map[int][]int, int, bool) {
	d := m.patPreview[slot]
	if d == nil {
		return nil, 0, false
	}
	per := formats.PPQ / 24 // the pattern's ticks are 480 to the beat, the clock's 24
	length := int(d.Length) / per
	if length <= 0 {
		return nil, 0, false
	}
	notes := map[int][]int{}
	for _, n := range d.Notes {
		key := int(n.Pad.Bank())*16 + int(n.Pad.Pad())
		notes[key] = append(notes[key], int(n.Tick)/per)
	}
	return notes, length, true
}

// ptnFit says how many of the hits fit the notes of a pattern, at the bar it could have begun on that fits best (a pattern of one
// bar has just the one).
func (m Model) ptnFit(slot proto.PadIndex, hits []hitAt) (score, phase int, ok bool) {
	notes, length, ok := m.ptnNotes(slot)
	if !ok {
		return 0, 0, false
	}
	step := ticksPerBar
	if length <= ticksPerBar {
		length = ticksPerBar
	}
	best, bestPhase := -1, 0
	for ph := 0; ph < length; ph += step {
		n := 0
		for _, h := range hits {
			pos := ((h.tick-ph)%length + length) % length
			for _, at := range notes[h.key] {
				if diff := min((pos-at+length)%length, (at-pos+length)%length); diff <= 4 {
					n++
					break
				}
			}
		}
		if n > best {
			best, bestPhase = n, ph
		}
	}
	return best, bestPhase, true
}

// ptnHits are the pads heard within the last ticks.
func (m Model) ptnHits(ticks int) []hitAt {
	var hits []hitAt
	if m.mon == nil || m.clock == nil {
		return hits
	}
	for _, h := range m.mon.hitLog {
		if h.tick <= m.clock.ticks && m.clock.ticks-h.tick <= ticks {
			hits = append(hits, h)
		}
	}
	return hits
}

// ptnConfirm makes the pattern that was sent the one that plays when the pads heard lately fit its notes clearly better than the
// notes of the pattern that played: only then has the unit really switched. Called when a pad is heard.
func (m Model) ptnConfirm() Model {
	if m.perfNext == nil || m.clock == nil {
		return m
	}
	hits := m.ptnHits(ticksPerBar)
	if len(hits) < 3 {
		return m
	}
	next, _, ok := m.ptnFit(*m.perfNext, hits)
	if !ok {
		return m
	}
	now := 0
	if m.perfNow != nil {
		now, _, _ = m.ptnFit(*m.perfNow, hits)
	}
	if next*10 < len(hits)*7 || next*10 < now*10+len(hits)*2 {
		return m
	}
	return m.ptnTakeOver()
}

// ptnTakeOver makes the pattern that was sent the one that plays.
func (m Model) ptnTakeOver() Model {
	lp := m.looper()
	m.perfNow, m.perfNext, m.perfAuto = m.perfNext, nil, false
	lp.ptFallback = 0
	if m.mon != nil {
		m.mon.played, m.mon.playedAt = map[int]bool{}, map[int]int{} // the new pattern starts with nothing heard
	}
	m.logCue()
	return m
}

// ptnTick is the last resort: a pattern that was sent and that sounds like the one that plays (so that no pad tells them apart) is
// taken for the one that plays when a whole pattern has gone by after the time it was due. A pattern the unit has been seen to
// play (found from the pads heard) ends the wait too.
func (m Model) ptnTick() Model {
	if m.perfNext == nil || m.clock == nil {
		return m
	}
	lp := m.looper()
	switch {
	case m.perfNow != nil && *m.perfNow == *m.perfNext:
		m.perfNext, lp.ptFallback = nil, 0
	case lp.ptFallback > 0 && m.clock.ticks >= lp.ptFallback:
		return m.ptnTakeOver()
	}
	return m
}

// ptnDue is the clock tick after which a pattern that was sent is taken for the one that plays, whatever is heard: one pattern
// length after the pattern that plays was due to end. 0 when that is not known.
func (m Model) ptnDue() int {
	if m.perfNow == nil || m.clock == nil || !m.clockRuns() {
		return 0
	}
	d := m.patPreview[*m.perfNow]
	if d == nil {
		return 0
	}
	length := d.Bars() * ticksPerBar
	return m.clock.ticks + 2*length
}

// webPtn is the Patterns page.
type webPtn struct {
	Bank  string       `json:"bank"`
	Slots []webPtnSlot `json:"slots"`
	Known bool         `json:"known"`
	// Now is the pattern of this bank that plays (as far as it is known), Next the one that was sent and has not taken over yet;
	// -1 for none.
	Now  int `json:"now"`
	Next int `json:"next"`
}

type webPtnSlot struct {
	N      int  `json:"n"`
	Exists bool `json:"exists"`
	Pads   int  `json:"pads"`
}

func (m Model) webPtn() webPtn {
	bank := m.ptnBankNow()
	p := webPtn{Bank: string(rune('A' + bank)), Slots: []webPtnSlot{}, Now: -1, Next: -1}
	if m.perfNow != nil && int(m.perfNow.Bank()) == bank {
		p.Now = int(m.perfNow.Pad())
	}
	if m.perfNext != nil && int(m.perfNext.Bank()) == bank {
		p.Next = int(m.perfNext.Pad())
	}
	if rows := m.patterns[bank]; rows != nil {
		p.Known = true
		for i, r := range rows {
			p.Slots = append(p.Slots, webPtnSlot{N: i + 1, Exists: r.Exists, Pads: len(r.Pads)})
		}
	}
	return p
}
