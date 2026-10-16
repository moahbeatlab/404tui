package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/moahbeatlab/404tui/proto"
)

// The left side of the Perform view: the things to read from across a stage, drawn large: the beat
// of the bar as four big blocks, the pattern cue, the patterns of the bank, each bus's effect with
// its state and its knobs as tall faders, and the other controllers.

const faderW = 7

// faderCell is how wide a fader is in a panel inner cells wide that holds n of them: wider when there is room.
func faderCell(inner, n int) int { return clamp(inner/max(n, 1), faderW, 15) }

// bigFader is a controller as a tall bar rows high and three cells wide, its value under it, a
// label and a small line under that; a line of faderW cells each. style lights the bar, st.faint draws its track.
func (m Model) bigFader(val, rows int, style lipgloss.Style, valueText, label, sub string, cw int) []string {
	st := m.st
	const blocks = " ▁▂▃▄▅▆▇█"
	center := func(s string) string {
		pad := max(cw-ansi.StringWidth(s), 0)
		return strings.Repeat(" ", pad/2) + s + strings.Repeat(" ", pad-pad/2)
	}
	fill := val * rows * 8 / 127
	out := make([]string, 0, rows+3)
	for r := 0; r < rows; r++ {
		rem := clamp(fill-(rows-1-r)*8, 0, 8)
		bar := clamp(cw-4, 3, 9)
		cell := st.faint.Render("╎" + strings.Repeat(" ", bar-2) + "╎")
		if rem > 0 {
			ch := string([]rune(blocks)[rem])
			cell = style.Render(strings.Repeat(ch, bar))
		}
		lead := (cw - bar) / 2
		out = append(out, strings.Repeat(" ", lead)+cell+strings.Repeat(" ", cw-bar-lead))
	}
	out = append(out, style.Render(center(valueText)), style.Render(center(ansi.Truncate(label, cw, ""))), st.faint.Render(center(ansi.Truncate(sub, cw, ""))))
	return out
}

// perfBeats draws the beat of the bar as four big blocks: the one that is now lit, the ones gone by
// dimmer. Three lines of 4*7+3 cells.
func (m Model) perfBeats() []string {
	st := m.st
	c := m.clock
	if c == nil || time.Since(c.at) > 2*time.Second || !c.running {
		return []string{st.dim.Render("no clock yet: it starts with the unit's sequencer"), "", ""}
	}
	pos := max(c.ticks-1, 0)
	beat := pos / 24 % 4
	var rows [3]strings.Builder
	for i := 0; i < 4; i++ {
		var style lipgloss.Style
		glyph := "░"
		switch {
		case i == beat && i == 0:
			style, glyph = lipgloss.NewStyle().Foreground(st.th.accent).Bold(true), "█"
		case i == beat:
			style, glyph = lipgloss.NewStyle().Foreground(st.th.hot).Bold(true), "█"
		case i < beat:
			style, glyph = st.good, "▓"
		default:
			style = st.faint
		}
		for r := 0; r < 3; r++ {
			cell := strings.Repeat(glyph, 7)
			if r == 1 && i == beat {
				// The number of the beat, knocked out of the block.
				cell = strings.Repeat(glyph, 3) + fmt.Sprint(i+1) + strings.Repeat(glyph, 3)
			}
			if i > 0 {
				rows[r].WriteString(" ")
			}
			rows[r].WriteString(style.Render(cell))
		}
	}
	return []string{rows[0].String(), rows[1].String(), rows[2].String()}
}

