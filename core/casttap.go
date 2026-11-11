package core

import (
	"math"
	"sync"
	"time"

	"github.com/moahbeatlab/404tui/formats"
)

// The sound of an offline cast. With no unit (no audio input) the program plays samples and patterns itself; while the cast runs the
// audio output is wrapped in a castTap that notes what is played and when (the pattern is one looping sample that is replaced when it
// changes), and when the cast ends the notes are mixed into the cast's WAV the way the output played them: one voice (a sample, or a
// loop that a new one replaces from the same place) and the preview over it.

type tapKind int

const (
	tapPlay tapKind = iota
	tapLoop
	tapReplace
	tapPreview
	tapStopPreview
	tapStop
)

type tapEvent struct {
	at   time.Duration
	kind tapKind
	pcm  []int16 // stereo frames
}

type castTap struct {
	inner Audio
	start time.Time
	mu    sync.Mutex
	ev    []tapEvent
}

func (t *castTap) note(k tapKind, s formats.Sample) {
	e := tapEvent{at: time.Since(t.start), kind: k}
	if k != tapStop && k != tapStopPreview {
		e.pcm = stereoOf(s)
	}
	t.mu.Lock()
	t.ev = append(t.ev, e)
	t.mu.Unlock()
}

// stereoOf is a sample as stereo frames, a mono one on both sides, as the output plays it.
func stereoOf(s formats.Sample) []int16 {
	c := max(s.Channels, 1)
	out := make([]int16, 0, len(s.Samples)/c*2)
	for i := 0; i+c <= len(s.Samples); i += c {
		out = append(out, s.Samples[i], s.Samples[i+c-1])
	}
	return out
}

func (t *castTap) Play(s formats.Sample) error { t.note(tapPlay, s); return t.inner.Play(s) }
func (t *castTap) Stop()                       { t.note(tapStop, formats.Sample{}); t.inner.Stop() }

// castTapFull has the optional parts of the output as well (a loop that is replaced, a preview over it).
type castTapFull struct {
	*castTap
	loop looper
	rep  replacer
	prev previewer
}

func (t *castTapFull) Loop(s formats.Sample) error { t.note(tapLoop, s); return t.loop.Loop(s) }
func (t *castTapFull) ReplaceLoop(s formats.Sample) error {
	t.note(tapReplace, s)
	return t.rep.ReplaceLoop(s)
}
func (t *castTapFull) Preview(s formats.Sample) error {
	t.note(tapPreview, s)
	return t.prev.Preview(s)
}
func (t *castTapFull) StopPreview() { t.note(tapStopPreview, formats.Sample{}); t.prev.StopPreview() }

// tapOf wraps the output; a nil output stays nil (nothing to hear).
func tapOf(au Audio, start time.Time) (Audio, *castTap) {
	if au == nil {
		return nil, nil
	}
	base := &castTap{inner: au, start: start}
	lp, ok1 := au.(looper)
	rp, ok2 := au.(replacer)
	pv, ok3 := au.(previewer)
	if ok1 && ok2 && ok3 {
		return &castTapFull{castTap: base, loop: lp, rep: rp, prev: pv}, base
	}
	return base, base
}

// mix plays the notes into frames of stereo, n frames long.
func (t *castTap) mix(n int) []int16 {
	t.mu.Lock()
	ev := append([]tapEvent(nil), t.ev...)
	t.mu.Unlock()
	acc := make([]int32, n*2)
	var cur, pre []int16
	var curAt, preAt int
	var looping bool
	frame := func(at time.Duration) int { return min(max(int(math.Round(at.Seconds()*formats.SampleRate)), 0), n) }
	pos := 0
	render := func(to int) {
		for ; pos < to; pos++ {
			if cur != nil {
				acc[pos*2] += int32(cur[curAt*2])
				acc[pos*2+1] += int32(cur[curAt*2+1])
				if curAt++; curAt*2 >= len(cur) {
					curAt = 0
					if !looping {
						cur = nil
					}
				}
			}
			if pre != nil {
				acc[pos*2] += int32(pre[preAt*2])
				acc[pos*2+1] += int32(pre[preAt*2+1])
				if preAt++; preAt*2 >= len(pre) {
					pre = nil
				}
			}
		}
	}
	for _, e := range ev {
		render(frame(e.at))
		switch e.kind {
		case tapPlay, tapLoop:
			pre = nil
			cur, curAt, looping = e.pcm, 0, e.kind == tapLoop
		case tapReplace:
			if cur != nil && looping && len(e.pcm) > 0 {
				cur, curAt = e.pcm, curAt%(len(e.pcm)/2)
			} else {
				cur, curAt, looping = e.pcm, 0, true
			}
		case tapPreview:
			pre, preAt = e.pcm, 0
		case tapStopPreview:
			pre = nil
		case tapStop:
			cur, pre = nil, nil
		}
		if len(cur) == 0 {
			cur = nil
		}
		if len(pre) == 0 {
			pre = nil
		}
	}
	render(n)
	out := make([]int16, len(acc))
	for i, v := range acc {
		out[i] = int16(max(-32768, min(32767, v)))
	}
	return out
}
