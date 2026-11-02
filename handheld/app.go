// Package handheld is a front end for 404tui's file engine (package lab) that needs only a D-pad and
// the four buttons on the right of a handheld console (an R36S under ArkOS): Up Down Left Right
// and A B X Y. Nothing else: no shoulder buttons, no Start or Select, no sticks, no keyboard.
//
// The way M8 does it, Y is the shift key. Held, it turns the D-pad into the way between screens
// (Left and Right) and between banks or zoom levels (Up and Down), and A, B and X into their
// second functions. Tapped alone, Y shows the key map of the screen.
//
// It draws 640x480 frames in software (PNG for tests and screenshots, a framebuffer or a web page
// on a device), so it runs wherever Go does, without SDL.
package handheld

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/lab"
)

// Key is one of the eight keys, and the two shoulder buttons for paging.
type Key int

const (
	Up Key = iota
	Down
	Left
	Right
	A
	B
	X
	Y
	L1 // the shoulder buttons: page up and page down
	R1
	L2 // the triggers: play one slice, play the whole order (in the slicer)
	R2
)

func (k Key) String() string {
	if k >= L1 {
		return []string{"L1", "R1", "L2", "R2"}[k-L1]
	}
	return "UDLRABXY"[k : k+1]
}

// Screen is one of the screens, side by side: Y with Left and Right moves between them.
type Screen int

const (
	Pads Screen = iota
	Files
	Slice
	Seq
	Save
	Unit
)

var screenNames = []string{"PADS", "FILES", "SLICE", "SEQ", "SAVE", "UNIT"}

// Audio plays a sample for the ear; a nil Audio is silent.
type Audio interface {
	Play(formats.Sample) error
	Stop()
}

type entry struct {
	name    string
	dir     bool
	project bool // a folder with a PADCONF.BIN: a project of the SP-404MKII
	size    int64
}

// App is the state of the front end.
type App struct {
	lab    *lab.Lab
	Root   string // where the samples are: the file browser cannot go above it
	OutDir string // where projects are written
	Audio  Audio
	// AudioLate delivers an audio output that opened after the start (opening it can take long or block); Run takes it.
	AudioLate chan Audio

	// Quit is set when the app should end. quitAsk: an unsaved project was asked about, the next quit really ends.
	Quit    bool
	quitAsk bool
	saveAsk bool   // X on SAVE asked once
	saved   bool   // the project was written and not changed since
	loadAsk string // the project folder A was pressed on once, which would replace the work

	scr  Screen
	bank int
	cur  int // pad in the bank, 0..15
	info lab.Info
	pcm  map[int]int // pad -> handle of its decoded sample, for the waveform

	dir     string
	entries []entry
	fcur    int
	prev    map[string]int // file path -> handle of the decoded file, for the preview

	sid        int // handle of the sample being cut
	spad       int // its pad
	sl         *formats.Slicing
	at         int // the cursor, in frames
	sliceSteps int // the length of the slicer's result in steps, 0 for one round (the slicer menu)
	zoom       int // 0 shows the whole sample, each step doubles
	sel        int // slice under the cursor
	playing    int // pad lit by X, -1 for none

	menu     *menu
	picker   *picker // the pattern picker, when it is open
	carry    *carry  // a pad being moved or copied
	pat      int     // pattern in the sequencer, 1..160
	step     int     // the cursor's step, 16 to a bar
	col      int     // the cursor's column in the tracker, 0..7
	cols     [8]int  // the pad of each column
	colsSet  bool    // the columns were chosen (by default or by the user)
	colsOwn  bool    // the user changed a column: they stay when the pattern changes
	seqOn    bool    // the pattern is playing
	seqStart time.Time
	// Now is the clock; nil is the real one. A test sets it.
	Now func() time.Time

	link    Link
	Connect func() (Link, error) // finds and opens the unit; nil when there is no way to
	unitCur int
	confirm bool
	job     *job
	connAt  time.Time

	slot  int // PROJECT_nn to write
	wrote []string

	yHeld, yUsed, help bool
	msg                string
	msgBad             bool
}

// New starts an app on a folder of samples, with an empty project.
func New(root, outDir string, a Audio) *App {
	app := &App{pat: 1, lab: lab.New(), Root: root, OutDir: outDir, Audio: a, AudioLate: make(chan Audio, 1), pcm: map[int]int{}, prev: map[string]int{}, playing: -1, slot: 1}
	app.lab.NewProject("PROJECT_01")
	app.dir = root
	app.refresh()
	app.list()
	return app
}

// OpenProject loads a project folder (with a PADCONF.BIN in it) into the app.
func (a *App) OpenProject(dir string) error {
	a.lab.Begin()
	err := filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		a.lab.Add(rel, b)
		return nil
	})
	if err != nil {
		return err
	}
	if _, err := a.lab.Finish(); err != nil {
		return err
	}
	a.pcm = map[int]int{}
	a.refresh()
	return nil
}

