package terminal

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/core"
	"github.com/moahbeatlab/404tui/device"
	"github.com/moahbeatlab/404tui/perflog"
	"github.com/moahbeatlab/404tui/sim"
)

// TestMakeACastTake writes the take of a scripted session (CAST_DEMO_DIR is the folder for take.events.jsonl): the keys of the script,
// with the pauses between them, and the screen after each. It is how the terminal view of the replay is tried without a unit.
func TestMakeACastTake(t *testing.T) {
	dir := os.Getenv("CAST_DEMO_DIR")
	if dir == "" {
		t.Skip("CAST_DEMO_DIR is not set")
	}
	base := t.TempDir()
	if err := core.WriteDemo(filepath.Join(base, "samples")); err != nil {
		t.Fatal(err)
	}
	core.Terminal = true
	defer func() { core.SetSize(core.MinCols, core.MinRows); core.Terminal = false }()
	app := core.New(filepath.Join(base, "samples"), filepath.Join(base, "out"), &core.NullAudio{})
	app.Connect = func() (core.Link, error) { return core.DeviceLink{Dev: device.New(sim.Demo(), "sim")}, nil }
	if err := app.Script("A A D R D D A  R A A D A  R A A U U U A", nil); err != nil { // kick, snare and a hat on the first pads
		t.Fatal(err)
	}
	tc, full := TrueColor, FullGlyphs
	m := NewModel(app, Options{Profile: &tc, Glyphs: &full})
	m.Update(tea.WindowSizeMsg{Width: 150, Height: 44})
	press(m, "esc") // the kit's last step leaves a dialog open

	t0 := time.Now()
	vt := time.Duration(0)
	defer core.SetCastClock(core.SetCastClock(func() time.Time { return t0.Add(vt) }))
	l, err := perflog.Create(filepath.Join(dir, "take.events.jsonl"), "take.wav", t0)
	if err != nil {
		t.Fatal(err)
	}
	l.Add(t0, "audio_start", perflog.P("rate", 48000))
	app.CastTo(l)
	m.View()
	for _, st := range castScript {
		vt += time.Duration(st.wait * float64(time.Second))
		for i := 0; i < st.ticks; i++ { // the program ticks while it waits
			m.Update(tickMsg(time.Now()))
		}
		press(m, st.key)
		m.View()
	}
	vt += 2 * time.Second
	m.View()
	l.Close()
}

var castScript = []struct {
	wait  float64
	ticks int
	key   string
}{
	{1.5, 0, "h"}, {1.5, 0, "l"}, {0.5, 0, "l"}, {0.5, 0, "j"}, {1.5, 0, "2"}, {2, 0, "3"}, {2, 0, "4"}, {2, 0, "5"}, {2, 0, "6"}, {2, 0, "1"},
}
