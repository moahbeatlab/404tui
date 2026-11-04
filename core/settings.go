package core

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/moahbeatlab/404tui/app"
)

// The settings the app keeps between starts: the folder the file browser was in, the favourite folders, the autosave
// interval. They live in settings.json next to the card folder (the parent of OutDir), never inside it: what is in the
// card folder is what gets copied to the SP-404MKII's card.
type settings struct {
	LastDir     string   `json:"last_dir,omitempty"`
	Favourites  []string `json:"favourites,omitempty"`
	AutosaveMin *int     `json:"autosave_min,omitempty"`

	// ONLINE and OFFLINE (the PROJECTS screen of a computer): the folder of projects to work on without the unit, and the project
	// (1..99) that was open last in it.
	ExportDir      string `json:"export_dir,omitempty"` // where banks and projects are exported to
	BackupDir      string `json:"backup_dir,omitempty"` // where the unit's projects are backed up to
	OfflineDir     string `json:"offline_dir,omitempty"`
	LastProjectDir string `json:"last_project_dir,omitempty"`  // where a project was last opened from
	NoNative       bool   `json:"no_native_capture,omitempty"` // recording and the meter with ffmpeg or sox, not the program's own capture
	Debug          int    `json:"debug,omitempty"`             // the debug log: 0 off, 1 on, 2 with the wire bytes (see package dbg)
	LastOffline    int    `json:"last_offline,omitempty"`

	// The Config screen: the rule that sends a pad to a bus by its name (nil: the default), the recorder, the pad codes
	// and the MIDI settings, as the terminal program's Config had them.
	Theme         int          `json:"theme,omitempty"`
	Buses         *app.BusRule `json:"buses,omitempty"`
	RecordDir     string       `json:"record_dir,omitempty"`
	RecordDevice  string       `json:"record_device,omitempty"`
	LiveCode      string       `json:"live_code,omitempty"`
	RecCode       string       `json:"rec_code,omitempty"`
	MidiPort      string       `json:"midi_port,omitempty"`
	MidiChan      string       `json:"midi_chan,omitempty"`
	MidiMap       [4]string    `json:"midi_map,omitempty"`
	MidiLevelChan string       `json:"midi_level_chan,omitempty"`
	BusNotes      [2]string    `json:"bus_notes,omitempty"`
	LooperBank    int          `json:"looper_bank,omitempty"`
}

// dataDir is where the app keeps its own files (settings, autosaves): beside the card folder.
func (a *App) dataDir() string { return filepath.Dir(filepath.Clean(a.OutDir)) }

func (a *App) settingsPath() string { return filepath.Join(a.dataDir(), "settings.json") }

func (a *App) loadSettings() {
	b, err := os.ReadFile(a.settingsPath())
	if err != nil {
		return
	}
	json.Unmarshal(b, &a.set)
}

func (a *App) saveSettings() {
	b, err := json.MarshalIndent(&a.set, "", "  ")
	if err != nil {
		return
	}
	os.MkdirAll(a.dataDir(), 0o755)
	tmp := a.settingsPath() + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		os.Rename(tmp, a.settingsPath())
	}
}

func (a *App) isFavourite(dir string) bool {
	for _, f := range a.set.Favourites {
		if f == dir {
			return true
		}
	}
	return false
}

// toggleFavourite adds the folder to the favourites or takes it off; it says which.
func (a *App) toggleFavourite(dir string) {
	for i, f := range a.set.Favourites {
		if f == dir {
			a.set.Favourites = append(a.set.Favourites[:i], a.set.Favourites[i+1:]...)
			a.saveSettings()
			a.say("off the favourites: "+short2(dir, 40), false)
			return
		}
	}
	a.set.Favourites = append(a.set.Favourites, dir)
	a.saveSettings()
	a.say("a favourite: "+short2(dir, 44), false)
}
