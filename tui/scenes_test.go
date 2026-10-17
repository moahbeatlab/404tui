package tui

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/moahbeatlab/404tui/midiin"
)

// perfRig is the Perform view with a fake MIDI input and output, and helpers to play the unit.
type perfRig struct {
	t    *testing.T
	s    *screen
	link *midiLink
	src  *fakeMIDI
}

func newPerfRig(t *testing.T, w, h int) *perfRig {
	t.Helper()
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: w, Height: h})
	r := &perfRig{t: t, s: s, link: &midiLink{ch: make(chan midiin.Event, 4)}, src: &fakeMIDI{}}
	m := s.m()
	m.midi, m.midiSrc = r.link, r.src
	s.model = m
	s.press("7")
	return r
}

func (r *perfRig) feed(e midiin.Event) {
	e.At = time.Now()
	next, _ := r.s.m().midiEvent(midiMsg{link: r.link, ev: e})
	r.s.model = next
}

func (r *perfRig) tick(n int) {
	for range n {
		r.feed(midiin.Event{Kind: midiin.Clock})
	}
}

func (r *perfRig) cc(ch, n, v int) {
	r.feed(midiin.Event{Kind: midiin.CC, Channel: ch, Data1: n, Data2: v})
}

func (r *perfRig) act(act string, bus int) {
	r.s.model = r.s.m().sceneAct(act, bus, 0)
}

func (r *perfRig) fade(bus int, pos float64) {
	r.s.model = r.s.m().sceneAct("fade", bus, pos)
}

func (r *perfRig) sc() *sceneState { return r.s.m().sc }

// save puts scene A and B on a bus with the knobs at the values given (-1 for a knob not moved).
func (r *perfRig) save(bus int, a, b [3]int) {
	for i, vals := range [][3]int{a, b} {
		for k, v := range vals {
			if v >= 0 {
				r.cc(bus+1, 16+k, v)
			}
		}
		r.act([]string{"a", "b"}[i], bus)
	}
}

func (r *perfRig) sent() [][]byte {
	r.src.mu.Lock()
	defer r.src.mu.Unlock()
	return append([][]byte(nil), r.src.sent...)
}

func (r *perfRig) clearSent() {
	r.src.mu.Lock()
	r.src.sent = nil
	r.src.mu.Unlock()
}

// valid fails the test for a message that is not a controller on channel 1 or 2 with 7-bit data.
func validMessage(t *testing.T, msg []byte) {
	t.Helper()
	if len(msg) != 3 || msg[0] != 0xB0 && msg[0] != 0xB1 || msg[1] > 127 || msg[2] > 127 {
		t.Fatalf("not a MIDI controller message: % x", msg)
	}
}

func TestAGlideTakesExactlyTheBarsItWasGivenAndMovesOnlyForward(t *testing.T) {
	for _, bars := range []int{1, 2, 4, 8} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d bars reverse=%v", bars, reverse), func(t *testing.T) {
				r := newPerfRig(t, 170, 90)
				a, b := [3]int{0, 20, 127}, [3]int{100, 80, 0}
				if reverse {
					a, b = b, a
				}
				r.save(0, a, b)
				r.act("bars", bars)
				r.feed(midiin.Event{Kind: midiin.Start})
				r.fade(0, 0)
				r.clearSent()
				r.tick(7) // not on a bar: the glide waits
				r.act("glide", 0)
				if g := r.sc().bus[0].glide; g == nil || !g.waiting {
					t.Fatal("the glide is not waiting for a bar")
				}
				if len(r.sent()) != 0 {
					t.Fatal("something was sent while the glide waits")
				}
				// Count the ticks from the bar it starts on to the end.
				ticks := 0
				var last [3]int
				for i := range last {
					last[i] = a[i]
				}
				for i := 0; i < ticksPerBar*(bars+3) && r.sc().bus[0].glide != nil; i++ {
					r.tick(1)
					if g := r.sc().bus[0].glide; g != nil && !g.waiting {
						ticks++
					}
					for _, msg := range r.sent() {
						validMessage(t, msg)
					}
					// Each knob goes one way only.
					for k := range 3 {
						if v := knobSent(r.sent(), k); v >= 0 {
							if (b[k] >= a[k] && v < last[k]) || (b[k] < a[k] && v > last[k]) {
								t.Fatalf("knob %d went from %d to %d, against its direction %d -> %d", k+1, last[k], v, a[k], b[k])
							}
							last[k] = v
						}
					}
					if mix := r.sc().bus[0].mix; mix < 0 || mix > 1 {
						t.Fatalf("mix %v", mix)
					}
				}
				// The glide runs bars*96 ticks after the bar it began on (the tick of the bar itself starts it).
				if want := bars * ticksPerBar; ticks < want-1 || ticks > want+1 {
					t.Errorf("the glide took %d ticks, want %d", ticks, want)
				}
				if r.sc().bus[0].glide != nil {
					t.Fatal("the glide did not end")
				}
				for k := range 3 {
					if got := knobSent(r.sent(), k); got != b[k] {
						t.Errorf("knob %d ended at %d, want %d", k+1, got, b[k])
					}
				}
				if mix := r.sc().bus[0].mix; (!reverse && mix != 1) || (reverse && mix != 1) {
					t.Errorf("the glide ended at mix %v", mix)
				}
			})
		}
	}
}

