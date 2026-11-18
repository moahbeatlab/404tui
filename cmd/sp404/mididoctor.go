package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/moahbeatlab/404tui/midiin"
)

// midiDoctor is `sp404 doctor midi --channel 11 [--port name]`: a few notes on each channel to the unit's MIDI port, to hear
// whether a synth behind the unit (MIDI thru) gets them. Channels: 11, 11-14 or 11,13.
func midiDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor midi", flag.ContinueOnError)
	chFlag := fs.String("channel", "", "the MIDI channels: 11, 11-14 or 11,13")
	port := fs.String("port", os.Getenv("SP404_MIDI_PORT"), "a part of the MIDI output's name (default: 404, as the app)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *chFlag == "" {
		return fmt.Errorf("which channels? sp404 doctor midi --channel 11 (or 11-14, or 11,13)")
	}
	chans, err := parseChannels(*chFlag)
	if err != nil {
		return err
	}
	part := *port
	if part == "" {
		part = "404"
	}
	outs, ok := midiin.Default().(midiin.Outputs)
	if !ok {
		return midiin.ErrUnavailable
	}
	out, name, err := outs.OpenOut(part)
	if err != nil {
		return err
	}
	defer out.Close()
	fmt.Printf("sending to %s\n", name)
	names := []string{"C3", "D#3", "G3", "C4"}
	for _, ch := range chans {
		fmt.Printf("channel %d: %s ", ch, strings.Join(names, " "))
		for _, n := range []byte{48, 51, 55, 60} {
			if err := out.Send([]byte{0x90 | byte(ch-1), n, 110}); err != nil {
				return err
			}
			time.Sleep(300 * time.Millisecond)
			out.Send([]byte{0x80 | byte(ch-1), n, 0})
			time.Sleep(60 * time.Millisecond)
			fmt.Print(".")
		}
		out.Send([]byte{0xB0 | byte(ch-1), 123, 0}) // all notes off
		fmt.Println()
		time.Sleep(700 * time.Millisecond)
	}
	fmt.Println("done: did the synth play each channel?")
	return nil
}

func parseChannels(s string) ([]int, error) {
	var out []int
	for _, part := range strings.Split(s, ",") {
		lo, hi, isRange := strings.Cut(part, "-")
		a, err1 := strconv.Atoi(strings.TrimSpace(lo))
		b := a
		var err2 error
		if isRange {
			b, err2 = strconv.Atoi(strings.TrimSpace(hi))
		}
		if err1 != nil || err2 != nil || a < 1 || b > 16 || a > b {
			return nil, fmt.Errorf("channels %q: such as 11, 11-14 or 11,13", s)
		}
		for c := a; c <= b; c++ {
			out = append(out, c)
		}
	}
	return out, nil
}
