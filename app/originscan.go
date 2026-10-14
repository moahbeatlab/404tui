package app

import (
	"encoding/binary"
	"math"
	"sort"
	"strconv"

	"github.com/moahbeatlab/404tui/formats"

	"github.com/moahbeatlab/404tui/proto"
)

// lengthTolerance is how far apart in seconds two lengths may be for the same sample: a file in
// another sample rate is converted when it is assigned, and the conversion rounds.
const lengthTolerance = 0.02

// WAVSeconds is the length of a WAV file from its first bytes (the format and the data
// chunk). It is false for anything else, or when the data chunk is further in than the bytes
// given.
func WAVSeconds(head []byte) (float64, bool) {
	if len(head) < 12 || string(head[:4]) != "RIFF" || string(head[8:12]) != "WAVE" {
		return 0, false
	}
	var rate, align uint32
	for at := 12; at+8 <= len(head); {
		id, size := string(head[at:at+4]), binary.LittleEndian.Uint32(head[at+4:])
		body := at + 8
		switch id {
		case "fmt ":
			if body+16 > len(head) {
				return 0, false
			}
			rate = binary.LittleEndian.Uint32(head[body+4:])
			align = uint32(binary.LittleEndian.Uint16(head[body+12:]))
		case "data":
			if rate == 0 || align == 0 {
				return 0, false
			}
			return float64(size) / float64(align) / float64(rate), true
		}
		at = body + int(size) + int(size&1)
	}
	return 0, false
}

// AmbiguousPad is a pad whose sample fits more than one file of the index.
type AmbiguousPad struct {
	Pad        proto.PadIndex
	Candidates []string
}

// OriginScan is what a scan for the origins of the samples on the pads found.
type OriginScan struct {
	// Found are the pads whose file is certain.
	Found []OriginRecord
	// Ambiguous are pads that fit several files, which a name and a length cannot tell apart.
	Ambiguous []AmbiguousPad
	// Unknown are the pads no file of the index fits.
	Unknown []proto.PadIndex
	// Known is how many pads had their origin already.
	Known int
}

// ScanOrigins looks for the files the samples on the pads came from, among the files of the
// index. A pad name is a file's name cut to fit, so the files whose pad name is the same are
// the candidates; when there are several, the sample's length (seconds gives a candidate's,
// padSeconds the pad's) narrows them, and then content, which compares the start of the audio. A pad with exactly one left is Found. progress is told
// of each pad with a line about what was decided.
func ScanOrigins(rows []PadRow, project uint8, have *Origins, files []IndexedFile,
	seconds func(path string) (float64, bool), padSeconds func(PadRow) (float64, bool),
	content func(row PadRow, candidates []string) []string,
	progress func(done, total int, line string)) OriginScan {
	byName := map[string][]string{}
	for _, f := range files {
		byName[PadName(f.Path)] = append(byName[PadName(f.Path)], f.Path)
	}
	var scan OriginScan
	var todo []PadRow
	for _, row := range rows {
		if !row.HasSample {
			continue
		}
		if _, ok := have.Of(project, row.Pad, row.Name); ok {
			scan.Known++
			continue
		}
		todo = append(todo, row)
	}
	say := func(done int, line string) {
		if progress != nil {
			progress(done, len(todo), line)
		}
	}
	for i, row := range todo {
		cands := byName[row.Name]
		if len(cands) == 0 {
			scan.Unknown = append(scan.Unknown, row.Pad)
			say(i, row.Pad.String()+" "+row.Name+": not in the index")
			continue
		}
		if seconds != nil && padSeconds != nil {
			if want, ok := padSeconds(row); ok {
				var fit []string
				for _, c := range cands {
					if got, ok := seconds(c); !ok || math.Abs(got-want) <= lengthTolerance {
						fit = append(fit, c)
					}
				}
				cands = fit
			}
		}
		// Still several: the audio itself says which, when content can read it.
		if len(cands) > 1 && content != nil {
			cands = content(row, cands)
		}
		switch len(cands) {
		case 0:
			scan.Unknown = append(scan.Unknown, row.Pad)
			say(i, row.Pad.String()+" "+row.Name+": no file of that name has its length")
		case 1:
			scan.Found = append(scan.Found, OriginRecord{Pad: row.Pad, Path: cands[0], Name: row.Name, Matched: true})
			say(i, row.Pad.String()+" "+row.Name+" ← "+cands[0])
		default:
			sort.Strings(cands)
			scan.Ambiguous = append(scan.Ambiguous, AmbiguousPad{Pad: row.Pad, Candidates: cands})
			say(i, row.Pad.String()+" "+row.Name+": fits "+strconv.Itoa(len(cands))+" files")
		}
	}
	return scan
}

