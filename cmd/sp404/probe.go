package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/moahbeatlab/404tui/midiin"
	"github.com/moahbeatlab/404tui/proto"
)

const probeUsage = `usage:
  sp404 probe pad <pad> [seconds]       press a pad over the USB link, as holding it (A1..J16, 1 second by default); play a pattern first
                                        to hear whether it layers over it and goes through the bus effects
  sp404 probe pc <bank> <pattern> [now|loop]
                                        send a Program Change (bank A..J = MIDI channel, pattern 1..16) and print what the unit sends for 12 s:
                                        now = at once, loop = at the next loop start (the note on channel 13 the unit sends at every loop)
The MIDI port is the first with "404" in its name; SP404_MIDI_PORT changes that.`

// runProbe tries what the unit does when it is told something, so that it is known before it is built into the program.
func runProbe(args []string, o options, l *slog.Logger) error {
	if len(args) < 2 {
		return fmt.Errorf("%s", probeUsage)
	}
	switch args[0] {
	case "pad":
		pad, err := proto.ParsePad(args[1])
		if err != nil {
			return err
		}
		secs := 1.0
		if len(args) > 2 {
			if _, err := fmt.Sscanf(args[2], "%g", &secs); err != nil || secs <= 0 {
				return fmt.Errorf("seconds: %q", args[2])
			}
		}
		dev, err := open(o, l)
		if err != nil {
			return err
		}
		defer dev.Close()
		fmt.Printf("pressing %s for %.1f s...\n", pad, secs)
		t0 := time.Now()
		if err := dev.PreviewStart(pad); err != nil {
			return err
		}
		time.Sleep(time.Duration(secs * float64(time.Second)))
		if err := dev.PreviewStop(pad); err != nil {
			return err
		}
		fmt.Printf("done after %.2f s: did it sound, over the pattern, and through the bus effects?\n", time.Since(t0).Seconds())
		return nil
	case "pc":
		return probeProgramChange(args[1:])
	}
	return fmt.Errorf("%s", probeUsage)
}

func probeProgramChange(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("%s", probeUsage)
	}
	bank := strings.ToUpper(args[0])
	if len(bank) != 1 || bank[0] < 'A' || bank[0] > 'J' {
		return fmt.Errorf("bank %q: A to J", args[0])
	}
	var pat int
	if _, err := fmt.Sscanf(args[1], "%d", &pat); err != nil || pat < 1 || pat > 16 {
		return fmt.Errorf("pattern %q: 1 to 16", args[1])
	}
	when := "now"
	if len(args) > 2 {
		when = args[2]
	}
	if when != "now" && when != "loop" {
		return fmt.Errorf("%q: now or loop", when)
	}
	part := os.Getenv("SP404_MIDI_PORT")
	if part == "" {
		part = "404"
	}
	src := midiin.Default()
	outs, ok := src.(midiin.Outputs)
	if !ok {
		return midiin.ErrUnavailable
	}
	out, outName, err := outs.OpenOut(part)
	if err != nil {
		return err
	}
	defer out.Close()

	type stamped struct {
		e midiin.Event
		t time.Time
	}
	events := make(chan stamped, 4096)
	stop, inName, err := src.Open(part, func(e midiin.Event) {
		select {
		case events <- stamped{e, time.Now()}:
		default:
		}
	})
	if err != nil {
		return err
	}
	defer stop()
	fmt.Printf("MIDI in: %s, out: %s\n", inName, outName)

	var lastMarker time.Time
	var period time.Duration
	marker := func(s stamped) bool {
		if s.e.Kind == midiin.NoteOn && s.e.Channel == 13 && s.e.Data1 == 48 {
			if !lastMarker.IsZero() {
				period = s.t.Sub(lastMarker)
			}
			lastMarker = s.t
			return true
		}
		return false
	}
	if when == "loop" {
		fmt.Println("waiting for the next loop start (is a pattern playing?)...")
		timeout := time.After(30 * time.Second)
	wait:
		for {
			select {
			case s := <-events:
				if marker(s) {
					break wait
				}
			case <-timeout:
				return fmt.Errorf("no loop start in 30 s: the pattern does not run, or the unit does not send it")
			}
		}
	} else { // learn the loop from what comes in for a moment, to say where in the loop it was sent
		deadline := time.After(1500 * time.Millisecond)
	listen:
		for {
			select {
			case s := <-events:
				marker(s)
			case <-deadline:
				break listen
			}
		}
	}
	sent := time.Now()
	msg := []byte{0xC0 | byte(bank[0]-'A'), byte(pat - 1)}
	if err := out.Send(msg); err != nil {
		return err
	}
	where := "no loop start known"
	if !lastMarker.IsZero() {
		where = fmt.Sprintf("%.2f s after the last loop start", sent.Sub(lastMarker).Seconds())
		if period > 0 {
			where += fmt.Sprintf(" (a loop takes %.2f s)", period.Seconds())
		}
	}
	fmt.Printf("sent program change %d on channel %d (%s %d): %s\n", pat-1, int(bank[0]-'A')+1, bank, pat, where)
	fmt.Println("what the unit sends for 12 s (the clock is only counted); watch and listen when the pattern changes:")
	end := time.After(12 * time.Second)
	clocks := 0
	for {
		select {
		case s := <-events:
			if s.e.Kind == midiin.Clock {
				clocks++
				continue
			}
			note := ""
			if marker(s) {
				note = "   <- loop start"
			}
			fmt.Printf("  +%5.2f s  %s%s\n", s.t.Sub(sent).Seconds(), s.e, note)
		case <-end:
			fmt.Printf("%d clock ticks. Tell me when the pattern changed, and which pattern plays now.\n", clocks)
			return nil
		}
	}
}