// sideCue is the cue as lines: what plays, what comes next, and what comes in and goes out.
func (m Model) sideCue() []string {
	st := m.st
	if m.perfNow == nil && m.perfNext == nil {
		return []string{st.dim.Render("a  the pattern that plays"), st.dim.Render("n  the next one")}
	}
	nowD, now := m.perfTracks(m.perfNow)
	nextD, next := m.perfTracks(m.perfNext)
	var out []string
	line := st.accent.Render("NOW  ") + st.bold.Render(slotText(m.perfNow))
	if m.perfAuto {
		line += st.dim.Render("  heard")
	}
	if nowD != nil {
		line += st.dim.Render("  " + plural(len(now), "track"))
	}
	out = append(out, line)
	if m.perfNext == nil {
		return out
	}
	out = append(out, lipgloss.NewStyle().Foreground(st.th.bus1).Bold(true).Render("NEXT ")+st.bold.Render(slotText(m.perfNext)))
	if nextD != nil && nowD != nil {
		inNow, inNext := map[proto.PadIndex]bool{}, map[proto.PadIndex]bool{}
		for _, t := range now {
			inNow[t.pad] = true
		}
		for _, t := range next {
			inNext[t.pad] = true
		}
		var on, off []string
		for _, t := range next {
			if !inNow[t.pad] {
				on = append(on, t.name)
			}
		}
		for _, t := range now {
			if !inNext[t.pad] {
				off = append(off, t.name)
			}
		}
		if len(on) > 0 {
			out = append(out, lipgloss.NewStyle().Foreground(st.th.good).Bold(true).Render("IN   ")+st.text.Render(strings.Join(on, ", ")))
		}
		if len(off) > 0 {
			out = append(out, lipgloss.NewStyle().Foreground(st.th.hot).Bold(true).Render("OUT  ")+st.text.Render(strings.Join(off, ", ")))
		}
	}
	return out
}

// sidePatterns lists the patterns of a bank, at most four, the one that plays and the next first.
func (m Model) sidePatterns(bank int) []string {
	st := m.st
	rows := m.patterns[bank]
	head := st.bold.Render(fmt.Sprintf("PATTERNS · bank %c", 'A'+bank))
	if rows == nil {
		return []string{head, st.dim.Render("reading…")}
	}
	var have []int
	for i, r := range rows {
		if r.Exists {
			have = append(have, i)
		}
	}
	if len(have) == 0 {
		return []string{head, st.dim.Render("none in this bank")}
	}
	is := func(slot proto.PadIndex, p *proto.PadIndex) bool { return p != nil && *p == slot }
	// The ones that matter first, then the others in order, four in all, shown in slot order.
	var pick []int
	for _, i := range have {
		if is(rows[i].Slot, m.perfNow) || is(rows[i].Slot, m.perfNext) {
			pick = append(pick, i)
		}
	}
	for _, i := range have {
		if len(pick) >= 4 {
			break
		}
		if !is(rows[i].Slot, m.perfNow) && !is(rows[i].Slot, m.perfNext) {
			pick = append(pick, i)
		}
	}
	for a := 0; a < len(pick); a++ {
		for b := a + 1; b < len(pick); b++ {
			if pick[b] < pick[a] {
				pick[a], pick[b] = pick[b], pick[a]
			}
		}
	}
	out := []string{head}
	for _, i := range pick {
		r := rows[i]
		mark, style := "  ", st.text
		switch {
		case is(r.Slot, m.perfNow):
			mark, style = st.accent.Render("▶ "), st.accent
		case is(r.Slot, m.perfNext):
			mark, style = lipgloss.NewStyle().Foreground(st.th.bus1).Bold(true).Render("▷ "), lipgloss.NewStyle().Foreground(st.th.bus1)
		}
		out = append(out, mark+style.Render(fmt.Sprintf("%-4s", r.Slot))+st.dim.Render(plural(len(r.Pads), "pad")))
	}
	if more := len(have) - len(pick); more > 0 {
		out = append(out, st.dim.Render(fmt.Sprintf("+%d more (a or n lists them all)", more)))
	}
	return out
}

// panel draws lines in a rounded box w wide with its title in the top edge, in colour.
func (m Model) panel(title string, colour lipgloss.Color, lines []string, w int) []string {
	border := lipgloss.NewStyle().Foreground(colour)
	inner := w - 4
	label := " " + title + " "
	top := border.Render("╭─") + lipgloss.NewStyle().Foreground(colour).Bold(true).Render(label) +
		border.Render(strings.Repeat("─", max(w-3-ansi.StringWidth(label), 0))+"╮")
	out := []string{top}
	for _, l := range lines {
		l = ansi.Truncate(l, inner, "…")
		out = append(out, border.Render("│ ")+l+strings.Repeat(" ", max(inner-ansi.StringWidth(l), 0))+border.Render(" │"))
	}
	return append(out, border.Render("╰"+strings.Repeat("─", w-2)+"╯"))
}

