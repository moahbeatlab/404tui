package tui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/proto"
)

var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

var helpText = [][2]string{
	{":", "open the command palette"},
	{"1-4, Shift-Tab", "switch section: Pads, Patterns, Files, Projects"},
	{"h j k l, arrows", "move (on the pad grid: left, down, up, right)"},
	{"[ / ]", "previous / next bank"},
	{"g / G", "first / last"},
	{"} / {  (Alt-n / Alt-p)", "jump to the next / previous folder in a file list"},
	{"Ctrl-D / Ctrl-U", "half a page down / up in lists (Page Down / Up: a whole page); in the search box Ctrl-D pages down and Ctrl-U clears the text"},
	{"/ (Pads, Projects)", "find a pad or project by name"},
	{"Enter (Pads)", "preview the pad: on the unit, or on this computer after O"},
	{"o, P (Pads)", "o switches where pads preview (unit / this computer; O and 0 work too); P plays the pad once on this computer"},
	{"C (Pads)", "chromatic: play the pad as notes on this computer (a w s e d f … are piano keys, z / x shift the octave)"},
	{"n / d (Pads)", "rename / delete the sample"},
	{"v / V (Pads)", "mark a pad / every pad with a sample"},
	{"S, , . (Pads)", "edit the pad's settings in a dialog; step its level down/up"},
	{"b, < >, - + (Pads)", "bank tempo and volume: edit in a dialog, or step them"},
	{"c (Pads)", "copy the pad (or marked pads) to another pad, in any bank"},
	{"x, then p or s", "pick up a pad's sample, then move or swap it onto another"},
	{"e / R (Pads)", "export the bank / restore an export (p there: also make a pattern)"},
	{"Enter (Patterns)", "open the pattern; b retargets its pads to another bank"},
	{"a (Pads)", "file dialog for this pad; it opens where you left it"},
	{"Enter, a (Files)", "assign the file: picker opens on the target bank's next free pad"},
	{"[ ] (Files)", "choose the target bank for assigns"},
	{"A-J, 1-16 (picker)", "type a bank letter and a pad number: C, 7, Enter"},
	{"e / R (Projects)", "export the project / restore one from a backup folder"},
	{"/ (Files, dialog)", "search every audio file in the source by keyword: Enter assigns, Tab shows it in the tree"},
	{"Space (Files)", "listen to the file on this computer"},
	{"Ctrl-A (Files, search, dialog)", "autoplay: a file plays as the cursor rests on it; moving stops the old one at once"},
	{"v (Files)", "mark audio files; Enter then fills free pads in order"},
	{"Tab / l / h (Files)", "open or close a folder, go to the parent"},
	{"v, V, J (Files)", "mark a file or folder; every file in the folder; all junk (._ files, .DS_Store)"},
	{"d, c, x, p (Files)", "delete the marked (SD card only); copy / cut to the clipboard; paste into the folder in front"},
	{"N, n (Files)", "new folder; rename"},
	{"R (Files)", "restore pads from this folder's export (asks which bank)"},
	{"t (Files)", "next source: SD card, unit storage, this computer"},
	{"Enter, n (Projects)", "select / rename the project"},
	{"T", "next colour theme"},
	{"r", "refresh from the unit"},
	{"?", "toggle this help"},
	{"q, Ctrl-C", "quit (Esc backs out of whatever is open first)"},
}

// View draws a header, the body with the navigation rail down its left, and a status bar;
// the help, a prompt, a picker, the palette or a wait is drawn over the body.
func (m Model) View() string {
	g := m.geom()
	var main string
	switch {
	case m.help:
		main = m.center(m.helpBox(), g.bodyH)
	case m.search != nil:
		main = m.center(m.searchBox(g), g.bodyH)
	case m.editor != nil:
		main = m.center(m.editorBox(), g.bodyH)
	case m.chromatic != nil:
		main = m.center(m.chromaticBox(), g.bodyH)
	case m.browser != nil:
		main = m.center(m.browserBox(g), g.bodyH)
	case m.prompt != nil:
		main = m.center(m.promptBox(), g.bodyH)
	case m.picker != nil:
		main = m.center(m.pickerBox(), g.bodyH)
	case m.toggles != nil:
		main = m.center(m.togglesBox(), g.bodyH)
	case m.palette != nil:
		main = m.center(m.paletteBox(), g.bodyH)
	case m.unit == nil:
		main = m.center(m.waitingBox(), g.bodyH)
	case m.busy != "":
		main = m.center(m.busyBox(), g.bodyH)
	case m.snap == nil:
		main = m.center(m.st.dim.Render("no data from the unit; r tries again"), g.bodyH)
	default:
		main = m.withRail(g, fit(m.body(g), g.bodyH))
	}
	parts := []string{m.header(), m.rule(), fit(main, g.bodyH)}
	if m.tagJob.active() {
		parts = append(parts, m.indexBar())
	}
	screen := strings.Join(append(parts, m.statusBar()), "\n")
	return lipgloss.NewStyle().MaxWidth(m.width).Render(screen)
}

// withRail puts the navigation rail to the left of the body, with a rule between them.
func (m Model) withRail(g geom, body string) string {
	rail := m.railView(g)
	rule := strings.TrimSuffix(strings.Repeat(m.st.faint.Render("│")+"\n", g.bodyH), "\n")
	return lipgloss.JoinHorizontal(lipgloss.Top, rail, rule, "  ", body)
}

// rule is the line under the header.
func (m Model) rule() string {
	return m.st.faint.Render(strings.Repeat("─", m.width))
}

// railBank is the bank highlighted on the rail: the target of assigns on Files, else the
// bank in view.
func (m Model) railBank() int {
	if m.section == secFiles {
		return m.target
	}
	return m.bank
}

// railView is the navigation down the left: the sections, and under them the banks or the
// file sources of the section in front.
func (m Model) railView(g geom) string {
	st := m.st
	counts := m.bankCounts()
	items := m.rail(g.bodyH)
	lines := make([]string, len(items))
	for i, it := range items {
		text, active, base := "", false, st.text
		switch it.kind {
		case railSection:
			text = fmt.Sprintf(" %d %s", it.index+1, sectionNames[it.index])
			active = it.index == m.section
		case railBank:
			n := counts[it.index]
			left, right := fmt.Sprintf(" %c", 'A'+it.index), countLabel(n)+" "
			text = left + strings.Repeat(" ", max(g.railW-len(left)-len(right), 1)) + right
			active = it.index == m.railBank()
			if n == 0 {
				base = st.faint
			}
		case railRoot:
			root := roots()[it.index]
			text = " " + rootName(root)
			active = root == m.root
		case railLabel:
			text, base = " "+railLabelText(it.index), st.faint
		}
		text = ansi.Truncate(text, g.railW, "")
		text += strings.Repeat(" ", g.railW-ansi.StringWidth(text))
		if active {
			text = lipgloss.NewStyle().Background(st.th.accent).Foreground(st.th.cursorFg).Bold(true).Render(text)
		} else {
			text = base.Render(text)
		}
		lines[i] = text
	}
	return strings.Join(lines, "\n")
}

func (m Model) body(g geom) string {
	switch m.section {
	case secPads:
		return m.padsBody(g)
	case secPatterns:
		return m.patternsBody(g)
	case secFiles:
		return m.filesBody(g)
	}
	return m.projectsBody(g)
}

// ---- chrome ----

func (m Model) header() string {
	st := m.st
	badge := lipgloss.NewStyle().Background(st.th.accent).Foreground(st.th.cursorFg).Bold(true).Render(" 404tui ")
	if m.unit == nil {
		return spaceBetween(badge, st.warn.Render("○ no unit")+" ", m.width)
	}
	if m.snap == nil {
		return badge
	}
	label := fmt.Sprintf("  PROJECT %d", m.snap.Project+1)
	if m.snap.ProjectName != "" {
		label = "  " + m.snap.ProjectName
	}
	left := badge + st.bold.Render(label)
	free := float64(m.snap.FreeKb) / 1024 / 1024
	left += st.dim.Render(fmt.Sprintf("   %.1f GB free", free))

	// For a few seconds after connecting, say so loudly.
	right := st.good.Render("●") + " " + st.dim.Render(m.snap.Port)
	if m.connect != nil && m.frame-m.connectedFrame < 50 {
		right = st.good.Render("● CONNECTED  " + m.snap.Port)
	}
	if m.playing {
		right = st.hot.Render(fmt.Sprintf("▶ %s  ", m.playingPad)) + right
	} else if m.hasSelected {
		right = st.dim.Render(fmt.Sprintf("◆ %s  ", m.selected)) + right
	}
	return spaceBetween(left, right+" ", m.width)
}

func spaceBetween(left, right string, width int) string {
	gap := width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 1 {
		return ansi.Truncate(left, max(width-ansi.StringWidth(right)-1, 0), "…") + " " + right
	}
	return left + strings.Repeat(" ", gap) + right
}

