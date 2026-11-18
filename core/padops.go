package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/lab"
	"github.com/moahbeatlab/404tui/proto"
)

// The pad actions of the terminal program's Pads tab that were not here: rename, find a pad by name, set up pads that were never
// set up, the switches of every pad of a bank, and the export of a bank to a folder (WAV files and samples.json, the format of
// SparkyMK2's pad export) and the restore from one.

// renamePads asks for a name for the pad (or, with pads marked, for a prefix or a suffix to put on each name).
func (a *App) renamePad(n int) {
	tg := a.targetsFor(n)
	if len(tg) > 1 {
		m := &menu{title: fmt.Sprintf("rename %d marked pads", len(tg))}
		add := func(l string, run func(int)) { m.items = append(m.items, menuItem{label: l, run: run}) }
		add("Add a prefix...", func(int) {
			a.askText("prefix for the names", "", "", func(s string) { a.renameEach(tg, func(old string) string { return s + old }) })
		})
		add("Add a suffix...", func(int) {
			a.askText("suffix for the names", "", "", func(s string) { a.renameEach(tg, func(old string) string { return old + s }) })
		})
		a.menu = m
		return
	}
	a.askText("rename "+a.label(n), "", a.info.Pads[n].Name, func(s string) {
		a.renameEach([]int{n}, func(string) string { return s })
	})
}

func (a *App) renameEach(pads []int, f func(old string) string) {
	pre := a.lab.Snapshot()
	done := 0
	for _, n := range pads {
		name := strings.TrimSpace(f(a.info.Pads[n].Name))
		if name == "" {
			continue
		}
		if err := a.lab.RenamePad(n, name); err != nil {
			a.say(err.Error(), true)
			return
		}
		done++
	}
	a.commitUndo(pre, A, 0)
	a.refresh()
	a.say(fmt.Sprintf("renamed %d pads", done), false)
}

// findPad asks for part of a name and moves the cursor to the next pad (from the one in front, around the banks) that has it.
func (a *App) findPad() {
	a.askText("find a pad by name", "", "", func(s string) {
		q := strings.ToLower(s)
		if q == "" {
			return
		}
		start := a.padN()
		for i := 1; i <= 160; i++ {
			n := (start + i) % 160
			if p := a.info.Pads[n]; p.File && strings.Contains(strings.ToLower(p.Name), q) {
				a.bank, a.cur = n/16, n%16
				a.say(fmt.Sprintf("%s: %s", a.label(n), p.Name), false)
				return
			}
		}
		a.say(fmt.Sprintf("no pad with %q in its name", s), true)
	})
}

// setupPads puts the settings of pads that were never set up (pan at the centre, hold 100%, time stretch 100%) right.
func (a *App) setupPads(all bool) {
	pre := a.lab.Snapshot()
	lo, hi := a.bank*16, a.bank*16+16
	if all {
		lo, hi = 0, 160
	}
	pads, fixed := 0, 0
	for n := lo; n < hi; n++ {
		if !a.info.Pads[n].File {
			continue
		}
		if f, err := a.lab.SetupPad(n); err == nil && f > 0 {
			pads++
			fixed += f
		}
	}
	a.commitUndo(pre, A, 0)
	a.refresh()
	if pads == 0 {
		a.say("every pad is set up", false)
		return
	}
	a.say(fmt.Sprintf("%d pads set up (%d settings)", pads, fixed), false)
}

// bankSwitchesMenu switches one setting on or off on every pad of the bank that has a sample.
func (a *App) bankSwitchesMenu() {
	m := &menu{title: fmt.Sprintf("switches of every pad of bank %c", 'A'+a.bank)}
	for _, st := range padPages[1] {
		st := st
		if st.kind != kSwitch && st.kind != kFlag {
			continue
		}
		m.items = append(m.items, menuItem{label: st.label, vals: []string{"on", "off"}, val: 0, keepOpen: false, run: func(v int) {
			pre := a.lab.Snapshot()
			n := 0
			for p := a.bank * 16; p < a.bank*16+16; p++ {
				if a.info.Pads[p].File {
					a.setSetting(p, st, int32(1-v))
					n++
				}
			}
			a.commitUndo(pre, A, 0)
			a.say(fmt.Sprintf("%s %s on %d pads", st.label, []string{"on", "off"}[v], n), false)
		}})
	}
	m.items = append(m.items, menuItem{label: "Bus of every pad", vals: []string{"off", "BUS 1", "BUS 2"}, run: func(v int) {
		pre := a.lab.Snapshot()
		for p := a.bank * 16; p < a.bank*16+16; p++ {
			if a.info.Pads[p].File {
				a.lab.SetPadParam(p, 0x7A, int32(v))
			}
		}
		a.commitUndo(pre, A, 0)
		a.refresh()
		a.say(fmt.Sprintf("bus %s on the pads of bank %c", []string{"off", "BUS 1", "BUS 2"}[v], 'A'+a.bank), false)
	}})
	a.menu = m
}