// besideEach puts two blocks of lines side by side, each padded to its width and the shorter one
// to the height of the taller.
func besideEach(a []string, wa int, b []string, wb int) []string {
	n := max(len(a), len(b))
	out := make([]string, n)
	for i := range out {
		var l, r string
		if i < len(a) {
			l = a[i]
		}
		if i < len(b) {
			r = b[i]
		}
		out[i] = l + strings.Repeat(" ", max(wa-ansi.StringWidth(l), 0)) + r + strings.Repeat(" ", max(wb-ansi.StringWidth(r), 0))
	}
	return out
}

// padLines adds blank lines to a block until it is n lines high.
func padLines(lines []string, n int) []string {
	for len(lines) < n {
		lines = append(lines, "")
	}
	return lines
}

// fresh is how long a controller stays lit bright after it moved.
const fresh = 1500 * time.Millisecond

// faderRow lays out faders side by side, rows+3 lines high.
func faderRow(cells [][]string) []string {
	if len(cells) == 0 {
		return nil
	}
	out := make([]string, len(cells[0]))
	for _, c := range cells {
		for r := range out {
			out[r] += c[r]
		}
	}
	return out
}

// fxPanel is a bus's effect: whether it is on, what you noted for it, and its three knobs, C1 to C3,
// as tall faders that light up in the bus colour while you turn them.
func (m Model) fxPanel(bus, w, rows int, now time.Time) (string, lipgloss.Color, []string) {
	cw := faderCell(w-4, 3)
	st := m.st
	colour, _ := st.th.busColour(int32(bus + 1))
	bold := lipgloss.NewStyle().Foreground(colour).Bold(true)
	state := st.dim.Render("?  " + map[int]string{0: "l", 1: "L"}[bus] + " learns its on/off")
	if _, _, _, ok := fxMsg(m.mem.FxMsg[bus]); ok {
		state = lipgloss.NewStyle().Background(st.th.faint).Foreground(st.th.text).Render("  off  ")
		if m.fxOn[bus] {
			state = lipgloss.NewStyle().Background(st.th.good).Foreground(st.th.cursorFg).Bold(true).Render("  ON   ")
		}
	}
	note := m.mem.BusNotes[bus]
	if note == "" {
		note = "no note (f F)"
	}
	lines := []string{state + "  " + st.text.Render(note)}
	cells := make([][]string, 0, 3)
	for k := 0; k < 3; k++ {
		label := fmt.Sprintf("C%d", k+1)
		if k >= len(m.fxKnobList(bus)) {
			cells = append(cells, m.bigFader(0, rows, st.faint, "-", label, "", cw))
			continue
		}
		var ch, cc int
		fmt.Sscanf(m.fxKnobList(bus)[k], "ch %d cc %d", &ch, &cc)
		val, style, vt := 0, st.faint, "-"
		if c := m.ccOf(ch, cc); c != nil {
			val, vt, style = c.val, fmt.Sprint(c.val), st.dim
			if now.Sub(c.at) < fresh {
				style = bold
			}
		}
		cells = append(cells, m.bigFader(val, rows, style, vt, label, fmt.Sprintf("%d:%d", ch, cc), cw))
	}
	lines = append(lines, "")
	lines = append(lines, faderRow(cells)...)
	if len(m.mem.FxKnobs[bus]) == 0 {
		lines = append(lines, st.dim.Render(fmt.Sprintf("ch %d cc 16-18   %s learns other knobs", bus+1, map[int]string{0: "k", 1: "K"}[bus])))
	}
	return fmt.Sprintf("BUS %d", bus+1), colour, lines
}

// controlsPanel draws the other controllers as tall faders.
func (m Model) controlsPanel(w, rows int, now time.Time) (string, lipgloss.Color, []string) {
	st := m.st
	var all []*ccState
	for _, c := range m.mon.controls() {
		if !m.isFxKnob(c.ch, c.cc) && !m.isFxSwitch(c.ch, c.cc) {
			all = append(all, c)
		}
	}
	if len(all) == 0 {
		return "CONTROLS", st.th.accent, []string{st.dim.Render("turn a knob or move a fader"), st.dim.Render("on the unit or its MIDI in")}
	}
	cw := 9
	n := max((w-4)/cw, 1)
	title := "CONTROLS"
	if len(all) > n {
		title += fmt.Sprintf("  +%d more", len(all)-n)
		all = all[:n]
	}
	cells := make([][]string, 0, len(all))
	for _, c := range all {
		style := st.dim
		if now.Sub(c.at) < fresh {
			style = st.accent
		}
		cells = append(cells, m.bigFader(c.val, rows, style, fmt.Sprint(c.val), fmt.Sprintf("%d:%d", c.ch, c.cc), "", cw))
	}
	return title, st.th.accent, faderRow(cells)
}