func (a *App) refresh() {
	a.saved = false
	b, err := a.lab.Info()
	if err == nil {
		a.info = lab.Info{}
		json.Unmarshal(b, &a.info)
	}
}

func (a *App) list() {
	a.entries = nil
	des, err := os.ReadDir(a.dir)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	for _, de := range des {
		n := de.Name()
		if strings.HasPrefix(n, ".") {
			continue
		}
		if de.IsDir() {
			_, perr := os.Stat(filepath.Join(a.dir, n, "PADCONF.BIN"))
			a.entries = append(a.entries, entry{name: n, dir: true, project: perr == nil})
			continue
		}
		switch strings.ToLower(filepath.Ext(n)) {
		case ".wav", ".aif", ".aiff", ".flac", ".mp3", ".smp":
			fi, _ := de.Info()
			a.entries = append(a.entries, entry{name: n, size: fi.Size()})
		}
	}
	sort.SliceStable(a.entries, func(i, j int) bool {
		if a.entries[i].dir != a.entries[j].dir {
			return a.entries[i].dir
		}
		return strings.ToLower(a.entries[i].name) < strings.ToLower(a.entries[j].name)
	})
	a.fcur = 0
}

func (a *App) padN() int { return a.bank*16 + a.cur }

func (a *App) say(s string, bad bool) { a.msg, a.msgBad = s, bad }

// Screen is the screen in front.
func (a *App) Screen() Screen { return a.scr }

// Message is the last status line.
func (a *App) Message() string { return a.msg }

// Info is the project as the lab sees it.
func (a *App) Info() lab.Info { return a.info }

// Slices is how many slices the slicer holds, 0 when no sample is open.
func (a *App) Slices() int {
	if a.sl == nil {
		return 0
	}
	return len(a.sl.Slices)
}

// triggerKey is L2 (one slice) and R2 (the whole order): what the slicer plays, from any place on its screen.
func (a *App) triggerKey(whole bool) {
	if a.scr != Slice || a.sl == nil {
		return
	}
	if whole {
		a.playResult(false)
	} else {
		a.playSlice()
	}
}

// pageKey is L1 (back) and R1 (forward): a page in a list, a bank, a bar, the zoom, ten slots.
func (a *App) pageKey(fwd bool) {
	d := -1
	if fwd {
		d = 1
	}
	switch a.scr {
	case Pads:
		a.bank = (a.bank + 10 + d) % 10
	case Files:
		a.fcur = max(0, min(a.fcur+d*20, len(a.entries)-1))
	case Slice:
		if a.sl != nil {
			a.zoom = max(0, min(a.zoom+d, 8))
		}
	case Seq:
		a.step = max(0, min(a.step+d*16, a.steps()-1))
	case Save:
		a.slot = max(1, min(a.slot+d*10, 99))
		a.wrote = nil
	case Unit:
		a.unitCur = max(0, min(a.unitCur+d*8, 15))
	}
}

// RequestQuit is Select+Start. A project with samples that was not written is asked about once: the
// same request again quits, any other key cancels.
func (a *App) RequestQuit() {
	if !a.saved && !a.quitAsk && a.hasWork() {
		a.quitAsk = true
		a.say("not saved! Select+Start again to quit, any key to stay", true)
		return
	}
	a.Quit = true
}

func (a *App) hasWork() bool {
	for _, p := range a.info.Pads {
		if p.File {
			return true
		}
	}
	return false
}

// Press handles one key going down. repeat counts the repeats of a held key (0 for the first press), which
// make a cursor move faster.
func (a *App) Press(k Key, repeat int) {
	a.msg = ""
	a.quitAsk = false
	if k != X {
		a.saveAsk = false
	}
	if k == Y {
		if repeat == 0 {
			a.yHeld, a.yUsed = true, false
			if a.help { // Y with the help open closes it, and the keys that follow are chords as usual
				a.help = false
				a.yUsed = true
			}
		}
		return
	}
	if a.help {
		a.help = false
		return
	}
	if a.picker != nil {
		if k != Y {
			a.pickerKey(k)
		}
		return
	}
	if a.menu != nil {
		a.menuKey(k)
		return
	}
	if k == L1 || k == R1 {
		a.pageKey(k == R1)
		return
	}
	if k == L2 || k == R2 {
		a.triggerKey(k == R2)
		return
	}
	if a.yHeld {
		a.yUsed = true
		a.shifted(k)
		return
	}
	switch a.scr {
	case Pads:
		a.padsKey(k)
	case Files:
		a.filesKey(k)
	case Slice:
		a.sliceKey(k, repeat)
	case Seq:
		a.seqKey(k, repeat)
	case Save:
		a.saveKey(k)
	case Unit:
		a.unitKey(k)
	}
}

