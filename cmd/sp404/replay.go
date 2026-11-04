package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"

	"github.com/moahbeatlab/404tui/replay"
)

const replayUsage = `sp404 replay: show a recorded performance, and render it as a video

  sp404 [-web addr] replay <take.wav> [take.events.jsonl]
        opens the replay in the browser (the events file is the one beside the WAV, name.events.jsonl, unless given;
        -web sets the address, 127.0.0.1 and any free port by default). Ctrl-C ends it.
  sp404 replay render <take.wav> [events.jsonl] [options]
        draws the replay frame by frame and writes a video (MP4: H.264 and AAC); see sp404 replay render -h

In the page
  Space play/pause, arrows 5 s (Shift 30 s), [ and ] move the events against the sound by 10 ms, f full screen.
  The bar at the bottom has:
    title             the title of the video (shown in the header)
    pattern from here the unit does not say which pattern plays: choose it where it starts (it snaps to the loop start);
                      the pads follow the pattern's bank. Click an amber tag on the timeline to remove it; "clear" removes all.
    style             palette, accent colour, background (colour, gradient, picture), logo, handle, subtitle
    - / + / fit, Ctrl+wheel   zoom the timeline (the lanes get wider and scroll; the slider stays the whole take, with a mark for the part shown)
    sidebar           captions, chapters, camera and style are tabs of a sidebar on the right (their buttons open and close it)
    undo, redo        steps back and forth through the patterns, captions, chapters and title (Ctrl-Z, Ctrl-Shift-Z)
    camera            a video of you playing (no sound needed): type its path, match one pad hit with the frame by eye (sync), then drag the box on
                      the picture. Scenes (a lane on the timeline) give the box a place and the panels shown, from a time on, for each format.
                      A scene's change can glide, cross-fade, pop in, slide in or cut (smooth, slow start and end, or springy);
                      panels that a scene turns on or off fade.
                      replay render puts the camera pictures into the video (-no-camera leaves them out for a quick draft)
    chapters          the chapters of the video (a lane on the timeline, a label on the picture, and the list for the description on YouTube)
    bus change        what a bus is used for can change in the take: from this time on bus N is X (a lane on the timeline)
    captions          text at times (a lane on the timeline: drag to move, the edges change the length). A caption can "look at" a pad,
                      a bus, the pattern and so on: the rest is blurred and dimmed, the target stays sharp
  What it shows: the pads (hits, counts, velocity, mutes, isolation), the pattern with its playhead and the settings of its
  pads, the four buses with effect and knobs, level and spectrum, the tempo, and a list of what happens (pads pressed, knobs
  moved, effects, start/stop, pattern changes, loop starts). Mutes, isolation and patterns sent from the web app are in the
  log, so they are shown as they were; for patterns changed on the unit itself the unit says nothing, so they are set by hand.
  What is set (title, cues, look) is kept beside the log in name.events.cues.json.

Page parameters (for one viewing; also usable with render -q "...")
  format=vertical   1080x1920 for shorts and reels      title=...       the title
  preset=midnight   neon, forest, mono, 404 dark         accent=%23ff3b30  the accent colour
  handle=@name  subtitle=...  bgcolor=%23101020  hud=0 (no bar)  t=12 (start at 12 s)  pattern=A1 (one pattern throughout)

` + renderUsage

// runReplay shows a recorded performance: sp404 replay take.wav [take.events.jsonl].
func runReplay(args []string, o options) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "-help" || args[0] == "help" {
		fmt.Println(replayUsage)
		return nil
	}
	if args[0] == "render" {
		return runRender(args[1:], o)
	}
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
