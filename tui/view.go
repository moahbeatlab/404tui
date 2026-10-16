package tui

import (
	"cmp"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
	"github.com/moahbeatlab/404tui/version"
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
	{"Z (Pads, Files)", "slicer: cut a sample into slices on a waveform (equal, random, onsets, or tapped while it plays), put samples together with A, play the slices in an order or at random, roll, swing, then b bounces takes onto pads; the keys are listed in the dialog"},
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
	m = m.withBusColours()
	g := m.geom()
	// A dialog is drawn in the middle of the screen it belongs to, over it, not instead of it.
	dialog := func(box string) string {
		base := ""
		if m.unit != nil && m.snap != nil {
			base = m.withRail(g, fit(m.body(g), g.bodyH))
		}
		return overlay(base, box, m.width, g.bodyH)
	}
	var main string
	switch {
	case m.help:
		main = dialog(m.helpBox())
	case m.report != nil:
		main = dialog(m.reportBox())
	case m.palette != nil:
		main = dialog(m.paletteBox())
	case m.addAsk != nil:
		main = dialog(m.addAskBox())
	case m.search != nil:
		main = dialog(m.searchBox(g))
	case m.editor != nil:
		main = dialog(m.editorBox())
	case m.chromatic != nil:
		main = dialog(m.chromaticBox())
	case m.browser != nil:
		main = dialog(m.browserBox(g))
	case m.prompt != nil:
		main = dialog(m.promptBox())
	case m.picker != nil:
		main = dialog(m.pickerBox())
	case m.slicer != nil && !m.slicer.adding && m.section == secSlicer && m.busy == "":
		main = m.center(m.slicerBox(), g.bodyH)
	case m.toggles != nil:
		main = dialog(m.togglesBox())
	case m.lenPick != nil:
		main = dialog(m.lengthPickBox())
	case m.tmplPicker != nil:
		main = dialog(m.templatePickerBox())
	case m.rangePick != nil:
		main = dialog(m.rangePickerBox())
	case m.laneMap != nil:
		main = dialog(m.laneAssignBox())
	case m.palette != nil:
		main = dialog(m.paletteBox())
	case m.unit == nil:
		main = m.center(m.waitingBox(), g.bodyH)
	case m.busy != "" && m.snap == nil:
		// The first read of the unit: the dashboard, with what is being read under it.
		main = m.center(m.dashboard([]string{m.busyBox()}), g.bodyH)
	case m.busy != "":
		main = dialog(m.busyBox())
	case m.snap == nil:
		main = m.center(m.st.dim.Render("no data from the unit; r tries again"), g.bodyH)
	default:
		main = m.withRail(g, fit(m.body(g), g.bodyH))
	}
	parts := []string{m.header(), m.tabBar(), fit(main, g.bodyH)}
	if m.tagJob.active() {
		parts = append(parts, m.indexBar())
	}
	screen := strings.Join(append(parts, m.statusBar()), "\n")
	return lipgloss.NewStyle().MaxWidth(m.width).Render(screen)
}

// overlay draws box in the middle of base, a screen of width by height cells, over what is
// there: the lines of the box replace the same cells of the lines of the screen, and the rest
// of the screen stays as it was.
func overlay(base, box string, width, height int) string {
	lines := strings.Split(base, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	boxLines := strings.Split(box, "\n")
	boxW := 0
	for _, l := range boxLines {
		boxW = max(boxW, ansi.StringWidth(l))
	}
	top, left := max((height-len(boxLines))/2, 0), max((width-boxW)/2, 0)
	for i, l := range boxLines {
		y := top + i
		if y >= len(lines) {
			break
		}
		line := lines[y]
		if w := ansi.StringWidth(line); w < width {
			line += strings.Repeat(" ", width-w)
		}
		l += strings.Repeat(" ", boxW-ansi.StringWidth(l))
		lines[y] = ansi.Truncate(line, left, "") + l + ansi.TruncateLeft(line, left+boxW, "")
	}
	return strings.Join(lines, "\n")
}

// withRail puts the navigation rail to the left of the body, with a rule between them.
func (m Model) withRail(g geom, body string) string {
	if g.railW == 0 {
		return " " + strings.ReplaceAll(body, "\n", "\n ")
	}
	rail := m.railView(g)
	rule := strings.TrimSuffix(strings.Repeat(m.st.faint.Render("│")+"\n", g.bodyH), "\n")
	return lipgloss.JoinHorizontal(lipgloss.Top, rail, rule, "  ", body)
}

// How the tabs sit on the row under the header, when the header has no room for them: each
// tab's label with room around it, and the rule between two tabs.
const (
	tabGap    = 3
	tabFormat = "  %d %s  "
)

// In the header the tabs are closer: a label with a space around it, and a gap between.
const (
	headTabFormat = " %d %s "
	headTabGap    = 2
)

// tabLayout is the row the tabs are on and the columns each covers, as start and end (end not
// in). They are right-aligned: in the header when it has the room after the project and the
// state of the link, else on the row under it. Before there is a unit to show, there are none.
func (m Model) tabLayout() (row int, spans [][2]int) {
	if m.unit == nil || m.snap == nil {
		return rowRule, nil
	}
	place := func(format string, gap int) (spans [][2]int, total int) {
		for i, s := range railSections {
			total += len(fmt.Sprintf(format, i+1, sectionNames[s]))
			if i > 0 {
				total += gap
			}
		}
		x := m.width - total - 1
		for i, s := range railSections {
			w := len(fmt.Sprintf(format, i+1, sectionNames[s]))
			spans = append(spans, [2]int{x, x + w})
			x += w + gap
		}
		return spans, total
	}
	left, status := m.headerParts()
	if badge := m.recBadge(); badge != "" {
		status = badge + "  " + status
	}
	spans, total := place(headTabFormat, headTabGap)
	if ansi.StringWidth(left)+2+ansi.StringWidth(status)+3+total+1 <= m.width {
		return rowHeader, spans
	}
	spans, _ = place(tabFormat, tabGap)
	return rowRule, spans
}

// tabAt is the section of the tab at column x of row y.
func (m Model) tabAt(x, y int) (int, bool) {
	row, spans := m.tabLayout()
	if y != row {
		return 0, false
	}
	for i, span := range spans {
		if x >= span[0] && x < span[1] {
			return railSections[i], true
		}
	}
	return 0, false
}

// activeTab is the section whose tab is lit: the Sequencer's for a pattern it opened, the
// Pads' for the patterns panel under them.
func (m Model) activeTab() int {
	switch {
	case m.section == secPatterns && m.seqTab:
		return secSequencer
	case m.section == secPatterns:
		return secPads
	}
	return m.section
}

// tabsText draws the tabs at the columns spans give, the one in front lit, joined by the
// rule when they are on their own row (compact is for the header).
func (m Model) tabsText(spans [][2]int, compact bool) string {
	st := m.st
	var b strings.Builder
	for i, s := range railSections {
		format, gap := tabFormat, tabGap
		if compact {
			format, gap = headTabFormat, headTabGap
		}
		text := fmt.Sprintf(format, i+1, sectionNames[s])
		if s == m.activeTab() {
			b.WriteString(lipgloss.NewStyle().Background(st.th.accent).Foreground(st.th.cursorFg).Bold(true).Render(text))
		} else {
			b.WriteString(st.dim.Render(text))
		}
		if i < len(railSections)-1 {
			if compact {
				b.WriteString(strings.Repeat(" ", gap))
			} else {
				b.WriteString(st.faint.Render(strings.Repeat("─", gap)))
			}
		}
	}
	return b.String()
}

// tabBar is the line under the header: the tabs with a rule across the rest of the width when
// the header has no room for them, else only the rule.
func (m Model) tabBar() string {
	row, spans := m.tabLayout()
	if row == rowHeader || len(spans) == 0 {
		return m.rule()
	}
	tabs := m.tabsText(spans, false)
	rest := max(spans[0][0], 0)
	return m.st.faint.Render(strings.Repeat("─", rest)) + tabs + m.st.faint.Render("─")
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
	case secSlicer:
		return m.slicerTabBody(g)
	case secConfig:
		return m.configBody(g)
	case secPerform:
		return m.perfBody(g)
	case secSequencer:
		return m.sequencerTabBody(g)
	}
	return m.projectsBody(g)
}

// ---- chrome ----

func (m Model) header() string {
	left, status := m.headerParts()
	if badge := m.recBadge(); badge != "" {
		// A recording is shown first, where it cannot be missed, whatever else the header says.
		status = badge + "  " + status
	}
	line := left
	if status != "" {
		line += "  " + status
	}
	if row, spans := m.tabLayout(); row == rowHeader && len(spans) > 0 {
		// The tabs are in the top right corner; the state of the link is after the project.
		line += strings.Repeat(" ", max(spans[0][0]-ansi.StringWidth(line), 1)) + m.tabsText(spans, true)
	}
	return ansi.Truncate(line, m.width, "…")
}

