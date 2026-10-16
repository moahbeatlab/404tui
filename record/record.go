// Package record records audio from the unit, which shows up on the computer as a USB audio
// interface, to a WAV file. It does not capture sound itself: it runs ffmpeg (or, where that is
// missing, sox), which both know how to read an input device on every system.
package record

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// DefaultDevice is a part of the name the unit's audio interface goes by.
const DefaultDevice = "404"

// Backend starts recordings.
type Backend interface {
	// Start begins recording the input whose name holds device into a new WAV file in dir.
	Start(dir, device string) (*Session, error)
}

// Session is a recording that is running.
type Session struct {
	// Path is the file being written, and Tool the program writing it.
	Path, Tool string
	Started    time.Time

	stop func() error
	done chan struct{}
	once sync.Once
	mu   sync.Mutex
	err  error
	tail bytes.Buffer
}

// Done is closed when the recording has ended, because it was stopped or because the program
// stopped by itself; Err says why, once it is.
func (s *Session) Done() <-chan struct{} { return s.done }

// Err is why the recording ended: nil when it was stopped on purpose, otherwise what the program
// said last.
func (s *Session) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Stop ends the recording and waits for the file to be finished.
func (s *Session) Stop() error {
	var err error
	s.once.Do(func() { err = s.stop() })
	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
		return errors.New("the recorder did not stop in time")
	}
	return err
}

// Size is how big the file is now.
func (s *Session) Size() int64 {
	if info, err := os.Stat(s.Path); err == nil {
		return info.Size()
	}
	return 0
}

// FileName is the name a recording started at t gets.
func FileName(t time.Time) string { return t.Format("2006-01-02_15-04-05") + ".wav" }

// Default is the recorder this computer can run: ffmpeg when it is installed, else sox, else none.
func Default() Backend { return execBackend{ffmpeg: lookPath("ffmpeg"), sox: lookPath("sox")} }

func lookPath(name string) string {
	p, _ := exec.LookPath(name)
	return p
}

type execBackend struct{ ffmpeg, sox string }

// ErrNoTool is what Start says when neither ffmpeg nor sox is installed.
var ErrNoTool = errors.New("no recorder: install ffmpeg (or sox) to record")

func (b execBackend) Start(dir, device string) (*Session, error) {
	if device == "" {
		device = DefaultDevice
	}
	if dir == "" {
		return nil, errors.New("no folder to record into: set one in Config")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, FileName(time.Now()))
	switch {
	case b.ffmpeg != "":
		args, err := ffmpegArgs(runtime.GOOS, b.ffmpeg, device, path)
		if err != nil {
			return nil, err
		}
		return run(b.ffmpeg, args, path, "ffmpeg", func(in io.Writer, p *os.Process) error {
			// ffmpeg ends a recording and finishes the file when it is told q.
			_, err := io.WriteString(in, "q\n")
			return err
		})
	case b.sox != "":
		args, err := soxArgs(runtime.GOOS, device, path)
		if err != nil {
			return nil, err
		}
		return run(b.sox, args, path, "sox", func(in io.Writer, p *os.Process) error {
			return interrupt(p)
		})
	}
	return nil, ErrNoTool
}

// run starts a program writing path, and watches it.
func run(bin string, args []string, path, tool string, stop func(io.Writer, *os.Process) error) (*Session, error) {
	cmd := exec.Command(bin, args...)
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	s := &Session{Path: path, Tool: tool, Started: time.Now(), done: make(chan struct{})}
	s.stop = func() error { return stop(in, cmd.Process) }
	go func() {
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			s.mu.Lock()
			s.tail.WriteString(sc.Text() + "\n")
			if s.tail.Len() > 2000 {
				s.tail.Next(s.tail.Len() - 2000)
			}
			s.mu.Unlock()
		}
	}()
	go func() {
		werr := cmd.Wait()
		s.mu.Lock()
		if werr != nil {
			last := strings.TrimSpace(lastLine(s.tail.String()))
			if last == "" {
				last = werr.Error()
			}
			s.err = errors.New(last)
		}
		s.mu.Unlock()
		close(s.done)
	}()
	// A program that is going to refuse the device does so at once.
	select {
	case <-s.done:
		return nil, fmt.Errorf("%s stopped: %v", tool, s.Err())
	case <-time.After(700 * time.Millisecond):
	}
	return s, nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

// ---- finding the unit's input ----

var (
	avfLine   = regexp.MustCompile(`\[(\d+)\] (.+)$`)
	dshowLine = regexp.MustCompile(`"([^"]+)" \(audio\)`)
)

