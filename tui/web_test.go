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
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/midiin"
	"github.com/moahbeatlab/404tui/proto"
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
	r.s.send(WebCommand(cmd)) // through Update, with what comes back
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

func TestATapFromTheBrowserIsolatesAPadAndADoubleTapMutesIt(t *testing.T) {
	r, w := webRig(t)
	bus, _ := proto.ParamByName("bus-fx")
	level, _ := proto.ParamByName("level")
	for i, v := range []int32{1, 1, 2} {
		p, _ := proto.NewPadIndex(uint16(i))
		r.s.m().unit.SetParam(bus.ID, proto.TargetPad(p), v)
		r.s.m().unit.SetParam(level.ID, proto.TargetPad(p), 100)
	}
	r.s.press("r")
	busOf := func(name string) int32 {
		r.s.press("r")
		return r.s.m().snap.Pads[pad(t, name).Index()].Params["bus-fx"]
	}
	// A tap isolates; another tap on the pad brings everything back.
	r.web(`{"a":"solo","i":0}`)
	if busOf("A1") != 1 || busOf("A2") != 0 || busOf("A3") != 2 {
		t.Errorf("buses %d %d %d, want 1 0 2", busOf("A1"), busOf("A2"), busOf("A3"))
	}
	if f := w.last(t).Focus; !f.On || f.Dry != 1 {
		t.Errorf("focus %+v", f)
	}
	r.web(`{"a":"solo","i":0}`)
	if busOf("A1") != 1 || busOf("A2") != 1 || busOf("A3") != 2 || w.last(t).Focus.On {
		t.Error("the second tap did not bring the buses back")
	}
	// Two pads on one bus kept, then one let go.
	r.web(`{"a":"solo","i":0}`)
	r.web(`{"a":"solo","i":1}`)
	if busOf("A1") != 1 || busOf("A2") != 1 || busOf("A3") != 2 {
		t.Errorf("two held pads: buses %d %d %d, want 1 1 2", busOf("A1"), busOf("A2"), busOf("A3"))
	}
	r.web(`{"a":"solo","i":0}`)
	if busOf("A1") != 0 || busOf("A2") != 1 {
		t.Errorf("A1 let go: buses %d %d, want 0 1", busOf("A1"), busOf("A2"))
	}
	r.web(`{"a":"solo","i":1}`)
	if busOf("A1") != 1 || w.last(t).Focus.On {
		t.Error("the last pad let go did not bring everything back")
	}
	// A double tap: the first tap isolates, the second undoes that and mutes.
	r.web(`{"a":"solo","i":0}`)
	r.web(`{"a":"mute","i":0,"n":1}`)
	if busOf("A2") != 1 || w.last(t).Focus.On {
		t.Error("the double tap left the pad isolated")
	}
	if got := levelOf(t, r, "A1"); got != 0 {
		t.Errorf("the double tap left A1 at level %d, want 0", got)
	}
	r.web(`{"a":"mute","i":0}`)
	if got := levelOf(t, r, "A1"); got != 100 {
		t.Errorf("A1 at level %d after unmuting, want 100", got)
	}
	// A pad that goes to no bus has nothing to isolate.
	r.web(`{"a":"solo","i":5}`)
	if w.last(t).Focus.On {
		t.Error("a pad without a bus was isolated")
	}
}

func TestAPatternIsChosenFromTheBrowserForNowAndNext(t *testing.T) {
	r, w := webRig(t)
	one, _ := formats.StartPattern([]proto.PadIndex{pad(t, "A1"), pad(t, "A2")}, 1)
	for _, slot := range []string{"A2", "A3"} {
		r.s.sim.SetPattern(pad(t, slot), pad(t, "A1"))
		r.s.sim.AddFile(formats.PatternPath(1, pad(t, slot)), one)
	}
	r.s.press("r", "7")
	st := w.last(t)
	if len(st.Patterns) != 16 || !st.Patterns[1].Exists || !st.Patterns[2].Exists || st.Patterns[0].Exists {
		t.Fatalf("patterns of bank A: %+v", st.Patterns)
	}
	r.web(`{"a":"cue","i":0,"n":2}`) // an empty slot: nothing
	if w.last(t).Next != "—" {
		t.Error("an empty pattern slot was taken")
	}
	r.web(`{"a":"cue","i":2,"n":2}`)
	st = w.last(t)
	if st.Next != "A3" || !st.Patterns[2].Next || st.Patterns[2].Now {
		t.Errorf("next %q %+v", st.Next, st.Patterns[2])
	}
	r.web(`{"a":"nextnow"}`)
	st = w.last(t)
	if st.Now != "A3" || st.Next != "—" || !st.Patterns[2].Now {
		t.Errorf("after next to now: now %q next %q", st.Now, st.Next)
	}
	r.web(`{"a":"cue","i":1,"n":1}`)
	if w.last(t).Now != "A2" {
		t.Errorf("now is %q, want A2", w.last(t).Now)
	}
	for _, c := range []string{`{"a":"cue","i":1,"n":3}`, `{"a":"cue","i":99,"n":1}`, `{"a":"cue","i":-1,"n":1}`} {
		r.web(c)
	}
	r.web(`{"a":"cue","i":2,"n":2}`)
	r.web(`{"a":"clearnext"}`)
	if w.last(t).Next != "—" {
		t.Error("clear next did not clear")
	}
}

func TestTheLooperIsShownAndControlledFromTheBrowser(t *testing.T) {
	r, w := webRig(t)
	r.s.press("r")
	l := w.last(t).Looper
	if l.Bank != "A" || l.Bars != 2 || l.Phase != "idle" || l.Target != "A4" || len(l.Pads) != 16 || !l.Pads[0].Has || !l.Pads[3].Target {
		t.Fatalf("looper %+v", l)
	}
	r.web(`{"a":"lpbars"}`)
	r.web(`{"a":"lpbank","n":1}`)
	r.web(`{"a":"lpdelay","n":5}`)
	l = w.last(t).Looper
	if l.Bars != 4 || l.Bank != "B" || l.Delay != 5 {
		t.Errorf("after bars, bank, delay: %+v", l)
	}
	r.web(`{"a":"lpbank","n":-1}`)
	r.s.model = r.s.m().WithRecorder(&fakeRecorder{stopped: new(int)})
	// No clock, no take.
	r.web(`{"a":"lparm"}`)
	if got := w.last(t); got.Looper.Phase != "idle" || !strings.Contains(got.Msg, "clock") {
		t.Errorf("armed without a clock: %+v %q", got.Looper, got.Msg)
	}
	// Armed with one.
	r.s.model = func() tea.Model {
		m := r.s.m()
		m.clock = &clockState{running: true, at: time.Now()}
		return m
	}()
	r.web(`{"a":"lparm"}`)
	if got := w.last(t).Looper; got.Phase != "armed" || got.Target != "A4" {
		t.Errorf("not armed: %+v", got)
	}
	r.web(`{"a":"lparm"}`) // again: cancelled
	if got := w.last(t).Looper.Phase; got != "idle" {
		t.Errorf("not cancelled: %s", got)
	}
	// A pad is deleted.
	r.web(`{"a":"lpdel","i":2}`)
	if l = w.last(t).Looper; l.Pads[2].Has || l.Target != "A3" {
		t.Errorf("A3 not deleted: %+v", l.Pads[2])
	}
	r.web(`{"a":"lpdel","i":99}`) // out of range: nothing
}
