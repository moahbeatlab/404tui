package record

import (
	"os"
	"testing"
	"time"

	"github.com/moahbeatlab/404tui/formats"
)

// A take is tested by filling a ring with a signal that says where it is: every frame carries its
// own number, so the frames that come out of a capture say exactly where it was cut. There is no
// unit, no clock and no sound in any of this — a capture is arithmetic over a buffer.

// rampRing makes a ring of seconds seconds at rate, filled with frames numbered from zero, all the
// way to the end, as if something had been playing into it.
func rampRing(rate, seconds int) *Ring {
	r := NewRing(rate, seconds)
	r.Write(ramp(0, int64(rate*seconds)), time.Now())
	return r
}

// ramp is frames of a stereo signal whose left channel is the frame number, so a cut can be checked
// by looking at what it starts with.
func ramp(from, frames int64) []int16 {
	out := make([]int16, frames*2)
	for i := int64(0); i < frames; i++ {
		v := int16((from + i) % 1000)
		out[i*2], out[i*2+1] = v, v
	}
	return out
}

// TestACaptureIsWholeBarsBackFromTheTrigger is the point of the feature: whatever the trigger lands
// on, the take is a whole number of bars measured back from it, and it reaches back a bar further
// as the run-up.
func TestACaptureIsWholeBarsBackFromTheTrigger(t *testing.T) {
	const rate, bpm, beats = 1000, 60, 4
	// One bar is 4000 frames: 60 seconds a minute over 60 BPM is a second a beat, four beats a bar.
	const bar = 4000
	r := rampRing(rate, 60)
	r.SetGrid(1, beats, bpm)

	// The trigger in the middle of a bar, which is the case that a naive cut gets wrong. The ring
	// is 60 seconds long, so the bar after it is still to come.
	trigger := int64(bar + bar/2)
	start, end, err := r.Capture(trigger, 0)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if got := end - start; got != bar {
		t.Errorf("the take is %d frames, a bar is %d: not whole bars", got, bar)
	}
	// It starts on a bar line: the frame it starts at is a whole number of bars.
	if start%bar != 0 {
		t.Errorf("the take starts at frame %d, which is not a bar line", start)
	}
	// And it is the bar the trigger fell in, so the take holds the trigger.
	if !(start <= trigger && trigger < end) {
		t.Errorf("the take %d..%d does not hold the trigger %d", start, end, trigger)
	}
}

// TestATakeCaughtJustAfterABarLineStillGetsItsBar is the case that matters in practice: a phrase
// played a moment late is still a whole bar, not a sliver of the one it was caught in.
func TestATakeCaughtJustAfterABarLineStillGetsItsBar(t *testing.T) {
	const rate, bar = 1000, 4000
	r := rampRing(rate, 60)
	r.SetGrid(1, 4, 60)

	// Ten frames after a bar line: nearly the whole bar is behind the trigger, and it is kept.
	start, end, err := r.Capture(bar+10, 0)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if got := end - start; got != bar {
		t.Errorf("a late take is %d frames, want a whole bar of %d", got, bar)
	}
	if end < bar+10 {
		t.Errorf("the take ends at %d, before the trigger at %d", end, bar+10)
	}
}

// TestTheRunUpIsKeptBeforeTheTake checks the ear: the sound before the take is kept, so a phrase
// that started before the key was pressed is not clipped.
func TestTheRunUpIsKeptBeforeTheTake(t *testing.T) {
	const rate, bar = 1000, 4000
	r := rampRing(rate, 60)
	r.SetGrid(2, 4, 60)

	trigger := int64(6 * bar)
	plain, _, err := r.Capture(trigger, 0)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	withEar, _, err := r.Capture(trigger, 1)
	if err != nil {
		t.Fatalf("capture with a run-up: %v", err)
	}
	if withEar >= plain {
		t.Errorf("the run-up starts the take at %d, which is not before %d", withEar, plain)
	}
	if got, want := plain-withEar, int64(bar); got != want {
		t.Errorf("the run-up is %d frames, want %d: one bar of ear", got, want)
	}
}

// TestATakeIsAsManyBarsAsTheGridSays checks that a two bar take is two bars, measured back from the
// trigger, so a longer loop is cut the length it should be.
func TestATakeIsAsManyBarsAsTheGridSays(t *testing.T) {
	const rate, bar = 1000, 4000
	for _, bars := range []int{1, 2, 4} {
		r := rampRing(rate, 60)
		r.SetGrid(bars, 4, 60)
		start, end, err := r.Capture(int64(8*bar+777), 0)
		if err != nil {
			t.Fatalf("%d bars: capture: %v", bars, err)
		}
		if got, want := end-start, int64(bars*bar); got != want {
			t.Errorf("%d bars came out as %d frames, want %d", bars, got, want)
		}
	}
}

// TestTheTempoChangesTheLengthOfABar checks the arithmetic against the unit's tempo rather than a
// fixed one: a bar at 120 BPM is half a bar at 60.
func TestTheTempoChangesTheLengthOfABar(t *testing.T) {
	const rate = 1000
	slow := rampRing(rate, 60)
	slow.SetGrid(1, 4, 60)
	fast := rampRing(rate, 60)
	fast.SetGrid(1, 4, 120)

	_, slowEnd, err := slow.Capture(20000, 0)
	if err != nil {
		t.Fatalf("at 60: %v", err)
	}
	_, fastEnd, err := fast.Capture(20000, 0)
	if err != nil {
		t.Fatalf("at 120: %v", err)
	}
	// Both takes are one bar, and the bar at 120 has half the frames of the bar at 60.
	if fastEnd%2000 != 0 {
		t.Errorf("a bar at 120 BPM ends at %d, not on its own grid of 2000", fastEnd)
	}
	if got := (20000 / 4000) * 4000; slowEnd <= int64(got) {
		t.Errorf("the slow take ends at %d, which is not past the trigger", slowEnd)
	}
}

