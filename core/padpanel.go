package core

import (
	"fmt"
	"sort"

	"github.com/moahbeatlab/404tui/formats"
)

// The pad panel (A on a pad): the settings of the pad first, each a horizontal fader, then the tools. Three pages, L1 and R1
// turn between them: SOUND (level, pan, pitch, fine pitch, stretch, sample BPM), SHAPE (attack, hold, release and the
// switches: play mode, gate, loop, one shot, fixed velocity, vinyl, BPM sync, mute group, bus), TOOLS (the actions on the
// sample and on the pad). X plays the pad with the settings as they are, so a change can be heard.

type padPanel struct {
	pad, page int
	row       [2]int // the row of each settings page
	tools     *menu
}

const (
	kFader = iota
	kSwitch
	kChoice
	kFlag // a bit of the mode flags
)

type padSetting struct {
	label       string
	id          byte
	kind        int
	lo, hi, def int32
	step        int32
	fast        bool // a held key runs faster
	bit         uint
	names       []string // the names of a choice's values (index = value - lo)
	show        func(v int32) string
}

func pct100(v int32) string { return fmt.Sprintf("%d%%", v/100) }

var padPages = [2][]padSetting{
	{
		{label: "Level", id: 0x69, lo: 0, hi: 127, def: 127, step: 1, fast: true, show: func(v int32) string { return fmt.Sprint(v) }},
		{label: "Pan", id: 0x78, lo: 13, hi: 115, def: 64, step: 1, fast: true, show: func(v int32) string {
			switch {
			case v == 64:
				return "C"
			case v < 64:
				return fmt.Sprintf("L%d", (64-v)*100/51)
			}
			return fmt.Sprintf("R%d", (v-64)*100/51)
		}},
		{label: "Pitch", id: 0x73, lo: -12, hi: 12, def: 0, step: 1, show: func(v int32) string { return fmt.Sprintf("%+d st", v) }},
		{label: "Fine pitch", id: 0x74, lo: -100, hi: 100, def: 0, step: 1, fast: true, show: func(v int32) string { return fmt.Sprintf("%+d ct", v) }},
		{label: "Time stretch", id: 0x76, lo: 5000, hi: 15000, def: 10000, step: 100, fast: true, show: pct100},
		{label: "Sample BPM", id: 0x6F, lo: 4000, hi: 20000, def: 12000, step: 100, fast: true, show: func(v int32) string { return fmt.Sprintf("%d", v/100) }},
	},
	{
		{label: "Attack", id: 0x7D, lo: 0, hi: 127, def: 0, step: 1, fast: true, show: func(v int32) string { return fmt.Sprint(v) }},
		{label: "Hold", id: 0x7E, lo: 1, hi: 100, def: 100, step: 1, fast: true, show: func(v int32) string { return fmt.Sprintf("%d%%", v) }},
		{label: "Release", id: 0x7F, lo: 0, hi: 127, def: 0, step: 1, fast: true, show: func(v int32) string { return fmt.Sprint(v) }},
		{label: "Play mode", id: 0x75, kind: kChoice, lo: 0, hi: 3, def: 0, names: []string{"forward", "reverse", "ping-pong", "rev ping-pong"}},
		{label: "Gate", id: 0x6A, kind: kSwitch, lo: 0, hi: 1},
		{label: "Loop", id: 0x6B, kind: kSwitch, lo: 0, hi: 1},
		{label: "One shot", id: 0x70, kind: kFlag, bit: 5},
		{label: "Fixed velocity", id: 0x70, kind: kFlag, bit: 0},
		{label: "Vinyl", id: 0x77, kind: kSwitch, lo: 0, hi: 1},
		{label: "BPM sync", id: 0x6E, kind: kSwitch, lo: 0, hi: 1},
		{label: "Mute group", id: 0x6D, kind: kChoice, lo: 0, hi: 10, names: []string{"off", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}},
		{label: "Bus", id: 0x7A, kind: kChoice, lo: 0, hi: 2, names: []string{"off", "BUS 1", "BUS 2"}},
	},
}

var padPageNames = []string{"SOUND", "SHAPE", "TOOLS"}

// value reads a setting of the pad (0 for one the file does not hold).
func (a *App) settingValue(pad int, st padSetting) int32 {
	v, err := a.lab.PadParam(pad, st.id)
	if err != nil {
		return st.def
	}
	if st.kind == kFlag {
		return (v >> st.bit) & 1
	}
	if st.kind != kSwitch && (v < st.lo || v > st.hi) { // a pad that was never set up has zeros here: the default
		return st.def
	}
	return v
}

func (a *App) setSetting(pad int, st padSetting, v int32) {
	if st.kind == kFlag {
		cur, err := a.lab.PadParam(pad, st.id)
		if err != nil {
			return
		}
		cur &^= 1 << st.bit
		cur |= (v & 1) << st.bit
		v = cur
	}
	if st.kind != kFlag {
		v = min(max(v, st.lo), st.hi)
	}
	if err := a.lab.SetPadParam(pad, st.id, v); err != nil {
		a.say(err.Error(), true)
		return
	}
	a.refresh()
}