// knobSent is the last value sent for a knob (0 to 2), or -1.
func knobSent(sent [][]byte, k int) int {
	for i := len(sent) - 1; i >= 0; i-- {
		if int(sent[i][1]) == 16+k {
			return int(sent[i][2])
		}
	}
	return -1
}

func TestAGlideThatIsStoppedStaysWhereItIs(t *testing.T) {
	r := newPerfRig(t, 170, 90)
	r.save(0, [3]int{0, -1, -1}, [3]int{100, -1, -1})
	r.act("bars", 2)
	r.feed(midiin.Event{Kind: midiin.Start})
	r.fade(0, 0)
	r.act("glide", 0)
	r.tick(ticksPerBar + ticksPerBar/2) // the bar, then half of the first bar of the glide
	r.act("glide", 0)                   // a tap on the button stops it
	if r.sc().bus[0].glide != nil {
		t.Fatal("the glide was not stopped")
	}
	mix := r.sc().bus[0].mix
	if mix < 0.2 || mix > 0.3 {
		t.Errorf("a quarter of the way: the fade is at %v", mix)
	}
	r.clearSent()
	r.tick(ticksPerBar)
	if n := len(r.sent()); n != 0 {
		t.Errorf("%d messages were sent after the glide was stopped", n)
	}
	// Starting again goes to the nearer end that it is not at: from a quarter of the way, to B.
	r.act("glide", 0)
	if g := r.sc().bus[0].glide; g == nil || g.from != mix || g.to != 1 {
		t.Errorf("a new glide from %v: %+v", mix, g)
	}
}

func TestAGlideRefusesWhenThereIsNothingToFadeOrNoClock(t *testing.T) {
	r := newPerfRig(t, 170, 90)
	// No scenes at all.
	r.feed(midiin.Event{Kind: midiin.Start})
	r.act("glide", 0)
	if r.sc().bus[0].glide != nil || !strings.Contains(r.s.m().message, "save scene A and scene B") {
		t.Errorf("a glide without scenes: %q", r.s.m().message)
	}
	// Scenes in which the unit never said a knob.
	r.act("a", 0)
	r.act("b", 0)
	r.act("glide", 0)
	if r.sc().bus[0].glide != nil || !strings.Contains(r.s.m().message, "nothing to fade") {
		t.Errorf("a glide of scenes with no values: %q", r.s.m().message)
	}
	r.tick(ticksPerBar * 2)
	if n := len(r.sent()); n != 0 {
		t.Errorf("%d messages were sent for scenes that hold nothing", n)
	}
	// Good scenes but the clock stopped.
	r2 := newPerfRig(t, 170, 90)
	r2.save(0, [3]int{0, -1, -1}, [3]int{100, -1, -1})
	r2.act("glide", 0)
	if r2.sc().bus[0].glide != nil || !strings.Contains(r2.s.m().message, "MIDI clock") {
		t.Errorf("a glide without a clock: %q", r2.s.m().message)
	}
	r2.feed(midiin.Event{Kind: midiin.Start})
	r2.feed(midiin.Event{Kind: midiin.Stop})
	r2.act("glide", 0)
	if r2.sc().bus[0].glide != nil {
		t.Error("a glide began with the clock stopped")
	}
}

