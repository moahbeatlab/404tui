package tui

import (
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestWriteScreens writes what each section looks like to the file named by SCREENS_OUT,
// for a look at the layout; it does nothing otherwise.
func TestWriteScreens(t *testing.T) {
	out := os.Getenv("SCREENS_OUT")
	if out == "" {
		t.Skip("SCREENS_OUT not set")
	}
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 110, Height: 34})
	var text string
	for _, keys := range [][]string{{"l", "v"}, {"2", "l", "enter"}, {"3", "tab", "j", "j"}, {"enter"}, {"esc", "4"}} {
		s.press(keys...)
		text += s.model.View() + "\n\n"
	}
	s.send(tea.WindowSizeMsg{Width: 150, Height: 34})
	s.press("1")
	text += s.model.View() + "\n"
	if err := os.WriteFile(out, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
