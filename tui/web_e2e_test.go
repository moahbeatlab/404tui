package tui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/device"
	"github.com/moahbeatlab/404tui/midiin"
	"github.com/moahbeatlab/404tui/proto"
	"github.com/moahbeatlab/404tui/sim"
	"github.com/moahbeatlab/404tui/web"
)

// A webClient, as the tests need it: a WebSocket to the server, commands out, states in.
type webClient struct {
	c      net.Conn
	r      *bufio.Reader
	states chan webState
}

func dialWebClient(t *testing.T, addr string) *webClient {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	fmt.Fprintf(c, "GET /ws HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n", addr)
	b := &webClient{c: c, r: bufio.NewReader(c), states: make(chan webState, 4096)}
	for {
		l, err := b.r.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if l == "\r\n" {
			break
		}
	}
	go func() {
		for {
			var h [2]byte
			if _, err := io.ReadFull(b.r, h[:]); err != nil {
				return
			}
			n := int(h[1] & 0x7F)
			switch n {
			case 126:
				var x [2]byte
				io.ReadFull(b.r, x[:])
				n = int(x[0])<<8 | int(x[1])
			case 127:
				var x [8]byte
				io.ReadFull(b.r, x[:])
				n = int(x[4])<<24 | int(x[5])<<16 | int(x[6])<<8 | int(x[7])
			}
			buf := make([]byte, n)
			if _, err := io.ReadFull(b.r, buf); err != nil {
				return
			}
			var s webState
			if json.Unmarshal(buf, &s) == nil && h[0]&0x0F == 1 {
				b.states <- s
			}
		}
	}()
	return b
}

func (b *webClient) send(cmd string) {
	p := []byte(cmd)
	mask := []byte{9, 8, 7, 6}
	f := []byte{0x81}
	if len(p) < 126 {
		f = append(f, 0x80|byte(len(p)))
	} else {
		f = append(f, 0x80|126, byte(len(p)>>8), byte(len(p)))
	}
	f = append(f, mask...)
	for i, c := range p {
		f = append(f, c^mask[i%4])
	}
	b.c.Write(f)
}

// until reads states until one satisfies ok, or fails after a while.
func (b *webClient) until(t *testing.T, what string, ok func(webState) bool) webState {
	t.Helper()
	deadline := time.After(40 * time.Second)
	for {
		select {
		case s := <-b.states:
			if ok(s) {
				return s
			}
		case <-deadline:
			t.Fatalf("no state with %s", what)
		}
	}
}

// startWeb runs the program as `sp404 serve` does, on a simulated unit and a fake MIDI input, with a real
// server, and returns a webClient connected to it and a way to play the unit's clock.
func startWeb(t *testing.T) (*webClient, *fakeMIDI, *tea.Program) {
	t.Helper()
	s := sim.New()
	for i, name := range []string{"kick", "snare", "hat"} {
		p, _ := proto.NewPadIndex(uint16(i))
		s.SetSample(p, name, 48_000)
	}
	d := device.New(s, "sim")
	t.Cleanup(func() { d.Close() })
	srv := &web.Server{}
	l, err := srv.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	fm := &fakeMIDI{}
	mdl := New(d, &fakePlayer{}, &memStore{}).WithMIDI(fm).WithWeb(srv)
	mdl.ranger = false
	prog := tea.NewProgram(mdl, tea.WithoutRenderer(), tea.WithInput(nil), tea.WithOutput(io.Discard))
	srv.OnCommand = func(msg []byte) { prog.Send(WebCommand(append([]byte(nil), msg...))) }
	done := make(chan struct{})
	go func() { prog.Run(); close(done) }()
	t.Cleanup(func() { prog.Quit(); <-done })
	br := dialWebClient(t, l.Addr().String())
	br.until(t, "the pads", func(s webState) bool { return s.Unit && len(s.Pads) == 16 })
	return br, fm, prog
}

func (f *fakeMIDI) emit(e midiin.Event) {
	f.mu.Lock()
	on := f.on
	f.mu.Unlock()
	if on != nil {
		on(e)
	}
}

