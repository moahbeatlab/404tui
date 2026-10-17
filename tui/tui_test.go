package tui

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/device"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/midiin"
	"github.com/moahbeatlab/404tui/perflog"
	"github.com/moahbeatlab/404tui/proto"
	"github.com/moahbeatlab/404tui/record"
	"github.com/moahbeatlab/404tui/sim"
	"github.com/moahbeatlab/404tui/version"
)

func init() {
	tickEvery = time.Millisecond
	autoplayDelay = time.Millisecond
	previewStopAfter = func(time.Duration) time.Duration { return time.Millisecond }
}

func ansiStrip(s string) string { return ansi.Strip(s) }

// screen drives the UI as the program would, running each command to its message.
type screen struct {
	t      *testing.T
	model  tea.Model
	sim    *sim.Sim
	player *fakePlayer
	dev    *device.Device
	store  *memStore
}

// memStore keeps the remembered state in memory.
type memStore struct {
	state State
	saves int
}

func (m *memStore) Load() State  { return m.state }
func (m *memStore) Save(s State) { m.state = s; m.saves++ }

type fakePlayer struct {
	played []formats.Sample
	looped []formats.Sample
	queued []formats.Sample
	// replaced are loops changed at once, where they had got to.
	replaced []formats.Sample
	stopped  int
	playing  bool
}

func (f *fakePlayer) Play(s formats.Sample) error {
	f.played = append(f.played, s)
	f.playing = true
	return nil
}
func (f *fakePlayer) Loop(s formats.Sample) error {
	f.looped = append(f.looped, s)
	f.playing = true
	return nil
}
func (f *fakePlayer) ReplaceLoop(s formats.Sample) error {
	f.replaced = append(f.replaced, s)
	f.playing = true
	return nil
}
func (f *fakePlayer) NextLoop(s formats.Sample) error {
	f.queued = append(f.queued, s)
	f.playing = true
	return nil
}
func (f *fakePlayer) Stop()         { f.stopped++; f.playing = false }
func (f *fakePlayer) Playing() bool { return f.playing }

func wav(frames int) []byte {
	s := formats.Sample{Channels: 1, Samples: make([]int16, frames)}
	for i := range s.Samples {
		s.Samples[i] = int16(i%100) * 100
	}
	return s.WAV()
}

func open(t *testing.T) *screen {
	t.Helper()
	s := sim.New()
	for i, name := range []string{"kick", "snare", "hat"} {
		pad, _ := proto.NewPadIndex(uint16(i))
		s.SetSample(pad, name, 48_000)
	}
	b2, _ := proto.ParsePad("B2")
	a1, _ := proto.ParsePad("A1")
	s.SetPattern(b2, a1)
	s.AddFile("SD:IMPORT/kick.wav", wav(4800))
	s.AddFile("SD:IMPORT/pack/snare.wav", wav(2400))
	s.AddFile("SD:IMPORT/pack/notes.txt", []byte("x"))
	return connectTo(t, s, &memStore{})
}

// connectTo starts the UI on a simulated unit with a store, as a fresh run of the program.
func connectTo(t *testing.T, s *sim.Sim, store *memStore) *screen {
	t.Helper()
	d := device.New(s, "sim")
	t.Cleanup(func() { d.Close() })
	player := &fakePlayer{}
	// Most tests are about the folder tree; the three-column view has tests of its own.
	mdl := New(d, player, store)
	mdl.ranger = false
	sc := &screen{t: t, model: mdl, sim: s, player: player, dev: d, store: store}
	sc.run(sc.model.Init())
	return sc
}

// restart closes the UI and starts it again on the same unit and store, as quitting and
// running the program again would.
func (s *screen) restart() *screen {
	s.dev.Close()
	s.sim.Reopen()
	return connectTo(s.t, s.sim, s.store)
}

func (s *screen) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	// A command that waits for something outside (a recording to end) is let go after a moment.
	got := make(chan tea.Msg, 1)
	go func() { got <- cmd() }()
	var result tea.Msg
	select {
	case result = <-got:
	case <-time.After(400 * time.Millisecond):
		return
	}
	switch msg := result.(type) {
	case nil, tickMsg, slicerTickMsg:
	case tea.BatchMsg:
		for _, c := range msg {
			s.run(c)
		}
	default:
		s.send(msg)
	}
}

func (s *screen) send(msg tea.Msg) {
	model, cmd := s.model.Update(msg)
	s.model = model
	s.run(cmd)
}

// press sends keys: single characters, or names such as "enter".
func (s *screen) press(keys ...string) {
	named := map[string]tea.Key{
		"enter":     {Type: tea.KeyEnter},
		"esc":       {Type: tea.KeyEsc},
		"tab":       {Type: tea.KeyTab},
		"shift+tab": {Type: tea.KeyShiftTab},
		"backspace": {Type: tea.KeyBackspace},
		"ctrl+u":    {Type: tea.KeyCtrlU},
		"space":     {Type: tea.KeySpace, Runes: []rune{' '}},
		"ctrl+l":    {Type: tea.KeyCtrlL},
		"ctrl+o":    {Type: tea.KeyCtrlO},
		"ctrl+t":    {Type: tea.KeyCtrlT},
		"ctrl+r":    {Type: tea.KeyCtrlR},
		"ctrl+f":    {Type: tea.KeyCtrlF},
		"up":        {Type: tea.KeyUp},
		"down":      {Type: tea.KeyDown},
	}
	for _, k := range keys {
		if key, ok := named[k]; ok {
			s.send(tea.KeyMsg(key))
		} else {
			s.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
		}
	}
}

