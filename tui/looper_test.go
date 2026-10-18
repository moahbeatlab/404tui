package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/moahbeatlab/404tui/midiin"
)

func TestTheLooperRecordsAFullLoopFromBarOneAndPutsItOnTheNextFreePad(t *testing.T) {
	loopSettle = 0
	t.Cleanup(func() { loopSettle = 600 * time.Millisecond })
	r := newPerfRig(t, 200, 40)
	stopped := 0
	r.s.model = r.s.m().WithRecorder(&fakeRecorder{dir: "", stopped: &stopped})
	r.s.press("8")
	r.s.shows("Looper", "BANK A")
	r.s.press("b") // 2 -> 4
	r.s.press("b") // 4 -> 8
	r.s.press("b") // 8 -> 1
	if got := r.s.m().loopBars(); got != 1 {
		t.Fatalf("the loop is %d bars, want 1", got)
	}
	// Without a clock there is nothing to follow.
	r.s.press("a")
	if r.s.m().lp.phase != lpIdle || !strings.Contains(r.s.m().message, "clock") {
		t.Fatalf("armed without a clock: phase %d, %q", r.s.m().lp.phase, r.s.m().message)
	}
	r.feed(midiin.Event{Kind: midiin.Start})
	r.tick(1)
	r.s.press("a")
	if m := r.s.m(); m.lp.phase != lpArmed || m.lp.target.String() != "A4" {
		t.Fatalf("phase %d target %s, want armed on A4 (%q)", m.lp.phase, m.lp.target, m.message)
	}
	if !strings.Contains(r.s.m().looperBadge(), "armed") {
		t.Errorf("no badge while armed: %q", r.s.m().looperBadge())
	}
	// The recorder needs a moment: bar 1 comes after it.
	r.s.m().lp.session.Started = time.Now().Add(-5 * time.Second)
	at := time.Now().Add(-2500 * time.Millisecond)
	tickAt := func(d time.Duration, run bool) {
		e := midiin.Event{Kind: midiin.Clock, At: at.Add(d)}
		next, cmd := r.s.m().midiEvent(midiMsg{link: r.link, ev: e})
		r.s.model = next
		if run {
			r.s.run(cmd)
		}
	}
	for r.s.m().clock.ticks < ticksPerBar-1 {
		tickAt(-time.Hour, false)
	}
	tickAt(0, false) // bar 1
	if r.s.m().lp.phase != lpRecording {
		t.Fatalf("not recording on bar 1: phase %d, ticks %d", r.s.m().lp.phase, r.s.m().clock.ticks)
	}
	r.s.shows("REC")
	for r.s.m().clock.ticks < 2*ticksPerBar-1 {
		tickAt(time.Second, false)
	}
	tickAt(2*time.Second, true) // the end: the take is cut and saved
	if r.s.m().lp.phase != lpIdle {
		t.Fatalf("still busy: phase %d, %q", r.s.m().lp.phase, r.s.m().message)
	}
	m := r.s.m()
	if stopped != 1 {
		t.Errorf("the recorder was stopped %d times, want 1", stopped)
	}
	row := m.snap.Pads[3]
	if !row.HasSample || !strings.HasPrefix(row.Name, "LOOP ") {
		t.Fatalf("A4: %+v (%q)", row, m.message)
	}
	if row.Params["gate"] != 0 {
		t.Errorf("the loop is gated")
	}
	if got := float64(row.Size); got <= 0 {
		t.Errorf("empty sample")
	}
	// The next take would go on A5; deleting A4 frees it again.
	if pad, _ := m.loopFreePad(); pad.String() != "A5" {
		t.Errorf("next free pad %s, want A5", pad)
	}
	r.s.press("d") // the cursor is on the new loop: ask
	if !strings.Contains(r.s.m().message, "d again") {
		t.Fatalf("no question: %q", r.s.m().message)
	}
	r.s.press("d")
	if pad, _ := r.s.m().loopFreePad(); pad.String() != "A4" {
		t.Errorf("after the delete the free pad is %s, want A4 (%q)", pad, r.s.m().message)
	}
}

func TestTheLooperDropsATakeWhenTheClockStops(t *testing.T) {
	r := newPerfRig(t, 200, 40)
	stopped := 0
	r.s.model = r.s.m().WithRecorder(&fakeRecorder{stopped: &stopped})
	r.s.press("8")
	r.feed(midiin.Event{Kind: midiin.Start})
	r.tick(1)
	r.s.press("a")
	r.s.press("esc")
	if r.s.m().lp.phase != lpIdle || stopped != 1 {
		t.Errorf("Esc did not cancel the take: phase %d, stopped %d", r.s.m().lp.phase, stopped)
	}
}
