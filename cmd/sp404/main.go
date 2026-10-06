// Command sp404 talks to a Roland SP-404MKII over USB.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/device"
	"github.com/moahbeatlab/404tui/playback"
	"github.com/moahbeatlab/404tui/proto"
	"github.com/moahbeatlab/404tui/serialport"
	"github.com/moahbeatlab/404tui/sim"
	"github.com/moahbeatlab/404tui/tui"
)

const usage = `usage: sp404 [flags] <command>

commands:
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
	dev, err := open(port, simulate, verbose)
	if err != nil {
		return err
	}
	defer dev.Close()

	switch args[0] {
	case "menu":
		// Without an audio output the files just cannot be auditioned.
		var player app.Player
		if p, err := playback.Open(); err == nil {
			player = p
		}
		_, err := tea.NewProgram(tui.New(dev, player, tui.DefaultStore()), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
		return err
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

func open(port string, simulate, verbose bool) (*device.Device, error) {
	var opts []device.Option
	if verbose {
		opts = append(opts, device.WithLogger(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))))
	}
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
