package doctor_test

import (
	"bytes"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/moahbeatlab/404tui/device"
	"github.com/moahbeatlab/404tui/doctor"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
	"github.com/moahbeatlab/404tui/sim"
)

func run(t *testing.T, s *sim.Sim, opt doctor.Options) ([]doctor.Result, string) {
	t.Helper()
	d := device.New(s, "sim")
	t.Cleanup(func() { d.Close() })
	var out bytes.Buffer
	results := doctor.Run(d, &out, slog.New(slog.NewTextHandler(io.Discard, nil)), opt)
	return results, out.String()
}

func byName(results []doctor.Result, part string) doctor.Result {
	for _, r := range results {
		if strings.Contains(r.Name, part) {
			return r
		}
	}
	return doctor.Result{Name: "missing: " + part, Err: io.EOF}
}

func pad(t *testing.T, name string) proto.PadIndex {
	p, err := proto.ParsePad(name)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDemoUnitPassesEveryCheck(t *testing.T) {
	results, out := run(t, sim.Demo(), doctor.Options{Preview: true})
	if n := doctor.Failures(results); n != 0 {
		t.Fatalf("%d checks failed:\n%s", n, out)
	}
	for _, want := range []string{"status poll", "project settings", "pad blocks", "sample files match", "file API: the unit's storage",
		"file API: the SD card", "PADCONF.BIN", "patterns", "unsolicited", "preview"} {
		if r := byName(results, want); r.Skipped {
			t.Errorf("%q was skipped: %s", want, r.Detail)
		}
	}
	for _, want := range []string{"[ ok ] status poll", "project 1", "4 pads hold a sample", "4 sample files", "magic RFPD, version 3", "1 patterns in the project", "All checks passed.", "Read-only"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestEmptyProjectSkipsWhatIsNotThere(t *testing.T) {
	// A real unit always has its project folders, even for an empty project.
	s := sim.New()
	s.AddFile("ROLAND/SP-404MKII/PROJECT_01/PICTURE/startup_1.bmp", []byte("bmp"))
	results, out := run(t, s, doctor.Options{})
	if n := doctor.Failures(results); n != 0 {
		t.Fatalf("%d checks failed:\n%s", n, out)
	}
	if !byName(results, "PADCONF.BIN").Skipped || !byName(results, "patterns").Skipped {
		t.Errorf("an empty project should skip its project file and patterns:\n%s", out)
	}
}

func TestDoctorCatchesDisagreements(t *testing.T) {
	// A sample file that is not a valid SMP, though the pad block says it holds one.
	s := sim.Demo()
	s.SetSample(pad(t, "D4"), "broken", 5000)
	results, out := run(t, s, doctor.Options{})
	if r := byName(results, "sample files match"); r.OK() || !strings.Contains(r.Err.Error(), "D4") {
		t.Errorf("a broken sample passed: %v\n%s", r.Err, out)
	}

	// A pad block whose size is not the file's.
	s = sim.Demo()
	smp := sim.Tone(220, 0.2).SMP()
	s.SetSMP(pad(t, "D5"), "short", smp)
	s.AddFile(pad(t, "D5").SamplePath(1), append(smp, 1, 2, 3, 4))
	if r := byName(mustRun(t, s), "sample files match"); r.OK() || !strings.Contains(r.Err.Error(), "D5") {
		t.Errorf("a size mismatch passed: %v", r.Err)
	}

	// A pattern whose file plays other pads than the unit says it does.
	s = sim.Demo()
	s.AddFile("ROLAND/SP-404MKII/PROJECT_01/PTN/PTN00018.BIN", sim.NotePattern(pad(t, "A1"), pad(t, "C9")))
	r := byName(mustRun(t, s), "patterns")
	if r.OK() || !strings.Contains(r.Err.Error(), "B2") || !strings.Contains(r.Err.Error(), "our parser reads") {
		t.Errorf("a pattern mismatch passed: %v", r.Err)
	}

	// A project file that names another project.
	s = sim.Demo()
	s.AddFile("ROLAND/SP-404MKII/PROJECT_01/PADCONF.BIN", sim.Padconf("SOMEONE ELSE"))
	if r := byName(mustRun(t, s), "PADCONF.BIN"); r.OK() || !strings.Contains(r.Err.Error(), "SOMEONE ELSE") {
		t.Errorf("a name mismatch passed: %v", r.Err)
	}
	s = sim.Demo()
	s.AddFile("ROLAND/SP-404MKII/PROJECT_01/PADCONF.BIN", []byte("not a project file at all, really not......"))
	if r := byName(mustRun(t, s), "PADCONF.BIN"); r.OK() || !strings.Contains(r.Err.Error(), "RFPD") {
		t.Errorf("a bad magic passed: %v", r.Err)
	}
}

func mustRun(t *testing.T, s *sim.Sim) []doctor.Result {
	results, _ := run(t, s, doctor.Options{})
	return results
}

func TestDoctorReportsAUnitThatDoesNotAnswer(t *testing.T) {
	s := sim.Demo()
	d := device.New(s, "sim")
	s.Close()
	var out bytes.Buffer
	results := doctor.Run(d, &out, slog.New(slog.NewTextHandler(io.Discard, nil)), doctor.Options{})
	d.Close()
	if r := byName(results, "status poll"); r.OK() {
		t.Errorf("a closed link passed the status poll:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "check(s) failed") || !strings.Contains(out.String(), "-trace") {
		t.Errorf("the summary does not say what to do:\n%s", out.String())
	}
	_ = formats.HeaderLen
}
