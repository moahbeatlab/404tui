package tui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/proto"
)

// Rows of the screen: the header, a rule under it, the body (with the rail down its left
// side), and the status bar.
const (
	rowHeader = 0
	rowRule   = 1
	rowBody   = 2
	// chrome is the rows that are not the body.
	chrome = 3
)

// geom is where things are on the screen. The views draw by it and the mouse reads it, so
// they cannot disagree. Positions of body content are relative to the body's own corner
// (bodyX, rowBody).
type geom struct {
	// railW is the width of the navigation rail; the body starts at bodyX.
	railW        int
	bodyX, bodyW int
	bodyH        int
	// top is the rows above the pad grid: the bank bar and a gap.
	top int
	// side puts the detail panel beside the grid instead of under it.
	side         bool
	tileW, tileH int
	gridW, gridH int
	// detailW and detailH size the panel under or beside the grid.
	detailW int
	detailH int
	listW   int
}

func (m Model) geom() geom {
	rows := chrome
	if m.tagJob.active() {
		rows++
	}
	g := geom{railW: 16, bodyH: max(m.height-rows, 4)}
	if m.width < 80 {
		g.railW = 12
	}
	g.bodyX = g.railW + 2
	g.bodyW = max(m.width-g.bodyX-1, 20)
	if m.section == secPads || m.section == secPatterns {
		g.top = 2
	}
	g.side = g.bodyW >= 100
	avail := g.bodyW
	if g.side {
		avail = g.bodyW * 52 / 100
	}
	g.tileW = clamp((avail-3)/4, 8, 24)
	g.tileH = 3
	// With rows to spare the pads are a row taller, which gives their settings a line of their
	// own: the pads' panel (4 rows of 4 and their gaps, a frame) and the patterns' under it.
	if g.side && (m.section == secPads || m.section == secPatterns) && g.bodyH-g.top >= 4*4+3+2+13 {
		g.tileH = 4
	}
	// The detail panel wants about 13 rows under the grid; below that the tiles shrink.
	if (g.side && g.bodyH-g.top < 15) || (!g.side && g.bodyH-g.top < 27) {
		g.tileH = 2
	}
	g.gridW = 4*g.tileW + 3
	g.gridH = 4*g.tileH + 3
	if g.side {
		g.detailW = g.bodyW - g.gridW - 2
		if m.section == secPads || m.section == secPatterns {
			// The pads sit in a box of their own, two columns wider than the grid.
			g.detailW -= 2
		}
		g.detailH = min(g.bodyH-g.top, 13)
	} else {
		g.detailW = g.bodyW
		g.detailH = min(g.bodyH-g.top-g.gridH-1, 13)
	}
	g.listW = clamp(g.bodyW*55/100, 28, 90)
	return g
}

// railSections are the sections the rail lists, in order. The patterns are not one of them:
// they are the panel under the pads, reached with Tab.
var railSections = []int{secPads, secFiles, secProjects}

// railKind is what a row of the rail stands for.
type railKind int

const (
	railNone railKind = iota
	railSection
	railBank
	railRoot
	railLabel
)

type railItem struct {
	kind railKind
	// index is the section, bank or file source the row stands for.
	index int
}

// rail lists what each row of the rail stands for, from the top of the body: the four
// sections, then the banks or file sources of the section in front. The view draws it and
// the mouse reads it.
func (m Model) rail(height int) []railItem {
	items := make([]railItem, 0, height)
	for _, s := range railSections {
		items = append(items, railItem{railSection, s})
	}
	banks := func() []railItem {
		var out []railItem
		for b := 0; b < proto.Banks; b++ {
			out = append(out, railItem{railBank, b})
		}
		return out
	}
	// add appends a labelled group, windowed to the room that is left.
	add := func(label int, group []railItem, current int, reserve int) {
		items = append(items, railItem{railNone, 0}, railItem{railLabel, label})
		room := max(height-len(items)-reserve, 1)
		from, to := window(len(group), current, room)
		items = append(items, group[from:to]...)
	}
	switch m.section {
	case secPads, secPatterns:
		add(0, banks(), m.bank, 0)
	case secFiles:
		var group []railItem
		current := 0
		for i, r := range roots() {
			group = append(group, railItem{railRoot, i})
			if r == m.root {
				current = i
			}
		}
		add(1, group, current, 0)
		// The bank a file goes to when it is assigned.
		add(2, banks(), m.target, 0)
	}
	for len(items) < height {
		items = append(items, railItem{railNone, 0})
	}
	return items[:height]
}

func railLabelText(label int) string {
	switch label {
	case 0:
		return "BANKS"
	case 1:
		return "SOURCE"
	}
	return "TARGET BANK"
}

// padPosition is the cell of the 4×4 grid a pad position occupies: pad 1 is top left, as on the unit.
func padPosition(p int) (col, row int) {
	return p % 4, p / 4
}

