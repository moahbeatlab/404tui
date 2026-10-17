package tui

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/midiin"
)

type fakeWeb struct {
	mu    sync.Mutex
	state [][]byte
}

func (f *fakeWeb) Publish(b []byte) {
	f.mu.Lock()
	f.state = append(f.state, append([]byte(nil), b...))
	f.mu.Unlock()
}

func (f *fakeWeb) last(t *testing.T) webState {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.state) == 0 {
		t.Fatal("nothing was published")
	}
	var s webState
	if err := json.Unmarshal(f.state[len(f.state)-1], &s); err != nil {
		t.Fatalf("the state is not JSON: %v", err)
	}
	return s
}

// webRig is the Perform view with a web side, and a way to send it commands as the browser does.
func webRig(t *testing.T) (*perfRig, *fakeWeb) {
	r := newPerfRig(t, 170, 80)
	w := &fakeWeb{}
	m := r.s.m()
	r.s.model = m.WithWeb(w)
	return r, w
}

func (r *perfRig) web(cmd string) {
	next, c := r.s.m().Update(WebCommand(cmd))
	r.s.model = next
	runCmd(c)
}

// runCmd runs the commands the program would, each on its own and given a moment: a batch holds the
// wait for the next MIDI message, which never ends here.
func runCmd(c tea.Cmd) {
	if c == nil {
		return
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- c() }()
	select {
	case msg := <-done:
		if b, ok := msg.(tea.BatchMsg); ok {
			for _, x := range b {
				runCmd(x)
			}
		}
	case <-time.After(100 * time.Millisecond):
	}
}

func TestTheWebStateHoldsWhatPerformShows(t *testing.T) {
	r, w := webRig(t)
	r.feed(midiin.Event{Kind: midiin.Start})
	r.tick(100)
	r.s.m().publishWeb() // midiEvent is called by Update in the program; here it is called by hand
	s := w.last(t)
	if len(s.Pads) != 16 || s.Bank != "A" || !s.Unit {
		t.Fatalf("pads %d bank %q unit %v", len(s.Pads), s.Bank, s.Unit)
	}
	if !s.Running || s.Beat < 0 || s.Bar != 2 {
		t.Errorf("clock: running %v beat %d bar %d after 100 ticks", s.Running, s.Beat, s.Bar)
	}
	if s.Pads[0].Name != "kick" || !s.Pads[0].Has || s.Pads[0].N != 1 {
		t.Errorf("pad 1 is %+v", s.Pads[0])
	}
	if s.Motion.Bars != 2 || len(s.Motion.Bus[0].Slots) != 4 {
		t.Errorf("motion %+v", s.Motion)
	}
	// What comes out is plain JSON the page can read: every field it uses is there.
	raw := string(w.state[len(w.state)-1])
	for _, key := range []string{`"pads"`, `"focus"`, `"motion"`, `"fx"`, `"bpm"`, `"now"`, `"next"`, `"rec"`, `"msg"`, `"curve"`, `"knobs"`} {
		if !strings.Contains(raw, key) {
			t.Errorf("no %s in the state", key)
		}
	}
}

func TestACommandFromTheBrowserDoesWhatTheButtonDoes(t *testing.T) {
	r, w := webRig(t)
	r.feed(midiin.Event{Kind: midiin.Start})
	r.tick(3)
	// A pad: a tap mutes it (as a click does), and the state says so.
	r.web(`{"a":"pad","i":0}`)
	if !w.last(t).Pads[0].Muted {
		t.Error("a tap on pad 1 did not mute it")
	}
	r.web(`{"a":"pad","i":0}`)
	// Focus on and off.
	r.web(`{"a":"focus"}`)
	if !w.last(t).Focus.On {
		t.Error("focus did not switch on")
	}
	r.web(`{"a":"focus"}`)
	if w.last(t).Focus.On {
		t.Error("focus did not switch off")
	}
	// The length, overdub.
	r.web(`{"a":"bars","n":4}`)
	r.web(`{"a":"bars","n":5}`) // not a length: ignored
	if got := w.last(t).Motion.Bars; got != 4 {
		t.Errorf("the length is %d, want 4", got)
	}
	r.web(`{"a":"dub"}`)
	if !w.last(t).Motion.Overdub {
		t.Error("overdub did not switch on")
	}
	// A slot arms, the clock reaches a bar, the recording runs, speed and direction of the slot tapped.
	r.web(`{"a":"motion","bus":1,"slot":2}`)
	if got := w.last(t).Motion.Bus[1].Slots[2].State; got != motionArmed {
		t.Errorf("slot 3 of BUS 2 is in state %d, want armed", got)
	}
	if w.last(t).Motion.Bus[1].Sel != 2 {
		t.Error("the slot tapped is not the selected one")
	}
	r.web(`{"a":"mspeed","bus":1}`)
	r.web(`{"a":"mrev","bus":1}`)
	if sl := w.last(t).Motion.Bus[1].Slots[2]; sl.Speed != 2 || !sl.Rev {
		t.Errorf("speed %v rev %v", sl.Speed, sl.Rev)
	}
	r.web(`{"a":"mclear","bus":1}`)
	if sl := w.last(t).Motion.Bus[1].Slots[2]; sl.State != motionIdle || sl.Speed != 1 || sl.Rev {
		t.Errorf("slot after clear: %+v", sl)
	}
}

