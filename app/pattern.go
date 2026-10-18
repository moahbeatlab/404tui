package app

import (
	"errors"
	"fmt"
	"sort"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// GridStepsPerBar is how many columns one bar is quantized into for display: a 16th-note grid.
// The whole pattern's grid has this many columns per bar; the view pages through it one bar at a
// time.
const GridStepsPerBar = 16

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
	// Notes are the pattern's notes with their places in the file, for editing.
	Notes []formats.NoteRecord
}

// StepTick is the tick a column of the grid starts at.
func (d *PatternDetail) StepTick(col int) uint32 {
	return uint32(uint64(col) * uint64(d.Length) / uint64(max(d.Grid.Steps, 1)))
}

// StepTicks is how many ticks a column of the grid spans.
func (d *PatternDetail) StepTicks() uint32 {
	return d.Length / uint32(max(d.Grid.Steps, 1))
}

// Bars is how many bars long the pattern is, rounded up and at least 1.
func (d *PatternDetail) Bars() int {
	barTicks := uint32(max(d.Beats, 1)) * formats.PPQ
	bars := int((d.Length + barTicks - 1) / barTicks)
	return max(bars, 1)
}

// GridOffset is how far a tick is from the nearest step of the grid, in ticks: positive when it
// is late (after the step), negative when early, 0 on the grid. A note swung, shifted or
// humanized off its step shows here by how much.
func (d *PatternDetail) GridOffset(tick uint32) int {
	step := int(d.StepTicks())
	if step == 0 {
		return 0
	}
	off := int(tick) % step
	if off*2 > step {
		off -= step
	}
	return off
}

// TicksToMillis is how long a number of ticks lasts at the bank's tempo.
func (d *PatternDetail) TicksToMillis(ticks int) float64 {
	if d.BankTempo <= 0 {
		return 0
	}
	return float64(ticks) * 60000 / (d.BankTempo * formats.PPQ)
}

// StepsPerPage is how many columns one page (one bar) of the grid shows.
func (d *PatternDetail) StepsPerPage() int {
	return GridStepsPerBar
}

