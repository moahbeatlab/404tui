package generate

import "github.com/moahbeatlab/404tui/formats"

// Rolls are fills that speed up: the hits come closer (quarters, 8ths, 16ths, then 32nds as a step that plays two hits) and louder, or are
// doubled by a flam. The steps are 16ths, so a 32nd is a step with a ratchet of 2 and a triplet of 16ths one of 3.
var RollKinds = []string{"accelerating roll", "32nd roll", "snare triplets", "flam run", "tom roll", "kick roll", "hat roll", "clap roll"}

// rollOn lays hits of a lane on the steps from to (not including) of a bar, every gap steps, rising in velocity from v0 to v1; from the step
// ratchetFrom on (-1: never) each hit plays n times.
func (p *Pattern) rollOn(c *Ctx, lane formats.Lane, bar, from, to, gap int, v0, v1 uint8, ratchetFrom, n int) {
	if !c.has(lane) || !c.open(lane) {
		return
	}
	r := p.ensure(lane)
	for i := from; i < to; i += gap {
		v := int(v0) + (int(v1)-int(v0))*(i-from)/max(to-from-1, 1)
		idx := bar*16 + i
		r.Steps[idx] = uint8(max(min(v, 127), 1))
		if ratchetFrom >= 0 && i >= ratchetFrom {
			r.setRatchet(idx, n)
		}
	}
}

// rollLane is the lane a roll is played on: the snare when the kit has one (the style's backbeat may be a clap), else the backbeat's.
func (p *Pattern) rollLane(c *Ctx) formats.Lane {
	if c.has(formats.LaneSnare) && c.open(formats.LaneSnare) {
		return formats.LaneSnare
	}
	return p.backbeat(c)
}

// quiet takes the lanes that would play over a roll out of the steps from to.
func (p *Pattern) quiet(c *Ctx, bar, from, to int, lanes ...formats.Lane) {
	for _, l := range lanes {
		if r := p.Row(l); r != nil && c.open(l) {
			for i := from; i < to; i++ {
				r.Steps[bar*16+i] = 0
			}
		}
	}
}

// roll writes one of the RollKinds into the end of a bar.
func (p *Pattern) roll(c *Ctx, kind string, bar int) {
	snare := p.rollLane(c)
	switch kind {
	case "accelerating roll": // the whole bar: quarters, 8ths, 16ths, then 32nds, from soft to loud
		p.quiet(c, bar, 8, 16, formats.LaneClosedHat, formats.LaneOpenHat, formats.LaneShaker)
		p.rollOn(c, snare, bar, 0, 8, 4, 35, 55, -1, 1)
		p.rollOn(c, snare, bar, 8, 12, 2, 65, 85, -1, 1)
		p.rollOn(c, snare, bar, 12, 14, 1, 95, 105, -1, 1)
		p.rollOn(c, snare, bar, 14, 16, 1, 115, 127, 14, 2)
	case "32nd roll": // the last beat in 32nds
		p.quiet(c, bar, 12, 16, formats.LaneClosedHat, formats.LaneOpenHat)
		p.rollOn(c, snare, bar, 12, 16, 1, 60, 127, 12, 2)
	case "snare triplets": // triplets over the second half
		p.quiet(c, bar, 8, 16, formats.LaneClosedHat, formats.LaneOpenHat)
		p.rollOn(c, snare, bar, 8, 16, 2, 70, 125, 8, 3)
	case "flam run": // flams on the last beats
		p.quiet(c, bar, 10, 16, formats.LaneClosedHat, formats.LaneOpenHat)
		if c.has(snare) && c.open(snare) {
			r := p.ensure(snare)
			for k, s := range []int{10, 12, 14, 15} {
				r.Steps[bar*16+s] = uint8(85 + k*12)
				r.setFlam(bar*16+s, 22+k*4)
			}
		}
	case "tom roll": // down the toms, 16ths, the last beat in 32nds
		p.quiet(c, bar, 8, 16, formats.LaneClosedHat, formats.LaneOpenHat)
		p.rollOn(c, formats.LaneTom, bar, 8, 16, 1, 70, 120, 14, 2)
	case "kick roll": // the kick goes into 32nds before the next bar
		p.rollOn(c, formats.LaneKick, bar, 12, 16, 1, 90, 124, 14, 2)
	case "hat roll": // triplets on the hats over the last two beats, the open hat out
		p.quiet(c, bar, 8, 16, formats.LaneOpenHat)
		p.rollOn(c, formats.LaneClosedHat, bar, 8, 16, 1, 60, 110, 12, 3)
	case "clap roll":
		p.quiet(c, bar, 10, 16, formats.LaneClosedHat, formats.LaneOpenHat)
		p.rollOn(c, formats.LaneClap, bar, 10, 16, 1, 55, 120, 14, 2)
	}
}
