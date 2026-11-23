package core

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/lab"
)

// Loops out of a whole track in the slicer: the track's tempo and a downbeat give bar lines, the cuts snap to them, a slice says
// how many bars it is, and one slice goes to a pad as a loop. Listening from the cursor (tap) skips back and forth through it.

// sliceBars is the bar grid of the track in the slicer.
type sliceBars struct {
	bpm     float64 // the track's tempo; 0: no bar grid
	fromTag bool    // the tempo was read from the name
	down    int     // a downbeat (frames): bar 1 starts there
	downSet bool    // the downbeat was set (else the start of the sample)
	noSnap  bool    // the cuts go where they are put, not to the nearest bar line
}

// tapSkip is how far L2 and R2 go while the track plays from the cursor.
const tapSkip = 10 * time.Second

var (
	bpmWord = regexp.MustCompile(`(?i)(\d{2,3}(?:\.\d+)?)\s*bpm`)
	bpmPart = regexp.MustCompile(`(?:^|[_\-\s.])(\d{2,3})(?:[_\-\s.]|$)`)
)

// bpmOfName is the tempo a sample's name gives: "125bpm", "125 BPM" or a number 60-200 standing alone ("track_125_Am").
func bpmOfName(name string) float64 {
	if m := bpmWord.FindStringSubmatch(name); m != nil {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil && v >= 40 && v <= 300 {
			return v
		}
	}
	for _, m := range bpmPart.FindAllStringSubmatch(name, -1) {
		if v, _ := strconv.Atoi(m[1]); v >= 60 && v <= 200 {
			return float64(v)
		}
	}
	return 0
}

// resetBars starts the bar grid of a sample that comes into the slicer: the tempo from its name, bar 1 at its start.
func (a *App) resetBars() {
	a.bars = sliceBars{}
	if a.spad >= 0 && a.spad < len(a.info.Pads) {
		if v := bpmOfName(a.info.Pads[a.spad].Name); v > 0 {
			a.bars = sliceBars{bpm: v, fromTag: true}
		}
	}
}

// barFrames is how long a bar (4/4) is at the track's tempo; 0 without a bar grid.
func (a *App) barFrames() float64 {
	if a.bars.bpm <= 0 {
		return 0
	}
	return 4 * 60 / a.bars.bpm * formats.SampleRate
}

// barLine is where bar k+1 starts (k from the downbeat, negative before it).
func (a *App) barLine(k int) int {
	return a.bars.down + int(math.Round(float64(k)*a.barFrames()))
}

// nearestBar is the bar line nearest to a frame, inside the sample.
func (a *App) nearestBar(f int) int {
	bf := a.barFrames()
	if bf <= 0 {
		return f
	}
	k := int(math.Round(float64(f-a.bars.down) / bf))
	at := a.barLine(k)
	for at < 0 {
		k++
		at = a.barLine(k)
	}
	for at >= a.sl.Frames && k > 0 {
		k--
		at = a.barLine(k)
	}
	return at
}

// snapping says whether a cut goes to the nearest bar line.
func (a *App) snapping() bool { return a.barFrames() > 0 && !a.bars.noSnap }

// barText is where a frame is in bars and beats: "bar 17.1"; "" without a bar grid.
func (a *App) barText(f int) string {
	bf := a.barFrames()
	if bf <= 0 {
		return ""
	}
	if f < a.bars.down {
		return "before bar 1"
	}
	pos := float64(f-a.bars.down) / bf
	bar := int(pos)
	beat := int((pos-float64(bar))*4) + 1
	return fmt.Sprintf("bar %d.%d", bar+1, beat)
}

// barsText is a length in bars: "8 bars ✓" when it is whole bars (to a millisecond), else "7.98 bars".
func (a *App) barsText(frames int) string {
	bf := a.barFrames()
	if bf <= 0 {
		return ""
	}
	n := float64(frames) / bf
	whole := math.Round(n)
	if whole >= 1 && math.Abs(float64(frames)-whole*bf) <= formats.SampleRate/1000 {
		return count(int(whole), "bar") + " ✓"
	}
	return fmt.Sprintf("%.2f bars", n)
}

// wholeBars says whether a slice is whole bars long.
func (a *App) wholeBars(frames int) bool { return strings.HasSuffix(a.barsText(frames), "✓") }

// setDownbeat puts bar 1 at the cursor (on the bar line nearest to it when it is near one, else where it is).
func (a *App) setDownbeat() {
	if a.sl == nil {
		return
	}
	if a.barFrames() <= 0 {
		a.say("no tempo for the bars yet: set it in the tools (CUT: Bars, the tempo)", true)
		return
	}
	at := a.at
	if a.tap { // while it plays the key comes a little late: the onset just before, up to 200 ms
		if on, err := a.lab.Transients(a.sid, 256, a.autoSensitivity()); err == nil {
			best := -1
			for _, o := range on {
				if o <= at && at-o <= formats.SampleRate/5 && o > best {
					best = o
				}
			}
			if best >= 0 {
				at = best
			}
		}
	}
	a.at = at
	a.bars.down, a.bars.downSet = at, true
	a.say(fmt.Sprintf("bar 1 starts at %.3f s: the cuts snap to the bars of %s BPM", float64(a.at)/formats.SampleRate, bpmText(a.bars.bpm)), false)
}