func TestTheWebCanSetAndMuteThePadVolumes(t *testing.T) {
	r, w := webRig(t)
	r.web(`{"a":"level","i":0,"v":64}`)
	if got := w.last(t).Pads[0].Level; got != 64 {
		t.Errorf("level on the screen %d, want 64", got)
	}
	if got := levelOf(t, r, "A1"); got != 64 {
		t.Errorf("level on the unit %d, want 64", got)
	}
	r.web(`{"a":"level","i":0,"v":500}`)
	r.web(`{"a":"level","i":1,"v":-9}`)
	if levelOf(t, r, "A1") != 127 || levelOf(t, r, "A2") != 0 {
		t.Errorf("levels not held to 0..127: %d %d", levelOf(t, r, "A1"), levelOf(t, r, "A2"))
	}
	r.web(`{"a":"level","i":0,"v":90}`)
	r.web(`{"a":"mute","i":0}`)
	if levelOf(t, r, "A1") != 0 || !w.last(t).Pads[0].Muted {
		t.Error("mute did not silence the pad")
	}
	r.web(`{"a":"mute","i":0}`)
	if levelOf(t, r, "A1") != 90 || w.last(t).Pads[0].Muted {
		t.Errorf("the second mute gave %d", levelOf(t, r, "A1"))
	}
}

func TestNothingABrowserSendsCanBreakTheProgram(t *testing.T) {
	r, _ := webRig(t)
	for _, c := range []string{
		``, `x`, `{}`, `[]`, `null`, `{"a":1}`, `{"a":"nothing"}`, `{"a":"pad","i":-1}`, `{"a":"pad","i":16}`, `{"a":"pad","i":99999999999}`,
		`{"a":"level","i":-3,"v":1}`, `{"a":"level","i":16,"v":1}`, `{"a":"mute","i":-1}`, `{"a":"motion","bus":7,"slot":9}`,
		`{"a":"motion","bus":-1,"slot":-1}`, `{"a":"mclear","bus":5}`, `{"a":"mspeed","bus":-5}`, `{"a":"bars","n":-1}`,
		`{"a":"level","i":0,"v":"x"}`, `{"a":"pad","i":0.5}`, strings.Repeat("{", 5000),
	} {
		r.web(c)
	}
	// Every command, at every index, with and without a clock.
	for run := range 2 {
		if run == 1 {
			r.feed(midiin.Event{Kind: midiin.Start})
			r.tick(200)
		}
		for _, a := range []string{"pad", "level", "mute", "focus", "rec", "motion", "mclear", "mspeed", "mrev", "bars", "dub", "nextnow", "clearnext"} {
			for i := -1; i <= 17; i++ {
				r.web(fmt.Sprintf(`{"a":%q,"i":%d,"bus":%d,"slot":%d,"n":%d,"v":%d}`, a, i, i%3, i%5, []int{1, 2, 3, 4, 8, 9}[(i+1)%6], i*9))
				r.tick(2)
			}
		}
	}
	_ = r.s.m().View()
}

func TestWithoutAUnitTheWebStateIsStillValid(t *testing.T) {
	w := &fakeWeb{}
	m := NewConnecting(func() (app.Unit, io.Closer, error) { return nil, nil, ErrNoUnit }, nil, &memStore{}).WithWeb(w)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	next, _ = next.(Model).Update(WebCommand(`{"a":"pad","i":3}`))
	next.(Model).publishWeb()
	if s := w.last(t); s.Unit || len(s.Pads) != 0 {
		t.Errorf("state without a unit: %+v", s)
	}
	raw := string(w.state[len(w.state)-1])
	if !strings.Contains(raw, `"pads":[]`) {
		t.Errorf("pads must be an empty list, not null: %s", raw)
	}
}

func TestEveryUpdatePublishesAndASnapshotIsCheap(t *testing.T) {
	r, w := webRig(t)
	before := len(w.state)
	next, _ := r.s.m().Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	r.s.model = next
	if len(w.state) == before {
		t.Error("an update did not publish the state")
	}
	start := time.Now()
	for range 200 {
		r.s.m().publishWeb()
	}
	if per := time.Since(start) / 200; per > 2*time.Millisecond {
		t.Errorf("a snapshot takes %v: too slow for the clock's rate", per)
	}
}
