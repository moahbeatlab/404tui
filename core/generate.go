package core

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/generate"
	"github.com/moahbeatlab/404tui/lab"
	"github.com/moahbeatlab/404tui/proto"
)

// The generator in the editor: the patterns of a bank (a family of variations of one pattern, random ones, layers, a drift or an arc) are written
// into the next free slots after the kit's bank, as one step that can be undone. In live mode the editor's patterns go to the unit
// as they appear, so this is also how the generator puts its patterns on the unit. Each run takes a new seed.

// kitBank is the first bank whose pads, by their names, hold a kick, and the pad of each voice in it.
func (a *App) kitBank() (int, map[formats.Lane]proto.PadIndex) {
	if b := a.set.GenKit.Bank; b >= 1 && b <= 10 { // the bank chosen in Generator kit: its pads by their names
		pads := app.GuessRoles(a.padRows(b - 1))
		if pads == nil {
			pads = map[formats.Lane]proto.PadIndex{}
		}
		return b - 1, pads
	}
	for bank := 0; bank < 10; bank++ {
		if pads := app.GuessRoles(a.padRows(bank)); len(pads) > 0 {
			if _, ok := pads[formats.LaneKick]; ok {
				return bank, pads
			}
		}
	}
	return -1, nil
}

// autoKitBank is the first bank with a pad named like a kick, -1 when there is none.
func (a *App) autoKitBank() int {
	for bank := 0; bank < 10; bank++ {
		if _, ok := app.GuessRoles(a.padRows(bank))[formats.LaneKick]; ok {
			return bank
		}
	}
	return -1
}

func (a *App) generateArc(st generate.Style, mode string, bars int) {
	bank, pads, alts := a.generatorKit()
	if bank < 0 || len(pads) == 0 {
		a.say("no bank has a pad named like a kick: name the pads of the kit (kick, snare, hat...) or choose them in Generator kit", true)
		return
	}
	has := func(l formats.Lane) bool { _, ok := pads[l]; return ok }
	seed := rand.New(rand.NewSource(time.Now().UnixNano())).Int63n(1 << 30)
	results, err := generate.Generate(mode, st, bars, seed, has)
	if err != nil {
		a.say(err.Error(), true)
		return
	}

	var slots []int
	from := (bank+1)*16 + 1
	if generate.OnePattern(mode) { // one pattern: where the cursor is (the pattern in front, when it is empty), else the next free slot
		from = max(a.pat, 1)
	}
	for n := from; n <= 160 && len(slots) < len(results); n++ {
		if _, ok := a.patternDoc(n); !ok {
			slots = append(slots, n)
		}
	}
	if len(slots) < len(results) {
		a.say(fmt.Sprintf("only %d free pattern slots after the kit's bank, %d are needed", len(slots), len(results)), true)
		return
	}
	pre := a.lab.Snapshot()
	for i, r := range results {
		b, err := r.Pattern.BytesAlt(pads, alts)
		if err == nil {
			err = a.lab.SetPatternBytes(slots[i], b)
		}
		if err != nil {
			a.say(r.Section.Name+": "+err.Error(), true)
			return
		}
	}
	if generate.OnePattern(mode) { // one pattern leaves the tempo of the project alone
		a.commitUndo(pre, A, 0)
		a.openPattern(slots[0])
		a.say(fmt.Sprintf("%s: %s in %s, seed %d (%d BPM style)", st.Name, results[0].Section.Name, a.label(slots[0]-1), seed, st.BPM), false)
		return
	}
	bpm := float64(st.BPM)
	if err := a.lab.SetTempo(bpm); err != nil {
		a.say(err.Error(), true)
		return
	}
	seen := map[int]bool{}
	for _, n := range slots {
		if bk := (n - 1) / 16; !seen[bk] {
			seen[bk] = true
			a.lab.SetBankTempo(bk, bpm)
		}
	}
	a.commitUndo(pre, A, 0)
	a.openPattern(slots[0]) // the groove (the first of a family)
	a.say(fmt.Sprintf("%s %s: %s to %s from the kit in bank %c, seed %d, %.0f BPM", st.Name, mode, a.label(slots[0]-1), a.label(slots[len(slots)-1]-1), 'A'+bank, seed, bpm), false)
}

// patternDoc reads a pattern of the project; ok is false when the slot holds nothing.
func (a *App) patternDoc(n int) (lab.PatternDoc, bool) {
	var d lab.PatternDoc
	b, err := a.lab.Pattern(n)
	if err != nil || json.Unmarshal(b, &d) != nil {
		return d, false
	}
	return d, true
}
