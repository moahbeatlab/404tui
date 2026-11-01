package handheld

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/lab"
)

// waveCols is the width of the slicer's waveform in columns, one per pixel.
const waveCols = 600

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
		step := col * 2 * (1 + repeat/2)
		if k == Left {
			step = -step
		}
		a.at = min(max(a.at+step, 0), frames-1)
	case Up, Down:
		a.at = a.jump(k == Up)
	case A:
		i := a.sliceAt(a.at)
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

// autoSlice cuts where sounds start.
func (a *App) autoSlice() {
	sb, _ := json.Marshal(a.sl)
	args, _ := json.Marshal(lab.SlicerArgs{ID: a.sid, Limit: 16, Sens: 0.5, Anchor: -1})
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
func (a *App) slicesToPads() {
	if len(a.sl.Slices) < 2 {
		a.say("cut it first (A) or Y+A for auto", true)
		return
	}
	sb, _ := json.Marshal(a.sl)
	base := a.info.Pads[a.spad].Name
	first, done := a.spad+1, 0
	for i := range a.sl.Slices {
		n := first + i
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

// unzipTo writes a zip under dir and returns the paths it wrote.
func unzipTo(zb []byte, dir string) ([]string, error) {
	zr, err := zip.NewReader(bytes.NewReader(zb), int64(len(zb)))
	if err != nil {
		return nil, err
	}
	var names []string
	for _, f := range zr.File {
		dst := filepath.Join(dir, filepath.FromSlash(f.Name))
		if !strings.HasPrefix(dst, filepath.Clean(dir)+string(filepath.Separator)) {
			return nil, fmt.Errorf("bad path %q", f.Name)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, err
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return nil, err
		}
		names = append(names, f.Name)
	}
	return names, nil
}
