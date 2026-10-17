package app_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
)

func TestJunkIsRecognisedAndNeverAudio(t *testing.T) {
	for _, name := range []string{"._E_126_S_23.wav", ".DS_Store", "Thumbs.db", "desktop.ini", "__MACOSX", "._x"} {
		if !app.IsJunk(name) {
			t.Errorf("%q is junk", name)
		}
	}
	for _, name := range []string{"kick.wav", "My Samples", "_x.wav", "a._b.wav", ".hidden"} {
		if app.IsJunk(name) {
			t.Errorf("%q is not junk", name)
		}
	}
	// The macOS companion of a WAV has the extension but is not audio.
	if formats.IsAudioFile("._E_126_S_23.wav") || !formats.IsAudioFile("E_126_S_23.wav") {
		t.Error("IsAudioFile judges the macOS companion file wrongly")
	}
	for path, want := range map[string]app.Volume{
		"SD:IMPORT/x.wav": app.VolumeCard, "ROLAND/SP-404MKII": app.VolumeUnit, filepath.Join(os.TempDir(), "x.wav"): app.VolumeComputer, "~/x": app.VolumeComputer,
	} {
		if got := app.VolumeOf(path); got != want {
			t.Errorf("VolumeOf(%q) = %v, want %v", path, got, want)
		}
	}
}

func cardFiles(s interface {
	File(string) ([]byte, bool)
}, paths ...string) []bool {
	out := make([]bool, len(paths))
	for i, p := range paths {
		_, out[i] = s.File(p)
	}
	return out
}

func TestDeletePathsRemovesFilesAndWholeFolders(t *testing.T) {
	d, s := connect(t)
	r := app.NewRouted(d)
	for _, f := range []string{"SD:IMPORT/._a.wav", "SD:IMPORT/a.wav", "SD:IMPORT/pack/deep/x.wav", "SD:IMPORT/pack/y.wav", "SD:IMPORT/keep.wav"} {
		s.AddFile(f, []byte("x"))
	}
	count, err := app.CountPaths(r, []string{"SD:IMPORT/pack", "SD:IMPORT/._a.wav"})
	if err != nil || count != (app.Count{Files: 3, Folders: 2}) {
		t.Fatalf("count = %+v (%v)", count, err)
	}
	var seen []string
	deleted, failed := app.DeletePaths(r, []string{"SD:IMPORT/pack", "SD:IMPORT/._a.wav", "SD:IMPORT/missing.wav"}, func(i int, p string) { seen = append(seen, p) })
	if deleted != 2 || len(failed) != 1 || !strings.Contains(failed[0], "missing.wav") || len(seen) != 3 {
		t.Errorf("deleted %d, failed %v, saw %v", deleted, failed, seen)
	}
	if got := cardFiles(s, "SD:IMPORT/pack/deep/x.wav", "SD:IMPORT/pack/y.wav", "SD:IMPORT/._a.wav", "SD:IMPORT/a.wav", "SD:IMPORT/keep.wav"); !slices.Equal(got, []bool{false, false, false, true, true}) {
		t.Errorf("files left = %v", got)
	}
	if entries, _ := r.ListDir("SD:IMPORT"); len(entries) != 2 {
		t.Errorf("IMPORT holds %v", entries)
	}
	if _, err := app.CountPaths(r, []string{"SD:nowhere"}); err == nil {
		t.Error("counting a missing path succeeded")
	}
}