func TestOnlyTheValuesTheUnitSaidAreFaded(t *testing.T) {
	r := newPerfRig(t, 170, 90)
	// C1 and C2 are known (C2 keeps its last value in B); C3, the switch and the effect were never said.
	r.save(0, [3]int{10, 50, -1}, [3]int{90, -1, -1})
	r.act("bars", 1)
	r.feed(midiin.Event{Kind: midiin.Start})
	r.clearSent()
	r.fade(0, 1)
	for _, msg := range r.sent() {
		validMessage(t, msg)
		// C1 and C2 were said (C2 still stood at 50 when B was saved); C3, the switch and the effect never were.
		if msg[1] != 16 && msg[1] != 17 {
			t.Errorf("sent controller %d, which the unit never said", msg[1])
		}
	}
	if got := knobSent(r.sent(), 0); got != 90 {
		t.Errorf("C1 ended at %d, want 90", got)
	}
}

func TestTwoIdenticalScenesSendTheSameValueAndNothingElse(t *testing.T) {
	r := newPerfRig(t, 170, 90)
	r.save(0, [3]int{64, 64, 64}, [3]int{64, 64, 64})
	r.act("bars", 1)
	r.feed(midiin.Event{Kind: midiin.Start})
	r.act("glide", 0)
	r.clearSent()
	r.tick(ticksPerBar * 3)
	for _, msg := range r.sent() {
		validMessage(t, msg)
		if msg[2] != 64 {
			t.Errorf("sent %d for scenes that are the same", msg[2])
		}
	}
	if n := len(r.sent()); n > 3 {
		t.Errorf("%d messages for scenes that are the same, want at most one per knob", n)
	}
}

func TestEachBusGlidesOnItsOwn(t *testing.T) {
	r := newPerfRig(t, 170, 90)
	r.save(0, [3]int{0, -1, -1}, [3]int{100, -1, -1})
	r.save(1, [3]int{127, -1, -1}, [3]int{27, -1, -1})
	r.act("bars", 1)
	r.feed(midiin.Event{Kind: midiin.Start})
	r.fade(0, 0)
	r.fade(1, 0)
	r.clearSent()
	r.act("glide", 0) // only bus 1
	r.tick(ticksPerBar + ticksPerBar/2)
	for _, msg := range r.sent() {
		validMessage(t, msg)
		if msg[0] != 0xB0 {
			t.Fatalf("bus 2 was moved by the glide of bus 1: % x", msg)
		}
	}
	// Now bus 2 as well, a bar later; both end, each at its own B, each on its own channel.
	r.act("glide", 1)
	r.tick(ticksPerBar * 3)
	var lastCh = map[byte]int{}
	for _, msg := range r.sent() {
		validMessage(t, msg)
		lastCh[msg[0]] = int(msg[2])
	}
	if lastCh[0xB0] != 100 || lastCh[0xB1] != 27 {
		t.Errorf("the buses ended at %d and %d, want 100 and 27", lastCh[0xB0], lastCh[0xB1])
	}
	if r.sc().bus[0].glide != nil || r.sc().bus[1].glide != nil {
		t.Error("a glide did not end")
	}
}

func TestTheClockStoppingAndContinuingDoesNotBreakAGlide(t *testing.T) {
	r := newPerfRig(t, 170, 90)
	r.save(0, [3]int{0, -1, -1}, [3]int{100, -1, -1})
	r.act("bars", 1)
	r.feed(midiin.Event{Kind: midiin.Start})
	r.fade(0, 0)
	r.act("glide", 0)
	r.tick(ticksPerBar + 30)
	mix := r.sc().bus[0].mix
	r.feed(midiin.Event{Kind: midiin.Stop})
	r.clearSent()
	time.Sleep(10 * time.Millisecond)
	if r.sc().bus[0].mix != mix || len(r.sent()) != 0 {
		t.Error("the glide moved while the clock was stopped")
	}
	r.feed(midiin.Event{Kind: midiin.Continue})
	r.tick(ticksPerBar)
	if r.sc().bus[0].glide != nil || r.sc().bus[0].mix != 1 {
		t.Errorf("after Continue the glide is %+v at %v", r.sc().bus[0].glide, r.sc().bus[0].mix)
	}
	for _, msg := range r.sent() {
		validMessage(t, msg)
	}
}