// ---- export and restore of a bank

type sidecarPad struct {
	Pad    string           `json:"pad"`
	File   string           `json:"file"`
	Name   string           `json:"name"`
	Params map[string]int32 `json:"params"`
	Chops  []uint32         `json:"chopPoints"`
}

// sidecarPattern is a pattern of the bank in an export: its slot and its file as the unit keeps it (PTNnnnnn.BIN).
type sidecarPattern struct {
	Slot string `json:"slot"`
	File string `json:"file"`
}

type sidecarFile struct {
	Format   string           `json:"format"`
	Version  int              `json:"version"`
	Project  string           `json:"project"`
	Pads     []sidecarPad     `json:"pads"`
	Bank     string           `json:"bank,omitempty"`     // the bank of a whole-bank export
	Patterns []sidecarPattern `json:"patterns,omitempty"` // its patterns, in a whole-bank export
}

func (a *App) exportDir() string { return filepath.Join(a.dataDir(), "exports") }

// exportRoot is the folder the chooser starts in for an export: the one chosen last, else the exports folder.
func (a *App) exportRoot() string {
	if a.set.ExportDir != "" {
		return expandHome(a.set.ExportDir)
	}
	return a.exportDir()
}

// exportBank writes the pads of the bank (the marked ones among them) as WAV files named after the pad, and samples.json with
// their settings, into a folder of its own below the exports folder. The whole bank brings its patterns too (slots 1 to 16).
func (a *App) exportBank() {
	pads := a.targetsFor(a.padN())
	bank := -1
	if len(a.marks) == 0 {
		bank = a.bank
		pads = nil
		for p := a.bank * 16; p < a.bank*16+16; p++ {
			if a.info.Pads[p].File {
				pads = append(pads, p)
			}
		}
	}
	if len(pads) == 0 {
		a.say("no pad with a sample in this bank", true)
		return
	}
	dir := filepath.Join(a.exportDir(), fmt.Sprintf("%s_bank_%c", app.SanitizeName(a.ProjectName()), 'A'+a.bank))
	if Terminal { // a computer asks where to: the folder chooser, then a folder of its own in it
		a.pickFolder(fmt.Sprintf("export bank %c into the folder", 'A'+a.bank), a.exportRoot(), func(root string) {
			a.set.ExportDir = root
			a.saveSettings()
			a.freeTarget(root, filepath.Base(dir), true, func(d string) { a.exportPadsTo(pads, bank, d) })
		})
		return
	}
	a.exportPadsTo(pads, bank, dir)
}

