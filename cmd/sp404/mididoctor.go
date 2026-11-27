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

// midiDoctor is `sp404 doctor midi --channel 11 [--port name]` (--ports lists them): a few notes on each channel to the unit's MIDI port, to hear
// whether a synth behind the unit (MIDI thru) gets them. Channels: 11, 11-14 or 11,13. With --listen it listens instead (midiListen).
func midiDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor midi", flag.ContinueOnError)
	chFlag := fs.String("channel", "", "the MIDI channels: 11, 11-14 or 11,13")
	port := fs.String("port", os.Getenv("SP404_MIDI_PORT"), "a part of the MIDI port's name (default: 404, as the app; with --listen: every input)")
	listen := fs.Bool("listen", false, "listen instead: print the notes that come in on the port (a synth on USB), as the app gets them")
	secs := fs.Int("seconds", 30, "with --listen: how long to listen")
	list := fs.Bool("ports", false, "list the MIDI inputs and outputs there are")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *list {
		return midiPorts()
	}
	if *listen {
		return midiListen(*port, *chFlag, time.Duration(*secs)*time.Second)
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

var noteNames = []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}

// midiListen is `sp404 doctor midi --listen [--channel 12] [--port name]`: the input ports there are, then every note (and the
// clock, Start and Stop, counted) that comes in on the port, for a while: whether the app would get what the synth plays.
func midiListen(part, chFlag string, d time.Duration) error {
	src := midiin.Default()
	ports, err := src.Ports()
	if err != nil {
		return err
	}
	fmt.Println("MIDI inputs:")
	for _, p := range ports {
		fmt.Println("  " + p)
	}
	want := map[int]bool{}
	if chFlag != "" {
		chans, err := parseChannels(chFlag)
		if err != nil {
			return err
		}
		for _, c := range chans {
			want[c] = true
		}
	}
	events := make(chan midiin.Event, 512)
	on := func(e midiin.Event) {
		select {
		case events <- e:
		default:
		}
	}
	var stop func()
	var name string
	if part == "" { // every input: a hub has several ports, the channel says whose a note is
		var names []string
		stop, names, err = midiin.OpenAll(src, on)
		name = strings.Join(names, ", ")
	} else {
		stop, name, err = src.Open(part, on)
	}
	if err != nil {
		return err
	}
	defer stop()
	fmt.Printf("listening to %s for %s: play on the synth (Ctrl-C stops)\n", name, d)
	var notes, others, clocks int
	end := time.After(d)
	for {
		select {
		case <-end:
			fmt.Printf("done: %d notes", notes)
			if len(want) > 0 {
				fmt.Printf(" on the channels asked for, %d on others", others)
			}
			fmt.Printf(", %d clock pulses\n", clocks)
			if notes == 0 {
				fmt.Println("no notes: on an MC-101 the track needs Tx USB and Tx Note on (SHIFT + TRACK SEL), and the channel is the track's (MENU, SYSTEM, MIDI)")
			}
			return nil
		case e := <-events:
			switch e.Kind {
			case midiin.Clock:
				clocks++
			case midiin.Start, midiin.Stop, midiin.Continue:
				fmt.Println(e.Kind)
			case midiin.NoteOn, midiin.NoteOff:
				if len(want) > 0 && !want[e.Channel] {
					if e.Kind == midiin.NoteOn && e.Data2 > 0 {
						others++
					}
					continue
				}
				if e.Kind == midiin.NoteOn && e.Data2 > 0 {
					notes++
					fmt.Printf("ch %2d  on   %-3s (%3d)  vel %d\n", e.Channel, fmt.Sprintf("%s%d", noteNames[e.Data1%12], e.Data1/12-1), e.Data1, e.Data2)
				} else {
					fmt.Printf("ch %2d  off  %-3s (%3d)\n", e.Channel, fmt.Sprintf("%s%d", noteNames[e.Data1%12], e.Data1/12-1), e.Data1)
				}
			default:
				if len(want) == 0 || want[e.Channel] {
					fmt.Println(e)
				}
			}
		}
	}
}

// midiPorts is `sp404 doctor midi --ports`: the MIDI inputs and outputs the system has.
func midiPorts() error {
	src := midiin.Default()
	ins, err := src.Ports()
	if err != nil {
		return err
	}
	show := func(what string, names []string) {
		fmt.Printf("MIDI %s (%d):\n", what, len(names))
		for _, n := range names {
			fmt.Println("  " + n)
		}
		if len(names) == 0 {
			fmt.Println("  none: is the device plugged in (a data cable, not a hub that only charges) and on?")
		}
	}
	show("inputs", ins)
	if l, ok := src.(midiin.OutLister); ok {
		outs, err := l.OutPorts()
		if err != nil {
			return err
		}
		show("outputs", outs)
	}
	fmt.Println("--port takes a part of a name: 404 for the SP-404MKII on its own USB, h2midi for a CME hub")
	return nil
}
