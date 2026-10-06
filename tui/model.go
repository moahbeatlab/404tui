// Package tui is the full-screen terminal UI. The Pads section is laid out like the unit's
// own 4×4 pad grid, with a strip of banks above it and the selected pad's waveform and
// settings below; Patterns, Files and Projects follow the same frame. Keys are vim-style,
// ":" opens a command palette, and the mouse works too.
package tui

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/device"
	"github.com/moahbeatlab/404tui/proto"
	"github.com/moahbeatlab/404tui/proto/fileapi"
)

// The sections, in the order of their number keys.
const (
	secPads = iota
	secPatterns
	secFiles
	secProjects
	secCount
)

var sectionNames = [secCount]string{"Pads", "Patterns", "Files", "Projects"}

const (
	tickEvery = 120 * time.Millisecond
	// Ticks between looks at what the unit reports on its own.
	pollTicks = 4
	// waveColumns is how many points of a waveform are kept; more than any screen is wide.
	waveColumns = 96
)

// promptKind is what a text prompt is for.
type promptKind int

const (
	promptRenamePad promptKind = iota
	promptRenameProject
	promptRenameFile
	promptBulkRename
	promptDelete
	promptExport
	promptRestore
	promptSearch
	// promptExportName names the folder a bank is exported into, inside the one chosen in
	// the folder browser (prompt.path).
	promptExportName
	// promptAssignFile takes the path of an audio file to put on prompt.pads[0].
	promptAssignFile
	promptProjectExport
	promptProjectExportName
	promptProjectRestore
	// promptConfirmRestore asks whether to overwrite a project with the backup read from
	// prompt.plan.
	promptConfirmRestore
)

// prompt is a question in front of the section: a name or path to type, or a delete to
// confirm.
type prompt struct {
	kind   promptKind
	buffer string
	// What the answer applies to.
	pads    []proto.PadIndex
	rows    []app.PadRow
	project uint8
	path    string
	// suffix: a bulk rename adds a suffix instead of a prefix.
	suffix bool
	// plan is the backup a project restore asks about.
	plan *app.RestorePlan
}

// picker is the pad chooser a file is assigned through, or the bank chooser of a
// retarget.
type picker struct {
	retarget bool
	// Assign: the files to put on pads, in order.
	files []string
	// retarget: the pattern slot and the bank its pads are in.
	slot       proto.PadIndex
	sourceBank int
	bank       int
	pad        int
	// digits is the pad number being typed.
	digits string
	// confirm is set once the chosen pad holds a sample and the user has been told.
	confirm bool
}

type palette struct {
	filter string
	cursor int
}

// Model is the whole UI. It never calls the unit from Update: every call runs as a
// command and comes back as a message, so the screen stays responsive.
type Model struct {
	unit   app.Unit
	player app.Player
	// ops keeps edits and the look at the unit's own reports apart: an edit waits for the
	// unit's verdict among those reports.
	ops           *sync.Mutex
	st            styles
	themeAt       int
	width, height int

	section int
	help    bool
	message string
	// busy names what the UI is waiting for; keys other than quit wait with it.
	busy    string
	frame   int
	polling bool

	prompt  *prompt
	picker  *picker
	palette *palette
	browser *browser

	snap *app.Snapshot
	// bank is the bank in front on Pads and Patterns; pad is the cursor within a bank.
	bank int
	pad  int
	// marked holds pads (by position in the bank) for bulk delete and rename.
	marked map[int]bool
	// moving is a pad picked up to be moved onto another.
	moving    proto.PadIndex
	hasMoving bool

	// selected is the pad the unit has selected, and playing the one sounding.
	selected    proto.PadIndex
	hasSelected bool
	playing     bool
	playingPad  proto.PadIndex

	patterns      map[int][]app.PatternRow
	patternCursor int
	pattern       *app.PatternDetail

	root      string
	tree      *app.Tree
	fileAt    int
	fileMoved string
	// fileMarks are audio files picked for a multi-file assign.
	fileMarks map[string]bool
	// listening is the file being auditioned.
	listening string

	projectAt int

	waves    map[waveKey][]proto.Peak
	waveBusy bool
	infos    map[string]fileInfo
	infoBusy bool

	// lastAssigned is the pad a file last went onto; a file goes to the next free one.
	lastAssigned proto.PadIndex
	hasLast      bool
	// target is the bank an assigned file goes to; it follows the last pad assigned to, and
	// is kept apart from the bank in view on Pads.
	target int

	// store keeps the state between runs; mem is what it holds and saved the last bytes
	// written, shared by the copies of the model.
	store Store
	mem   State
	saved *[]byte
}

