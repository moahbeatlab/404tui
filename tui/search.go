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
	// tagsOnly makes the words match tags only (the index kept on the card), not names.
	tagsOnly bool
	// facets are the tags the matches have, with how many files, for the sidebar.
	facets []app.TagCount
	// want is the path of the result to put the cursor on when the results first come.
	want string
}

// facetLimit is how many tags the sidebar keeps.
const facetLimit = 300

// tagsLoadedMsg is the tag index read from the card for a tag search.
type tagsLoadedMsg struct {
	tags *app.TagIndex
	// none means there was no saved index and no walk was asked for.
	none bool
	err  error
}

// searchKind names the kind of a search, for what is remembered of it.
func searchKind(s *search) string {
	if s.tagsOnly {
		return "tags|" + s.root
	}
	return "files|" + s.root
}

// rememberSearch keeps what is typed in a search and the result in front, so that the next
// search of the same kind opens on them.
func (m *Model) rememberSearch(s *search) {
	if m.mem.Searches == nil {
		m.mem.Searches = map[string]SearchMemory{}
	}
	mem := SearchMemory{Query: s.input}
	if f, ok := s.current(); ok {
		mem.At = f.Path
	}
	m.mem.Searches[searchKind(s)] = mem
}

// fillResults lists the results for what is typed, with the cursor on the file that was in
// front when the search was last closed, if it is among them.
func (m *Model) fillResults(s *search) {
	s.results = m.searchResults(s)
	if s.tagsOnly {
		s.facets = m.tags.Facets(s.input, s.root, facetLimit)
	}
	s.at = 0
	for i, f := range s.results {
		if s.want != "" && f.Path == s.want {
			s.at = i
			break
		}
	}
	s.want = ""
}

// enterSearchMode puts a search in the state of its kind: what was typed last time comes back,
// and what the kind needs (the tags read from the card, or the file index built) is started
// when it is not there yet.
func (m *Model) enterSearchMode(s *search) tea.Cmd {
	mem := m.mem.Searches[searchKind(s)]
	s.input, s.want, s.at = mem.Query, mem.At, 0
	if s.tagsOnly {
		if m.tags != nil {
			m.fillResults(s)
			return m.wanted()
		}
		s.building = true
		unit, ops := m.unit, m.ops
		return func() tea.Msg {
			ops.Lock()
			defer ops.Unlock()
			tags, err := app.LoadTagIndex(unit)
			return tagsLoadedMsg{tags: tags, err: err}
		}
	}
	if m.indexes[s.root] != nil {
		m.fillResults(s)
		return m.wanted()
	}
	// The index is built in the background (it may already be); the dialog shows how far it is.
	s.building = true
	return m.startIndex(s.root, true)
}

// switchSearchTab goes between searching tags and searching file names, each with the query
// it had.
func (m Model) switchSearchTab() (tea.Model, tea.Cmd) {
	s := m.search
	m.rememberSearch(s)
	s.tagsOnly = !s.tagsOnly
	s.results, s.facets, s.building, s.confirm = nil, nil, false, false
	cmd := m.enterSearchMode(s)
	return m, cmd
}

// openTagSearch opens a search that matches tags only. The tag index is read from the card
// the first time; indexing a folder (i) is what puts files in it.
func (m Model) openTagSearch(root string, forPad bool, pad proto.PadIndex) (tea.Model, tea.Cmd) {
	m.stopListening()
	s := &search{root: root, forPad: forPad, pad: pad, tagsOnly: true}
	m.search = s
	cmd := m.enterSearchMode(s)
	return m, cmd
}

func (m Model) tagsLoaded(msg tagsLoadedMsg) (tea.Model, tea.Cmd) {
	s := m.search
	if msg.err != nil {
		m.message = "reading the tags failed: " + msg.err.Error()
		m.search = nil
		return m, nil
	}
	m.tags = msg.tags
	if s != nil && s.tagsOnly {
		s.building = false
		m.fillResults(s)
		return m, m.wanted()
	}
	return m, nil
}

// searchResults are the results for what is typed: files whose names and folders match, or
// with tagsOnly the files whose tags do.
func (m Model) searchResults(s *search) []app.IndexedFile {
	if s.tagsOnly {
		return m.tags.Matches(s.input, s.root, searchLimit)
	}
	if ix := m.indexes[s.root]; ix != nil {
		return ix.Search(s.input, searchLimit)
	}
	return nil
}

type indexMsg struct {
	root string
	ix   *app.Index
	tags *app.TagIndex
	// none means there was no saved index and no walk was asked for.
	none bool
	err  error
}

// openSearch opens the search over a source. The index is built the first time and kept
// until the files change.
func (m Model) openSearch(root string, forPad bool, pad proto.PadIndex) (tea.Model, tea.Cmd) {
	m.stopListening()
	s := &search{root: root, forPad: forPad, pad: pad}
	m.search = s
	cmd := m.enterSearchMode(s)
	return m, cmd
}

