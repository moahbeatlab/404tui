package app_test

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/device"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
	"github.com/moahbeatlab/404tui/sim"
)

// homeProject writes a project folder as an export from the unit would hold it: a
// PADCONF.BIN naming it, with a sample on A1.
func homeProject(t *testing.T, dir, name string) {
	t.Helper()
	smp := sim.Tone(220, 0.3).SMP()
	conf := formats.NewPadconf(nil, name)
	b := conf.Blocks[0]
	binary.LittleEndian.PutUint32(b[3:], uint32(len(smp)))
	binary.LittleEndian.PutUint32(b[7:], uint32(proto.SMPHeaderLen))
	binary.LittleEndian.PutUint32(b[11:], uint32(len(smp)))
	copy(b[175:], "home kick")
	for path, data := range map[string][]byte{
		"PADCONF.BIN":       conf.Bytes(),
		"SMPL/BANK1-01.SMP": smp,
	} {
		local := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(local, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// openOffline opens the projects in dir as a unit.
func openOffline(t *testing.T, dir string) (*device.Device, sim.Folder) {
	t.Helper()
	s, f, err := sim.OpenFolder(dir)
	if err != nil {
		t.Fatal(err)
	}
	return device.New(s, "offline"), f
}

func TestWorkingOfflineOnFoldersOfProjects(t *testing.T) {
	lib := t.TempDir()
	homeProject(t, filepath.Join(lib, "MyBeat"), "MY BEAT")
	wav := filepath.Join(lib, "SD", "IMPORT", "snare.wav")
	if err := os.MkdirAll(filepath.Dir(wav), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wav, sim.Tone(440, 0.2).WAV(), 0o644); err != nil {
		t.Fatal(err)
	}

	d, f := openOffline(t, lib)
	if len(f.Projects) != 1 || !f.Template {
		t.Fatalf("found %v, template %v", f.Projects, f.Template)
	}
	snap, err := app.Load(d)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Projects[0] != "MY BEAT" || !snap.Pads[0].HasSample || snap.Pads[0].Name != "home kick" {
		t.Fatalf("the project was not read: %q, A1 %+v", snap.Projects[0], snap.Pads[0])
	}

	// A sample from the SD folder goes onto A2, and a bank tempo changes: both land on disk.
	if _, err := app.Assign(d, d, pad(t, "A2"), "SD:IMPORT/snare.wav"); err != nil {
		t.Fatal(err)
	}
	if err := app.SetBankTempo(d, 2, 98.5); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(lib, "MyBeat", "SMPL", "BANK1-02.SMP")); err != nil {
		t.Errorf("the sample was not written to the project folder: %v", err)
	}

	// A project in an empty slot becomes a folder of its own.
	if err := d.SelectProject(1); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Assign(d, d, pad(t, "B1"), "SD:IMPORT/snare.wav"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(lib, "PROJECT_02", "PADCONF.BIN")); err != nil {
		t.Errorf("the new project has no PADCONF.BIN: %v", err)
	}
	// It can be exported like any project, and so restored onto the unit later.
	if _, err := app.ExportProject(d, 1, filepath.Join(t.TempDir(), "out"), nil); err != nil {
		t.Errorf("exporting the new project: %v", err)
	}
	d.Close()

	// Opened again, everything is still there.
	d, f = openOffline(t, filepath.Join(lib, "MyBeat"))
	defer d.Close()
	if len(f.Projects) != 2 {
		t.Fatalf("projects after reopening: %v", f.Projects)
	}
	snap, err = app.Load(d)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Project != 0 || !snap.Pads[1].HasSample || snap.Banks[2].Tempo != 98.5 {
		t.Errorf("project 1 after reopening: project %d, A2 %+v, bank C %v BPM", snap.Project, snap.Pads[1], snap.Banks[2].Tempo)
	}
	if err := d.SelectProject(1); err != nil {
		t.Fatal(err)
	}
	snap, _ = app.Load(d)
	if !snap.Pads[16].HasSample {
		t.Errorf("project 2 lost B1: %+v", snap.Pads[16])
	}
}
