package core

import (
	"errors"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/record"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Ctrl-T is a recording of its own: the keys and the screen, no MIDI; without audio a silent WAV goes beside the log.
func TestTheTerminalCastIsARecordingOfItsOwn(t *testing.T) {
	a, _ := newApp(t)
	dir := filepath.Join(t.TempDir(), "casts")
	a.set.RecordDir, a.set.CastNoAudio = dir, true
	a.ToggleCast()
	if a.castRun == nil || !strings.HasPrefix(a.recText(), "CAST ") {
		t.Fatalf("the cast should run and show CAST: %q", a.recText())
	}
	a.ToggleRecording()
	if a.rec != nil {
		t.Fatal("Ctrl-R does not start a recording while the cast runs")
	}
	a.CastKey("h")
	a.CastKey("ctrl+t")
	a.CastScreen("one\ntwo", 80)
	time.Sleep(20 * time.Millisecond)
	a.ToggleCast()
	if a.castRun != nil {
		t.Fatal("Ctrl-T again ends the cast")
	}
	logs, _ := filepath.Glob(filepath.Join(dir, "*.events.jsonl"))
	wavs, _ := filepath.Glob(filepath.Join(dir, "*.wav"))
	if len(logs) != 1 || len(wavs) != 1 {
		t.Fatalf("a log and a WAV: %v %v", logs, wavs)
	}
	b, _ := os.ReadFile(logs[0])
	s := string(b)
	if !strings.Contains(s, `"k":"key","key":"h"`) || !strings.Contains(s, `"k":"screen"`) {
		t.Errorf("the keys and the screen are logged:\n%s", s)
	}
	for _, not := range []string{"ctrl+t", `"k":"project"`, `"k":"pattern"`} {
		if strings.Contains(s, not) {
			t.Errorf("the cast has no %s:\n%s", not, s)
		}
	}
	if st, err := os.Stat(wavs[0]); err != nil || st.Size() < 44 {
		t.Errorf("the silent WAV: %v", err)
	}
}

type noInput struct{}

func (noInput) Start(dir, device string) (*record.Session, error) {
	return nil, errors.New(`no audio input with that name: "404"`)
}

// Offline (the unit is not there) the cast still runs, without the audio.
func TestTheTerminalCastRunsOffline(t *testing.T) {
	a, _ := newApp(t)
	dir := filepath.Join(t.TempDir(), "casts")
	a.set.RecordDir = dir
	a.plat.Recorder = noInput{}
	a.ToggleCast()
	if a.castRun == nil || a.rec != nil {
		t.Fatal("the cast runs, with no audio recording")
	}
	a.CastKey("h")
	loud := formats.Sample{Channels: 1, Samples: make([]int16, 4800)}
	for i := range loud.Samples {
		loud.Samples[i] = 10000
	}
	a.Audio.Play(loud) // the program plays a sample itself
	time.Sleep(60 * time.Millisecond)
	a.ToggleCast()
	wavs, _ := filepath.Glob(filepath.Join(dir, "*.wav"))
	if len(wavs) != 1 {
		t.Fatalf("a WAV: %v", wavs)
	}
	b, _ := os.ReadFile(wavs[0])
	heard := false
	for i := 44; i+2 < len(b); i += 3 {
		if b[i+1] != 0 || b[i+2] != 0 {
			heard = true
			break
		}
	}
	if !heard {
		t.Error("the sample the program played is in the WAV of the cast")
	}
	if _, ok := a.Audio.(*NullAudio); !ok {
		t.Errorf("the output is the plain one again: %T", a.Audio)
	}
}

func TestTheCastTapMixesLoopsAndPreviews(t *testing.T) {
	tone := func(v int16, frames int) []int16 {
		o := make([]int16, frames*2)
		for i := range o {
			o[i] = v
		}
		return o
	}
	tp := &castTap{ev: []tapEvent{
		{at: 0, kind: tapLoop, pcm: tone(100, 10)},
		{at: time.Second / 4800 * 25, kind: tapPreview, pcm: tone(7, 3)}, // frame 25 (of 48000: 1/48000 s each; the rate scales below)
		{at: 0, kind: tapStop},
	}}
	// events are in seconds: frame f is f/48000 s
	tp.ev[1].at = 25 * time.Second / 48000
	tp.ev[2].at = 40 * time.Second / 48000
	out := tp.mix(50)
	if out[0] != 100 || out[2*15] != 100 { // the loop goes round
		t.Errorf("the loop: %d %d", out[0], out[30])
	}
	if out[2*25] != 107 || out[2*27] != 107 || out[2*28] != 100 { // the preview over it, 3 frames
		t.Errorf("the preview: %d %d %d", out[50], out[54], out[56])
	}
	if out[2*45] != 0 { // stopped at frame 40
		t.Errorf("stop: %d", out[90])
	}
}