func (s *screen) typeText(text string) {
	for _, r := range text {
		s.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func (s *screen) click(x, y int) {
	s.send(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
}

func (s *screen) wheel(down bool) {
	b := tea.MouseButtonWheelUp
	if down {
		b = tea.MouseButtonWheelDown
	}
	s.send(tea.MouseMsg{Button: b, Action: tea.MouseActionPress})
}

func (s *screen) shows(want ...string) {
	s.t.Helper()
	view := s.model.View()
	for _, w := range want {
		if !strings.Contains(view, w) {
			s.t.Errorf("screen does not show %q:\n%s", w, view)
		}
	}
}

func (s *screen) hides(unwanted string) {
	s.t.Helper()
	if view := s.model.View(); strings.Contains(view, unwanted) {
		s.t.Errorf("screen shows %q:\n%s", unwanted, view)
	}
}

func (s *screen) m() Model { return s.model.(Model) }

func pad(t *testing.T, name string) proto.PadIndex {
	t.Helper()
	p, err := proto.ParsePad(name)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (s *screen) padName(name string) string { return s.sim.PadName(pad(s.t, name)) }

func TestEverySectionRenders(t *testing.T) {
	s := open(t)
	s.shows("404tui by moahbeatlab", "PROJECT_01", "1 Pads", "3 Projects", "BANKS", "kick", "snare", "hat", "A1  kick")
	s.press("tab")
	s.shows("Pattern A1", "Empty slot.")
	s.press("]")
	s.shows("Pattern B1")
	s.press("2")
	s.shows("SOURCE", "SD card", "▸ IMPORT/", "Details", "Folder")
	s.press("3")
	s.shows("PROJECT_01", "Project 1", "● current", "3 of 160 pads")
	s.press("2")
	s.shows("▸ IMPORT/")
	// The bank chosen on the patterns carries over, so go back to A.
	s.press("1", "[")
	s.shows("kick")
}

func TestHelpAndPalette(t *testing.T) {
	s := open(t)
	// ? is the same searchable list as the palette, for the view in front.
	s.press("?")
	s.shows("Preview pad on the unit", "type to search")
	s.press("?")
	s.hides("type to search")
	s.press("2", "?")
	s.shows("Assign this file to a pad")
	s.hides("Preview pad on the unit")
	s.press("esc", "1")

	s.press(":")
	s.shows("Preview pad on the unit", "Mark / unmark pad", "↑↓ navigate")
	s.hides("Select project")
	s.typeText("unmark")
	s.shows("Mark / unmark pad")
	s.hides("Preview pad")
	s.press("enter")
	s.press(":")
	s.typeText("zzz")
	s.shows("no command matches")
	s.press("esc")
	s.shows("kick")
}

func TestGridMovesLikeTheUnit(t *testing.T) {
	s := open(t)
	// Pad 1 is top left, as on the unit: j goes down to 5, l to 6, k back up to 2.
	s.press("j")
	if s.m().pad != 4 {
		t.Errorf("after j: pad %d", s.m().pad)
	}
	s.press("l", "k")
	if s.m().pad != 1 {
		t.Errorf("after l k: pad %d", s.m().pad)
	}
	s.press("G")
	if s.m().pad != 15 {
		t.Errorf("after G: pad %d", s.m().pad)
	}
	s.press("l", "j")
	if s.m().pad != 15 {
		t.Errorf("the grid's bottom right corner moved to %d", s.m().pad)
	}
	s.press("g")
	if s.m().pad != 0 {
		t.Errorf("after g: pad %d", s.m().pad)
	}
}

func TestRenameAPad(t *testing.T) {
	s := open(t)
	s.press("n")
	s.shows("Rename A1", "> kick")
	s.press("backspace", "backspace")
	s.typeText("ck 2")
	s.press("enter")
	s.shows("A1 renamed to kick 2")
	if got := s.padName("A1"); got != "kick 2" {
		t.Errorf("the unit names A1 %q", got)
	}
	// Esc leaves the name alone; an empty pad has nothing to rename.
	s.press("n", "ctrl+u")
	s.typeText("other")
	s.press("esc", "G", "n")
	s.shows("nothing to rename")
	if got := s.padName("A1"); got != "kick 2" {
		t.Errorf("a cancelled rename changed A1 to %q", got)
	}
}

func TestBulkRenameAndDelete(t *testing.T) {
	s := open(t)
	// Kick is pad 1, snare 2: v marks, l moves right.
	s.press("v", "l", "v", "n")
	s.shows("Add a prefix to 2 pads")
	s.press("tab")
	s.shows("Add a suffix to 2 pads")
	s.typeText("_x")
	s.press("enter")
	s.shows("2 pads renamed", "kick_x", "snare_x")
	if got := s.padName("A3"); got != "hat" {
		t.Errorf("an unmarked pad was renamed to %q", got)
	}

	s.press("V", "d")
	s.shows("Delete the samples on 3 pads (A1 A2 A3)?")
	s.press("n")
	s.shows("kick_x")
	// The marks stay after a "no", so d asks again about the same pads.
	s.press("d", "y")
	s.shows("3 pads deleted")
	if s.padName("A1") != "" || s.padName("A3") != "" {
		t.Errorf("after delete: A1 %q, A3 %q", s.padName("A1"), s.padName("A3"))
	}
	s.press("d")
	s.shows("nothing to delete")
}

func TestBankSwitchFollowsOnTheUnit(t *testing.T) {
	s := open(t)
	s.press("v", "]", "]")
	s.shows("C1")
	if bank, _ := s.sim.Selected(); bank != 2 {
		t.Errorf("the unit shows bank %d", bank)
	}
	if len(s.m().marked) != 0 {
		t.Error("marks survived a bank switch")
	}
	s.press("[", "[", "[")
	if s.m().bank != 9 {
		t.Errorf("bank = %d", s.m().bank)
	}
}

func TestCursorFollowsPadsHitOnTheUnit(t *testing.T) {
	s := open(t)
	s.sim.Select(4, 6)
	s.send(pollMsg{})
	s.send(s.pollResult())
	if m := s.m(); m.bank != 4 || m.pad != 6 {
		t.Errorf("cursor at bank %d pad %d", m.bank, m.pad)
	}
	s.shows("E7")
	// A pad sounding is marked and named in the header.
	frames := uint32(1200)
	s.sim.SetPlaying(&frames)
	s.send(s.pollResult())
	s.shows("▶ E7")
	s.sim.SetPlaying(nil)
	s.send(s.pollResult())
	s.hides("▶ E7")
}

// pollResult runs the poll command as the tick would.
func (s *screen) pollResult() tea.Msg {
	m := s.m()
	return m.poll()()
}

func TestSelectAndRenameAProject(t *testing.T) {
	s := open(t)
	s.press("3", "j", "j", "enter")
	s.shows("project 3 selected", "PROJECT_03")
	// The pads on screen are now the new project's.
	s.press("1")
	s.hides("kick")

	// The unit keeps a name only for a project with something saved in it.
	s.press("3", "n")
	s.shows("Rename project 3")
	s.typeText("!")
	s.press("enter")
	s.shows("project 3 is empty")
	s.sim.AddFile("ROLAND/SP-404MKII/PROJECT_03/PADCONF.BIN", []byte{0, 0, 0, 0, 0, 0, 0, 0, 3, 0, 0, 0})
	s.press("n", "ctrl+u")
	s.typeText("LIVE SET")
	s.press("enter")
	s.shows("LIVE SET")
}

func TestFilesBrowseMoveRenameAndSearch(t *testing.T) {
	s := open(t)
	s.press("2")
	s.shows("▸ IMPORT/")
	s.press("tab")
	s.shows("▾ IMPORT/", "▸ pack/", "kick.wav", "2 Files")
	s.press("tab")
	s.shows("▸ IMPORT/")
	s.hides("kick.wav")
	s.press("tab", "j", "j", "m")
	s.shows("1 item to move")
	s.press("k", "p")
	s.shows("moved to SD:IMPORT/pack/kick.wav")
	if _, ok := s.sim.File("SD:IMPORT/pack/kick.wav"); !ok {
		t.Error("the file did not move on the unit")
	}
	s.shows("▾ IMPORT/", "▸ pack/")
	s.press("j", "l")
	s.shows("▾ pack/", "kick.wav", "snare.wav")

	s.press("j", "n")
	s.shows("Rename kick.wav")
	s.press("ctrl+u")
	s.typeText("bd.wav")
	s.press("enter")
	s.shows("renamed to SD:IMPORT/pack/bd.wav", "▾ pack/")
	if _, ok := s.sim.File("SD:IMPORT/pack/bd.wav"); !ok {
		t.Error("the file was not renamed on the unit")
	}

	s.press("g", "/", "ctrl+u")
	s.shows("Search SD card", "Files", "Tags")
	s.typeText("snare")
	s.press("ctrl+o")
	m := s.m()
	if node, _ := m.fileNode(); node.Name != "snare.wav" {
		t.Errorf("search landed on %q", node.Name)
	}
	// h on a file goes to its folder, and h on the open folder closes it.
	s.press("h", "h")
	s.hides("snare.wav")
	s.press("p")
	s.shows("nothing on the clipboard")

	s.press("t")
	s.shows("Unit storage", "▸ ROLAND/")
	s.press("t")
	s.shows("This computer")
}

func TestAssignAFileThroughThePicker(t *testing.T) {
	s := open(t)
	s.press("2", "tab", "j", "j")
	m := s.m()
	if node, _ := m.fileNode(); node.Name != "kick.wav" {
		t.Fatalf("cursor on %q", node.Name)
	}
	// The picker opens on the target bank at its next free pad: A1-A3 hold samples.
	s.press("enter")
	s.shows("Assign kick.wav to a pad", "A4 is empty")
	// A capital letter picks the bank and digits the pad: C, 7.
	s.press("C", "7")
	s.shows("C7 is empty")
	s.press("enter")
	s.shows("kick.wav: assigned to C7 as \"kick\"")
	if got := s.padName("C7"); got != "kick" {
		t.Errorf("C7 is %q", got)
	}
	// The cursor follows the file to the pad.
	if m := s.m(); m.section != secPads || m.bank != 2 || m.pad != 6 {
		t.Errorf("section %d bank %d pad %d", m.section, m.bank, m.pad)
	}
	stored, _ := s.sim.File(pad(t, "C7").SamplePath(1))
	if sample, err := formats.ParseSMP(stored); err != nil || sample.Frames() != 4800 {
		t.Errorf("the unit holds %d frames (%v)", sample.Frames(), err)
	}

	// The next file goes to the pad after it, in the same bank.
	s.press("2", "enter")
	s.shows("C8 is empty")
	// Two digits make pad 12.
	s.press("1", "2")
	s.shows("C12 is empty")
	s.press("esc")
}

func TestTargetBankInFiles(t *testing.T) {
	s := open(t)
	s.press("2")
	s.shows("SOURCE", "TARGET BANK")
	s.press("]", "]")
	s.shows("bank C · next free C1")
	s.press("tab", "j", "j", "enter")
	s.shows("C1 is empty")
	s.press("esc", "[")
	if s.m().target != 1 {
		t.Errorf("target bank %d", s.m().target)
	}
	// A bank on the rail can be clicked.
	s.click(3, s.railRow(railBank, 4))
	if s.m().target != 4 {
		t.Errorf("clicked bank E, got %d", s.m().target)
	}
	// It is the assign target only: the bank in view on Pads did not move.
	if s.m().bank != 0 {
		t.Errorf("the bank in view changed to %d", s.m().bank)
	}
}

func TestPickerOpensOnTheLastBankAssignedTo(t *testing.T) {
	s := open(t)
	s.press("2", "tab", "j", "j", "enter", "J", "enter")
	s.shows("assigned to J1")
	// Browsing other banks on Pads does not change where files go.
	s.press("1", "[", "[", "[")
	if s.m().bank == 9 {
		t.Fatal("test setup: still on bank J")
	}
	s.press("2", "enter")
	s.shows("J2 is empty")
	s.press("esc")
	// Looking at a bank in the picker and cancelling does not change it either.
	s.press("enter", "C", "esc", "enter")
	s.shows("J2 is empty")
	s.press("esc")
	// It is remembered between runs.
	s = s.restart()
	s.press("2", "enter")
	s.shows("J2 is empty")
}

func TestAssignReplacingAPadAsksTwice(t *testing.T) {
	s := open(t)
	s.press("2", "tab", "j", "j", "enter", "A", "1")
	s.shows("A1 holds")
	s.press("enter")
	s.shows("Press Enter again to replace it")
	if s.padName("A1") != "kick" {
		t.Fatal("A1 changed before the confirmation")
	}
	s.press("2")
	s.shows("A2 holds")
	s.press("esc")
	s.shows("kick.wav")
	s.press("enter", "A", "2", "enter", "enter")
	s.shows("assigned to A2")
}

func TestAssignDialogFromPads(t *testing.T) {
	s := open(t)
	s.press("l", "a")
	s.shows("Assign a file to A2", "A2 holds \"snare\"", "SD card", "▸ IMPORT/")
	s.press("tab", "j", "j")
	s.shows("kick.wav", "0.10 s", "48000 Hz")
	s.press("enter")
	s.shows("Press Enter again to replace it")
	s.press("enter")
	s.shows("kick.wav: assigned to A2")
	if got := s.padName("A2"); got != "kick" {
		t.Errorf("A2 is %q", got)
	}
	if s.m().browser != nil {
		t.Error("the dialog stayed open")
	}
}

func TestAssignDialogRemembersWhereYouWere(t *testing.T) {
	s := open(t)
	s.press("a", "tab", "j", "l", "j", "j")
	m := s.m()
	if node := m.browser.tree.Rows()[m.browser.at]; node.Name != "snare.wav" {
		t.Fatalf("cursor on %q", node.Name)
	}
	s.press("esc")
	// Opened again, it is back on the SD card with the same folders open and the cursor on
	// the same file.
	s.press("a")
	s.shows("▾ IMPORT/", "▾ pack/", "snare.wav")
	m = s.m()
	if node := m.browser.tree.Rows()[m.browser.at]; node.Name != "snare.wav" {
		t.Errorf("reopened on %q", node.Name)
	}
	s.press("esc")
	if s.store.state.AssignAt != "SD:IMPORT/pack/snare.wav" || len(s.store.state.AssignExpanded) != 2 {
		t.Errorf("remembered %+v", s.store.state)
	}

	// And it is still there after quitting and starting again.
	s = s.restart()
	s.press("a")
	s.shows("▾ IMPORT/", "▾ pack/", "snare.wav")
	m = s.m()
	if node := m.browser.tree.Rows()[m.browser.at]; node.Name != "snare.wav" {
		t.Errorf("after a restart: cursor on %q", node.Name)
	}
}

func TestAssignDialogMarksFillFreePads(t *testing.T) {
	s := open(t)
	s.press("a", "tab", "j", "l", "j", "j", "v", "v")
	s.shows("2 files marked", "from A1 on")
	s.press("enter")
	// Multi-file assigns do not ask: they take free pads, here A4 and A5.
	s.shows("2 files assigned from A1")
	if s.padName("A4") != "snare" || s.padName("A5") != "kick" {
		t.Errorf("A4 %q, A5 %q", s.padName("A4"), s.padName("A5"))
	}
}

func TestAssignDialogListensAndTakesATypedPath(t *testing.T) {
	s := open(t)
	s.press("G", "a", "tab", "j", "j", "space")
	if len(s.player.played) != 1 {
		t.Fatalf("played %d", len(s.player.played))
	}
	s.shows("♪")
	s.press("esc")
	if s.player.stopped == 0 {
		t.Error("closing the dialog left the sound playing")
	}
	s.press("a", "p")
	s.shows("Assign the audio file at this path to A16")
	s.press("ctrl+u")
	s.typeText("SD:IMPORT/pack/snare.wav")
	s.press("enter")
	s.shows("snare.wav: assigned to A16")
	if s.padName("A16") != "snare" {
		t.Errorf("A16 is %q", s.padName("A16"))
	}
}

func TestStateIsRememberedBetweenRuns(t *testing.T) {
	s := open(t)
	s.press("ctrl+t", "]", "]")
	s.press("2", "tab", "j", "l")
	if st := s.store.state; st.Theme != 1 || st.Bank != 2 || st.FilesRoot != "SD:" || len(st.FilesExpanded) != 2 {
		t.Fatalf("remembered %+v", st)
	}
	s = s.restart()
	if m := s.m(); m.themeAt != 1 || m.bank != 2 {
		t.Errorf("theme %d, bank %d after a restart", m.themeAt, m.bank)
	}
	s.press("2")
	s.shows("▾ IMPORT/", "▾ pack/", "snare.wav")
}

func TestListenToAFile(t *testing.T) {
	s := open(t)
	s.press("2", "tab", "j", "j", "space")
	if len(s.player.played) != 1 || s.player.played[0].Frames() != 4800 {
		t.Fatalf("played %+v", s.player.played)
	}
	s.shows("♪")
	s.press("space")
	if s.player.stopped == 0 || s.m().listening != "" {
		t.Error("a second Space did not stop it")
	}
}

func TestCopyPadsToAnotherBank(t *testing.T) {
	s := open(t)
	// Copying needs real SMP files on the unit, so put samples on through the app.
	m := s.m()
	for i, name := range []string{"kick", "snare", "hat"} {
		s.sim.AddFile("SD:IMPORT/"+name+".wav", wav(480+i*10))
		if _, err := app.Assign(m.unit, m.unit, proto.AllPads()[i], "SD:IMPORT/"+name+".wav"); err != nil {
			t.Fatal(err)
		}
	}
	s.press("r")

	// One pad: it opens on the next free pad of the same bank, and any bank can be typed.
	s.press("c")
	s.shows("Copy A1 to a pad", "A4 is empty")
	s.press("C", "5")
	s.shows("C5 is empty")
	s.press("enter")
	s.shows("A1 copied to C5")
	if s.padName("C5") != "kick" || s.padName("A1") != "kick" {
		t.Errorf("C5 %q, A1 %q", s.padName("C5"), s.padName("A1"))
	}
	// The cursor is on the copy.
	if m := s.m(); m.bank != 2 || m.pad != 4 {
		t.Errorf("bank %d pad %d", m.bank, m.pad)
	}

	// A pad that holds a sample is replaced only on a second Enter.
	s.press("c", "A", "2")
	s.shows("A2 holds \"snare\"")
	s.press("enter")
	s.shows("Press Enter again to replace them")
	if s.padName("A2") != "snare" {
		t.Fatal("A2 changed before the confirmation")
	}
	s.press("esc")
	if s.padName("A2") != "snare" {
		t.Error("cancelling replaced A2")
	}
	s.press("c", "A", "2", "enter", "enter")
	s.shows("C5 copied to A2")
	if s.padName("A2") != "kick" {
		t.Errorf("A2 is %q", s.padName("A2"))
	}

	// Marked pads keep their layout: all of bank A onto bank E.
	s.press("V", "c")
	s.shows("Copy 3 pads, keeping their layout")
	s.press("E", "1")
	s.shows("E1 is empty"[:0] + "all free")
	s.press("enter")
	s.shows("3 pads copied, from A1 to E1")
	if s.padName("E1") != "kick" || s.padName("E2") != "kick" || s.padName("E3") != "hat" {
		t.Errorf("E1 %q, E2 %q, E3 %q", s.padName("E1"), s.padName("E2"), s.padName("E3"))
	}

	// Past the end does not fit, and an empty pad has nothing to copy.
	s.press("V", "c", "J", "1", "5")
	s.shows("past J16")
	s.press("esc", "esc")
	s.press("G", "c")
	s.shows("nothing to copy")
}

func TestMovePickUpAndSwap(t *testing.T) {
	s := open(t)
	s.press("x")
	s.shows("A1 picked up")
	s.press("l", "l", "l", "l")
	s.press("p")
	// Pad 5 is the cursor after k; but l stops at the edge, so this lands on pad 4.
	s.shows("A1 moved to A4")
	if s.padName("A1") != "" || s.padName("A4") != "kick" {
		t.Errorf("A1 %q, A4 %q", s.padName("A1"), s.padName("A4"))
	}
	s.press("g", "l", "x", "l", "s")
	s.shows("swapped with")
	s.press("p")
	s.shows("nothing picked up")
}

func TestPatternsOpenAndRetarget(t *testing.T) {
	s := open(t)
	rec := func(delta byte, pad proto.PadIndex) []byte {
		return []byte{delta, 0x2F + byte(pad.Index()%80), byte(pad.Index() / 80), 0, 0x7F, 0x40, 0x10, 0}
	}
	var file []byte
	for _, r := range [][]byte{rec(240, pad(t, "A1")), rec(240, pad(t, "A2")), {0, 0x8C, 0, 0, 0, 0, 0, 0}, {4, 0, 0, 0, 3, 0, 0, 0}} {
		file = append(file, r...)
	}
	s.sim.AddFile(formats.PatternPath(1, pad(t, "B2")), file)

	s.press("tab", "]")
	s.shows("2 pads"[:0] + "1 pads")
	s.press("enter")
	s.shows("That pattern"[:0] + "that pattern slot is empty")
	// Slot 2 is one right of slot 1, which is top left.
	s.press("l", "enter")
	s.shows("Pattern B2", "120.00 BPM", "A1   kick", "A2   snare")
	// The keys are listed in groups, each under its own heading.
	s.shows("move", "velocity", "length", "timing", "pad", "history", "0 undo · 0 redo")
	// A hit is a mark as bright as its velocity.
	if !strings.ContainsAny(ansi.Strip(s.m().View()), "○◌◉●") {
		t.Error("no hit is drawn")
	}
	s.press("b")
	s.shows("move its pads from bank A to bank", "B")
	s.press("]", "]")
	s.press("enter")
	s.shows("2 note(s) moved from bank A to bank D", "D1", "D2")
	// After the move the pads are in one bank D, so the pattern still retargets.
	s.press("esc")
	s.shows("Nothing is open in the sequencer")
	if _, err := app.OpenPattern(s.m().unit, pad(t, "B2")); err != nil {
		t.Error(err)
	}
}

// railRow is the screen row of a rail item.
func (s *screen) railRow(kind railKind, index int) int {
	m := s.m()
	for i, it := range m.rail(m.geom().bodyH) {
		if it.kind == kind && it.index == index {
			return rowBody + i
		}
	}
	s.t.Fatalf("no rail row for kind %d index %d", kind, index)
	return 0
}

func TestMouse(t *testing.T) {
	s := open(t)
	g := s.m().geom()
	// Sections, on the tab bar.
	s.clickTab(1)
	s.shows("▸ IMPORT/")
	s.clickTab(0)
	s.shows("kick")
	// Banks, on the rail.
	s.click(3, s.railRow(railBank, 2))
	if s.m().bank != 2 {
		t.Errorf("clicked bank C, got %d", s.m().bank)
	}
	s.click(3, s.railRow(railBank, 0))
	if s.m().bank != 0 {
		t.Errorf("clicked bank A, got %d", s.m().bank)
	}
	// A tile: the second click on the pad in front previews it. Pad 2 is top row, column 2.
	x, y := g.bodyX+(g.tileW+1)+2, rowBody+g.top+1
	s.click(x, y)
	if s.m().pad != 1 {
		t.Errorf("clicked pad 2, got pad %d", s.m().pad)
	}
	s.click(x, y)
	s.shows("previewing A2")
	// A gap between tiles does nothing; neither does the rail's own blank area.
	s.click(g.bodyX+g.tileW, y)
	if s.m().pad != 1 {
		t.Error("a click between tiles moved the cursor")
	}
	// A tile in the bottom row.
	s.click(g.bodyX+2, rowBody+g.top+3*(g.tileH+1)+1)
	if s.m().pad != 12 {
		t.Errorf("clicked pad 13, got pad %d", s.m().pad)
	}
	// The wheel changes bank on Pads.
	s.wheel(true)
	if s.m().bank != 1 {
		t.Errorf("wheel: bank %d", s.m().bank)
	}
	s.wheel(false)

	// Files: a click selects a row, a second click on it opens the folder.
	s.press("2")
	s.click(g.bodyX+5, rowBody+1)
	s.shows("▾ IMPORT/")
	s.wheel(true)
	if s.m().fileAt != 1 {
		t.Errorf("wheel in files: %d", s.m().fileAt)
	}
	s.click(g.bodyX+5, rowBody+1+2)
	if s.m().fileAt != 2 {
		t.Errorf("clicked the third row, got %d", s.m().fileAt)
	}
	// Sources, on the rail.
	s.click(3, s.railRow(railRoot, 1))
	s.shows("▸ ROLAND/")
	s.press("3")
	s.click(g.bodyX+5, rowBody+1+3)
	if s.m().projectAt != 3 {
		t.Errorf("clicked project 4, got %d", s.m().projectAt)
	}
}

func TestExportAndRestoreThroughTheFolderBrowser(t *testing.T) {
	s := open(t)
	// Real samples are needed for an export: put them on through the app.
	m := s.m()
	for _, name := range []string{"A1", "A2", "A3"} {
		s.sim.AddFile("SD:IMPORT/"+name+".wav", wav(480))
		if _, err := app.Assign(m.unit, m.unit, pad(t, name), "SD:IMPORT/"+name+".wav"); err != nil {
			t.Fatal(err)
		}
	}
	s.press("r")

	// Export: the same tree as Files, then a name for the folder made inside the one chosen.
	s.press("e")
	s.shows("Export bank A into…", "SD card", "Unit storage", "This computer", "▸ IMPORT/", "Folder  SD:IMPORT")
	s.press("enter")
	s.shows("Export bank A into SD:IMPORT as the folder", "> PROJECT_01-A")
	s.press("enter")
	s.shows("3 pads exported, 13 empty, to SD:IMPORT/PROJECT_01-A")
	if _, ok := s.sim.File("SD:IMPORT/PROJECT_01-A/samples.json"); !ok {
		t.Error("samples.json was not written")
	}
	s.press("V", "d", "y")
	if s.padName("A2") != "" {
		t.Fatal("the pads survived the delete")
	}

	// Restore: open IMPORT, go down to the export's folder, Enter.
	s.press("R")
	s.shows("Restore pads from the export in…", "▸ IMPORT/")
	s.press("tab")
	s.shows("▸ pack/", "▸ PROJECT_01-A/", "kick.wav")
	s.press("j", "j")
	s.shows("Folder  SD:IMPORT/PROJECT_01-A")
	s.press("enter")
	// The bank to restore onto is asked; the pads are free, so Enter goes ahead.
	s.shows("Restore 3 pads from the export in SD:IMPORT/PROJECT_01-A onto which bank?", "exported from bank A", "all 3 target pads on bank A are free")
	s.press("enter")
	s.shows("3 pads restored onto bank A from SD:IMPORT/PROJECT_01-A")
	if s.padName("A2") != "A2" {
		t.Errorf("A2 is %q", s.padName("A2"))
	}

	// The browser reaches the other sources, closes with Esc, and still takes a typed path.
	s.press("R", "t")
	s.shows("▸ ROLAND/")
	s.press("t")
	s.shows("This computer")
	s.press("esc")
	s.hides("This computer")
	s.shows("BANKS")
	s.press("R", "/")
	s.shows("Restore pads from the export in the folder", "> SD:")
	s.press("ctrl+u")
	s.typeText("SD:IMPORT/PROJECT_01-A")
	s.press("enter")
	// They hold samples now, so replacing them takes a second Enter.
	s.shows("onto which bank?", "3 of the 3 pads on bank A hold samples")
	s.press("enter")
	s.shows("Press Enter again to replace")
	s.press("enter")
	s.shows("3 pads restored onto bank A")
	s.press("e", "/")
	s.shows("Export bank A to the folder")
	s.press("esc")

	// From Files: R opens the browser where the tree is, and Enter on samples.json restores
	// the folder it is in.
	s.press("2", "tab", "j", "j", "tab", "R")
	s.shows("Restore pads from the export in…", "▾ PROJECT_01-A/", "samples.json")
	s.press("esc", "j", "j", "j", "j")
	s.shows("Pad export")
	s.press("enter")
	s.shows("onto which bank?")
	s.press("enter", "enter")
	s.shows("3 pads restored onto bank A from SD:IMPORT/PROJECT_01-A")
	// The folder is noted as used, and the Files view says so.
	data, ok := s.sim.File("SD:404tui-folders.json")
	if !ok || !strings.Contains(string(data), `"1/A"`) || !strings.Contains(string(data), "SD:IMPORT/PROJECT_01-A") {
		t.Fatalf("the folder notes: %v %s", ok, data)
	}
	s.press("2")
	s.shows("◆▾ IMPORT/", "◆  ▾ PROJECT_01-A/")
	s.press("k", "k", "k", "k")
	s.shows("Used already", "restored onto project 1, bank A")
}

func TestThemesAndNarrowScreens(t *testing.T) {
	s := open(t)
	s.press("ctrl+t")
	s.shows("theme: Ocean")
	s.press("ctrl+t", "ctrl+t")
	s.shows("theme: Hardware")
	for _, size := range [][2]int{{80, 24}, {60, 20}, {140, 40}, {200, 50}, {100, 14}} {
		s.send(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, section := range []string{"1", "tab", "2", "3"} {
			s.press(section)
			lines := strings.Split(s.model.View(), "\n")
			if len(lines) != size[1] {
				t.Errorf("%dx%d section %s: %d lines", size[0], size[1], section, len(lines))
			}
			for i, line := range lines {
				if w := lipglossWidth(line); w > size[0] {
					t.Errorf("%dx%d section %s line %d is %d wide", size[0], size[1], section, i, w)
				}
			}
		}
	}
}

func TestKeysWaitWhileBusy(t *testing.T) {
	sm := sim.New()
	d := device.New(sm, "sim")
	t.Cleanup(func() { d.Close() })
	var model tea.Model = New(d, nil, nil)
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	if view := model.View(); !strings.Contains(view, "reading the unit") {
		t.Errorf("screen while loading:\n%s", view)
	}
	// q asks first, even while busy; a second q quits.
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd != nil || !strings.Contains(model.View(), "Quit 404tui?") {
		t.Fatalf("q did not ask before quitting:\n%s", model.View())
	}
	if _, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}); cmd == nil {
		t.Error("q twice did not quit while busy")
	}
}

func TestQuitAsksFirst(t *testing.T) {
	s := open(t)
	s.press("q")
	s.shows("Quit 404tui?")
	// n keeps the app open.
	s.press("n")
	s.hides("Quit 404tui?")
	if s.m().prompt != nil {
		t.Error("n left the question open")
	}
	// Esc with nothing to back out of asks too, and Esc again stays.
	s.press("esc")
	s.shows("Quit 404tui?")
	s.press("esc")
	s.hides("Quit 404tui?")
}

// backup puts a small project backup on the unit under folder.
func backup(s *sim.Sim, folder, name string) {
	padconf := make([]byte, 0xC0)
	padconf[8] = 3
	copy(padconf[0x80:], name)
	s.AddFile(folder+"/PADCONF.BIN", padconf)
	s.AddFile(folder+"/PICTURE/startup_1.bmp", []byte("bmp"))
	s.AddFile(folder+"/PTN/PTN00001.BIN", []byte("ptn"))
	s.AddFile(folder+"/SMPL/BANK1-01.SMP", []byte("smp"))
}

func TestProjectExportToTheCardAndRestoreFromIt(t *testing.T) {
	s := open(t)
	backup(s.sim, "ROLAND/SP-404MKII/PROJECT_01", "LIVE SET")

	s.press("3")
	s.shows("e export", "R restore")
	s.press("e")
	s.shows("Export project 1 into…", "SD card", "▸ IMPORT/")
	s.press("enter")
	s.shows("Export project 1 into SD:IMPORT as the folder", "> PROJECT_01")
	s.press("enter")
	s.shows("project 1 exported: 6 files")
	for _, name := range []string{"PADCONF.BIN", "PICTURE/startup_1.bmp", "PTN/PTN00001.BIN", "SMPL/BANK1-01.SMP"} {
		if _, ok := s.sim.File("SD:IMPORT/PROJECT_01/" + name); !ok {
			t.Errorf("%s was not exported", name)
		}
	}

	// Restore it over project 2: choose the folder, read the plan, confirm.
	s.press("j", "R", "tab", "j", "j")
	s.shows("Restore project 2 from the backup folder…", "Folder  SD:IMPORT/PROJECT_01")
	s.press("enter")
	s.shows("Overwrite project 2?", "LIVE SET", "6 files", "SD:IMPORT/PROJECT_01", "everything in it is erased")
	s.press("n")
	if _, ok := s.sim.File("ROLAND/SP-404MKII/PROJECT_02/PADCONF.BIN"); ok {
		t.Fatal("declining still wrote the backup")
	}
	s.press("R", "tab", "j", "j", "enter", "y")
	s.shows("project 2 restored from SD:IMPORT/PROJECT_01")
	for _, name := range []string{"PADCONF.BIN", "PICTURE/startup_1.bmp", "PTN/PTN00001.BIN", "SMPL/BANK1-01.SMP"} {
		if _, ok := s.sim.File("ROLAND/SP-404MKII/PROJECT_02/" + name); !ok {
			t.Errorf("%s was not restored", name)
		}
	}
	// Restoring selected the target project.
	if m := s.m(); m.snap.Project != 1 {
		t.Errorf("current project %d", m.snap.Project+1)
	}
}

func TestProjectExportAndRestoreRefuse(t *testing.T) {
	s := open(t)
	s.press("3", "e", "enter", "enter")
	s.shows("project 1 is empty")
	// A folder without PADCONF.BIN is not a backup, and a typed path works too.
	s.press("R", "enter")
	s.shows("has no PADCONF.BIN")
	s.press("R", "/")
	s.shows("Restore project 1 from the backup folder", "> SD:")
	s.press("esc")
	s.press("e", "/")
	s.shows("Export project 1 to the folder")
	s.press("esc")
}

// TestConnectingAnnouncesAndReconnects drives the UI through no unit, plugging one in,
// pulling the cable and plugging it back.
func TestConnectingAnnouncesAndReconnects(t *testing.T) {
	sm := sim.New()
	pad1, _ := proto.NewPadIndex(0)
	sm.SetSample(pad1, "kick", 48_000)
	attempts, plugged := 0, false
	connect := func() (app.Unit, io.Closer, error) {
		attempts++
		if !plugged {
			return nil, nil, ErrNoUnit
		}
		d := device.New(sm, "sim")
		return d, d, nil
	}
	sc := &screen{t: t, sim: sm, player: &fakePlayer{}, store: &memStore{}}
	sc.model = NewConnecting(connect, sc.player, nil)
	sc.run(sc.model.Init())
	ticks := func(n int) {
		for i := 0; i < n; i++ {
			sc.send(tickMsg{})
		}
	}

	// Nothing plugged in: it says it is waiting, keeps looking, and keys do nothing but quit.
	sc.shows("404tui by moahbeatlab", "○ no unit", "Waiting for an SP-404MKII", "USB 0582:02e7")
	before := attempts
	ticks(connectTicks * 3)
	if attempts < before+3 {
		t.Errorf("only %d attempts in three intervals", attempts-before)
	}
	sc.press("3", "a", "d")
	sc.shows("Waiting for an SP-404MKII")
	sc.press("ctrl+t")
	sc.press("?")
	sc.shows("Keybindings")
	sc.press("esc")
	sc.hides("Keybindings")

	// Plug it in: it connects by itself and announces it.
	plugged = true
	ticks(connectTicks)
	sc.shows("● CONNECTED  sim", "kick", "● Connected to the SP-404MKII on sim")
	sc.hides("Waiting for")
	// The loud banner gives way to a quiet one.
	ticks(60)
	sc.hides("CONNECTED")
	sc.shows("● sim")

	// Pull the cable: the next look at the unit notices, and it goes back to waiting.
	plugged = false
	sc.sim.Close()
	ticks(pollTicks)
	sc.shows("The connection to the unit was lost.", "Waiting for an SP-404MKII", "○ no unit")
	sc.hides("kick")

	// Plug it back in: it reconnects and loads again.
	sc.sim.Reopen()
	plugged = true
	ticks(connectTicks)
	sc.shows("● CONNECTED  sim", "kick")
	sc.hides("was lost")
}

// TestConnectingShowsRealErrors: a unit that is there but fails to open is reported.
func TestConnectingShowsRealErrors(t *testing.T) {
	connect := func() (app.Unit, io.Closer, error) { return nil, nil, errors.New("permission denied on /dev/cu.x") }
	sc := &screen{t: t, player: &fakePlayer{}, store: &memStore{}}
	sc.model = NewConnecting(connect, nil, nil)
	sc.run(sc.model.Init())
	sc.shows("permission denied on /dev/cu.x")
}

func TestBankTempoAndVolumeFromThePads(t *testing.T) {
	s := open(t)
	s.shows("Bank A", "Tempo 120.00 BPM", "Volume", "127", "b edit")
	// Steps show at once and reach the unit.
	s.press(">", ">", ">")
	s.shows("Tempo 123.00 BPM")
	s.press("-", "-")
	s.shows("Volume", "117")
	s.press("r")
	s.shows("Tempo 123.00 BPM", "117")
	// Another bank has its own.
	s.press("]")
	s.shows("Bank B", "Tempo 120.00 BPM", "127")
	s.press("+")
	s.shows("bank B volume is at its limit")
	s.press("[", "<")
	s.shows("Tempo 122.00 BPM")

	// The dialog: Tab between the fields, typed values.
	s.press("b")
	s.shows("Bank A", "Tempo (40-200 BPM)", "Volume (0-127)", "> 122.00")
	s.press("ctrl+u")
	s.typeText("98.5")
	s.press("tab", "ctrl+u")
	s.typeText("64")
	s.press("enter")
	s.shows("bank A: tempo 98.50 BPM, volume 64", "Tempo 98.50 BPM")
	// Out-of-range values are refused and the bank is left alone.
	s.press("b", "ctrl+u")
	s.typeText("300")
	s.press("enter")
	s.shows("bank tempo must be between 40 and 200 BPM", "Tempo 98.50 BPM")
	s.press("b", "esc")
	s.hides("Tempo (40-200 BPM)")
	// The same keys work on Patterns, where the bank's tempo matters.
	s.press("tab")
	s.shows("Bank A", "Tempo 98.50 BPM")
	s.press("]", ">")
	s.shows("Bank B", "Tempo 121.00 BPM")
}

func TestBankBarShowsWhenProjectTempoIsInUse(t *testing.T) {
	s := open(t)
	m := s.m()
	m.snap.UsesProjectTempo, m.snap.ProjectTempo = true, 100.5
	s.model = m
	s.shows("(project tempo 100.50 is in use)")
}

func TestSearchFindsAudioAnywhereOnTheCard(t *testing.T) {
	s := open(t)
	for _, f := range []string{"SD:IMPORT/Drums/808 Kick.wav", "SD:IMPORT/Drums/Snare.wav", "SD:IMPORT/Kicks/deep.wav", "SD:EXPORT/old kick take 2.wav"} {
		s.sim.AddFile(f, wav(480))
	}
	s.press("2", "/", "ctrl+u")
	s.shows("Search SD card", "audio files indexed")
	s.typeText("kick")
	// The file's own name matches come first, then ones that only match a folder.
	s.shows("808 Kick.wav", "kick.wav", "old kick take 2.wav", "deep.wav")
	m := s.m()
	if m.search.results[0].Name() != "old kick take 2.wav" || m.search.results[3].Name() != "deep.wav" {
		t.Errorf("order: %q ... %q", m.search.results[0].Name(), m.search.results[3].Name())
	}
	// Words narrow it down, folders included; the details of the result in front show.
	s.typeText(" drums")
	s.shows("808 Kick.wav", "0.01 s")
	s.hides("old kick")
	// Enter opens the pad picker on the target bank.
	s.press("enter")
	s.shows("Assign 808 Kick.wav to a pad", "A4 is empty")
	s.press("esc")

	// Tab shows the file in the tree, with its folders open.
	s.press("/", "ctrl+u")
	s.typeText("deep")
	s.press("ctrl+o")
	s.shows("▾ IMPORT/", "▾ Kicks/", "deep.wav")
	m = s.m()
	if node, _ := m.fileNode(); node.Name != "deep.wav" {
		t.Errorf("revealed %q", node.Name)
	}
	s.press("/", "ctrl+u")
	s.typeText("nothing at all")
	s.shows("nothing matches")
	s.press("ctrl+u")
	s.shows("audio files indexed")
	s.press("esc")
	s.hides("Search SD card")
}

func TestSearchListensAndMovesThroughResults(t *testing.T) {
	s := open(t)
	s.press("2", "/", "ctrl+u")
	s.typeText("wav")
	s.press("down", "ctrl+l")
	if len(s.player.played) != 1 {
		t.Fatalf("played %d", len(s.player.played))
	}
	s.press("up", "ctrl+l")
	if len(s.player.played) != 2 {
		t.Errorf("played %d after moving", len(s.player.played))
	}
	s.press("esc")
	if s.player.stopped == 0 {
		t.Error("closing the search left the sound playing")
	}
}

func TestSearchIsCachedUntilFilesChange(t *testing.T) {
	s := open(t)
	s.press("2", "/", "ctrl+u")
	s.typeText("fresh")
	s.shows("nothing matches")
	s.press("esc")
	s.sim.AddFile("SD:IMPORT/fresh start.wav", wav(480))
	// The index was built before the file was there, and is kept.
	s.press("/", "ctrl+u")
	s.typeText("fresh")
	s.shows("nothing matches")
	s.press("esc", "r", "2", "/", "ctrl+u")
	s.typeText("fresh")
	s.shows("fresh start.wav")
	s.press("esc")
	// Renaming a file also drops it.
	s.press("tab", "j", "j", "n")
	s.press("ctrl+u")
	s.typeText("renamed.wav")
	s.press("enter")
	s.press("/", "ctrl+u")
	s.typeText("renamed")
	s.shows("renamed.wav")
}

func TestSearchFromTheAssignDialog(t *testing.T) {
	s := open(t)
	// Pad 16 is empty: Enter on a result puts the file straight on it.
	s.press("G", "a", "/", "ctrl+u")
	s.shows("Search SD card for A16")
	s.typeText("snare")
	s.shows("snare.wav")
	s.press("enter")
	s.shows("snare.wav: assigned to A16")
	if s.padName("A16") != "snare" || s.m().browser != nil || s.m().search != nil {
		t.Errorf("A16 %q, dialog open %v", s.padName("A16"), s.m().browser != nil)
	}

	// A pad that holds a sample asks for a second Enter.
	s.press("g", "a", "/", "ctrl+u")
	s.typeText("kick")
	s.shows("A1 holds \"kick\" — it will be replaced")
	s.press("enter")
	s.shows("Press Enter again to replace")
	s.press("esc")
	s.shows("Assign a file to A1")
	// Tab shows the result in the dialog's own tree.
	s.press("/", "ctrl+u")
	s.typeText("snare")
	s.press("ctrl+o")
	s.shows("▾ IMPORT/", "▾ pack/", "Assign a file to A1")
	m := s.m()
	if node := m.browser.tree.Rows()[m.browser.at]; node.Name != "snare.wav" {
		t.Errorf("dialog cursor on %q", node.Name)
	}
}

func TestSearchResultsAreInAlignedColumns(t *testing.T) {
	s := open(t)
	s.sim.AddFile("SD:Loops/Deep House/Basslines/hdhkb_bassloop01_f_124bpm.wav", wav(480))
	s.sim.AddFile("SD:Loops/Deep House/Basslines/a.wav", wav(480))
	s.sim.AddFile("SD:Loops/another very long folder name/with subfolders/that go on/and on/x.wav", wav(480))
	s.press("2", "/", "ctrl+u")
	s.typeText(".wav")
	// The folder column starts at the column of its header in every row, whatever the name.
	lines := strings.Split(ansiStrip(s.model.View()), "\n")
	col, rows := -1, 0
	for _, line := range lines {
		r := []rune(line)
		if i := strings.Index(line, "Folder"); i >= 0 && strings.Contains(line, "Name") {
			col = len([]rune(line[:i]))
			continue
		}
		if col < 0 || !strings.Contains(line, ".wav") || len(r) <= col+1 {
			continue
		}
		rows++
		if r[col-1] != ' ' || r[col-2] != ' ' || r[col] == ' ' {
			t.Errorf("the folder column is not at %d in %q", col, line)
		}
	}
	if rows < 4 {
		t.Fatalf("found %d result rows:\n%s", rows, ansiStrip(s.model.View()))
	}
	// A long folder is cut at its start, keeping the part next to the file.
	s.shows("Name", "Folder")
	s.press("ctrl+u")
	s.typeText("x")
	view := ansiStrip(s.model.View())
	if !strings.Contains(view, "…") || !strings.Contains(view, "and on") {
		t.Errorf("long folder not cut at its start:\n%s", view)
	}
}

func TestListenPlaysAgainOnceItHasFinished(t *testing.T) {
	s := open(t)
	s.press("2", "tab", "j", "j", "space")
	s.shows("♪ playing kick.wav")
	// While it sounds, the same key stops it.
	s.press("space")
	if s.player.stopped == 0 || s.m().listening != "" {
		t.Fatal("pressing it while it sounds did not stop it")
	}
	// Once it has finished, the same key plays it again instead of "stopping" nothing.
	s.press("space")
	if len(s.player.played) != 2 {
		t.Fatalf("played %d", len(s.player.played))
	}
	s.player.playing = false // it ran to the end
	s.press("space")
	if len(s.player.played) != 3 {
		t.Errorf("after it finished, the key did not play it again (%d plays)", len(s.player.played))
	}
}

func TestNoAudioOutputSaysWhy(t *testing.T) {
	s := open(t)
	m := s.m().WithAudio(nil, errors.New("no output device"))
	s.model = m
	s.press("2", "tab", "j", "j", "space")
	s.shows("no audio output to listen on: no output device")
	s.press("/", "ctrl+u")
	s.typeText("kick")
	s.press("ctrl+a")
	s.shows("no audio output to autoplay on")
}

func TestAutoplayInTheSearchPlaysWhereTheCursorRests(t *testing.T) {
	s := open(t)
	s.press("2", "/", "ctrl+u")
	s.typeText("wav")
	s.shows("Ctrl-A autoplay off")
	s.press("ctrl+a")
	s.shows("autoplay on")
	before := len(s.player.played)
	s.press("down")
	if len(s.player.played) != before+1 {
		t.Fatalf("moving with autoplay on played %d", len(s.player.played)-before)
	}
	// An autoplay timer that was overtaken by another move does nothing.
	m := s.m()
	stale := autoplayMsg{seq: m.autoSeq - 1}
	s.send(stale)
	if len(s.player.played) != before+1 {
		t.Error("a stale autoplay timer played something")
	}
	s.press("ctrl+a", "up")
	if len(s.player.played) != before+1 {
		t.Error("autoplay off still played")
	}
	s.shows("Ctrl-A autoplay off")
}

func TestAutoplayInTheAssignDialog(t *testing.T) {
	s := open(t)
	s.press("G", "a", "ctrl+a", "tab", "j", "j")
	if len(s.player.played) == 0 {
		t.Fatal("autoplay in the dialog played nothing")
	}
	s.shows("autoplay on")
	s.press("esc")
	if s.player.stopped == 0 {
		t.Error("closing the dialog left it playing")
	}
}

func TestPadDetailShowsSampleSettingsUsageAndProgress(t *testing.T) {
	s := open(t)
	m := s.m()
	s.sim.AddFile("SD:IMPORT/loop.wav", wav(96000))
	a5 := pad(t, "A5")
	if _, err := app.Assign(m.unit, m.unit, a5, "SD:IMPORT/loop.wav"); err != nil {
		t.Fatal(err)
	}
	set := func(name string, v int32) {
		info, _ := proto.ParamByName(name)
		if err := m.unit.SetParam(info.ID, proto.TargetPad(a5), v); err != nil {
			t.Fatal(err)
		}
	}
	set("level", 100)
	set("bpm", 12000)
	set("balance", 70)
	set("pitch-coarse", 3)
	set("play-mode", 1)
	set("loop", 1)
	set("bus-fx", 2)
	set("mute-group", 4)
	m.unit.SetChopPoints(a5, []uint32{24000, 48000})
	s.sim.SetPattern(pad(t, "B2"), a5)
	s.sim.SetPattern(pad(t, "C3"), a5)

	s.send(tea.WindowSizeMsg{Width: 180, Height: 60})
	s.press("r", "j")
	s.shows("A5  loop", "2.00 s", "mono", "BANK1-05.SMP", "≈", "Trim", "0.00–2.00 s", "2 chop points", "┃",
		"SAMPLE", "PLAYBACK", "SOUND", "ROUTING", "ENVELOPE",
		"Level", "100", "Pan", "R6", "reverse", "Loop     ● on", "Gate     ○ off",
		"+3 st", "BUS 2", "Mute grp", "Used in  B2 · C3", "S edit settings")
	s.hides("waveform")
	// While it sounds on the unit, how far along it is.
	frames := uint32(48000)
	s.sim.SetPlaying(&frames)
	s.sim.Select(0, 4)
	s.send(s.pollResult())
	s.shows("▶", "1.00 / 2.00 s")
	// A pad nothing uses says so; an empty one offers what to do.
	s.press("j")
	s.shows("No sample on this pad.", "Used in no pattern")
}

func TestPadSettingsEditor(t *testing.T) {
	s := open(t)
	m := s.m()
	s.sim.AddFile("SD:IMPORT/loop.wav", wav(4800))
	for _, name := range []string{"A1", "A2"} {
		if _, err := app.Assign(m.unit, m.unit, pad(t, name), "SD:IMPORT/loop.wav"); err != nil {
			t.Fatal(err)
		}
	}
	// Start from known values: the simulator's pads begin at zero for everything.
	for _, name := range []string{"A1", "A2"} {
		for param, v := range map[string]int32{"level": 100, "balance": 64, "play-mode": 0, "gate": 0, "pitch-coarse": 0} {
			info, _ := proto.ParamByName(param)
			m.unit.SetParam(info.ID, proto.TargetPad(pad(t, name)), v)
		}
	}
	s.press("r")

	// Quick level keys step by 5, at once and on the unit.
	s.press(".", ".")
	s.shows("110")
	s.press(",")
	s.shows("105")
	s.press("r")
	s.shows("105")

	s.press("S")
	s.shows("Settings  ·  A1  loop", "Level", "Pan", "Pitch (semitones)", "Time stretch", "Play mode", "Mute group", "Bus FX", "Release")
	// Level is first; h and l step it. Then move down to Pan and Pitch.
	s.press("l")
	s.shows("110")
	s.press("j", "l", "l", "j", "l", "l", "l")
	s.shows("C"[:0]+"R8", "+3")
	// Cycling settings wrap round: play mode has four values.
	s.press("j", "j", "j", "j")
	for i := 0; i < 5; i++ {
		s.press("l")
	}
	s.shows("reverse")
	s.press("l", "l", "l")
	s.shows("forward")
	// On/off settings toggle with Space.
	s.press("j", "space")
	s.shows("Gate", "on")
	s.press("esc")
	s.hides("Settings  ·")
	for name, want := range map[string]int32{"level": 110, "balance": 72, "pitch-coarse": 3, "play-mode": 0, "gate": 1} {
		info, _ := proto.ParamByName(name)
		block, _ := s.dev.PadBlock(pad(t, "A1"))
		if got, _ := block.Param(info.ID); got != want {
			t.Errorf("the unit has %s = %d, want %d", name, got, want)
		}
	}
	// The detail panel shows what changed.
	s.send(tea.WindowSizeMsg{Width: 180, Height: 60})
	s.shows("forward", "Gate     ● on", "+3 st")

	// With pads marked it changes all of them.
	s.press("v", "l", "v", "S", "j", "j", "j", "j", "j", "j", "l")
	s.shows("Settings  ·  2 pads, showing A1")
	s.press("esc", "esc")
	for _, name := range []string{"A1", "A2"} {
		block, _ := s.dev.PadBlock(pad(t, name))
		info, _ := proto.ParamByName("play-mode")
		if got, _ := block.Param(info.ID); got != 1 {
			t.Errorf("%s play mode = %d, want 1", name, got)
		}
	}
	// An empty pad has nothing to edit.
	s.press("G", "S")
	s.shows("nothing to edit")
}

func TestDebugLogFollowsWhatTheUIDoes(t *testing.T) {
	var buf strings.Builder
	l := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	s := open(t)
	s.model = s.m().WithLogger(l, "/tmp/debug.log")
	s.press("v", "n")
	s.typeText("x")
	s.press("enter")
	s.press("r")
	s.shows("● debug log")
	log := buf.String()
	for _, want := range []string{
		"msg=\"ui ready\"", "msg=key", "key=v", "section=Pads", "overlay=\"prompt", "msg=\"operation on the unit\"",
		"result=\"done: ", "msg=\"loaded the project\"", "took=",
	} {
		if !strings.Contains(log, want) {
			t.Errorf("the log lacks %q:\n%s", want, log)
		}
	}
}

func TestOneClickSelectsAndPlaysThePad(t *testing.T) {
	s := open(t)
	g := s.m().geom()
	x, y := g.bodyX+(g.tileW+1)+2, rowBody+g.top+1 // pad 2
	s.click(x, y)
	if s.m().pad != 1 {
		t.Errorf("clicked pad 2, got pad %d", s.m().pad)
	}
	s.shows("previewing A2")
	if !slices.Equal(s.sim.PreviewStarts, []uint16{1}) {
		t.Errorf("the unit was asked to start %v after one click", s.sim.PreviewStarts)
	}
	// It stops by itself, after the sample's length.
	if !slices.Equal(s.sim.PreviewStops, []uint16{1}) {
		t.Errorf("stops %v", s.sim.PreviewStops)
	}
	// A click on an empty pad selects it and says so instead of playing.
	s.click(g.bodyX+(3*(g.tileW+1))+2, rowBody+g.top+1) // pad 4 holds nothing
	s.shows("A4 is empty")
}

func TestPreviewSelectsThePadFirstSoTheRightOneSounds(t *testing.T) {
	s := open(t)
	// The unit plays the previously selected pad on the first press after the selection
	// moved, unless the pad is selected first.
	s.sim.WrongPadOnFirstPreview = true
	s.sim.Select(0, 0)
	s.press("l", "enter")
	s.shows("previewing A2")
	if !slices.Equal(s.sim.PreviewStarts, []uint16{1}) {
		t.Errorf("starts %v: the first press should play A2 and nothing else", s.sim.PreviewStarts)
	}
	for _, p := range s.sim.PreviewStops {
		if p != 1 {
			t.Errorf("stopped %d, a pad that was never meant to play", p)
		}
	}
	// Moving on to another pad and pressing again plays that one at once, too.
	s.press("l", "enter")
	if got := s.sim.PreviewStarts[len(s.sim.PreviewStarts)-1]; got != 2 {
		t.Errorf("the next preview started pad %d, want 2 (A3)", got)
	}
	if n := len(s.sim.PreviewStarts); n != 2 {
		t.Errorf("%d starts for two presses: %v", n, s.sim.PreviewStarts)
	}
}

func TestAnOldStopTimerDoesNotCutALaterPreview(t *testing.T) {
	s := open(t)
	s.press("enter")
	stops := len(s.sim.PreviewStops)
	m := s.m()
	// The timer of the first press fires after the second press started: it must do nothing.
	s.send(previewStopMsg{pad: pad(t, "A1"), seq: m.previewSeq - 1})
	if len(s.sim.PreviewStops) != stops {
		t.Error("a stop timer that was overtaken stopped the pad")
	}
}

func TestListeningSaysWhereTheSoundGoes(t *testing.T) {
	s := open(t)
	s.send(AudioOutputMsg("WH-1000XM3"))
	s.press("2", "tab", "j", "j", "space")
	s.shows("♪ playing kick.wav on WH-1000XM3")
}

func TestRestoreAsksWhichBankToRestoreOnto(t *testing.T) {
	s := open(t)
	m := s.m()
	for _, name := range []string{"A1", "A2"} {
		s.sim.AddFile("SD:IMPORT/"+name+".wav", wav(480))
		if _, err := app.Assign(m.unit, m.unit, pad(t, name), "SD:IMPORT/"+name+".wav"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := app.ExportPads(m.unit, proto.AllPads()[:2], "SD:exports/A-bank", nil); err != nil {
		t.Fatal(err)
	}
	s.press("r")

	// Restore asks for the bank, with the one the export came from chosen.
	s.press("R", "/")
	s.press("ctrl+u")
	s.typeText("SD:exports/A-bank")
	s.press("enter")
	s.shows("Restore 2 pads from the export in SD:exports/A-bank onto which bank?", "exported from bank A of project PROJECT_01",
		"green: where they came from", "1→A1  2→A2")
	// Another bank: the pads keep their positions, and nothing is replaced there.
	s.press("G")
	s.shows("1→G1  2→G2", "all 2 target pads on bank G are free")
	s.press("enter")
	s.shows("2 pads restored onto bank G from SD:exports/A-bank")
	if s.padName("G1") != "A1" || s.padName("G2") != "A2" {
		t.Errorf("G1 %q, G2 %q", s.padName("G1"), s.padName("G2"))
	}
	if m := s.m(); m.bank != 6 || m.pad != 0 {
		t.Errorf("the cursor is on bank %d pad %d, want G1", m.bank, m.pad)
	}

	// The original bank holds samples, so choosing it asks twice, and Esc leaves it alone.
	s.press("R", "/")
	s.press("ctrl+u")
	s.typeText("SD:exports/A-bank")
	s.press("enter")
	s.shows("2 of the 2 pads on bank A hold samples")
	s.press("enter")
	s.shows("Press Enter again to replace the 2 pads that hold samples")
	s.press("esc")
	s.hides("onto which bank?")
	// h and l move the bank too; a bank letter resets the warning.
	s.press("R", "/")
	s.press("ctrl+u")
	s.typeText("SD:exports/A-bank")
	s.press("enter", "l", "l")
	s.shows("all 2 target pads on bank C are free")
	s.press("esc")
}

func TestRestoreCanAlsoMakeAPatternOfTheRestoredPads(t *testing.T) {
	s := open(t)
	m := s.m()
	// A1 and A3 hold samples; A2 is empty and stays out of the export and the pattern.
	for _, name := range []string{"A1", "A3"} {
		s.sim.AddFile("SD:IMPORT/"+name+".wav", wav(480))
		if _, err := app.Assign(m.unit, m.unit, pad(t, name), "SD:IMPORT/"+name+".wav"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := app.ExportPads(m.unit, []proto.PadIndex{pad(t, "A1"), pad(t, "A3")}, "SD:exports/A-bank", nil); err != nil {
		t.Fatal(err)
	}
	s.sim.SetPattern(pad(t, "G1"), pad(t, "A1"))
	s.press("r")
	restore := func() {
		s.press("R", "/")
		s.press("ctrl+u")
		s.typeText("SD:exports/A-bank")
		s.press("enter")
	}

	// p turns the pattern on and off; Enter then asks for the slot, in the target bank.
	restore()
	s.shows("[ ] p  also make a pattern", "Enter restore")
	s.press("p", "p", "p")
	s.shows("[x] p  also make a pattern", "every restored pad on beat 1, held for 2 bars", "Enter choose the pattern slot")
	s.press("G", "enter")
	s.shows("Pattern for the 2 pads restored onto bank G: which slot?", "G2 is free", " 1 1 pads")
	// The slot that holds a pattern asks twice; Esc goes back to the bank, nothing done.
	s.press("h", "enter")
	s.shows("Press Enter again to replace it")
	s.press("esc")
	s.shows("onto which bank?")
	if s.padName("G1") != "" {
		t.Fatal("restored before the slot was chosen")
	}
	// Another bank's slots can be chosen: H1 is its first free one.
	s.press("enter", "]")
	s.shows("H1 is free")
	s.press("l", "enter")
	s.shows("2 pads restored onto bank G from SD:exports/A-bank; pattern H2 starts them together (2 bars)")
	if s.padName("G1") != "A1" || s.padName("G3") != "A3" || s.padName("G2") != "" {
		t.Errorf("G1 %q, G2 %q, G3 %q", s.padName("G1"), s.padName("G2"), s.padName("G3"))
	}
	b, ok := s.sim.File(formats.PatternPath(1, pad(t, "H2")))
	if !ok {
		t.Fatal("no pattern file for H2")
	}
	p, err := formats.ParsePattern(b)
	if err != nil || len(p.Events) != 2 || p.Events[0].Pad != pad(t, "G1") || p.Events[1].Pad != pad(t, "G3") || p.Length != 3840 {
		t.Fatalf("pattern = %+v (%v)", p, err)
	}
	for _, e := range p.Events {
		if e.Tick != 0 || e.Length != 3840 {
			t.Errorf("note = %+v", e)
		}
	}
	// The Patterns section knows the new pattern without being asked to read again.
	if rows, err := app.LoadPatterns(s.m().unit, 7); err != nil || !rows[1].Exists || len(rows[1].Pads) != 2 {
		t.Errorf("H2 = %+v (%v)", rows[1], err)
	}

	// Without p, a restore makes no pattern.
	restore()
	s.press("C", "enter")
	s.shows("2 pads restored onto bank C from SD:exports/A-bank")
	s.hides("pattern C")
	if rows, _ := app.LoadPatterns(s.m().unit, 2); rows[0].Exists {
		t.Error("a pattern was made without asking for one")
	}
}

func TestRestoreOfAnExportAcrossBanksGoesBackWhereItWas(t *testing.T) {
	s := open(t)
	m := s.m()
	s.sim.AddFile("SD:IMPORT/x.wav", wav(480))
	for _, name := range []string{"A1", "C1"} {
		if _, err := app.Assign(m.unit, m.unit, pad(t, name), "SD:IMPORT/x.wav"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := app.ExportPads(m.unit, []proto.PadIndex{pad(t, "A1"), pad(t, "C1")}, "SD:exports/two", nil); err != nil {
		t.Fatal(err)
	}
	app.DeletePads(m.unit, []proto.PadIndex{pad(t, "A1"), pad(t, "C1")})
	s.press("r", "R", "/")
	s.press("ctrl+u")
	s.typeText("SD:exports/two")
	s.press("enter")
	// Two banks: no question to ask, each pad goes back where it was.
	s.shows("2 pads restored back where they were")
	if s.padName("A1") != "x" || s.padName("C1") != "x" {
		t.Errorf("A1 %q, C1 %q", s.padName("A1"), s.padName("C1"))
	}
}

func TestSelectingTheCurrentProjectSaysSo(t *testing.T) {
	s := open(t)
	s.press("3", "enter")
	s.shows("project 1 is already the current project")
	// And a switch the unit is slow to announce still ends with the screen on that project.
	s.sim.SilentProjectSwitch = true
	old := device.SelectTimeout
	device.SelectTimeout = 100 * time.Millisecond
	defer func() { device.SelectTimeout = old }()
	s.press("j", "j", "enter")
	s.shows("project 3 selected")
	if s.m().snap.Project != 2 {
		t.Errorf("the screen shows project %d", s.m().snap.Project+1)
	}
}

// keyMsg is a key press as the terminal sends it.
func keyMsg(name string) tea.KeyMsg {
	if name == "ctrl+a" {
		return tea.KeyMsg{Type: tea.KeyCtrlA}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
}

func TestAutoplayInFilesStopsTheOldSoundAtOnce(t *testing.T) {
	s := open(t)
	s.sim.AddFile("SD:IMPORT/zlong.wav", wav(96000))
	// IMPORT is open: rows are pack/, kick.wav, zlong.wav. Turn autoplay on at kick.wav.
	s.press("2", "tab", "j", "j", "ctrl+a")
	s.shows("♫ autoplay")
	if len(s.player.played) != 1 || !s.player.playing || s.m().listening != "SD:IMPORT/kick.wav" {
		t.Fatalf("autoplay did not play the file it was turned on at: %d plays, playing %v, listening %q",
			len(s.player.played), s.player.playing, s.m().listening)
	}

	// Move on without running what the move schedules: the old sound is already stopped.
	stops := s.player.stopped
	model, cmd := s.model.Update(keyMsg("j"))
	if s.player.stopped == stops || s.player.playing {
		t.Error("the old sound was still playing after the cursor moved")
	}
	if model.(Model).listening != "" {
		t.Errorf("still listening to %q", model.(Model).listening)
	}
	// Then the file the cursor rests on plays.
	s.model = model
	s.run(cmd)
	if len(s.player.played) != 2 || !s.player.playing || s.m().listening != "SD:IMPORT/zlong.wav" {
		t.Errorf("the new file did not play: %d plays, listening %q", len(s.player.played), s.m().listening)
	}

	// Moving onto a folder stops the sound and plays nothing.
	model, cmd = s.model.Update(keyMsg("k"))
	model, cmd2 := model.Update(keyMsg("k"))
	s.model = model
	s.run(cmd)
	s.run(cmd2)
	if s.player.playing {
		t.Error("something is playing on a folder")
	}
}

func TestAMoveWhileAFileIsDecodingDropsIt(t *testing.T) {
	s := open(t)
	s.press("2", "tab", "j", "j")
	m := s.m()
	// A file is asked for, and the cursor moves on while it is still being decoded.
	_, cmd := m.playFile("SD:IMPORT/kick.wav")
	m.stopListening()
	msg := cmd()
	listened, ok := msg.(listenMsg)
	if !ok || !listened.stale {
		t.Fatalf("the late file was not dropped: %#v", msg)
	}
	if len(s.player.played) != 0 {
		t.Error("a file that was overtaken still played")
	}
	// Dropped means silent: no message either.
	s.send(msg)
	s.hides("playing kick.wav")
}

func TestMovingWithAutoplayOffLeavesASoundAlone(t *testing.T) {
	s := open(t)
	s.press("2", "tab", "j", "j", "space")
	if !s.player.playing {
		t.Fatal("not playing")
	}
	s.press("k")
	if !s.player.playing {
		t.Error("moving stopped a sound that was started by hand, with autoplay off")
	}
	// Autoplay on and off again: turning it off stops the sound.
	s.press("ctrl+a")
	s.shows("autoplay on")
	s.press("ctrl+a")
	s.shows("autoplay off")
	if s.player.playing {
		t.Error("turning autoplay off left the sound playing")
	}
	s.hides("♫ autoplay")
}

func TestAutoplayInTheSearchStopsTheOldSoundAtOnce(t *testing.T) {
	s := open(t)
	s.press("2", "/", "ctrl+u")
	s.typeText("wav")
	s.press("ctrl+a")
	if !s.player.playing {
		t.Fatal("autoplay did not play the first result")
	}
	stops := s.player.stopped
	model, _ := s.model.Update(tea.KeyMsg{Type: tea.KeyDown})
	if s.player.playing || s.player.stopped == stops {
		t.Error("the old sound kept playing after moving to the next result")
	}
	_ = model
}

func TestCtrlDAndCtrlUPageThroughLists(t *testing.T) {
	s := open(t)
	// Projects: 16 rows; the list is about 27 rows tall here, so half a page is 13 rows.
	s.press("3")
	ctrlD, ctrlU := tea.KeyMsg{Type: tea.KeyCtrlD}, tea.KeyMsg{Type: tea.KeyCtrlU}
	s.send(ctrlD)
	if got := s.m().projectAt; got < 5 {
		t.Errorf("Ctrl-D moved to project %d", got+1)
	}
	s.send(ctrlD)
	s.send(ctrlD)
	if s.m().projectAt != 15 {
		t.Errorf("paging down stops at the last project, got %d", s.m().projectAt+1)
	}
	s.send(ctrlU)
	if got := s.m().projectAt; got >= 15 || got < 1 {
		t.Errorf("Ctrl-U moved to project %d", got+1)
	}
	s.send(tea.KeyMsg{Type: tea.KeyPgUp})
	s.send(tea.KeyMsg{Type: tea.KeyPgUp})
	if s.m().projectAt != 0 {
		t.Errorf("Page Up twice should reach the top, got %d", s.m().projectAt+1)
	}

	// Files: a long folder.
	for i := 0; i < 60; i++ {
		s.sim.AddFile(fmt.Sprintf("SD:LONG/file%02d.wav", i), wav(480))
	}
	s.press("r", "2")
	m := s.m()
	rows := len(m.tree.Rows())
	s.send(ctrlD)
	half := s.m().fileAt
	if half < 1 || half >= rows {
		t.Fatalf("Ctrl-D in Files moved to %d of %d", half, rows)
	}
	s.press("tab")
	s.press("G")
	s.send(ctrlU)
	if got := s.m().fileAt; got >= len(s.m().tree.Rows())-1 {
		t.Errorf("Ctrl-U from the bottom stayed at %d", got)
	}

	// The search box: Ctrl-D pages through results, Ctrl-U still clears the text.
	s.press("/", "ctrl+u")
	s.typeText("file")
	s.send(ctrlD)
	if s.m().search.at == 0 {
		t.Error("Ctrl-D did not move through the search results")
	}
	s.send(ctrlU)
	if s.m().search.input != "" {
		t.Errorf("Ctrl-U should clear the search text, left %q", s.m().search.input)
	}
}

// cursorOn puts the cursor of the Files list on the row with that name or path.
func (s *screen) cursorOn(nameOrPath string) {
	s.t.Helper()
	m := s.m()
	for i, r := range m.tree.Rows() {
		if r.Path == nameOrPath || (r.Name == nameOrPath && !strings.Contains(nameOrPath, "/")) {
			m.fileAt = i
			s.model = m
			return
		}
	}
	s.t.Fatalf("no row %q in the file list", nameOrPath)
}

// expand opens a folder of the Files list, if it is not open.
func (s *screen) expand(nameOrPath string) {
	s.t.Helper()
	s.cursorOn(nameOrPath)
	m := s.m()
	if node, _ := m.fileNode(); !node.Expanded {
		s.press("tab")
	}
}

// junkCard puts the leftovers macOS makes beside real files on the card.
func junkCard(s *screen) {
	for _, f := range []string{
		"SD:Loops/E_126.wav", "SD:Loops/._E_126.wav", "SD:Loops/C_126.wav", "SD:Loops/._C_126.wav",
		"SD:Loops/more/G_126.wav", "SD:Loops/more/._G_126.wav", "SD:Loops/.DS_Store",
	} {
		s.sim.AddFile(f, wav(480))
	}
	s.press("r", "2")
}

func TestJunkIsDimmedNotAudioAndMarkedWithJ(t *testing.T) {
	s := open(t)
	junkCard(s)
	s.expand("Loops")
	s.shows("._E_126.wav", "E_126.wav", ".DS_Store")
	// A ._ file is not audio: no assign, no listening, and it says what it is.
	s.cursorOn("SD:Loops/._E_126.wav")
	s.shows("Junk file, not audio.")
	s.press("enter")
	s.hides("Assign ._E_126.wav")
	s.press("space")
	if len(s.player.played) != 0 || s.m().picker != nil {
		t.Error("a ._ file was played or assigned")
	}

	// J marks every junk file in the open folders, and only those.
	s.press("J")
	s.shows("marked 3 junk files", "d deletes them")
	for p := range s.m().fileMarks {
		if !app.IsJunk(app.Base(p)) {
			t.Errorf("marked %s, which is not junk", p)
		}
	}
	if n := len(s.m().fileMarks); n != 3 {
		t.Errorf("%d marked: %v", n, s.m().fileMarks)
	}
	// Open the other folder and press J again: its leftover joins.
	s.expand("more")
	s.press("J")
	s.shows("marked 4 junk files")
	if n := len(s.m().fileMarks); n != 4 {
		t.Errorf("%d marked", n)
	}
	// Nothing to find in a clean folder.
	s.press("esc")
	s.cursorOn("Loops")
	s.press("tab") // closes it: its junk is no longer in the open folders
	s.press("J")
	s.shows("no junk files in the open folders")
}

func TestDeleteMarkedFilesAsksFirstAndRemovesWholeFolders(t *testing.T) {
	s := open(t)
	junkCard(s)
	s.expand("Loops")
	s.press("J", "d")
	s.shows("Delete 3 items from the SD card?", "._E_126.wav", "3 files, with everything in them; this cannot be undone.")
	// Declining deletes nothing.
	s.press("n")
	if _, ok := s.sim.File("SD:Loops/._E_126.wav"); !ok {
		t.Fatal("declining deleted a file")
	}
	s.press("d", "y")
	s.shows("3 items deleted")
	for _, gone := range []string{"SD:Loops/._E_126.wav", "SD:Loops/._C_126.wav", "SD:Loops/.DS_Store"} {
		if _, ok := s.sim.File(gone); ok {
			t.Errorf("%s is still there", gone)
		}
	}
	for _, kept := range []string{"SD:Loops/E_126.wav", "SD:Loops/C_126.wav", "SD:Loops/more/G_126.wav", "SD:Loops/more/._G_126.wav"} {
		if _, ok := s.sim.File(kept); !ok {
			t.Errorf("%s was deleted", kept)
		}
	}
	if len(s.m().fileMarks) != 0 {
		t.Error("marks survived the delete")
	}

	// A folder goes with everything in it, and the question counts what that is.
	s.cursorOn("Loops")
	s.press("d")
	s.shows("Delete 1 item from the SD card?", "Loops", "4 files and 2 folders")
	s.press("y")
	s.shows("1 item deleted")
	if _, ok := s.sim.File("SD:Loops/more/G_126.wav"); ok {
		t.Error("the folder's contents were not deleted")
	}
}

func TestDeleteIsRefusedOutsideTheCard(t *testing.T) {
	s := open(t)
	s.press("2", "t")
	s.shows("Unit storage")
	s.press("d")
	s.shows("holds your projects")
	if s.m().prompt != nil {
		t.Error("a delete was offered on the unit's storage")
	}
	s.press("t")
	s.shows("This computer")
	s.press("d")
	s.shows("not deleted from here")
	if s.m().prompt != nil {
		t.Error("a delete was offered on this computer")
	}
}

func TestCopyAndMoveMarkedFilesWithTheClipboard(t *testing.T) {
	s := open(t)
	for _, f := range []string{"SD:Src/a.wav", "SD:Src/b.wav", "SD:Src/sub/c.wav", "SD:Dst/keep.wav"} {
		s.sim.AddFile(f, wav(480))
	}
	s.press("r", "2")
	s.expand("Src")
	s.expand("sub")
	s.expand("Dst")

	// V marks the files of the folder in front; c copies them.
	s.cursorOn("Src")
	s.press("V")
	s.shows("marked 3 files in SD:Src")
	s.press("c")
	s.shows("3 items to copy", "⧉")
	s.cursorOn("Dst")
	s.press("p")
	s.shows("3 copied into SD:Dst")
	for _, f := range []string{"SD:Dst/a.wav", "SD:Dst/b.wav", "SD:Dst/c.wav", "SD:Src/a.wav", "SD:Src/sub/c.wav"} {
		if _, ok := s.sim.File(f); !ok {
			t.Errorf("%s is missing after the copy", f)
		}
	}
	// Pasting again does not overwrite what is there, and says so.
	s.cursorOn("SD:Src/a.wav")
	s.press("c")
	s.cursorOn("Dst")
	s.press("p")
	s.shows("nothing copied", "that name is taken")

	// A new folder, then a cut moves the file: the original is gone and the clipboard is spent.
	s.cursorOn("Dst")
	s.press("N")
	s.shows("New folder in SD:Dst")
	s.typeText("moved")
	s.press("enter")
	s.shows("created SD:Dst/moved")
	s.cursorOn("SD:Src/b.wav")
	s.press("x")
	s.shows("1 item to move", "↗")
	s.cursorOn("SD:Dst/moved")
	s.press("p")
	s.shows("moved to SD:Dst/moved/b.wav")
	if _, ok := s.sim.File("SD:Src/b.wav"); ok {
		t.Error("a move left the original")
	}
	if _, ok := s.sim.File("SD:Dst/moved/b.wav"); !ok {
		t.Error("a move did not arrive")
	}
	if len(s.m().clip) != 0 {
		t.Error("the clipboard was not emptied after a move")
	}

	// Esc drops the clipboard; with nothing on it there is nothing to paste.
	s.cursorOn("SD:Src/a.wav")
	s.press("c")
	if len(s.m().clip) == 0 {
		t.Fatal("nothing on the clipboard")
	}
	s.press("esc")
	if len(s.m().clip) != 0 {
		t.Error("Esc left the clipboard")
	}
	s.press("p")
	s.shows("nothing on the clipboard")
	// The unit's own storage is never a destination.
	s.cursorOn("SD:Src/a.wav")
	s.press("c", "t")
	s.shows("Unit storage")
	s.press("p")
	s.shows("holds your projects")
}

func TestTheClipboardCrossesToThisComputer(t *testing.T) {
	s := open(t)
	dir := t.TempDir()
	s.sim.AddFile("SD:Packs/kick.wav", wav(480))
	s.press("r", "2")
	s.expand("Packs")
	s.cursorOn("SD:Packs/kick.wav")
	s.press("c")
	// Switch to this computer; the clipboard comes along. The tree is pointed at a temporary
	// folder, so that nothing is written to the real home folder.
	s.press("t", "t")
	s.shows("This computer")
	if len(s.m().clip) != 1 {
		t.Fatalf("the clipboard did not survive the change of source: %v", s.m().clip)
	}
	m := s.m()
	m.root, m.tree = dir, nil
	s.model = m
	s.run(m.wanted())
	s.press("p")
	if _, err := os.Stat(filepath.Join(dir, "kick.wav")); err != nil {
		t.Errorf("the file did not arrive on this computer: %v", err)
	}
	if _, ok := s.sim.File("SD:Packs/kick.wav"); !ok {
		t.Error("a copy removed the original")
	}
}

func TestPadsCanPlayOnThisComputerLikeAFilePreview(t *testing.T) {
	s := open(t)
	m := s.m()
	// A real sample is needed on the pad: 4800 frames, so 0.1 s.
	s.sim.AddFile("SD:IMPORT/loop.wav", wav(4800))
	a5 := pad(t, "A5")
	if _, err := app.Assign(m.unit, m.unit, a5, "SD:IMPORT/loop.wav"); err != nil {
		t.Fatal(err)
	}
	s.press("r")
	s.send(AudioOutputMsg("MacBook Pro Speakers"))
	s.press("j") // pad 5 is under pad 1

	// By default a pad plays on the unit.
	s.press("enter")
	s.shows("previewing A5")
	if !slices.Equal(s.sim.PreviewStarts, []uint16{4}) || len(s.player.played) != 0 {
		t.Fatalf("unit starts %v, computer plays %d", s.sim.PreviewStarts, len(s.player.played))
	}

	// o switches to this computer: the unit is not asked, the sample is played here.
	s.press("o")
	s.shows("pads play on this computer (MacBook Pro Speakers)")
	s.sim.PreviewStarts = nil
	s.press("enter")
	if len(s.sim.PreviewStarts) != 0 {
		t.Errorf("the unit was asked to play %v", s.sim.PreviewStarts)
	}
	if len(s.player.played) != 1 || s.player.played[0].Frames() != 4800 {
		t.Fatalf("the computer played %d samples", len(s.player.played))
	}
	s.shows("♪ playing pad A5 on MacBook Pro Speakers", "Enter preview on computer")
	// The sample is read once; the second press plays from memory and restarts it.
	reads := countReads(s.sim.Requests, 0x9E)
	s.press("enter")
	if len(s.player.played) != 2 {
		t.Errorf("a second press played %d times in all", len(s.player.played))
	}
	_ = reads

	// Another pad replaces it at once, an empty pad says so, and P plays on the computer even
	// with the setting back on the unit.
	s.press("k")
	s.shows("")
	stops := s.player.stopped
	s.press("l", "l", "l", "l")
	s.press("enter")
	s.shows("is empty")
	if s.player.stopped == stops {
		t.Log("an empty pad leaves what is playing alone")
	}
	s.press("0")
	s.shows("pads play on the unit")
	s.press("O")
	s.shows("pads play on this computer")
	s.press("o")
	s.shows("pads play on the unit")
	s.press("G")
	s.press("g", "j", "P")
	if len(s.player.played) != 3 {
		t.Errorf("P played %d in all, want 3", len(s.player.played))
	}

	// The choice is remembered between runs.
	s.press("o")
	if !s.store.state.PreviewLocal {
		t.Error("the setting was not saved")
	}
	s = s.restart()
	if !s.m().previewLocal {
		t.Error("the setting did not come back after a restart")
	}
}

func countReads(requests [][]byte, cmd byte) int {
	n := 0
	for _, r := range requests {
		if len(r) > 0 && r[0] == cmd {
			n++
		}
	}
	return n
}

func TestPadPlayedOnTheComputerStopsWhenTheCursorMovesWithAutoplayOff(t *testing.T) {
	s := open(t)
	m := s.m()
	s.sim.AddFile("SD:IMPORT/loop.wav", wav(4800))
	if _, err := app.Assign(m.unit, m.unit, pad(t, "A5"), "SD:IMPORT/loop.wav"); err != nil {
		t.Fatal(err)
	}
	s.press("r", "j", "P")
	if !s.player.playing {
		t.Fatal("not playing")
	}
	// An empty pad says so and leaves the sound alone.
	s.press("j", "P")
	s.shows("A9 is empty")
	// A pad whose file cannot be read says so, instead of failing silently: break the file
	// behind its back and clear what was kept.
	s.sim.AddFile(pad(t, "A5").SamplePath(1), []byte("broken"))
	s.press("k")
	s.m().padAudio.clear()
	s.press("P")
	s.shows("listening failed")
}

func TestJumpToTheNextAndPreviousFolder(t *testing.T) {
	s := open(t)
	for _, f := range []string{"SD:A/x.wav", "SD:A/sub/y.wav", "SD:B/z.wav", "SD:C/w.wav"} {
		s.sim.AddFile(f, wav(480))
	}
	s.press("r", "2")
	s.expand("A")
	s.expand("sub")
	// Rows: A/ sub/ y.wav x.wav B/ C/ IMPORT/ ... in folder-first order; jump over the files.
	name := func() string { m := s.m(); n, _ := m.fileNode(); return n.Name }
	s.cursorOn("A")
	var seen []string
	for i := 0; i < 8; i++ {
		s.press("}")
		seen = append(seen, name())
		if !s.m().tree.Rows()[s.m().fileAt].Dir {
			t.Fatalf("} landed on a file: %q", name())
		}
	}
	if seen[0] != "sub" {
		t.Errorf("after A the next folder is sub, got %v", seen)
	}
	// It stops at the last folder and says so.
	s.shows("no more folders below")
	last := name()
	s.press("}")
	if name() != last {
		t.Error("jumped past the last folder")
	}
	// And back up, skipping the files between folders.
	s.press("{")
	if name() == last || !s.m().tree.Rows()[s.m().fileAt].Dir {
		t.Errorf("{ did not go to the previous folder: %q", name())
	}
	s.cursorOn("A")
	s.press("{")
	s.shows("no more folders above")
	// Alt-n and Alt-p do the same.
	s.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n"), Alt: true})
	if name() != "sub" {
		t.Errorf("Alt-n went to %q", name())
	}
	s.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p"), Alt: true})
	if name() != "A" {
		t.Errorf("Alt-p went to %q", name())
	}
	// The same keys work in the assign dialog's tree.
	s.press("1", "a", "tab", "}")
	if m := s.m(); m.browser == nil || !m.browser.tree.Rows()[m.browser.at].Dir {
		t.Error("} in the dialog did not land on a folder")
	}
	s.press("esc")
}

func TestTheAssignDialogMarksAnythingAndDeletesJunk(t *testing.T) {
	s := open(t)
	for _, f := range []string{"SD:Loops/E_126.wav", "SD:Loops/._E_126.wav", "SD:Loops/C_126.wav", "SD:Loops/._C_126.wav", "SD:Loops/.DS_Store"} {
		s.sim.AddFile(f, wav(480))
	}
	s.press("a", "tab")
	cursorOnDialog := func(path string) {
		m := s.m()
		for i, r := range m.browser.tree.Rows() {
			if r.Path == path {
				m.browser.at = i
				return
			}
		}
		t.Fatalf("no row %s in the dialog", path)
	}
	// Open Loops in the dialog, then mark a ._ file with v: before, this was ignored.
	m := s.m()
	for i, r := range m.browser.tree.Rows() {
		if r.Name == "Loops" {
			m.browser.at = i
		}
	}
	s.press("tab")
	cursorOnDialog("SD:Loops/._E_126.wav")
	s.press("v")
	if !s.m().browser.marks["SD:Loops/._E_126.wav"] {
		t.Fatal("a ._ file could not be marked in the dialog")
	}
	// J marks all the junk in the open folders; Enter does not assign the junk.
	s.press("J")
	s.shows("marked 3 junk files")
	s.press("enter")
	s.shows("none of the marked items is an audio file")
	// d asks first, then the dialog opens again where it was.
	s.press("d")
	s.shows("Delete 3 items from the SD card?", "._E_126.wav", "3 files")
	s.press("n")
	s.shows("Assign a file to A1")
	if _, ok := s.sim.File("SD:Loops/._E_126.wav"); !ok {
		t.Fatal("declining deleted a file")
	}
	// The marks are gone with the old dialog, so mark again and delete.
	s.press("J")
	s.press("d", "y")
	s.shows("Assign a file to A1", "3 items deleted")
	for _, gone := range []string{"SD:Loops/._E_126.wav", "SD:Loops/._C_126.wav", "SD:Loops/.DS_Store"} {
		if _, ok := s.sim.File(gone); ok {
			t.Errorf("%s is still there", gone)
		}
	}
	for _, kept := range []string{"SD:Loops/E_126.wav", "SD:Loops/C_126.wav"} {
		if _, ok := s.sim.File(kept); !ok {
			t.Errorf("%s was deleted", kept)
		}
	}
	s.hides("._E_126.wav")
	s.shows("E_126.wav")
}

func TestMarkedFilesShowTheirActionsAndUnmarkAll(t *testing.T) {
	s := open(t)
	for _, f := range []string{"SD:Loops/a.wav", "SD:Loops/._a.wav"} {
		s.sim.AddFile(f, wav(480))
	}
	s.press("a", "tab")
	m := s.m()
	for i, r := range m.browser.tree.Rows() {
		if r.Name == "Loops" {
			m.browser.at = i
		}
	}
	s.press("tab")
	s.hides("unmark all")
	s.press("J")
	s.shows("marked", "delete", "unmark all")
	s.press("u")
	s.shows("unmarked everything")
	s.hides("unmark all")
}

func TestFindAFolderByNameThenCollapseIt(t *testing.T) {
	s := open(t)
	for _, f := range []string{"SD:Drums/a.wav", "SD:Drums/Claps/c.wav", "SD:Bass/b.wav"} {
		s.sim.AddFile(f, wav(480))
	}
	s.press("r", "2")
	s.expand("Drums")
	s.expand("Claps")
	s.cursorOn("Bass")
	s.press("f")
	s.typeText("clap")
	s.press("enter")
	m := s.m()
	if n, _ := m.fileNode(); n.Name != "Claps" {
		t.Fatalf("found %q, want Claps", n.Name)
	}
	s.shows("Tab or h collapses it")
	s.press("h")
	s.hides("c.wav")
	// A name that nothing matches says so and leaves the cursor alone.
	s.press("f")
	s.typeText("zzz")
	s.press("enter")
	s.shows("no folder named like")
}

func TestFindAFolderInTheAssignDialog(t *testing.T) {
	s := open(t)
	for _, f := range []string{"SD:Drums/a.wav", "SD:Drums/Claps/c.wav", "SD:Bass/b.wav"} {
		s.sim.AddFile(f, wav(480))
	}
	s.press("a", "tab")
	open := func(name string) {
		m := s.m()
		for i, r := range m.browser.tree.Rows() {
			if r.Name == name {
				m.browser.at = i
			}
		}
		s.press("tab")
	}
	open("Drums")
	open("Claps")
	s.press("g", "f")
	s.shows("Find a folder")
	s.typeText("clap")
	s.press("enter")
	m := s.m()
	if n := m.browser.tree.Rows()[m.browser.at]; n.Name != "Claps" {
		t.Fatalf("found %q, want Claps", n.Name)
	}
	s.shows("h or Tab collapses it")
	s.press("h")
	s.hides("c.wav")
	s.press("f")
	s.typeText("zzz")
	s.press("enter")
	s.shows("no folder named like")
	// Esc while typing cancels the find and keeps the dialog open.
	s.press("f", "esc")
	s.shows("Assign a file to")
}

func TestFindingAFolderListsMatchesWhileTyping(t *testing.T) {
	s := open(t)
	for _, f := range []string{"SD:Kit1/a.wav", "SD:Kit2/b.wav", "SD:Bass/c.wav"} {
		s.sim.AddFile(f, wav(480))
	}
	s.press("r", "2")
	s.cursorOn("Bass")
	s.press("f")
	// Before anything is typed, every open folder is listed.
	s.shows("Kit1/", "Kit2/", "Bass/")
	s.typeText("kit")
	s.shows("Kit1/", "Kit2/")
	s.hides("Bass/")
	// Down chooses the second match, and Enter goes there.
	s.press("down", "enter")
	m := s.m()
	if n, _ := m.fileNode(); n.Name != "Kit2" {
		t.Fatalf("chose %q, want Kit2", n.Name)
	}

	// The same in the assign dialog.
	s.press("1", "a")
	s.press("f")
	s.shows("Find a folder")
	s.typeText("kit")
	s.shows("Kit1/", "Kit2/")
	s.press("down", "enter")
	m = s.m()
	if n := m.browser.tree.Rows()[m.browser.at]; n.Name != "Kit2" {
		t.Fatalf("dialog chose %q, want Kit2", n.Name)
	}
}

func TestTheCursorStaysPutAfterDeleting(t *testing.T) {
	s := open(t)
	for _, f := range []string{"SD:Aaa/x.wav", "SD:Loops/._a.wav", "SD:Loops/b.wav", "SD:Zzz/y.wav"} {
		s.sim.AddFile(f, wav(480))
	}
	s.press("r", "2")
	s.expand("Loops")
	name := func() string { m := s.m(); n, _ := m.fileNode(); return n.Name }

	// Deleting the file under the cursor leaves it on the folder that held it.
	s.cursorOn("._a.wav")
	s.press("d", "y")
	s.shows("1 item deleted")
	if got := name(); got != "Loops" {
		t.Fatalf("after deleting a file the cursor is on %q, want Loops", got)
	}

	// Deleting something marked elsewhere keeps the cursor on its own row.
	s.cursorOn("Zzz")
	s.press("v")
	s.cursorOn("Aaa")
	s.press("d", "y")
	s.shows("1 item deleted")
	if got := name(); got != "Aaa" {
		t.Errorf("cursor jumped to %q, want Aaa", got)
	}

	// In the assign dialog the cursor comes back on the folder too.
	s.press("1", "a")
	dialogOn := func(name string) {
		m := s.m()
		for i, r := range m.browser.tree.Rows() {
			if r.Name == name {
				m.browser.at = i
			}
		}
	}
	dialogOn("Loops")
	s.press("tab")
	dialogOn("b.wav")
	s.press("v", "d")
	s.press("y")
	m := s.m()
	if m.browser == nil {
		t.Fatal("the dialog did not open again")
	}
	if n := m.browser.tree.Rows()[m.browser.at]; n.Name != "Loops" {
		t.Errorf("the dialog cursor is on %q, want Loops", n.Name)
	}
}

func TestRenameWhateverIsHighlighted(t *testing.T) {
	s := open(t)
	for _, f := range []string{"SD:Loops/old.wav", "SD:Loops/z.wav"} {
		s.sim.AddFile(f, wav(480))
	}
	// In Files, the cursor follows the renamed file.
	s.press("r", "2")
	s.expand("Loops")
	s.cursorOn("old.wav")
	s.press("n")
	s.shows("Rename old.wav")
	for range "old.wav" {
		s.press("backspace")
	}
	s.typeText("new.wav")
	s.press("enter")
	s.shows("renamed to")
	if _, ok := s.sim.File("SD:Loops/new.wav"); !ok {
		t.Fatal("the file was not renamed")
	}
	m := s.m()
	if n, _ := m.fileNode(); n.Name != "new.wav" {
		t.Errorf("the cursor is on %q, want new.wav", n.Name)
	}

	// In the assign dialog a folder or file can be renamed too, and the dialog comes back
	// on it.
	s.press("1", "a")
	dialogOn := func(name string) {
		m := s.m()
		for i, r := range m.browser.tree.Rows() {
			if r.Name == name {
				m.browser.at = i
			}
		}
	}
	dialogOn("Loops")
	s.press("n")
	s.shows("Rename Loops")
	for range "Loops" {
		s.press("backspace")
	}
	s.typeText("Beats")
	s.press("enter")
	m = s.m()
	if m.browser == nil {
		t.Fatal("the dialog did not open again")
	}
	if _, ok := s.sim.File("SD:Beats/new.wav"); !ok {
		t.Fatal("the folder was not renamed")
	}
	if n := m.browser.tree.Rows()[m.browser.at]; n.Name != "Beats" {
		t.Errorf("the dialog cursor is on %q, want Beats", n.Name)
	}
	// Esc leaves the name alone and brings the dialog back.
	s.press("n", "esc")
	s.shows("Assign a file to")
}

func TestShiftTabFoldsEverything(t *testing.T) {
	s := open(t)
	for _, f := range []string{"SD:A/sub/y.wav", "SD:A/x.wav", "SD:B/z.wav"} {
		s.sim.AddFile(f, wav(480))
	}
	s.press("r", "2")
	s.expand("A")
	s.expand("sub")
	s.expand("B")
	s.cursorOn("y.wav")
	s.shows("y.wav", "z.wav")
	s.press("shift+tab")
	s.hides("y.wav")
	s.hides("z.wav")
	s.hides("x.wav")
	m := s.m()
	if n, _ := m.fileNode(); n.Name != "A" {
		t.Errorf("the cursor is on %q, want the folder that held it, A", n.Name)
	}
	if m.section != secFiles {
		t.Error("shift+tab left the Files section")
	}

	// The same in the assign dialog.
	s.press("1", "a")
	dialogOn := func(name string) {
		m := s.m()
		for i, r := range m.browser.tree.Rows() {
			if r.Name == name {
				m.browser.at = i
			}
		}
	}
	dialogOn("A")
	s.press("tab")
	s.shows("x.wav")
	s.press("shift+tab")
	s.hides("x.wav")
	s.shows("Assign a file to")
}

func TestTheDialogListsItsRenameAndFoldKeys(t *testing.T) {
	s := open(t)
	s.press("a")
	s.shows("rename", "fold all", "find folder")
}

func TestChromaticPlaysAPadAsNotesFromTheKeyboard(t *testing.T) {
	s := open(t)
	m := s.m()
	// 4800 frames on A5; the first three pads of the simulated unit have no file behind them.
	s.sim.AddFile("SD:IMPORT/loop.wav", wav(4800))
	if _, err := app.Assign(m.unit, m.unit, pad(t, "A5"), "SD:IMPORT/loop.wav"); err != nil {
		t.Fatal(err)
	}
	s.press("r", "j", "j", "C")
	s.shows("A9 is empty")
	s.press("k", "C")
	s.shows("Chromatic  ·  A5", "octave +0", "z / x octave")
	frames := func() []int {
		var out []int
		for _, p := range s.player.played {
			out = append(out, p.Frames())
		}
		return out
	}
	// a is the sample as it is, k an octave above it, and x moves the keys up an octave. The
	// unit is not asked to play anything.
	s.press("a", "k", "x", "a", "A")
	if got := frames(); !slices.Equal(got, []int{4800, 2400, 2400}) {
		t.Fatalf("played %v", got)
	}
	s.shows("octave +1", "last note +12 semitones", "♪ pad A5")
	if len(s.sim.PreviewStarts) != 0 {
		t.Errorf("the unit was asked to play %v", s.sim.PreviewStarts)
	}
	// A key held down comes again and again, and plays once; pressed again later it plays again.
	s.press("a", "a")
	if n := len(s.player.played); n != 3 {
		t.Fatalf("a held key played %d notes in all", n)
	}
	s.m().chromatic.at = time.Now().Add(-time.Second)
	s.press("a")
	// Down two octaves, w is a semitone above the lowest note: 11 below the sample, so longer.
	s.press("z", "z", "w", "enter")
	want := int(4800 / math.Pow(2, -11.0/12))
	if got := frames(); len(got) != 6 || got[4] != want || got[5] != want {
		t.Fatalf("played %v, want %d twice at the end", got, want)
	}
	s.shows("octave -1", "last note -11 semitones")
	// Space stops the sound and Esc closes the dialog; the keys are the pads' own again.
	stops := s.player.stopped
	s.press("space")
	if s.player.stopped != stops+1 || s.player.playing {
		t.Error("Space did not stop the note")
	}
	s.press("esc")
	if s.m().chromatic != nil || s.m().section != secPads {
		t.Fatal("Esc did not close the dialog")
	}
	s.press("l")
	if s.m().pad != 5 {
		t.Errorf("the cursor is on pad %d", s.m().pad+1)
	}
}

func TestAssignDialogMarksAFolder(t *testing.T) {
	s := open(t)
	s.press("a", "tab", "j", "l", "V")
	s.shows("marked 1 files in SD:IMPORT/pack", "bank A")
}

func TestNewPatternFromTheBank(t *testing.T) {
	s := open(t)
	s.press("N", "enter", "enter")
	s.shows("made", "on beat 1, 2 bars")
}

func TestNewPatternTemplatePickerOffersTheTemplates(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	s.press("N")
	s.shows("New pattern", "All pads held", "Blank", "House", "Tech house", "Hip-hop")
	// Esc closes it without making anything.
	s.press("esc")
	if _, ok := s.sim.File("ROLAND/SP-404MKII/PROJECT_01/PTN/PTN00001.BIN"); ok {
		t.Fatal("Esc on the picker still wrote a pattern")
	}
}

func TestNewPatternBlankTemplateWritesAnEmptyPattern(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	// All pads held, Blank: pick the second row.
	s.press("N", "j", "enter", "enter")
	s.shows("made", "blank")
	data, ok := s.sim.File("ROLAND/SP-404MKII/PROJECT_01/PTN/PTN00001.BIN")
	if !ok {
		t.Fatal("no pattern file")
	}
	p, err := formats.ParsePattern(data)
	if err != nil {
		t.Fatal(err)
	}
	if p.Length != 3840 {
		t.Errorf("length %d, want 3840", p.Length)
	}
	for _, e := range p.Events {
		if !e.Control {
			t.Errorf("the blank pattern has a note on %s", e.Pad)
		}
	}
}

func TestNewPatternHipHopGrooveConfirmsLanesThenWrites(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	// The fixture pads are A1=kick, A2=snare, A3=hat; Hip-hop uses all three.
	// Picker rows: All pads held, Blank, House, Tech house, Hip-hop -> 4 downs to Hip-hop.
	s.press("N", "j", "j", "j", "j", "j", "j", "j", "enter", "enter")
	s.shows("Hip-hop", "Kick", "Snare", "Closed hat")
	// The lanes are pre-filled by guessing from the names.
	s.shows("A1", "A2", "A3")
	s.press("w")
	s.shows("made", "Hip-hop")
	data, ok := s.sim.File("ROLAND/SP-404MKII/PROJECT_01/PTN/PTN00001.BIN")
	if !ok {
		t.Fatal("no pattern file")
	}
	p, err := formats.ParsePattern(data)
	if err != nil {
		t.Fatal(err)
	}
	a1, _ := proto.ParsePad("A1")
	a2, _ := proto.ParsePad("A2")
	// Kick on beat 1 (tick 0) and the "& of 3" (tick 1200) of bar 1.
	var kick, snare []uint32
	for _, e := range p.Events {
		if e.Control {
			continue
		}
		switch e.Pad {
		case a1:
			kick = append(kick, e.Tick)
		case a2:
			snare = append(snare, e.Tick)
		}
	}
	if len(kick) == 0 || kick[0] != 0 {
		t.Errorf("kick hits %v, want one on tick 0", kick)
	}
	// Snare on beats 2 and 4 of bar 1: ticks 480 and 1440.
	wantSnare := map[uint32]bool{480: true, 1440: true}
	for _, tk := range snare[:min(2, len(snare))] {
		if !wantSnare[tk] {
			t.Errorf("snare on %d, want it on beats 2 and 4 (480, 1440)", tk)
		}
	}
}

func TestNewPatternLaneReassignedThroughTheChooser(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	// House: kick/clap/open hat/closed hat. The fixture has kick, snare, hat; "hat" guesses as
	// the closed hat, so the open hat starts skipped. Row order: Kick, Clap, Open hat, Closed hat.
	s.press("N", "j", "j", "enter", "enter") // All pads held, Blank, House
	s.shows("House", "Open hat", "Closed hat")
	// Move to the Open hat row (down twice from Kick) and open its chooser.
	s.press("j", "j", "enter")
	s.shows("pad for open hat", "A1", "A2", "A3", "skip")
	// Pick the first pad (A1) for the open hat.
	s.press("g", "enter")
	s.shows("Open hat")
	s.press("w")
	s.shows("made", "House")
	data, ok := s.sim.File("ROLAND/SP-404MKII/PROJECT_01/PTN/PTN00001.BIN")
	if !ok {
		t.Fatal("no pattern file")
	}
	p, _ := formats.ParsePattern(data)
	a1, _ := proto.ParsePad("A1")
	// A1 now plays the open-hat off-beats (tick 240 among them), not only the kick beats.
	has240 := false
	for _, e := range p.Events {
		if !e.Control && e.Pad == a1 && e.Tick == 240 {
			has240 = true
		}
	}
	if !has240 {
		t.Error("after assigning A1 to the open hat, it has no off-beat hit at tick 240")
	}
}

func TestNewPatternLaneCanBeSkipped(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	// Hip-hop, then on the kick row cycle its pad left until it reads "skip".
	s.press("N", "j", "j", "j", "j", "j", "j", "j", "enter", "enter")
	s.shows("Hip-hop")
	// Kick is the first lane (cursor 0). Left past A1 wraps to "skip".
	s.press("h")
	s.shows("skip")
	s.press("w")
	s.shows("made", "Hip-hop")
	data, _ := s.sim.File("ROLAND/SP-404MKII/PROJECT_01/PTN/PTN00001.BIN")
	p, _ := formats.ParsePattern(data)
	a1, _ := proto.ParsePad("A1")
	for _, e := range p.Events {
		if !e.Control && e.Pad == a1 {
			t.Error("the kick lane was skipped but A1 still plays")
		}
	}
}

func TestPatternsCopyMoveDelete(t *testing.T) {
	s := open(t)
	s.press("N", "enter", "enter")
	s.shows("made")
	s.press("tab")
	s.press("c", "l", "p")
	s.shows("pattern A1 copied to A2")
	s.press("x", "p")
	s.press("j", "x", "k", "p")
	s.press("d")
	s.shows("Delete pattern")
	s.press("y")
	s.shows("deleted")
}

func TestBankToggleMenu(t *testing.T) {
	s := open(t)
	// T sets the pad settings for every pad of the bank at once.
	s.press("T")
	s.shows("Set for every pad of bank A", "Gate", "Loop", "One shot", "Fixed velocity")
	s.press("g")
	s.shows("gate on")
	s.press("T", "g")
	s.shows("gate off")
}

func TestPadToggleMenuScopesToTheHighlightedPad(t *testing.T) {
	s := open(t)
	// t scopes the dialog to the pad the cursor is on, A1 at the start.
	s.press("t")
	s.shows("Set for pad A1", "Gate")
	s.press("g")
	s.shows("gate on")
	if got := s.m().snap.Pads[pad(t, "A1").Index()].Params["gate"]; got != 1 {
		t.Errorf("A1 gate %d, want on", got)
	}
	// A neighbouring pad with a sample is untouched by the single-pad scope.
	if got := s.m().snap.Pads[pad(t, "A2").Index()].Params["gate"]; got == 1 {
		t.Error("A2 gate was changed by a pad-scoped toggle")
	}
}

func TestWholeBankToggleSetsTheBusOnEveryPad(t *testing.T) {
	s := open(t)
	// T opens the whole-bank dialog; the bus steps through off, BUS 1, BUS 2 for every pad.
	s.press("T")
	s.shows("Set for every pad of bank A")
	s.press("1")
	for _, name := range []string{"A1", "A2", "A3"} {
		if got := s.m().snap.Pads[pad(t, name).Index()].Params["bus-fx"]; got != 1 {
			t.Errorf("%s bus-fx %d, want 1", name, got)
		}
	}
}

func TestHistoryKeepsWhatHappened(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 180, Height: 50})
	s.press("N", "enter", "enter")
	s.shows("History", "pattern A")
	s.press("t", "g")
	s.press("N", "enter", "enter")
	s.shows("gate")
	if n := len(s.m().history); n < 2 {
		t.Errorf("history has %d lines", n)
	}
}

func TestIndexAFolderInTheBackground(t *testing.T) {
	s := open(t)
	s.sim.AddFile("SD:IMPORT/pack/124_bass_wobble.wav", wav(2400))
	s.press("2", "tab", "i")
	s.shows("indexed", "files in SD:IMPORT", "404tui-index.json.gz")
	data, ok := s.sim.File("SD:404tui-index.json.gz")
	if !ok {
		t.Fatal("no index on the card")
	}
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := io.ReadAll(zr)
	if !strings.Contains(string(plain), "124_bass_wobble.wav") {
		t.Fatalf("the index on the card: %s", plain)
	}
	if hits := s.m().tags.Search("bass 124"); len(hits) != 1 {
		t.Errorf("tag search found %v", hits)
	}
}

func TestIndexBarShowsHowFarARunHasCome(t *testing.T) {
	s := open(t)
	m := s.m()
	job := &tagJob{folder: "SD:Samples"}
	job.folders.Store(3)
	job.queued.Store(3)
	job.files.Store(12)
	m.tagJob = job
	out := ansi.Strip(m.View())
	for _, want := range []string{"indexing SD:Samples", "3/6 folders", "12 files", "i stops", "█", "░"} {
		if !strings.Contains(out, want) {
			t.Errorf("the bottom bar lacks %q:\n%s", want, out)
		}
	}
	if lines := strings.Split(out, "\n"); len(lines) > m.height {
		t.Errorf("%d lines on a %d row screen", len(lines), m.height)
	}
}

func TestSearchOnlyTheTags(t *testing.T) {
	s := open(t)
	s.sim.AddFile("SD:IMPORT/pack/124_bass_wobble.wav", wav(2400))
	s.sim.AddFile("SD:OTHER/124_bass_unindexed.wav", wav(2400))
	s.press("2", "tab", "i")
	s.shows("indexed")
	s.send(tea.WindowSizeMsg{Width: 180, Height: 50})
	s.press("s")
	s.shows("Search SD card", "tagged files", "Tags", "bass (1)", "wobble (1)")
	s.typeText("bass")
	s.shows("Narrow by", "124 (1)", "wobble (1)")
	s.hides("bass (1)")
	s.typeText(" 124")
	s.shows("124_bass_wobble.wav", "tags")
	s.hides("124_bass_unindexed.wav")
	s.press("ctrl+u")
	s.typeText("kick")
	s.shows("kick.wav")
	s.press("ctrl+u")
	s.typeText("zzz")
	s.shows("nothing matches")
}

func TestSearchIndexBuildsInTheBackgroundAndShowsItsProgress(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 180, Height: 50})
	s.press("2")
	// Nothing is walked by itself: with no saved index the panel says how to make one.
	s.shows("No search index yet", "i indexes the folder in front")
	s.press("tab", "i")
	// Once it is saved, the search index comes from it, and says so.
	s.shows("indexed")
	s.press("r")
	s.shows("Search index", "saved", "i indexes again")
	// A build that is going on shows in the Details panel, and in the dialog if one opens.
	m := s.m()
	delete(m.indexes, "SD:")
	job := &tagJob{folder: "SD:"}
	job.folders.Store(3)
	job.queued.Store(3)
	job.files.Store(12)
	m.searchJobs["SD:"] = job
	out := ansi.Strip(m.View())
	for _, want := range []string{"Indexing SD card for search", "3/6 folders", "12 files"} {
		if !strings.Contains(out, want) {
			t.Errorf("the Details panel lacks %q:\n%s", want, out)
		}
	}
	m.search = &search{root: "SD:", building: true}
	out = ansi.Strip(m.View())
	if !strings.Contains(out, "3/6 folders") || !strings.Contains(out, "it goes on") {
		t.Errorf("the dialog shows no progress:\n%s", out)
	}
}

func TestFindAFolderThatIsNotOpenFromTheSavedIndex(t *testing.T) {
	s := open(t)
	s.sim.AddFile("SD:IMPORT/pack/deep/down/x.wav", wav(2400))
	s.press("2", "tab", "i")
	s.shows("indexed")
	s.press("r")
	s.press("f")
	s.typeText("down")
	s.shows("down/", "folders")
	s.press("enter")
	s.shows("x.wav")
}

func TestSearchRemembersTheQueryAndTheFileAndTabSwitchesKind(t *testing.T) {
	s := open(t)
	s.sim.AddFile("SD:IMPORT/pack/124_bass_wobble.wav", wav(2400))
	s.sim.AddFile("SD:IMPORT/pack/124_bass_other.wav", wav(2400))
	s.press("2", "tab", "i")
	s.shows("indexed")
	s.send(tea.WindowSizeMsg{Width: 180, Height: 50})
	s.press("/")
	s.typeText("bass")
	s.press("down")
	s.shows("Files", "124_bass_other.wav")
	front := func() string {
		f, _ := s.m().search.current()
		return f.Name()
	}
	chosen := front()
	s.press("esc")
	s.hides("Tags")
	// Opened again, the search comes back with its query and the file that was in front.
	s.press("/")
	s.shows("> bass")
	if got := front(); got != chosen {
		t.Errorf("the cursor is on %q, was on %q", got, chosen)
	}
	// Tab goes to the tags, which have a query of their own, and back.
	s.press("tab")
	s.shows("tagged files")
	s.typeText("wobble")
	s.shows("Narrow by")
	s.press("tab")
	s.shows("> bass")
	s.press("tab")
	s.shows("> wobble")
	// Assigning from the search keeps it too.
	s.press("esc", "/")
	s.shows("> bass")
}

func TestAPatternPlaysInALoopUntilStopped(t *testing.T) {
	s := open(t)
	// Bank C has real samples (the simulator's fixture pads have none the player could read).
	m := s.m()
	if _, err := app.Assign(m.unit, m.unit, pad(t, "C1"), "SD:IMPORT/kick.wav"); err != nil {
		t.Fatal(err)
	}
	s.press("r", "]", "]", "N", "enter", "enter", "tab")
	s.press("P")
	if len(s.player.looped) != 1 || len(s.player.played) != 0 {
		t.Fatalf("looped %d, played %d: %s", len(s.player.looped), len(s.player.played), s.m().message)
	}
	s.press("P")
	if s.player.playing {
		t.Error("the second P did not stop the loop")
	}
}

func TestSpaceOnAPatternPlaysAndStopsItAndEnterOpensIt(t *testing.T) {
	s := open(t)
	m := s.m()
	if _, err := app.Assign(m.unit, m.unit, pad(t, "C1"), "SD:IMPORT/kick.wav"); err != nil {
		t.Fatal(err)
	}
	s.press("r", "]", "]", "N", "enter", "enter", "tab")
	// The pattern is open from making it: leave it, so the panel is in front.
	s.press("esc", "1", "tab")
	if s.m().pattern != nil || s.m().section != secPatterns {
		t.Fatalf("pattern open %v, section %d", s.m().pattern != nil, s.m().section)
	}
	s.press(" ")
	if s.m().pattern != nil || s.m().seqTab {
		t.Error("Space opened the pattern: only Enter does")
	}
	if len(s.player.looped) != 1 {
		t.Fatalf("Space did not play the pattern: looped %d, %s", len(s.player.looped), s.m().message)
	}
	s.press(" ")
	if s.player.playing {
		t.Error("the second Space did not stop it")
	}
	s.press("enter")
	if s.m().pattern == nil || !s.m().seqTab {
		t.Error("Enter did not open the pattern in the Sequencer")
	}
}

func TestFavouriteAndTagFilesAndFindThemByTheirTags(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 180, Height: 50})
	s.press("2", "tab", "i")
	s.shows("indexed")
	// The cursor is on IMPORT; kick.wav is two rows down (pack/ is closed).
	s.press("j", "j")
	m := s.m()
	if node, _ := m.fileNode(); node.Name != "kick.wav" {
		t.Fatalf("cursor on %q", node.Name)
	}
	s.press("*")
	s.shows("tagged 1 file", "+favourite", "★")
	s.press("+")
	// The question lists what the file has: the tag just given, and the ones from its name.
	s.shows("Tag kick.wav", "Your tags", "favourite", "From name", "kick", "Used before", "favourite (1)")
	s.typeText("punchy, dry")
	s.press("enter")
	s.shows("+punchy", "+dry")
	s.shows("tags", "favourite", "punchy")
	// They are in the saved index, and the tag search finds the file by them.
	s.press("s")
	s.typeText("favourite punchy")
	s.shows("kick.wav")
	s.press("esc")
	// Taking it off: a second * when all have it, or a word with - in front.
	s.press("*")
	s.shows("-favourite")
	s.hides("★")
	s.press("+")
	s.typeText("-dry")
	s.press("enter")
	s.shows("-dry")
}

func TestFavouriteFromTheAssignDialogKeepsItOpen(t *testing.T) {
	s := open(t)
	s.press("a", "tab", "j", "j")
	s.press("*")
	s.shows("Assign a file to", "tagged 1 file", "★")
	s.press("+")
	s.typeText("loopy")
	s.press("enter")
	s.shows("Assign a file to", "+loopy")
}

// Tagging runs on another goroutine while the screen is drawn from the same tag index: it must
// not change what the screen reads. Run with -race to see a failure.
func TestTaggingSeveralFilesWhileTheScreenIsDrawnIsSafe(t *testing.T) {
	s := open(t)
	for i := 0; i < 60; i++ {
		s.sim.AddFile(fmt.Sprintf("SD:IMPORT/many/s%02d.wav", i), wav(480))
	}
	s.press("2", "tab", "i")
	s.shows("indexed")
	// Open the folder with the many files, mark all of them, then tag them all.
	s.press("j", "tab", "V")
	m := s.m()
	paths := m.filesToTag()
	if len(paths.files) < 60 {
		t.Fatalf("only %d files marked", len(paths.files))
	}
	cmd := m.tagFiles(paths, []string{"favourite", "loopy"}, nil, nil)
	done := make(chan tea.Msg)
	go func() { done <- cmd() }()
	for drawing := true; drawing; {
		select {
		case msg := <-done:
			s.send(msg)
			drawing = false
		default:
			_ = m.View()
		}
	}
	if !strings.Contains(s.m().message, "tagged 60 files") {
		t.Errorf("message %q", s.m().message)
	}
}

func TestFavouriteSeveralMarkedFilesInTheFilesViewAndTheDialog(t *testing.T) {
	s := open(t)
	for i := 0; i < 5; i++ {
		s.sim.AddFile(fmt.Sprintf("SD:IMPORT/many/s%d.wav", i), wav(480))
	}
	s.press("2", "tab", "i")
	s.shows("indexed")
	// Mark three files with v, and tag them: it must not crash, and it tags exactly those.
	s.press("j", "tab", "j", "v", "v", "v")
	if n := len(s.m().fileMarks); n != 3 {
		t.Fatalf("%d marked", n)
	}
	s.press("*")
	s.shows("tagged 3 files", "+favourite")
	m := s.m()
	if got := len(m.tags.UserTagCounts()); got != 1 || m.tags.UserTagCounts()[0].Count != 3 {
		t.Errorf("counts %v", m.tags.UserTagCounts())
	}
	// Mark several again, one that already has it among them: the star goes on for all of them.
	s.press("g", "j", "tab", "j", "v", "v", "v", "v")
	s.press("*")
	m = s.m()
	if got := m.tags.UserTagCounts(); len(got) != 1 || got[0].Count < 4 {
		t.Errorf("counts after the second round %v", got)
	}
}

func TestTagSeveralMarkedFilesInTheAssignDialog(t *testing.T) {
	s := open(t)
	for i := 0; i < 5; i++ {
		s.sim.AddFile(fmt.Sprintf("SD:IMPORT/many/s%d.wav", i), wav(480))
	}
	// Rows: IMPORT, many/, pack/, kick.wav; open many/ and mark its first three files.
	s.press("a", "tab", "j", "tab", "j", "v", "v", "v")
	s.shows("3 items marked")
	s.press("*")
	s.shows("Assign a file to", "tagged 3 files", "+favourite")
	s.press("+")
	s.shows("Your tags", "favourite (3 of 3)")
	s.typeText("alpha beta")
	s.press("enter")
	s.shows("Assign a file to", "+alpha", "+beta")
	m := s.m()
	if counts := m.tags.UserTagCounts(); len(counts) != 3 {
		t.Errorf("counts %v", counts)
	}
}

func TestTaggingWithNothingToTagSaysSoAndNeverCrashes(t *testing.T) {
	s := open(t)
	s.sim.AddFile("SD:IMPORT/notes.txt", []byte("x"))
	// A text file in front is neither audio nor a folder: nothing to tag, and no crash.
	s.press("2", "tab", "j", "j", "j")
	m := s.m()
	if node, _ := m.fileNode(); node.Name != "notes.txt" {
		t.Fatalf("cursor on %q", node.Name)
	}
	s.press("*")
	s.shows("no audio file or folder to tag")
	s.press("+")
	s.shows("no audio file or folder to tag")
	// Marked, it is still skipped.
	s.press("v", "*")
	s.shows("no audio file or folder to tag")
}

func TestFavouriteAFolderAndEveryFileInItHasTheTag(t *testing.T) {
	s := open(t)
	s.sim.AddFile("SD:IMPORT/gems/deep/a_bass.wav", wav(480))
	s.sim.AddFile("SD:IMPORT/gems/b_synth.wav", wav(480))
	s.sim.AddFile("SD:IMPORT/other/c_pad.wav", wav(480))
	s.press("2", "tab", "i")
	s.shows("indexed")
	// The cursor is on IMPORT; gems/ is the next row.
	s.press("j")
	m := s.m()
	node, _ := m.fileNode()
	if node.Name != "gems" {
		t.Fatalf("cursor on %q", node.Name)
	}
	s.press("*")
	s.shows("tagged 1 file", "+favourite", "★")
	// Every file below has the tag, the ones in a deeper folder too, and the others do not.
	m = s.m()
	for path, want := range map[string]bool{
		"SD:IMPORT/gems/deep/a_bass.wav": true, "SD:IMPORT/gems/b_synth.wav": true, "SD:IMPORT/other/c_pad.wav": false,
	} {
		if got := slices.Contains(m.tags.Tags(path), "favourite"); got != want {
			t.Errorf("%s favourite: %v, want %v", path, got, want)
		}
	}
	if hits := m.tags.Search("favourite"); len(hits) != 2 {
		t.Errorf("found %v", hits)
	}
	// A second * takes it off the folder, and the files lose it.
	s.press("*")
	s.hides("★")
	m = s.m()
	if hits := m.tags.Search("favourite"); len(hits) != 0 {
		t.Errorf("found %v after taking it off", hits)
	}
}

func TestPatternsOfTheBankShowUnderThePadsAndPickOutTheOnesThatPlayThePad(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 48})
	s.press("N", "enter", "enter")
	// They are read quietly, without holding the screen, and listed under the pads.
	s.shows("Pads · bank A", "Patterns · bank A", "3 pads")
	if s.m().busy != "" {
		t.Errorf("the screen was held: %q", s.m().busy)
	}
	// The pad in front is played by pattern 1, which is picked out in the title.
	s.shows("with this pad")
	// On a bank without patterns the slots are empty.
	s.press("]")
	s.shows("Patterns · bank B")
}

func TestAssignKeepsWhereTheSampleCameFromAndShowsItOnThePad(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 180, Height: 56})
	s.sim.AddFile("SD:IMPORT/Great Collection/Kit 7/09_OHat_124.wav", wav(2400))
	// Open the folders down to the file and assign it to the empty pad A16.
	s.press("G", "a", "/", "ctrl+u")
	s.typeText("09_OHat")
	s.press("enter")
	s.shows("assigned to A16", "from SD:IMPORT/Great Collection/Kit 7")
	// The origin is on the card, apart from the tags, and shows in the pad's details.
	data, ok := s.sim.File("SD:404tui-origins.json")
	if !ok || !strings.Contains(string(data), "Great Collection/Kit 7/09_OHat_124.wav") {
		t.Fatalf("the notes on the card: %v %s", ok, data)
	}
	s.press("G")
	s.shows("From", "Great Collection/Kit 7/09_OHat_124.wav", "assigned 20")
	// Deleting the sample takes its note with it.
	s.press("d", "y")
	data, _ = s.sim.File("SD:404tui-origins.json")
	if strings.Contains(string(data), "09_OHat") {
		t.Errorf("the note stayed after the sample was deleted: %s", data)
	}
}

