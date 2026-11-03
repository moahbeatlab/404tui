package core

import (
	"bytes"
	"flag"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/moahbeatlab/404tui/device"
	"github.com/moahbeatlab/404tui/sim"
)

// The handheld's screens are checked against pictures kept in testdata/handheld: what the terminal and the other computers
// get is drawn by other code (Terminal is set), and this test says when a change meant for them moved a pixel of the
// handheld. A change that is meant for the handheld too is made on purpose: go test ./core -run Handheld -update.
var updateGolden = flag.Bool("update", false, "write the pictures of the handheld's screens")

func TestHandheldScreensStayAsTheyAre(t *testing.T) {
	oldVersion := Version
	Version = "golden"
	defer func() { Version = oldVersion }()
	if Terminal {
		t.Skip("the handheld is drawn with Terminal off")
	}
	// the folders have fixed names: the screens show their paths
	base := filepath.Join(os.TempDir(), "404tui-golden")
	os.RemoveAll(base)
	defer os.RemoveAll(base)
	if err := WriteDemo(filepath.Join(base, "samples")); err != nil {
		t.Fatal(err)
	}
	a := New(filepath.Join(base, "samples"), filepath.Join(base, "out"), &NullAudio{})
	a.Connect = func() (Link, error) { return DeviceLink{Dev: device.New(sim.Demo(), "sim")}, nil }
	keys(t, a, kit)
	shots := []struct {
		name string
		play func()
	}{
		{"pads", func() { onScreen(t, a, Pads) }},
		{"padmenu", func() { keys(t, a, "A") }},
		{"padmenu-closed", func() { keys(t, a, "B") }},
		{"seq", func() {
			onScreen(t, a, Seq)
			keys(t, a, "A R R R R A D R A")
		}},
		{"files", func() { onScreen(t, a, Files) }},
		{"slice", func() {
			onScreen(t, a, Pads)
			a.go_(Slice)
		}},
		{"save", func() { onScreen(t, a, Save) }},
		{"unit", func() { onScreen(t, a, Unit) }},
		{"config", func() { onScreen(t, a, Config) }},
		{"perform", func() { onScreen(t, a, Perform) }},
		{"loop", func() { onScreen(t, a, Loop) }},
	}
	dir := filepath.Join("testdata", "handheld")
	for _, s := range shots {
		s.play()
		var buf bytes.Buffer
		if err := png.Encode(&buf, a.Render()); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, s.name+".png")
		if *updateGolden {
			os.MkdirAll(dir, 0o755)
			if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: no picture (go test ./core -run Handheld -update writes them): %v", s.name, err)
		}
		if !bytes.Equal(want, buf.Bytes()) {
			got := filepath.Join(os.TempDir(), "handheld-"+s.name+".png")
			os.WriteFile(got, buf.Bytes(), 0o644)
			t.Errorf("%s: the handheld's screen changed (the new one is %s, the old one %s)", s.name, got, path)
		}
	}
}
