package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/proto"
)

// patternConds is what is kept for a pattern: its conditions, and the seed that its chances are
// rolled from, so that what is heard and what is bounced are the same.
type patternConds struct {
	Seed  int64      `json:"seed,omitempty"`
	Notes []condNote `json:"notes,omitempty"`
}

type condNote struct {
	Pad  string `json:"pad"`
	Step int    `json:"step"`
	A    int    `json:"a,omitempty"`
	B    int    `json:"b,omitempty"`
	Prob int    `json:"prob,omitempty"`
}

func (p patternConds) conditions() app.Conditions {
	out := app.Conditions{}
	for _, n := range p.Notes {
		pad, err := proto.ParsePad(n.Pad)
		if err != nil {
			continue
		}
		out[app.NoteKey{Pad: pad, Step: n.Step}] = app.Cond{A: n.A, B: n.B, Prob: n.Prob}
	}
	return out
}

func condsOf(cs app.Conditions, seed int64) patternConds {
	out := patternConds{Seed: seed}
	for key, c := range cs {
		if c.Plain() {
			continue
		}
		out.Notes = append(out.Notes, condNote{Pad: key.Pad.String(), Step: key.Step, A: c.A, B: c.B, Prob: c.Prob})
	}
	return out
}

// condStore keeps the conditions of patterns, by a key that names the pattern (the source, the
// project and the slot). The unit's pattern files have no place for them. Saving is best effort.
type condStore interface {
	load(key string) patternConds
	save(key string, p patternConds)
}

// newCondStore keeps them in conditions.json next to the state file; with a store that is not a
// file (as in tests) in memory.
func newCondStore(store Store) condStore {
	if fs, ok := store.(FileStore); ok && fs.Path != "" {
		return &fileConds{path: filepath.Join(filepath.Dir(fs.Path), "conditions.json")}
	}
	return &memConds{m: map[string]patternConds{}}
}

type memConds struct {
	mu sync.Mutex
	m  map[string]patternConds
}

func (s *memConds) load(key string) patternConds {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[key]
}

func (s *memConds) save(key string, p patternConds) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(p.Notes) == 0 && p.Seed == 0 {
		delete(s.m, key)
		return
	}
	s.m[key] = p
}

type fileConds struct {
	mu   sync.Mutex
	path string
}

func (f *fileConds) read() map[string]patternConds {
	all := map[string]patternConds{}
	if b, err := os.ReadFile(f.path); err == nil {
		_ = json.Unmarshal(b, &all)
	}
	return all
}

func (f *fileConds) load(key string) patternConds {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.read()[key]
}

func (f *fileConds) save(key string, p patternConds) {
	f.mu.Lock()
	defer f.mu.Unlock()
	all := f.read()
	if len(p.Notes) == 0 && p.Seed == 0 {
		delete(all, key)
	} else {
		all[key] = p
	}
	b, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(f.path), 0o755) == nil {
		_ = os.WriteFile(f.path, b, 0o644)
	}
}
