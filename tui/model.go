// Package tui is the full-screen terminal UI. The Pads section is laid out like the unit's
// own 4×4 pad grid, with a strip of banks above it and the selected pad's waveform and
// settings below; Patterns, Files and Projects follow the same frame. Keys are vim-style,
// ":" opens a command palette, and the mouse works too.
package tui

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/device"
	"github.com/moahbeatlab/404tui/formats"
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

// previewStopAfter is how long a preview plays before it is stopped; tests shorten it.
var previewStopAfter = func(d time.Duration) time.Duration { return d }

// tickEvery is how often the UI ticks; tests shorten it.
var tickEvery = 120 * time.Millisecond

const ( // Ticks between looks at what the unit reports on its own.
	pollTicks = 4
	// waveColumns is how many points of a waveform are kept; more than any screen is wide.
	waveColumns = 320
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
	// promptDeleteFiles confirms deleting prompt.paths from the card; promptMkdir names a new
	// folder in prompt.path.
	promptDeleteFiles
	promptMkdir
	// promptFindFolder jumps to the next folder in the list whose name contains the text.
	promptFindFolder
	// promptBank edits a bank's tempo and volume: two fields, Tab moves between them.
	promptBank
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
	// paths and count are what a file delete is about to remove; reopen is the pad of the assign
	// dialog the question came from.
	paths  []string
	count  app.Count
	reopen *proto.PadIndex
	// fields and field are the bank dialog's tempo and volume, and the one being typed.
	fields [2]string
	field  int
	// choice is the folder picked from the matches while one is looked for (f).
	choice int
}

// picker is the pad chooser a file is assigned through, or the bank chooser of a
// retarget.
type picker struct {
	retarget bool
	// Assign: the files to put on pads, in order.
	files []string
	// restorePlan: an export being put back; the picker chooses the bank it goes onto.
	restorePlan *app.PadRestore
	// copyFrom: the pads being copied; the picker chooses where the first one goes.
	copyFrom []proto.PadIndex
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

// ErrNoUnit is what a Connect returns when no unit is plugged in yet; the UI keeps waiting
// quietly instead of reporting it.
var ErrNoUnit = errors.New("no unit")

// Connect opens a link to a unit. The UI calls it again and again until it succeeds, and
// again after the link is lost.
type Connect func() (app.Unit, io.Closer, error)

// connectTicks is how many ticks pass between attempts to find a unit.
const connectTicks = 8

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
	search  *search
	editor  *editor
	// chromatic is the dialog that plays a pad as notes from the keyboard.
	chromatic *chromatic
	// indexes are the file indexes searches use, one per source, kept until files change.
	indexes map[string]*app.Index

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
	// playPos is how many frames of it have played.
	playPos uint32

	// previewSeq numbers the pad previews on the unit, so that the stop timer of an earlier one
	// does nothing once a later one has started; previewing is the pad playing.
	previewSeq int
	previewing *proto.PadIndex
	// previewLocal plays a pad's sample on this computer, as a file is auditioned, instead of on
	// the unit; padAudio keeps the samples read for it.
	previewLocal bool
	padAudio     *padCache
	// audioOut names the computer's audio output, to say where listening goes.
	audioOut string

	patterns      map[int][]app.PatternRow
	patternCursor int
	pattern       *app.PatternDetail

	root   string
	tree   *app.Tree
	fileAt int
	// fileMarks are the files and folders marked in the list, for assign, delete, copy and move;
	// clip is what was copied or cut, kept across a change of source so a paste can land on
	// the other side.
	fileMarks map[string]bool
	clip      []string
	clipCut   bool
	// listening is the file being auditioned; autoplay plays a result as the cursor lands on
	// it, after autoSeq stops changing; audioErr says why there is no audio output.
	listening string
	// listenGen numbers the listen requests, shared by every copy of the model: a file that is
	// still being decoded when the sound is stopped or another is asked for must not start.
	listenGen *atomic.Int64
	autoplay  bool
	autoSeq   int
	audioErr  string

	projectAt int

	// facts are what the SMP file of each pad says (length, channels), read as the pad is
	// looked at; usage tells which pattern slots play each pad.
	facts      map[factKey]padFacts
	factsBusy  bool
	usage      map[proto.PadIndex][]proto.PadIndex
	usageState int // 0: not read, 1: reading, 2: in hand
	infos      map[string]fileInfo
	infoBusy   bool

	// lastAssigned is the pad a file last went onto; a file goes to the next free one.
	lastAssigned proto.PadIndex
	hasLast      bool
	// target is the bank an assigned file goes to; it follows the last pad assigned to, and
	// is kept apart from the bank in view on Pads.
	target int

	// connect finds a unit when there is none; closer closes the one in use.
	connect        Connect
	closer         io.Closer
	connecting     bool
	connectedFrame int
	lost           bool
	waitErr        string

	// log is the debug log (discarding when there is none); logPath says where it goes.
	log     *slog.Logger
	logPath string

	// store keeps the state between runs; mem is what it holds and saved the last bytes
	// written, shared by the copies of the model.
	store Store
	mem   State
	saved *[]byte
}

