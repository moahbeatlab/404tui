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

// searchLimit is how many results are listed.
const searchLimit = 300

// search is the keyword search over every audio file in a source: the index is built once,
// then results follow the keys you type.
type search struct {
	root     string
	input    string
	results  []app.IndexedFile
	at       int
	building bool
	// forPad is set when the search was opened from the assign dialog: Enter then puts the
	// file straight onto that pad.
	forPad  bool
	pad     proto.PadIndex
	confirm bool
}

type indexMsg struct {
	root string
	ix   *app.Index
	err  error
}

// openSearch opens the search over a source. The index is built the first time and kept
// until the files change.
func (m Model) openSearch(root string, forPad bool, pad proto.PadIndex) (tea.Model, tea.Cmd) {
	m.stopListening()
	s := &search{root: root, forPad: forPad, pad: pad}
	m.search = s
	if m.indexes[root] != nil {
		return m, m.wanted()
	}
	s.building = true
	unit := m.unit
	return m, m.edit(func() tea.Msg {
		ix, err := app.BuildIndex(unit, root, app.DefaultLimits)
		return indexMsg{root: root, ix: ix, err: err}
	})
}

// indexed handles a finished walk of a source.
func (m Model) indexed(msg indexMsg) (tea.Model, tea.Cmd) {
	s := m.search
	if msg.err != nil {
		if s != nil && s.root == msg.root {
			s.building = false
			m.message = fmt.Sprintf("searching %s failed: %v", display(msg.root), msg.err)
			m.search = nil
		}
		return m, nil
	}
	m.indexes[msg.root] = msg.ix
	if s != nil && s.root == msg.root {
		s.building = false
		s.results = msg.ix.Search(s.input, searchLimit)
		return m, m.wanted()
	}
	return m, nil
}

// current is the result in front.
func (s *search) current() (app.IndexedFile, bool) {
	if s.at < 0 || s.at >= len(s.results) {
		return app.IndexedFile{}, false
	}
	return s.results[s.at], true
}

func (m Model) searchKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := m.search
	name := key.String()
	refresh := func() {
		s.at, s.confirm = 0, false
		if ix := m.indexes[s.root]; ix != nil {
			s.results = ix.Search(s.input, searchLimit)
		}
	}
	switch {
	case name == "esc":
		m.search = nil
		m.stopListening()
		return m, nil
	case name == "up" || name == "ctrl+p":
		s.at, s.confirm = clamp(s.at-1, 0, max(len(s.results)-1, 0)), false
		wanted := m.wanted()
		play := m.scheduleAutoplay()
		return m, tea.Batch(wanted, play)
	case name == "down" || name == "ctrl+n":
		s.at, s.confirm = clamp(s.at+1, 0, max(len(s.results)-1, 0)), false
		wanted := m.wanted()
		play := m.scheduleAutoplay()
		return m, tea.Batch(wanted, play)
	case name == "pgup":
		s.at, s.confirm = clamp(s.at-10, 0, max(len(s.results)-1, 0)), false
		wanted := m.wanted()
		play := m.scheduleAutoplay()
		return m, tea.Batch(wanted, play)
	case name == "pgdown" || name == "ctrl+d":
		s.at, s.confirm = clamp(s.at+10, 0, max(len(s.results)-1, 0)), false
		wanted := m.wanted()
		play := m.scheduleAutoplay()
		return m, tea.Batch(wanted, play)
	case name == "ctrl+a":
		return m.toggleAutoplay()
	case name == "ctrl+l":
		if f, ok := s.current(); ok {
			return m.listen(f.Path)
		}
	case name == "tab":
		return m.reveal()
	case name == "enter":
		return m.searchEnter()
	case name == "backspace":
		if r := []rune(s.input); len(r) > 0 {
			s.input = string(r[:len(r)-1])
			refresh()
		}
	case name == "ctrl+u":
		s.input = ""
		refresh()
	case key.Type == tea.KeyRunes || key.Type == tea.KeySpace:
		s.input += string(key.Runes)
		refresh()
	}
	return m, m.wanted()
}

// searchEnter puts the result in front on a pad: straight onto the dialog's pad, or through
// the pad picker from the Files section.
func (m Model) searchEnter() (tea.Model, tea.Cmd) {
	s := m.search
	f, ok := s.current()
	if !ok {
		return m, nil
	}
	m.stopListening()
	if !s.forPad {
		m.search = nil
		m.picker = &picker{files: []string{f.Path}, bank: m.target, pad: m.nextFree(m.target)}
		return m, nil
	}
	// A pad that holds a sample is replaced only on a second Enter.
	if m.snap.Pads[s.pad.Index()].HasSample && !s.confirm {
		s.confirm = true
		return m, nil
	}
	pad := s.pad
	m.search, m.browser = nil, nil
	return m.assign([]string{f.Path}, pad)
}

