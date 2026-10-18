package app

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
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
		return formats.Imported{}, fmt.Errorf("%s: %w", pBase(file), err)
	}
	imported, err := formats.DecodeAudio(b, pExt(file))
	if err != nil {
		return formats.Imported{}, fmt.Errorf("%s: %w", pBase(file), err)
	}
	return imported, nil
}

// PadName is the name a pad gets from a file: its name without the extension, printable
// ASCII only, cut to fit.
func PadName(file string) string {
	base := pBase(file)
	stem := strings.TrimSuffix(base, pExt(base))
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
	// A sample put on a pad plays at full level, whatever the pad had before (and a pad that
	// had none reads as 0): 127, as a pad is when it is new.
	if p, ok := u.(interface {
		SetParam(id byte, target proto.Target, value int32) error
	}); ok {
		_ = p.SetParam(paramLevel, proto.TargetPad(pad), DefaultPadLevel)
	}
	return name, nil
}

// PutSampleLive is PutSample for a unit that is playing (the Looper and the Slicer put samples on pads in the middle of a
// pattern): the unit is not stopped for the import, if it can do without. When it cannot, the import is made the usual way.
func PutSampleLive(u Importer, pad proto.PadIndex, sample formats.Sample, name string) (string, error) {
	live, ok := u.(interface {
		ImportSMPLive(project uint8, pad proto.PadIndex, smp []byte, name string) error
	})
	if !ok {
		return PutSample(u, pad, sample, name)
	}
	current, err := u.CurrentProject()
	if err != nil {
		return "", err
	}
	if err := live.ImportSMPLive(current+1, pad, sample.SMP(), name); err != nil {
		return PutSample(u, pad, sample, name)
	}
	if p, ok := u.(interface {
		SetParam(id byte, target proto.Target, value int32) error
	}); ok {
		_ = p.SetParam(paramLevel, proto.TargetPad(pad), DefaultPadLevel)
	}
	return name, nil
}

// DefaultPadLevel is the level a pad gets with a sample put on it.
const DefaultPadLevel = 127

// Placement is where one file of a multi-file assign went.
type Placement struct {
	File string
	Pad  proto.PadIndex
	Err  error
	// Blank is a placeholder file, named EMPTY and holding only silence: its pad was left
	// empty, so the files after it keep their places.
	Blank bool
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

// ErrNoFreePad is a file that found no pad to go on.
var ErrNoFreePad = errors.New("no free pad left")

// AssignMany puts files onto the given pads in order (as many files as pads), calling
// progress before each one. A file that fails does not stop the rest.
func AssignMany(u Importer, s Storage, pads []proto.PadIndex, files []string, progress func(i int, file string)) []Placement {
	return AssignManyReport(u, s, pads, files, progress, nil)
}

// AssignManyReport is AssignMany that also calls report with each file's outcome as soon
// as it is known.
func AssignManyReport(u Importer, s Storage, pads []proto.PadIndex, files []string, progress func(i int, file string), report func(Placement)) []Placement {
	out := make([]Placement, 0, len(files))
	add := func(p Placement) {
		out = append(out, p)
		if report != nil {
			report(p)
		}
	}
	for i, file := range files {
		if i >= len(pads) {
			add(Placement{File: file, Err: fmt.Errorf("%w for %s", ErrNoFreePad, pBase(file))})
			continue
		}
		if progress != nil {
			progress(i, file)
		}
		imported, err := LoadSample(s, file)
		if err == nil && IsBlankPlaceholder(file, imported.Sample) {
			add(Placement{File: file, Pad: pads[i], Blank: true})
			continue
		}
		if err == nil {
			_, err = PutSample(u, pads[i], imported.Sample, PadName(file))
		}
		add(Placement{File: file, Pad: pads[i], Err: err})
	}
	return out
}

// IsAudioName reports whether a file name is one that can be put on a pad.
func IsAudioName(name string) bool {
	return formats.IsAudioFile(name)
}

// silence is the loudest a placeholder may be: dither and rounding noise, not sound.
const silence = 8

// IsBlankPlaceholder reports whether a file is a placeholder that keeps a position in a
// kit (named EMPTY, as sample packs do) and really is empty: no frames, or only silence.
// A file named EMPTY that holds sound is a sample like any other.
func IsBlankPlaceholder(file string, s formats.Sample) bool {
	if !strings.Contains(strings.ToUpper(pBase(file)), "EMPTY") {
		return false
	}
	for _, v := range s.Samples {
		if v > silence || v < -silence {
			return false
		}
	}
	return true
}

var numberPrefix = regexp.MustCompile(`^(\d{1,2})[ _.-]`)

// NumberedPads is where files named "09_Snare.wav" go: the number in front is the pad, 1 to
// 16, of the given bank (0..=9). It reports false unless every file has such a number, no
// two share one and none is above 16. A file numbered 0 belongs to no pad: it goes to the
// first pad of the bank that no other file is numbered for (a file named EMPTY does not
// count, it only keeps its place) and that taken says holds no sample.
func NumberedPads(files []string, bank int, taken func(proto.PadIndex) bool) ([]proto.PadIndex, bool) {
	pads := make([]proto.PadIndex, len(files))
	numbers := make([]int, len(files))
	claimed := map[int]bool{}
	for i, f := range files {
		m := numberPrefix.FindStringSubmatch(pBase(f))
		if m == nil {
			return nil, false
		}
		n, _ := strconv.Atoi(m[1])
		if n > proto.PadsPerBank || (n > 0 && slices.Contains(numbers[:i], n)) {
			return nil, false
		}
		numbers[i] = n
		if !strings.Contains(strings.ToUpper(pBase(f)), "EMPTY") {
			claimed[n] = true
		}
	}
	for i, n := range numbers {
		if n == 0 {
			for c := 1; c <= proto.PadsPerBank; c++ {
				pad, _ := proto.NewPadIndex(uint16(bank*proto.PadsPerBank + c - 1))
				if !claimed[c] && !taken(pad) {
					pads[i], claimed[c] = pad, true
					break
				}
				if c == proto.PadsPerBank {
					return nil, false
				}
			}
			continue
		}
		pads[i], _ = proto.NewPadIndex(uint16(bank*proto.PadsPerBank + n - 1))
	}
	return pads, true
}