// Release handles a key going up. Y let go with nothing pressed while it was held shows the help.
func (a *App) Release(k Key) {
	if k != Y {
		return
	}
	if a.yHeld && !a.yUsed {
		a.help = !a.help
	}
	a.yHeld = false
}

// shifted is a key with Y held.
func (a *App) shifted(k Key) {
	switch k {
	case Left:
		a.go_(Screen((int(a.scr) + len(screenNames) - 1) % len(screenNames)))
		return
	case Right:
		a.go_(Screen((int(a.scr) + 1) % len(screenNames)))
		return
	}
	switch a.scr {
	case Files:
		if k == X {
			a.nextPlace()
		}
	case Seq:
		a.seqShifted(k)
	case Pads:
		switch k {
		case Up:
			a.bank = (a.bank + 9) % 10
		case Down:
			a.bank = (a.bank + 1) % 10
		case B:
			if err := a.lab.ClearPad(a.padN()); err != nil {
				a.say(err.Error(), true)
				return
			}
			delete(a.pcm, a.padN())
			a.refresh()
			a.say(fmt.Sprintf("cleared %s", a.label(a.padN())), false)
		}
	case Slice:
		switch k {
		case B:
			a.sliceMenu()
		case Up:
			a.zoom = min(a.zoom+1, 8)
		case Down:
			a.zoom = max(a.zoom-1, 0)
		case A:
			a.autoSlice()
		case X:
			a.slicesToPads(false)
		}
	}
}

func (a *App) go_(s Screen) {
	a.scr = s
	if s == Slice && (a.sl == nil || a.spad != a.padN()) {
		a.openSlicer()
		if a.sl == nil {
			a.scr = Pads
		}
	}
	if s == Seq {
		a.refresh()
		a.autoCols()
	}
	if s == Files && a.entries == nil {
		a.list()
	}
	if s == Save {
		a.wrote = nil
	}
}

func (a *App) label(n int) string { return fmt.Sprintf("%c%d", 'A'+rune(n/16), n%16+1) }

func (a *App) padsKey(k Key) {
	if a.carry != nil {
		a.carryKey(k)
		return
	}
	switch k {
	case Up:
		a.cur = (a.cur + 12) % 16
	case Down:
		a.cur = (a.cur + 4) % 16
	case Left:
		a.cur = a.cur&^3 | (a.cur+3)&3
	case Right:
		a.cur = a.cur&^3 | (a.cur+1)&3
	case A:
		a.openPadMenu()
	case X:
		a.playPad(a.padN())
	case B:
		a.stop()
	}
}

func (a *App) stop() {
	if a.Audio != nil {
		a.Audio.Stop()
	}
	a.playing = -1
}

func (a *App) play(s formats.Sample) {
	if a.Audio == nil {
		a.say("no audio output", true)
		return
	}
	if err := a.Audio.Play(s); err != nil {
		a.say(err.Error(), true)
	}
}

// padSample is the decoded sample of a pad, kept by its handle.
func (a *App) padSample(n int) (int, bool) {
	if id, ok := a.pcm[n]; ok {
		return id, true
	}
	if n >= len(a.info.Pads) || !a.info.Pads[n].File {
		return 0, false
	}
	b, err := a.lab.PadSample(n)
	if err != nil {
		return 0, false
	}
	var m lab.Meta
	json.Unmarshal(b, &m)
	a.pcm[n] = m.ID
	return m.ID, true
}

func (a *App) playPad(n int) {
	id, ok := a.padSample(n)
	if !ok {
		a.say("nothing on "+a.label(n), true)
		return
	}
	s, err := a.lab.Sample(id)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	a.playing = n
	a.play(s)
}

func (a *App) filesKey(k Key) {
	if k != A {
		a.loadAsk = ""
	}
	switch k {
	case Up:
		a.fcur = max(a.fcur-1, 0)
	case Down:
		a.fcur = min(a.fcur+1, max(len(a.entries)-1, 0))
	case Left:
		a.up()
	case B:
		a.stop()
		a.go_(Pads)
	case Right:
		a.openEntry()
	case X:
		if e, ok := a.current(); ok && !e.dir {
			if id, err := a.decode(e); err == nil {
				s, _ := a.lab.Sample(id)
				a.play(s)
			} else {
				a.say(err.Error(), true)
			}
		}
	case A:
		e, ok := a.current()
		if !ok {
			return
		}
		if e.project {
			a.loadProject(e)
			return
		}
		if e.dir {
			a.openEntry()
			return
		}
		id, err := a.decode(e)
		if err != nil {
			a.say(err.Error(), true)
			return
		}
		name := strings.TrimSuffix(e.name, filepath.Ext(e.name))
		n := a.padN()
		if err := a.lab.Assign(n, id, name); err != nil {
			a.say(err.Error(), true)
			return
		}
		delete(a.pcm, n)
		a.refresh()
		a.stop()
		a.say(fmt.Sprintf("%s on %s", fit(name, 20), a.label(n)), false)
		a.sl = nil
		a.go_(Pads)
	}
}

