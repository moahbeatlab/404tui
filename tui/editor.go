package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/proto"
)

// fieldKind is how a setting is changed.
type fieldKind int

const (
	fieldNumber fieldKind = iota
	// fieldCycle wraps from its last value to its first.
	fieldCycle
)

// editField is one setting of a pad the editor offers.
type editField struct {
	name  string
	label string
	kind  fieldKind
	// step is how much - and + move it.
	step   int32
	format func(int32) string
}

func plain(v int32) string { return fmt.Sprint(v) }

func signedInt(v int32) string { return fmt.Sprintf("%+d", v) }

func percent(v int32) string { return fmt.Sprintf("%.0f%%", float64(v)/100) }

func bpmText(v int32) string { return fmt.Sprintf("%.2f", float64(v)/100) }

func onOffText(v int32) string {
	if v != 0 {
		return "on"
	}
	return "off"
}

var editFields = []editField{
	{"level", "Level", fieldNumber, 5, plain},
	{"balance", "Pan", fieldNumber, 4, panText},
	{"pitch-coarse", "Pitch (semitones)", fieldNumber, 1, signedInt},
	{"pitch-fine", "Fine pitch (cents)", fieldNumber, 10, signedInt},
	{"time-stretch", "Time stretch", fieldNumber, 500, percent},
	{"bpm", "BPM", fieldNumber, 100, bpmText},
	{"play-mode", "Play mode", fieldCycle, 1, playModeName},
	{"gate", "Gate", fieldCycle, 1, onOffText},
	{"loop", "Loop", fieldCycle, 1, onOffText},
	{"bpm-sync", "BPM sync", fieldCycle, 1, onOffText},
	{"vinyl", "Vinyl", fieldCycle, 1, onOffText},
	{"mute-group", "Mute group", fieldNumber, 1, offOr},
	{"bus-fx", "Bus FX", fieldCycle, 1, busName},
	{"attack", "Attack", fieldNumber, 5, plain},
	{"hold", "Hold", fieldNumber, 5, plain},
	{"release", "Release", fieldNumber, 5, plain},
}

// editor is the dialog for changing a pad's settings. With several pads marked it changes
// all of them, showing the values of the first.
type editor struct {
	pads []proto.PadIndex
	at   int
}

// openEditor opens the settings of the marked pads, or the one in front.
func (m Model) openEditor() (tea.Model, tea.Cmd) {
	pads := withSamples(m.targets())
	if len(pads) == 0 {
		m.message = "nothing to edit: no sample there"
		return m, nil
	}
	m.editor = &editor{pads: pads}
	return m, nil
}

func (m Model) editorKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	e := m.editor
	switch key.String() {
	case "esc", "enter", "q", "S":
		m.editor = nil
	case "j", "down", "tab":
		e.at = (e.at + 1) % len(editFields)
	case "k", "up", "shift+tab":
		e.at = (e.at + len(editFields) - 1) % len(editFields)
	case "g":
		e.at = 0
	case "G":
		e.at = len(editFields) - 1
	case "h", "left", "-", "_":
		return m.adjust(editFields[e.at], -1, e.pads)
	case "l", "right", "+", "=", " ":
		return m.adjust(editFields[e.at], 1, e.pads)
	}
	return m, nil
}

// adjust moves a setting of the pads one step and sends it to the unit. The change shows at
// once; if the unit refuses it, the pads are read again.
func (m Model) adjust(f editField, dir int32, pads []proto.PadIndex) (tea.Model, tea.Cmd) {
	info, ok := proto.ParamByName(f.name)
	if !ok {
		return m, nil
	}
	first := &m.snap.Pads[pads[0].Index()]
	cur := first.Params[f.name]
	next := cur + dir*f.step
	switch {
	case f.kind == fieldCycle && next > info.Max:
		next = info.Min
	case f.kind == fieldCycle && next < info.Min:
		next = info.Max
	default:
		next = max(min(next, info.Max), info.Min)
	}
	if next == cur && len(pads) == 1 {
		m.message = fmt.Sprintf("%s is at its limit", f.label)
		return m, nil
	}
	for _, pad := range pads {
		row := &m.snap.Pads[pad.Index()]
		if row.Params == nil {
			row.Params = map[string]int32{}
		}
		row.Params[f.name] = next
		switch f.name {
		case "level":
			row.Level = next
		case "bpm":
			row.BPM = float64(next) / 100
		}
	}
	unit, id := m.unit, info.ID
	return m, m.edit(func() tea.Msg {
		for _, pad := range pads {
			if err := unit.SetParam(id, proto.TargetPad(pad), next); err != nil {
				return doneMsg{message: fmt.Sprintf("%s on %s: %v", f.label, pad, err), reload: true}
			}
		}
		return noteMsg(fmt.Sprintf("%s %s on %s", f.label, f.format(next), padList(pads)))
	})
}

func padList(pads []proto.PadIndex) string {
	if len(pads) == 1 {
		return pads[0].String()
	}
	return fmt.Sprintf("%d pads", len(pads))
}

func (m Model) editorBox() string {
	st, e := m.st, m.editor
	row := m.snap.Pads[e.pads[0].Index()]
	title := fmt.Sprintf("Settings  ·  %s  %s", row.Pad, row.Name)
	if len(e.pads) > 1 {
		title = fmt.Sprintf("Settings  ·  %d pads, showing %s", len(e.pads), row.Pad)
	}
	lines := []string{st.bold.Render(title), ""}
	for i, f := range editFields {
		v := row.Params[f.name]
		value := f.format(v)
		if f.name == "level" {
			value = levelBar(12, int(v), 127) + " " + value
		}
		label := fmt.Sprintf("%-20s", f.label)
		switch {
		case i == e.at:
			lines = append(lines, lipgloss.NewStyle().Background(st.th.cursorBg).Foreground(st.th.cursorFg).Bold(true).Render(fmt.Sprintf(" %s %-28s", label, value)))
		default:
			lines = append(lines, " "+st.dim.Render(label)+" "+st.text.Render(value))
		}
	}
	lines = append(lines, "", st.dim.Render("j/k select  ·  h/l or - + change  ·  Space steps up  ·  Esc close"),
		st.faint.Render("changes go to the unit as you make them"))
	return st.modal.Render(strings.Join(lines, "\n"))
}

// nudge changes a setting of the marked pads, or the one in front, by one step: for the
// quick keys that need no dialog.
func (m Model) nudge(name string, dir int32) (tea.Model, tea.Cmd) {
	pads := withSamples(m.targets())
	if len(pads) == 0 {
		m.message = "nothing to change: no sample there"
		return m, nil
	}
	for _, f := range editFields {
		if f.name == name {
			return m.adjust(f, dir, pads)
		}
	}
	return m, nil
}

var _ = app.PreviewLength
