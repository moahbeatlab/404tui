package tui

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The page without a server (web/static/engine.js) has the effects and presets as data: fxdata.js, made from the tables here.
func TestFxDataJSMatchesTheTables(t *testing.T) {
	raw, err := os.ReadFile("../web/static/fxdata.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	text = text[strings.Index(text, "window.SP404_FX = ")+len("window.SP404_FX = "):]
	text = strings.TrimSuffix(strings.TrimSpace(text), ";")
	var data struct {
		Fx12    []string                           `json:"fx12"`
		Fx34    []string                           `json:"fx34"`
		Params  map[string][]struct{ N, K string } `json:"params"`
		Presets []struct {
			Name  string `json:"name"`
			Buses [4]struct {
				Fx string `json:"fx"`
				K  [6]int `json:"k"`
			} `json:"buses"`
		} `json:"presets"`
	}
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		t.Fatal(err)
	}
	if strings.Join(data.Fx12, "|") != strings.Join(fxNames, "|") || strings.Join(data.Fx34, "|") != strings.Join(fxNames34, "|") {
		t.Error("fxdata.js has other effects than fxnames.go: make it again")
	}
	if len(data.Params) != len(fxParams) {
		t.Errorf("fxdata.js has %d effects with knobs, fxparams.go has %d", len(data.Params), len(fxParams))
	}
	for name, ps := range fxParams {
		for k, p := range ps {
			if got := data.Params[name]; len(got) != 6 || got[k].N != p.name || got[k].K != p.kind {
				t.Errorf("fxdata.js differs from fxparams.go at %s knob %d", name, k+1)
			}
		}
	}
	if len(data.Presets) != len(punchPresets) {
		t.Fatalf("fxdata.js has %d presets, punchpresets.go %d", len(data.Presets), len(punchPresets))
	}
	for i, p := range punchPresets {
		for b, f := range p.bus {
			if g := data.Presets[i].Buses[b]; g.Fx != f.fx || g.K != f.k {
				t.Errorf("preset %s bus %d differs in fxdata.js", p.name, b+1)
			}
		}
	}
}
