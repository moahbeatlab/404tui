package main

import (
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

// runUnified is the full-screen UI: the screens of the handheld version (pads, sequencer, files, slicer, save, unit) in this
// terminal, laid out for the size of the window, played from the keyboard.
func runUnified(o options) error {
	base := filepath.Dir(filepath.Clean(o.out))
	if err := os.MkdirAll(o.samples, 0o755); err != nil {
		return err
	}
	os.MkdirAll(base, 0o755)
	quietStderr(filepath.Join(base, "404tui.log"))
	opt := termui.Options{Samples: o.samples, Out: o.out, Project: o.project, Version: version.Version}
	if o.basicGlyphs {
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
