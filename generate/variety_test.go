package generate

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/moahbeatlab/404tui/formats"
)

// distance is how many steps differ between two patterns (over all lanes), as a share of the steps that have a hit in either.
func distance(a, b Pattern) float64 {
	diff, any := 0, 0
	for _, l := range []formats.Lane{formats.LaneKick, formats.LaneClosedHat, formats.LaneOpenHat, formats.LaneClap, formats.LaneRim,
		formats.LanePerc, formats.LaneShaker, formats.LaneRide, formats.LaneTom, formats.LaneSnare} {
		ra, rb := a.Row(l), b.Row(l)
		n := max(a.Bars, b.Bars) * 16
		for i := 0; i < n; i++ {
			var x, y uint8
			if ra != nil && i < len(ra.Steps) {
				x = ra.Steps[i]
			}
			if rb != nil && i < len(rb.Steps) {
				y = rb.Steps[i]
			}
			if x > 0 || y > 0 {
				any++
			}
			if (x > 0) != (y > 0) {
				diff++
			}
		}
	}
	if any == 0 {
		return 0
	}
	return float64(diff) / float64(any)
}

// Do different seeds make different patterns? For each style and each part of the arc: how many of 200 seeds give a pattern of its own,
// how far two seeds' patterns are apart on average, and which fills and hat patterns come up.
func TestSeedsMakeDifferentPatterns(t *testing.T) {
	const seeds = 200
	for _, st := range Styles() {
		perSection := map[string][]Pattern{}
		fills, hats := map[string]int{}, map[string]int{}
		for seed := int64(1); seed <= seeds; seed++ {
			for _, r := range Arrange(st, DefaultArc(2), seed, hasAll) {
				perSection[r.Section.Name] = append(perSection[r.Section.Name], r.Pattern)
				for _, part := range strings.Split(r.Notes, "; ") {
					switch {
					case strings.HasPrefix(part, "fill: "):
						fills[strings.TrimPrefix(part, "fill: ")]++
					case strings.HasPrefix(part, "hats: "):
						hats[strings.TrimPrefix(part, "hats: ")]++
					}
				}
			}
		}
		var report strings.Builder
		fmt.Fprintf(&report, "\n%s, %d seeds\n", st.Name, seeds)
		for _, s := range DefaultArc(2) {
			ps := perSection[s.Name]
			uniq := map[string]bool{}
			for _, p := range ps {
				uniq[p.Text()] = true
			}
			sum, n := 0.0, 0
			for i := 0; i+1 < len(ps); i += 2 {
				sum += distance(ps[i], ps[i+1])
				n++
			}
			fmt.Fprintf(&report, "  %-10s %3d different of %d, two seeds differ in %2.0f%% of the steps in use\n", s.Name, len(uniq), seeds, 100*sum/float64(n))
			want := seeds / 2
			if s.Kind == "intro" || s.Kind == "outro" || s.Kind == "stop" {
				want = 20
			}
			{
				if len(uniq) < want {
					t.Errorf("%s %s: only %d different patterns in %d seeds", st.Name, s.Name, len(uniq), seeds)
				}
			}
		}
		for _, m := range []map[string]int{fills, hats} {
			var keys []string
			for k := range m {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Fprintf(&report, "    %-16s %3d\n", k, m[k])
			}
			report.WriteString("\n")
		}
		t.Log(report.String())
	}
}
