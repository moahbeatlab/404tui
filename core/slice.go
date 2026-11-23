package core

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/lab"
)

// openSlicer takes the sample of the pad in front into the slicer: one slice over the whole of it.
func (a *App) openSlicer() {
	n := a.padN()
	id, ok := a.padSample(n)
	if !ok {
		a.sl = nil
		a.say("pick a sample for "+a.label(n)+" first (A)", true)
		return
	}
	s, err := a.lab.Sample(id)
	if err != nil {
		a.sl = nil
		a.say(err.Error(), true)
		return
	}
	if a.sl != nil && a.slicingState() != a.slKept { // the slicing before is written first
		a.keepSlicing()
	}
	a.sid, a.spad, a.sfile, a.spath = id, n, "", ""
	a.sl = formats.NewSlicing(s.Frames(), 1)
	a.slMarks = nil
	a.resetBars()
	a.slRestored = ""
	if a.restoreSlicing() {
		a.say(a.slRestored+" (tools: Start again)", false)
	}
	a.at, a.zoom = 0, 0
	a.slUndo, a.slRedo = nil, nil
}

// view is the window of frames the waveform shows, around the cursor.
func (a *App) view() (from, to int) {
	frames := a.sl.Frames
	width := max(frames>>a.zoom, min(frames, 2000))
	from = min(max(a.at-width/2, 0), max(frames-width, 0))
	return from, from + width
}

func (a *App) sliceAt(frame int) int {
	for i, s := range a.sl.Slices {
		if frame >= s.Start && frame < s.End {
			return i
		}
	}
	return -1
}

// cursorPixels is how many pixels of the waveform the cursor moves at a press of a key held for repeat units of 160 ms:
// 2 at first, 8 after half a second, 32 after a second, 128 after a second and a half.
func cursorPixels(repeat int) int {
	switch {
	case repeat >= 10:
		return 128
	case repeat >= 6:
		return 32
	case repeat >= 3:
		return 8
	}
	return 2
}

func (a *App) sliceKey(k Key, repeat int) {
	if a.sl == nil {
		a.go_(Pads)
		return
	}
	if a.tap {
		a.tapKey(k)
		return
	}
	if a.lane {
		a.laneKey(k)
		return
	}
	frames := a.sl.Frames
	from, to := a.view()
	col := max((to-from)/waveCols, 1)
	switch k {
	case Left, Right:
		step := col * cursorPixels(repeat) // a held key runs from fine to far: 2, 8, 32, then 128 pixels at a press
		if k == Left {
			step = -step
		}
		a.at = min(max(a.at+step, 0), frames-1)
	case Up, Down:
		a.at = a.jump(k == Up)
	case A:
		i := a.sliceAt(a.at)
		if a.snapping() { // on the bar line: the loop is whole bars
			a.at = a.nearestBar(a.at)
			i = a.sliceAt(a.at)
		} else if zc, err := a.lab.ZeroCrossing(a.sid, a.at, 48); err == nil && a.sliceAt(zc) == i { // no click at the cut
			a.at = zc
		}
		if i < 0 { // in no slice (the rest of a slice that was made shorter): a new slice starts here
			if a.sl.StartIn(a.at) < 0 {
				a.say("cannot start a slice there: too close to the next one", true)
				return
			}
			a.say(fmt.Sprintf("%d slices: a new one starts here", len(a.sl.Slices)), false)
			return
		}
		if a.sl.Split(i, a.at) < 0 {
			a.say("cannot cut there: too close to an edge", true)
			return
		}
		a.say(fmt.Sprintf("%d slices", len(a.sl.Slices)), false)
	case B:
		if len(a.sl.Slices) <= 1 {
			a.go_(Pads)
			return
		}
		i := a.sliceAt(a.at)
		if i < 0 || !a.sl.Remove(i) {
			a.say("nothing to join", true)
			return
		}
		a.say(fmt.Sprintf("%d slices", len(a.sl.Slices)), false)
	case X:
		if Terminal && (a.orderOn || time.Now().Before(a.soundUntil)) { // on a computer X stops what sounds
			a.stop()
			a.say("stopped", false)
			return
		}
		if a.loopSlice() { // a slice of whole bars plays in a loop
			return
		}
		a.playSlice()
	}
}

