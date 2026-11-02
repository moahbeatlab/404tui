package handheld

import (
	"fmt"
	"strings"

	"github.com/moahbeatlab/404tui/formats"
)

// menu is a list of actions over a screen. With four keys a screen cannot have a key for each action, so a
// screen has a menu: Up and Down choose, Left and Right change the value of an item that has one, A runs it, B closes.
type menu struct {
	title string
	items []menuItem
	cur   int
}

type menuItem struct {
	label    string
	vals     []string // when set, Left and Right choose from them
	val      int
	run      func(val int) // what A does
	onChange func(val int) // what Left and Right do to the value, as it changes
	header   bool          // a heading for the items below it: not chosen, only read
	keepOpen bool
}

func (m *menu) item() *menuItem { m.settle(); return &m.items[m.cur] }

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
		a.menu = nil
	case A:
		it := *m.item()
		if !it.keepOpen {
			a.menu = nil
		}
		if it.run != nil {
			it.run(it.val)
		}
	}
}

func (a *App) drawMenu(c *canvas) {
	m := a.menu
	m.settle()
	w := 328
	rh := 20
	if len(m.items) > 14 {
		rh = 18
	}
	vis := min(len(m.items), 17)
	top := max(0, min(m.cur-vis/2, len(m.items)-vis))
	h := 40 + vis*rh
	x, y := (contentW-w)/2, max(36, (H-h)/2-20)
	c.rect(x, y, w, h, cPanel2)
	c.box(x, y, w, h, 2, cAmber)
	title := strings.ToUpper(m.title)
	if len(m.items) > vis {
		title += fmt.Sprintf("  %d/%d", m.cur+1, len(m.items))
	}
	c.text(x+14, y+10, fit(title, (w-28)/CellW), cAmber, true)
	for r := 0; r < vis; r++ {
		i := top + r
		it := m.items[i]
		yy := y + 36 + r*rh
		if it.header {
			c.rect(x+6, yy+rh/2-1, w-12, 1, cLine)
			c.rect(x+14, yy-1, textW(it.label)+10, rh-2, cPanel2)
			c.text(x+18, yy, it.label, cDim, true)
			continue
		}
		fg := cText
		if i == m.cur {
			c.rect(x+6, yy-2, w-12, rh, cAmberD)
			c.rect(x+6, yy-2, 3, rh, cAmber)
			fg = cWhite
		}
		c.text(x+18, yy, fit(it.label, (w-36-textW("< 000 BPM >"))/CellW+8), fg, i == m.cur)
		if len(it.vals) > 0 {
			v := it.vals[it.val]
			if i == m.cur {
				v = "< " + v + " >"
			}
			c.text(x+w-14-textW(v), yy, v, cAmber, i == m.cur)
		}
	}
	if top > 0 {
		c.text(x+w-30, y+10, "^", cAmber, true)
	}
	if top+vis < len(m.items) {
		c.text(x+w-30, y+h-18, "v", cAmber, true)
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
	m := &menu{title: "pad " + a.label(n)}
	add := func(l string, run func(int)) { m.items = append(m.items, menuItem{label: l, run: run}) }
	m.head("SAMPLE")
	add("Pick a sample...", func(int) { a.go_(Files) })
	if has {
		add("Slice...", func(int) { a.go_(Slice) })
		add("Chords & arps...", func(int) { a.openArpMenu(n) })
		add("Keyboard from it...", func(int) { a.openKeyboardMenu(n) })
		add("Reverse", func(int) { a.transform(n, "reverse", 0) })
		add("Normalize", func(int) { a.transform(n, "normalize", 0) })
		m.items = append(m.items, menuItem{label: "Pitch", vals: []string{"-12", "-5", "-2", "-1", "+1", "+2", "+5", "+12"}, val: 4,
			run: func(v int) {
				st := []int{-12, -5, -2, -1, 1, 2, 5, 12}[v]
				a.transform(n, "pitch", st)
			}})
		m.head("THE PAD")
		add("Pattern with this pad...", func(int) { a.go_(Seq) })
		add("Move or swap to...", func(int) { a.carry = &carry{from: n}; a.say("choose the pad, A puts it down", false) })
		add("Copy to...", func(int) { a.carry = &carry{from: n, copy: true}; a.say("choose the pad, A puts it down", false) })
		add("Clear the pad", func(int) {
			if err := a.lab.ClearPad(n); err == nil {
				delete(a.pcm, n)
				a.refresh()
				a.say("cleared "+a.label(n), false)
			}
		})
	}
	m.head(fmt.Sprintf("MARKED PADS (%d)", len(a.marks)))
	add("Arp the marked pads...", func(int) { a.openMarkedArpMenu() })
	add("Clear the marks", func(int) { a.clearMarks() })
	return m
}

// transform makes a changed copy of a pad's sample and puts it back on the pad.
func (a *App) transform(n int, op string, arg int) {
	id, ok := a.padSample(n)
	if !ok {
		return
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
		a.say(err.Error(), true)
		return
	}
	name := a.info.Pads[n].Name
	if err := a.lab.Assign(n, nid, name); err != nil {
		a.say(err.Error(), true)
		return
	}
	a.lab.Free(nid)
	delete(a.pcm, n)
	a.sl = nil
	a.refresh()
	a.say(fmt.Sprintf("%s: %s %s", a.label(n), op, map[bool]string{true: fmt.Sprintf("%+d", arg), false: ""}[op == "pitch"]), false)
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
