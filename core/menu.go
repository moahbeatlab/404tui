package core

import (
	"fmt"
	"strings"

	"github.com/moahbeatlab/404tui/formats"
)

// menu is a list of actions over a screen. With four keys a screen cannot have a key for each action, so a
// screen has a menu: Up and Down choose, Left and Right change the value of an item that has one, A runs it, B closes.
type menu struct {
	title   string
	items   []menuItem
	cur     int
	onClose func()                          // what B does besides closing the menu (a tool throws its trial away)
	parent  *menu                           // the menu this one was opened from: B goes back to it, and B there closes
	foot    func(g Gfx, col, row, cols int) // drawn under the items (a keyboard), footH rows high
	footH   int                             // in rows
	// tabs: the headings split the menu into pages (as the pad panel), L1 and R1 turn them. items is the page in front;
	// full is every item of the menu, so an index given when the menu was made still finds its item.
	tabs   []string
	groups [][]menuItem
	curs   []int
	tab    int
	full   []menuItem
}

// tabify makes a page of each group of items under a heading; the heading becomes the name of the page.
func (m *menu) tabify() {
	m.full = m.items
	m.tabs, m.groups, m.curs = nil, nil, nil
	for i := 0; i < len(m.full); i++ {
		if m.full[i].tab {
			m.tabs = append(m.tabs, m.full[i].label)
			end := i + 1
			for end < len(m.full) && !m.full[end].tab {
				end++
			}
			m.groups = append(m.groups, m.full[i+1:end])
			m.curs = append(m.curs, 0)
		}
	}
	m.tab = 0
	m.items = m.groups[0]
	m.cur = 0
}

// setTab shows page i, with the cursor where it was left.
func (m *menu) setTab(i int) {
	m.curs[m.tab] = m.cur
	m.tab = (i + len(m.groups)) % len(m.groups)
	m.items = m.groups[m.tab]
	m.cur = m.curs[m.tab]
}

// at is the item numbered as when the menu was made.
func (m *menu) at(i int) *menuItem {
	if m.full != nil {
		return &m.full[i]
	}
	return &m.items[i]
}

type menuItem struct {
	label    string
	vals     []string // when set, Left and Right choose from them
	val      int
	run      func(val int) // what A does
	onChange func(val int) // what Left and Right do to the value, as it changes
	header   bool          // a heading for the items below it: not chosen, only read
	tab      bool          // a heading that is also a tab (the menu has tabs)
	keepOpen bool
}

func (m *menu) item() *menuItem { m.settle(); return &m.items[m.cur] }

// tabHead adds a heading that starts a tab: with tabs the menu shows one tab at a time, the headings inside it stay headings.
func (m *menu) tabHead(label string) {
	m.items = append(m.items, menuItem{label: label, header: true, tab: true})
}

// head adds a heading to the menu.
func (m *menu) head(label string) { m.items = append(m.items, menuItem{label: label, header: true}) }

// step moves the cursor d places (1 or -1), over the headings, round the ends.
func (m *menu) step(d int) {
	for i := 0; i < len(m.items); i++ {
		m.cur = (m.cur + d + len(m.items)) % len(m.items)
		if !m.items[m.cur].header {
			return
		}
	}
}

// settle puts the cursor on an item that can be chosen (the menu opens on one).
func (m *menu) settle() {
	if len(m.items) > 0 && m.items[m.cur].header {
		m.step(1)
	}
}

func (a *App) menuKey(k Key) {
	m := a.menu
	m.settle()
	switch k {
	case Up:
		m.step(-1)
	case Down:
		m.step(1)
	case Left, Right:
		it := m.item()
		if len(it.vals) > 0 {
			d := 1
			if k == Left {
				d = len(it.vals) - 1
			}
			it.val = (it.val + d) % len(it.vals)
			if it.onChange != nil {
				it.onChange(it.val)
			}
		}
	case L1, R1:
		if m.tabs != nil {
			d := 1
			if k == L1 {
				d = -1
			}
			m.setTab(m.tab + d)
			return
		}
		// a page: ten values of an item that has many (a tempo, a pad), else eight rows of the list
		it := m.item()
		d := 1
		if k == L1 {
			d = -1
		}
		if len(it.vals) > 12 {
			it.val = ((it.val+d*10)%len(it.vals) + len(it.vals)) % len(it.vals)
			if it.onChange != nil {
				it.onChange(it.val)
			}
		} else {
			m.cur = max(0, min(m.cur+d*8, len(m.items)-1))
			if m.items[m.cur].header {
				m.step(d)
			}
		}
	case B:
		if m.onClose != nil {
			m.onClose()
		}
		a.menu = m.parent // the menu it was opened from, or none
	case A:
		it := *m.item()
		if !it.keepOpen {
			a.menu = nil
		}
		if it.run != nil {
			it.run(it.val)
		}
		if Terminal && a.menu != nil && a.menu != m && a.menu.parent == nil && a.menu.title != m.title { // (a computer's Esc; the handheld's B closes)
			a.menu.parent = m // a menu that came out of this one: B comes back here
		}
	}
}

