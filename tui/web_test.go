package tui

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
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
	// A pad is deleted (at once: the clock does not run).
	r.s.model = func() tea.Model {
		m := r.s.m()
		m.clock = &clockState{}
		return m
	}()
	r.web(`{"a":"lpdel","i":2}`)
	if l = w.last(t).Looper; l.Pads[2].Has || l.Target != "A3" {
		t.Errorf("A3 not deleted: %+v", l.Pads[2])
	}
	r.web(`{"a":"lpdel","i":99}`) // out of range: nothing
}

func TestTheLevelIsPublishedWhateverIsRecorded(t *testing.T) {
	r, w := webRig(t)
	r.s.press("r")
	if got := w.last(t); got.Level != nil || got.LevelNote == "" {
		t.Fatalf("level %+v note %q before any reading", got.Level, got.LevelNote)
	}
	m := r.s.m()
	m.lvl = newLevelMeter()
	m.lvl.ok = true
	m.lvl.peak, m.lvl.hold, m.lvl.top = [2]float64{-20.4, math.Inf(-1)}, [2]float64{-6, -30}, -2
	m.lvl.clipped = time.Now()
	r.s.model = m
	r.s.press("r")
	l := w.last(t).Level
	if l == nil || l.Peak[0] != -20 || l.Peak[1] != -60 || l.Hold[0] != -6 || !l.Clip || l.Src != "input" {
		t.Errorf("level %+v", l)
	}
}

func TestTheSlicerOfThePageCutsAPadOntoTheNextFreePad(t *testing.T) {
	r, w := webRig(t)
	r.s.press("r")
	m := r.s.m()
	from, ok := m.slFreePad(0)
	if !ok {
		t.Skip("bank A is full")
	}
	src := formats.Sample{Channels: 1}
	for i := 0; i < 96000; i++ {
		src.Samples = append(src.Samples, int16(i/6000*1500))
	}
	if _, err := app.PutSample(m.unit, from, src, "SRC"); err != nil {
		t.Fatal(err)
	}
	r.s.run(m.load(""))
	s := w.last(t).Slicer
	if s.Bank != "A" || s.Pad != -1 || s.Slices != 8 || s.Order != "shuffle" || len(s.Pads) != 16 || s.Target != "A" {
		t.Fatalf("slicer %+v", s)
	}
	r.web(`{"a":"slgo"}`)
	if got := w.last(t); !strings.Contains(got.Msg, "tap the pad") {
		t.Errorf("sliced nothing without a pad: %q", got.Msg)
	}
	r.web(`{"a":"slbank","n":1}`)
	r.web(`{"a":"slbank","n":-1}`)
	r.web(fmt.Sprintf(`{"a":"slpad","i":%d}`, from.Pad()))
	r.web(`{"a":"slslices"}`)
	r.web(`{"a":"slorder"}`)
	r.web(`{"a":"sldiv"}`)
	r.web(`{"a":"sltbank","n":1}`)
	r.web(`{"a":"sltbank","n":-1}`)
	s = w.last(t).Slicer
	if s.Pad != int(from.Pad()) || s.Slices != 16 || s.Order != "in order" || s.Div != 4 || s.Target != "A" || s.Free == "" {
		t.Fatalf("slicer after the buttons: %+v", s)
	}
	r.web(`{"a":"slgo"}`)
	s = w.last(t).Slicer
	if s.Phase != "ready" || s.Dest < 0 || !strings.Contains(s.Src, "16 slices") {
		t.Fatalf("not ready after slicing: %+v (%q)", s, r.s.m().message)
	}
	// While the slices wait, the grid is the bank they go to: a pad of another bank is chosen.
	r.web(`{"a":"sltbank","n":1}`)
	r.web(`{"a":"slpad","i":6}`)
	s = w.last(t).Slicer
	if s.Target != "B" || s.Dest != 6 || s.Phase != "ready" {
		t.Fatalf("after choosing B7: %+v", s)
	}
	r.web(`{"a":"slbank","n":1}`) // the source cannot change now
	if got := w.last(t).Slicer.Bank; got != "A" {
		t.Errorf("the source bank changed while the slices wait: %s", got)
	}
	r.web(`{"a":"slgo"}`)
	m = r.s.m()
	if !strings.Contains(m.message, "16 slices") || !strings.Contains(m.message, "B7") {
		t.Fatalf("message %q", m.message)
	}
	if got := m.snap.Bank(1)[6].Name; !strings.HasPrefix(got, "SLC ") {
		t.Errorf("B7 holds %q, want the slices", got)
	}
	// The result starts muted: its level is 0 and a mute brings it back to the level it had.
	if row := m.snap.Bank(1)[6]; row.Level != 0 || !m.lvlMuted(row.Pad) {
		t.Errorf("B7 is not muted: level %d, muted %v", row.Level, m.lvlMuted(row.Pad))
	}
	if s := w.last(t).Slicer; len(s.Pads) == 16 && s.Phase != "pick" {
		t.Errorf("phase %s", s.Phase)
	}
	if s = w.last(t).Slicer; s.Phase != "pick" {
		t.Errorf("still waiting after the slices were placed: %+v", s)
	}
}