// parseAVFoundation is the index of the first audio input of an ffmpeg avfoundation device list
// whose name holds want, or -1.
func parseAVFoundation(list, want string) int {
	audio := false
	for _, line := range strings.Split(list, "\n") {
		switch {
		case strings.Contains(line, "AVFoundation audio devices"):
			audio = true
		case strings.Contains(line, "AVFoundation video devices"):
			audio = false
		case audio:
			if m := avfLine.FindStringSubmatch(strings.TrimSpace(line)); m != nil && strings.Contains(strings.ToLower(m[2]), strings.ToLower(want)) {
				var n int
				fmt.Sscan(m[1], &n)
				return n
			}
		}
	}
	return -1
}

// parseDShow is the name of the first audio input of an ffmpeg dshow list whose name holds want.
func parseDShow(list, want string) string {
	for _, line := range strings.Split(list, "\n") {
		if m := dshowLine.FindStringSubmatch(line); m != nil && strings.Contains(strings.ToLower(m[1]), strings.ToLower(want)) {
			return m[1]
		}
	}
	return ""
}

// parsePulseSources is the name of the first source (not a monitor) of `pactl list short sources`
// whose name holds want.
func parsePulseSources(list, want string) string {
	for _, line := range strings.Split(list, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && !strings.HasSuffix(f[1], ".monitor") && strings.Contains(strings.ToLower(f[1]), strings.ToLower(want)) {
			return f[1]
		}
	}
	return ""
}

// listOutput runs a command and gives what it printed, both ways: ffmpeg's device lists come on
// stderr and end with an error exit.
func listOutput(bin string, args ...string) string {
	var buf bytes.Buffer
	cmd := exec.Command(bin, args...)
	cmd.Stdout, cmd.Stderr = &buf, &buf
	_ = cmd.Run()
	return buf.String()
}

// ffmpegArgs are the arguments that make ffmpeg record the input that holds device into path:
// 24-bit 48 kHz stereo WAV.
func ffmpegArgs(goos, bin, device, path string) ([]string, error) {
	in, err := ffmpegInput(goos, bin, device)
	if err != nil {
		return nil, err
	}
	out := append([]string{"-hide_banner", "-loglevel", "error", "-y"}, in...)
	return append(out, "-ac", "2", "-ar", "48000", "-c:a", "pcm_s24le", path), nil
}

// ffmpegInput are the arguments that make ffmpeg read the audio input whose name holds device.
func ffmpegInput(goos, bin, device string) ([]string, error) {
	switch goos {
	case "darwin":
		n := parseAVFoundation(listOutput(bin, "-hide_banner", "-f", "avfoundation", "-list_devices", "true", "-i", ""), device)
		if n < 0 {
			return nil, fmt.Errorf("no audio input with %q in its name: is the unit plugged in and on?", device)
		}
		return []string{"-f", "avfoundation", "-i", fmt.Sprintf(":%d", n)}, nil
	case "windows":
		name := parseDShow(listOutput(bin, "-hide_banner", "-list_devices", "true", "-f", "dshow", "-i", "dummy"), device)
		if name == "" {
			return nil, fmt.Errorf("no audio input with %q in its name: is the unit plugged in and on?", device)
		}
		return []string{"-f", "dshow", "-i", "audio=" + name}, nil
	}
	src := parsePulseSources(listOutput("pactl", "list", "short", "sources"), device)
	if src == "" {
		return nil, fmt.Errorf("no audio input with %q in its name: is the unit plugged in and on?", device)
	}
	return []string{"-f", "pulse", "-i", src}, nil
}

// soxArgs are the arguments that make sox record a device into path. sox names the device the
// way the system does, so device has to be (most of) its name.
func soxArgs(goos, device, path string) ([]string, error) {
	var driver string
	switch goos {
	case "darwin":
		driver = "coreaudio"
	case "windows":
		return nil, errors.New("recording with sox is not supported on Windows: install ffmpeg")
	default:
		driver = "pulseaudio"
	}
	return []string{"-q", "-t", driver, device, "-b", "24", "-r", "48000", "-c", "2", path}, nil
}

// NewSession is a recording that something else writes: stop is called to end it, and the session
// is done once it returns. It is for the programs that record by other means, and for tests.
func NewSession(path, tool string, stop func() error) *Session {
	s := &Session{Path: path, Tool: tool, Started: time.Now(), done: make(chan struct{})}
	s.stop = func() error {
		err := stop()
		close(s.done)
		return err
	}
	return s
}

// Tool names the program a backend would record with, "" when it has none.
func Tool(b Backend) string {
	if e, ok := b.(execBackend); ok {
		switch {
		case e.ffmpeg != "":
			return "ffmpeg"
		case e.sox != "":
			return "sox"
		}
	}
	return ""
}
