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
	"github.com/moahbeatlab/404tui/midiin"
	"github.com/moahbeatlab/404tui/perflog"
	"github.com/moahbeatlab/404tui/proto"
	"github.com/moahbeatlab/404tui/proto/fileapi"
	"github.com/moahbeatlab/404tui/record"
)

// The sections, in the order of their number keys.
const (
	secPads = iota
	secPatterns
	secFiles
	secProjects
	secSlicer
	secSequencer
	secConfig
	secPerform
	secCount
)

var sectionNames = [secCount]string{"Pads", "Patterns", "Files", "Projects", "Slicer", "Sequencer", "Config", "Perform"}

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
	// promptEraseProject confirms erasing prompt.project.
	promptEraseProject
	// promptDeletePatterns confirms deleting the pattern slots in prompt.pads.
	promptDeletePatterns
	// promptBusScan confirms setting the buses of the pads in prompt.buses; promptBusWords asks
	// for the words of a bus.
	promptBusScan
	// promptRepairPads confirms setting the pads in prompt.pads, which were never set up, to
	// the settings a new pad has.
	promptRepairPads
	promptBusWords
	// promptRecDir asks for the folder recordings go into, promptRecDevice for a part of the name
	// of the input to record.
	promptRecDir
	promptRecDevice
	// promptLiveCode and promptRecCode ask for the pads that switch the live mode and a recording.
	promptLiveCode
	promptRecCode
	// promptMidi asks for a part of the MIDI setting that prompt.busOf numbers: 0 the input, 1 the
	// control channel, 2 to 5 the four messages.
	promptMidi
	// promptBusNote asks for a note on the effect of a bus (prompt.busOf).
	promptBusNote
	// promptDeleteFiles confirms deleting prompt.paths from the card; promptMkdir names a new
	// folder in prompt.path.
	promptDeleteFiles
	// promptTags asks which tags to add to prompt.paths.
	promptTags
	promptMkdir
	// promptFindFolder jumps to the next folder in the list whose name contains the text.
	promptFindFolder
	// promptBank edits a bank's tempo and volume: two fields, Tab moves between them.
	promptBank
	// promptQuit asks before leaving the app, as q is easy to press by accident.
	promptQuit
)

// prompt is a question in front of the section: a name or path to type, or a delete to
// confirm.
type prompt struct {
	kind   promptKind
	buffer string
	back   int // runes between the cursor and the end of buffer
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
	paths   []string
	folders []string // promptTags: folders to tag next to the files in paths
	count   app.Count
	reopen  *proto.PadIndex
	// fields and field are the bank dialog's tempo and volume, and the one being typed.
	fields [2]string
	field  int
	// choice is the folder picked from the matches while one is looked for (f).
	choice int
	// buses are the pads a bus scan would change, each with the bus it would get; scanAll is
	// set when it looks at the whole project, not the bank (or the marked pads) in front.
	buses   map[proto.PadIndex]int32
	scanAll bool
	// busOf is the bus whose words a promptBusWords is about.
	busOf int
}

// picker is the pad chooser a file is assigned through, or the bank chooser of a
// retarget.
type picker struct {
	retarget bool
	// Assign: the files to put on pads, in order.
	files []string
	// restorePlan: an export being put back; the picker chooses the bank it goes onto.
	restorePlan *app.PadRestore
	// withPattern: the restore also makes a pattern of the restored pads (p). slots is the
	// second step that asks which pattern slot, in bank ptnBank at position pad.
	withPattern bool
	slots       bool
	ptnBank     int
	// saveFrom: the slot chooser saves this open pattern, as it is heard, into the slot chosen.
	saveFrom *proto.PadIndex
	// copyFrom: the pads being copied; the picker chooses where the first one goes.
	copyFrom []proto.PadIndex
	// retarget: the pattern slot and the bank its pads are in.
	slot       proto.PadIndex
	sourceBank int
	bank       int
	pad        int
	// bounces are rendered samples, each under its name, that the picker puts on the pad chosen
	// (one) or on the free pads from there on (several).
	bounces     []formats.Sample
	bounceNames []string
	// bounceWhat names what the bounces are in messages: "takes" (the default) or "slices".
	bounceWhat string
	// note is a line under the title of the pad chooser, about what is put on the pad.
	note string
	// digits is the pad number being typed.
	digits string
	// confirm is set once the chosen pad holds a sample and the user has been told.
	confirm bool
}

// report is a titled list of lines.
type report struct {
	title string
	lines []string
}

type palette struct {
	filter string
	cursor int
	// items are the commands it lists when it is for the dialog in front (title says which);
	// without them it lists the commands of the section.
	items []command
	title string
}

// patToolKind is one of the groove tools that reshape the whole open pattern.
type patToolKind int

const (
	toolSwing patToolKind = iota
	toolQuantize
	toolShift
	toolReverse
	toolHumanize
	// toolDouble, toolExtend and toolHalve change the pattern's length: always the whole pattern.
	toolDouble
	toolExtend
	toolHalve
)

// patTool is the little menu over the open pattern: a tool is chosen with j/k, its amount set
// with h/l (or +/-), and Enter applies it. Reverse has no amount.
type patTool struct {
	kind patToolKind
	// pad, when set, scopes the tools to that one pad's track; nil applies to the whole pattern.
	pad *proto.PadIndex
	// swing is the swing percentage 50..75; quantStrength 0..100; shift the number of steps
	// (can be negative); human the timing/velocity amount 0..100. Each tool keeps its own.
	swing         int
	quantStrength int
	shift         int
	human         int
	// eighths swings/quantizes on eighth notes instead of sixteenths.
	eighths bool
}

// ErrNoUnit is what a Connect returns when no unit is plugged in yet; the UI keeps waiting
// quietly instead of reporting it.
var ErrNoUnit = errors.New("no unit")

// Connect opens a link to a unit. The UI calls it again and again until it succeeds, and
// again after the link is lost.
type Connect func() (app.Unit, io.Closer, error)

