package handheld

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/lab"
	"github.com/moahbeatlab/404tui/proto"
)

// Pattern templates: the genre grooves of the computer app (house, techno, DnB, trap, ...), written into the next
// free pattern. Which pad plays which voice is guessed from the sample names ("kick", "snare", "hat"...); a voice
// with no match takes a pad of the columns that has a sample, so a kit with other names still gets a beat.

var barChoices = []string{"1 bar", "2 bars", "3 bars", "4 bars", "5 bars", "6 bars", "7 bars", "8 bars"}

func (a *App) templatesMenu() {
	m := &menu{title: "new pattern from a template"}
	m.items = append(m.items, menuItem{label: "Blank pattern", vals: barChoices, val: 3, run: func(v int) { a.blankPattern(v + 1) }})
	for _, g := range formats.GrooveTemplates() {
		g := g
		m.items = append(m.items, menuItem{
			label: fmt.Sprintf("%s  %s", g.Name, shortTempo(g.Tempo)),
			vals:  barChoices, val: g.Bars() - 1,
			run: func(v int) { a.fromTemplate(g, v+1) },
		})
	}
	a.menu = m
}

// shortTempo is "122-127" from "122–127 BPM".
func shortTempo(t string) string {
	return strings.ReplaceAll(strings.TrimSpace(strings.TrimSuffix(t, "BPM")), "\u2013", "-") // the font has no en dash
}

// freePattern is the first pattern slot from the one in front on that holds nothing.
func (a *App) freePattern() int {
	for i := 0; i < 160; i++ {
		n := (a.pat-1+i)%160 + 1
		b, err := a.lab.Pattern(n)
		var d lab.PatternDoc
		if err != nil || json.Unmarshal(b, &d) != nil {
			return n
		}
	}
	return -1
}

func (a *App) blankPattern(bars int) {
	n := a.freePattern()
	if n < 0 {
		a.say("all 160 patterns are used", true)
		return
	}
	if err := a.lab.NewPattern(n, bars); err != nil {
		a.say(err.Error(), true)
		return
	}
	a.openPattern(n)
	a.say(fmt.Sprintf("a blank pattern of %d bars: %s", bars, a.label(n-1)), false)
}

func (a *App) openPattern(n int) {
	a.pat, a.step = n, 0
	a.refresh()
	a.autoCols()
	a.replay()
}

// laneMap says which pad plays each voice of a groove.
func (a *App) laneMap(g formats.GrooveTemplate) (map[formats.Lane]proto.PadIndex, []string) {
	rows := make([]app.PadRow, 0, 160)
	for i, p := range a.info.Pads {
		idx, ok := proto.NewPadIndex(uint16(i))
		if !ok {
			continue
		}
		rows = append(rows, app.PadRow{Pad: idx, HasSample: p.File, Name: p.Name})
	}
	pads := app.GuessRoles(rows)
	used := map[proto.PadIndex]bool{}
	for _, p := range pads {
		used[p] = true
	}
	// voices without a name match: the pads of the columns that have a sample, then any pad that has one
	var spare []proto.PadIndex
	cands := append([]int{}, a.cols[:]...)
	for i := range a.info.Pads {
		cands = append(cands, i)
	}
	for _, i := range cands {
		if idx, ok := proto.NewPadIndex(uint16(i)); ok && a.info.Pads[i].File && !used[idx] {
			spare = append(spare, idx)
			used[idx] = true
		}
	}
	var said []string
	for _, ln := range g.Lanes {
		p, ok := pads[ln]
		if !ok && len(spare) > 0 {
			p, spare = spare[0], spare[1:]
			pads[ln] = p
			ok = true
		}
		if ok {
			said = append(said, fmt.Sprintf("%s %s", ln.LaneName(), a.label(int(p.Index()))))
		}
	}
	return pads, said
}

func (a *App) fromTemplate(g formats.GrooveTemplate, bars int) {
	n := a.freePattern()
	if n < 0 {
		a.say("all 160 patterns are used", true)
		return
	}
	pads, said := a.laneMap(g)
	if len(pads) == 0 {
		a.say("no pad has a sample: put some on the pads first", true)
		return
	}
	b, err := g.BuildGroove(pads, bars)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	if err := a.lab.SetPatternBytes(n, b); err != nil {
		a.say(err.Error(), true)
		return
	}
	a.colsOwn = false // the columns follow the pads of the new pattern
	a.openPattern(n)
	a.say(fmt.Sprintf("%s %s: %s", a.label(n-1), g.Name, strings.Join(said, ", ")), false)
}
