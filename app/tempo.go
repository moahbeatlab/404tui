package app

import (
	"fmt"
	"math"
	"path"
	"regexp"
	"strings"
	"unicode"

	"github.com/moahbeatlab/404tui/proto"
)

// Sample packs put the tempo in the names: "124_Kit02/09_OHat_124_HDK_02.wav". A tempo is a
// word of two or three digits, optionally followed by "bpm", from 40 to 200.
var tempoWord = regexp.MustCompile(`^(\d{2,3})(?:bpm)?$`)

// TempoFromName is the tempo a name states, in BPM, and whether it states one. A number in
// front that is the pad (01 to 16) is not one: it is below the lowest tempo.
func TempoFromName(name string) (float64, bool) {
	stem := strings.TrimSuffix(name, path.Ext(name))
	words := strings.FieldsFunc(strings.ToLower(stem), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for _, w := range words {
		m := tempoWord.FindStringSubmatch(w)
		if m == nil {
			continue
		}
		var bpm float64
		fmt.Sscan(m[1], &bpm)
		if bpm >= MinBankBPM && bpm <= MaxBankBPM {
			return bpm, true
		}
	}
	return 0, false
}

// TempoPlan is the tempos a set of files states: one per file that names one, and one for
// the bank when they all agree.
type TempoPlan struct {
	// Files maps a file to its tempo.
	Files map[string]float64
	// Bank is the bank's tempo when HasBank.
	Bank    float64
	HasBank bool
}

// Steps is how many tempo settings the plan makes.
func (t TempoPlan) Steps() int {
	n := len(t.Files)
	if t.HasBank {
		n++
	}
	return n
}

func isPlaceholder(file string) bool {
	return strings.Contains(strings.ToUpper(path.Base(file)), "EMPTY")
}

// PlanTempos reads the tempos out of the names of files. The bank's tempo is the one every
// file that names a tempo agrees on; when none does, the folder's name is tried. Files
// named EMPTY say nothing. Without withBank only the files' own tempos are planned.
func PlanTempos(files []string, withBank bool) TempoPlan {
	plan := TempoPlan{Files: map[string]float64{}}
	agreed, conflict := 0.0, false
	for _, f := range files {
		if isPlaceholder(f) {
			continue
		}
		bpm, ok := TempoFromName(path.Base(f))
		if !ok {
			continue
		}
		plan.Files[f] = bpm
		if agreed != 0 && agreed != bpm {
			conflict = true
		}
		agreed = bpm
	}
	if !withBank || conflict {
		return plan
	}
	if agreed == 0 && len(files) > 0 {
		agreed, _ = TempoFromName(path.Base(path.Dir(files[0])))
	}
	plan.Bank, plan.HasBank = agreed, agreed != 0
	return plan
}

// SetPadTempo sets the tempo a pad's sample was recorded at, 40 to 200 BPM.
func SetPadTempo(p Pads, pad proto.PadIndex, bpm float64) error {
	info, ok := proto.ParamByName("bpm")
	if !ok {
		return fmt.Errorf("this version does not know the pad tempo")
	}
	return p.SetParam(info.ID, proto.TargetPad(pad), int32(math.Round(bpm*100)))
}

// TempoResult is what ApplyTempos did.
type TempoResult struct {
	Pads     int
	BankSet  bool
	Failures []string
}

// ApplyTempos sets the planned tempos after a multi-file assign: each placed file's pad,
// then the bank of the first placed pad. progress is called before each step, with the
// number done and what is next.
func ApplyTempos(p Pads, placed []Placement, plan TempoPlan, progress func(done int, what string)) TempoResult {
	var res TempoResult
	done := 0
	step := func(what string) {
		if progress != nil {
			progress(done, what)
		}
		done++
	}
	var firstPad *proto.PadIndex
	for _, pl := range placed {
		if pl.Err != nil || pl.Blank {
			continue
		}
		if firstPad == nil {
			pad := pl.Pad
			firstPad = &pad
		}
		bpm, ok := plan.Files[pl.File]
		if !ok {
			continue
		}
		step(fmt.Sprintf("tempo %g BPM on %s", bpm, pl.Pad))
		if err := SetPadTempo(p, pl.Pad, bpm); err != nil {
			res.Failures = append(res.Failures, fmt.Sprintf("tempo of %s (%v)", pl.Pad, err))
			continue
		}
		res.Pads++
	}
	if plan.HasBank && firstPad != nil {
		bank := int(firstPad.Bank())
		step(fmt.Sprintf("tempo %g BPM on bank %c", plan.Bank, 'A'+bank))
		if err := SetBankTempo(p, bank, plan.Bank); err != nil {
			res.Failures = append(res.Failures, fmt.Sprintf("tempo of bank %c (%v)", 'A'+bank, err))
		} else {
			res.BankSet = true
		}
	}
	return res
}
