package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/proto"
)

// The Perform view is for the stage: the names of the samples of the pattern that plays, as big as
// the screen allows, so that it is plain what there is to mute and bring back, and the next
// pattern under it with what changes. The unit does not say which pattern runs, so it is told:
// a chooses the one that plays, n the next, Space makes the next the one that plays.

func (m Model) perfKey(name string) (tea.Model, tea.Cmd) {
	switch name {
	case "a", "enter":
		return m.openPatternAskFor(secPerform, 1)
	case "n":
		return m.openPatternAskFor(secPerform, 2)
	case " ", "space":
		if m.perfNext == nil {
			m.message = "no next pattern: n chooses it"
			return m, nil
		}
		m.perfNow, m.perfNext = m.perfNext, nil
		m.logCue()
		return m, m.wanted()
	case "x":
		m.perfNext = nil
		m.logCue()
	case "v":
		m.perfPatterns = !m.perfPatterns
	case "f", "F":
		bus := 1
		if name == "F" {
			bus = 2
		}
		m.prompt = &prompt{kind: promptBusNote, busOf: bus, buffer: m.mem.BusNotes[bus-1]}
	}
	return m, nil
}

// perfBusLines say, for each bus, the effect the user noted for it (the unit does not tell) and
// which of the pattern's samples go to it, in the colour of the bus.
func (m Model) perfBusLines(tracks []perfTrack, width int) []string {
	st := m.st
	var out []string
	for bus := int32(1); bus <= 2; bus++ {
		colour, _ := st.th.busColour(bus)
		style := lipgloss.NewStyle().Foreground(colour).Bold(true)
		note := m.mem.BusNotes[bus-1]
		if note == "" {
			note = "—  (" + map[int32]string{1: "f", 2: "F"}[bus] + " notes the effect)"
		}
		var names []string
		for _, t := range tracks {
			if m.snap.Pads[t.pad.Index()].Params["bus-fx"] == bus {
				names = append(names, t.name)
			}
		}
		list := "nothing of this pattern"
		if len(names) > 0 {
			list = strings.Join(names, "  ·  ")
		}
		head := style.Render(fmt.Sprintf("BUS %d", bus)) + "  " + st.text.Render(note)
		out = append(out, head, "      "+st.dim.Render(ansi.Truncate(list, max(width-8, 10), "…")))
	}
	return out
}

// perfRecLine is the banner of a recording that runs, for the Perform view; empty when none does.
func (m Model) perfRecLine() string {
	if m.rec == nil {
		return ""
	}
	return lipgloss.NewStyle().Foreground(m.st.th.hot).Bold(true).Render("● RECORDING") +
		m.st.dim.Render("  "+display(m.rec.Path)+"   Ctrl-R stops it")
}

// perfPads are the pads that play in a pattern, in pad order, with their sample names.
type perfTrack struct {
	pad  proto.PadIndex
	name string
}

func (m Model) perfTracks(slot *proto.PadIndex) (*app.PatternDetail, []perfTrack) {
	if slot == nil {
		return nil, nil
	}
	d := m.patPreview[*slot]
	if d == nil {
		return nil, nil
	}
	pads := slices.Clone(d.PadsUsed)
	slices.SortFunc(pads, func(a, b proto.PadIndex) int { return int(a.Index()) - int(b.Index()) })
	var out []perfTrack
	for _, p := range pads {
		out = append(out, perfTrack{pad: p, name: m.snap.Pads[p.Index()].Name})
	}
	return d, out
}

// perfHead is the line over a pattern: which, how fast, how long, how many tracks.
func (m Model) perfHead(label string, slot *proto.PadIndex, d *app.PatternDetail, tracks []perfTrack, style lipgloss.Style) string {
	st := m.st
	if slot == nil {
		return style.Render(label) + "  " + st.dim.Render("—")
	}
	if d == nil {
		return style.Render(label) + "  " + st.bold.Render(slot.String()) + st.dim.Render("  reading…")
	}
	return style.Render(label) + "  " + st.bold.Render(slot.String()) +
		st.dim.Render(fmt.Sprintf("   %.2f BPM  ·  %s  ·  %s", d.BankTempo, plural(d.Bars(), "bar"), plural(len(tracks), "track")))
}

// perfBank is the bank the unit is on, as it last said, else the one on show.
func (m Model) perfBank() int {
	if m.hasSelected {
		return int(m.selected.Bank())
	}
	return m.bank
}

func (m Model) perfBody(g geom) string {
	if !m.perfPatterns {
		return m.perfBankBody(g)
	}
	return m.perfPatternBody(g)
}