// padFacts is the outcome of reading a pad's sample header.
type padFacts struct {
	facts app.PadFacts
	err   error
}

// factKey identifies the sample cached facts belong to.
type factKey struct {
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
	var routed app.Unit
	if unit != nil {
		routed = app.NewRouted(unit)
	}
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
		facts:     map[factKey]padFacts{},
		infos:     map[string]fileInfo{},
		indexes:   map[string]*app.Index{},
		listenGen: new(atomic.Int64),
		padAudio:  &padCache{samples: map[factKey]formats.Sample{}},
		store:     store,
		saved:     new([]byte),
		log:       slog.New(slog.DiscardHandler),
	}
	if store != nil {
		m.mem = store.Load()
		if m.mem.Theme >= 0 && m.mem.Theme < len(themes) {
			m.themeAt = m.mem.Theme
			m.st = newStyles(themes[m.themeAt])
		}
		m.bank = clamp(m.mem.Bank, 0, proto.Banks-1)
		m.target = clamp(m.mem.TargetBank, 0, proto.Banks-1)
		m.previewLocal = m.mem.PreviewLocal
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
	if unit == nil {
		m.busy = ""
	}
	return m
}

// AudioOutputMsg tells the UI the name of the computer's audio output; send it to the
// program once it is known.
type AudioOutputMsg string

// WithAudio tells the UI about the audio output: the player, or why there is none.
func (m Model) WithAudio(player app.Player, err error) Model {
	m.player = player
	if err != nil {
		m.audioErr = err.Error()
	}
	return m
}

// WithLogger turns on the debug log: connections, every operation on the unit with its
// timing, errors, and the keys pressed. path is shown so it can be found.
func (m Model) WithLogger(l *slog.Logger, path string) Model {
	m.log, m.logPath = l, path
	l.Info("ui ready", "unit connected", m.unit != nil)
	return m
}

// NewConnecting returns the UI for a unit that may not be plugged in yet: it waits, with a
// screen saying so, connects by itself, and reconnects if the cable is pulled.
func NewConnecting(connect Connect, player app.Player, store Store) Model {
	m := New(nil, player, store)
	m.connect = connect
	return m
}

// persist writes the state if it changed since it was last written.
func (m *Model) persist() {
	if m.store == nil {
		return
	}
	m.mem.Theme, m.mem.Bank, m.mem.TargetBank = m.themeAt, m.bank, m.target
	m.mem.PreviewLocal = m.previewLocal
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
		// lost is set when the link to the unit is gone.
		lost bool
	}
	// connectMsg is the outcome of an attempt to find a unit.
	connectMsg struct {
		unit   app.Unit
		closer io.Closer
		err    error
	}
	factsMsg struct {
		key   factKey
		facts app.PadFacts
		err   error
	}
	usageMsg struct {
		usage   map[proto.PadIndex][]proto.PadIndex
		err     error
		project uint8
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
	// padRestoreMsg carries a pad export that was read, to choose where it goes.
	padRestoreMsg struct {
		plan *app.PadRestore
		err  error
	}
	// planMsg carries a backup read and checked, to be confirmed before it overwrites a project.
	planMsg struct {
		plan *app.RestorePlan
		err  error
	}
	// previewMsg is the outcome of starting a pad previewing on the unit; previewStopMsg fires
	// when it should stop.
	previewMsg struct {
		pad      proto.PadIndex
		seq      int
		err      error
		duration time.Duration
	}
	previewStopMsg struct {
		pad proto.PadIndex
		seq int
	}
	// listenMsg is the outcome of starting a file playing.
	listenMsg struct {
		path string
		err  error
		// stale: something else was asked for while this one was decoding, so it was dropped.
		stale bool
	}
	// autoplayMsg fires a moment after the cursor stopped on a result.
	autoplayMsg struct{ seq int }
	noteMsg     string
)

