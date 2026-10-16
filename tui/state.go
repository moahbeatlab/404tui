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
	// PreviewLocal plays the pads on this computer instead of on the unit.
	PreviewLocal bool `json:"previewLocal"`
	// TargetBank is the bank files are assigned to: the one last assigned to.
	TargetBank int `json:"targetBank"`
	// LastAssigned is the last pad a file was assigned to, as its index plus one (0: none).
	LastAssigned int `json:"lastAssigned"`

	FilesRoot     string   `json:"filesRoot"`
	FilesExpanded []string `json:"filesExpanded"`
	FilesAt       string   `json:"filesAt"`

	// Searches is what was last typed in each search, and the file that was in front, by
	// kind and source ("files|SD:", "tags|SD:"): the search opens on it again.
	Searches map[string]SearchMemory `json:"searches,omitempty"`

	// OfflineDir is the folder of projects last worked on offline.
	OfflineDir string `json:"offlineDir,omitempty"`
	// Projects is the project last open on each source, by source ("unit", or the folder worked
	// on offline), as its number from 1: a folder opened offline goes back to it.
	Projects map[string]int `json:"projects,omitempty"`

	// Buses is the rule that sends a pad to a bus by its name, as the Config view sets it; nil is
	// the default.
	Buses *app.BusRule `json:"buses,omitempty"`

	// RecordDir is the folder recordings go into, RecordDevice a part of the name of the input to
	// record; empty means the defaults.
	RecordDir    string `json:"recordDir,omitempty"`
	RecordDevice string `json:"recordDevice,omitempty"`

	// LiveCode and RecCode are the pads, hit in turn on the unit, that switch the live mode and a
	// recording ("1 3 3 7", "1 16 16 16" when empty; "off" turns one off).
	LiveCode string `json:"liveCode,omitempty"`
	RecCode  string `json:"recCode,omitempty"`
	// MidiPort is a part of the name of the MIDI input to listen to ("404" when empty); MidiChan
	// the channel that controls the app ("16" when empty, "off" for none); MidiMap the message
	// for each action: live mode, recording, next becomes now, clear next ("note 60" or "cc 20";
	// notes 60 to 63 when empty, "off" for none).
	MidiPort string    `json:"midiPort,omitempty"`
	MidiChan string    `json:"midiChan,omitempty"`
	MidiMap  [4]string `json:"midiMap,omitempty"`
	// BusNotes are what the user wrote of the effect on each bus (the unit does not say), shown in
	// the Perform view.
	BusNotes [2]string `json:"busNotes,omitempty"`
	// FxMsg is the MIDI message that says a bus's effect is on or off ("ch 1 cc 83" or "ch 1 note 60"),
	// learned from the unit in the Perform view.
	FxMsg [2]string `json:"fxMsg,omitempty"`
	// FxKnobs are the controllers ("ch 1 cc 16") shown as faders and dials under each bus's effect,
	// up to three a bus, learned from the unit.
	FxKnobs [2][]string `json:"fxKnobs,omitempty"`

	// ScanDir is the folder the last scan for where the samples came from looked in.
	ScanDir string `json:"scanDir,omitempty"`

	AssignRoot     string   `json:"assignRoot"`
	AssignExpanded []string `json:"assignExpanded"`
	AssignAt       string   `json:"assignAt"`
}

// SearchMemory is one remembered search.
type SearchMemory struct {
	Query string `json:"query"`
	// At is the path of the result that was in front.
	At string `json:"at,omitempty"`
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
