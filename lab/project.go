// Package lab is the part of 404tui that works on files only: a project (PADCONF.BIN, SMPL,
// PTN) held in memory, samples, the slicer, chords and arps, and patterns. It has no I/O, so
// the same code runs natively (tests) and as WebAssembly in the browser (cmd/labwasm), where
// the 404jam web app uses it to prepare a project for the SP-404MKII without a unit or a server.
package lab

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// Lab is a project in memory plus the samples being worked on.
type Lab struct {
	files   map[string][]byte // project files by path inside the project: "SMPL/BANK1-01.SMP"
	pad     *formats.Padconf
	staged  map[string][]byte // files added since Begin, before Finish sorts them out
	samples map[int]formats.Sample
	next    int
}

// New is an empty lab with no project.
func New() *Lab {
	l := &Lab{}
	l.Reset()
	return l
}

// Reset forgets the project and every sample.
func (l *Lab) Reset() {
	l.files = map[string][]byte{}
	l.pad = nil
	l.staged = nil
	l.samples = map[int]formats.Sample{}
	l.next = 1
}

// NewProject starts an empty project called name.
func (l *Lab) NewProject(name string) {
	l.files = map[string][]byte{}
	l.pad = formats.NewPadconf(nil, clean(name, 31))
}

// Begin starts adding the files of a project; Add stages each one and Finish builds the
// project from them.
func (l *Lab) Begin() { l.staged = map[string][]byte{} }

// Add stages one file by its path (any folder depth, / or \ as separator).
func (l *Lab) Add(p string, data []byte) {
	if l.staged == nil {
		l.Begin()
	}
	l.staged[strings.ReplaceAll(p, "\\", "/")] = data
}

// ErrNoProject is a set of files with no PADCONF.BIN in it.
var ErrNoProject = errors.New("no PADCONF.BIN in those files: pick the project folder (or a zip of it) from the SD card")

// Finish makes the staged files the project: the folder holding PADCONF.BIN is the project's
// root (whatever is above it, such as ROLAND/SP-404MKII/PROJECT_01, is dropped), and only its
// PADCONF.BIN, SMPL, PTN and PICTURE files are kept. It returns the number of files kept.
func (l *Lab) Finish() (int, error) {
	staged := l.staged
	l.staged = nil
	root, found := "", false
	for p := range staged {
		if path.Base(p) == "PADCONF.BIN" {
			dir := path.Dir(p)
			if dir == "." {
				dir = ""
			}
			if !found || len(dir) < len(root) || (len(dir) == len(root) && dir < root) {
				root, found = dir, true
			}
		}
	}
	if !found {
		return 0, ErrNoProject
	}
	prefix := root
	if prefix != "" {
		prefix += "/"
	}
	files := map[string][]byte{}
	for p, data := range staged {
		if !strings.HasPrefix(p, prefix) {
			continue
		}
		rel := strings.TrimPrefix(p, prefix)
		top, _, _ := strings.Cut(rel, "/")
		if rel != "PADCONF.BIN" && top != "SMPL" && top != "PTN" && top != "PICTURE" {
			continue
		}
		files[rel] = data
	}
	pc, err := formats.ParsePadconf(files["PADCONF.BIN"])
	if err != nil {
		return 0, err
	}
	delete(files, "PADCONF.BIN")
	l.files, l.pad = files, pc
	if pc.Name == "" {
		pc.Name = clean(path.Base(root), 31)
	}
	return len(files) + 1, nil
}

// LoadZip loads a zip of a project folder (or of a card with the project in it).
func (l *Lab) LoadZip(b []byte) (int, error) {
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return 0, fmt.Errorf("not a zip file: %w", err)
	}
	l.Begin()
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || strings.Contains(f.Name, "__MACOSX/") || strings.HasPrefix(path.Base(f.Name), "._") {
			continue
		}
		if f.UncompressedSize64 > 1<<28 {
			return 0, fmt.Errorf("%s is too big", f.Name)
		}
		rc, err := f.Open()
		if err != nil {
			return 0, err
		}
		var buf bytes.Buffer
		_, err = buf.ReadFrom(rc)
		rc.Close()
		if err != nil {
			return 0, err
		}
		l.Add(f.Name, buf.Bytes())
	}
	return l.Finish()
}

// ExportZip is the project as a zip with everything under a folder called folder (the name of
// a PROJECT_nn folder on the card).
func (l *Lab) ExportZip(folder string) ([]byte, error) {
	if l.pad == nil {
		return nil, errors.New("there is no project")
	}
	folder = strings.Trim(clean(folder, 40), "/")
	if folder == "" {
		folder = "PROJECT_01"
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	put := func(name string, data []byte) error {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: folder + "/" + name, Method: zip.Store})
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
	if err := put("PADCONF.BIN", l.pad.Bytes()); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(l.files))
	for n := range l.files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if err := put(n, l.files[n]); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// PadconfBytes is the PADCONF.BIN as it would be written.
