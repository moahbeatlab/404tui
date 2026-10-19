package tui

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/proto"
	"github.com/moahbeatlab/404tui/record"
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

// perfActive says whether what Perform shows is kept read: on its tab, or for the browsers.
func (m Model) perfActive() bool { return m.section == secPerform || m.web != nil }

// WithWeb gives the model a web side to publish to.
func (m Model) WithWeb(s WebSink) Model {
	m.web = s
	return m
}

// webState is what the page shows.
type webState struct {
	Unit bool   `json:"unit"`
	Bank string `json:"bank"`
	// BankFill is how many pads of each bank hold a sample.
	BankFill []int     `json:"bankFill"`
	BPM      float64   `json:"bpm"`
	Running  bool      `json:"running"`
	Clock    string    `json:"clock"`
	Bar      int       `json:"bar"`
	Beat     int       `json:"beat"` // 0 to 3, -1 when there is no clock
	Rec      bool      `json:"rec"`
	Now      string    `json:"now"`
	Next     string    `json:"next"`
	Msg      string    `json:"msg"`
	Pads     []webPad  `json:"pads"`
	Focus    webFocus  `json:"focus"`
	Motion   webMotion `json:"motion"`
	Fx       [2]webFx  `json:"fx"`
	Punch    webPunch  `json:"punch"`
	Lfo      []webLfo  `json:"lfo"`
	// Patterns are the 16 slots of the bank, to choose what plays now and next.
	Patterns []webPattern `json:"patterns"`
	Version  string       `json:"version"`
	Looper   webLooper    `json:"looper"`
	Slicer   webSlicer    `json:"slicer"`
	Ptn      webPtn       `json:"ptn"`
	// Level is how loud the sound is that is recorded (or, outside a recording, that the unit sends); nil until read.
	Level *webLevel `json:"level"`
	// LevelNote says why there is none: no recorder, or what the listening program said.
	LevelNote string `json:"levelNote"`
}

// webLevel is the level meter: the peak of each channel and the held one, in dB below full scale (-60 is the floor).
type webLevel struct {
	Peak [2]float64 `json:"peak"`
	Hold [2]float64 `json:"hold"`
	Top  float64    `json:"top"` // the loudest so far
	Clip bool       `json:"clip"`
	// Src is what is heard: the input, a recording, or a take of the Looper.
	Src string `json:"src"`
}

// webLooper is the Looper: where the loops go, and how a take is going.
type webLooper struct {
	Bank string `json:"bank"`
	Bars int    `json:"bars"`
	// Later says what waits for beat 1 of the pattern (empty for nothing).
	Later string `json:"later"`
	// Pattern is the number of the pattern of the bank in which all 16 pads play, 0 when there is none (or it is
	// not read yet: Known says which).
	Pattern     int          `json:"pattern"`
	PatternName string       `json:"patternName"` // the slot it is in, such as "A2"
	Known       bool         `json:"known"`
	Pads        []webLoopPad `json:"pads"`
}

