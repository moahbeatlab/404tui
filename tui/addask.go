package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// addAsk is the dialog a opens on the Slicer and Sequencer tabs: a sample or a pattern. The
// sample goes through the file dialogs that are there already, the pattern through a chooser
// laid out like the pads.
type addAsk struct {
	// sec is the tab it was opened on.
	sec int
	// chooser is set once p was pressed: the slots of a bank to pick a pattern from.
	chooser *patternChooser
}

// slicerTabKey takes the keys of the Slicer tab, where nothing is open yet.
func (m Model) slicerTabKey(name string) (tea.Model, tea.Cmd) {
	switch name {
	case "a", "enter":
		m.addAsk = &addAsk{sec: secSlicer}
	}
	return m, nil
}

// addAskKey takes every key while the dialog is open.
func (m Model) addAskKey(name string) (tea.Model, tea.Cmd) {
	a := m.addAsk
	if c := a.chooser; c != nil {
		return m.addChooserKey(a, c, name)
	}
	switch name {
	case "esc", "q", "a":
		m.addAsk = nil
	case "f":
		if a.sec == secSlicer {
			// A new sample for the slicer is chosen on the Files screen: Z slices the file in front.
			m.addAsk, m.sliceWant = nil, true
			m.message = "choose an audio file and press Z"
			return m.goTo(secFiles)
		}
	case "s":
		if a.sec == secPatterns {
			// A sample for the sequencer goes onto a pad, in the file dialog that a opens on Pads.
			m.addAsk = nil
			return m.openAssign(m.padRow().Pad)
		}
	case "p":
		if a.sec == secSlicer {
			// A pad's sample for the slicer: Z on the pad.
			m.addAsk, m.sliceWant = nil, true
			m.message = "choose a pad and press Z"
			return m.goTo(secPads)
		}
		if m.unit == nil || m.snap == nil {
			m.message = "no unit to read patterns from"
			return m, nil
		}
		c := &patternChooser{bank: m.bank}
		if err := m.loadChooser(c); err != nil {
			m.message = "patterns: " + err.Error()
			return m, nil
		}
		a.chooser = c
	}
	return m, nil
}

// addChooserKey moves in the pattern chooser; Enter opens the pattern in the sequencer.
func (m Model) addChooserKey(a *addAsk, c *patternChooser, name string) (tea.Model, tea.Cmd) {
	if moved, note := m.chooserMove(c, name); moved {
		if note != "" {
			m.message = note
		}
		return m, nil
	}
	switch name {
	case "esc", "q", "p":
		a.chooser = nil
	case "enter":
		row := c.rows[c.cursor]
		if !row.Exists {
			m.message = "that pattern slot is empty"
			return m, nil
		}
		m.addAsk = nil
		m.bank, m.patternCursor = int(row.Slot.Bank()), int(row.Slot.Pad())
		m.busy = "reading the pattern"
		return m, m.openPattern(row.Slot, "")
	}
	return m, nil
}

// addAskBox draws the dialog: the two choices, or the patterns to pick from.
func (m Model) addAskBox() string {
	st, a := m.st, m.addAsk
	if a.chooser != nil {
		keys := "   arrows move  [ ] bank  Enter take it  Esc back"
		lines := m.chooserGrid(a.chooser, nil, 56, keys)
		return st.modal.Render(strings.Join(lines, "\n"))
	}
	lines := []string{
		st.bold.Render("Slicer: choose what to slice"), "",
		st.accent.Render("p") + st.text.Render("  a pad  ") + st.dim.Render("one of the pads on the unit"),
		st.accent.Render("f") + st.text.Render("  a new sample  ") + st.dim.Render("from the file view"), "",
		st.dim.Render("Esc cancel"),
	}
	if a.sec == secPatterns {
		lines = []string{
			st.bold.Render("Sequencer: add a sample or a pattern"), "",
			st.accent.Render("s") + st.text.Render("  a sample  ") + st.dim.Render("put it on a pad"),
			st.accent.Render("p") + st.text.Render("  a pattern  ") + st.dim.Render("open it to edit"), "",
			st.dim.Render("Esc cancel"),
		}
	}
	return st.modal.Render(strings.Join(lines, "\n"))
}

// slicerTabBody is the Slicer tab while no slicer is open: what to do to open one.
func (m Model) slicerTabBody(g geom) string {
	st := m.st
	key := func(k, what string) string { return st.accent.Render(fmt.Sprintf("%-8s", k)) + st.dim.Render(what) }
	lines := []string{
		st.dim.Render("Nothing is open in the slicer."), "",
		key("a", "choose a pad, or a new sample from the file view"),
	}
	return m.box("Slicer", strings.Join(lines, "\n"), g.bodyW+g.bodyX-1, g.bodyH, true)
}
