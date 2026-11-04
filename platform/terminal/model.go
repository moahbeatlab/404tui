package terminal

import (
	"fmt"
	"github.com/moahbeatlab/404tui/dbg"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"

	"github.com/moahbeatlab/404tui/core"
)

// Options say what to run.
type Options struct {
	Samples, Out string // the folder the file browser starts in, the folder the card is written to
	Project      string // a project folder to open (with a PADCONF.BIN)
	Version      string
	// Platform is what this system offers the app: sound, a recorder, MIDI, a way to find the unit.
	Platform core.Platform
	// Profile and Glyphs override what the terminal is found to support.
	Profile *Profile
	Glyphs  *Glyphs
}

// Run shows the app in the terminal until it is quit.
func Run(o Options) error {
	core.Terminal = true
	core.Version = o.Version
	app := core.New(o.Samples, o.Out, nil)
	app.SetPlatform(o.Platform)
	if o.Project != "" {
		if err := app.OpenProject(o.Project); err != nil {
			return err
		}
	}
	app.ShowSplash(2500 * time.Millisecond)
	m := NewModel(app, o)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if o.Platform.Audio != nil {
		go func() { // opening the output can take long, or block: the screen and the keys come first
			if au, err := o.Platform.Audio(); err == nil {
				app.AudioLate <- au
			}
		}()
	}
	_, err := p.Run()
	app.FlushAutosave()
	return err
}

// DetectProfile is the colour depth of this terminal.
func DetectProfile() Profile {
	if c := strings.ToLower(os.Getenv("COLORTERM")); c == "truecolor" || c == "24bit" || os.Getenv("WT_SESSION") != "" {
		return TrueColor
	}
	if termenv.ColorProfile() == termenv.TrueColor {
		return TrueColor
	}
	return ANSI256
}

// DetectGlyphs is the set of characters to draw with: all of them, except in an old Windows console.
func DetectGlyphs() Glyphs {
	switch strings.ToLower(os.Getenv("SP404_GLYPHS")) {
	case "basic":
		return BasicGlyphs
	case "full":
		return FullGlyphs
	}
	if runtime.GOOS == "windows" && os.Getenv("WT_SESSION") == "" && os.Getenv("TERM_PROGRAM") == "" {
		return BasicGlyphs
	}
	return FullGlyphs
}

// shift is a modifier armed from the keyboard for the next key.
type shift int

const (
	noShift shift = iota
	yShift
	selShift
)

// Model is the bubbletea model: the app, the size of the window and what the keyboard has armed.
type Model struct {
	app        *core.App
	cols, rows int
	profile    Profile
	glyphs     Glyphs

	armed   shift // Y or Select was pressed and waits for the key it goes with
	armedAt time.Time
	goTo    bool // g was pressed: h or l now goes to the screen before or after, as gT and gt do in vim
	aHeld   bool // A is held by e until e again
	last    string
	lastAt  time.Time
	rep     int
}

type tickMsg time.Time

// NewModel makes the model for an app.
func NewModel(app *core.App, o Options) *Model {
	m := &Model{app: app, profile: DetectProfile(), glyphs: DetectGlyphs(), cols: core.MinCols, rows: core.MinRows}
	if o.Profile != nil {
		m.profile = *o.Profile
	}
	if o.Glyphs != nil {
		m.glyphs = *o.Glyphs
	}
	return m
}

func (m *Model) Init() tea.Cmd { return tick() }

func tick() tea.Cmd {
	return tea.Tick(40*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) tooSmall() bool { return m.cols < core.MinCols || m.rows < core.MinRows }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	defer func() { // a crash leaves its stack in the debug log (and goes on to the screen)
		if r := recover(); r != nil {
			dbg.Logger.Error("panic", "value", r, "stack", string(debug.Stack()))
			panic(r)
		}
	}()
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.cols, m.rows = msg.Width, msg.Height
		if !m.tooSmall() {
			core.SetSize(m.cols, m.rows)
		}
	case tickMsg:
		select {
		case au := <-m.app.AudioLate:
			m.app.Audio = au
		default:
		}
		m.app.Tick()
		if m.armed != noShift && time.Since(m.armedAt) > 4*time.Second {
			m.disarm()
		}
		if m.app.Quit {
			return m, tea.Quit
		}
		return m, tick()
	case tea.KeyMsg:
		m.key(msg)
		if m.app.Quit {
			return m, tea.Quit
		}
	}
	return m, nil
}

// disarm lets go of a shift nobody used.
func (m *Model) disarm() {
	m.armed = noShift
	m.app.CancelShift()
	m.aHeld = false
}

func (m *Model) send(k core.Key, rep int) {
	m.app.Dispatch(core.Event{Key: k, Down: true, Repeat: rep})
	m.app.Dispatch(core.Event{Key: k, Down: false})
}

