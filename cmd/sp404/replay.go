package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"

	"github.com/moahbeatlab/404tui/replay"
)

// runReplay shows a recorded performance: sp404 replay take.wav [take.events.jsonl].
func runReplay(args []string, o options) error {
	if len(args) < 1 || len(args) > 2 {
		return fmt.Errorf("usage: sp404 replay <audio.wav> [events.jsonl]   (without the second the file beside the WAV is used)")
	}
	events := ""
	if len(args) == 2 {
		events = args[1]
	}
	fmt.Println("reading the recording...")
	r, err := replay.Load(args[0], events)
	if err != nil {
		return err
	}
	addr := o.webAddr
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	url, stop, err := r.Serve(addr)
	if err != nil {
		return err
	}
	defer stop()
	fmt.Printf("%d events, %.1f s of sound. Open %s (Space plays, the arrows move, [ and ] move the events against the sound, f is full screen). Ctrl-C ends.\n",
		len(r.Events), r.Analysis.Duration, url)
	openBrowser(url)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	return nil
}

// openBrowser asks the system to open the address (nothing happens, quietly, where it cannot).
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if cmd.Start() == nil {
		go cmd.Wait()
	}
}
