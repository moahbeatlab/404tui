package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/generate"
	"github.com/moahbeatlab/404tui/lab"
	"github.com/moahbeatlab/404tui/proto"
)

const generateUsage = `usage: sp404 generate <project folder or zip> [options]

Writes drum patterns into a copy of a project, played with the drum kit on one bank. The project itself is not changed.

  -mode family|random|layers|drift|arc|fills
        family  (default) variations of ONE pattern: slot 1 is the groove, each slot after it changes one thing (ghost notes, the percussion,
                another hat, a polymeter, an echo, chance, a fill), then a build, a stop, a drop, a break and an outro: 16 patterns
        random  16 different patterns, each rolled on its own
        layers  the groove, then one layer more in each slot, up to the full style: 16 patterns
        drift   each pattern is a small change of the one before: 16 patterns
        fills   the groove, then the same groove with a different drum fill or roll at its end in each slot (snare rolls that speed up
                into 32nds, flams, tom and kick rolls, hat triplets, clap stacks): 16 patterns, to choose from
        arc     8 patterns along a tension curve (intro, groove, variation, build, stop, drop, break, outro)
  -style house|tech house|techno|all   the genre (default house); all makes house, tech house and techno, one bank each
  -kit A                           the bank with the drum kit, whose pad names say which pad is the kick, the claps, the hats (default A)
  -into B                          the bank the patterns start in; they take the next free slots from its first pad (default: the bank after the kit)
  -seed N                          the same seed makes the same patterns; without one a new seed is chosen and printed
  -bars 2                          the bars of each pattern (1 to 8)
  -tempo BPM                       the tempo of the project and of the banks the patterns are in (default: the style's usual one)
  -keeptempo                       leave the tempo of the project as it is (the banks the patterns are in still get theirs)
  -o folder                        where the copy goes (default: <project>-generated beside it)
  -dry                             only print the patterns, write nothing
`

