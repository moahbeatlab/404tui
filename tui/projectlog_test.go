package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moahbeatlab/404tui/perflog"
)

func TestTheEventLogOfAWebRecordingHoldsThePadsAndPatterns(t *testing.T) {
	s := open(t)
	m := s.m()
	path := filepath.Join(t.TempDir(), "take.events.jsonl")
	l, err := perflog.Create(path, "take.wav", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	cmd := m.projectLogCmd(l, time.Now())
	if cmd == nil {
		t.Fatal("no command: the model has no unit state yet")
	}
	if done, ok := cmd().(projectSnapDone); !ok || done.err != nil {
		t.Fatalf("%v", done)
	}
	l.Close()
	b, _ := os.ReadFile(path)
	text := string(b)
	for _, want := range []string{`"k":"project"`, `"source":"unit"`, `"pads":[`, `"patterns":[`, `"Gate":`, `"bank_tempos":[`} {
		if !strings.Contains(text, want) {
			t.Errorf("the project line lacks %s: %.300s", want, text)
		}
	}
}

func TestAPatternSentFromThePageIsLoggedAndTakenOverAtTheLoopStart(t *testing.T) {
	r := newPerfRig(t, 100, 40)
	path := filepath.Join(t.TempDir(), "take.events.jsonl")
	l, _ := perflog.Create(path, "take.wav", time.Now())
	m := r.s.m()
	m.perfLog = l
	slot := pad(t, "B3")
	r.s.model = m.patternRequested(slot)
	if mm := r.s.m(); mm.perfNext == nil || *mm.perfNext != slot || mm.perfNow != nil {
		t.Fatalf("after the request: now %v next %v", mm.perfNow, mm.perfNext)
	}
	r.feed(midiinMarker())
	mm := r.s.m()
	if mm.perfNow == nil || *mm.perfNow != slot || mm.perfNext != nil {
		t.Fatalf("after the loop start: now %v next %v", mm.perfNow, mm.perfNext)
	}
	l.Close()
	b, _ := os.ReadFile(path)
	if n := strings.Count(string(b), `"k":"pattern"`); n != 2 || !strings.Contains(string(b), `"sent":true`) {
		t.Errorf("the log: %s", b)
	}
}