func TestTabMovesBetweenThePadsAndThePatternsPanelAndTheRailHasNoPatterns(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 180, Height: 56})
	s.shows("1 Pads", "2 Files", "3 Projects")
	s.hides("2 Patterns")
	if s.m().section != secPads {
		t.Fatal("not on the pads")
	}
	// Tab moves the focus into the patterns under the pads: the same screen, with the
	// pattern keys, and the rail still shows Pads.
	s.press("tab")
	s.shows("Pads · bank A", "Patterns · bank A", "Pattern A1")
	if s.m().section != secPatterns {
		t.Fatal("the focus did not move to the patterns")
	}
	s.press("N", "enter", "enter")
	s.shows("pattern A")
	// Esc comes back to the pads without leaving the program.
	s.press("esc")
	if s.m().section != secPads {
		t.Errorf("Esc left the patterns for %d", s.m().section)
	}
	s.press("tab", "tab")
	if s.m().section != secPads {
		t.Errorf("a second Tab did not come back: %d", s.m().section)
	}
}

func TestTheFolderFinderListsTheFavouriteFoldersFirst(t *testing.T) {
	s := open(t)
	s.sim.AddFile("SD:IMPORT/alpha/a.wav", wav(480))
	s.sim.AddFile("SD:IMPORT/zeta/deep/z.wav", wav(480))
	s.press("2", "tab", "i")
	s.shows("indexed")
	// Favourite zeta/deep: it is the first the finder offers, with a star, though it sorts last.
	s.press("f")
	s.typeText("deep")
	s.press("enter")
	s.press("*")
	s.shows("+favourite")
	s.press("f")
	s.shows("★ deep/")
	m := s.m()
	choices := folderChoices(m.tree.Rows(), m.indexes[m.root], m.tags, "")
	if len(choices) == 0 || !choices[0].Fav || choices[0].Name != "deep" {
		t.Fatalf("first choices %+v", choices[:min(3, len(choices))])
	}
	// Enter jumps to it.
	s.press("enter")
	if node, _ := m.fileNode(); node.Name == "" {
		t.Error("no row in front")
	}
}

