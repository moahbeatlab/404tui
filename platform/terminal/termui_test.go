package terminal

import (
	"image/color"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/core"
)

var (
	ground = color.RGBA{11, 13, 16, 255}
	amber  = color.RGBA{255, 176, 0, 255}
)

func TestBasicGlyphsHaveNoBrailleOrEighths(t *testing.T) {
	scr := core.TextScreen{Cols: 4, Rows: 1, Cells: []core.Cell{{R: '⣿'}, {R: '▂'}, {R: '╭'}, {R: '●'}}}
	got := string([]rune{Cells(scr, BasicGlyphs)[0][0].R, Cells(scr, BasicGlyphs)[0][1].R, Cells(scr, BasicGlyphs)[0][2].R, Cells(scr, BasicGlyphs)[0][3].R})
	if got != "█ +o" {
		t.Fatalf("got %q", got)
	}
	if Cells(scr, FullGlyphs)[0][0].R != '⣿' {
		t.Fatal("full glyphs keep the braille")
	}
}

var sgrRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

func TestDrawGivesTheCharactersAndResetsEachLine(t *testing.T) {
	cells := [][]Cell{
		{{R: 'a', Fg: amber, Bg: ground}, {R: 'b', Fg: amber, Bg: ground}},
		{{R: '▀', Fg: amber, Bg: ground}, {R: ' ', Bg: ground}},
	}
	for _, p := range []Profile{TrueColor, ANSI256} {
		out := Draw(cells, p)
		lines := strings.Split(out, "\n")
		if len(lines) != 2 || !strings.HasSuffix(lines[0], "\x1b[0m") {
			t.Fatalf("profile %d: %q", p, out)
		}
		if got := sgrRe.ReplaceAllString(lines[0], "") + "|" + sgrRe.ReplaceAllString(lines[1], ""); got != "ab|▀ " {
			t.Fatalf("profile %d: text %q", p, got)
		}
	}
	if !strings.Contains(Draw(cells, TrueColor), "38;2;255;176;0") {
		t.Fatal("true colour codes expected")
	}
	if !strings.Contains(Draw(cells, ANSI256), "38;5;") {
		t.Fatal("256-colour codes expected")
	}
}

