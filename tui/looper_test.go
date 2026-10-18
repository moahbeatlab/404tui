package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/midiin"
)

func TestTheLooperSaysWhichPatternItMade(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 200, Height: 40})
	s.press("8", "p")
	if msg := s.m().message; !strings.Contains(msg, "made (bank A, pad ") || !strings.Contains(msg, "all 16 pads") {
		t.Fatalf("message %q", msg)
	}
	if name := s.m().message; !strings.Contains(name, "pattern A") {
		t.Errorf("no pattern name in %q", name)
	}
}

func TestTheLooperChangesBankAndLengthAndDeletesOnBeat1WhileTheClockRuns(t *testing.T) {
	r := newPerfRig(t, 200, 40)
	r.s.press("8")
	r.s.shows("Looper", "BANK A")
	r.s.press("]")
	r.s.shows("BANK B")
	r.s.press("[")
	r.s.press("b")
	if got := r.s.m().loopBars(); got != 4 {
		t.Fatalf("the loop is %d bars after b, want 4", got)
	}
	// The cursor is on A1, which holds a sample: d asks, d again deletes. With no clock it is done at once.
	r.s.press("d")
	if !strings.Contains(r.s.m().message, "d again") {
		t.Fatalf("no question: %q", r.s.m().message)
	}
	r.s.press("d")
	if r.s.m().snap.Bank(0)[0].HasSample {
		t.Fatalf("A1 still holds its sample (%q)", r.s.m().message)
	}
	// With a running clock the next delete waits for beat 1 of the pattern.
	r.feed(midiin.Event{Kind: midiin.Start})
	r.tick(5)
	r.s.press("l", "d", "d")
	if !strings.Contains(r.s.m().message, "beat 1") || !r.s.m().snap.Bank(0)[1].HasSample {
		t.Fatalf("not queued for beat 1: %q", r.s.m().message)
	}
	m := r.s.m()
	m.clock.ticks = m.loopBars()*ticksPerBar - 1
	next, cmd := m.midiEvent(midiMsg{link: r.link, ev: midiin.Event{Kind: midiin.Clock, At: time.Now()}})
	r.s.model = next
	r.s.run(cmd)
	if r.s.m().snap.Bank(0)[1].HasSample {
		t.Errorf("A2 still holds its sample after beat 1 (%q)", r.s.m().message)
	}
}

func TestARemixKeepsTheLengthAndMovesTheSlices(t *testing.T) {
	src := formats.Sample{Channels: 1}
	for i := 0; i < 8000; i++ {
		src.Samples = append(src.Samples, int16(i/1000*1000+100))
	}
	out := remixSample(src, 8)
	if out.Frames() != src.Frames() {
		t.Fatalf("a remix is %d frames, the loop %d", out.Frames(), src.Frames())
	}
	same := true
	for i := range src.Samples {
		if out.Samples[i] != src.Samples[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("the slices are in their old order")
	}
}

func TestTheLooperRemixesAPadOntoTheNextFreePad(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 200, Height: 40})
	s.press("8")
	m := s.m()
	from, ok := m.loopFreePad()
	if !ok {
		t.Skip("the bank is full")
	}
	src := formats.Sample{Channels: 1}
	for i := 0; i < 48000; i++ {
		src.Samples = append(src.Samples, int16(i/6000*2000))
	}
	if _, err := app.PutSample(m.unit, from, src, "SRC"); err != nil {
		t.Fatal(err)
	}
	s.run(m.load(""))
	m = s.m()
	m.looper().cursor = int(from.Pad())
	s.model = m
	s.press("s")
	if got := s.m().loopSlices(); got != 16 {
		t.Fatalf("s: %d slices, want 16 after 8", got)
	}
	s.press("x")
	m = s.m()
	if !strings.Contains(m.message, "in 16 slices") {
		t.Fatalf("message %q", m.message)
	}
	found := false
	for _, r := range m.snap.Bank(m.loopBank()) {
		if strings.HasPrefix(r.Name, "SLC ") {
			found = true
		}
	}
	if !found {
		t.Errorf("no remix on the bank (%q)", m.message)
	}
}
