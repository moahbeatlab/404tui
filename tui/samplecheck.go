package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/app"
)

// checkSamples reads the sample of the pad in front (or of each marked pad) from the unit and
// compares it with the file it came from, converted as it is for the unit: the same samples, or
// where they differ and by how much. The file is the one noted when it was assigned, or found by
// a scan (F, W).
func (m Model) checkSamples() (tea.Model, tea.Cmd) {
	rows := m.targets()
	var with []app.PadRow
	for _, r := range rows {
		if r.HasSample {
			with = append(with, r)
		}
	}
	if len(with) == 0 {
		m.message = "no sample there to check"
		return m, nil
	}
	unit, project, origins := m.unit, m.snap.Project, m.origins
	if origins == nil {
		loaded, err := app.LoadOrigins(unit)
		if err != nil {
			m.message = "reading where the samples came from failed: " + err.Error()
			return m, nil
		}
		origins = loaded
	}
	m.busy = "checking " + plural(len(with), "sample")
	ops := m.ops
	return m, func() tea.Msg {
		ops.Lock()
		defer ops.Unlock()
		var lines []string
		for _, row := range with {
			head := fmt.Sprintf("%-4s %s", row.Pad, row.Name)
			origin, ok := origins.Of(project, row.Pad, row.Name)
			if !ok {
				lines = append(lines, head, "     no file is noted for it: F or W in Pads looks for it", "")
				continue
			}
			lines = append(lines, head, "     file  "+display(origin.Path))
			want, err := app.LoadSample(unit, origin.Path)
			if err != nil {
				lines = append(lines, "     the file could not be read: "+err.Error(), "")
				continue
			}
			got, err := app.StoredSample(unit, row)
			if err != nil {
				lines = append(lines, "     the pad could not be read: "+err.Error(), "")
				continue
			}
			check := app.CompareSamples(want.Sample, got)
			lines = append(lines, "     "+check.Verdict())
			if want.SourceRate != 48000 {
				lines = append(lines, fmt.Sprintf("     the file is %d Hz and was converted to 48 kHz", want.SourceRate))
			}
			lines = append(lines, "")
		}
		return checkMsg{lines: lines}
	}
}

func (m Model) reportBox() string {
	st, r := m.st, m.report
	height := max(m.height-12, 6)
	lines := append([]string{st.bold.Render(r.title), ""}, r.lines...)
	if len(lines) > height {
		lines = append(lines[:height-1], st.dim.Render(fmt.Sprintf("… %d more lines (check fewer pads at once)", len(lines)-height+1)))
	}
	lines = append(lines, "", st.dim.Render("any key closes"))
	return st.modal.Width(min(m.width-6, 100)).Render(strings.Join(lines, "\n"))
}
