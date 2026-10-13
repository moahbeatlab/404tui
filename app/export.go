package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// Pad export and restore: each pad becomes a WAV named after it ("C05 Kick.wav", so a
// folder sorts in pad order) and a sidecar, samples.json, keeps every pad's settings.
// Restoring reads the sidecar and puts each sound back on its own pad with its settings,
// so a bank with gaps comes back with the same gaps. The contents are the format
// SparkyMK2's pad export uses; only the file name differs (it calls it sparkymk2.json).
const (
	Sidecar       = "samples.json"
	sidecarFormat = "sparkymk2-pads"
)

// markers are the parameters the pad block stores as byte offsets but the sidecar keeps
// in frames.
var markers = []string{"start", "end", "loop-top"}

type sidecar struct {
	Format  string     `json:"format"`
	Version int        `json:"version"`
	Project string     `json:"project"`
	Pads    []savedPad `json:"pads"`
}

type savedPad struct {
	// Pad is the pad the sound goes back to ("C05").
	Pad  string `json:"pad"`
	File string `json:"file"`
	Name string `json:"name"`
	// Params are pad parameters by name, markers in sample frames.
	Params     map[string]int32 `json:"params"`
	ChopPoints []uint32         `json:"chopPoints"`
}

// ExportUnit is what an export reads from the unit.
type ExportUnit interface {
	Storage
	Projects
	Pads
	Importer
}

// PadLabel is a pad as bank letter and two-digit number, so names sort in pad order.
func PadLabel(pad proto.PadIndex) string {
	return fmt.Sprintf("%c%02d", 'A'+pad.Bank(), pad.Pad()+1)
}

// padOf is the pad a label or file name starts with: "C05", "C05 Kick.wav".
func padOf(name string) (proto.PadIndex, bool) {
	if len(name) < 3 || (len(name) > 3 && name[3] != ' ') {
		return proto.PadIndex{}, false
	}
	bank := name[0] &^ 0x20
	if bank < 'A' || bank > 'J' || name[1] < '0' || name[1] > '9' || name[2] < '0' || name[2] > '9' {
		return proto.PadIndex{}, false
	}
	n := uint16(name[1]-'0')*10 + uint16(name[2]-'0')
	if n < 1 || n > 16 {
		return proto.PadIndex{}, false
	}
	return proto.PadFromBankPad(uint16(bank-'A'), n-1)
}

// SanitizeName keeps a name usable on the card's FAT filesystem and on the computer.
func SanitizeName(name string) string {
	clean := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < ' ' {
			return '_'
		}
		return r
	}, name)
	clean = strings.TrimRight(strings.TrimSpace(clean), ".")
	if clean == "" {
		return "Sample"
	}
	return clean
}

func exportFileName(pad proto.PadIndex, name string) string {
	return PadLabel(pad) + " " + SanitizeName(name) + ".wav"
}

// snapshot is a pad's settings as the sidecar keeps them; nil for an empty pad.
func snapshot(u interface {
	PadBlock(pad proto.PadIndex) (*proto.PadBlock, error)
	ReadPrefix(path string, n uint32) ([]byte, bool, error)
}, project uint8, pad proto.PadIndex) (*savedPad, error) {
	block, err := u.PadBlock(pad)
	if err != nil {
		return nil, err
	}
	if !block.HasSample() {
		return nil, nil
	}
	head, exists, err := u.ReadPrefix(pad.SamplePath(project), formats.HeaderLen)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("%s: sample file is missing", pad)
	}
	info, err := formats.ParseHeader(head)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", pad, err)
	}
	start, end := block.StartEndBytes()
	params := map[string]int32{}
	for _, p := range proto.Params {
		if p.Scope != proto.ScopePad || contains(markers, p.Name) {
			continue
		}
		if v, ok := block.Param(p.ID); ok {
			params[p.Name] = v
		}
	}
	params["start"] = int32(info.FrameAt(start))
	params["end"] = int32(info.FrameAt(end))
	params["loop-top"] = int32(info.FrameAt(block.LoopStartBytes()))

	chops := []uint32{}
	for _, c := range block.ChopPoints() {
		if c >= 0 {
			chops = append(chops, uint32(c))
		}
	}
	name := block.Name()
	return &savedPad{Pad: PadLabel(pad), File: exportFileName(pad, name), Name: name, Params: params, ChopPoints: chops}, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// ExportPads exports pads as WAV files into dir (a folder on the computer, the unit's
