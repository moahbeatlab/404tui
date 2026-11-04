package tui

import (
	"bytes"
	"testing"

	"github.com/moahbeatlab/404tui/midiin"
)

func TestAPatternIsSentWhenTheLoopStarts(t *testing.T) {
	r := newPerfRig(t, 100, 40)
	slot := pad(t, "C5")
	r.s.model = r.s.m().patternCue(slot, false)
	if len(r.sent()) != 0 || r.sc().pcNext == nil {
		t.Fatalf("it should wait for the loop: sent %v, waiting %v", r.sent(), r.sc().pcNext)
	}
	r.tick(5)
	if len(r.sent()) != 0 {
		t.Fatalf("sent before the loop started: %v", r.sent())
	}
	r.feed(midiin.Event{Kind: midiin.NoteOn, Channel: 12, Data1: 48, Data2: 127}) // another channel: not the marker
	if len(r.sent()) != 0 {
		t.Fatal("a note on another channel is no loop start")
	}
	r.feed(midiin.Event{Kind: midiin.NoteOn, Channel: 13, Data1: 48, Data2: 127})
	got := r.sent()
	if len(got) != 1 || !bytes.Equal(got[0], []byte{0xC2, 4}) {
		t.Fatalf("sent %v, want program change 4 on channel 3 (C5)", got)
	}
	m := r.s.m()
	if r.sc().pcNext != nil || m.perfNow == nil || *m.perfNow != slot || m.perfNext != nil {
		t.Errorf("after: waiting %v, now %v, next %v", r.sc().pcNext, m.perfNow, m.perfNext)
	}
	r.feed(midiin.Event{Kind: midiin.NoteOn, Channel: 13, Data1: 48, Data2: 127})
	if len(r.sent()) != 1 {
		t.Errorf("sent again at the next loop: %v", r.sent())
	}
	// now: at once
	r.s.model = r.s.m().patternCue(pad(t, "A2"), true)
	got = r.sent()
	if len(got) != 2 || !bytes.Equal(got[1], []byte{0xC0, 1}) {
		t.Fatalf("now: sent %v", got)
	}
}
