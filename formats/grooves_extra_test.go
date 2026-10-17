package formats

import (
	"testing"

	"github.com/moahbeatlab/404tui/proto"
)

func TestEveryGrooveParsesAndBuilds(t *testing.T) {
	for _, g := range GrooveTemplates() {
		pads := map[Lane]proto.PadIndex{}
		for i, l := range g.Lanes {
			pad, _ := proto.NewPadIndex(uint16(i))
			pads[l] = pad
		}
		if len(g.FirstBars(2)) != len(g.Lanes) {
			t.Errorf("%s: %d lanes parse of %d", g.Name, len(g.FirstBars(2)), len(g.Lanes))
		}
		if _, err := g.BuildGroove(pads, 0); err != nil {
			t.Errorf("%s: %v", g.Name, err)
		}
	}
}
