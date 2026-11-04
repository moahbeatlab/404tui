package main

import (
	"runtime"

	"github.com/moahbeatlab/404tui/midiin"
	"github.com/moahbeatlab/404tui/record"
	"os"
	"path/filepath"

	"github.com/moahbeatlab/404tui/core"
	"github.com/moahbeatlab/404tui/dbg"
	"github.com/moahbeatlab/404tui/device"
	"github.com/moahbeatlab/404tui/platform/terminal"
	"github.com/moahbeatlab/404tui/playback"
	"github.com/moahbeatlab/404tui/serialport"
	"github.com/moahbeatlab/404tui/sim"
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
	if o.trace || o.verbose { // -v and -trace turn the debug log on from the start (CONFIG does it while the program runs)
		mode := dbg.Basic
		if o.trace {
			mode = dbg.Trace
		}
		if p, err := dbg.Set(mode, o.logPath); err == nil {
			defer dbg.Set(dbg.Off, "")
			_ = p
		}
	}
	opt := terminal.Options{Samples: o.samples, Out: o.out, Project: o.project, Version: version.Full()}
	if o.basicGlyphs {
		g := terminal.BasicGlyphs
		opt.Glyphs = &g
	}
	opt.Platform = core.Platform{
		Name: runtime.GOOS,
		Audio: func() (core.Audio, error) {
			if o.audio == "silent" { // for tests: everything works, nothing is played
				return &core.NullAudio{}, nil
			}
			p, err := playback.OpenBackend(o.audio)
			if err != nil {
				return nil, err
			}
			return p, nil
		},
		Recorder: record.Default(),
		MIDI:     midiin.Default(),
		Connect: func() (core.Link, error) {
			if o.simulate {
				return core.DeviceLink{Dev: device.New(sim.Demo(), "sim", device.WithLogger(dbg.Logger))}, nil
			}
			p, name, err := serialport.OpenFirst()
			if err != nil {
				return nil, err
			}
			return core.DeviceLink{Dev: device.New(p, name, device.WithLogger(dbg.Logger))}, nil
		},
		Keyboard: true,
	}
	return terminal.Run(opt)
}