// perfTile is one pad of the bank as a big tile: its number and length in bars, the name in block
// letters, and a bar under it that fills while it sounds. The pad the unit has selected is in the
// accent colour, the one that sounds in the hot one.
func (m Model) perfTile(row app.PadRow, w, h int) []string {
	st := m.st
	selected := m.hasSelected && m.selected == row.Pad
	hit := m.mon.hot(int(row.Pad.Bank()), int(row.Pad.Pad()), time.Now())
	sounding := m.playing && m.playingPad == row.Pad || hit
	bg, fg := st.th.tile, st.th.text
	switch {
	case !row.HasSample:
		bg, fg = st.th.tileEmpty, st.th.faint
	case sounding:
		bg, fg = st.th.hot, st.th.cursorFg
	case selected:
		bg, fg = st.th.accent, st.th.cursorFg
	}
	style := lipgloss.NewStyle().Background(bg).Foreground(fg).Bold(true)
	line := func(text string) string {
		pad := max(w-ansi.StringWidth(text), 0)
		return style.Render(text + strings.Repeat(" ", pad))
	}
	num := fmt.Sprint(row.Pad.Pad() + 1)
	head := " " + num
	if row.HasSample {
		if bars := m.padBars(row, 99); bars != "" {
			head += strings.Repeat(" ", max(w-len(head)-len(bars)-2, 1)) + bars
		}
		if bus := row.Params["bus-fx"]; bus == 1 || bus == 2 {
			colour, _ := st.th.busColour(bus)
			if len(head) < w-2 {
				head = lipgloss.NewStyle().Background(bg).Foreground(colour).Bold(true).Render(busBadge(bus)) + style.Render(" ") + style.Render(strings.TrimPrefix(head, " "))
			}
		}
	}
	out := []string{line(head)}
	body := h - 2
	switch {
	case !row.HasSample:
		for i := 0; i < body; i++ {
			out = append(out, line(""))
		}
	case body >= 3:
		name := strings.TrimSpace(row.Name)
		scale := 1
		if bigWidth(name, 2) <= w-2 {
			scale = 2
		}
		if maxChars := (w - 2 + scale) / (4 * scale); len([]rune(name)) > maxChars {
			name = string([]rune(name)[:max(maxChars, 1)])
		}
		bt := bigText(name, scale)
		// The name sits in the middle of the tile.
		above := (body - 3) / 2
		for i := 0; i < above; i++ {
			out = append(out, line(""))
		}
		for i := 0; i < 3; i++ {
			out = append(out, line(" "+bt[i]))
		}
		for i := above + 3; i < body; i++ {
			out = append(out, line(""))
		}
	default:
		out = append(out, line(" "+ansi.Truncate(row.Name, w-2, "…")))
	}
	// The bar under the name: how far the sounding sample has gone.
	bar := line("")
	if sounding {
		if f, ok := m.facts[factKey{row.Pad, row.Size, row.Name}]; ok && f.err == nil && f.facts.Frames > 0 {
			bar = style.Render(" " + levelBar(w-2, int(min(m.playPos, f.facts.Frames)), int(f.facts.Frames)) + " ")
		}
	}
	return append(out, bar)
}

// perfBankBody is the Perform view as it opens: the 16 pads of the bank the unit is on, as big as the
// screen allows, with a line over them for the bank and the cue, and one under for its patterns.
func (m Model) perfBankBody(g geom) string {
	st := m.st
	bank := m.perfBank()
	width := g.bodyW + g.bodyX - 3
	var top []string
	if rec := m.perfRecLine(); rec != "" {
		top = append(top, rec)
	}
	head := lipgloss.NewStyle().Foreground(st.th.accent).Bold(true).Render(fmt.Sprintf("BANK %c", 'A'+bank)) +
		st.dim.Render(fmt.Sprintf("   %.2f BPM", m.bankTempo(bank)))
	if m.playing {
		head += st.dim.Render("   ▶ " + m.playingPad.String() + " " + m.snap.Pads[m.playingPad.Index()].Name)
	}
	if bars := 0; true {
		if d, _ := m.perfTracks(m.perfNow); d != nil {
			bars = d.Bars()
		}
		if clock := m.clockText(bars); clock != "" {
			head += "   " + lipgloss.NewStyle().Foreground(st.th.good).Bold(true).Render("♩ "+clock)
		}
	}
	top = append(top, head, m.perfCue(width))
	// Patterns of the bank, under the pads.
	var bottom []string
	if rows := m.patterns[bank]; rows != nil {
		var parts []string
		for i, r := range rows {
			if r.Exists {
				parts = append(parts, fmt.Sprintf("%d·%s", i+1, plural(len(r.Pads), "pad")))
			}
		}
		line := "no patterns in this bank"
		if len(parts) > 0 {
			line = strings.Join(parts, "   ")
		}
		bottom = append(bottom, st.dim.Render("PATTERNS  ")+st.text.Render(ansi.Truncate(line, width-12, "…")))
	}
	bottom = append(bottom, m.perfMovement(width)...)
	bottom = append(bottom, st.dim.Render("a n  cue the patterns   Space  next becomes now   v  patterns big   f F  note a bus effect   Ctrl-R  record"))
	// The tiles take what is left.
	avail := g.bodyH - 2 - len(top) - len(bottom) - 2 - 3
	tileH := max(avail/4, 3)
	tileW := max((width-3*2)/4, 10)
	rows := m.snap.Bank(bank)
	var lines []string
	lines = append(lines, top...)
	lines = append(lines, "")
	for r := 0; r < 4; r++ {
		tiles := make([][]string, 4)
		for c := 0; c < 4; c++ {
			tiles[c] = m.perfTile(rows[r*4+c], tileW, tileH)
		}
		for l := 0; l < tileH; l++ {
			var b strings.Builder
			for c := 0; c < 4; c++ {
				b.WriteString(tiles[c][l])
				if c < 3 {
					b.WriteString("  ")
				}
			}
			lines = append(lines, b.String())
		}
		if r < 3 {
			lines = append(lines, "")
		}
	}
	lines = append(lines, "")
	lines = append(lines, bottom...)
	return m.box("Perform", strings.Join(lines, "\n"), g.bodyW+g.bodyX-1, g.bodyH, true)
}