func (l *Lab) PadconfBytes() ([]byte, error) {
	if l.pad == nil {
		return nil, errors.New("there is no project")
	}
	return l.pad.Bytes(), nil
}

var nonPrint = regexp.MustCompile(`[^\x20-\x7e]`)

// clean keeps the printable ASCII of s, cut to n characters.
func clean(s string, n int) string {
	s = nonPrint.ReplaceAllString(s, "")
	if len(s) > n {
		s = s[:n]
	}
	return strings.TrimSpace(s)
}

// PadInfo is one pad of the project.
type PadInfo struct {
	N      int    `json:"n"`
	Label  string `json:"label"` // A1 .. J16
	Name   string `json:"name"`
	Has    bool   `json:"has"`    // the pad block says it holds a sample
	File   bool   `json:"file"`   // the SMP file is there
	Frames int    `json:"frames"` // length of the sample, in frames at 48 kHz
	Ch     int    `json:"ch"`
	Level  int    `json:"level"`
}

// PatternInfo is one pattern file of the project.
type PatternInfo struct {
	N      int    `json:"n"` // 1..160, A1..J16
	Label  string `json:"label"`
	Notes  int    `json:"notes"`
	Pads   int    `json:"pads"`
	Length uint32 `json:"length"`
	Bars   int    `json:"bars"`
	Bad    string `json:"bad,omitempty"`
}

// Info is the overview of the project.
type Info struct {
	Open     bool          `json:"open"`
	Name     string        `json:"name"`
	Tempo    float64       `json:"tempo"`
	Pads     []PadInfo     `json:"pads"`
	Patterns []PatternInfo `json:"patterns"`
	Files    int           `json:"files"`
}

func padFile(n int) string {
	return fmt.Sprintf("SMPL/BANK%d-%02d.SMP", n/16+1, n%16+1)
}

func padLabel(n int) string { return fmt.Sprintf("%c%d", 'A'+rune(n/16), n%16+1) }

var ptnName = regexp.MustCompile(`^PTN/PTN(\d{5})\.BIN$`)

func patFile(n int) string { return fmt.Sprintf("PTN/PTN%05d.BIN", n) }

// Info describes the project: its pads and patterns.
func (l *Lab) Info() ([]byte, error) {
	in := Info{Pads: []PadInfo{}, Patterns: []PatternInfo{}}
	if l.pad != nil {
		in.Open, in.Name, in.Files = true, l.pad.Name, len(l.files)+1
		in.Tempo = float64(l.pad.Settings[0]) / 100
		for n := 0; n < 160; n++ {
			pi := PadInfo{N: n, Label: padLabel(n)}
			if blk, err := proto.ParsePadBlock(l.pad.Blocks[n]); err == nil {
				pi.Name, pi.Has = blk.Name(), blk.HasSample()
				if v, ok := blk.Param(0x69); ok {
					pi.Level = int(v)
				}
			}
			if f, ok := l.files[padFile(n)]; ok {
				pi.File = true
				if h, err := formats.ParseHeader(f); err == nil {
					pi.Ch = h.Channels
					pi.Frames = int(h.Frames())
				}
			}
			in.Pads = append(in.Pads, pi)
		}
		for name, data := range l.files {
			m := ptnName.FindStringSubmatch(name)
			if m == nil {
				continue
			}
			var n int
			fmt.Sscanf(m[1], "%d", &n)
			if n < 1 || n > 160 {
				continue
			}
			pi := PatternInfo{N: n, Label: padLabel(n - 1)}
			if p, err := formats.ParsePattern(data); err != nil {
				pi.Bad = err.Error()
			} else {
				pads := map[uint16]bool{}
				for _, e := range p.Events {
					if !e.Control {
						pi.Notes++
						pads[e.Pad.Index()] = true
					}
				}
				pi.Pads, pi.Length = len(pads), p.Length
				pi.Bars = int(p.Length) / (p.BeatsPerBar() * formats.PPQ)
			}
			in.Patterns = append(in.Patterns, pi)
		}
		sort.Slice(in.Patterns, func(i, j int) bool { return in.Patterns[i].N < in.Patterns[j].N })
	}
	return json.Marshal(in)
}

// SetName renames the project (31 characters).
func (l *Lab) SetName(name string) error {
	if l.pad == nil {
		return errors.New("there is no project")
	}
	l.pad.Name = clean(name, 31)
	return nil
}

// SetTempo sets the project tempo in BPM (20 to 300); the banks keep their own.
func (l *Lab) SetTempo(bpm float64) error {
	if l.pad == nil {
		return errors.New("there is no project")
	}
	if bpm < 20 || bpm > 300 {
		return errors.New("the tempo is 20 to 300 BPM")
	}
	l.pad.Settings[0] = uint32(bpm*100 + 0.5)
	return nil
}