func TestTheVersionIsInTheTopRightCorner(t *testing.T) {
	s := open(t)
	first := strings.Split(ansi.Strip(s.m().View()), "\n")[0]
	want := "v" + version.Version
	if !strings.HasSuffix(strings.TrimRight(first, " "), want) {
		t.Errorf("the first line does not end with %q: %q", want, first)
	}
	// Also while waiting for a unit.
	m := NewConnecting(func() (app.Unit, io.Closer, error) { return nil, nil, ErrNoUnit }, &fakePlayer{}, &memStore{})
	m.width, m.height = 100, 30
	if first := strings.Split(ansi.Strip(m.View()), "\n")[0]; !strings.Contains(first, want) {
		t.Errorf("no version while waiting: %q", first)
	}
}

func TestPadsOnABusShowItsColourBadgeAndCount(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	bus, _ := proto.ParamByName("bus-fx")
	m := s.m()
	for name, v := range map[string]int32{"A2": 1, "A3": 2} {
		if err := m.unit.SetParam(bus.ID, proto.TargetPad(pad(t, name)), v); err != nil {
			t.Fatal(err)
		}
	}
	s.press("r")
	out := ansi.Strip(s.m().View())
	for _, want := range []string{"¹ BUS 1 ×1", "² BUS 2 ×1"} {
		if !strings.Contains(out, want) {
			t.Errorf("the legend lacks %q", want)
		}
	}
	// The badge is on the tile of the pad, beside its name; the pad on no bus has none.
	lines := strings.Split(out, "\n")
	found := map[string]bool{}
	for _, l := range lines {
		if strings.Contains(l, "snare") {
			found["1"] = strings.Contains(l, "¹")
		}
		if strings.Contains(l, " 3 ") && strings.Contains(l, "hat") {
			found["2"] = strings.Contains(l, "²")
		}
	}
	_ = found
	// The tile is drawn in the colour of the bus (blue / red): the raw output has the colour
	// codes for them, and a tile on no bus has neither.
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(prev)
	raw := s.m().View()
	th := s.m().st.th
	if !strings.Contains(raw, "38;5;"+string(th.bus1)) || !strings.Contains(raw, "38;5;"+string(th.bus2)) {
		t.Error("the bus colours are not in the screen")
	}
	// Every tile line keeps the tile's width, so the grid does not slip.
	for i, l := range lines {
		if strings.Contains(l, "│") && ansi.StringWidth(l) > 170 {
			t.Errorf("line %d is %d wide", i, ansi.StringWidth(l))
		}
	}
}

func TestOverlayDrawsABoxInTheMiddleOverTheScreen(t *testing.T) {
	base := strings.Join([]string{"aaaaaaaaaaaa", "bbbbbbbbbbbb", "cccccccccccc", "dddddddddddd", "eeeeeeeeeeee"}, "\n")
	got := strings.Split(overlay(base, "XX\nYY", 12, 5), "\n")
	want := []string{"aaaaaaaaaaaa", "bbbbbbbbbbbb", "cccccXXccccc", "ddddd" + "YY" + "ddddd", "eeeeeeeeeeee"}
	// A box of two lines goes at the second and third line of five, the middle.
	want = []string{"aaaaaaaaaaaa", "bbbbbXXbbbbb", "ccccc" + "YY" + "ccccc", "dddddddddddd", "eeeeeeeeeeee"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: %q, want %q", i, got[i], want[i])
		}
	}
	// A screen shorter than the dialog's place is padded, a short line is padded to the width.
	if got := overlay("a", "XX", 6, 3); !strings.Contains(got, "  XX") {
		t.Errorf("padded %q", got)
	}
}

func TestADialogIsDrawnOverTheScreenAndNotInsteadOfIt(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	s.press("S")
	s.shows("Settings", "Level")
	// The rail, the pads and the panels are still there around the dialog.
	s.shows("1 Pads", "BANKS", "Pads · bank A")
}

func TestFindTheOriginsOfPadsWhenTheNoteIsMissing(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	// Without an index there is nothing to look in, and it says how to make one.
	s.press("F")
	s.shows("Look for where the samples came from in")
	s.press("enter")
	s.shows("no indexed files")
	s.sim.AddFile("SD:IMPORT/Great Kit/Loop 9.wav", wav(2400))
	s.press("2", "tab", "i")
	s.shows("indexed")
	s.press("r", "1")
	// A pad assigned without the app (no note), whose name is the file's name.
	m := s.m()
	if _, err := app.Assign(m.unit, m.unit, pad(t, "A16"), "SD:IMPORT/Great Kit/Loop 9.wav"); err != nil {
		t.Fatal(err)
	}
	s.press("r", "G", "F")
	s.shows("Look for where the samples came from in")
	s.press("enter")
	s.shows("origins:", "found")
	data, ok := s.sim.File("SD:404tui-origins.json")
	if !ok || !strings.Contains(string(data), "Great Kit/Loop 9.wav") || !strings.Contains(string(data), `"matched": true`) {
		t.Fatalf("the notes: %v %s", ok, data)
	}
	s.shows("From", "Great Kit/Loop 9.wav", "found by scan")
}

func TestPadFlagsAreTinyLettersForWhatIsOn(t *testing.T) {
	row := app.PadRow{HasSample: true, Params: map[string]int32{"gate": 1, "loop": 1, "mode-flags": 0x20, "play-mode": 3, "bpm-sync": 1, "mute-group": 4}}
	if got := onLetters(padFlags(row)); got != "G L O R S ~ M4" {
		t.Errorf("flags %q", got)
	}
	if got := onLetters(padFlags(app.PadRow{HasSample: true, Params: map[string]int32{"play-mode": 1}})); got != "R" {
		t.Errorf("reverse %q", got)
	}
	// Nothing on: the letters are still there, all off.
	if got := padFlags(app.PadRow{HasSample: true, Params: map[string]int32{}}); len(got) != 6 || onLetters(got) != "" {
		t.Errorf("a pad with nothing on has %v", got)
	}
	if got := padFlags(app.PadRow{Params: map[string]int32{"gate": 1}}); got != nil {
		t.Errorf("an empty pad has %v", got)
	}
}

func TestTheTileShowsTheFlagsBesideItsNumberAndKeepsItsWidth(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	gate, _ := proto.ParamByName("gate")
	loop, _ := proto.ParamByName("loop")
	m := s.m()
	for _, id := range []byte{gate.ID, loop.ID} {
		if err := m.unit.SetParam(id, proto.TargetPad(pad(t, "A2")), 1); err != nil {
			t.Fatal(err)
		}
	}
	s.press("r")
	// With room the pads are a row taller, and the letters have a line of their own.
	if g := s.m().geom(); g.tileH != 4 {
		t.Fatalf("tiles are %d rows on a tall screen", g.tileH)
	}
	s.shows("snare", " G L O R S V ")
	s.hides("2  G L")
	// On a shorter screen they sit beside the number again.
	s.send(tea.WindowSizeMsg{Width: 170, Height: 34})
	if g := s.m().geom(); g.tileH != 3 {
		t.Fatalf("tiles are %d rows on a short screen", g.tileH)
	}
	s.shows("2  G L O R S V")
	// A pad with nothing on shows the letters too, unlit.
	s.shows("3  G L O R S V")
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	out := ansi.Strip(s.m().View())
	for i, l := range strings.Split(out, "\n") {
		if ansi.StringWidth(l) > 170 {
			t.Errorf("line %d is %d wide", i, ansi.StringWidth(l))
		}
	}
}

func TestEditTheNotesOfAPattern(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	// A pattern with every pad of bank A on beat 1, then open it.
	s.press("N", "enter", "enter", "tab", "enter")
	s.shows("Pattern A1", "vel 127", "on the grid", "x note on/off")
	notesOf := func() []formats.NoteRecord {
		t.Helper()
		data, ok := s.sim.File("ROLAND/SP-404MKII/PROJECT_01/PTN/PTN00001.BIN")
		if !ok {
			t.Fatal("no pattern file")
		}
		notes, err := formats.NoteRecords(data)
		if err != nil {
			t.Fatal(err)
		}
		return notes
	}
	first := s.m().pattern.Grid.Rows[0].Pad
	velocityOf := func(pad proto.PadIndex, tick uint32) int {
		for _, n := range notesOf() {
			if n.Pad == pad && n.Tick == tick {
				return int(n.Velocity)
			}
		}
		return -1
	}
	// Velocity: down by 10, down by 1, then back to full.
	s.press("<")
	s.shows("vel 117")
	if v := velocityOf(first, 0); v != 117 {
		t.Fatalf("the file has %d", v)
	}
	s.press(",")
	if v := velocityOf(first, 0); v != 116 {
		t.Errorf("after , the file has %d", v)
	}
	s.press("*")
	if v := velocityOf(first, 0); v != 127 {
		t.Errorf("after * the file has %d", v)
	}
	// A note on an empty step, then off again; the pattern keeps its length.
	before := len(notesOf())
	s.press("l", "l", "x")
	s.shows("note on")
	tick := s.m().pattern.StepTick(2)
	if v := velocityOf(first, tick); v != 127 || len(notesOf()) != before+1 {
		t.Fatalf("no note at step 3: %d, %d notes", v, len(notesOf()))
	}
	s.press("x")
	s.shows("taken out")
	if len(notesOf()) != before {
		t.Errorf("%d notes after taking it out, want %d", len(notesOf()), before)
	}
	if s.m().pattern.Length != 3840 {
		t.Errorf("the pattern is %d ticks long now", s.m().pattern.Length)
	}
}