func countLabel(n int) string {
	if n == 0 {
		return "·"
	}
	return fmt.Sprint(n)
}

// bankCounts is how many pads hold a sample in each bank (patterns: how many slots do).
func (m Model) bankCounts() [proto.Banks]int {
	var counts [proto.Banks]int
	if m.section == secPatterns {
		for b, rows := range m.patterns {
			for _, r := range rows {
				if r.Exists {
					counts[b]++
				}
			}
		}
		return counts
	}
	for _, row := range m.snap.Pads {
		if row.HasSample {
			counts[row.Pad.Bank()]++
		}
	}
	return counts
}

func roots() []string {
	return []string{app.CardRoot, app.StorageRoot, localRoot()}
}

func rootName(root string) string {
	switch root {
	case app.CardRoot:
		return "SD card"
	case app.StorageRoot:
		return "Unit storage"
	}
	return "This computer"
}

// statusBar shows the message, and the keys that matter here as chips.
func (m Model) statusBar() string {
	st := m.st
	chip := func(key, what string) string { return st.accent.Render(key) + st.dim.Render(" "+what) }
	var chips []string
	// The bar holds the few keys used most; the rest is under ? and, in the Files view, grouped
	// in the Details panel.
	switch m.section {
	case secPads:
		chips = []string{chip("Enter", "preview on "+map[bool]string{true: "computer", false: "unit"}[m.previewLocal]), chip("a", "assign"), chip("v", "mark"), chip("t", "set"), chip("c", "copy"), chip("d", "delete"), chip("[ ]", "bank")}
	case secPatterns:
		chips = []string{chip("Enter", "open"), chip("N", "new"), chip("P", "play"), chip("d", "delete"), chip("c", "copy"), chip("[ ]", "bank")}
	case secFiles:
		if len(m.fileMarks) > 0 || len(m.clip) > 0 {
			chips = []string{chip("Enter", "assign"), chip("d", "delete"), chip("c", "copy"), chip("x", "move"), chip("p", "paste"), chip("u", "unmark"), chip("Esc", "clear")}
			break
		}
		chips = []string{chip("Enter", "assign"), chip("v", "mark"), chip("*", "favourite"), chip("/", "search"), chip("s", "tags"), chip("f", "folder"), chip("i", "index"), chip("Space", "listen")}
	case secProjects:
		chips = []string{chip("Enter", "select"), chip("n", "rename"), chip("e", "export"), chip("R", "restore"), chip("X", "erase")}
	}
	chips = append(chips, chip("?", "all keys"), chip("q", "quit"))
	line := strings.Join(chips, st.faint.Render("  "))
	if m.autoplay {
		line = st.good.Render("♫ autoplay") + st.faint.Render("  │  ") + line
	}
	if m.logPath != "" {
		line = st.warn.Render("● debug log") + st.faint.Render("  │  ") + line
	}
	if m.listening != "" && m.player != nil && m.player.Playing() {
		line = st.hot.Render("♪ "+ansi.Truncate(app.Base(m.listening), 28, "…")) + st.faint.Render("  │  ") + line
	}
	if m.message != "" {
		line = st.warn.Render(m.message) + st.faint.Render("  │  ") + line
	}
	return ansi.Truncate(" "+line, m.width, "…")
}

func (m Model) center(content string, height int) string {
	return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, content)
}

// fit makes text exactly height lines tall.
func fit(text string, height int) string {
	lines := strings.Split(text, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines[:height], "\n")
}

// window picks the rows to show so that the cursor stays in view.
func window(count, cursor, height int) (from, to int) {
	if count <= height {
		return 0, count
	}
	from = min(max(cursor-height/2, 0), count-height)
	return from, from + height
}

// ---- boxes ----

// box draws a bordered box with its title set into the top edge.
func (m Model) box(title, body string, width, height int, focused bool) string {
	border := m.st.th.faint
	if focused {
		border = m.st.th.accent
	}
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border)
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, width-2, "…")
	}
	for len(lines) < height-2 {
		lines = append(lines, "")
	}
	rendered := style.Width(width - 2).Render(strings.Join(lines[:max(height-2, 0)], "\n"))
	if title == "" {
		return rendered
	}
	label := " " + title + " "
	first, rest, _ := strings.Cut(rendered, "\n")
	edge := ansi.Cut(first, 0, 2) + m.st.bold.Render(label) +
		ansi.Cut(first, 2+ansi.StringWidth(label), ansi.StringWidth(first))
	return edge + "\n" + rest
}

// ---- pad tiles ----

type tileState struct {
	empty, cursor, marked, playing, selected, moving bool
}

// tile draws one pad of the grid: its number, name and level, on a background that says
// what state it is in.
func (m Model) tile(w, h int, num, name string, level int32, ts tileState) string {
	th := m.st.th
	bg, fg := th.tile, th.text
	switch {
	case ts.cursor:
		bg, fg = th.cursorBg, th.cursorFg
	case ts.playing:
		bg, fg = th.hot, th.cursorFg
	case ts.marked:
		bg = th.markedBg
	case ts.empty:
		bg, fg = th.tileEmpty, th.faint
	}
	style := lipgloss.NewStyle().Background(bg).Foreground(fg).Width(w)
	bold := style.Bold(true)

	flags := ""
	switch {
	case ts.playing:
		flags = "▶"
	case ts.moving:
		flags = "↗"
	case ts.marked:
		flags = "✓"
	case ts.selected:
		flags = "◆"
	}
	numLine := " " + num
	if flags != "" {
		numLine = " " + num + strings.Repeat(" ", max(w-2-ansi.StringWidth(num)-ansi.StringWidth(flags), 1)) + flags
	}
	nameLine := " " + ansi.Truncate(name, w-2, "…")
	if ts.empty {
		nameLine = ""
	}
	bar := ""
	if !ts.empty {
		bar = " " + levelBar(w-2, int(level), 127)
	}
	if h == 2 {
		first := numLine
		if !ts.empty {
			first = " " + num + " " + ansi.Truncate(name, max(w-3-ansi.StringWidth(num), 1), "…")
			if flags != "" {
				first = ansi.Truncate(first, w-2, "") + " " + flags
			}
		}
		return bold.Render(first) + "\n" + style.Render(bar)
	}
	return bold.Render(numLine) + "\n" + style.Render(nameLine) + "\n" + style.Render(bar)
}

// levelBar draws value out of max as a bar of width cells.
func levelBar(width, value, max int) string {
	if width < 1 {
		return ""
	}
	filled := value * width / max
	return strings.Repeat("━", clamp(filled, 0, width)) + strings.Repeat("╌", width-clamp(filled, 0, width))
}

// grid draws the 4×4 grid with tile(p) giving each position's tile, pad 1 at the top left, as on the unit.
func (m Model) grid(g geom, tile func(p int) string) string {
	rows := make([]string, 4)
	for row := 0; row < 4; row++ {
		cells := make([]string, 0, 7)
		for col := 0; col < 4; col++ {
			if col > 0 {
				cells = append(cells, " ")
			}
			cells = append(cells, tile(row*4+col))
		}
		rows[row] = lipgloss.JoinHorizontal(lipgloss.Top, cells...)
	}
	var out []string
	for i, r := range rows {
		out = append(out, r)
		if i < 3 {
			out = append(out, "")
		}
	}
	return strings.Join(out, "\n")
}

func (m Model) padsBody(g geom) string {
	bank := m.snap.Bank(m.bank)
	grid := m.grid(g, func(p int) string {
		row := bank[p]
		return m.tile(g.tileW, g.tileH, fmt.Sprint(p+1), row.Name, row.Level, tileState{
			empty:    !row.HasSample,
			cursor:   p == m.pad,
			marked:   m.marked[p],
			playing:  m.playing && m.playingPad == row.Pad,
			selected: m.hasSelected && m.selected == row.Pad,
			moving:   m.hasMoving && m.moving == row.Pad,
		})
	})
	detail := m.padDetail(g)
	bar := m.bankBar() + "\n\n"
	if g.side {
		// The patterns of the bank sit under the pads, when there is room for them.
		if strip := m.patternStrip(g); strip != "" && g.bodyH-g.top-lipgloss.Height(grid) >= lipgloss.Height(strip)+1 {
			grid = lipgloss.JoinVertical(lipgloss.Left, grid, "", strip)
		}
		return bar + lipgloss.JoinHorizontal(lipgloss.Top, grid, "  ", m.sideColumn(g, detail))
	}
	if g.detailH < 4 {
		return bar + grid
	}
	return bar + grid + "\n\n" + detail
}

