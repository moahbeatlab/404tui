package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/midiin"
	"github.com/moahbeatlab/404tui/record"
)

func TestTheLooperRecordsAFullLoopFromTheNextBeatAndPutsItOnTheNextFreePad(t *testing.T) {
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
	r.s.press("t") // start on the next beat
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
	for r.s.m().clock.ticks < 23 {
		tickAt(-time.Hour, false)
	}
	tickAt(0, false) // the next beat
	if r.s.m().lp.phase != lpRecording {
		t.Fatalf("not recording on the next beat: phase %d, ticks %d", r.s.m().lp.phase, r.s.m().clock.ticks)
	}
	r.s.shows("REC")
	for r.s.m().clock.ticks < 24+ticksPerBar-1 {
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
	if !strings.Contains(m.message, "peak") {
		t.Errorf("the message does not say how loud the loop is: %q", m.message)
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

func TestTheLooperCanStartOnTheNextBarAndCalibratesTheDelay(t *testing.T) {
	loopSettle = 0
	t.Cleanup(func() { loopSettle = 600 * time.Millisecond })
	r := newPerfRig(t, 200, 40)
	stopped := 0
	r.s.model = r.s.m().WithRecorder(&fakeRecorder{stopped: &stopped, loud: 4194304})
	r.s.press("8")
	if r.s.m().takeAlign() != ticksPerBar {
		t.Fatal("a take does not start on the bar by default")
	}
	r.s.press("t")
	if !r.s.m().mem.LooperOnBeat || r.s.m().takeAlign() != 24 {
		t.Fatal("t did not switch to the beat")
	}
	r.s.press("t")
	// A calibration waits for a bar whatever is set, and is one bar long.
	r.feed(midiin.Event{Kind: midiin.Start})
	r.tick(1)
	r.s.press("c")
	if m := r.s.m(); m.lp.phase != lpArmed || !m.lp.cal || m.takeBars() != 1 || m.takeAlign() != ticksPerBar {
		t.Fatalf("not calibrating: %+v", m.lp)
	}
	r.s.m().lp.session.Started = time.Now().Add(-5 * time.Second)
	at := time.Now().Add(-2500 * time.Millisecond)
	tickAt := func(d time.Duration, run bool) {
		next, cmd := r.s.m().midiEvent(midiMsg{link: r.link, ev: midiin.Event{Kind: midiin.Clock, At: at.Add(d)}})
		r.s.model = next
		if run {
			r.s.run(cmd)
		}
	}
	for r.s.m().clock.ticks < ticksPerBar-1 {
		tickAt(-time.Hour, false)
	}
	tickAt(0, false)
	for r.s.m().clock.ticks < 2*ticksPerBar-1 {
		tickAt(time.Second, false)
	}
	tickAt(2*time.Second, true)
	m := r.s.m()
	// The fake recording is silent where the bar was: the calibration says so, and puts nothing on a pad.
	if m.lp.phase != lpIdle || !strings.Contains(m.message, "calibration failed: nothing was heard") {
		t.Fatalf("calibration did not run: phase %d %q", m.lp.phase, m.message)
	}
	if m.snap.Pads[3].HasSample {
		t.Error("a calibration put a loop on a pad")
	}
}

func TestAnArmOnBeatTwoStartsOnBeatOneOfTheNextBar(t *testing.T) {
	r := newPerfRig(t, 200, 40)
	r.s.model = r.s.m().WithRecorder(&fakeRecorder{stopped: new(int)})
	r.s.press("8")
	r.feed(midiin.Event{Kind: midiin.Start})
	r.tick(ticksPerBar + 25) // beat 2 of bar 2
	r.s.press("a")
	// The recorder is up for 400 ms by the next bar, which is a bar away at most.
	r.s.m().lp.session.Started = time.Now().Add(-400 * time.Millisecond)
	for r.s.m().clock.ticks < 2*ticksPerBar-1 {
		r.tick(1)
		if r.s.m().lp.phase != lpArmed {
			t.Fatalf("left the armed state at tick %d: phase %d", r.s.m().clock.ticks, r.s.m().lp.phase)
		}
	}
	r.tick(1)
	if m := r.s.m(); m.lp.phase != lpRecording || m.lp.startTick != 2*ticksPerBar {
		t.Errorf("phase %d, started on tick %d, want recording from tick %d", m.lp.phase, m.lp.startTick, 2*ticksPerBar)
	}
}

// ringRecorder is a backend whose ring is given: the program listens to it itself.
type ringRecorder struct {
	fakeRecorder
	ring *record.Ring
}

func (r *ringRecorder) StartRing(device string, seconds int) (*record.Ring, error) {
	return r.ring, nil
}

func TestTheLooperCutsATakeOutOfTheMemoryItListensInto(t *testing.T) {
	r := newPerfRig(t, 200, 40)
	ring := record.NewRing(48000, 90)
	base := time.Now().Add(-12 * time.Second)
	// 12 s of a ramp that arrives on time: block n ends at base + (n+1) * 20 ms.
	for n := 0; n < 600; n++ {
		data := make([]int16, 960*2)
		for i := range 960 {
			data[2*i], data[2*i+1] = int16((n*960+i)%20000+500), int16((n*960+i)%20000+500)
		}
		ring.Write(data, base.Add(time.Duration(n+1)*20*time.Millisecond))
	}
	stopped := 0
	r.s.model = r.s.m().WithRecorder(&ringRecorder{fakeRecorder: fakeRecorder{stopped: &stopped}, ring: ring})
	r.s.press("8")
	r.s.send(tickMsg{}) // the Update that starts the listening
	if r.s.m().ring == nil {
		t.Fatal("the program did not start listening")
	}
	r.s.send(tickMsg{})
	if lv := r.s.m().webLevel(); lv == nil || lv.Src != "input" {
		t.Errorf("no level from the ring: %+v", lv)
	}
	r.s.press("b", "b", "b") // 1 bar
	r.feed(midiin.Event{Kind: midiin.Start})
	r.tick(1)
	r.s.press("a")
	// No recorder to start: armed at once, and the meter goes on.
	if m := r.s.m(); m.lp.phase != lpArmed || m.lp.session != nil || !m.lp.ringTake || m.lvl == nil {
		t.Fatalf("not armed on the ring: %+v", m.lp)
	}
	at := base.Add(3 * time.Second)
	tickAt := func(d time.Duration, run bool) {
		next, cmd := r.s.m().midiEvent(midiMsg{link: r.link, ev: midiin.Event{Kind: midiin.Clock, At: at.Add(d)}})
		r.s.model = next
		if run {
			r.s.run(cmd)
		}
	}
	for r.s.m().clock.ticks < ticksPerBar-1 {
		tickAt(-time.Hour, false)
	}
	tickAt(0, false)
	if r.s.m().lp.phase != lpRecording {
		t.Fatalf("not recording at the bar: %d", r.s.m().lp.phase)
	}
	for r.s.m().clock.ticks < 2*ticksPerBar-1 {
		tickAt(time.Second, false)
	}
	tickAt(2*time.Second, true)
	m := r.s.m()
	row := m.snap.Pads[3]
	if m.lp.phase != lpIdle || !row.HasSample || !strings.HasPrefix(row.Name, "LOOP ") {
		t.Fatalf("no loop on A4: phase %d, %+v, %q", m.lp.phase, row, m.message)
	}
	if m.ring == nil || m.lvl == nil {
		t.Error("the listening stopped with the take")
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
	if !strings.Contains(m.message, "remixed in 16 slices") {
		t.Fatalf("message %q", m.message)
	}
	found := false
	for _, r := range m.snap.Bank(m.loopBank()) {
		if strings.HasPrefix(r.Name, "RMX ") {
			found = true
		}
	}
	if !found {
		t.Errorf("no remix on the bank (%q)", m.message)
	}
}
