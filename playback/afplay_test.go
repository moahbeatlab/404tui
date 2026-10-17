package playback

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/moahbeatlab/404tui/formats"
)

// fakeAfplay puts a stand-in for afplay first on the PATH: it records the file it is given
// (and that it was a WAV) and then sleeps like a playing sound.
func fakeAfplay(t *testing.T, sleep string) (record string) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in for afplay is a shell script")
	}
	dir := t.TempDir()
	record = filepath.Join(dir, "args")
	script := "#!/bin/sh\nhead -c 4 \"$1\" > '" + record + "'\nsleep " + sleep + "\n"
	if err := os.WriteFile(filepath.Join(dir, "afplay"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	return record
}

func TestAfplayPlaysAWavAndStops(t *testing.T) {
	record := fakeAfplay(t, "5")
	p, err := OpenBackend(BackendAfplay)
	if err != nil {
		t.Fatal(err)
	}
	if p.Playing() {
		t.Error("playing before anything was played")
	}
	if err := p.Play(formats.Sample{Channels: 1, Samples: make([]int16, 480)}); err != nil {
		t.Fatal(err)
	}
	if !p.Playing() {
		t.Error("not playing right after Play")
	}
	// It was handed a WAV file.
	var head []byte
	for i := 0; i < 250 && len(head) == 0; i++ {
		time.Sleep(20 * time.Millisecond)
		head, _ = os.ReadFile(record)
	}
	if string(head) != "RIFF" {
		t.Errorf("afplay got a file starting %q", head)
	}
	start := time.Now()
	p.Stop()
	if p.Playing() || time.Since(start) > 2*time.Second {
		t.Errorf("Stop left it playing or took %v", time.Since(start))
	}
}

func TestAfplayStopsByItselfAndTidiesUp(t *testing.T) {
	fakeAfplay(t, "0.1")
	a, err := OpenAfplay()
	if err != nil {
		t.Fatal(err)
	}
	a.Play(formats.Sample{Channels: 2, Samples: make([]int16, 960)})
	a.mu.Lock()
	file := a.file
	a.mu.Unlock()
	for i := 0; i < 100 && a.Playing(); i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if a.Playing() {
		t.Error("still playing after the sound ended")
	}
	time.Sleep(50 * time.Millisecond)
	if _, err := os.Stat(file); err == nil {
		t.Errorf("the temporary file %s was left behind", file)
	}
	// A new sound replaces the old one.
	a.Play(formats.Sample{Channels: 1, Samples: make([]int16, 10)})
	a.Play(formats.Sample{Channels: 1, Samples: make([]int16, 10)})
	a.Stop()
}

func TestUnknownBackendIsRefused(t *testing.T) {
	if _, err := OpenBackend("alsa"); err == nil {
		t.Error("an unknown backend was accepted")
	}
}
