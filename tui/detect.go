package tui

import (
	"fmt"
	"slices"

	"github.com/moahbeatlab/404tui/proto"
)

// The unit sends every note a pattern plays over MIDI, on the channel of the pad's bank. The pads
// heard since the pattern started are therefore a part of the pads of the pattern that plays; the
// slot whose pads fit them best is the pattern that plays.

// detectPattern looks for the pattern that plays among the slots, from the pads heard since the last
// start, and makes it the one the Perform view shows (marked as detected). A slot that was chosen by
// hand stays as long as it fits what is heard.
func (m Model) detectPattern() Model {
	if m.mon == nil || len(m.mon.played) == 0 {
		return m
	}
	heard := map[int]bool{}
	for k := range m.mon.played {
		heard[k] = true
	}
	if m.perfNow != nil && !m.perfAuto && m.slotFits(*m.perfNow, heard) {
		return m
	}
	var best []int // slot indexes, fewest extra pads first
	extra := map[int]int{}
	for _, rows := range m.patterns {
		for _, r := range rows {
			if !r.Exists || !m.slotFits(r.Slot, heard) {
				continue
			}
			n := len(r.Pads) - len(heard)
			extra[int(r.Slot.Index())] = n
			best = append(best, int(r.Slot.Index()))
		}
	}
	if len(best) == 0 {
		return m
	}
	slices.SortFunc(best, func(a, b int) int { return extra[a] - extra[b] })
	// An ambiguous result (two slots as close) waits for more pads to be heard.
	if len(best) > 1 && extra[best[0]] == extra[best[1]] {
		return m
	}
	slot, _ := proto.NewPadIndex(uint16(best[0]))
	if m.perfNow != nil && *m.perfNow == slot && m.perfAuto {
		return m
	}
	m.perfNow, m.perfAuto = &slot, true
	m.message = fmt.Sprintf("the pattern that plays is %s (heard from the unit)", slot)
	m.logCue()
	return m
}

// slotFits says whether every pad heard (bank*16 + pad) is one of the pads of the slot's pattern.
func (m Model) slotFits(slot proto.PadIndex, heard map[int]bool) bool {
	var pads []int
	for _, rows := range m.patterns {
		for _, r := range rows {
			if r.Slot.Index() == slot.Index() {
				if !r.Exists {
					return false
				}
				for _, p := range r.Pads {
					pads = append(pads, int(p.Bank())*16+int(p.Pad()))
				}
			}
		}
	}
	for k := range heard {
		if !slices.Contains(pads, k) {
			return false
		}
	}
	return len(pads) > 0
}
