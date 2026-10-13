// Package app holds what the tool does with a unit, apart from how it is shown: reading a
// project's state, renaming and deleting pads, listing patterns, browsing its storage.
//
// It asks for the unit through the small interfaces below, which *device.Device satisfies.
package app

import (
	"fmt"
	"time"

	"github.com/moahbeatlab/404tui/device"
	"github.com/moahbeatlab/404tui/proto"
)

// Projects is the unit's project slots and overall state.
type Projects interface {
	PortName() string
	Status() (*device.Status, error)
	Activity() (device.Activity, error)
	ProjectNames() ([]string, error)
	ProjectSettings(project uint8) (*proto.ProjectSettings, error)
	SelectProject(project uint8) error
	SetProjectName(project uint8, name string) error
	FreeKb() (uint32, error)
}

// Pads is the pads of the current project.
type Pads interface {
	PadBlocks() ([]*proto.PadBlock, error)
	PadBlock(pad proto.PadIndex) (*proto.PadBlock, error)
	SetSampleName(pad proto.PadIndex, name string) error
	PadOp(op proto.PadOp, pad proto.PadIndex) error
	Preview(pad proto.PadIndex, duration time.Duration) error
	Previewer
	SetParam(id byte, target proto.Target, value int32) error
}

// Patterns is the pattern slots of the current project.
type Patterns interface {
	PatternExists(slot proto.PadIndex) (bool, error)
	PatternPads(pattern proto.PadIndex) ([]proto.PadIndex, error)
}

// Events is what the unit reports on its own.
type Events interface {
	TakeEvents() []proto.Message
}

// Importer puts samples and settings onto pads.
type Importer interface {
	CurrentProject() (uint8, error)
	ImportSMP(project uint8, pad proto.PadIndex, smp []byte, name string) error
	SetChopPoints(pad proto.PadIndex, points []uint32) error
}

// Mover moves samples between pads.
type Mover interface {
	MoveSample(from, to proto.PadIndex, mode proto.MoveMode) error
}

// Waveforms reads waveform peaks of the samples on the pads.
type Waveforms interface {
	Peaks(pad proto.PadIndex, channel uint8, startFrame, points, samplesPerPoint uint32) ([]proto.Peak, error)
}

// Unit is everything the tool uses of a connected SP-404MKII.
type Unit interface {
	Projects
	Pads
	Patterns
	Storage
	Events
	Importer
	Mover
	Waveforms
	PatternFiles
	Activities
	Restorer
}

// Activities is what the unit is doing right now.
type Activities interface {
	Activity() (device.Activity, error)
}

// Parameter ids shown in the pad table.
const (
	paramLevel        = 0x69
	paramGate         = 0x6A
	paramLoop         = 0x6B
	paramBPMSync      = 0x6E
	paramBPM          = 0x6F
	paramSelectedBank = 0x00
)

// PreviewLength is how long a pad plays when previewed.
const PreviewLength = time.Second

// PadRow is one pad as the pad table shows it.
type PadRow struct {
	Pad       proto.PadIndex
	HasSample bool
	Name      string
	Level     int32
	BPM       float64
	// Size is the sample file's size in bytes.
	Size    uint32
	Gate    bool
	Loop    bool
	BPMSync bool
	// Chops is how many chop points the pad has.
	Chops int
	// Params are the pad's settings by name (see proto.Params), as the unit stores them.
	Params map[string]int32
	// StartBytes, EndBytes and LoopBytes are the play region and loop start as byte offsets
	// into the SMP file; ChopFrames are the chop points in sample frames.
	StartBytes, EndBytes, LoopBytes uint32
	ChopFrames                      []uint32
}

// Snapshot is the state of the unit and its current project at one moment.
type Snapshot struct {
	Port string
	// Project is the current project, 0-based.
	Project     uint8
	ProjectName string
	FreeKb      uint32
	Selected    proto.PadIndex
	HasSelected bool
	// Projects are the names of the 16 slots, empty for unnamed ones.
	Projects []string
	// Pads are all 160 pads in order, A1 to J16.
	Pads []PadRow
	// Banks are the tempo and volume of each bank. While the project's tempo is in use
	// (UsesProjectTempo) the bank tempos have no effect.
	Banks            [proto.Banks]BankInfo
	UsesProjectTempo bool
	ProjectTempo     float64
}

// Bank returns the 16 pads of a bank, 0..=9.
func (s *Snapshot) Bank(bank int) []PadRow {
	return s.Pads[bank*proto.PadsPerBank : (bank+1)*proto.PadsPerBank]
}

