package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/proto"
)

// Isolating a pad (Perform): the pad keeps its bus and every other pad on the same bus goes dry, so
// the effect is heard on that pad alone. What the pads had is remembered and put back.
//
// A double click on a pad isolates it, and a double click again brings everything back. The
// ISOLATE button (i, or a click on its panel) is the same for several pads: click the pads to
// add or take them out; leaving the button puts everything back.

// isolation is what is isolated and what was changed for it.
type isolation struct {
	// pads are the ones that keep their bus.
	pads map[proto.PadIndex]bool
	// was holds the bus of each pad that was set dry, to put back.
	was map[proto.PadIndex]int32
	// multi is the button: a click on a pad adds or takes it out.
	multi bool
}

// doubleClick is how close two clicks on a pad must be to count as a double click.
const doubleClick = 400 * time.Millisecond

// busOf is the bus a pad goes to, as it was before it was set dry for an isolation.
func (m Model) busOf(pad proto.PadIndex) int32 {
	if m.iso != nil {
		if b, ok := m.iso.was[pad]; ok {
			return b
		}
	}
	return m.snap.Pads[pad.Index()].Params["bus-fx"]
}

// setIsolated makes pads the isolated ones: the others on their buses go dry, the ones no longer
// in the way go back to what they had. An empty set puts everything back.
func (m Model) setIsolated(pads map[proto.PadIndex]bool) (tea.Model, tea.Cmd) {
	multi := m.iso != nil && m.iso.multi
	was := map[proto.PadIndex]int32{}
	if m.iso != nil {
		for p, b := range m.iso.was {
			was[p] = b
		}
	}
	buses := map[int32]bool{}
	for p := range pads {
		if b := m.busOf(p); b > 0 {
			buses[b] = true
		}
	}
	type write struct {
		pad   proto.PadIndex
		value int32
	}
	var writes []write
	for _, row := range m.snap.Pads {
		if !row.HasSample {
			continue
		}
		pad := row.Pad
		cur, dried := was[pad]
		if !dried {
			cur = row.Params["bus-fx"]
		}
		switch {
		case !pads[pad] && buses[cur]:
			if !dried {
				was[pad] = cur
				writes = append(writes, write{pad, 0})
			}
		case dried:
			delete(was, pad)
			writes = append(writes, write{pad, cur})
		}
	}
	slices.SortFunc(writes, func(a, b write) int { return int(a.pad.Index()) - int(b.pad.Index()) })
	if len(pads) == 0 && !multi && len(writes) == 0 {
		m.iso = nil
		return m, nil
	}
	m.iso = &isolation{pads: pads, was: was, multi: multi}
	if len(pads) == 0 && !multi {
		m.iso = nil
	}
	if len(writes) == 0 {
		return m, nil
	}
	for _, w := range writes {
		row := &m.snap.Pads[w.pad.Index()]
		if row.Params == nil {
			row.Params = map[string]int32{}
		}
		row.Params["bus-fx"] = w.value
	}
	info, _ := proto.ParamByName("bus-fx")
	unit, id := m.unit, info.ID
	m.busy = "isolating"
	count := len(writes)
	return m, m.edit(func() tea.Msg {
		for _, w := range writes {
			if err := unit.SetParam(id, proto.TargetPad(w.pad), w.value); err != nil {
				return doneMsg{message: fmt.Sprintf("bus on %s: %v", w.pad, err), reload: true}
			}
		}
		return doneMsg{message: fmt.Sprintf("%s changed", plural(count, "pad"))}
	})
}

// isolateSwitch is the switch of the ISOLATE panel: on, to tap the pads that keep their bus; off, with
// everything put back (also after a double tap isolated one pad).
func (m Model) isolateSwitch() (tea.Model, tea.Cmd) {
	if m.iso != nil {
		return m.isolateOff()
	}
	return m.isolateButton()
}

// isolateButton switches the multi-pad isolation on, or off with everything put back.
func (m Model) isolateButton() (tea.Model, tea.Cmd) {
	if m.iso != nil && m.iso.multi {
		m.iso.multi = false
		return m.setIsolated(nil)
	}
	pads := map[proto.PadIndex]bool{}
	if m.iso != nil {
		pads = m.iso.pads
	}
	was := map[proto.PadIndex]int32{}
	if m.iso != nil {
		was = m.iso.was
	}
	m.iso = &isolation{pads: pads, was: was, multi: true}
	m.message = "ISOLATE: click the pads to keep on their bus; i or Esc puts everything back"
	return m, nil
}