// menuLabel is an item's label as a keyboard reads it: the keys it names are the keyboard's.
func menuLabel(l string) string {
	if !Terminal {
		return l
	}
	return menuKeys.Replace(l)
}

var menuKeys = strings.NewReplacer("(B throws it away)", "(Esc throws it away)", "(Select+B)", "(t)", "(Y+B)", "(Enter)", "(Y+A)", "(f)")

func (a *App) drawMenu(g Gfx) {
	m := a.menu
	m.settle()
	wc := min(max(contentCols*3/4, 41), 64)
	wc = min(wc, contentCols-2)
	tabRows := 0
	if m.tabs != nil {
		tabRows = 1
	}
	vis := min(len(m.items), max(Rows-3-3-tabRows-m.footH, 3))
	top := max(0, min(m.cur-vis/2, len(m.items)-vis))
	hr := 3 + tabRows + vis + m.footH // the rows of the dialog: a blank one at the top and the bottom for the frame
	x := (contentCols - wc) / 2
	y := 1 + max((Rows-3-hr)/2, 0)
	g.Fill(x, y, wc, hr, cPanel2)
	g.Frame(x, y, wc, hr, cAmber)
	title := strings.ToUpper(m.title)
	if len(m.items) > vis {
		title += fmt.Sprintf("  %d/%d", m.cur+1, len(m.items))
	}
	g.Text(x+2, y+1, fit(title, wc-6), cAmber, true)
	if m.tabs != nil {
		tx := x + 2
		for i, name := range m.tabs {
			if i == m.tab {
				g.Fill(tx-1, y+2, len(name)+2, 1, cAmber)
				g.Text(tx, y+2, name, cBg, true)
			} else {
				g.Text(tx, y+2, name, cDim, false)
			}
			tx += len(name) + 3
		}
	}
	iy := y + 2 + tabRows
	for r := 0; r < vis; r++ {
		i := top + r
		it := m.items[i]
		yy := iy + r
		if it.header {
			g.HRule(x+1, yy, wc-2, cLine)
			g.Fill(x+1, yy, len(it.label)+2, 1, cPanel2)
			g.Text(x+2, yy, it.label, cDim, true)
			continue
		}
		fg := cText
		if i == m.cur {
			g.Highlight(x+1, yy, wc-2, 1)
			fg = cWhite
		}
		vw := 0
		if len(it.vals) > 0 {
			v := it.vals[it.val]
			if i == m.cur {
				v = "< " + v + " >"
			}
			vw = len(v) + 1
			g.Text(x+wc-2-len(v), yy, v, cAmber, i == m.cur)
		}
		g.Text(x+2, yy, fit(menuLabel(it.label), wc-4-vw), fg, i == m.cur)
	}
	if m.foot != nil {
		m.foot(g, x, iy+vis, wc)
	}
	if top > 0 {
		g.Text(x+wc-4, y+1, "^", cAmber, true)
	}
	if top+vis < len(m.items) {
		g.Text(x+wc-4, y+hr-2, "v", cAmber, true)
	}
}

// carry is a pad picked up to be put down on another one.
type carry struct {
	from int
	copy bool
}

// openPadMenu is the actions on a pad, as a menu of their own.
func (a *App) openPadMenu() { a.menu = a.padToolsMenu(a.padN()) }