// screenText is the characters of the screen, a line for each row.
func screenText(s core.TextScreen) string {
	var sb strings.Builder
	for r := 0; r < s.Rows; r++ {
		for c := 0; c < s.Cols; c++ {
			sb.WriteRune(s.Cells[r*s.Cols+c].R)
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func newModel(t *testing.T) *Model {
	t.Helper()
	dir := t.TempDir()
	app := core.New(dir+"/samples", dir+"/out", &core.NullAudio{})
	t.Cleanup(func() { core.SetSize(core.MinCols, core.MinRows); core.Terminal = false })
	core.Terminal = true
	m := NewModel(app, Options{})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return m
}

func press(m *Model, s string) {
	switch s {
	case "enter":
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	case "esc":
		m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	case "up":
		m.Update(tea.KeyMsg{Type: tea.KeyUp})
	case "ctrl+c":
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	default:
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
	}
}

func (m *Model) text() string { return screenText(m.app.RenderCells()) }

func TestVimKeysAndArrowsMoveTheCursor(t *testing.T) {
	m := newModel(t)
	press(m, "x") // any key ends the splash
	if !strings.Contains(m.text(), "A1") {
		t.Fatal("pad A1 first")
	}
	press(m, "l")
	if !strings.Contains(m.text(), "A2") {
		t.Fatalf("l goes right:\n%s", m.text())
	}
	press(m, "j")
	if !strings.Contains(m.text(), "A6") {
		t.Fatal("j goes down a row of 4")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if !strings.Contains(m.text(), "A1") {
		t.Fatal("the arrows go back")
	}
}

func TestYArmsAShiftForTheNextKey(t *testing.T) {
	m := newModel(t)
	press(m, "x")
	press(m, "y")
	if !strings.Contains(m.text(), "SHIFT") {
		t.Fatal("the panel shows the shift while it is armed")
	}
	press(m, "l") // Y+Right: the next screen
	txt := m.text()
	if !strings.Contains(txt, "PATTERN A1") || strings.Contains(txt, "SHIFT") {
		t.Fatalf("Y then l goes to SEQ, and the shift is let go:\n%s", txt)
	}
}

func TestSelectArmsForTheNextKey(t *testing.T) {
	m := newModel(t)
	press(m, "x")
	for i := 0; i < 3; i++ { // PADS, SEQ, PROJECTS, FILES
		press(m, "y")
		press(m, "l")
	}
	press(m, "s")
	press(m, "b") // Select+B on FILES: the file tools
	if !strings.Contains(m.text(), "Mark every file here") {
		t.Fatalf("s then b opens the file tools:\n%s", m.text())
	}
}

func TestQuestionMarkTogglesTheHelp(t *testing.T) {
	m := newModel(t)
	press(m, "x")
	press(m, "?")
	if !strings.Contains(m.text(), "arrows or h j k l") {
		t.Fatalf("the help names the keyboard:\n%s", m.text())
	}
	press(m, "?")
	if strings.Contains(m.text(), "arrows or h j k l") {
		t.Fatal("? again closes it")
	}
}

func TestATooSmallWindowSaysSo(t *testing.T) {
	m := newModel(t)
	m.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	if v := m.View(); !strings.Contains(v, "needs a window of at least 80x30") {
		t.Fatalf("view: %q", v)
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 34})
	if v := m.View(); strings.Contains(v, "needs a window") {
		t.Fatal("a big enough window shows the screen again")
	}
}

func TestEverySizeHasAPanelOfAThirdOfTheWidth(t *testing.T) {
	for _, sz := range [][2]int{{80, 30}, {100, 34}, {120, 40}, {160, 48}, {200, 60}, {132, 43}} {
		m := newModel(t)
		m.Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
		press(m, "x")
		f := m.app.RenderCells()
		if f.Cols != sz[0] || f.Rows != sz[1] {
			t.Fatalf("%v: screen %dx%d cells", sz, f.Cols, f.Rows)
		}
		col := -1
		for r := 0; r < f.Rows && col < 0; r++ {
			for c := 0; c+3 < f.Cols; c++ {
				if f.Cells[r*f.Cols+c].R == 'B' && f.Cells[r*f.Cols+c+1].R == 'P' && f.Cells[r*f.Cols+c+2].R == 'M' { // the tempo, first in the panel
					col = c - 1
					break
				}
			}
		}
		want := sz[0] * 70 / 100
		if col < want-4 || col > want+4 {
			t.Errorf("%v: the panel starts at column %d, wanted about %d", sz, col, want)
		}
		if txt := m.text(); !strings.Contains(txt, "ALWAYS") || !strings.Contains(txt, "BPM") || !strings.Contains(txt, "autosave") {
			t.Errorf("%v: panel or footer missing:\n%s", sz, txt)
		}
	}
}

func TestSearchTypedOnTheKeyboard(t *testing.T) {
	m := newModel(t)
	press(m, "x")
	for i := 0; i < 3; i++ { // SEQ, PROJECTS, FILES
		press(m, "y")
		press(m, "l")
	}
	press(m, "/")
	for _, ch := range "kick" {
		press(m, string(ch))
	}
	if !strings.Contains(m.text(), "kick_") {
		t.Fatalf("typed text is in the search:\n%s", m.text())
	}
	press(m, "enter")
	if strings.Contains(m.text(), "SEARCH") {
		t.Fatal("Enter closes the keyboard")
	}
}

func TestThePanelNamesTheKeysOfTheKeyboard(t *testing.T) {
	m := newModel(t)
	press(m, "x")
	txt := m.text()
	for _, want := range []string{"hjkl", "[ ]", "g h", "y a"} { // move, bank, screen, mark pad
		if !strings.Contains(txt, want) {
			t.Errorf("the panel should name %q:\n%s", want, txt)
		}
	}
	for _, not := range []string{"L1 R1", "Y+", "^v<>"} {
		if strings.Contains(txt, not) {
			t.Errorf("the panel still names the handheld's %q:\n%s", not, txt)
		}
	}
	press(m, "y")
	press(m, "l") // SEQ
	txt = m.text()
	if !strings.Contains(txt, "tools") || !strings.Contains(txt, "y x") || strings.Contains(txt, "Sel+B") {
		t.Errorf("SEQ names t (tools) and y x:\n%s", txt)
	}
	core.Terminal = false
	if txt := screenText(m.app.RenderCells()); !strings.Contains(txt, "L1 R1") {
		t.Error("without a terminal the panel names the buttons")
	}
}

func TestScreensByNumberTabAndGHL(t *testing.T) {
	m := newModel(t)
	name := func() string { return m.app.ScreenName() }
	press(m, "4")
	if name() != "FILES" {
		t.Fatalf("4 is FILES, got %s", name())
	}
	press(m, "3")
	if name() != "UNIT" {
		t.Fatalf("3 is PROJECTS, got %s", name())
	}
	press(m, "4")
	press(m, "g")
	press(m, "l")
	if name() != "SLICE" && name() != "PADS" { // SLICE bounces to PADS when the pad is empty
		t.Fatalf("gl goes to the next screen, got %s", name())
	}
	press(m, "g")
	press(m, "g")
	if name() != "PADS" {
		t.Fatalf("gg is the first screen, got %s", name())
	}
	press(m, "g")
	press(m, "h")
	if name() != "CONFIG" {
		t.Fatalf("gh from the first screen wraps to the last, got %s", name())
	}
	press(m, "2")
	press(m, "tab")
	if name() != "UNIT" {
		t.Fatalf("Tab goes on, got %s", name())
	}
}

func TestEnterOpensTheSliceMenuAndSpaceStops(t *testing.T) {
	dir := t.TempDir()
	if err := core.WriteDemo(dir + "/samples"); err != nil {
		t.Fatal(err)
	}
	app := core.New(dir+"/samples", dir+"/out", &core.NullAudio{})
	t.Cleanup(func() { core.SetSize(core.MinCols, core.MinRows); core.Terminal = false })
	core.Terminal = true
	m := NewModel(app, Options{})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	press(m, "4")
	press(m, "a") // into the first folder
	press(m, "a") // its first sample: the pad dialog
	press(m, "a") // the pad it opens on
	press(m, "5")
	if m.app.ScreenName() != "SLICE" {
		t.Fatalf("a sample on the pad opens the slicer, got %s", m.app.ScreenName())
	}
	press(m, "enter")
	if !strings.Contains(strings.ToLower(m.text()), "slice settings") {
		t.Fatalf("Enter in the slicer opens its menu:\n%s", m.text())
	}
	press(m, "esc")
	press(m, "1")
	press(m, " ")
	if !m.app.Playing() {
		t.Fatal("Space plays the pad")
	}
	press(m, "x")
	if m.app.Playing() {
		t.Fatal("x stops what plays")
	}
	press(m, " ")
	if !m.app.Playing() {
		t.Fatal("Space plays when nothing does")
	}
	press(m, " ")
	if m.app.Playing() {
		t.Fatal("Space stops what plays")
	}
}

func TestSpacePlaysAndStopsTheSampleAndThePattern(t *testing.T) {
	dir := t.TempDir()
	if err := core.WriteDemo(dir + "/samples"); err != nil {
		t.Fatal(err)
	}
	app := core.New(dir+"/samples", dir+"/out", &core.NullAudio{})
	t.Cleanup(func() { core.SetSize(core.MinCols, core.MinRows); core.Terminal = false })
	core.Terminal = true
	m := NewModel(app, Options{})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	press(m, "4")
	press(m, "a")
	press(m, "a") // a sample: the pad dialog
	press(m, "a") // A1
	press(m, "1")
	press(m, " ")
	if !m.app.Playing() {
		t.Fatal("Space on PADS plays the sample")
	}
	press(m, " ")
	if m.app.Playing() {
		t.Fatal("Space again stops it")
	}
	press(m, "2")
	press(m, "a") // a note
	press(m, " ")
	if !m.app.Playing() {
		t.Fatal("Space on SEQ plays the pattern")
	}
	press(m, " ")
	if m.app.Playing() {
		t.Fatal("Space again stops the pattern")
	}
}

func TestEscClosesAMenuAndBDoesNot(t *testing.T) {
	m := newModel(t)
	press(m, "a") // the pad menu... on an empty pad: the file dialog or a message
	m.app.Dispatch(core.Event{Key: core.A, Down: true})
	m.app.Dispatch(core.Event{Key: core.A, Down: false})
	if !m.app.Modal() {
		t.Skip("no menu opens on an empty pad here")
	}
	press(m, "b")
	if !m.app.Modal() {
		t.Fatal("b does not close a menu")
	}
	press(m, "esc")
	if m.app.Modal() {
		t.Fatal("Esc closes it")
	}
}

func TestOnSeqXHearsTheNoteSpacePlaysAndFFreezes(t *testing.T) {
	dir := t.TempDir()
	if err := core.WriteDemo(dir + "/samples"); err != nil {
		t.Fatal(err)
	}
	app := core.New(dir+"/samples", dir+"/out", &core.NullAudio{})
	t.Cleanup(func() { core.SetSize(core.MinCols, core.MinRows); core.Terminal = false })
	core.Terminal = true
	m := NewModel(app, Options{})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	press(m, "3")
	press(m, "a")
	press(m, "a")
	press(m, "2")
	press(m, "a") // a note
	press(m, "x")
	if m.app.Playing() {
		t.Fatal("x on SEQ hears the note, it does not start the pattern")
	}
	press(m, "f")
	if !strings.Contains(m.text(), "FROZEN") {
		t.Fatalf("f freezes the view:\n%s", m.text())
	}
	press(m, " ")
	if !m.app.Playing() {
		t.Fatal("Space plays the pattern")
	}
	press(m, " ")
	if m.app.Playing() {
		t.Fatal("Space stops it")
	}
}

// zoomIs says whether the slicer's panel shows the zoom with that value on its line.
func zoomIs(m *Model, v string) bool {
	for _, l := range strings.Split(m.text(), "\n") {
		if strings.Contains(l, "zoom") && strings.Contains(l, v) {
			return true
		}
	}
	return false
}

func TestPlusAndMinusZoomTheSlicer(t *testing.T) {
	dir := t.TempDir()
	if err := core.WriteDemo(dir + "/samples"); err != nil {
		t.Fatal(err)
	}
	app := core.New(dir+"/samples", dir+"/out", &core.NullAudio{})
	t.Cleanup(func() { core.SetSize(core.MinCols, core.MinRows); core.Terminal = false })
	core.Terminal = true
	m := NewModel(app, Options{})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	press(m, "4")
	press(m, "a")
	press(m, "a")
	press(m, "a") // the pad dialog
	press(m, "5")
	if !zoomIs(m, "1x") {
		t.Fatalf("the slicer starts whole:\n%s", m.text())
	}
	press(m, "+")
	if !zoomIs(m, "2x") {
		t.Fatalf("+ zooms in:\n%s", m.text())
	}
	press(m, "-")
	if !zoomIs(m, "1x") {
		t.Fatal("- zooms out")
	}
}
