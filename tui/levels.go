package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/midiin"
	"github.com/moahbeatlab/404tui/proto"
)

// The volume of the pads from a controller: on a channel of its own (15 by default, Config), the 16
// encoders turn the levels of the 16 pads of the bank the unit is on, and their pushes mute (level 0)
// and bring back. It works in any view; Perform shows the level as the bar of each pad.

// ec4FirstCC is the controller of the first encoder's turn: 60 to 75, relative (64 is no turn, above it
// up, below it down, by the distance); the pushes are the notes of the same numbers.
const ec4FirstCC = 60

// levelPad is the pad of the bank on show at a place (0 to 15) of the grid.
func (m Model) levelPad(i int) (proto.PadIndex, bool) {
	if m.snap == nil {
		return proto.PadIndex{}, false
	}
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
	delete(m.lvlWas, pad)
	return m.levelSet(pad, m.snap.Pads[pad.Index()].Level+int32(steps))
}

// levelMute silences a pad, or brings its level back.
func (m Model) levelMute(i int) (Model, tea.Cmd) {
	pad, ok := m.levelPad(i)
	if !ok {
		return m, nil
	}
	return m.levelMutePad(pad)
}

// levelMutePad mutes a pad, or brings it back to the level it had.
func (m Model) levelMutePad(pad proto.PadIndex) (Model, tea.Cmd) {
	if m.unit == nil {
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

// muteNewPad puts a pad that just got a sample at level 0, on the unit, for the Slicer's results: they start silent, and a
// mute (hold the pad, or its tap) brings them in.
func muteNewPad(unit interface {
	SetParam(id byte, target proto.Target, value int32) error
}, pad proto.PadIndex) bool {
	info, ok := proto.ParamByName("level")
	if !ok {
		return false
	}
	return unit.SetParam(info.ID, proto.TargetPad(pad), 0) == nil
}

// lvlMuted says whether a pad was muted from here.
func (m Model) lvlMuted(pad proto.PadIndex) bool {
	_, muted := m.lvlWas[pad]
	return muted
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

// levelEvent handles a message of the volume channel: a turn of an encoder (CC 60 to 75) and a push
// (notes 60 to 75).
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
		if i >= 0 && i < 16 {
			nm, cmd := m.levelMute(i)
			return nm, cmd, true
		}
	}
	return m, nil, false
}
