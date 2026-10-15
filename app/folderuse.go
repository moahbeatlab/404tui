package app

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/moahbeatlab/404tui/proto"
)

// FolderUseName is the file on the SD card that remembers which folders were restored onto
// which banks of which projects, so that the Files view can say a folder was used already.
// It is keyed by project and bank, so it holds 160 entries at most and a new restore onto a
// bank replaces the one before.
const FolderUseName = "404tui-folders.json"

// FolderUsePath is where the notes live.
var FolderUsePath = Join(CardRoot, FolderUseName)

// FolderUse is one bank that a folder was put on.
type FolderUse struct {
	// Folder is the folder that was restored, as the Files view names it.
	Folder string `json:"folder"`
	// Pads is how many of the bank's pads came from it.
	Pads int `json:"pads"`
	// At is when it was restored, or found.
	At string `json:"at"`
	// Matched is set when a scan of the project found it, not a restore that made it.
	Matched bool `json:"matched,omitempty"`
}

// FolderUses are the notes, by project and bank ("1/C").
type FolderUses struct {
	Banks map[string]FolderUse `json:"banks"`
}

func folderUseKey(project uint8, bank int) string {
	return fmt.Sprintf("%d/%c", project+1, 'A'+bank)
}

// LoadFolderUses reads the notes from the card. A card without them gives an empty set.
func LoadFolderUses(s Storage) (*FolderUses, error) {
	f := &FolderUses{Banks: map[string]FolderUse{}}
	data, exists, err := readIfThere(s, FolderUsePath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", FolderUsePath, err)
	}
	if !exists {
		return f, nil
	}
	if err := json.Unmarshal(data, f); err != nil {
		return nil, fmt.Errorf("%s is not readable (%v): delete it to start again", FolderUsePath, err)
	}
	if f.Banks == nil {
		f.Banks = map[string]FolderUse{}
	}
	return f, nil
}

// Save writes the notes to the card.
func (f *FolderUses) Save(s Storage) error {
	data, err := json.MarshalIndent(f, "", " ")
	if err != nil {
		return err
	}
	if err := s.WriteFile(FolderUsePath, data); err != nil {
		return fmt.Errorf("writing %s: %w", FolderUsePath, err)
	}
	return nil
}

// FolderUseRecord is a bank a folder is now on.
type FolderUseRecord struct {
	Bank    int
	Folder  string
	Pads    int
	Matched bool
}

// RecordFolderUses notes the banks of a project that folders were put on, and writes the
// notes. It reads the card's notes first, so that what other runs wrote is kept.
func RecordFolderUses(s Storage, project uint8, records []FolderUseRecord) (*FolderUses, error) {
	f, err := LoadFolderUses(s)
	if err != nil {
		return nil, err
	}
	at := time.Now().Format("2006-01-02 15:04")
	for _, r := range records {
		f.Banks[folderUseKey(project, r.Bank)] = FolderUse{Folder: r.Folder, Pads: r.Pads, At: at, Matched: r.Matched}
	}
	return f, f.Save(s)
}

// Clone is a copy that shares nothing with the notes.
func (f *FolderUses) Clone() *FolderUses {
	return &FolderUses{Banks: maps.Clone(f.Banks)}
}

// BankUse is a folder's place on a bank.
type BankUse struct {
	Project int // 1-based, as the unit shows it
	Bank    int
	FolderUse
}

// Of is where a folder was put: its own entries, and the entries of the folders inside it.
func (f *FolderUses) Of(folder string) (own, inside []BankUse) {
	if f == nil {
		return nil, nil
	}
	for key, use := range f.Banks {
		var project int
		var bank rune
		if _, err := fmt.Sscanf(key, "%d/%c", &project, &bank); err != nil {
			continue
		}
		u := BankUse{Project: project, Bank: int(bank - 'A'), FolderUse: use}
		switch {
		case use.Folder == folder:
			own = append(own, u)
		case strings.HasPrefix(use.Folder, strings.TrimSuffix(folder, "/")+"/"):
			inside = append(inside, u)
		}
	}
	order := func(a, b BankUse) int {
		if a.Project != b.Project {
			return a.Project - b.Project
		}
		return a.Bank - b.Bank
	}
	slices.SortFunc(own, order)
	slices.SortFunc(inside, order)
	return own, inside
}

// Used is whether the folder, or one inside it, was put on a bank.
func (f *FolderUses) Used(folder string) bool {
	own, inside := f.Of(folder)
	return len(own)+len(inside) > 0
}

// FolderUsesFromOrigins works out which folder each bank of a project was filled from: the
// folder most of the bank's samples came from, when it is at least half of them and more than
// one pad. A pad's origin counts only while the pad still has the name it was given.
func FolderUsesFromOrigins(o *Origins, project uint8, rows []PadRow) []FolderUseRecord {
	var out []FolderUseRecord
	for bank := range 10 {
		held, from := 0, map[string]int{}
		for _, row := range rows {
			if !row.HasSample || int(row.Pad.Bank()) != bank {
				continue
			}
			held++
			if origin, ok := o.Of(project, row.Pad, row.Name); ok {
				from[Dir(origin.Path)]++
			}
		}
		best, most := "", 0
		for folder, n := range from {
			if n > most || n == most && folder < best {
				best, most = folder, n
			}
		}
		if most > 1 && most*2 >= held {
			out = append(out, FolderUseRecord{Bank: bank, Folder: best, Pads: most, Matched: true})
		}
	}
	return out
}

// RestoredBanks counts the pads an export put on each bank.
func RestoredBanks(targets []proto.PadIndex) map[int]int {
	out := map[int]int{}
	for _, t := range targets {
		out[int(t.Bank())]++
	}
	return out
}
