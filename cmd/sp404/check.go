package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/moahbeatlab/404tui/doctor"
	"github.com/moahbeatlab/404tui/midiin"
	"github.com/moahbeatlab/404tui/playback"
	"github.com/moahbeatlab/404tui/record"
	"github.com/moahbeatlab/404tui/serialport"
	"github.com/moahbeatlab/404tui/tui"
	"github.com/moahbeatlab/404tui/version"
	"github.com/moahbeatlab/404tui/web"
)

// A check is FAIL when the program cannot work on this computer, WARN when a part of it cannot
// (no sound, no MIDI) and INFO for what is only reported.
const (
	levelOK   = "ok  "
	levelWarn = "WARN"
	levelFail = "FAIL"
	levelInfo = "info"
)

// selfCheck is the check of a whole installation, without a unit: the computer, then the program on
// a simulated unit, then, if one is plugged in, the doctor on the real one. It returns an error
// when something that must work does not.
func selfCheck(o options, w io.Writer, l *slog.Logger) error {
	fails, warns := 0, 0
	line := func(level, name, detail string) {
		switch level {
		case levelFail:
			fails++
		case levelWarn:
			warns++
		}
		fmt.Fprintf(w, "%s  %-26s %s\n", level, name, detail)
	}
	fmt.Fprintf(w, "404tui %s check: %s/%s, %s\n\n", version.Version, runtime.GOOS, runtime.GOARCH, runtime.Version())

	// The computer.
	line(levelInfo, "terminal", terminalInfo())
	if dir, err := os.UserConfigDir(); err != nil {
		line(levelWarn, "settings folder", "none: settings are not remembered ("+err.Error()+")")
	} else if err := writable(filepath.Join(dir, "404tui")); err != nil {
		line(levelWarn, "settings folder", err.Error())
	} else {
		line(levelOK, "settings folder", filepath.Join(dir, "404tui"))
	}
	if dir, err := os.UserCacheDir(); err != nil {
		line(levelWarn, "cache folder", "none: the debug log goes to the current folder")
	} else if err := writable(filepath.Join(dir, "404tui")); err != nil {
		line(levelWarn, "cache folder", err.Error())
	} else {
		line(levelOK, "cache folder", filepath.Join(dir, "404tui"))
	}
	if ports, err := serialport.All(); err != nil {
		line(levelFail, "serial ports", "cannot list them: "+err.Error())
	} else {
		unit := 0
		for _, p := range ports {
			if p.Matches {
				unit++
			}
		}
		line(levelOK, "serial ports", fmt.Sprintf("%d found, %d of them an SP-404MKII", len(ports), unit))
	}
	if out, err := playback.OpenBackend(o.audio); err != nil {
		line(levelWarn, "audio output", "none, files cannot be played here: "+err.Error())
	} else {
		name := playback.OutputName()
		if name == "" {
			name = backendName(o.audio)
		}
		line(levelOK, "audio output", name)
		if c, ok := out.(io.Closer); ok {
			c.Close()
		}
	}
	if tool := record.Tool(record.Default()); tool == "" {
		line(levelWarn, "recording", "neither ffmpeg nor sox is installed: Ctrl-R cannot record")
	} else {
		line(levelOK, "recording", "records with "+tool)
	}
	line(webCheck())
	switch ports, err := midiin.Default().Ports(); {
	case errors.Is(err, midiin.ErrUnavailable):
		line(levelWarn, "MIDI input", "not in this build (needs go build -tags midi, with cgo and the ALSA headers on Linux; the release builds have it)")
	case err != nil:
		line(levelWarn, "MIDI input", err.Error())
	default:
		line(levelOK, "MIDI input", fmt.Sprintf("%d input(s): %s", len(ports), strings.Join(ports, ", ")))
	}

	// The program, on the simulated unit.
	sim := options{simulate: true}
	dev, err := open(sim, l)
	if err != nil {
		line(levelFail, "simulated unit", err.Error())
	} else {
		results := doctor.Run(dev, io.Discard, l, doctor.Options{})
		if n := doctor.Failures(results); n > 0 {
			for _, r := range results {
				if !r.OK() {
					line(levelFail, "doctor (simulated): "+r.Name, r.Err.Error())
				}
			}
		} else {
			line(levelOK, "doctor (simulated)", fmt.Sprintf("%d checks", len(results)))
		}
		start := time.Now()
		draws, err := tui.Smoke(dev)
		if err != nil {
			line(levelFail, "screens (simulated)", err.Error())
		} else {
			line(levelOK, "screens (simulated)", fmt.Sprintf("7 tabs at 3 sizes, %d drawings, %s", draws, time.Since(start).Round(time.Millisecond)))
		}
		dev.Close()
	}

	// A real unit, when there is one.
	if o.simulate {
		line(levelInfo, "unit", "skipped (-sim)")
	} else if ports, err := serialport.Find(); err != nil || len(ports) == 0 {
		line(levelInfo, "unit", "no SP-404MKII plugged in: only the simulation was checked")
	} else if real, err := open(o, l); err != nil {
		line(levelFail, "unit", err.Error())
	} else {
		results := doctor.Run(real, io.Discard, l, doctor.Options{})
		real.Close()
		if n := doctor.Failures(results); n > 0 {
			for _, r := range results {
				if !r.OK() {
					line(levelFail, "doctor: "+r.Name, r.Err.Error())
				}
			}
		} else {
			line(levelOK, "doctor", fmt.Sprintf("%d read-only checks on %s", len(results), ports[0].Name))
		}
	}

	fmt.Fprintf(w, "\n%d failed, %d warnings\n", fails, warns)
	if fails > 0 {
		return fmt.Errorf("%d check(s) failed", fails)
	}
	return nil
}

func terminalInfo() string {
	size := "size unknown"
	if st, err := os.Stdout.Stat(); err == nil && st.Mode()&os.ModeCharDevice == 0 {
		size = "output is not a terminal"
	}
	colour := os.Getenv("COLORTERM")
	if colour == "" {
		colour = os.Getenv("TERM")
	}
	if colour == "" {
		colour = "TERM not set"
	}
	return size + ", " + colour
}

// writable makes the folder and writes and removes a file in it.
func writable(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "check-*")
	if err != nil {
		return err
	}
	f.Close()
	return os.Remove(f.Name())
}

// webCheck starts the web page of Perform on a free local port, fetches it, and opens a WebSocket to it:
// what a phone does, on this computer.
func webCheck() (level, name, detail string) {
	srv := &web.Server{}
	l, err := srv.Listen("127.0.0.1:0")
	if err != nil {
		return levelFail, "web page", "cannot listen: " + err.Error()
	}
	defer l.Close()
	resp, err := http.Get("http://" + l.Addr().String() + "/")
	if err != nil {
		return levelFail, "web page", err.Error()
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return levelFail, "web page", fmt.Sprintf("the page answers %d", resp.StatusCode)
	}
	urls := srv.URLs(l)
	return levelOK, "web page", "serves Perform (sp404 serve): " + strings.Replace(urls[0], "/?key=", "/", 1)
}
