package core

import (
	"sync"
	"time"
)

// A Remote drives the program from outside, for a web page: it says what is on the screen as data (the menu that is open, the pattern) and does what a
// user does (opens the pattern's tools, changes a setting of a tool, runs an entry, goes back). It does it through the program's own menu keys, so a tool
// behaves as it does in the terminal: opening it applies it to the pattern as a trial, a change of a setting applies it again, "Keep it" commits it and
// going back throws it away. The program is not made for two users at a time: every call, and the ticks of Loop, take the lock.
type Remote struct {
	mu sync.Mutex
	a  *App
}

// NewRemote wraps a program (which the caller no longer touches but through the Remote).
func NewRemote(a *App) *Remote { return &Remote{a: a} }

// Do runs f on the program, alone.
func (r *Remote) Do(f func(*App)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f(r.a)
}

// Loop ticks the program (what a screen's loop does, about every 50 ms) until stop is closed.
func (r *Remote) Loop(stop <-chan struct{}) {
	t := time.NewTicker(50 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			r.Do(func(a *App) { a.Tick() })
		}
	}
}

// RItem is an entry of a menu. Kind is "head" (a heading, only read), "choice" (Vals to choose from, Val the one chosen) or "action" (run it).
type RItem struct {
	Index int      `json:"i"` // for Set and Run: the same on every page of the menu
	Label string   `json:"label"`
	Kind  string   `json:"kind"`
	Vals  []string `json:"vals,omitempty"`
	Val   int      `json:"val,omitempty"`
	Opens bool     `json:"opens,omitempty"` // an action that opens another menu ("...")
}

// RMenu is the menu that is open: its title, its pages (when it has them) and the entries of the page in front.
type RMenu struct {
	Title string   `json:"title"`
	Tabs  []string `json:"tabs,omitempty"`
	Tab   int      `json:"tab"`
	Items []RItem  `json:"items"`
	Depth int      `json:"depth"` // how many menus Back goes through to close them all
}

// RTrack is the notes of a pad in the pattern.
type RTrack struct {
	Pad   int     `json:"pad"`
	Label string  `json:"label"`
	Notes []RNote `json:"notes"`
}

// RNote is a note: its step (a sixteenth, from 0) and tick, velocity, length in ticks and pitch (0 plain).
type RNote struct {
	Step  int    `json:"step"`
	Tick  uint32 `json:"tick"`
	Vel   int    `json:"vel"`
	Len   int    `json:"len"`
	Pitch int    `json:"pitch,omitempty"`
}

// RPattern is the pattern in front.
type RPattern struct {
	Slot   int      `json:"slot"` // 1 to 160
	Label  string   `json:"label"`
	Exists bool     `json:"exists"`
	Bars   int      `json:"bars"`
	Steps  int      `json:"steps"`
	Tempo  float64  `json:"tempo"`
	Tracks []RTrack `json:"tracks"`
}

func (a *App) remoteMenu() *RMenu {
	m := a.menu
	if m == nil {
		return nil
	}
	m.settle()
	out := &RMenu{Title: m.title, Tabs: m.tabs, Tab: m.tab, Items: []RItem{}}
	for p := m.parent; p != nil; p = p.parent {
		out.Depth++
	}
	out.Depth++
	for i := range m.items {
		it := &m.items[i]
		ri := RItem{Index: m.fullIndex(it), Label: menuLabel(it.label)}
		switch {
		case it.header || it.tab:
			ri.Kind = "head"
		case len(it.vals) > 0:
			ri.Kind, ri.Vals, ri.Val = "choice", it.vals, it.val
		default:
			ri.Kind = "action"
			ri.Opens = len(it.label) > 3 && it.label[len(it.label)-3:] == "..."
		}
		out.Items = append(out.Items, ri)
	}
	return out
}

// fullIndex is the number an item has in the whole menu (the same on every page).
func (m *menu) fullIndex(it *menuItem) int {
	if m.full == nil {
		for i := range m.items {
			if &m.items[i] == it {
				return i
			}
		}
		return -1
	}
	for i := range m.full {
		if &m.full[i] == it {
			return i
		}
	}
	return -1
}