func TestCopyPathsWithinTheCardAndAcrossVolumes(t *testing.T) {
	d, s := connect(t)
	r := app.NewRouted(d)
	s.AddFile("SD:IMPORT/kick.wav", []byte("kick"))
	s.AddFile("SD:IMPORT/pack/snare.wav", []byte("snare"))
	s.AddFile("SD:IMPORT/pack/deep/hat.wav", []byte("hat"))
	s.AddFile("SD:IMPORT/taken/kick.wav", []byte("other"))
	s.AddFile("SD:OUT/.keep", []byte(""))

	// A copy leaves the original and makes a whole tree of a folder.
	res := app.CopyPaths(r, []string{"SD:IMPORT/kick.wav", "SD:IMPORT/pack"}, "SD:OUT", false, nil)
	if !slices.Equal(res.Done, []string{"SD:OUT/kick.wav", "SD:OUT/pack"}) || res.Failed != nil {
		t.Fatalf("copy: %+v", res)
	}
	if got := cardFiles(s, "SD:IMPORT/kick.wav", "SD:OUT/kick.wav", "SD:OUT/pack/deep/hat.wav", "SD:IMPORT/pack/deep/hat.wav"); !slices.Equal(got, []bool{true, true, true, true}) {
		t.Errorf("after copy: %v", got)
	}
	if data, _ := s.File("SD:OUT/pack/snare.wav"); string(data) != "snare" {
		t.Errorf("copied content %q", data)
	}

	// A move within the card is a rename: the original is gone.
	res = app.CopyPaths(r, []string{"SD:OUT/kick.wav"}, "SD:IMPORT/pack", true, nil)
	if !slices.Equal(res.Done, []string{"SD:IMPORT/pack/kick.wav"}) || res.Failed != nil {
		t.Fatalf("move: %+v", res)
	}
	if got := cardFiles(s, "SD:OUT/kick.wav", "SD:IMPORT/pack/kick.wav"); !slices.Equal(got, []bool{false, true}) {
		t.Errorf("after move: %v", got)
	}

	// Nothing is overwritten, a folder cannot go into itself, and a file cannot be moved onto itself.
	res = app.CopyPaths(r, []string{"SD:IMPORT/kick.wav", "SD:IMPORT/pack", "SD:IMPORT/pack/snare.wav"}, "SD:IMPORT/taken", false, nil)
	if len(res.Failed) != 1 || !strings.Contains(res.Failed[0], "kick.wav") || !strings.Contains(res.Failed[0], "taken") {
		t.Errorf("a taken name: %+v", res)
	}
	if data, _ := s.File("SD:IMPORT/taken/kick.wav"); string(data) != "other" {
		t.Errorf("the file that was there was overwritten: %q", data)
	}
	res = app.CopyPaths(r, []string{"SD:IMPORT/pack"}, "SD:IMPORT/pack/deep", false, nil)
	if len(res.Failed) != 1 || !strings.Contains(res.Failed[0], "into itself") {
		t.Errorf("a folder into itself: %+v", res)
	}
	res = app.CopyPaths(r, []string{"SD:IMPORT/pack/snare.wav"}, "SD:IMPORT/pack", true, nil)
	if len(res.Failed) != 1 || !strings.Contains(res.Failed[0], "already in that folder") {
		t.Errorf("onto itself: %+v", res)
	}

	// Down to this computer and back up; a cut across volumes deletes the original only
	// after the copy worked.
	dir := t.TempDir()
	res = app.CopyPaths(r, []string{"SD:OUT/pack"}, dir, false, nil)
	if len(res.Done) != 1 || res.Failed != nil {
		t.Fatalf("download: %+v", res)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "pack", "deep", "hat.wav")); err != nil || string(data) != "hat" {
		t.Errorf("downloaded file: %q (%v)", data, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "local.wav"), []byte("local"), 0o644); err != nil {
		t.Fatal(err)
	}
	res = app.CopyPaths(r, []string{filepath.Join(dir, "local.wav")}, "SD:OUT", false, nil)
	if !slices.Equal(res.Done, []string{"SD:OUT/local.wav"}) {
		t.Fatalf("upload: %+v", res)
	}
	if data, _ := s.File("SD:OUT/local.wav"); string(data) != "local" {
		t.Errorf("uploaded content %q", data)
	}
	res = app.CopyPaths(r, []string{"SD:OUT/pack"}, filepath.Join(dir, "elsewhere"), true, nil)
	if len(res.Failed) != 1 {
		t.Errorf("a move into a missing folder: %+v", res)
	}
	os.Mkdir(filepath.Join(dir, "elsewhere"), 0o755)
	res = app.CopyPaths(r, []string{"SD:OUT/local.wav"}, filepath.Join(dir, "elsewhere"), true, nil)
	if len(res.Done) != 1 || cardFiles(s, "SD:OUT/local.wav")[0] {
		t.Errorf("a move to the computer: %+v, original still there: %v", res, cardFiles(s, "SD:OUT/local.wav"))
	}
}

func TestPlayRegionCutsToWhatThePadPlays(t *testing.T) {
	// 10 stereo frames: samples 0..19.
	s := formats.Sample{Channels: 2, Samples: make([]int16, 20)}
	for i := range s.Samples {
		s.Samples[i] = int16(i)
	}
	// Bytes 512+8 to 512+24: frames 2 to 6 (4 bytes a stereo frame... 2 channels of 2 bytes).
	got := app.PlayRegion(s, formats.HeaderLen+2*4, formats.HeaderLen+6*4)
	if got.Frames() != 4 || got.Samples[0] != 4 || got.Samples[len(got.Samples)-1] != 11 {
		t.Errorf("frames %d, samples %v", got.Frames(), got.Samples)
	}
	// The usual case: the whole sample is selected.
	if all := app.PlayRegion(s, formats.HeaderLen, formats.HeaderLen+40); all.Frames() != 10 {
		t.Errorf("whole sample: %d frames", all.Frames())
	}
	// Nonsense plays the whole sample: end before start, past the end, nothing set.
	for _, c := range [][2]uint32{{formats.HeaderLen + 40, formats.HeaderLen + 8}, {0, 99999}, {0, 0}} {
		if got := app.PlayRegion(s, c[0], c[1]); got.Frames() != 10 {
			t.Errorf("region %v played %d frames, want all 10", c, got.Frames())
		}
	}
	mono := formats.Sample{Channels: 1, Samples: []int16{1, 2, 3, 4, 5, 6}}
	if got := app.PlayRegion(mono, formats.HeaderLen+4, formats.HeaderLen+10); !slices.Equal(got.Samples, []int16{3, 4, 5}) {
		t.Errorf("mono region: %v", got.Samples)
	}
}

func TestPadAudioReadsTheSampleFromTheUnit(t *testing.T) {
	d, s := connect(t)
	router := app.NewRouted(d)
	s.AddFile("SD:IMPORT/loop.wav", wav(4800, 5))
	a5 := pad(t, "A5")
	if _, err := app.Assign(d, router, a5, "SD:IMPORT/loop.wav"); err != nil {
		t.Fatal(err)
	}
	snap, _ := app.Load(d)
	got, err := app.PadAudio(router, snap.Pads[a5.Index()])
	if err != nil || got.Frames() != 4800 || got.Channels != 1 {
		t.Fatalf("pad audio: %d frames, %d channels (%v)", got.Frames(), got.Channels, err)
	}
	// It is what was put there.
	want, _ := formats.DecodeAudio(wav(4800, 5), "wav")
	if !slices.Equal(got.Samples, want.Sample.Samples) {
		t.Error("the sample read back differs from the one assigned")
	}
	if _, err := app.PadAudio(router, snap.Pads[0]); err == nil {
		t.Error("reading a pad with no sample succeeded")
	}
}
