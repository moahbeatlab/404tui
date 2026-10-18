package app

import (
	"errors"
	"fmt"
	"math"
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
	return FadeLoop(formats.Sample{Channels: ch, Samples: append([]int16(nil), rec.Samples[start*ch:end*ch]...)}), nil
}

// FadeLoop fades the start and the end of a loop over a millisecond, so the cut makes no click. It works in place.
func FadeLoop(out formats.Sample) formats.Sample {
	ch, frames := out.Channels, out.Frames()
	fade := min(loopFade, frames/2)
	for i := 0; i < fade; i++ {
		g := float64(i) / float64(fade)
		for c := 0; c < ch; c++ {
			out.Samples[i*ch+c] = int16(float64(out.Samples[i*ch+c]) * g)
			j := (frames-1-i)*ch + c
			out.Samples[j] = int16(float64(out.Samples[j]) * g)
		}
	}
	return out
}

// NormalizeTo is the peak, in dB below full scale, that a normalised loop gets.
const NormalizeTo = -1.0

// PeakDB is the highest sample of s in dB below full scale (0 is full scale); a silent sample is -Inf.
func PeakDB(s formats.Sample) float64 {
	peak := 0
	for _, v := range s.Samples {
		a := int(v)
		if a < 0 {
			a = -a
		}
		peak = max(peak, a)
	}
	if peak == 0 {
		return math.Inf(-1)
	}
	return 20 * math.Log10(float64(peak)/32768)
}

// ApplyGain makes s louder by db (or, when normalize is set, as loud as its peak can be, NormalizeTo): the samples
// that would go past full scale are held there. It returns the gain it applied in dB.
func ApplyGain(s formats.Sample, db float64, normalize bool) (formats.Sample, float64) {
	if normalize {
		if p := PeakDB(s); !math.IsInf(p, -1) {
			db = NormalizeTo - p
		}
	}
	if db == 0 {
		return s, 0
	}
	g := math.Pow(10, db/20)
	out := formats.Sample{Channels: s.Channels, Samples: make([]int16, len(s.Samples))}
	for i, v := range s.Samples {
		out.Samples[i] = int16(math.Max(-32768, math.Min(32767, math.Round(float64(v)*g))))
	}
	return out, db
}

// HitDelay finds how late a hit is in a loop that has one on its first beat: the milliseconds from the start to
// the first sample that is a third of the peak. A hit just before the start (late in the loop) comes out negative.
func HitDelay(s formats.Sample) (int, error) {
	peak := 0
	for _, v := range s.Samples {
		peak = max(peak, int(math.Abs(float64(v))))
	}
	if peak < 330 { // -40 dB
		return 0, errors.New("nothing was heard: is the pattern playing a hit on beat 1?")
	}
	ch := max(s.Channels, 1)
	for i := 0; i+ch <= len(s.Samples); i += ch {
		for c := 0; c < ch; c++ {
			if int(math.Abs(float64(s.Samples[i+c]))) >= peak/3 {
				frame := i / ch
				if frame > s.Frames()/2 {
					frame -= s.Frames()
				}
				return int(math.Round(float64(frame) / formats.SampleRate * 1000)), nil
			}
		}
	}
	return 0, errors.New("no hit found")
}
