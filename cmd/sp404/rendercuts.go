package main

import (
	"fmt"
	"math"
	"strings"

	"github.com/moahbeatlab/404tui/replay"
)

// span is a stretch of the take that goes into the video: n frames from the time `from` of the take, which start at frame `first` of the video.
type span struct {
	from      float64
	n, first  int
	cutBefore bool // a cut (or the start of the part) comes before it: its sound fades in
	cutAfter  bool
}

// planFrames lists what goes into the video: the parts of start..end that are not cut, as frames. Every part takes whole frames, so the sound of a
// part is exactly as long as its pictures and the video stays in step with it however many cuts there are.
func planFrames(cuts []replay.Cut, start, end float64, fps int) (spans []span, times []float64) {
	for _, k := range replay.KeepSpans(cuts, start, end) {
		n := int((k.To - k.From) * float64(fps))
		if n < 1 {
			continue
		}
		spans = append(spans, span{from: k.From, n: n, first: len(times), cutBefore: k.From > start+1e-9, cutAfter: k.To < end-1e-9})
		for i := 0; i < n; i++ {
			times = append(times, k.From+float64(i)/float64(fps))
		}
	}
	return spans, times
}

// audioFilter joins the sound of the parts (from input 1) with a very short fade at every cut, so that a join does not click.
func audioFilter(spans []span, fps int) string {
	const fade = 0.012
	var parts, names []string
	for i, s := range spans {
		d := float64(s.n) / float64(fps)
		f := fmt.Sprintf("[1:a]atrim=start=%.4f:duration=%.4f,asetpts=PTS-STARTPTS", s.from, d)
		if s.cutBefore {
			f += fmt.Sprintf(",afade=t=in:st=0:d=%.3f", fade)
		}
		if s.cutAfter {
			f += fmt.Sprintf(",afade=t=out:st=%.4f:d=%.3f", math.Max(d-fade, 0), fade)
		}
		parts = append(parts, f+fmt.Sprintf("[a%d]", i))
		names = append(names, fmt.Sprintf("[a%d]", i))
	}
	return strings.Join(parts, ";") + ";" + strings.Join(names, "") + fmt.Sprintf("concat=n=%d:v=0:a=1[a]", len(spans))
}
