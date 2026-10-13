// Package doctor checks, without changing anything, that this program's reading of a unit
// agrees with what the unit says. Each check compares something we parse or compute with
// the unit's own answer, so a failure points at a place where the implementation and the
// real device part ways.
package doctor

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/device"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// Unit is what the checks read from.
type Unit interface {
	app.Unit
	PortName() string
}

// Result is the outcome of one check.
type Result struct {
	Name    string
	Detail  string
	Err     error
	Skipped bool
	Took    time.Duration
}

// OK reports whether the check passed (a skipped one did not fail).
func (r Result) OK() bool { return r.Err == nil }

type check struct {
	name string
	run  func() (detail string, err error)
}

// Options select what is checked.
type Options struct {
	// Preview also plays pad 1 on the unit for a moment: the only check that makes a sound.
	Preview bool
}

// Run goes through the checks, printing each as it finishes, and returns the results.
func Run(u Unit, w io.Writer, log *slog.Logger, opt Options) []Result {
	fmt.Fprintf(w, "404tui doctor: %s/%s, %s, unit on %s\n", runtime.GOOS, runtime.GOARCH, runtime.Version(), u.PortName())
	fmt.Fprintln(w, "Read-only: nothing on the unit is changed.")
	fmt.Fprintln(w)

	var (
		status   *device.Status
		blocks   []*proto.PadBlock
		project  uint8
		projectN uint8 // 1-based
	)
	checks := []check{
		{name: "status poll", run: func() (string, error) {
			st, err := u.Status()
			if err != nil {
				return "", err
			}
			status, project, projectN = st, st.Project, st.Project+1
			sel := "no pad selected"
			if st.HasSelected {
				sel = "pad " + st.Selected.String() + " selected"
			}
			return fmt.Sprintf("project %d (%q), %s", projectN, st.Settings.Name(), sel), nil
		}},
		{name: "project settings", run: func() (string, error) {
			if status == nil {
				return "", errSkip("no status")
			}
			s := status.Settings
			var bad []string
			for b := 0; b < proto.Banks; b++ {
				bs := s.Bank(b)
				if t := float64(bs.Tempo) / 100; t < 40 || t > 200 {
					bad = append(bad, fmt.Sprintf("bank %c tempo %.2f", 'A'+b, t))
				}
			}
			if len(bad) > 0 {
				return "", fmt.Errorf("values outside the documented ranges: %s", strings.Join(bad, ", "))
			}
			mode := "bank tempo"
			if s.UsesProjectTempo() {
				mode = "project tempo"
			}
			return fmt.Sprintf("project tempo %.2f BPM, %s in use; bank A %.2f BPM, volume %d; bank J %.2f BPM, volume %d",
				float64(s.ProjectTempo())/100, mode, float64(s.Bank(0).Tempo)/100, s.Bank(0).Volume,
				float64(s.Bank(9).Tempo)/100, s.Bank(9).Volume), nil
		}},
		{name: "project names", run: func() (string, error) {
			names, err := u.ProjectNames()
			if err != nil {
				return "", err
			}
			named := 0
			for _, n := range names {
				if n != "" {
					named++
				}
			}
			if len(names) != device.Projects {
				return "", fmt.Errorf("got %d names, expected %d", len(names), device.Projects)
			}
			return fmt.Sprintf("%d slots, %d named; current is %q", len(names), named, names[project]), nil
		}},
		{name: "pad blocks (160)", run: func() (string, error) {
			var err error
			if blocks, err = u.PadBlocks(); err != nil {
				return "", err
			}
			samples, named := 0, 0
			for i, b := range blocks {
				if int(b.Pad.Index()) != i {
					return "", fmt.Errorf("block %d answers for pad %s", i, b.Pad)
				}
				if b.HasSample() {
					samples++
					if b.Name() != "" {
						named++
					}
				}
			}
			return fmt.Sprintf("%d blocks in order; %d pads hold a sample, %d of them named", len(blocks), samples, named), nil
		}},
		{name: "sample files match the pad blocks", run: func() (string, error) {
			if blocks == nil {
				return "", errSkip("no pad blocks")
			}
			checked, bad := 0, []string{}
			for _, b := range blocks {
				if !b.HasSample() {
					continue
				}
				path := b.Pad.SamplePath(projectN)
				st, exists, err := u.Stat(path)
				if err != nil {
					return "", err
				}
				if !exists {
					bad = append(bad, fmt.Sprintf("%s: %s is missing", b.Pad, path))
					continue
				}
				head, _, err := u.ReadPrefix(path, formats.HeaderLen)
				if err != nil {
					return "", err
				}
				info, err := formats.ParseHeader(head)
				if err != nil {
					bad = append(bad, fmt.Sprintf("%s: %v", b.Pad, err))
					continue
				}
				checked++
				switch {
				case st.Size != b.FileSize():
					bad = append(bad, fmt.Sprintf("%s: the file is %d bytes, the pad block says %d", b.Pad, st.Size, b.FileSize()))
				case formats.HeaderLen+info.PCMLen != st.Size:
					bad = append(bad, fmt.Sprintf("%s: the header says %d bytes of audio, the file has %d", b.Pad, info.PCMLen, st.Size-formats.HeaderLen))
				}
			}
			if len(bad) > 0 {
				return "", fmt.Errorf("%d of %d samples disagree: %s", len(bad), checked+len(bad), strings.Join(bad[:min(len(bad), 4)], "; "))
			}
			return fmt.Sprintf("%d sample files: size, header and path all agree with the pad blocks", checked), nil
		}},
		{name: "file API: the unit's storage", run: func() (string, error) {
			entries, err := u.ListDir("ROLAND/SP-404MKII")
			if err != nil {
				return "", err
			}
			want := fmt.Sprintf("PROJECT_%02d", projectN)
			var names []string
			for _, e := range entries {
				names = append(names, e.Name)
			}
			if projectN != 0 && !slices.Contains(names, want) {
				return "", fmt.Errorf("ROLAND/SP-404MKII has %d entries but not %s", len(names), want)
			}
			free, err := u.FreeKb()
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%d entries, %s present, %.1f GB free", len(names), want, float64(free)/1024/1024), nil
		}},
		{name: "file API: the SD card", run: func() (string, error) {
			entries, err := u.ListDir(app.CardRoot)
			if err != nil {
				return "", errSkip("no readable card: " + err.Error())
			}
			var names []string
			for _, e := range entries {
				names = append(names, e.Name)
			}
			return fmt.Sprintf("%d entries: %s", len(names), strings.Join(names[:min(len(names), 8)], ", ")), nil
		}},
		{name: "PADCONF.BIN", run: func() (string, error) {
			path := fmt.Sprintf("ROLAND/SP-404MKII/PROJECT_%02d/PADCONF.BIN", projectN)
			head, exists, err := u.ReadPrefix(path, 0xA0)
			if err != nil {
				return "", err
			}
			if !exists {
				return "", errSkip("the project has no PADCONF.BIN (an empty slot)")
			}
			if !bytes.HasPrefix(head, []byte("RFPD")) {
				return "", fmt.Errorf("%s does not start with RFPD: % x", path, head[:min(len(head), 8)])
			}
			version := head[8]
			detail := fmt.Sprintf("magic RFPD, version %d", version)
			if version >= 3 && len(head) >= 0xA0 && status != nil {
				name := strings.Trim(string(head[0x80:0xA0]), "\x00 ")
				if name != status.Settings.Name() {
					return "", fmt.Errorf("the file names the project %q, the unit says %q (the file is a snapshot and can lag)", name, status.Settings.Name())
				}
				detail += fmt.Sprintf(", name %q matches", name)
			}
			return detail, nil
		}},
		{name: "patterns: our parser against the unit", run: func() (string, error) {
			var slots []proto.PadIndex
			for _, slot := range proto.AllPads() {
				exists, err := u.PatternExists(slot)
				if err != nil {
					return "", err
				}
				if exists {
					slots = append(slots, slot)
				}
			}
			if len(slots) == 0 {
				return "", errSkip("this project has no patterns")
			}
			var bad []string
			checked := 0
			for _, slot := range slots[:min(len(slots), 6)] {
				fromUnit, err := u.PatternPads(slot)
				if err != nil {
					return "", err
				}
				detail, err := app.OpenPattern(u, slot)
				if err != nil {
					bad = append(bad, fmt.Sprintf("%s: %v", slot, err))
					continue
				}
				checked++
				slices.SortFunc(fromUnit, func(a, b proto.PadIndex) int { return int(a.Index()) - int(b.Index()) })
				if !slices.Equal(fromUnit, detail.PadsUsed) {
					bad = append(bad, fmt.Sprintf("%s: the unit lists pads %v, our parser reads %v", slot, fromUnit, detail.PadsUsed))
				}
			}
			if len(bad) > 0 {
				return "", fmt.Errorf("%s", strings.Join(bad, "; "))
			}
			return fmt.Sprintf("%d patterns in the project; %d read: the pads our parser finds match the unit's own list", len(slots), checked), nil
		}},
		{name: "unsolicited messages", run: func() (string, error) {
			events := u.TakeEvents()
			if len(events) == 0 {
				return "none left over", nil
			}
			// What the unit sends along with answers, or on its own, and we know about: the
			// project names that follow every settings reply, the settings reply itself when
			// a names request asked for it, change notifications, and status polls.
			known := map[string]string{
				"ch05 cmd 35": "project names after settings replies",
				"ch05 cmd 7d": "settings replies",
				"ch05 cmd 1a": "change notifications",
				"ch05 cmd 7e": "status replies",
			}
			counts, unexpected := map[string]int{}, map[string]int{}
			for i := range events {
				p := events[i].Payload()
				key := fmt.Sprintf("ch%02x", byte(events[i].Channel))
				if len(p) > 0 {
					key += fmt.Sprintf(" cmd %02x", p[0])
				}
				if _, ok := known[key]; ok {
					counts[key]++
				} else {
					unexpected[key]++
				}
			}
			describe := func(m map[string]int, names map[string]string) string {
				var parts []string
				for k, n := range m {
					label := k
					if names != nil {
						label = names[k]
					}
					parts = append(parts, fmt.Sprintf("%s ×%d", label, n))
				}
				slices.Sort(parts)
				return strings.Join(parts, ", ")
			}
			detail := fmt.Sprintf("%d expected (%s)", len(events)-sum(unexpected), describe(counts, known))
			if len(unexpected) > 0 {
				detail += fmt.Sprintf("; %d NOT recognised: %s — send the -trace log", sum(unexpected), describe(unexpected, nil))
			}
			return detail, nil
		}},
	}
	if opt.Preview {
		checks = append(checks, check{name: "preview on the unit (makes a sound)", run: func() (string, error) {
			first := -1
			for i, b := range blocks {
				if b.HasSample() {
					first = i
					break
				}
			}
			if first < 0 {
				return "", errSkip("no pad holds a sample")
			}
			pad := blocks[first].Pad
			if err := u.Preview(pad, 700*time.Millisecond); err != nil {
				return "", err
			}
			return fmt.Sprintf("played %s for 0.7 s; you should have heard it from the unit", pad), nil
		}})
	}

	var results []Result
	for _, c := range checks {
		start := time.Now()
		detail, err := c.run()
		r := Result{Name: c.name, Detail: detail, Took: time.Since(start).Round(time.Millisecond)}
		if s, ok := err.(skipError); ok {
			r.Skipped, r.Detail = true, string(s)
		} else {
			r.Err = err
		}
		results = append(results, r)
		log.Info("doctor check", "name", r.Name, "ok", r.OK(), "skipped", r.Skipped, "detail", r.Detail, "err", r.Err, "took", r.Took)
		switch {
		case r.Skipped:
			fmt.Fprintf(w, "[skip] %-38s %s\n", r.Name, r.Detail)
		case r.Err != nil:
			fmt.Fprintf(w, "[FAIL] %-38s %v  (%s)\n", r.Name, r.Err, r.Took)
		default:
			fmt.Fprintf(w, "[ ok ] %-38s %s  (%s)\n", r.Name, r.Detail, r.Took)
		}
	}
	failed := Failures(results)
	fmt.Fprintln(w)
	if failed == 0 {
		fmt.Fprintln(w, "All checks passed.")
	} else {
		fmt.Fprintf(w, "%d check(s) failed. Run again with -v -log doctor.log (and -trace for the wire bytes) and send the log.\n", failed)
	}
	return results
}

// Failures counts the checks that failed.
func Failures(results []Result) int {
	n := 0
	for _, r := range results {
		if !r.OK() {
			n++
		}
	}
	return n
}

func sum(m map[string]int) int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}

type skipError string

func (s skipError) Error() string { return string(s) }

func errSkip(why string) error { return skipError(why) }
