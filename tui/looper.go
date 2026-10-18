package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// The Looper view is the bank of loops that play together: a pattern in which all 16 pads start on beat 1 and last the
// whole pattern, so that a sample put on a pad plays from the next round on. A pad that is no longer wanted is deleted: its
// note in the pattern is silent then, and the pad is free for the next one. The samples come from the unit's own looper, the
// Slicer or anywhere else; this view only keeps the bank and the pattern in order.

// loopBarChoices are the lengths of a loop the b key goes through.
var loopBarChoices = []int{1, 2, 4, 8}

type looperState struct {
	// cursor is the pad in front (0 to 15); delAsk is the pad d asked to delete (-1 for none).
	cursor, delAsk int
	// slBank and slPad are the pad the web page's Slicer will cut (slPad -1 for none); slSet says they were chosen.
	slBank, slPad int
	slSet         bool
	// slOut are slices that wait for a pad (slHave), from slFrom in slN slices; slDest is the pad chosen for them (-1 for none).
	slOut       formats.Sample
	slHave      bool
	slFrom      string
	slOrder     string
	slN, slDest int
	// jobs wait for the pattern to come round.
	jobs []lpJob
}

// looper is the state of the Looper, made when it is first wanted.
func (m *Model) looper() *looperState {
	if m.lp == nil {
		m.lp = &looperState{delAsk: -1}
	}
	return m.lp
}

func (m Model) loopBank() int { return clamp(m.mem.LooperBank, 0, proto.Banks-1) }

// loopBars is how long a loop is.
func (m Model) loopBars() int {
	if slices.Contains(loopBarChoices, m.mem.LooperBars) {
		return m.mem.LooperBars
	}
	return 2
}

func (m Model) loopFreePad() (proto.PadIndex, bool) {
	for _, row := range m.snap.Bank(m.loopBank()) {
		if !row.HasSample {
			return row.Pad, true
		}
	}
	return proto.PadIndex{}, false
}

func (m Model) clockRuns() bool {
	return m.clock != nil && m.clock.running && time.Since(m.clock.at) < 2*time.Second
}

// looperKey is a key of the Looper view.
func (m Model) looperKey(name string) (tea.Model, tea.Cmd) {
	lp := m.looper()
	ask := lp.delAsk
	lp.delAsk = -1
	switch name {
	case "w":
		return m.laterToggle()
	case "s":
		return m.looperSlicesNext()
	case "x":
		return m.looperRemix(lp.cursor)
	case "[":
		return m.looperBankStep(-1)
	case "]":
		return m.looperBankStep(1)
	case "b":
		return m.looperBarsNext()
	case "h", "left":
		lp.cursor = clamp(lp.cursor-1, 0, 15)
	case "l", "right":
		lp.cursor = clamp(lp.cursor+1, 0, 15)
	case "k", "up":
		lp.cursor = clamp(lp.cursor-4, 0, 15)
	case "j", "down":
		lp.cursor = clamp(lp.cursor+4, 0, 15)
	case "p":
		return m.looperPattern()
	case "d":
		row := m.snap.Bank(m.loopBank())[lp.cursor]
		if !row.HasSample {
			m.message = fmt.Sprintf("%s holds no sample", row.Pad)
			return m, nil
		}
		if ask != lp.cursor {
			lp.delAsk = lp.cursor
			m.message = fmt.Sprintf("d again deletes %s %s (its note in the pattern stays, silent)", row.Pad, row.Name)
			return m, nil
		}
		return m.looperDelete(lp.cursor)
	}
	return m, nil
}

// looperBankStep moves the bank the loops go to, up or down.
func (m Model) looperBankSet(bank int) (tea.Model, tea.Cmd) {
	if bank < 0 || bank >= proto.Banks {
		return m, nil
	}
	return m.looperBankStep(bank - m.loopBank())
}

func (m Model) looperBankStep(dir int) (tea.Model, tea.Cmd) {
	m.mem.LooperBank = (m.loopBank() + dir + proto.Banks) % proto.Banks
	m.message = fmt.Sprintf("loops go to bank %c", 'A'+m.loopBank())
	return m, m.wanted()
}

// looperBarsNext goes to the next length of a loop.
func (m Model) looperBarsNext() (tea.Model, tea.Cmd) {
	i := slices.Index(loopBarChoices, m.loopBars())
	m.mem.LooperBars = loopBarChoices[(i+1)%len(loopBarChoices)]
	m.message = fmt.Sprintf("a loop is %s long", plural(m.mem.LooperBars, "bar"))
	return m, nil
}