// waveKey identifies the sample a cached waveform belongs to.
type waveKey struct {
	pad  proto.PadIndex
	size uint32
	name string
}

// fileInfo is what is known of an audio file after decoding it.
type fileInfo struct {
	err      error
	rate     int
	channels int
	frames   int
	peaks    []int16
}

// New returns the UI for a connected unit. player may be nil, in which case files cannot
// be auditioned.
func New(unit app.Unit, player app.Player, store Store) Model {
	routed := app.NewRouted(unit)
	m := Model{
		unit:      routed,
		player:    player,
		ops:       &sync.Mutex{},
		st:        newStyles(themes[0]),
		width:     100,
		height:    30,
		busy:      "reading the unit",
		marked:    map[int]bool{},
		patterns:  map[int][]app.PatternRow{},
		root:      app.CardRoot,
		fileMarks: map[string]bool{},
		waves:     map[waveKey][]proto.Peak{},
		infos:     map[string]fileInfo{},
		store:     store,
		saved:     new([]byte),
	}
	if store != nil {
		m.mem = store.Load()
		if m.mem.Theme >= 0 && m.mem.Theme < len(themes) {
			m.themeAt = m.mem.Theme
			m.st = newStyles(themes[m.themeAt])
		}
		m.bank = clamp(m.mem.Bank, 0, proto.Banks-1)
		m.target = clamp(m.mem.TargetBank, 0, proto.Banks-1)
		if i := m.mem.LastAssigned - 1; i >= 0 {
			if pad, ok := proto.NewPadIndex(uint16(i)); ok {
				m.lastAssigned, m.hasLast = pad, true
			}
		}
		if root, ok := rootFromKey(m.mem.FilesRoot); ok {
			m.root = root
		}
	}
	*m.saved = stateBytes(m.mem)
	return m
}

// persist writes the state if it changed since it was last written.
func (m *Model) persist() {
	if m.store == nil {
		return
	}
	m.mem.Theme, m.mem.Bank, m.mem.TargetBank = m.themeAt, m.bank, m.target
	m.mem.LastAssigned = 0
	if m.hasLast {
		m.mem.LastAssigned = int(m.lastAssigned.Index()) + 1
	}
	m.mem.FilesRoot = rootKey(m.root)
	if m.tree != nil {
		m.mem.FilesExpanded = m.tree.Expanded()
		if node, ok := m.fileNode(); ok {
			m.mem.FilesAt = node.Path
		}
	}
	if b := stateBytes(m.mem); !bytes.Equal(b, *m.saved) {
		m.store.Save(m.mem)
		*m.saved = b
	}
}

// ---- messages ----

type (
	tickMsg   struct{}
	loadedMsg struct {
		snap    *app.Snapshot
		err     error
		message string
	}
	// doneMsg ends an edit; the pads are read again when reload is set. focus, when set,
	// puts the cursor on that pad.
	doneMsg struct {
		message string
		reload  bool
		focus   *proto.PadIndex
		// assigned marks a file put on pad focus.
		assigned bool
	}
	patternsMsg struct {
		bank int
		rows []app.PatternRow
		err  error
	}
	patternMsg struct {
		detail *app.PatternDetail
		err    error
		note   string
	}
	dirMsg struct {
		root    string
		path    string
		row     int
		entries []fileapi.DirEntry
		err     error
	}
	// treeMsg carries the tree listed again after a change to the files.
	treeMsg struct {
		root string
		tree *app.Tree
		err  error
		note string
		// at is the path the cursor goes to, when it is in the tree.
		at string
	}
	pollMsg struct {
		changed  bool
		activity device.Activity
		have     bool
	}
	waveMsg struct {
		key   waveKey
		peaks []proto.Peak
		err   error
	}
	infoMsg struct {
		path string
		info fileInfo
	}
	// browseMsg carries the tree the folder browser opens on; browseDirMsg the entries of a
	// folder opened in it.
	browseMsg struct {
		root string
		tree *app.Tree
		at   string
		err  error
	}
	browseDirMsg struct {
		root    string
		path    string
		row     int
		entries []fileapi.DirEntry
		err     error
	}
	// planMsg carries a backup read and checked, to be confirmed before it overwrites a project.
	planMsg struct {
		plan *app.RestorePlan
		err  error
	}
	noteMsg string
)

