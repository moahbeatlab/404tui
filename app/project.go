package app

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/moahbeatlab/404tui/device"
	"github.com/moahbeatlab/404tui/proto"
)

// Project export and restore: a project is a folder of files in the unit's own storage
// (PADCONF.BIN, PICTURE, PTN, SMPL). Exporting copies that folder to the SD card or this
// computer; restoring writes such a folder back over a project, as the official app's
// "Import to MKII" does. The backup folder is the one holding PADCONF.BIN.

// Restorer writes a project back to the unit.
type Restorer interface {
	CurrentProject() (uint8, error)
	SelectProject(project uint8) error
	FreeKb() (uint32, error)
	RestoreProject(project uint8, files []device.File, progress func(i int, name string)) error
}

func projectFolder(project uint8) string {
	return fmt.Sprintf("ROLAND/SP-404MKII/PROJECT_%02d", project+1)
}

// ExportProject copies a project (0-based) into dir, which is made if it is not there, and
// returns a one-line summary. dir can be on the SD card or this computer.
func ExportProject(s Storage, project uint8, dir string, progress func(done int, name string)) (string, error) {
	root := projectFolder(project)
	dir = strings.TrimRight(dir, "/")
	if _, exists, err := s.Stat(root + "/PADCONF.BIN"); err != nil {
		return "", err
	} else if !exists {
		return "", fmt.Errorf("project %d is empty: nothing to export", project+1)
	}
	if err := MkdirAll(s, dir); err != nil {
		return "", fmt.Errorf("creating %s: %w", dir, err)
	}
	files, bytes := 0, 0
	var walk func(remote, local string) error
	walk = func(remote, local string) error {
		entries, err := s.ListDir(remote)
		if err != nil {
			return fmt.Errorf("listing %s: %w", remote, err)
		}
		for _, e := range entries {
			from, to := remote+"/"+e.Name, local+"/"+e.Name
			if e.IsDir() {
				if err := MkdirAll(s, to); err != nil {
					return fmt.Errorf("creating %s: %w", to, err)
				}
				if err := walk(from, to); err != nil {
					return err
				}
				continue
			}
			data, err := s.ReadFile(from)
			if err != nil {
				return fmt.Errorf("reading %s: %w", from, err)
			}
			if err := s.WriteFile(to, data); err != nil {
				return fmt.Errorf("writing %s: %w", to, err)
			}
			files++
			bytes += len(data)
			if progress != nil {
				progress(files, e.Name)
			}
		}
		return nil
	}
	if err := walk(root, dir); err != nil {
		return "", err
	}
	return fmt.Sprintf("project %d exported: %d files, %.1f MB, to %s", project+1, files, float64(bytes)/1e6, dir), nil
}

// RestorePlan is a backup read and checked, ready to be written over a project.
type RestorePlan struct {
	// Project is the project (0-based) it will overwrite.
	Project uint8
	Dir     string
	Files   []device.File
	Total   int64
	// Name is the project name inside the backup, empty when it has none.
	Name      string
	FreeBytes int64
}

// PlanProjectRestore reads the backup in dir and checks it fits. It changes nothing on the
// unit, so the caller can show what is about to be overwritten and ask first.
func PlanProjectRestore(s interface {
	Storage
	FreeKb() (uint32, error)
}, project uint8, dir string) (*RestorePlan, error) {
	dir = strings.TrimRight(dir, "/")
	padconf, err := s.ReadFile(dir + "/PADCONF.BIN")
	if err != nil {
		return nil, fmt.Errorf("%s has no PADCONF.BIN: choose the backup folder itself (%w)", dir, err)
	}
	plan := &RestorePlan{Project: project, Dir: dir, Name: backupName(padconf)}
	plan.Files = append(plan.Files, device.File{Name: "PADCONF.BIN", Data: padconf})
	// The official app writes PADCONF first, then pictures, the pattern chain, patterns and
	// samples.
	for _, sub := range []string{"PICTURE", "PTN", "SMPL"} {
		entries, err := s.ListDir(dir + "/" + sub)
		if err != nil {
			continue
		}
		var names []string
		for _, e := range entries {
			if !e.IsDir() {
				names = append(names, e.Name)
			}
		}
		sort.SliceStable(names, func(i, j int) bool {
			ci, cj := strings.HasSuffix(names[i], ".CHN"), strings.HasSuffix(names[j], ".CHN")
			if ci != cj {
				return ci
			}
			return names[i] < names[j]
		})
		for _, name := range names {
			data, err := s.ReadFile(dir + "/" + sub + "/" + name)
			if err != nil {
				return nil, fmt.Errorf("reading %s/%s: %w", sub, name, err)
			}
			plan.Files = append(plan.Files, device.File{Name: sub + "/" + name, Data: data})
		}
	}
	for _, f := range plan.Files {
		plan.Total += int64(len(f.Data))
	}
	free, err := s.FreeKb()
	if err != nil {
		return nil, err
	}
	plan.FreeBytes = int64(free) * 1024
	if plan.Total > plan.FreeBytes {
		return nil, fmt.Errorf("not enough space on the unit: the backup needs %.1f MB, %.1f MB are free",
			float64(plan.Total)/1e6, float64(plan.FreeBytes)/1e6)
	}
	return plan, nil
}

// backupName is the project name stored in a PADCONF.BIN, or "" for one from older
// firmware, which has none. The settings sit at 0x10 and the name at 0x80.
func backupName(padconf []byte) string {
	if len(padconf) < 0xA0 || padconf[8] < 3 {
		return ""
	}
	payload := []byte{proto.ReplyProjectSettings, 0, 0}
	payload = append(payload, padconf[0x10:0xA0]...)
	settings, err := proto.ParseProjectSettings(payload)
	if err != nil {
		return ""
	}
	return settings.Name()
}

// RestoreProject writes a planned backup over its project. The first thing the unit does is
// erase the current project, so the target is selected first.
func RestoreProject(u Restorer, plan *RestorePlan, progress func(i int, name string)) error {
	if plan == nil || len(plan.Files) == 0 {
		return errors.New("nothing to restore")
	}
	current, err := u.CurrentProject()
	if err != nil {
		return err
	}
	if current != plan.Project {
		if err := u.SelectProject(plan.Project); err != nil {
			return fmt.Errorf("selecting project %d: %w", plan.Project+1, err)
		}
	}
	return u.RestoreProject(plan.Project, plan.Files, progress)
}