func runGenerate(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Print(generateUsage)
		return nil
	}
	// the project comes first, the options after it
	src := args[0]
	fl := flag.NewFlagSet("generate", flag.ContinueOnError)
	styleName := fl.String("style", "house", "")
	kit := fl.String("kit", "A", "")
	into := fl.String("into", "", "")
	seed := fl.Int64("seed", 0, "")
	bars := fl.Int("bars", 2, "")
	tempo := fl.Float64("tempo", 0, "")
	out := fl.String("o", "", "")
	dry := fl.Bool("dry", false, "")
	mode := fl.String("mode", "family", "")
	keepTempo := fl.Bool("keeptempo", false, "")
	if err := fl.Parse(args[1:]); err != nil {
		return err
	}
	var styles []generate.Style
	if strings.EqualFold(strings.TrimSpace(*styleName), "all") {
		styles = generate.Styles()
	} else {
		st, err := generate.StyleByName(*styleName)
		if err != nil {
			return err
		}
		styles = []generate.Style{st}
	}
	kitBank, err := bankLetter(*kit)
	if err != nil {
		return fmt.Errorf("-kit: %w", err)
	}
	intoBank := kitBank + 1
	if *into != "" {
		if intoBank, err = bankLetter(*into); err != nil {
			return fmt.Errorf("-into: %w", err)
		}
	}
	if *bars < 1 || *bars > formats.MaxStartBars {
		return fmt.Errorf("-bars: 1 to %d", formats.MaxStartBars)
	}
	if *seed == 0 {
		*seed = rand.New(rand.NewSource(time.Now().UnixNano())).Int63n(1 << 30)
	}

	l, err := loadProject(src)
	if err != nil {
		return err
	}
	var info lab.Info
	raw, err := l.Info()
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &info); err != nil {
		return err
	}

	// which pad is which: from the names of the kit bank's pads
	var rows []app.PadRow
	for _, p := range info.Pads[kitBank*16 : kitBank*16+16] {
		pad, _ := proto.NewPadIndex(uint16(p.N))
		rows = append(rows, app.PadRow{Pad: pad, HasSample: p.Has, Name: p.Name})
	}
	pads := app.GuessRoles(rows)
	alts := app.GuessAlternates(rows, pads) // a second pad for a voice, played in turn with the first
	if _, ok := pads[formats.LaneKick]; !ok {
		return fmt.Errorf("bank %c has no pad named like a kick: name the pads of the kit (kick, snare, clap, hat...) or say which bank has it with -kit", 'A'+kitBank)
	}
	fmt.Printf("kit in bank %c:\n", 'A'+kitBank)
	for _, r := range rows {
		role := ""
		for lane, pad := range pads {
			if pad == r.Pad {
				role = lane.LaneName()
			}
		}
		for lane, pad := range alts {
			if pad == r.Pad {
				role = lane.LaneName() + " (2nd)"
			}
		}
		if r.HasSample || role != "" {
			fmt.Printf("  %-4s %-24s %s\n", padName(r.Pad), r.Name, role)
		}
	}
	has := func(l formats.Lane) bool { _, ok := pads[l]; return ok }

	// the slots that are taken
	taken := map[int]bool{}
	for _, p := range info.Patterns {
		taken[p.N] = true
	}
	type job struct {
		st      generate.Style
		seed    int64
		results []generate.Result
		slots   []int
	}
	var jobs []job
	for k, st := range styles {
		sd := *seed + int64(k)*1000003
		results, err := generate.Generate(*mode, st, *bars, sd, has)
		if err != nil {
			return err
		}
		// the next free slots from the first pad of the bank (each style has a bank of its own)
		var slots []int
		for n := (intoBank+k)*16 + 1; n <= 160 && len(slots) < len(results); n++ {
			if !taken[n] {
				slots = append(slots, n)
				taken[n] = true
			}
		}
		if len(slots) < len(results) {
			return fmt.Errorf("only %d free pattern slots from bank %c on, %d are needed", len(slots), 'A'+intoBank+k, len(results))
		}
		jobs = append(jobs, job{st, sd, results, slots})
	}

	for _, j := range jobs {
		fmt.Printf("\n%s, %s, seed %d (the same seed makes the same patterns)\n", j.st.Name, *mode, j.seed)
		for i, r := range j.results {
			fmt.Printf("\n%s: %s\n%s", slotName(j.slots[i]), r.Describe(), r.Pattern.Text())
		}
	}
	if *dry {
		return nil
	}

	for ji, j := range jobs {
		bpm := *tempo
		if bpm == 0 {
			bpm = float64(j.st.BPM)
		}
		for i, r := range j.results {
			b, err := r.Pattern.BytesAlt(pads, alts)
			if err != nil {
				return fmt.Errorf("%s: %w", r.Section.Name, err)
			}
			if err := l.SetPatternBytes(j.slots[i], b); err != nil {
				return err
			}
		}
		if ji == 0 && !*keepTempo {
			if err := l.SetTempo(bpm); err != nil {
				return err
			}
		}
		seen := map[int]bool{}
		for _, n := range j.slots {
			if bk := (n - 1) / 16; !seen[bk] {
				seen[bk] = true
				if err := l.SetBankTempo(bk, bpm); err != nil {
					return err
				}
			}
		}
	}
	dst := *out
	if dst == "" {
		dst = strings.TrimSuffix(strings.TrimRight(src, "/"), ".zip") + "-generated"
	}
	if err := writeProject(l, dst); err != nil {
		return err
	}
	fmt.Printf("\nwritten to %s:\n", dst)
	for _, j := range jobs {
		bpm := *tempo
		if bpm == 0 {
			bpm = float64(j.st.BPM)
		}
		fmt.Printf("  %s (%s, seed %d): %d patterns in %s to %s, %.0f BPM\n", j.st.Name, *mode, j.seed, len(j.results), slotName(j.slots[0]), slotName(j.slots[len(j.slots)-1]), bpm)
	}
	return nil
}

func bankLetter(s string) (int, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if len(s) != 1 || s[0] < 'A' || s[0] > 'J' {
		return 0, fmt.Errorf("%q is not a bank (A to J)", s)
	}
	return int(s[0] - 'A'), nil
}

func slotName(n int) string { return fmt.Sprintf("%c%d", 'A'+(n-1)/16, (n-1)%16+1) }

func padName(p proto.PadIndex) string {
	return fmt.Sprintf("%c%d", 'A'+p.Index()/16, p.Index()%16+1)
}

// loadProject reads a project folder (or a zip of one) into a lab.
func loadProject(src string) (*lab.Lab, error) {
	l := lab.New()
	st, err := os.Stat(src)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		b, err := os.ReadFile(src)
		if err != nil {
			return nil, err
		}
		if _, err := l.LoadZip(b); err != nil {
			return nil, err
		}
		return l, nil
	}
	l.Begin()
	err = filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		l.Add(filepath.ToSlash(rel), b)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if _, err := l.Finish(); err != nil {
		return nil, err
	}
	return l, nil
}

// writeProject writes the lab's project as a folder (PADCONF.BIN, PTN, SMPL, PICTURE).
func writeProject(l *lab.Lab, dst string) error {
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("%s exists already: give another folder with -o (nothing is overwritten)", dst)
	}
	return l.ExportEach(func(name string, data []byte) error {
		p := filepath.Join(dst, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		return os.WriteFile(p, data, 0o644)
	})
}
