package tui

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/midiin"
	"github.com/moahbeatlab/404tui/perflog"
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
		m.perfNow, m.perfNext, m.perfAuto = m.perfNext, nil, false
		if m.mon != nil {
			m.mon.played, m.mon.playedAt = map[int]bool{}, map[int]int{} // the new pattern starts with nothing heard
		}
		m.logCue()
		return m, m.wanted()
	case "i":
		return m.isolateButton()
	case "esc":
		if m.iso != nil {
			return m.isolateOff()
		}
	case "x":
		m.perfNext = nil
		m.logCue()
	case "p":
		m.perfPlayer = !m.perfPlayer
	case "s":
		if c := m.clock; c != nil && time.Since(c.at) < 2*time.Second {
			m.perfSync = max(c.ticks-1, 0)
			m.message = "the pattern starts now: the playhead is back at its beginning"
		} else {
			m.message = "no clock to follow: the unit sends it when it plays"
		}
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
	text := lipgloss.NewStyle().Foreground(m.st.th.hot).Bold(true).Render("● RECORDING") +
		m.st.dim.Render("  "+display(m.rec.Path)+"   Ctrl-R stops it")
	if m.lvl != nil && m.lvl.ok {
		text += "   " + m.clipBadge(time.Now()) + m.st.text.Render(" "+strings.TrimSpace(dbText(math.Max(m.lvl.hold[0], m.lvl.hold[1]))))
	}
	return text
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
	heard := ""
	if m.perfAuto && label == "NOW" {
		heard = "  (heard from the unit)"
	}
	return style.Render(label) + "  " + st.bold.Render(slot.String()) + st.dim.Render(heard) +
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
	if m.vc != nil {
		m.vc.hit = perfHit{}
	}
	if m.perfPlayer {
		return m.perfPlayerBody(g)
	}
	return m.perfBankBody(g)
}

// perfTile is a pad of the Pads view, only bigger: the same tile (number, length in bars, bus badge,
// the unit's selected pad, the settings and the level bar) with the name in block letters between
// them. A pad hit on the unit lights it by how hard it was hit and fades.
func (m Model) perfTile(row app.PadRow, w, h int) []string {
	ts := tileState{
		empty:    !row.HasSample,
		bus:      row.Params["bus-fx"],
		flags:    padFlags(row),
		muted:    m.padMuted(row.Pad),
		solo:     m.soloed[row.Pad],
		cursor:   m.hasSelected && m.selected == row.Pad,
		playing:  m.playing && m.playingPad == row.Pad,
		selected: m.hasSelected && m.selected == row.Pad,
		bars:     m.padBars(row, w),
		glow:     m.mon.glow(int(row.Pad.Bank()), int(row.Pad.Pad()), time.Now()),
	}
	ts.used, ts.inPattern = m.padUsed(row.Pad)
	if ts.empty {
		// A pad without a sample makes no sound: the pattern playing it is nothing to see.
		ts.glow = 0
	}
	// The tile as Pads draws it at four rows: number, name, settings, level.
	parts := strings.Split(m.tile(w, 4, fmt.Sprint(row.Pad.Pad()+1), row.Name, row.Level, ts), "\n")
	if h <= 4 || ts.empty && h <= 4 {
		return parts
	}
	bg, fg := m.tileColours(ts)
	style := lipgloss.NewStyle().Background(bg).Foreground(fg).Bold(true)
	line := func(text string) string {
		return style.Render(text + strings.Repeat(" ", max(w-ansi.StringWidth(text), 0)))
	}
	body := h - 3
	mid := make([]string, body)
	for i := range mid {
		mid[i] = line("")
	}
	if !ts.empty {
		// The whole name, in the terminal's own bold text, broken after _ - or a space when it is
		// longer than the tile, and in the middle of the tile.
		lines := wrapName(strings.TrimSpace(row.Name), w-2, body)
		above := (body - len(lines)) / 2
		for i, l := range lines {
			mid[above+i] = line(" " + l)
		}
	}
	out := []string{parts[0]}
	out = append(out, mid...)
	return append(out, parts[2], parts[3])
}