func settingText(st padSetting, v int32) string {
	switch st.kind {
	case kSwitch, kFlag:
		if v != 0 {
			return "ON"
		}
		return "off"
	case kChoice:
		if i := int(v - st.lo); i >= 0 && i < len(st.names) {
			return st.names[i]
		}
		return fmt.Sprint(v)
	}
	return st.show(v)
}

func (a *App) openPadPanel() {
	n := a.padN()
	pp := &padPanel{pad: n}
	pp.tools = a.padToolsMenu(n)
	if !a.info.Pads[n].File {
		pp.page = 2 // an empty pad has no settings: the tools (pick a sample...)
	} else {
		a.pp = pp
		fixed := 0
		for _, t := range a.padTargets() {
			f, _ := a.lab.SetupPad(t)
			fixed += f
		}
		if fixed > 0 {
			a.refresh() // a pad that was never set up: the pan at its centre, the hold at 100% ...
			a.say(fmt.Sprintf("%s was never set up: pan centre, hold 100%%", a.label(n)), false)
		}
	}
	a.pp = pp
}

// padTargets are the pads a change in the panel goes to: the marked pads that have a sample when some are marked (bulk editing),
// else the pad in front.
func (a *App) padTargets() []int { return a.targetsFor(a.pp.pad) }

// targetsFor is padTargets for a pad in front.
func (a *App) targetsFor(front int) []int {
	var out []int
	for _, p := range a.marks {
		if a.info.Pads[p].File {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []int{front}
	}
	sort.Ints(out)
	return out
}

// refPad is the pad whose values the panel shows: the pad in front when it is one of the targets, else the first target.
func (a *App) refPad(targets []int) int {
	for _, t := range targets {
		if t == a.pp.pad {
			return t
		}
	}
	return targets[0]
}

// ppKey is a key in the pad panel.
func (a *App) ppKey(k Key, repeat int) {
	pp := a.pp
	has := a.info.Pads[pp.pad].File
	if has && (k == L1 || k == R1) {
		d := 1
		if k == L1 {
			d = -1
		}
		pp.page = (pp.page + 3 + d) % 3
		return
	}
	if pp.page == 2 { // the tools are a menu, run by the menu's own keys
		a.menu = pp.tools
		a.menuKey(k)
		switch {
		case a.menu == pp.tools: // moved about, or an item that stays open
			a.menu = nil
		default: // closed (B, or an action done) or an action opened its own menu
			a.pp = nil
		}
		return
	}
	rows := padPages[pp.page]
	r := &pp.row[pp.page]
	switch k {
	case Up:
		*r = (*r + len(rows) - 1) % len(rows)
	case Down:
		*r = (*r + 1) % len(rows)
	case Left, Right:
		st := rows[*r]
		targets := a.padTargets()
		ref := a.refPad(targets)
		v := a.settingValue(ref, st)
		d := int32(1)
		if k == Left {
			d = -1
		}
		switch st.kind {
		case kSwitch, kFlag:
			v = 1 - v
		case kChoice:
			n := st.hi - st.lo + 1
			v = st.lo + ((v-st.lo+d)%n+n)%n
		default:
			acc := int32(1)
			if st.fast {
				acc = int32(accel(repeat))
			}
			v += d * st.step * acc
		}
		a.setEach(targets, ref, st, v, false)
	case A:
		st := rows[*r]
		targets := a.padTargets()
		ref := a.refPad(targets)
		v := a.settingValue(ref, st)
		switch st.kind {
		case kSwitch, kFlag:
			v = 1 - v
		case kChoice:
			v = st.lo + (v-st.lo+1)%(st.hi-st.lo+1)
		default:
			v = st.def // a fader goes back to its default
		}
		a.setEach(targets, ref, st, v, true) // every pad to the same, the default or the switch
	case X:
		a.playPad(pp.pad)
	case B:
		a.pp = nil
	}
}

// setEach puts a setting on every target: a fader moves each pad by the same amount (so that the differences between the pads are
// kept, a pad at its end stays there), a switch or a choice is set to the same on all of them. v is the new value of the ref pad.
func (a *App) setEach(targets []int, ref int, st padSetting, v int32, absolute bool) {
	old := a.settingValue(ref, st)
	for _, t := range targets {
		nv := v
		if st.kind == kFader && !absolute {
			nv = a.settingValue(t, st) + (v - old)
			if t == ref {
				nv = v
			}
		}
		a.setSetting(t, st, nv)
	}
}

// ---- drawing

func (a *App) drawPadPanel(g Gfx) {
	pp := a.pp
	has := a.info.Pads[pp.pad].File
	wc, hr := contentCols-2, Rows-3 // the dialog in cells; its first and last row are blank, for the frame
	if pp.page != 2 {               // the settings: as many rows as they need, a blank one between them when there is room
		n := len(padPages[pp.page])
		hr = min(hr, 5+n*(1+btoi(n*2 <= hr-5)))
	}
	x0, y0 := 1, 1
	g.Fill(x0, y0, wc, hr, cPanel2)
	g.Frame(x0, y0, wc, hr, cAmber)
	targets := a.padTargets()
	ref := a.refPad(targets)
	title := fmt.Sprintf("PAD %s", a.label(ref))
	if len(targets) > 1 {
		title = fmt.Sprintf("%d MARKED PADS", len(targets))
	}
	ty := y0 + 1
	g.Text(x0+2, ty, title, cAmber, true)
	if len(targets) > 1 {
		g.Text(x0+2+len(title)+1, ty, "all change", cText, false)
	} else if has {
		g.Text(x0+2+len(title)+1, ty, fit(a.info.Pads[pp.pad].Name, 22), cText, false)
	}
	tx := x0 + wc - 2
	for i := len(padPageNames) - 1; i >= 0; i-- {
		if !has && i < 2 {
			continue
		}
		name := padPageNames[i]
		tx -= len(name) + 2
		if i == pp.page {
			g.Fill(tx-1, ty, len(name)+2, 1, cAmber)
			g.Text(tx, ty, name, cBg, true)
		} else {
			g.Text(tx, ty, name, cDim, false)
		}
	}
	g.HRule(x0+1, ty+1, wc-2, cLine)
	if pp.page == 2 {
		a.drawToolsList(g, x0+1, ty+2, wc-2, hr-5, pp.tools)
		return
	}
	rows := padPages[pp.page]
	pitch := 1 + btoi(len(rows)*2 <= hr-5) // a row for a setting, and a blank one between them when there is room
	bx := x0 + 20
	bw := max(min(wc-20-14, 40), 12)
	for i, st := range rows {
		y := ty + 2 + i*pitch
		v := a.settingValue(ref, st)
		mixed := false
		for _, t := range targets {
			mixed = mixed || a.settingValue(t, st) != v
		}
		fg := cText
		if i == pp.row[pp.page] {
			g.Highlight(x0+1, y, wc-2, 1)
			fg = cWhite
		}
		if st.id == 0x6A || st.id == 0x6B { // gate and loop: the letter, lit when on (as on the pads)
			g.Badge(x0+2, y, st.label[:1], v != 0)
			g.Text(x0+4, y, st.label, fg, i == pp.row[pp.page])
		} else {
			g.Text(x0+2, y, st.label, fg, i == pp.row[pp.page])
		}
		vt := settingText(st, v)
		switch st.kind {
		case kFader:
			frac := float64(v-st.lo) / float64(st.hi-st.lo)
			dfrac := float64(st.def-st.lo) / float64(st.hi-st.lo)
			g.Fader(bx, y, bw, frac, dfrac, st.lo < 0 || st.id == 0x78)
		case kSwitch, kFlag:
			pill, pf := cPanel, cDim
			if v != 0 {
				pill, pf = cAmber, cBg
			}
			g.Fill(bx, y, 7, 1, pill)
			g.Text(bx+(7-len(vt))/2, y, vt, pf, true)
			vt = ""
		case kChoice:
			g.Text(bx, y, "< "+vt+" >", cAmber, i == pp.row[pp.page])
			vt = ""
		}
		if mixed { // the marked pads differ: the value shown is the first one's
			vt += " *"
		}
		if vt != "" {
			g.Text(bx+bw+1, y, vt, cAmber, true)
		}
	}
	hint := tk("<> change   A reset/toggle   X hear the pad   L1 R1 page   B close")
	g.Text(x0+2, y0+hr-2, fit(hint, wc-4), cDim, false)
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// drawToolsList draws the tools page: the menu's items in the panel's frame (x, y, w, h in cells).
func (a *App) drawToolsList(g Gfx, x, y, w, h int, m *menu) {
	m.settle()
	vis := min(len(m.items), h)
	top := max(0, min(m.cur-vis/2, len(m.items)-vis))
	for r := 0; r < vis; r++ {
		i := top + r
		it := m.items[i]
		yy := y + r
		if it.header {
			g.HRule(x, yy, w, cLine)
			g.Fill(x, yy, len(it.label)+2, 1, cPanel2)
			g.Text(x+1, yy, it.label, cDim, true)
			continue
		}
		fg := cText
		if i == m.cur {
			g.Highlight(x, yy, w, 1)
			fg = cWhite
		}
		vw := 0
		if len(it.vals) > 0 {
			v := it.vals[it.val]
			if i == m.cur {
				v = "< " + v + " >"
			}
			vw = len(v) + 1
			g.Text(x+w-1-len(v), yy, v, cAmber, i == m.cur)
		}
		g.Text(x+2, yy, fit(it.label, w-3-vw), fg, i == m.cur)
	}
}

var _ = formats.SampleRate

// padValue is a setting of a pad as it counts: a value outside what the unit allows (zero in a pad never set up) is the default.
func (a *App) padValue(n int, id byte, def int32) int32 {
	for _, page := range padPages {
		for _, st := range page {
			if st.id == id && st.kind != kFlag {
				v := a.settingValue(n, st)
				return v
			}
		}
	}
	if v, err := a.lab.PadParam(n, id); err == nil {
		return v
	}
	return def
}
