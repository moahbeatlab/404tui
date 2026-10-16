package app

import (
	"fmt"
	"math"

	"github.com/moahbeatlab/404tui/formats"
)

// StoredSample is the whole sample file of a pad as the unit keeps it, whatever the pad's start
// and end are.
func StoredSample(u interface {
	Storage
	CurrentProject() (uint8, error)
}, row PadRow) (formats.Sample, error) {
	current, err := u.CurrentProject()
	if err != nil {
		return formats.Sample{}, err
	}
	data, err := u.ReadFile(row.Pad.SamplePath(current + 1))
	if err != nil {
		return formats.Sample{}, fmt.Errorf("reading %s: %w", row.Pad, err)
	}
	sample, err := formats.ParseSMP(data)
	if err != nil {
		return formats.Sample{}, fmt.Errorf("%s: %w", row.Pad, err)
	}
	return sample, nil
}

// SampleCheck is what comparing a sample on the unit with the file it came from found.
type SampleCheck struct {
	// Want and Got are the audio of the file as 404tui converts it for the unit, and what the unit
	// holds.
	WantFrames, GotFrames     int
	WantChannels, GotChannels int
	// Identical is set when every sample is the same.
	Identical bool
	// Differing is how many samples of the part both have are not the same, MaxDiff the largest
	// difference (of 32768), ErrDB the level of the difference under the sample's own, in dB.
	Differing, Compared int
	MaxDiff             int
	ErrDB               float64
	// WantPeak and GotPeak are the loudest samples of each.
	WantPeak, GotPeak int
}

// CompareSamples compares the audio of a file, converted as for the unit, with the sample the unit
// holds.
func CompareSamples(want, got formats.Sample) SampleCheck {
	c := SampleCheck{
		WantFrames: want.Frames(), GotFrames: got.Frames(),
		WantChannels: want.Channels, GotChannels: got.Channels,
	}
	peak := func(s formats.Sample) int {
		p := 0
		for _, v := range s.Samples {
			p = max(p, int(math.Abs(float64(v))))
		}
		return p
	}
	c.WantPeak, c.GotPeak = peak(want), peak(got)
	n := min(len(want.Samples), len(got.Samples))
	c.Compared = n
	var sig, diff float64
	for i := 0; i < n; i++ {
		d := int(want.Samples[i]) - int(got.Samples[i])
		if d != 0 {
			c.Differing++
			c.MaxDiff = max(c.MaxDiff, int(math.Abs(float64(d))))
		}
		sig += float64(want.Samples[i]) * float64(want.Samples[i])
		diff += float64(d) * float64(d)
	}
	if diff > 0 && sig > 0 {
		c.ErrDB = 10 * math.Log10(diff/sig)
	}
	c.Identical = c.Differing == 0 && len(want.Samples) == len(got.Samples) && want.Channels == got.Channels
	return c
}

// Verdict says in a line what the check found.
func (c SampleCheck) Verdict() string {
	switch {
	case c.Identical:
		return fmt.Sprintf("identical: %d frames, %d channel(s), peak %d", c.GotFrames, c.GotChannels, c.GotPeak)
	case c.WantChannels != c.GotChannels:
		return fmt.Sprintf("different: the file has %d channel(s), the pad %d", c.WantChannels, c.GotChannels)
	case c.WantFrames != c.GotFrames:
		return fmt.Sprintf("different length: the file is %d frames (%.3f s), the pad %d (%.3f s)",
			c.WantFrames, float64(c.WantFrames)/formats.SampleRate, c.GotFrames, float64(c.GotFrames)/formats.SampleRate)
	}
	return fmt.Sprintf("%d of %d samples differ, by %d at most, %.0f dB under the sound; peaks %d and %d",
		c.Differing, c.Compared, c.MaxDiff, c.ErrDB, c.WantPeak, c.GotPeak)
}
