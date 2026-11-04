package core

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moahbeatlab/404tui/device"
	"github.com/moahbeatlab/404tui/sim"
)

// docs/tools.md is written from the program itself: for every screen the keys (on a computer and on the handheld) and every
// menu with its settings, found by opening the menus and the dialogs under them. It cannot go out of date: this test fails
// when the file differs from what the program says. go test ./core -run ToolsReference -update-docs writes it.
var updateDocs = flag.Bool("update-docs", false, "write docs/tools.md")

// docState is a program with a kit on the pads, a pattern with notes and a unit to talk to: what the menus need to show everything.
func docState(t *testing.T) *App {
	base := filepath.Join(os.TempDir(), "404tui-docs") // a fixed folder: the file menus show it
	os.RemoveAll(base)
	t.Cleanup(func() { os.RemoveAll(base) })
	if err := WriteDemo(filepath.Join(base, "samples")); err != nil {
		t.Fatal(err)
	}
	a := New(filepath.Join(base, "samples"), filepath.Join(base, "out"), &NullAudio{})
	a.Connect = func() (Link, error) { return DeviceLink{Dev: device.New(sim.Demo(), "sim")}, nil }
	keys(t, a, kit)
	a.go_(Seq)
	keys(t, a, "A R R R R A D A")
	a.go_(Pads)
	return a
}

type docMenu struct {
	heading string // how it is opened, for the reader
	screen  Screen
	open    func(a *App)
}

func menuSource(m *menu) []menuItem {
	if m.full != nil {
		return m.full
	}
	return m.items
}

// walkTo opens the menu and goes down the dialogs by the item numbers of the path.
func walkTo(a *App, open func(*App), path []int) *menu {
	open(a)
	m := a.menu
	for _, idx := range path {
		if m == nil {
			return nil
		}
		it := menuSource(m)[idx]
		a.menu = nil
		if it.run != nil {
			it.run(it.val)
		}
		m = a.menu
	}
	return m
}

func choices(it menuItem) string {
	if len(it.vals) == 0 {
		return ""
	}
	v := it.vals
	var s string
	if len(v) <= 10 {
		s = strings.Join(v, " / ")
	} else {
		s = fmt.Sprintf("%s, %s, ... %s (%d values)", v[0], v[1], v[len(v)-1], len(v))
	}
	if it.val >= 0 && it.val < len(v) {
		s += "; now " + v[it.val]
	}
	return s
}

func dumpMenu(t *testing.T, sb *strings.Builder, open func(*App), path []int, indent string, depth int) {
	a := docState(t)
	m := walkTo(a, open, path)
	if m == nil {
		return
	}
	title := strings.TrimSpace(m.title)
	if len(path) > 0 {
		fmt.Fprintf(sb, "%s- **%s** (a dialog):\n", indent, title)
		indent += "  "
	}
	for idx, it := range menuSource(m) {
		switch {
		case it.tab:
			fmt.Fprintf(sb, "%s\n%s*Page %s*\n\n", "", indent, strings.Title(strings.ToLower(it.label)))
			continue
		case it.header:
			fmt.Fprintf(sb, "%s\n%s*%s*\n\n", "", indent, strings.Title(strings.ToLower(it.label)))
			continue
		}
		line := fmt.Sprintf("%s- `%s`", indent, it.label)
		if c := choices(it); c != "" {
			line += " — " + c
		}
		fmt.Fprintln(sb, line)
		if it.run != nil && strings.HasSuffix(it.label, "...") && depth < 2 {
			dumpMenu(t, sb, open, append(append([]int{}, path...), idx), indent+"  ", depth+1)
		}
	}
}

// padPanelDoc lists the settings of the pad panel's two pages with their ranges.
func padPanelDoc() string {
	var sb strings.Builder
	for pi, page := range padPages {
		fmt.Fprintf(&sb, "*Page %s* (`[` `]` turn the pages)\n\n", padPageNames[pi])
		for _, st := range page {
			var what string
			switch st.kind {
			case kSwitch, kFlag:
				what = "off / on"
			case kChoice:
				what = strings.Join(st.names, " / ")
			default:
				what = fmt.Sprintf("%s to %s (the default is %s)", st.showOr(st.lo), st.showOr(st.hi), st.showOr(st.def))
			}
			fmt.Fprintf(&sb, "- `%s` — %s\n", st.label, what)
		}
		sb.WriteString("\n")
	}
	fmt.Fprintf(&sb, "*Page %s* — the pad's tools, listed below.\n\n", padPageNames[2])
	return sb.String()
}

func (st padSetting) showOr(v int32) string {
	if st.show != nil {
		return st.show(v)
	}
	return fmt.Sprint(v)
}

// configDoc lists the items of the CONFIG screen.
func configDoc(a *App) string {
	Terminal = true // the computer's settings: the writing of the project is among them
	defer func() { Terminal = false }()
	var sb strings.Builder
	for _, it := range a.cfgItems() {
		if it.head != "" {
			fmt.Fprintf(&sb, "\n*%s*\n\n", strings.Title(strings.ToLower(it.head)))
			continue
		}
		fmt.Fprintf(&sb, "- `%s` — %s\n", it.label, strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(it.hint, "<", "left"), ">", "right")))
	}
	return sb.String()
}

func keyTable(a *App, s Screen) string {
	return fmt.Sprintf("**Keys on a computer:** %s\n\n**Keys on the handheld:** %s\n\n", keyRow(a, s, true), keyRow(a, s, false))
}

