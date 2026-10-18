package app

import (
	"testing"
	"time"

	"github.com/moahbeatlab/404tui/formats"
)

func TestCutLoopEndsWhereTheClockSaysAndFades(t *testing.T) {
	// Ten seconds of a ramp: the value of a frame is its number / 1000 (so a frame can be told by its value).
	n := 10 * formats.SampleRate
	rec := formats.Sample{Channels: 1, Samples: make([]int16, n)}
	for i := range rec.Samples {
		rec.Samples[i] = int16(i%30000 + 1000)
	}
	stopped := time.Unix(100, 0)
	// The loop is 2 s long and ended 1 s before the stop; the sound is 10 ms late.
	end := stopped.Add(-time.Second)
	got, err := CutLoop(rec, stopped, end.Add(-2*time.Second), end, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if got.Frames() != 2*formats.SampleRate {
		t.Fatalf("%d frames, want %d", got.Frames(), 2*formats.SampleRate)
	}
	// The file's end is the sound of 10 ms before the stop, so the clock's end is 0.99 s from it.
	wantEnd := n - formats.SampleRate + 480
	if got.Samples[len(got.Samples)/2] != rec.Samples[wantEnd-formats.SampleRate] {
		t.Errorf("the middle of the loop is not where it should be")
	}
	if got.Samples[0] != 0 || got.Samples[len(got.Samples)-1] != 0 {
		t.Errorf("the ends were not faded: %d %d", got.Samples[0], got.Samples[len(got.Samples)-1])
	}
	// Too short a recording says so.
	if _, err := CutLoop(rec, stopped, end.Add(-20*time.Second), end, 0); err == nil {
		t.Error("a loop longer than the recording was cut")
	}
	if _, err := CutLoop(rec, stopped, stopped.Add(time.Second), stopped.Add(3*time.Second), 0); err == nil {
		t.Error("a loop that ends after the recording was cut")
	}
}