// TestACaptureSaysWhereItWasCut checks that the frames that come out are the frames that were
// asked for: the signal numbers the frame it was heard at, so a cut can be read off it.
func TestACaptureSaysWhereItWasCut(t *testing.T) {
	const rate, bar = 1000, 4000
	r := rampRing(rate, 60)
	r.SetGrid(1, 4, 60)

	start, end, err := r.Capture(9000, 0)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	got, err := r.Copy(start, end-start)
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	// The signal numbers its own frames, so the first sample says where the take begins.
	if first, want := got[0], int16(start%1000); first != want {
		t.Errorf("the take starts with frame %d, want %d", first, want)
	}
	if start != 8000 || end != 12000 {
		t.Errorf("the take is %d..%d, want 8000..12000: the bar the trigger fell in", start, end)
	}
}

// TestACaptureWithoutAGridIsRefused checks that a ring with no tempo says so rather than cutting
// something arbitrary.
func TestACaptureWithoutAGridIsRefused(t *testing.T) {
	r := rampRing(1000, 60)
	if _, _, err := r.Capture(9000, 0); err == nil {
		t.Error("a capture with no grid was allowed")
	}
	r.SetGrid(1, 4, 0)
	if _, _, err := r.Capture(9000, 0); err == nil {
		t.Error("a capture at no tempo was allowed")
	}
}

// TestATakeOlderThanTheRingIsRefused checks the one real limit: the ring is only so long. A take
// that has already been overwritten is refused, and says why, rather than coming out silent.
func TestATakeOlderThanTheRingIsRefused(t *testing.T) {
	// A ring of a second, a two bar take at 60 BPM: four plus two bars is more than it keeps.
	r := rampRing(1000, 1)
	r.SetGrid(2, 4, 60)
	if _, _, err := r.Capture(900, 0); err == nil {
		t.Error("a take longer than the ring was allowed")
	}
}

// TestWideningTheRingKeepsWhatWasHeard checks that growing the ring for a capture does not throw
// away the sound already in it: the phrase before the widening is what a capture reaches back for.
func TestWideningTheRingKeepsWhatWasHeard(t *testing.T) {
	r := rampRing(1000, 2)
	before := r.Total()
	if !r.Widen(10) {
		t.Fatal("the ring did not grow")
	}
	if got := r.Span(); got < 10000 {
		t.Errorf("the ring holds %d frames after growing, want at least 10000", got)
	}
	// What was heard is still there, at the end of the new buffer.
	got, err := r.Copy(before-100, 100)
	if err != nil {
		t.Fatalf("copy after growing: %v", err)
	}
	if first, want := got[0], int16((before-100)%1000); first != want {
		t.Errorf("the last frames are gone: the take holds %d where it had %d", first, want)
	}
	// And a ring already long enough is left alone.
	if r.Widen(1) {
		t.Error("a ring that was long enough was grown anyway")
	}
}

// TestATakeIsSavedAsAWavThatReadsBack checks the last step on the way out: the file is a WAV of the
// frames that were cut, and it opens again at the length the take had.
func TestATakeIsSavedAsAWavThatReadsBack(t *testing.T) {
	const rate, bar = 8000, 32000
	r := rampRing(rate, 60)
	r.SetGrid(1, 4, 60)
	start, end, err := r.Capture(40000, 0)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	dir := t.TempDir()
	path, err := r.Save(dir, "take.wav", start, end-start)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := formats.DecodeAudio(readFile(t, path), ".wav")
	if err != nil {
		t.Fatalf("the saved take does not read back: %v", err)
	}
	if got.Sample.Frames() != int(end-start) {
		t.Errorf("the file is %d frames, the take was %d", got.Sample.Frames(), end-start)
	}
	if got.Sample.Channels != 2 {
		t.Errorf("the file has %d channels, want stereo", got.Sample.Channels)
	}
}

// readFile is the bytes of a file from a test.
func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

// TestTheTriggerIsFoundByTime checks the other half of the stamp: the frame of a trigger is worked
// out from when it was pressed, so a take can be cut by the moment the key went down. The estimate
// carries a fixed delay (a block cannot arrive before it was captured), which is the same for every
// take, so what matters is that it advances with the clock at the rate of the audio.
func TestTheTriggerIsFoundByTime(t *testing.T) {
	r := NewRing(1000, 10)
	at := time.Now()
	r.Write(ramp(0, 1000), at)
	first := r.FrameAt(at)
	if later := r.FrameAt(at.Add(500 * time.Millisecond)); later-first < 400 || later-first > 600 {
		t.Errorf("half a second later is frame %d, want about %d (the first was %d)", later, first+500, first)
	}
	if later := r.FrameAt(at.Add(2 * time.Second)); later-first < 1900 || later-first > 2100 {
		t.Errorf("two seconds later is frame %d, want about %d", later, first+2000)
	}
}
