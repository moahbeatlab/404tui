package tui

import (
	"fmt"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/perflog"
	"github.com/moahbeatlab/404tui/proto"
)

// A replay needs to know what the project was: the pads (names, bus, settings) and the patterns (their notes). The first line after
// "start" of the event log says it, as the unit has it when the recording begins; it is read in the background.

// projectSnapDone says the project line was written (or why not).
type projectSnapDone struct{ err error }

// projectLogCmd reads the patterns of the unit and writes the project line to the event log.
func (m Model) projectLogCmd(l *perflog.Log, started time.Time) tea.Cmd {
	if l == nil || m.unit == nil || m.snap == nil {
		return nil
	}
	snap, unit, ops := m.snap, m.unit, m.ops
	return func() tea.Msg {
		out := map[string]any{"name": snap.ProjectName, "tempo": snap.ProjectTempo, "project_tempo": snap.UsesProjectTempo}
		bt := make([]float64, 0, len(snap.Banks))
		for _, b := range snap.Banks {
			bt = append(bt, b.Tempo)
		}
		out["bank_tempos"] = bt
		type snapPad struct {
			N     int               `json:"n"`
			Label string            `json:"label"`
			Name  string            `json:"name"`
			Bus   int               `json:"bus,omitempty"`
			Info  map[string]string `json:"info"`
			Peaks []int             `json:"peaks,omitempty"`
		}
		pads := []snapPad{}
		for n, row := range snap.Pads {
			if !row.HasSample {
				continue
			}
			sp := snapPad{N: n, Label: row.Pad.String(), Name: row.Name, Info: padSettingTexts(row)}
			if v := row.Params["bus-fx"]; v == 1 || v == 2 {
				sp.Bus = int(v)
			}
			// a small picture of the sound: 32 peaks, the loudest 100 (a sample file is 16-bit stereo, 4 bytes a frame)
			if frames := row.Size / 4; frames > 64 {
				ops.Lock()
				pk, err := unit.Peaks(row.Pad, 0, 0, 32, frames/32)
				ops.Unlock()
				if err == nil {
					top := 1
					amp := make([]int, 0, len(pk))
					for _, v := range pk {
						a := max(abs(int(v.Min)), abs(int(v.Max)))
						amp = append(amp, a)
						top = max(top, a)
					}
					for _, a := range amp {
						sp.Peaks = append(sp.Peaks, a*100/top)
					}
				}
			}
			pads = append(pads, sp)
		}
		out["pads"] = pads
		type snapPattern struct {
			Slot   int      `json:"slot"`
			Label  string   `json:"label"`
			Bars   int      `json:"bars"`
			Length int      `json:"length"`
			Notes  [][5]int `json:"notes"`
		}
		pats := []snapPattern{}
		ops.Lock()
		current, err := unit.CurrentProject()
		ops.Unlock()
		if err == nil {
			for bank := 0; bank < proto.Banks; bank++ {
				ops.Lock()
				rows, err := app.LoadPatterns(unit, bank)
				ops.Unlock()
				if err != nil {
					continue
				}
				for _, r := range rows {
					if !r.Exists {
						continue
					}
					ops.Lock()
					data, err := unit.ReadFile(formats.PatternPath(current+1, r.Slot))
					ops.Unlock()
					if err != nil {
						continue
					}
					p, err := formats.ParsePattern(data)
					if err != nil {
						continue
					}
					sp := snapPattern{Slot: int(r.Slot.Index()) + 1, Label: r.Slot.String(), Length: int(p.Length), Bars: max(int(p.Length)/(formats.PPQ*4), 1)}
					for _, e := range p.Events {
						if !e.Control {
							sp.Notes = append(sp.Notes, [5]int{int(e.Tick), int(e.Pad.Index()), int(e.Velocity), int(e.Length), int(e.Pitch)})
						}
					}
					if len(sp.Notes) > 0 {
						pats = append(pats, sp)
					}
				}
			}
		}
		out["patterns"] = pats
		l.Add(started, "project", perflog.P("source", "unit"), perflog.P("data", out))
		return projectSnapDone{}
	}
}

// padSettingTexts are the settings of a pad as the replay shows them (the names are the ones of the program's pad panel).
func padSettingTexts(row app.PadRow) map[string]string {
	p := row.Params
	onOff := func(v int32) string {
		if v != 0 {
			return "on"
		}
		return "off"
	}
	info := map[string]string{"Level": strconv.Itoa(int(p["level"])), "Gate": onOff(p["gate"]), "Loop": onOff(p["loop"]),
		"One shot": onOff(p["mode-flags"] & (1 << 5)), "BPM sync": onOff(p["bpm-sync"])}
	switch p["bus-fx"] {
	case 1, 2:
		info["Bus"] = fmt.Sprintf("BUS %d", p["bus-fx"])
	default:
		info["Bus"] = "off"
	}
	if g := p["mute-group"]; g > 0 {
		info["Mute group"] = strconv.Itoa(int(g))
	}
	info["Mode"] = []string{"forward", "reverse", "ping-pong", "reverse ping-pong"}[max(min(int(p["play-mode"]), 3), 0)]
	switch b := p["balance"]; {
	case b == 64 || b == 0:
		info["Pan"] = "C"
	case b < 64:
		info["Pan"] = "L" + strconv.Itoa(int(64-b))
	default:
		info["Pan"] = "R" + strconv.Itoa(int(b-64))
	}
	return info
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
