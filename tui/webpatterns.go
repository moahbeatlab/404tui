package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/proto"
)

// The Patterns page of the web UI: the patterns of a bank, and a tap makes the unit play one next. The unit is told by a MIDI
// Program Change on the channel of the bank (1 for A to 10 for J) with the number of the pattern (0 to 15), as the unit's
// "PC Rx" setting wants it; the unit does not say it took it, so the page marks the last one sent.

// ptnBankNow is the bank the Patterns page shows: the one chosen there, else the unit's.
func (m Model) ptnBankNow() int {
	if lp := m.lp; lp != nil && lp.ptSet {
		return clamp(lp.ptBank, 0, proto.Banks-1)
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
		lp.ptSet, lp.ptBank, lp.ptLast = true, (i%proto.Banks+proto.Banks)%proto.Banks, -1
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
		lp.ptLastBank, lp.ptLast = bank, i
		m.message = fmt.Sprintf("pattern %c%d is sent: the unit plays it next (the unit needs PC Rx on)", 'A'+bank, i+1)
	}
	return m, nil
}

// webPtn is the Patterns page.
type webPtn struct {
	Bank  string       `json:"bank"`
	Slots []webPtnSlot `json:"slots"`
	Known bool         `json:"known"`
	// Last is the pattern sent last in this bank, -1 for none.
	Last int `json:"last"`
}

type webPtnSlot struct {
	N      int  `json:"n"`
	Exists bool `json:"exists"`
	Pads   int  `json:"pads"`
}

func (m Model) webPtn() webPtn {
	bank := m.ptnBankNow()
	p := webPtn{Bank: string(rune('A' + bank)), Slots: []webPtnSlot{}, Last: -1}
	if lp := m.lp; lp != nil && lp.ptLast >= 0 && lp.ptLastBank == bank {
		p.Last = lp.ptLast
	}
	if rows := m.patterns[bank]; rows != nil {
		p.Known = true
		for i, r := range rows {
			p.Slots = append(p.Slots, webPtnSlot{N: i + 1, Exists: r.Exists, Pads: len(r.Pads)})
		}
	}
	return p
}