// perfCue is one line about the cue: the pattern that plays, the next, and what changes.
func (m Model) perfCue(width int) string {
	st := m.st
	if m.perfNow == nil && m.perfNext == nil {
		return st.dim.Render("cue: a chooses the pattern that plays, n the next one (the unit does not say which runs)")
	}
	nowD, now := m.perfTracks(m.perfNow)
	nextD, next := m.perfTracks(m.perfNext)
	var b strings.Builder
	b.WriteString(st.accent.Render("NOW ") + st.bold.Render(slotText(m.perfNow)))
	if nowD != nil {
		b.WriteString(st.dim.Render(fmt.Sprintf(" %s", plural(len(now), "track"))))
	}
	if m.perfNext != nil {
		b.WriteString(lipgloss.NewStyle().Foreground(st.th.bus1).Bold(true).Render("   NEXT ") + st.bold.Render(slotText(m.perfNext)))
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
				b.WriteString(lipgloss.NewStyle().Foreground(st.th.good).Bold(true).Render("   IN ") + st.text.Render(strings.Join(on, ", ")))
			}
			if len(off) > 0 {
				b.WriteString(lipgloss.NewStyle().Foreground(st.th.hot).Bold(true).Render("   OUT ") + st.text.Render(strings.Join(off, ", ")))
			}
		}
	}
	return ansi.Truncate(b.String(), width, "…")
}

func slotText(p *proto.PadIndex) string {
	if p == nil {
		return "—"
	}
	return p.String()
}

