package handheld

import (
	"errors"
	"fmt"
	"log"
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
			log.Printf("framebuffer %s: %v", fbPath, err)
			return err
		}
		log.Printf("framebuffer %s: %dx%d (virtual %dx%d, offset %d,%d) %d bpp, stride %d; drawing %dx%d at %d,%d",
			fbPath, fb.w, fb.h, fb.vw, fb.vh, fb.xoff, fb.yoff, fb.bpp, fb.stride, W, H, max((fb.w-W)/2, 0), max((fb.h-H)/2, 0))
		defer fb.Close()
	}
	if inputPath == "" {
		inputPath = findPad()
		log.Printf("input: searched, found %q", inputPath)
	}
	if inputPath != "" {
		if err := Evdev(inputPath, nil, events); err != nil {
			log.Printf("input %s: %v", inputPath, err)
			return err
		}
		log.Printf("input: reading %s", inputPath)
	} else {
		log.Println("input: NO pad found; devices were:\n" + deviceNames())
	}
	var web *Web
	if webAddr != "" {
		web = &Web{App: a}
		go func() { err := web.Serve(webAddr); fmt.Fprintln(os.Stderr, err); log.Println("web:", err) }()
		log.Println("web: serving on", webAddr)
	}
	if fb == nil && web == nil {
		log.Println("nothing to draw on: no -fb and no -web")
		return errors.New("nothing to draw on: give -fb, -web or -script")
	}
	tick := time.NewTicker(100 * time.Millisecond)
	log.Println("running: screen", screenNames[a.scr])
	frames, beat := 0, time.Now()
	for {
		select {
		case e := <-events:
			if e.Down && e.Repeat == 0 {
				log.Printf("key %s down (screen %s)", keyNames[e.Key], screenNames[a.scr])
			}
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
			if frames++; frames == 1 {
				log.Println("first frame drawn")
			}
		}
		if web != nil {
			web.mu.Unlock()
		}
		if time.Since(beat) > time.Minute {
			beat = time.Now()
			log.Printf("alive: %d frames, screen %s", frames, screenNames[a.scr])
		}
	}
}

// deviceNames lists the names and handlers of the input devices, for the log.
func deviceNames() string {
	b, err := os.ReadFile("/proc/bus/input/devices")
	if err != nil {
		return err.Error()
	}
	var sb strings.Builder
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "N: ") || strings.HasPrefix(l, "H: ") {
			sb.WriteString("  " + l + "\n")
		}
	}
	return sb.String()
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
