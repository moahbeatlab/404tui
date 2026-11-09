package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moahbeatlab/404tui/core"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/web"
)

func toolsServer(t *testing.T, key string) (*httptest.Server, *core.Remote) {
	t.Helper()
	dir := t.TempDir()
	samples := filepath.Join(dir, "samples")
	if err := core.WriteDemo(samples); err != nil {
		t.Fatal(err)
	}
	o := options{samples: samples, out: filepath.Join(dir, "out")}
	remote := newToolsApp(o, &sharedUnit{}, nil)
	t.Cleanup(func() { core.Terminal = false })
	srv := &web.Server{Key: key}
	srv.Mount("/tools/api", toolsAPI{remote})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, remote
}

func post(t *testing.T, ts *httptest.Server, key, body string) core.RState {
	t.Helper()
	url := ts.URL + "/tools/api"
	if key != "" {
		url += "?key=" + key
	}
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%s: %d", body, resp.StatusCode)
	}
	var st core.RState
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestTheToolsPageTalksToTheProgram(t *testing.T) {
	ts, _ := toolsServer(t, "")
	st := post(t, ts, "", `{"op":"state"}`)
	if st.Menu != nil || st.Pattern.Slot != 1 {
		t.Fatalf("at first: %+v", st)
	}
	st = post(t, ts, "", `{"op":"open"}`)
	if st.Menu == nil || len(st.Menu.Tabs) < 4 {
		t.Fatalf("the pattern tools have pages: %+v", st.Menu)
	}
	// every page, setting and value is there, as in the terminal
	st = post(t, ts, "", `{"op":"tab","tab":2}`)
	var fill int = -1
	for _, it := range st.Menu.Items {
		if strings.HasPrefix(it.Label, "Fill") {
			fill = it.Index
		}
	}
	if fill < 0 {
		t.Fatalf("Fill is on the Notes page: %+v", st.Menu.Items)
	}
	st = post(t, ts, "", `{"op":"run","i":`+itoa(fill)+`}`)
	if st.Menu == nil || !strings.HasPrefix(strings.ToUpper(st.Menu.Title), "FILL") {
		t.Fatalf("Fill's dialog: %+v", st.Menu)
	}
	var choices int
	for _, it := range st.Menu.Items {
		if it.Kind == "choice" {
			choices++
			if len(it.Vals) < 2 {
				t.Fatalf("a choice lists its values: %+v", it)
			}
		}
	}
	if choices < 3 {
		t.Fatalf("Fill has its settings: %d choices", choices)
	}
	if len(st.Pattern.Tracks) == 0 {
		t.Fatal("the trial is in the pattern")
	}
	if st.Msg == "" {
		t.Fatal("the trial says what it did")
	}
	st = post(t, ts, "", `{"op":"back"}`)
	st = post(t, ts, "", `{"op":"back"}`)
	if st.Menu != nil {
		t.Fatal("closed")
	}
	if len(st.Pattern.Tracks) != 0 {
		t.Fatalf("the trial was thrown away: %+v", st.Pattern.Tracks)
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func TestTheToolsRouteNeedsTheKey(t *testing.T) {
	ts, _ := toolsServer(t, "secret")
	resp, err := http.Post(ts.URL+"/tools/api", "application/json", strings.NewReader(`{"op":"state"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("without the key: %d", resp.StatusCode)
	}
	if st := post(t, ts, "secret", `{"op":"state"}`); st.Pattern.Slot != 1 {
		t.Fatalf("with it: %+v", st)
	}
	resp, _ = http.Post(ts.URL+"/tools/api?key=secret", "application/json", strings.NewReader(`{"op":"nothing"}`))
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("an unknown operation: %d", resp.StatusCode)
	}
}

func TestTheToolsPageSlicesAndOpensPadTools(t *testing.T) {
	ts, remote := toolsServer(t, "")
	remote.Do(func(a *core.App) {
		if err := a.Script("A A D R D D A  R A A D A  R A A U U U A", nil); err != nil { // a sample on a pad, the way a user does it
			t.Fatal(err)
		}
	})
	st := post(t, ts, "", `{"op":"state"}`)
	// the demo pads: find one with a sample
	resp, err := http.Post(ts.URL+"/tools/api", "application/json", strings.NewReader(`{"op":"pads"}`))
	if err != nil {
		t.Fatal(err)
	}
	var pads []core.RPad
	json.NewDecoder(resp.Body).Decode(&pads)
	resp.Body.Close()
	pad := -1
	for _, p := range pads {
		if p.Has {
			pad = p.Pad
			break
		}
	}
	if pad < 0 {
		t.Skip("the demo project has no pad with a sample")
	}
	st = post(t, ts, "", `{"op":"slicer","pad":`+itoa(pad)+`}`)
	if st.Screen != "slicer" || st.Slicer == nil || len(st.Slicer.Peaks) == 0 {
		t.Fatalf("the slicer: %+v", st.Slicer)
	}
	st = post(t, ts, "", `{"op":"slicer-cut","i":`+itoa(st.Slicer.Frames/2)+`}`)
	if len(st.Slicer.Slices) != 2 {
		t.Fatalf("a cut: %d slices", len(st.Slicer.Slices))
	}
	st = post(t, ts, "", `{"op":"slicer-menu","kind":"tools"}`)
	if st.Menu == nil {
		t.Fatal("the slicer's tools are a menu")
	}
	st = post(t, ts, "", `{"op":"escape"}`)
	st = post(t, ts, "", `{"op":"slicer-close"}`)
	st = post(t, ts, "", `{"op":"pad-tools","pad":`+itoa(pad)+`}`)
	if st.Menu == nil {
		t.Fatal("the tools of a pad are a menu")
	}
	post(t, ts, "", `{"op":"escape"}`)
}

// A MIDI file sent from the page opens the form of the program; the form's last entry writes the pattern, and the pattern comes back as a file.
func TestTheToolsPageImportsAndExportsMIDI(t *testing.T) {
	ts, _ := toolsServer(t, "")
	notes := []formats.MIDINote{{Tick: 0, Len: 240, Note: 36, Vel: 100}, {Tick: 480, Len: 240, Note: 43, Vel: 90}, {Tick: 960, Len: 240, Note: 48, Vel: 80}}
	file := formats.WriteMIDI([]formats.MIDIOut{{Name: "bass", Notes: notes}}, 120, 4, 4*formats.PPQ)
	b64 := base64.StdEncoding.EncodeToString(file)
	st := post(t, ts, "", `{"op":"midi-open","text":"bass.mid","data":"`+b64+`"}`)
	if st.Menu == nil || !strings.HasPrefix(st.Menu.Title, "MIDI file") {
		t.Fatalf("no form: %+v (%s)", st.Menu, st.Msg)
	}
	last := st.Menu.Items[len(st.Menu.Items)-1]
	st = post(t, ts, "", fmt.Sprintf(`{"op":"run","i":%d}`, last.Index))
	if st.Menu != nil || !strings.Contains(st.Msg, "3 notes") {
		t.Fatalf("after the import: menu %v, %q", st.Menu, st.Msg)
	}
	resp, err := http.Post(ts.URL+"/tools/api", "application/json", strings.NewReader(fmt.Sprintf(`{"op":"midi-export","slot":%d}`, st.Pattern.Slot)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct{ Name, Data string }
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.Name == "" {
		t.Fatalf("export: %v %+v", err, out)
	}
	raw, err := base64.StdEncoding.DecodeString(out.Data)
	if err != nil {
		t.Fatal(err)
	}
	g, err := formats.ParseMIDI(raw)
	if err != nil || len(g.Parts) != 1 || len(g.Parts[0].Notes) != 3 {
		t.Fatalf("the file that came back: %v %+v", err, g.Parts)
	}
	// a damaged upload is refused, not a crash
	r2, err := http.Post(ts.URL+"/tools/api", "application/json", strings.NewReader(`{"op":"midi-open","text":"x.mid","data":"!!!"}`))
	if err != nil {
		t.Fatal(err)
	}
	r2.Body.Close()
	if r2.StatusCode != 400 {
		t.Fatalf("a broken upload gave %d", r2.StatusCode)
	}
}
