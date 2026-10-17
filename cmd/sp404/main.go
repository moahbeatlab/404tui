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
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/device"
	"github.com/moahbeatlab/404tui/doctor"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/midiin"
	"github.com/moahbeatlab/404tui/playback"
	"github.com/moahbeatlab/404tui/record"
	"github.com/moahbeatlab/404tui/serialport"
	"github.com/moahbeatlab/404tui/sim"
	"github.com/moahbeatlab/404tui/tui"
	"github.com/moahbeatlab/404tui/version"
	"github.com/moahbeatlab/404tui/web"
)

const usage = `usage: sp404 [flags] <command>

commands:
  menu           full-screen terminal UI (the default when no command is given)
  serve          run without a screen and serve Perform to a browser on this network (phone, tablet)
  midimap        print the map of the Faderfox EC4 for 404tui (one group of 16 pushes)
  check          check this installation: the computer, the program on a simulated unit, a plugged-in unit
  doctor         read-only checks of this program against the unit; run it first
                 (add -preview as an argument to also play a pad on the unit)
  ports          list every serial port, and why each is or is not taken for the unit
  status         current project, selected pad and project names
  pads           pads of the current project that hold a sample
  ls [path]      list a folder (SD:… for the card, otherwise the device's own storage)
  beep           play a test tone on the computer's audio output
  play <file>    play an audio file on the computer's audio output

flags:
`

// options are what the flags say.
type options struct {
	port     string
	simulate bool
	verbose  bool
	trace    bool
	logPath  string
	// audio is the playback method: oto (the default) or afplay.
	audio string
	// offline is a folder of projects on this computer to work on without the unit.
	offline string
	// webAddr is where to serve the web page of Perform ("" for not at all; serve uses :8404), webKey
	// the key a browser has to give (kept for the user when empty).
	webAddr, webKey string
}

func main() {
	var o options
	flag.StringVar(&o.port, "port", "", "serial port (default: the first SP-404MKII found)")
	flag.StringVar(&o.offline, "offline", "", "work offline on a folder of projects on this computer (exported projects, each a folder with PADCONF.BIN)")
	flag.BoolVar(&o.simulate, "sim", false, "talk to a simulated device instead of the hardware")
	flag.BoolVar(&o.verbose, "v", false, "write a debug log: every transaction with its timing, port discovery, connects and UI actions")
	flag.BoolVar(&o.trace, "trace", false, "also log every message on the wire, as hex (implies -v)")
	flag.StringVar(&o.logPath, "log", "", "debug log file (default: the user cache folder for the UI, standard error otherwise)")
	flag.StringVar(&o.audio, "audio", "", "how files are played on this computer: oto (default) or afplay (macOS's own player)")
	flag.StringVar(&o.webAddr, "web", "", "serve the Perform web page on this address, e.g. :8404 (the serve command does it by itself)")
	flag.StringVar(&o.webKey, "web-key", "", "key a browser must give to connect (default: one kept for you; \"none\" lets everyone on the network in)")
	showVersion := flag.Bool("version", false, "print the version and stop")
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println("404tui " + version.Version)
		return
	}
	args := flag.Args()
	if len(args) == 0 {
		args = []string{"menu"}
	}
	if err := run(args, o); err != nil {
		fmt.Fprintln(os.Stderr, "sp404:", err)
		holdWindow()
		os.Exit(1)
	}
}

// logging is the debug log: where it goes and a way to close it.
type logging struct {
	*slog.Logger
	path  string
	close func()
}

// newLogging sets up the debug log. Without -v or -trace there is none. The terminal UI owns
// the screen, so its log goes to a file; other commands log to standard error.
func newLogging(o options, forUI bool) (logging, error) {
	if !o.verbose && !o.trace {
		return logging{Logger: slog.New(slog.DiscardHandler), close: func() {}}, nil
	}
	level := slog.LevelDebug
	if o.trace {
		level = device.LevelTrace
	}
	path := o.logPath
	if path == "" && forUI {
		if dir, err := os.UserCacheDir(); err == nil {
			path = filepath.Join(dir, "404tui", "debug.log")
		} else {
			path = "404tui-debug.log"
		}
	}
	out, closer := io.Writer(os.Stderr), func() {}
	if path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return logging{}, err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return logging{}, err
		}
		out, closer = f, func() { f.Close() }
	}
	handler := slog.NewTextHandler(out, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.LevelKey && a.Value.Any() == device.LevelTrace {
				a.Value = slog.StringValue("TRACE")
			}
			return a
		},
	})
	l := logging{Logger: slog.New(handler), path: path, close: closer}
	serialport.SetLogger(l.Logger)
	l.banner(o)
	return l, nil
}