// askBarTempo asks for the track's tempo.
func (a *App) askBarTempo() {
	cur := ""
	if a.bars.bpm > 0 {
		cur = strconv.FormatFloat(a.bars.bpm, 'f', -1, 64)
	}
	a.askText("the tempo of the track (BPM)", "e.g. 125 or 122.5; empty: no bars", cur, func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			a.bars.bpm = 0
			a.say("no bars: the cuts go where they are put", false)
			return
		}
		v, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
		if err != nil || v < 40 || v > 300 {
			a.say("a tempo from 40 to 300 BPM", true)
			return
		}
		a.bars.bpm, a.bars.fromTag = v, false
		a.say(fmt.Sprintf("bars at %s BPM: put the cursor on a downbeat and set it (%s)", bpmText(v), downbeatKey()), false)
	})
}

// barCuts cuts the whole sample every n bars from the downbeat (a slice before it if it does not start the sample).
func (a *App) barCuts(n int) {
	bf := a.barFrames()
	if bf <= 0 {
		a.say("no tempo for the bars yet: set it first (Bars, the tempo)", true)
		return
	}
	sl := formats.NewSlicing(a.sl.Frames, 1)
	for k := 0; ; k += n {
		at := a.barLine(k)
		if at >= a.sl.Frames-formats.MinSliceFrames {
			break
		}
		if at <= formats.MinSliceFrames {
			continue
		}
		sl.Split(len(sl.Slices)-1, at)
	}
	a.sl = sl
	a.at = a.bars.down
	a.say(fmt.Sprintf("%d slices of %s from bar 1", len(sl.Slices), count(n, "bar")), false)
}

// downbeatKey names the key that sets the downbeat.
func downbeatKey() string { return tk("Sel+^") }

// ---- listening from the cursor

// tapSkipKey is L2 (back) and R2 (forward) while the track plays from the cursor: it goes on 10 s earlier or later.
func (a *App) tapSkipKey(fwd bool) {
	pos := a.at
	d := int(tapSkip.Seconds() * formats.SampleRate)
	if !fwd {
		d = -d
	}
	to := max(pos+d, 0)
	if to >= a.sl.Frames-1 {
		a.say("the end of the sample", false)
		return
	}
	a.tapFrom(to)
}

// tapFrom plays the sample from a frame on, the cursor going with it.
func (a *App) tapFrom(from int) {
	s, err := a.lab.Sample(a.sid)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	ch := max(s.Channels, 1)
	from = max(0, min(from, s.Frames()-1))
	a.tapStart, a.tapT0, a.at = from, time.Now(), from
	a.play(formats.Sample{Channels: s.Channels, Samples: s.Samples[from*ch:]})
	where := clockTime(from)
	if b := a.barText(from); b != "" {
		where += "  " + b
	}
	a.say(where, false)
}

// ---- one slice to a pad

// sliceToPad asks for a pad and puts the slice under the cursor there as a loop, its tempo the track's.
func (a *App) sliceToPad() {
	i := a.sliceAt(a.at)
	if i < 0 {
		a.say("the cursor is between slices", true)
		return
	}
	a.pickFreePad("put slice "+fmt.Sprint(i+1)+" on a pad", a.spad+1, func(n int) { a.sliceTo(i, n) })
}

func (a *App) sliceTo(i, n int) {
	sb, _ := json.Marshal(a.sl)
	mb, err := a.lab.SliceAudio(a.sid, sb, i)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	var m lab.Meta
	json.Unmarshal(mb, &m)
	sg := a.sl.Slices[i]
	base := fit(strings.TrimSpace(a.info.Pads[a.spad].Name), 10)
	name := fmt.Sprintf("%s_s%d", base, i+1)
	if a.barFrames() > 0 && sg.Start >= a.bars.down {
		name = fmt.Sprintf("%s_b%d", base, int(float64(sg.Start-a.bars.down)/a.barFrames()+0.5)+1)
	}
	err = a.assignPad(n, m.ID, name)
	a.lab.Free(m.ID)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	delete(a.pcm, n)
	said := fmt.Sprintf("slice %d on %s", i+1, a.label(n))
	if a.barFrames() > 0 {
		a.lab.SetPadParam(n, 0x6B, 1) // loop
		a.lab.SetPadParam(n, 0x6F, int32(math.Round(a.bars.bpm*100)))
		said += fmt.Sprintf(" as a loop, %s at %s BPM", a.barsText(sg.End-sg.Start), bpmText(a.bars.bpm))
	}
	a.refresh()
	a.say(said, false)
}

// loopSlice plays the slice under the cursor in a loop (whole bars, on a computer): the seam is heard; X again stops it.
func (a *App) loopSlice() bool {
	i := a.sliceAt(a.at)
	lp, ok := a.Audio.(looper)
	if i < 0 || !ok || !Terminal || a.seqOn || !a.wholeBars(a.sl.Slices[i].End-a.sl.Slices[i].Start) {
		return false
	}
	sb, _ := json.Marshal(a.sl)
	mb, err := a.lab.SliceAudio(a.sid, sb, i)
	if err != nil {
		return false
	}
	var m lab.Meta
	json.Unmarshal(mb, &m)
	s, err := a.lab.Sample(m.ID)
	a.lab.Free(m.ID)
	if err != nil || lp.Loop(s) != nil {
		return false
	}
	a.soundUntil = time.Now().Add(24 * time.Hour) // X stops it
	a.say(fmt.Sprintf("slice %d in a loop (%s): x stops it", i+1, a.barsText(s.Frames())), false)
	return true
}
