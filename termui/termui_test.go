package termui

import (
	"image"
	"image/color"
	"image/draw"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/handheld"
)

var (
	ground = color.RGBA{11, 13, 16, 255}
	amber  = color.RGBA{255, 176, 0, 255}
)

// frameOf makes a frame of 1 cell holding the pixels fill paints, and no letters.
func frameOf(fill func(img *image.RGBA)) handheld.Frame {
	img := image.NewRGBA(image.Rect(0, 0, cellW, cellH))
	draw.Draw(img, img.Bounds(), image.NewUniform(ground), image.Point{}, draw.Src)
	fill(img)
	return handheld.Frame{Img: img, Base: img, Text: make([]handheld.Glyph, 1), Cols: 1, Rows: 1}
}

func rect(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	draw.Draw(img, image.Rect(x0, y0, x1, y1), image.NewUniform(c), image.Point{}, draw.Src)
}

func TestAnEmptyCellIsASpace(t *testing.T) {
	c := Cells(frameOf(func(*image.RGBA) {}), FullGlyphs)[0][0]
	if c.R != ' ' || c.Bg != ground {
		t.Fatalf("got %q on %v", c.R, c.Bg)
	}
}

func TestHalfACellIsAHalfBlock(t *testing.T) {
	c := Cells(frameOf(func(i *image.RGBA) { rect(i, 0, 0, 8, 8, amber) }), FullGlyphs)[0][0]
	// the top half in amber is ▀ with amber on the ground, or ▄ with the colours the other way round: the same picture
	okA := c.R == '▀' && c.Fg == amber && c.Bg == ground
	okB := c.R == '▄' && c.Fg == ground && c.Bg == amber
	if !okA && !okB {
		t.Fatalf("got %q fg %v bg %v", c.R, c.Fg, c.Bg)
	}
}

func TestABarFromTheBottomIsALowerBlock(t *testing.T) {
	c := Cells(frameOf(func(i *image.RGBA) { rect(i, 0, 10, 8, 16, amber) }), FullGlyphs)[0][0] // 6 px of 16: 3/8
	if c.R != '▃' || c.Fg != amber {
		t.Fatalf("got %q fg %v", c.R, c.Fg)
	}
}

func TestAWaveformColumnIsABar(t *testing.T) {
	c := Cells(frameOf(func(i *image.RGBA) { // 2 px wide bars at four heights, standing on the bottom
		for k, h := range []int{4, 8, 12, 8} {
			rect(i, 2*k, 16-h, 2*k+2, 16, amber)
		}
	}), FullGlyphs)[0][0]
	if !strings.ContainsRune("▄▅▆", c.R) || c.Fg != amber {
		t.Fatalf("got %q fg %v", c.R, c.Fg)
	}
}

func TestThinLinesAreBraille(t *testing.T) {
	c := Cells(frameOf(func(i *image.RGBA) { rect(i, 2, 0, 3, 16, amber); rect(i, 5, 4, 6, 12, amber) }), FullGlyphs)[0][0]
	if c.R < 0x2800 || c.R > 0x28ff {
		t.Fatalf("got %q", c.R)
	}
	b := Cells(frameOf(func(i *image.RGBA) { rect(i, 2, 0, 3, 16, amber); rect(i, 5, 4, 6, 12, amber) }), BasicGlyphs)[0][0]
	if b.R >= 0x2800 && b.R <= 0x28ff {
		t.Fatalf("basic glyphs must not use braille, got %q", b.R)
	}
}

func TestALetterStaysALetter(t *testing.T) {
	f := frameOf(func(*image.RGBA) {})
	f.Text[0] = handheld.Glyph{R: 'Q', Fg: amber, Bold: true}
	c := Cells(f, FullGlyphs)[0][0]
	if c.R != 'Q' || c.Fg != amber || !c.Bold || c.Bg != ground {
		t.Fatalf("got %+v", c)
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

// screenText is the letters of the frame, a line for each row.
func screenText(f handheld.Frame) string {
	var sb strings.Builder
	for r := 0; r < f.Rows; r++ {
		for c := 0; c < f.Cols; c++ {
			if g := f.Text[r*f.Cols+c]; g.R != 0 {
				sb.WriteRune(g.R)
			} else {
				sb.WriteByte(' ')
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func newModel(t *testing.T) *Model {
	t.Helper()
	dir := t.TempDir()
	app := handheld.New(dir+"/samples", dir+"/out", &handheld.NullAudio{})
	t.Cleanup(func() { handheld.SetSize(handheld.MinCols, handheld.MinRows); handheld.Terminal = false })
	handheld.Terminal = true
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

func (m *Model) text() string { return screenText(m.app.RenderFrame()) }

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
	press(m, "y")
	press(m, "l")
	press(m, "s")
	press(m, "b") // Select+B on SEQ: the pattern tools
	if !strings.Contains(m.text(), "TRACKS") {
		t.Fatalf("s then b opens the pattern tools:\n%s", m.text())
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
		f := m.app.RenderFrame()
		if f.Cols != sz[0] || f.Rows != sz[1] || f.Img.Bounds().Dx() != sz[0]*cellW {
			t.Fatalf("%v: frame %dx%d cells", sz, f.Cols, f.Rows)
		}
		col := -1
		for r := 0; r < f.Rows && col < 0; r++ {
			for c := 0; c+3 < f.Cols; c++ {
				if f.Text[r*f.Cols+c].R == 'B' && f.Text[r*f.Cols+c+1].R == 'P' && f.Text[r*f.Cols+c+2].R == 'M' { // the tempo, first in the panel
					col = c - 1
					break
				}
			}
		}
		want := sz[0] * 70 / 100
		if col < want-4 || col > want+4 {
			t.Errorf("%v: the panel starts at column %d, wanted about %d", sz, col, want)
		}
		if txt := m.text(); !strings.Contains(txt, "hold Y") || !strings.Contains(txt, "BPM") || !strings.Contains(txt, "autosave") {
			t.Errorf("%v: panel or footer missing:\n%s", sz, txt)
		}
	}
}

func TestSearchTypedOnTheKeyboard(t *testing.T) {
	m := newModel(t)
	press(m, "x")
	press(m, "y")
	press(m, "l")
	press(m, "y")
	press(m, "l") // FILES
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