// contentFrames is how many frames from the start are compared, and clipBytes how much of a
// candidate file is read for them: enough for that many frames of 16-bit stereo, plus room for
// a header, and for the longer clip a file in a lower sample rate makes once it is converted.
const (
	contentFrames = 6000
	clipBytes     = 40960
	maxCandidates = 64
)

// MatchByContent narrows candidate files to those that start with the audio the pad holds:
// the start of each file is converted as an assign would convert it, and compared with the
// start of the pad's sample. A pad whose start is silent cannot be told apart this way, and a
// file that is not a WAV cannot be read, so then every candidate stays. When none matches
// (the sample was changed on the unit, or the pad was trimmed) the candidates stay too.
func MatchByContent(u interface {
	Storage
	CurrentProject() (uint8, error)
}, row PadRow, candidates []string) []string {
	if len(candidates) > maxCandidates {
		return candidates
	}
	project, err := u.CurrentProject()
	if err != nil {
		return candidates
	}
	head, exists, err := u.ReadPrefix(row.Pad.SamplePath(project+1), formats.HeaderLen+contentFrames*4)
	if err != nil || !exists {
		return candidates
	}
	info, err := formats.ParseHeader(head)
	if err != nil {
		return candidates
	}
	pcm := head[formats.HeaderLen:]
	want := make([]int16, len(pcm)/2)
	loud := false
	for i := range want {
		want[i] = int16(binary.BigEndian.Uint16(pcm[2*i:]))
		loud = loud || want[i] > 64 || want[i] < -64
	}
	if !loud {
		return candidates
	}
	var same []string
	for _, c := range candidates {
		raw, exists, err := u.ReadPrefix(c, clipBytes)
		if err != nil || !exists {
			continue
		}
		clip := truncatedWAV(raw)
		if clip == nil {
			return candidates
		}
		got, err := formats.DecodeAudio(clip, ".wav")
		if err != nil || got.Sample.Channels != info.Channels {
			continue
		}
		n := min(len(got.Sample.Samples), len(want), contentFrames*info.Channels)
		if n < 400 {
			continue
		}
		close := 0
		for i := 0; i < n; i++ {
			if d := int(got.Sample.Samples[i]) - int(want[i]); d >= -2 && d <= 2 {
				close++
			}
		}
		if close*100 >= n*99 {
			same = append(same, c)
		}
	}
	if len(same) == 0 {
		return candidates
	}
	return same
}

// truncatedWAV is the first bytes of a WAV file made into a whole small WAV file: the sizes in
// it are set to what is there, whole frames only. It is nil for anything else.
func truncatedWAV(b []byte) []byte {
	if len(b) < 12 || string(b[:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return nil
	}
	var align uint32
	for at := 12; at+8 <= len(b); {
		id, size := string(b[at:at+4]), binary.LittleEndian.Uint32(b[at+4:])
		body := at + 8
		switch id {
		case "fmt ":
			if body+16 > len(b) {
				return nil
			}
			align = uint32(binary.LittleEndian.Uint16(b[body+12:]))
		case "data":
			if align == 0 {
				return nil
			}
			avail := uint32(len(b)-body) / align * align
			if avail == 0 {
				return nil
			}
			out := append([]byte(nil), b[:body+int(avail)]...)
			binary.LittleEndian.PutUint32(out[4:], uint32(len(out)-8))
			binary.LittleEndian.PutUint32(out[body-4:], avail)
			return out
		}
		at = body + int(size) + int(size&1)
	}
	return nil
}