// perfBankBody is the Perform view as it opens: the 16 pads of the bank the unit is on, as big as the
// screen allows, with a line over them for the bank and the cue, and one under for its patterns.
func (m Model) perfBankBody(g geom) string {
	st := m.st
	bank := m.perfBank()
	width := g.bodyW + g.bodyX - 3
	if width >= 110 {
		return m.perfSplitBody(g, width)
	}
	var top []string
	if rec := m.perfRecLine(); rec != "" {
		top = append(top, rec)
	}
	head := lipgloss.NewStyle().Foreground(st.th.accent).Bold(true).Render(fmt.Sprintf("BANK %c", 'A'+bank)) +
		st.dim.Render(fmt.Sprintf("   %.2f BPM", m.bankTempo(bank)))
	if m.playing {
		head += st.dim.Render("   ▶ " + m.playingPad.String() + " " + m.snap.Pads[m.playingPad.Index()].Name)
	}
	bars := 0
	if d, _ := m.perfTracks(m.perfNow); d != nil {
		bars = d.Bars()
	}
	if clock := m.clockText(bars); clock != "" {
		head += "   " + lipgloss.NewStyle().Foreground(st.th.good).Bold(true).Render("♩ "+clock)
	}
	top = append(top, head, m.perfTicker(bars), m.perfCue(width))
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
	bottom = append(bottom, m.perfFxLines(width)...)
	bottom = append(bottom, m.perfMovement(width)...)
	bottom = append(bottom, st.dim.Render("a n cue   Space next→now   v patterns   p player   f F note the effect   l L learn its on/off   ^R rec"))
	// The tiles take what is left.
	avail := g.bodyH - 2 - len(top) - len(bottom) - 2 - 3
	tileH := max(avail/4, 3)
	tileW := max((width-3)/4, 10)
	rows := m.snap.Bank(bank)
	var lines []string
	lines = append(lines, top...)
	lines = append(lines, "")
	// The grid of the Pads view, with the same gaps, only bigger.
	lines = append(lines, m.grid(g, func(p int) string {
		return strings.Join(m.perfTile(rows[p], tileW, tileH), "\n")
	}))
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
	lines := m.perfControls(now, width)
	last := m.mon.last(3)
	text := ""
	for _, l := range last {
		text += strings.Join(strings.Fields(l.text), " ") + "   "
	}
	lines = append(lines, st.dim.Render("LAST    ")+st.text.Render(ansi.Truncate(strings.TrimSpace(text), width-10, "…")))
	return lines
}

// knobGlyph is a controller as a dial that fills as it is turned up.
func knobGlyph(v int) string {
	switch f := float64(v) / 127; {
	case f < 0.06:
		return "○"
	case f < 0.37:
		return "◔"
	case f < 0.63:
		return "◑"
	case f < 0.94:
		return "◕"
	}
	return "●"
}

// perfControls draws the controllers the unit (or anything on its MIDI) has sent as a row of strips,
// each a fader beside a dial with its value and its channel and number. It is always three lines
// high. Those moved in the last seconds are in the accent colour, the others dimmed.
func (m Model) perfControls(now time.Time, width int) []string {
	st := m.st
	var all []*ccState
	for _, c := range m.mon.controls() {
		if !m.isFxKnob(c.ch, c.cc) {
			all = append(all, c)
		}
	}
	if len(all) == 0 {
		return []string{
			st.dim.Render("CONTROLS  turn a knob or move a fader on the unit (or the controller on its MIDI in):"),
			st.dim.Render("          each one that sends a controller shows here, with its fader, dial and number"),
			"",
		}
	}
	const cellW = 10
	n := max(min((width-10)/cellW, 16), 1)
	shown := all
	if len(shown) > n {
		shown = shown[:n]
	}
	var out [3]string
	for _, c := range shown {
		cell := m.controlCell(c, fmt.Sprintf("%d:%d", c.ch, c.cc), now)
		for k := 0; k < 3; k++ {
			out[k] += cell[k]
		}
	}
	if len(all) > len(shown) {
		out[2] += st.dim.Render(fmt.Sprintf("+%d more", len(all)-len(shown)))
	}
	return []string{st.dim.Render("CONTROLS ") + out[0], strings.Repeat(" ", 9) + out[1], strings.Repeat(" ", 9) + out[2]}
}

// isFxKnob says whether a controller is one of the knobs shown under a bus's effect.
func (m Model) isFxKnob(ch, cc int) bool {
	text := fmt.Sprintf("ch %d cc %d", ch, cc)
	return slices.Contains(m.fxKnobList(0), text) || slices.Contains(m.fxKnobList(1), text)
}