func (m Model) mouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action == tea.MouseActionPress {
		m.log.Debug("mouse", "x", msg.X, "y", msg.Y, "button", fmt.Sprint(msg.Button), "section", sectionNames[m.section], "busy", m.busy)
	}
	if m.unit == nil || m.busy != "" || m.snap == nil || m.help || m.prompt != nil || m.picker != nil || m.palette != nil || m.toggles != nil || m.browser != nil || m.chromatic != nil || m.slicer != nil {
		return m, nil
	}
	wheel := 0
	switch {
	case msg.Button == tea.MouseButtonWheelUp && msg.Action == tea.MouseActionPress:
		wheel = -1
	case msg.Button == tea.MouseButtonWheelDown && msg.Action == tea.MouseActionPress:
		wheel = 1
	case msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress:
		return m, nil
	}
	g := m.geom()
	if wheel != 0 {
		return m.wheel(wheel)
	}
	switch {
	case msg.X < g.railW && msg.Y >= rowBody:
		return m.clickRail(msg.Y-rowBody, g)
	case msg.X >= g.bodyX && msg.Y >= rowBody:
		return m.clickBody(msg.X-g.bodyX, msg.Y-rowBody, g)
	}
	return m, nil
}

// clickRail handles a click on a row of the rail.
func (m Model) clickRail(row int, g geom) (tea.Model, tea.Cmd) {
	items := m.rail(g.bodyH)
	if row < 0 || row >= len(items) {
		return m, nil
	}
	item := items[row]
	switch item.kind {
	case railSection:
		return m.goTo(item.index)
	case railBank:
		return m.clickBank(item.index)
	case railRoot:
		if root := roots()[item.index]; root != m.root {
			m.root, m.tree, m.fileAt = root, nil, 0
			m.fileMarks = map[string]bool{}
			cmd := m.wanted()
			return m, cmd
		}
	}
	return m, nil
}

func (m Model) clickBank(bank int) (tea.Model, tea.Cmd) {
	if m.section == secFiles {
		m.target = bank
		return m, nil
	}
	if m.section == secPatterns {
		m.bank, m.pattern = bank, nil
		cmd := m.wanted()
		return m, cmd
	}
	if bank == m.bank {
		return m, nil
	}
	m.bank = (bank + proto.Banks - 1) % proto.Banks
	return m.stepBank(1)
}

// stepBank moves a bank on from the current one, as ] does.
func (m Model) stepBank(by int) (tea.Model, tea.Cmd) {
	cmd := m.switchBank(by)
	return m, cmd
}

func (m Model) wheel(by int) (tea.Model, tea.Cmd) {
	switch m.section {
	case secPads:
		return m.stepBank(by)
	case secPatterns:
		if m.pattern == nil {
			m.bank = (m.bank + proto.Banks + by) % proto.Banks
			cmd := m.wanted()
			return m, cmd
		}
	case secFiles:
		if m.tree != nil {
			m.fileAt = clamp(m.fileAt+by, 0, max(len(m.tree.Rows())-1, 0))
			cmd := m.wanted()
			return m, cmd
		}
	case secProjects:
		m.projectAt = clamp(m.projectAt+by, 0, len(m.snap.Projects)-1)
	}
	return m, nil
}

// clickBody handles a click in the body, with x and y counted from its corner: a pad or
// pattern tile, or a list row.
func (m Model) clickBody(x, y int, g geom) (tea.Model, tea.Cmd) {
	switch m.section {
	case secPads, secPatterns:
		if m.section == secPatterns && m.pattern != nil {
			return m, nil
		}
		y -= g.top
		if y < 0 {
			return m, nil
		}
		col, row := x/(g.tileW+1), y/(g.tileH+1)
		inside := x < g.gridW && y < g.gridH
		if !inside || x%(g.tileW+1) == g.tileW || y%(g.tileH+1) == g.tileH {
			return m, nil
		}
		p := row*4 + col
		if m.section == secPatterns {
			m.patternCursor = p
			return m, nil
		}
		// A click selects the pad and plays it, as hitting a pad does.
		m.pad = p
		model, cmd := m.padsKey("enter")
		next := model.(Model)
		return next, tea.Batch(cmd, next.wanted())
	case secFiles:
		if m.tree == nil || x >= g.listW {
			return m, nil
		}
		rows := len(m.tree.Rows())
		from, _ := window(rows, m.fileAt, g.bodyH-2)
		// The box has a border row on top.
		if at := from + y - 1; at >= 0 && at < rows && y >= 1 {
			if at == m.fileAt {
				return m.filesKey("tab")
			}
			m.fileAt = at
			wanted := m.wanted()
			play := m.scheduleAutoplay()
			return m, tea.Batch(wanted, play)
		}
	case secProjects:
		if x >= g.listW {
			return m, nil
		}
		from, _ := window(len(m.snap.Projects), m.projectAt, g.bodyH-2)
		if at := from + y - 1; at >= 0 && at < len(m.snap.Projects) && y >= 1 {
			if at == m.projectAt {
				return m.projectsKey("enter")
			}
			m.projectAt = at
		}
	}
	return m, nil
}
