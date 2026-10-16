package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/proto"
)

// addAsk is the dialog a opens on the Slicer and Sequencer tabs. The Slicer takes a pad or a
// new sample from the file view, the Sequencer a pattern or a sample for a pad. The pads and
// the patterns are chosen in a list of their own, drawn over the tab.
type addAsk struct {
	// sec is the tab it was opened on.
	sec int
	// pads is set once a pad is being chosen (Slicer), list once a pattern is (Sequencer).
	pads *padChooser
	list *patternList
}

// chooser reports whether a pad or a pattern is being chosen, not yet what to choose.
func (a *addAsk) chooser() bool { return a.pads != nil || a.list != nil }

// padChooser is the 16 pads of a bank, laid out like the pads.
type padChooser struct{ bank, cursor int }

// patternList is every pattern on the unit, in the order of the banks.
type patternList struct {
	rows   []app.PatternRow
	cursor int
}

// slicerTabKey takes the keys of the Slicer tab, where nothing is open yet.
func (m Model) slicerTabKey(name string) (tea.Model, tea.Cmd) {
	switch name {
	case "a", "enter":
		m.addAsk = &addAsk{sec: secSlicer}
	}
	return m, nil
}

// sequencerTabKey takes the keys of the Sequencer tab, where no pattern is open yet.
func (m Model) sequencerTabKey(name string) (tea.Model, tea.Cmd) {
	switch name {
	case "a", "enter":
		m.addAsk = &addAsk{sec: secSequencer}
	case "N":
		// A new pattern opens in the editor, which is the Sequencer's.
		m.section, m.seqTab = secPatterns, true
		next, cmd := m.patternsKey(name)
		nm := next.(Model)
		if nm.tmplPicker == nil && nm.pattern == nil && nm.busy == "" {
			nm.section, nm.seqTab = secSequencer, false
		}
		return nm, cmd
	}
	return m, nil
}

// addAllPatterns reads the patterns of every bank (the ones read before are not read again).
func (m Model) allPatterns() ([]app.PatternRow, error) {
	var out []app.PatternRow
	for b := 0; b < proto.Banks; b++ {
		rows := m.patterns[b]
		if rows == nil {
			var err error
			m.ops.Lock()
			rows, err = app.LoadPatterns(m.unit, b)
			m.ops.Unlock()
			if err != nil {
				return nil, err
			}
		}
		for _, r := range rows {
			if r.Exists {
				out = append(out, r)
			}
		}
	}
	return out, nil
}

// addAskKey takes every key while the dialog is open.
func (m Model) addAskKey(name string) (tea.Model, tea.Cmd) {
	a := m.addAsk
	switch {
	case a.pads != nil:
		return m.padChooserKey(a, name)
	case a.list != nil:
		return m.patternListKey(a, name)
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
		if a.sec == secSequencer {
			// A sample for the sequencer goes onto a pad, in the file dialog that a opens on Pads.
			m.addAsk = nil
			return m.openAssign(m.padRow().Pad)
		}
	case "p":
		if m.unit == nil || m.snap == nil {
			m.message = "no unit to read from"
			return m, nil
		}
		if a.sec == secSlicer {
			a.pads = &padChooser{bank: m.bank, cursor: m.pad}
			return m, nil
		}
		rows, err := m.allPatterns()
		if err != nil {
			m.message = "patterns: " + err.Error()
			return m, nil
		}
		if len(rows) == 0 {
			m.message = "there is no pattern on the unit yet: N makes one"
			return m, nil
		}
		a.list = &patternList{rows: rows}
	}
	return m, nil
}

// padChooserKey moves over the pads; Enter opens the slicer on the pad in front.
func (m Model) padChooserKey(a *addAsk, name string) (tea.Model, tea.Cmd) {
	c := a.pads
	switch name {
	case "esc", "q", "p":
		a.pads = nil
	case "h", "left":
		c.cursor = clamp(c.cursor-1, 0, proto.PadsPerBank-1)
	case "l", "right":
		c.cursor = clamp(c.cursor+1, 0, proto.PadsPerBank-1)
	case "k", "up":
		c.cursor = clamp(c.cursor-4, 0, proto.PadsPerBank-1)
	case "j", "down":
		c.cursor = clamp(c.cursor+4, 0, proto.PadsPerBank-1)
	case "[":
		c.bank = (c.bank + proto.Banks - 1) % proto.Banks
	case "]":
		c.bank = (c.bank + 1) % proto.Banks
	case "enter", " ":
		row := m.snap.Bank(c.bank)[c.cursor]
		if !row.HasSample {
			m.message = fmt.Sprintf("%s is empty", row.Pad)
			return m, nil
		}
		m.addAsk = nil
		return m.openSlicerPad(row)
	}
	return m, nil
}

