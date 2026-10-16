package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/proto"
)

// askRepairPads finds the pads of the bank (or only the marked ones) that were never set up, as the
// pads of a project made offline were before this knew better: hold 1 and the pan at its left
// end. It asks before it gives them the settings of a new pad.
func (m Model) askRepairPads() (tea.Model, tea.Cmd) {
	var pads []proto.PadIndex
	rows := m.snap.Bank(m.bank)
	if len(m.marked) > 0 {
		rows = m.targets()
	}
	for _, row := range rows {
		if app.NeverSetUp(row) {
			pads = append(pads, row.Pad)
		}
	}
	if len(pads) == 0 {
		m.message = "no pad here looks never set up (hold 1 and the pan at the far left)"
		return m, nil
	}
	m.prompt = &prompt{kind: promptRepairPads, pads: pads}
	return m, nil
}

func (m Model) repairBox(p *prompt) string {
	st := m.st
	names := make([]string, len(p.pads))
	for i, pad := range p.pads {
		names[i] = pad.String()
	}
	lines := []string{
		st.bold.Render(fmt.Sprintf("Set up %s that were never set up?", plural(len(p.pads), "pad"))),
		st.dim.Render(strings.Join(names, " ")), "",
		st.text.Render("Their hold is 1 and their pan at the far left, which cuts a sample short and puts it to one side."),
		st.text.Render("They get the pan in the centre, hold 100 and time stretch 100%; nothing else changes."), "",
		st.dim.Render("y set them up   n/Esc leave them"),
	}
	return st.modal.Render(strings.Join(lines, "\n"))
}
