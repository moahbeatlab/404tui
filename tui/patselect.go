package tui

import (
	"fmt"
	"maps"
	"slices"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// noteSelection is the notes of one track chosen to be changed together: V starts it at the
// cursor, h and l stretch it over the steps between where it started and the cursor, and a takes
// every note of the track.
type noteSelection struct {
	pad    proto.PadIndex
	anchor int
	// ticks are where the chosen notes start.
	ticks map[uint32]bool
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

// selectRange chooses the notes of the selection's track that start in the steps from the
// anchor to the cursor.
func (m Model) selectRange() {
	s, d := m.patSel, m.pattern
	lo, hi := min(s.anchor, m.patCol), max(s.anchor, m.patCol)
	s.ticks = map[uint32]bool{}
	for _, n := range d.Notes {
		if n.Pad == s.pad {
			if c := noteCol(d, n.Tick); c >= lo && c <= hi {
				s.ticks[n.Tick] = true
			}
		}
	}
}

// selectedCols are the steps of a track where a chosen note starts, for drawing them.
func (m Model) selectedCols(d *app.PatternDetail, pad proto.PadIndex) map[int]bool {
	if !m.selecting() || d != m.pattern || m.patSel.pad != pad {
		return nil
	}
	cols := map[int]bool{}
	for _, n := range d.Notes {
		if n.Pad == pad && m.patSel.ticks[n.Tick] {
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
	m.patSel = &noteSelection{pad: pad, anchor: m.patCol}
	m.selectRange()
	m.message = "select: h l stretch it over the notes of " + pad.String() + ", a takes all of them, then change them together (the keys are listed below); Esc leaves"
	return m, nil
}

// selectionLine says what is chosen, for the line under the grid.
func (m Model) selectionLine() string {
	s := m.patSel
	n := len(m.chosenNotes())
	text := fmt.Sprintf("SELECT  %s of %s", plural(n, "note"), s.pad)
	if n == 0 {
		text += "  (h l reach a note, a takes them all)"
	}
	return text
}

// chosenNotes are the notes of the open pattern that are chosen.
func (m Model) chosenNotes() []formats.NoteRecord {
	var out []formats.NoteRecord
	for _, n := range m.pattern.Notes {
		if n.Pad == m.patSel.pad && m.patSel.ticks[n.Tick] {
			out = append(out, n)
		}
	}
	return out
}

// selectionHelp are the keys of the selection, in place of the pattern's own while it is open.
func (m Model) selectionHelp() []helpGroup {
	return []helpGroup{
		{"select", []helpKey{{"h l", "stretch"}, {"g G { }", "move on"}, {"a", "all notes of the track"}, {"Esc V", "leave"}}},
		{"velocity", []helpKey{{", .", "by 1"}, {"< >", "by 10"}, {"*", "pad's own level"}}},
		{"length", []helpKey{{"+ -", "by a step"}}},
		{"timing", []helpKey{{"H L", "nudge 5 ticks"}, {"( )", "1 tick"}, {"0", "back on the grid"}}},
		{"notes", []helpKey{{"x d", "take them out"}, {"c C", "condition"}, {"m s", "mute, solo the track"}, {"B", "bounce to a pad"}}},
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
	case "h", "left", "l", "right", "g", "G", "{", "[", "}", "]":
		// The cursor moves (the pattern's own keys), and the selection stretches with it.
		next, cmd := m.patternsKeyMove(name)
		nm := next.(Model)
		nm.selectRange()
		return nm, cmd, true
	case "a":
		s.ticks = map[uint32]bool{}
		for _, n := range d.Notes {
			if n.Pad == s.pad {
				s.ticks[n.Tick] = true
			}
		}
		m.message = fmt.Sprintf("every note of %s is chosen: %s", s.pad, plural(len(s.ticks), "note"))
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
		next, cmd := m.editSelection(formats.NoteEdit{Snap: uint32(step)}, "back on the grid")
		return next, cmd, true
	case "x", "d", "backspace":
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
		m.message = "no note is chosen: h l reach one, a takes them all"
		return m, nil
	}
	ticks, pad := maps.Clone(s.ticks), s.pad
	// Where the notes will be, for the selection to follow them.
	next := map[uint32]bool{}
	for t := range ticks {
		if edit.Delete {
			break
		}
		tick := int64(t)
		if edit.Snap > 0 {
			g := int64(edit.Snap)
			tick = (tick + g/2) / g * g
		}
		tick += int64(edit.Move)
		next[uint32(min(max(tick, 0), int64(max(d.Length, 1))-1))] = true
	}
	if edit.Delete {
		m.patSel = nil
	} else {
		s.ticks = next
	}
	note := fmt.Sprintf("%s: %s of %s %s", d.Slot, plural(len(ticks), "note"), pad, what)
	return m.editPatternBytes(func(b []byte) ([]byte, error) {
		out, _, err := formats.EditNotes(b, pad, ticks, edit)
		return out, err
	}, note)
}

// changeSelectionCondition steps the condition of the chosen notes, the first one's, and gives
// the next to all of them.
func (m Model) changeSelectionCondition(forward bool) (tea.Model, tea.Cmd) {
	chosen := m.chosenNotes()
	if len(chosen) == 0 {
		m.message = "no note is chosen: h l reach one, a takes them all"
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
	m.message = fmt.Sprintf("%s of %s: %s%s", plural(len(chosen), "note"), m.patSel.pad, next, m.cycleNote())
	return m, nil
}

// sortedTicks lists the chosen ticks in order, for messages and tests.
func (s *noteSelection) sortedTicks() []uint32 {
	return slices.Sorted(maps.Keys(s.ticks))
}
