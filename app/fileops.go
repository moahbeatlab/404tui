package app

import (
	"fmt"
	"strings"

	"github.com/moahbeatlab/404tui/proto/fileapi"
)

// File operations on the unit's card and this computer: counting, deleting, copying and moving
// files and folders. Each works on whole trees and never overwrites: a name that is already
// taken is reported and left alone.

// Volume is where a path lives; a rename only works inside one.
type Volume int

const (
	// VolumeUnit is the unit's own storage, where projects live.
	VolumeUnit Volume = iota
	// VolumeCard is the SD card.
	VolumeCard
	// VolumeComputer is this computer's filesystem.
	VolumeComputer
)

// VolumeOf says where a path is.
func VolumeOf(path string) Volume {
	switch {
	case IsLocal(path):
		return VolumeComputer
	case fileapi.OnCard(path):
		return VolumeCard
	}
	return VolumeUnit
}

func (v Volume) String() string {
	switch v {
	case VolumeCard:
		return "the SD card"
	case VolumeComputer:
		return "this computer"
	}
	return "the unit's storage"
}

// IsJunk reports whether a name is something an operating system leaves behind and nobody
// wants on a card: macOS's "._" companions and .DS_Store, Windows' Thumbs.db and desktop.ini.
func IsJunk(name string) bool {
	switch name {
	case ".DS_Store", "Thumbs.db", "desktop.ini", "__MACOSX", ".Spotlight-V100", ".Trashes", ".fseventsd":
		return true
	}
	return strings.HasPrefix(name, "._")
}

// Count is how much a set of paths holds.
type Count struct {
	Files, Folders int
}

// CountPaths counts the files and folders in paths, folders' contents included. The folders
// themselves are counted too, so a single file is {1, 0} and an empty folder {0, 1}.
func CountPaths(s Storage, paths []string) (Count, error) {
	var c Count
	for _, p := range paths {
		if err := countTree(s, p, &c); err != nil {
			return c, err
		}
	}
	return c, nil
}

func countTree(s Storage, path string, c *Count) error {
	st, exists, err := s.Stat(path)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%s is not there", Base(path))
	}
	if !st.IsDir() {
		c.Files++
		return nil
	}
	c.Folders++
	entries, err := s.ListDir(path)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := countTree(s, Join(path, e.Name), c); err != nil {
			return err
		}
	}
	return nil
}

// DeletePaths deletes files and, with everything in them, folders. It goes on past one that
// fails, and returns how many of the paths went and a line for each that did not.
func DeletePaths(s Storage, paths []string, progress func(done int, path string)) (deleted int, failed []string) {
	for i, p := range paths {
		if progress != nil {
			progress(i, p)
		}
		if err := deleteTree(s, p); err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", Base(p), err))
			continue
		}
		deleted++
	}
	return deleted, failed
}

func deleteTree(s Storage, path string) error {
	st, exists, err := s.Stat(path)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("not there")
	}
	if !st.IsDir() {
		return s.Unlink(path)
	}
	entries, err := s.ListDir(path)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := deleteTree(s, Join(path, e.Name)); err != nil {
			return err
		}
	}
	return s.Rmdir(path)
}

// Moved is the outcome of copying or moving paths into a folder.
type Moved struct {
	// Done are the new paths.
	Done []string
	// Failed has a line for each path that was not copied or moved, and why.
	Failed []string
}

// CopyPaths copies files and folders into destDir, or moves them (cut). A move inside one
// volume is a rename; across volumes it is a copy and then, only if the copy worked, a delete
// of the original. A name already taken in destDir is never overwritten, and a folder cannot
// go into itself.
func CopyPaths(s Storage, sources []string, destDir string, cut bool, progress func(done int, path string)) Moved {
	var out Moved
	for i, src := range sources {
		if progress != nil {
			progress(i, src)
		}
		dest := Join(destDir, Base(src))
		fail := func(why string) { out.Failed = append(out.Failed, fmt.Sprintf("%s (%s)", Base(src), why)) }
		switch {
		case src == dest:
			fail("it is already in that folder")
			continue
		case dest == src+"/" || strings.HasPrefix(dest, src+"/"):
			fail("a folder cannot go into itself")
			continue
		}
		if _, taken, err := s.Stat(dest); err != nil {
			fail(err.Error())
			continue
		} else if taken {
			fail("that name is taken in " + display(destDir))
			continue
		}
		var err error
		if cut && VolumeOf(src) == VolumeOf(dest) {
			err = s.Rename(src, dest)
		} else if err = copyTree(s, src, dest); err == nil && cut {
			if derr := deleteTree(s, src); derr != nil {
				err = fmt.Errorf("copied, but the original could not be deleted: %w", derr)
			}
		}
		if err != nil {
			fail(err.Error())
			continue
		}
		out.Done = append(out.Done, dest)
	}
	return out
}

func display(dir string) string {
	if dir == StorageRoot {
		return "the unit's storage"
	}
	return dir
}

func copyTree(s Storage, src, dest string) error {
	st, exists, err := s.Stat(src)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("not there")
	}
	if !st.IsDir() {
		data, err := s.ReadFile(src)
		if err != nil {
			return fmt.Errorf("reading it: %w", err)
		}
		return s.WriteFile(dest, data)
	}
	if err := s.Mkdir(dest); err != nil {
		return err
	}
	entries, err := s.ListDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := copyTree(s, Join(src, e.Name), Join(dest, e.Name)); err != nil {
			return err
		}
	}
	return nil
}
