package app

import (
	"fmt"
	"path"
	"strings"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// MaxPadName is how much of a file's name goes on the pad.
const MaxPadName = 16

// LoadSample reads and decodes an audio file from the unit or the computer.
func LoadSample(s Storage, file string) (formats.Imported, error) {
	b, err := s.ReadFile(file)
	if err != nil {
		return formats.Imported{}, fmt.Errorf("%s: %w", path.Base(file), err)
	}
	imported, err := formats.DecodeAudio(b, path.Ext(file))
	if err != nil {
		return formats.Imported{}, fmt.Errorf("%s: %w", path.Base(file), err)
	}
	return imported, nil
}

// PadName is the name a pad gets from a file: its name without the extension, printable
// ASCII only, cut to fit.
func PadName(file string) string {
	base := path.Base(file)
	stem := strings.TrimSuffix(base, path.Ext(base))
	var name []rune
	for _, r := range stem {
		if r >= ' ' && r <= '~' && len(name) < MaxPadName {
			name = append(name, r)
		}
	}
	if strings.TrimSpace(string(name)) == "" {
		return "Sample"
	}
	return string(name)
}

// Assign puts an audio file onto a pad of the current project and returns the name the
// pad now has.
func Assign(u Importer, s Storage, pad proto.PadIndex, file string) (string, error) {
	imported, err := LoadSample(s, file)
	if err != nil {
		return "", err
	}
	return PutSample(u, pad, imported.Sample, PadName(file))
}

// PutSample puts decoded audio onto a pad of the current project.
func PutSample(u Importer, pad proto.PadIndex, sample formats.Sample, name string) (string, error) {
	current, err := u.CurrentProject()
	if err != nil {
		return "", err
	}
	if err := u.ImportSMP(current+1, pad, sample.SMP(), name); err != nil {
		return "", fmt.Errorf("importing onto %s: %w", pad, err)
	}
	return name, nil
}

// Placement is where one file of a multi-file assign went.
type Placement struct {
	File string
	Pad  proto.PadIndex
	Err  error
}

// FreePads lists, from start on, up to count pads that hold no sample, to the end of the
// last bank. taken says which pads hold one.
func FreePads(start proto.PadIndex, count int, taken func(proto.PadIndex) bool) []proto.PadIndex {
	var pads []proto.PadIndex
	for i := start.Index(); i < proto.PadCount && len(pads) < count; i++ {
		if pad, _ := proto.NewPadIndex(i); !taken(pad) {
			pads = append(pads, pad)
		}
	}
	return pads
}

// AssignMany puts files onto the given pads in order (as many files as pads), calling
// progress before each one. A file that fails does not stop the rest.
func AssignMany(u Importer, s Storage, pads []proto.PadIndex, files []string, progress func(i int, file string)) []Placement {
	out := make([]Placement, 0, len(files))
	for i, file := range files {
		if i >= len(pads) {
			out = append(out, Placement{File: file, Err: fmt.Errorf("no free pad left for %s", path.Base(file))})
			continue
		}
		if progress != nil {
			progress(i, file)
		}
		_, err := Assign(u, s, pads[i], file)
		out = append(out, Placement{File: file, Pad: pads[i], Err: err})
	}
	return out
}

// IsAudioName reports whether a file name is one that can be put on a pad.
func IsAudioName(name string) bool {
	return formats.IsAudioFile(name)
}
