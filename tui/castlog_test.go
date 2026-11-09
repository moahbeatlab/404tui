package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moahbeatlab/404tui/perflog"
)

func TestTheCastLogsKeysAndTheScreenThatChanged(t *testing.T) {
	r := newPerfRig(t, 100, 40)
	path := filepath.Join(t.TempDir(), "take.events.jsonl")
	l, _ := perflog.Create(path, "take.wav", time.Now())
	m := r.s.m()
	m.perfLog = l
	r.s.model = m
	r.s.model.View() // the program draws after each message
	r.s.press("2")   // a key that changes the screen
	r.s.model.View()
	r.s.press("7")
	r.s.model.View()
	l.Close()
	b, _ := os.ReadFile(path)
	text := string(b)
	if n := strings.Count(text, `"k":"key"`); n != 2 || !strings.Contains(text, `"key":"2"`) {
		t.Errorf("keys: %d in %.400s", n, text)
	}
	if !strings.Contains(text, `"k":"screen","w":100,"h":`) {
		t.Errorf("no whole screen: %.300s", text)
	}
	if !strings.Contains(text, `"k":"screen","d":{`) {
		t.Errorf("no change of the screen: %.300s", text)
	}
}
