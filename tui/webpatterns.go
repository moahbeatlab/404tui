package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

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
		lp.ptBoundary = m.ptnBoundary()
		m.logCue()
		m.message = fmt.Sprintf("pattern %s is sent: the unit plays it next (the unit needs PC Rx on)", slot)
		return m, m.wanted()
	}
	return m, nil
}

// ptnBoundary is the clock tick at which the pattern that plays ends and the next one starts: the next multiple of its length
// from where it began (the unit's start, or when the last one took over). 0 when its length is not known.
func (m Model) ptnBoundary() int {
	if m.perfNow == nil || m.clock == nil || !m.clockRuns() {
		return 0
	}
	d := m.patPreview[*m.perfNow]
	if d == nil {
		return 0
	}
	length := d.Bars() * ticksPerBar
	phase := 0
	if m.lp != nil {
		phase = m.lp.ptPhase
	}
	n := (m.clock.ticks-phase)/length + 1
	return phase + n*length
}

// ptnTick lets the pattern that was sent take over when its time has come, or when the notes heard say it plays already.
func (m Model) ptnTick() Model {
	if m.perfNext == nil || m.clock == nil {
		return m
	}
	lp := m.looper()
	switch {
	case m.perfNow != nil && *m.perfNow == *m.perfNext:
		m.perfNext, lp.ptBoundary = nil, 0
	case lp.ptBoundary > 0 && m.clock.ticks >= lp.ptBoundary:
		m.perfNow, m.perfNext, m.perfAuto = m.perfNext, nil, false
		lp.ptBoundary, lp.ptPhase = 0, m.clock.ticks
		if m.mon != nil {
			m.mon.played, m.mon.playedAt = map[int]bool{}, map[int]int{} // the new pattern starts with nothing heard
		}
		m.logCue()
	}
	return m
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
