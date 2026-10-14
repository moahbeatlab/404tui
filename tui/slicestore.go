package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/moahbeatlab/404tui/formats"
)

// sliceStore keeps what the slicer was set to for each sample, by a key that names the sample,
// so that opening it again finds the slices as they were left. Saving is best effort.
type sliceStore interface {
	load(key string) (*formats.Slicing, bool)
	save(key string, s *formats.Slicing)
}

// newSliceStore keeps the slicings in slices.json next to the state file; with a store that
// is not a file (as in tests) they are kept in memory for as long as the program runs.
func newSliceStore(store Store) sliceStore {
	if fs, ok := store.(FileStore); ok && fs.Path != "" {
		return &fileSlices{path: filepath.Join(filepath.Dir(fs.Path), "slices.json")}
	}
	return &memSlices{m: map[string]*formats.Slicing{}}
}

type memSlices struct {
	mu sync.Mutex
	m  map[string]*formats.Slicing
}

func (s *memSlices) load(key string) (*formats.Slicing, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[key]
	if !ok {
		return nil, false
	}
	return v.Clone(), true
}

func (s *memSlices) save(key string, v *formats.Slicing) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = v.Clone()
}

type fileSlices struct {
	mu   sync.Mutex
	path string
}

func (f *fileSlices) read() map[string]*formats.Slicing {
	all := map[string]*formats.Slicing{}
	if b, err := os.ReadFile(f.path); err == nil {
		_ = json.Unmarshal(b, &all)
	}
	return all
}

func (f *fileSlices) load(key string) (*formats.Slicing, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.read()[key]
	return v, ok && v != nil
}

func (f *fileSlices) save(key string, v *formats.Slicing) {
	f.mu.Lock()
	defer f.mu.Unlock()
	all := f.read()
	all[key] = v
	b, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(f.path), 0o755) == nil {
		_ = os.WriteFile(f.path, b, 0o644)
	}
}