// find puts the cursor on the item with this number (on its page) and says whether there is one that can be chosen.
func (m *menu) find(index int) *menuItem {
	if index < 0 || index >= len(m.items) && m.full == nil || m.full != nil && index >= len(m.full) {
		return nil
	}
	it := m.at(index)
	if it.header || it.tab {
		return nil
	}
	if m.tabs != nil {
		for t, g := range m.groups {
			for i := range g {
				if &g[i] == it {
					m.setTab(t)
				}
			}
		}
	}
	for i := range m.items {
		if &m.items[i] == it {
			m.cur = i
			return it
		}
	}
	return nil
}

// Menu is the menu that is open, or nil.
func (r *Remote) Menu() *RMenu {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.a.remoteMenu()
}

// OpenPatternTools opens the menu of the pattern in front (the tools and settings of SEQ).
func (r *Remote) OpenPatternTools() *RMenu {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.a.go_(Seq)
	r.a.seqMenu()
	return r.a.remoteMenu()
}

// Tab turns the open menu to a page.
func (r *Remote) Tab(i int) *RMenu {
	r.mu.Lock()
	defer r.mu.Unlock()
	if m := r.a.menu; m != nil && m.tabs != nil {
		m.setTab(i)
	}
	return r.a.remoteMenu()
}

// Set changes a choice of the open menu to its value number val, as Left and Right do (a tool applies itself again).
func (r *Remote) Set(index, val int) *RMenu {
	r.mu.Lock()
	defer r.mu.Unlock()
	if m := r.a.menu; m != nil {
		if it := m.find(index); it != nil && len(it.vals) > 0 && val >= 0 && val < len(it.vals) && val != it.val {
			it.val = (val - 1 + len(it.vals)) % len(it.vals) // one step before: Right makes it val, and runs what a change runs
			r.a.menuKey(Right)
		}
	}
	return r.a.remoteMenu()
}

// Run runs an entry of the open menu, as A does (it may open another menu, or close this one).
func (r *Remote) Run(index int) *RMenu {
	r.mu.Lock()
	defer r.mu.Unlock()
	if m := r.a.menu; m != nil {
		if it := m.find(index); it != nil {
			r.a.menuKey(A)
		}
	}
	return r.a.remoteMenu()
}

// Back goes back a menu, as B does: a tool that is open throws its trial away.
func (r *Remote) Back() *RMenu {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.a.menu != nil {
		r.a.menuKey(B)
	}
	return r.a.remoteMenu()
}

// Pattern is the pattern in front: its notes, track by track.
func (r *Remote) Pattern() RPattern {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.a.remotePattern()
}

// SetPattern puts the pattern of a slot (1 to 160) in front.
func (r *Remote) SetPattern(slot int) RPattern {
	r.mu.Lock()
	defer r.mu.Unlock()
	if slot >= 1 && slot <= 160 {
		r.a.rowExtra = nil
		r.a.pat = slot
		r.a.step = 0
	}
	return r.a.remotePattern()
}

func (a *App) remotePattern() RPattern {
	d, ok := a.doc()
	p := RPattern{Slot: a.pat, Label: a.label(a.pat - 1), Exists: ok, Bars: d.Bars, Steps: d.Bars * 16, Tempo: a.tempo(), Tracks: []RTrack{}}
	byPad := map[int]*RTrack{}
	for _, n := range d.Notes {
		t := byPad[n.Pad]
		if t == nil {
			p.Tracks = append(p.Tracks, RTrack{Pad: n.Pad, Label: a.label(n.Pad), Notes: []RNote{}})
			t = &p.Tracks[len(p.Tracks)-1]
			byPad[n.Pad] = t
		}
		t.Notes = append(t.Notes, RNote{Step: int(n.Tick) / stepTicks, Tick: n.Tick, Vel: n.Vel, Len: n.Len, Pitch: n.Pitch})
	}
	// the pointers into p.Tracks may have moved when it grew: build again from the final slice
	out := make([]RTrack, 0, len(p.Tracks))
	for _, t := range p.Tracks {
		out = append(out, t)
	}
	p.Tracks = out
	return p
}
