package main

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
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
  sp404 probe watch [seconds] [pad...]         poll the unit and print everything that changes, to find out whether it says which pattern plays:
                                        the status (ten times a second, byte by byte), the project settings (once a second) and the MIDI
                                        it sends. Type a line and press Enter to put a mark into the list ("pattern 2 on the unit").
                                        Pads (B3 C7 ...) are read once a second too, to see whether a
                                        mute or the like changes anything in them. Ends after the seconds (300 by default) or with Ctrl+C, and keeps a copy in probe-watch-<time>.txt
The MIDI port is the first with "404" in its name; SP404_MIDI_PORT changes that.`

// runProbe tries what the unit does when it is told something, so that it is known before it is built into the program.
func runProbe(args []string, o options, l *slog.Logger) error {
	if len(args) > 0 && args[0] == "watch" {
		return runProbeWatch(args[1:], o, l)
	}
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

func runProbeWatch(args []string, o options, l *slog.Logger) error {
	secs := 300.0
	var pads []proto.PadIndex
	for i, a := range args {
		if pad, err := proto.ParsePad(a); err == nil {
			pads = append(pads, pad)
		} else if _, err := fmt.Sscanf(a, "%g", &secs); err != nil || secs <= 0 || i > 0 {
			return fmt.Errorf("%q: seconds first, then pads like B3", a)
		}
	}
	dev, err := open(o, l)
	if err != nil {
		return err
	}
	defer dev.Close()
	name := "probe-watch-" + time.Now().Format("2006-01-02_15-04-05") + ".txt"
	f, err := os.Create(name)
	if err != nil {
		return err
	}
	defer f.Close()
	var mu sync.Mutex
	t0 := time.Now()
	out := func(format string, a ...any) {
		mu.Lock()
		defer mu.Unlock()
		line := fmt.Sprintf("%7.2f  ", time.Since(t0).Seconds()) + fmt.Sprintf(format, a...)
		fmt.Println(line)
		fmt.Fprintln(f, line)
	}
	out("watching for %.0f s (copy in %s). Do things on the unit; type a line + Enter to mark what you did.", secs, name)

	// the MIDI the unit sends (the clock is counted, not listed)
	part := os.Getenv("SP404_MIDI_PORT")
	if part == "" {
		part = "404"
	}
	var clocks atomic.Int64
	if stop, in, err := midiin.Default().Open(part, func(e midiin.Event) {
		if e.Kind == midiin.Clock {
			clocks.Add(1)
			return
		}
		note := ""
		if e.Kind == midiin.NoteOn && e.Channel == 13 && e.Data1 == 48 {
			note = "   <- loop start"
		}
		out("MIDI    %s%s", e, note)
	}); err == nil {
		defer stop()
		out("MIDI in: %s", in)
	} else {
		out("no MIDI (%v): only the serial link is watched", err)
	}

	go func() { // marks
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			if t := strings.TrimSpace(sc.Text()); t != "" {
				out("MARK    %s", t)
			}
		}
	}()

	diff := func(what string, prev, cur []byte, skip func(i int) bool) {
		var ch []string
		for i := 0; i < len(cur) || i < len(prev); i++ {
			if skip != nil && skip(i) {
				continue
			}
			var a, b = -1, -1
			if i < len(prev) {
				a = int(prev[i])
			}
			if i < len(cur) {
				b = int(cur[i])
			}
			if a != b {
				ch = append(ch, fmt.Sprintf("[%d] %s>%s", i, hexOrNone(a), hexOrNone(b)))
			}
		}
		if len(ch) > 0 {
			out("%-8s %s   (%s)", what, strings.Join(ch, "  "), hexline(cur, skip))
		}
	}
	var lastStatus, lastSettings []byte
	lastPad := map[proto.PadIndex][]byte{}
	end := time.After(time.Duration(secs * float64(time.Second)))
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	n := 0
	for {
		select {
		case <-end:
			out("done (%d clock ticks)", clocks.Load())
			fmt.Println("The list is in", name, ": send it to me together with what you did and when.")
			return nil
		case <-sig:
			out("stopped (%d clock ticks)", clocks.Load())
			fmt.Println("The list is in", name, ": send it to me together with what you did and when.")
			return nil
		case <-tick.C:
		}
		n++
		raw, err := dev.RawStatus()
		if err == nil {
			if lastStatus == nil {
				out("STATUS  %s", hexline(raw, frames))
			} else {
				diff("STATUS", lastStatus, raw, frames) // the bytes that count the frames of a sounding pad change all the time
			}
			lastStatus = raw
			if n%10 == 1 {
				proj := byte(0)
				if len(raw) > 1 {
					proj = raw[1]
				}
				if ps, err := dev.ProjectSettings(proj); err == nil {
					cur := ps.Raw()
					if lastSettings == nil {
						out("SETTING %d bytes: %s", len(cur), hexline(cur, nil))
					} else {
						diff("SETTING", lastSettings, cur, nil)
					}
					lastSettings = cur
				}
				for _, pad := range pads {
					if b, err := dev.PadBlock(pad); err == nil {
						cur := b.Raw()
						if lastPad[pad] == nil {
							out("PAD %-4s %d bytes", pad, len(cur))
						} else {
							diff("PAD "+pad.String(), lastPad[pad], cur, nil)
						}
						lastPad[pad] = cur
					}
				}
			}
		}
	}
}

func frames(i int) bool { return i >= 8 && i < 12 }

func hexOrNone(v int) string {
	if v < 0 {
		return "--"
	}
	return fmt.Sprintf("%02x", v)
}

func hexline(b []byte, skip func(int) bool) string {
	parts := make([]string, len(b))
	for i, c := range b {
		parts[i] = fmt.Sprintf("%02x", c)
		if skip != nil && skip(i) {
			parts[i] = ".."
		}
	}
	return strings.Join(parts, " ")
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