// fxKnobList is the controllers of a bus's effect (bus 0 or 1): the unit's CTRL 1 to 3 send CC 16 to 18 on channel 1 for bus 1 and channel 2 for bus 2.
func (m Model) fxKnobList(bus int) []string {
	return []string{fmt.Sprintf("ch %d cc 16", bus+1), fmt.Sprintf("ch %d cc 17", bus+1), fmt.Sprintf("ch %d cc 18", bus+1)}
}

// ccOf is what the controller on a channel last sent, nil before it sent anything.
func (m Model) ccOf(ch, cc int) *ccState {
	if m.mon == nil {
		return nil
	}
	return m.mon.ccs[[2]int{ch, cc}]
}

// isFxSwitch is whether the controller is what says a bus's effect is on or off, or which it is.
func (m Model) isFxSwitch(ch, cc int) bool {
	return (ch == 1 || ch == 2) && (cc == ccFxSwitch || cc == ccFxNumber)
}

// controlCell is a controller as three lines ten cells wide: a fader beside a dial, its value, and
// a label. One moved lately is in the accent colour, the others dimmed; c is nil for one that has
// not sent anything yet.
func (m Model) controlCell(c *ccState, label string, now time.Time) [3]string {
	st := m.st
	const cellW = 10
	const blocks = " ▁▂▃▄▅▆▇█"
	val, style := 0, st.faint
	if c != nil {
		val, style = c.val, st.dim
		if now.Sub(c.at) < monMoving {
			style = st.accent
			if now.Sub(c.at) > 2*time.Second {
				style = st.text
			}
		}
	}
	level := val * 24 / 127
	var fader [3]string
	for k := 0; k < 3; k++ {
		cnt := clamp(level-8*(2-k), 0, 8)
		if cnt == 0 {
			fader[k] = st.faint.Render("╎")
		} else {
			fader[k] = style.Render(string([]rune(blocks)[cnt]))
		}
	}
	value := "  -"
	if c != nil {
		value = fmt.Sprintf("%3d", val)
	}
	mid := [3]string{style.Render("(" + knobGlyph(val) + ")"), style.Render(value), st.dim.Render(label)}
	var out [3]string
	for k := 0; k < 3; k++ {
		cell := fader[k] + " " + mid[k]
		out[k] = cell + strings.Repeat(" ", max(cellW-ansi.StringWidth(cell), 0))
	}
	return out
}

// nowBars is the length in bars of a pattern, 0 when it is not known.
func nowBars(d *app.PatternDetail) int {
	if d == nil {
		return 0
	}
	return d.Bars()
}

// perfTicker is the line that ticks with the MIDI clock: the sixteen steps of the bar fill up as the
// bar goes by, the one that is now is lit, and the first of each beat is marked. With the length of
// the pattern that plays it also shows where the loop is. Every clock tick redraws the screen, so it
// keeps time with the unit.
func (m Model) perfTicker(loopBars int) string {
	st := m.st
	c := m.clock
	if c == nil || time.Since(c.at) > 2*time.Second {
		return st.dim.Render("TICK  no clock: the unit sends it when it plays (its MIDI sync setting must send clock)")
	}
	pos := max(c.ticks-1, 0)
	step, beat := pos/6%16, pos/24%4
	hotStyle := lipgloss.NewStyle().Foreground(st.th.hot).Bold(true)
	var b strings.Builder
	b.WriteString(st.dim.Render("TICK  "))
	for i := 0; i < 16; i++ {
		if i > 0 && i%4 == 0 {
			b.WriteString(" ")
		}
		switch {
		case i == step && c.running:
			b.WriteString(hotStyle.Render("██"))
		case i < step && c.running:
			b.WriteString(st.good.Render("▐▌"))
		case i%4 == 0:
			b.WriteString(st.dim.Render("▐▌"))
		default:
			b.WriteString(st.faint.Render("▐▌"))
		}
	}
	text := fmt.Sprintf("   BEAT %d", beat+1)
	if loopBars > 0 {
		inLoop := pos / 96 % loopBars
		frac := (pos % (96 * loopBars)) * 100 / (96 * loopBars)
		text += fmt.Sprintf("   LOOP BAR %d/%d ", inLoop+1, loopBars)
		b.WriteString(st.text.Render(text) + st.accent.Render(levelBar(16, frac, 100)))
		return b.String()
	}
	b.WriteString(st.text.Render(text))
	return b.String()
}

