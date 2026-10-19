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
	// MidiLevelChan is the channel of the pad volumes (Levels): "15" when empty, "off" for none.
	MidiLevelChan string `json:"midiLevelChan,omitempty"`
	// LooperBank is the bank loops go to (0 to 9).
	LooperBank int `json:"looperBank,omitempty"`
	// LooperSlices is how many slices a remix cuts a loop in (4, 8, 16 or 32; 8 when unset).
	LooperSlices int `json:"looperSlices,omitempty"`
	// The web page's Slicer: how many slices, cut at random places, the order (0 shuffle, 1 in order, 2 random), the grid
	// (0 none, else notes to the bar), its swing, and the bank the result goes to (0 the Looper's, 1 to 10 for A to J).
	SlicerSlices    int  `json:"slicerSlices,omitempty"`
	SlicerRandomCut bool `json:"slicerRandomCut,omitempty"`
	SlicerOrder     int  `json:"slicerOrder,omitempty"`
	SlicerDiv       int  `json:"slicerDiv,omitempty"`
	SlicerSwing     int  `json:"slicerSwing,omitempty"`
	SlicerTarget    int  `json:"slicerTarget,omitempty"`
	// The dice of the Slicer (see mangleOpts): drop-outs, reversed, pitch spread, rolls, humanize (percent), fade (ms).
	SlicerDrop, SlicerRev, SlicerPitch, SlicerRoll, SlicerHuman, SlicerFade int
	// Punch are the four snapshots of the effects of all four buses (see punch.go).
	Punch [4]PunchSnap `json:"punch"`
	// BusNotes are what the user wrote of the effect on each bus (the unit does not say), shown in
	// the Perform view.
	BusNotes [2]string `json:"busNotes,omitempty"`

	// ScanDir is the folder the last scan for where the samples came from looked in.
	ScanDir string `json:"scanDir,omitempty"`

	AssignRoot     string   `json:"assignRoot"`
	AssignExpanded []string `json:"assignExpanded"`
	AssignAt       string   `json:"assignAt"`
}

// PunchBus is the effect of one bus as a snapshot holds it: which effect (CC 83), on or off (1, 0), and its six
// knobs; -1 is a value the unit had not said.
type PunchBus struct {
	Set   bool   `json:"set,omitempty"`
	On    int    `json:"on"`
	Num   int    `json:"num"`
	Knobs [6]int `json:"knobs"`
}

// PunchSnap is a snapshot of the four buses.
type PunchSnap [4]PunchBus

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
