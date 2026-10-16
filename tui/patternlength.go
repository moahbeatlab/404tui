package tui

import (
	"fmt"
	"math"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
)

// lengthPick is the question of how many bars a new pattern is, for the template at row. The
// longest sample of the bank is measured against the bank's tempo, so a loop that is four bars
// long suggests four.
type lengthPick struct {
	row, bars int
	// loop is the longest sample of the bank, its length in seconds, and what that is in bars at
	// the bank's tempo; zero when the bank has no sample to measure.
	loop    string
	seconds float64
	bpm     float64
}

// loopBars is how many bars of 4/4 a length of audio is at a tempo.
func loopBars(seconds, bpm float64) float64 {
	if bpm <= 0 {
		return 0
	}
	return seconds * bpm / 240
}

// askPatternLength opens the question, with the template's own length in front, or the length
// of the longest sample of the bank when that is a loop of a few bars.
func (m Model) askPatternLength(row, bars int) (tea.Model, tea.Cmd) {
	p := &lengthPick{row: row, bars: bars, bpm: m.bankTempo(m.bank)}
	m.ops.Lock()
	for _, pad := range m.bankSampledPads() {
		facts, err := app.ReadPadFacts(m.unit, m.snap.Pads[pad.Index()])
		if err == nil && facts.Seconds > p.seconds {
			p.seconds, p.loop = facts.Seconds, m.snap.Pads[pad.Index()].Name
		}
	}
	m.ops.Unlock()
	if n := int(math.Round(loopBars(p.seconds, p.bpm))); n > 1 && n <= formats.MaxStartBars && math.Abs(loopBars(p.seconds, p.bpm)-float64(n)) < 0.15 {
		p.bars = n
	}
	m.lenPick = p
	return m, nil
}

func (m Model) lengthPickKey(name string) (tea.Model, tea.Cmd) {
	p := m.lenPick
	switch name {
	case "esc", "q":
		m.lenPick = nil
		m.tmplPicker = &templatePicker{cursor: p.row}
	case "h", "left", "-":
		p.bars = max(p.bars-1, 1)
	case "l", "right", "+", "=":
		p.bars = min(p.bars+1, formats.MaxStartBars)
	case "enter", " ":
		m.lenPick = nil
		return m.buildPattern(p.row, p.bars)
	default:
		if len(name) == 1 && name[0] >= '1' && name[0] <= '0'+byte(formats.MaxStartBars) {
			p.bars = int(name[0] - '0')
		}
	}
	return m, nil
}

func (m Model) lengthPickBox() string {
	st, p := m.st, m.lenPick
	title := "A pattern from every sampled pad"
	switch {
	case p.row == 1:
		title = "A blank pattern"
	case p.row > 1:
		if g, ok := grooveAt(p.row); ok {
			title = g.Name
		}
	}
	lines := []string{st.bold.Render("How long is the pattern?") + st.dim.Render("   "+title), ""}
	var opts []string
	for n := 1; n <= formats.MaxStartBars; n++ {
		cell := fmt.Sprintf(" %d ", n)
		if n == p.bars {
			opts = append(opts, lipgloss.NewStyle().Background(st.th.cursorBg).Foreground(st.th.cursorFg).Bold(true).Render(cell))
		} else {
			opts = append(opts, st.text.Render(cell))
		}
	}
	lines = append(lines, strings.Join(opts, " ")+st.dim.Render("  bars"))
	if p.bpm > 0 {
		lines = append(lines, "", st.dim.Render(fmt.Sprintf("%s at %.2f BPM is %.1f s", plural(p.bars, "bar"), p.bpm, float64(p.bars)*240/p.bpm)))
	}
	if p.loop != "" {
		lines = append(lines, st.dim.Render(fmt.Sprintf("the longest sample here, %s, is %.2f s: about %.1f bars", p.loop, p.seconds, loopBars(p.seconds, p.bpm))))
	}
	lines = append(lines, "", st.dim.Render("h l or 1-8 change  ·  Enter make it  ·  Esc back"))
	return st.modal.Render(strings.Join(lines, "\n"))
}