// jump is the next slice start after the cursor (or the one before it).
func (a *App) jump(next bool) int {
	best := -1
	for _, s := range a.sl.Slices {
		if next && s.Start > a.at && (best < 0 || s.Start < best) {
			best = s.Start
		}
		if !next && s.Start < a.at && s.Start > best {
			best = s.Start
		}
	}
	if best < 0 {
		if next {
			return a.sl.Frames - 1
		}
		return 0
	}
	return best
}

func (a *App) playSlice() {
	i := a.sliceAt(a.at)
	if i < 0 {
		a.say("the cursor is between slices", true)
		return
	}
	sb, _ := json.Marshal(a.sl)
	mb, err := a.lab.SliceAudio(a.sid, sb, i)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	var m lab.Meta
	json.Unmarshal(mb, &m)
	s, err := a.lab.Sample(m.ID)
	a.lab.Free(m.ID)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	a.play(s)
	a.slPlay = slicePlay{slice: i, frames: s.Frames(), at: time.Now()}
}

// slicePlay is the slice that x played: for the playhead.
type slicePlay struct {
	slice, frames int
	at            time.Time
	loop          bool // played in a loop, until X
}

// playhead is where in the sample what plays now is (a slice, or the order's step), false when nothing of the slicer plays.
func (a *App) playhead() (int, bool) {
	if a.sl == nil || a.tap {
		return 0, false
	}
	slice, off, of := -1, 0, 0
	if a.orderOn || time.Now().Before(a.soundUntil) && !a.playT0.IsZero() && a.slPlay.at.Before(a.playT0) {
		i := a.playingStep()
		if i < 0 || a.placed[i].Rest {
			return 0, false
		}
		p := a.placed[i]
		frame := int(time.Since(a.playT0).Seconds() * formats.SampleRate)
		if a.orderOn && a.orderFrames > 0 {
			frame %= a.orderFrames
		}
		slice, off, of = p.Slice, frame-p.At, p.Frames
	} else if !a.slPlay.at.IsZero() && time.Now().Before(a.soundUntil) {
		slice, of = a.slPlay.slice, a.slPlay.frames
		off = int(time.Since(a.slPlay.at).Seconds() * formats.SampleRate)
		if a.slPlay.loop {
			off %= of
		}
	}
	if slice < 0 || slice >= len(a.sl.Slices) || of <= 0 {
		return 0, false
	}
	sg := a.sl.Slices[slice]
	n := sg.End - sg.Start
	d := min(int(float64(off)/float64(of)*float64(n)), n-1) // the played length (pitch, rolls) laid over the slice
	if of <= n {
		d = min(off, n-1)
	}
	if sg.Reverse {
		return sg.End - 1 - d, true
	}
	return sg.Start + d, true
}

func (a *App) autoSensitivity() float64 {
	if a.autoSens > 0 {
		return a.autoSens
	}
	return 0.5
}

func (a *App) autoLimit() int {
	if a.autoMax > 0 {
		return a.autoMax
	}
	return 16
}

// equalParts cuts the whole sample into n slices of the same length.
func (a *App) equalParts(n int) {
	a.sl = formats.NewSlicing(a.sl.Frames, n)
	a.at = 0
	a.say(fmt.Sprintf("%d equal slices", len(a.sl.Slices)), false)
}

// randomParts cuts the whole sample into about n slices at random places (a new dice every time).
func (a *App) randomParts(n int) {
	a.sl.Seed++
	a.sliceOp("random", lab.SlicerArgs{N: n, Seed: a.sl.Seed, Anchor: -1}, "%d slices at random places")
}

// gridParts cuts the sample on the tempo grid of the pad's bank: a slice for every step (a beat fraction).
func (a *App) gridParts(beats float64) {
	step := 48000 * 60 / a.tempo() * beats
	a.sliceOp("grid", lab.SlicerArgs{Step: step, Anchor: -1}, "%d slices on the grid")
}