func TestTheBrowserCanChangeTheBank(t *testing.T) {
	r, w := webRig(t)
	r.s.press("r")
	s := w.last(t)
	if s.Bank != "A" || len(s.BankFill) != 10 || s.BankFill[0] == 0 {
		t.Fatalf("bank %q, fill %v", s.Bank, s.BankFill)
	}
	r.web(`{"a":"bank","i":3}`)
	if got := w.last(t).Bank; got != "D" {
		t.Errorf("bank %s after choosing D", got)
	}
	r.web(`{"a":"bank","i":99}`) // out of range: nothing
	if got := w.last(t).Bank; got != "D" {
		t.Errorf("bank %s after a bank out of range", got)
	}
}

func TestADeleteAndASlicePlacementWaitForBeat1OrTheLastBeatOfThePattern(t *testing.T) {
	t.Run("beat 1", func(t *testing.T) { testQueuedDelete(t, false); testQueuedPlace(t, false) })
	t.Run("last beat", func(t *testing.T) { testQueuedDelete(t, true); testQueuedPlace(t, true) })
}

func testQueuedDelete(t *testing.T, lastBeat bool) {
	r, w := webRig(t)
	r.s.press("r")
	m0 := r.s.m()
	m0.mem.LaterLastBeat = lastBeat
	r.s.model = m0
	r.feed(midiin.Event{Kind: midiin.Start})
	r.tick(5)
	length := r.s.m().loopBars() * ticksPerBar
	at := r.s.m().laterAt()
	if at == 0 {
		at = length
	}
	if !r.s.m().snap.Bank(0)[0].HasSample {
		t.Skip("A1 holds no sample")
	}
	// A delete is queued, shown, and can be taken back.
	r.web(`{"a":"lpdel","i":0}`)
	l := w.last(t).Looper
	if !l.Pads[0].Del || !strings.Contains(l.Later, "delete A1") || !r.s.m().snap.Bank(0)[0].HasSample {
		t.Fatalf("not queued: %+v", l)
	}
	r.web(`{"a":"lpdel","i":0}`)
	if w.last(t).Looper.Pads[0].Del {
		t.Fatal("asking again did not take the delete back")
	}
	r.web(`{"a":"lpdel","i":0}`)
	// Nothing happens before the last beat ...
	r.tick(at - 5 - 1)
	if !r.s.m().snap.Bank(0)[0].HasSample {
		t.Fatal("deleted before the last beat")
	}
	// ... and the last beat does it.
	m := r.s.m()
	next, cmd := m.midiEvent(midiMsg{link: r.link, ev: midiin.Event{Kind: midiin.Clock, At: time.Now()}})
	r.s.model = next
	if got := r.s.m().clock.ticks; got != at {
		t.Fatalf("at tick %d, want %d", got, at)
	}
	r.s.run(cmd)
	if r.s.m().snap.Bank(0)[0].HasSample {
		t.Errorf("A1 still holds its sample after the last beat began (%q)", r.s.m().message)
	}
	if w.last(t).Looper.Later != "" {
		t.Errorf("still queued: %q", w.last(t).Looper.Later)
	}
}