func TestAnOutputThatCannotBeOpenedOrSentToIsReportedNotFatal(t *testing.T) {
	r := newPerfRig(t, 170, 90)
	r.src.openOutErr = errors.New("no MIDI output with \"404\" in its name")
	r.save(0, [3]int{0, -1, -1}, [3]int{100, -1, -1})
	r.act("bars", 1)
	r.feed(midiin.Event{Kind: midiin.Start})
	r.fade(0, 1)
	r.act("glide", 0)
	r.tick(ticksPerBar * 3)
	r.s.shows("cannot send to the unit")
	// The unit shows up later: sending begins by itself (it tries again after a few seconds).
	r.src.openOutErr = nil
	r.sc().tried = time.Now().Add(-time.Minute)
	r.fade(0, 0.5)
	if len(r.sent()) == 0 {
		t.Error("nothing was sent once the output could be opened")
	}
	// An output that fails when sent to is let go and opened again later, with no panic.
	r.src.sendErr = errors.New("device unplugged")
	r.fade(0, 0.1)
	r.fade(0, 0.9)
	r.src.sendErr = nil
	r.sc().tried = time.Now().Add(-time.Minute)
	r.fade(0, 0.3)
	r.s.m().View()
}

// Whatever is done, in any order, nothing breaks: the screen can always be drawn, every message sent
// is a valid controller, and the fade stays between A and B.
func TestScenesAndMotionSurviveRandomPlay(t *testing.T) {
	// A short run for every test run; SCENES_FUZZ=1 plays 25 seeds of 600 steps (about two minutes).
	seeds, steps := int64(5), 200
	if os.Getenv("SCENES_FUZZ") != "" {
		seeds, steps = 25, 600
	}
	for seed := int64(1); seed <= seeds; seed++ {
		t.Run(fmt.Sprint("seed", seed), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))
			r := newPerfRig(t, 110+rng.Intn(90), 40+rng.Intn(60))
			for step := 0; step < steps; step++ {
				switch rng.Intn(16) {
				case 0, 1, 2:
					r.tick(1 + rng.Intn(60))
				case 3:
					r.cc(1+rng.Intn(2), 16+rng.Intn(3), rng.Intn(128))
				case 4:
					r.act([]string{"a", "b"}[rng.Intn(2)], rng.Intn(2))
				case 5:
					r.fade(rng.Intn(2), rng.Float64()*2.4-0.7) // also past both ends
				case 6:
					r.act("glide", rng.Intn(2))
				case 7:
					r.act("bars", []int{1, 2, 4, 8}[rng.Intn(4)])
				case 8:
					r.s.model = r.s.m().motionAct(rng.Intn(2), rng.Intn(motionSlots))
				case 9:
					r.feed(midiin.Event{Kind: midiin.Start})
				case 10:
					r.feed(midiin.Event{Kind: midiin.Stop})
				case 11:
					r.feed(midiin.Event{Kind: midiin.Continue})
				case 12:
					r.act([]string{"jumpa", "jumpb"}[rng.Intn(2)], rng.Intn(2))
				case 13:
					r.s.send(tea.WindowSizeMsg{Width: 100 + rng.Intn(110), Height: 30 + rng.Intn(70)})
				case 14:
					// A tap anywhere on the screen.
					r.s.send(tea.MouseMsg{X: rng.Intn(220), Y: rng.Intn(100), Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
				case 15:
					r.src.sendErr = map[bool]error{true: errors.New("gone"), false: nil}[rng.Intn(5) == 0]
				}
				if out := r.s.m().View(); strings.TrimSpace(ansi.Strip(out)) == "" {
					t.Fatalf("step %d: the screen is empty", step)
				}
				if sc := r.sc(); sc != nil {
					for bus := range 2 {
						if mix := sc.bus[bus].mix; mix < 0 || mix > 1 {
							t.Fatalf("step %d: the fade of bus %d is %v", step, bus+1, mix)
						}
					}
				}
			}
			for _, msg := range r.sent() {
				validMessage(t, msg)
			}
		})
	}
}

