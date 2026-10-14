package app

import (
	"encoding/json"
	"fmt"
	"maps"
	"time"

	"github.com/moahbeatlab/404tui/proto"
)

// OriginsName is the file on the SD card that remembers where the samples on the pads came
// from. A pad's name holds 16 characters, too few for a folder, so the folder is kept here.
const OriginsName = "404tui-origins.json"

// OriginsPath is where the notes live.
var OriginsPath = Join(CardRoot, OriginsName)

// Origin is where the sample on a pad came from.
type Origin struct {
	// Path is the file that was assigned, with the folders it was in.
	Path string `json:"path"`
	// Name is the name the pad was given. The note is only shown while the pad still has it.
	Name string `json:"name"`
	// At is when it was assigned.
	At string `json:"at"`
}

// Origins are the notes, by project and pad ("1/B9").
type Origins struct {
	Pads map[string]Origin `json:"pads"`
}

func originKey(project uint8, pad proto.PadIndex) string {
	return fmt.Sprintf("%d/%s", project+1, pad)
}

// LoadOrigins reads the notes from the card. A card without them gives an empty set.
func LoadOrigins(s Storage) (*Origins, error) {
	o := &Origins{Pads: map[string]Origin{}}
	data, exists, err := readIfThere(s, OriginsPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", OriginsPath, err)
	}
	if !exists {
		return o, nil
	}
	if err := json.Unmarshal(data, o); err != nil {
		return nil, fmt.Errorf("%s is not readable (%v): delete it to start again", OriginsPath, err)
	}
	if o.Pads == nil {
		o.Pads = map[string]Origin{}
	}
	return o, nil
}

// Save writes the notes to the card.
func (o *Origins) Save(s Storage) error {
	data, err := json.MarshalIndent(o, "", " ")
	if err != nil {
		return err
	}
	if err := s.WriteFile(OriginsPath, data); err != nil {
		return fmt.Errorf("writing %s: %w", OriginsPath, err)
	}
	return nil
}

// Clone is a copy that shares nothing with the notes.
func (o *Origins) Clone() *Origins {
	return &Origins{Pads: maps.Clone(o.Pads)}
}

// Of is where the sample on a pad came from, as long as the pad still has the name it was
// given: if the sample was changed on the unit since, the note is not about it any more.
func (o *Origins) Of(project uint8, pad proto.PadIndex, name string) (Origin, bool) {
	if o == nil {
		return Origin{}, false
	}
	n, ok := o.Pads[originKey(project, pad)]
	return n, ok && n.Name == name
}

// OriginRecord is a file that was put on a pad.
type OriginRecord struct {
	Pad  proto.PadIndex
	Path string
	Name string
}

// RecordOrigins notes where the samples just assigned came from, and writes the notes. It
// reads the card's notes first, so that what other runs wrote is kept, and returns the new set.
func RecordOrigins(s Storage, project uint8, records []OriginRecord) (*Origins, error) {
	o, err := LoadOrigins(s)
	if err != nil {
		return nil, err
	}
	at := time.Now().Format("2006-01-02 15:04")
	for _, r := range records {
		o.Pads[originKey(project, r.Pad)] = Origin{Path: r.Path, Name: r.Name, At: at}
	}
	return o, o.Save(s)
}

// ForgetOrigins takes the notes of pads whose samples are gone, and writes the notes.
func ForgetOrigins(s Storage, project uint8, pads []proto.PadIndex) (*Origins, error) {
	o, err := LoadOrigins(s)
	if err != nil {
		return nil, err
	}
	for _, pad := range pads {
		delete(o.Pads, originKey(project, pad))
	}
	return o, o.Save(s)
}
