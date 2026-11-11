package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/lab"
)

// a project with a drum kit in bank A (the pads are named like a kit), written as a folder
func kitProject(t *testing.T) string {
	l := lab.New()
	l.NewProject("KIT")
	tone := formats.Sample{Channels: 1, Samples: make([]int16, 2400)}
	for i, name := range []string{"909_Kick", "909_Snare", "909_Clap", "909_CHat", "909_OHat", "909_Perc", "909_Rim", "909_Tom"} {
		var m struct{ ID int }
		raw, err := l.Decode(tone.WAV(), ".wav")
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		if err := l.Assign(i, m.ID, name); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(t.TempDir(), "KIT")
	if err := writeProject(l, dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

// sp404 generate writes the arrangement into a copy: the project is not changed, the copy has the patterns in the bank after the kit, and
// they play the kit's pads.
func TestGenerateWritesPatternsIntoACopy(t *testing.T) {
	src := kitProject(t)
	out := filepath.Join(t.TempDir(), "copy")
	if err := runGenerate([]string{src, "-style", "techno", "-mode", "arc", "-seed", "3", "-o", out}); err != nil {
		t.Fatal(err)
	}
	if ents, _ := os.ReadDir(filepath.Join(src, "PTN")); len(ents) != 0 {
		t.Errorf("the project was changed: %d patterns", len(ents))
	}
	l, err := loadProject(out)
	if err != nil {
		t.Fatal(err)
	}
	var info lab.Info
	raw, _ := l.Info()
	if err := json.Unmarshal(raw, &info); err != nil {
		t.Fatal(err)
	}
	if len(info.Patterns) != 8 || info.Patterns[0].N != 17 || info.Patterns[7].N != 24 { // B1..B8
		t.Fatalf("patterns: %+v", info.Patterns)
	}
	if info.Patterns[1].Bars != 2 || info.Patterns[4].Bars != 1 { // the groove is two bars, the stop one
		t.Errorf("bars: %+v", info.Patterns)
	}
	b, _ := l.PatternBytes(18)
	p, err := formats.ParsePattern(b)
	if err != nil || len(p.Events) == 0 {
		t.Fatalf("the groove: %v, %d events", err, len(p.Events))
	}
	for _, e := range p.Events {
		if e.Pad.Index() > 15 {
			t.Fatalf("a note on pad %d: the patterns play the kit in bank A", e.Pad.Index())
		}
	}
	if err := runGenerate([]string{src, "-o", out}); err == nil {
		t.Error("an existing folder was overwritten")
	}
	if err := runGenerate([]string{src, "-kit", "C", "-dry"}); err == nil {
		t.Error("a bank with no kit was accepted")
	}
}

// the default is a family: sixteen patterns that are variations of the first; -style all fills three banks, one for each genre
func TestGenerateFamilyAndAllStyles(t *testing.T) {
	src := kitProject(t)
	out := filepath.Join(t.TempDir(), "copy")
	if err := runGenerate([]string{src, "-style", "all", "-seed", "9", "-o", out}); err != nil {
		t.Fatal(err)
	}
	l, err := loadProject(out)
	if err != nil {
		t.Fatal(err)
	}
	var info lab.Info
	raw, _ := l.Info()
	if err := json.Unmarshal(raw, &info); err != nil {
		t.Fatal(err)
	}
	if len(info.Patterns) != 9*16 || info.Patterns[0].N != 17 || info.Patterns[9*16-1].N != 160 { // B1..J16: a bank for each of the nine styles
		t.Fatalf("%d patterns, first %d, last %d", len(info.Patterns), info.Patterns[0].N, info.Patterns[len(info.Patterns)-1].N)
	}
	if err := runGenerate([]string{src, "-mode", "nope", "-dry"}); err == nil {
		t.Error("an unknown mode was accepted")
	}
}