// headerParts are the name and project, and after them the state of the link and the version.
// Before there is a unit to show there is only the name.
func (m Model) headerParts() (left, right string) {
	st := m.st
	badge := lipgloss.NewStyle().Background(st.th.accent).Foreground(st.th.cursorFg).Bold(true).Render(" 404tui by moahbeatlab ")
	ver := st.faint.Render("v" + version.Version)
	if m.unit == nil {
		return badge, st.warn.Render("○ no unit") + "  " + ver
	}
	if m.snap == nil {
		return badge, ver
	}
	label := fmt.Sprintf("  PROJECT %d", m.snap.Project+1)
	if m.snap.ProjectName != "" {
		label = "  " + m.snap.ProjectName
	}
	left = badge + st.bold.Render(label)
	if m.offline == "" {
		free := float64(m.snap.FreeKb) / 1024 / 1024
		left += st.dim.Render(fmt.Sprintf("   %.1f GB free", free))
	}

	// For a few seconds after connecting, say so loudly.
	right = st.good.Render("●") + " " + st.dim.Render(m.snap.Port)
	if m.connect != nil && m.frame-m.connectedFrame < 50 {
		right = st.good.Render("● CONNECTED  " + m.snap.Port)
	}
	if m.offline != "" {
		// Offline is never easy to miss: a badge in the header for as long as it lasts.
		offline := lipgloss.NewStyle().Background(st.th.warn).Foreground(st.th.cursorFg).Bold(true).Render(" ◌ OFFLINE ")
		left = badge + " " + offline + st.bold.Render(label)
		right = st.warn.Render("no unit · saving to " + ansi.Truncate(filepath.Base(m.offline), 24, "…"))
		if m.unitBack {
			right = st.hot.Render("unit plugged in: U connects")
		}
	}
	if m.playing {
		right = st.hot.Render(fmt.Sprintf("▶ %s  ", m.playingPad)) + right
	} else if m.hasSelected {
		right = st.dim.Render(fmt.Sprintf("◆ %s  ", m.selected)) + right
	}
	return left, right + "  " + ver
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
		chips = []string{chip("Enter", "preview on "+map[bool]string{true: "computer", false: "unit"}[m.previewLocal]), chip("a", "assign"), chip("v", "mark"), chip("t", "set"), chip("c", "copy"), chip("d", "delete"), chip("B", "buses by name"), chip("K", "check sample"), chip("Tab", "patterns"), chip("[ ]", "bank")}
	case secPatterns:
		if m.seqTab {
			chips = []string{chip("a", "other pattern"), chip("P", "play"), chip("Esc", "close")}
			break
		}
		chips = []string{chip("Enter", "open"), chip("N", "new"), chip("Space", "play / stop"), chip("d", "delete"), chip("c", "copy"), chip("[ ]", "bank")}
	case secFiles:
		if len(m.fileMarks) > 0 || len(m.clip) > 0 {
			chips = []string{chip("Enter", "assign"), chip("d", "delete"), chip("c", "copy"), chip("x", "move"), chip("p", "paste"), chip("u", "unmark"), chip("Esc", "clear")}
			break
		}
		chips = []string{chip("Enter", "assign"), chip("v", "mark"), chip("*", "favourite"), chip("/", "search"), chip("s", "tags"), chip("f", "folder"), chip("i", "index"), chip("w", "view"), chip("Space", "listen")}
	case secSlicer:
		chips = []string{chip("a", "choose a pad or a sample")}
	case secSequencer:
		chips = []string{chip("a", "choose a pattern"), chip("N", "new pattern")}
	case secConfig:
		chips = []string{chip("j k", "move"), chip("h l", "change"), chip("Enter", "edit"), chip("Space", "toggle")}
	case secPerform:
		chips = []string{chip("a", "now"), chip("n", "next"), chip("Space", "next becomes now"), chip("x", "clear next"), chip("f F", "bus effect notes")}
	case secProjects:
		chips = []string{chip("Enter", "select"), chip("n", "rename"), chip("e", "export"), chip("R", "restore"), chip("X", "erase")}
	}
	switch {
	case m.addAsk != nil && m.addAsk.sec == secSlicer:
		chips = []string{chip("p", "pad"), chip("f", "new sample"), chip("Esc", "cancel")}
	case m.addAsk != nil && m.addAsk.chooser():
		chips = []string{chip("Enter", "take it"), chip("↑ ↓", "move"), chip("[ ]", "bank"), chip("Esc", "back")}
	case m.addAsk != nil:
		chips = []string{chip("s", "sample"), chip("p", "pattern"), chip("Esc", "cancel")}
	case m.sliceWant:
		chips = []string{chip("Z", "slice the file or pad under the cursor"), chip("1 2", "pads or files"), chip("Esc", "cancel")}
	}
	rec := chip("^R", "record")
	if m.rec != nil {
		rec = chip("^R", "stop recording")
	}
	chips = append(chips, rec, chip("?", "all keys"), chip("q", "quit"))
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
	message := m.message
	if message == "" && m.slicer != nil && m.slicer.adding {
		message = slicerAddHint
	}
	if message != "" {
		line = st.warn.Render(message) + st.faint.Render("  │  ") + line
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
	// bus is the bus the pad goes to (1 or 2); 0 for none.
	bus int32
	// flags are the pad's settings as tiny letters, each lit when it is on (see padFlags).
	flags []padFlag
	// muted and solo are the pad's state in the mix played on this computer.
	muted, solo bool
	// noLevel leaves out the level bar under the name: a pattern slot has no level. progress,
	// from 0 to 1, draws how far through its round it is instead; below 0 there is none.
	noLevel  bool
	progress float64
	// bars is how long the pad's sample is in bars at the bank's tempo, when it is a loop of
	// them; empty otherwise.
	bars string
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
	// A pad on a bus gets its colour on the bar under the name and a small badge in the corner.
	// The pieces of a line are styled without the tile's width, which would pad each of them.
	piece := lipgloss.NewStyle().Background(bg).Foreground(fg)
	pieceBold := piece.Bold(true)
	busStyle, badge := pieceBold, ""
	busColour, onBus := th.busColour(ts.bus)
	if onBus && !ts.empty {
		busStyle = pieceBold.Foreground(busColour)
		if ts.cursor || ts.playing {
			// On the bright tiles the colour would be lost: the badge keeps the dark text.
			busStyle = pieceBold
		}
		badge = busBadge(ts.bus)
	}
	// Muted or soloed in the mix on this computer: a mark before the others.
	mixMark := ""
	switch {
	case ts.muted:
		mixMark = "×"
	case ts.solo:
		mixMark = "S"
	}
	right := badge
	if badge != "" && flags != "" {
		right += " "
	}
	right += flags
	if mixMark != "" && !ts.empty {
		right = mixMark + " " + right
	} else {
		mixMark = ""
	}
	// The length in bars goes before the other corner marks; its place is kept in right.
	barsText := ""
	if ts.bars != "" && !ts.empty {
		barsText = ts.bars
		right = strings.Repeat(" ", ansi.StringWidth(barsText)+1) + right
	}
	// line puts text at the left and the corner marks at the right, to exactly the tile's width.
	// A setting that is on is lit, one that is off is faint, so the letters are always there.
	onStyle := lipgloss.NewStyle().Background(bg).Foreground(th.good).Bold(true)
	offStyle := lipgloss.NewStyle().Background(bg).Foreground(th.faint)
	if ts.cursor || ts.playing {
		onStyle, offStyle = pieceBold, piece.Faint(true)
	}
	line := func(left string) string {
		// The pad's settings sit after the number, as many as there is room for.
		extra, extraW := "", 0
		if len(ts.flags) > 0 && !ts.empty && h < 4 {
			room := w - ansi.StringWidth(left) - ansi.StringWidth(right) - 3
			for i, f := range ts.flags {
				sep := " "
				if i == 0 {
					sep = "  "
				}
				if extraW+len(sep)+ansi.StringWidth(f.letter) > room {
					break
				}
				style := offStyle
				if f.on {
					style = onStyle
				}
				extra += piece.Render(sep) + style.Render(f.letter)
				extraW += len(sep) + ansi.StringWidth(f.letter)
			}
		}
		gap := max(w-ansi.StringWidth(left)-extraW-ansi.StringWidth(right)-1, 1)
		out := pieceBold.Render(left) + extra + piece.Render(strings.Repeat(" ", gap))
		if barsText != "" {
			barsStyle := piece.Foreground(th.dim)
			if ts.cursor || ts.playing {
				barsStyle = piece.Faint(true)
			}
			out += barsStyle.Render(barsText) + piece.Render(" ")
		}
		if mixMark != "" {
			markStyle := lipgloss.NewStyle().Background(bg).Foreground(th.bad).Bold(true)
			if ts.solo {
				markStyle = markStyle.Foreground(th.good)
			}
			if ts.cursor || ts.playing {
				markStyle = pieceBold
			}
			out += markStyle.Render(mixMark) + piece.Render(" ")
		}
		if badge != "" {
			out += busStyle.Render(badge)
			if flags != "" {
				out += piece.Render(" ")
			}
		}
		if flags != "" {
			out += pieceBold.Render(flags)
		}
		return out + piece.Render(" ")
	}
	numLine := line(" " + num)
	if right == "" && (len(ts.flags) == 0 || h >= 4) {
		numLine = bold.Render(" " + num)
	}
	nameLine := " " + ansi.Truncate(name, w-2, "…")
	if ts.empty {
		nameLine = ""
	}
	bar := style.Render("")
	switch {
	case ts.empty:
	case ts.noLevel && ts.progress >= 0:
		bar = style.Render(" " + levelBar(w-2, int(ts.progress*1000), 1000))
	case ts.noLevel:
	default:
		fill := piece.Render(" ")
		barText := levelBar(w-2, int(level), 127)
		if onBus && !ts.cursor && !ts.playing {
			bar = fill + lipgloss.NewStyle().Background(bg).Foreground(busColour).Render(barText) + fill
		} else {
			bar = style.Render(" " + barText)
		}
	}
	if h == 2 {
		first := numLine
		if !ts.empty {
			first = line(" " + num + " " + ansi.Truncate(name, max(w-3-ansi.StringWidth(num)-ansi.StringWidth(right)-1, 1), "…"))
			if right == "" {
				first = bold.Render(" " + num + " " + ansi.Truncate(name, max(w-3-ansi.StringWidth(num), 1), "…"))
			}
		}
		return first + "\n" + bar
	}
	if h >= 4 {
		// A row of its own for the settings, under the name.
		flagLine, used := piece.Render(" "), 1
		if !ts.empty {
			for i, f := range ts.flags {
				sep := ""
				if i > 0 {
					sep = " "
				}
				if used+len(sep)+ansi.StringWidth(f.letter) > w-1 {
					break
				}
				style := offStyle
				if f.on {
					style = onStyle
				}
				flagLine += piece.Render(sep) + style.Render(f.letter)
				used += len(sep) + ansi.StringWidth(f.letter)
			}
		}
		flagLine += piece.Render(strings.Repeat(" ", max(w-used, 0)))
		return numLine + "\n" + style.Render(nameLine) + "\n" + flagLine + "\n" + bar
	}
	return numLine + "\n" + style.Render(nameLine) + "\n" + bar
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
			bus:      row.Params["bus-fx"],
			flags:    padFlags(row),
			muted:    m.muted[row.Pad],
			solo:     m.soloed[row.Pad],
			cursor:   p == m.pad && m.section == secPads,
			marked:   m.marked[p],
			playing:  m.playing && m.playingPad == row.Pad,
			selected: m.hasSelected && m.selected == row.Pad,
			moving:   m.hasMoving && m.moving == row.Pad,
			bars:     m.padBars(row, g.tileW),
		})
	})
	onPatterns := m.section == secPatterns
	detail := m.padDetail(g)
	if onPatterns {
		detail = m.patternSlotDetail(g)
	}
	bar := m.bankBar() + "\n\n"
	if g.side {
		// The pads sit in a panel, and the patterns of the bank in another under it, as the
		// same kind of grid, when there is room for both.
		pads := m.box(fmt.Sprintf("Pads · bank %c", 'A'+m.bank), grid, g.gridW+2, lipgloss.Height(grid)+2, !onPatterns)
		left := pads
		if panel := m.patternPanel(g); panel != "" && g.bodyH-g.top-lipgloss.Height(pads) >= lipgloss.Height(panel) {
			left = lipgloss.JoinVertical(lipgloss.Left, pads, panel)
		} else if strip := m.patternStrip(g); strip != "" && g.bodyH-g.top-lipgloss.Height(pads) >= lipgloss.Height(strip) {
			// Not enough rows for the grid: the slots as a few short lines.
			left = lipgloss.JoinVertical(lipgloss.Left, pads, strip)
		}
		return bar + lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", m.sideColumn(g, detail))
	}
	if g.detailH < 4 {
		return bar + grid
	}
	return bar + grid + "\n\n" + detail
}

