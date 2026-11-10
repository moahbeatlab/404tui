package core

import (
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