// busFx is what the unit last said of a bus's effect.
type busFx struct {
	onSeen, on bool
	numSeen    bool
	num        int
}

// fxEvent takes a message from the unit about the effect of a bus: on channel 1 (bus 1) or 2 (bus 2)
// CC 19 switches it and CC 83 says which effect it is.
func (m Model) fxEvent(e midiin.Event) Model {
	if e.Kind != midiin.CC || e.Channel < 1 || e.Channel > 2 {
		return m
	}
	bus := e.Channel - 1
	f := m.fx[bus]
	switch e.Data1 {
	case ccFxSwitch:
		f.on, f.onSeen = e.Data2 >= 64, true
	case ccFxNumber:
		f.num, f.numSeen = e.Data2, true
	default:
		return m
	}
	if f != m.fx[bus] {
		ps := []perflog.Pair{perflog.P("bus", bus+1), perflog.P("on", f.on)}
		if f.numSeen {
			ps = append(ps, perflog.P("fx", f.num), perflog.P("name", fxName(f.num)))
		}
		m.perfLog.Add(e.At, "fx", ps...)
	}
	m.fx[bus] = f
	return m
}

// fxState is a bus's effect for the screen: whether it is on, and its name when the unit has said.
func (m Model) fxState(bus int) (state, name string) {
	st := m.st
	f := m.fx[bus]
	switch {
	case !f.onSeen:
		state = st.dim.Render("?")
	case f.on:
		state = lipgloss.NewStyle().Foreground(st.th.good).Bold(true).Render("● ON")
	default:
		state = st.faint.Render("○ off")
	}
	if f.numSeen {
		name = fxName(f.num)
	}
	return state, name
}

// perfFxHead is the line with the effect of each bus: on or off, its name, and what you noted for it.
func (m Model) perfFxHead(width int) string {
	st := m.st
	text := st.dim.Render("FX  ")
	for bus := 1; bus <= 2; bus++ {
		colour, _ := st.th.busColour(int32(bus))
		style := lipgloss.NewStyle().Foreground(colour).Bold(true)
		state, name := m.fxState(bus - 1)
		note := m.mem.BusNotes[bus-1]
		text += style.Render(fmt.Sprintf("BUS %d ", bus)) + state + st.text.Render("  "+strings.TrimSpace(name+"  "+note))
		if bus == 1 {
			text += "     "
		}
	}
	return ansi.Truncate(text, width, "…")
}

// perfFxLines is the effects: a line for each bus (on or off, and what you noted for it), and under it
// the knobs learned for each as faders and dials. Narrow screens get the line alone.
func (m Model) perfFxLines(width int) []string {
	head := m.perfFxHead(width)
	if width < 80 {
		return []string{head}
	}
	st := m.st
	now := time.Now()
	half := max((width-4)/2, 34)
	var rows [3]string
	for bus := 0; bus < 2; bus++ {
		var col [3]string
		knobs := m.fxKnobList(bus)
		for _, text := range knobs {
			var ch, cc int
			fmt.Sscanf(text, "ch %d cc %d", &ch, &cc)
			cell := m.controlCell(m.ccOf(ch, cc), fmt.Sprintf("%d:%d", ch, cc), now)
			for k := 0; k < 3; k++ {
				col[k] += cell[k]
			}
		}
		if len(knobs) == 0 {
			key := map[int]string{0: "k", 1: "K"}[bus]
			col[0] = st.dim.Render(fmt.Sprintf("%s  assigns a knob of BUS %d's effect", key, bus+1))
		}
		for k := 0; k < 3; k++ {
			rows[k] += col[k] + strings.Repeat(" ", max(half-ansi.StringWidth(col[k]), 0))
		}
	}
	return []string{head, rows[0], rows[1], rows[2]}
}

// patternTick is where the playhead is in the pattern d, in its own ticks, from the MIDI clock: the
// clock counts 24 to a quarter note and the pattern 480. It says false when no clock is running.
func (m Model) patternTick(d *app.PatternDetail) (uint32, bool) {
	c := m.clock
	if c == nil || !c.running || time.Since(c.at) > 2*time.Second || d == nil || d.Length == 0 {
		return 0, false
	}
	pos := max(c.ticks-1-m.perfSync, 0)
	return uint32(pos*(formats.PPQ/24)) % d.Length, true
}

