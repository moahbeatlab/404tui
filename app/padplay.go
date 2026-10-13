package app

import (
	"fmt"

	"github.com/moahbeatlab/404tui/formats"
)

// PlayRegion cuts a sample to the part a pad plays: start and end are byte offsets into the
// SMP file, as the pad block keeps them. A region that makes no sense plays the whole sample.
func PlayRegion(s formats.Sample, startBytes, endBytes uint32) formats.Sample {
	frames := uint32(s.Frames())
	frame := func(b uint32) uint32 {
		if b <= formats.HeaderLen {
			return 0
		}
		return (b - formats.HeaderLen) / (2 * uint32(max(s.Channels, 1)))
	}
	from, to := frame(startBytes), frame(endBytes)
	if to == 0 || to > frames {
		to = frames
	}
	if from >= to {
		return s
	}
	ch := max(s.Channels, 1)
	return formats.Sample{Channels: s.Channels, Samples: s.Samples[int(from)*ch : int(to)*ch]}
}

// PadAudio reads a pad's sample from the unit and cuts it to the part the pad plays, so that
// it can be played on this computer. It is the sample as stored: the unit's pitch, envelope,
// effects and level are not applied.
func PadAudio(u interface {
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
	return PlayRegion(sample, row.StartBytes, row.EndBytes), nil
}
