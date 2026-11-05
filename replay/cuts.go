package replay

import (
	"math"
	"sort"
)

// Cut is a part of the take (seconds of the audio) that is left out of the video. Everything else (captions, chapters, patterns, the camera)
// keeps the time of the take; only the playback and the video skip the cut, and the times shown for the video shift by what was cut before them.
type Cut struct {
	From float64 `json:"from"`
	To   float64 `json:"to"`
}

// NormalizeCuts sorts the cuts, clips them to the take (0..dur; dur <= 0 means no upper limit), drops the empty ones and joins the ones that touch.
func NormalizeCuts(cuts []Cut, dur float64) []Cut {
	c := make([]Cut, 0, len(cuts))
	for _, x := range cuts {
		if math.IsNaN(x.From) || math.IsNaN(x.To) {
			continue
		}
		x.From = math.Max(x.From, 0)
		if dur > 0 {
			x.To = math.Min(x.To, dur)
		}
		if x.To-x.From >= 0.01 {
			c = append(c, x)
		}
	}
	sort.Slice(c, func(i, j int) bool { return c[i].From < c[j].From })
	var out []Cut
	for _, x := range c {
		if n := len(out); n > 0 && x.From <= out[n-1].To+0.0005 {
			out[n-1].To = math.Max(out[n-1].To, x.To)
			continue
		}
		out = append(out, x)
	}
	return out
}

// KeepSpans are the parts of from..to that are not cut, in order.
func KeepSpans(cuts []Cut, from, to float64) []Cut {
	var out []Cut
	at := from
	for _, c := range NormalizeCuts(cuts, 0) {
		if c.To <= at {
			continue
		}
		if c.From >= to {
			break
		}
		if c.From > at {
			out = append(out, Cut{at, c.From})
		}
		at = math.Max(at, c.To)
	}
	if at < to {
		out = append(out, Cut{at, to})
	}
	return out
}

// OutTime is the time in the video of a time of the take: what is cut before it is taken off. A time inside a cut is the place where the cut is.
func OutTime(cuts []Cut, t float64) float64 {
	off := 0.0
	for _, c := range NormalizeCuts(cuts, 0) {
		if c.From >= t {
			break
		}
		off += math.Min(c.To, t) - c.From
	}
	return t - off
}
