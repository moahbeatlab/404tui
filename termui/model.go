package termui

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"

	"github.com/moahbeatlab/404tui/handheld"
)

// Options say what to run.
type Options struct {
	Samples, Out string // the folder the file browser starts in, the folder the card is written to
	Project      string // a project folder to open (with a PADCONF.BIN)
	Version      string
	// Audio opens the sound output; it is called on the side, and its result is used when it comes. nil for no sound.
	Audio func() (handheld.Audio, error)
	// Connect finds a plugged-in unit for the UNIT screen.
	Connect func() (handheld.Link, error)
	// Profile and Glyphs override what the terminal is found to support.
	Profile *Profile
	Glyphs  *Glyphs
}

// Run shows the app in the terminal until it is quit.
func Run(o Options) error {
	handheld.Terminal = true
	handheld.Version = o.Version
	app := handheld.New(o.Samples, o.Out, nil)
	app.Connect = o.Connect
	if o.Project != "" {
		if err := app.OpenProject(o.Project); err != nil {
			return err
		}
	}
	app.ShowSplash(2500 * time.Millisecond)
	m := NewModel(app, o)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if o.Audio != nil {
		go func() { // opening the output can take long, or block: the screen and the keys come first
			if au, err := o.Audio(); err == nil {
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
	app        *handheld.App
	cols, rows int
	profile    Profile
	glyphs     Glyphs

	armed   shift // Y or Select was pressed and waits for the key it goes with
	armedAt time.Time
	aHeld   bool // A is held by e until e again
	last    string
	lastAt  time.Time
	rep     int
}

type tickMsg time.Time

// NewModel makes the model for an app.
func NewModel(app *handheld.App, o Options) *Model {
	m := &Model{app: app, profile: DetectProfile(), glyphs: DetectGlyphs(), cols: handheld.MinCols, rows: handheld.MinRows}
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
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) tooSmall() bool { return m.cols < handheld.MinCols || m.rows < handheld.MinRows }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.cols, m.rows = msg.Width, msg.Height
		if !m.tooSmall() {
			handheld.SetSize(m.cols, m.rows)
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

func (m *Model) send(k handheld.Key, rep int) {
	m.app.Dispatch(handheld.Event{Key: k, Down: true, Repeat: rep})
	m.app.Dispatch(handheld.Event{Key: k, Down: false})
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
	if m.app.Typing() { // the search keyboard is open: the keys type into it
		switch {
		case s == "ctrl+c":
			m.app.RequestQuit()
		case s == "enter" || s == "esc":
			m.send(handheld.X, 0)
		case s == "backspace":
			m.send(handheld.B, 0)
		case msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace:
			m.app.TypeSearch(string(msg.Runes))
		case s == "up":
			m.send(handheld.Up, 0)
		case s == "down":
			m.send(handheld.Down, 0)
		}
		return
	}
	var k handheld.Key
	switch s {
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
			m.app.Dispatch(handheld.Event{Key: handheld.Y, Down: true})
			m.send(handheld.A, 0)
			m.app.Dispatch(handheld.Event{Key: handheld.Y, Down: false})
		}
		return
	case "ctrl+z", "ctrl+y", "u", "ctrl+r": // undo and redo: Y+L2, Y+R2
		m.disarm()
		m.app.Dispatch(handheld.Event{Key: handheld.Y, Down: true})
		l := handheld.L2
		if s == "ctrl+y" || s == "ctrl+r" {
			l = handheld.R2
		}
		m.send(l, 0)
		m.app.Dispatch(handheld.Event{Key: handheld.Y, Down: false})
		return
	case "?":
		m.disarm()
		m.app.ToggleHelp()
		return
	case "y", "Y":
		m.arm(yShift, handheld.Y)
		return
	case "s", "S", "tab":
		m.arm(selShift, handheld.Sel)
		return
	case "e", "E":
		if m.aHeld {
			m.aHeld = false
			m.app.Dispatch(handheld.Event{Key: handheld.A, Down: false})
		} else {
			m.aHeld = true
			m.app.Dispatch(handheld.Event{Key: handheld.A, Down: true})
		}
		return
	case "up", "k", "K":
		k = handheld.Up
	case "down", "j", "J":
		k = handheld.Down
	case "left", "h", "H":
		k = handheld.Left
	case "right", "l", "L":
		k = handheld.Right
	case " ":
		k = handheld.X
	case "a", "A", "enter":
		k = handheld.A
	case "b", "B", "esc", "backspace":
		k = handheld.B
	case "x", "X":
		k = handheld.X
	case "[", "pgup", "ctrl+u":
		k = handheld.L1
	case "]", "pgdown", "ctrl+d":
		k = handheld.R1
	case ",", "{", "<":
		k = handheld.L2
	case ".", "}", ">":
		k = handheld.R2
	default:
		return
	}
	if m.aHeld && (k == handheld.A || k == handheld.B) { // a or b with A held: let go
		m.aHeld = false
		m.app.Dispatch(handheld.Event{Key: handheld.A, Down: false})
		return
	}
	rep := 0 // a direction key or a shoulder held down arrives as the same key again and again (every 30 ms or so): that is
	// the repeat the app speeds up by. Typing the key twice is slower than that, and any other key never repeats
	if s == m.last && time.Since(m.lastAt) < 70*time.Millisecond && (k <= handheld.Right || k >= handheld.L1 && k <= handheld.R2) {
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
func (m *Model) arm(which shift, key handheld.Key) {
	if m.armed == which {
		m.armed = noShift
		m.app.Dispatch(handheld.Event{Key: key, Down: false})
		return
	}
	if m.armed != noShift {
		m.app.CancelShift()
	}
	m.armed, m.armedAt = which, time.Now()
	m.app.Dispatch(handheld.Event{Key: key, Down: true})
}

func (m *Model) View() string {
	if m.tooSmall() {
		return fmt.Sprintf("404tui needs a window of at least %dx%d characters; this one is %dx%d.\nMake it bigger (or the font smaller). Ctrl-C quits.", handheld.MinCols, handheld.MinRows, m.cols, m.rows)
	}
	return Render(m.app.RenderCells(), m.profile, m.glyphs)
}