// OpenOffline opens a folder of projects on this computer as a unit, to work without one. The
// note says what was found there.
type OpenOffline func(dir string) (unit app.Unit, closer io.Closer, note string, err error)

// probeTicks is how many ticks pass between looks for a unit while working offline.
const probeTicks = connectTicks * 4

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
	// history is every message shown in the status line, with when; lastLogged is the last.
	history    []logEntry
	lastLogged string
	// busy names what the UI is waiting for; keys other than quit wait with it.
	busy string
	// prog is how far the operation named by busy has come, when it can tell.
	prog    *progress
	frame   int
	polling bool

	prompt  *prompt
	picker  *picker
	palette *palette
	// toggles is the menu of on/off settings that t offers for the whole bank.
	toggles *toggleMenu
	// tmplPicker is the "new pattern" template list (N); laneMap is the lane->pad confirm dialog
	// a genre template opens before it is written. Both are nil when closed.
	tmplPicker *templatePicker
	// lenPick is the question of how long a new pattern is, asked after its template.
	lenPick *lengthPick
	laneMap *laneAssign
	browser *browser
	search  *search
	editor  *editor
	// chromatic is the dialog that plays a pad as notes from the keyboard.
	chromatic *chromatic
	// slicer is the dialog that cuts a sample into slices and plays them in an order or at
	// random; slices keeps what it was set to for each sample.
	slicer *slicer
	// addAsk is the dialog that a asks the Slicer and Sequencer tabs open: a sample or a pattern.
	addAsk *addAsk
	// sliceWant is set while a sample is being chosen for a new slicer on the Pads or Files
	// screen (Z takes the one under the cursor).
	sliceWant bool
	// seqTab is set while a pattern that the Sequencer tab opened is in front: its tab is lit,
	// and leaving the pattern goes back to that tab.
	seqTab bool
	// recorder starts recordings of the unit's audio input; rec is the one running.
	recorder record.Backend
	rec      *record.Session
	vc       *viewCache  // the last drawing, kept over the clock ticks that change nothing visible
	lvl      *levelMeter // the level of what is heard: of the recording that runs, else of the input
	// meterErr is why the input could not be listened to; meterAgain is when to try again, meterBusy
	// that it is being started.
	meterErr   string
	meterAgain time.Time
	meterBusy  bool
	// perfNow and perfNext are the pattern that plays and the one that comes next, as the
	// Perform view shows them: what the user says, the unit does not tell.
	perfNow, perfNext *proto.PadIndex
	// midiSrc is where MIDI comes from, midi the input that is open (nil when none is), midiOpening
	// is set while one is being opened, and clock counts the MIDI clock.
	midiSrc     midiin.Source
	midi        *midiLink
	midiOpening bool
	midiErr     string
	midiPadAt   time.Time
	clock       *clockState
	// mon is what the MIDI input said lately; perfLog the event log of the recording that runs.
	mon     *monitor
	perfLog *perflog.Log
	// fx is what the unit last said of the effect of each bus: switched on or off (CC 19), and which
	// effect it is (CC 83).
	fx [2]busFx
	// codes holds the pads hit lately, for the codes that start the live mode and a recording;
	// liveOn is set while the live mode is on, liveBack the view it goes back to.
	codes    *codeState
	liveOn   bool
	liveBack int
	// perfPatterns shows the Perform view's patterns, big, in place of the pads of the bank.
	perfPatterns bool
	// perfPlayer shows the pattern that plays as the Sequencer draws it, with a playhead that
	// follows the MIDI clock; perfSync is the clock tick the loop began on.
	perfPlayer bool
	perfSync   int
	// perfAuto is set when the pattern that plays was found from the notes the unit sends.
	perfAuto bool
	// report is a list of lines shown in a dialog until a key closes it.
	report *report
	// cfgAt is the row of the Config view the cursor is on.
	cfgAt int
	// seqSlot is the pattern the Sequencer tab had open when it was left; it is read again when
	// the tab is back.
	seqSlot *proto.PadIndex
	slices  sliceStore
	// conds keeps the conditions of the patterns' notes (when a note plays over the loops);
	// patConds and patSeed are those of the open pattern, read when it was opened.
	conds    condStore
	patConds app.Conditions
	patSeed  int64
	// indexes are the file indexes searches use, one per source, kept until files change.
	indexes map[string]*app.Index
	// tags is the tag index kept at the root of the SD card, once it has been read; tagJob is
	// the indexing run in the background, if one has started.
	tags   *app.TagIndex
	tagJob *tagJob
	// searchJobs are the builds of the search indexes in the background, by source.
	searchJobs map[string]*tagJob

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
	// patRow and patCol are the cursor in the open pattern's step grid; patHist is the undo and
	// redo stacks of edits to it.
	patRow, patCol int
	// patSub is the part of a split step the cursor is on.
	patSub int
	// patSubMode is on while h l move between the parts of a step and x switches them.
	patSubMode bool
	patHist    *patternHistory
	// patFocus expands the track on the cursor row into a big step lane; the others stay
	// one-line strips. Esc collapses it again.
	patFocus bool
	// projectBack is set once the project last open on this source has been gone back to (or
	// there was none to go back to), after connecting; until then the project is not recorded.
	projectBack bool
	// loopTry is a file heard in place of a pad's sample in the pattern that loops, while the
	// assign dialog for that pad is open: nothing is written until it is assigned.
	loopTry *loopTry
	// patRange limits the open pattern to some of its bars: only those are shown, edited and
	// looped. nil is the whole pattern. rangePick is the list v opens to choose it.
	patRange *barRange
	// patSel is the notes of a track chosen to be changed together, while V has them open.
	patSel    *noteSelection
	rangePick *rangePicker
	// patTool is the groove-tools menu over the open pattern, nil when it is closed.
	patTool *patTool
	// loopSlot is the pattern that was last started looping; loopDirty asks for it to be mixed
	// again, because something it plays changed.
	// patPreview are the patterns read to show the one in front of the cursor in the patterns
	// panel, by slot; previewBusy is set while one is read.
	patPreview map[proto.PadIndex]*app.PatternDetail
	// patUnread are the patterns that could not be read for the preview.
	patUnread   map[proto.PadIndex]bool
	previewBusy bool
	loopSlot    *proto.PadIndex
	loopDirty   bool
	// full is full-screen mode: the header and the tabs are away, the body has the room.
	full bool
	// muted and soloed are pads silenced, or alone to sound, in the mix played on this computer.
	muted, soloed map[proto.PadIndex]bool
	// iso is what Perform has isolated; clickPadAt and clickAt find a double click.
	iso        *isolation
	sc         *sceneState
	clickPadAt proto.PadIndex
	clickAt    time.Time
	// loopStart is when the looping pattern started, and loopRound how long a round lasts.
	loopStart time.Time
	loopRound time.Duration
	// loopLoops is how many loops of the pattern the round has: the cycle of its conditions.
	loopLoops int
	// patMarks are pattern slots (by slot index) marked for deleting; patHeld is a pattern
	// picked up to be copied (or, with patCut, moved) into another slot; patConfirm is the
	// slot index p was pressed on once, where a pattern would be replaced.
	patMarks   map[int]bool
	patHeld    proto.PadIndex
	patHolding bool
	patCut     bool
	patConfirm int
	pattern    *app.PatternDetail

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
	// byNumber makes a multi-file assign put each file on the pad its name's number says
	// (09_… on pad 9) instead of on the free pads in order.
	byNumber bool
	autoSeq  int
	audioErr string

	projectAt int

	// facts are what the SMP file of each pad says (length, channels), read as the pad is
	// looked at; usage tells which pattern slots play each pad.
	facts      map[factKey]padFacts
	factsBusy  bool
	usage      map[proto.PadIndex][]proto.PadIndex
	usageState int // 0: not read, 1: reading, 2: in hand
	// stripBusy is set while the patterns of the bank are read in the background, for the strip
	// under the pads.
	stripBusy bool
	// origins say where the samples on the pads came from; originsBusy is set while they are
	// read from the card.
	origins *app.Origins
	// folderUses are the notes of which folders were restored onto which banks.
	folderUses     *app.FolderUses
	folderUsesBusy bool
	// ranger is the three-column view of the files; autoOpen are the folders opened only to be
	// previewed, previewBusy a preview being read and previewSkip the folders that could not be.
	ranger         bool
	autoOpen       map[string]bool
	dirPreviewBusy bool
	previewSkip    map[string]bool
	originsBusy    bool
	infos          map[string]fileInfo
	infoBusy       bool

	// lastAssigned is the pad a file last went onto; a file goes to the next free one.
	lastAssigned proto.PadIndex
	hasLast      bool
	// target is the bank an assigned file goes to; it follows the last pad assigned to, and
	// is kept apart from the bank in view on Pads.
	target int

	// connect finds a unit when there is none; closer closes the one in use.
	connect Connect
	closer  io.Closer
	// offline is the folder of projects on this computer worked on in place of a unit, ""
	// when connected to one. openOffline opens such a folder; probe says whether a unit is
	// plugged in, and unitBack that it was found while working offline.
	offline      string
	openOffline  OpenOffline
	probe        func() bool
	probing      bool
	unitBack     bool
	startOffline string
	// previewWas is where pads played before going offline, which plays them on this computer.
	previewWas     bool
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
		unit:        routed,
		player:      player,
		ops:         &sync.Mutex{},
		prog:        &progress{},
		st:          newStyles(themes[0]),
		width:       100,
		height:      30,
		busy:        "reading the unit",
		marked:      map[int]bool{},
		patterns:    map[int][]app.PatternRow{},
		patMarks:    map[int]bool{},
		patConfirm:  -1,
		searchJobs:  map[string]*tagJob{},
		patPreview:  map[proto.PadIndex]*app.PatternDetail{},
		muted:       map[proto.PadIndex]bool{},
		soloed:      map[proto.PadIndex]bool{},
		root:        app.CardRoot,
		fileMarks:   map[string]bool{},
		ranger:      true,
		autoOpen:    map[string]bool{},
		previewSkip: map[string]bool{},
		facts:       map[factKey]padFacts{},
		infos:       map[string]fileInfo{},
		indexes:     map[string]*app.Index{},
		listenGen:   new(atomic.Int64),
		padAudio:    &padCache{samples: map[factKey]formats.Sample{}},
		store:       store,
		slices:      newSliceStore(store),
		conds:       newCondStore(store),
		saved:       new([]byte),
		log:         slog.New(slog.DiscardHandler),
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