// sliceOp runs one of the engine's slicing operations on the slicing in the slicer.
func (a *App) sliceOp(op string, args lab.SlicerArgs, said string) {
	sb, _ := json.Marshal(a.sl)
	ab, _ := json.Marshal(args)
	out, err := a.lab.Slicing(op, sb, ab)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	sl := &formats.Slicing{}
	if json.Unmarshal(out, sl) != nil {
		return
	}
	seed := a.sl.Seed
	a.sl, a.at = sl, 0
	a.sl.Seed = seed
	a.say(fmt.Sprintf(said, len(sl.Slices)), false)
}

// autoSlice cuts where sounds start.
func (a *App) autoSlice() {
	sb, _ := json.Marshal(a.sl)
	args, _ := json.Marshal(lab.SlicerArgs{ID: a.sid, Limit: a.autoLimit(), Sens: a.autoSensitivity(), Anchor: -1})
	out, err := a.lab.Slicing("onsets", sb, args)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	sl := &formats.Slicing{}
	if json.Unmarshal(out, sl) != nil {
		return
	}
	a.sl = sl
	a.at = 0
	a.say(fmt.Sprintf("%d slices at the onsets", len(sl.Slices)), false)
}

// slicesToPads puts each slice on its own pad, starting after the pad that was cut.
func (a *App) slicesToPads(shuffled bool) {
	if len(a.sl.Slices) < 2 && a.slicesMarked() == 0 {
		a.say("cut it first (A) or Y+A for auto", true)
		return
	}
	// on pads that are free: the first run of empty pads after the one that was cut (then from the start), never on
	// top of a sample the project already has
	n := len(a.padOrder())
	first := a.freeRun(n, a.spad+1)
	if first < 0 {
		a.say(fmt.Sprintf("no room: %d empty pads in a row are needed (clear some with Y+B)", n), true)
		return
	}
	a.slicesToPadsFrom(first, shuffled)
}

// slicesToPadsPick asks where the slices go: the first pad of a run of empty ones.
func (a *App) slicesToPadsPick(shuffled bool) {
	n := len(a.padOrder())
	if n < 2 && a.slicesMarked() == 0 {
		a.say("cut it first (A) or Y+A for auto", true)
		return
	}
	start := a.freeRun(n, a.spad+1)
	if start < 0 {
		start = a.spad + 1
	}
	a.pickPad(fmt.Sprintf("First pad of the %d slices", n), start, false, func(first int) {
		for i := 0; i < n; i++ {
			if first+i > 159 || a.info.Pads[first+i].File {
				a.say(fmt.Sprintf("%d empty pads in a row are needed from %s", n, a.label(first)), true)
				return
			}
		}
		a.slicesToPadsFrom(first, shuffled)
	})
}

func (a *App) slicesToPadsFrom(first int, shuffled bool) {
	sb, _ := json.Marshal(a.sl)
	base := a.sliceName()
	done := 0
	order := a.padOrder()
	if shuffled { // the slices on the pads in a random order
		a.sl.Seed++
		rand.New(rand.NewSource(a.sl.Seed)).Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
	}
	for pos, i := range order {
		n := first + pos
		if n > 159 {
			break
		}
		mb, err := a.lab.SliceAudio(a.sid, sb, i)
		if err != nil {
			continue
		}
		var m lab.Meta
		json.Unmarshal(mb, &m)
		err = a.assignPad(n, m.ID, fmt.Sprintf("%s_%02d", fit(base, 14), i+1))
		a.lab.Free(m.ID)
		if err == nil {
			delete(a.pcm, n)
			a.setPadTempo(n, a.bars.bpm) // the track's tempo (from its name, heard or set)
			done++
		}
	}
	a.refresh()
	said := fmt.Sprintf("%d slices on %s to %s", done, a.label(first), a.label(first+done-1))
	if a.bars.bpm > 0 {
		said += fmt.Sprintf(" at %s BPM", bpmText(a.bars.bpm))
	}
	a.say(said+": the markers stay (1 shows the pads)", false)
}