// storage or its SD card) with a samples.json sidecar recording each pad's settings so
// a restore can put everything back. Empty pads are skipped. It returns a one-line summary.
func ExportPads(u ExportUnit, pads []proto.PadIndex, dir string, progress func(done, total int)) (string, error) {
	dir = strings.TrimRight(dir, "/")
	if err := MkdirAll(u, dir); err != nil {
		return "", fmt.Errorf("creating %s: %w", dir, err)
	}
	current, err := u.CurrentProject()
	if err != nil {
		return "", err
	}
	project := current + 1
	names, err := u.ProjectNames()
	if err != nil {
		return "", err
	}
	var saved []savedPad
	empty := 0
	for i, pad := range pads {
		if progress != nil {
			progress(i, len(pads))
		}
		record, err := snapshot(u, project, pad)
		if err != nil {
			return "", err
		}
		if record == nil {
			empty++
			continue
		}
		smp, err := u.ReadFile(pad.SamplePath(project))
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", pad, err)
		}
		sample, err := formats.ParseSMP(smp)
		if err != nil {
			return "", fmt.Errorf("%s: %w", pad, err)
		}
		if err := u.WriteFile(dir+"/"+record.File, sample.WAV()); err != nil {
			return "", fmt.Errorf("writing %s: %w", record.File, err)
		}
		saved = append(saved, *record)
	}
	if len(saved) == 0 {
		return "", errors.New("no pads with a sample to export")
	}
	sheet := sidecar{Format: sidecarFormat, Version: 1, Project: names[current], Pads: saved}
	data, err := json.MarshalIndent(sheet, "", "  ")
	if err != nil {
		return "", err
	}
	if err := u.WriteFile(dir+"/"+Sidecar, data); err != nil {
		return "", fmt.Errorf("writing %s: %w", Sidecar, err)
	}
	return fmt.Sprintf("%d pads exported, %d empty, to %s", len(saved), empty, dir), nil
}

func parseSidecar(b []byte) (sidecar, error) {
	var sheet sidecar
	if err := json.Unmarshal(b, &sheet); err != nil {
		return sheet, fmt.Errorf("%s is not readable: %w", Sidecar, err)
	}
	if sheet.Format != sidecarFormat {
		return sheet, fmt.Errorf("%s is not a pad export", Sidecar)
	}
	return sheet, nil
}

// RestoreUnit is what a restore needs of the unit.
type RestoreUnit interface {
	Storage
	Importer
	Pads
}

// RestorePad is one pad of an export: where it was exported from, its name and file.
type RestorePad struct {
	// From is the pad the sound was exported from.
	From  proto.PadIndex
	Name  string
	File  string
	saved savedPad
}

// PadRestore is an export read and ready to be put back, with the choice of where.
type PadRestore struct {
	Dir     string
	Project string
	Pads    []RestorePad
	// Banks are the banks (0..9) the pads were exported from, in order. When there is one,
	// the export can go to any bank; with several, each pad goes back where it was.
	Banks []int
}

// PlanPadRestore reads the sidecar of an export in dir. It changes nothing on the unit.
func PlanPadRestore(s Storage, dir string) (*PadRestore, error) {
	dir = strings.TrimRight(dir, "/")
	b, err := s.ReadFile(dir + "/" + Sidecar)
	if err != nil {
		return nil, fmt.Errorf("reading %s in %s: %w (choose the folder of a pad export)", Sidecar, dir, err)
	}
	sheet, err := parseSidecar(b)
	if err != nil {
		return nil, err
	}
	plan := &PadRestore{Dir: dir, Project: sheet.Project}
	seen := map[int]bool{}
	for _, saved := range sheet.Pads {
		pad, ok := padOf(saved.Pad)
		if !ok {
			return nil, fmt.Errorf("%q is not a pad", saved.Pad)
		}
		plan.Pads = append(plan.Pads, RestorePad{From: pad, Name: saved.Name, File: saved.File, saved: saved})
		if bank := int(pad.Bank()); !seen[bank] {
			seen[bank] = true
			plan.Banks = append(plan.Banks, bank)
		}
	}
	if len(plan.Pads) == 0 {
		return nil, fmt.Errorf("the export in %s lists no pads", dir)
	}
	slices.Sort(plan.Banks)
	return plan, nil
}