// WithOffline lets the UI work on a folder of projects when no unit is plugged in: o on the
// waiting screen chooses the folder, and start (when set) opens one at once. probe, when
// set, tells it a unit was plugged in meanwhile.
func (m Model) WithOffline(open OpenOffline, probe func() bool, start string) Model {
	m.openOffline, m.probe, m.startOffline = open, probe, start
	return m
}

// projectSource names where the projects come from, for remembering which one was open: the
// folder worked on offline, or the unit.
func (m Model) projectSource() string {
	if m.offline != "" {
		return m.offline
	}
	return "unit"
}

// rememberProject records the project open now as the one to come back to on this source. The
// first time after connecting to a folder offline, it goes back to the project that was open
// there last time instead (a folder opens on its first project; a unit keeps its own).
func (m *Model) rememberProject() tea.Cmd {
	src, now := m.projectSource(), int(m.snap.Project)
	if !m.projectBack {
		m.projectBack = true
		if last := m.mem.Projects[src] - 1; m.offline != "" && last >= 0 && last != now && last < len(m.snap.Projects) {
			unit, project := m.unit, uint8(last)
			m.busy = fmt.Sprintf("opening project %d again", last+1)
			return m.edit(func() tea.Msg {
				if err := unit.SelectProject(project); err != nil {
					return doneMsg{message: fmt.Sprintf("project %d, open last time: %v", project+1, err), reload: true}
				}
				return doneMsg{message: fmt.Sprintf("project %d, as last time", project+1), reload: true}
			})
		}
	}
	if m.mem.Projects == nil {
		m.mem.Projects = map[string]int{}
	}
	m.mem.Projects[src] = now + 1
	return nil
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
		// patterns marks that a pattern was written: what is known of them is read again.
		patterns bool
		// origins are the notes of where samples came from, after an assign or a delete.
		origins *app.Origins
		// folderUses are the notes of which folders are on which banks, after a restore or a scan.
		folderUses *app.FolderUses
	}
	patternsMsg struct {
		bank int
		rows []app.PatternRow
		err  error
		// quiet is set for patterns read for the strip under the pads, not waited for.
		quiet bool
	}
	patternMsg struct {
		detail *app.PatternDetail
		err    error
		note   string
		// undo is the pattern's bytes before an edit, when the message comes from one.
		undo []byte
		// op says how the edit touches the history: a normal edit pushes undo and clears redo;
		// an undo or redo moves bytes between the two stacks instead.
		op patEditOp
	}
	dirMsg struct {
		root    string
		path    string
		row     int
		entries []fileapi.DirEntry
		err     error
		// preview: opened to look into; into: the cursor goes in once it is open.
		preview, into bool
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
		// offline is the folder opened in place of a unit, and note what was found in it.
		offline, note string
	}
	// unitSeenMsg says whether a unit is plugged in, looked for while working offline.
	unitSeenMsg bool
	factsMsg    struct {
		key   factKey
		facts app.PadFacts
		err   error
	}
	// recStartedMsg and recEndedMsg: a recording began (or could not), or it is over.
	recStartedMsg struct {
		session *record.Session
		err     error
	}
	recEndedMsg struct {
		session   *record.Session
		path      string
		size      int64
		err       error
		onPurpose bool
	}
	// midiOpenedMsg is the result of opening the MIDI input; midiMsg is a message from it.
	midiOpenedMsg struct {
		link *midiLink
		err  error
	}
	midiMsg struct {
		link *midiLink
		ev   midiin.Event
	}
	// checkMsg is what comparing samples with their files found, a line or two for each.
	checkMsg struct{ lines []string }
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
		// round is how long one round of a looping pattern lasts; 0 for anything else.
		round time.Duration
		// loops is how many loops of the pattern the round has (its conditions' cycle).
		loops int
	}
	// autoplayMsg fires a moment after the cursor stopped on a result.
	autoplayMsg struct{ seq int }
	noteMsg     string
)