// perfSidebar is the whole left side, h lines high and width cells wide: panels for the clock, the
// cue, the patterns, each bus's effect with its knobs, the other controllers, what came last and the
// keys. From 76 cells wide two panels sit side by side.
func (m Model) perfSidebar(width, h int) []string {
	st := m.st
	now := time.Now()
	bank := m.perfBank()
	// Clock and cue need room side by side; the buses and the controllers fit from a narrower side.
	wideTop, wide := width >= 76, width >= 52
	half := width / 2
	// The faders take the height that is left: the panels around them need about 30 lines side by
	// side, and a recording adds its own.
	spare, per := h-31, 2
	if !wideTop {
		spare -= 4
	}
	if !wide {
		spare, per = h-51, 3
	}
	if m.rec != nil {
		spare -= 6
	}
	rows := clamp(spare/per, 3, 14)
	var out []string
	add := func(l ...string) { out = append(out, l...) }
	if rec := m.perfRecLine(); rec != "" {
		add(rec)
	}
	if lines := m.levelLines(width - 4); lines != nil {
		add(m.panel("RECORDING", st.th.hot, lines, width)...)
	}
	bars := 0
	if d, _ := m.perfTracks(m.perfNow); d != nil {
		bars = d.Bars()
	}
	pw, pt := width, width
	if wide {
		pw = half
	}
	if wideTop {
		pt = half
	}
	clock := []string{lipgloss.NewStyle().Foreground(st.th.accent).Bold(true).Render(fmt.Sprintf("BANK %c", 'A'+bank)) +
		st.dim.Render(fmt.Sprintf("   %.2f BPM", m.bankTempo(bank)))}
	if c := m.clockText(bars); c != "" {
		clock = append(clock, lipgloss.NewStyle().Foreground(st.th.good).Bold(true).Render("♩ "+c))
	} else {
		clock = append(clock, "")
	}
	clock = append(clock, m.perfBeats()...)
	cue := m.sideCue()
	add(m.pairPanels(wideTop, pt, "CLOCK", st.th.accent, clock, width-pt*b2i(wideTop), "CUE", st.th.bus1, cue)...)
	pats := m.sidePatterns(bank)
	add(m.panel(fmt.Sprintf("PATTERNS · bank %c", 'A'+bank), st.th.good, pats[1:], width)...)
	if m.midiSrc == nil {
		add(st.dim.Render("no MIDI in this build: effects, knobs and the clock need it (make build)"))
	} else {
		if m.fxLearn != 0 || m.fxKnob != 0 {
			add(m.perfFxHead(width))
		}
		w2 := width - pw*b2i(wide)
		ta, ca, la := m.fxPanel(0, pw, rows, now)
		tb, cb, lb := m.fxPanel(1, w2, rows, now)
		add(m.pairPanels(wide, pw, ta, ca, la, w2, tb, cb, lb)...)
		tc, cc, lc := m.controlsPanel(pw, rows, now)
		var last []string
		for _, l := range m.mon.last(max(rows, 3)) {
			last = append(last, st.text.Render(strings.Join(strings.Fields(l.text), " ")))
		}
		if len(last) == 0 {
			last = []string{st.dim.Render("nothing yet")}
		}
		add(m.pairPanels(wide, pw, tc, cc, lc, w2, "LAST MIDI", st.th.faint, last)...)
	}
	add(m.panel("KEYS", st.th.faint, []string{
		st.dim.Render("a n  cue   Space  next→now   v  patterns   p  player"),
		st.dim.Render("f F  effect note   l L  learn on/off"),
		st.dim.Render("k K  its knobs   ^R  record"),
	}, width)...)
	if len(out) > h {
		out = out[:h]
	}
	return out
}

func (m Model) pairPanels(wide bool, wa int, ta string, ca lipgloss.Color, la []string, wb int, tb string, cb lipgloss.Color, lb []string) []string {
	if !wide {
		return append(m.panel(ta, ca, la, wa), m.panel(tb, cb, lb, wb)...)
	}
	n := max(len(la), len(lb))
	return besideEach(m.panel(ta, ca, padLines(la, n), wa), wa, m.panel(tb, cb, padLines(lb, n), wb), wb)
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
