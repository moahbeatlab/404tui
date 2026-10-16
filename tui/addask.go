package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/moahbeatlab/404tui/proto"
)

// addAsk is the dialog a opens on the Slicer tab, which takes a pad or a new sample from the
// file view, and the list of patterns a opens in the Sequencer. The pads and the patterns are
// chosen in a list of their own, drawn over the tab.
type addAsk struct {
	// sec is the tab it was opened on.
	sec int
	// perf is 1 or 2 when the list is for the Perform view's pattern that plays or the next.
	perf int
	// pads is set once a pad is being chosen (Slicer), patterns once a pattern is (Sequencer).
	pads     *padChooser
	patterns *patternChooser
}

// chooser reports whether a pad or a pattern is being chosen, not yet what to choose.
func (a *addAsk) chooser() bool { return a.pads != nil || a.patterns != nil }

// padChooser is the 16 pads of a bank, laid out like the pads.
type padChooser struct{ bank, cursor int }

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
		return m.openPatternAsk()
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

// openPatternAsk opens the list of the patterns of a bank, to choose the one the Sequencer edits.
func (m Model) openPatternAsk() (tea.Model, tea.Cmd) {
	return m.openPatternAskFor(secSequencer, 0)
}

// openPatternAskFor opens the same list for the Sequencer (perf 0) or for the Perform view.
func (m Model) openPatternAskFor(sec, perf int) (tea.Model, tea.Cmd) {
	if m.unit == nil || m.snap == nil {
		m.message = "no unit to read patterns from"
		return m, nil
	}
	c := &patternChooser{bank: m.bank}
	if err := m.loadChooser(c); err != nil {
		m.message = "patterns: " + err.Error()
		return m, nil
	}
	m.addAsk = &addAsk{sec: sec, perf: perf, patterns: c}
	return m, m.wanted()
}

// addAskKey takes every key while the dialog is open.
func (m Model) addAskKey(name string) (tea.Model, tea.Cmd) {
	a := m.addAsk
	switch {
	case a.pads != nil:
		return m.padChooserKey(a, name)
	case a.patterns != nil:
		return m.patternChooserKey(a, name)
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
	case "p":
		if m.unit == nil || m.snap == nil {
			m.message = "no unit to read from"
			return m, nil
		}
		a.pads = &padChooser{bank: m.bank, cursor: m.pad}
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

// patternChooserKey moves over the patterns of a bank, with [ ] for another; Enter opens the
// one in front in the editor, on the Sequencer tab.
func (m Model) patternChooserKey(a *addAsk, name string) (tea.Model, tea.Cmd) {
	c := a.patterns
	if moved, note := m.chooserMove(c, name); moved {
		if note != "" {
			m.message = note
		}
		return m, m.wanted()
	}
	switch name {
	case "esc", "q", "a":
		m.addAsk = nil
	case "enter", " ":
		row := c.rows[c.cursor]
		if !row.Exists {
			m.message = "that pattern slot is empty"
			return m, nil
		}
		m.addAsk = nil
		if a.perf != 0 {
			slot := row.Slot
			if a.perf == 1 {
				m.perfNow = &slot
			} else {
				m.perfNext = &slot
			}
			return m, m.wanted()
		}
		return m.openSequencerPattern(row.Slot)
	}
	return m, nil
}

// openSequencerPattern opens a pattern in the editor, on the Sequencer tab.
func (m Model) openSequencerPattern(slot proto.PadIndex) (tea.Model, tea.Cmd) {
	m.section, m.seqTab, m.seqSlot = secPatterns, true, nil
	m.bank, m.patternCursor = int(slot.Bank()), int(slot.Pad())
	m.busy = "reading the pattern"
	return m, m.openPattern(slot, "")
}

// addAskBox draws the dialog: the choices, the pads, or the patterns.
func (m Model) addAskBox() string {
	st, a := m.st, m.addAsk
	switch {
	case a.pads != nil:
		return m.padChooserBox(a.pads)
	case a.patterns != nil:
		return m.patternChooserBox(a.patterns)
	}
	lines := []string{
		st.bold.Render("Slicer: choose what to slice"), "",
		st.accent.Render("p") + st.text.Render("  a pad  ") + st.dim.Render("one of the pads on the unit"),
		st.accent.Render("f") + st.text.Render("  a new sample  ") + st.dim.Render("from the file view"), "",
		st.dim.Render("Esc cancel"),
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

// patternChooserBox draws the slots of a bank as the pads lie, and under them the pattern in
// front, as the patterns panel shows it. The box has the same size whatever is in it.
func (m Model) patternChooserBox(c *patternChooser) string {
	st := m.st
	width := clamp(m.width*80/100, 60, 130)
	room := max(m.geom().bodyH-14, 6)
	lines := m.chooserGrid(c, nil, width-2, "   arrows move  [ ] bank  Enter open  Esc back")
	lines = append(lines, "")
	var preview []string
	if r := c.rows[c.cursor]; r.Exists {
		preview = m.patternPreviewLines(r.Slot, width-2, room)
	} else {
		preview = []string{st.faint.Render("empty slot")}
	}
	for len(preview) < room {
		preview = append(preview, "")
	}
	lines = append(lines, preview[:room]...)
	return st.modal.Width(width).Render(strings.Join(lines, "\n"))
}

// patternPreviewLines show a pattern's first page of steps: a heading, the ruler, and a row of
// notes for each track.
func (m Model) patternPreviewLines(slot proto.PadIndex, width, room int) []string {
	st := m.st
	d := m.patPreview[slot]
	if d == nil {
		if m.patUnread[slot] {
			return []string{st.faint.Render("pattern " + slot.String() + " cannot be shown")}
		}
		return []string{st.faint.Render("reading pattern " + slot.String() + "…")}
	}
	sp := spanFor(d, 0, width, 1, 4, gridGapW)
	tracks := len(d.Grid.Rows)
	shown := min(tracks, max(room-3, 1))
	head := st.bold.Render("Pattern "+slot.String()) +
		st.dim.Render(fmt.Sprintf("  %.2f BPM  ·  %d/4  ·  %s  ·  %s", d.BankTempo, d.Beats, plural(d.Bars(), "bar"), plural(len(d.PadsUsed), "pad")))
	lines := []string{head + "  " + m.spanHeader(d, sp), m.stepRuler(d, sp)}
	if tracks == 0 {
		lines = append(lines, st.dim.Render("  (no notes)"))
	}
	for _, row := range d.Grid.Rows[:shown] {
		name := m.snap.Pads[row.Pad.Index()].Name
		label := fmt.Sprintf("%-4s %-14s", row.Pad, ansi.Truncate(name, 14, "…"))
		lines = append(lines, m.rowLabel(row.Pad, label, st.bold)+" "+m.noteRow(d, row.Pad, false, sp))
	}
	if shown < tracks {
		lines = append(lines, st.faint.Render(fmt.Sprintf("  … and %s more", plural(tracks-shown, "track"))))
	}
	return lines
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
		key("a", "choose a pattern to edit"),
		key("N", "make a new pattern"),
	}
	return m.box("Sequencer", strings.Join(lines, "\n"), g.bodyW+g.bodyX-1, g.bodyH, true)
}