func tick() tea.Cmd {
	return tea.Tick(tickEvery, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m Model) Init() tea.Cmd {
	midi := m.startMIDI()
	if m.unit == nil && m.startOffline != "" && m.openOffline != nil {
		return tea.Batch(m.goOffline(m.startOffline), tick(), midi)
	}
	if m.unit == nil {
		return tea.Batch(m.tryConnect(), tick(), midi)
	}
	return tea.Batch(m.load(""), tick(), midi)
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

// viewCache holds the last drawing. The clock is 24 ticks to a quarter note, about fifty messages a
// second, and drawing the wide Perform view takes milliseconds: only every sixth tick (a sixteenth
// note, the finest thing the ticker and the playhead show) is drawn again.
type viewCache struct {
	out  string
	keep bool
	// hit is where the last drawing of the Perform view put its pads and its ISOLATE panel, for the
	// mouse (counted from the corner of the view's frame).
	hit perfHit
}

// perfHit is the place of the clickable things of the wide Perform view.
type perfHit struct {
	ok             bool
	gx, gy, tw, th int // the pad grid: its corner and the size of a tile
	sideW          int // the width of the sidebar, which starts at column 1
	panelY, panelH int // the ISOLATE panel in the sidebar
}

// ---- update ----

// Update handles a message, then keeps the state that is remembered between runs.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.vc == nil {
		m.vc = &viewCache{}
	}
	m.vc.keep = false // only a quiet clock tick (midiEvent) says the drawing can stay
	model, cmd := m.update(msg)
	if updated, ok := model.(Model); ok {
		updated = updated.logMessage()
		if updated.loopDirty {
			updated.loopDirty = false
			if again := updated.requeueLoop(); again != nil {
				cmd = tea.Batch(cmd, again)
			}
			// A pattern under the slicer's slices is mixed again for the same reasons.
			if again := updated.slicerRemix(); again != nil {
				cmd = tea.Batch(cmd, again)
			}
		}
		var listen tea.Cmd
		updated, listen = updated.syncMeter()
		if listen != nil {
			cmd = tea.Batch(cmd, listen)
		}
		var start tea.Cmd
		updated, start = updated.autoIndex(nil)
		updated.persist()
		if start != nil {
			return updated, tea.Batch(cmd, start)
		}
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
	case midiOpenedMsg:
		return m.midiOpened(msg)
	case midiMsg:
		return m.midiEvent(msg)
	case tickMsg:
		m.frame++
		m.sampleLevel()
		if m.midiSrc != nil && m.midi == nil && !m.midiOpening && m.frame%50 == 25 {
			// The unit's MIDI input appears when it is plugged in: it is looked for now and then.
			m.midiOpening = true
			return m, tea.Batch(tick(), m.startMIDI())
		}
		if m.unit == nil {
			if m.connect != nil && !m.connecting && m.frame%connectTicks == 0 {
				m.connecting = true
				return m, tea.Batch(tick(), m.tryConnect())
			}
			return m, tick()
		}
		if m.offline != "" && m.probe != nil && !m.unitBack && !m.probing && m.frame%probeTicks == 0 {
			m.probing = true
			probe := m.probe
			return m, tea.Batch(tick(), func() tea.Msg { return unitSeenMsg(probe()) })
		}
		if m.frame%m.pollEvery() == 0 && m.busy == "" && !m.polling {
			m.polling = true
			return m, tea.Batch(tick(), m.poll())
		}
		return m, tick()
	case pollMsg:
		return m.polled(msg)
	case connectMsg:
		return m.connected(msg)
	case unitSeenMsg:
		m.probing = false
		if bool(msg) && m.offline != "" && !m.unitBack {
			m.unitBack = true
			m.message = "An SP-404MKII is plugged in: U leaves offline mode and connects to it (your offline work is saved)"
		}
		return m, nil
	case indexMsg:
		return m.indexed(msg)
	case loadedMsg:
		m.busy = ""
		// The pads were read again: what a looping pattern plays may have changed. A file being
		// tried in the loop has been assigned (or given up) by now: the pad's own sound plays.
		m.loopDirty = true
		m.loopTry = nil
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
			m.patPreview = map[proto.PadIndex]*app.PatternDetail{}
			m.pattern, m.patFocus, m.patRange = nil, false, nil
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
		if cmd := m.rememberProject(); cmd != nil {
			return m, cmd
		}
		return m, m.wanted()
	case doneMsg:
		m.message = msg.message
		if msg.origins != nil {
			m.origins = msg.origins
		}
		if msg.folderUses != nil {
			m.folderUses = msg.folderUses
		}
		if msg.focus != nil {
			if m.slicer == nil || m.section != secSlicer {
				// A slicer in front stays in front: its bounce lands on the pad without a change of view.
				m.section = secPads
			}
			m.bank, m.pad = int(msg.focus.Bank()), int(msg.focus.Pad())
			if msg.assigned {
				m.lastAssigned, m.hasLast = *msg.focus, true
				m.target = int(msg.focus.Bank())
			}
		}
		if msg.patterns {
			m.patterns = map[int][]app.PatternRow{}
			m.patPreview = map[proto.PadIndex]*app.PatternDetail{}
			m.usage, m.usageState = nil, 0
		}
		if msg.reload {
			m.busy = "reading the unit"
			return m, m.load(msg.message)
		}
		m.busy = ""
	case noteMsg:
		m.message = string(msg)
		// A pad setting changed (the level, say): a looping pattern is mixed again.
		m.loopDirty = true
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
	case slicerLoadedMsg:
		return m.slicerLoaded(msg)
	case slicerTickMsg:
		return m.slicerTicked(msg)
	case slicerBedMsg:
		return m.slicerBedMixed(msg)
	case patBounceMsg:
		return m.patternBounced(msg)
	case folderUsesMsg:
		m.folderUsesBusy = false
		if msg.err == nil && m.folderUses == nil {
			m.folderUses = msg.uses
		}
		return m, m.wanted()
	case originsMsg:
		m.originsBusy = false
		if msg.err == nil && m.origins == nil {
			m.origins = msg.origins
		}
		return m, m.wanted()
	case patPreviewMsg:
		m.previewBusy = false
		if msg.err == nil && msg.detail != nil {
			m.patPreview[msg.detail.Slot] = msg.detail
			delete(m.patUnread, msg.detail.Slot)
		} else {
			// Not asked for again, or a pattern that cannot be read would be asked for for ever.
			if m.patUnread == nil {
				m.patUnread = map[proto.PadIndex]bool{}
			}
			m.patUnread[msg.slot] = true
		}
		return m, m.wanted()
	case loopQueuedMsg:
		if msg.err != nil {
			m.message = "the change is not in the loop: " + msg.err.Error()
		} else if msg.round > 0 {
			// The cycle may have changed length with the change.
			m.loopRound, m.loopLoops = msg.round, max(msg.loops, 1)
		}
		return m, nil
	case tagsSavedMsg:
		return m.tagsWritten(msg)
	case askTagsMsg:
		return m.tagsRead(msg)
	case userTagsMsg:
		return m.userTagged(msg)
	case tagsLoadedMsg:
		return m.tagsLoaded(msg)
	case tagIndexMsg:
		return m.tagIndexed(msg)
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
			if msg.round > 0 {
				// Where the loop is, from when it started: the line under the steps follows it.
				m.loopStart, m.loopRound, m.loopLoops = time.Now(), msg.round, max(msg.loops, 1)
			}
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
		if msg.quiet {
			m.stripBusy = false
			if msg.err == nil {
				m.patterns[msg.bank] = msg.rows
			}
			return m, m.wanted()
		}
		m.busy = ""
		if msg.err != nil {
			m.message = "reading patterns failed: " + msg.err.Error()
			break
		}
		m.patterns[msg.bank] = msg.rows
		if p := m.picker; p != nil && p.slots && p.ptnBank == msg.bank {
			p.pad = firstFreeSlot(msg.rows)
		}
	case patternMsg:
		m.busy = ""
		if msg.err != nil {
			m.message = msg.err.Error()
			break
		}
		m.pattern = msg.detail
		if m.pattern != nil {
			m.patConds, m.patSeed = m.patternConditions(m.pattern.Slot)
			// Every pad of the bank with a sample has a row, notes or not.
			m.pattern.WithRowsFor(m.patternBankPads(m.pattern))
			if msg.undo != nil {
				if m.patHist == nil || m.patHist.slot != m.pattern.Slot {
					m.patHist = &patternHistory{slot: m.pattern.Slot}
				}
				// msg.undo is the state this edit replaced on disk. A normal edit records it on
				// the undo stack and drops any redo future; an undo/redo moves it to the other
				// stack (the key handler already popped the snapshot it applied).
				switch msg.op {
				case editUndo:
					m.patHist.push(&m.patHist.redo, msg.undo)
				case editRedo:
					m.patHist.push(&m.patHist.undo, msg.undo)
				default:
					m.patHist.push(&m.patHist.undo, msg.undo)
					m.patHist.redo = nil
				}
				m.loopDirty = true
			}
			// What is shown of it beside the patterns is this, now.
			m.patPreview[m.pattern.Slot] = m.pattern
			m.patRow = clamp(m.patRow, 0, max(len(m.pattern.Grid.Rows)-1, 0))
			m.patCol = clamp(m.patCol, 0, max(m.pattern.Grid.Steps-1, 0))
			// A range of another pattern, or past the end of this one (halved), is let go.
			if r := m.patRange; r != nil && (r.slot != m.pattern.Slot || r.last >= m.pattern.Bars()) {
				m.patRange = nil
			}
		}
		if msg.note != "" {
			m.message = msg.note
			// A retarget changes which pads the patterns play.
			m.usage, m.usageState = nil, 0
		}
	case dirMsg:
		if msg.preview {
			m.dirPreviewBusy = false
		} else {
			m.busy = ""
		}
		switch {
		case msg.err != nil:
			m.previewSkip[msg.path] = true
			m.message = fmt.Sprintf("listing %s failed: %v", display(msg.path), msg.err)
		// The answer to a question about a volume the tree has since left.
		case msg.root != m.root:
		case msg.row < 0:
			m.tree = app.NewTree(msg.path, msg.entries)
			m.fileAt = 0
			return m, m.wanted()
		case m.tree != nil && msg.row < len(m.tree.Rows()) && m.tree.Rows()[msg.row].Path == msg.path:
			cursor := ""
			if rows := m.tree.Rows(); m.fileAt < len(rows) {
				cursor = rows[m.fileAt].Path
			}
			m.tree.Expand(msg.row, msg.entries)
			if msg.preview {
				m.autoOpen[msg.path] = true
			}
			// What opened above the cursor moved it down.
			if i := m.tree.Index(cursor); cursor != "" && i >= 0 {
				m.fileAt = i
			}
			if msg.into {
				if kids := m.tree.Children(msg.row); len(kids) > 0 {
					m.fileAt = kids[0]
				} else {
					m.message = msg.path[strings.LastIndexByte(msg.path, '/')+1:] + " is empty"
				}
			}
			return m, m.wanted()
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
		return m.runRestore(msg.plan, -1, nil)
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
	case meterStartedMsg:
		return m.meterStarted(msg)
	case meterEndedMsg:
		return m.meterEnded(msg), nil
	case recStartedMsg:
		if msg.err != nil {
			m.message = "recording did not start: " + msg.err.Error()
			return m, nil
		}
		m.rec = msg.session
		if m.lvl != nil && m.lvl.src != nil {
			m.lvl.src.Stop() // the recording reads the input now
		}
		m.lvl = newLevelMeter()
		m.log.Info("recording started", "path", msg.session.Path, "tool", msg.session.Tool)
		m.message = "recording to " + display(msg.session.Path) + " (" + msg.session.Tool + "): Ctrl-R stops it"
		if m.midi != nil {
			// What the unit sends goes into a log beside the audio.
			l, note := m.startPerfLog(msg.session.Path, msg.session.Started)
			m.perfLog = l
			if note != "" {
				m.log.Warn("event log", "err", note)
				m.message = note
			} else {
				m.log.Info("event log started", "path", l.Path)
				m.message += " · events to " + display(l.Path)
			}
		}
		return m, waitRecording(msg.session)
	case recEndedMsg:
		m.log.Info("recording ended", "path", msg.path, "bytes", msg.size, "err", msg.err)
		if m.rec == msg.session {
			m.rec, m.lvl = nil, nil
			m.closePerfLog()
			m.perfLog = nil
		}
		switch {
		case msg.err != nil && !msg.onPurpose:
			m.message = "the recording stopped: " + msg.err.Error()
		case msg.size == 0:
			m.message = "the recording is empty: " + display(msg.path)
		default:
			m.message = fmt.Sprintf("recorded %s to %s", humanBytes(msg.size), display(msg.path))
		}
		return m, nil
	case checkMsg:
		m.busy = ""
		m.report = &report{title: "Sample check", lines: msg.lines}
		return m, nil
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
	if msg.lost {
		return m.lose()
	}
	var codeCmd tea.Cmd
	if msg.have {
		a := msg.activity
		if m.padHit(m.hasSelected, a) && time.Since(m.midiPadAt) > 10*time.Second {
			m, codeCmd = m.codeHit(int(a.Selected.Pad()) + 1)
		}
		if a.Playing != m.playing || a.Playing && a.Selected != m.playingPad {
			m.perfLog.Add(time.Now(), "sound", perflog.P("on", a.Playing), perflog.P("bank", string(rune('A'+a.Selected.Bank()))), perflog.P("pad", int(a.Selected.Pad())+1))
		}
		if a.HasSelected && (!m.hasSelected || a.Selected != m.selected) {
			m.perfLog.Add(time.Now(), "select", perflog.P("bank", string(rune('A'+a.Selected.Bank()))), perflog.P("pad", int(a.Selected.Pad())+1))
		}
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
		// Something was changed on the unit's own panel: what was read of its patterns is old too.
		m.patterns = map[int][]app.PatternRow{}
		m.patPreview = map[proto.PadIndex]*app.PatternDetail{}
		m.busy = "reading the unit"
		cmds := []tea.Cmd{codeCmd, m.load("")}
		if m.pattern != nil && m.patSel == nil && !m.patSubMode {
			cmds = append(cmds, m.openPattern(m.pattern.Slot, "changed on the unit"))
		}
		return m, tea.Batch(cmds...)
	}
	return m, tea.Batch(codeCmd, m.wanted())
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
	m.log.Info("connected", "port", msg.unit.PortName(), "offline", msg.offline)
	m.unit, m.closer = app.NewRouted(msg.unit), msg.closer
	m.projectBack = false
	m.waitErr, m.lost = "", false
	m.connectedFrame = m.frame
	m.busy = "reading the unit"
	if msg.offline != "" {
		// The pads have no unit to sound on: they play on this computer.
		m.offline, m.unitBack = msg.offline, false
		m.mem.OfflineDir = msg.offline
		m.previewWas, m.previewLocal = m.previewLocal, true
		return m, m.load(fmt.Sprintf("◌ Working offline on %s: %s", display(msg.offline), msg.note))
	}
	return m, m.load(fmt.Sprintf("● Connected to the SP-404MKII on %s", msg.unit.PortName()))
}

// goOffline opens a folder of projects on this computer in place of a unit.
func (m Model) goOffline(dir string) tea.Cmd {
	open := m.openOffline
	return func() tea.Msg {
		unit, closer, note, err := open(dir)
		if err != nil {
			return connectMsg{err: fmt.Errorf("working offline on %s: %w", dir, err)}
		}
		return connectMsg{unit: unit, closer: closer, offline: dir, note: note}
	}
}

// leaveOffline saves and closes the folder worked on offline, and connects to the unit.
func (m Model) leaveOffline() (tea.Model, tea.Cmd) {
	m.log.Info("leaving offline mode", "folder", m.offline)
	if m.closer != nil {
		m.closer.Close()
	}
	m.stopListening()
	m.unit, m.closer, m.snap = nil, nil, nil
	m.offline, m.unitBack, m.previewLocal = "", false, m.previewWas
	m.busy, m.polling, m.connecting = "", false, true
	m.prompt, m.picker, m.palette, m.browser, m.chromatic, m.toggles, m.slicer = nil, nil, nil, nil, nil, nil, nil
	m.pattern, m.patterns, m.patPreview = nil, map[int][]app.PatternRow{}, map[proto.PadIndex]*app.PatternDetail{}
	m.tree, m.infos = nil, map[string]fileInfo{}
	clear(m.indexes)
	m.message = "left offline mode; connecting to the unit"
	return m, m.tryConnect()
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
	m.prompt, m.picker, m.palette, m.browser, m.chromatic, m.toggles, m.slicer = nil, nil, nil, nil, nil, nil, nil
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
	if b := m.browser; b != nil && b.kind == browseAssign {
		// A folder in the dialog says where its samples were used: the notes are read for that.
		if m.folderUses == nil && !m.folderUsesBusy {
			m.folderUsesBusy = true
			return m.loadFolderUses()
		}
		if m.origins == nil && !m.originsBusy {
			m.originsBusy = true
			return m.loadOrigins()
		}
	}
	if b := m.browser; b != nil && b.kind == browseAssign && b.tree != nil && b.at < len(b.tree.Rows()) && !m.infoBusy {
		node := b.tree.Rows()[b.at]
		if _, have := m.infos[node.Path]; !node.Dir && isAudio(node.Name) && !have {
			m.infoBusy = true
			return m.fileInfoCmd(node.Path)
		}
	}
	if m.section == secPerform && m.snap != nil {
		// The bank the unit is on: the lengths of its samples, and its patterns, are read for the tiles.
		bank := m.perfBank()
		if !m.factsBusy {
			for _, row := range m.snap.Bank(bank) {
				if !row.HasSample {
					continue
				}
				key := factKey{row.Pad, row.Size, row.Name}
				if _, have := m.facts[key]; !have {
					m.factsBusy = true
					return m.padFactsCmd(key, row)
				}
			}
		}
		if m.patterns[bank] == nil && !m.stripBusy {
			m.stripBusy = true
			return m.loadPatternsQuietly(bank)
		}
	}
	if m.section == secPerform && !m.previewBusy {
		// The patterns the Perform view shows are read to know which pads play in them.
		for _, slot := range []*proto.PadIndex{m.perfNow, m.perfNext} {
			if slot != nil && m.patPreview[*slot] == nil && !m.patUnread[*slot] {
				m.previewBusy = true
				return m.loadPreview(*slot)
			}
		}
	}
	if a := m.addAsk; a != nil && a.patterns != nil && !m.previewBusy {
		// The pattern the Sequencer's chooser is on is read to be shown under the slots.
		if r := a.patterns.rows[a.patterns.cursor]; r.Exists && m.patPreview[r.Slot] == nil && !m.patUnread[r.Slot] {
			m.previewBusy = true
			return m.loadPreview(r.Slot)
		}
	}
	if s := m.slicer; s != nil && !m.previewBusy {
		// The pattern shown under the slicer.
		if slot, ok := s.previewSlot(); ok && m.patPreview[slot] == nil && !m.patUnread[slot] {
			m.previewBusy = true
			return m.loadPreview(slot)
		}
	}
	switch m.section {
	case secPatterns:
		if m.patterns[m.bank] == nil {
			m.busy = "reading patterns"
			return m.loadPatterns(m.bank)
		}
		// The pattern in front is read to be shown in the panel beside, quietly.
		if rows := m.patterns[m.bank]; m.pattern == nil && !m.previewBusy && m.patternCursor < len(rows) {
			if cur := rows[m.patternCursor]; cur.Exists && m.patPreview[cur.Slot] == nil {
				m.previewBusy = true
				return m.loadPreview(cur.Slot)
			}
		}
	case secFiles:
		if m.tree == nil {
			m.busy = "listing files"
			return m.loadFilesTree()
		}
		if cmd := m.rangerPreview(); cmd != nil {
			return cmd
		}
		if m.folderUses == nil && !m.folderUsesBusy {
			m.folderUsesBusy = true
			return m.loadFolderUses()
		}
		// The pads' origins give the folders their marker and say which pads came from them.
		if m.origins == nil && !m.originsBusy {
			m.originsBusy = true
			return m.loadOrigins()
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
		// The other pads of the bank are read one after another: the tiles say how many bars
		// each sample is.
		if !m.factsBusy {
			for _, row := range m.snap.Bank(m.bank) {
				if !row.HasSample {
					continue
				}
				key := factKey{row.Pad, row.Size, row.Name}
				if _, have := m.facts[key]; !have {
					m.factsBusy = true
					return m.padFactsCmd(key, row)
				}
			}
		}
		if m.usageState == 0 {
			m.usageState = 1
			return m.usageCmd(m.snap.Project)
		}
		if m.origins == nil && !m.originsBusy {
			m.originsBusy = true
			return m.loadOrigins()
		}
		// The strip of patterns under the pads: read quietly, the pads stay in use.
		if m.patterns[m.bank] == nil && !m.stripBusy {
			m.stripBusy = true
			return m.loadPatternsQuietly(m.bank)
		}
	}
	return nil
}

// loadPatternsQuietly reads the patterns of a bank without holding the screen: the answer is
// not about something the user waits for.
func (m Model) loadPatternsQuietly(bank int) tea.Cmd {
	return func() tea.Msg {
		m.ops.Lock()
		defer m.ops.Unlock()
		rows, err := app.LoadPatterns(m.unit, bank)
		return patternsMsg{bank: bank, rows: rows, err: err, quiet: true}
	}
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

// logEntry is a line of the history panel.
type logEntry struct {
	at   time.Time
	text string
}

// maxHistory is how many lines of history are kept.
const maxHistory = 500

// logMessage adds the status line to the history when it has changed.
func (m Model) logMessage() Model {
	if m.message == "" || m.message == m.lastLogged {
		if m.message == "" {
			m.lastLogged = ""
		}
		return m
	}
	m.lastLogged = m.message
	m.history = append(m.history[max(len(m.history)+1-maxHistory, 0):], logEntry{at: time.Now(), text: m.message})
	return m
}

// originsMsg is the notes of where samples came from, read from the card.
type originsMsg struct {
	origins *app.Origins
	err     error
}

// folderUsesMsg is the notes of which folders were put on which banks, read from the card.
type folderUsesMsg struct {
	uses *app.FolderUses
	err  error
}

// loadFolderUses reads the notes from the card, without holding the screen.
func (m Model) loadFolderUses() tea.Cmd {
	return func() tea.Msg {
		m.ops.Lock()
		defer m.ops.Unlock()
		f, err := app.LoadFolderUses(m.unit)
		return folderUsesMsg{uses: f, err: err}
	}
}

// loadOrigins reads the notes from the card, without holding the screen.
func (m Model) loadOrigins() tea.Cmd {
	return func() tea.Msg {
		m.ops.Lock()
		defer m.ops.Unlock()
		o, err := app.LoadOrigins(m.unit)
		return originsMsg{origins: o, err: err}
	}
}

// barRange is some bars of a pattern, first to last (0-based, both in), of the pattern in slot.
type barRange struct {
	slot        proto.PadIndex
	first, last int
}

// bars is how many bars the range holds.
func (r barRange) bars() int { return r.last - r.first + 1 }

// label names the range as the header and the list do: "Bar 3" or "Bars 3–4".
func (r barRange) label() string {
	if r.first == r.last {
		return fmt.Sprintf("Bar %d", r.first+1)
	}
	return fmt.Sprintf("Bars %d–%d", r.first+1, r.last+1)
}

// rangePicker is the list of ranges v offers: the whole pattern, each bar, each two bars.
type rangePicker struct {
	cursor int
	items  []barRange // first -1: the whole pattern
}

// loopTry is an audio file standing in for a pad's sample in the looping pattern.
type loopTry struct {
	pad  proto.PadIndex
	path string
}

// patternHistory is the undo and redo stacks for the open pattern: each entry is the pattern's
// bytes at one point. undo grows as edits are made (newest last), redo grows as edits are
// undone. Both are capped at patHistoryMax.
type patternHistory struct {
	slot proto.PadIndex
	undo [][]byte
	redo [][]byte
}

// patHistoryMax is how many edits back the open pattern can be undone.
const patHistoryMax = 10

// patEditOp says how a finished pattern edit touches the history.
type patEditOp int

const (
	// editNormal is an ordinary edit: push the before-bytes onto undo and drop the redo stack.
	editNormal patEditOp = iota
	// editUndo and editRedo are the result of stepping through the history; they do not change
	// the stacks (the key handler already moved the bytes between them).
	editUndo
	editRedo
)

// push appends a snapshot to a stack, keeping at most patHistoryMax (dropping the oldest).
func (h *patternHistory) push(stack *[][]byte, snap []byte) {
	*stack = append(*stack, snap)
	if len(*stack) > patHistoryMax {
		*stack = (*stack)[len(*stack)-patHistoryMax:]
	}
}

// patternBankPads are the pads with a sample in the bank a pattern plays (or, when its pads
// span banks or it has none, the bank of its slot): the rows its grid offers.
// patPage is which bar (0-based) of the open pattern the cursor is on; the pattern view shows
// one bar per page.
func (m Model) patPage() int {
	if m.pattern == nil {
		return 0
	}
	return m.patCol / m.pattern.StepsPerPage()
}

func (m Model) patternBankPads(d *app.PatternDetail) []proto.PadIndex {
	if m.snap == nil {
		return nil
	}
	bank := int(d.Slot.Bank())
	if d.HasSourceBank {
		bank = d.SourceBank
	}
	var pads []proto.PadIndex
	for _, row := range m.snap.Bank(bank) {
		if row.HasSample {
			pads = append(pads, row.Pad)
		}
	}
	return pads
}

// patPreviewMsg is a pattern read for the panel beside the patterns.
type patPreviewMsg struct {
	slot   proto.PadIndex
	detail *app.PatternDetail
	err    error
}

// loadPreview reads a pattern to show it beside the patterns, without holding the screen.
func (m Model) loadPreview(slot proto.PadIndex) tea.Cmd {
	return func() tea.Msg {
		m.ops.Lock()
		defer m.ops.Unlock()
		d, err := app.OpenPattern(m.unit, slot)
		return patPreviewMsg{slot: slot, detail: d, err: err}
	}
}