// SingleBank reports whether the export came from one bank, and which.
func (p *PadRestore) SingleBank() (int, bool) {
	if len(p.Banks) == 1 {
		return p.Banks[0], true
	}
	return 0, false
}

// Targets lists where each pad goes. A bank of -1 puts every pad back where it was; any
// other bank puts the pads of a one-bank export onto the same positions in that bank, so
// pad 5 of the export becomes pad 5 of the bank.
func (p *PadRestore) Targets(bank int) ([]proto.PadIndex, error) {
	out := make([]proto.PadIndex, len(p.Pads))
	for i, rp := range p.Pads {
		if bank < 0 {
			out[i] = rp.From
			continue
		}
		if _, single := p.SingleBank(); !single {
			return nil, fmt.Errorf("this export spans banks %s, so its pads can only go back where they were", bankLetters(p.Banks))
		}
		to, ok := proto.PadFromBankPad(uint16(bank), rp.From.Pad())
		if !ok {
			return nil, fmt.Errorf("no bank %d", bank)
		}
		out[i] = to
	}
	return out, nil
}

func bankLetters(banks []int) string {
	var b strings.Builder
	for _, bank := range banks {
		b.WriteByte(byte('A' + bank))
	}
	return b.String()
}

// RestorePadsTo puts an export onto a bank (-1: back where each pad came from): each sound
// goes on its target pad, then that pad's settings. Pads the export does not mention are
// left alone. It returns a one-line summary.
func RestorePadsTo(u RestoreUnit, plan *PadRestore, bank int, progress func(done, total int)) (string, error) {
	targets, err := plan.Targets(bank)
	if err != nil {
		return "", err
	}
	for i, rp := range plan.Pads {
		if progress != nil {
			progress(i, len(plan.Pads))
		}
		imported, err := LoadSample(u, plan.Dir+"/"+rp.File)
		if err != nil {
			return "", err
		}
		if _, err := PutSample(u, targets[i], imported.Sample, rp.Name); err != nil {
			return "", err
		}
		if err := applySaved(u, targets[i], rp.saved); err != nil {
			return "", fmt.Errorf("%s settings: %w", targets[i], err)
		}
	}
	where := "back where they were"
	if bank >= 0 {
		where = fmt.Sprintf("onto bank %c", 'A'+bank)
	}
	return fmt.Sprintf("%d pads restored %s from %s", len(plan.Pads), where, plan.Dir), nil
}

// RestorePads puts an export back where each pad came from.
func RestorePads(u RestoreUnit, dir string, progress func(done, total int)) (string, error) {
	plan, err := PlanPadRestore(u, dir)
	if err != nil {
		return "", err
	}
	if _, err := RestorePadsTo(u, plan, -1, progress); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d pads restored from %s", len(plan.Pads), plan.Dir), nil
}

// applySaved writes a pad's saved settings back.
func applySaved(u RestoreUnit, pad proto.PadIndex, saved savedPad) error {
	set := func(name string, value int32) error {
		info, ok := proto.ParamByName(name)
		// A parameter this version does not know is left at its default.
		if !ok || info.Scope != proto.ScopePad {
			return nil
		}
		return u.SetParam(info.ID, proto.TargetPad(pad), value)
	}
	// Mode first: turning on one shot can clear gate and loop, and the saved values of
	// those should win.
	if mode, ok := saved.Params["mode-flags"]; ok {
		if err := set("mode-flags", mode); err != nil {
			return err
		}
	}
	for name, value := range saved.Params {
		if name == "mode-flags" || contains(markers, name) {
			continue
		}
		if err := set(name, value); err != nil {
			return err
		}
	}
	// Markers last, end before start so start never lands past a stale end.
	for _, name := range []string{"end", "start", "loop-top"} {
		if value, ok := saved.Params[name]; ok {
			if err := set(name, value); err != nil {
				return err
			}
		}
	}
	if len(saved.ChopPoints) > 0 {
		return u.SetChopPoints(pad, saved.ChopPoints)
	}
	return nil
}

// HasSidecar reports whether a folder holds a pad export.
func HasSidecar(s Storage, dir string) bool {
	_, exists, err := s.Stat(dir + "/" + Sidecar)
	return err == nil && exists
}

// DefaultExportDir suggests where to export a bank of a project.
func DefaultExportDir(project string, bank int) string {
	return fmt.Sprintf("SD:exports/%s-%c", SanitizeName(path.Clean(project)), 'A'+bank)
}