func tick() tea.Cmd {
	return tea.Tick(tickEvery, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.load(""), tick())
}

// edit runs a change on the unit as a command, apart from the poll of its reports.
func (m *Model) edit(change func() tea.Msg) tea.Cmd {
	ops := m.ops
	return func() tea.Msg {
		ops.Lock()
		defer ops.Unlock()
		return change()
	}
}

func (m Model) load(message string) tea.Cmd {
	return func() tea.Msg {
		m.ops.Lock()
		defer m.ops.Unlock()
		snap, err := app.Load(m.unit)
		return loadedMsg{snap: snap, err: err, message: message}
	}
}

func (m Model) loadDir(path string, row int) tea.Cmd {
	root := m.root
	return func() tea.Msg {
		m.ops.Lock()
		defer m.ops.Unlock()
		entries, err := m.unit.ListDir(path)
		return dirMsg{root: root, path: path, row: row, entries: entries, err: err}
	}
}

func (m Model) loadPatterns(bank int) tea.Cmd {
	return func() tea.Msg {
		m.ops.Lock()
		defer m.ops.Unlock()
		rows, err := app.LoadPatterns(m.unit, bank)
		return patternsMsg{bank: bank, rows: rows, err: err}
	}
}

func (m Model) openPattern(slot proto.PadIndex, note string) tea.Cmd {
	return func() tea.Msg {
		m.ops.Lock()
		defer m.ops.Unlock()
		detail, err := app.OpenPattern(m.unit, slot)
		return patternMsg{detail: detail, err: err, note: note}
	}
}

func (m Model) poll() tea.Cmd {
	unit, ops := m.unit, m.ops
	return func() tea.Msg {
		if !ops.TryLock() {
			return pollMsg{}
		}
		defer ops.Unlock()
		msg := pollMsg{changed: app.ProjectChanged(unit.TakeEvents())}
		if a, err := unit.Activity(); err == nil {
			msg.activity, msg.have = a, true
		}
		return msg
	}
}

// padWave reads the waveform of a pad's sample.
func (m Model) padWave(key waveKey, frames uint32, channels int) tea.Cmd {
	return func() tea.Msg {
		m.ops.Lock()
		defer m.ops.Unlock()
		per := max((frames+waveColumns-1)/waveColumns, 1)
		peaks, err := m.unit.Peaks(key.pad, 1, 0, min(waveColumns, frames), per)
		return waveMsg{key: key, peaks: peaks, err: err}
	}
}

// fileInfoCmd decodes an audio file to describe it and draw its waveform.
func (m Model) fileInfoCmd(path string) tea.Cmd {
	return func() tea.Msg {
		imported, err := app.LoadSample(m.unit, path)
		if err != nil {
			return infoMsg{path: path, info: fileInfo{err: err}}
		}
		s := imported.Sample
		return infoMsg{path: path, info: fileInfo{
			rate: imported.SourceRate, channels: imported.SourceChannels, frames: s.Frames(),
			peaks: columns(s.Samples, s.Channels, waveColumns),
		}}
	}
}

// columns reduces audio to the loudest absolute value in each of n slices of it.
func columns(samples []int16, channels, n int) []int16 {
	frames := len(samples) / max(channels, 1)
	if frames == 0 {
		return nil
	}
	n = min(n, frames)
	out := make([]int16, n)
	for c := 0; c < n; c++ {
		lo, hi := c*frames/n, (c+1)*frames/n
		var peak int32
		for f := lo; f < hi; f++ {
			for ch := 0; ch < channels; ch++ {
				v := int32(samples[f*channels+ch])
				if v < 0 {
					v = -v
				}
				peak = max(peak, v)
			}
		}
		out[c] = int16(min(peak, 32767))
	}
	return out
}

// ---- update ----

