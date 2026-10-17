package tui

import (
	"encoding/json"
	"math"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// The web page (see package web) is a second screen for Perform: the program sends it its state as JSON
// whenever it changes, and it sends commands back, which do what the buttons and pads of the terminal do.

// WebSink takes the state to show in the browsers.
type WebSink interface {
	Publish(state []byte)
}

// WebCommand is a command a browser sent, as the JSON text it came in.
type WebCommand []byte

// WithWebURL is the address of the web page, to show in Perform.
func (m Model) WithWebURL(u string) Model {
	m.webURL = u
	return m
}

// WithWeb gives the model a web side to publish to.
func (m Model) WithWeb(s WebSink) Model {
	m.web = s
	return m
}

// webState is what the page shows.
type webState struct {
	Unit    bool      `json:"unit"`
	Bank    string    `json:"bank"`
	BPM     float64   `json:"bpm"`
	Running bool      `json:"running"`
	Clock   string    `json:"clock"`
	Bar     int       `json:"bar"`
	Beat    int       `json:"beat"` // 0 to 3, -1 when there is no clock
	Rec     bool      `json:"rec"`
	Now     string    `json:"now"`
	Next    string    `json:"next"`
	Msg     string    `json:"msg"`
	Pads    []webPad  `json:"pads"`
	Focus   webFocus  `json:"focus"`
	Motion  webMotion `json:"motion"`
	Fx      [2]webFx  `json:"fx"`
	Version string    `json:"version"`
}

type webPad struct {
	N     int     `json:"n"` // 1 to 16
	Name  string  `json:"name"`
	Has   bool    `json:"has"`
	Bus   int     `json:"bus"`
	Level int     `json:"level"`
	Muted bool    `json:"muted"` // silent in the mix on this computer or by the pattern on the unit
	Heard bool    `json:"heard"` // in the pattern that plays, and heard lately
	InPat bool    `json:"inPat"` // in the pattern that plays, not heard yet
	Glow  float64 `json:"glow"`  // 0 to 1, how lately it was hit
	Sel   bool    `json:"sel"`   // the pad the unit has selected
	Keep  bool    `json:"keep"`  // kept by focus
	Bars  string  `json:"bars"`  // its length in bars, when known
	Dry   bool    `json:"dry"`   // put on no bus by focus
}

type webFocus struct {
	On    bool `json:"on"`
	Multi bool `json:"multi"`
	Dry   int  `json:"dry"`
}

type webMotion struct {
	Bars    int         `json:"bars"`
	Overdub bool        `json:"overdub"`
	Bus     [2]webBusMo `json:"bus"`
}

type webBusMo struct {
	Sel   int                    `json:"sel"`
	Slots [motionSlots]webMoSlot `json:"slots"`
}

type webMoSlot struct {
	State  int     `json:"state"` // 0 idle, 1 armed, 2 recording, 3 waiting, 4 playing
	Filled bool    `json:"filled"`
	Bars   int     `json:"bars"`
	Pos    float64 `json:"pos"` // 0 to 1 in the loop while it plays, in steps of 1/32; the bar of a recording
	Speed  float64 `json:"speed"`
	Rev    bool    `json:"rev"`
	Knob   int     `json:"knob"`  // the knob that moves the most, 1 to 3
	Curve  []int   `json:"curve"` // what it does over the loop, 0 to 127
}

type webFx struct {
	Seen  bool   `json:"seen"`
	On    bool   `json:"on"`
	Name  string `json:"name"`
	Note  string `json:"note"`
	Knobs [3]int `json:"knobs"` // -1 until the unit sent it
}

// webSnapshot is the state of Perform for the page.
func (m Model) webSnapshot() webState {
	s := webState{Unit: m.unit != nil && m.snap != nil, Beat: -1, Pads: []webPad{}, Msg: m.message}
	s.Motion.Bars = m.scenes().bars
	s.Motion.Overdub = m.scenes().overdub
	s.Rec = m.rec != nil
	if m.snap == nil {
		return s
	}
	bank := m.perfBank()
	s.Bank = string(rune('A' + bank))
	s.BPM = m.bankTempo(bank)
	bars := 0
	if d, _ := m.perfTracks(m.perfNow); d != nil {
		bars = d.Bars()
	}
	s.Clock = m.clockText(bars)
	if c := m.clock; c != nil && time.Since(c.at) < 2*time.Second {
		s.Running = c.running
		s.Bar, s.Beat = c.ticks/96+1, c.ticks/24%4
	}
	s.Now, s.Next = slotText(m.perfNow), slotText(m.perfNext)
	now := time.Now()
	for i, row := range m.snap.Bank(bank) {
		p := webPad{N: i + 1, Name: row.Name, Has: row.HasSample, Bus: int(m.busOf(row.Pad)), Level: int(row.Level),
			Muted: m.padMuted(row.Pad) || m.lvlWas[row.Pad] > 0, Sel: m.hasSelected && m.selected == row.Pad,
			Bars: m.padBars(row, 20)}
		p.Heard, p.InPat = m.padUsed(row.Pad)
		if m.iso != nil {
			_, p.Keep = m.iso.pads[row.Pad]
			if was, ok := m.iso.was[row.Pad]; ok && !p.Keep {
				p.Dry, p.Bus = true, int(was)
			}
		}
		if row.HasSample {
			p.Glow = math.Round(m.mon.glow(bank, int(row.Pad.Pad()), now)*5) / 5
		}
		s.Pads = append(s.Pads, p)
	}
	if m.iso != nil {
		s.Focus = webFocus{On: true, Multi: m.iso.multi, Dry: len(m.iso.was)}
	}
	sc := m.scenes()
	for b := range sc.bus {
		bs := &sc.bus[b]
		s.Motion.Bus[b].Sel = bs.sel
		for k := range bs.mo {
			mo := bs.mo[k]
			sl := webMoSlot{State: mo.state, Filled: mo.filled(), Bars: mo.total / ticksPerBar, Speed: mo.rate(), Rev: mo.rev}
			switch mo.state {
			case motionPlaying:
				sl.Pos = math.Floor(mo.fpos/float64(max(mo.total, 1))*32) / 32
			case motionRecording:
				sl.Pos = math.Floor(float64(mo.pos)/float64(max(mo.total, 1))*32) / 32
			}
			if mo.filled() {
				knob, curve := mo.curve(16)
				sl.Knob, sl.Curve = knob+1, curve
			}
			s.Motion.Bus[b].Slots[k] = sl
		}
	}
	for b := range s.Fx {
		f := m.fx[b]
		fx := webFx{Seen: f.onSeen, On: f.on, Note: m.mem.BusNotes[b], Knobs: [3]int{-1, -1, -1}}
		if f.numSeen {
			fx.Name = fxName(f.num)
		}
		for k := range fx.Knobs {
			if c := m.ccOf(b+1, 16+k); c != nil {
				fx.Knobs[k] = c.val
			}
		}
		s.Fx[b] = fx
	}
	return s
}

// publishWeb sends the state to the browsers.
func (m Model) publishWeb() {
	if m.web == nil {
		return
	}
	b, err := json.Marshal(m.webSnapshot())
	if err == nil {
		m.web.Publish(b)
	}
}

// webCmd is a command from the page.
type webCmd struct {
	A     string  `json:"a"`
	I     int     `json:"i"`
	Bus   int     `json:"bus"`
	Slot  int     `json:"slot"`
	N     int     `json:"n"`
	Value float64 `json:"v"`
}

// webCommand does what the page asked for.
func (m Model) webCommand(raw WebCommand) (tea.Model, tea.Cmd) {
	var c webCmd
	if err := json.Unmarshal(raw, &c); err != nil {
		return m, nil
	}
	m.log.Debug("web: command", "cmd", string(raw))
	bus := clamp(c.Bus, 0, 1)
	switch c.A {
	case "pad":
		if m.snap == nil || c.I < 0 || c.I > 15 {
			return m, nil
		}
		return m.clickPad(m.snap.Bank(m.perfBank())[c.I].Pad)
	case "level":
		nm, cmd := m.levelSet2(c.I, int32(c.Value))
		return nm, cmd
	case "mute":
		nm, cmd := m.levelMute(c.I)
		return nm, cmd
	case "focus":
		return m.isolateSwitch()
	case "rec":
		return m.toggleRecording()
	case "motion":
		return m.motionAct(bus, c.Slot), nil
	case "mclear":
		return m.motionClear(bus), nil
	case "mspeed":
		return m.motionSpeed(bus), nil
	case "mrev":
		return m.motionReverse(bus), nil
	case "bars":
		if c.N == 1 || c.N == 2 || c.N == 3 || c.N == 4 || c.N == 8 {
			return m.sceneAct("bars", c.N, 0), nil
		}
	case "dub":
		return m.motionOverdub(), nil
	case "nextnow":
		return m.perfKey(" ")
	case "clearnext":
		return m.perfKey("x")
	}
	return m, nil
}

// levelSet2 sets the level of the pad at a place of the bank on show (0 to 15).
func (m Model) levelSet2(i int, v int32) (Model, tea.Cmd) {
	pad, ok := m.levelPad(i)
	if !ok || m.unit == nil {
		return m, nil
	}
	delete(m.lvlWas, pad)
	return m.levelSet(pad, v)
}