func keyRow(a *App, s Screen, terminal bool) string {
	Terminal = terminal
	defer func() { Terminal = false }()
	a.scr = s
	plain, shift := a.hints()
	var parts []string
	for _, h := range plain {
		parts = append(parts, fmt.Sprintf("`%s` %s", tk(h.key), h.what))
	}
	for _, h := range shift {
		parts = append(parts, fmt.Sprintf("`%s` %s", tk(h.key), h.what))
	}
	return strings.Join(parts, " · ")
}

func TestToolsReference(t *testing.T) {
	menus := []struct {
		screen Screen
		name   string
		intro  string
		menus  []docMenu
	}{
		{Pads, "PADS", "The 16 pads of a bank. `a` opens the pad panel: SOUND and SHAPE pages with the pad's settings, and the TOOLS page.", []docMenu{
			{"Pad tools (the TOOLS page of the pad panel, `t`)", Pads, func(a *App) { a.openPadMenu() }},
		}},
		{Seq, "SEQ", "The step grid of the pattern in front. `Enter` (or `b`) opens the note panel, `t` the pattern's tools and settings.", []docMenu{
			{"Pattern tools and settings (`t`)", Seq, func(a *App) { a.seqMenu() }},
		}},
		{Unit, "PROJECTS", "On a computer: the 16 projects of the unit when one is plugged in (online), else those of a folder on this computer (offline). The program looks for the unit all the time, and the start screen offers `u` online and `o` offline (the project open last). `a` opens a project's menu, `t` chooses online or offline and the offline folder. On the handheld this screen is called UNIT.", []docMenu{
			{"Project menu of the unit (`a`, online)", Unit, func(a *App) { connectUnit(t, a); a.unitMenu() }},
			{"Online or offline (`t`)", Unit, func(a *App) { a.go_(Unit); a.modeMenu() }},
			{"Project menu of the offline folder (`a`, offline)", Unit, func(a *App) { a.go_(Unit); a.offlineMenu() }},
		}},
		{Files, "FILES", "The file browser: this computer, the unit's card and its storage. `t` chooses where the files are loaded from (the computer, the samples folder, the unit's SD card, a USB drive, favourites) and leads to the file tools.", []docMenu{
			{"Load files from where (`t`)", Files, func(a *App) { a.placesMenu() }},
			{"File tools (in the menu of `t`)", Files, func(a *App) { a.filesMenu() }},
		}},
		{Slice, "SLICE", "The slicer: cut a sample into slices and play them in an order. `Enter` opens the settings, `t` the tools.", []docMenu{
			{"Slice settings (`Enter`)", Slice, func(a *App) { a.go_(Slice); a.sliceMenu() }},
			{"Slice tools (`t`)", Slice, func(a *App) { a.go_(Slice); a.sliceToolsMenu() }},
		}},
		{Config, "CONFIG", "The program's settings; on a computer the writing of the project (what SAVE does on the handheld) is its first section.", nil},
		{Save, "SAVE (handheld only)", "Writes the project as a folder for the SD card. On a computer this is in CONFIG, under PROJECT.", nil},
		{Perform, "PERF (handheld only)", "The unit's MIDI as it plays, and the pattern now and next.", nil},
		{Loop, "LOOP (handheld only)", "Makes patterns from the slices of the pads.", nil},
	}
	var sb strings.Builder
	sb.WriteString("# Tools and settings of every screen\n\n")
	sb.WriteString("*Written by the program's own test (`go test ./core -run ToolsReference -update-docs`): the keys and the menus below are what the program has, with the values a menu offers. Do not edit by hand.*\n\n")
	for _, sc := range menus {
		fmt.Fprintf(&sb, "## %s\n\n%s\n\n", sc.name, sc.intro)
		if strings.Contains(sc.name, "handheld only") {
			fmt.Fprintf(&sb, "**Keys on the handheld:** %s\n\n", keyRow(docState(t), sc.screen, false))
		} else {
			fmt.Fprintln(&sb, keyTable(docState(t), sc.screen))
		}
		switch sc.screen {
		case Pads:
			sb.WriteString("### Pad panel (`a`)\n\nThe settings of the pad under the cursor, written into the project at once. With pads marked (`y a`) a change goes to all of them.\n\n" + padPanelDoc())
		case Seq:
			sb.WriteString("### Note panel (`Enter` or `b`)\n\nThe note under the cursor: `Velocity` 1 to 127, `Length` from 15 ticks to the end of the pattern, `Timing` (how far off the grid, early or late, in ticks), `Pitch` (-12 to +12 semitones above the pad's own; plain when it is not changed). `a` sets a value back to its default, `x` hears the note, `Esc` closes. On a step with no note `a` puts one.\n\n")
		case Config:
			sb.WriteString("### Settings\n" + configDoc(docState(t)) + "\n")
		}
		for _, dm := range sc.menus {
			fmt.Fprintf(&sb, "### %s\n\n", dm.heading)
			dumpMenu(t, &sb, dm.open, nil, "", 0)
			sb.WriteString("\n")
		}
	}
	got := sb.String()
	if *updateDocs {
		if err := os.WriteFile("../docs/tools.md", []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile("../docs/tools.md")
	if err != nil || string(want) != got {
		t.Fatal("docs/tools.md is out of date: go test ./core -run ToolsReference -update-docs")
	}
}
