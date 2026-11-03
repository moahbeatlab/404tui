package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/moahbeatlab/404tui/device"
	"github.com/moahbeatlab/404tui/handheld"
	"github.com/moahbeatlab/404tui/playback"
	"github.com/moahbeatlab/404tui/serialport"
	"github.com/moahbeatlab/404tui/sim"
	"github.com/moahbeatlab/404tui/termui"
	"github.com/moahbeatlab/404tui/version"
)

// runLab is the lab: the screens of the handheld (pads, sequencer, files, slicer, save, unit) in this terminal,
// laid out for the size of the window and played from the keyboard.
func runLab(args []string, o options) error {
	fs := flag.NewFlagSet("lab", flag.ContinueOnError)
	home, _ := os.UserHomeDir()
	base := filepath.Join(home, "404tui")
	samples := fs.String("samples", filepath.Join(base, "samples"), "the folder the file browser starts in")
	out := fs.String("out", filepath.Join(base, "out"), "where projects are written: the card folder is made in it; settings and autosaves go beside it")
	project := fs.String("project", "", "a project folder (with a PADCONF.BIN) to open")
	ascii := fs.Bool("basic-glyphs", false, "draw with block characters only (no braille or eighths), for a terminal font that has little")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, "usage: sp404 [flags] lab [lab flags]\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := os.MkdirAll(*samples, 0o755); err != nil {
		return err
	}
	os.MkdirAll(base, 0o755)
	quietStderr(filepath.Join(base, "lab.log"))
	opt := termui.Options{Samples: *samples, Out: *out, Project: *project, Version: version.Version}
	if *ascii {
		g := termui.BasicGlyphs
		opt.Glyphs = &g
	}
	opt.Audio = func() (handheld.Audio, error) {
		p, err := playback.OpenBackend(o.audio)
		if err != nil {
			return nil, err
		}
		return p, nil
	}
	opt.Connect = func() (handheld.Link, error) {
		if o.simulate {
			return handheld.DeviceLink{Dev: device.New(sim.Demo(), "sim")}, nil
		}
		p, name, err := serialport.OpenFirst()
		if err != nil {
			return nil, err
		}
		return handheld.DeviceLink{Dev: device.New(p, name)}, nil
	}
	return termui.Run(opt)
}
