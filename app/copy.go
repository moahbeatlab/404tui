package app

import (
	"errors"
	"fmt"
	"sort"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// The unit has no copy command, so a pad is copied the way an export and a restore do it:
// read its sample and settings, write the sample onto the other pad, and apply the settings.

// CopyUnit is what copying pads needs of the unit.
type CopyUnit interface {
	Storage
	Pads
	Importer
}

// Copy is one pad copied onto another.
type Copy struct{ From, To proto.PadIndex }

// PlanCopy lays pads out from start on, keeping their positions relative to each other:
// copying A2, A3 to C5 puts them on C5, C6. A pad that would land on itself is left out. It
// fails when something would land past J16.
func PlanCopy(from []proto.PadIndex, start proto.PadIndex) ([]Copy, error) {
	if len(from) == 0 {
		return nil, errors.New("nothing to copy")
	}
	sources := append([]proto.PadIndex(nil), from...)
	sort.Slice(sources, func(i, j int) bool { return sources[i].Index() < sources[j].Index() })
	var copies []Copy
	for _, src := range sources {
		at := int(start.Index()) + int(src.Index()) - int(sources[0].Index())
		to, ok := proto.NewPadIndex(uint16(at))
		if !ok {
			return nil, fmt.Errorf("%d pads do not fit from %s on: %s would land past J16", len(sources), start, src)
		}
		if to != src {
			copies = append(copies, Copy{From: src, To: to})
		}
	}
	return copies, nil
}

// Occupied counts the copies whose destination holds a sample.
func Occupied(copies []Copy, hasSample func(proto.PadIndex) bool) int {
	n := 0
	for _, c := range copies {
		if hasSample(c.To) {
			n++
		}
	}
	return n
}

// CopyPads copies each pad's sample and settings onto its destination and returns how many
// were copied, with a line per failure. Every source is read before anything is written, so
// copies that overlap (shifting pads along a bank) do not overwrite what they still need.
func CopyPads(u CopyUnit, copies []Copy) (copied int, failed []string) {
	current, err := u.CurrentProject()
	if err != nil {
		return 0, []string{err.Error()}
	}
	project := current + 1

	type read struct {
		copy   Copy
		sample formats.Sample
		saved  *savedPad
	}
	var reads []read
	for _, c := range copies {
		saved, err := snapshot(u, project, c.From)
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", c.From, err))
			continue
		}
		if saved == nil {
			failed = append(failed, fmt.Sprintf("%s (no sample)", c.From))
			continue
		}
		smp, err := u.ReadFile(c.From.SamplePath(project))
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", c.From, err))
			continue
		}
		sample, err := formats.ParseSMP(smp)
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", c.From, err))
			continue
		}
		reads = append(reads, read{c, sample, saved})
	}
	for _, r := range reads {
		if _, err := PutSample(u, r.copy.To, r.sample, r.saved.Name); err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", r.copy.To, err))
			continue
		}
		if err := applySaved(u, r.copy.To, *r.saved); err != nil {
			failed = append(failed, fmt.Sprintf("%s settings (%v)", r.copy.To, err))
			continue
		}
		copied++
	}
	return copied, failed
}
