// Command sp404 talks to a Roland SP-404MKII over USB.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/device"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/playback"
	"github.com/moahbeatlab/404tui/proto"
	"github.com/moahbeatlab/404tui/serialport"
	"github.com/moahbeatlab/404tui/sim"
	"github.com/moahbeatlab/404tui/tui"
)

const usage = `usage: sp404 [flags] <command>

commands:
  beep         play a test tone on the computer's audio output
  play <file>  play an audio file on the computer's audio output
  menu         full-screen terminal UI (the default when no command is given)
  ports        list connected SP-404MKII serial ports
  status       current project, selected pad and project names
  pads         pads of the current project that hold a sample
  ls [path]    list a folder (SD:… for the card, otherwise the device's own storage)

flags:
`

func main() {
	port := flag.String("port", "", "serial port (default: the first SP-404MKII found)")
	simulate := flag.Bool("sim", false, "talk to a simulated device instead of the hardware")
	verbose := flag.Bool("v", false, "log the session's diagnostics")
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
		flag.PrintDefaults()
	}
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		args = []string{"menu"}
	}
	if err := run(args, *port, *simulate, *verbose); err != nil {
		fmt.Fprintln(os.Stderr, "sp404:", err)
		os.Exit(1)
	}
}

func run(args []string, port string, simulate, verbose bool) error {
	if args[0] == "ports" {
		ports, err := serialport.Find()
		if err != nil {
			return err
		}
		for _, p := range ports {
			fmt.Println(p.Name, p.SerialNumber, p.Product)
		}
		return nil
	}
	switch args[0] {
	case "beep":
		return audioTest("")
	case "play":
		if len(args) < 2 {
			return fmt.Errorf("usage: sp404 play <audio file>")
		}
		return audioTest(args[1])
	}
	if args[0] == "menu" {
		// The terminal UI starts without a unit: it waits for one, and says when it connects.
		// Without an audio output the files just cannot be auditioned, and it says why.
		var player app.Player
		p, audioErr := playback.Open()
		if audioErr == nil {
			player = p
		}
		model := tui.NewConnecting(connector(port, simulate, verbose), player, tui.DefaultStore()).WithAudio(player, audioErr)
		_, err := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
		return err
	}
	dev, err := open(port, simulate, verbose)
	if err != nil {
		return err
	}
	defer dev.Close()

	switch args[0] {
	case "status":
		return status(dev)
	case "pads":
		return pads(dev)
	case "ls":
		path := "ROLAND/SP-404MKII"
		if len(args) > 1 {
			path = args[1]
		}
		return list(dev, path)
	}
	return fmt.Errorf("unknown command %q", args[0])
}

func options(verbose bool) []device.Option {
	if !verbose {
		return nil
	}
	return []device.Option{device.WithLogger(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))}
}

// connector opens a link to the unit each time the UI asks.
func connector(port string, simulate, verbose bool) tui.Connect {
	return func() (app.Unit, io.Closer, error) {
		if !simulate && port == "" {
			if ports, err := serialport.Find(); err != nil {
				return nil, nil, err
			} else if len(ports) == 0 {
				return nil, nil, tui.ErrNoUnit
			}
		}
		dev, err := open(port, simulate, verbose)
		if err != nil {
			if errors.Is(err, serialport.ErrNotFound) {
				return nil, nil, tui.ErrNoUnit
			}
			return nil, nil, err
		}
		return dev, dev, nil
	}
}

func open(port string, simulate, verbose bool) (*device.Device, error) {
	opts := options(verbose)
	if simulate {
		return device.New(demo(), "sim", opts...), nil
	}
	if port == "" {
		p, name, err := serialport.OpenFirst()
		if err != nil {
			return nil, err
		}
		return device.New(p, name, opts...), nil
	}
	p, err := serialport.Open(port)
	if err != nil {
		return nil, err
	}
	return device.New(p, port, opts...), nil
}

// demo is a simulated device with a little content on it.
func demo() *sim.Sim {
	s := sim.New()
	for i, name := range []string{"kick", "snare", "hat"} {
		pad, _ := proto.NewPadIndex(uint16(i))
		s.SetSample(pad, name, 48_000)
	}
	s.AddFile("SD:IMPORT/pack/break.wav", make([]byte, 1024))
	return s
}

func status(dev *device.Device) error {
	st, err := dev.Status()
	if err != nil {
		return err
	}
	names, err := dev.ProjectNames()
	if err != nil {
		return err
	}
	fmt.Printf("port     %s\n", dev.PortName())
	fmt.Printf("project  %d (%s)\n", st.Project+1, st.Settings.Name())
	if st.HasSelected {
		fmt.Printf("selected %s\n", st.Selected)
	}
	fmt.Printf("tempo    %.2f BPM\n", float64(st.Settings.ProjectTempo())/100)
	for i, name := range names {
		mark := " "
		if uint8(i) == st.Project {
			mark = "*"
		}
		fmt.Printf("%s %2d %s\n", mark, i+1, name)
	}
	return nil
}

func pads(dev *device.Device) error {
	blocks, err := dev.PadBlocks()
	if err != nil {
		return err
	}
	for _, b := range blocks {
		if b.HasSample() {
			fmt.Printf("%-4s %-24s %8d bytes\n", b.Pad, b.Name(), b.FileSize())
		}
	}
	return nil
}

func list(dev *device.Device, path string) error {
	entries, err := dev.ListDir(path)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			fmt.Println(e.Name + "/")
		} else {
			fmt.Println(e.Name)
		}
	}
	return nil
}

// audioTest plays a tone, or a file, and says what it is doing, so that silence in the UI
// can be told apart from a problem with the computer's audio output.
func audioTest(file string) error {
	p, err := playback.Open()
	if err != nil {
		return fmt.Errorf("opening the audio output failed: %w", err)
	}
	fmt.Println("audio output opened")
	sample := formats.Sample{Channels: 1, Samples: make([]int16, 48000)}
	switch {
	case file != "":
		imported, err := formats.ReadAudio(file)
		if err != nil {
			return err
		}
		sample = imported.Sample
		fmt.Printf("%s: %.2f s, %d Hz source, %d channel(s)\n", file, float64(sample.Frames())/formats.SampleRate, imported.SourceRate, imported.SourceChannels)
	default:
		for i := range sample.Samples {
			sample.Samples[i] = int16(math.Sin(2*math.Pi*440*float64(i)/formats.SampleRate) * 12000)
		}
		fmt.Println("playing a 1 second 440 Hz tone; if you hear nothing, check which output the system uses")
	}
	if err := p.Play(sample); err != nil {
		return err
	}
	for p.Playing() {
		time.Sleep(50 * time.Millisecond)
	}
	fmt.Println("done")
	return nil
}