func testQueuedPlace(t *testing.T, lastBeat bool) {
	r, w := webRig(t)
	r.s.press("r")
	m := r.s.m()
	from, ok := m.slFreePad(0)
	if !ok {
		t.Skip("bank A is full")
	}
	src := formats.Sample{Channels: 1}
	for i := 0; i < 48000; i++ {
		src.Samples = append(src.Samples, int16(i/6000*1500))
	}
	if _, err := app.PutSample(m.unit, from, src, "SRC"); err != nil {
		t.Fatal(err)
	}
	r.s.run(m.load(""))
	m0 := r.s.m()
	m0.mem.LaterLastBeat = lastBeat
	r.s.model = m0
	r.feed(midiin.Event{Kind: midiin.Start})
	r.tick(3)
	length := r.s.m().loopBars() * ticksPerBar
	at := r.s.m().laterAt()
	if at == 0 {
		at = length
	}
	r.web(fmt.Sprintf(`{"a":"slpad","i":%d}`, from.Pad()))
	if got := w.last(t).Slicer.Pad; got != int(from.Pad()) {
		t.Fatalf("slpad: pad %d, busy %q, msg %q", got, r.s.m().busy, r.s.m().message)
	}
	r.web(`{"a":"slgo"}`)
	free := w.last(t).Slicer.Dest
	r.web(`{"a":"slgo"}`) // place
	s := w.last(t).Slicer
	if s.Phase != "pick" || !strings.Contains(s.Later, "slices →") {
		t.Fatalf("not queued: %+v (%q)", s, r.s.m().message)
	}
	r.tick(at - 3 - 1)
	if r.s.m().snap.Bank(0)[free].HasSample {
		t.Fatal("placed before the last beat")
	}
	next, cmd := r.s.m().midiEvent(midiMsg{link: r.link, ev: midiin.Event{Kind: midiin.Clock, At: time.Now()}})
	r.s.model = next
	r.s.run(cmd)
	if got := r.s.m().snap.Bank(0)[free].Name; !strings.HasPrefix(got, "SLC ") {
		t.Errorf("pad %d holds %q after the last beat began (%q)", free, got, r.s.m().message)
	}
}

func TestTheBanksOfTheLooperAndTheSlicerCanBeChosenDirectly(t *testing.T) {
	r, w := webRig(t)
	r.s.press("r")
	r.web(`{"a":"lpbankset","i":7}`)
	if got := w.last(t).Looper.Bank; got != "H" {
		t.Errorf("looper bank %s, want H", got)
	}
	r.web(`{"a":"lpbankset","i":99}`)
	if got := w.last(t).Looper.Bank; got != "H" {
		t.Errorf("looper bank %s after a bank out of range", got)
	}
	r.web(`{"a":"slbankset","i":4}`)
	r.web(`{"a":"sltbankset","i":9}`)
	if s := w.last(t).Slicer; s.Bank != "E" || s.Target != "J" {
		t.Errorf("slicer from %s to %s, want E to J", s.Bank, s.Target)
	}
}

func TestHoldingAPadInTheSlicerMutesIt(t *testing.T) {
	r, w := webRig(t)
	r.s.press("r")
	if !r.s.m().snap.Bank(0)[0].HasSample {
		t.Skip("A1 holds no sample")
	}
	m := r.s.m()
	m.snap.Pads[0].Level = 100 // the simulated pad is silent at first
	r.s.model = m
	r.web(`{"a":"slmute","i":0}`)
	s := w.last(t).Slicer
	if !s.Pads[0].Muted || r.s.m().snap.Bank(0)[0].Level != 0 {
		t.Fatalf("A1 is not muted: %+v level %d (%q)", s.Pads[0], r.s.m().snap.Bank(0)[0].Level, r.s.m().message)
	}
	r.web(`{"a":"slmute","i":0}`)
	if s = w.last(t).Slicer; s.Pads[0].Muted || r.s.m().snap.Bank(0)[0].Level == 0 {
		t.Errorf("A1 is not back: %+v", s.Pads[0])
	}
	r.web(`{"a":"slmute","i":15}`) // an empty pad: nothing
}

