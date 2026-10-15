package tui

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// realSample is a WAV from disk: SLICER_SAMPLE, or the .wav files named in SLICER_SAMPLES
// (a folder). Without one the tests skip, so the suite needs no audio files.
func realSamples(t *testing.T) []string {
	t.Helper()
	var paths []string
	if p := os.Getenv("SLICER_SAMPLE"); p != "" {
		paths = append(paths, p)
	}
	if dir := os.Getenv("SLICER_SAMPLES"); dir != "" {
		more, _ := filepath.Glob(filepath.Join(dir, "*.wav"))
		paths = append(paths, more...)
	}
	if len(paths) == 0 {
		t.Skip("set SLICER_SAMPLE=file.wav (or SLICER_SAMPLES=folder) to run on real audio")
	}
	return paths
}

// slicerOpenOn opens the slicer on pad A5 holding the given WAV file.
func slicerOpenOn(t *testing.T, path string, w, h int) *screen {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: w, Height: h})
	s.sim.AddFile("SD:IMPORT/real.wav", data)
	m := s.m()
	if _, err := app.Assign(m.unit, m.unit, pad(t, "A5"), "SD:IMPORT/real.wav"); err != nil {
		t.Fatal(err)
	}
	// A pattern on A2 that plays the sample, for P to put under the slices.
	if ptn, err := formats.StartPattern([]proto.PadIndex{pad(t, "A5")}, 2); err == nil {
		s.sim.SetPattern(pad(t, "A2"), pad(t, "A5"))
		s.sim.AddFile(formats.PatternPath(1, pad(t, "A2")), ptn)
	}
	s.press("r", "j", "Z")
	if s.m().slicer == nil {
		t.Fatalf("%s: the slicer did not open:\n%s", path, s.model.View())
	}
	return s
}

// checkSlicer is what must hold after any key: the box fits the screen, the slices lie in the
// sample, each is at least a frame long, and the cursor is on one of them.
func (s *screen) checkSlicer(context string) {
	s.t.Helper()
	m := s.m()
	if m.slicer == nil {
		return
	}
	sl := m.slicer.sl
	if len(sl.Slices) == 0 {
		s.t.Fatalf("%s: no slices left", context)
	}
	if m.slicer.at < 0 || m.slicer.at >= len(sl.Slices) {
		s.t.Fatalf("%s: the cursor %d is outside %d slices", context, m.slicer.at, len(sl.Slices))
	}
	for i, x := range sl.Slices {
		if x.Start < 0 || x.End > sl.Frames || x.End <= x.Start {
			s.t.Fatalf("%s: slice %d is %d..%d of %d frames", context, i+1, x.Start, x.End, sl.Frames)
		}
		if x.Chance < 0 || x.Chance > 100 || x.Gain < 0 {
			s.t.Fatalf("%s: slice %d has chance %d gain %d", context, i+1, x.Chance, x.Gain)
		}
	}
	box := strings.Split(m.slicerBox(), "\n")
	if len(box) > m.geom().bodyH {
		s.t.Fatalf("%s: the box is %d lines, the screen has room for %d", context, len(box), m.geom().bodyH)
	}
	for i, line := range box {
		if w := ansi.StringWidth(line); w > m.width {
			s.t.Fatalf("%s: box line %d is %d wide on a %d wide screen", context, i, w, m.width)
		}
	}
}