type webLoopPad struct {
	N     int    `json:"n"`
	Name  string `json:"name"`
	Has   bool   `json:"has"`
	Del   bool   `json:"del"` // waits to be deleted on the last beat
	Muted bool   `json:"muted"`
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

type webPattern struct {
	N      int  `json:"n"` // 1 to 16
	Exists bool `json:"exists"`
	Pads   int  `json:"pads"`
	Now    bool `json:"now"`
	Next   bool `json:"next"`
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
	s := webState{Unit: m.unit != nil && m.snap != nil, Beat: -1, Pads: []webPad{}, Patterns: []webPattern{}, Msg: m.message}
	s.Motion.Bars = m.scenes().bars
	s.Motion.Overdub = m.scenes().overdub
	s.Rec = m.rec != nil
	if m.snap == nil {
		return s
	}
	bank := m.perfBank()
	s.Bank = string(rune('A' + bank))
	for b := 0; b < proto.Banks; b++ {
		n := 0
		for _, row := range m.snap.Bank(b) {
			if row.HasSample {
				n++
			}
		}
		s.BankFill = append(s.BankFill, n)
	}
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
	for i, r := range m.patterns[bank] {
		s.Patterns = append(s.Patterns, webPattern{N: i + 1, Exists: r.Exists, Pads: len(r.Pads),
			Now: m.perfNow != nil && *m.perfNow == r.Slot, Next: m.perfNext != nil && *m.perfNext == r.Slot})
	}
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
	s.Punch = m.webPunch()
	s.Lfo = m.webLfo()
	s.Looper = m.webLooper()
	s.Slicer = m.webSlicer()
	s.Ptn = m.webPtn()
	s.Level = m.webLevel()
	if s.Level == nil {
		switch _, ok := m.recorder.(record.Metering); {
		case !ok:
			s.LevelNote = "no level: install ffmpeg (or sox) on the computer"
		case m.meterErr != "":
			s.LevelNote = "no level: " + m.meterErr
		default:
			s.LevelNote = "listening…"
		}
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

// webLevel is the meter for the page, in whole dB.
func (m Model) webLevel() *webLevel {
	if m.lvl == nil || !m.lvl.ok {
		return nil
	}
	db := func(v float64) float64 {
		if math.IsInf(v, -1) || v < -60 {
			return -60
		}
		return math.Round(v)
	}
	l := &webLevel{Top: db(m.lvl.top), Clip: m.lvl.clipping(time.Now()), Src: "input"}
	for c := range 2 {
		l.Peak[c], l.Hold[c] = db(m.lvl.peak[c]), db(m.lvl.hold[c])
	}
	if m.rec != nil {
		l.Src = "recording"
	}
	return l
}

// webLooper is the Looper for the page.
func (m Model) webLooper() webLooper {
	bank := m.loopBank()
	l := webLooper{Bank: string(rune('A' + bank)), Bars: m.loopBars(), Pads: []webLoopPad{}}
	if rows := m.patterns[bank]; rows != nil {
		l.Known = true
		for i, r := range rows {
			if r.Exists && len(r.Pads) == proto.PadsPerBank {
				l.Pattern, l.PatternName = i+1, r.Slot.String()
				break
			}
		}
	}
	for i, row := range m.snap.Bank(bank) {
		l.Pads = append(l.Pads, webLoopPad{N: i + 1, Name: row.Name, Has: row.HasSample, Del: m.looperQueued(fmt.Sprintf("del:%d", i)), Muted: m.lvlMuted(row.Pad)})
	}
	l.Later = m.laterText()
	return l
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
	Name  string  `json:"name"`
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
	case "solo":
		// A hold on a pad: it keeps its bus and the others on that bus go dry; held again, it lets go (when
		// no pad is kept any more, everything goes back). A muted pad is brought back first.
		if m.snap == nil || c.I < 0 || c.I > 15 {
			return m, nil
		}
		pad := m.snap.Bank(m.perfBank())[c.I].Pad
		// The pads kept so far: a hold adds the pad, or takes it out when it is kept already.
		keep := map[proto.PadIndex]bool{}
		if m.iso != nil {
			for p := range m.iso.pads {
				keep[p] = true
			}
		}
		if keep[pad] {
			delete(keep, pad)
			if len(keep) == 0 && (m.iso == nil || !m.iso.multi) {
				return m.isolateOff()
			}
			m.message = fmt.Sprintf("%s is no longer kept on its bus", pad)
			return m.setIsolated(keep)
		}
		var undo tea.Cmd
		if _, muted := m.lvlWas[pad]; muted {
			m, undo = m.levelMute(c.I)
		}
		if m.busOf(pad) == 0 {
			m.message = fmt.Sprintf("%s goes to no bus: nothing to isolate", pad)
			return m, undo
		}
		keep[pad] = true
		m.message = fmt.Sprintf("%s kept on %s: hold it again to let it go", pad, busName(m.busOf(pad)))
		nm, cmd := m.setIsolated(keep)
		return nm, tea.Batch(undo, cmd)
	case "lpbankset":
		if m.snap == nil {
			return m, nil
		}
		return m.looperBankSet(c.I)
	case "view":
		m.webView = c.Name
		return m, m.wanted()
	case "ptn", "ptnbank", "ptnbankset", "ptnfollow":
		return m.ptnCommand(c.A, c.I)
	case "slbank", "slpad", "sltbank", "slslices", "slcut", "slorder", "sldiv", "slswing", "slgo", "slagain", "slcancel", "slbankset", "sltbankset", "slmute", "slmang":
		return m.slicerCommand(c.A, c.N, c.I)
	case "lpbank", "lppattern", "lpdel", "lpslices", "lpremix":
		if m.snap == nil {
			return m, nil
		}
		if m.busy != "" && !lpSettings[c.A] {
			m.message = "busy (" + m.busy + "): try again in a moment"
			return m, nil
		}
		switch c.A {
		case "lpbank":
			return m.looperBankStep(clamp(c.N, -1, 1))
		case "lppattern":
			return m.looperPattern()
		case "lpslices":
			return m.looperSlicesNext()
		case "lpremix":
			return m.looperRemix(c.I)
		default:
			return m.looperDelete(c.I)
		}
	case "level":
		nm, cmd := m.levelSet2(c.I, int32(c.Value))
		return nm, cmd
	case "mute":
		// A double tap: the first tap isolated the pad, which is undone before the mute.
		if c.N == 1 && m.iso != nil && m.snap != nil && c.I >= 0 && c.I <= 15 &&
			len(m.iso.pads) == 1 && m.iso.pads[m.snap.Bank(m.perfBank())[c.I].Pad] {
			nm, off := m.isolateOff()
			nm, cmd := nm.(Model).levelMute(c.I)
			return nm, tea.Batch(off, cmd)
		}
		nm, cmd := m.levelMute(c.I)
		return nm, cmd
	case "focus":
		return m.isolateSwitch()
	case "rec":
		return m.toggleRecording()
	case "motion":
		return m.motionAct(bus, c.Slot), nil
	case "punch":
		return m.punchAct(map[int]string{0: "up", 1: "down"}[c.N], c.I), nil
	case "punchpreset":
		return m.punchPreset(c.I, c.N), nil
	case "morph":
		return m.morphSet(c.N), nil
	case "lfoset":
		return m.lfoSet(c.I, c.Name, c.N), nil
	case "lfoon":
		return m.lfoToggle(c.I), nil
	case "punchmode":
		return m.punchAct("mode", 0), nil
	case "punchsave":
		return m.punchAct("save", 0), nil
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
	case "bank":
		// The unit moves to the bank, as it does when [ or ] is pressed in the Pads view.
		if m.snap == nil || m.busy != "" || c.I < 0 || c.I >= proto.Banks || c.I == m.perfBank() {
			return m, nil
		}
		m.bank = m.perfBank()
		cmd := m.switchBank(c.I - m.bank)
		return m, cmd
	case "cue":
		// A pattern of the bank on show is the one that plays (n 1) or the next (n 2).
		rows := m.patterns[m.perfBank()]
		if c.I < 0 || c.I >= len(rows) || !rows[c.I].Exists || (c.N != 1 && c.N != 2) {
			return m, nil
		}
		slot := rows[c.I].Slot
		if c.N == 1 {
			m.perfNow, m.perfAuto = &slot, false
		} else {
			m.perfNext = &slot
		}
		m.logCue()
		return m, m.wanted()
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

// lpSettings are the commands of the Looper and the Slicer that only change a setting: they do not wait for the unit.
var lpSettings = map[string]bool{
	"lpbank": true, "lpbankset": true, "lpslices": true,
	"slbank": true, "slbankset": true, "sltbank": true, "sltbankset": true, "slslices": true, "slcut": true, "slorder": true, "slmang": true, "sldiv": true, "slswing": true,
}
