package tui

import (
	"fmt"
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
	{"/", "find by name"},
	{"Enter (Pads)", "preview the pad on the unit"},
	{"a (Pads)", "pick a file to assign to the pad"},
	{"n / d (Pads)", "rename / delete the sample"},
	{"v / V (Pads)", "mark a pad / every pad with a sample"},
	{"x, then p or s", "pick up a pad's sample, then move or swap it onto another"},
	{"e / R (Pads)", "export the bank / restore an export"},
	{"Enter (Patterns)", "open the pattern; b retargets its pads to another bank"},
	{"a (Pads)", "file dialog for this pad; it opens where you left it"},
	{"Enter, a (Files)", "assign the file: picker opens on the target bank's next free pad"},
	{"[ ] (Files)", "choose the target bank for assigns"},
	{"A-J, 1-16 (picker)", "type a bank letter and a pad number: C, 7, Enter"},
	{"e / R (Projects)", "export the project / restore one from a backup folder"},
	{"Space (Files)", "listen to the file on this computer"},
	{"v (Files)", "mark audio files; Enter then fills free pads in order"},
	{"Tab / l / h (Files)", "open or close a folder, go to the parent"},
	{"n, m then p (Files)", "rename; mark then move into the open folder"},
	{"R (Files)", "restore pads from this folder's export"},
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
	case m.browser != nil:
		main = m.center(m.browserBox(g), g.bodyH)
	case m.prompt != nil:
		main = m.center(m.promptBox(), g.bodyH)
	case m.picker != nil:
		main = m.center(m.pickerBox(), g.bodyH)
	case m.palette != nil:
		main = m.center(m.paletteBox(), g.bodyH)
	case m.busy != "":
		main = m.center(m.st.accent.Render(spinner[m.frame%len(spinner)])+" "+m.st.text.Render(m.busy+"…"), g.bodyH)
	case m.snap == nil:
		main = m.center(m.st.dim.Render("no data from the unit; r tries again"), g.bodyH)
	default:
		main = m.withRail(g, fit(m.body(g), g.bodyH))
	}
	screen := strings.Join([]string{m.header(), m.rule(), fit(main, g.bodyH), m.statusBar()}, "\n")
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

	right := st.dim.Render(m.snap.Port)
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
	switch m.section {
	case secPads:
		chips = []string{chip("Enter", "preview"), chip("a", "assign"), chip("v", "mark"), chip("n", "rename"), chip("d", "delete"), chip("x", "move"), chip("e", "export"), chip("[ ]", "bank")}
	case secPatterns:
		chips = []string{chip("Enter", "open"), chip("b", "retarget"), chip("[ ]", "bank"), chip("Esc", "back")}
	case secFiles:
		chips = []string{chip("Enter", "assign"), chip("[ ]", "target bank"), chip("Space", "listen"), chip("v", "mark"), chip("Tab", "open"), chip("n", "rename"), chip("m", "move"), chip("t", "source")}
	case secProjects:
		chips = []string{chip("Enter", "select"), chip("n", "rename"), chip("e", "export"), chip("R", "restore")}
	}
	chips = append(chips, chip(":", "commands"), chip("?", "help"), chip("q", "quit"))
	line := strings.Join(chips, st.faint.Render("  "))
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
	if g.side {
		return lipgloss.JoinHorizontal(lipgloss.Top, grid, "  ", lipgloss.JoinVertical(lipgloss.Left, detail, m.session(g)))
	}
	if g.detailH < 4 {
		return grid
	}
	return grid + "\n\n" + detail
}