// loadProject replaces the project in memory with a project folder. With unsaved work it asks first: A again.
func (a *App) loadProject(e entry) {
	dir := filepath.Join(a.dir, e.name)
	if a.hasWork() && !a.saved && a.loadAsk != dir {
		a.loadAsk = dir
		a.say("replaces the project you are working on (not saved): A again", true)
		return
	}
	a.loadAsk = ""
	a.stop()
	if err := a.OpenProject(dir); err != nil {
		a.say("cannot load: "+err.Error(), true)
		return
	}
	var n int
	if _, err := fmt.Sscanf(strings.ToUpper(e.name), "PROJECT_%d", &n); err == nil && n >= 1 && n <= 99 {
		a.slot = n
	}
	a.sl, a.pat, a.step = nil, 1, 0
	a.bank, a.cur = 0, 0
	a.saved = true // what is on screen is what is on the card
	files := 0
	for _, p := range a.info.Pads {
		if p.File {
			files++
		}
	}
	a.say(fmt.Sprintf("loaded %s: %d pads with samples", e.name, files), false)
	a.go_(Pads)
}

// up goes to the parent folder, with the cursor on the folder it came from. Anywhere on the card is reachable.
func (a *App) up() {
	if a.dir == "/" || a.dir == filepath.Dir(a.dir) {
		a.say("this is the top", false)
		return
	}
	from := filepath.Base(a.dir)
	a.dir = filepath.Dir(a.dir)
	a.list()
	for i, e := range a.entries {
		if e.dir && e.name == from {
			a.fcur = i
		}
	}
}

// places are where Y+X in FILES jumps to: the samples folder, the usb drive, the games card, the top.
func (a *App) places() []string {
	return []string{a.Root, "/mnt/usbdrive", "/roms", "/"}
}

func (a *App) nextPlace() {
	ps := a.places()
	next := ps[0]
	for i, p := range ps {
		if p == a.dir {
			next = ps[(i+1)%len(ps)]
		}
	}
	for tries := 0; tries < len(ps); tries++ {
		if st, err := os.Stat(next); err == nil && st.IsDir() {
			break
		}
		for i, p := range ps {
			if p == next {
				next = ps[(i+1)%len(ps)]
				break
			}
		}
	}
	a.dir = next
	a.list()
	a.say("in "+next, false)
}

func (a *App) current() (entry, bool) {
	if a.fcur < 0 || a.fcur >= len(a.entries) {
		return entry{}, false
	}
	return a.entries[a.fcur], true
}

func (a *App) openEntry() {
	if e, ok := a.current(); ok && e.dir {
		a.dir = filepath.Join(a.dir, e.name)
		a.list()
	}
}

// decode reads a file of the browser into the lab, once.
func (a *App) decode(e entry) (int, error) {
	p := filepath.Join(a.dir, e.name)
	if id, ok := a.prev[p]; ok {
		return id, nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return 0, err
	}
	mb, err := a.lab.Decode(b, filepath.Ext(e.name))
	if err != nil {
		return 0, err
	}
	var m lab.Meta
	json.Unmarshal(mb, &m)
	a.prev[p] = m.ID
	return m.ID, nil
}

// Save writes the project to OutDir/PROJECT_nn.
func (a *App) saveKey(k Key) {
	switch k {
	case Left:
		a.slot = max(a.slot-1, 1)
		a.wrote = nil
	case Right:
		a.slot = min(a.slot+1, 99)
		a.wrote = nil
	case B:
		a.go_(Pads)
	case A:
		a.write()
	case X: // quit: asks once when there is unsaved work
		if a.saved || !a.hasWork() || a.saveAsk {
			a.Quit = true
			return
		}
		a.saveAsk = true
		a.say("not saved! X again to quit, any other key to stay", true)
	}
}

// cardFolder is where projects are on the SP-404MKII's card, under its root.
const cardFolder = "ROLAND/SP-404MKII"

// ProjectName is the folder the project is written to.
func (a *App) ProjectName() string { return fmt.Sprintf("PROJECT_%02d", a.slot) }

func (a *App) write() {
	names, err := a.exportTo(filepath.Join(a.OutDir, cardFolder))
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	a.wrote = names
	a.saved = true
	a.say(fmt.Sprintf("wrote %d files", len(names)), false)
}