// CellNotes are the notes of a pad that fall in a column of the grid.
func (d *PatternDetail) CellNotes(pad proto.PadIndex, col int) []formats.NoteRecord {
	var out []formats.NoteRecord
	for _, n := range d.Notes {
		if n.Pad != pad || d.Length == 0 {
			continue
		}
		if min(int(uint64(n.Tick)*uint64(d.Grid.Steps)/uint64(d.Length)), d.Grid.Steps-1) == col {
			out = append(out, n)
		}
	}
	return out
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
	notes, err := formats.NoteRecords(b)
	if err != nil {
		return nil, fmt.Errorf("%s: unreadable pattern (%w)", slot, err)
	}
	detail := &PatternDetail{
		Notes:     notes,
		Slot:      slot,
		BankTempo: float64(settings.Bank(int(slot.Bank())).Tempo) / 100,
		Beats:     pattern.BeatsPerBar(),
		Length:    pattern.Length,
	}
	detail.Grid = BuildGrid(pattern.Events, pattern.Length, detail.Bars()*GridStepsPerBar)
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

// WriteGroove writes already-built pattern bytes into slot of the current project, replacing
// whatever the slot held. The bytes come from formats.GrooveTemplate.BuildGroove or
// formats.BlankPattern; this just resolves the slot's path and writes it.
func WriteGroove(u PatternFiles, slot proto.PadIndex, built []byte) error {
	current, err := u.CurrentProject()
	if err != nil {
		return err
	}
	if err := u.WriteFile(formats.PatternPath(current+1, slot), built); err != nil {
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

// CopyPatternKeeping puts the pattern in slot from into slot to, with only the notes of the
// pads keep says yes to: the others are taken out, the timing of the rest unchanged. It says
// how many notes were left out.
func CopyPatternKeeping(u PatternFiles, from, to proto.PadIndex, keep func(proto.PadIndex) bool) (dropped int, err error) {
	if from == to {
		return 0, fmt.Errorf("%s is the slot itself", from)
	}
	current, err := u.CurrentProject()
	if err != nil {
		return 0, err
	}
	data, err := u.ReadFile(formats.PatternPath(current+1, from))
	if err != nil {
		return 0, fmt.Errorf("reading pattern %s: %w", from, err)
	}
	notes, err := formats.NoteRecords(data)
	if err != nil {
		return 0, err
	}
	for _, n := range notes {
		if keep(n.Pad) {
			continue
		}
		if err := formats.RemoveNote(data, n.Offset); err != nil {
			return 0, err
		}
		dropped++
	}
	if dropped == len(notes) && len(notes) > 0 {
		return 0, fmt.Errorf("no note of %s would be left", from)
	}
	if err := u.WriteFile(formats.PatternPath(current+1, to), data); err != nil {
		return 0, fmt.Errorf("writing pattern %s: %w", to, err)
	}
	return dropped, nil
}

// EditPattern reads the pattern of a slot of the current project, lets edit change its bytes
// (it may return new ones), and writes it back. Nothing is written when edit fails. It returns
// the bytes the pattern had before, so that the change can be undone.
func EditPattern(u PatternFiles, slot proto.PadIndex, edit func(b []byte) ([]byte, error)) (before []byte, err error) {
	current, err := u.CurrentProject()
	if err != nil {
		return nil, err
	}
	path := formats.PatternPath(current+1, slot)
	before, err = u.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: read failed (%w)", slot, err)
	}
	after, err := edit(append([]byte(nil), before...))
	if err != nil {
		return nil, err
	}
	if err := u.WriteFile(path, after); err != nil {
		return nil, fmt.Errorf("%s: write failed (%w)", slot, err)
	}
	return before, nil
}

var errPadInPattern = errors.New("the pad has that note already")

// AddPadNote puts a note for a pad on beat 1 of a pattern, as long as the whole pattern. It says false (and writes
// nothing) when the pad has such a note already.
func AddPadNote(u PatternFiles, slot, pad proto.PadIndex) (bool, error) {
	_, err := EditPattern(u, slot, func(b []byte) ([]byte, error) {
		p, err := formats.ParsePattern(b)
		if err != nil {
			return nil, err
		}
		notes, err := formats.NoteRecords(b)
		if err != nil {
			return nil, err
		}
		for _, n := range notes {
			if n.Pad == pad && n.Tick == 0 && uint32(n.Length) >= p.Length {
				return nil, errPadInPattern
			}
		}
		return formats.InsertNote(b, 0, pad, 127, uint16(min(p.Length, 65535)))
	})
	if errors.Is(err, errPadInPattern) {
		return false, nil
	}
	return err == nil, err
}

// WithRowsFor adds an empty row to the grid for each of pads that has none, in pad order, so
// that a pad keeps its row when its last note is taken out, and a pad with no note yet can be
// given one.
func (d *PatternDetail) WithRowsFor(pads []proto.PadIndex) {
	have := map[proto.PadIndex]bool{}
	for _, r := range d.Grid.Rows {
		have[r.Pad] = true
	}
	for _, p := range pads {
		if !have[p] {
			d.Grid.Rows = append(d.Grid.Rows, GridRow{Pad: p, Hits: make([]bool, d.Grid.Steps)})
			have[p] = true
		}
	}
	sort.Slice(d.Grid.Rows, func(i, j int) bool { return d.Grid.Rows[i].Pad.Index() < d.Grid.Rows[j].Pad.Index() })
}

// NotesAt are the notes of a pad that sound at a column of the grid: the ones that start
// there, or else the ones that started before and still last.
func (d *PatternDetail) NotesAt(pad proto.PadIndex, col int) []formats.NoteRecord {
	if starting := d.CellNotes(pad, col); len(starting) > 0 {
		return starting
	}
	at := d.StepTick(col)
	var out []formats.NoteRecord
	for _, n := range d.Notes {
		if n.Pad == pad && n.Tick < at && at < n.Tick+uint32(n.Length) {
			out = append(out, n)
		}
	}
	return out
}

// Split reads how a step of a pad is divided, as the unit's SUBSTEP does: into n parts (2, 3 or
// 4; 1 when it is not) of which on says which sound. The notes of a split step are as long as a
// part and start on a part; a note that is not like that is a plain one.
func (d *PatternDetail) Split(pad proto.PadIndex, col int) (n int, on []bool) {
	notes := d.CellNotes(pad, col)
	if len(notes) == 0 {
		return 1, nil
	}
	step, start := d.StepTicks(), d.StepTick(col)
	n = 1
	if l := uint32(notes[0].Length); l > 0 && l < step && step/l >= 2 && step/l <= 4 {
		n = int(step / l)
	}
	on = make([]bool, n)
	for _, note := range notes {
		part := step / uint32(n)
		off := note.Tick - start
		idx := int(off / max(part, 1))
		if n > 1 && (uint32(note.Length) != part || off%part != 0) || idx >= n {
			if n == 1 {
				idx = 0
			} else {
				return 1, []bool{true}
			}
		}
		on[idx] = true
	}
	return n, on
}