// Load reads the unit's state and every pad of the current project.
func Load(u interface {
	Projects
	Pads
}) (*Snapshot, error) {
	st, err := u.Status()
	if err != nil {
		return nil, err
	}
	free, err := u.FreeKb()
	if err != nil {
		return nil, err
	}
	names, err := u.ProjectNames()
	if err != nil {
		return nil, err
	}
	blocks, err := u.PadBlocks()
	if err != nil {
		return nil, err
	}
	snap := &Snapshot{
		Port:        u.PortName(),
		Project:     st.Project,
		ProjectName: st.Settings.Name(),
		FreeKb:      free,
		Selected:    st.Selected,
		HasSelected: st.HasSelected,
		Projects:    names,
		Pads:        make([]PadRow, 0, len(blocks)),
	}
	for b := 0; b < proto.Banks; b++ {
		bs := st.Settings.Bank(b)
		snap.Banks[b] = BankInfo{Tempo: float64(bs.Tempo) / 100, Volume: int(bs.Volume)}
	}
	snap.UsesProjectTempo = st.Settings.UsesProjectTempo()
	snap.ProjectTempo = float64(st.Settings.ProjectTempo()) / 100
	for _, b := range blocks {
		level, _ := b.Param(paramLevel)
		bpm, _ := b.Param(paramBPM)
		flag := func(id byte) bool {
			v, _ := b.Param(id)
			return v != 0
		}
		snap.Pads = append(snap.Pads, PadRow{
			Pad:        b.Pad,
			HasSample:  b.HasSample(),
			Name:       b.Name(),
			Level:      level,
			BPM:        float64(bpm) / 100,
			Size:       b.FileSize(),
			Gate:       flag(paramGate),
			Loop:       flag(paramLoop),
			BPMSync:    flag(paramBPMSync),
			Chops:      chops(b),
			Params:     params(b),
			ChopFrames: chopFrames(b),
		})
		row := &snap.Pads[len(snap.Pads)-1]
		row.StartBytes, row.EndBytes = b.StartEndBytes()
		row.LoopBytes = b.LoopStartBytes()
	}
	return snap, nil
}

// ProjectChanged reports whether the unit announced a change to its project data
// (1A proj 00 00 00), as it does when a project is switched or edited on its own panel.
func ProjectChanged(events []proto.Message) bool {
	for i := range events {
		p := events[i].Payload()
		if events[i].Channel == proto.ChannelControl && len(p) > 2 && p[0] == proto.ReplyChanged {
			return true
		}
	}
	return false
}

// ShowBank makes the unit show a bank, 0..=9, as choosing it on the unit would.
func ShowBank(p Pads, bank int) error {
	return p.SetParam(paramSelectedBank, proto.TargetGlobal, int32(bank))
}

// DeletePads deletes the samples of pads and returns a line per failure.
func DeletePads(p Pads, pads []proto.PadIndex) (failed []string) {
	for _, pad := range pads {
		if err := p.PadOp(proto.PadOpDelete, pad); err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", pad, err))
		}
	}
	return failed
}

// BulkRename adds text before (or, with suffix, after) the name of every row that holds a
// sample; each keeps its own distinct name otherwise. It returns how many were renamed
// and a line per failure.
func BulkRename(p Pads, rows []PadRow, text string, suffix bool) (renamed int, failed []string) {
	for _, row := range rows {
		// An empty pad has no name to add to.
		if !row.HasSample {
			continue
		}
		name := text + row.Name
		if suffix {
			name = row.Name + text
		}
		if err := p.SetSampleName(row.Pad, name); err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", row.Pad, err))
			continue
		}
		renamed++
	}
	return renamed, failed
}

// PatternRow is one pattern slot.
type PatternRow struct {
	Slot   proto.PadIndex
	Exists bool
	// Pads are the pads the pattern plays.
	Pads []proto.PadIndex
}

// LoadPatterns reads the 16 pattern slots of a bank, 0..=9.
func LoadPatterns(p Patterns, bank int) ([]PatternRow, error) {
	rows := make([]PatternRow, 0, proto.PadsPerBank)
	for i := 0; i < proto.PadsPerBank; i++ {
		slot, _ := proto.PadFromBankPad(uint16(bank), uint16(i))
		row := PatternRow{Slot: slot}
		var err error
		if row.Exists, err = p.PatternExists(slot); err != nil {
			return nil, err
		}
		if row.Exists {
			if row.Pads, err = p.PatternPads(slot); err != nil {
				return nil, err
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func chops(b *proto.PadBlock) int {
	n := 0
	for _, c := range b.ChopPoints() {
		if c >= 0 {
			n++
		}
	}
	return n
}

// params reads every pad setting whose place in the pad block is known.
func params(b *proto.PadBlock) map[string]int32 {
	out := map[string]int32{}
	for _, p := range proto.Params {
		if p.Scope != proto.ScopePad {
			continue
		}
		if v, ok := b.Param(p.ID); ok {
			out[p.Name] = v
		}
	}
	return out
}

func chopFrames(b *proto.PadBlock) []uint32 {
	var out []uint32
	for _, c := range b.ChopPoints() {
		if c >= 0 {
			out = append(out, uint32(c))
		}
	}
	return out
}
