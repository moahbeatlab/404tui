package core

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/dbg"
	"github.com/moahbeatlab/404tui/midiin"
	"github.com/moahbeatlab/404tui/record"
)

// The CONFIG screen: the settings of the program that are not about a project: the rule that sends a pad to a bus by its name,
// the recorder, the pad codes of the unit, MIDI, and the colours of the terminal.

// Defaults of the settings.
const (
	defaultLiveCode  = "1 3 3 7"
	defaultRecCode   = "1 16 16 16"
	defaultMidiPort  = "404"
	defaultMidiChan  = 16
	defaultLevelChan = 15
	defaultCtlPort   = "ec4" // the controller's input (its own MIDI port, beside the unit's)
	defaultCtlChan   = 16    // the pattern group of the controller
)

var defaultMidiMap = [4]string{"note 62", "note 60", "note 74", "note 75"}

// midiActions names what the four messages do, in the order of settings.MidiMap.
var midiActions = [4]string{"live mode", "recording", "next becomes now", "clear next"}

// busRule is the rule that sends a pad to a bus by its name: the one set here, or the default.
func (a *App) busRule() app.BusRule {
	if a.set.Buses != nil && a.set.Buses.Version == app.BusRuleVersion {
		return *a.set.Buses
	}
	return app.DefaultBusRule()
}

func (a *App) setBusRule(r app.BusRule) {
	a.set.Buses = &r
	a.saveSettings()
}

// busColourRGB is the colour a bus is drawn in; the zero colour for the program's own.
func busColourRGB(name string) (color.RGBA, bool) {
	for _, c := range app.BusColours {
		if c.Name == name {
			n, _ := strconv.Atoi(c.Code)
			return xterm(n), true
		}
	}
	return color.RGBA{}, false
}

// busColour is the colour of a bus (1 or 2) as the rule has it, the default blue and red otherwise.
func (a *App) busColour(bus int) color.RGBA {
	r := a.busRule()
	name := r.Bus1Colour
	def := xterm(75)
	if bus == 2 {
		name, def = r.Bus2Colour, xterm(196)
	}
	if c, ok := busColourRGB(name); ok {
		return c
	}
	return def
}

func busName(b int) string {
	switch b {
	case 1:
		return "BUS 1"
	case 2:
		return "BUS 2"
	}
	return "no bus"
}

// cycleColour is the colour after (or before) the named one, the program's own ("") at one end.
func cycleColour(name string, by int) string {
	names := []string{""}
	for _, c := range app.BusColours {
		names = append(names, c.Name)
	}
	i := slices.Index(names, name)
	return names[(max(i, 0)+by+len(names))%len(names)]
}

// splitWords reads a list of words typed with commas or spaces between them: lower case, once each.
func splitWords(text string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return r == ',' || r == ' ' || r == ';' }) {
		if !slices.Contains(out, w) {
			out = append(out, w)
		}
	}
	return out
}