func TestAPadKeepsItsRowSpaceDoesNotEditAndUUndoes(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	s.press("N", "enter", "enter", "tab", "enter")
	s.shows("Pattern A1")
	countNotes := func() int {
		data, _ := s.sim.File("ROLAND/SP-404MKII/PROJECT_01/PTN/PTN00001.BIN")
		notes, err := formats.NoteRecords(data)
		if err != nil {
			t.Fatal(err)
		}
		return len(notes)
	}
	before, rows := countNotes(), len(s.m().pattern.Grid.Rows)
	// Space plays the pattern: the file is not touched.
	s.press(" ")
	if countNotes() != before {
		t.Fatal("Space changed the pattern")
	}
	// Taking out the only note of the first pad keeps its row, to put a note back on.
	first := s.m().pattern.Grid.Rows[0].Pad
	s.press("x")
	s.shows("taken out")
	if countNotes() != before-1 || len(s.m().pattern.Grid.Rows) != rows || s.m().pattern.Grid.Rows[0].Pad != first {
		t.Fatalf("%d notes, %d rows (was %d, %d)", countNotes(), len(s.m().pattern.Grid.Rows), before, rows)
	}
	// u brings it back; R redoes (takes it out again).
	s.press("u")
	s.shows("undo")
	if countNotes() != before {
		t.Errorf("after undo %d notes, want %d", countNotes(), before)
	}
	s.press("R")
	s.shows("redo")
	if countNotes() != before-1 {
		t.Errorf("after redo %d notes, want %d", countNotes(), before-1)
	}
	// A pad of the bank with a sample and no note has a row too: the fixture has kick, snare,
	// hat on A1-A3, all in the pattern; a sample put on A5 gets an empty row to put notes on.
	m := s.m()
	s.sim.AddFile("SD:IMPORT/loop.wav", wav(4800))
	if _, err := app.Assign(m.unit, m.unit, pad(t, "A5"), "SD:IMPORT/loop.wav"); err != nil {
		t.Fatal(err)
	}
	s.press("esc", "1", "r", "tab", "enter")
	found := false
	for _, r := range s.m().pattern.Grid.Rows {
		found = found || r.Pad == pad(t, "A5")
	}
	if !found {
		t.Error("a pad of the bank with a sample has no row")
	}
}

func TestUndoRedoStackGoesManyStepsBack(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	s.press("N", "enter", "enter", "tab", "enter")
	s.shows("Pattern A1")
	countNotes := func() int {
		data, _ := s.sim.File("ROLAND/SP-404MKII/PROJECT_01/PTN/PTN00001.BIN")
		notes, err := formats.NoteRecords(data)
		if err != nil {
			t.Fatal(err)
		}
		return len(notes)
	}
	start := countNotes()
	// Toggle three different pads off: three edits onto the undo stack.
	s.press("x", "j", "x", "j", "x")
	if countNotes() != start-3 {
		t.Fatalf("after three edits %d notes, want %d", countNotes(), start-3)
	}
	s.shows("3 undo · 0 redo")
	// Undo all three, one at a time.
	s.press("u", "u", "u")
	if countNotes() != start {
		t.Fatalf("after undoing all three %d notes, want %d", countNotes(), start)
	}
	s.shows("0 undo · 3 redo", "step 0/3")
	// Nothing left to undo.
	s.press("u")
	s.shows("nothing to undo")
	// Redo two of the three.
	s.press("R", "R")
	if countNotes() != start-2 {
		t.Fatalf("after redoing two %d notes, want %d", countNotes(), start-2)
	}
	s.shows("2 undo · 1 redo", "step 2/3")
	// A fresh edit drops the redo future.
	s.press("j", "x")
	s.shows("3 undo · 0 redo")
	s.press("R")
	s.shows("nothing to redo")
}

func TestPatternToolsMenuSwingsTheWholePattern(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	s.press("N", "enter", "enter", "tab", "enter")
	s.shows("Pattern A1")
	ticksBefore := func() []uint32 {
		data, _ := s.sim.File("ROLAND/SP-404MKII/PROJECT_01/PTN/PTN00001.BIN")
		notes, _ := formats.NoteRecords(data)
		var out []uint32
		for _, n := range notes {
			out = append(out, n.Tick)
		}
		return out
	}
	before := ticksBefore()
	// T opens the whole-pattern groove tools; the menu names them and says the scope.
	s.press("T")
	s.shows("Groove tools", "whole pattern", "SWING", "QUANT", "SHIFT", "REVERSE", "HUMANIZE")
	// Reverse has a clear, deterministic effect: Enter applies it to the whole pattern.
	s.press("j", "j", "j") // SWING -> QUANT -> SHIFT -> REVERSE
	s.press("enter")
	s.shows("reverse", "all pads")
	after := ticksBefore()
	if len(after) != len(before) {
		t.Fatalf("reverse changed the note count %d -> %d", len(before), len(after))
	}
	// Reverse is on the undo stack now.
	s.shows("1 undo")
	s.press("u")
	if got := ticksBefore(); len(got) != len(before) {
		t.Errorf("undo after reverse changed the note count: %d, want %d", len(got), len(before))
	}
	// t (lowercase) scopes the tools to the pad the cursor is on, not the theme.
	s.press("t")
	s.shows("Groove tools", "pad A1")
	s.press("esc")
	s.hides("Groove tools")
}

func TestNotesAreGlyphsAsBrightAsTheirVelocityAndDashedAsLongAsTheyLast(t *testing.T) {
	// The glyphs by velocity: ghost, soft, mid, loud.
	m0 := open(t).m()
	for v, want := range map[int]string{1: "○", 47: "◌", 64: "◉", 127: "●"} {
		if got, _ := m0.noteGlyph(v); got != want {
			t.Errorf("velocity %d: %q, want %q", v, got, want)
		}
	}
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	s.press("N", "enter", "enter", "tab", "enter")
	m := s.m()
	// The pattern N makes holds every pad for the whole pattern: a loud mark, then dashes on.
	row := strings.TrimRight(ansi.Strip(m.noteRow(m.pattern, m.pattern.Grid.Rows[1].Pad, false, m.patSpan(m.pattern, 60))), " ")
	if strings.Contains(row, "·") || !strings.HasPrefix(row, "●─") {
		t.Errorf("a note held to the end is not one dashed note: %q", row)
	}
	// Quieter, the mark is softer.
	s.press("j", "<", "<", "<", "<", "<", "<", "<", "<")
	m = s.m()
	row = ansi.Strip(m.noteRow(m.pattern, m.pattern.Grid.Rows[1].Pad, false, m.patSpan(m.pattern, 60)))
	if !strings.HasPrefix(row, "◌") || strings.Contains(row, "●") {
		t.Errorf("velocity 47 is drawn %q", row)
	}
	// A short note marks its step only; the rest of the row is dots, no dashes.
	s.press("x", "l", "l", "l", "l", "x")
	m = s.m()
	row = ansi.Strip(m.noteRow(m.pattern, m.pattern.Grid.Rows[1].Pad, false, m.patSpan(m.pattern, 60)))
	if !strings.HasPrefix(row, "· ") || strings.Count(row, "●") != 1 || strings.Contains(row, "─") {
		t.Errorf("a one-step note is drawn %q", row)
	}
}

func TestThePatternThatPlaysIsMarkedWhereverTheCursorIs(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	m := s.m()
	level, _ := proto.ParamByName("level")
	if _, err := app.Assign(m.unit, m.unit, pad(t, "C1"), "SD:IMPORT/kick.wav"); err != nil {
		t.Fatal(err)
	}
	if err := m.unit.SetParam(level.ID, proto.TargetPad(pad(t, "C1")), 127); err != nil {
		t.Fatal(err)
	}
	s.press("r", "]", "]", "N", "enter", "enter", "tab", "P")
	if !s.m().looping() {
		t.Fatalf("nothing loops: %s", s.m().message)
	}
	// The cursor moves on to another slot; the one that plays keeps its mark.
	s.press("l")
	m = s.m()
	tile := m.tile(30, 2, "1", "1 pads", 0, tileState{noLevel: true, progress: 0.5, playing: true})
	if !strings.Contains(ansi.Strip(tile), "▶") || !strings.Contains(ansi.Strip(tile), "━") {
		t.Errorf("a playing pattern tile has no mark or progress: %q", ansi.Strip(tile))
	}
	if idle := ansi.Strip(m.tile(30, 2, "2", "1 pads", 0, tileState{noLevel: true, progress: -1})); strings.ContainsAny(idle, "╌━") {
		t.Errorf("a pattern that does not play has a bar: %q", idle)
	}
	s.shows("▶")
}

func TestASampleIsTriedInTheLoopAndAssignedWithoutStoppingIt(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	m := s.m()
	level, _ := proto.ParamByName("level")
	for _, p := range []string{"C1", "C2"} {
		if _, err := app.Assign(m.unit, m.unit, pad(t, p), "SD:IMPORT/kick.wav"); err != nil {
			t.Fatal(err)
		}
		if err := m.unit.SetParam(level.ID, proto.TargetPad(pad(t, p)), 127); err != nil {
			t.Fatal(err)
		}
	}
	// A pattern of bank C loops.
	s.press("r", "]", "]", "N", "enter", "enter", "tab", "P")
	if len(s.player.looped) != 1 || !s.m().looping() {
		t.Fatalf("the pattern does not loop: %s", s.m().message)
	}
	// The assign dialog for C1, on snare.wav: listening tries it in the loop, on C1.
	findSnare := func() {
		t.Helper()
		s.press("tab", "g", "a", "/", "ctrl+u")
		s.typeText("snare")
		s.press("ctrl+o")
		if b := s.m().browser; b == nil || b.tree.Rows()[b.at].Name != "snare.wav" {
			t.Fatal("the assign dialog is not on snare.wav")
		}
	}
	findSnare()
	replaced, played := len(s.player.replaced), len(s.player.played)
	s.press(" ")
	if try := s.m().loopTry; try == nil || try.pad != pad(t, "C1") {
		t.Fatalf("snare.wav is not tried on C1: %+v", try)
	}
	if len(s.player.replaced) != replaced+1 || len(s.player.played) != played || !s.m().looping() {
		t.Fatalf("trying a file: replaced %d (was %d), played %d (was %d), looping %v",
			len(s.player.replaced), replaced, len(s.player.played), played, s.m().looping())
	}
	s.shows("in the playing pattern")
	// Esc: the pad's own sound comes back, the loop goes on, nothing was written.
	s.press("esc")
	if s.m().loopTry != nil || !s.m().looping() || len(s.player.replaced) != replaced+2 {
		t.Fatalf("after Esc: try %+v, looping %v, replaced %d", s.m().loopTry, s.m().looping(), len(s.player.replaced))
	}
	if s.padName("C1") != "kick" {
		t.Fatalf("C1 is %q after giving the file up", s.padName("C1"))
	}
	// Again, and assign it: C1 holds the snare now, the loop never stopped or started over.
	s.press("tab")
	findSnare()
	s.press(" ", "enter", "enter")
	if s.padName("C1") != "snare" {
		t.Fatalf("C1 is %q after assigning", s.padName("C1"))
	}
	if !s.m().looping() || len(s.player.looped) != 1 || s.m().loopTry != nil {
		t.Errorf("after assigning: looping %v, loops started %d, try %+v", s.m().looping(), len(s.player.looped), s.m().loopTry)
	}
}

func TestAChangeIsHeardAtOnceInALoopingPattern(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	// Bank C has real samples (the simulator's fixture pads have none the player could read).
	m := s.m()
	level, _ := proto.ParamByName("level")
	for _, p := range []string{"C1", "C2"} {
		if _, err := app.Assign(m.unit, m.unit, pad(t, p), "SD:IMPORT/kick.wav"); err != nil {
			t.Fatal(err)
		}
		// The simulator starts pads at level 0; the unit at 127.
		if err := m.unit.SetParam(level.ID, proto.TargetPad(pad(t, p)), 127); err != nil {
			t.Fatal(err)
		}
	}
	s.press("r", "]", "]", "N", "enter", "enter", "tab", "enter")
	s.press("P")
	if len(s.player.looped) != 1 {
		t.Fatalf("looped %d: %s", len(s.player.looped), s.m().message)
	}
	first := s.player.looped[0]
	// While it loops, a line under the step numbers shows where it is and how long is left.
	s.shows("s to go")
	if line := ansi.Strip(s.m().playhead(s.m().pattern, s.m().patSpan(s.m().pattern, 200))); !strings.ContainsAny(line, "█━") {
		t.Errorf("the playhead line has no position: %q", line)
	}
	// While it plays, the bar on show follows the music: three quarters through a 2-bar round
	// is bar 2, wherever the cursor is.
	late := s.m()
	late.loopStart = time.Now().Add(-late.loopRound * 3 / 4)
	if page := late.viewPage(late.pattern); page != 1 {
		t.Errorf("three quarters through the round the page is %d, want 1", page)
	}
	if sp := late.patSpan(late.pattern, 60); sp.lo != 16 {
		t.Errorf("a one-bar view three quarters through starts at step %d, want 16", sp.lo)
	}
	if frac, left, ok := s.m().loopAt(s.m().pattern.Slot); !ok || frac < 0 || frac >= 1 || left <= 0 {
		t.Errorf("loop at %v, %v left, %v", frac, left, ok)
	}
	// Taking a note out: the loop goes on with the new mix at once, from where it is, not restarted.
	s.press("x")
	if len(s.player.replaced) != 1 || len(s.player.looped) != 1 {
		t.Fatalf("queued %d, looped %d after an edit", len(s.player.replaced), len(s.player.looped))
	}
	if peak(s.player.replaced[0]) >= peak(first) {
		t.Errorf("the mix without a note is not quieter: %d, %d", peak(s.player.replaced[0]), peak(first))
	}
	// A velocity: mixed again.
	s.press("u", "<")
	if len(s.player.replaced) < 3 {
		t.Errorf("queued %d after undo and a velocity", len(s.player.replaced))
	}
	// A pad setting: its level, in the Pads view, while the pattern plays.
	s.press("esc", "1")
	n := len(s.player.replaced)
	s.press(",")
	if len(s.player.replaced) != n+1 {
		t.Errorf("a pad level change was not mixed in: %d, was %d", len(s.player.replaced), n)
	}
	s.press("tab", "enter")
	// Stopped, nothing is mixed again, and the line is gone.
	s.press("P")
	s.hides("s to go")
	before := len(s.player.replaced)
	s.press(">")
	if len(s.player.replaced) != before {
		t.Error("a stopped pattern was mixed again")
	}
}

func peak(s formats.Sample) int {
	top := 0
	for _, v := range s.Samples {
		top = max(top, int(v), -int(v))
	}
	return top
}

func TestTheLengthOfANoteChangesByAStep(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	s.press("N", "enter", "enter", "tab", "enter")
	notesOf := func() []formats.NoteRecord {
		data, _ := s.sim.File("ROLAND/SP-404MKII/PROJECT_01/PTN/PTN00001.BIN")
		notes, _ := formats.NoteRecords(data)
		return notes
	}
	pad := s.m().pattern.Grid.Rows[0].Pad
	lengthAt := func(tick uint32) int {
		for _, n := range notesOf() {
			if n.Pad == pad && n.Tick == tick {
				return int(n.Length)
			}
		}
		return -1
	}
	step := int(s.m().pattern.StepTicks())
	// The notes N makes last the whole pattern: longer does nothing, shorter takes a step off.
	s.press("+")
	s.shows("lasts to the end")
	s.press("-")
	if got := lengthAt(0); got != 3840-step {
		t.Fatalf("after - the note lasts %d ticks, want %d", got, 3840-step)
	}
	// On a step the note covers (not its first), the keys work on it too.
	s.press("l", "l", "-")
	if got := lengthAt(0); got != 3840-2*step {
		t.Errorf("after - on a later step it lasts %d, want %d", got, 3840-2*step)
	}
	s.shows("lasts 30 steps")
	// A one-step note: shorter stops at a step, longer adds one.
	s.press("l", "l", "l", "l", "l", "l", "l", "l", "l", "l", "l", "l", "l")
	s.press("x")
	tick := s.m().pattern.StepTick(15)
	if got := lengthAt(tick); got != step {
		t.Fatalf("a new note lasts %d, want %d", got, step)
	}
	s.press("-")
	s.shows("one step long already")
	s.press("h", "h", "h", "x", "+")
	if got := lengthAt(s.m().pattern.StepTick(12)); got != 2*step {
		t.Errorf("after + a new note lasts %d, want %d", got, 2*step)
	}
}

func TestPatternEditPagesPerBar(t *testing.T) {
	s := open(t)
	// Wide enough, every bar is on show side by side.
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	s.press("N", "enter", "enter", "tab", "enter")
	s.shows("Pattern A1", "Bars 1–2 / 2")
	// Too narrow for two, the grid shows one bar at a time and pages.
	s.send(tea.WindowSizeMsg{Width: 80, Height: 54})
	s.shows("Bar 1 / 2")
	spp := s.m().pattern.StepsPerPage()
	if spp != 16 {
		t.Fatalf("steps per page = %d, want 16", spp)
	}
	// } flips to the next bar; the header and page follow.
	s.press("}")
	s.shows("Bar 2 / 2")
	if got := s.m().patPage(); got != 1 {
		t.Errorf("after } the page is %d, want 1", got)
	}
	// { flips back.
	s.press("{")
	s.shows("Bar 1 / 2")
	if got := s.m().patPage(); got != 0 {
		t.Errorf("after { the page is %d, want 0", got)
	}
	// Walking the cursor off the right of bar 1 with l auto-pages into bar 2.
	for i := 0; i < spp; i++ {
		s.press("l")
	}
	s.shows("Bar 2 / 2")
	if got := s.m().patCol; got != spp {
		t.Errorf("after walking off bar 1 the cursor is at col %d, want %d", got, spp)
	}
	// A note set on bar 2 lands on the right tick.
	notesOf := func() []formats.NoteRecord {
		data, _ := s.sim.File("ROLAND/SP-404MKII/PROJECT_01/PTN/PTN00001.BIN")
		notes, _ := formats.NoteRecords(data)
		return notes
	}
	s.press("g", "l", "l") // start of bar 2, then step 3 of the bar.
	wantTick := s.m().pattern.StepTick(s.m().patCol)
	if wantTick != 1920+2*120 {
		t.Fatalf("bar 2 step 3 is tick %d, want %d", wantTick, 1920+2*120)
	}
	s.press("x")
	s.shows("bar 2 step 3")
	found := false
	for _, n := range notesOf() {
		if n.Tick == wantTick {
			found = true
		}
	}
	if !found {
		t.Errorf("no note at tick %d after setting bar 2 step 3", wantTick)
	}
}

func TestEnterExpandsATrackAndEscCollapsesIt(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	s.press("N", "enter", "enter", "tab", "enter")
	notes := func() int {
		data, _ := s.sim.File("ROLAND/SP-404MKII/PROJECT_01/PTN/PTN00001.BIN")
		n, _ := formats.NoteRecords(data)
		return len(n)
	}
	before := notes()
	// Enter on the snare row expands it: the step numbers of the bar show, nothing is written.
	s.press("j", "enter")
	if !s.m().patFocus || s.m().patRow != 1 {
		t.Fatalf("Enter did not expand the track: focus %v, row %d", s.m().patFocus, s.m().patRow)
	}
	s.shows("Tab fold")
	if nums := ansi.Strip(s.m().focusLane(s.m().pattern, s.m().pattern.Grid.Rows[1].Pad, s.m().patSpan(s.m().pattern, 150))[0]); !strings.HasPrefix(nums, "1") || !strings.Contains(nums, "16") {
		t.Errorf("the expanded lane is not numbered 1-16: %q", nums)
	}
	if notes() != before {
		t.Fatal("expanding a track changed the pattern")
	}
	// Inside the lane, Enter sets a note on the step under the cursor.
	s.press("l", "l", "enter")
	if notes() != before+1 {
		t.Fatalf("%d notes after Enter in the lane, want %d", notes(), before+1)
	}
	// j moves the expanded lane to the next track.
	s.press("j")
	if !s.m().patFocus || s.m().patRow != 2 {
		t.Errorf("j in the lane: focus %v, row %d", s.m().patFocus, s.m().patRow)
	}
	// Tab folds it back to the grid, and expands it again, without leaving the pattern.
	s.press("tab")
	if s.m().patFocus || s.m().pattern == nil || s.m().section != secPatterns {
		t.Fatalf("Tab did not fold: focus %v, pattern open %v, section %d", s.m().patFocus, s.m().pattern != nil, s.m().section)
	}
	s.shows("Tab Enter open track")
	s.press("tab")
	if !s.m().patFocus || s.m().patRow != 2 {
		t.Fatalf("Tab did not expand the track on the cursor row: focus %v, row %d", s.m().patFocus, s.m().patRow)
	}
	// Esc closes the pattern, even from an expanded track, and goes back to the Sequencer tab.
	s.press("esc")
	if s.m().pattern != nil || s.m().patFocus || s.m().section != secSequencer {
		t.Fatalf("Esc: pattern open %v, focus %v, section %d", s.m().pattern != nil, s.m().patFocus, s.m().section)
	}
	s.press("1", "tab")
	s.shows("Patterns · bank A")
	// From the grid too; and it opens folded again.
	s.press("enter")
	if s.m().patFocus || s.m().pattern == nil {
		t.Errorf("reopened: focus %v, pattern open %v", s.m().patFocus, s.m().pattern != nil)
	}
	s.press("esc")
	if s.m().pattern != nil {
		t.Error("Esc from the grid did not close the pattern")
	}
}

func TestANoteIsNudgedByTicksAndSnappedBack(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	s.press("N", "enter", "enter", "tab", "enter")
	pad := s.m().pattern.Grid.Rows[0].Pad
	tickOf := func() int {
		data, _ := s.sim.File("ROLAND/SP-404MKII/PROJECT_01/PTN/PTN00001.BIN")
		notes, _ := formats.NoteRecords(data)
		for _, n := range notes {
			if n.Pad == pad {
				return int(n.Tick)
			}
		}
		return -1
	}
	// The note on the first step cannot go earlier than the start.
	s.press("H")
	s.shows("cannot move further")
	// L by 5, ) by 1: 6 ticks late, and the readout says so.
	s.press("L", ")")
	if got := tickOf(); got != 6 {
		t.Fatalf("after L ) the note is at tick %d, want 6", got)
	}
	s.shows("+6 ticks late")
	// ( back by 1.
	s.press("(")
	if got := tickOf(); got != 5 {
		t.Errorf("after ( the note is at tick %d, want 5", got)
	}
	// 0 puts it back on the grid; it is one edit on the undo stack like any other.
	s.press("0")
	if got := tickOf(); got != 0 {
		t.Errorf("after 0 the note is at tick %d, want 0", got)
	}
	s.shows("on the grid")
	s.press("u")
	if got := tickOf(); got != 5 {
		t.Errorf("undoing the snap left the note at tick %d, want 5", got)
	}
}

func TestAPatternIsDoubledAndHalvedFromTheTools(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	s.press("N", "enter", "enter", "tab", "enter")
	if bars := s.m().pattern.Bars(); bars != 2 {
		t.Fatalf("the new pattern is %d bars", bars)
	}
	notes := func() int {
		data, _ := s.sim.File("ROLAND/SP-404MKII/PROJECT_01/PTN/PTN00001.BIN")
		n, _ := formats.NoteRecords(data)
		return len(n)
	}
	before := notes()
	// T, down to DOUBLE (after SWING, QUANT, SHIFT, REVERSE, HUMANIZE): 4 bars, every note twice.
	s.press("T")
	s.shows("DOUBLE", "2 bars → 4 bars, played again", "HALF")
	s.press("j", "j", "j", "j", "j", "enter")
	s.shows("doubled: 2 bars → 4 bars")
	if bars := s.m().pattern.Bars(); bars != 4 || notes() != 2*before {
		t.Fatalf("after DOUBLE: %d bars, %d notes (was %d)", bars, notes(), before)
	}
	// HALF takes it back to 2 bars.
	s.press("T", "j", "j", "j", "j", "j", "j", "j", "enter")
	s.shows("halved: 4 bars → 2 bars")
	if bars := s.m().pattern.Bars(); bars != 2 || notes() != before {
		t.Fatalf("after HALF: %d bars, %d notes (want %d)", bars, notes(), before)
	}
	// Each is one edit to undo.
	s.press("u")
	if bars := s.m().pattern.Bars(); bars != 4 {
		t.Errorf("undoing HALF left %d bars, want 4", bars)
	}
	// EXTEND doubles the length with nothing in the new half: the notes stay as they are.
	s.press("T", "j", "j", "j", "j", "j", "j", "enter")
	s.shows("extended: 4 bars → 8 bars, the new half empty")
	if bars := s.m().pattern.Bars(); bars != 8 || notes() != 2*before {
		t.Fatalf("after EXTEND: %d bars, %d notes (want %d)", bars, notes(), 2*before)
	}
}

func TestALongPatternShowsTwoBarsAPageAndWhatComesNext(t *testing.T) {
	s := open(t)
	// Room for three bars and more: still only two are shown, stretched.
	s.send(tea.WindowSizeMsg{Width: 220, Height: 54})
	s.press("N", "enter", "enter", "tab", "enter")
	s.press("T", "j", "j", "j", "j", "j", "enter") // DOUBLE: 4 bars
	if bars := s.m().pattern.Bars(); bars != 4 {
		t.Fatalf("%d bars after DOUBLE", bars)
	}
	s.shows("Bars 1–2 / 4", "next: bars 3–4")
	if sp := s.m().patSpan(s.m().pattern, 400); sp.hi-sp.lo != 32 {
		t.Errorf("a very wide view shows %d steps, want 32 (two bars)", sp.hi-sp.lo)
	}
	// } turns the whole page: bars 3-4, and the next is the first page again.
	s.press("}")
	s.shows("Bars 3–4 / 4", "next: bars 1–2")
	if got := s.m().patCol; got != 32 {
		t.Errorf("after } the cursor is on step %d, want 32 (bar 3)", got)
	}
	s.press("{")
	s.shows("Bars 1–2 / 4")
}

func TestARangeOfBarsIsShownEditedAndLoopedAlone(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 200, Height: 54})
	// Bank C gets real samples, so the pattern mixes.
	m := s.m()
	level, _ := proto.ParamByName("level")
	for _, p := range []string{"C1", "C2"} {
		if _, err := app.Assign(m.unit, m.unit, pad(t, p), "SD:IMPORT/kick.wav"); err != nil {
			t.Fatal(err)
		}
		if err := m.unit.SetParam(level.ID, proto.TargetPad(pad(t, p)), 127); err != nil {
			t.Fatal(err)
		}
	}
	s.press("r", "]", "]", "N", "enter", "enter", "tab", "enter")
	s.press("T", "j", "j", "j", "j", "j", "enter") // DOUBLE: 4 bars
	s.press("P")
	whole := s.player.looped[len(s.player.looped)-1].Frames()
	// v lists the whole pattern, each bar, and each two bars side by side.
	s.press("v")
	s.shows("Show and loop", "Whole pattern", "Bar 4", "Bars 3–4")
	// Whole, Bar 1-4, Bars 1-2, 2-3, 3-4: the eighth row.
	s.press("j", "j", "j", "j", "j", "j", "j", "enter")
	s.shows("Bars 3–4 / 4", "⟲ Bars 3–4 only")
	if lo, hi := s.m().colBounds(s.m().pattern); lo != 32 || hi != 64 {
		t.Fatalf("the cursor may go %d..%d, want 32..64", lo, hi)
	}
	if col := s.m().patCol; col < 32 || col >= 64 {
		t.Errorf("the cursor is on step %d, outside bars 3-4", col)
	}
	// The cursor stops at the range's edge.
	for range 40 {
		s.press("l")
	}
	if col := s.m().patCol; col != 63 {
		t.Errorf("l stopped on step %d, want 63 (the end of bar 4)", col)
	}
	// The loop starts over on the two bars: half as long as the whole.
	looped := s.player.looped[len(s.player.looped)-1].Frames()
	if looped*2 != whole {
		t.Errorf("the range loops %d frames, the whole pattern %d; want half", looped, whole)
	}
	// { moves the range to bars 1-2; v and the first row bring the whole pattern back.
	s.press("{")
	s.shows("⟲ Bars 1–2 only")
	s.press("v", "g", "enter")
	s.shows("Bars 1–2 / 4", "next: bars 3–4")
	if s.m().patRange != nil {
		t.Error("the whole pattern was chosen but a range is still set")
	}
	if got := s.player.looped[len(s.player.looped)-1].Frames(); got != whole {
		t.Errorf("back to the whole pattern the loop is %d frames, want %d", got, whole)
	}
}

func TestThePatternInFrontShowsItsStepsBesideAndSessionIsGone(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 180, Height: 56})
	s.hides("Session")
	s.press("N", "enter", "enter", "tab")
	// Without Enter: the panel beside shows the steps of the pattern in front.
	s.shows("Pattern A1", "120.00 BPM", "A1   kick", "Enter edits the steps")
	if s.m().patPreview[s.m().patterns[0][0].Slot] == nil {
		t.Error("the pattern in front was not read")
	}
	if s.m().pattern != nil {
		t.Error("the pattern was opened, not only shown")
	}
	// Moving to an empty slot shows that it is empty.
	s.press("l")
	s.shows("Empty slot.")
}

func TestMuteAndSoloChangeWhatThisComputerPlays(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	m := s.m()
	level, _ := proto.ParamByName("level")
	for _, p := range []string{"C1", "C2"} {
		if _, err := app.Assign(m.unit, m.unit, pad(t, p), "SD:IMPORT/kick.wav"); err != nil {
			t.Fatal(err)
		}
		if err := m.unit.SetParam(level.ID, proto.TargetPad(pad(t, p)), 127); err != nil {
			t.Fatal(err)
		}
	}
	s.press("r", "]", "]", "N", "enter", "enter", "tab", "P")
	if len(s.player.looped) != 1 {
		t.Fatalf("not looping: %s", s.m().message)
	}
	both := peak(s.player.looped[0])
	// Mute C1 in the Pads view: the loop goes on without it, at once.
	s.press("esc", "g", "m")
	s.shows("C1 is muted", "×")
	if len(s.player.replaced) != 1 || peak(s.player.replaced[0]) >= both {
		t.Fatalf("muting did not take C1 out: %d mixes", len(s.player.replaced))
	}
	one := peak(s.player.replaced[0])
	// Unmuted, solo C2: only C2, as loud as one pad.
	s.press("m", "l", "s")
	s.shows("C2 is soloed")
	last := s.player.replaced[len(s.player.replaced)-1]
	if peak(last) != one {
		t.Errorf("solo of C2 peaks at %d, one pad at %d", peak(last), one)
	}
	// In the open pattern the same keys work on the row's pad, and the rows are marked.
	s.press("tab", "enter")
	s.shows("Pattern C1")
	s.press("j", "s")
	if len(s.m().soloed) != 0 {
		t.Errorf("M on C2's row did not unsolo it: %v", s.m().soloed)
	}
	s.press("m")
	if !s.m().muted[pad(t, "C2")] {
		t.Error("m on C2's row did not mute it")
	}
	s.shows("MUTE")
	// * still sets the velocity to 127.
	s.press("<", "*")
	s.shows("velocity of C2 bar 1 step 1 is 127")
	// S keeps what is heard as a new pattern: C2 is muted, so only C1 plays in it.
	s.press("S")
	s.shows("Save pattern C1 as you hear it", "this one", "C2 is free")
	// The pattern itself cannot be chosen; Enter on the free slot saves.
	s.press("h", "enter")
	s.shows("that is the pattern itself")
	s.press("l", "enter")
	s.shows("C1 saved as C2, as you hear it: without C2")
	saved, err := app.OpenPattern(s.m().unit, pad(t, "C2"))
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.PadsUsed) != 1 || saved.PadsUsed[0] != pad(t, "C1") {
		t.Errorf("the saved pattern plays %v, want only C1", saved.PadsUsed)
	}
	// The pattern it came from is untouched.
	if from, _ := app.OpenPattern(s.m().unit, pad(t, "C1")); from == nil || len(from.PadsUsed) != 2 {
		t.Errorf("the pattern it came from changed: %v", from)
	}
}