// reveal shows the result in front in the file tree it came from.
func (m Model) reveal() (tea.Model, tea.Cmd) {
	s := m.search
	f, ok := s.current()
	if !ok {
		return m, nil
	}
	m.search = nil
	m.stopListening()
	open := app.Ancestors(s.root, f.Path)
	if s.forPad && m.browser != nil {
		b := m.browser
		b.root, b.tree, b.loading, b.confirm = s.root, nil, true, false
		return m, m.loadBrowser(s.root, open, f.Path)
	}
	m.section, m.root, m.tree = secFiles, s.root, nil
	m.busy = "listing files"
	unit, ops, root, path := m.unit, m.ops, s.root, f.Path
	return m, func() tea.Msg {
		ops.Lock()
		defer ops.Unlock()
		tree, err := app.LoadTree(unit, root, open)
		return treeMsg{root: root, tree: tree, err: err, at: path}
	}
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

// padTo fills s with spaces to w cells.
func padTo(s string, w int) string {
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}

// cutStart shortens s to w cells by cutting its start: "…/DEEP HOUSE/Basslines".
func cutStart(s string, w int) string {
	if ansi.StringWidth(s) <= w {
		return s
	}
	return "…" + ansi.TruncateLeft(s, ansi.StringWidth(s)-(w-1), "")
}

func (m Model) searchBox(g geom) string {
	st, s := m.st, m.search
	width := min(m.width-6, 112)
	leftW := width * 11 / 20
	height := max(min(g.bodyH-9, 20), 5)

	title := fmt.Sprintf("Search %s for audio files", rootName(s.root))
	if s.forPad {
		title += fmt.Sprintf(" for %s", s.pad)
	}
	status := ""
	switch ix := m.indexes[s.root]; {
	case s.building:
		status = st.accent.Render(spinner[m.frame%len(spinner)]) + st.dim.Render(" indexing "+rootName(s.root)+"… the first search takes a moment")
	case ix != nil:
		status = st.dim.Render(fmt.Sprintf("%d of %d files", len(s.results), len(ix.Files)))
		if ix.Truncated {
			status += st.warn.Render("  · the index is partial: it hit a limit")
		}
		if strings.TrimSpace(s.input) == "" {
			status = st.dim.Render(fmt.Sprintf("%d audio files indexed — type to search", len(ix.Files)))
		}
	}

	// Results are two aligned columns: the file name, then the folder it is in. The name
	// column is as wide as the longest name on screen, within limits, and a folder that does
	// not fit is cut at its start, so the part next to the file stays.
	from, to := window(len(s.results), s.at, height-1)
	nameW := 12
	for i := from; i < to; i++ {
		nameW = max(nameW, ansi.StringWidth(s.results[i].Name()))
	}
	nameW = min(nameW, leftW*65/100)
	dirW := max(leftW-nameW-4, 6)
	var rows []string
	if to > from {
		rows = append(rows, st.faint.Render(" "+padTo("Name", nameW)+"  Folder"))
	}
	for i := from; i < to; i++ {
		f := s.results[i]
		dir := strings.TrimSuffix(strings.TrimSuffix(f.Rel, f.Name()), "/")
		row := " " + padTo(ansi.Truncate(f.Name(), nameW, "…"), nameW) + "  " + cutStart(dir, dirW)
		if i == s.at {
			rows = append(rows, lipgloss.NewStyle().Background(st.th.cursorBg).Foreground(st.th.cursorFg).Bold(true).Width(leftW).Render(row))
			continue
		}
		rows = append(rows, st.text.Render(" "+padTo(ansi.Truncate(f.Name(), nameW, "…"), nameW))+"  "+st.dim.Render(cutStart(dir, dirW)))
	}
	if len(rows) == 0 && !s.building && strings.TrimSpace(s.input) != "" {
		rows = []string{st.dim.Render(" nothing matches")}
	}
	left := lipgloss.NewStyle().Width(leftW).Render(fit(strings.Join(rows, "\n"), height))

	right := ""
	if f, ok := s.current(); ok {
		node := app.Node{Path: f.Path, Name: f.Name()}
		lines := []string{st.accent.Render(f.Name()), st.dim.Render(ansi.Truncate(display(f.Path), width-leftW-6, "…")), ""}
		lines = append(lines, m.audioInfo(node, width-leftW-6)...)
		right = strings.Join(lines, "\n")
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, " ", lipgloss.NewStyle().Width(width-leftW-4).Render(right))

	held := ""
	if s.forPad {
		switch {
		case s.confirm:
			held = st.bad.Render("Press Enter again to replace " + fmt.Sprintf("%q on %s", m.snap.Pads[s.pad.Index()].Name, s.pad))
		case m.snap.Pads[s.pad.Index()].HasSample:
			held = st.warn.Render(fmt.Sprintf("%s holds %q — it will be replaced", s.pad, m.snap.Pads[s.pad.Index()].Name))
		}
	}
	lines := []string{
		st.bold.Render(title), "",
		st.accent.Render("> ") + s.input + "█", status, "",
		body, held,
		st.dim.Render("Enter assign  ·  Tab show in tree  ·  Ctrl-L listen  ·  Ctrl-A autoplay " + onOff(m.autoplay) + "  ·  ↑↓ move  ·  Esc close"),
	}
	return st.modal.Width(width).Render(strings.Join(lines, "\n"))
}
