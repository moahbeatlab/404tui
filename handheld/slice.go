package handheld

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/lab"
)

// waveCols is the width of the slicer's waveform in columns, one per pixel (the panel has the right of the screen).
const waveCols = 440

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
	a.sid, a.spad = id, n
	a.sl = formats.NewSlicing(s.Frames(), 1)
	a.at, a.zoom = 0, 0
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
		if zc, err := a.lab.ZeroCrossing(a.sid, a.at, 48); err == nil && a.sliceAt(zc) == i { // no click at the cut
			a.at = zc
		}
		if i < 0 || a.sl.Split(i, a.at) < 0 {
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
	if len(a.sl.Slices) < 2 {
		a.say("cut it first (A) or Y+A for auto", true)
		return
	}
	sb, _ := json.Marshal(a.sl)
	base := a.info.Pads[a.spad].Name
	// on pads that are free: the first run of empty pads after the one that was cut (then from the start), never on
	// top of a sample the project already has
	first := a.freeRun(len(a.sl.Slices), a.spad+1)
	if first < 0 {
		a.say(fmt.Sprintf("no room: %d empty pads in a row are needed (clear some with Y+B)", len(a.sl.Slices)), true)
		return
	}
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
		err = a.lab.Assign(n, m.ID, fmt.Sprintf("%s_%02d", fit(base, 14), i+1))
		a.lab.Free(m.ID)
		if err == nil {
			delete(a.pcm, n)
			done++
		}
	}
	a.refresh()
	a.bank, a.cur = first/16, first%16
	a.sl = nil
	a.scr = Pads
	a.say(fmt.Sprintf("%d slices on %s...", done, a.label(first)), false)
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
	return out
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
	stage := filepath.Join(dir, ".404h-writing-"+name)
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
