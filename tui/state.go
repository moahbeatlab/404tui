package tui

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/moahbeatlab/404tui/app"
)

// State is what the UI remembers between runs: where you were in the file trees, the bank
// you were on, the last pad a file went onto, and the colour theme.
type State struct {
	Theme int `json:"theme"`
	Bank  int `json:"bank"`
	// TargetBank is the bank files are assigned to: the one last assigned to.
	TargetBank int `json:"targetBank"`
	// LastAssigned is the last pad a file was assigned to, as its index plus one (0: none).
	LastAssigned int `json:"lastAssigned"`

	FilesRoot     string   `json:"filesRoot"`
	FilesExpanded []string `json:"filesExpanded"`
	FilesAt       string   `json:"filesAt"`

	AssignRoot     string   `json:"assignRoot"`
	AssignExpanded []string `json:"assignExpanded"`
	AssignAt       string   `json:"assignAt"`
}

// Store keeps the State. Saving is best effort: losing it costs only a remembered position.
type Store interface {
	Load() State
	Save(State)
}

// FileStore keeps the State in a JSON file.
type FileStore struct{ Path string }

// DefaultStore keeps the State in the user's config folder, or nowhere if there is none.
func DefaultStore() Store {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil
	}
	return FileStore{Path: filepath.Join(dir, "404tui", "state.json")}
}

func (f FileStore) Load() State {
	var s State
	if b, err := os.ReadFile(f.Path); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	return s
}

func (f FileStore) Save(s State) {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(f.Path), 0o755) == nil {
		_ = os.WriteFile(f.Path, b, 0o644)
	}
}

// rootKey is how a file source is written in the state: the unit's own storage has an
// empty path, which would read as "nothing remembered".
func rootKey(root string) string {
	if root == app.StorageRoot {
		return "unit"
	}
	return root
}

// rootFromKey is the file source a remembered key stands for, if it still is one.
func rootFromKey(key string) (string, bool) {
	for _, root := range roots() {
		if key != "" && rootKey(root) == key {
			return root, true
		}
	}
	return "", false
}

// stateBytes is a State as bytes, to tell whether it changed.
func stateBytes(s State) []byte {
	b, _ := json.Marshal(s)
	return b
}

var _ = bytes.Equal
