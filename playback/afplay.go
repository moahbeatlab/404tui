package playback

import (
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/moahbeatlab/404tui/formats"
)

// Output plays samples on the computer's audio output; Player (the oto one) and Afplay are
// the two ways to do it.
type Output interface {
	// Play stops whatever is playing and starts the sample from the beginning.
	Play(formats.Sample) error
	Stop()
	// Playing reports whether a sound is still sounding.
	Playing() bool
}

// Backends are the ways to play a sound.
const (
	BackendOto     = "oto"
	BackendAfplay  = "afplay"
	BackendDefault = BackendOto
)

// OpenBackend opens a playback method by name: "oto" (a library that talks to the audio
// system directly, the default) or "afplay" (macOS's own player, run once per sound).
func OpenBackend(name string) (Output, error) {
	switch name {
	case "", BackendOto:
		p, err := Open()
		if err != nil {
			return nil, err
		}
		return p, nil
	case BackendAfplay:
		return OpenAfplay()
	}
	return nil, fmt.Errorf("unknown audio backend %q (use %s or %s)", name, BackendOto, BackendAfplay)
}

// Afplay plays each sound by running macOS's afplay on a temporary WAV file. It plays on the
// system's default output like any other program, so it shows whether a silence comes from
// the output or from the other method.
type Afplay struct {
	path string

	mu   sync.Mutex
	cmd  *exec.Cmd
	file string
	done chan struct{}
}

// OpenAfplay finds afplay; it fails where there is none.
func OpenAfplay() (*Afplay, error) {
	path, err := exec.LookPath("afplay")
	if err != nil {
		return nil, fmt.Errorf("afplay is not available here: %w", err)
	}
	return &Afplay{path: path}, nil
}

func (a *Afplay) Play(s formats.Sample) error {
	a.Stop()
	f, err := os.CreateTemp("", "404tui-*.wav")
	if err != nil {
		return err
	}
	if _, err := f.Write(s.WAV()); err != nil {
		f.Close()
		os.Remove(f.Name())
		return err
	}
	f.Close()
	cmd := exec.Command(a.path, f.Name())
	if err := cmd.Start(); err != nil {
		os.Remove(f.Name())
		return fmt.Errorf("starting afplay: %w", err)
	}
	done := make(chan struct{})
	a.mu.Lock()
	a.cmd, a.file, a.done = cmd, f.Name(), done
	a.mu.Unlock()
	go func() {
		_ = cmd.Wait()
		os.Remove(f.Name())
		close(done)
	}()
	return nil
}

func (a *Afplay) Stop() {
	a.mu.Lock()
	cmd, done := a.cmd, a.done
	a.cmd, a.done = nil, nil
	a.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		<-done
	}
}

func (a *Afplay) Playing() bool {
	a.mu.Lock()
	done := a.done
	a.mu.Unlock()
	if done == nil {
		return false
	}
	select {
	case <-done:
		return false
	default:
		return true
	}
}