// patternStrip shows the 16 pattern slots of the bank on show under the pads: a slot that
// holds a pattern with how many pads it plays, and the ones that play the pad in front picked
// out. It is empty until the patterns are read.
func (m Model) patternStrip(g geom) string {
	rows := m.patterns[m.bank]
	if rows == nil || g.gridW < 40 {
		return ""
	}
	st := m.st
	used := map[proto.PadIndex]bool{}
	if m.usageState == 2 {
		for _, slot := range m.usage[m.padRow().Pad] {
			used[slot] = true
		}
	}
	const perRow = 8
	cell := max((g.gridW-4)/perRow, 7)
	var lines []string
	for start := 0; start < len(rows); start += perRow {
		var line string
		for _, r := range rows[start:min(start+perRow, len(rows))] {
			num := fmt.Sprint(int(r.Slot.Index())%proto.PadsPerBank + 1)
			text := fmt.Sprintf("%2s ·", num)
			style := st.faint
			if r.Exists {
				text = fmt.Sprintf("%2s ■%d", num, len(r.Pads))
				style = st.text
				if used[r.Slot] {
					style = st.warn
				}
			}
			line += style.Render(padTo(text, cell))
		}
		lines = append(lines, line)
	}
	title := fmt.Sprintf("Patterns · bank %c", 'A'+m.bank)
	if n := len(used); n > 0 {
		verb := "play"
		if n == 1 {
			verb = "plays"
		}
		title += fmt.Sprintf(" · %s %s this pad", plural(n, "pattern"), verb)
	}
	return m.box(title, strings.Join(lines, "\n"), g.gridW, len(lines)+2, false)
}

// bankBar is the line above the grids: the bank's tempo and volume, and how to change them.
func (m Model) bankBar() string {
	st := m.st
	info := m.snap.Banks[m.bank]
	tempo := st.text.Render(fmt.Sprintf("%.2f BPM", info.Tempo))
	if m.snap.UsesProjectTempo {
		tempo = st.faint.Render(fmt.Sprintf("%.2f BPM", info.Tempo)) + st.dim.Render(fmt.Sprintf("  (project tempo %.2f is in use)", m.snap.ProjectTempo))
	}
	return st.accent.Render(fmt.Sprintf("Bank %c", 'A'+m.bank)) +
		st.dim.Render("   Tempo ") + tempo +
		st.dim.Render("   Volume ") + st.accent.Render(levelBar(12, info.Volume, 127)) + st.text.Render(fmt.Sprintf(" %d", info.Volume)) +
		st.faint.Render("    b edit  < > tempo  - + volume")
}

// padDetail is the panel under (or beside) the grid: what is on the selected pad. The sample
// (length, trim, chop points), its settings, the patterns that play it, and while it
// sounds on the unit, how far along it is.
func (m Model) padDetail(g geom) string {
	st := m.st
	row := m.padRow()
	title := fmt.Sprintf("%s  %s", row.Pad, row.Name)
	if !row.HasSample {
		title = row.Pad.String()
	}
	if len(m.marked) > 0 {
		title += st.good.Render(fmt.Sprintf("   %d marked", len(m.marked)))
	}
	height := max(g.detailH, 5)
	if !row.HasSample {
		lines := []string{st.dim.Render("No sample on this pad."), "", st.dim.Render("a  picks a file for it    c  copies another pad here")}
		if used := m.usedIn(row); used != "" {
			lines = append(lines, "", used)
		}
		return m.box(title, strings.Join(lines, "\n"), g.detailW, height, false)
	}

	inner := g.detailW - 4
	res, have := m.facts[factKey{row.Pad, row.Size, row.Name}]
	p := func(name string) int32 { return row.Params[name] }
	flags := p("mode-flags")
	// Each line is a label and its value, the labels lined up; the sample comes first, then
	// the settings in groups that sit side by side when there is room.
	label := func(l string) string { return st.dim.Render(fmt.Sprintf("%-9s", l)) }
	row2 := func(l, v string) string { return label(l) + st.text.Render(v) }
	onOff := func(l string, on bool) string {
		if on {
			return label(l) + st.good.Render("● on")
		}
		return label(l) + st.faint.Render("○ off")
	}
	section := func(title string, body ...string) []string {
		return append([]string{st.accent.Render(strings.ToUpper(title))}, body...)
	}

	var sample []string
	switch {
	case !have:
		sample = append(sample, st.faint.Render("reading the sample…"))
	case res.err != nil:
		sample = append(sample, st.bad.Render(res.err.Error()))
	default:
		f := res.facts
		channels := "mono"
		if f.Channels > 1 {
			channels = "stereo"
		}
		sample = append(sample,
			row2("Length", fmt.Sprintf("%.2f s  ·  %s  ·  %.1f KB", f.Seconds, channels, float64(row.Size)/1024)),
			row2("File", f.File))
		if bar := m.trimBar(row, f, clamp(inner-48, 10, 36)); bar != "" {
			sample = append(sample, strings.Replace(bar, "Trim  ", label("Trim"), 1))
		}
		if row.BPM > 0 {
			tempo := fmt.Sprintf("%.2f BPM", row.BPM)
			if f.Seconds > 0 {
				tempo += fmt.Sprintf("  ≈ %.1f bars", f.Seconds*row.BPM/60/4)
			}
			sample = append(sample, row2("Tempo", tempo))
		}
	}
	if used := strings.TrimPrefix(m.usedIn(row), st.dim.Render("Used in ")); used != "" {
		sample = append(sample, label("Used in")+used)
	}
	if m.playing && m.playingPad == row.Pad && have && res.err == nil && res.facts.Frames > 0 {
		f := res.facts
		frac := min(float64(m.playPos)/float64(f.Frames), 1)
		sample = append(sample, st.hot.Render("▶ ")+st.hot.Render(levelBar(20, int(frac*100), 100))+
			st.text.Render(fmt.Sprintf("  %.2f / %.2f s", float64(m.playPos)/48000, f.Seconds)))
	}

	groups := [][]string{
		section("Playback",
			row2("Mode", playModeName(p("play-mode"))),
			onOff("Gate", p("gate") != 0),
			onOff("Loop", p("loop") != 0),
			onOff("One shot", flags&0x20 != 0),
			onOff("BPM sync", p("bpm-sync") != 0)),
		section("Sound",
			label("Level")+st.accent.Render(levelBar(10, int(row.Level), 127))+st.text.Render(fmt.Sprintf(" %d", row.Level)),
			row2("Pan", panText(p("balance"))),
			row2("Pitch", fmt.Sprintf("%+d st %+d ct", p("pitch-coarse"), p("pitch-fine"))),
			row2("Stretch", fmt.Sprintf("%.0f%%", float64(p("time-stretch"))/100)),
			onOff("Vinyl", p("vinyl") != 0)),
		section("Routing",
			row2("Bus FX", busName(p("bus-fx"))),
			row2("Mute grp", offOr(p("mute-group"))),
			row2("Pad link", offOr(p("pad-link"))),
			row2("Roll", fmt.Sprint(p("roll")))),
		section("Envelope",
			row2("Attack", fmt.Sprint(p("attack"))),
			row2("Hold", fmt.Sprint(p("hold"))),
			row2("Release", fmt.Sprint(p("release")))),
	}
	// As many groups per row as fit, each in a column of its own width.
	const colW = 27
	perRow := max(min(inner/colW, len(groups)), 1)
	lines := section("Sample", sample...)
	for i := 0; i < len(groups); i += perRow {
		cols := make([]string, 0, perRow)
		for _, grp := range groups[i:min(i+perRow, len(groups))] {
			cols = append(cols, lipgloss.NewStyle().Width(colW).Render(strings.Join(grp, "\n")))
		}
		lines = append(lines, "", lipgloss.JoinHorizontal(lipgloss.Top, cols...))
	}
	lines = append(lines, "", st.faint.Render("S edit settings  ·  , . level  ·  c copy  ·  Enter preview"))
	body := strings.Join(lines, "\n")
	return m.box(title, body, g.detailW, max(height, strings.Count(body, "\n")+3), false)
}

// usedIn is the line naming the patterns that play a pad.
func (m Model) usedIn(row app.PadRow) string {
	st := m.st
	if m.usageState != 2 {
		return st.dim.Render("Used in ") + st.faint.Render("reading patterns…")
	}
	slots := m.usage[row.Pad]
	if len(slots) == 0 {
		return st.dim.Render("Used in ") + st.faint.Render("no pattern")
	}
	names := make([]string, len(slots))
	for i, slot := range slots {
		names[i] = slot.String()
	}
	return st.dim.Render("Used in ") + st.warn.Render(strings.Join(names, " · "))
}