// looperDelete deletes the sample of a pad (0 to 15) of the bank, on the last beat of the pattern when the clock runs
// (asking again takes it back).
func (m Model) looperDelete(i int) (tea.Model, tea.Cmd) {
	if i < 0 || i > 15 || m.snap == nil {
		return m, nil
	}
	m.looper() // made here, so that the queue stays with the model that goes back
	row := m.snap.Bank(m.loopBank())[i]
	key := fmt.Sprintf("del:%d", i)
	if m.looperUnlater(key) {
		m.message = fmt.Sprintf("%s is no longer deleted when the pattern comes round", row.Pad)
		return m, nil
	}
	if !row.HasSample {
		m.message = fmt.Sprintf("%s holds no sample", row.Pad)
		return m, nil
	}
	if m.looperLater(lpJob{key: key, label: "delete " + row.Pad.String(), cmd: m.deleteCmd([]proto.PadIndex{row.Pad})}) {
		m.message = fmt.Sprintf("%s is deleted on %s, in %.1f beats (delete it again to keep it)", row.Pad, m.laterName(), m.beatsToLater())
		return m, nil
	}
	m.busy = "deleting " + row.Pad.String()
	return m, m.deleteCmd([]proto.PadIndex{row.Pad})
}

// looperPattern writes the pattern of the bank in which all 16 pads start on beat 1.
func (m Model) looperPattern() (tea.Model, tea.Cmd) {
	unit, bank, bars := m.unit, m.loopBank(), m.loopBars()
	m.busy = "making a pattern"
	return m, m.edit(func() tea.Msg {
		slot, err := firstFreePatternSlot(unit, bank)
		if err != nil {
			return doneMsg{message: err.Error()}
		}
		pads := proto.AllPads()[bank*proto.PadsPerBank : (bank+1)*proto.PadsPerBank]
		if err := app.WriteStartPattern(unit, slot, pads, bars); err != nil {
			return doneMsg{message: "the pattern was not made: " + err.Error()}
		}
		return doneMsg{message: fmt.Sprintf("pattern %s made (bank %c, pad %d): all 16 pads on beat 1, %s", slot, 'A'+slot.Bank(), slot.Pad()+1, plural(bars, "bar")), reload: true, patterns: true}
	})
}

// looperBody is the Looper view.
func (m Model) looperBody(g geom) string {
	st, lp := m.st, m.looper()
	bank := m.loopBank()
	width := g.bodyW + g.bodyX - 3
	head := lipgloss.NewStyle().Foreground(st.th.accent).Bold(true).Render(fmt.Sprintf("BANK %c", 'A'+bank)) +
		st.dim.Render("   pattern ") + st.bold.Render(plural(m.loopBars(), "bar"))
	if clock := m.clockText(m.loopBars()); clock != "" {
		head += "   " + lipgloss.NewStyle().Foreground(st.th.good).Bold(true).Render("♩ "+clock)
	}
	status := st.text.Render(fmt.Sprintf("a delete or a placed sample waits for %s", m.laterName()))
	if pad, ok := m.loopFreePad(); ok {
		status = st.text.Render(fmt.Sprintf("next free pad: %s   ·   a delete waits for %s", pad, m.laterName()))
	} else {
		status = lipgloss.NewStyle().Foreground(st.th.hot).Render(fmt.Sprintf("bank %c is full: d deletes a loop", 'A'+bank))
	}
	if t := m.laterText(); t != "" {
		status += "\n" + lipgloss.NewStyle().Foreground(st.th.hot).Bold(true).Render(t)
	}
	pattern := st.dim.Render("pattern: reading…")
	if rows := m.patterns[bank]; rows != nil {
		pattern = lipgloss.NewStyle().Foreground(st.th.hot).Render("no pattern with all 16 pads: p makes one")
		for i, r := range rows {
			if r.Exists && len(r.Pads) == proto.PadsPerBank {
				pattern = st.dim.Render(fmt.Sprintf("pattern %s plays all 16 pads", rows[i].Slot))
				break
			}
		}
	}
	keys := st.dim.Render("b length   [ ] bank   p pattern   d delete   w delete/place on beat 1 or last beat   s slices   x remix   h j k l pad")
	top := []string{head, "", status, pattern, ""}
	bottom := []string{"", keys}
	avail := g.bodyH - 2 - len(top) - len(bottom) - 3
	tileH := max(avail/4, 3)
	tileW := max((width-3)/4, 10)
	rows := m.snap.Bank(bank)
	lines := append([]string{}, top...)
	lines = append(lines, m.grid(g, func(p int) string {
		row := rows[p]
		ts := tileState{
			empty:   !row.HasSample,
			bus:     row.Params["bus-fx"],
			flags:   padFlags(row),
			cursor:  lp.cursor == p,
			bars:    m.padBars(row, tileW),
			noLevel: false,
		}
		return m.tile(tileW, tileH, fmt.Sprint(row.Pad.Pad()+1), row.Name, row.Level, ts)
	}))
	lines = append(lines, bottom...)
	return m.box("Looper", strings.Join(lines, "\n"), g.bodyW+g.bodyX-1, g.bodyH, true)
}
