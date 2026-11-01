package handheld

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Run is the main loop on a device: events from the pad (or the web page), a frame to the framebuffer after each
// one and every 100 ms.
func Run(a *App, fbPath, inputPath, webAddr string) error {
	events := make(chan Event, 64)
	var fb *Framebuffer
	if fbPath != "" {
		var err error
		if fb, err = OpenFramebuffer(fbPath); err != nil {
			return err
		}
		defer fb.Close()
	}
	if inputPath == "" {
		inputPath = findPad()
	}
	if inputPath != "" {
		if err := Evdev(inputPath, nil, events); err != nil {
			return err
		}
	}
	var web *Web
	if webAddr != "" {
		web = &Web{App: a}
		go func() { fmt.Fprintln(os.Stderr, web.Serve(webAddr)) }()
	}
	if fb == nil && web == nil {
		return errors.New("nothing to draw on: give -fb, -web or -script")
	}
	tick := time.NewTicker(100 * time.Millisecond)
	for {
		select {
		case e := <-events:
			if web != nil {
				web.mu.Lock()
			}
			a.Dispatch(e)
			if web != nil {
				web.mu.Unlock()
			}
		case <-tick.C:
			a.Tick()
		}
		if web != nil {
			web.mu.Lock()
		}
		if fb != nil {
			fb.Show(a.Render())
		}
		if web != nil {
			web.mu.Unlock()
		}
	}
}

// findPad looks for a gamepad among the input devices: the handler of a device named like one.
func findPad() string {
	b, err := os.ReadFile("/proc/bus/input/devices")
	if err != nil {
		return ""
	}
	for _, blk := range strings.Split(string(b), "\n\n") {
		low := strings.ToLower(blk)
		if !strings.Contains(low, "joypad") && !strings.Contains(low, "gamepad") && !strings.Contains(low, "joystick") {
			continue
		}
		for _, f := range strings.Fields(blk) {
			if strings.HasPrefix(f, "event") {
				return "/dev/input/" + f
			}
		}
	}
	return ""
}
