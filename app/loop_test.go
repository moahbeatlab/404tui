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

func TestApplyGainAndNormalize(t *testing.T) {
	s := formats.Sample{Channels: 1, Samples: []int16{0, 8192, -16384, 30000}}
	if got := PeakDB(s); got > -0.7 || got < -0.9 {
		t.Errorf("peak %.2f dB, want about -0.77", got)
	}
	quiet := formats.Sample{Channels: 1, Samples: []int16{0, 1000, -2000}}
	loud, gain := ApplyGain(quiet, 0, true)
	if p := PeakDB(loud); p < NormalizeTo-0.05 || p > NormalizeTo+0.05 || gain <= 0 {
		t.Errorf("normalised to %.2f dB with %.2f dB of gain", p, gain)
	}
	// Past full scale is held there, not wrapped.
	clipped, _ := ApplyGain(s, 12, false)
	if clipped.Samples[3] != 32767 || clipped.Samples[2] != -32768 {
		t.Errorf("clipped %v", clipped.Samples)
	}
	same, g := ApplyGain(s, 0, false)
	if g != 0 || &same.Samples[0] != &s.Samples[0] {
		t.Error("no gain must leave the sample alone")
	}
	if silent, g := ApplyGain(formats.Sample{Channels: 1, Samples: []int16{0, 0}}, 0, true); g != 0 || PeakDB(silent) < 0 == false && false {
		t.Error("silence has nothing to normalise")
	}
}

func TestHitDelayFindsTheHitOnTheFirstBeat(t *testing.T) {
	// A bar at 48 kHz with a hit 12 ms after the start, and one that is 5 ms before the start (late in the loop).
	mk := func(at int) formats.Sample {
		s := formats.Sample{Channels: 2, Samples: make([]int16, 2*48000)}
		s.Samples[2*at], s.Samples[2*at+1] = 20000, 20000
		return s
	}
	if ms, err := HitDelay(mk(576)); err != nil || ms != 12 {
		t.Errorf("hit 12 ms late: %d %v", ms, err)
	}
	if ms, err := HitDelay(mk(48000 - 240)); err != nil || ms != -5 {
		t.Errorf("hit 5 ms early: %d %v", ms, err)
	}
	if _, err := HitDelay(formats.Sample{Channels: 1, Samples: make([]int16, 100)}); err == nil {
		t.Error("silence has no hit")
	}
}