// banner writes what is needed to read the rest of a log: the program, the machine, the flags.
func (l logging) banner(o options) {
	rev, modified := "unknown", ""
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				rev = s.Value
			case "vcs.modified":
				if s.Value == "true" {
					modified = " (modified)"
				}
			}
		}
	}
	state := "none"
	if dir, err := os.UserConfigDir(); err == nil {
		state = filepath.Join(dir, "404tui", "state.json")
	}
	l.Info("404tui debug log", "version", version.Version, "revision", rev+modified, "go", runtime.Version(), "os", runtime.GOOS, "arch", runtime.GOARCH,
		"args", os.Args[1:], "port", o.port, "sim", o.simulate, "trace", o.trace, "state file", state)
}

func run(args []string, o options) error {
	forUI := args[0] == "menu"
	l, err := newLogging(o, forUI)
	if err != nil {
		return err
	}
	defer l.close()
	// A crash leaves its stack in the log as well as on the screen.
	defer func() {
		if r := recover(); r != nil {
			l.Error("panic", "value", r, "stack", string(debug.Stack()))
			panic(r)
		}
	}()

	switch args[0] {
	case "ports":
		return listPorts()
	case "midimap":
		fmt.Print(tui.EC4Sheet())
		return nil
	case "check":
		return selfCheck(o, os.Stdout, l.Logger)
	case "beep":
		return audioTest("", o.audio)
	case "play":
		if len(args) < 2 {
			return fmt.Errorf("usage: sp404 play <audio file>")
		}
		return audioTest(args[1], o.audio)
	case "menu":
		return runUI(o, l, false)
	case "serve":
		return runUI(o, l, true)
	}

	dev, err := open(o, l.Logger)
	if err != nil {
		l.Error("could not open the unit", "err", err)
		return err
	}
	defer dev.Close()
	l.Info("unit opened", "port", dev.PortName())
	switch args[0] {
	case "doctor":
		results := doctor.Run(dev, os.Stdout, l.Logger, doctor.Options{Preview: slices.Contains(args[1:], "-preview")})
		if n := doctor.Failures(results); n > 0 {
			return fmt.Errorf("%d check(s) failed", n)
		}
		return nil
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

// runUI starts the terminal UI, which waits for a unit by itself.
func runUI(o options, l logging, headless bool) error {
	// Without an audio output the files just cannot be auditioned, and it says why.
	var player app.Player
	p, audioErr := playback.OpenBackend(o.audio)
	if audioErr == nil {
		player = p
		l.Info("audio output opened", "backend", backendName(o.audio))
	} else {
		l.Warn("no audio output", "err", audioErr)
	}
	model := tui.NewConnecting(connector(o, l.Logger), player, tui.DefaultStore()).WithAudio(player, audioErr).WithRecorder(record.Default()).WithMIDI(midiin.Default()).
		WithOffline(offlineOpener(l.Logger), unitPresent(o), o.offline)
	if l.path != "" {
		model = model.WithLogger(l.Logger, l.path)
		fmt.Fprintln(os.Stderr, "debug log:", l.path)
	}
	// The web side: the state of Perform for the browsers, and their commands.
	var srv *web.Server
	if o.webAddr != "" || headless {
		if o.webAddr == "" {
			o.webAddr = ":8404"
		}
		srv = &web.Server{Key: webKey(o.webKey)}
		listener, err := srv.Listen(o.webAddr)
		if err != nil {
			return fmt.Errorf("web page: %w", err)
		}
		defer listener.Close()
		urls := srv.URLs(listener)
		model = model.WithWeb(srv).WithWebURL(urls[0])
		fmt.Fprintln(os.Stderr, "web page for Perform: open one of these on a phone on this network:")
		for _, u := range urls {
			fmt.Fprintln(os.Stderr, "  "+u)
		}
		l.Info("web page", "addr", listener.Addr().String(), "urls", urls)
	}
	opts := []tea.ProgramOption{tea.WithAltScreen(), tea.WithMouseCellMotion()}
	if headless {
		// No screen and no keyboard: the program runs for the browsers.
		opts = []tea.ProgramOption{tea.WithoutRenderer(), tea.WithInput(nil)}
	}
	program := tea.NewProgram(model, opts...)
	if srv != nil {
		srv.OnCommand = func(msg []byte) { program.Send(tui.WebCommand(append([]byte(nil), msg...))) }
	}
	// Finding the output's name takes a moment, so it comes later: the UI then says where the
	// sound of a file goes.
	if player != nil {
		go func() {
			if name := playback.OutputName(); name != "" {
				program.Send(tui.AudioOutputMsg(name))
			}
		}()
	}
	_, err := program.Run()
	return err
}

// listPorts shows every serial port the system reports and why each is or is not the unit.
func listPorts() error {
	ports, err := serialport.All()
	if err != nil {
		return err
	}
	if len(ports) == 0 {
		fmt.Println("the system lists no serial ports at all")
		return nil
	}
	for _, p := range ports {
		verdict := "SP-404MKII"
		if !p.Matches {
			verdict = "no: " + p.Reason
		}
		id := "-"
		if p.USB {
			id = p.VID + ":" + p.PID
		}
		fmt.Printf("%-34s %-10s %-22s %s\n", p.Name, id, p.Product, verdict)
	}
	return nil
}

func options2(l *slog.Logger) []device.Option {
	return []device.Option{device.WithLogger(l)}
}

// connector opens a link to the unit each time the UI asks.
func connector(o options, l *slog.Logger) tui.Connect {
	attempt := 0
	return func() (app.Unit, io.Closer, error) {
		attempt++
		if !o.simulate && o.port == "" {
			ports, err := serialport.Find()
			if err != nil {
				l.Warn("looking for the unit failed", "attempt", attempt, "err", err)
				return nil, nil, err
			}
			if len(ports) == 0 {
				// Quiet after the first few: it polls about once a second.
				if attempt <= 3 || attempt%30 == 0 {
					l.Debug("no unit found yet", "attempt", attempt)
				}
				return nil, nil, tui.ErrNoUnit
			}
		}
		dev, err := open(o, l)
		if err != nil {
			l.Warn("could not open the unit", "attempt", attempt, "err", err)
			if errors.Is(err, serialport.ErrNotFound) {
				return nil, nil, tui.ErrNoUnit
			}
			return nil, nil, err
		}
		l.Info("unit opened", "port", dev.PortName(), "attempt", attempt)
		return dev, dev, nil
	}
}

// offlineOpener opens a folder of projects as a simulated unit, to work without the real one.
func offlineOpener(l *slog.Logger) tui.OpenOffline {
	return func(dir string) (app.Unit, io.Closer, string, error) {
		s, f, err := sim.OpenFolder(dir)
		if err != nil {
			return nil, nil, "", err
		}
		l.Info("working offline", "folder", f.Dir, "projects", len(f.Projects), "template", f.Template)
		dev := device.New(s, "offline", options2(l)...)
		note := fmt.Sprintf("%d projects found", len(f.Projects))
		if len(f.Projects) == 1 {
			note = "1 project found"
		}
		if !f.Template {
			note += "; no project from the unit here, so new projects get default settings (export one from the unit to copy its settings)"
		}
		return dev, dev, note, nil
	}
}

// unitPresent says whether a unit is plugged in, without opening it; nil for the simulation.
func unitPresent(o options) func() bool {
	if o.simulate {
		return nil
	}
	return func() bool {
		ports, err := serialport.Find()
		return err == nil && len(ports) > 0
	}
}

func open(o options, l *slog.Logger) (*device.Device, error) {
	opts := options2(l)
	if o.simulate {
		return device.New(sim.Demo(), "sim", opts...), nil
	}
	if o.port == "" {
		p, name, err := serialport.OpenFirst()
		if err != nil {
			return nil, err
		}
		return device.New(p, name, opts...), nil
	}
	p, err := serialport.Open(o.port)
	if err != nil {
		return nil, err
	}
	return device.New(p, o.port, opts...), nil
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

func backendName(name string) string {
	if name == "" {
		return playback.BackendDefault
	}
	return name
}

// audioTest plays a tone, or a file, with each playback method in turn (or the one asked
// for), saying which is playing, so that silence in the UI can be told apart from a problem
// with the computer's audio output.
func audioTest(file, only string) error {
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
	}
	if name := playback.OutputName(); name != "" {
		fmt.Println("the system's default output is:", name)
	}
	backends := []string{playback.BackendOto, playback.BackendAfplay}
	if only != "" {
		backends = []string{only}
	}
	heard := 0
	for _, name := range backends {
		p, err := playback.OpenBackend(name)
		if err != nil {
			fmt.Printf("[%s] not available: %v\n", name, err)
			continue
		}
		heard++
		fmt.Printf("[%s] playing", name)
		if file == "" {
			fmt.Print(" a 1 second 440 Hz tone")
		}
		fmt.Println(" …")
		if err := p.Play(sample); err != nil {
			return fmt.Errorf("[%s] %w", name, err)
		}
		for p.Playing() {
			time.Sleep(50 * time.Millisecond)
		}
		time.Sleep(400 * time.Millisecond)
	}
	if heard == 0 {
		return fmt.Errorf("no audio output could be opened")
	}
	if len(backends) > 1 {
		fmt.Println("\nWhich did you hear: both, only the first (oto), only the second (afplay), or neither?")
		fmt.Println("only afplay -> run the UI with: sp404 -audio afplay")
		fmt.Println("neither     -> the system output is the problem (see the device named above)")
	}
	return nil
}

// webKey is the key for the browsers: the one asked for, "none" for no key, else the one kept for the user.
func webKey(asked string) string {
	switch asked {
	case "":
		return web.LoadKey()
	case "none":
		return ""
	}
	return asked
}