// key plays a key of the keyboard as the buttons: the arrows or h j k l (as in vim) are the D-pad; a b x (and Enter, Esc,
// Space) are the buttons; y and s arm a shift for the next key; e holds A; u and Ctrl-R undo and redo, / searches, q
// quits.
func (m *Model) key(msg tea.KeyMsg) {
	s := msg.String()
	if m.tooSmall() {
		if s == "ctrl+c" {
			m.app.Quit = true
		}
		return
	}
	if m.app.Typing() { // a search or a prompt is open: the keys type into it
		switch {
		case s == "ctrl+c":
			m.app.RequestQuit()
		case s == "enter":
			m.app.TypeEnter()
		case s == "esc":
			m.app.TypeEscape()
		case s == "backspace":
			m.app.TypeBackspace()
		case s == "tab": // the search: folders or files
			m.app.TypeTab()
		case msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace:
			m.app.TypeText(string(msg.Runes))
		}
		return
	}
	if len(s) == 1 && m.app.SplashText(s) { // the splash: u online, o offline
		return
	}
	if m.goTo {
		m.goTo = false
		switch s {
		case "h", "left", "H", "T", "shift+tab":
			m.app.NextScreen(-1)
			return
		case "l", "right", "L", "t", "tab":
			m.app.NextScreen(1)
			return
		case "g": // gg: the first screen, as in vim
			m.app.GoScreen(0)
			return
		case "esc":
			return
		}
	}
	var k core.Key
	switch s {
	case "g":
		m.disarm()
		m.goTo = true
		return
	case "t": // the tools of the screen
		if m.armed == noShift && !m.aHeld {
			m.app.ToolsMenu()
		}
		m.disarm()
		return
	case "n": // the slicer: a new order of the slices, at once
		if m.app.ScreenName() == "SLICE" && m.armed == noShift && !m.app.Modal() {
			m.disarm()
			m.app.NewOrder()
			return
		}
		return
	case "+", "=": // the slicer: zoom in
		m.disarm()
		m.app.Zoom(1)
		return
	case "-", "_": // the slicer: zoom out
		m.disarm()
		m.app.Zoom(-1)
		return
	case "f": // SEQ: freeze, only what is in view plays
		m.disarm()
		m.app.ToggleFreeze()
		return
	case "z": // SEQ: one bar or as many as fit
		m.disarm()
		m.app.ToggleBars()
		return
	case "ctrl+c":
		m.disarm()
		m.app.RequestQuit()
		return
	case "q": // as in vim: quit (asks when the project is not saved)
		m.disarm()
		m.app.RequestQuit()
		return
	case "/": // as in vim: search, in the file browser (Y+A)
		m.disarm()
		if m.app.OnFiles() {
			m.app.Dispatch(core.Event{Key: core.Y, Down: true})
			m.send(core.A, 0)
			m.app.Dispatch(core.Event{Key: core.Y, Down: false})
		}
		return
	case "ctrl+e": // PADS: export the bank
		m.disarm()
		m.app.ExportBank()
		return
	case "ctrl+r": // record the unit's audio, as everywhere in the program
		m.disarm()
		m.app.ToggleRecording()
		return
	case "ctrl+z", "ctrl+y", "u", "R": // undo and redo: Y+L2, Y+R2
		m.disarm()
		m.app.Dispatch(core.Event{Key: core.Y, Down: true})
		l := core.L2
		if s == "ctrl+y" || s == "R" {
			l = core.R2
		}
		m.send(l, 0)
		m.app.Dispatch(core.Event{Key: core.Y, Down: false})
		return
	case "?":
		m.disarm()
		m.app.ToggleHelp()
		return
	case "y", "Y":
		m.arm(yShift, core.Y)
		return
	case "m", "M": // SEQ: mute the track; FILES and PADS: mark the file or the pad, M marks them all (and unmarks them all again)
		if m.armed == noShift && !m.aHeld && !m.app.Modal() {
			switch {
			case m.app.ScreenName() == "SEQ" && s == "m":
				m.app.SeqTrack(1)
			case m.app.ScreenName() == "FILES" && s == "m":
				m.app.MarkFile()
			case m.app.ScreenName() == "FILES":
				m.app.MarkAll()
			case m.app.ScreenName() == "PADS" && s == "m":
				m.app.MarkPad()
			case m.app.ScreenName() == "PADS":
				m.app.MarkAllPads()
			}
		}
		return
	case "s", "S":
		if m.app.ScreenName() == "SEQ" && m.armed == noShift && !m.aHeld && !m.app.Modal() { // SEQ: s is solo
			m.app.SeqTrack(2)
			return
		}
		m.arm(selShift, core.Sel)
		return
	case "1", "2", "3", "4", "5", "6", "7", "8", "9": // the screens, straight to
		m.disarm()
		m.app.GoScreen(int(s[0] - '1'))
		return
	case "tab", "shift+tab": // the next screen, or the one before
		m.disarm()
		if s == "tab" {
			m.app.NextScreen(1)
		} else {
			m.app.NextScreen(-1)
		}
		return
	case "e", "E":
		if m.aHeld {
			m.aHeld = false
			m.app.Dispatch(core.Event{Key: core.A, Down: false})
		} else {
			m.aHeld = true
			m.app.Dispatch(core.Event{Key: core.A, Down: true})
		}
		return
	case "up", "k", "K":
		k = core.Up
	case "down", "j", "J":
		k = core.Down
	case "left", "h", "H":
		k = core.Left
	case "right", "l", "L":
		k = core.Right
	case " ": // play and stop
		if m.app.PlayStop() {
			m.disarm()
			return
		}
		k = core.X
	case "enter": // the slicer's menu; a button A elsewhere
		if m.app.OpenMenu() {
			m.disarm()
			return
		}
		k = core.A
	case "a", "A":
		k = core.A
	case "b", "B": // the button B; it does not close a menu or a page: that is Esc
		if m.app.Modal() {
			return
		}
		k = core.B
	case "esc", "backspace":
		k = core.B
	case "x", "X":
		k = core.X
		if m.armed == noShift && !m.aHeld && !m.app.Modal() && (m.app.ScreenName() == "PADS" || m.app.ScreenName() == "FILES") { // Space plays, x stops
			m.app.StopSound()
			return
		}
		if m.armed == noShift && !m.aHeld && m.app.ScreenName() == "SEQ" && !m.app.Modal() { // SEQ: x hears the note, Space plays the pattern
			k = core.L2
		}
	case "[", "]": // SEQ: the bank (the pattern at the same place); elsewhere L1 and R1
		if m.app.ScreenName() == "SEQ" && m.armed == noShift && !m.aHeld && !m.app.Modal() {
			m.app.SeqBank(map[string]int{"[": -1, "]": 1}[s])
			return
		}
		k = map[string]core.Key{"[": core.L1, "]": core.R1}[s]
	case "pgup", "ctrl+u":
		k = core.L1
	case "pgdown", "ctrl+d":
		k = core.R1
	case "{", "}": // SEQ: the bar before and after (L1 and R1 there); elsewhere the triggers L2, R2
		k = map[string]core.Key{"{": core.L2, "}": core.R2}[s]
		if m.app.ScreenName() == "SEQ" && m.armed == noShift && !m.aHeld && !m.app.Modal() {
			k = map[string]core.Key{"{": core.L1, "}": core.R1}[s]
		}
	case ",", "<":
		k = core.L2
	case ".", ">":
		k = core.R2
	default:
		return
	}
	if m.aHeld && (k == core.A || k == core.B) { // a or b with A held: let go
		m.aHeld = false
		m.app.Dispatch(core.Event{Key: core.A, Down: false})
		return
	}
	rep := 0 // a direction key or a shoulder held down arrives as the same key again and again (every 30 ms or so): that is
	// the repeat the app speeds up by. Typing the key twice is slower than that, and any other key never repeats
	if s == m.last && time.Since(m.lastAt) < 70*time.Millisecond && (k <= core.Right || k >= core.L1 && k <= core.R2) {
		m.rep++
		rep = m.rep
	} else {
		m.rep = 0
	}
	m.last, m.lastAt = s, time.Now()
	m.send(k, rep)
	if m.armed != noShift {
		m.armed = noShift
		m.app.CancelShift()
	}
}

// arm presses Y or Select and keeps it down for the next key; the same key again lets go (Y alone is the help page).
func (m *Model) arm(which shift, key core.Key) {
	if m.armed == which {
		m.armed = noShift
		m.app.Dispatch(core.Event{Key: key, Down: false})
		return
	}
	if m.armed != noShift {
		m.app.CancelShift()
	}
	m.armed, m.armedAt = which, time.Now()
	m.app.Dispatch(core.Event{Key: key, Down: true})
}

func (m *Model) View() string {
	if m.tooSmall() {
		return fmt.Sprintf("404tui needs a window of at least %dx%d characters; this one is %dx%d.\nMake it bigger (or the font smaller). Ctrl-C quits.", core.MinCols, core.MinRows, m.cols, m.rows)
	}
	st := time.Now()
	cells := m.app.RenderCells()
	out := Render(cells, m.profile, m.glyphs)
	if d := time.Since(st); d > 80*time.Millisecond && dbg.Enabled() {
		dbg.Logger.Warn("slow frame", "ms", d.Milliseconds(), "screen", m.app.ScreenName(), "cols", m.cols, "rows", m.rows)
	}
	return out
}