func (a *App) recordDir() string {
	if a.set.RecordDir != "" {
		return expandHome(a.set.RecordDir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Music", "404tui")
}

func (a *App) recordDevice() string {
	if a.set.RecordDevice != "" {
		return a.set.RecordDevice
	}
	return record.DefaultDevice
}

func expandHome(p string) string {
	if p == "~" || len(p) > 1 && p[:2] == "~/" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}

// parseCode reads a pad code written as numbers 1 to 16 with spaces, commas or dashes between them; empty or "off" is no code.
func parseCode(s string) []int {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || s == "off" || s == "-" {
		return nil
	}
	var out []int
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' || r == '-' || r == '.' }) {
		n, err := strconv.Atoi(f)
		if err != nil || n < 1 || n > 16 {
			return nil
		}
		out = append(out, n)
	}
	return out
}

func codeText(code []int) string {
	if len(code) == 0 {
		return "off"
	}
	parts := make([]string, len(code))
	for i, n := range code {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, " ")
}

func (a *App) liveCode() []int {
	if a.set.LiveCode != "" {
		return parseCode(a.set.LiveCode)
	}
	return parseCode(defaultLiveCode)
}

func (a *App) recCode() []int {
	if a.set.RecCode != "" {
		return parseCode(a.set.RecCode)
	}
	return parseCode(defaultRecCode)
}

func (a *App) midiPort() string {
	if a.set.MidiPort != "" {
		return a.set.MidiPort
	}
	return defaultMidiPort
}

// channelOf reads a MIDI channel setting: 1 to 16, 0 for off, def when empty.
func channelOf(s string, def int) int {
	switch s = strings.ToLower(strings.TrimSpace(s)); s {
	case "":
		return def
	case "off":
		return 0
	}
	if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= 16 {
		return n
	}
	return def
}

// ctlPort is the part of the name of the controller's MIDI input; "" is off.
func (a *App) ctlPort() string {
	switch p := strings.ToLower(strings.TrimSpace(a.set.MidiCtlPort)); p {
	case "":
		return defaultCtlPort
	case "off":
		return ""
	default:
		return p
	}
}

// ctlSend: the controller's pattern is sent to the unit (a Program Change), unless it is turned off.
func (a *App) ctlSend() bool { return strings.ToLower(strings.TrimSpace(a.set.MidiCtlSend)) != "off" }

func (a *App) ctlChan() int { return channelOf(a.set.MidiCtlChan, defaultCtlChan) }

func (a *App) midiChan() int  { return channelOf(a.set.MidiChan, defaultMidiChan) }
func (a *App) levelChan() int { return channelOf(a.set.MidiLevelChan, defaultLevelChan) }

func textOrOff(s string) string {
	if s == "" {
		return "off"
	}
	return s
}

func chanText(n int) string {
	if n == 0 {
		return "off"
	}
	return strconv.Itoa(n)
}

func (a *App) midiMapText(i int) string {
	if a.set.MidiMap[i] != "" {
		return a.set.MidiMap[i]
	}
	return defaultMidiMap[i]
}

// parseMidiMap reads "note 60", "cc 20" or just "60" (a note); false for "off" or what is not one.
func parseMidiMap(s string) (kind midiin.Kind, n int, ok bool) {
	f := strings.Fields(strings.ToLower(s))
	if len(f) == 1 {
		f = []string{"note", f[0]}
	}
	if len(f) != 2 {
		return 0, 0, false
	}
	n, err := strconv.Atoi(f[1])
	if err != nil || n < 0 || n > 127 {
		return 0, 0, false
	}
	switch f[0] {
	case "note":
		return midiin.NoteOn, n, true
	case "cc":
		return midiin.CC, n, true
	}
	return 0, 0, false
}

// ---- the screen

// cfgItem is a row of the Config screen: a heading, or a setting with its value and what A and Left/Right do to it.
type cfgItem struct {
	head   string
	label  string
	value  func(a *App) string
	colour func(a *App) (color.RGBA, bool) // the value drawn in a colour (a bus)
	hint   string
	change func(a *App, by int) // Left, Right
	edit   func(a *App)         // A
}

func (a *App) cfgItems() []cfgItem {
	rule := a.busRule()
	words := func(list []string) string { return strings.Join(list, ", ") }
	editWords := func(bus int) func(a *App) {
		return func(a *App) {
			r := a.busRule()
			cur := r.Bus1Words
			if bus == 2 {
				cur = r.Bus2Words
			}
			a.askText(fmt.Sprintf("words for samples on BUS %d", bus), "", words(cur), func(s string) {
				r := a.busRule()
				if bus == 1 {
					r.Bus1Words = splitWords(s)
				} else {
					r.Bus2Words = splitWords(s)
				}
				a.setBusRule(r)
				a.say(fmt.Sprintf("BUS %d: %d words", bus, len(splitWords(s))), false)
			})
		}
	}
	colourChange := func(bus int) func(a *App, by int) {
		return func(a *App, by int) {
			r := a.busRule()
			if bus == 1 {
				r.Bus1Colour = cycleColour(r.Bus1Colour, by)
			} else {
				r.Bus2Colour = cycleColour(r.Bus2Colour, by)
			}
			a.setBusRule(r)
		}
	}
	colourName := func(bus int) func(a *App) string {
		return func(a *App) string {
			r := a.busRule()
			n := r.Bus1Colour
			if bus == 2 {
				n = r.Bus2Colour
			}
			if n == "" {
				return "the default"
			}
			return n
		}
	}
	busColoured := func(bus int) func(a *App) (color.RGBA, bool) {
		return func(a *App) (color.RGBA, bool) { return a.busColour(bus), true }
	}
	promptSetting := func(title, hint string, get func(a *App) string, set func(a *App, s string)) func(a *App) {
		return func(a *App) {
			a.askText(title, hint, get(a), func(s string) { set(a, s); a.saveSettings() })
		}
	}
	var items []cfgItem
	if Terminal { // SAVE of the handheld is a part of this screen on a computer
		items = append(items,
			cfgItem{head: "PROJECT"},
			cfgItem{label: "Project number", value: func(a *App) string { return a.ProjectName() }, hint: "< > change",
				change: func(a *App, by int) { a.slot = max(1, min(a.slot+by, 99)); a.wrote = nil }},
			cfgItem{label: "Write it to the card folder", value: func(a *App) string {
				if a.wrote != nil {
					return fmt.Sprintf("written, %d files", len(a.wrote))
				}
				return short2(filepath.Join(a.OutDir, cardFolder), 34)
			}, hint: "A writes", edit: func(a *App) { a.write() }},
			cfgItem{label: "Export it into a folder...", value: func(a *App) string { return "" }, hint: "A", edit: func(a *App) { a.exportProjectAsk() }},
			cfgItem{label: "Autosave", value: func(a *App) string { return a.autosaveSetting() }, hint: "< > change",
				change: func(a *App, by int) { a.setAutosave(by) }},
			cfgItem{label: "Quit the program", value: func(a *App) string { return "" }, hint: "A", edit: func(a *App) { a.RequestQuit() }},
		)
	}
	items = append(items, []cfgItem{
		{head: "BUSES BY PAD NAME"},
		{label: "Words for samples on BUS 1", value: func(a *App) string { return words(a.busRule().Bus1Words) }, colour: busColoured(1), hint: "A edits", edit: editWords(1)},
		{label: "Words for samples on BUS 2", value: func(a *App) string { return words(a.busRule().Bus2Words) }, colour: busColoured(2), hint: "A edits", edit: editWords(2)},
		{label: "Colour of BUS 1", value: colourName(1), colour: busColoured(1), hint: "< > change", change: colourChange(1)},
		{label: "Colour of BUS 2", value: colourName(2), colour: busColoured(2), hint: "< > change", change: colourChange(2)},
		{label: "A name with none of the words", value: func(a *App) string { return busName(a.busRule().Other) }, hint: "< > change",
			change: func(a *App, by int) { r := a.busRule(); r.Other = (r.Other + by + 3) % 3; a.setBusRule(r) }},
		{label: "Set the bus when a sample is put on a pad", value: func(a *App) string { return onOff(a.busRule().OnAssign) }, hint: "A or < > switch",
			change: func(a *App, by int) { r := a.busRule(); r.OnAssign = !r.OnAssign; a.setBusRule(r) },
			edit:   func(a *App) { r := a.busRule(); r.OnAssign = !r.OnAssign; a.setBusRule(r) }},
		{label: "Set the buses of this bank by the names...", value: func(a *App) string { return "" }, hint: "A", edit: func(a *App) { a.busScanMenu() }},
		{label: "Back to the defaults", value: func(a *App) string { return "" }, hint: "A",
			edit: func(a *App) {
				a.set.Buses = nil
				a.saveSettings()
				a.say("the bus rule and colours are back to their defaults", false)
			}},
		{head: "RECORDER"},
		{label: "Record into folder", value: func(a *App) string { return a.recordDir() }, hint: "A edits",
			edit: promptSetting("record into folder", "", func(a *App) string { return a.set.RecordDir }, func(a *App, s string) { a.set.RecordDir = s })},
		{label: "Input with this in its name", value: func(a *App) string { return a.recordDevice() }, hint: "A edits",
			edit: promptSetting("input with this in its name", "", func(a *App) string { return a.recordDevice() }, func(a *App, s string) { a.set.RecordDevice = s })},
		{label: "Record and meter with", value: func(a *App) string {
			switch {
			case !record.Native():
				return "ffmpeg or sox (no native capture here)"
			case a.set.NoNative:
				return "ffmpeg or sox"
			}
			return "the program itself"
		}, hint: "< > change", change: func(a *App, by int) {
			if !record.Native() {
				a.say("this system has no native capture: ffmpeg or sox is used", false)
				return
			}
			a.set.NoNative = !a.set.NoNative
			record.UseNative = !a.set.NoNative
			a.saveSettings()
			if a.lvl != nil && a.lvl.src != nil { // the meter listens the new way from now on
				a.lvl.src.Stop()
				a.lvl.src, a.lvl.err = nil, ""
			}
			a.say("recording and the level meter use "+map[bool]string{true: "ffmpeg or sox", false: "the program's own capture"}[a.set.NoNative], false)
		}},
		{label: "Recorder", value: func(a *App) string { return a.recorderLine() }, hint: ""},
		{head: "PAD CODES ON THE UNIT"},
		{label: "Live mode (the PERF screen)", value: func(a *App) string { return codeText(a.liveCode()) }, hint: "A edits",
			edit: promptSetting("pad code for the live mode", "pad numbers 1-16 with spaces; off", func(a *App) string { return codeText(a.liveCode()) }, func(a *App, s string) { a.set.LiveCode = strings.ToLower(s) })},
		{label: "Start and stop a recording", value: func(a *App) string { return codeText(a.recCode()) }, hint: "A edits",
			edit: promptSetting("pad code for a recording", "pad numbers 1-16 with spaces; off", func(a *App) string { return codeText(a.recCode()) }, func(a *App, s string) { a.set.RecCode = strings.ToLower(s) })},
		{head: "MIDI"},
		{label: "Input with this in its name", value: func(a *App) string { return a.midiPort() }, hint: "A edits",
			edit: promptSetting("MIDI input", "a part of the name of the input; the unit's is SP-404MKII, so 404", func(a *App) string { return a.midiPort() }, func(a *App, s string) { a.set.MidiPort = strings.ToLower(s) })},
		{label: "Control channel", value: func(a *App) string { return chanText(a.midiChan()) }, hint: "A edits",
			edit: promptSetting("MIDI control channel", "1 to 16, or off", func(a *App) string { return chanText(a.midiChan()) }, func(a *App, s string) { a.applyChannel(&a.set.MidiChan, s) })},
	}...)
	for i := 0; i < 4; i++ {
		i := i
		label := [4]string{"Live mode", "Start and stop a recording", "Next pattern becomes the one that plays", "Clear the next pattern"}[i]
		items = append(items, cfgItem{label: label, value: func(a *App) string { return a.midiMapText(i) }, hint: "A edits",
			edit: promptSetting("MIDI message for "+midiActions[i], "note 60 or cc 20 (a controller turned up past half), off for none", func(a *App) string { return a.midiMapText(i) },
				func(a *App, s string) { a.applyMidiMap(i, s) })})
	}
	if Terminal { // the handheld has no MIDI: its CONFIG stays as it is
		items = append(items,
			cfgItem{label: "Controller input with this in its name", value: func(a *App) string { return textOrOff(a.ctlPort()) }, hint: "A edits",
				edit: promptSetting("MIDI input of the controller", "a part of the name of its input (EC4), off for none", func(a *App) string { return textOrOff(a.ctlPort()) }, func(a *App, s string) { a.set.MidiCtlPort = strings.ToLower(s); a.restartCtl() })},
			cfgItem{label: "Controller channel", value: func(a *App) string { return chanText(a.ctlChan()) }, hint: "A edits",
				edit: promptSetting("MIDI channel of the controller", "1 to 16, or off", func(a *App) string { return chanText(a.ctlChan()) }, func(a *App, s string) { a.applyChannel(&a.set.MidiCtlChan, s) })},
			cfgItem{label: "Send its pattern to the unit", value: func(a *App) string {
				return map[bool]string{true: "yes (a Program Change)", false: "no (the controller sends it)"}[a.ctlSend()]
			}, hint: "< > change",
				change: func(a *App, by int) {
					a.set.MidiCtlSend = map[bool]string{true: "off", false: "on"}[a.ctlSend()]
					a.saveSettings()
				}},
			cfgItem{label: "Controller", value: func(a *App) string { return a.ctlStatus() }},
		)
	}
	items = append(items,
		cfgItem{label: "Pad volumes channel", value: func(a *App) string { return chanText(a.levelChan()) }, hint: "A edits",
			edit: promptSetting("MIDI channel of the pad volumes", "1 to 16, or off", func(a *App) string { return chanText(a.levelChan()) }, func(a *App, s string) { a.applyChannel(&a.set.MidiLevelChan, s) })},
		cfgItem{label: "MIDI", value: func(a *App) string { return a.midiLine() }},
		cfgItem{head: "COLOURS"},
		cfgItem{label: "Colour theme of the terminal", value: func(a *App) string { return textThemes[a.set.Theme%len(textThemes)].name }, hint: "< > change",
			change: func(a *App, by int) {
				a.set.Theme = (a.set.Theme + by + len(textThemes)) % len(textThemes)
				SetTextTheme(a.set.Theme)
				a.saveSettings()
			}},
	)
	if Terminal { // the debug log: a file for whoever looks at a problem
		items = append(items,
			cfgItem{head: "DEBUG"},
			cfgItem{label: "Debug log", value: func(a *App) string { return a.debugValue() }, hint: "< > change",
				change: func(a *App, by int) { a.setDebug(dbg.Mode() + by) },
				edit:   func(a *App) { a.setDebug(dbg.Mode() + 1) }},
			cfgItem{label: "The log is written to", value: func(a *App) string {
				if p := dbg.Path(); p != "" {
					return short2(p, 40)
				}
				return "(it is off)"
			}},
		)
	}
	if !Terminal { // the handheld does not record or meter: only a computer has this choice
		items = slices.DeleteFunc(items, func(it cfgItem) bool { return it.label == "Record and meter with" })
	}
	_ = rule
	if Terminal { // there is no PERF screen on a computer: its pad code has nothing to start
		items = slices.DeleteFunc(items, func(it cfgItem) bool { return it.label == "Live mode (the PERF screen)" })
	}
	return items
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// applyChannel keeps a typed MIDI channel: 1 to 16 or off.
func (a *App) applyChannel(dst *string, text string) {
	text = strings.ToLower(strings.TrimSpace(text))
	if n, err := strconv.Atoi(text); text != "off" && (err != nil || n < 1 || n > 16) {
		a.say("a MIDI channel is 1 to 16 (or off)", true)
		return
	}
	*dst = text
}

// applyMidiMap keeps a typed MIDI message: note 60, cc 20 or off.
func (a *App) applyMidiMap(i int, text string) {
	text = strings.ToLower(strings.TrimSpace(text))
	if text != "off" {
		if _, _, ok := parseMidiMap(text); !ok {
			a.say("a message is note 60 or cc 20 (0 to 127), or off", true)
			return
		}
	}
	a.set.MidiMap[i] = text
}

// recorderLine says how recording works, and whether this computer can do it.
func (a *App) recorderLine() string {
	tool := record.Tool(record.Default())
	if tool == "" {
		return "no recorder installed: ffmpeg (or sox) records the unit's audio"
	}
	return "records the unit's audio to a WAV with " + tool
}

// midiLine says whether this build can listen to MIDI.
func (a *App) midiLine() string { return a.midiStatus() }

// cfgCursor keeps the cursor on a row that is a setting, not a heading.
func (a *App) cfgMove(by int) {
	items := a.cfgItems()
	for i := a.cfgAt + by; i >= 0 && i < len(items); i += by {
		if items[i].head == "" {
			a.cfgAt = i
			return
		}
	}
}

func (a *App) configKey(k Key) {
	items := a.cfgItems()
	if a.cfgAt >= len(items) || items[a.cfgAt].head != "" {
		a.cfgMove(1)
		items = a.cfgItems()
	}
	it := items[a.cfgAt]
	switch k {
	case Up:
		a.cfgMove(-1)
	case Down:
		a.cfgMove(1)
	case Left, Right:
		if it.change != nil {
			by := 1
			if k == Left {
				by = -1
			}
			it.change(a, by)
		}
	case A:
		switch {
		case it.edit != nil:
			it.edit(a)
		case it.change != nil:
			it.change(a, 1)
		}
	case B:
		a.go_(Pads)
	case L1:
		a.cfgMove(-8)
	case R1:
		a.cfgMove(8)
	}
}

func (a *App) drawConfig(g Gfx) {
	items := a.cfgItems()
	rows := Rows - 4
	top := max(0, min(a.cfgAt-rows/2, len(items)-rows))
	lw := min(max(contentCols/2-2, 28), 44)
	for i := 0; i < rows && top+i < len(items); i++ {
		it := items[top+i]
		y := 2 + i
		if it.head != "" {
			g.Text(2, y, it.head, cAmber, true)
			g.HRule(2+len(it.head)+1, y, contentCols-4-len(it.head)-1, cLine)
			continue
		}
		fg := cText
		if top+i == a.cfgAt {
			g.Highlight(1, y, contentCols-2, 1)
			fg = cWhite
		}
		g.Text(3, y, fit(it.label, lw-1), fg, top+i == a.cfgAt)
		v := it.value(a)
		vc := cText
		if top+i == a.cfgAt {
			vc = cWhite
		}
		if it.colour != nil {
			if c, ok := it.colour(a); ok {
				vc = c
			}
		}
		if top+i == a.cfgAt && it.change != nil {
			v = "< " + v + " >"
		}
		g.Text(3+lw, y, fit(v, contentCols-lw-14), vc, top+i == a.cfgAt)
		if it.hint != "" {
			g.Text(contentCols-1-len(it.hint), y, it.hint, cDim, false)
		}
	}
}

// ---- setting the buses of the pads by their names

type busChange struct {
	pad int
	bus int
}

// busChanges are the pads of the bank (the marked ones among them, when some are marked; or the whole project) that are not on
// the bus their name says, each with that bus, and how many of the pads that were looked at the rule sorts to each bus.
func (a *App) busChanges(all bool) (changes []busChange, words1, words2, none, right int) {
	rule := a.busRule()
	lo, hi := a.bank*16, a.bank*16+16
	if all {
		lo, hi = 0, 160
	}
	marked := map[int]bool{}
	for _, m := range a.marks {
		marked[m] = true
	}
	for n := lo; n < hi; n++ {
		p := a.info.Pads[n]
		if !p.File || (!all && len(marked) > 0 && !marked[n]) {
			continue
		}
		want := int(rule.BusFor(p.Name))
		switch rule.Match(p.Name) {
		case 1:
			words1++
		case 2:
			words2++
		default:
			none++
		}
		if v, err := a.lab.PadParam(n, 0x7A); err == nil && int(v) == want {
			right++
			continue
		}
		changes = append(changes, busChange{n, want})
	}
	return
}

// busScanMenu asks before setting the bus of the pads that are not on the one their name says.
func (a *App) busScanMenu() {
	for _, all := range []bool{false} {
		changes, w1, w2, none, right := a.busChanges(all)
		scope := fmt.Sprintf("bank %c", 'A'+a.bank)
		if len(a.marks) > 0 {
			scope = fmt.Sprintf("the %d marked pads", len(a.marks))
		}
		if len(changes) == 0 {
			a.say(fmt.Sprintf("every pad of %s with a sample is on the bus its name says (%d by BUS 1 words, %d by BUS 2, %d by neither)", scope, w1, w2, none), false)
			if right == 0 {
				a.say("no pad with a sample in "+scope, false)
			}
			return
		}
		m := &menu{title: fmt.Sprintf("set the bus of %d pads by their names?", len(changes))}
		m.head(scope)
		add := func(l string, run func(int)) { m.items = append(m.items, menuItem{label: l, run: run}) }
		to := map[int]int{}
		for _, c := range changes {
			to[c.bus]++
		}
		add(fmt.Sprintf("Yes: %d to BUS 1, %d to BUS 2, %d to none", to[1], to[2], to[0]), func(int) { a.applyBusChanges(changes) })
		add("This whole project instead...", func(int) {
			all, _, _, _, _ := a.busChanges(true)
			a.confirmBusChanges(all)
		})
		add("No", func(int) {})
		m.head("EXAMPLES")
		for i, c := range changes {
			if i >= 6 {
				m.items = append(m.items, menuItem{label: fmt.Sprintf("... and %d more", len(changes)-6), header: true})
				break
			}
			m.items = append(m.items, menuItem{label: fmt.Sprintf("%s %s -> %s", a.label(c.pad), fit(a.info.Pads[c.pad].Name, 20), busName(c.bus)), run: func(int) {}})
		}
		a.menu = m
	}
}

func (a *App) confirmBusChanges(changes []busChange) {
	m := &menu{title: fmt.Sprintf("set the bus of %d pads of the project?", len(changes))}
	m.items = append(m.items, menuItem{label: "Yes", run: func(int) { a.applyBusChanges(changes) }}, menuItem{label: "No", run: func(int) {}})
	a.menu = m
}

func (a *App) applyBusChanges(changes []busChange) {
	pre := a.lab.Snapshot()
	n := 0
	for _, c := range changes {
		if err := a.lab.SetPadParam(c.pad, 0x7A, int32(c.bus)); err == nil {
			n++
		}
	}
	a.commitUndo(pre, A, 0)
	a.refresh()
	a.say(fmt.Sprintf("the bus of %d pads set by their names", n), false)
}

// configContext is the panel's part of the CONFIG screen: what the setting under the cursor is about, and, for the buses, what the
// rule makes of the pads of the bank in front.
func (a *App) configContext(g Gfx, x, y0, w, y1 int) {
	items := a.cfgItems()
	if a.cfgAt >= len(items) || items[a.cfgAt].head != "" {
		return
	}
	it := items[a.cfgAt]
	g.Text(x, y0, fit(it.label, w), cAmber, true)
	for i, l := range wrap(cfgNotes[it.label], w) {
		if y0+2+i >= y1 {
			break
		}
		g.Text(x, y0+2+i, l, cDim, false)
	}
	if strings.Contains(strings.ToLower(it.label), "bus") || strings.Contains(strings.ToLower(it.label), "words") {
		rule := a.busRule()
		y := y0 + 2 + len(wrap(cfgNotes[it.label], w)) + 1
		if y+2 > y1 {
			return
		}
		g.Text(x, y, fmt.Sprintf("BANK %c BY THE RULE", 'A'+a.bank), cDim, true)
		for i := 0; i < 16 && y+1+i < y1; i++ {
			n := a.bank*16 + i
			p := a.info.Pads[n]
			if !p.File {
				continue
			}
			now, _ := a.lab.PadParam(n, 0x7A)
			want := int(rule.BusFor(p.Name))
			mark := " "
			col := cText
			if int(now) != want {
				mark, col = ">", cAmber
			}
			g.Text(x, y+1+i, fit(fmt.Sprintf("%s %s %s %s", a.label(n), fit(p.Name, 10), mark, busName(want)), w), col, false)
		}
	}
}

// cfgNotes say what a setting is for.
var cfgNotes = map[string]string{
	"Words for samples on BUS 1":                 "A word matches a word of the pad's name (Kick_125, OHat, DrumLoop, 14_Perc1), a longer one by its start. Where a name has words of both buses, BUS 1 wins.",
	"Words for samples on BUS 2":                 "A word matches a word of the pad's name (Kick_125, OHat, DrumLoop, 14_Perc1), a longer one by its start. Where a name has words of both buses, BUS 1 wins.",
	"Set the bus when a sample is put on a pad":  "When a sample is assigned to a pad its bus is set by the rule.",
	"Set the buses of this bank by the names...": "Sets the bus of the pads of the bank (the marked ones, when some are marked) by the rule; it asks first.",
	"Record into folder":                         "Where recordings of the unit's audio are kept.",
	"Input with this in its name":                "A part of the name of the audio or MIDI input to use; the unit's is SP-404MKII, so 404.",
	"Live mode (the PERF screen)":                "Hit those pads in turn on the unit, in any bank, within four seconds. A pad that repeats needs a sample.",
	"Start and stop a recording":                 "Hit those pads in turn on the unit, within four seconds; or send the MIDI message below.",
	"Control channel":                            "The MIDI channel that controls the program from a controller (1-16, or off).",
	"Pad volumes channel":                        "The channel of the 16 pad volumes (turns and pushes of a controller).",
	"Colour theme of the terminal":               "The colours of the screens on a computer's terminal.",
}

// wrap breaks text into lines of at most w characters, at spaces.
func wrap(text string, w int) []string {
	var lines []string
	cur := ""
	for _, word := range strings.Fields(text) {
		if cur != "" && len(cur)+1+len(word) > w {
			lines = append(lines, cur)
			cur = ""
		}
		if cur != "" {
			cur += " "
		}
		cur += word
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}