// perfPlayerBody is the pattern that plays as the Sequencer draws it: a row for every track with its
// notes, over the bars the playhead is in, and the playhead above them moving with the unit's clock.
func (m Model) perfPlayerBody(g geom) string {
	st := m.st
	width := g.bodyW + g.bodyX - 3
	var lines []string
	if rec := m.perfRecLine(); rec != "" {
		lines = append(lines, rec)
	}
	d, tracks := m.perfTracks(m.perfNow)
	bank := m.perfBank()
	head := lipgloss.NewStyle().Foreground(st.th.accent).Bold(true).Render(fmt.Sprintf("BANK %c", 'A'+bank)) +
		st.dim.Render(fmt.Sprintf("   %.2f BPM", m.bankTempo(bank)))
	if clock := m.clockText(nowBars(d)); clock != "" {
		head += "   " + lipgloss.NewStyle().Foreground(st.th.good).Bold(true).Render("♩ "+clock)
	}
	lines = append(lines, head, m.perfTicker(nowBars(d)), "")
	switch {
	case m.perfNow == nil:
		lines = append(lines, st.dim.Render("a  chooses the pattern that is playing: the unit does not say which one runs"))
	case d == nil:
		lines = append(lines, st.bold.Render(m.perfNow.String())+st.dim.Render("  reading…"))
	default:
		tick, running := m.patternTick(d)
		cur := -1
		if running {
			cur = min(int(uint64(tick)*uint64(d.Grid.Steps)/uint64(d.Length)), d.Grid.Steps-1)
		}
		// The bars the playhead is in; with no clock, the first.
		page := 0
		if cur >= 0 {
			page = cur / d.StepsPerPage()
		}
		sp := spanFor(d, page, width, 1, 4, gridGapW)
		lines = append(lines, m.perfHead("NOW", m.perfNow, d, tracks, st.accent), st.bold.Render(spanLabel(d, sp)), "")
		// The playhead: a mark over the step that is playing, on the same places as the notes.
		var ph strings.Builder
		ph.WriteString(strings.Repeat(" ", 21))
		hot := lipgloss.NewStyle().Foreground(st.th.hot).Bold(true)
		for c := sp.lo; c < sp.hi; c++ {
			cell := strings.Repeat(" ", max(sp.w, 1))
			if c == cur {
				cell = hot.Render("▼") + strings.Repeat(" ", max(sp.w, 1)-1)
			}
			ph.WriteString(cell)
			if c+1 < sp.hi {
				ph.WriteString(strings.Map(func(r rune) rune {
					if r == '│' {
						return ' '
					}
					return r
				}, sp.gap(c-sp.lo)))
			}
		}
		lines = append(lines, ph.String())
		for _, row := range d.Grid.Rows {
			name := m.snap.Pads[row.Pad.Index()].Name
			label := fmt.Sprintf("%-4s %-14s", row.Pad, ansi.Truncate(name, 14, "…"))
			style := st.bold
			if !slices.Contains(row.Hits, true) {
				style = st.dim
			}
			if m.unitMuted(d, row.Pad, time.Now()) {
				// The label says it, where the long notes of the row cannot push it out of the box.
				label = fmt.Sprintf("%-4s %-8s MUTED", row.Pad, ansi.Truncate(name, 8, "…"))
				style = lipgloss.NewStyle().Foreground(st.th.bad).Bold(true)
			}
			lines = append(lines, m.rowLabel(row.Pad, label, style)+" "+m.noteRow(d, row.Pad, false, sp))
		}
		if !running {
			lines = append(lines, "", st.dim.Render("no clock: the playhead follows the unit's MIDI clock while it plays"))
		}
	}
	lines = append(lines, "", m.perfFxLines(width)[0])
	lines = append(lines, st.dim.Render("tap anywhere or p  back to the pads   s  the pattern starts now   a n  cue   Space  next→now   ^R  record"))
	return m.box("Perform · player", strings.Join(lines, "\n"), g.bodyW+g.bodyX-1, g.bodyH, true)
}