// A tap on every cell of the screen, with scenes saved and a glide running, at several sizes.
func TestATapOnEveryCellOfThePerformViewIsHarmless(t *testing.T) {
	for _, size := range [][2]int{{110, 40}, {170, 54}, {200, 90}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			r := newPerfRig(t, size[0], size[1])
			r.save(0, [3]int{0, 10, 20}, [3]int{100, 90, 80})
			r.save(1, [3]int{5, 15, 25}, [3]int{105, 95, 85})
			r.feed(midiin.Event{Kind: midiin.Start})
			for y := 0; y < size[1]; y++ {
				for x := 0; x < size[0]; x++ {
					r.s.send(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
					if x%7 == 0 {
						r.tick(3)
					}
				}
				r.s.m().View()
			}
			for _, msg := range r.sent() {
				validMessage(t, msg)
			}
		})
	}
}

// The panel can be drawn at any width and height, with any state of the fade.
func TestTheScenePanelsDrawAtAnySize(t *testing.T) {
	r := newPerfRig(t, 170, 90)
	r.save(0, [3]int{0, 10, 20}, [3]int{100, 90, 80})
	for w := 60; w <= 230; w += 3 {
		for _, h := range []int{20, 35, 47, 60, 75, 100} {
			for _, mix := range []float64{0, 0.33, 1} {
				r.s.send(tea.WindowSizeMsg{Width: w, Height: h})
				r.fade(0, mix)
				r.s.m().View()
			}
		}
	}
}

func TestFourMotionSlotsPerBusAreButtonsThatShowTheirCurve(t *testing.T) {
	r := newPerfRig(t, 170, 80)
	r.feed(midiin.Event{Kind: midiin.Start})
	r.act("bars", 1)
	lines := func() []string { return strings.Split(ansi.Strip(r.s.m().View()), "\n") }
	// Four empty slots on each bus, side by side.
	empty := 0
	for _, l := range lines() {
		empty += strings.Count(l, "empty")
	}
	if empty != 8 {
		t.Fatalf("%d empty slots on the screen, want 8", empty)
	}
	// tapSlot taps the slot (0 to 3) of the first line of slots on the screen, found by its "empty".
	tapSlot := func(slot int) {
		t.Helper()
		for y, l := range lines() {
			if at := strings.Index(l, "empty"); at >= 0 {
				x := len([]rune(l[:at])) + 1 + slot*11
				r.s.send(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
				return
			}
		}
		t.Fatal("no empty slot on the screen")
	}
	// A tap on the second slot of bus 1 records into it.
	tapSlot(1)
	if s := r.sc().bus[0]; s.mo[1].state != motionArmed || s.mo[0].state != motionIdle {
		t.Fatalf("slot 2 state %d, slot 1 state %d", s.mo[1].state, s.mo[0].state)
	}
	r.tick(ticksPerBar)
	// C2 is turned up over the bar: a rising curve.
	for i := 0; i < 8; i++ {
		r.cc(1, 17, i*16)
		r.tick(ticksPerBar / 8)
	}
	r.tick(2)
	if r.sc().bus[0].mo[1].state != motionPlaying {
		t.Fatalf("slot 2 is %d after its bar", r.sc().bus[0].mo[1].state)
	}
	spark := r.sc().bus[0].mo[1].sparkline(8)
	if !strings.HasPrefix(spark, "C2 ") || !strings.Contains(spark, "▁") || !strings.Contains(spark, "▇") {
		t.Errorf("curve %q does not rise from low to high on C2", spark)
	}
	r.s.send(tea.WindowSizeMsg{Width: 170, Height: 80}) // a redraw: the view is kept over quiet clock ticks
	r.s.shows("▶ C2")
	// Another slot of the same bus cannot play at the same time.
	tapSlot(0)
	if r.sc().bus[0].mo[1].state == motionPlaying || r.sc().bus[0].mo[0].state != motionArmed {
		t.Error("two slots armed or playing on one bus")
	}
}