// Update handles a message, then keeps the state that is remembered between runs.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := m.update(msg)
	if updated, ok := model.(Model); ok {
		updated.persist()
		return updated, cmd
	}
	return model, cmd
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.MouseMsg:
		return m.mouse(msg)
	case tickMsg:
		m.frame++
		if m.frame%pollTicks == 0 && m.busy == "" && !m.polling {
			m.polling = true
			return m, tea.Batch(tick(), m.poll())
		}
		return m, tick()
	case pollMsg:
		return m.polled(msg)
	case loadedMsg:
		m.busy = ""
		if msg.err != nil {
			m.message = "reading the unit failed: " + msg.err.Error()
			break
		}
		// Another project's patterns and waveforms are not this one's.
		if m.snap == nil || m.snap.Project != msg.snap.Project {
			m.patterns = map[int][]app.PatternRow{}
			m.pattern = nil
		}
		m.waves = map[waveKey][]proto.Peak{}
		m.snap = msg.snap
		if m.hasSelected = msg.snap.HasSelected; m.hasSelected {
			m.selected = msg.snap.Selected
		}
		if msg.message != "" {
			m.message = msg.message
		}
		return m, m.wanted()
	case doneMsg:
		m.message = msg.message
		if msg.focus != nil {
			m.section, m.bank, m.pad = secPads, int(msg.focus.Bank()), int(msg.focus.Pad())
			if msg.assigned {
				m.lastAssigned, m.hasLast = *msg.focus, true
				m.target = int(msg.focus.Bank())
			}
		}
		if msg.reload {
			m.busy = "reading the unit"
			return m, m.load(msg.message)
		}
		m.busy = ""
	case noteMsg:
		m.message = string(msg)
	case patternsMsg:
		m.busy = ""
		if msg.err != nil {
			m.message = "reading patterns failed: " + msg.err.Error()
			break
		}
		m.patterns[msg.bank] = msg.rows
	case patternMsg:
		m.busy = ""
		if msg.err != nil {
			m.message = msg.err.Error()
			break
		}
		m.pattern = msg.detail
		if msg.note != "" {
			m.message = msg.note
		}
	case dirMsg:
		m.busy = ""
		switch {
		case msg.err != nil:
			m.message = fmt.Sprintf("listing %s failed: %v", display(msg.path), msg.err)
		// The answer to a question about a volume the tree has since left.
		case msg.root != m.root:
		case msg.row < 0:
			m.tree = app.NewTree(msg.path, msg.entries)
			m.fileAt = 0
			return m, m.wanted()
		case m.tree != nil && msg.row < len(m.tree.Rows()) && m.tree.Rows()[msg.row].Path == msg.path:
			m.tree.Expand(msg.row, msg.entries)
		}
	case treeMsg:
		m.busy = ""
		m.message = msg.note
		switch {
		case msg.err != nil:
			m.message = fmt.Sprintf("%s; listing %s failed: %v", msg.note, display(msg.root), msg.err)
		case msg.root == m.root:
			m.tree = msg.tree
			m.fileAt = max(min(m.fileAt, len(msg.tree.Rows())-1), 0)
			if i := msg.tree.Index(msg.at); msg.at != "" && i >= 0 {
				m.fileAt = i
			}
			return m, m.wanted()
		}
	case browseMsg:
		if b := m.browser; b != nil && msg.root == b.root {
			b.loading = false
			if msg.err != nil {
				m.message = fmt.Sprintf("listing %s failed: %v", display(msg.root), msg.err)
				break
			}
			b.tree = msg.tree
			if i := msg.tree.Index(msg.at); i >= 0 {
				b.at = i
			}
			return m, m.wanted()
		}
	case browseDirMsg:
		if b := m.browser; b != nil && msg.root == b.root && b.tree != nil {
			b.loading = false
			switch {
			case msg.err != nil:
				m.message = fmt.Sprintf("listing %s failed: %v", display(msg.path), msg.err)
			case msg.row < len(b.tree.Rows()) && b.tree.Rows()[msg.row].Path == msg.path:
				b.tree.Expand(msg.row, msg.entries)
			}
			return m, m.wanted()
		}
	case planMsg:
		m.busy = ""
		if msg.err != nil {
			m.message = "restore: " + msg.err.Error()
			break
		}
		m.prompt = &prompt{kind: promptConfirmRestore, plan: msg.plan}
	case waveMsg:
		m.waveBusy = false
		if msg.err == nil {
			m.waves[msg.key] = msg.peaks
		} else {
			m.waves[msg.key] = []proto.Peak{}
		}
		return m, m.wanted()
	case infoMsg:
		m.infoBusy = false
		m.infos[msg.path] = msg.info
		return m, m.wanted()
	case tea.KeyMsg:
		return m.key(msg)
	}
	return m, nil
}

