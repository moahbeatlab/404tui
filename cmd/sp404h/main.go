// Command sp404h is the handheld front end for the SP-404MKII: samples, slicing, patterns and projects
// on a console with a D-pad and four buttons (R36S under ArkOS). See package handheld.
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
)

func main() {
	root := flag.String("samples", "samples", "folder of samples to browse")
	out := flag.String("out", "", "where projects are written, the card (default: <samples>/../out)")
	open := flag.String("open", "", "a project folder (with a PADCONF.BIN) to start from")
	demo := flag.Bool("demo", false, "write demo samples into -samples first")
	script := flag.String("script", "", "keys to play, then exit (U D L R A B X Y, Y+R, R*8, @name for a PNG)")
	shots := flag.String("shots", "shots", "folder for the PNGs of @name")
	fbPath := flag.String("fb", "", "framebuffer to draw on, e.g. /dev/fb0")
	input := flag.String("input", "", "input device of the pad, e.g. /dev/input/event2")
	web := flag.String("web", "", "serve the screen and buttons as a web page on this address, e.g. :8405")
	silent := flag.Bool("silent", false, "no audio")
	simulate := flag.Bool("sim", false, "the UNIT screen finds a simulated unit")
	flag.Parse()

	if *out == "" {
		*out = filepath.Join(filepath.Dir(*root), "out")
	}
	if *demo {
		if err := handheld.WriteDemo(*root); err != nil {
			fatal(err)
		}
	}
	var audio handheld.Audio
	if *script != "" {
		audio = &handheld.NullAudio{}
	}
	if !*silent && *script == "" {
		if p, err := playback.Open(); err == nil {
			audio = p
		} else {
			fmt.Fprintln(os.Stderr, "no audio:", err)
		}
	}
	app := handheld.New(*root, *out, audio)
	app.Connect = func() (handheld.Link, error) {
		if *simulate {
			return handheld.DeviceLink{Dev: device.New(sim.Demo(), "sim")}, nil
		}
		p, name, err := serialport.OpenFirst()
		if err != nil {
			return nil, err
		}
		return handheld.DeviceLink{Dev: device.New(p, name)}, nil
	}
	if *open != "" {
		if err := app.OpenProject(*open); err != nil {
			fatal(err)
		}
	}
	if *script != "" {
		os.MkdirAll(*shots, 0o755)
		err := app.Script(*script, func(name string) {
			if err := handheld.WritePNG(filepath.Join(*shots, name+".png"), app.Render()); err != nil {
				fatal(err)
			}
		})
		if err != nil {
			fatal(err)
		}
		return
	}
	if err := handheld.Run(app, *fbPath, *input, *web); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "sp404h:", err)
	os.Exit(1)
}
