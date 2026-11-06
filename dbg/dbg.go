// Package dbg is the debug log of the full-screen program: off until it is turned on (the -v flag, or CONFIG), then a file that says what the
// program did and how long it took: keys, status lines, what is sent to the unit and what comes back, the transactions of the device with
// their timing, slow frames, crashes. It can be turned on and off while the program runs; every part of the program writes to Logger.
package dbg

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"

	"github.com/moahbeatlab/404tui/device"
	"github.com/moahbeatlab/404tui/version"
)

// Modes of the log.
const (
	Off   = 0
	Basic = 1 // what the program does, and the transactions with the unit with their timing
	Trace = 2 // and every message on the wire, as hex
)

type state struct {
	h    slog.Handler
	mode int
	path string
	f    *os.File
}

var (
	mu  sync.Mutex
	cur atomic.Pointer[state]
	// Logger is what the program writes to. It does nothing while the log is off.
	Logger = slog.New(gate{})
)

// gate is a handler that passes records on to the file handler of the moment, or drops them.
type gate struct{}

func (gate) Enabled(ctx context.Context, l slog.Level) bool {
	s := cur.Load()
	return s != nil && s.h.Enabled(ctx, l)
}
func (gate) Handle(ctx context.Context, r slog.Record) error {
	if s := cur.Load(); s != nil {
		return s.h.Handle(ctx, r)
	}
	return nil
}
func (g gate) WithAttrs(attrs []slog.Attr) slog.Handler { return &attrGate{attrs: attrs} }
func (g gate) WithGroup(name string) slog.Handler       { return g }

// attrGate is a gate that adds attributes (a logger made with With).
type attrGate struct{ attrs []slog.Attr }

func (a *attrGate) Enabled(ctx context.Context, l slog.Level) bool { return gate{}.Enabled(ctx, l) }
func (a *attrGate) Handle(ctx context.Context, r slog.Record) error {
	if s := cur.Load(); s != nil {
		r.AddAttrs(a.attrs...)
		return s.h.Handle(ctx, r)
	}
	return nil
}
func (a *attrGate) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &attrGate{attrs: append(append([]slog.Attr(nil), a.attrs...), attrs...)}
}
func (a *attrGate) WithGroup(string) slog.Handler { return a }

// DefaultPath is where the log goes unless a path is given: the user's cache folder.
func DefaultPath() string {
	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, "404tui", "debug.log")
	}
	return "404tui-debug.log"
}

// Set turns the log to a mode (Off, On, Trace) and returns the file it writes to ("" when off). path "" is DefaultPath. The file is added to;
// one that has grown past 20 MB is started again.
func Set(mode int, path string) (string, error) {
	mu.Lock()
	defer mu.Unlock()
	if old := cur.Swap(nil); old != nil && old.f != nil {
		old.f.Close()
	}
	if mode == Off {
		return "", nil
	}
	if path == "" {
		path = DefaultPath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	flags := os.O_CREATE | os.O_APPEND | os.O_WRONLY
	if st, err := os.Stat(path); err == nil && st.Size() > 20<<20 {
		flags = os.O_CREATE | os.O_TRUNC | os.O_WRONLY
	}
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return "", err
	}
	level := slog.LevelDebug
	if mode == Trace {
		level = device.LevelTrace
	}
	h := slog.NewTextHandler(io.Writer(f), &slog.HandlerOptions{Level: level, ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
		if a.Key == slog.LevelKey && a.Value.Any() == device.LevelTrace {
			a.Value = slog.StringValue("TRACE")
		}
		return a
	}})
	cur.Store(&state{h: h, mode: mode, path: path, f: f})
	banner()
	return path, nil
}

// Mode is the mode the log is in.
func Mode() int {
	if s := cur.Load(); s != nil {
		return s.mode
	}
	return Off
}

// Path is the file the log is written to, "" while it is off.
func Path() string {
	if s := cur.Load(); s != nil {
		return s.path
	}
	return ""
}

// Enabled says whether the log is on.
func Enabled() bool { return cur.Load() != nil }

// banner writes what is needed to read the rest of a log: the program, the machine.
func banner() {
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
	Logger.Info("404tui debug log", "version", version.Full(), "revision", rev+modified, "go", runtime.Version(), "os", runtime.GOOS, "arch", runtime.GOARCH, "args", os.Args[1:])
}
