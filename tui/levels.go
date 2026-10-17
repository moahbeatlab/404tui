package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/moahbeatlab/404tui/midiin"
	"github.com/moahbeatlab/404tui/proto"
)

// The Levels view of Perform: the volume of each pad of the bank as a bar. On the EC4 the 16 encoders turn
// the levels of the 16 pads and their pushes mute (level 0) and bring back; the view itself is switched
// by the push of E16 (see ec4.go), the button in VIEW, or v.

// ec4FirstCC is the controller of the first encoder's turn: 60 to 75 on the control channel, relative
// (64 is no turn, above it up, below it down).
const ec4FirstCC = 60

func (m Model) toggleLevels() Model {
	m.perfLevels = !m.perfLevels
	if m.perfLevels {
		m.message = "Levels: encoders turn the volume of the pads, a push mutes · E16 or v goes back"
	} else {
		m.message = "Stage view"
	}
	return m
}

// levelPad is the pad of the bank on show at a place (0 to 15) of the grid.
func (m Model) levelPad(i int) (proto.PadIndex, bool) {
	rows := m.snap.Bank(m.perfBank())
	if i < 0 || i >= len(rows) {
		return proto.PadIndex{}, false
	}
	return rows[i].Pad, true
}

// levelTurn moves the level of a pad by a number of steps; a muted pad is brought back first.
func (m Model) levelTurn(i, steps int) (Model, tea.Cmd) {
	pad, ok := m.levelPad(i)
	if !ok || steps == 0 || m.unit == nil {
		return m, nil
	}
	if _, muted := m.lvlWas[pad]; muted {
		delete(m.lvlWas, pad)
	}
	return m.levelSet(pad, m.snap.Pads[pad.Index()].Level+int32(steps))
}

// levelMute silences a pad, or brings its level back.
func (m Model) levelMute(i int) (Model, tea.Cmd) {
	pad, ok := m.levelPad(i)
	if !ok || m.unit == nil {
		return m, nil
	}
	if was, muted := m.lvlWas[pad]; muted {
		delete(m.lvlWas, pad)
		return m.levelSet(pad, was)
	}
	cur := m.snap.Pads[pad.Index()].Level
	if cur == 0 {
		return m, nil
	}
	m.lvlWas[pad] = cur
	return m.levelSet(pad, 0)
}

// levelSet puts a pad's level on the screen and sends it to the unit.
func (m Model) levelSet(pad proto.PadIndex, v int32) (Model, tea.Cmd) {
	info, ok := proto.ParamByName("level")
	if !ok {
		return m, nil
	}
	v = max(min(v, info.Max), 0)
	row := &m.snap.Pads[pad.Index()]
	if row.Params == nil {
		row.Params = map[string]int32{}
	}
	row.Params["level"], row.Level = v, v
	unit, id := m.unit, info.ID
	return m, m.edit(func() tea.Msg {
		if err := unit.SetParam(id, proto.TargetPad(pad), v); err != nil {
			return doneMsg{message: fmt.Sprintf("level of %s: %v", pad, err), reload: true}
		}
		return nil
	})
}

// levelEvent handles a message of the control channel while the Levels view is on: a turn of an encoder
// (CC 60 to 75) and a push (notes 60 to 74; 75 leaves the view).
func (m Model) levelEvent(e midiin.Event) (Model, tea.Cmd, bool) {
	switch e.Kind {
	case midiin.CC:
		i := int(e.Data1) - ec4FirstCC
		if i < 0 || i > 15 || e.Data2 == 64 {
			return m, nil, false
		}
		nm, cmd := m.levelTurn(i, int(e.Data2)-64)
		return nm, cmd, true
	case midiin.NoteOn:
		i := int(e.Data1) - ec4FirstNote
		switch {
		case i == 15:
			return m.toggleLevels(), nil, true
		case i >= 0 && i < 15:
			nm, cmd := m.levelMute(i)
			return nm, cmd, true
		}
	}
	return m, nil, false
}

// levelTile is a pad of the Levels view: number and name over a bar that stands in the tile as high as
// the level, and the number of the level (or MUTED) under it.
func (m Model) levelTile(i int, w, h int) []string {
	th := m.st.th
	rows := m.snap.Bank(m.perfBank())
	row := rows[i]
	_, muted := m.lvlWas[row.Pad]
	level := int(row.Level)
	empty := !row.HasSample
	bar, base := th.accent, th.tile
	if empty {
		bar, base = th.faint, th.tileEmpty
	}
	if muted || level == 0 && !empty {
		bar = th.bad
	}
	head := lipgloss.NewStyle().Background(base).Foreground(th.text).Bold(true)
	pad := func(s string) string {
		r := []rune(s)
		if len(r) > w {
			r = r[:w]
		}
		return string(r) + strings.Repeat(" ", w-len(r))
	}
	out := []string{head.Render(pad(fmt.Sprintf(" %d %s", row.Pad.Pad()+1, strings.TrimSpace(row.Name))))}
	body := max(h-2, 1)
	filled := (level*body*2 + 127) / 254 // rounded, in rows
	if level > 0 {
		filled = max(filled, 1)
	}
	for r := 0; r < body; r++ {
		bg := base
		if body-r <= filled && !empty {
			bg = bar
		}
		out = append(out, lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", w)))
	}
	label := fmt.Sprintf(" %d", level)
	if muted {
		label = fmt.Sprintf(" MUTED (%d)", m.lvlWas[row.Pad])
	}
	if empty {
		label = ""
	}
	foot := lipgloss.NewStyle().Background(base).Foreground(th.text).Bold(true)
	return append(out, foot.Render(pad(label)))
}
