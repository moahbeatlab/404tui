package app

import (
	"fmt"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// PadFacts is what the SMP file of a pad says about its sample.
type PadFacts struct {
	Channels int
	Frames   uint32
	// Seconds is the whole sample's length.
	Seconds float64
	// File is the sample's file name on the card, such as BANK1-05.SMP.
	File string
	// StartFrame, EndFrame and LoopFrame are the play region and loop start in frames.
	StartFrame, EndFrame, LoopFrame uint32
}

// ReadPadFacts reads the header of a pad's sample file and works out its length and where
// the play region and loop start fall. It reads 512 bytes, not the sample.
func ReadPadFacts(u interface {
	Storage
	CurrentProject() (uint8, error)
}, row PadRow) (PadFacts, error) {
	current, err := u.CurrentProject()
	if err != nil {
		return PadFacts{}, err
	}
	path := row.Pad.SamplePath(current + 1)
	head, exists, err := u.ReadPrefix(path, formats.HeaderLen)
	if err != nil {
		return PadFacts{}, err
	}
	if !exists {
		return PadFacts{}, fmt.Errorf("%s: sample file is missing", row.Pad)
	}
	info, err := formats.ParseHeader(head)
	if err != nil {
		return PadFacts{}, fmt.Errorf("%s: %w", row.Pad, err)
	}
	return PadFacts{
		Channels:   info.Channels,
		Frames:     info.Frames(),
		Seconds:    float64(info.Frames()) / formats.SampleRate,
		File:       path[len(path)-len("BANK1-05.SMP"):],
		StartFrame: info.FrameAt(row.StartBytes),
		EndFrame:   info.FrameAt(row.EndBytes),
		LoopFrame:  info.FrameAt(row.LoopBytes),
	}, nil
}

// PatternUsage tells which pattern slots play each pad of the current project. It asks the
// unit about all 160 slots, so it is done once and kept.
func PatternUsage(p Patterns) (map[proto.PadIndex][]proto.PadIndex, error) {
	usage := map[proto.PadIndex][]proto.PadIndex{}
	for _, slot := range proto.AllPads() {
		exists, err := p.PatternExists(slot)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		pads, err := p.PatternPads(slot)
		if err != nil {
			return nil, err
		}
		for _, pad := range pads {
			usage[pad] = append(usage[pad], slot)
		}
	}
	return usage, nil
}