// padToolsMenu is the actions on a pad: what is done with its sample (the sample tools), and the pad itself.
func (a *App) padToolsMenu(n int) *menu {
	has := a.info.Pads[n].File
	tg := a.targetsFor(n) // the marked pads, when some are marked: the bulk of the tools below
	bulk := len(tg) > 1
	count := ""
	if bulk {
		count = fmt.Sprintf(" (%d marked pads)", len(tg))
	}
	m := &menu{title: "pad " + a.label(n)}
	add := func(l string, run func(int)) { m.items = append(m.items, menuItem{label: l, run: run}) }
	m.head("SAMPLE")
	add("Pick a sample...", func(int) { a.go_(Files) })
	if has {
		add("Slice...", func(int) { a.go_(Slice) })
		add("Chords & arps...", func(int) { a.openArpMenu(n) })
		add("Arp from this one sample (scales, chords)...", func(int) { a.openOnePadMenu(n) })
		add("Keyboard from it...", func(int) { a.openKeyboardMenu(n) })
		each := func(op string, arg int) {
			if bulk {
				a.transformMany(tg, op, arg)
				return
			}
			a.transform(n, op, arg)
		}
		add("Rename..."+map[bool]string{true: count, false: ""}[bulk], func(int) { a.renamePad(n) })
		add("Reverse"+count, func(int) { each("reverse", 0) })
		add("Normalize"+count, func(int) { each("normalize", 0) })
		m.items = append(m.items, menuItem{label: "Pitch" + count, vals: []string{"-12", "-5", "-2", "-1", "+1", "+2", "+5", "+12"}, val: 4,
			run: func(v int) { each("pitch", []int{-12, -5, -2, -1, 1, 2, 5, 12}[v]) }})
		m.head("THE PAD")
		add("Pattern with this pad...", func(int) { a.go_(Seq) })
		add("Move or swap to...", func(int) { a.carry = &carry{from: n}; a.say("choose the pad, A puts it down", false) })
		add("Copy to...", func(int) { a.carry = &carry{from: n, copy: true}; a.say("choose the pad, A puts it down", false) })
		add("Clear"+map[bool]string{true: count, false: " the pad"}[bulk], func(int) {
			cleared := 0
			for _, p := range tg {
				if err := a.lab.ClearPad(p); err == nil {
					delete(a.pcm, p)
					cleared++
				}
			}
			a.refresh()
			a.say(fmt.Sprintf("cleared %d pads", cleared), false)
		})
	}
	m.head(fmt.Sprintf("BANK %c", 'A'+a.bank))
	add("Find a pad by name...", func(int) { a.findPad() })
	add("Set up the pads that were never set up", func(int) { a.setupPads(false) })
	add("Switches for every pad of the bank...", func(int) { a.bankSwitchesMenu() })
	add("Set the buses of the bank by the names...", func(int) { a.busScanMenu() })
	add("Export the bank to a folder", func(int) { a.exportBank() })
	add("Restore pads from an export...", func(int) { a.restoreMenu() })
	m.head(fmt.Sprintf("MARKED PADS (%d)", len(a.marks)))
	add("Mark every pad of this bank with a sample", func(int) { a.markBank() })
	add("Arp the marked pads...", func(int) { a.openMarkedArpMenu() })
	add("Clear the marks", func(int) { a.clearMarks() })
	return m
}

// transform makes a changed copy of a pad's sample and puts it back on the pad.
func (a *App) transform(n int, op string, arg int) {
	if err := a.transformPad(n, op, arg); err != nil {
		a.say(err.Error(), true)
		return
	}
	a.sl = nil
	a.refresh()
	a.say(fmt.Sprintf("%s: %s %s", a.label(n), op, map[bool]string{true: fmt.Sprintf("%+d", arg), false: ""}[op == "pitch"]), false)
}

// transformMany does it on every pad of a bulk selection and says how many it changed.
func (a *App) transformMany(pads []int, op string, arg int) {
	done := 0
	for _, n := range pads {
		if err := a.transformPad(n, op, arg); err != nil {
			a.say(err.Error(), true)
			continue
		}
		done++
	}
	a.sl = nil
	a.refresh()
	a.say(fmt.Sprintf("%s on %d pads", op, done), false)
}

func (a *App) transformPad(n int, op string, arg int) error {
	id, ok := a.padSample(n)
	if !ok {
		return fmt.Errorf("nothing on %s", a.label(n))
	}
	var nid int
	var err error
	switch op {
	case "reverse":
		nid, err = a.lab.Reversed(id)
	case "normalize":
		nid, err = a.lab.Normalize(id, -0.3)
	case "pitch":
		nid, err = a.lab.Pitched(id, arg)
	}
	if err != nil {
		return err
	}
	name := a.info.Pads[n].Name
	if err := a.lab.Assign(n, nid, name); err != nil {
		return err
	}
	a.lab.Free(nid)
	delete(a.pcm, n)
	return nil
}

func (a *App) carryKey(k Key) {
	switch k {
	case Up, Down, Left, Right:
		cc := a.carry
		a.carry = nil
		a.padsKey(k)
		a.carry = cc
	case B:
		a.carry = nil
		a.say("cancelled", false)
	case A:
		c := a.carry
		a.carry = nil
		to := a.padN()
		if to == c.from {
			return
		}
		if c.copy {
			id, ok := a.padSample(c.from)
			if !ok {
				return
			}
			if err := a.lab.Assign(to, id, a.info.Pads[c.from].Name); err != nil {
				a.say(err.Error(), true)
				return
			}
			delete(a.pcm, to)
			a.say(fmt.Sprintf("%s copied to %s", a.label(c.from), a.label(to)), false)
		} else {
			if err := a.lab.MovePad(c.from, to); err != nil {
				a.say(err.Error(), true)
				return
			}
			delete(a.pcm, c.from)
			delete(a.pcm, to)
			a.say(fmt.Sprintf("%s moved to %s", a.label(c.from), a.label(to)), false)
		}
		a.sl = nil
		a.refresh()
	}
}

var _ = formats.PPQ
