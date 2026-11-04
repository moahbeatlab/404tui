package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/moahbeatlab/404tui/app"
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

type sidecarFile struct {
	Format  string       `json:"format"`
	Version int          `json:"version"`
	Project string       `json:"project"`
	Pads    []sidecarPad `json:"pads"`
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
// their settings, into a folder of its own below the exports folder.
func (a *App) exportBank() {
	pads := a.targetsFor(a.padN())
	if len(a.marks) == 0 {
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
			a.freeTarget(root, filepath.Base(dir), true, func(d string) { a.exportPadsTo(pads, d) })
		})
		return
	}
	a.exportPadsTo(pads, dir)
}

// exportPadsTo writes the pads as WAV files and samples.json into the folder.
func (a *App) exportPadsTo(pads []int, dir string) {
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
	b, _ := json.MarshalIndent(sc, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "samples.json"), b, 0o644); err != nil {
		a.say(err.Error(), true)
		return
	}
	a.say(fmt.Sprintf("%d pads exported to %s", len(sc.Pads), short2(dir, 40)), false)
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

// restorePads puts every pad of an export back on its own pad, with its settings.
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
	pre := a.lab.Snapshot()
	done := 0
	for _, sp := range sc.Pads {
		pad, err := proto.ParsePad(sp.Pad)
		if err != nil {
			continue
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
		id := meta.ID
		n := int(pad.Index())
		if err := a.lab.Assign(n, id, sp.Name); err != nil {
			continue
		}
		for name, v := range sp.Params {
			if p, ok := proto.ParamByName(name); ok && p.Scope == proto.ScopePad {
				a.lab.SetPadParam(n, p.ID, v)
			}
		}
		done++
	}
	a.commitUndo(pre, A, 0)
	a.pcm = map[int]int{}
	a.refresh()
	a.say(fmt.Sprintf("%d pads restored from %s", done, short2(dir, 30)), false)
}