// perfPatternBody is the other Perform view, v: the samples of the pattern that plays, big, and
// the next pattern's changes.
func (m Model) perfPatternBody(g geom) string {
	st := m.st
	nowD, now := m.perfTracks(m.perfNow)
	nextD, next := m.perfTracks(m.perfNext)
	inNow, inNext := map[proto.PadIndex]bool{}, map[proto.PadIndex]bool{}
	for _, t := range now {
		inNow[t.pad] = true
	}
	for _, t := range next {
		inNext[t.pad] = true
	}
	hasNext := m.perfNext != nil && nextD != nil
	width := g.bodyW + g.bodyX - 3
	cols := 1
	switch {
	case width >= 150:
		cols = 3
	case width >= 90:
		cols = 2
	}
	colW := width/cols - 2

	var lines []string
	if rec := m.perfRecLine(); rec != "" {
		lines = append(lines, rec, "")
	}
	lines = append(lines, m.perfHead("NOW", m.perfNow, nowD, now, st.accent), "")
	if m.perfNow == nil {
		lines = append(lines, st.dim.Render("a  chooses the pattern that is playing"), st.dim.Render("n  chooses the one that comes next"))
	}
	// A track that is not in the next pattern is marked: it is the one to mute.
	tag := func(pad proto.PadIndex) string {
		switch {
		case hasNext && !inNext[pad]:
			return lipgloss.NewStyle().Foreground(st.th.hot).Bold(true).Render("  ▼ OUT NEXT")
		case hasNext:
			return st.dim.Render("  stays")
		}
		return ""
	}
	rowsNeeded := (len(now) + cols - 1) / cols
	roomForBig := g.bodyH - 14
	big := len(now) > 0 && rowsNeeded*5 <= roomForBig
	for i := 0; i < len(now); i += cols {
		chunk := now[i:min(i+cols, len(now))]
		var cells [5][]string
		for _, t := range chunk {
			label := st.accent.Render(fmt.Sprintf("%-4s", t.pad)) + tag(t.pad)
			name := strings.TrimSpace(t.name)
			if big {
				scale := 1
				if bigWidth(name, 2) <= colW {
					scale = 2
				}
				maxChars := (colW + scale) / (4 * scale)
				if r := []rune(name); len(r) > maxChars {
					name = string(r[:max(maxChars, 1)])
				}
				bt := bigText(name, scale)
				cells[0] = append(cells[0], lipgloss.NewStyle().Width(colW+2).Render(label))
				for k := 0; k < 3; k++ {
					cells[k+1] = append(cells[k+1], lipgloss.NewStyle().Width(colW+2).Foreground(st.th.text).Bold(true).Render(bt[k]))
				}
				cells[4] = append(cells[4], strings.Repeat(" ", colW+2))
			} else {
				text := label + "  " + st.bold.Render(ansi.Truncate(name, max(colW-18, 8), "…"))
				cells[0] = append(cells[0], lipgloss.NewStyle().Width(colW+2).Render(text))
			}
		}
		for k := 0; k < 5; k++ {
			if len(cells[k]) > 0 {
				lines = append(lines, strings.Join(cells[k], ""))
			}
		}
	}
	lines = append(lines, "")
	lines = append(lines, m.perfHead("NEXT", m.perfNext, nextD, next, lipgloss.NewStyle().Foreground(st.th.bus1).Bold(true)))
	if hasNext {
		var on, off []string
		for _, t := range next {
			if !inNow[t.pad] {
				on = append(on, fmt.Sprintf("%s %s", t.pad, t.name))
			}
		}
		for _, t := range now {
			if !inNext[t.pad] {
				off = append(off, fmt.Sprintf("%s %s", t.pad, t.name))
			}
		}
		good := lipgloss.NewStyle().Foreground(st.th.good).Bold(true)
		bad := lipgloss.NewStyle().Foreground(st.th.hot).Bold(true)
		switch {
		case len(on) == 0 && len(off) == 0:
			lines = append(lines, st.dim.Render("the same pads play"))
		default:
			if len(on) > 0 {
				lines = append(lines, good.Render("BRING IN  ")+st.text.Render(ansi.Truncate(strings.Join(on, "  ·  "), width-12, "…")))
			}
			if len(off) > 0 {
				lines = append(lines, bad.Render("TAKE OUT  ")+st.text.Render(ansi.Truncate(strings.Join(off, "  ·  "), width-12, "…")))
			}
		}
		lines = append(lines, "")
		var names []string
		for _, t := range next {
			names = append(names, fmt.Sprintf("%s %s", t.pad, t.name))
		}
		lines = append(lines, st.dim.Render(ansi.Truncate(strings.Join(names, "  ·  "), width-2, "…")))
	}
	lines = append(lines, "")
	lines = append(lines, m.perfBusLines(now, width)...)
	lines = append(lines, "")
	lines = append(lines, m.perfMovement(width)...)
	lines = append(lines, "", st.dim.Render("a  the pattern that plays   n  the next one   Space  next becomes now   x  clear next   f F  note the effect of BUS 1, 2   Ctrl-R  record"))
	return m.box("Perform", strings.Join(lines, "\n"), g.bodyW+g.bodyX-1, g.bodyH, true)
}

// perfMovement shows what the unit sends as it happens: the controllers that were moved lately as
// bars, and the last messages. It keeps the same height whatever is going on, so the view does not
// jump.
func (m Model) perfMovement(width int) []string {
	st := m.st
	if m.midiSrc == nil {
		return []string{st.dim.Render("MOVING  no MIDI in this build: the unit's knobs and pads show here when it has MIDI (make build)")}
	}
	if m.midi == nil {
		return []string{st.dim.Render("MOVING  waiting for the unit's MIDI input (Config)")}
	}
	now := time.Now()
	moving := m.mon.moving(now)
	const barW = 24
	lines := make([]string, 0, 4)
	for i := 0; i < 3; i++ {
		head := st.dim.Render("MOVING  ")
		if i > 0 {
			head = strings.Repeat(" ", 8)
		}
		if i < len(moving) {
			c := moving[i]
			fade := st.accent
			if now.Sub(c.at) > 2*time.Second {
				fade = st.dim
			}
			lines = append(lines, head+st.text.Render(fmt.Sprintf("ch %-2d cc %-3d ", c.ch, c.cc))+fade.Render(levelBar(barW, c.val, 127))+st.text.Render(fmt.Sprintf(" %3d", c.val)))
		} else if i == 0 {
			lines = append(lines, head+st.dim.Render("turn a knob or hit a pad on the unit"))
		} else {
			lines = append(lines, "")
		}
	}
	last := m.mon.last(3)
	text := ""
	for _, l := range last {
		text += strings.Join(strings.Fields(l.text), " ") + "   "
	}
	lines = append(lines, st.dim.Render("LAST    ")+st.text.Render(ansi.Truncate(strings.TrimSpace(text), width-10, "…")))
	return lines
}
