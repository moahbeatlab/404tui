package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/moahbeatlab/404tui/perflog"
	"github.com/muesli/termenv"
)

// TestMakeACastTake writes a take of a scripted session (CAST_DEMO_DIR is the folder for events.jsonl): the keys of the script, with
// the pauses between them, and the screen after each of them. It is how the terminal view of the replay is tried without a unit.
func TestMakeACastTake(t *testing.T) {
	dir := os.Getenv("CAST_DEMO_DIR")
	if dir == "" {
		t.Skip("CAST_DEMO_DIR is not set")
	}
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor) // a terminal has colours; the test's output has none unless it is told
	defer lipgloss.SetColorProfile(prev)
	r := open(t)
	r.send(tea.WindowSizeMsg{Width: 130, Height: 38})
	base := time.Now()
	vt := time.Duration(0)
	castClock = func() time.Time { return base.Add(vt) }
	defer func() { castClock = time.Now }()
	l, err := perflog.Create(filepath.Join(dir, "take.events.jsonl"), "take.wav", base)
	if err != nil {
		t.Fatal(err)
	}
	m := r.m()
	m.ranger = true // the three-column files view, as the program starts (the other tests turn it off)
	m.perfLog = l
	r.model = m
	l.Add(base, "audio_start", perflog.P("rate", 48000))
	r.model.View()
	for _, st := range castScript {
		vt += time.Duration(st.wait * float64(time.Second))
		r.press(st.key)
		r.model.View()
	}
	vt += 2 * time.Second
	r.model.View()
	l.Close()
}

var castScript = []struct {
	wait float64
	key  string
}{
	{2, "1"}, {2, "2"}, {1, "down"}, {0.6, "down"}, {0.6, "enter"}, {1.5, "2"}, {1.5, "3"}, {2, "4"}, {2, "5"}, {2, "6"}, {2, "7"}, {2, "1"},
}