// trimBar draws the sample's length as a line: the part that plays solid, what is trimmed
// away dotted, chop points as ┃ and the loop start as ◆.
func (m Model) trimBar(row app.PadRow, f app.PadFacts, width int) string {
	st := m.st
	if f.Frames == 0 || width < 8 {
		return ""
	}
	cells := make([]rune, width)
	for i := range cells {
		frame := uint32(uint64(i) * uint64(f.Frames) / uint64(width))
		if frame >= f.StartFrame && frame < f.EndFrame {
			cells[i] = '━'
		} else {
			cells[i] = '╌'
		}
	}
	at := func(frame uint32) int { return min(int(uint64(frame)*uint64(width)/uint64(f.Frames)), width-1) }
	if f.LoopFrame > f.StartFrame && f.LoopFrame < f.EndFrame {
		cells[at(f.LoopFrame)] = '◆'
	}
	for _, c := range row.ChopFrames {
		cells[at(c)] = '┃'
	}
	text := fmt.Sprintf("  %.2f–%.2f s", float64(f.StartFrame)/48000, float64(f.EndFrame)/48000)
	if n := len(row.ChopFrames); n > 0 {
		text += fmt.Sprintf("  ·  %d chop points", n)
	}
	return st.dim.Render("Trim  ") + st.accent.Render(string(cells)) + st.dim.Render(text)
}

func playModeName(v int32) string {
	switch v {
	case 0:
		return "forward"
	case 1:
		return "reverse"
	case 2:
		return "fwd ping-pong"
	case 3:
		return "rev ping-pong"
	}
	return fmt.Sprint(v)
}

// panText is the balance as the unit shows it: 64 is the centre.
func panText(v int32) string {
	switch d := int(v) - 64; {
	case d == 0:
		return "C"
	case d < 0:
		return fmt.Sprintf("L%d", -d)
	default:
		return fmt.Sprintf("R%d", d)
	}
}

func busName(v int32) string {
	switch v {
	case 0:
		return "off"
	case 1, 2:
		return fmt.Sprintf("BUS %d", v)
	}
	return fmt.Sprint(v)
}

// offOr is a setting where 0 means off.
func offOr(v int32) string {
	if v == 0 {
		return "off"
	}
	return fmt.Sprint(v)
}

// session is a small panel for wide screens: the unit, its card, and what is picked up.
func (m Model) session(g geom) string {
	st := m.st
	total := 1.0
	free := float64(m.snap.FreeKb) / 1024 / 1024
	lines := []string{
		st.dim.Render("Project  ") + st.text.Render(fmt.Sprintf("%d  %s", m.snap.Project+1, m.snap.ProjectName)),
		st.dim.Render("Free     ") + st.accent.Render(levelBar(12, int(free*100), int(max(free, total)*100))) + st.text.Render(fmt.Sprintf(" %.1f GB", free)),
		st.dim.Render("Unit     ") + st.text.Render(m.unitLine()),
	}
	if m.hasMoving {
		lines = append(lines, st.warn.Render(fmt.Sprintf("Picked up %s: p moves, s swaps", m.moving)))
	}
	if len(m.clip) > 0 {
		lines = append(lines, st.warn.Render(fmt.Sprintf("Clipboard: %s to %s", plural(len(m.clip), "item"), map[bool]string{true: "move", false: "copy"}[m.clipCut])))
	}
	return m.box("Session", strings.Join(lines, "\n"), g.detailW, len(lines)+2, false)
}

func (m Model) unitLine() string {
	switch {
	case m.playing:
		return fmt.Sprintf("playing %s", m.playingPad)
	case m.hasSelected:
		return fmt.Sprintf("selected %s", m.selected)
	}
	return "idle"
}

// waveRows is how many terminal rows the waveform is drawn in.
const waveRows = 6

// brailleDots are the dot bits of a braille cell, indexed [column][row].
var brailleDots = [2][4]rune{{0x01, 0x02, 0x04, 0x40}, {0x08, 0x10, 0x20, 0x80}}

// bars draws amplitudes (0..32768) as a mirrored waveform in braille dots: two columns and
// four rows of resolution per cell, growing out from a centre line. Quiet tails are lifted
// with a square root so a decay stays visible next to a loud attack.
func (m Model) bars(amps []int, width int) []string {
	peak := 1
	for _, a := range amps {
		peak = max(peak, a)
	}
	cols := width * 2
	half := waveRows * 4 / 2
	heights := make([]int, cols)
	for c := range heights {
		lo, hi := c*len(amps)/cols, (c+1)*len(amps)/cols
		hi = max(hi, lo+1)
		a := 0
		for _, v := range amps[lo:min(hi, len(amps))] {
			a = max(a, v)
		}
		heights[c] = max(1, int(math.Sqrt(float64(a)/float64(peak))*float64(half)+0.5))
	}
	rows := make([]string, waveRows)
	for r := 0; r < waveRows; r++ {
		var line strings.Builder
		for c := 0; c < width; c++ {
			cell := rune(0x2800)
			for sub := 0; sub < 2; sub++ {
				h := heights[c*2+sub]
				for d := 0; d < 4; d++ {
					dot := r*4 + d // 0 is the top of the picture
					if dot >= half-h && dot < half+h {
						cell |= brailleDots[sub][d]
					}
				}
			}
			line.WriteRune(cell)
		}
		rows[r] = m.st.accent.Render(line.String())
	}
	return rows
}

// timeAxis labels the two ends of a waveform that is width cells wide.
func (m Model) timeAxis(seconds float64, width int) string {
	end := fmt.Sprintf("%.2f s", seconds)
	gap := max(width-len("0.00 s")-len(end), 1)
	return m.st.faint.Render("0.00 s" + strings.Repeat(" ", gap) + end)
}

// ---- patterns ----

func (m Model) patternsBody(g geom) string {
	if m.pattern != nil {
		return m.patternDetail(g)
	}
	rows := m.patterns[m.bank]
	if rows == nil {
		return "  " + m.st.dim.Render("reading patterns…")
	}
	grid := m.grid(g, func(p int) string {
		row := rows[p]
		name, state := "", tileState{empty: !row.Exists, cursor: p == m.patternCursor, marked: m.patMarks[int(row.Slot.Index())], moving: m.patHolding && m.patHeld == row.Slot}
		if row.Exists {
			name = fmt.Sprintf("%d pads", len(row.Pads))
		}
		return m.tile(g.tileW, g.tileH, fmt.Sprint(p+1), name, 0, state)
	})
	cur := rows[m.patternCursor]
	lines := []string{m.st.dim.Render("Empty slot.")}
	if cur.Exists {
		names := make([]string, len(cur.Pads))
		for i, pad := range cur.Pads {
			names[i] = pad.String()
		}
		lines = []string{m.st.text.Render("Plays " + strings.Join(names, " ")), "", m.st.dim.Render("Enter opens the step grid"), m.st.dim.Render("d delete  v mark  c copy  x move  p put")}
	}
	detail := m.box("Pattern "+cur.Slot.String(), strings.Join(lines, "\n"), g.detailW, max(g.detailH, 5), false)
	bar := m.bankBar() + "\n\n"
	if g.side {
		return bar + lipgloss.JoinHorizontal(lipgloss.Top, grid, "  ", m.sideColumn(g, detail))
	}
	if g.detailH < 4 {
		return bar + grid
	}
	return bar + grid + "\n\n" + detail
}

// patternDetail is an opened pattern: its step grid, one row per pad it plays.
func (m Model) patternDetail(g geom) string {
	st, d := m.st, m.pattern
	info := fmt.Sprintf("%.2f BPM  ·  %d/4  ·  %d ticks  ·  %d pads", d.BankTempo, d.Beats, d.Length, len(d.PadsUsed))
	lines := []string{st.dim.Render(info), ""}
	ruler := strings.Repeat(" ", 20)
	for i := 0; i < d.Grid.Steps; i++ {
		if i > 0 && i%4 == 0 {
			ruler += "  "
		}
		if i%4 == 0 {
			ruler += st.faint.Render(fmt.Sprint(i/4 + 1))
		} else {
			ruler += " "
		}
		ruler += " "
	}
	lines = append(lines, ruler)
	if len(d.Grid.Rows) == 0 {
		lines = append(lines, st.dim.Render("  (no notes)"))
	}
	for _, row := range d.Grid.Rows {
		name := m.snap.Pads[row.Pad.Index()].Name
		label := fmt.Sprintf("%-4s %-14s", row.Pad, ansi.Truncate(name, 14, "…"))
		var cells strings.Builder
		for i, hit := range row.Hits {
			if i > 0 && i%4 == 0 {
				cells.WriteString("  ")
			}
			if hit {
				cells.WriteString(st.accent.Render("■"))
			} else {
				cells.WriteString(st.faint.Render("·"))
			}
			cells.WriteString(" ")
		}
		lines = append(lines, st.bold.Render(label)+" "+cells.String())
	}
	hint := "b retargets its pads to another bank   Esc back"
	if !d.HasSourceBank {
		hint = "its pads span more than one bank, so it cannot be retargeted   Esc back"
	}
	lines = append(lines, "", st.dim.Render(hint))
	return m.box("Pattern "+d.Slot.String(), strings.Join(lines, "\n"), g.bodyW, g.bodyH, true)
}