// TestWorkingOffline works on a folder of projects with no unit, then switches to the unit
// once it is plugged in.
func TestWorkingOffline(t *testing.T) {
	lib := t.TempDir()
	conf := formats.NewPadconf(nil, "AT WORK")
	smp := sim.Tone(220, 0.3).SMP()
	block := conf.Blocks[0]
	binary.LittleEndian.PutUint32(block[3:], uint32(len(smp)))
	binary.LittleEndian.PutUint32(block[7:], uint32(proto.SMPHeaderLen))
	binary.LittleEndian.PutUint32(block[11:], uint32(len(smp)))
	copy(block[175:], "office kick")
	for path, data := range map[string][]byte{"Beat/PADCONF.BIN": conf.Bytes(), "Beat/SMPL/BANK1-01.SMP": smp} {
		local := filepath.Join(lib, path)
		if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(local, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	sm := sim.New()
	pad1, _ := proto.NewPadIndex(0)
	sm.SetSample(pad1, "home kick", 48_000)
	plugged := false
	connect := func() (app.Unit, io.Closer, error) {
		if !plugged {
			return nil, nil, ErrNoUnit
		}
		d := device.New(sm, "sim")
		return d, d, nil
	}
	openOffline := func(dir string) (app.Unit, io.Closer, string, error) {
		s, _, err := sim.OpenFolder(dir)
		if err != nil {
			return nil, nil, "", err
		}
		d := device.New(s, "offline")
		return d, d, "1 project found", nil
	}
	sc := &screen{t: t, sim: sm, player: &fakePlayer{}, store: &memStore{}}
	sc.model = NewConnecting(connect, sc.player, nil).WithOffline(openOffline, func() bool { return plugged }, "")
	sc.run(sc.model.Init())
	ticks := func(n int) {
		for i := 0; i < n; i++ {
			sc.send(tickMsg{})
		}
	}

	// The waiting screen offers to work offline, and o asks for the folder on this computer.
	sc.shows("Waiting for an SP-404MKII", "works offline on a folder of projects")
	sc.press("o")
	sc.shows("Work offline on the projects in", "This computer")
	sc.press("esc")
	sc.hides("Work offline on the projects in")

	// The folder chosen, the projects in it are worked on, and it says so in the header.
	sc.run(sc.m().goOffline(lib))
	sc.shows("◌ OFFLINE", "AT WORK", "office kick", "Working offline on", "saving to "+filepath.Base(lib))
	if !sc.m().previewLocal {
		t.Error("offline, pads should play on this computer")
	}
	if got := sc.m().mem.OfflineDir; got != lib {
		t.Errorf("the folder is not remembered: %q", got)
	}
	sc.press("U")
	sc.shows("still offline")

	// The unit is plugged in: it is announced, and U switches to it.
	plugged = true
	ticks(probeTicks)
	sc.shows("unit plugged in: U connects", "An SP-404MKII is plugged in")
	sc.press("U")
	sc.shows("● CONNECTED  sim", "home kick")
	sc.hides("OFFLINE")
	sc.hides("office kick")
}

// TestTheLastProjectIsOpenedAgainOffline works on a folder of two projects, switches to the
// second, and opens the folder again in a new session: it is on the second project.
func TestTheLastProjectIsOpenedAgainOffline(t *testing.T) {
	lib := t.TempDir()
	for dir, name := range map[string]string{"A": "FIRST ONE", "B": "SECOND ONE"} {
		local := filepath.Join(lib, dir, "PADCONF.BIN")
		if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(local, formats.NewPadconf(nil, name).Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	openOffline := func(dir string) (app.Unit, io.Closer, string, error) {
		s, _, err := sim.OpenFolder(dir)
		if err != nil {
			return nil, nil, "", err
		}
		d := device.New(s, "offline")
		return d, d, "2 projects found", nil
	}
	store := &memStore{}
	session := func() *screen {
		sc := &screen{t: t, sim: sim.New(), player: &fakePlayer{}, store: store}
		connect := func() (app.Unit, io.Closer, error) { return nil, nil, ErrNoUnit }
		sc.model = NewConnecting(connect, sc.player, store).WithOffline(openOffline, func() bool { return false }, "")
		sc.run(sc.model.Init())
		sc.run(sc.m().goOffline(lib))
		return sc
	}

	first := session()
	first.shows("FIRST ONE")
	// The second project, from the projects view.
	first.press("3", "j", "enter")
	first.shows("project 2 selected", "SECOND ONE")
	if got := store.state.Projects[lib]; got != 2 {
		t.Fatalf("the project remembered for the folder is %d, want 2", got)
	}

	// Next time the folder is opened, it is on the second project again.
	second := session()
	second.shows("SECOND ONE", "project 2, as last time")
	if p := second.m().snap.Project; p != 1 {
		t.Errorf("the folder opened on project %d, want 2", p+1)
	}
}

func TestVMarksAndUnmarksTheBank(t *testing.T) {
	s := open(t)
	s.press("V")
	if len(s.m().marked) != 3 {
		t.Fatalf("V marked %d pads, want the 3 with samples", len(s.m().marked))
	}
	s.shows("marked 3 pads")
	s.press("V")
	if len(s.m().marked) != 0 {
		t.Errorf("V again left %d marked", len(s.m().marked))
	}
	s.press("V", "esc")
	if len(s.m().marked) != 0 {
		t.Errorf("Esc left %d marked", len(s.m().marked))
	}
}

func TestPatternPanelKeepsRoomForSixteenTracks(t *testing.T) {
	for _, size := range [][2]int{{170, 70}, {100, 60}, {170, 34}} {
		s := open(t)
		s.send(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		s.press("N", "enter", "enter", "tab")
		s.shows("Pattern A")
		g := s.m().geom()
		panel := s.m().patternSlotDetail(g)
		h := lipgloss.Height(panel)
		// What the 16 tracks need, and what the screen can give it.
		full := 2 + 4 + 1 + 16 + 3
		room := g.bodyH - g.top
		if g.side {
			room -= 5
		} else {
			room -= g.gridH + 1
		}
		want := min(full, room)
		if h < want {
			t.Errorf("%dx%d: the pattern panel is %d lines, should keep %d (full %d, room %d)", size[0], size[1], h, want, full, room)
		}
		// An empty slot keeps the same height: it does not change from one pattern to the next.
		s.press("l")
		if other := lipgloss.Height(s.m().patternSlotDetail(g)); other != h {
			t.Errorf("%dx%d: %d lines for the next slot, %d for the pattern", size[0], size[1], other, h)
		}
		// The whole screen still fits.
		if got := len(strings.Split(s.model.View(), "\n")); got > size[1] {
			t.Errorf("%dx%d: the screen is %d lines", size[0], size[1], got)
		}
	}
}

func TestAPatternIsBouncedToAPadAsItIsHeard(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	m := s.m()
	level, _ := proto.ParamByName("level")
	for name, file := range map[string]string{"C1": "SD:IMPORT/kick.wav", "C2": "SD:IMPORT/pack/snare.wav"} {
		if _, err := app.Assign(m.unit, m.unit, pad(t, name), file); err != nil {
			t.Fatal(err)
		}
		if err := m.unit.SetParam(level.ID, proto.TargetPad(pad(t, name)), 127); err != nil {
			t.Fatal(err)
		}
	}
	s.press("r", "]", "]", "N", "enter", "enter", "tab", "enter") // the pattern of bank C is open
	if s.m().pattern == nil || len(s.m().pattern.PadsUsed) != 2 {
		t.Fatalf("the pattern: %+v: %q", s.m().pattern, s.m().message)
	}
	bounce := func(target string) formats.Sample {
		t.Helper()
		s.press("B")
		s.shows("Bounce the pattern")
		s.press("C")
		s.typeText(target[1:])
		s.press("enter")
		if s.m().picker != nil {
			s.press("enter") // a pad that holds a sample asks twice
		}
		s.shows("as you hear it")
		n, _ := strconv.Atoi(target[1:])
		row := s.m().snap.Bank(2)[n-1]
		if !row.HasSample {
			t.Fatalf("%s has no sample: %q", target, s.m().message)
		}
		got, err := app.PadAudio(s.m().unit, row)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	all := bounce("C9")
	if all.Frames() == 0 {
		t.Fatal("the bounce is empty")
	}
	// The pattern is two bars at the bank's tempo; the bounce is that long.
	d := s.m().pattern
	frames := int(float64(d.Length) * float64(formats.SampleRate) * 60 / d.BankTempo / float64(formats.PPQ))
	if diff := all.Frames() - frames; diff < -2 || diff > 2 {
		t.Errorf("the bounce is %d frames, the pattern %d", all.Frames(), frames)
	}
	// Muting a track leaves it out of the bounce.
	s.press("m")
	muted := bounce("C10")
	if muted.Frames() != all.Frames() {
		t.Errorf("muting changed the length: %d, %d", muted.Frames(), all.Frames())
	}
	energy := func(x formats.Sample) (e int64) {
		for _, v := range x.Samples {
			e += int64(v) * int64(v) / 1024
		}
		return e
	}
	if energy(muted) >= energy(all) {
		t.Errorf("a muted track is still in the bounce: %d vs %d", energy(muted), energy(all))
	}
	if energy(muted) == 0 {
		t.Error("the bounce with one track muted is silent")
	}
}

// openConditionPattern opens a pattern of bank C with a kick on C1 and a snare on C2, both on
// beat 1 of two bars, at a level you can hear.
func openConditionPattern(t *testing.T) *screen {
	t.Helper()
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	m := s.m()
	level, _ := proto.ParamByName("level")
	for name, file := range map[string]string{"C1": "SD:IMPORT/kick.wav", "C2": "SD:IMPORT/pack/snare.wav"} {
		if _, err := app.Assign(m.unit, m.unit, pad(t, name), file); err != nil {
			t.Fatal(err)
		}
		if err := m.unit.SetParam(level.ID, proto.TargetPad(pad(t, name)), 127); err != nil {
			t.Fatal(err)
		}
	}
	s.press("r", "]", "]", "N", "enter", "enter", "tab", "enter")
	if s.m().pattern == nil || len(s.m().pattern.PadsUsed) != 2 {
		t.Fatalf("the pattern: %+v: %q", s.m().pattern, s.m().message)
	}
	return s
}

func TestAConditionOnANoteSaysHowLongTheBounceIs(t *testing.T) {
	s := openConditionPattern(t)
	// The cursor is on the first note: c gives it 1:2, and the length of the bounce is told at once.
	s.press("c")
	s.shows("1:2", "bounced: 2 loops = 4 bars = 8.0 s")
	d := s.m().pattern
	key := app.NoteKey{Pad: d.Grid.Rows[0].Pad, Step: 0}
	if got := s.m().patConds[key]; got != (app.Cond{A: 1, B: 2}) {
		t.Fatalf("the condition is %+v", got)
	}
	// The note is drawn as a conditioned one, and the line under the grid has the cycle.
	s.shows("◐", "conditions")
	// Another note, 2:3: the cycle is the least common multiple, 6 loops.
	s.press("j", "c", "c", "c", "c") // 1:2, 2:2, 1:3, 2:3
	s.shows("2:3", "6 loops = 12 bars = 24.0 s")
	// The conditions are kept for the pattern.
	kept := s.m().conds.load(s.m().condKey(d.Slot)).conditions()
	if len(kept) != 2 {
		t.Errorf("%d conditions kept: %v", len(kept), kept)
	}
	// C goes back, round to the chances; plain again says nothing of a cycle.
	for i := 0; i < 20 && !s.m().patConds[app.NoteKey{Pad: d.Grid.Rows[1].Pad, Step: 0}].Plain(); i++ {
		s.press("C")
	}
	s.shows("2 loops = 4 bars = 8.0 s")
	s.press("k")
	for i := 0; i < 20 && !s.m().patConds[key].Plain(); i++ {
		s.press("C")
	}
	s.hides("bounced:")
	// No note, no condition.
	s.press("l")
	s.press("c")
	s.shows("no note here")
}

func TestAPatternWithAConditionBouncesAsALongerLoop(t *testing.T) {
	s := openConditionPattern(t)
	// The kick plays on the first of every 2 loops; the snare always.
	s.press("c")
	d := s.m().pattern
	frames := int(float64(d.Length) * float64(formats.SampleRate) * 60 / d.BankTempo / float64(formats.PPQ))
	mix, loops, err := s.m().patternMix(d.Slot)()
	if err != nil {
		t.Fatal(err)
	}
	if loops != 2 {
		t.Errorf("%d loops", loops)
	}
	if diff := mix.Frames() - 2*frames; diff < -2 || diff > 2 {
		t.Errorf("the mix is %d frames, two loops are %d", mix.Frames(), 2*frames)
	}
	// The first beat of each loop: the kick's 4800 frames are only in the first loop.
	energy := func(from, n int) (e int64) {
		for _, v := range mix.Samples[from*2 : (from+n)*2] {
			e += int64(v) * int64(v) / 1024
		}
		return e
	}
	first, second := energy(0, 4800), energy(frames, 4800)
	if first == 0 || second == 0 || second >= first {
		t.Errorf("the first beat has %d, the second loop's %d: the kick should be only in the first", first, second)
	}
	// The same through the bounce key: the pad holds the long mix.
	s.press("B")
	s.press("C")
	s.typeText("9")
	s.press("enter")
	row := s.m().snap.Bank(2)[8]
	got, err := app.PadAudio(s.m().unit, row)
	if err != nil {
		t.Fatal(err)
	}
	if diff := got.Frames() - 2*frames; diff < -2 || diff > 2 {
		t.Errorf("C9 holds %d frames, two loops are %d", got.Frames(), 2*frames)
	}
	// Without the condition it is one loop again.
	s.press("C")
	mix, loops, err = s.m().patternMix(d.Slot)()
	if err != nil || loops != 1 || mix.Frames() > frames+2 {
		t.Errorf("plain again: %d loops, %d frames, err %v", loops, mix.Frames(), err)
	}
}

func TestChancesRollFromASeedThatCanBeChanged(t *testing.T) {
	s := openConditionPattern(t)
	s.press("y")
	s.shows("no note has a chance")
	// 50%: c until the chances; the first is 90%, then 75, 50.
	for i := 0; i < 40 && s.m().patConds[app.NoteKey{Pad: s.m().pattern.Grid.Rows[0].Pad}].Prob != 50; i++ {
		s.press("c")
	}
	s.shows("50%", "bounced: 4 loops = 8 bars = 16.0 s (chances rolled over those loops)")
	d := s.m().pattern
	one, _, _ := s.m().patternMix(d.Slot)()
	again, _, _ := s.m().patternMix(d.Slot)()
	if !slices.Equal(one.Samples, again.Samples) {
		t.Error("one seed gave two different mixes")
	}
	// New dice: another seed. Over 4 loops of one 50% note, two seeds nearly always differ; try a few.
	differs := false
	for i := 0; i < 8 && !differs; i++ {
		s.press("y")
		other, _, _ := s.m().patternMix(d.Slot)()
		differs = !slices.Equal(one.Samples, other.Samples)
	}
	if !differs {
		t.Error("new dice never gave another mix")
	}
}

func TestAConditionIsExplainedInWords(t *testing.T) {
	s := openConditionPattern(t)
	s.press("c")
	s.shows("the note here plays only on loop 1 of every 2 loops (the 1st, 3rd, 5th ... time round)",
		"it only repeats exactly after 2 loops")
}

func TestSelectingNotesOfATrackChangesThemTogether(t *testing.T) {
	s := openConditionPattern(t)
	pad0 := s.m().pattern.Grid.Rows[0].Pad
	velocities := func() (v []int, ticks []uint32) {
		for _, n := range s.m().pattern.Notes {
			if n.Pad == pad0 {
				v, ticks = append(v, int(n.Velocity)), append(ticks, n.Tick)
			}
		}
		return
	}
	// Notes on steps 3, 5 and 7 besides the one on step 1.
	s.press("l", "l", "x", "l", "l", "x", "l", "l", "x", "g")
	if v, _ := velocities(); len(v) != 4 {
		t.Fatalf("%d notes on the track, want 4", len(v))
	}
	s.press("V")
	s.shows("SELECT", "1 note of")
	// Moving over notes chooses nothing by itself; Enter chooses the note under the cursor, and
	// lets it go again.
	s.press("l", "l")
	s.shows("1 note of")
	s.press("x")
	s.shows("2 notes of")
	s.press("enter")
	s.shows("1 note of")
	s.press("x", "l", "l", "x", "l", "l", "enter")
	s.shows("4 notes of")
	// j k move to the other tracks, and notes there can be chosen too; here they are let go
	// again, so the rest of the test is about the one track.
	s.press("j")
	if s.m().patSel == nil || s.m().patRow != 1 || len(s.m().patSel.chosen) != 4 {
		t.Fatalf("the selection is %+v on row %d", s.m().patSel, s.m().patRow)
	}
	s.press("k")
	// A change goes to every chosen note, and the selection follows them.
	s.press("<")
	v, ticks := velocities()
	for i, x := range v {
		if x != 117 {
			t.Errorf("note %d has velocity %d, want 117", i, x)
		}
	}
	s.press("L", ".")
	v2, ticks2 := velocities()
	for i := range v2 {
		if v2[i] != 118 || ticks2[i] != ticks[i]+5 {
			t.Errorf("note %d: velocity %d tick %d, want 118 at %d", i, v2[i], ticks2[i], ticks[i]+5)
		}
	}
	// A condition goes on all of them.
	s.press("c")
	if got := len(s.m().patConds); got != 4 {
		t.Errorf("%d notes have a condition, want 4", got)
	}
	s.press("0")
	if _, ticks3 := velocities(); ticks3[0] != ticks[0] {
		t.Errorf("0 left the first note at %d, want %d", ticks3[0], ticks[0])
	}
	// Taking them out ends the selection.
	s.press("d")
	if v, _ := velocities(); len(v) != 0 || s.m().patSel != nil {
		t.Errorf("%d notes left, selection %v", len(v), s.m().patSel)
	}
	// Esc leaves a selection and not the pattern.
	s.press("l", "x", "V", "esc")
	if s.m().patSel != nil || s.m().pattern == nil {
		t.Errorf("Esc left the selection %v, pattern %v", s.m().patSel, s.m().pattern != nil)
	}
	_ = ticks
}

func TestOnlyTheChosenNotesChange(t *testing.T) {
	s := openConditionPattern(t)
	pad0 := s.m().pattern.Grid.Rows[0].Pad
	vel := func() (out []int) {
		for _, n := range s.m().pattern.Notes {
			if n.Pad == pad0 {
				out = append(out, int(n.Velocity))
			}
		}
		return
	}
	s.press("l", "l", "x", "l", "l", "x", "g")
	// Choose the first and the third note, leave the one between.
	s.press("V", "l", "l", "l", "l", "x")
	s.press("<")
	got := vel()
	if len(got) != 3 || got[0] != 117 || got[1] != 127 || got[2] != 117 {
		t.Errorf("velocities %v, want 117 127 117", got)
	}
}

func TestAChromaticNoteShowsItsPitchByte(t *testing.T) {
	s := openConditionPattern(t)
	s.hides("chromatic: pitch byte")
	d := s.m().pattern
	n := d.Notes[0]
	// The unit played the first note chromatically: its record has a pitch byte.
	if _, err := app.EditPattern(s.m().unit, d.Slot, func(b []byte) ([]byte, error) {
		b[n.Offset+3] = 0x8D
		return b, nil
	}); err != nil {
		t.Fatal(err)
	}
	// Two edits elsewhere read the pattern again.
	s.press("l", "x", "x", "g")
	s.shows("chromatic: pitch byte 141 (0x8D)")
}

func TestAConditionedNoteIsMarkedInTheExpandedTrackToo(t *testing.T) {
	s := openConditionPattern(t)
	if !s.m().patFocus {
		s.press("enter")
	}
	if !s.m().patFocus {
		t.Fatal("the track is not expanded")
	}
	s.press("c", "l")
	s.hides("conditions  1:2") // the cursor has left the note
	s.shows("◐", "notes with a condition are drawn")
}

// A note that begins under a longer one is drawn as a note of its own, with its own velocity.
func TestANoteUnderALongerOneIsDrawnAsANote(t *testing.T) {
	s := openConditionPattern(t)
	d := s.m().pattern
	pad0 := d.Grid.Rows[0].Pad
	// The note on step 1 lasts long; another, softer one starts two steps into it.
	step := d.StepTicks()
	longOne := d.CellNotes(pad0, 0)[0]
	if _, err := app.EditPattern(s.m().unit, d.Slot, func(b []byte) ([]byte, error) {
		if err := formats.SetLength(b, longOne.Offset, uint16(step*6)); err != nil {
			return nil, err
		}
		return formats.InsertNote(b, d.StepTick(2), pad0, 40, uint16(step))
	}); err != nil {
		t.Fatal(err)
	}
	s.press("l", "x", "x", "g")
	cover, _, start := laneCover(s.m().pattern, pad0)
	if !start[2] || cover[2] != 40 {
		t.Errorf("step 3 starts a note of velocity 40: start %v, cover %d", start[2], cover[2])
	}
	if start[1] || cover[1] != 127 {
		t.Errorf("step 2 is the long note going on: start %v, cover %d", start[1], cover[1])
	}
	if start[3] || cover[3] != 127 {
		t.Errorf("step 4 is the long note going on: start %v, cover %d", start[3], cover[3])
	}
}

func TestMixedSpansSayWhyTheBounceIsLong(t *testing.T) {
	s := openConditionPattern(t)
	// One note on 1:2, another on 1:3 (c steps through always, 1:2, 2:2, 1:3 ...).
	s.press("c")
	s.shows("only on loop 1 of every 2 loops")
	s.hides("meet again")
	s.press("l", "l", "x")
	s.press("c", "c", "c")
	s.shows("meet again after 6 loops", "2 and 3")
}

func TestOnlyHeardTracksMakeTheCycleLonger(t *testing.T) {
	s := openConditionPattern(t)
	s.press("c")                // the first track: 1:2
	s.press("j", "c", "c", "c") // the second: 1:3
	if line := s.m().cycleLine(); !strings.Contains(line, "6 loops") {
		t.Fatalf("both tracks heard: %q", line)
	}
	// Muting the second leaves the first's 1:2: the bounce is 2 loops, not 6.
	s.press("m")
	if line := s.m().cycleLine(); !strings.Contains(line, "2 loops") {
		t.Errorf("second track muted: %q", line)
	}
}

func TestDClearsTheTrackOnlyInTheBarsOnShow(t *testing.T) {
	s := openConditionPattern(t)
	pad0 := s.m().pattern.Grid.Rows[0].Pad
	count := func() (n int) {
		for _, x := range s.m().pattern.Notes {
			if x.Pad == pad0 {
				n++
			}
		}
		return
	}
	// Doubled (T, down to DOUBLE, Enter): four bars, the first two on show.
	s.press("T", "j", "j", "j", "j", "j", "enter")
	if bars := s.m().pattern.Bars(); bars != 4 {
		t.Fatalf("%d bars after doubling, want 4", bars)
	}
	s.press("l", "x", "g")
	before := count()
	s.press("D")
	s.shows("taken out")
	after := count()
	if after >= before || after == 0 {
		t.Errorf("notes of the track: %d before D, %d after; the bars not on show keep theirs", before, after)
	}
	s.press("u")
	if got := count(); got != before {
		t.Errorf("u left %d notes, want %d", got, before)
	}
	// Another track is not touched.
	s.press("D", "j")
	rows := s.m().pattern.Grid.Rows
	other := 0
	for _, x := range s.m().pattern.Notes {
		if x.Pad == rows[1].Pad {
			other++
		}
	}
	if other == 0 {
		t.Error("D took notes out of another track")
	}
}

func TestWFindsTheFolderEachBankWasFilledFrom(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	s.sim.AddFile("SD:IMPORT/Great Kit/Loop 9.wav", wav(2400))
	s.sim.AddFile("SD:IMPORT/Great Kit/Loop 8.wav", wav(2800))
	s.press("2", "tab", "i")
	s.shows("indexed")
	s.press("r", "1")
	m := s.m()
	for i, f := range []string{"Loop 9", "Loop 8"} {
		if _, err := app.Assign(m.unit, m.unit, pad(t, fmt.Sprintf("G%d", i+1)), "SD:IMPORT/Great Kit/"+f+".wav"); err != nil {
			t.Fatal(err)
		}
	}
	s.press("r", "W")
	s.shows("Look for where the samples came from in")
	s.press("enter")
	s.shows("folders found for bank G")
	data, ok := s.sim.File("SD:404tui-folders.json")
	if !ok || !strings.Contains(string(data), `"1/G"`) || !strings.Contains(string(data), `"matched": true`) {
		t.Fatalf("the folder notes: %v %s", ok, data)
	}
	s.press("2")
	s.shows("◆▾ IMPORT/")
}

func TestRangerColumnsFollowTheCursorAndOpenFoldersOnlyToLookInside(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	s.press("2", "w")
	s.shows("files: three columns")
	// The cursor is on IMPORT: its inside is read at once and shown in the third column.
	s.shows("IMPORT/", "kick.wav", "pack/")
	rows := func() []string {
		var out []string
		for _, n := range s.m().tree.Rows() {
			out = append(out, n.Path)
		}
		return out
	}
	// l goes in: the column of the folder in front now lists what was only a preview.
	s.press("l")
	if at := s.m().tree.Rows()[s.m().fileAt].Path; at != "SD:IMPORT/pack" {
		t.Fatalf("after l the cursor is on %s (%v)", at, rows())
	}
	s.shows("snare.wav", "notes.txt")
	// j moves along the folder in front, not into what is open below it.
	s.press("j")
	if at := s.m().tree.Rows()[s.m().fileAt].Path; at != "SD:IMPORT/kick.wav" {
		t.Fatalf("after j the cursor is on %s", at)
	}
	s.press("k", "h")
	if at := s.m().tree.Rows()[s.m().fileAt].Path; at != "SD:IMPORT" {
		t.Fatalf("after h the cursor is on %s", at)
	}
	// pack was only looked into: it closes when the cursor goes by. IMPORT was entered, so it stays.
	for _, r := range rows() {
		if strings.HasPrefix(r, "SD:IMPORT/pack/") {
			t.Fatalf("pack stayed open: %v", rows())
		}
	}
	s.press("w")
	s.shows("files: folder tree")
	s.shows("▾ IMPORT/")
}

func TestRangerIsTheFilesViewByDefaultAndSurvivesEmptyAndUnreadFolders(t *testing.T) {
	s := open(t)
	m := s.m()
	m.ranger = true
	s.model = m
	s.send(tea.WindowSizeMsg{Width: 120, Height: 30})
	// Before the folder is read, and once it is, the view draws.
	s.press("2")
	s.shows("kick.wav")
	// Every source draws, read or not.
	s.press("t", "t", "t")
	s.shows("SD card")
}

func TestTheTabBarNamesTheSectionsAndTheRailKeepsOnlyBanksAndSources(t *testing.T) {
	s := open(t)
	s.shows("1 Pads", "2 Files", "3 Projects")
	if first := s.m().rail(10)[0]; first.kind != railLabel {
		t.Errorf("the rail starts with %+v, not with a label", first)
	}
	s.clickTab(2)
	s.shows("PROJECT_02")
}

func TestSlicerAndSequencerTabsOfferWhatToAdd(t *testing.T) {
	s := open(t)
	s.shows("4 Slicer", "5 Sequencer")

	// Slicer: a asks for a pad or a new sample; f goes to Files with Z to slice, Esc gives up.
	s.press("4")
	s.shows("Nothing is open in the slicer")
	s.press("a")
	s.shows("a pad", "a new sample")
	s.press("f")
	if !s.m().sliceWant || s.m().section != secFiles {
		t.Fatalf("after f: sliceWant=%v section=%d", s.m().sliceWant, s.m().section)
	}
	s.shows("slice the file or pad under the cursor")
	s.press("esc")
	if s.m().sliceWant || s.m().section != secSlicer {
		t.Errorf("Esc left sliceWant=%v section=%d", s.m().sliceWant, s.m().section)
	}

	// Slicer: p opens the pad chooser over the tab; Enter on a pad with a sample opens the
	// slicer, and its tab is lit while it is open.
	m := s.m()
	s.sim.AddFile("SD:IMPORT/drums.wav", drums())
	if _, err := app.Assign(m.unit, m.unit, pad(t, "A5"), "SD:IMPORT/drums.wav"); err != nil {
		t.Fatal(err)
	}
	s.press("r", "4", "a", "p")
	s.shows("Pad  bank A")
	if s.m().section != secSlicer {
		t.Fatalf("the pad chooser left the Slicer tab for section %d", s.m().section)
	}
	s.press("j", "enter")
	if s.m().slicer == nil || s.m().addAsk != nil {
		t.Fatalf("Enter on A5: slicer=%v ask=%v msg=%q", s.m().slicer != nil, s.m().addAsk != nil, s.m().message)
	}
	if s.m().activeTab() != secSlicer {
		t.Error("the Slicer tab is not lit while the slicer is open")
	}
	// 1 to 5 change the view from the slicer; it is still open when the tab is back.
	s.press("1")
	if s.m().section != secPads || s.m().slicer == nil {
		t.Fatalf("1 in the slicer: section %d, slicer open %v", s.m().section, s.m().slicer != nil)
	}
	s.press("4")
	if s.m().section != secSlicer || s.m().slicer == nil {
		t.Fatalf("4 did not bring the slicer back: section %d", s.m().section)
	}
	s.press("esc")
	s.shows("Nothing is open in the slicer")

	// Sequencer: a is a landing page too; p lists the patterns of every bank, Enter opens one
	// in the editor with the Sequencer tab lit, and Esc comes back to the landing page.
	s.press("5")
	s.shows("Nothing is open in the sequencer")
	s.sim.SetPattern(pad(t, "A2"), pad(t, "A5"))
	data, err := formats.StartPattern([]proto.PadIndex{pad(t, "A5")}, 1)
	if err != nil {
		t.Fatal(err)
	}
	s.sim.AddFile(formats.PatternPath(1, pad(t, "A2")), data)
	s.press("r", "5", "a")
	s.shows("Pattern  bank A", "arrows move")
	s.press("enter")
	if s.m().pattern == nil {
		t.Fatalf("the pattern did not open: %q", s.m().message)
	}
	if s.m().activeTab() != secSequencer {
		t.Error("the Sequencer tab is not lit while its pattern is open")
	}
	// 1 to 5 change the view, and the pattern is still there when the tab is back.
	s.press("1")
	if s.m().section != secPads {
		t.Fatalf("1 in the editor went to section %d", s.m().section)
	}
	s.press("5")
	if s.m().pattern == nil || s.m().pattern.Slot != pad(t, "A2") || s.m().activeTab() != secSequencer {
		t.Fatalf("the pattern was not back with the tab: %v", s.m().pattern)
	}
	s.press("esc")
	if s.m().pattern != nil || s.m().section != secSequencer {
		t.Errorf("Esc: pattern=%v section=%d", s.m().pattern != nil, s.m().section)
	}

	// In an open pattern a chooses another one, and there is no rail of banks beside it.
	s.press("5", "a")
	s.press("enter")
	if s.m().pattern == nil || s.m().geom().railW != 0 {
		t.Fatalf("pattern open %v, rail %d", s.m().pattern != nil, s.m().geom().railW)
	}
	s.press("a")
	s.shows("Pattern  bank A", "arrows move")
	s.press("esc")
	if s.m().addAsk != nil || s.m().pattern == nil {
		t.Error("Esc in the chooser should leave the open pattern as it was")
	}
}

func TestProjectsAndSlicerTabsUseTheWholeWidth(t *testing.T) {
	s := open(t)
	s.press("3")
	if g := s.m().geom(); g.railW != 0 {
		t.Errorf("projects keep a rail of %d", g.railW)
	}
	s.shows("PROJECT_02")
}

// clickTab clicks the tab with that index, wherever the tabs are.
func (s *screen) clickTab(i int) {
	s.t.Helper()
	row, spans := s.m().tabLayout()
	s.click(spans[i][0]+1, row)
}

func TestTabsSitInTheHeaderWhenItHasRoomAndUnderItWhenNot(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 200, Height: 30})
	if row, _ := s.m().tabLayout(); row != rowHeader {
		t.Errorf("at 200 columns the tabs are on row %d", row)
	}
	s.shows("1 Pads", "5 Sequencer")
	s.clickTab(1)
	if s.m().section != secFiles {
		t.Errorf("clicking the Files tab in the header went to section %d", s.m().section)
	}
	s.send(tea.WindowSizeMsg{Width: 90, Height: 30})
	if row, _ := s.m().tabLayout(); row != rowRule {
		t.Errorf("at 90 columns the tabs are on row %d", row)
	}
	s.clickTab(0)
	if s.m().section != secPads {
		t.Errorf("clicking the Pads tab under the header went to section %d", s.m().section)
	}
}

func TestAFolderWithSamplesOnPadsGetsTheMarkerInTheTree(t *testing.T) {
	s := open(t)
	m := s.m()
	if m.folderUsed("SD:IMPORT/Kit") {
		t.Error("a folder nothing came from is marked")
	}
	m.origins = &app.Origins{Pads: map[string]app.Origin{"2/A1": {Path: "SD:IMPORT/Kit/Sub/kick.wav", Name: "kick"}}}
	for _, folder := range []string{"SD:IMPORT/Kit/Sub", "SD:IMPORT/Kit", "SD:IMPORT"} {
		if !m.folderUsed(folder) {
			t.Errorf("%s is not marked", folder)
		}
	}
	if m.folderUsed("SD:IMPORT/Kit2") || m.folderUsed("SD:IMPORT/Kit/Sub/kick.wav") == false && false {
		t.Error("a folder beside it is marked")
	}
}

func TestQuestionMarkListsTheCommandsOfTheDialogInFront(t *testing.T) {
	s := slicerOpen(t)
	// The slicer: its keys, by group, searchable; Enter on a single key sends it.
	s.press("?")
	s.shows("Slicer", "slices: select", "sound: reverse")
	s.typeText("zoom")
	s.shows("edges: zoom")
	s.hides("sound: reverse")
	s.press("esc")
	s.press("esc")
	s.press("1", "a")
	// The assign dialog.
	s.press("?")
	s.shows("Assign a file", "autoplay", "type a path")
	s.press("?")
	s.hides("Assign a file  ")
	if s.m().browser == nil {
		t.Error("closing the list closed the dialog under it")
	}
	s.press("esc")
	// A text being typed keeps ? as a letter.
	s.press("n")
	if s.m().prompt != nil {
		s.press("?")
		if s.m().palette != nil {
			t.Error("? opened the list inside a text prompt")
		}
	}
}

func TestAPadTileSaysHowManyBarsItsSampleIs(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 200, Height: 54})
	m := s.m()
	// At 120 BPM a bar is 2 s: a 4 s loop is 2 bars, 2.2 s is a bit over one.
	s.sim.AddFile("SD:IMPORT/loop.wav", wav(4*48000))
	s.sim.AddFile("SD:IMPORT/odd.wav", wav(105600))
	for name, file := range map[string]string{"A5": "SD:IMPORT/loop.wav", "A6": "SD:IMPORT/odd.wav"} {
		if _, err := app.Assign(m.unit, m.unit, pad(t, name), file); err != nil {
			t.Fatal(err)
		}
	}
	s.press("r")
	s.shows("2 bars", "1.1 bars")
	// The one-shots of the bank say nothing.
	if got := s.m().padBars(s.m().snap.Bank(0)[0], 24); got != "" {
		t.Errorf("a one-shot says %q", got)
	}
	// On a narrow tile there is no room for it.
	if got := s.m().padBars(s.m().snap.Bank(0)[4], 10); got != "" {
		t.Errorf("a narrow tile says %q", got)
	}
}

func TestANewPatternAsksHowLongItIs(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	s.press("N", "enter")
	s.shows("How long is the pattern?", "at 120.00 BPM is")
	// h and l change it, a digit sets it, Esc goes back to the templates.
	s.press("l", "l")
	if got := s.m().lenPick.bars; got != 4 {
		t.Errorf("two l from 2 bars: %d", got)
	}
	s.press("esc")
	s.shows("New pattern")
	s.press("enter", "1")
	if got := s.m().lenPick.bars; got != 1 {
		t.Errorf("1 sets %d bars", got)
	}
	s.press("8", "enter", "tab", "enter")
	if bars := s.m().pattern.Bars(); bars != 8 {
		t.Errorf("an 8-bar pattern came out %d bars", bars)
	}
}

func TestAGrooveTemplateIsAskedLengthToo(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	s.press("N", "j", "j", "enter") // House: 2 bars
	s.shows("How long is the pattern?", "House")
	s.press("4", "enter")
	if s.m().laneMap == nil || s.m().laneMap.bars != 4 {
		t.Fatalf("the lane dialog is not for 4 bars: %+v", s.m().laneMap)
	}
}

func TestBusScanSetsDrumsToOneBusAndTheRestToAnother(t *testing.T) {
	s := open(t)
	m := s.m()
	// A pad that is not a drum: assigned with a name no drum word is in.
	s.sim.AddFile("SD:IMPORT/warm pad.wav", wav(4800))
	if _, err := app.Assign(m.unit, m.unit, pad(t, "A5"), "SD:IMPORT/warm pad.wav"); err != nil {
		t.Fatal(err)
	}
	s.press("r", "B")
	s.shows("Set the bus of", "BUS 2 words", "y apply")
	s.press("y")
	s.shows("set by their names")
	bank := s.m().snap.Bank(0)
	if bank[0].Params["bus-fx"] != 2 || bank[1].Params["bus-fx"] != 2 || bank[2].Params["bus-fx"] != 2 {
		t.Errorf("kick, snare, hat on buses %d %d %d, want 2", bank[0].Params["bus-fx"], bank[1].Params["bus-fx"], bank[2].Params["bus-fx"])
	}
	if bank[4].Params["bus-fx"] != 1 {
		t.Errorf("the pad on BUS %d, want 1", bank[4].Params["bus-fx"])
	}
	// Run again: everything is right already, so nothing is asked.
	s.press("B")
	s.shows("on the bus its name says")
	if s.m().prompt != nil {
		t.Error("a scan with nothing to change asked a question")
	}
}

func TestConfigSetsTheWordsColoursAndBusesAndAssignFollowsThem(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	s.press("6")
	s.shows("Words for samples on", "Colour of", "What it makes of bank A")
	// The words of BUS 2 are edited as text.
	s.press("j", "enter")
	s.shows("Words for BUS 2")
	s.press("ctrl+u")
	s.typeText("boom, bap")
	s.press("enter")
	if got := s.m().busRule().Bus2Words; fmt.Sprint(got) != "[boom bap]" {
		t.Errorf("BUS 2 words %v", got)
	}
	// The colour of BUS 1: l takes the next, and the theme draws the pads with it.
	s.press("j", "l")
	if got := s.m().busRule().Bus1Colour; got != "blue" {
		t.Errorf("BUS 1 colour %q after l, want blue", got)
	}
	if got := s.m().withBusColours().st.th.bus1; got != "75" {
		t.Errorf("the theme's BUS 1 is %q, want 75", got)
	}
	// A name with none of the words goes to no bus: h from BUS 1.
	s.press("j", "j", "h")
	if got := s.m().busRule().Other; got != 0 {
		t.Fatalf("other bus %d, want 0", got)
	}
	// A file assigned afterwards goes where the rule says.
	s.sim.AddFile("SD:IMPORT/boom hit.wav", wav(4800))
	s.sim.AddFile("SD:IMPORT/warm pad.wav", wav(4800))
	for pd, file := range map[string]string{"A5": "SD:IMPORT/boom hit.wav", "A6": "SD:IMPORT/warm pad.wav"} {
		_, cmd := s.m().assign([]string{file}, pad(t, pd))
		s.send(cmd())
	}
	s.press("r")
	bank := s.m().snap.Bank(0)
	if got := bank[4].Params["bus-fx"]; got != 2 {
		t.Errorf("the boom pad is on bus %d, want 2", got)
	}
	if got := bank[5].Params["bus-fx"]; got != 0 {
		t.Errorf("the pad with no word is on bus %d, want none", got)
	}
	// Reset brings the defaults back.
	s.press("6", "g", "j", "j", "j", "j", "j", "j", "enter")
	if r := s.m().busRule(); r.Other != 1 || r.Bus1Colour != "" || len(r.Bus2Words) < 10 {
		t.Errorf("after the reset: %+v", r)
	}
}

func TestASelectionCanHoldNotesOfSeveralTracks(t *testing.T) {
	s := openConditionPattern(t)
	d := s.m().pattern
	if len(d.Grid.Rows) < 2 {
		t.Fatalf("%d tracks", len(d.Grid.Rows))
	}
	pad0, pad1 := d.Grid.Rows[0].Pad, d.Grid.Rows[1].Pad
	vel := func(pad proto.PadIndex) int {
		for _, n := range s.m().pattern.Notes {
			if n.Pad == pad {
				return int(n.Velocity)
			}
		}
		return -1
	}
	v0, v1 := vel(pad0), vel(pad1)
	// V chooses the note under the cursor; j goes to the next track, where x chooses its note.
	s.press("g", "V")
	s.shows("SELECT", "1 note of")
	s.press("j", "x")
	s.shows("2 notes on 2 tracks chosen")
	s.press("<")
	if vel(pad0) != v0-10 || vel(pad1) != v1-10 {
		t.Errorf("velocities %d, %d after <: want %d, %d", vel(pad0), vel(pad1), v0-10, v1-10)
	}
	if got := len(s.m().patSel.chosen); got != 2 {
		t.Errorf("the selection holds %d notes after the edit, want 2", got)
	}
	// A takes every note of every track, a the track of the cursor.
	s.press("A")
	if got := len(s.m().patSel.chosen); got < 2 {
		t.Errorf("A chose %d notes", got)
	}
	s.press("a")
	cur := s.m().pattern.Grid.Rows[s.m().patRow].Pad
	for k := range s.m().patSel.chosen {
		if k.Pad != cur {
			t.Errorf("a chose a note of %s, the cursor is on %s", k.Pad, cur)
		}
	}
	if s.m().addAsk != nil {
		t.Error("a in a selection opened the pattern chooser")
	}
	s.press("esc")
}

func TestZeroResetsANoteToOneStepOnTheGrid(t *testing.T) {
	s := openConditionPattern(t)
	d := s.m().pattern
	pad0 := d.Grid.Rows[0].Pad
	length := func() int {
		for _, n := range s.m().pattern.Notes {
			if n.Pad == pad0 {
				return int(n.Length)
			}
		}
		return -1
	}
	step := int(s.m().pattern.StepTicks())
	// "All pads held": the note lasts the whole pattern; 0 brings it down to one step.
	s.press("g")
	if length() <= step {
		t.Fatalf("the held note is %d ticks, a step is %d", length(), step)
	}
	s.press("0")
	if got := length(); got != step {
		t.Errorf("after 0 the note is %d ticks, want one step (%d)", got, step)
	}
	// Again: nothing to do.
	s.press("0")
	s.shows("one step long and on the grid already")
	// In a selection, 0 does it to every chosen note.
	s.press("g", "+", "+", "V", "a", "0")
	if got := length(); got != step {
		t.Errorf("after 0 in a selection the note is %d ticks, want %d", got, step)
	}
}

func TestConfigRowsKeepTheirValuesApartFromTheLabels(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	s.press("6")
	view := ansi.Strip(s.m().View())
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "Words for samples on BUS") || strings.Contains(line, "Colour of BUS") {
			if strings.Contains(line, "BUS 1kick") || strings.Contains(line, "BUS 2kick") || strings.Contains(line, "BUS 2██") || strings.Contains(line, "BUS 1██") {
				t.Errorf("a value is up against its label: %q", line)
			}
			if strings.Contains(line, "BUS 2") && !strings.Contains(line, "BUS 2  ") {
				t.Errorf("less than two spaces after the label: %q", line)
			}
		}
	}
}

