package tui

import (
	"fmt"
	"maps"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// noteSelection is the notes chosen to be changed together, on any number of tracks: V opens it,
// h j k l move the cursor over the pattern, and Enter chooses the note under it or lets it go
// again.
type noteSelection struct {
	// chosen are the notes, each by its pad and the tick it starts at.
	chosen map[formats.PadTick]bool
}

// selecting says whether a selection is open on the open pattern.
func (m Model) selecting() bool { return m.patSel != nil && m.pattern != nil }

// noteCol is the step a tick lies in, as the grid draws it.
func noteCol(d *app.PatternDetail, tick uint32) int {
	if d.Length == 0 {
		return 0
	}
	return min(int(uint64(tick)*uint64(d.Grid.Steps)/uint64(d.Length)), d.Grid.Steps-1)
}

// toggleChosen chooses the notes that start in the step under the cursor, or lets them go.
func (m Model) toggleChosen() (tea.Model, tea.Cmd) {
	s, d := m.patSel, m.pattern
	pad, _, ok := m.cellOfPattern()
	if !ok {
		return m, nil
	}
	notes := d.CellNotes(pad, m.patCol)
	if len(notes) == 0 {
		m.message = "no note starts here: h j k l go to one, x chooses it"
		return m, nil
	}
	if s.chosen[formats.PadTick{Pad: pad, Tick: notes[0].Tick}] {
		for _, n := range notes {
			delete(s.chosen, formats.PadTick{Pad: pad, Tick: n.Tick})
		}
	} else {
		for _, n := range notes {
			s.chosen[formats.PadTick{Pad: pad, Tick: n.Tick}] = true
		}
	}
	m.message = m.chosenText()
	return m, nil
}

// chosenText says how many notes are chosen, on how many tracks.
func (m Model) chosenText() string {
	pads := map[proto.PadIndex]bool{}
	for k := range m.patSel.chosen {
		pads[k.Pad] = true
	}
	notes := len(m.chosenNotes())
	if len(pads) <= 1 {
		for pad := range pads {
			return fmt.Sprintf("%s of %s chosen", plural(notes, "note"), pad)
		}
		return "0 notes chosen"
	}
	return fmt.Sprintf("%s on %s chosen", plural(notes, "note"), plural(len(pads), "track"))
}

// selectedCols are the steps of a track where a chosen note starts, for drawing them.
func (m Model) selectedCols(d *app.PatternDetail, pad proto.PadIndex) map[int]bool {
	if !m.selecting() || d != m.pattern {
		return nil
	}
	cols := map[int]bool{}
	for _, n := range d.Notes {
		if n.Pad == pad && m.patSel.chosen[formats.PadTick{Pad: pad, Tick: n.Tick}] {
			cols[noteCol(d, n.Tick)] = true
		}
	}
	return cols
}

// startSelecting opens a selection on the track the cursor is on, with the note there chosen.
func (m Model) startSelecting() (tea.Model, tea.Cmd) {
	pad, _, ok := m.cellOfPattern()
	if !ok {
		return m, nil
	}
	m.patSel = &noteSelection{chosen: map[formats.PadTick]bool{}}
	for _, n := range m.pattern.CellNotes(pad, m.patCol) {
		m.patSel.chosen[formats.PadTick{Pad: pad, Tick: n.Tick}] = true
	}
	m.message = "select: h j k l go to a note, on any track, x chooses it or lets it go, then change the chosen ones together"
	return m, nil
}

// selectionLine says what is chosen, for the line under the grid.
func (m Model) selectionLine() string {
	return "SELECT  " + m.chosenText()
}

// chosenNotes are the notes of the open pattern that are chosen.
func (m Model) chosenNotes() []formats.NoteRecord {
	var out []formats.NoteRecord
	for _, n := range m.pattern.Notes {
		if m.patSel.chosen[formats.PadTick{Pad: n.Pad, Tick: n.Tick}] {
			out = append(out, n)
		}
	}
	return out
}

// selectionHelp are the keys of the selection, in place of the pattern's own while it is open.
func (m Model) selectionHelp() []helpGroup {
	return []helpGroup{
		{"select", []helpKey{{"h j k l", "move"}, {"x Enter", "choose / let go"}, {"a", "all of the track"}, {"A", "all of every track"}, {"Esc V", "leave"}}},
		{"velocity", []helpKey{{", .", "by 1"}, {"< >", "by 10"}, {"*", "pad's own level"}}},
		{"length", []helpKey{{"+ -", "by a step"}}},
		{"timing", []helpKey{{"H L", "nudge 5 ticks"}, {"( )", "1 tick"}, {"0", "reset: one step long, on the grid"}}},
		{"notes", []helpKey{{"d", "take them out"}, {"c C", "condition"}, {"m s", "mute, solo the track"}, {"B", "bounce to a pad"}}},
		{"history", []helpKey{{"u", "undo"}, {"R", "redo"}, {"", m.patHistoryLabel()}}},
	}
}

// selectionKey handles a key while notes are chosen; false for the keys that go on to the
// pattern's own, which are the ones that move the cursor or play and bounce the track.
func (m Model) selectionKey(name string) (tea.Model, tea.Cmd, bool) {
	s, d := m.patSel, m.pattern
	step := int(max(d.StepTicks(), 1))
	switch name {
	case "esc", "V":
		m.patSel = nil
		m.message = ""
		return m, nil, true
	case "h", "left", "l", "right", "j", "down", "k", "up", "g", "G", "{", "[", "}", "]":
		// The cursor moves with the pattern's own keys; what is chosen stays.
		next, cmd := m.patternsKeyMove(name)
		return next, cmd, true
	case "x", "enter":
		next, cmd := m.toggleChosen()
		return next, cmd, true
	case "a", "A":
		pad, _, ok := m.cellOfPattern()
		if !ok {
			return m, nil, true
		}
		s.chosen = map[formats.PadTick]bool{}
		for _, n := range d.Notes {
			if name == "A" || n.Pad == pad {
				s.chosen[formats.PadTick{Pad: n.Pad, Tick: n.Tick}] = true
			}
		}
		m.message = "every note chosen: " + m.chosenText()
		return m, nil, true
	case ",", ".", "<", ">", "*":
		by := map[string]int{",": -1, ".": 1, "<": -10, ">": 10}[name]
		edit := formats.NoteEdit{Velocity: by}
		what := fmt.Sprintf("velocity %+d", by)
		if name == "*" {
			edit, what = formats.NoteEdit{SetVelocity: 127}, "velocity 127"
		}
		next, cmd := m.editSelection(edit, what)
		return next, cmd, true
	case "+", "=", "-":
		by := step
		if name == "-" {
			by = -step
		}
		next, cmd := m.editSelection(formats.NoteEdit{Length: by}, fmt.Sprintf("length %+d ticks", by))
		return next, cmd, true
	case "H", "L", "(", ")":
		by := map[string]int{"H": -5, "L": 5, "(": -1, ")": 1}[name]
		next, cmd := m.editSelection(formats.NoteEdit{Move: by}, fmt.Sprintf("moved %+d ticks", by))
		return next, cmd, true
	case "0":
		next, cmd := m.editSelection(formats.NoteEdit{Snap: uint32(step), SetLength: step}, "reset: one step long, on the grid")
		return next, cmd, true
	case "d", "backspace":
		next, cmd := m.editSelection(formats.NoteEdit{Delete: true}, "taken out")
		return next, cmd, true
	case "c", "C":
		next, cmd := m.changeSelectionCondition(name == "c")
		return next, cmd, true
	case "m", "M", "s", "S", "B", "P", " ", "u", "R":
		// Mute, solo, bounce, play and the history work on the track or the pattern as they are.
		return m, nil, false
	}
	m.message = "that key is not for a selection: Esc leaves it"
	return m, nil, true
}

// editSelection applies edit to the chosen notes and keeps them chosen where they land.
func (m Model) editSelection(edit formats.NoteEdit, what string) (tea.Model, tea.Cmd) {
	s, d := m.patSel, m.pattern
	if len(m.chosenNotes()) == 0 {
		m.message = "no note is chosen: h j k l go to one, x chooses it, a takes a track, A all"
		return m, nil
	}
	chosen := maps.Clone(s.chosen)
	// Where the notes will be, for the selection to follow them.
	next := map[formats.PadTick]bool{}
	for k := range chosen {
		if edit.Delete {
			break
		}
		tick := int64(k.Tick)
		if edit.Snap > 0 {
			g := int64(edit.Snap)
			tick = (tick + g/2) / g * g
		}
		tick += int64(edit.Move)
		next[formats.PadTick{Pad: k.Pad, Tick: uint32(min(max(tick, 0), int64(max(d.Length, 1))-1))}] = true
	}
	if edit.Delete {
		m.patSel = nil
	} else {
		s.chosen = next
	}
	pads := map[proto.PadIndex]bool{}
	for k := range chosen {
		pads[k.Pad] = true
	}
	where := "of " + (func() string {
		for pad := range pads {
			if len(pads) == 1 {
				return pad.String()
			}
		}
		return plural(len(pads), "track")
	})()
	note := fmt.Sprintf("%s: %s %s %s", d.Slot, plural(len(chosen), "note"), where, what)
	return m.editPatternBytes(func(b []byte) ([]byte, error) {
		out, _, err := formats.EditNoteSet(b, chosen, edit)
		return out, err
	}, note)
}

// changeSelectionCondition steps the condition of the chosen notes, the first one's, and gives
// the next to all of them.
func (m Model) changeSelectionCondition(forward bool) (tea.Model, tea.Cmd) {
	chosen := m.chosenNotes()
	if len(chosen) == 0 {
		m.message = "no note is chosen: h l go to one, x chooses it, a takes them all"
		return m, nil
	}
	dir := 1
	if !forward {
		dir = -1
	}
	first := app.NoteKey{Pad: chosen[0].Pad, Step: int(chosen[0].Tick) / (formats.PPQ / 4)}
	next := app.StepCond(m.patConds[first], dir)
	conds := maps.Clone(m.patConds)
	if conds == nil {
		conds = app.Conditions{}
	}
	for _, n := range chosen {
		conds[app.NoteKey{Pad: n.Pad, Step: int(n.Tick) / (formats.PPQ / 4)}] = next
	}
	m.patConds = conds.PrunedTo(m.pattern.Notes)
	m.saveConditions()
	m.loopDirty = true
	m.message = fmt.Sprintf("%s: %s%s", m.chosenText(), next, m.cycleNote())
	return m, nil
}
