package perflog

import (
	"path/filepath"
	"testing"
	"time"
)

func TestALogHoldsTheEventsWithTheirTimeFromTheStart(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "2026-10-17 20.01.wav")
	if got := PathFor(audio); got != filepath.Join(dir, "2026-10-17 20.01.events.jsonl") {
		t.Errorf("path %q", got)
	}
	start := time.Now()
	l, err := Create(PathFor(audio), audio, start)
	if err != nil {
		t.Fatal(err)
	}
	l.Add(start.Add(1500*time.Millisecond), "on", P("ch", 3), P("n", 52), P("v", 100), P("pad", "C5"))
	l.Add(start.Add(2*time.Second), "beat", P("n", 4), P("bpm", 120.04))
	l.Add(start.Add(3*time.Second), "text", P("text", "say \"hi\"\n"))
	if l.Count() != 4 {
		t.Errorf("count %d", l.Count())
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	l.Add(start, "late") // after Close: ignored
	es, err := Read(PathFor(audio))
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 4 || es[0]["k"] != "start" || es[0]["audio"] != "2026-10-17 20.01.wav" {
		t.Fatalf("entries %v", es)
	}
	if es[1]["t"] != 1.5 || es[1]["n"] != 52.0 || es[1]["pad"] != "C5" {
		t.Errorf("note %v", es[1])
	}
	if es[2]["bpm"] != 120.04 && es[2]["bpm"] != 120.0 {
		t.Errorf("beat %v", es[2])
	}
	if es[3]["text"] != "say \"hi\"\n" {
		t.Errorf("text %v", es[3])
	}
}

func TestANilLogDoesNothing(t *testing.T) {
	var l *Log
	l.Add(time.Now(), "x")
	if l.Count() != 0 || l.Close() != nil {
		t.Error("a nil log did something")
	}
}