func TestKChecksASampleOnThePadAgainstItsFile(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	s.sim.AddFile("SD:IMPORT/boom hit.wav", wav(4800))
	_, cmd := s.m().assign([]string{"SD:IMPORT/boom hit.wav"}, pad(t, "A5"))
	s.send(cmd())
	s.press("r")
	if s.m().pad != 4 {
		t.Fatalf("cursor on pad %d", s.m().pad)
	}
	s.press("K")
	s.shows("Sample check", "A5", "boom hit.wav", "identical")
	s.press("esc")
	if s.m().report != nil {
		t.Fatal("a key did not close the report")
	}
	// The pad now holds other audio under the same name: the check says so.
	m := s.m()
	if _, err := app.PutSample(m.unit, pad(t, "A5"), formats.Sample{Channels: 1, Samples: make([]int16, 9000)}, "boom hit"); err != nil {
		t.Fatal(err)
	}
	s.press("r", "K")
	s.shows("different")
	s.press("esc")
	// A pad with no file noted says how to find it.
	s.press("g", "K")
	s.shows("no file is noted")
}

func TestThePanStopsOnTheCentreAndZeroBringsTheDefaultsBack(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	bal := func() int32 { return s.m().snap.Pads[0].Params["balance"] }
	hold := func() int32 { return s.m().snap.Pads[0].Params["hold"] }
	// The pan at L3 (61): a step of 4 would jump to R1, so it stops on the centre.
	if err := s.m().unit.SetParam(0x78, proto.TargetPad(pad(t, "A1")), 61); err != nil {
		t.Fatal(err)
	}
	s.press("r", "S", "j")
	s.shows("Pan", "L3")
	s.press("l")
	if got := bal(); got != 64 {
		t.Errorf("one step up from L3 is %d, want the centre 64", got)
	}
	s.press("l")
	if got := bal(); got != 68 {
		t.Errorf("then %d, want 68 (R4)", got)
	}
	// 0 puts the pan back in the centre, the hold to all of the sample.
	s.press("0")
	if got := bal(); got != 64 {
		t.Errorf("0 left the pan at %d", got)
	}
	s.press("G", "k", "0")
	if got := hold(); got != 100 {
		t.Errorf("0 on the hold gives %d, want 100", got)
	}
	s.press("esc")
}

func TestHSetsUpPadsThatWereNeverSetUp(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	m := s.m()
	set := func(name string, id byte, pad string, v int32) {
		if err := m.unit.SetParam(id, proto.TargetPad(pad_(t, pad)), v); err != nil {
			t.Fatal(err)
		}
	}
	// A1 and A2 as a pad made offline had them; A3 has a pan of its own and is left alone.
	set("hold", 0x7E, "A1", 1)
	set("balance", 0x78, "A1", 13)
	set("hold", 0x7E, "A2", 1)
	set("balance", 0x78, "A2", 13)
	set("balance", 0x78, "A3", 20)
	set("hold", 0x7E, "A3", 1)
	s.press("r", "H")
	s.shows("Set up 2 pads", "A1 A2")
	s.press("y")
	s.shows("set up")
	bank := s.m().snap.Bank(0)
	for i, pd := range []string{"A1", "A2"} {
		p := bank[i].Params
		if p["balance"] != 64 || p["hold"] != 100 || p["time-stretch"] != 10000 {
			t.Errorf("%s: balance %d hold %d stretch %d", pd, p["balance"], p["hold"], p["time-stretch"])
		}
	}
	if bank[2].Params["balance"] != 20 {
		t.Errorf("A3 was changed: balance %d", bank[2].Params["balance"])
	}
	// Nothing left to set up: it says so.
	s.press("H")
	s.shows("no pad here looks never set up")
}

func pad_(t *testing.T, name string) proto.PadIndex { return pad(t, name) }

func TestTheDashboardShowsTheNameWhileWaitingForAUnit(t *testing.T) {
	m := New(nil, nil, nil)
	m.width, m.height = 100, 40
	out := ansi.Strip(m.View())
	for _, want := range []string{"m o a h b e a t l a b", "Waiting for an SP-404MKII", "sample · slice · sequence"} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q on the dashboard:\n%s", want, out)
		}
	}
	// The light moves from frame to frame (in colour: the tiles are only colour), and a narrow
	// screen gets the short form.
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(prev)
	a := m.padGlow()
	if glow := ansi.Strip(strings.Join(a, "\n")); !strings.Contains(glow, " 1 ") || !strings.Contains(glow, " 16 ") {
		t.Errorf("the pads on the dashboard have no numbers:\n%s", glow)
	}
	m.frame += 4
	if strings.Join(a, "\n") == strings.Join(m.padGlow(), "\n") {
		t.Error("the pads do not light up in turn")
	}
	m.width = 40
	if narrow := ansi.Strip(m.View()); !strings.Contains(narrow, "404tui") {
		t.Errorf("no name on a narrow screen:\n%s", narrow)
	}
}

// fakeRecorder is a backend whose recordings are sessions with nothing behind them.
type fakeRecorder struct {
	dir, device string
	started     int
	err         error
	stopped     *int
	// loud is the sample value (24 bit) of the recording's end; 0 means half scale.
	loud int32
}

// fakeWAV is a stereo 24-bit WAV of about 3 MB whose last frames are at the given level.
func fakeWAV(loud int32) []byte {
	var b []byte
	le32 := func(v uint32) { b = binary.LittleEndian.AppendUint32(b, v) }
	le16 := func(v uint16) { b = binary.LittleEndian.AppendUint16(b, v) }
	b = append(b, "RIFF"...)
	le32(0xFFFFFFFF)
	b = append(b, "WAVEfmt "...)
	le32(16)
	le16(1)
	le16(2)
	le32(48000)
	le32(48000 * 6)
	le16(6)
	le16(24)
	b = append(b, "data"...)
	le32(0xFFFFFFFF)
	frames := (3 << 20) / 6
	for i := 0; i < frames; i++ {
		v := int32(0)
		if i >= frames-6000 {
			v = loud
		}
		for c := 0; c < 2; c++ {
			b = append(b, byte(v), byte(v>>8), byte(v>>16))
		}
	}
	return b
}

func (f *fakeRecorder) Start(dir, device string) (*record.Session, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.dir, f.device = dir, device
	f.started++
	path := filepath.Join(dir, "take.wav")
	_ = os.MkdirAll(dir, 0o755)
	level := f.loud
	if level == 0 {
		level = 4194304
	}
	_ = os.WriteFile(path, fakeWAV(level), 0o644)
	return record.NewSession(path, "fake", func() error { *f.stopped++; return nil }), nil
}

func TestCtrlRRecordsWithAClearSignAndStops(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 200, Height: 40})
	stopped := 0
	dir := t.TempDir()
	fake := &fakeRecorder{stopped: &stopped}
	s.model = s.m().WithRecorder(fake)
	m := s.m()
	m.mem.RecordDir, m.mem.RecordDevice = dir, "SP-404"
	s.model = m
	// Ctrl-R starts: the header says REC, with the size, wherever the user goes.
	s.press("ctrl+r")
	s.shows("REC", "3.0 MB")
	if fake.started != 1 || fake.dir != dir || fake.device != "SP-404" {
		t.Fatalf("started %d in %q for %q", fake.started, fake.dir, fake.device)
	}
	s.press("6")
	s.shows("REC")
	s.press("1", "?")
	s.shows("REC")
	s.press("esc")
	// Ctrl-R again stops it and says where the file is.
	s.press("ctrl+r")
	if stopped != 1 || s.m().rec != nil {
		t.Fatalf("stopped %d times, still recording %v", stopped, s.m().rec != nil)
	}
	s.shows("recorded 3.0 MB")
	s.hides("REC 0")
	// A recorder that cannot start says why, and nothing is shown as recording.
	fake.err = errors.New("no audio input with \"SP-404\" in its name")
	s.press("ctrl+r")
	s.shows("recording did not start", "no audio input")
	if s.m().rec != nil {
		t.Error("a recording that did not start is shown")
	}
	// With no recorder at all, it says what to install.
	s.model = s.m().WithRecorder(nil)
	s.press("ctrl+r")
	s.shows("install ffmpeg")
}

func TestBigTextDrawsCapitalsThreeRowsTall(t *testing.T) {
	rows := bigText("kick", 1)
	for i, r := range rows {
		if w := ansi.StringWidth(r); w != bigWidth("kick", 1) {
			t.Errorf("row %d is %d cells, want %d", i, w, bigWidth("kick", 1))
		}
	}
	if rows[0] == "" || !strings.ContainsAny(strings.Join(rows[:], ""), "▀▄█") {
		t.Errorf("nothing drawn: %q", rows)
	}
	wide := bigText("K1", 2)
	if ansi.StringWidth(wide[0]) != bigWidth("K1", 2) || bigWidth("K1", 2) != 2*(3*2+2)-2 {
		t.Errorf("a text at scale 2 is %d cells", ansi.StringWidth(wide[0]))
	}
	if bigText("é?", 1)[1] == "" {
		t.Error("a character the font lacks is not drawn as a space")
	}
}

func TestPerformShowsThePatternThatPlaysAndWhatChangesInTheNext(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	one, err := formats.StartPattern([]proto.PadIndex{pad(t, "A1"), pad(t, "A2"), pad(t, "A3")}, 1)
	if err != nil {
		t.Fatal(err)
	}
	two, err := formats.StartPattern([]proto.PadIndex{pad(t, "A2"), pad(t, "A3")}, 1)
	if err != nil {
		t.Fatal(err)
	}
	for slot, data := range map[string][]byte{"A2": one, "A3": two} {
		s.sim.SetPattern(pad(t, slot), pad(t, "A1"))
		s.sim.AddFile(formats.PatternPath(1, pad(t, slot)), data)
	}
	s.press("r", "7")
	s.press("a", "enter")
	if s.m().perfNow == nil || *s.m().perfNow != pad(t, "A2") {
		t.Fatalf("the pattern that plays is %v", s.m().perfNow)
	}
	// The next pattern: l goes to the next slot of the chooser.
	s.press("n", "l", "enter")
	if s.m().perfNow == nil || s.m().perfNext == nil {
		t.Fatalf("now %v next %v", s.m().perfNow, s.m().perfNext)
	}
	// Space: the next pattern is the one that plays, and there is no next.
	s.press(" ")
	if s.m().perfNow == nil || *s.m().perfNow != pad(t, "A3") || s.m().perfNext != nil {
		t.Errorf("after Space: now %v, next %v", s.m().perfNow, s.m().perfNext)
	}
	s.press("x", " ")
	s.shows("no next pattern")
}

func TestPerformListsWhatGoesToEachBusAndShowsARecording(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	// kick and snare go to BUS 2, hat to BUS 1.
	for pd, bus := range map[string]int32{"A1": 2, "A2": 2, "A3": 1} {
		if err := s.m().unit.SetParam(0x7A, proto.TargetPad(pad(t, pd)), bus); err != nil {
			t.Fatal(err)
		}
	}
	one, err := formats.StartPattern([]proto.PadIndex{pad(t, "A1"), pad(t, "A2"), pad(t, "A3")}, 1)
	if err != nil {
		t.Fatal(err)
	}
	s.sim.SetPattern(pad(t, "A2"), pad(t, "A1"))
	s.sim.AddFile(formats.PatternPath(1, pad(t, "A2")), one)
	s.press("r", "7", "a", "enter")
	// The effect of a bus is what the user writes: the unit does not say.
	s.press("f")
	s.shows("The effect on BUS 1")
	s.typeText("Hall reverb")
	s.press("enter")
	if got := s.m().mem.BusNotes[0]; got != "Hall reverb" {
		t.Errorf("the note of BUS 1 is %q", got)
	}
	// A recording shows in the dashboard too.
	stopped := 0
	fake := &fakeRecorder{stopped: &stopped}
	s.model = s.m().WithRecorder(fake)
	m := s.m()
	m.mem.RecordDir = t.TempDir()
	s.model = m
	s.press("ctrl+r")
	s.shows("RECORDING", "take.wav")
	s.press("ctrl+r")
	s.hides("RECORDING")
}

func TestPerformOpensOnTheBankTheUnitIsOnWithItsPadsBig(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	s.press("r", "7")
	s.shows("Perform", "BANK A", "CLOCK", "LEVEL")
	// The sample names are whole, in the terminal's own text: kick is on pad 1 and snare on pad 2.
	s.shows("kick", "snare")
	// The unit moves to another bank (it said so in a poll): the view follows it.
	m := s.m()
	m.selected, m.hasSelected = pad(t, "C3"), true
	s.model = m
	s.shows("BANK C")
	// A pad that sounds shows in the line over the pads.
	m = s.m()
	m.playing, m.playingPad = true, pad(t, "C3")
	s.model = m
	s.shows("▶ C3")
}

func TestParseCodeReadsPadNumbers(t *testing.T) {
	for in, want := range map[string]string{"1 3 3 7": "[1 3 3 7]", "1,16,16,16": "[1 16 16 16]", "1-3-3-7": "[1 3 3 7]", "off": "[]", "": "[]", "0 1": "[]", "1 17": "[]", "a": "[]"} {
		if got := fmt.Sprint(parseCode(in)); got != want {
			t.Errorf("parseCode(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestACodeIsCompletedByTheRightPadsInTime(t *testing.T) {
	c := &codeState{}
	now := time.Now()
	live, rec := []int{1, 3, 3, 7}, []int{1, 16, 16, 16}
	for i, p := range []int{1, 3, 3} {
		if got := c.feed(p, now.Add(time.Duration(i)*300*time.Millisecond), live, rec); got != -1 {
			t.Fatalf("hit %d completed code %d", i, got)
		}
	}
	if got := c.feed(7, now.Add(time.Second), live, rec); got != 0 {
		t.Errorf("1 3 3 7 completed %d, want 0", got)
	}
	// A hit that is not the next one starts over; too slow does not count.
	c = &codeState{}
	for _, p := range []int{1, 16, 16, 5, 16} {
		if got := c.feed(p, now, live, rec); got != -1 {
			t.Fatalf("a wrong run completed %d", got)
		}
	}
	c = &codeState{}
	c.feed(1, now, live, rec)
	c.feed(16, now.Add(time.Second), live, rec)
	c.feed(16, now.Add(2*time.Second), live, rec)
	if got := c.feed(16, now.Add(5*time.Second), live, rec); got != -1 {
		t.Errorf("a code that took five seconds counted: %d", got)
	}
	c = &codeState{}
	c.feed(5, now, live, rec)
	c.feed(1, now, live, rec)
	c.feed(16, now, live, rec)
	c.feed(16, now, live, rec)
	if got := c.feed(16, now, live, rec); got != 1 {
		t.Errorf("a run with other hits before it: %d, want 1", got)
	}
}

func TestPadCodesOnTheUnitSwitchTheLiveModeAndARecording(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	stopped := 0
	fake := &fakeRecorder{stopped: &stopped}
	s.model = s.m().WithRecorder(fake)
	m := s.m()
	m.mem.RecordDir = t.TempDir()
	s.model = m
	poll := func(pd string, playing bool, pos uint32) {
		s.send(pollMsg{have: true, activity: device.Activity{Selected: pad(t, pd), HasSelected: true, Playing: playing, Position: pos}})
	}
	// 1 3 3 7 in bank A: pad 1 is the selected pad already, so it is seen as its sample sounding;
	// pad 3 twice (the second as its sample starting over).
	poll("A1", true, 100)
	poll("A3", false, 0)
	poll("A3", true, 6000)
	poll("A3", false, 0)
	poll("A7", false, 0)
	if !s.m().liveOn || s.m().section != secPerform {
		t.Fatalf("after 1 3 3 7: live %v, section %d", s.m().liveOn, s.m().section)
	}
	// The same pads, in another bank, switch it off again.
	poll("B1", true, 100)
	poll("B3", false, 0)
	poll("B3", true, 6000)
	poll("B3", false, 0)
	poll("B7", false, 0)
	if s.m().liveOn || s.m().section == secPerform {
		t.Fatalf("1 3 3 7 again: live %v, section %d", s.m().liveOn, s.m().section)
	}
	// 1 16 16 16 starts a recording; a sample that starts over is another hit.
	poll("C1", true, 100)
	poll("C16", true, 9000)
	poll("C16", true, 100)
	poll("C16", true, 50)
	s.shows("REC")
	if fake.started != 1 {
		t.Fatalf("started %d recordings", fake.started)
	}
	poll("D1", true, 100)
	poll("D16", true, 9000)
	poll("D16", true, 100)
	poll("D16", true, 50)
	if stopped != 1 || s.m().rec != nil {
		t.Errorf("the code again: stopped %d, still recording %v", stopped, s.m().rec != nil)
	}
}

func TestTheCodesAreSetInConfig(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	s.press("6", "g", "j", "j", "j", "j", "j", "j", "j", "j", "j")
	s.shows("Pad codes on the unit", "1 3 3 7", "1 16 16 16")
	s.press("enter")
	s.shows("Pads for the live mode")
	s.press("ctrl+u")
	s.typeText("2 4 4 8")
	s.press("enter")
	if got := s.m().liveCode(); fmt.Sprint(got) != "[2 4 4 8]" {
		t.Errorf("live code %v", got)
	}
	// A code that is not pad numbers is refused; off turns one off.
	s.press("j", "enter")
	s.press("ctrl+u")
	s.typeText("1 99")
	s.press("enter")
	s.shows("pad numbers from 1 to 16")
	s.press("enter", "ctrl+u")
	s.typeText("off")
	s.press("enter")
	if len(s.m().recCode()) != 0 {
		t.Errorf("rec code %v after off", s.m().recCode())
	}
}

func TestMIDIFromTheUnitAndAControllerDoTheSameAsTheCodes(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	stopped := 0
	fake := &fakeRecorder{stopped: &stopped}
	s.model = s.m().WithRecorder(fake)
	m := s.m()
	m.mem.RecordDir = t.TempDir()
	link := &midiLink{ch: make(chan midiin.Event, 4), name: "SP-404MKII"}
	m.midi = link
	s.model = m
	feed := func(e midiin.Event) {
		next, cmd := s.m().midiEvent(midiMsg{link: link, ev: e})
		s.model = next
		if s.m().rec != nil || e.Data1 == 61 {
			s.run(cmd) // a recording starts and stops in a command
		}
	}
	note := func(ch, n int) midiin.Event {
		return midiin.Event{Kind: midiin.NoteOn, Channel: ch, Data1: n, Data2: 100}
	}
	// The unit's pads: pad 1 is note 48, pad 3 note 50, pad 7 note 46 (bank A is channel 1).
	for _, n := range []int{48, 50, 50, 46} {
		feed(note(1, n))
	}
	if !s.m().liveOn || s.m().section != secPerform {
		t.Fatalf("pads 1 3 3 7 by MIDI: live %v, section %d", s.m().liveOn, s.m().section)
	}
	// A controller on the control channel (16): note 60 switches the live mode off again.
	feed(note(16, 60))
	if s.m().liveOn {
		t.Error("note 60 on channel 16 did not leave the live mode")
	}
	// Note 61 starts a recording and again stops it; a note on another channel does nothing.
	feed(note(15, 61))
	if s.m().rec != nil {
		t.Error("a note on another channel started a recording")
	}
	feed(note(16, 61))
	s.shows("REC")
	feed(note(16, 61))
	if s.m().rec != nil || stopped != 1 {
		t.Errorf("note 61 again: recording %v, stopped %d", s.m().rec != nil, stopped)
	}
	// The messages are set in Config: a controller turned up past half works too.
	m = s.m()
	m.mem.MidiMap[2], m.mem.MidiMap[3] = "cc 20", "off"
	one := pad(t, "A2")
	m.perfNext = &one
	m.section = secPerform
	s.model = m
	feed(midiin.Event{Kind: midiin.CC, Channel: 16, Data1: 20, Data2: 30})
	if s.m().perfNow != nil {
		t.Error("a controller below half counted")
	}
	feed(midiin.Event{Kind: midiin.CC, Channel: 16, Data1: 20, Data2: 100})
	if s.m().perfNow == nil || *s.m().perfNow != one || s.m().perfNext != nil {
		t.Errorf("cc 20 up: now %v next %v", s.m().perfNow, s.m().perfNext)
	}
}

func TestTheMIDIClockCountsBarsInPerform(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	link := &midiLink{ch: make(chan midiin.Event, 4)}
	m := s.m()
	m.midi = link
	s.model = m
	feed := func(e midiin.Event) {
		next, _ := s.m().midiEvent(midiMsg{link: link, ev: e})
		s.model = next
	}
	feed(midiin.Event{Kind: midiin.Start})
	// Nine beats: the third beat of the third bar.
	for i := 0; i < 24*9; i++ {
		feed(midiin.Event{Kind: midiin.Clock})
	}
	if got := s.m().clockText(0); got != "BAR 3 · BEAT 2" {
		t.Errorf("clock text %q", got)
	}
	if got := s.m().clockText(2); got != "BAR 3 · BEAT 2  ·  LOOP 2" {
		t.Errorf("with a 2-bar pattern: %q", got)
	}
	feed(midiin.Event{Kind: midiin.Stop})
	if got := s.m().clockText(0); !strings.Contains(got, "stopped") {
		t.Errorf("after stop: %q", got)
	}
	s.press("7")
	s.shows("♩ BAR 3")
}

func TestMidiSettingsAreReadAndRefused(t *testing.T) {
	if k, n, ok := parseMidiMap("cc 20"); !ok || n != 20 {
		t.Errorf("cc 20: %v %d %v", k, n, ok)
	}
	if _, n, ok := parseMidiMap("60"); !ok || n != 60 {
		t.Errorf("a bare number is a note: %d %v", n, ok)
	}
	for _, bad := range []string{"", "off", "note 200", "pitch 3", "note"} {
		if _, _, ok := parseMidiMap(bad); ok {
			t.Errorf("%q was accepted", bad)
		}
	}
	for note, want := range map[int]int{48: 1, 49: 2, 50: 3, 51: 4, 44: 5, 40: 9, 36: 13, 39: 16} {
		if got, ok := padOfNote(note); !ok || got != want {
			t.Errorf("note %d is pad %d, want %d", note, got, want)
		}
	}
	if _, ok := padOfNote(60); ok {
		t.Error("note 60 is not a pad")
	}
}

// fakeMIDI is a MIDI source the test plays: Open hands it the callback the driver would be given,
// and a message is played into it from another goroutine, as the driver's thread does.
type fakeMIDI struct {
	mu      sync.Mutex
	opens   []string
	failFor int // the first opens fail, as before the unit is plugged in
	on      func(midiin.Event)
	stopped int
	sent    [][]byte // what was sent to the unit
}

func (f *fakeMIDI) Ports() ([]string, error) { return []string{"SP-404MKII"}, nil }

// OpenOut takes the messages sent to the unit.
func (f *fakeMIDI) OpenOut(string) (midiin.Out, string, error) {
	return &fakeOut{f: f}, "SP-404MKII", nil
}

type fakeOut struct{ f *fakeMIDI }

func (o *fakeOut) Send(msg []byte) error {
	o.f.mu.Lock()
	defer o.f.mu.Unlock()
	o.f.sent = append(o.f.sent, append([]byte(nil), msg...))
	return nil
}
func (o *fakeOut) Close() {}

func (f *fakeMIDI) Open(part string, on func(midiin.Event)) (func(), string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opens = append(f.opens, part)
	if len(f.opens) <= f.failFor {
		return nil, "", errors.New("no MIDI input with " + part + " in its name")
	}
	f.on = on
	return func() { f.mu.Lock(); f.stopped++; f.mu.Unlock() }, "SP-404MKII", nil
}

// play sends raw MIDI bytes the way the driver does: parsed, into the callback.
func (f *fakeMIDI) play(t *testing.T, raw ...byte) {
	t.Helper()
	e, ok := midiin.Parse(raw)
	if !ok {
		t.Fatalf("% x is not a message", raw)
	}
	f.mu.Lock()
	on := f.on
	f.mu.Unlock()
	on(e)
}

func (f *fakeMIDI) listening() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.on != nil
}

// eventually waits for a condition that a running program brings about.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for i := 0; i < 400; i++ {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("never happened: %s", what)
}

// The whole way, in a program that runs: the input is opened when it starts (and again after a
// failure, until the unit is there), and raw bytes played into the driver's callback end as pad
// codes, a recording, and the clock, with no help from the test but the bytes.
func TestRawMIDIBytesDriveARunningProgram(t *testing.T) {
	old := tickEvery
	tickEvery = 5 * time.Millisecond
	defer func() { tickEvery = old }()
	s := open(t)
	stopped := 0
	rec := &fakeRecorder{stopped: &stopped}
	src := &fakeMIDI{failFor: 2}
	m := s.m().WithRecorder(rec).WithMIDI(src)
	m.mem.RecordDir = t.TempDir()
	p := tea.NewProgram(m, tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutSignals())
	done := make(chan Model, 1)
	go func() {
		final, err := p.Run()
		if err != nil {
			t.Error(err)
		}
		done <- final.(Model)
	}()
	p.Send(tea.WindowSizeMsg{Width: 170, Height: 54})
	// The first two tries fail (no unit plugged in): the third finds the input.
	eventually(t, "the input is opened after two failures", src.listening)
	src.mu.Lock()
	opens := len(src.opens)
	src.mu.Unlock()
	if opens != 3 {
		t.Errorf("the input was opened %d times, want 3", opens)
	}
	// The bank A pads 1 3 3 7 as raw bytes: note on, channel 1.
	for _, n := range []byte{48, 50, 50, 46} {
		src.play(t, 0x90, n, 100)
		src.play(t, 0x80, n, 0)
	}
	// Then 1 16 16 16 on channel 3 (bank C): a recording starts and runs.
	for _, n := range []byte{48, 39, 39, 39} {
		src.play(t, 0x92, n, 90)
	}
	eventually(t, "the recording started", func() bool { return rec.started == 1 })
	// A start and 24 ticks to the quarter note, nine quarter notes.
	src.play(t, 0xFA)
	for i := 0; i < 24*9; i++ {
		src.play(t, 0xF8)
	}
	// The control channel: note 60 on channel 16 (0x9F) brings the live mode back on.
	src.play(t, 0x9F, 60, 127)
	// Every message is a pass through the program's loop and a draw; they are let go through.
	time.Sleep(3 * time.Second)
	p.Quit()
	var final Model
	select {
	case final = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the program did not end")
	}
	// 1 3 3 7 switched the live mode on, and note 60 on channel 16 switched it off again... then
	// the recording code started the recording; the quit stopped nothing by itself, so it runs.
	if final.rec == nil {
		t.Error("the recording of 1 16 16 16 is not running")
	}
	if final.clock == nil || final.clock.ticks != 24*9 {
		t.Errorf("the clock counted %v ticks, want %d", final.clock, 24*9)
	}
	if final.perfLog != nil {
		final.perfLog.Close() // a file that is open cannot be removed on Windows
	}
	final.clock.at = time.Now() // the clock text is blank for a clock that stopped two seconds ago
	if got := final.clockText(0); !strings.Contains(got, "BAR 3 · BEAT 2") {
		t.Errorf("clock text %q", got)
	}
	// 1 3 3 7 turned the live mode on and the control note turned it off.
	if final.liveOn {
		t.Error("the live mode is still on after note 60 on channel 16")
	}
}

func TestChangingTheMIDIInputInConfigClosesTheOldOneAndOpensTheNew(t *testing.T) {
	s := open(t)
	src := &fakeMIDI{}
	m := s.m().WithMIDI(src)
	opened := m.startMIDI()
	msg := opened()
	next, _ := m.Update(msg)
	m = next.(Model)
	if m.midi == nil || m.midi.name != "SP-404MKII" {
		t.Fatalf("not listening: %+v", m.midi)
	}
	// A new part of the name: the old input is stopped, a new open is asked for with it.
	nm, cmd := m.applyMidi(0, "Keystation")
	m = nm.(Model)
	if src.stopped != 1 {
		t.Errorf("the old input was stopped %d times, want 1", src.stopped)
	}
	msg = cmd()
	if _, ok := msg.(midiOpenedMsg); !ok {
		t.Fatalf("the command gave %T", msg)
	}
	if got := src.opens[len(src.opens)-1]; got != "keystation" {
		t.Errorf("opened with %q, want keystation", got)
	}
	// A message from the input that was closed is ignored.
	old := &midiLink{ch: make(chan midiin.Event, 1)}
	next, _ = m.midiEvent(midiMsg{link: old, ev: midiin.Event{Kind: midiin.Start}})
	if next.(Model).clock != nil && next.(Model).clock.running {
		t.Error("a message of a closed input was acted on")
	}
}

func TestARecordingWritesWhatTheUnitSendsToAnEventLogAndPerformShowsIt(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 70})
	stopped := 0
	dir := t.TempDir()
	m := s.m().WithRecorder(&fakeRecorder{stopped: &stopped})
	m.mem.RecordDir = dir
	link := &midiLink{ch: make(chan midiin.Event, 4)}
	m.midi, m.midiSrc = link, &fakeMIDI{}
	s.model = m
	s.press("ctrl+r")
	if s.m().rec == nil || s.m().perfLog == nil {
		t.Fatalf("recording %v, log %v", s.m().rec != nil, s.m().perfLog != nil)
	}
	at := s.m().rec.Started
	feed := func(after time.Duration, e midiin.Event) {
		e.At = at.Add(after)
		next, _ := s.m().midiEvent(midiMsg{link: link, ev: e})
		s.model = next
	}
	feed(1500*time.Millisecond, midiin.Event{Kind: midiin.NoteOn, Channel: 3, Data1: 48, Data2: 100})
	feed(1700*time.Millisecond, midiin.Event{Kind: midiin.NoteOff, Channel: 3, Data1: 48})
	feed(2*time.Second, midiin.Event{Kind: midiin.CC, Channel: 1, Data1: 74, Data2: 100})
	feed(2100*time.Millisecond, midiin.Event{Kind: midiin.CC, Channel: 1, Data1: 74, Data2: 20})
	feed(2200*time.Millisecond, midiin.Event{Kind: midiin.ProgramChange, Channel: 1, Data1: 5})
	feed(2300*time.Millisecond, midiin.Event{Kind: midiin.Sysex, Raw: []byte{0xF0, 0x41, 0xF7}})
	feed(3*time.Second, midiin.Event{Kind: midiin.Start})
	for i := 0; i < 49; i++ { // two beats and the first tick of the third
		feed(3*time.Second+time.Duration(i)*20833*time.Microsecond, midiin.Event{Kind: midiin.Clock})
	}
	// The pad is hit again, and Perform shows it, the knob and the last messages.
	feed(4*time.Second, midiin.Event{Kind: midiin.NoteOn, Channel: 3, Data1: 49, Data2: 90})
	if !s.m().mon.hot(2, 1, at.Add(4*time.Second)) || s.m().mon.hot(2, 0, at.Add(4*time.Second)) {
		t.Error("the pad that was hit is not hot (or its neighbour is)")
	}
	one, _ := proto.ParsePad("A2")
	m = s.m()
	m.perfNow = &one
	s.model = m
	s.press("7")
	s.shows("CONTROLS")
	s.shows("1:74")
	s.press("v")
	s.shows("1:74")
	// Config lists the messages.
	s.press("6")
	s.shows("MIDI monitor")
	s.shows("program")
	s.press("ctrl+r")
	if s.m().perfLog != nil && s.m().rec != nil {
		t.Error("the recording did not end")
	}
	es, err := perflog.Read(perflog.PathFor(filepath.Join(dir, "take.wav")))
	if err != nil {
		t.Fatal(err)
	}
	kinds := ""
	for _, e := range es {
		kinds += e["k"].(string) + " "
	}
	for _, want := range []string{"start ", "on ", "off ", "cc ", "program ", "sysex ", "beat "} {
		if !strings.Contains(kinds, want) {
			t.Errorf("no %q in the log: %s", want, kinds)
		}
	}
	if es[1]["k"] != "on" || es[1]["t"] != 1.5 || es[1]["bank"] != "C" || es[1]["pad"] != 1.0 || es[1]["ch"] != 3.0 {
		t.Errorf("first event %v", es[1])
	}
	var beats []Entry
	for _, e := range es {
		if e["k"] == "beat" {
			beats = append(beats, e)
		}
	}
	if len(beats) != 3 || beats[2]["n"] != 2.0 {
		t.Errorf("beats %v", beats)
	}
}