// patternListKey moves over the patterns; Enter opens the one in front in the editor, on the
// Sequencer tab.
func (m Model) patternListKey(a *addAsk, name string) (tea.Model, tea.Cmd) {
	l := a.list
	page := max(min(m.geom().bodyH-12, 20), 4)
	switch name {
	case "esc", "q", "p":
		a.list = nil
	case "j", "down":
		l.cursor = clamp(l.cursor+1, 0, len(l.rows)-1)
	case "k", "up":
		l.cursor = clamp(l.cursor-1, 0, len(l.rows)-1)
	case "g":
		l.cursor = 0
	case "G":
		l.cursor = len(l.rows) - 1
	case "ctrl+d", "pgdown":
		l.cursor = clamp(l.cursor+page, 0, len(l.rows)-1)
	case "ctrl+u", "pgup":
		l.cursor = clamp(l.cursor-page, 0, len(l.rows)-1)
	case "enter", " ":
		row := l.rows[l.cursor]
		m.addAsk = nil
		m.section, m.seqTab = secPatterns, true
		m.bank, m.patternCursor = int(row.Slot.Bank()), int(row.Slot.Pad())
		m.busy = "reading the pattern"
		return m, m.openPattern(row.Slot, "")
	}
	return m, nil
}

// addAskBox draws the dialog: the choices, the pads, or the patterns.
func (m Model) addAskBox() string {
	st, a := m.st, m.addAsk
	switch {
	case a.pads != nil:
		return m.padChooserBox(a.pads)
	case a.list != nil:
		return m.patternListBox(a.list)
	}
	lines := []string{
		st.bold.Render("Slicer: choose what to slice"), "",
		st.accent.Render("p") + st.text.Render("  a pad  ") + st.dim.Render("one of the pads on the unit"),
		st.accent.Render("f") + st.text.Render("  a new sample  ") + st.dim.Render("from the file view"), "",
		st.dim.Render("Esc cancel"),
	}
	if a.sec == secSequencer {
		lines = []string{
			st.bold.Render("Sequencer: choose what to work on"), "",
			st.accent.Render("p") + st.text.Render("  a pattern  ") + st.dim.Render("open it to edit"),
			st.accent.Render("s") + st.text.Render("  a sample  ") + st.dim.Render("put it on a pad"), "",
			st.dim.Render("Esc cancel"),
		}
	}
	return st.modal.Render(strings.Join(lines, "\n"))
}

// padChooserBox draws the 16 pads of the bank as they lie, with their names.
func (m Model) padChooserBox(c *padChooser) string {
	st := m.st
	lines := []string{st.bold.Render(fmt.Sprintf("Pad  bank %c", 'A'+c.bank)) + st.dim.Render("   arrows move  [ ] bank  Enter slice it  Esc back")}
	cell := 18
	rows := m.snap.Bank(c.bank)
	for r := 0; r < 4; r++ {
		var b strings.Builder
		for col := 0; col < 4; col++ {
			i := r*4 + col
			text := fmt.Sprintf("%2d ·", i+1)
			if rows[i].HasSample {
				text = fmt.Sprintf("%2d %s", i+1, rows[i].Name)
			}
			text = ansi.Truncate(text, cell-1, "…")
			text += strings.Repeat(" ", max(cell-ansi.StringWidth(text), 0))
			switch {
			case i == c.cursor:
				text = lipgloss.NewStyle().Background(st.th.accent).Foreground(st.th.cursorFg).Bold(true).Render(text)
			case rows[i].HasSample:
				text = st.text.Render(text)
			default:
				text = st.faint.Render(text)
			}
			b.WriteString(text)
		}
		lines = append(lines, b.String())
	}
	return st.modal.Render(strings.Join(lines, "\n"))
}

// patternListBox draws every pattern, a page of them, the one in front lit. The box is as tall
// as the page of the list whatever there is to show.
func (m Model) patternListBox(l *patternList) string {
	st := m.st
	height := max(min(m.geom().bodyH-12, 20), 4)
	width := clamp(m.width*50/100, 62, 90)
	lines := []string{st.bold.Render(fmt.Sprintf("Pattern  %d on the unit", len(l.rows))) + st.dim.Render("   ↑ ↓ move  Enter open  Esc"), ""}
	from, to := window(len(l.rows), l.cursor, height)
	for i := from; i < from+height; i++ {
		if i >= to {
			lines = append(lines, "")
			continue
		}
		r := l.rows[i]
		var pads []string
		for _, p := range r.Pads {
			pads = append(pads, p.String())
		}
		text := fmt.Sprintf(" %-4s %2s  %s", r.Slot.String(), fmt.Sprintf("%d", len(r.Pads)), strings.Join(pads, " "))
		text = ansi.Truncate(text, width-2, "…")
		text += strings.Repeat(" ", max(width-2-ansi.StringWidth(text), 0))
		if i == l.cursor {
			text = lipgloss.NewStyle().Background(st.th.accent).Foreground(st.th.cursorFg).Bold(true).Render(text)
		} else {
			text = st.text.Render(text)
		}
		lines = append(lines, text)
	}
	return st.modal.Width(width).Render(strings.Join(lines, "\n"))
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

// sequencerTabBody is the Sequencer tab while no pattern is open.
func (m Model) sequencerTabBody(g geom) string {
	st := m.st
	key := func(k, what string) string { return st.accent.Render(fmt.Sprintf("%-8s", k)) + st.dim.Render(what) }
	lines := []string{
		st.dim.Render("Nothing is open in the sequencer."), "",
		key("a", "choose a pattern to edit, or a sample to put on a pad"),
		key("N", "make a new pattern"),
	}
	return m.box("Sequencer", strings.Join(lines, "\n"), g.bodyW+g.bodyX-1, g.bodyH, true)
}