// exportPadsTo writes the pads as WAV files and samples.json into the folder; with a bank (not -1) its patterns too.
func (a *App) exportPadsTo(pads []int, bank int, dir string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		a.say(err.Error(), true)
		return
	}
	sc := sidecarFile{Format: "sparkymk2-pads", Version: 1, Project: a.ProjectName()}
	for _, n := range pads {
		id, ok := a.padSample(n)
		if !ok {
			continue
		}
		wav, err := a.lab.WAV(id)
		if err != nil {
			a.say(err.Error(), true)
			return
		}
		pad, _ := proto.NewPadIndex(uint16(n))
		file := fmt.Sprintf("%s %s.wav", app.PadLabel(pad), app.SanitizeName(a.info.Pads[n].Name))
		if err := os.WriteFile(filepath.Join(dir, file), wav, 0o644); err != nil {
			a.say(err.Error(), true)
			return
		}
		params := map[string]int32{}
		for _, p := range proto.Params {
			if p.Scope != proto.ScopePad || p.Name == "start" || p.Name == "end" || p.Name == "loop-top" {
				continue
			}
			if v, err := a.lab.PadParam(n, p.ID); err == nil {
				params[p.Name] = v
			}
		}
		sc.Pads = append(sc.Pads, sidecarPad{Pad: app.PadLabel(pad), File: file, Name: a.info.Pads[n].Name, Params: params, Chops: []uint32{}})
	}
	if bank >= 0 {
		sc.Bank = string(rune('A' + bank))
		for n := bank*16 + 1; n <= bank*16+16; n++ {
			pb, err := a.lab.PatternBytes(n)
			if err != nil || len(pb) == 0 {
				continue
			}
			file := fmt.Sprintf("pattern %s.bin", a.label(n-1))
			if err := os.WriteFile(filepath.Join(dir, file), pb, 0o644); err != nil {
				a.say(err.Error(), true)
				return
			}
			sc.Patterns = append(sc.Patterns, sidecarPattern{Slot: a.label(n - 1), File: file})
		}
	}
	b, _ := json.MarshalIndent(sc, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "samples.json"), b, 0o644); err != nil {
		a.say(err.Error(), true)
		return
	}
	msg := fmt.Sprintf("%d pads exported to %s", len(sc.Pads), short2(dir, 40))
	if len(sc.Patterns) > 0 {
		msg = fmt.Sprintf("%d pads and %d patterns exported to %s", len(sc.Pads), len(sc.Patterns), short2(dir, 40))
	}
	a.say(msg, false)
}

// restoreMenu lists the exports there are and puts one back.
func (a *App) restoreMenu() {
	ents, _ := os.ReadDir(a.exportDir())
	var dirs []string
	for _, e := range ents {
		if e.IsDir() {
			if _, err := os.Stat(filepath.Join(a.exportDir(), e.Name(), "samples.json")); err == nil {
				dirs = append(dirs, e.Name())
			}
		}
	}
	sort.Strings(dirs)
	m := &menu{title: "restore pads from an export"}
	for _, d := range dirs {
		d := d
		m.items = append(m.items, menuItem{label: d, run: func(int) { a.restorePads(filepath.Join(a.exportDir(), d)) }})
	}
	m.items = append(m.items, menuItem{label: "A folder somewhere else...", run: func(int) {
		a.askText("folder of an export", "the folder with samples.json in it", "", func(s string) { a.restorePads(expandHome(s)) })
	}})
	a.menu = m
}

// restorePads reads an export and puts it back. An export of one bank asks for the bank it goes to (its own first); the pads
// keep their places in the bank and the patterns their slots, their notes moved to the pads of that bank. Patterns already in
// those slots are asked about.
func (a *App) restorePads(dir string) {
	b, err := os.ReadFile(filepath.Join(dir, "samples.json"))
	if err != nil {
		a.say("there is no samples.json in "+short2(dir, 30), true)
		return
	}
	var sc sidecarFile
	if err := json.Unmarshal(b, &sc); err != nil {
		a.say("samples.json: "+err.Error(), true)
		return
	}
	from, single := exportBankOf(sc)
	if !single {
		a.restoreTo(dir, sc, -1, true)
		return
	}
	m := &menu{title: fmt.Sprintf("restore bank %c into bank", 'A'+from)}
	for i := 0; i < 10; i++ {
		to := (from + i) % 10
		label := fmt.Sprintf("Bank %c", 'A'+to)
		if to == from {
			label += " (where it came from)"
		}
		if used := a.bankPads(to); used > 0 {
			label += fmt.Sprintf("  %d pads in use", used)
		}
		m.items = append(m.items, menuItem{label: label, run: func(int) { a.askPatternSlots(dir, sc, from, to) }})
	}
	a.menu = m
}

// exportBankOf is the bank the pads (and patterns) of an export came from, when it is one.
func exportBankOf(sc sidecarFile) (int, bool) {
	bank := -1
	for _, sp := range sc.Pads {
		p, err := proto.ParsePad(sp.Pad)
		if err != nil {
			continue
		}
		if bank >= 0 && int(p.Bank()) != bank {
			return 0, false
		}
		bank = int(p.Bank())
	}
	for _, pt := range sc.Patterns {
		p, err := proto.ParsePad(pt.Slot)
		if err != nil {
			continue
		}
		if bank >= 0 && int(p.Bank()) != bank {
			return 0, false
		}
		bank = int(p.Bank())
	}
	return bank, bank >= 0
}