func tick() tea.Cmd {
	return tea.Tick(tickEvery, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m Model) Init() tea.Cmd {
	if m.unit == nil {
		return tea.Batch(m.tryConnect(), tick())
	}
	return tea.Batch(m.load(""), tick())
}

// tryConnect looks for a unit and opens it.
func (m Model) tryConnect() tea.Cmd {
	connect := m.connect
	if connect == nil {
		return nil
	}
	return func() tea.Msg {
		unit, closer, err := connect()
		return connectMsg{unit: unit, closer: closer, err: err}
	}
}

// edit runs a change on the unit as a command, apart from the poll of its reports.
func (m *Model) edit(change func() tea.Msg) tea.Cmd {
	ops, log := m.ops, m.log
	return func() tea.Msg {
		ops.Lock()
		defer ops.Unlock()
		start := time.Now()
		msg := change()
		log.Debug("operation on the unit", "took", time.Since(start).Round(time.Millisecond), "result", describe(msg))
		return msg
	}
}

// describe is a message in a few words, for the log.
func describe(msg tea.Msg) string {
	switch msg := msg.(type) {
	case doneMsg:
		return "done: " + msg.message
	case noteMsg:
		return "note: " + string(msg)
	case patternMsg:
		if msg.err != nil {
			return "pattern error: " + msg.err.Error()
		}
		return "pattern: " + msg.note
	case treeMsg:
		if msg.err != nil {
			return "tree error: " + msg.err.Error()
		}
		return "tree: " + msg.note
	case padRestoreMsg:
		if msg.err != nil {
			return "export read error: " + msg.err.Error()
		}
		return fmt.Sprintf("export read: %d pads", len(msg.plan.Pads))
	case planMsg:
		if msg.err != nil {
			return "restore plan error: " + msg.err.Error()
		}
		return fmt.Sprintf("restore plan: %d files, %d bytes", len(msg.plan.Files), msg.plan.Total)
	case indexMsg:
		if msg.err != nil {
			return "index error: " + msg.err.Error()
		}
		return fmt.Sprintf("index of %s: %d files, truncated %v", display(msg.root), len(msg.ix.Files), msg.ix.Truncated)
	case deletePlanMsg:
		return fmt.Sprintf("delete plan: %d files, %d folders, err %v", msg.count.Files, msg.count.Folders, msg.err)
	case nil:
		return "nothing"
	}
	return fmt.Sprintf("%T", msg)
}

func (m Model) load(message string) tea.Cmd {
	return func() tea.Msg {
		m.ops.Lock()
		defer m.ops.Unlock()
		start := time.Now()
		snap, err := app.Load(m.unit)
		m.log.Debug("loaded the project", "took", time.Since(start).Round(time.Millisecond), "err", err)
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
		a, err := unit.Activity()
		switch {
		case err == nil:
			msg.activity, msg.have = a, true
		case errors.Is(err, device.ErrClosed):
			msg.lost = true
		}
		return msg
	}
}

// padFactsCmd reads the header of a pad's sample file.
func (m Model) padFactsCmd(key factKey, row app.PadRow) tea.Cmd {
	return func() tea.Msg {
		m.ops.Lock()
		defer m.ops.Unlock()
		facts, err := app.ReadPadFacts(m.unit, row)
		return factsMsg{key: key, facts: facts, err: err}
	}
}

// usageCmd finds out which patterns play which pads.
func (m Model) usageCmd(project uint8) tea.Cmd {
	return func() tea.Msg {
		m.ops.Lock()
		defer m.ops.Unlock()
		start := time.Now()
		usage, err := app.PatternUsage(m.unit)
		m.log.Debug("read which patterns play which pads", "took", time.Since(start).Round(time.Millisecond), "pads", len(usage), "err", err)
		return usageMsg{usage: usage, err: err, project: project}
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
		if m.unit == nil {
			if m.connect != nil && !m.connecting && m.frame%connectTicks == 0 {
				m.connecting = true
				return m, tea.Batch(tick(), m.tryConnect())
			}
			return m, tick()
		}
		if m.frame%pollTicks == 0 && m.busy == "" && !m.polling {
			m.polling = true
			return m, tea.Batch(tick(), m.poll())
		}
		return m, tick()
	case pollMsg:
		return m.polled(msg)
	case connectMsg:
		return m.connected(msg)
	case indexMsg:
		return m.indexed(msg)
	case loadedMsg:
		m.busy = ""
		if msg.err != nil && errors.Is(msg.err, device.ErrClosed) {
			return m.lose()
		}
		if msg.err != nil {
			m.log.Error("reading the unit failed", "err", msg.err)
			m.message = "reading the unit failed: " + msg.err.Error()
			break
		}
		// Another project's patterns and waveforms are not this one's.
		if m.snap == nil || m.snap.Project != msg.snap.Project {
			m.patterns = map[int][]app.PatternRow{}
			m.pattern = nil
			m.usage, m.usageState = nil, 0
		}
		m.facts = map[factKey]padFacts{}
		m.padAudio.clear()
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
	case AudioOutputMsg:
		m.audioOut = string(msg)
		m.log.Info("audio output", "device", m.audioOut)
	case previewMsg:
		if msg.seq != m.previewSeq {
			break
		}
		if msg.err != nil {
			m.previewing = nil
			m.message = fmt.Sprintf("preview %s failed: %v", msg.pad, msg.err)
			break
		}
		pad, seq := msg.pad, msg.seq
		return m, tea.Tick(previewStopAfter(msg.duration), func(time.Time) tea.Msg { return previewStopMsg{pad: pad, seq: seq} })
	case previewStopMsg:
		if msg.seq != m.previewSeq || m.previewing == nil {
			break
		}
		m.previewing = nil
		unit, pad := m.unit, msg.pad
		return m, m.edit(func() tea.Msg {
			_ = app.StopPreview(unit, pad)
			return nil
		})
	case chromaticMsg:
		return m.chromaticLoaded(msg)
	case listenMsg:
		if msg.stale {
			break
		}
		if msg.err != nil {
			if m.listening == msg.path {
				m.listening = ""
			}
			m.message = "listening failed: " + msg.err.Error()
		} else if m.listening == msg.path {
			m.message = "♪ playing " + app.Base(msg.path)
			if m.audioOut != "" {
				m.message += " on " + m.audioOut
			}
		}
	case autoplayMsg:
		if m.autoplay && msg.seq == m.autoSeq {
			if path := m.autoplayPath(); path != "" {
				return m.playFile(path)
			}
		}
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
			// A retarget changes which pads the patterns play.
			m.usage, m.usageState = nil, 0
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
	case padRestoreMsg:
		m.busy = ""
		if msg.err != nil {
			m.message = "restore: " + msg.err.Error()
			m.log.Warn("could not read the export", "err", msg.err)
			break
		}
		if bank, single := msg.plan.SingleBank(); single {
			// The pads can go to any bank: ask which.
			m.picker = &picker{restorePlan: msg.plan, bank: bank, sourceBank: bank}
			break
		}
		// An export that spans banks goes back where each pad came from.
		return m.runRestore(msg.plan, -1)
	case deletePlanMsg:
		m.busy = ""
		if msg.err != nil {
			m.message = "delete: " + msg.err.Error()
			if msg.reopen != nil {
				return m.openAssign(*msg.reopen)
			}
			break
		}
		m.prompt = &prompt{kind: promptDeleteFiles, paths: msg.paths, count: msg.count, reopen: msg.reopen}
	case reopenAssignMsg:
		m.busy = ""
		m.message = msg.note
		return m.openAssign(msg.pad)
	case planMsg:
		m.busy = ""
		if msg.err != nil {
			m.message = "restore: " + msg.err.Error()
			break
		}
		m.prompt = &prompt{kind: promptConfirmRestore, plan: msg.plan}
	case factsMsg:
		m.factsBusy = false
		m.facts[msg.key] = padFacts{facts: msg.facts, err: msg.err}
		return m, m.wanted()
	case usageMsg:
		// An answer about a project that is no longer the one in view is dropped.
		if m.snap != nil && msg.project == m.snap.Project {
			m.usage, m.usageState = msg.usage, 2
			if msg.err != nil {
				m.usageState = 0
				m.usage = nil
			}
		} else {
			m.usageState = 0
		}
		return m, nil
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
	if msg.lost {
		return m.lose()
	}
	if msg.have {
		a := msg.activity
		m.playing, m.playPos = a.Playing, a.Position
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

// connected takes up a unit that was found.
func (m Model) connected(msg connectMsg) (tea.Model, tea.Cmd) {
	m.connecting = false
	switch {
	case msg.err != nil:
		m.waitErr = ""
		if !errors.Is(msg.err, ErrNoUnit) {
			m.waitErr = msg.err.Error()
			m.log.Warn("connecting failed", "err", msg.err)
		}
		return m, nil
	case m.unit != nil:
		// Another attempt got there first.
		if msg.closer != nil {
			msg.closer.Close()
		}
		return m, nil
	}
	m.log.Info("connected", "port", msg.unit.PortName())
	m.unit, m.closer = app.NewRouted(msg.unit), msg.closer
	m.waitErr, m.lost = "", false
	m.connectedFrame = m.frame
	m.busy = "reading the unit"
	return m, m.load(fmt.Sprintf("● Connected to the SP-404MKII on %s", msg.unit.PortName()))
}

// lose handles the link to the unit going away: everything on screen is dropped, and the UI
// goes back to waiting for it.
func (m Model) lose() (tea.Model, tea.Cmd) {
	m.log.Warn("the link to the unit is gone", "reconnects", m.connect != nil)
	if m.connect == nil {
		m.message = "the connection to the unit was lost"
		m.busy = ""
		return m, nil
	}
	if m.closer != nil {
		m.closer.Close()
	}
	m.stopListening()
	m.unit, m.closer, m.snap = nil, nil, nil
	m.lost, m.busy, m.polling, m.connecting = true, "", false, false
	m.prompt, m.picker, m.palette, m.browser, m.chromatic = nil, nil, nil, nil, nil
	m.message = ""
	return m, nil
}

// wanted starts whatever the section in front needs and is not in hand yet: the patterns of
// a bank, the file listing, a waveform.
func (m *Model) wanted() tea.Cmd {
	if m.busy != "" || m.snap == nil {
		return nil
	}
	if s := m.search; s != nil && !m.infoBusy {
		if f, ok := s.current(); ok {
			if _, have := m.infos[f.Path]; !have {
				m.infoBusy = true
				return m.fileInfoCmd(f.Path)
			}
		}
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
		if row := m.padRow(); row.HasSample && !m.factsBusy {
			key := factKey{row.Pad, row.Size, row.Name}
			if _, have := m.facts[key]; !have {
				m.factsBusy = true
				return m.padFactsCmd(key, row)
			}
		}
		if m.usageState == 0 {
			m.usageState = 1
			return m.usageCmd(m.snap.Project)
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

// padCache keeps the samples read from the unit to play a pad on this computer, so that the
// second press does not read them again. It is shared by every copy of the model and used
// from commands, hence the lock.
type padCache struct {
	mu      sync.Mutex
	samples map[factKey]formats.Sample
}

func (c *padCache) get(k factKey) (formats.Sample, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, ok := c.samples[k]
	return s, ok
}

func (c *padCache) put(k factKey, s formats.Sample) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.samples[k] = s
}

func (c *padCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.samples)
}