// wrapName breaks a pad name into at most max lines of w cells: after an underscore, hyphen or space
// where it can, anywhere when a piece is longer than a line. What does not fit the last line is cut
// with an ellipsis, so the name is whole whenever the tile has room for it.
func wrapName(name string, w, max int) []string {
	w = maxInt(w, 4)
	var pieces []string
	cur := ""
	for _, r := range name {
		cur += string(r)
		if r == '_' || r == '-' || r == ' ' {
			pieces = append(pieces, cur)
			cur = ""
		}
	}
	if cur != "" {
		pieces = append(pieces, cur)
	}
	var lines []string
	line := ""
	flush := func() {
		if line != "" {
			lines = append(lines, strings.TrimRight(line, " "))
		}
		line = ""
	}
	for _, p := range pieces {
		for ansi.StringWidth(p) > w {
			// A piece longer than a line is cut where the line ends.
			flush()
			head := ansi.Truncate(p, w, "")
			lines = append(lines, head)
			p = strings.TrimPrefix(p, head)
		}
		if ansi.StringWidth(line)+ansi.StringWidth(p) > w {
			flush()
		}
		line += p
	}
	flush()
	if len(lines) > max {
		lines = lines[:max]
		lines[max-1] = ansi.Truncate(lines[max-1]+"…", w, "…")
	}
	return lines
}

// perfSplitBody is the Perform view on a wide screen: the left half for what is read from afar (the
// beat, the cue, the patterns of the bank, the effects and the knobs), the pads on the right.
func (m Model) perfSplitBody(g geom, width int) string {
	bank := m.perfBank()
	side := max(width*40/100, 44)
	gridW := width - side - 2
	inner := g.bodyH - 2
	tileW := max((gridW-3)/4, 10)
	tileH := max((inner-3)/4, 3)
	rows := m.snap.Bank(bank)
	if m.vc != nil {
		m.vc.hit = perfHit{ok: true, gx: 1 + side + 2, gy: 1, tw: tileW, th: tileH, sideW: side}
	}
	grid := m.grid(g, func(p int) string {
		return strings.Join(m.perfTile(rows[p], tileW, tileH), "\n")
	})
	left := lipgloss.NewStyle().Width(side).Render(strings.Join(m.perfSidebar(side, inner), "\n"))
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", grid)
	return m.box("Perform", body, g.bodyW+g.bodyX-1, g.bodyH, true)
}

// padUsed says whether a pad is one of the pattern that plays and was heard in it lately (it stays lit,
// where the glow of a hit is gone in a moment), or is in the pattern and not heard yet. A pad that is
// not in the pattern is neither, however lately it was heard: it was the last pattern's.
func (m Model) padUsed(pad proto.PadIndex) (heard, inPattern bool) {
	if m.perfNow == nil {
		return false, false
	}
	d := m.patPreview[*m.perfNow]
	if d == nil || !slices.Contains(d.PadsUsed, pad) {
		return false, false
	}
	if m.mon != nil && m.clock != nil {
		// Heard within a round of the pattern, and a bar over for a pattern that is late.
		window := (d.Bars() + 1) * ticksPerBar
		heard = m.mon.heardSince(int(pad.Bank())*16+int(pad.Pad()), m.clock.ticks, window)
	}
	return heard, !heard
}

// clickPerform handles a click in the wide Perform view: on a pad, or on the ISOLATE panel.
func (m Model) clickPerform(x, y int) (tea.Model, tea.Cmd) {
	if m.vc == nil || !m.vc.hit.ok {
		return m, nil
	}
	h := m.vc.hit
	for _, b := range h.btns {
		if y >= b.y && y < b.y+max(b.h, 1) && x >= b.x && x < b.x+b.w {
			return m.perfButton(b, x)
		}
	}
	x, y = x-h.gx, y-h.gy
	if x < 0 || y < 0 || x%(h.tw+1) >= h.tw || y%(h.th+1) >= h.th {
		return m, nil
	}
	col, row := x/(h.tw+1), y/(h.th+1)
	if col > 3 || row > 3 {
		return m, nil
	}
	return m.clickPad(m.snap.Bank(m.perfBank())[row*4+col].Pad)
}

// perfButton does what the button that was clicked is for; x is where in the line it was.
func (m Model) perfButton(b hitBtn, x int) (tea.Model, tea.Cmd) {
	switch b.act {
	case "iso":
		return m.isolateSwitch()
	case "fade":
		pos := 0.0
		if b.w > 1 {
			pos = float64(x-b.x) / float64(b.w-1)
		}
		return m.sceneAct("fade", b.arg, pos), nil
	case "bars":
		return m.sceneAct("bars", b.arg, 0), nil
	case "view":
		switch b.arg {
		case 0:
			return m.perfKey("p")
		case 1:
			return m.toggleRecording()
		}
		return m.toggleFullScreen()
	}
	return m.sceneAct(b.act, b.arg, 0), nil
}
