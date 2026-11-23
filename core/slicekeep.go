package core

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"time"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
)

// The slicings are kept: a file's next to it (<file>.slices.json, in its folder), a pad's sample in the program's folder by the
// sound itself (slices/<hash>.json), so that opening it again finds the markers, the order, the slices' settings and the bars as
// they were left. A change is written about a second after it was made.

// keptSlicing is what is written.
type keptSlicing struct {
	Name    string            `json:"name"`
	Slicing *formats.Slicing  `json:"slicing"`
	Bars    keptBars          `json:"bars"`
	Saved   time.Time         `json:"saved"`
	Extra   map[string]string `json:"extra,omitempty"`
}

type keptBars struct {
	BPM     float64 `json:"bpm,omitempty"`
	FromTag bool    `json:"from_name,omitempty"`
	Conf    float64 `json:"heard,omitempty"`
	Down    int     `json:"bar1,omitempty"`
	DownSet bool    `json:"bar1_set,omitempty"`
	NoSnap  bool    `json:"no_snap,omitempty"`
}

// slicingPath is where the slicing in the slicer is kept: next to its file when it is a file in a folder that can be written to,
// else in the program's folder by the sound.
func (a *App) slicingPath() string {
	if a.spath != "" && app.IsLocal(a.spath) {
		p := a.spath + ".slices.json"
		if _, err := os.Stat(p); err == nil {
			return p
		}
		if f, err := os.CreateTemp(filepath.Dir(p), ".404tui-*"); err == nil { // the folder can be written to
			f.Close()
			os.Remove(f.Name())
			return p
		}
	}
	s, err := a.lab.Sample(a.sid)
	if err != nil {
		return ""
	}
	return filepath.Join(a.dataDir(), "slices", fmt.Sprintf("%016x.json", soundHash(s)))
}

// soundHash names a sound by what it is: its length and every 64th frame.
func soundHash(s formats.Sample) uint64 {
	h := fnv.New64a()
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(len(s.Samples)))
	h.Write(b[:])
	for i := 0; i < len(s.Samples); i += 64 {
		binary.LittleEndian.PutUint16(b[:2], uint16(s.Samples[i]))
		h.Write(b[:2])
	}
	return h.Sum64()
}

// slicingState is the slicing and the bars as text: what is written when it changed.
func (a *App) slicingState() string {
	if a.sl == nil {
		return ""
	}
	b, _ := json.Marshal(struct {
		S *formats.Slicing
		B sliceBars
	}{a.sl, a.bars})
	return fmt.Sprintf("%s%+v", b, a.bars)
}

// keepSlicingTick writes the slicing a second after a change (and not while the sample plays from the cursor).
func (a *App) keepSlicingTick() {
	if a.sl == nil || a.tap {
		return
	}
	st := a.slicingState()
	if st == a.slKept {
		a.slChanged = time.Time{}
		return
	}
	if a.slChanged.IsZero() {
		a.slChanged = time.Now()
		return
	}
	if time.Since(a.slChanged) < time.Second {
		return
	}
	a.keepSlicing()
}

// keepSlicing writes the slicing now.
func (a *App) keepSlicing() {
	if a.sl == nil {
		return
	}
	st := a.slicingState()
	p := a.slicingPath()
	if p == "" {
		return
	}
	k := keptSlicing{Name: a.sliceName(), Slicing: a.sl, Saved: time.Now(), Bars: keptBars{BPM: a.bars.bpm, FromTag: a.bars.fromTag,
		Conf: a.bars.conf, Down: a.bars.down, DownSet: a.bars.downSet, NoSnap: a.bars.noSnap}}
	b, err := json.MarshalIndent(k, "", " ")
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(p), 0o755)
	tmp := p + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil && os.Rename(tmp, p) == nil {
		a.slKept, a.slChanged = st, time.Time{}
	}
}

// restoreSlicing puts the slicing of last time back, when there is one for this sound; it says whether it did.
func (a *App) restoreSlicing() bool {
	a.slKept, a.slChanged = a.slicingState(), time.Time{}
	p := a.slicingPath()
	if p == "" {
		return false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	var k keptSlicing
	if json.Unmarshal(b, &k) != nil || k.Slicing == nil || k.Slicing.Frames != a.sl.Frames || len(k.Slicing.Slices) == 0 {
		return false
	}
	a.sl = k.Slicing
	a.bars = sliceBars{bpm: k.Bars.BPM, fromTag: k.Bars.FromTag, conf: k.Bars.Conf, down: k.Bars.Down, downSet: k.Bars.DownSet, noSnap: k.Bars.NoSnap}
	a.slKept = a.slicingState()
	a.slRestored = fmt.Sprintf("the slicing of %s is back: %d slices", k.Saved.Format("Jan 2 15:04"), len(k.Slicing.Slices))
	return true
}

// startAgain lets go of the slicing (and the one kept): one slice, the bars as they are found anew.
func (a *App) startAgain() {
	if a.sl == nil {
		return
	}
	if p := a.slicingPath(); p != "" {
		os.Remove(p)
	}
	a.sl = formats.NewSlicing(a.sl.Frames, 1)
	a.at = 0
	a.resetBars()
	a.slKept = a.slicingState()
	a.say("one slice again (the kept slicing is gone; u brings the slices back)", false)
}