// indexed handles a finished walk of a source.
func (m Model) indexed(msg indexMsg) (tea.Model, tea.Cmd) {
	s := m.search
	if msg.err != nil {
		m.message = fmt.Sprintf("indexing %s for search failed: %v", display(msg.root), msg.err)
		if s != nil && s.root == msg.root && !s.tagsOnly {
			s.building = false
			m.search = nil
		}
		return m, nil
	}
	if msg.none {
		if m.tags == nil {
			m.tags = msg.tags
		}
		return m, nil
	}
	m.indexes[msg.root] = msg.ix
	if m.tags == nil {
		m.tags = msg.tags
	}
	if s != nil && s.root == msg.root && !s.tagsOnly {
		s.building = false
		m.fillResults(s)
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
		s.results = m.searchResults(s)
		if s.tagsOnly {
			s.facets = m.tags.Facets(s.input, s.root, facetLimit)
		}
	}
	switch {
	case name == "esc":
		m.rememberSearch(s)
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
		return m.switchSearchTab()
	case name == "ctrl+o":
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
	m.rememberSearch(s)
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
	m.rememberSearch(s)
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
	// The dialog is big in both tabs; the tags tab also has the tags to pick from, in a sidebar.
	sideW := 0
	width = min(m.width-4, 170)
	height = max(min(g.bodyH-9, 44), 5)
	leftW = (width - 2) * 11 / 20
	rightW := width - leftW - 4
	if s.tagsOnly {
		sideW = 28
		leftW = (width - sideW - 2) * 11 / 20
		rightW = width - sideW - leftW - 5
	}

	title := fmt.Sprintf("Search %s", rootName(s.root))
	if s.forPad {
		title += fmt.Sprintf(" for %s", s.pad)
	}
	tab := func(label string, on bool) string {
		if on {
			return lipgloss.NewStyle().Background(st.th.cursorBg).Foreground(st.th.cursorFg).Bold(true).Render(" " + label + " ")
		}
		return st.dim.Render(" " + label + " ")
	}
	tabs := tab("Files", !s.tagsOnly) + " " + tab("Tags", s.tagsOnly)
	status := ""
	switch ix := m.indexes[s.root]; {
	case s.tagsOnly && s.building:
		status = st.accent.Render(spinner[m.frame%len(spinner)]) + st.dim.Render(" reading "+app.TagIndexName+"…")
	case s.tagsOnly && (m.tags == nil || len(m.tags.Files) == 0):
		status = st.warn.Render("no tags yet: press i on a folder in the file list to index it")
	case s.tagsOnly:
		status = st.dim.Render(fmt.Sprintf("%d of %d tagged files — type tags, such as: bass 124", len(s.results), len(m.tags.Files)))
		if strings.TrimSpace(s.input) == "" {
			status = st.dim.Render(fmt.Sprintf("%d tagged files — type tags, such as: bass 124", len(m.tags.Files)))
		}
	case s.building:
		status = st.accent.Render(spinner[m.frame%len(spinner)]) + st.dim.Render(" indexing "+rootName(s.root)+"…")
		if job := m.searchJobs[s.root]; job != nil && job.active() {
			status += st.dim.Render(fmt.Sprintf(" %d/%d folders · %s — you can close this: it goes on", job.folders.Load(), job.folders.Load()+job.queued.Load(), plural(int(job.files.Load()), "file")))
		}
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
		lines := []string{st.accent.Render(f.Name()), st.dim.Render(ansi.Truncate(display(f.Path), rightW-2, "…")), ""}
		if tags := m.tags.Tags(f.Path); len(tags) > 0 {
			lines = append(lines, st.dim.Render("tags  ")+st.good.Render(ansi.Truncate(strings.Join(tags, " "), rightW-10, "…")), "")
		}
		lines = append(lines, m.audioInfo(node, rightW-2)...)
		right = strings.Join(lines, "\n")
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, " ", lipgloss.NewStyle().Width(rightW).Render(right))
	if s.tagsOnly {
		body = lipgloss.JoinHorizontal(lipgloss.Top, m.tagSidebar(s, sideW, height), " ", body)
	}

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
		st.bold.Render(title) + "   " + tabs, "",
		st.accent.Render("> ") + s.input + "█", status, "",
		body, held,
		st.dim.Render("Enter assign  ·  Tab files/tags  ·  Ctrl-O tree  ·  Ctrl-L listen  ·  Ctrl-A autoplay " + onOff(m.autoplay) + "  ·  ↑↓  ·  Esc"),
	}
	return st.modal.Width(width).Render(strings.Join(lines, "\n"))
}

// tagSidebar lists the tags the matches have, most files first, each with its count:
// "bass (341)". With nothing typed it is every tag; with a query it is what is left to
// narrow it by.
func (m Model) tagSidebar(s *search, width, height int) string {
	st := m.st
	title := "Tags"
	if strings.TrimSpace(s.input) != "" {
		title = "Narrow by"
	}
	rows := []string{st.faint.Render(padTo(title, width))}
	if len(s.facets) == 0 {
		rows = append(rows, st.dim.Render("no tags"))
	}
	for i, f := range s.facets {
		if i >= height-1 {
			break
		}
		count := fmt.Sprintf(" (%d)", f.Count)
		rows = append(rows, st.text.Render(ansi.Truncate(f.Tag, width-len(count), "…"))+st.dim.Render(count))
	}
	return lipgloss.NewStyle().Width(width).Render(fit(strings.Join(rows, "\n"), height))
}
