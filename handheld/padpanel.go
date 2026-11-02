package handheld

import (
	"fmt"

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
	}
	a.pp = pp
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
		v := a.settingValue(pp.pad, st)
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
			if st.fast && repeat >= 14 {
				acc = 8
			} else if st.fast && repeat >= 6 {
				acc = 4
			}
			v += d * st.step * acc
		}
		a.setSetting(pp.pad, st, v)
	case A:
		st := rows[*r]
		v := a.settingValue(pp.pad, st)
		switch st.kind {
		case kSwitch, kFlag:
			v = 1 - v
		case kChoice:
			v = st.lo + (v-st.lo+1)%(st.hi-st.lo+1)
		default:
			v = st.def // a fader goes back to its default
		}
		a.setSetting(pp.pad, st, v)
	case X:
		a.playPad(pp.pad)
	case B:
		a.pp = nil
	}
}

// ---- drawing

func (a *App) drawPadPanel(c *canvas) {
	pp := a.pp
	has := a.info.Pads[pp.pad].File
	x0, y0, w, h := 16, 34, contentW-32, 392
	c.rect(x0, y0, w, h, cPanel2)
	c.box(x0, y0, w, h, 2, cAmber)
	title := fmt.Sprintf("PAD %s", a.label(pp.pad))
	c.text(x0+14, y0+10, title, cAmber, true)
	if has {
		c.text(x0+14+textW(title)+10, y0+10, fit(a.info.Pads[pp.pad].Name, 22), cText, false)
	}
	tx := x0 + w - 14
	for i := len(padPageNames) - 1; i >= 0; i-- {
		if !has && i < 2 {
			continue
		}
		name := padPageNames[i]
		tx -= textW(name) + 14
		if i == pp.page {
			c.rect(tx-6, y0+6, textW(name)+12, 22, cAmber)
			c.text(tx, y0+10, name, cBg, true)
		} else {
			c.text(tx, y0+10, name, cDim, false)
		}
	}
	c.rect(x0+10, y0+34, w-20, 1, cLine)
	if pp.page == 2 {
		a.drawToolsList(c, x0+10, y0+44, w-20, h-54, pp.tools)
		return
	}
	rows := padPages[pp.page]
	rh := 36
	if len(rows) > 7 {
		rh = 25
	}
	for i, st := range rows {
		y := y0 + 46 + i*rh
		v := a.settingValue(pp.pad, st)
		if i == pp.row[pp.page] {
			c.rect(x0+8, y-4, w-16, rh-2, cAmberD)
			c.rect(x0+8, y-4, 3, rh-2, cAmber)
		}
		c.text(x0+22, y+2, st.label, cText, i == pp.row[pp.page])
		bx, bw := x0+150, 190
		vt := settingText(st, v)
		switch st.kind {
		case kFader:
			c.rect(bx, y+4, bw, 10, cPanel)
			frac := float64(v-st.lo) / float64(st.hi-st.lo)
			dfrac := float64(st.def-st.lo) / float64(st.hi-st.lo)
			if st.lo < 0 || st.id == 0x78 { // bipolar: from the middle out
				mid := bx + int(dfrac*float64(bw))
				end := bx + int(frac*float64(bw))
				c.rect(min(mid, end), y+4, abs(end-mid)+1, 10, cAmber)
			} else {
				c.rect(bx, y+4, int(frac*float64(bw)), 10, cAmber)
			}
			c.rect(bx+int(dfrac*float64(bw)), y+1, 2, 16, cDim) // where the default is
			c.rect(bx+int(frac*float64(bw))-1, y, 4, 18, cWhite)
		case kSwitch, kFlag:
			pill, fg := cPanel, cDim
			if v != 0 {
				pill, fg = cAmber, cBg
			}
			c.rect(bx, y, 56, 18, pill)
			c.text(bx+(56-textW(vt))/2, y+1, vt, fg, true)
			vt = ""
		case kChoice:
			c.text(bx, y+1, "< "+vt+" >", cAmber, i == pp.row[pp.page])
			vt = ""
		}
		if vt != "" {
			c.text(bx+bw+14, y+1, vt, cAmber, true)
		}
	}
	hint := "<> change   A reset/toggle   X hear the pad   L1 R1 page   B close"
	c.text(x0+14, y0+h-22, fit(hint, (w-28)/CellW), cDim, false)
}

// drawToolsList draws the tools page: the menu's items in the panel's frame.
func (a *App) drawToolsList(c *canvas, x, y, w, h int, m *menu) {
	m.settle()
	rh := 20
	vis := min(len(m.items), h/rh)
	top := max(0, min(m.cur-vis/2, len(m.items)-vis))
	for r := 0; r < vis; r++ {
		i := top + r
		it := m.items[i]
		yy := y + r*rh
		if it.header {
			c.rect(x, yy+rh/2-1, w, 1, cLine)
			c.rect(x+8, yy-1, textW(it.label)+10, rh-2, cPanel2)
			c.text(x+12, yy, it.label, cDim, true)
			continue
		}
		fg := cText
		if i == m.cur {
			c.rect(x, yy-2, w, rh, cAmberD)
			c.rect(x, yy-2, 3, rh, cAmber)
			fg = cWhite
		}
		c.text(x+14, yy, it.label, fg, i == m.cur)
		if len(it.vals) > 0 {
			v := it.vals[it.val]
			if i == m.cur {
				v = "< " + v + " >"
			}
			c.text(x+w-10-textW(v), yy, v, cAmber, i == m.cur)
		}
	}
}

var _ = formats.SampleRate