// isolateOff puts everything back.
func (m Model) isolateOff() (tea.Model, tea.Cmd) {
	if m.iso == nil {
		return m, nil
	}
	m.iso.multi = false
	m.message = "everything is back on its bus"
	return m.setIsolated(nil)
}

// clickPad is a click on a pad of the Perform view. One click mutes the pad (on this computer, as
// m does in the pattern: the unit has no mute to set), a double click isolates it, or brings
// everything back; with the ISOLATE button on, a click adds the pad or takes it out.
func (m Model) clickPad(pad proto.PadIndex) (tea.Model, tea.Cmd) {
	row := m.snap.Pads[pad.Index()]
	if !row.HasSample {
		return m, nil
	}
	if m.iso != nil && m.iso.multi {
		pads := map[proto.PadIndex]bool{}
		for p := range m.iso.pads {
			pads[p] = true
		}
		if pads[pad] {
			delete(pads, pad)
		} else if m.busOf(pad) == 0 {
			m.message = fmt.Sprintf("%s goes to no bus: nothing to isolate", pad)
			return m, nil
		} else {
			pads[pad] = true
		}
		return m.setIsolated(pads)
	}
	now := time.Now()
	double := m.clickPadAt == pad && m.clickAt.Add(doubleClick).After(now)
	m.clickPadAt, m.clickAt = pad, now
	if !double {
		m.muted[pad] = !m.muted[pad]
		return m, nil
	}
	// The first click of the double click muted the pad: that is undone.
	m.muted[pad] = !m.muted[pad]
	m.clickAt = time.Time{}
	if m.iso != nil {
		return m.isolateOff()
	}
	if m.busOf(pad) == 0 {
		m.message = fmt.Sprintf("%s goes to no bus: nothing to isolate", pad)
		return m, nil
	}
	m.message = fmt.Sprintf("%s alone on %s: double click again, or Esc, to bring the others back", pad, busName(m.busOf(pad)))
	return m.setIsolated(map[proto.PadIndex]bool{pad: true})
}

// isolateStatus says what the focus does now, for under the buttons of the VIEW panel.
func (m Model) isolateStatus() []string {
	st := m.st
	var names []string
	if m.iso != nil {
		for p := range m.iso.pads {
			names = append(names, p.String())
		}
		slices.Sort(names)
	}
	switch {
	case m.iso != nil && m.iso.multi:
		list := "no pad yet"
		if len(names) > 0 {
			list = strings.Join(names, " ")
		}
		return []string{
			st.hot.Bold(true).Render("focus: keeping "+list) + st.text.Render("  ·  "+plural(len(m.iso.was), "pad")+" dry"),
			st.dim.Render("tap pads to add or take them out; tap focus again: all back"),
		}
	case m.iso != nil:
		return []string{
			st.hot.Bold(true).Render("focus: "+strings.Join(names, " ")+" alone") + st.text.Render("  ·  "+plural(len(m.iso.was), "pad")+" dry"),
			st.dim.Render("double tap it, or tap focus: all back"),
		}
	}
	return []string{st.dim.Render("focus: tap it, then the pads that keep their bus (or double tap a pad)")}
}

// Shutdown puts back what the program changed on the unit (the buses of the pads that focus made dry),
// for the end of a program that is stopped from outside (the serve command, on Ctrl-C).
func Shutdown(final tea.Model) {
	if m, ok := final.(Model); ok {
		m.restoreIsolationNow()
	}
}

// restoreIsolationNow puts the buses back before the program ends, so a pad is not left dry.
func (m Model) restoreIsolationNow() {
	if m.iso == nil || len(m.iso.was) == 0 || m.unit == nil {
		return
	}
	info, _ := proto.ParamByName("bus-fx")
	m.ops.Lock()
	defer m.ops.Unlock()
	for pad, bus := range m.iso.was {
		if err := m.unit.SetParam(info.ID, proto.TargetPad(pad), bus); err != nil {
			m.log.Warn("putting a bus back failed", "pad", pad.String(), "err", err)
		}
	}
}
