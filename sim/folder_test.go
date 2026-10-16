package sim

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/moahbeatlab/404tui/formats"
)

// project makes a project folder named dir whose PADCONF.BIN holds name.
func project(t *testing.T, lib, dir, name string) {
	t.Helper()
	p := filepath.Join(lib, dir)
	if err := os.MkdirAll(filepath.Join(p, "SMPL"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "PADCONF.BIN"), formats.NewPadconf(nil, name).Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestFoldersAreNamedForTheirSlotAndProject(t *testing.T) {
	lib := t.TempDir()
	project(t, lib, "Construction Kits House 01", "Construction Kits House 01")
	project(t, lib, "PROJECT_02", "Construction Kits TH Flavors")
	project(t, lib, "PROJECT_03", "PROJECT_03") // still the default name: the folder stays

	s, folder, err := OpenFolder(lib)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"01 Construction Kits House 01", "02 Construction Kits TH Flavors", "PROJECT_03"} {
		if !exists(filepath.Join(lib, want)) {
			t.Errorf("no folder %q after opening", want)
		}
	}
	if exists(filepath.Join(lib, "PROJECT_02")) {
		t.Error("the folder of the second project kept its old name")
	}
	if len(folder.Projects) != 3 || s.names[1] != "Construction Kits TH Flavors" {
		t.Fatalf("projects %v, second is %q", folder.Projects, s.names[1])
	}

	// Opened again, the slots are where they were, by the numbers in front.
	_, again, err := OpenFolder(lib)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(again.Projects[1]) != "02 Construction Kits TH Flavors" {
		t.Errorf("the second slot is %q", again.Projects[1])
	}

	// A project renamed while working takes its folder along, and keeps its slot.
	s.project = 1
	s.names[1] = "Flavors: Deep / Dark"
	s.flush()
	if !exists(filepath.Join(lib, "02 Flavors_ Deep _ Dark")) || exists(filepath.Join(lib, "02 Construction Kits TH Flavors")) {
		entries, _ := os.ReadDir(lib)
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("after the rename the folders are %v", names)
	}
}

func TestANumberedFolderKeepsItsSlotWhenOthersAreMissing(t *testing.T) {
	lib := t.TempDir()
	project(t, lib, "05 Only one", "Only one")
	s, folder, err := OpenFolder(lib)
	if err != nil {
		t.Fatal(err)
	}
	if s.names[4] != "Only one" || len(folder.Projects) != 1 || filepath.Base(folder.Projects[0]) != "05 Only one" {
		t.Errorf("names[4] = %q, projects %v", s.names[4], folder.Projects)
	}
}