func TestABrowserPlaysThroughTheWholePipelineWithAClock(t *testing.T) {
	br, fm, _ := startWeb(t)
	// The program listens to the unit's MIDI as it would; wait for it to have opened the input.
	for range 100 {
		fm.mu.Lock()
		ok := fm.on != nil
		fm.mu.Unlock()
		if ok {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	fm.emit(midiin.Event{Kind: midiin.Start})
	for range 7 {
		fm.emit(midiin.Event{Kind: midiin.Clock})
	}
	st := br.until(t, "a running clock", func(s webState) bool { return s.Running })
	if st.Bank != "A" || len(st.Motion.Bus[0].Slots) != 4 {
		t.Errorf("state %+v", st.Bank)
	}
	// Arm a slot of BUS 1: the state says so. The clock reaches the next bar, the recording begins and
	// a turn of C1 is kept; at the end the slot plays and the curve is in the state.
	br.send(`{"a":"bars","n":1}`)
	br.send(`{"a":"motion","bus":0,"slot":1}`)
	br.until(t, "slot 2 armed", func(s webState) bool { return s.Motion.Bus[0].Slots[1].State == motionArmed })
	for range 96 {
		fm.emit(midiin.Event{Kind: midiin.Clock})
	}
	br.until(t, "slot 2 recording", func(s webState) bool { return s.Motion.Bus[0].Slots[1].State == motionRecording })
	for i := range 90 {
		if i%10 == 0 {
			fm.emit(midiin.Event{Kind: midiin.CC, Channel: 1, Data1: 16, Data2: 10 + i})
		}
		fm.emit(midiin.Event{Kind: midiin.Clock})
	}
	for range 10 {
		fm.emit(midiin.Event{Kind: midiin.Clock})
	}
	st = br.until(t, "slot 2 playing", func(s webState) bool { return s.Motion.Bus[0].Slots[1].State == motionPlaying })
	sl := st.Motion.Bus[0].Slots[1]
	if !sl.Filled || sl.Bars != 1 || sl.Knob != 1 || len(sl.Curve) != 16 {
		t.Errorf("the recorded slot: %+v", sl)
	}
	if sl.Curve[0] >= sl.Curve[15] {
		t.Errorf("the curve does not rise: %v", sl.Curve)
	}
	// What the loop plays was sent to the unit (the clock runs on through the loop).
	for range 60 {
		fm.emit(midiin.Event{Kind: midiin.Clock})
	}
	br.until(t, "the loop past half", func(s webState) bool {
		return s.Motion.Bus[0].Slots[1].State == motionPlaying && s.Motion.Bus[0].Slots[1].Pos >= 0.5
	})
	fm.mu.Lock()
	sent := len(fm.sent)
	fm.mu.Unlock()
	if sent == 0 {
		t.Error("the motion sent nothing to the unit")
	}
}

func TestACommandIsAnsweredWithTheNewStateWithinAFewMilliseconds(t *testing.T) {
	br, _, _ := startWeb(t)
	var worst, total time.Duration
	const n = 40
	for i := range n {
		bars := []int{1, 2, 3, 4, 8}[i%5]
		if i%5 == 0 && i == 0 {
			bars = 8 // make the first one a change
		}
		start := time.Now()
		br.send(fmt.Sprintf(`{"a":"bars","n":%d}`, bars))
		br.until(t, "the length", func(s webState) bool { return s.Motion.Bars == bars })
		d := time.Since(start)
		total += d
		worst = max(worst, d)
		time.Sleep(15 * time.Millisecond)
	}
	t.Logf("round trip for a command: mean %v, worst %v", total/n, worst)
	if total/n > 150*time.Millisecond { // a few milliseconds when it runs normally; the race detector is ten times slower
		t.Errorf("a command takes %v on average to come back as a state", total/n)
	}
}

func TestAFloodOfCommandsAndClockTicksKeepsTheLastStateRight(t *testing.T) {
	br, fm, _ := startWeb(t)
	for range 100 {
		fm.mu.Lock()
		ok := fm.on != nil
		fm.mu.Unlock()
		if ok {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	fm.emit(midiin.Event{Kind: midiin.Start})
	rng := rand.New(rand.NewSource(7))
	acts := []string{"pad", "mute", "level", "focus", "motion", "mclear", "mspeed", "mrev", "bars", "dub", "solo", "cue", "nextnow", "clearnext"}
	for i := 0; i < 1500; i++ {
		a := acts[rng.Intn(len(acts))]
		br.send(fmt.Sprintf(`{"a":%q,"i":%d,"bus":%d,"slot":%d,"n":%d,"v":%d}`, a, rng.Intn(18)-1, rng.Intn(2), rng.Intn(4), []int{1, 2, 3, 4, 8}[rng.Intn(5)], rng.Intn(140)-5))
		for range rng.Intn(5) {
			fm.emit(midiin.Event{Kind: midiin.Clock})
		}
		if i%7 == 0 {
			fm.emit(midiin.Event{Kind: midiin.CC, Channel: 1 + rng.Intn(2), Data1: 16 + rng.Intn(3), Data2: rng.Intn(128)})
		}
	}
	// Whatever happened, the program is alive and a last command is answered.
	br.send(`{"a":"bars","n":3}`)
	st := br.until(t, "the last length", func(s webState) bool { return s.Motion.Bars == 3 })
	if len(st.Pads) != 16 {
		t.Errorf("%d pads after the flood", len(st.Pads))
	}
	for _, p := range st.Pads {
		if p.Level < 0 || p.Level > 127 {
			t.Errorf("pad %d has the level %d", p.N, p.Level)
		}
	}
	for b := range 2 {
		for _, sl := range st.Motion.Bus[b].Slots {
			if sl.Pos < 0 || sl.Pos > 1 || strings.Contains(fmt.Sprint(sl.Curve), "-") {
				t.Errorf("slot out of range: %+v", sl)
			}
		}
	}
}