func (a *App) bankPads(b int) int {
	n := 0
	for p := b * 16; p < b*16+16; p++ {
		if a.info.Pads[p].File {
			n++
		}
	}
	return n
}

// askPatternSlots goes on with the restore, asking first when patterns of the export would replace patterns in the bank.
func (a *App) askPatternSlots(dir string, sc sidecarFile, from, to int) {
	var taken []string
	for _, pt := range sc.Patterns {
		if n, ok := movedSlot(pt.Slot, to); ok {
			if pb, _ := a.lab.PatternBytes(n); len(pb) > 0 {
				taken = append(taken, a.label(n-1))
			}
		}
	}
	if len(taken) == 0 {
		a.restoreTo(dir, sc, to, true)
		return
	}
	m := &menu{title: fmt.Sprintf("bank %c has patterns in %s", 'A'+to, strings.Join(taken, " "))}
	m.items = append(m.items,
		menuItem{label: fmt.Sprintf("Replace them (%d patterns, u undoes it)", len(taken)), run: func(int) { a.restoreTo(dir, sc, to, true) }},
		menuItem{label: "Keep them: only the empty slots get patterns", run: func(int) { a.restoreTo(dir, sc, to, false) }})
	a.menu = m
}

// movedSlot is the pattern slot (1..160) of a slot label moved to bank to (-1: where it is).
func movedSlot(label string, to int) (int, bool) {
	p, err := proto.ParsePad(label)
	if err != nil {
		return 0, false
	}
	if to >= 0 {
		p, _ = proto.PadFromBankPad(uint16(to), p.Pad())
	}
	return int(p.Index()) + 1, true
}

// restoreTo puts the pads of an export on bank to (-1: each back where it came from) with their settings, and the patterns in
// the slots of that bank, playing its pads; replace false leaves the slots that have a pattern alone. It is one undo.
func (a *App) restoreTo(dir string, sc sidecarFile, to int, replace bool) {
	pre := a.lab.Snapshot()
	done, pats, kept := 0, 0, 0
	for _, sp := range sc.Pads {
		pad, err := proto.ParsePad(sp.Pad)
		if err != nil {
			continue
		}
		if to >= 0 {
			pad, _ = proto.PadFromBankPad(uint16(to), pad.Pad())
		}
		raw, err := os.ReadFile(filepath.Join(dir, sp.File))
		if err != nil {
			continue
		}
		mb, err := a.lab.Decode(raw, ".wav")
		if err != nil {
			continue
		}
		var meta lab.Meta
		if json.Unmarshal(mb, &meta) != nil {
			continue
		}
		n := int(pad.Index())
		if err := a.assignPad(n, meta.ID, sp.Name); err != nil {
			continue
		}
		for name, v := range sp.Params {
			if p, ok := proto.ParamByName(name); ok && p.Scope == proto.ScopePad {
				a.lab.SetPadParam(n, p.ID, v)
			}
		}
		done++
	}
	for _, pt := range sc.Patterns {
		src, err := proto.ParsePad(pt.Slot)
		if err != nil {
			continue
		}
		n, _ := movedSlot(pt.Slot, to)
		if old, _ := a.lab.PatternBytes(n); len(old) > 0 && !replace {
			kept++
			continue
		}
		pb, err := os.ReadFile(filepath.Join(dir, pt.File))
		if err != nil {
			continue
		}
		if to >= 0 && to != int(src.Bank()) { // the notes on the exported bank's pads play the pads of this one; other banks stay
			formats.RetargetBank(pb, src.Bank(), uint16(to))
		}
		if a.lab.SetPatternBytes(n, pb) == nil {
			pats++
		}
	}
	a.commitUndo(pre, A, 0)
	a.pcm = map[int]int{}
	a.refresh()
	where := ""
	if to >= 0 {
		where = fmt.Sprintf(" onto bank %c", 'A'+to)
	}
	msg := fmt.Sprintf("%d pads restored%s from %s", done, where, short2(dir, 30))
	if pats > 0 || kept > 0 {
		msg = fmt.Sprintf("%d pads and %d patterns restored%s", done, pats, where)
		if kept > 0 {
			msg += fmt.Sprintf(", %d slots kept", kept)
		}
	}
	a.say(msg, false)
}
