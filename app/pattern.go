package app

import (
	"fmt"
	"sort"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// GridSteps is how many columns a pattern's length is quantized into for display.
const GridSteps = 16

// PatternFiles is what reading and rewriting a pattern needs of the unit.
type PatternFiles interface {
	Storage
	CurrentProject() (uint8, error)
	ProjectSettings(project uint8) (*proto.ProjectSettings, error)
}

// GridRow is one pad's row in the step grid.
type GridRow struct {
	Pad  proto.PadIndex
	Hits []bool
}

// Grid is a pattern laid out as steps: evenly spaced columns across the pattern's length,
// one row per pad with at least one note, in pad order.
type Grid struct {
	Steps int
	Rows  []GridRow
}

// BuildGrid lays out note events as a step grid.
func BuildGrid(events []formats.Event, length uint32, steps int) Grid {
	steps = max(steps, 1)
	byPad := map[proto.PadIndex][]bool{}
	for _, e := range events {
		if e.Control {
			continue
		}
		hits, ok := byPad[e.Pad]
		if !ok {
			hits = make([]bool, steps)
			byPad[e.Pad] = hits
		}
		col := 0
		if length > 0 {
			col = min(int(uint64(e.Tick)*uint64(steps)/uint64(length)), steps-1)
		}
		hits[col] = true
	}
	grid := Grid{Steps: steps}
	for pad, hits := range byPad {
		grid.Rows = append(grid.Rows, GridRow{Pad: pad, Hits: hits})
	}
	sort.Slice(grid.Rows, func(i, j int) bool { return grid.Rows[i].Pad.Index() < grid.Rows[j].Pad.Index() })
	return grid
}

// CommonBank is the single bank every one of pads belongs to; ok is false when they span
// more than one (or there are none), since retargeting only makes sense within one bank.
func CommonBank(pads []proto.PadIndex) (bank int, ok bool) {
	if len(pads) == 0 {
		return 0, false
	}
	for _, p := range pads {
		if p.Bank() != pads[0].Bank() {
			return 0, false
		}
	}
	return int(pads[0].Bank()), true
}

// PatternDetail is a pattern opened for viewing.
type PatternDetail struct {
	Slot proto.PadIndex
	// BankTempo is the tempo of the slot's bank, in BPM.
	BankTempo float64
	Beats     int
	Length    uint32
	Grid      Grid
	PadsUsed  []proto.PadIndex
	// SourceBank is the bank every pad it plays is in; HasSourceBank is false when they
	// span several.
	SourceBank    int
	HasSourceBank bool
}

// OpenPattern reads a pattern slot of the current project.
func OpenPattern(u PatternFiles, slot proto.PadIndex) (*PatternDetail, error) {
	current, err := u.CurrentProject()
	if err != nil {
		return nil, err
	}
	b, err := u.ReadFile(formats.PatternPath(current+1, slot))
	if err != nil {
		return nil, fmt.Errorf("%s: no pattern (%w)", slot, err)
	}
	pattern, err := formats.ParsePattern(b)
	if err != nil {
		return nil, fmt.Errorf("%s: unreadable pattern (%w)", slot, err)
	}
	settings, err := u.ProjectSettings(current)
	if err != nil {
		return nil, err
	}
	detail := &PatternDetail{
		Slot:      slot,
		BankTempo: float64(settings.Bank(int(slot.Bank())).Tempo) / 100,
		Beats:     pattern.BeatsPerBar(),
		Length:    pattern.Length,
		Grid:      BuildGrid(pattern.Events, pattern.Length, GridSteps),
	}
	for _, row := range detail.Grid.Rows {
		detail.PadsUsed = append(detail.PadsUsed, row.Pad)
	}
	detail.SourceBank, detail.HasSourceBank = CommonBank(detail.PadsUsed)
	return detail, nil
}

// RetargetPattern rewrites a pattern's notes from one bank to another in place on the
// unit and returns how many notes moved.
func RetargetPattern(u PatternFiles, slot proto.PadIndex, from, to int) (int, error) {
	if from == to {
		return 0, fmt.Errorf("target bank is the same as the source; nothing to do")
	}
	current, err := u.CurrentProject()
	if err != nil {
		return 0, err
	}
	path := formats.PatternPath(current+1, slot)
	b, err := u.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("%s: read failed (%w)", slot, err)
	}
	changed := formats.RetargetBank(b, uint16(from), uint16(to))
	if changed == 0 {
		return 0, fmt.Errorf("no notes were retargeted")
	}
	if err := u.WriteFile(path, b); err != nil {
		return 0, fmt.Errorf("write failed: %w", err)
	}
	return changed, nil
}

// StartBars is the length of the pattern a restore can make.
const StartBars = 2

// WriteStartPattern writes a new pattern into slot of the current project: bars bars in
// which every one of pads starts on the first beat and is held to the end. Whatever the
// slot held is replaced.
func WriteStartPattern(u PatternFiles, slot proto.PadIndex, pads []proto.PadIndex, bars int) error {
	b, err := formats.StartPattern(pads, bars)
	if err != nil {
		return err
	}
	current, err := u.CurrentProject()
	if err != nil {
		return err
	}
	if err := u.WriteFile(formats.PatternPath(current+1, slot), b); err != nil {
		return fmt.Errorf("pattern %s: write failed: %w", slot, err)
	}
	return nil
}

// StepGlyphs draws one row of the grid: a beat marker every 4 steps.
func StepGlyphs(hits []bool, hit, rest string) string {
	out := ""
	for i, h := range hits {
		if i > 0 && i%4 == 0 {
			out += " "
		}
		if h {
			out += hit
		} else {
			out += rest
		}
	}
	return out
}

// DeletePatterns removes the patterns in slots from the current project. It goes on past
// one that fails and returns a line for each.
func DeletePatterns(u PatternFiles, slots []proto.PadIndex) (deleted int, failed []string) {
	current, err := u.CurrentProject()
	if err != nil {
		return 0, []string{err.Error()}
	}
	for _, slot := range slots {
		if err := u.Unlink(formats.PatternPath(current+1, slot)); err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", slot, err))
			continue
		}
		deleted++
	}
	return deleted, failed
}

// CopyPattern puts the pattern in slot from into slot to, replacing what is there. With
// move, the pattern is gone from its old slot afterwards.
func CopyPattern(u PatternFiles, from, to proto.PadIndex, move bool) error {
	if from == to {
		return fmt.Errorf("%s is the slot itself", from)
	}
	current, err := u.CurrentProject()
	if err != nil {
		return err
	}
	data, err := u.ReadFile(formats.PatternPath(current+1, from))
	if err != nil {
		return fmt.Errorf("reading pattern %s: %w", from, err)
	}
	if err := u.WriteFile(formats.PatternPath(current+1, to), data); err != nil {
		return fmt.Errorf("writing pattern %s: %w", to, err)
	}
	if move {
		if err := u.Unlink(formats.PatternPath(current+1, from)); err != nil {
			return fmt.Errorf("copied to %s, but removing %s failed: %w", to, from, err)
		}
	}
	return nil
}
