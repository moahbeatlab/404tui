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

// bigFader is a controller as a tall bar rows high and three cells wide, its value under it and a
// label under that; a line of faderW cells each. style lights the bar, st.faint draws its track.
func (m Model) bigFader(val, rows int, style lipgloss.Style, valueText, label string) []string {
	st := m.st
	const blocks = " ▁▂▃▄▅▆▇█"
	center := func(s string) string {
		pad := max(faderW-ansi.StringWidth(s), 0)
		return strings.Repeat(" ", pad/2) + s + strings.Repeat(" ", pad-pad/2)
	}
	fill := val * rows * 8 / 127
	out := make([]string, 0, rows+2)
	for r := 0; r < rows; r++ {
		rem := clamp(fill-(rows-1-r)*8, 0, 8)
		cell := st.faint.Render("╎ ╎")
		if rem > 0 {
			ch := string([]rune(blocks)[rem])
			cell = style.Render(strings.Repeat(ch, 3))
		}
		out = append(out, "  "+cell+"  ")
	}
	out = append(out, style.Render(center(valueText)), st.dim.Render(center(ansi.Truncate(label, faderW, ""))))
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

// sideFx draws the effect of each bus side by side: its name, a big ON or off, what you noted for it,
// and its knobs as tall faders.
func (m Model) sideFx(width, rows int, now time.Time) []string {
	st := m.st
	half := max((width-2)/2, faderW*3)
	var cols [2][]string
	for bus := 0; bus < 2; bus++ {
		colour, _ := st.th.busColour(int32(bus + 1))
		badge := lipgloss.NewStyle().Foreground(colour).Bold(true).Render(fmt.Sprintf("BUS %d ", bus+1))
		state := st.dim.Render("?  " + map[int]string{0: "l", 1: "L"}[bus] + " learns")
		if _, _, _, ok := fxMsg(m.mem.FxMsg[bus]); ok {
			state = lipgloss.NewStyle().Background(st.th.faint).Foreground(st.th.text).Render("  off  ")
			if m.fxOn[bus] {
				state = lipgloss.NewStyle().Background(st.th.good).Foreground(st.th.cursorFg).Bold(true).Render("  ON   ")
			}
		}
		note := m.mem.BusNotes[bus]
		if note == "" {
			note = "—"
		}
		col := []string{badge + state, st.text.Render(ansi.Truncate(note, half, "…"))}
		knobs := m.mem.FxKnobs[bus]
		cells := make([][]string, 0, 3)
		for _, text := range knobs {
			var ch, cc int
			fmt.Sscanf(text, "ch %d cc %d", &ch, &cc)
			c := m.mon.ccs[[2]int{ch, cc}]
			val, style, vt := 0, st.faint, "-"
			if c != nil {
				val, vt = c.val, fmt.Sprint(c.val)
				style = st.dim
				if now.Sub(c.at) < monMoving {
					style = st.accent
				}
			}
			cells = append(cells, m.bigFader(val, rows, style, vt, fmt.Sprintf("%d:%d", ch, cc)))
		}
		if len(cells) == 0 {
			key := map[int]string{0: "k", 1: "K"}[bus]
			col = append(col, st.dim.Render(key+" assigns a knob"))
		}
		for r := 0; r < rows+2 && len(cells) > 0; r++ {
			var b strings.Builder
			for _, c := range cells {
				b.WriteString(c[r])
			}
			col = append(col, b.String())
		}
		cols[bus] = col
	}
	height := max(len(cols[0]), len(cols[1]))
	out := make([]string, 0, height)
	for i := 0; i < height; i++ {
		var l, r string
		if i < len(cols[0]) {
			l = cols[0][i]
		}
		if i < len(cols[1]) {
			r = cols[1][i]
		}
		out = append(out, l+strings.Repeat(" ", max(half-ansi.StringWidth(l), 0))+r)
	}
	return out
}

// sideControls draws the other controllers as tall faders in a row.
func (m Model) sideControls(width, rows int, now time.Time) []string {
	st := m.st
	var all []*ccState
	for _, c := range m.mon.controls() {
		if !m.isFxKnob(c.ch, c.cc) {
			all = append(all, c)
		}
	}
	head := st.bold.Render("CONTROLS")
	if len(all) == 0 {
		return []string{head, st.dim.Render("turn a knob or move a fader"), st.dim.Render("on the unit or its MIDI in")}
	}
	n := max(width/faderW, 1)
	shown := all
	if len(shown) > n {
		shown = shown[:n]
		head += st.dim.Render(fmt.Sprintf("  +%d more", len(all)-n))
	}
	cells := make([][]string, 0, len(shown))
	for _, c := range shown {
		style := st.dim
		if now.Sub(c.at) < monMoving {
			style = st.accent
		}
		cells = append(cells, m.bigFader(c.val, rows, style, fmt.Sprint(c.val), fmt.Sprintf("%d:%d", c.ch, c.cc)))
	}
	out := []string{head}
	for r := 0; r < rows+2; r++ {
		var b strings.Builder
		for _, c := range cells {
			b.WriteString(c[r])
		}
		out = append(out, b.String())
	}
	return out
}

// perfSidebar is the whole left side, h lines high and width cells wide.
func (m Model) perfSidebar(width, h int) []string {
	st := m.st
	now := time.Now()
	bank := m.perfBank()
	rows := clamp((h-30)/2, 3, 7)
	var out []string
	add := func(l ...string) { out = append(out, l...) }
	if rec := m.perfRecLine(); rec != "" {
		add(rec)
	}
	bars := 0
	if d, _ := m.perfTracks(m.perfNow); d != nil {
		bars = d.Bars()
	}
	head := lipgloss.NewStyle().Foreground(st.th.accent).Bold(true).Render(fmt.Sprintf("BANK %c", 'A'+bank)) +
		st.dim.Render(fmt.Sprintf("   %.2f BPM", m.bankTempo(bank)))
	add(head)
	if lines := m.levelLines(width); lines != nil {
		// While recording: how loud it is, and the clip light.
		add(lines...)
		add("")
	}
	if clock := m.clockText(bars); clock != "" {
		add(lipgloss.NewStyle().Foreground(st.th.good).Bold(true).Render("♩ " + clock))
	}
	add(m.perfBeats()...)
	add("")
	add(m.sideCue()...)
	add("")
	add(m.sidePatterns(bank)...)
	add("")
	if m.midiSrc == nil {
		add(st.dim.Render("no MIDI in this build: effects, knobs and the clock need it (make build)"))
	} else {
		if m.fxLearn != 0 || m.fxKnob != 0 {
			add(m.perfFxHead(width))
		}
		add(m.sideFx(width, rows, now)...)
		add("")
		add(m.sideControls(width, rows, now)...)
		if last := m.mon.last(2); len(last) > 0 {
			var parts []string
			for _, l := range last {
				parts = append(parts, strings.Join(strings.Fields(l.text), " "))
			}
			add(st.dim.Render("LAST ") + st.text.Render(strings.Join(parts, " · ")))
		}
	}
	add("")
	add(st.dim.Render("a n  cue   Space  next→now"), st.dim.Render("v  patterns   p  player"), st.dim.Render("f F  effect note   l L  learn on/off"), st.dim.Render("k K  its knobs   ^R  record"))
	for i := range out {
		out[i] = ansi.Truncate(out[i], width, "…")
	}
	return out
}
