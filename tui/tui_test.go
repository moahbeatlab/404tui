package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/device"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
	"github.com/moahbeatlab/404tui/sim"
)

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
	played  []formats.Sample
	stopped int
}

func (f *fakePlayer) Play(s formats.Sample) error { f.played = append(f.played, s); return nil }
func (f *fakePlayer) Stop()                       { f.stopped++ }

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
	sc := &screen{t: t, model: New(d, player, store), sim: s, player: player, dev: d, store: store}
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
	switch msg := cmd().(type) {
	case nil, tickMsg:
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
	s.shows("404tui", "PROJECT_01", "1 Pads", "4 Projects", "BANKS", "kick", "snare", "hat", "A1  kick")
	s.press("2")
	s.shows("Pattern A1", "Empty slot.")
	s.press("]")
	s.shows("Pattern B1")
	s.press("3")
	s.shows("SOURCE", "SD card", "▸ IMPORT/", "Details", "Folder")
	s.press("4")
	s.shows("PROJECT_01", "Project 1", "● current", "3 of 160 pads")
	s.press("shift+tab")
	s.shows("▸ IMPORT/")
	// The bank chosen on Patterns carries over, so go back to A.
	s.press("2", "[", "1")
	s.shows("kick")
}

func TestHelpAndPalette(t *testing.T) {
	s := open(t)
	s.press("?")
	s.shows("Keybindings")
	s.press("3")
	s.shows("Keybindings")
	s.press("?")
	s.hides("Keybindings")

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
	s.press("V", "d", "y")
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
	s.press("4", "j", "j", "enter")
	s.shows("project 3 selected", "PROJECT_03")
	// The pads on screen are now the new project's.
	s.press("1")
	s.hides("kick")

	// The unit keeps a name only for a project with something saved in it.
	s.press("4", "n")
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
	s.press("3")
	s.shows("▸ IMPORT/")
	s.press("tab")
	s.shows("▾ IMPORT/", "▸ pack/", "kick.wav", "Files"[:0]+"3 Files")
	s.press("tab")
	s.shows("▸ IMPORT/")
	s.hides("kick.wav")
	s.press("tab", "j", "j", "m")
	s.shows("marked SD:IMPORT/kick.wav")
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

	s.press("g", "/")
	s.shows("Find by name")
	s.typeText("snare")
	s.press("enter")
	m := s.m()
	if node, _ := m.fileNode(); node.Name != "snare.wav" {
		t.Errorf("search landed on %q", node.Name)
	}
	// h on a file goes to its folder, and h on the open folder closes it.
	s.press("h", "h")
	s.hides("snare.wav")
	s.press("p")
	s.shows("nothing marked")

	s.press("t")
	s.shows("Unit storage", "▸ ROLAND/")
	s.press("t")
	s.shows("This computer")
}

func TestAssignAFileThroughThePicker(t *testing.T) {
	s := open(t)
	s.press("3", "tab", "j", "j")
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
	s.press("3", "enter")
	s.shows("C8 is empty")
	// Two digits make pad 12.
	s.press("1", "2")
	s.shows("C12 is empty")
	s.press("esc")
}

func TestTargetBankInFiles(t *testing.T) {
	s := open(t)
	s.press("3")
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
	s.press("3", "tab", "j", "j", "enter", "J", "enter")
	s.shows("assigned to J1")
	// Browsing other banks on Pads does not change where files go.
	s.press("1", "[", "[", "[")
	if s.m().bank == 9 {
		t.Fatal("test setup: still on bank J")
	}
	s.press("3", "enter")
	s.shows("J2 is empty")
	s.press("esc")
	// Looking at a bank in the picker and cancelling does not change it either.
	s.press("enter", "C", "esc", "enter")
	s.shows("J2 is empty")
	s.press("esc")
	// It is remembered between runs.
	s = s.restart()
	s.press("3", "enter")
	s.shows("J2 is empty")
}

func TestAssignReplacingAPadAsksTwice(t *testing.T) {
	s := open(t)
	s.press("3", "tab", "j", "j", "enter", "A", "1")
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
	s.press("a", "/")
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
	s.press("T", "]", "]")
	s.press("3", "tab", "j", "l")
	if st := s.store.state; st.Theme != 1 || st.Bank != 2 || st.FilesRoot != "SD:" || len(st.FilesExpanded) != 2 {
		t.Fatalf("remembered %+v", st)
	}
	s = s.restart()
	if m := s.m(); m.themeAt != 1 || m.bank != 2 {
		t.Errorf("theme %d, bank %d after a restart", m.themeAt, m.bank)
	}
	s.press("3")
	s.shows("▾ IMPORT/", "▾ pack/", "snare.wav")
}

func TestListenToAFile(t *testing.T) {
	s := open(t)
	s.press("3", "tab", "j", "j", "space")
	if len(s.player.played) != 1 || s.player.played[0].Frames() != 4800 {
		t.Fatalf("played %+v", s.player.played)
	}
	s.shows("♪")
	s.press("space")
	if s.player.stopped == 0 || s.m().listening != "" {
		t.Error("a second Space did not stop it")
	}
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

	s.press("2", "]")
	s.shows("2 pads"[:0] + "1 pads")
	s.press("enter")
	s.shows("That pattern"[:0] + "that pattern slot is empty")
	// Slot 2 is one right of slot 1, which is top left.
	s.press("l", "enter")
	s.shows("Pattern B2", "120.00 BPM", "A1   kick", "A2   snare", "■")
	s.press("b")
	s.shows("move its pads from bank A to bank", "B")
	s.press("]", "]")
	s.press("enter")
	s.shows("2 note(s) moved from bank A to bank D", "D1", "D2")
	// After the move the pads are in one bank D, so the pattern still retargets.
	s.press("esc")
	s.shows("Pattern B2")
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
	// Sections, on the rail.
	s.click(3, s.railRow(railSection, secFiles))
	s.shows("▸ IMPORT/")
	s.click(3, s.railRow(railSection, secPads))
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
	x, y := g.bodyX+(g.tileW+1)+2, rowBody+1
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
	s.click(g.bodyX+2, rowBody+3*(g.tileH+1)+1)
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
	s.press("3")
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
	s.press("4")
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
	s.shows("3 pads restored from SD:IMPORT/PROJECT_01-A")
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
	s.shows("3 pads restored")
	s.press("e", "/")
	s.shows("Export bank A to the folder")
	s.press("esc")

	// From Files: R opens the browser where the tree is, and Enter on samples.json restores
	// the folder it is in.
	s.press("3", "tab", "j", "j", "tab", "R")
	s.shows("Restore pads from the export in…", "▾ PROJECT_01-A/", "samples.json")
	s.press("esc", "/")
	s.typeText("samples")
	s.press("enter")
	s.shows("Pad export")
	s.press("enter")
	s.shows("3 pads restored from SD:IMPORT/PROJECT_01-A")
}

func TestThemesAndNarrowScreens(t *testing.T) {
	s := open(t)
	s.press("T")
	s.shows("theme: Ocean")
	s.press("T", "T")
	s.shows("theme: Hardware")
	for _, size := range [][2]int{{80, 24}, {60, 20}, {140, 40}, {200, 50}, {100, 14}} {
		s.send(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, section := range []string{"1", "2", "3", "4"} {
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
	if _, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}); cmd == nil {
		t.Error("q did not quit while busy")
	}
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

	s.press("4")
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
	s.press("4", "e", "enter", "enter")
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