// padDetail is the panel under (or beside) the grid: the selected pad's waveform and settings.
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
	inner := g.detailW - 4
	var lines []string
	if !row.HasSample {
		lines = []string{st.dim.Render("No sample on this pad."), "", st.dim.Render("a  picks a file for it    n  names it")}
	} else {
		lines = append(lines, m.waveform(row, inner)...)
		flag := func(label string, on bool) string {
			if on {
				return st.good.Render("● ") + st.text.Render(label)
			}
			return st.faint.Render("○ " + label)
		}
		lines = append(lines,
			st.dim.Render("Level ")+st.accent.Render(levelBar(12, int(row.Level), 127))+st.text.Render(fmt.Sprintf(" %-3d", row.Level))+
				st.dim.Render("  BPM ")+st.text.Render(fmt.Sprintf("%.2f", row.BPM))+
				st.dim.Render("  Size ")+st.text.Render(fmt.Sprintf("%.1f KB", float64(row.Size)/1024)),
			flag("Gate", row.Gate)+"  "+flag("Loop", row.Loop)+"  "+flag("BPM sync", row.BPMSync)+
				st.dim.Render(fmt.Sprintf("  Chops %d", row.Chops)))
	}
	return m.box(title, strings.Join(lines, "\n"), g.detailW, max(g.detailH, 5), false)
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
	if m.fileMoved != "" {
		lines = append(lines, st.warn.Render("Moving "+app.Base(m.fileMoved)))
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

// waveform is the pad's waveform as two rows of block characters, or a note when it is
// not there.
func (m Model) waveform(row app.PadRow, width int) []string {
	st := m.st
	peaks, have := m.waves[waveKey{row.Pad, row.Size, row.Name}]
	switch {
	case !have:
		return []string{st.faint.Render("loading waveform…"), ""}
	case len(peaks) == 0:
		return []string{st.faint.Render("(no waveform)"), ""}
	}
	amps := make([]int, len(peaks))
	for i, p := range peaks {
		amps[i] = max(abs16(p.Min), abs16(p.Max))
	}
	return m.bars(amps, width)
}

func abs16(v int16) int {
	if v < 0 {
		return -int(v)
	}
	return int(v)
}

// bars draws amplitudes (0..32768) as two rows of block characters, scaled to width.
func (m Model) bars(amps []int, width int) []string {
	const blocks = " ▁▂▃▄▅▆▇█"
	runes := []rune(blocks)
	peak := 1
	for _, a := range amps {
		peak = max(peak, a)
	}
	var top, bottom strings.Builder
	for c := 0; c < width; c++ {
		lo, hi := c*len(amps)/width, max((c+1)*len(amps)/width, c*len(amps)/width+1)
		a := 0
		for _, v := range amps[lo:min(hi, len(amps))] {
			a = max(a, v)
		}
		level := a * 16 / peak
		bottom.WriteRune(runes[min(level, 8)])
		top.WriteRune(runes[max(level-8, 0)])
	}
	return []string{m.st.accent.Render(top.String()), m.st.accent.Render(bottom.String())}
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
		name, state := "", tileState{empty: !row.Exists, cursor: p == m.patternCursor}
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
		lines = []string{m.st.text.Render("Plays " + strings.Join(names, " ")), "", m.st.dim.Render("Enter opens the step grid")}
	}
	detail := m.box("Pattern "+cur.Slot.String(), strings.Join(lines, "\n"), g.detailW, max(g.detailH, 5), false)
	if g.side {
		return lipgloss.JoinHorizontal(lipgloss.Top, grid, "  ", lipgloss.JoinVertical(lipgloss.Left, detail, m.session(g)))
	}
	if g.detailH < 4 {
		return grid
	}
	return grid + "\n\n" + detail
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
	detail := m.box("Details", m.fileDetail(g.bodyH-2, detailW-2), detailW, g.bodyH, false)
	return lipgloss.JoinHorizontal(lipgloss.Top, list, " ", detail)
}

func (m Model) treeLines(height int) string {
	return m.treeRender(m.tree, m.fileAt, height, func(n app.Node) string {
		switch {
		case m.fileMarks[n.Path]:
			return m.st.good.Render("✓")
		case n.Path == m.fileMoved:
			return m.st.warn.Render("↗")
		case n.Path == m.listening:
			return m.st.hot.Render("♪")
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
	var lines []string
	free, _ := proto.PadFromBankPad(uint16(m.target), uint16(m.nextFree(m.target)))
	lines = append(lines, st.dim.Render("Target  ")+st.accent.Render(fmt.Sprintf("bank %c · next free %s", 'A'+m.target, free)), st.dim.Render("[ ] changes it; it follows the last pad you assigned"), "")
	if m.fileMoved != "" {
		lines = append(lines, st.warn.Render("Moving: "+m.fileMoved), "")
	}
	node, ok := m.fileNode()
	if !ok {
		return strings.Join(lines, "\n")
	}
	kind := "File"
	if node.Dir {
		kind = "Folder"
	}
	lines = append(lines, st.accent.Render(node.Name), st.dim.Render(kind+" · "+display(node.Path)), "")
	switch {
	case isAudio(node.Name):
		lines = append(lines, m.audioInfo(node, width)...)
	case node.Name == app.Sidecar:
		lines = append(lines, st.good.Render("Pad export"), st.dim.Render("Enter or R restores it onto the pads it lists"))
	case node.Dir:
		lines = append(lines, st.dim.Render("R restores pads from an export in this folder"))
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
	lines = append(lines, m.bars(amps, max(width-2, 10))...)
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
	case promptSearch:
		title = "Find by name"
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
	return st.modal.Width(min(m.width-4, 72)).Render(st.bold.Render(title) + "\n\n" + st.accent.Render("> ") + p.buffer + "█" + "\n\n" + st.dim.Render(hint+"Enter apply  ·  Esc cancel"))
}

// pickerBox is the pad chooser of an assign, or the bank chooser of a retarget.
func (m Model) pickerBox() string {
	st, p := m.st, m.picker
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