// TestSlicerE2EWorksOnARealSample takes real audio through the things a person does: cut it
// up every way, edit the slices, play, bounce to a pad, and read the pad back.
func TestSlicerE2EWorksOnARealSample(t *testing.T) {
	for _, path := range realSamples(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			s := slicerOpenOn(t, path, 140, 50)
			s.checkSlicer("open")
			frames := s.slicing().Frames
			if frames < 1000 {
				t.Fatalf("only %d frames read from %s", frames, path)
			}

			// Every way of cutting.
			for _, step := range []struct {
				name string
				keys []string
			}{
				{"onsets", []string{"o"}},
				{"16 equal", []string{"e"}},
				{"random cut", []string{"R"}},
				{"grid on, steps", []string{"T", ",", "."}},
				{"swing and humanize", []string{"<", ">", "<", "-", "+"}},
				{"cut and delete", []string{"l", "l", "c", "d", "u"}},
				{"settings", []string{"r", "(", ")", "x", "m", "X"}},
				{"nudge", []string{"h", "[", "]", "{", "}", "f"}},
				{"order lane", []string{"tab", "a", "l", "l", "d", "u", "tab"}},
				{"modes", []string{"M", "M", "M", "S", "S"}},
				{"zoom", []string{"z", "z", "z", "0", "I"}},
				{"pattern", []string{"P", "enter"}},
			} {
				s.press(step.keys...)
				s.checkSlicer(step.name)
			}

			// Play for a while: the tick keeps the head moving through every slice.
			s.press(" ")
			for i := 0; i < 400; i++ {
				s.send(slicerTickMsg{})
				if i%50 == 0 {
					s.checkSlicer(fmt.Sprintf("playing tick %d", i))
				}
			}
			s.press(" ")

			// Leave and come back: the cuts are remembered.
			n := len(s.slicing().Slices)
			s.press("esc", "Z")
			if s.m().slicer == nil {
				t.Fatalf("the slicer did not open again:\n%s", s.model.View())
			}
			if len(s.slicing().Slices) != n {
				t.Errorf("slices not remembered: %d, was %d", len(s.slicing().Slices), n)
			}

			// Bounce what is on screen to A9 and read it back from the unit.
			want := s.m().slicer.takeSample(0)
			if want.Frames() == 0 {
				t.Fatal("the bounce is empty")
			}
			loud := false
			for _, v := range want.Samples {
				if v > 500 || v < -500 {
					loud = true
					break
				}
			}
			if !loud {
				t.Error("the bounce of a real sample is silent")
			}
			s.press("b", "A", "9", "enter")
			s.shows("bounced")
			row := s.m().snap.Bank(0)[8]
			if !row.HasSample {
				t.Fatal("A9 has no sample after the bounce")
			}
			got, err := app.PadAudio(s.m().unit, row)
			if err != nil {
				t.Fatal(err)
			}
			if got.Frames() != want.Frames() {
				t.Errorf("A9 holds %d frames, bounced %d", got.Frames(), want.Frames())
			}

		})
	}
}

// TestSlicerE2EFuzz presses random slicer keys on a real sample, at several screen sizes, and
// checks the invariants after every key. A failure names the seed and the keys that led to it.
func TestSlicerE2EFuzz(t *testing.T) {
	keys := strings.Fields(`h l g G [ ] H L { } f c d e t r x m ( ) < > - + = X o R I 0 i z T , . n b
		u M S w W v A p E D C N 1 2 3 4 s y B P P tab shift+tab space enter esc j k`)
	sizes := [][2]int{{80, 30}, {100, 40}, {140, 50}, {220, 60}}
	for _, path := range realSamples(t) {
		for seed := int64(1); seed <= 6; seed++ {
			size := sizes[int(seed)%len(sizes)]
			name := fmt.Sprintf("%s/seed%d/%dx%d", filepath.Base(path), seed, size[0], size[1])
			t.Run(name, func(t *testing.T) {
				s := slicerOpenOn(t, path, size[0], size[1])
				rng := rand.New(rand.NewSource(seed))
				var trail []string
				defer func() {
					if r := recover(); r != nil {
						tail := trail[max(0, len(trail)-25):]
						t.Fatalf("panic %v after keys: %v\n%s", r, tail, debug.Stack())
					}
				}()
				for i := 0; i < 600; i++ {
					k := keys[rng.Intn(len(keys))]
					trail = append(trail, k)
					if s.m().slicer == nil {
						s.press("Z") // esc or q closed it: open it again
						if s.m().slicer == nil {
							s.press("2", "r", "j", "Z")
						}
						if s.m().slicer == nil {
							continue
						}
					}
					s.press(k)
					if s.m().slicer != nil && s.m().slicer.sl != nil {
						tail := trail[max(0, len(trail)-12):]
						s.checkSlicer(fmt.Sprintf("after %v", tail))
					}
					if i%25 == 0 {
						s.send(slicerTickMsg{})
					}
				}
			})
		}
	}
}