// ---- files ----

func (m Model) filesBody(g geom) string {
	title := rootName(m.root)
	if n := len(m.fileMarks); n > 0 {
		title += fmt.Sprintf("  ·  %d marked", n)
	}
	detailW := g.bodyW - g.listW - 1
	list := m.box(title, m.treeLines(g.bodyH-2), g.listW, g.bodyH, true)
	detail := m.box("Details", m.withKeyGuide(m.fileDetail(g.bodyH-2, detailW-2), filesKeyGuide, g.bodyH-2, detailW-2), detailW, g.bodyH, false)
	return lipgloss.JoinHorizontal(lipgloss.Top, list, " ", detail)
}

func (m Model) treeLines(height int) string {
	return m.treeRender(m.tree, m.fileAt, height, func(n app.Node) string {
		switch {
		case m.fileMarks[n.Path]:
			return m.st.good.Render("✓")
		case m.clipMarker(n.Path) != "":
			return m.st.warn.Render(m.clipMarker(n.Path))
		case n.Path == m.listening:
			return m.st.hot.Render("♪")
		case m.favouriteMark(n.Path) != "":
			return m.favouriteMark(n.Path)
		}
		return " "
	})
}

// treeRender draws a folder tree with the cursor row highlighted; markOf gives the one
// character shown before a row's name.
func (m Model) treeRender(tree *app.Tree, at, height int, markOf func(app.Node) string) string {
	st := m.st
	if tree == nil {
		return st.dim.Render(" listing…")
	}
	nodes := tree.Rows()
	if len(nodes) == 0 {
		return st.dim.Render(" (empty)")
	}
	from, to := window(len(nodes), at, height)
	lines := make([]string, 0, to-from)
	for i := from; i < to; i++ {
		node := nodes[i]
		icon := "  "
		switch {
		case node.Expanded:
			icon = "▾ "
		case node.Dir:
			icon = "▸ "
		}
		name := node.Name
		style := st.text
		switch {
		case node.Dir:
			name += "/"
			style = st.accent
		case app.IsJunk(node.Name):
			style = st.faint
		case isAudio(node.Name):
			style = st.text
		case node.Name == app.Sidecar:
			style = st.good
		default:
			style = st.dim
		}
		mark := markOf(node)
		line := " " + mark + strings.Repeat("  ", node.Depth) + icon + style.Render(name)
		if i == at {
			line = lipgloss.NewStyle().Background(st.th.cursorBg).Foreground(st.th.cursorFg).Bold(true).Render(
				" " + ansi.Strip(mark) + strings.Repeat("  ", node.Depth) + icon + name)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m Model) fileDetail(height, width int) string {
	st := m.st
	lines := m.indexLines(width)
	free, _ := proto.PadFromBankPad(uint16(m.target), uint16(m.nextFree(m.target)))
	lines = append(lines, st.dim.Render("Target  ")+st.accent.Render(fmt.Sprintf("bank %c · next free %s", 'A'+m.target, free)), st.dim.Render("[ ] changes it; it follows the last pad you assigned"), "")
	if n := len(m.fileMarks); n > 0 {
		lines = append(lines, st.good.Render(plural(n, "item")+" marked"), st.dim.Render("d delete  c copy  x move  u unmark all  Enter assigns the audio ones"), "")
	}
	if len(m.clip) > 0 {
		verb := "copy"
		if m.clipCut {
			verb = "move"
		}
		lines = append(lines, st.warn.Render(fmt.Sprintf("Clipboard: %s to %s", plural(len(m.clip), "item"), verb)), st.dim.Render("p pastes into the folder in front"), "")
	}
	node, ok := m.fileNode()
	if !ok {
		return strings.Join(lines, "\n")
	}
	kind := "File"
	if node.Dir {
		kind = "Folder"
	}
	lines = append(lines, st.accent.Render(node.Name), st.dim.Render(kind+" · "+display(node.Path)))
	if tags := m.tags.Tags(node.Path); len(tags) > 0 && !node.Dir {
		lines = append(lines, st.dim.Render("tags ")+st.good.Render(ansi.Truncate(strings.Join(tags, " "), width-6, "…")))
	}
	lines = append(lines, "")
	switch {
	case isAudio(node.Name):
		lines = append(lines, m.audioInfo(node, width)...)
	case app.IsJunk(node.Name):
		lines = append(lines, st.warn.Render("Junk file, not audio."), st.dim.Render("A macOS or Windows leftover."), st.dim.Render("J marks all of them; d deletes."))
	case node.Name == app.Sidecar:
		lines = append(lines, st.good.Render("Pad export"), st.dim.Render("Enter or R restores it; you choose the bank"))
	case node.Dir:
		lines = append(lines, st.dim.Render("R restores pads from an export here; you choose the bank"))
	}
	return strings.Join(lines, "\n")
}

func (m Model) audioInfo(node app.Node, width int) []string {
	st := m.st
	info, have := m.infos[node.Path]
	switch {
	case !have:
		return []string{st.faint.Render("reading…")}
	case info.err != nil:
		return []string{st.bad.Render(info.err.Error())}
	}
	seconds := float64(info.frames) / 48000
	ch := "mono"
	if info.channels > 1 {
		ch = fmt.Sprintf("%d ch", info.channels)
	}
	lines := []string{st.text.Render(fmt.Sprintf("%.2f s  ·  %d Hz  ·  %s", seconds, info.rate, ch))}
	if info.rate != 48000 {
		lines = append(lines, st.warn.Render("converted to 48 kHz on assign"))
	}
	amps := make([]int, len(info.peaks))
	for i, p := range info.peaks {
		amps[i] = int(p)
	}
	lines = append(lines, "")
	waveW := max(width-2, 10)
	lines = append(lines, m.bars(amps, waveW)...)
	lines = append(lines, m.timeAxis(seconds, waveW))
	lines = append(lines, "", st.dim.Render("Enter assigns it to a pad    Space listens"))
	return lines
}

// ---- projects ----

func (m Model) projectsBody(g geom) string {
	st := m.st
	names := m.snap.Projects
	from, to := window(len(names), m.projectAt, g.bodyH-2)
	lines := make([]string, 0, to-from)
	for i := from; i < to; i++ {
		name := names[i]
		if name == "" {
			name = "—"
		}
		mark := "  "
		if uint8(i) == m.snap.Project {
			mark = st.good.Render("● ")
		}
		line := fmt.Sprintf(" %s%2d  %s", mark, i+1, name)
		if i == m.projectAt {
			line = lipgloss.NewStyle().Background(st.th.cursorBg).Foreground(st.th.cursorFg).Bold(true).Render(
				fmt.Sprintf(" %s%2d  %s", ansi.Strip(mark), i+1, name))
		}
		lines = append(lines, line)
	}
	i := m.projectAt
	title := names[i]
	if title == "" {
		title = "(unnamed)"
	}
	card := []string{st.accent.Render(fmt.Sprintf("Project %d", i+1)), st.bold.Render(title), ""}
	if uint8(i) == m.snap.Project {
		samples := 0
		for _, r := range m.snap.Pads {
			if r.HasSample {
				samples++
			}
		}
		card = append(card, st.good.Render("● current"),
			st.dim.Render("Samples  ")+st.text.Render(fmt.Sprintf("%d of %d pads", samples, len(m.snap.Pads))),
			st.dim.Render("Free     ")+st.text.Render(fmt.Sprintf("%.1f GB", float64(m.snap.FreeKb)/1024/1024)))
	} else {
		card = append(card, st.dim.Render("Not loaded."), st.dim.Render("Enter loads this project."))
	}
	list := m.box("Projects", strings.Join(lines, "\n"), g.listW, g.bodyH, true)
	detail := m.box("Details", strings.Join(card, "\n"), g.bodyW-g.listW-1, g.bodyH, false)
	return lipgloss.JoinHorizontal(lipgloss.Top, list, " ", detail)
}

// waitingBox is what the screen says while there is no unit.
func (m Model) waitingBox() string {
	st := m.st
	title := "Waiting for an SP-404MKII"
	lines := []string{
		st.accent.Render(spinner[m.frame%len(spinner)]) + " " + st.bold.Render(title), "",
		st.text.Render("Connect it with a USB-C cable and switch it on."),
		st.dim.Render("Looking for USB 0582:02e7; it connects by itself."),
	}
	if m.lost {
		lines = append([]string{st.bad.Render("The connection to the unit was lost.")}, append([]string{""}, lines...)...)
	}
	if m.waitErr != "" {
		lines = append(lines, "", st.warn.Render(m.waitErr))
	}
	lines = append(lines, "", st.dim.Render("q quit  ·  ? help"))
	return st.modal.Render(strings.Join(lines, "\n"))
}

// ---- overlays ----

func (m Model) helpBox() string {
	lines := []string{m.st.bold.Render("Keybindings  (? or Esc closes)"), ""}
	for _, row := range helpText {
		lines = append(lines, m.st.accent.Render(fmt.Sprintf("%-22s", row[0]))+" "+m.st.text.Render(row[1]))
	}
	return m.st.modal.Render(strings.Join(lines, "\n"))
}

func (m Model) paletteBox() string {
	st := m.st
	found := m.matches()
	lines := []string{st.accent.Render("> ") + m.palette.filter + "█", ""}
	if len(found) == 0 {
		lines = append(lines, st.dim.Render("no command matches"))
	}
	from, to := window(len(found), m.palette.cursor, max(m.height-12, 5))
	for i := from; i < to; i++ {
		c := found[i]
		line := fmt.Sprintf("%-7s %s", st.dim.Render(c.key), c.label)
		if i == m.palette.cursor {
			line = lipgloss.NewStyle().Background(st.th.cursorBg).Foreground(st.th.cursorFg).Bold(true).Render(fmt.Sprintf("%-7s %s", c.key, c.label))
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", st.dim.Render("↑↓ navigate  ·  Enter run  ·  Esc close"))
	return st.modal.Width(min(m.width-4, 70)).Render(strings.Join(lines, "\n"))
}

func (m Model) promptBox() string {
	st, p := m.st, m.prompt
	var title, hint string
	switch p.kind {
	case promptRenamePad:
		title = fmt.Sprintf("Rename %s", p.pads[0])
	case promptRenameProject:
		title = fmt.Sprintf("Rename project %d", p.project+1)
	case promptTags:
		title = fmt.Sprintf("Tag %s", plural(len(p.paths), "file"))
		hint = "words add tags, -word takes one off  ·  "
	case promptRenameFile:
		title = fmt.Sprintf("Rename %s", app.Base(p.path))
	case promptBulkRename:
		where := "prefix"
		if p.suffix {
			where = "suffix"
		}
		title = fmt.Sprintf("Add a %s to %d pads", where, len(p.pads))
		hint = "Tab switches prefix/suffix  ·  "
	case promptExport:
		title = fmt.Sprintf("Export bank %c to the folder", 'A'+m.bank)
		hint = "SD:…, a path on the unit, or /path on this computer  ·  "
	case promptExportName:
		title = fmt.Sprintf("Export bank %c into %s as the folder", 'A'+m.bank, display(p.path))
	case promptRestore:
		title = "Restore pads from the export in the folder"
		hint = "replaces the samples on the pads it lists  ·  "
	case promptBank:
		field := func(i int, label, value string) string {
			line := st.dim.Render(fmt.Sprintf("%-16s", label))
			if i == p.field {
				return line + st.accent.Render("> ") + value + "█"
			}
			return line + "  " + st.text.Render(value)
		}
		return st.modal.Width(min(m.width-6, 56)).Render(st.bold.Render(fmt.Sprintf("Bank %c", 'A'+m.bank)) + "\n\n" +
			field(0, "Tempo (40-200 BPM)", p.fields[0]) + "\n" + field(1, "Volume (0-127)", p.fields[1]) +
			"\n\n" + st.dim.Render("Tab switches field  ·  Enter apply  ·  Esc cancel"))
	case promptDeleteFiles:
		return m.deleteBox()
	case promptMkdir:
		title = fmt.Sprintf("New folder in %s", display(p.path))
	case promptSearch:
		title = "Find by name"
	case promptFindFolder:
		title = "Find a folder in the list"
	case promptAssignFile:
		title = fmt.Sprintf("Assign the audio file at this path to %s", p.pads[0])
		hint = "SD:…, a path on the unit, or /path on this computer  ·  "
	case promptProjectExport:
		title = fmt.Sprintf("Export project %d to the folder", p.project+1)
		hint = "SD:…, or /path on this computer  ·  "
	case promptProjectExportName:
		title = fmt.Sprintf("Export project %d into %s as the folder", p.project+1, display(p.path))
	case promptProjectRestore:
		title = fmt.Sprintf("Restore project %d from the backup folder", p.project+1)
		hint = "the folder holding PADCONF.BIN  ·  "
	case promptConfirmRestore:
		plan := p.plan
		name := plan.Name
		if name == "" {
			name = "(no name)"
		}
		current := m.snap.Projects[plan.Project]
		if current == "" {
			current = "(unnamed)"
		}
		return st.modal.Width(min(m.width-6, 78)).Render(
			st.bad.Render(fmt.Sprintf("Overwrite project %d?", plan.Project+1)) + "\n\n" +
				st.dim.Render("Backup   ") + st.text.Render(fmt.Sprintf("%s  ·  %d files, %.1f MB", name, len(plan.Files), float64(plan.Total)/1e6)) + "\n" +
				st.dim.Render("From     ") + st.text.Render(display(plan.Dir)) + "\n" +
				st.dim.Render("Replaces ") + st.warn.Render(fmt.Sprintf("project %d, %s — everything in it is erased first", plan.Project+1, current)) + "\n" +
				st.dim.Render("Free     ") + st.text.Render(fmt.Sprintf("%.1f GB on the unit", float64(plan.FreeBytes)/1e9)) +
				"\n\n" + st.dim.Render("y overwrite   n/Esc keep"))
	case promptDeletePatterns:
		names := make([]string, len(p.pads))
		for i, slot := range p.pads {
			names[i] = slot.String()
		}
		return st.modal.Render(st.bold.Render("Delete pattern "+strings.Join(names, " ")+"?") + "\n\n" + st.dim.Render("y delete   n/Esc keep"))
	case promptEraseProject:
		name := m.snap.Projects[p.project]
		if name == "" {
			name = "(empty)"
		}
		return st.modal.Width(min(m.width-6, 70)).Render(
			st.bad.Render(fmt.Sprintf("Erase project %d, %s?", p.project+1, name)) + "\n\n" +
				st.warn.Render("Every sample, pattern and setting in it is lost, and it becomes empty.") + "\n" +
				st.dim.Render("Export it first (e) if you may want it back.") +
				"\n\n" + st.dim.Render("y erase   n/Esc keep"))
	case promptDelete:
		names := make([]string, len(p.pads))
		for i, pad := range p.pads {
			names[i] = pad.String()
		}
		what := "the sample on " + names[0]
		if len(names) > 1 {
			what = fmt.Sprintf("the samples on %d pads (%s)", len(names), strings.Join(names, " "))
		}
		return st.modal.Render(st.bold.Render("Delete "+what+"?") + "\n\n" + st.dim.Render("y delete   n/Esc keep"))
	}
	if p.kind == promptTags {
		return m.tagsPromptBox(p)
	}
	if p.kind == promptFindFolder && m.tree != nil {
		return m.folderFinderBox(m.geom(), m.tree.Rows(), m.indexes[m.root], p.buffer, p.choice, "Find a folder in "+rootName(m.root))
	}
	return st.modal.Width(min(m.width-4, 72)).Render(st.bold.Render(title) + "\n\n" + st.accent.Render("> ") + p.buffer + "█" + "\n\n" + st.dim.Render(hint+"Enter apply  ·  Esc cancel"))
}

// pickerBox is the pad chooser of an assign, or the bank chooser of a retarget.
func (m Model) pickerBox() string {
	st, p := m.st, m.picker
	if p.restorePlan != nil {
		return m.restorePickerBox()
	}
	if p.retarget {
		var strip strings.Builder
		for b := 0; b < proto.Banks; b++ {
			cell := fmt.Sprintf(" %c ", 'A'+b)
			switch {
			case b == p.bank:
				strip.WriteString(lipgloss.NewStyle().Background(st.th.accent).Foreground(st.th.cursorFg).Bold(true).Render(cell))
			case b == p.sourceBank:
				strip.WriteString(st.faint.Render(cell))
			default:
				strip.WriteString(st.text.Render(cell))
			}
		}
		return st.modal.Render(st.bold.Render(fmt.Sprintf("Pattern %s: move its pads from bank %c to bank", p.slot, 'A'+p.sourceBank)) +
			"\n\n" + strip.String() + "\n\n" + st.dim.Render("←/→ or A-J choose  ·  Enter apply  ·  Esc cancel"))
	}
	if len(p.copyFrom) > 0 {
		return m.copyPickerBox()
	}
	title := fmt.Sprintf("Assign %s to a pad", app.Base(p.files[0]))
	if len(p.files) > 1 {
		title = fmt.Sprintf("Assign %d files, from the pad you choose onward, to free pads", len(p.files))
	}
	var strip strings.Builder
	counts := m.bankCountsForPicker()
	for b := 0; b < proto.Banks; b++ {
		cell := fmt.Sprintf(" %c ", 'A'+b)
		switch {
		case b == p.bank:
			strip.WriteString(lipgloss.NewStyle().Background(st.th.accent).Foreground(st.th.cursorFg).Bold(true).Render(cell))
		case counts[b] > 0:
			strip.WriteString(st.text.Render(cell))
		default:
			strip.WriteString(st.faint.Render(cell))
		}
	}
	grid := m.pickerGrid()
	target, _ := proto.PadFromBankPad(uint16(p.bank), uint16(p.pad))
	row := m.snap.Pads[target.Index()]
	line := st.dim.Render(fmt.Sprintf("%s is empty", target))
	if row.HasSample {
		line = st.warn.Render(fmt.Sprintf("%s holds %q — it will be replaced", target, row.Name))
		if p.confirm {
			line = st.bad.Render("Press Enter again to replace it")
		}
	}
	typed := ""
	if p.digits != "" {
		typed = st.accent.Render("  pad " + p.digits + "…")
	}
	return st.modal.Render(st.bold.Render(title) + "\n\n" + strip.String() + typed + "\n\n" + grid + "\n\n" + line +
		"\n\n" + st.dim.Render("A-J bank  ·  1-16 pad  ·  hjkl move  ·  [ ] bank  ·  Enter assign  ·  Esc cancel"))
}

// restorePickerBox asks which bank an export goes onto.
func (m Model) restorePickerBox() string {
	st, p := m.st, m.picker
	plan := p.restorePlan
	if p.slots {
		return m.patternSlotBox()
	}
	var strip strings.Builder
	counts := m.bankCountsForPicker()
	for b := 0; b < proto.Banks; b++ {
		cell := fmt.Sprintf(" %c ", 'A'+b)
		switch {
		case b == p.bank:
			strip.WriteString(lipgloss.NewStyle().Background(st.th.accent).Foreground(st.th.cursorFg).Bold(true).Render(cell))
		case b == p.sourceBank:
			strip.WriteString(st.good.Render(cell))
		case counts[b] > 0:
			strip.WriteString(st.text.Render(cell))
		default:
			strip.WriteString(st.faint.Render(cell))
		}
	}
	title := fmt.Sprintf("Restore %d pads from the export in %s onto which bank?", len(plan.Pads), display(plan.Dir))
	info := st.dim.Render(fmt.Sprintf("exported from bank %c", 'A'+p.sourceBank))
	if plan.Project != "" {
		info += st.dim.Render(" of project " + plan.Project)
	}
	targets, err := plan.Targets(p.bank)
	line := ""
	switch {
	case err != nil:
		line = st.bad.Render(err.Error())
	default:
		occupied := 0
		names := make([]string, len(targets))
		for i, t := range targets {
			if m.snap.Pads[t.Index()].HasSample {
				occupied++
			}
			names[i] = fmt.Sprintf("%d→%s", plan.Pads[i].From.Pad()+1, t)
		}
		list := st.dim.Render(ansi.Truncate(strings.Join(names, "  "), 70, "…"))
		switch {
		case occupied > 0 && p.confirm:
			line = st.bad.Render(fmt.Sprintf("Press Enter again to replace the %d pads that hold samples", occupied))
		case occupied > 0:
			line = st.warn.Render(fmt.Sprintf("%d of the %d pads on bank %c hold samples — they will be replaced", occupied, len(targets), 'A'+p.bank))
		default:
			line = st.good.Render(fmt.Sprintf("all %d target pads on bank %c are free", len(targets), 'A'+p.bank))
		}
		line += "\n" + list
	}
	pattern, enter := st.dim.Render("[ ] p  also make a pattern that starts the restored pads together"), "Enter restore"
	if p.withPattern {
		pattern = st.accent.Render("[x] p  also make a pattern") + st.dim.Render(fmt.Sprintf(": every restored pad on beat 1, held for %d bars", app.StartBars))
		enter = "Enter choose the pattern slot"
	}
	return st.modal.Render(st.bold.Render(title) + "\n" + info + "\n\n" + strip.String() + st.faint.Render("   green: where they came from") +
		"\n\n" + line + "\n\n" + pattern + "\n\n" + st.dim.Render("A-J or ←/→ choose the bank  ·  p pattern  ·  "+enter+"  ·  Esc cancel"))
}

// patternSlotBox asks which pattern slot the pattern of a restore goes into.
func (m Model) patternSlotBox() string {
	st, p := m.st, m.picker
	var strip strings.Builder
	for b := 0; b < proto.Banks; b++ {
		cell := fmt.Sprintf(" %c ", 'A'+b)
		if b == p.ptnBank {
			strip.WriteString(lipgloss.NewStyle().Background(st.th.accent).Foreground(st.th.cursorFg).Bold(true).Render(cell))
		} else {
			strip.WriteString(st.text.Render(cell))
		}
	}
	title := fmt.Sprintf("Pattern for the %d pads restored onto bank %c: which slot?", len(p.restorePlan.Pads), 'A'+p.bank)
	info := st.dim.Render(fmt.Sprintf("every pad starts on beat 1 and is held for %d bars", app.StartBars))
	rows := m.patterns[p.ptnBank]
	if rows == nil {
		return st.modal.Render(st.bold.Render(title) + "\n" + info + "\n\n" + strip.String() + "\n\n" + st.dim.Render("reading the patterns of this bank…"))
	}
	const w = 14
	grid := make([]string, 4)
	for row := 0; row < 4; row++ {
		var cells []string
		for col := 0; col < 4; col++ {
			pos := row*4 + col
			label := fmt.Sprintf("%2d", pos+1)
			style := lipgloss.NewStyle().Width(w).Background(st.th.tileEmpty).Foreground(st.th.faint)
			if rows[pos].Exists {
				label = fmt.Sprintf("%2d %d pads", pos+1, len(rows[pos].Pads))
				style = style.Background(st.th.tile).Foreground(st.th.text)
			}
			if pos == p.pad {
				style = style.Background(st.th.cursorBg).Foreground(st.th.cursorFg).Bold(true)
			}
			cells = append(cells, style.Render(" "+label))
		}
		grid[row] = strings.Join(cells, " ")
	}
	slot := rows[p.pad]
	line := st.good.Render(fmt.Sprintf("%s is free", slot.Slot))
	if slot.Exists {
		line = st.warn.Render(fmt.Sprintf("%s holds a pattern that plays %d pads — it will be replaced", slot.Slot, len(slot.Pads)))
		if p.confirm {
			line = st.bad.Render("Press Enter again to replace it")
		}
	}
	return st.modal.Render(st.bold.Render(title) + "\n" + info + "\n\n" + strip.String() + "\n\n" + strings.Join(grid, "\n") + "\n\n" + line +
		"\n\n" + st.dim.Render("A-J or [ ] bank  ·  hjkl move  ·  Enter restore and make the pattern  ·  Esc back"))
}

// copyPickerBox is the pad chooser of a copy: where the pad (or the first of several) goes.
func (m Model) copyPickerBox() string {
	st, p := m.st, m.picker
	title := fmt.Sprintf("Copy %s to a pad", p.copyFrom[0])
	if len(p.copyFrom) > 1 {
		title = fmt.Sprintf("Copy %d pads, keeping their layout, from the pad you choose onward", len(p.copyFrom))
	}
	var strip strings.Builder
	counts := m.bankCountsForPicker()
	for b := 0; b < proto.Banks; b++ {
		cell := fmt.Sprintf(" %c ", 'A'+b)
		switch {
		case b == p.bank:
			strip.WriteString(lipgloss.NewStyle().Background(st.th.accent).Foreground(st.th.cursorFg).Bold(true).Render(cell))
		case counts[b] > 0:
			strip.WriteString(st.text.Render(cell))
		default:
			strip.WriteString(st.faint.Render(cell))
		}
	}
	target, _ := proto.PadFromBankPad(uint16(p.bank), uint16(p.pad))
	line := st.dim.Render(fmt.Sprintf("%s is empty", target))
	copies, err := app.PlanCopy(p.copyFrom, target)
	switch {
	case err != nil:
		line = st.bad.Render(err.Error())
	case len(copies) == 0:
		line = st.dim.Render("that is the pad itself")
	default:
		held := func(pad proto.PadIndex) bool { return m.snap.Pads[pad.Index()].HasSample }
		n := app.Occupied(copies, held)
		switch {
		case n > 0 && p.confirm:
			line = st.bad.Render("Press Enter again to replace them")
		case n == 1 && len(copies) == 1:
			line = st.warn.Render(fmt.Sprintf("%s holds %q — it will be replaced", target, m.snap.Pads[copies[0].To.Index()].Name))
		case n > 0:
			line = st.warn.Render(fmt.Sprintf("%d of the %d pads it lands on hold samples — they will be replaced", n, len(copies)))
		case len(copies) > 1:
			line = st.dim.Render(fmt.Sprintf("%s to %s, all free", copies[0].To, copies[len(copies)-1].To))
		}
	}
	typed := ""
	if p.digits != "" {
		typed = st.accent.Render("  pad " + p.digits + "…")
	}
	return st.modal.Render(st.bold.Render(title) + "\n\n" + strip.String() + typed + "\n\n" + m.pickerGrid() + "\n\n" + line +
		"\n\n" + st.dim.Render("A-J bank  ·  1-16 pad  ·  hjkl move  ·  [ ] bank  ·  Enter copy  ·  Esc cancel"))
}

func (m Model) bankCountsForPicker() [proto.Banks]int {
	var counts [proto.Banks]int
	for _, row := range m.snap.Pads {
		if row.HasSample {
			counts[row.Pad.Bank()]++
		}
	}
	return counts
}

// pickerGrid is a small pad grid for choosing a target, showing what is on each pad.
func (m Model) pickerGrid() string {
	st, p := m.st, m.picker
	const w = 14
	rows := make([]string, 4)
	for row := 0; row < 4; row++ {
		var cells []string
		for col := 0; col < 4; col++ {
			pos := row*4 + col
			pad, _ := proto.PadFromBankPad(uint16(p.bank), uint16(pos))
			r := m.snap.Pads[pad.Index()]
			label := fmt.Sprintf("%2d %s", pos+1, r.Name)
			style := lipgloss.NewStyle().Width(w).Background(st.th.tile).Foreground(st.th.text)
			switch {
			case pos == p.pad:
				style = style.Background(st.th.cursorBg).Foreground(st.th.cursorFg).Bold(true)
			case !r.HasSample:
				style = style.Background(st.th.tileEmpty).Foreground(st.th.faint)
				label = fmt.Sprintf("%2d", pos+1)
			}
			cells = append(cells, style.Render(ansi.Truncate(" "+label, w, "…")))
		}
		rows[row] = strings.Join(cells, " ")
	}
	return strings.Join(rows, "\n")
}

// folderFinderBox is the whole-dialog folder finder, laid out like the file search: the
// text typed, then the open folders matching it as two columns (name, and the folder it is
// in) with the chosen one highlighted, and its full path below.
func (m Model) folderFinderBox(g geom, rows []app.Node, ix *app.Index, input string, choice int, title string) string {
	st := m.st
	width := min(m.width-6, 112)
	height := max(min(g.bodyH-9, 20), 5)
	matches := folderChoices(rows, ix, input)
	choice = clamp(choice, 0, max(len(matches)-1, 0))

	status := st.dim.Render(fmt.Sprintf("%d of %d folders", len(matches), len(folderChoices(rows, ix, ""))))
	if ix == nil {
		status += st.faint.Render("  · only the open ones: i indexes folders so that all are found")
	}
	var list []string
	from, to := window(len(matches), choice, height-1)
	nameW := 12
	for i := from; i < to; i++ {
		nameW = max(nameW, ansi.StringWidth(matches[i].Name)+1)
	}
	nameW = min(nameW, (width-6)*55/100)
	dirW := max(width-nameW-8, 6)
	if to > from {
		list = append(list, st.faint.Render(" "+padTo("Name", nameW)+"  Folder"))
	}
	for i := from; i < to; i++ {
		r := matches[i]
		name := ansi.Truncate(r.Name+"/", nameW, "…")
		dir := cutStart(display(app.Dir(r.Path)), dirW)
		if i == choice {
			list = append(list, lipgloss.NewStyle().Background(st.th.cursorBg).Foreground(st.th.cursorFg).Bold(true).Width(width-4).Render(" "+padTo(name, nameW)+"  "+dir))
			continue
		}
		list = append(list, st.accent.Render(" "+padTo(name, nameW))+"  "+st.dim.Render(dir))
	}
	if len(matches) == 0 {
		list = []string{st.dim.Render(" no folder matches")}
	}
	full := ""
	if len(matches) > 0 {
		full = st.dim.Render(ansi.Truncate(display(matches[choice].Path), width-4, "…"))
	}
	lines := []string{
		st.bold.Render(title), "",
		st.accent.Render("> ") + input + "█", status, "",
		fit(strings.Join(list, "\n"), height), full,
		st.dim.Render("Enter jump  ·  ↑↓ choose  ·  then h or Tab collapses it  ·  Esc back"),
	}
	return st.modal.Width(width).Render(strings.Join(lines, "\n"))
}

// busyBox is the spinner and what is being waited for, with a bar under it when the
// operation reports how far it has come.
func (m Model) busyBox() string {
	line := m.st.accent.Render(spinner[m.frame%len(spinner)]) + " " + m.st.text.Render(m.busy+"…")
	inner := min(max(m.width-14, 24), 64)
	var rows []string
	switch bar := m.prog.bar(inner); {
	case bar != "":
		rows = []string{line, "", m.st.accent.Render(bar), ""}
		// The steps done so far, the newest at the bottom, then the one in hand.
		for _, l := range m.prog.recent(max(min(m.geom().bodyH-12, 10), 1)) {
			rows = append(rows, m.st.dim.Render(clip(l, inner)))
		}
		if cur := m.prog.file(inner); cur != "" {
			rows = append(rows, m.st.text.Render("▸ "+cur))
		}
	case m.prog.sweepBar(inner, m.frame) != "":
		rows = []string{line, "", m.st.accent.Render(m.prog.sweepBar(inner, m.frame)), ""}
	default:
		return line
	}
	return m.st.modal.Width(inner + 6).Render(strings.Join(rows, "\n"))
}

// clip shortens a line to width cells, keeping its end, where the pad or the tempo is.
func clip(s string, width int) string {
	if r := []rune(s); len(r) > width && width > 1 {
		return "…" + string(r[len(r)-width+1:])
	}
	return s
}

// sideColumn stacks the detail panel, the session panel and the log history, which takes
// the rows that are left.
func (m Model) sideColumn(g geom, detail string) string {
	session := m.session(g)
	column := lipgloss.JoinVertical(lipgloss.Left, detail, session)
	left := g.bodyH - g.top - lipgloss.Height(column)
	if left < 5 {
		return column
	}
	return lipgloss.JoinVertical(lipgloss.Left, column, m.historyBox(g, min(left, 14)))
}

// historyBox lists what has happened, the newest at the bottom.
func (m Model) historyBox(g geom, height int) string {
	st := m.st
	room := height - 2
	entries := m.history[max(len(m.history)-room, 0):]
	lines := make([]string, 0, room)
	for _, e := range entries {
		text := st.text
		if strings.Contains(e.text, "failed") || strings.Contains(e.text, "not deleted") || strings.Contains(e.text, "did not") {
			text = st.bad
		}
		lines = append(lines, st.faint.Render(e.at.Format("15:04:05"))+"  "+text.Render(e.text))
	}
	if len(lines) == 0 {
		lines = []string{st.faint.Render("nothing yet")}
	}
	return m.box("History", strings.Join(lines, "\n"), g.detailW, height, false)
}

// keyGroup is a line of keys in a guide: what they are for, and the keys with what each does.
type keyGroup struct {
	label string
	keys  [][2]string
}

// filesKeyGuide groups the keys of the Files view by what they are for.
var filesKeyGuide = []keyGroup{
	{"Assign", [][2]string{{"Enter", "assign"}, {"v", "mark"}, {"V", "mark folder"}, {"J", "mark junk"}, {"u", "unmark"}}},
	{"Edit", [][2]string{{"c", "copy"}, {"x", "move"}, {"p", "paste"}, {"d", "delete"}, {"n", "rename"}, {"N", "new folder"}}},
	{"Tag", [][2]string{{"*", "favourite"}, {"+", "add tags (-word removes)"}}},
	{"Find", [][2]string{{"/", "names"}, {"s", "tags"}, {"f", "folder"}, {"i", "index folder"}, {"r", "refresh"}}},
	{"View", [][2]string{{"Space", "listen"}, {"^A", "autoplay"}, {"[ ]", "target bank"}, {"t", "source"}, {"Tab", "open"}, {"h", "close"}}},
}

// withKeyGuide puts a guide of the keys at the bottom of a panel that is height rows tall
// and width wide, when there is room left under the text.
func (m Model) withKeyGuide(text string, groups []keyGroup, height, width int) string {
	st := m.st
	var guide []string
	for _, grp := range groups {
		line := ""
		first := true
		emit := func() {
			label := strings.Repeat(" ", 8)
			if first {
				label = padTo(grp.label, 8)
			}
			guide = append(guide, st.faint.Render(label)+line)
			line, first = "", false
		}
		for _, k := range grp.keys {
			item := st.accent.Render(k[0]) + st.dim.Render(" "+k[1])
			if line != "" && ansi.StringWidth(line)+ansi.StringWidth(item)+2 > width-8 {
				emit()
			}
			if line != "" {
				line += "  "
			}
			line += item
		}
		emit()
	}
	used := strings.Count(text, "\n") + 1
	if used+len(guide)+1 > height {
		return text
	}
	blank := strings.Repeat("\n", height-used-len(guide))
	return text + blank + strings.Join(guide, "\n")
}
