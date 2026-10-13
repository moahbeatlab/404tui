package app

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/moahbeatlab/404tui/proto/fileapi"
)

// Storage is a place files live: the unit's own storage, its SD card, or this computer.
// Paths address the unit's storage bare ("ROLAND/SP-404MKII") and its SD card with an
// "SD:" prefix; an absolute path is on the computer.
type Storage interface {
	ListDir(path string) ([]fileapi.DirEntry, error)
	Rename(from, to string) error
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte) error
	Stat(path string) (st fileapi.FileStat, exists bool, err error)
	Mkdir(path string) error
	// Unlink deletes a file and Rmdir an empty folder.
	Unlink(path string) error
	Rmdir(path string) error
	ReadPrefix(path string, n uint32) (data []byte, exists bool, err error)
}

// Router sends each path to the unit or to this computer, by what it looks like.
type Router struct {
	Unit  Storage
	Local Storage
}

// NewRouter returns a Storage that reaches both the unit and the computer.
func NewRouter(unit Storage) Router {
	return Router{Unit: unit, Local: Local{}}
}

// IsLocal reports whether path is on this computer: absolute, or starting with "." or "~".
func IsLocal(path string) bool {
	return filepath.IsAbs(path) || strings.HasPrefix(path, ".") || strings.HasPrefix(path, "~")
}

func (r Router) pick(path string) Storage {
	if IsLocal(path) {
		return r.Local
	}
	return r.Unit
}

func (r Router) ListDir(p string) ([]fileapi.DirEntry, error)  { return r.pick(p).ListDir(p) }
func (r Router) Rename(from, to string) error                  { return r.pick(from).Rename(from, to) }
func (r Router) ReadFile(p string) ([]byte, error)             { return r.pick(p).ReadFile(p) }
func (r Router) WriteFile(p string, d []byte) error            { return r.pick(p).WriteFile(p, d) }
func (r Router) Mkdir(p string) error                          { return r.pick(p).Mkdir(p) }
func (r Router) Unlink(p string) error                         { return r.pick(p).Unlink(p) }
func (r Router) Rmdir(p string) error                          { return r.pick(p).Rmdir(p) }
func (r Router) Stat(p string) (fileapi.FileStat, bool, error) { return r.pick(p).Stat(p) }
func (r Router) ReadPrefix(p string, n uint32) ([]byte, bool, error) {
	return r.pick(p).ReadPrefix(p, n)
}

// Local is this computer's filesystem as a Storage.
type Local struct{}

func expand(path string) string {
	if rest, ok := strings.CutPrefix(path, "~"); ok {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, rest)
		}
	}
	return path
}

// ListDir lists a folder, leaving out hidden entries.
func (Local) ListDir(path string) ([]fileapi.DirEntry, error) {
	entries, err := os.ReadDir(expand(path))
	if err != nil {
		return nil, err
	}
	out := make([]fileapi.DirEntry, 0, len(entries))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		kind := fileapi.DtReg
		// A link to a folder is a folder.
		if info, err := os.Stat(filepath.Join(expand(path), e.Name())); err == nil && info.IsDir() {
			kind = fileapi.DtDir
		}
		out = append(out, fileapi.DirEntry{Name: e.Name(), Kind: kind})
	}
	sort.Slice(out, func(i, j int) bool { return NaturalLess(out[i].Name, out[j].Name) })
	return out, nil
}

func (Local) Rename(from, to string) error {
	if _, err := os.Lstat(expand(to)); err == nil {
		return os.ErrExist
	}
	return os.Rename(expand(from), expand(to))
}

func (Local) ReadFile(path string) ([]byte, error) { return os.ReadFile(expand(path)) }

func (Local) WriteFile(path string, data []byte) error {
	return os.WriteFile(expand(path), data, 0o644)
}

func (Local) Mkdir(path string) error { return os.Mkdir(expand(path), 0o755) }

func (Local) Unlink(path string) error { return os.Remove(expand(path)) }

// Rmdir removes an empty folder; os.Remove refuses a full one.
func (Local) Rmdir(path string) error { return os.Remove(expand(path)) }

func (Local) Stat(path string) (fileapi.FileStat, bool, error) {
	info, err := os.Stat(expand(path))
	if os.IsNotExist(err) {
		return fileapi.FileStat{}, false, nil
	}
	if err != nil {
		return fileapi.FileStat{}, false, err
	}
	mode := fileapi.ModeFile
	if info.IsDir() {
		mode = fileapi.ModeDir
	}
	return fileapi.FileStat{Mode: mode, Size: uint32(min(info.Size(), 1<<32-1))}, true, nil
}

func (Local) ReadPrefix(path string, n uint32) ([]byte, bool, error) {
	f, err := os.Open(expand(path))
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	buf := make([]byte, n)
	got, _ := f.Read(buf)
	return buf[:got], true, nil
}

// MkdirAll creates a folder and the ones above it that are missing: the device's mkdir
// fails unless the parent exists, so each level is made in turn.
func MkdirAll(s Storage, dir string) error {
	for _, path := range segments(dir) {
		if _, exists, err := s.Stat(path); err != nil {
			return err
		} else if exists {
			continue
		}
		if err := s.Mkdir(path); err != nil {
			return err
		}
	}
	return nil
}

// segments lists every ancestor of a path, from its first element down to the path itself,
// with any "SD:" prefix or leading slash kept.
func segments(dir string) []string {
	prefix, rest := "", dir
	if after, ok := strings.CutPrefix(dir, fileapi.Card); ok {
		prefix, rest = fileapi.Card, after
	} else if strings.HasPrefix(dir, "/") {
		prefix = "/"
	}
	var out []string
	walked := ""
	for _, part := range strings.Split(rest, "/") {
		if part == "" {
			continue
		}
		if walked == "" {
			walked = part
		} else {
			walked += "/" + part
		}
		out = append(out, prefix+walked)
	}
	return out
}

// Routed is a Unit whose file operations reach the computer as well as the unit, by the
// shape of the path. Everything else is the unit's own.
type Routed struct {
	Unit
	files Router
}

// NewRouted wraps a unit so paths on this computer work wherever a path is taken.
func NewRouted(u Unit) *Routed {
	return &Routed{Unit: u, files: NewRouter(u)}
}

func (r *Routed) ListDir(p string) ([]fileapi.DirEntry, error)  { return r.files.ListDir(p) }
func (r *Routed) Rename(from, to string) error                  { return r.files.Rename(from, to) }
func (r *Routed) ReadFile(p string) ([]byte, error)             { return r.files.ReadFile(p) }
func (r *Routed) WriteFile(p string, d []byte) error            { return r.files.WriteFile(p, d) }
func (r *Routed) Mkdir(p string) error                          { return r.files.Mkdir(p) }
func (r *Routed) Unlink(p string) error                         { return r.files.Unlink(p) }
func (r *Routed) Rmdir(p string) error                          { return r.files.Rmdir(p) }
func (r *Routed) Stat(p string) (fileapi.FileStat, bool, error) { return r.files.Stat(p) }
func (r *Routed) ReadPrefix(p string, n uint32) ([]byte, bool, error) {
	return r.files.ReadPrefix(p, n)
}
