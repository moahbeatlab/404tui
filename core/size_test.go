package core

import (
	"fmt"
	"strings"
	"testing"
)

// The screens are laid out in cells for the size of the window: on every size every screen and dialog must draw without
// a letter landing on another one, and the frame is as big as the screen.
func TestEveryScreenFitsEverySize(t *testing.T) {
	oldVersion := Version
	Version = "0.1-alpha"
	t.Cleanup(func() { SetSize(MinCols, MinRows); Terminal = false; Version = oldVersion })
	states := []struct {
		name string
		do   string // keys, after the kit is on A1..A3
	}{
		{"pads", ""},
		{"pad panel", "A"},
		{"pad panel shape", "A ]"},
		{"pad tools", "A ] ]"},
		{"seq", "Y+R A"},
		{"seq, a note", "Y+R A D A"},
		{"note panel", "Y+R A B"},
		{"pattern tools", "Y+R A S+B"},
		{"add track dialog", "Y+R A Y+X"},
		{"pattern picker", "Y+R A Y+B"},
		{"files", "Y+R Y+R"},
		{"search keyboard", "Y+R Y+R Y+A"},
		{"slice", "Y+R Y+R Y+R"},
		{"slice cut", "Y+R Y+R Y+R Y+A"},
		{"slice settings", "Y+R Y+R Y+R Y+A Y+B"},
		{"slice tools", "Y+R Y+R Y+R Y+A S+B"},
		{"save", "Y+L Y+L Y+L Y+L"},
		{"unit", "Y+L Y+L Y+L"},
		{"config", "Y+L Y+L"},
		{"config, a prompt", "Y+L Y+L D D D D D D D D D A"},
		{"perform", "Y+L"},
		{"help", "Y"},
	}
	for _, sz := range [][2]int{{80, 30}, {81, 31}, {100, 34}, {120, 40}, {132, 43}, {160, 48}, {200, 60}, {240, 70}} {
		for _, st := range states {
			t.Run(fmt.Sprintf("%dx%d %s", sz[0], sz[1], st.name), func(t *testing.T) {
				Terminal = sz[0]%2 == 0 // half of them as in a terminal, with the keyboard's hints
				SetSize(MinCols, MinRows)
				a, _ := newApp(t)
				keys(t, a, kit)
				SetSize(sz[0], sz[1])
				if st.do != "" {
					keys(t, a, st.do)
				}
				pix := a.Render()
				if pix.Bounds().Dx() != sz[0]*CellW || pix.Bounds().Dy() != sz[1]*CellH {
					t.Fatalf("frame is %v for %dx%d cells", pix.Bounds(), sz[0], sz[1])
				}
				scr := a.RenderCells()
				if scr.Cols != sz[0] || scr.Rows != sz[1] || len(scr.Cells) != sz[0]*sz[1] {
					t.Fatalf("text screen is %dx%d for %dx%d cells", scr.Cols, scr.Rows, sz[0], sz[1])
				}
				if len(scr.Clash) > 0 {
					t.Errorf("letters over letters (%d): %s", len(scr.Clash), strings.Join(scr.Clash[:min(len(scr.Clash), 4)], "; "))
				}
				// the panel is on the right, a third of the width at least 24 cells, and nothing of the screen is in it
				if panelCols < 24 || panelCols*10 < sz[0]*3-9 {
					t.Errorf("panel is %d of %d columns", panelCols, sz[0])
				}
			})
		}
	}
}
