package app

import (
	"errors"
	"fmt"
	"time"

	"github.com/moahbeatlab/404tui/formats"
)

// loopFade is how many frames the start and the end of a cut loop are faded over, so the cut makes no click.
const loopFade = 48

// LoopFrames is how many frames of 48 kHz audio a loop of the length d has.
func LoopFrames(d time.Duration) int {
	return int(d.Seconds()*formats.SampleRate + 0.5)
}

// CutLoop takes frames frames out of a recording of the unit's audio so that the loop ends where the moment end
// of the clock is: the file is known to end at stopped (when the recorder was told to stop), and the sound the
// computer hears is delay late. loopStart and loopEnd are the clock's time of the first and of the last tick of
// the loop. The result is as long as the loop, with a short fade at both ends.
func CutLoop(rec formats.Sample, stopped, loopStart, loopEnd time.Time, delay time.Duration) (formats.Sample, error) {
	frames := LoopFrames(loopEnd.Sub(loopStart))
	if frames <= 0 || rec.Channels < 1 {
		return formats.Sample{}, errors.New("the loop has no length")
	}
	// The end of the file is the end of the recorder's stop; what was said at loopEnd is that far from it.
	end := rec.Frames() - LoopFrames(stopped.Add(-delay).Sub(loopEnd))
	start := end - frames
	switch {
	case end > rec.Frames():
		return formats.Sample{}, errors.New("the recording ends before the loop does: the delay is too large")
	case start < 0:
		return formats.Sample{}, fmt.Errorf("the recording began %.2f s too late for the loop", float64(-start)/formats.SampleRate)
	}
	ch := rec.Channels
	out := formats.Sample{Channels: ch, Samples: append([]int16(nil), rec.Samples[start*ch:end*ch]...)}
	fade := min(loopFade, frames/2)
	for i := 0; i < fade; i++ {
		g := float64(i) / float64(fade)
		for c := 0; c < ch; c++ {
			out.Samples[i*ch+c] = int16(float64(out.Samples[i*ch+c]) * g)
			j := (frames-1-i)*ch + c
			out.Samples[j] = int16(float64(out.Samples[j]) * g)
		}
	}
	return out, nil
}