// patternPanel is the patterns of the bank on show as tiles, like the Patterns view's grid
// but not as tall, in a panel under the pads: a slot with how many pads its pattern plays,
// and the ones that play the pad in front marked. Until the patterns are read it is empty.
func (m Model) patternPanel(g geom) string {
	rows := m.patterns[m.bank]
	title := fmt.Sprintf("Patterns · bank %c", 'A'+m.bank)
	const tileH = 2
	if g.gridW < 40 {
		return ""
	}
	height := 4*tileH + 3 + 2
	if rows == nil {
		return m.box(title, m.st.faint.Render(" reading patterns…"), g.gridW+2, height, false)
	}
	used := map[proto.PadIndex]bool{}
	if m.usageState == 2 {
		for _, slot := range m.usage[m.padRow().Pad] {
			used[slot] = true
		}
	}
	grid := m.grid(g, func(p int) string {
		row := rows[p]
		name := ""
		if row.Exists {
			name = fmt.Sprintf("%d pads", len(row.Pads))
		}
		state := tileState{empty: !row.Exists, marked: used[row.Slot]}
		if m.section == secPatterns {
			// The patterns have the focus: the cursor is here, and what is marked for deleting
			// or picked up shows as in the Patterns view.
			state = tileState{empty: !row.Exists, cursor: p == m.patternCursor, marked: m.patMarks[int(row.Slot.Index())], moving: m.patHolding && m.patHeld == row.Slot}
		}
		// No level under a pattern; the one that plays is lit, with how far through its round it is.
		state.noLevel, state.progress = true, -1
		if frac, _, ok := m.loopAt(row.Slot); ok {
			state.playing, state.progress = true, frac
		}
		return m.tile(g.tileW, tileH, fmt.Sprint(p+1), name, 0, state)
	})
	if n := len(used); n > 0 {
		title += fmt.Sprintf(" · %s with this pad", plural(n, "pattern"))
	}
	return m.box(title, grid, g.gridW+2, height, m.section == secPatterns)
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
				if _, _, ok := m.loopAt(r.Slot); ok {
					text, style = fmt.Sprintf("%2s ▶%d", num, len(r.Pads)), st.hot
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
		m.busLegend() +
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
	// key is the letter a setting has on the pad tiles, in a column of its own.
	key := func(letter string) string { return st.accent.Render(fmt.Sprintf("%-3s", letter)) }
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
		if o, ok := m.origins.Of(m.snap.Project, row.Pad, row.Name); ok {
			sample = append(sample, label("From")+st.text.Render(cutStart(display(o.Path), max(inner-12, 20))), label("")+st.faint.Render(originWhen(o)))
		}
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
		// The letters the pad tiles use for these settings stand in front of them, so this
		// panel is also the key to the tiles.
		section("Playback",
			key("R~")+row2("Mode", playModeName(p("play-mode"))),
			key("G")+onOff("Gate", p("gate") != 0),
			key("L")+onOff("Loop", p("loop") != 0),
			key("O")+onOff("One shot", flags&0x20 != 0),
			key("S")+onOff("BPM sync", p("bpm-sync") != 0)),
		section("Sound",
			key("")+label("Level")+st.accent.Render(levelBar(10, int(row.Level), 127))+st.text.Render(fmt.Sprintf(" %d", row.Level)),
			key("")+row2("Pan", panText(p("balance"))),
			key("")+row2("Pitch", fmt.Sprintf("%+d st %+d ct", p("pitch-coarse"), p("pitch-fine"))),
			key("")+row2("Stretch", fmt.Sprintf("%.0f%%", float64(p("time-stretch"))/100)),
			key("V")+onOff("Vinyl", p("vinyl") != 0)),
		section("Routing",
			key("")+row2("Bus FX", busName(p("bus-fx"))),
			key("M")+row2("Mute grp", offOr(p("mute-group"))),
			key("")+row2("Pad link", offOr(p("pad-link"))),
			key("")+row2("Roll", fmt.Sprint(p("roll")))),
		section("Envelope",
			row2("Attack", fmt.Sprint(p("attack"))),
			row2("Hold", fmt.Sprint(p("hold"))),
			row2("Release", fmt.Sprint(p("release")))),
	}
	// As many groups per row as fit, each in a column of its own width.
	const colW = 30
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

// heldBox is a small panel for wide screens: the unit, its card, and what is picked up.
func (m Model) heldBox(g geom) string {
	st := m.st
	var lines []string
	if m.hasMoving {
		lines = append(lines, st.warn.Render(fmt.Sprintf("Picked up %s: p moves, s swaps", m.moving)))
	}
	if len(m.clip) > 0 {
		lines = append(lines, st.warn.Render(fmt.Sprintf("Clipboard: %s to %s", plural(len(m.clip), "item"), map[bool]string{true: "move", false: "copy"}[m.clipCut])))
	}
	if len(lines) == 0 {
		return ""
	}
	return m.box("Held", strings.Join(lines, "\n"), g.detailW, len(lines)+2, false)
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
	// With room beside it the patterns are the panel under the pads, with the focus on it.
	if g.side {
		return m.padsBody(g)
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
	detail := m.patternSlotDetail(g)
	bar := m.bankBar() + "\n\n"
	if g.detailH < 4 {
		return bar + grid
	}
	return bar + grid + "\n\n" + detail
}

// patternSlotDetail is the panel about the pattern slot in front: the pads it plays and what
// can be done to it.
func (m Model) patternSlotDetail(g geom) string {
	rows := m.patterns[m.bank]
	if rows == nil {
		return m.box("Pattern", m.st.faint.Render("reading patterns…"), g.detailW, max(g.detailH, 5), false)
	}
	cur := rows[m.patternCursor]
	st := m.st
	lines := []string{st.dim.Render("Empty slot."), "", st.dim.Render("N makes a pattern of the bank's samples in the first free slot")}
	if cur.Exists {
		lines = []string{st.faint.Render("reading the pattern…")}
		// The pattern in front is shown as its step grid, as Enter opens it.
		if d := m.patPreview[cur.Slot]; d != nil {
			info := fmt.Sprintf("%.2f BPM  ·  %d/4  ·  %d ticks  ·  %s", d.BankTempo, d.Beats, d.Length, plural(len(d.PadsUsed), "pad"))
			sp := m.patSpan(d, g.detailW-2)
			lines = []string{st.dim.Render(info), m.spanHeader(d, sp), "", m.stepRuler(d, sp)}
			if line := m.playhead(d, sp); line != "" {
				lines = append(lines, line)
			}
			if len(d.Grid.Rows) == 0 {
				lines = append(lines, st.dim.Render("  (no notes)"))
			}
			for _, row := range d.Grid.Rows {
				name := m.snap.Pads[row.Pad.Index()].Name
				label := fmt.Sprintf("%-4s %-14s", row.Pad, ansi.Truncate(name, 14, "…"))
				lines = append(lines, m.rowLabel(row.Pad, label, st.bold)+" "+m.noteRow(d, row.Pad, false, sp)+m.mixBadge(row.Pad))
			}
		}
		lines = append(lines, "", st.dim.Render("Enter edits the steps  ·  Space or P plays it in a loop, again stops it"), st.dim.Render("d delete  v mark  c copy  x move  p put"))
	}
	body := strings.Join(lines, "\n")
	return m.box("Pattern "+cur.Slot.String(), body, g.detailW, max(g.detailH, 5, strings.Count(body, "\n")+3, m.patternPanelRoom(g)), false)
}

// patternPanelRoom is the height the pattern panel keeps for the most a pattern can have: the 16
// pads of a bank as tracks, under their heading and over the keys. The panel stays that tall when
// the screen has the room (what is below it, the history, keeps a few lines), so it does not
// change size from one pattern to the next; on a low screen it is as tall as its pattern needs.
func (m Model) patternPanelRoom(g geom) int {
	const full = 2 + 4 + 1 + proto.PadsPerBank + 3 // frame, heading, a playhead line, tracks, keys
	room := g.bodyH - g.top
	if g.side {
		room -= 5 // the history under it
	} else {
		room -= g.gridH + 1
	}
	return min(full, room)
}

// patternDetail is an opened pattern: its step grid, one row per pad it plays.
func (m Model) patternDetail(g geom) string {
	st, d := m.st, m.pattern
	info := fmt.Sprintf("%.2f BPM  ·  %d/4  ·  %d ticks  ·  %d pads", d.BankTempo, d.Beats, d.Length, len(d.PadsUsed))
	// As many whole bars as the box is wide are on show; while it plays they follow the music.
	width := g.bodyW - 2
	sp := m.patSpan(d, width)
	lines := []string{st.dim.Render(info), m.spanHeader(d, sp), ""}
	lines = append(lines, m.stepRuler(d, sp))
	if line := m.playhead(d, sp); line != "" {
		lines = append(lines, line)
	}
	if len(d.Grid.Rows) == 0 {
		lines = append(lines, st.dim.Render("  (no notes)"))
	}
	// Each note is a block as tall as its velocity and as wide as it lasts, as the unit draws
	// them; the cursor is the step that is lit.
	for r, row := range d.Grid.Rows {
		name := m.snap.Pads[row.Pad.Index()].Name
		label := fmt.Sprintf("%-4s %-14s", row.Pad, ansi.Truncate(name, 14, "…"))
		labelStyle := st.bold
		if !slices.Contains(row.Hits, true) {
			// A pad of the bank with no note in the pattern: there to be given one.
			labelStyle = st.dim
		}
		if m.patFocus && r == m.patRow {
			// The expanded track: a lit label with a marker, three lines of steps on the grid's own
			// places, set off by a rule above and below; the other tracks fade (noteRow).
			lane := m.focusLane(d, row.Pad, sp)
			focused := fmt.Sprintf("▶ %-4s %-12s", row.Pad, ansi.Truncate(name, 12, "…"))
			lit := lipgloss.NewStyle().Background(st.th.cursorBg).Foreground(st.th.cursorFg).Bold(true)
			pad := strings.Repeat(" ", 20)
			rule := st.accent.Render(strings.Repeat("─", lipgloss.Width(lane[1])))
			// The name sits on the rule over the lane, like a heading for it.
			lines = append(lines, m.rowLabel(row.Pad, focused, lit)+" "+rule+m.mixBadge(row.Pad),
				pad+lane[0],
				pad+lane[1],
				pad+lane[2], pad+lane[3], pad+rule)
			continue
		}
		if !m.patFocus && r == m.patRow {
			// The track the cursor is on, the one Enter expands.
			labelStyle = st.accent.Bold(true)
		} else if m.patFocus {
			labelStyle = st.faint
		}
		lines = append(lines, m.rowLabel(row.Pad, label, labelStyle)+" "+m.noteRow(d, row.Pad, !m.patFocus && r == m.patRow, sp)+m.mixBadge(row.Pad))
	}
	if m.selecting() {
		lines = append(lines, "", st.hot.Render(m.selectionLine()))
	} else if pad, notes, ok := m.cellOfPattern(); ok {
		where := st.bold.Render(fmt.Sprintf("[%s · %s]", pad, m.patStepLabel()))
		if len(notes) > 0 {
			loudest := 0
			for _, n := range notes {
				loudest = max(loudest, int(n.Velocity))
			}
			length := float64(notes[0].Length) / float64(max(d.StepTicks(), 1))
			here := where + st.text.Render(fmt.Sprintf("  vel %d  ", loudest)) +
				st.accent.Render(levelBar(16, loudest, 127)) +
				st.text.Render(fmt.Sprintf("  ·  %s  ·  ", plural(int(math.Round(length)), "step"))) +
				m.offsetLabel(d, notes[0].Tick)
			// A note played chromatically on the unit has a pitch byte; what it means in
			// semitones is not known, so it is shown as it is.
			if pitch := notes[0].Pitch; pitch != 0 {
				here += st.dim.Render("  ·  ") + st.accent.Render(fmt.Sprintf("chromatic: pitch byte %d (0x%02X)", pitch, pitch))
			}
			lines = append(lines, "", here)
		} else {
			lines = append(lines, "", where+st.dim.Render("  no note  ·  Enter puts one"))
		}
	}
	if line := m.cycleLine(); line != "" {
		cond := ""
		if _, keys, ok := m.condKeysAt(); ok {
			cond = st.text.Render(m.patConds[keys[0]].String()) + st.dim.Render("  ·  ")
		}
		lines = append(lines, st.dim.Render("conditions  ")+cond+st.dim.Render("bounced: ")+st.accent.Render(line)+st.dim.Render("   notes with a condition are drawn ")+lipgloss.NewStyle().Foreground(st.th.cond).Bold(true).Render("◐"))
		for _, e := range m.cycleExplain() {
			lines = append(lines, strings.Split(st.faint.Render(ansi.Wrap(e, max(g.bodyW-6, 10), "")), "\n")...)
		}
	}
	if m.patTool != nil {
		lines = append(lines, "", m.patToolMenu())
		lines = append(lines, "", st.faint.Render(strings.Repeat("─", max(g.bodyW-4, 10))))
		lines = append(lines, m.helpLines([]helpGroup{{"tools", []helpKey{{"j k", "pick"}, {"h l", "amount"}, {"e", "8th/16th grid"}, {"Enter", "apply"}, {"Esc", "close"}}}}, max(g.bodyW-4, 10), 9)...)
		return m.box("Pattern "+d.Slot.String(), strings.Join(lines, "\n"), g.bodyW, g.bodyH, true)
	}
	lines = append(lines, "", st.faint.Render(strings.Repeat("─", max(g.bodyW-4, 10))))
	lines = append(lines, m.helpLines(m.patternHelp(d), max(g.bodyW-4, 10), 9)...)
	return m.box("Pattern "+d.Slot.String(), strings.Join(lines, "\n"), g.bodyW, g.bodyH, true)
}

// patternHelp are the keys of the open pattern, in groups like the slicer's; with a track
// expanded the first group is that track's.
func (m Model) patternHelp(d *app.PatternDetail) []helpGroup {
	if m.selecting() {
		return m.selectionHelp()
	}
	move := helpGroup{"move", []helpKey{{"h j k l", "move"}, {"{ }", "turn page"}, {"v", "show/loop bars"}, {"Tab Enter", "open track"}, {"x", "note on/off"}, {"V", "select notes"}, {"D", "clear track here"},
		{"t", "tools (this pad)"}, {"T", "tools: swing, quantize, shift, double, extend, half"}, {"Space P", "play"}, {"a", "other pattern"}, {"Esc", "close"}}}
	if m.patFocus {
		move = helpGroup{"track", []helpKey{{"h l", "step"}, {"j k", "other track"}, {"{ }", "turn page"}, {"Enter x", "note on/off"}, {"Space P", "play"}, {"Tab", "fold"}, {"a", "other pattern"}, {"Esc", "close"}}}
	}
	history := helpGroup{"history", []helpKey{{"u", "undo"}, {"R", "redo"}}}
	if d.HasSourceBank {
		history.keys = append(history.keys, helpKey{"b", "retarget bank"})
	}
	history.keys = append(history.keys, helpKey{"", m.patHistoryLabel()})
	return []helpGroup{
		move,
		{"velocity", []helpKey{{", .", "by 1"}, {"< >", "by 10"}, {"*", "pad's own level"}}},
		{"length", []helpKey{{"+ -", "by a step"}}},
		{"timing", []helpKey{{"H L", "nudge 5 ticks"}, {"( )", "1 tick"}, {"0", "reset: one step long, on the grid"}}},
		{"pad", []helpKey{{"m", "mute"}, {"s", "solo"}, {"S", "save as heard"}, {"B", "bounce to a pad"}}},
		{"trig", []helpKey{{"c C", "condition"}, {"y", "roll the dice again"}}},
		history,
	}
}

// offsetLabel says how far a note at tick is off the grid, as swing, shift or humanize leave
// it: "on the grid", or e.g. "+24 ticks late (20% of a step, 23 ms)".
func (m Model) offsetLabel(d *app.PatternDetail, tick uint32) string {
	off := d.GridOffset(tick)
	if off == 0 {
		return m.st.dim.Render("on the grid")
	}
	word := "late"
	if off < 0 {
		word = "early"
	}
	pct := math.Abs(float64(off)) * 100 / float64(max(d.StepTicks(), 1))
	return m.st.hot.Render(fmt.Sprintf("%+d ticks %s", off, word)) +
		m.st.dim.Render(fmt.Sprintf(" (%.0f%% of a step, %.0f ms)", pct, math.Abs(d.TicksToMillis(off))))
}

// patHistoryLabel is how far the open pattern can be undone and redone, e.g. "step 2/3  ·  2 undo · 1 redo": the edit the pattern is at, of all there are.
func (m Model) patHistoryLabel() string {
	var u, r int
	if h := m.patHist; h != nil && m.pattern != nil && h.slot == m.pattern.Slot {
		u, r = len(h.undo), len(h.redo)
	}
	return fmt.Sprintf("step %d/%d  ·  %d undo · %d redo", u, u+r, u, r)
}

// patToolMenu draws the groove-tools menu over the open pattern: the five tools in a column with
// the chosen one lit and its amount shown.
func (m Model) patToolMenu() string {
	st, t := m.st, m.patTool
	res := "16ths"
	if t.eighths {
		res = "8ths"
	}
	rows := []struct {
		kind   patToolKind
		label  string
		amount string
	}{
		{toolSwing, "SWING", fmt.Sprintf("%d%%  (%s)", t.swing, res)},
		{toolQuantize, "QUANT", fmt.Sprintf("%d%%  (%s)", t.quantStrength, res)},
		{toolShift, "SHIFT", fmt.Sprintf("%+d  (%s)", t.shift, res)},
		{toolReverse, "REVERSE", "—"},
		{toolHumanize, "HUMANIZE", fmt.Sprintf("%d", t.human)},
		{toolDouble, "DOUBLE", fmt.Sprintf("%s → %s, played again", plural(m.pattern.Bars(), "bar"), plural(m.pattern.Bars()*2, "bar"))},
		{toolExtend, "EXTEND", fmt.Sprintf("%s → %s, the new half empty", plural(m.pattern.Bars(), "bar"), plural(m.pattern.Bars()*2, "bar"))},
		{toolHalve, "HALF", fmt.Sprintf("%s → %s, first half kept", plural(m.pattern.Bars(), "bar"), plural(max(m.pattern.Bars()/2, 1), "bar"))},
	}
	var out []string
	for _, r := range rows {
		marker, name := "  ", st.text.Render(fmt.Sprintf("%-9s", r.label))
		amount := st.dim.Render(r.amount)
		if r.kind == t.kind {
			marker = st.accent.Render("▶ ")
			name = st.bold.Render(fmt.Sprintf("%-9s", r.label))
			amount = st.accent.Render(r.amount)
		}
		out = append(out, marker+name+" "+amount)
	}
	where := "whole pattern"
	if t.pad != nil {
		where = "pad " + t.pad.String()
	}
	return st.bold.Render("Groove tools  ("+where+")") + "\n" + strings.Join(out, "\n")
}

// ---- files ----

func (m Model) filesBody(g geom) string {
	title := rootName(m.root)
	if n := len(m.fileMarks); n > 0 {
		title += fmt.Sprintf("  ·  %d marked", n)
	}
	if m.ranger {
		return m.rangerBody(g)
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
		case n.Dir && m.folderUsed(n.Path):
			return m.usedMark()
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
		lines = append(lines, m.folderUsage(node.Path, width)...)
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
			st.dim.Render("Samples  ")+st.text.Render(fmt.Sprintf("%d of %d pads", samples, len(m.snap.Pads))))
		if m.offline == "" {
			card = append(card, st.dim.Render("Free     ")+st.text.Render(fmt.Sprintf("%.1f GB", float64(m.snap.FreeKb)/1024/1024)))
		} else {
			card = append(card, st.dim.Render("Folder   ")+st.warn.Render(display(m.offline)))
		}
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
		st.accent.Render(spinner[m.frame%len(spinner)]) + " " + st.bold.Render(title),
		st.text.Render("Connect it with a USB-C cable and switch it on."),
		st.dim.Render("Looking for USB 0582:02e7; it connects by itself."),
	}
	if m.lost {
		lines = append([]string{st.bad.Render("The connection to the unit was lost.")}, append([]string{""}, lines...)...)
	}
	if m.waitErr != "" {
		lines = append(lines, "", st.warn.Render(m.waitErr))
	}
	if m.openOffline != nil {
		lines = append(lines, "", st.text.Render("Not at the unit? ")+st.accent.Render("o")+st.text.Render(" works offline on a folder of projects on this computer."))
	}
	lines = append(lines, "", st.dim.Render("q quit  ·  ? help"))
	return m.dashboard(lines)
}

// ---- overlays ----

func (m Model) helpBox() string {
	lines := []string{m.st.bold.Render("Keybindings  (? or Esc closes)"), ""}
	for _, row := range helpText {
		lines = append(lines, m.st.accent.Render(fmt.Sprintf("%-22s", row[0]))+" "+m.st.text.Render(row[1]))
	}
	return m.st.modal.Render(strings.Join(lines, "\n"))
}

// rangePickerBox draws the list of ranges v opens over a pattern.
func (m Model) rangePickerBox() string {
	st, p := m.st, m.rangePick
	lines := []string{st.bold.Render("Show and loop"), ""}
	for i, r := range p.items {
		label, note := r.label(), ""
		if r.first < 0 {
			label, note = "Whole pattern", "every bar"
		} else if r.bars() > 1 {
			note = "two bars"
		}
		line := fmt.Sprintf("%-16s %s", label, st.dim.Render(note))
		if i == p.cursor {
			line = lipgloss.NewStyle().Background(st.th.cursorBg).Foreground(st.th.cursorFg).Bold(true).Render(fmt.Sprintf("%-16s %s", label, note))
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", st.dim.Render("only the bars chosen are shown, edited and played"),
		st.dim.Render("↑↓ choose  ·  Enter set  ·  Esc close"))
	return st.modal.Width(min(m.width-4, 60)).Render(strings.Join(lines, "\n"))
}

// templatePickerBox draws the new-pattern template list N opens.
func (m Model) templatePickerBox() string {
	st := m.st
	items := templateItems()
	lines := []string{st.bold.Render("New pattern"), ""}
	for i, it := range items {
		note := ""
		switch i {
		case 0:
			note = "every sampled pad held for the whole pattern"
		case 1:
			note = "an empty pattern to program by hand"
		default:
			// A groove names the voices it plays, so the kit it wants is plain.
			if g, ok := grooveAt(i); ok {
				var names []string
				for _, l := range g.Lanes {
					names = append(names, strings.ToLower(l.LaneName()))
				}
				note = strings.Join(names, ", ")
			}
		}
		line := fmt.Sprintf("%-38s %s", it, st.dim.Render(note))
		if i == m.tmplPicker.cursor {
			line = lipgloss.NewStyle().Background(st.th.cursorBg).Foreground(st.th.cursorFg).Bold(true).Render(fmt.Sprintf("%-38s %s", it, note))
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", st.dim.Render("↑↓ navigate  ·  Enter choose  ·  Esc close"))
	return st.modal.Width(min(m.width-4, 110)).Render(strings.Join(lines, "\n"))
}

// laneAssignBox draws the lane->pad confirm dialog for a genre groove, or its pad chooser when a
// lane is being reassigned.
func (m Model) laneAssignBox() string {
	st, a := m.st, m.laneMap
	if a.choosing {
		return m.laneChooseBox()
	}
	// padLabel is a lane's chosen pad as "I12  clap_011", or "— skip".
	padLabel := func(lane formats.Lane) string {
		p := a.lane[lane]
		if p == nil {
			return "— skip"
		}
		name := m.snap.Pads[p.Index()].Name
		if name == "" {
			name = "no name"
		}
		return fmt.Sprintf("%s  %s", p.String(), name)
	}
	lines := []string{st.bold.Render(a.tmpl.Name + " — pads for each voice"), ""}
	for i, lane := range a.tmpl.Lanes {
		label := padLabel(lane)
		line := fmt.Sprintf("%-11s %s", lane.LaneName(), st.dim.Render(label))
		if i == a.cursor {
			line = lipgloss.NewStyle().Background(st.th.cursorBg).Foreground(st.th.cursorFg).Bold(true).Render(fmt.Sprintf("%-11s %s", lane.LaneName(), label))
		}
		lines = append(lines, line)
	}
	lines = append(lines, "",
		st.dim.Render("↑↓ pick a voice  ·  Enter choose its pad  ·  ←→ quick change  ·  w write  ·  Esc back"))
	return st.modal.Width(min(m.width-4, 72)).Render(strings.Join(lines, "\n"))
}

// laneChooseBox draws the pad chooser for one lane: "skip" and then every sampled pad of the
// bank, with the one in front highlighted.
func (m Model) laneChooseBox() string {
	st, a := m.st, m.laneMap
	lane := a.tmpl.Lanes[a.cursor]
	lines := []string{st.bold.Render(a.tmpl.Name + " — pad for " + strings.ToLower(lane.LaneName())), ""}
	rows := []string{"— skip"}
	for _, pad := range a.pads {
		name := m.snap.Pads[pad.Index()].Name
		if name == "" {
			name = "no name"
		}
		rows = append(rows, fmt.Sprintf("%s  %s", pad.String(), name))
	}
	// The chooser rows run pads first then skip in the key handler (skip is last); show skip
	// last here too to match.
	rows = append(rows[1:], rows[0])
	for i, r := range rows {
		line := st.dim.Render(r)
		if i == a.chooseAt {
			line = lipgloss.NewStyle().Background(st.th.cursorBg).Foreground(st.th.cursorFg).Bold(true).Render(r)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "",
		st.dim.Render("↑↓ pick a pad  ·  Enter set  ·  Esc back to the voices"))
	return st.modal.Width(min(m.width-4, 72)).Render(strings.Join(lines, "\n"))
}

func (m Model) paletteBox() string {
	st := m.st
	found := m.matches()
	lines := []string{st.accent.Render("> ") + m.palette.filter + "█", ""}
	if m.palette.title != "" && m.palette.items != nil {
		lines[1] = st.bold.Render(m.palette.title) + st.dim.Render("   ? or Esc closes")
	}
	if len(found) == 0 {
		lines = append(lines, st.dim.Render("no command matches"))
	}
	from, to := window(len(found), m.palette.cursor, max(m.height-12, 5))
	for i := from; i < to; i++ {
		c := found[i]
		line := fmt.Sprintf("%s %s", st.accent.Render(fmt.Sprintf("%-9s", c.key)), c.label)
		if i == m.palette.cursor {
			line = lipgloss.NewStyle().Background(st.th.cursorBg).Foreground(st.th.cursorFg).Bold(true).Render(fmt.Sprintf("%-9s %s", c.key, c.label))
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", st.dim.Render("type to search  ·  ↑↓ navigate  ·  Enter run  ·  ? or Esc close"))
	return st.modal.Width(min(m.width-4, 90)).Render(strings.Join(lines, "\n"))
}

func (m Model) promptBox() string {
	st, p := m.st, m.prompt
	var title, hint string
	switch p.kind {
	case promptQuit:
		return st.modal.Render(st.bold.Render("Quit 404tui?") + "\n\n" + st.dim.Render("y/Enter/q quit   n/Esc stay"))
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
	case promptBusScan:
		return m.busScanBox(p)
	case promptRepairPads:
		return m.repairBox(p)
	case promptLiveCode:
		title, hint = "Pads for the live mode", "pad numbers 1 to 16, hit in turn on the unit, in any bank; off for none  ·  "
	case promptRecCode:
		title, hint = "Pads for recording", "pad numbers 1 to 16, hit in turn on the unit, in any bank; off for none  ·  "
	case promptBusNote:
		title, hint = fmt.Sprintf("The effect on BUS %d", p.busOf), "what you set on the unit, e.g. Hall reverb, shown in Perform  ·  "
	case promptRecDir:
		title, hint = "Record into folder", "empty for the default, Music/404tui  ·  "
	case promptRecDevice:
		title, hint = "Input to record", "a part of its name; the unit's is SP-404MKII, so 404  ·  "
	case promptBusWords:
		title, hint = fmt.Sprintf("Words for BUS %d", p.busOf), "separated by commas or spaces; a pad whose name holds one goes to the bus  ·  "
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
	return st.modal.Width(min(m.width-4, 72)).Render(st.bold.Render(title) + "\n\n" + st.accent.Render("> ") + p.shown() + "\n\n" + st.dim.Render(hint+"Enter apply  ·  Esc cancel"))
}

// pickerBox is the pad chooser of an assign, or the bank chooser of a retarget.
func (m Model) pickerBox() string {
	st, p := m.st, m.picker
	if p.slots {
		return m.patternSlotBox()
	}
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
	title := "Bounce the slices to a pad"
	if len(p.bounces) > 1 {
		title = fmt.Sprintf("Bounce %d takes of the slices, from the pad you choose onward, to free pads", len(p.bounces))
		if p.bounceWhat == "slices" {
			title = fmt.Sprintf("Put the %d marked slices on free pads, from the pad you choose onward", len(p.bounces))
		}
	}
	if p.bounceWhat == "pattern" {
		title = "Bounce the pattern, as you hear it, to a pad"
	}
	if p.bounceWhat == "stretch" {
		title = "Put the stretched sample on a pad\n" + st.dim.Render(p.note)
	}
	if len(p.bounces) == 0 {
		title = fmt.Sprintf("Assign %s to a pad", app.Base(p.files[0]))
	}
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
	if len(p.bounces) > 1 {
		pads := m.takePads(p)
		var names []string
		for _, pad := range pads {
			names = append(names, pad.String())
		}
		line = st.dim.Render("goes on " + strings.Join(names, ", ") + "; pads that hold a sample are left alone")
		if len(pads) < len(p.bounces) {
			line = st.warn.Render(fmt.Sprintf("only %d of %d %s fit in this bank from here: %s", len(pads), len(p.bounces), cmp.Or(p.bounceWhat, "takes"), strings.Join(names, ", ")))
		}
		if len(pads) == 0 {
			line = st.bad.Render("no free pad in this bank from here on")
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
	var title, info, enter string
	if p.saveFrom != nil {
		title = fmt.Sprintf("Save pattern %s as you hear it: into which slot?", *p.saveFrom)
		info = st.dim.Render("muted pads are left out (with solo on, every pad but the soloed ones)")
		enter = "Enter save"
	} else {
		title = fmt.Sprintf("Pattern for the %d pads restored onto bank %c: which slot?", len(p.restorePlan.Pads), 'A'+p.bank)
		info = st.dim.Render(fmt.Sprintf("every pad starts on beat 1 and is held for %d bars", app.StartBars))
		enter = "Enter restore and make the pattern"
	}
	rows := m.patterns[p.ptnBank]
	if rows == nil {
		return st.modal.Render(st.bold.Render(title) + "\n" + info + "\n\n" + strip.String() + "\n\n" + st.dim.Render("reading the patterns of this bank…"))
	}
	grid := m.dialogGrid(func(pos int) string {
		name := ""
		if rows[pos].Exists {
			name = fmt.Sprintf("%d pads", len(rows[pos].Pads))
		}
		this := p.saveFrom != nil && rows[pos].Slot == *p.saveFrom
		if this {
			name = "this one"
		}
		return m.tile(dialogTileW, 2, fmt.Sprint(pos+1), name, 0, tileState{
			empty:    !rows[pos].Exists && !this,
			cursor:   pos == p.pad,
			marked:   this,
			noLevel:  true,
			progress: -1,
		})
	})
	slot := rows[p.pad]
	line := st.good.Render(fmt.Sprintf("%s is free", slot.Slot))
	switch {
	case p.saveFrom != nil && slot.Slot == *p.saveFrom:
		line = st.warn.Render("that is the pattern being saved: choose another slot")
	case slot.Exists:
		line = st.warn.Render(fmt.Sprintf("%s holds a pattern that plays %d pads — it will be replaced", slot.Slot, len(slot.Pads)))
		if p.confirm {
			line = st.bad.Render("Press Enter again to replace it")
		}
	}
	return st.modal.Render(st.bold.Render(title) + "\n" + info + "\n\n" + strip.String() + "\n\n" + grid + "\n\n" + line +
		"\n\n" + st.dim.Render("A-J or [ ] bank  ·  hjkl move  ·  "+enter+"  ·  Esc back"))
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

// pickerGrid is a small pad grid for choosing a target, drawn with the tiles of the pad grid:
// number, name and level, a pad on a bus marked, the empty ones dark.
func (m Model) pickerGrid() string {
	p := m.picker
	return m.dialogGrid(func(pos int) string {
		pad, _ := proto.PadFromBankPad(uint16(p.bank), uint16(pos))
		r := m.snap.Pads[pad.Index()]
		return m.tile(dialogTileW, 2, fmt.Sprint(pos+1), r.Name, r.Level, tileState{
			empty:  !r.HasSample,
			bus:    r.Params["bus-fx"],
			muted:  m.muted[r.Pad],
			solo:   m.soloed[r.Pad],
			cursor: pos == p.pad,
		})
	})
}

// dialogTileW is the width of a pad tile in the dialogs that choose a pad or a slot.
const dialogTileW = 17

// dialogGrid lays 16 tiles out 4×4 as on the unit, with a row between them when the screen
// is tall enough.
func (m Model) dialogGrid(tile func(pos int) string) string {
	rows := make([]string, 0, 7)
	for row := 0; row < 4; row++ {
		var cells []string
		for col := 0; col < 4; col++ {
			if col > 0 {
				cells = append(cells, " ")
			}
			cells = append(cells, tile(row*4+col))
		}
		if row > 0 && m.height >= 36 {
			rows = append(rows, "")
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
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
	matches := folderChoices(rows, ix, m.tags, input)
	choice = clamp(choice, 0, max(len(matches)-1, 0))

	status := st.dim.Render(fmt.Sprintf("%d of %d folders", len(matches), len(folderChoices(rows, ix, m.tags, ""))))
	if ix == nil {
		status += st.faint.Render("  · only the open ones: i indexes folders so that all are found")
	}
	var list []string
	from, to := window(len(matches), choice, height-1)
	nameW := 12
	for i := from; i < to; i++ {
		nameW = max(nameW, ansi.StringWidth(matches[i].Name)+3)
	}
	nameW = min(nameW, (width-6)*55/100)
	dirW := max(width-nameW-8, 6)
	if to > from {
		list = append(list, st.faint.Render(" "+padTo("Name", nameW)+"  Folder"))
	}
	for i := from; i < to; i++ {
		r := matches[i]
		label := r.Name + "/"
		if r.Fav {
			label = "★ " + label
		}
		name := ansi.Truncate(label, nameW, "…")
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
	column := detail
	// What is picked up or on the clipboard is told under the details, while it is.
	if notes := m.heldBox(g); notes != "" {
		column = lipgloss.JoinVertical(lipgloss.Left, column, notes)
	}
	left := g.bodyH - g.top - lipgloss.Height(column)
	if left < 5 {
		return column
	}
	return lipgloss.JoinVertical(lipgloss.Left, column, m.historyBox(g, left))
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
			label := strings.Repeat(" ", 10)
			if first {
				label = padTo("  "+grp.label, 10)
			}
			guide = append(guide, st.faint.Render(label)+line)
			line, first = "", false
		}
		for _, k := range grp.keys {
			item := st.accent.Render(k[0]) + st.dim.Render(" "+k[1])
			if line != "" && ansi.StringWidth(line)+ansi.StringWidth(item)+2 > width-12 {
				emit()
			}
			if line != "" {
				line += "  "
			}
			line += item
		}
		emit()
	}
	guide = append([]string{st.faint.Render(strings.Repeat("─", max(width, 1)))}, guide...)
	used := strings.Count(text, "\n") + 1
	if used+len(guide)+1 > height {
		return text
	}
	blank := strings.Repeat("\n", height-used-len(guide))
	return text + blank + strings.Join(guide, "\n")
}

// busLegend says how many pads of the bank go to each bus, in the colours of the tiles: blue
// for BUS 1, red for BUS 2. It is empty when no pad goes to a bus.
func (m Model) busLegend() string {
	var n [3]int
	for _, row := range m.snap.Bank(m.bank) {
		if row.HasSample {
			if bus := row.Params["bus-fx"]; bus == 1 || bus == 2 {
				n[bus]++
			}
		}
	}
	out := ""
	for bus := int32(1); bus <= 2; bus++ {
		if n[bus] == 0 {
			continue
		}
		colour, _ := m.st.th.busColour(bus)
		out += "   " + lipgloss.NewStyle().Foreground(colour).Bold(true).Render(fmt.Sprintf("%s BUS %d ×%d", busBadge(bus), bus, n[bus]))
	}
	return out
}

// originWhen says when a pad's origin was noted, and whether it was found afterwards.
func originWhen(o app.Origin) string {
	if o.Matched {
		return "found by scan " + o.At
	}
	return "assigned " + o.At
}

// padFlag is one setting of a pad as a tiny letter, and whether it is on.
type padFlag struct {
	letter string
	on     bool
}

// padFlags are a pad's settings as tiny letters for the tile, each marked on or off: G gate,
// L loop, O one shot, R reverse, S BPM sync, V vinyl, and when it has one, M and the number of
// its mute group. Ping-pong (both ways) shows as ~ when it is on.
func padFlags(row app.PadRow) []padFlag {
	if !row.HasSample {
		return nil
	}
	p := row.Params
	mode := p["play-mode"]
	flags := []padFlag{
		{"G", p["gate"] != 0},
		{"L", p["loop"] != 0},
		{"O", p["mode-flags"]&0x20 != 0},
		{"R", mode == 1 || mode == 3},
		{"S", p["bpm-sync"] != 0},
		{"V", p["vinyl"] != 0},
	}
	if mode == 2 || mode == 3 {
		flags = append(flags, padFlag{"~", true})
	}
	if g := p["mute-group"]; g > 0 {
		flags = append(flags, padFlag{fmt.Sprintf("M%d", g), true})
	}
	return flags
}

// onLetters are the letters of the flags that are on, for tests and messages.
func onLetters(flags []padFlag) string {
	var on []string
	for _, f := range flags {
		if f.on {
			on = append(on, f.letter)
		}
	}
	return strings.Join(on, " ")
}

// noteBlocks are the heights a note's block can have, by velocity.
var noteBlocks = []rune("▁▂▃▄▅▆▇█")

// noteBlock is the block for a velocity, 1 to 127.
func noteBlock(velocity int) string {
	return string(noteBlocks[clamp((velocity-1)*len(noteBlocks)/127, 0, len(noteBlocks)-1)])
}

// noteGlyphs are the four marks a note can have, softest to loudest.
var noteGlyphs = []string{"○", "◌", "◉", "●"}

// noteGlyph is the mark for a note at a velocity, 1 to 127: brighter and fuller as it is louder,
// so a lane shows its dynamics at a glance. The style tracks the glyph so brightness reads too.
func (m Model) noteGlyph(velocity int) (string, lipgloss.Style) {
	bucket := clamp(velocity*len(noteGlyphs)/128, 0, len(noteGlyphs)-1)
	styles := []lipgloss.Style{m.st.faint, m.st.dim, m.st.accent, m.st.hot}
	return noteGlyphs[bucket], styles[bucket]
}

// noteRow draws one pad's notes across the bars of sp: each step is one glyph whose brightness
// is the note's velocity (a dot when empty); a note longer than a step trails dashes through the
// steps it covers, reading as ◉───; a bar line parts the bars. onCursorRow shows the cursor as a
// reverse-video step. cover/owner span the whole pattern so a note sustaining in from a bar not
// on show still shows.
func (m Model) noteRow(d *app.PatternDetail, pad proto.PadIndex, onCursorRow bool, sp stepSpan) string {
	st := m.st
	cover, _, start := laneCover(d, pad)
	// A pad that is not heard in the mix on this computer has its notes greyed out, and so do the
	// tracks around one that is expanded, so the eye stays on it.
	dimmed := !heard(pad, m.muted, m.soloed)
	if m.patFocus && d == m.pattern && len(d.Grid.Rows) > 0 && d.Grid.Rows[clamp(m.patRow, 0, len(d.Grid.Rows)-1)].Pad != pad {
		dimmed = true
	}
	cursorStyle := lipgloss.NewStyle().Background(st.th.cursorBg).Foreground(st.th.cursorFg).Bold(true)
	chosen := m.selectedCols(d, pad)
	chosenStyle := lipgloss.NewStyle().Background(st.th.markedBg).Foreground(st.th.text).Bold(true)
	lo, hi := sp.lo, sp.hi
	var b strings.Builder
	w := max(sp.w, 1)
	for c := lo; c < hi; c++ {
		// sustained is a step a note from before carries into: it draws a dash, not a fresh mark;
		// carries is a note going on into the next step, which dashes run on to.
		sustained := cover[c] > 0 && !start[c]
		carries := cover[c] > 0 && c+1 < len(cover) && cover[c+1] > 0 && !start[c+1] && !dimmed
		mark, style := "·", st.faint
		switch {
		case sustained:
			mark, style = "─", st.accent
		case cover[c] > 0:
			mark, style = m.noteGlyph(cover[c])
			if d == m.pattern && m.conditioned(d, pad, c) {
				// A note with a condition: half lit, in a colour of its own (green: no other mark in
				// the grid has it).
				mark, style = "◐", lipgloss.NewStyle().Foreground(st.th.cond).Bold(true)
			}
		}
		// A step is w cells: its mark, then dashes when the note goes on, else spaces.
		rest := " "
		if carries {
			rest = "─"
		}
		if dimmed && cover[c] > 0 {
			style = st.faint
		}
		switch {
		case onCursorRow && c == m.patCol:
			b.WriteString(cursorStyle.Render(mark + strings.Repeat(rest, w-1)))
		case chosen[c] && !sustained:
			b.WriteString(chosenStyle.Render(mark + strings.Repeat(" ", w-1)))
		default:
			b.WriteString(style.Render(mark) + st.accent.Render(strings.Repeat(rest, w-1)))
		}
		if c+1 == hi {
			break
		}
		// The gap between steps is wider every 4th (the beat), a bar line after a bar. A note
		// carrying into the next step fills the gap with dashes so it reads as one held note.
		gap := sp.gap(c - lo)
		switch {
		case carries:
			b.WriteString(m.fillGap(gap, "─", st.accent))
		case strings.Contains(gap, "│"):
			b.WriteString(st.faint.Render(gap))
		default:
			b.WriteString(gap)
		}
	}
	return b.String()
}

// conditioned says whether a note that starts in the step has a condition that is not plain.
func (m Model) conditioned(d *app.PatternDetail, pad proto.PadIndex, col int) bool {
	if len(m.patConds) == 0 {
		return false
	}
	for _, n := range d.CellNotes(pad, col) {
		if c, ok := m.patConds[app.NoteKey{Pad: n.Pad, Step: int(n.Tick) / (formats.PPQ / 4)}]; ok && !c.Plain() {
			return true
		}
	}
	return false
}

// stepSpan is the columns of a pattern's grid on show: [lo, hi), always whole bars, each step w
// cells wide, and extra cells shared out over the gaps so the bars fill the room they are given.
type stepSpan struct{ lo, hi, w, extra int }

// gap is what goes after step i of the span (i counted from its start): barGap, widened by its
// share of the span's extra cells. A bar line stays in the middle of its gap.
func (sp stepSpan) gap(i int) string {
	base := barGap(i)
	gaps := sp.hi - sp.lo - 1
	if gaps <= 0 || sp.extra <= 0 {
		return base
	}
	// Every gap gets the same share, the gaps after a beat what is left of it split evenly; what
	// does not split evenly is left out, so the steps stay regular.
	add := sp.extra / gaps
	if (i+1)%4 == 0 {
		if beats := gaps / 4; beats > 0 {
			add += sp.extra % gaps / beats
		}
	}
	if strings.Contains(base, "│") {
		return strings.Repeat(" ", 1+add/2) + "│" + strings.Repeat(" ", 1+add-add/2)
	}
	return base + strings.Repeat(" ", add)
}

// fillGap draws a gap with line in place of its spaces, in style, keeping a bar line faint.
func (m Model) fillGap(gap, line string, style lipgloss.Style) string {
	var b strings.Builder
	for _, r := range gap {
		if r == '│' {
			b.WriteString(m.st.faint.Render("│"))
		} else {
			b.WriteString(style.Render(line))
		}
	}
	return b.String()
}

// barGap is what goes after step i of a span (i counted from the span's start) in the compact
// grid: a bar line after the last step of a bar, two spaces after a beat, else one.
func barGap(i int) string {
	switch {
	case (i+1)%app.GridStepsPerBar == 0:
		return " │ "
	case (i+1)%4 == 0:
		return "  "
	}
	return " "
}

// spanFor is the bars of d to show in width cells, after the 20-cell label column and a little
// room for a MUTE or SOLO badge: as many whole bars as fit with steps minW cells wide (at least
// one bar), then the steps made as wide as the room allows, up to maxW. A bar is 16 steps and
// gapW cells of gaps. The bars start at a multiple of their count, so the bar page is among them.
func spanFor(d *app.PatternDetail, page, width, minW, maxW, gapW int) stepSpan {
	room := width - 20 - 2
	n := clamp(room/(16*minW+gapW), 1, min(d.Bars(), maxBarsOnShow))
	return spanBars(d, page/n*n, n, width, minW, maxW, gapW)
}

// spanBars is the span of n bars of d from bar start, its steps as wide as width allows (see
// spanFor).
func spanBars(d *app.PatternDetail, start, n, width, minW, maxW, gapW int) stepSpan {
	room := width - 20 - 2
	w := clamp((room/n-gapW)/16, minW, maxW)
	sp := stepSpan{lo: start * d.StepsPerPage(), hi: min((start+n)*d.StepsPerPage(), d.Grid.Steps), w: w}
	// What the steps and their gaps take (the last bar line is not drawn); the rest of the room
	// goes into the gaps, at most doubling each.
	steps := sp.hi - sp.lo
	used := steps*w + (steps/16)*gapW - 3
	sp.extra = clamp(room-used, 0, (steps-1)*(w+1))
	return sp
}

// maxBarsOnShow is the most bars the grid shows at once: more than two squeezes the steps, so
// longer patterns turn pages of two instead.
const maxBarsOnShow = 2

// The gaps in one bar of the compact grid: 12 single, 3 beat pairs, the bar line.
const (
	gridGapW = 12 + 3*2 + 3
)

// patSpan is the span of a pattern's compact grid in a body width cells wide.
func (m Model) patSpan(d *app.PatternDetail, width int) stepSpan {
	if len(m.muted) > 0 || len(m.soloed) > 0 {
		// Room for a MUTE or SOLO badge after the rows.
		width -= 7
	}
	if r, ok := m.rangeOf(d); ok {
		// A range is shown whole, however few bars, stretched over the width.
		return spanBars(d, r.first, r.bars(), width, 1, 4, gridGapW)
	}
	return spanFor(d, m.viewPage(d), width, 1, 4, gridGapW)
}

// centered puts s (one cell wide) in the middle of w cells of fill.
func centered(s string, w int, fill string) string {
	left := (w - 1) / 2
	return strings.Repeat(fill, left) + s + strings.Repeat(fill, w-1-left)
}

// spanLabel names the bars a span shows, e.g. "Bar 2 / 4" or "Bars 1–2 / 2".
func spanLabel(d *app.PatternDetail, sp stepSpan) string {
	first, last := sp.lo/d.StepsPerPage()+1, (sp.hi-1)/d.StepsPerPage()+1
	if first == last {
		return fmt.Sprintf("Bar %d / %d", first, d.Bars())
	}
	return fmt.Sprintf("Bars %d–%d / %d", first, last, d.Bars())
}

// spanHeader is spanLabel with, when the pattern is longer than what is on show, a dot per page
// (the one on show lit) and the bars the next page holds, so it is plain there is more.
func (m Model) spanHeader(d *app.PatternDetail, sp stepSpan) string {
	st := m.st
	head := st.bold.Render(spanLabel(d, sp))
	if r, ok := m.rangeOf(d); ok {
		return head + "   " + st.accent.Render("⟲ "+r.label()+" only") + "   " + st.dim.Render("{ } move it  ·  v whole pattern")
	}
	spp := d.StepsPerPage()
	perPage := max((sp.hi-sp.lo)/spp, 1)
	pages := (d.Bars() + perPage - 1) / perPage
	if pages < 2 {
		return head
	}
	at := sp.lo / spp / perPage
	var dots []string
	for p := range pages {
		if p == at {
			dots = append(dots, st.accent.Render("●"))
		} else {
			dots = append(dots, st.faint.Render("○"))
		}
	}
	next := (at + 1) % pages
	first, last := next*perPage+1, min((next+1)*perPage, d.Bars())
	bars := fmt.Sprintf("bar %d", first)
	if last > first {
		bars = fmt.Sprintf("bars %d–%d", first, last)
	}
	return head + "   " + strings.Join(dots, " ") + "   " + st.dim.Render("next: "+bars+"  }")
}

// laneCover is, for each step of the whole pattern, the velocity a pad sounds there (0: none) and
// which of d.Notes it comes from (-1: none); a note covers every step it lasts into. start says
// where a note starts: there the cover is the loudest note that starts in the step, so a note
// that begins under a longer one is drawn as itself and not taken for the long one going on.
func laneCover(d *app.PatternDetail, pad proto.PadIndex) (cover, owner []int, start []bool) {
	steps := d.Grid.Steps
	cover, owner, start = make([]int, steps), make([]int, steps), make([]bool, steps)
	for i := range owner {
		owner[i] = -1
	}
	starts := make([]int, steps)
	for i, n := range d.Notes {
		if n.Pad != pad || d.Length == 0 {
			continue
		}
		first := min(int(uint64(n.Tick)*uint64(steps)/uint64(d.Length)), steps-1)
		end := n.Tick + uint32(n.Length)
		for c := first + 1; c < steps && d.StepTick(c) < end; c++ {
			if int(n.Velocity) >= cover[c] {
				cover[c], owner[c] = int(n.Velocity), i
			}
		}
		if int(n.Velocity) >= starts[first] {
			starts[first] = int(n.Velocity)
			start[first] = true
			owner[first] = i
		}
	}
	for c := range steps {
		if start[c] {
			cover[c] = starts[c]
		}
	}
	return cover, owner, start
}

// focusLane draws the expanded track over the same bars and step places as the compact rows
// (sp), so its notes sit right under theirs, as three lines: the step numbers within the bar,
// the notes (with the cursor), and a bar as tall as each note's velocity. A step here is the
// compact step and the gap after it, the mark at its left like a compact row's. The lines are
// meant to sit right of the 20-cell label column.
func (m Model) focusLane(d *app.PatternDetail, pad proto.PadIndex, sp stepSpan) []string {
	st := m.st
	cover, _, start := laneCover(d, pad)
	cursorStyle := lipgloss.NewStyle().Background(st.th.cursorBg).Foreground(st.th.cursorFg).Bold(true)
	chosen := m.selectedCols(d, pad)
	chosenStyle := lipgloss.NewStyle().Background(st.th.markedBg).Foreground(st.th.text).Bold(true)
	w := max(sp.w, 1) + 1
	var nums, notes, vels, lens strings.Builder
	for c := sp.lo; c < sp.hi; c++ {
		i := c - sp.lo
		num := st.faint
		if i%4 == 0 {
			num = st.bold
		}
		n := fmt.Sprint(i%d.StepsPerPage() + 1)
		if len(n) >= w && i%4 != 0 {
			// Too narrow for every two-digit number to stand apart: those off the beat are ticks.
			n = "·"
		}
		nums.WriteString(num.Render(n + strings.Repeat(" ", max(w-len([]rune(n)), 0))))
		// A note shows only where it starts, with its velocity as a bar; how long it lasts is
		// the line under the bars: ┣━━━ from where it starts to where it ends. A note that
		// begins under a longer one is a note of its own.
		sustained := cover[c] > 0 && !start[c]
		carries := cover[c] > 0 && c+1 < len(cover) && cover[c+1] > 0 && !start[c+1]
		tail := " "
		mark, style, vel := "·", st.faint, strings.Repeat(" ", w)
		length := strings.Repeat(" ", w)
		switch {
		case sustained:
			length = st.accent.Render(strings.Repeat("━", w))
		case cover[c] > 0:
			length = st.accent.Render("┣" + strings.Repeat(map[bool]string{true: "━", false: " "}[carries], w-1))
			mark, style = m.noteGlyph(cover[c])
			vel = st.accent.Render(noteBlock(cover[c]) + strings.Repeat(" ", w-1))
			if d == m.pattern && m.conditioned(d, pad, c) {
				mark, style = "◐", lipgloss.NewStyle().Foreground(st.th.cond).Bold(true)
				vel = lipgloss.NewStyle().Foreground(st.th.cond).Render(noteBlock(cover[c]) + strings.Repeat(" ", w-1))
			}
		}
		switch {
		case c == m.patCol:
			notes.WriteString(cursorStyle.Render(mark + strings.Repeat(tail, w-1)))
		case chosen[c] && !sustained:
			notes.WriteString(chosenStyle.Render(mark + strings.Repeat(" ", w-1)))
		default:
			notes.WriteString(style.Render(mark) + st.accent.Render(strings.Repeat(tail, w-1)))
		}
		vels.WriteString(vel)
		lens.WriteString(length)
		if c+1 < sp.hi {
			// What the compact row has after a step beyond the one space the lane's step already
			// takes, so the two line up.
			gap := strings.TrimPrefix(sp.gap(i), " ")
			g := st.faint.Render(gap)
			nums.WriteString(g)
			vels.WriteString(g)
			notes.WriteString(g)
			if carries {
				lens.WriteString(m.fillGap(gap, "━", st.accent))
			} else {
				lens.WriteString(g)
			}
		}
	}
	return []string{nums.String(), notes.String(), vels.String(), lens.String()}
}

// loopAt is where the looping pattern is in its round, from 0 to 1, and how long is left of
// the round; false when the pattern on show is not looping.
func (m Model) loopAt(slot proto.PadIndex) (float64, time.Duration, bool) {
	if m.loopSlot == nil || *m.loopSlot != slot || m.loopRound <= 0 || m.player == nil || !m.player.Playing() ||
		m.listening != "pattern "+slot.String() || m.loopStart.IsZero() {
		return 0, 0, false
	}
	in := time.Since(m.loopStart) % m.loopRound
	// With conditions the round is several loops of the pattern: the place is in the loop.
	within := math.Mod(float64(in)/float64(m.loopRound)*float64(max(m.loopLoops, 1)), 1)
	return within, m.loopRound - in, true
}

// playhead is a line under the step numbers that follows the looping pattern: done is solid,
// the step that sounds is lit, what is still to come is dashed, and the label says how long
// until the round starts again. It is empty when the pattern is not playing.
func (m Model) playhead(d *app.PatternDetail, sp stepSpan) string {
	frac, left, ok := m.loopAt(d.Slot)
	if !ok {
		return ""
	}
	return m.headLine(d, sp, m.loopStep(d, frac), fmt.Sprintf("▶ %.1f s to go", left.Seconds()))
}

// headLine is the playhead under the step numbers with the pattern at step now, and label at
// its start.
func (m Model) headLine(d *app.PatternDetail, sp stepSpan, now int, label string) string {
	st := m.st
	// fill draws a gap in a line's style, keeping the bar line where there is one.
	fill := m.fillGap
	var b strings.Builder
	b.WriteString(st.hot.Render(fmt.Sprintf("%-20s", label)))
	for c := sp.lo; c < sp.hi; c++ {
		gap := ""
		if c+1 < sp.hi {
			gap = sp.gap(c - sp.lo)
		}
		w := max(sp.w, 1)
		switch {
		case c < now:
			b.WriteString(st.accent.Render(strings.Repeat("━", w)) + fill(gap, "━", st.accent))
		case c == now:
			b.WriteString(st.hot.Render(strings.Repeat("█", w)) + fill(gap, "┄", st.faint))
		default:
			b.WriteString(st.faint.Render(strings.Repeat("┄", w)) + fill(gap, "┄", st.faint))
		}
	}
	return b.String()
}

// stepRuler is the line of beat numbers over the bars of sp, lined up with noteRow: the first
// step of each of a bar's four beats is numbered 1-4, the three steps between are dim ticks.
func (m Model) stepRuler(d *app.PatternDetail, sp stepSpan) string {
	ruler := strings.Repeat(" ", 20)
	for c := sp.lo; c < sp.hi; c++ {
		i := c - sp.lo
		if i%4 == 0 {
			ruler += m.st.dim.Render(fmt.Sprintf("%d", i%d.StepsPerPage()/4+1))
		} else {
			ruler += m.st.faint.Render("·")
		}
		ruler += strings.Repeat(" ", max(sp.w, 1)-1)
		if c+1 < sp.hi {
			ruler += m.st.faint.Render(sp.gap(i))
		}
	}
	return ruler
}

// loopStep is the step of d a loop frac (0 to 1) of the way through its round is on: within the
// range when one is set, since only that loops.
func (m Model) loopStep(d *app.PatternDetail, frac float64) int {
	lo, n := 0, d.Grid.Steps
	if r, ok := m.rangeOf(d); ok {
		lo, n = r.first*d.StepsPerPage(), r.bars()*d.StepsPerPage()
	}
	return lo + min(int(frac*float64(n)), n-1)
}

// viewPage is the bar of d to keep on show: while d loops, the bar the playhead is in, so the
// pages turn with the music; else, for the open pattern, the bar the cursor is on.
func (m Model) viewPage(d *app.PatternDetail) int {
	if frac, _, ok := m.loopAt(d.Slot); ok {
		return m.loopStep(d, frac) / d.StepsPerPage()
	}
	if d == m.pattern {
		return m.patPage()
	}
	return 0
}

// mixBadge is a pad's state in the mix played on this computer, shown after its row in a
// pattern: a red MUTE, a green SOLO, or nothing.
func (m Model) mixBadge(pad proto.PadIndex) string {
	badge := lipgloss.NewStyle().Bold(true).Foreground(m.st.th.cursorFg)
	switch {
	case m.muted[pad]:
		return "  " + badge.Background(m.st.th.bad).Render(" MUTE ")
	case m.soloed[pad]:
		return "  " + badge.Background(m.st.th.good).Render(" SOLO ")
	}
	return ""
}

// rowLabel is a pad's label in a pattern: greyed out, and struck through when muted, when the
// pad is not heard in the mix on this computer.
func (m Model) rowLabel(pad proto.PadIndex, label string, style lipgloss.Style) string {
	switch {
	case m.muted[pad]:
		style = m.st.faint.Strikethrough(true)
	case !heard(pad, m.muted, m.soloed):
		style = m.st.faint
	}
	return style.Render(label)
}

// padBars says how long the pad's sample is in bars at the bank's tempo: "4 bars" when whole,
// "3.6 bars" when not, and nothing for a sample shorter than about a bar, or before its length is
// known, or on a tile too narrow for it.
func (m Model) padBars(row app.PadRow, tileW int) string {
	if !row.HasSample || tileW < 14 {
		return ""
	}
	f, ok := m.facts[factKey{row.Pad, row.Size, row.Name}]
	if !ok || f.err != nil {
		return ""
	}
	b := loopBars(f.facts.Seconds, m.bankTempo(int(row.Pad.Bank())))
	switch {
	case b < 0.9:
		return ""
	case math.Abs(b-math.Round(b)) < 0.06:
		return plural(int(math.Round(b)), "bar")
	}
	return fmt.Sprintf("%.1f bars", b)
}