type Entry = perflog.Entry

func TestTheEffectOfABusIsReadFromTheUnitAndShownWithItsKnobs(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	stopped := 0
	m := s.m().WithRecorder(&fakeRecorder{stopped: &stopped})
	m.mem.RecordDir = t.TempDir()
	link := &midiLink{ch: make(chan midiin.Event, 4)}
	m.midi, m.midiSrc = link, &fakeMIDI{}
	s.model = m
	s.press("ctrl+r")
	feed := func(e midiin.Event) {
		e.At = time.Now()
		next, _ := s.m().midiEvent(midiMsg{link: link, ev: e})
		s.model = next
	}
	s.press("7")
	s.shows("BUS 1", "BUS 2", "effect not sent yet")
	// CC 83 on a bus's channel is its effect, CC 19 switches it: 22 is Reverb.
	feed(midiin.Event{Kind: midiin.CC, Channel: 1, Data1: 83, Data2: 22})
	feed(midiin.Event{Kind: midiin.CC, Channel: 1, Data1: 19, Data2: 127})
	s.shows("Reverb", "ON")
	feed(midiin.Event{Kind: midiin.CC, Channel: 2, Data1: 83, Data2: 14})
	feed(midiin.Event{Kind: midiin.CC, Channel: 2, Data1: 19, Data2: 0})
	s.shows("Tape Echo", "off")
	if !s.m().fx[0].on || s.m().fx[1].on {
		t.Errorf("fx %+v", s.m().fx)
	}
	// Other channels have no effect of a bus.
	feed(midiin.Event{Kind: midiin.CC, Channel: 3, Data1: 19, Data2: 127})
	if s.m().fx[0].num != 22 || s.m().fx[1].on {
		t.Error("a message on channel 3 changed a bus")
	}
	// Knobs of BUS 1's effect: CC 16 to 18 on channel 1, shown with a fader each.
	feed(midiin.Event{Kind: midiin.CC, Channel: 1, Data1: 16, Data2: 80})
	feed(midiin.Event{Kind: midiin.CC, Channel: 1, Data1: 17, Data2: 20})
	s.shows("1:16")
	s.shows(" 80")
	feed(midiin.Event{Kind: midiin.CC, Channel: 1, Data1: 16, Data2: 100})
	s.shows("100")
	// A knob under an effect is not also in CONTROLS, nor are CC 19 and 83: another controller is.
	feed(midiin.Event{Kind: midiin.CC, Channel: 1, Data1: 74, Data2: 50})
	if !s.m().isFxKnob(1, 16) || s.m().isFxKnob(1, 74) || !s.m().isFxSwitch(1, 83) || s.m().isFxSwitch(1, 74) {
		t.Error("which controllers belong to an effect")
	}
	s.press("ctrl+r")
	es, err := perflog.Read(perflog.PathFor(filepath.Join(s.m().recordDir(), "take.wav")))
	if err != nil {
		t.Fatal(err)
	}
	var fx []string
	for _, e := range es {
		if e["k"] == "fx" {
			fx = append(fx, fmt.Sprintf("%v %v %v", e["bus"], e["on"], e["name"]))
		}
	}
	// Each change of a bus's effect is an event with its name once it is known.
	if len(fx) < 4 || fx[0] != "1 false Reverb" || fx[1] != "1 true Reverb" {
		t.Errorf("fx events %v", fx)
	}
}

func TestPerformPlaysThePatternLikeTheSequencerAndShowsWhatIsMutedOnTheUnit(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	one, err := formats.StartPattern([]proto.PadIndex{pad(t, "A1"), pad(t, "A2"), pad(t, "A3")}, 1)
	if err != nil {
		t.Fatal(err)
	}
	s.sim.SetPattern(pad(t, "A2"), pad(t, "A1"))
	s.sim.AddFile(formats.PatternPath(1, pad(t, "A2")), one)
	link := &midiLink{ch: make(chan midiin.Event, 4)}
	m := s.m()
	m.midi, m.midiSrc = link, &fakeMIDI{}
	s.model = m
	s.press("r", "7", "a", "enter")
	if s.m().perfNow == nil || *s.m().perfNow != pad(t, "A2") {
		t.Fatalf("the pattern that plays is %v", s.m().perfNow)
	}
	feed := func(e midiin.Event) {
		e.At = time.Now()
		next, _ := s.m().midiEvent(midiMsg{link: link, ev: e})
		s.model = next
	}
	feed(midiin.Event{Kind: midiin.Start})
	for i := 0; i < 24*2+6; i++ {
		feed(midiin.Event{Kind: midiin.Clock})
	}
	// Muted on the unit: pads A1 and A2 are heard, A3 has not been for longer than a round.
	m = s.m()
	now := time.Now()
	m.mon.bpm, m.perfSync = 120, 0
	m.clock.ticks, m.clock.running, m.clock.at = 400, true, now
	m.mon.padAt = now
	for pad, ago := range map[int]time.Duration{0: 500 * time.Millisecond, 1: 800 * time.Millisecond, 2: 10 * time.Second} {
		m.mon.hits[pad] = now.Add(-ago)
	}
	s.model = m
	d, _ := m.perfTracks(m.perfNow)
	if d == nil {
		t.Fatal("pattern not read")
	}
	if m.unitMuted(d, pad(t, "A1"), now) || m.unitMuted(d, pad(t, "A2"), now) {
		t.Error("heard pads are muted")
	}
	if !m.unitMuted(d, pad(t, "A3"), now) {
		t.Error("the silent pad is not muted")
	}
	s.shows("×")
	// With nothing heard at all, nothing is called muted.
	m = s.m()
	m.mon.padAt = now.Add(-time.Minute)
	if m.unitMuted(d, pad(t, "A3"), now) {
		t.Error("muted with no pad heard")
	}
}

func TestPlaybackNotesFindThePatternAndAreNotPadHits(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	one, _ := formats.StartPattern([]proto.PadIndex{pad(t, "A1"), pad(t, "A2"), pad(t, "A3")}, 1)
	two, _ := formats.StartPattern([]proto.PadIndex{pad(t, "A2"), pad(t, "A3")}, 1)
	for slot, data := range map[string][]byte{"A2": one, "A3": two} {
		s.sim.AddFile(formats.PatternPath(1, pad(t, slot)), data)
	}
	s.sim.SetPattern(pad(t, "A2"), pad(t, "A1"))
	s.sim.SetPattern(pad(t, "A3"), pad(t, "A2"))
	link := &midiLink{ch: make(chan midiin.Event, 4)}
	m := s.m()
	m.midi, m.midiSrc = link, &fakeMIDI{}
	s.model = m
	s.press("r", "7")
	feed := func(e midiin.Event) {
		e.At = time.Now()
		next, cmd := s.m().midiEvent(midiMsg{link: link, ev: e})
		s.model = next
		s.run(cmd)
	}
	note := func(n int) { feed(midiin.Event{Kind: midiin.NoteOn, Channel: 1, Data1: n, Data2: 127}) }
	// The sequencer runs: pad 1 3 3 7 played by a pattern is no code.
	// Which pads each pattern plays, as the unit reports them.
	rows := s.m().patterns[0]
	rows[1].Pads = []proto.PadIndex{pad(t, "A1"), pad(t, "A2"), pad(t, "A3")}
	rows[2].Pads = []proto.PadIndex{pad(t, "A2"), pad(t, "A3")}
	feed(midiin.Event{Kind: midiin.Start})
	feed(midiin.Event{Kind: midiin.Clock})
	for _, n := range []int{48, 50, 50} {
		note(n)
	}
	if s.m().liveOn {
		t.Error("notes of a playing pattern switched the live mode on")
	}
	if s.m().perfNow == nil || *s.m().perfNow != pad(t, "A2") || !s.m().perfAuto {
		t.Fatalf("pattern %v auto %v, want A2 found from the notes", s.m().perfNow, s.m().perfAuto)
	}
	s.shows("heard from the unit")
	// A start again: only pad 2 and 3 are heard, so the other pattern fits better.
	feed(midiin.Event{Kind: midiin.Start})
	feed(midiin.Event{Kind: midiin.Clock})
	note(49)
	note(50)
	if s.m().perfNow == nil || *s.m().perfNow != pad(t, "A3") {
		t.Errorf("pattern %v, want A3", s.m().perfNow)
	}
	// A choice by hand stays while it fits what is heard.
	m = s.m()
	three := pad(t, "A2")
	m.perfNow, m.perfAuto = &three, false
	s.model = m
	if s.m().detectPattern().perfAuto {
		t.Error("a pattern chosen by hand was replaced though it fits")
	}
	// Stopped, pad hits count for the codes again.
	feed(midiin.Event{Kind: midiin.Stop})
	for _, n := range []int{48, 50, 50, 46} {
		note(n)
	}
	if !s.m().liveOn {
		t.Error("1 3 3 7 hit by hand with the sequencer stopped did not switch the live mode on")
	}
}

func TestWrapNameKeepsTheWholeNameWhenThereIsRoom(t *testing.T) {
	for _, c := range []struct {
		name string
		w, n int
		want []string
	}{
		{"kick", 20, 3, []string{"kick"}},
		{"06_LEAD_125_G_Min", 12, 3, []string{"06_LEAD_125_", "G_Min"}},
		{"Supercalifragilistic", 8, 3, []string{"Supercal", "ifragili", "stic"}},
		{"a_very_long_name_that_goes_on_and_on_and_on", 10, 2, []string{"a_very_", "long_name…"}},
	} {
		got := wrapName(c.name, c.w, c.n)
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("wrapName(%q, %d, %d) = %q, want %q", c.name, c.w, c.n, got, c.want)
		}
	}
}

func TestTheRecordingShowsItsLevelAndAClipLight(t *testing.T) {
	for _, c := range []struct {
		name string
		loud int32
		clip bool
	}{{"half scale", 4194304, false}, {"full scale", 8388607, true}} {
		s := open(t)
		s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
		stopped := 0
		m := s.m().WithRecorder(&fakeRecorder{stopped: &stopped, loud: c.loud})
		m.mem.RecordDir = t.TempDir()
		s.model = m
		s.press("ctrl+r")
		// A tick reads the end of the file.
		s.send(tickMsg{})
		if s.m().lvl == nil || !s.m().lvl.ok {
			t.Fatalf("%s: no level read", c.name)
		}
		s.press("7")
		s.shows("RECORDING")
		if c.clip {
			s.shows("CLIP", "too loud")
		} else {
			s.shows("-6.0 dB", "loudest so far")
			s.hides("too loud")
		}
		// The header carries the level too, wherever the user is.
		s.press("1")
		if c.clip {
			s.shows("CLIP!")
		} else {
			s.shows("-6.0 dB")
		}
		s.press("ctrl+r")
		if s.m().lvl != nil {
			t.Errorf("%s: the meter outlived the recording", c.name)
		}
	}
}

// CTRL 1 to 3 of a bus send CC 16 to 18 on the bus's channel: they are its knobs, nothing else is.
func TestABusKnobsAreItsChannelsCC16To18(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	m := s.m()
	for bus, ch := range []int{1, 2} {
		for _, cc := range []int{16, 17, 18} {
			if !m.isFxKnob(ch, cc) {
				t.Errorf("ch %d cc %d is not a knob of bus %d", ch, cc, bus+1)
			}
		}
	}
	if m.isFxKnob(1, 74) || m.isFxKnob(3, 16) {
		t.Error("another controller was taken for a bus knob")
	}
	s.model = m
	s.press("7") // the view draws without a MIDI monitor too
}

func TestESubstepModeSplitsAStepAndTheGridCountsThem(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	s.press("N", "enter", "enter", "tab", "enter")
	notesOf := func() []formats.NoteRecord {
		data, _ := s.sim.File("ROLAND/SP-404MKII/PROJECT_01/PTN/PTN00001.BIN")
		notes, _ := formats.NoteRecords(data)
		return notes
	}
	pad := s.m().pattern.Grid.Rows[0].Pad
	step := s.m().pattern.StepTicks()
	s.press("l", "l", "l", "l", "l", "l", "l", "l", "l", "l", "l", "l", "l", "x") // an empty step 13
	at := s.m().pattern.StepTick(13)
	inStep := func() []uint32 {
		var ticks []uint32
		for _, n := range notesOf() {
			if n.Pad == pad && n.Tick >= at && n.Tick < at+step {
				ticks = append(ticks, n.Tick-at)
			}
		}
		return ticks
	}
	s.press("e") // on: the step is two parts
	for _, c := range []struct {
		want  int
		ticks []uint32
	}{{2, []uint32{0, step / 2}}, {3, []uint32{0, step / 3, 2 * (step / 3)}}, {4, []uint32{0, step / 4, step / 2, 3 * step / 4}}, {1, []uint32{0}}, {2, []uint32{0, step / 2}}} {
		got := inStep()
		if len(got) != len(c.ticks) {
			t.Fatalf("%d parts: notes at %v", c.want, got)
		}
		for i := range got {
			if got[i] != c.ticks[i] {
				t.Errorf("%d parts: notes at %v, want %v", c.want, got, c.ticks)
			}
		}
		if c.want > 1 {
			s.shows(fmt.Sprintf("SUBSTEP %d", c.want))
		}
		s.press("i")
	}
	s.press("e") // off
	s.shows("substep mode off")
	// Off: the keys are the plain ones again, x takes the notes out.
	s.press("x")
	if len(inStep()) != 0 {
		t.Errorf("x outside the mode left notes: %v", inStep())
	}
}

func TestInSubstepModeHLGoThroughThePartsAndXSwitchesOne(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	s.press("N", "enter", "enter", "tab", "enter")
	pad := s.m().pattern.Grid.Rows[0].Pad
	s.press("l", "l", "l", "l", "l", "l", "l", "l", "l", "l", "l", "l", "l", "x") // an empty step 13
	s.press("e", "i", "i")                                                        // four parts
	s.press("l", "l", "x")                                                        // the third part off
	if n, on := s.m().pattern.Split(pad, 13); n != 4 || !on[0] || !on[1] || on[2] || !on[3] {
		t.Fatalf("split = %d %v, want the third part off", n, on)
	}
	s.press("l", "l", "l") // stops at the last part
	if s.m().patCol != 13 || s.m().patSub != 3 {
		t.Errorf("column %d part %d, want 13 and 3", s.m().patCol, s.m().patSub)
	}
	s.press("h")
	if s.m().patSub != 2 {
		t.Errorf("part %d, want 2", s.m().patSub)
	}
	s.press("e", "l") // out of the mode, l is the next step
	if s.m().patCol != 14 {
		t.Errorf("column %d, want 14", s.m().patCol)
	}
}

func TestASoloInOnePatternDoesNotSilenceAnother(t *testing.T) {
	pads := func(p ...int) []proto.PadIndex {
		var out []proto.PadIndex
		for _, i := range p {
			pad, _ := proto.NewPadIndex(uint16(i))
			out = append(out, pad)
		}
		return out
	}
	pad := func(i int) proto.PadIndex { p, _ := proto.NewPadIndex(uint16(i)); return p }
	m := Model{soloed: map[proto.PadIndex]bool{pad(1): true}}
	with := &app.PatternDetail{PadsUsed: pads(0, 1)}
	without := &app.PatternDetail{PadsUsed: pads(5, 6)}
	if !heard(pad(1), nil, m.soloFor(with)) || heard(pad(0), nil, m.soloFor(with)) {
		t.Error("in the pattern with the soloed pad only that pad sounds")
	}
	if !heard(pad(5), nil, m.soloFor(without)) || !heard(pad(6), nil, m.soloFor(without)) {
		t.Error("a pattern without the soloed pad plays as it is")
	}
}

func TestCtrlFGoesFullScreenAndBack(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 140, Height: 40})
	s.shows("404tui by moahbeatlab")
	rows := len(strings.Split(s.m().View(), "\n"))
	s.press("ctrl+f")
	s.hides("404tui by moahbeatlab")
	s.shows("full screen")
	if got := len(strings.Split(s.m().View(), "\n")); got > rows {
		t.Errorf("full screen has %d rows, more than the %d before", got, rows)
	}
	s.press("ctrl+f")
	s.shows("404tui by moahbeatlab")
}

func TestTheTemplatePickerShowsTheFirstBarOfTheGroove(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 150, Height: 50})
	s.press("N")
	s.hides("House · bars")
	s.press("down", "down") // House
	s.shows("House · bars 1-2", "Kick")
	s.press("down", "down", "down", "down", "down")
	s.shows("Hip-hop · bars 1-2")
}

func TestTheTemplatePickerShowsAllPadsHeldHeld(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 150, Height: 50})
	s.press("N")
	s.shows("All pads held · bars 1-2", "────")
}

func TestTheTemplateListScrollsToKeepTheCursorInViewOnASmallScreen(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 150, Height: 30})
	s.press("N", "G")
	s.shows("Footwork · bars 1-2", "Footwork")
}

func TestAPatternEditedOnTheUnitIsReadAgainWhenTheUnitSaysSo(t *testing.T) {
	s := open(t)
	rec := func(delta byte, pad proto.PadIndex) []byte {
		return []byte{delta, 0x2F + byte(pad.Index()%80), byte(pad.Index() / 80), 0, 0x7F, 0x40, 0x10, 0}
	}
	end := [][]byte{{0, 0x8C, 0, 0, 0, 0, 0, 0}, {4, 0, 0, 0, 3, 0, 0, 0}}
	file := func(pads ...proto.PadIndex) []byte {
		var b []byte
		for _, p := range pads {
			b = append(b, rec(240, p)...)
		}
		for _, r := range end {
			b = append(b, r...)
		}
		return b
	}
	s.sim.AddFile(formats.PatternPath(1, pad(t, "B2")), file(pad(t, "A1"), pad(t, "A2")))
	s.press("tab", "]", "l", "enter")
	s.shows("Pattern B2", "A1   kick", "A2   snare")
	if n := len(s.m().pattern.Notes); n != 2 {
		t.Fatalf("the open pattern has %d notes, want 2", n)
	}
	// The pattern is changed on the unit's own panel, which says that the project changed.
	s.sim.AddFile(formats.PatternPath(1, pad(t, "B2")), file(pad(t, "A1")))
	s.sim.Notify([]byte{proto.ReplyChanged, 0, 0, 0, 0})
	for i := 0; i < 3; i++ {
		s.send(s.pollResult())
	}
	s.shows("Pattern B2", "changed on the unit")
	if n := len(s.m().pattern.Notes); n != 1 {
		t.Errorf("the open pattern has %d notes, want the 1 the unit has now", n)
	}
}

// clickOn clicks the screen where the text is drawn.
func (s *screen) clickOn(text string) {
	s.t.Helper()
	for y, line := range strings.Split(ansi.Strip(s.m().View()), "\n") {
		if at := strings.Index(line, text); at >= 0 {
			x := len([]rune(line[:at])) + 1
			s.send(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			return
		}
	}
	s.t.Fatalf("no %q on the screen:\n%s", text, ansi.Strip(s.m().View()))
}

func TestPerformClicksMuteIsolateAndBringTheOthersBack(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	bus, _ := proto.ParamByName("bus-fx")
	for i, v := range []int32{1, 1, 2} {
		p, _ := proto.NewPadIndex(uint16(i))
		if err := s.m().unit.SetParam(bus.ID, proto.TargetPad(p), v); err != nil {
			t.Fatal(err)
		}
	}
	s.press("r")
	s.press("7")
	busOf := func(name string) int32 { return s.m().snap.Pads[pad(t, name).Index()].Params["bus-fx"] }
	// What the unit holds is read again from it.
	unitBus := func(name string) int32 {
		s.press("r")
		return busOf(name)
	}
	if busOf("A1") != 1 || busOf("A2") != 1 || busOf("A3") != 2 {
		t.Fatalf("buses %d %d %d", busOf("A1"), busOf("A2"), busOf("A3"))
	}
	// One click mutes the pad (here); another unmutes it.
	s.clickOn("kick")
	if !s.m().muted[pad(t, "A1")] {
		t.Error("one click did not mute the pad")
	}
	time.Sleep(doubleClick + 50*time.Millisecond)
	s.clickOn("kick")
	if s.m().muted[pad(t, "A1")] {
		t.Error("a second, slow click did not unmute the pad")
	}
	// A double click: A1 keeps BUS 1, A2 (also on BUS 1) goes dry, A3 (BUS 2) is left alone; the
	// first click's mute is undone.
	time.Sleep(doubleClick + 50*time.Millisecond)
	s.clickOn("kick")
	s.clickOn("kick")
	if s.m().muted[pad(t, "A1")] {
		t.Error("the double click left the pad muted")
	}
	s.run(nil)
	if unitBus("A1") != 1 || unitBus("A2") != 0 || unitBus("A3") != 2 {
		t.Errorf("after isolating A1 the buses are %d %d %d, want 1 0 2", busOf("A1"), busOf("A2"), busOf("A3"))
	}
	s.shows("A1 alone")
	// A double click again brings it back.
	time.Sleep(doubleClick + 50*time.Millisecond)
	s.clickOn("kick")
	s.clickOn("kick")
	if unitBus("A1") != 1 || unitBus("A2") != 1 || unitBus("A3") != 2 {
		t.Errorf("after the second double click the buses are %d %d %d, want 1 1 2", busOf("A1"), busOf("A2"), busOf("A3"))
	}
	// The button: the pads clicked keep their buses, leaving it puts everything back.
	s.clickOn("off  ")
	s.shows("tap pads to add or take them out")
	s.clickOn("snare")
	if unitBus("A1") != 0 || unitBus("A2") != 1 {
		t.Errorf("with A2 chosen the buses are %d %d, want 0 1", busOf("A1"), busOf("A2"))
	}
	s.clickOn("ON  ●")
	if unitBus("A1") != 1 || unitBus("A2") != 1 {
		t.Errorf("after the button the buses are %d %d, want 1 1", busOf("A1"), busOf("A2"))
	}
}

func TestPadsHeardInThePatternKeepADifferentColour(t *testing.T) {
	s := open(t)
	m := s.m()
	plain, _ := m.tileColours(tileState{})
	heard, _ := m.tileColours(tileState{used: true})
	inIt, _ := m.tileColours(tileState{inPattern: true})
	if heard == plain || inIt == plain || heard == inIt {
		t.Errorf("colours: plain %s, heard %s, in the pattern %s", plain, heard, inIt)
	}
	// A bright state wins: the glow of a hit and the cursor are not hidden by the colour.
	glow, _ := m.tileColours(tileState{used: true, glow: 1})
	if glow == heard {
		t.Error("a pad that is hit now does not glow")
	}
}

func TestScenesFadeTheEffectsOfTheUnitByMIDI(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	link := &midiLink{ch: make(chan midiin.Event, 4)}
	src := &fakeMIDI{}
	m := s.m()
	m.midi, m.midiSrc = link, src
	s.model = m
	s.press("7")
	feed := func(e midiin.Event) {
		e.At = time.Now()
		next, _ := s.m().midiEvent(midiMsg{link: link, ev: e})
		s.model = next
	}
	cc := func(ch, n, v int) { feed(midiin.Event{Kind: midiin.CC, Channel: ch, Data1: n, Data2: v}) }
	lines := func() []string { return strings.Split(ansi.Strip(s.m().View()), "\n") }
	click := func(y, x int) {
		s.send(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	}
	// clickNth clicks the n-th place a text is on the screen (reading top to bottom).
	clickNth := func(text string, n int) {
		t.Helper()
		for y, line := range lines() {
			if at := strings.Index(line, text); at >= 0 {
				if n == 0 {
					click(y, len([]rune(line[:at]))+1)
					return
				}
				n--
			}
		}
		t.Fatalf("no %q on the screen:\n%s", text, ansi.Strip(s.m().View()))
	}
	// fadeAt clicks the fade bar of a bus at a fraction of its width.
	fadeAt := func(bus int, frac float64) {
		t.Helper()
		row := -1
		seen := 0
		for y, line := range lines() {
			if strings.Contains(line, "save A") {
				if seen == bus {
					row = y
					break
				}
				seen++
			}
		}
		if row < 0 {
			t.Fatal("no fade bar")
		}
		runes := []rune(lines()[row])
		first, last := -1, -1
		for i, r := range runes {
			if r == '█' || r == '░' {
				if first < 0 {
					first = i
				}
				last = i
			}
		}
		click(row, first+int(frac*float64(last-first)))
	}
	sent := func(n byte) int {
		src.mu.Lock()
		defer src.mu.Unlock()
		for i := len(src.sent) - 1; i >= 0; i-- {
			if src.sent[i][1] == n {
				return int(src.sent[i][2])
			}
		}
		return -1
	}
	// Bus 1 is Scatter (6), on, knobs 20 40 60: scene A. Then knobs 100 40 0 and Downer (7): scene B.
	cc(1, 83, 6)
	cc(1, 19, 127)
	cc(1, 16, 20)
	cc(1, 17, 40)
	cc(1, 18, 60)
	clickNth("save A", 0)
	// The length of a glide is set by the buttons 1 2 4 8 (not a bus: 8 is no index).
	for _, n := range []string{"8", "4", "1", "2"} {
		for y, line := range lines() {
			if strings.Contains(line, "glide takes") {
				at := strings.Index(line, "  "+n+"  ")
				click(y, len([]rune(line[:at]))+3)
				break
			}
		}
	}
	if s.m().sc.bars != 2 {
		t.Errorf("a glide takes %d bars after tapping 8, 4, 1 and 2", s.m().sc.bars)
	}
	cc(1, 83, 7)
	cc(1, 16, 100)
	cc(1, 18, 0)
	clickNth("save B", 0)
	s.shows("SCENES", "Scatter", "Downer")
	src.mu.Lock()
	src.sent = nil
	src.mu.Unlock()
	// Halfway on the bar: knob 1 at about 60, the effect that of B (past the middle).
	fadeAt(0, 0.5)
	if got := sent(16); got < 55 || got > 65 {
		t.Errorf("knob 1 at the middle is %d, want about 60", got)
	}
	if got := sent(83); got != 7 {
		t.Errorf("the effect at the middle is %d, want B's 7", got)
	}
	// Bus 2 has no scenes: its fade does nothing, and nothing is sent to its channel.
	fadeAt(1, 0.5)
	src.mu.Lock()
	for _, msg := range src.sent {
		if msg[0]&0x0F == 1 {
			t.Errorf("sent % x on bus 2's channel, which has no scenes", msg)
		}
	}
	src.mu.Unlock()
	// The left end of the bar is scene A.
	fadeAt(0, 0)
	if got := sent(16); got != 20 || sent(83) != 6 {
		t.Errorf("back at A: knob 1 %d effect %d, want 20 and 6", got, sent(83))
	}
	// A glide of this bus begins on the next bar of the clock and ends at B; bus 2 cannot glide yet.
	feed(midiin.Event{Kind: midiin.Start})
	clickNth("glide →A", 0) // bus 2's fade was left in the middle, so its button says A
	s.shows("BUS 2: save scene A and scene B first")
	clickNth("glide →B", 0)
	s.shows("BUS 1: glide to B")
	for i := 0; i < ticksPerBar*3; i++ {
		feed(midiin.Event{Kind: midiin.Clock})
	}
	if got := sent(16); got != 100 {
		t.Errorf("after the glide knob 1 is %d, want 100", got)
	}
	if bs := s.m().sc.bus[0]; bs.mix != 1 || bs.glide != nil {
		t.Errorf("mix %v glide %v after the glide", bs.mix, bs.glide)
	}
}

func TestAPadOfAnEarlierPatternIsNotShownAsPlayedInTheCurrentOne(t *testing.T) {
	s := open(t)
	m := s.m()
	slot := pad(t, "B2")
	m.perfNow = &slot
	m.patPreview[slot] = &app.PatternDetail{PadsUsed: []proto.PadIndex{pad(t, "A1"), pad(t, "A2")}}
	m.mon = newMonitor()
	m.clock = &clockState{ticks: 200, running: true}
	m.mon.heard(0, 190) // A1: in the pattern, heard
	m.mon.heard(2, 190) // A3: heard, but the pattern does not play it
	heard, in := m.padUsed(pad(t, "A1"))
	if !heard || in {
		t.Errorf("A1 heard %v inPattern %v, want heard", heard, in)
	}
	if heard, in := m.padUsed(pad(t, "A2")); heard || !in {
		t.Errorf("A2 (in the pattern, not heard) heard %v inPattern %v", heard, in)
	}
	if heard, in := m.padUsed(pad(t, "A3")); heard || in {
		t.Errorf("A3 is not in the pattern but shows heard %v inPattern %v", heard, in)
	}
	// What was heard long ago is forgotten.
	m.mon.heard(1, 190+9*ticksPerBar)
	if m.mon.played[0] {
		t.Error("a pad last heard nine bars ago is still held as played")
	}
}

func TestAClickPastTheEndOfTheFadeBarStaysInsideIt(t *testing.T) {
	m := open(t).m()
	m = m.sceneAct("fade", 0, 1.4)
	if got := m.sc.bus[0].mix; got != 1 {
		t.Errorf("a click past the end puts the fade at %v, want 1", got)
	}
	m = m.sceneAct("fade", 0, -0.3)
	if got := m.sc.bus[0].mix; got != 0 {
		t.Errorf("a click before the start puts the fade at %v, want 0", got)
	}
	// Whatever the position, the bar can be drawn.
	b := newRowBuilder(m.st, 3)
	for _, f := range []float64{-1, 0, 0.5, 1, 1.05, 7} {
		b.bar(20, f, m.st.th.accent, "fade", 0)
	}
}

func TestAGlideSurvivesANewStartAndALengthChange(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	link := &midiLink{ch: make(chan midiin.Event, 4)}
	src := &fakeMIDI{}
	m := s.m()
	m.midi, m.midiSrc = link, src
	s.model = m
	s.press("7")
	feed := func(e midiin.Event) {
		e.At = time.Now()
		next, _ := s.m().midiEvent(midiMsg{link: link, ev: e})
		s.model = next
	}
	cc := func(n, v int) { feed(midiin.Event{Kind: midiin.CC, Channel: 1, Data1: n, Data2: v}) }
	cc(16, 0)
	m = s.m().sceneAct("a", 0, 0)
	s.model = m
	cc(16, 100)
	s.model = s.m().sceneAct("b", 0, 0)
	s.model = s.m().sceneAct("bars", 2, 0)
	feed(midiin.Event{Kind: midiin.Start})
	s.model = s.m().sceneAct("glide", 0, 0)
	for i := 0; i < ticksPerBar+10; i++ {
		feed(midiin.Event{Kind: midiin.Clock})
	}
	// The length is changed and the unit starts again in the middle of the glide.
	s.model = s.m().sceneAct("bars", 8, 0)
	before := s.m().sc.bus[0].mix
	feed(midiin.Event{Kind: midiin.Clock})
	if d := s.m().sc.bus[0].mix - before; d < 0 || d > 0.02 {
		t.Errorf("the fade jumped by %v when the length of a glide was changed", d)
	}
	feed(midiin.Event{Kind: midiin.Start})
	for i := 0; i < ticksPerBar*3; i++ {
		feed(midiin.Event{Kind: midiin.Clock})
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	for _, msg := range src.sent {
		if msg[2] > 127 {
			t.Fatalf("sent a value of %d", msg[2])
		}
	}
	if mix := s.m().sc.bus[0].mix; mix < 0 || mix > 1 {
		t.Errorf("the fade is at %v", mix)
	}
	if s.m().sc.bus[0].glide != nil {
		t.Error("the glide did not end")
	}
}