// padOrder is the slices in the order they go to pads: the play order (each slice once, at its first place), then the
// slices that are not in it, so a shuffled order is the order on the pads too.
func (a *App) padOrder() []int {
	n := len(a.sl.Slices)
	seen := make([]bool, n)
	out := make([]int, 0, n)
	for _, i := range a.sl.Order {
		if i >= 0 && i < n && !seen[i] {
			seen[i] = true
			out = append(out, i)
		}
	}
	for i := 0; i < n; i++ {
		if !seen[i] {
			out = append(out, i)
		}
	}
	if a.slicesMarked() > 0 { // marked slices: only they go
		var m []int
		for _, i := range out {
			if a.slMarks[a.sl.Slices[i].Start] {
				m = append(m, i)
			}
		}
		out = m
	}
	return out
}

// slicesMarked is how many slices are marked (marks of slices that are gone do not count).
func (a *App) slicesMarked() int {
	n := 0
	for _, s := range a.sl.Slices {
		if a.slMarks[s.Start] {
			n++
		}
	}
	return n
}

// MarkSlice is m of a keyboard in the slicer: the slice under the cursor marked or not; all is M: every slice, or none.
func (a *App) MarkSlice(all bool) {
	if a.scr != Slice || a.sl == nil || a.Modal() || a.prompt != nil || a.lane {
		return
	}
	a.CancelShift()
	if a.slMarks == nil {
		a.slMarks = map[int]bool{}
	}
	if all {
		on := a.slicesMarked() < len(a.sl.Slices)
		a.slMarks = map[int]bool{}
		if on {
			for _, s := range a.sl.Slices {
				a.slMarks[s.Start] = true
			}
		}
	} else {
		i := a.sliceAt(a.at)
		if i < 0 {
			a.say("the cursor is in no slice", true)
			return
		}
		st := a.sl.Slices[i].Start
		a.slMarks[st] = !a.slMarks[st]
		if !a.slMarks[st] {
			delete(a.slMarks, st)
		}
	}
	if n := a.slicesMarked(); n > 0 {
		a.say(fmt.Sprintf("%d of %d slices marked: only they go to pads (m, M none)", n, len(a.sl.Slices)), false)
	} else {
		a.say("no slice marked: all of them go to pads", false)
	}
}

// freeRun is the first pad at or after from (wrapping round to the start) that begins n empty pads in a row, or -1.
func (a *App) freeRun(n, from int) int {
	pads := a.info.Pads
	free := func(i int) bool { return i < len(pads) && !pads[i].File }
	for off := 0; off < 160; off++ {
		start := (from + off) % 160
		if start+n > 160 {
			continue
		}
		ok := true
		for j := 0; j < n; j++ {
			if !free(start + j) {
				ok = false
				break
			}
		}
		if ok {
			return start
		}
	}
	return -1
}

// exportTo writes the project as the folder dir/<project name>, one file at a time, into a staging folder that
// replaces the old folder when all of it is written: a half-written project never stands where a good one was, and
// files of an earlier save that the project no longer has do not stay behind. It returns the paths it wrote.
func (a *App) exportTo(dir string) ([]string, error) {
	name := a.ProjectName()
	final := filepath.Join(dir, name)
	stage := filepath.Join(dir, ".404tui-writing-"+name)
	if err := os.RemoveAll(stage); err != nil {
		return nil, err
	}
	var names []string
	// the folders the unit's own exports have, also when one is empty (PICTURE usually is)
	for _, d := range []string{"PTN", "SMPL", "PICTURE"} {
		if err := os.MkdirAll(filepath.Join(stage, d), 0o755); err != nil {
			return nil, err
		}
	}
	err := a.lab.ExportEach(func(rel string, data []byte) error {
		dst := filepath.Join(stage, filepath.FromSlash(rel))
		if !strings.HasPrefix(dst, stage+string(filepath.Separator)) {
			return fmt.Errorf("bad path %q", rel)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return err
		}
		names = append(names, name+"/"+rel)
		return nil
	})
	if err != nil {
		os.RemoveAll(stage)
		return nil, err
	}
	if err := os.RemoveAll(final); err != nil {
		return nil, err
	}
	if err := os.Rename(stage, final); err != nil {
		return nil, err
	}
	return names, nil
}