// polled applies what the unit reported on its own: a project change reloads, and the pad
// it has selected or is playing is followed.
func (m Model) polled(msg pollMsg) (tea.Model, tea.Cmd) {
	m.polling = false
	if msg.have {
		a := msg.activity
		m.playing = a.Playing
		if a.HasSelected {
			if a.Playing {
				m.playingPad = a.Selected
			}
			moved := !m.hasSelected || a.Selected != m.selected
			m.selected, m.hasSelected = a.Selected, true
			// Hitting a pad on the unit moves the cursor to it.
			if moved && m.section == secPads && m.prompt == nil && m.picker == nil && m.busy == "" {
				m.bank, m.pad = int(a.Selected.Bank()), int(a.Selected.Pad())
				m.marked = map[int]bool{}
			}
		}
	}
	if msg.changed && m.busy == "" {
		m.busy = "reading the unit"
		return m, m.load("")
	}
	return m, m.wanted()
}

// wanted starts whatever the section in front needs and is not in hand yet: the patterns of
// a bank, the file listing, a waveform.
func (m *Model) wanted() tea.Cmd {
	if m.busy != "" || m.snap == nil {
		return nil
	}
	if b := m.browser; b != nil && b.kind == browseAssign && b.tree != nil && b.at < len(b.tree.Rows()) && !m.infoBusy {
		node := b.tree.Rows()[b.at]
		if _, have := m.infos[node.Path]; !node.Dir && isAudio(node.Name) && !have {
			m.infoBusy = true
			return m.fileInfoCmd(node.Path)
		}
	}
	switch m.section {
	case secPatterns:
		if m.patterns[m.bank] == nil {
			m.busy = "reading patterns"
			return m.loadPatterns(m.bank)
		}
	case secFiles:
		if m.tree == nil {
			m.busy = "listing files"
			return m.loadFilesTree()
		}
		if node, ok := m.fileNode(); ok && !node.Dir && isAudio(node.Name) && !m.infoBusy {
			if _, have := m.infos[node.Path]; !have {
				m.infoBusy = true
				return m.fileInfoCmd(node.Path)
			}
		}
	case secPads:
		if row := m.padRow(); row.HasSample && !m.waveBusy {
			key := waveKey{row.Pad, row.Size, row.Name}
			if _, have := m.waves[key]; !have {
				channels := 1
				m.waveBusy = true
				frames := (row.Size - min(row.Size, 512)) / 2
				return m.padWave(key, max(frames, 1), channels)
			}
		}
	}
	return nil
}

// loadFilesTree lists the files root with the folders that were open last time open again,
// and the cursor where it was.
func (m *Model) loadFilesTree() tea.Cmd {
	root, unit, ops := m.root, m.unit, m.ops
	var open []string
	at := ""
	if rootKey(root) == m.mem.FilesRoot {
		open, at = m.mem.FilesExpanded, m.mem.FilesAt
	}
	return func() tea.Msg {
		ops.Lock()
		defer ops.Unlock()
		tree, err := app.LoadTree(unit, root, open)
		return treeMsg{root: root, tree: tree, err: err, at: at}
	}
}

// nextFree is the pad a file should go to in a bank: the first one without a sample after
// the pad that last received a file, else the first free one; position 0 when it is full.
func (m *Model) nextFree(bank int) int {
	start := 0
	if m.hasLast && int(m.lastAssigned.Bank()) == bank {
		start = int(m.lastAssigned.Pad()) + 1
	}
	rows := m.snap.Bank(bank)
	for i := 0; i < proto.PadsPerBank; i++ {
		if p := (start + i) % proto.PadsPerBank; !rows[p].HasSample {
			return p
		}
	}
	return 0
}

func (m *Model) padRow() app.PadRow {
	return m.snap.Bank(m.bank)[m.pad]
}

func (m *Model) fileNode() (app.Node, bool) {
	if m.tree == nil || m.fileAt >= len(m.tree.Rows()) {
		return app.Node{}, false
	}
	return m.tree.Rows()[m.fileAt], true
}

// padAt is the pad at position p of the bank in front.
func (m *Model) padAt(p int) proto.PadIndex {
	pad, _ := proto.PadFromBankPad(uint16(m.bank), uint16(p))
	return pad
}

// display is a path as shown: the unit's own storage has no prefix to tell it apart.
func display(path string) string {
	if path == app.StorageRoot {
		return "the unit's storage"
	}
	return path
}

func isAudio(name string) bool {
	return strings.Contains(name, ".") && app.IsAudioName(name)
}