func TestTheLooperSettingsDoNotWaitForTheUnit(t *testing.T) {
	r, w := webRig(t)
	r.s.press("r")
	m := r.s.m()
	m.busy = "reading the unit"
	r.s.model = m
	r.web(`{"a":"lpbank","n":1}`)
	r.web(`{"a":"lpbankset","i":4}`)
	if got := w.last(t).Looper.Bank; got != "E" {
		t.Errorf("looper bank %s while the unit was busy, want E", got)
	}
	r.web(`{"a":"lppattern"}`)
	if got := r.s.m().message; !strings.Contains(got, "busy") {
		t.Errorf("a command that needs the unit did not say why it waits: %q", got)
	}
}

func TestTheDiceOfTheSlicerKeepTheLengthAndChangeTheSound(t *testing.T) {
	src := formats.Sample{Channels: 1}
	for i := 0; i < 48000; i++ {
		src.Samples = append(src.Samples, int16(1000+i/4000*500))
	}
	plain, _ := sliceSample(src, sliceOpts{n: 8, order: slOrderInOrder})
	if plain.Frames() != src.Frames() {
		t.Fatalf("plain slicing is %d frames, the sample %d", plain.Frames(), src.Frames())
	}
	for i := range src.Samples {
		// The ends of a slice have a short fade against clicks; inside, it is the sample.
		if at := i % 6000; at > 400 && at < 5600 && plain.Samples[i] != src.Samples[i] {
			t.Fatalf("slices in order differ from the sample at frame %d", i)
		}
	}
	for _, o := range []sliceOpts{
		{n: 8, order: slOrderShuffle, mangle: mangleOpts{drop: 75}},
		{n: 8, order: slOrderInOrder, mangle: mangleOpts{rev: 100}},
		{n: 16, order: slOrderRandom, mangle: mangleOpts{pitch: 12, roll: 100, fade: 50}},
		{n: 8, order: slOrderShuffle, div: 8, bpm: 120, mangle: mangleOpts{drop: 25, rev: 50, pitch: 5, roll: 50, human: 100, fade: 10}},
	} {
		out, order := sliceSample(src, o)
		if out.Frames() != src.Frames() {
			t.Errorf("%+v: %d frames, want %d", o, out.Frames(), src.Frames())
		}
		if len(order) == 0 {
			t.Errorf("%+v: no order", o)
		}
		same := true
		for i := range src.Samples {
			if at := i % 3000; at > 400 && at < 2600 && out.Samples[i] != src.Samples[i] {
				same = false
				break
			}
		}
		if same {
			t.Errorf("%+v: the dice changed nothing", o)
		}
	}
}

func TestTheDiceCanBeSetFromThePage(t *testing.T) {
	r, w := webRig(t)
	r.s.press("r")
	r.web(`{"a":"slmang","i":0,"n":1}`)
	r.web(`{"a":"slmang","i":2,"n":-1}`)
	s := w.last(t).Slicer
	if len(s.Mangle) != 6 || s.Mangle[0] != 25 || s.Mangle[2] != 12 || s.Active != 2 {
		t.Fatalf("dice %v, active %d", s.Mangle, s.Active)
	}
	r.web(`{"a":"slmang","i":6,"n":0}`)
	if s = w.last(t).Slicer; s.Active != 0 {
		t.Errorf("not all off: %v", s.Mangle)
	}
}

func TestTheCurrentPatternIsForgottenWhenTheProjectChanges(t *testing.T) {
	r, _ := webRig(t)
	r.s.press("r")
	m := r.s.m()
	slot := m.snap.Bank(0)[0].Pad
	m.perfNow, m.perfNext = &slot, &slot
	r.s.model = m
	cur := *m.snap
	cur.Project = m.snap.Project + 1
	next, _ := m.Update(loadedMsg{snap: &cur})
	if nm := next.(Model); nm.perfNow != nil || nm.perfNext != nil {
		t.Errorf("the pattern of the other project is still current: %v %v", nm.perfNow, nm.perfNext)
	}
}
