package replay

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moahbeatlab/404tui/perflog"
	"time"
)

func writeWAV(t *testing.T, path string, bits int, secs float64, amp float64) {
	t.Helper()
	rate := 48000
	n := int(secs * float64(rate))
	bytesPer := bits / 8
	data := make([]byte, 0, n*2*bytesPer)
	for i := 0; i < n; i++ {
		v := amp * math.Sin(2*math.Pi*1000*float64(i)/float64(rate))
		for c := 0; c < 2; c++ {
			switch bits {
			case 16:
				data = binary.LittleEndian.AppendUint16(data, uint16(int16(v*32767)))
			case 24:
				x := int32(v * 8388607)
				data = append(data, byte(x), byte(x>>8), byte(x>>16))
			}
		}
	}
	h := make([]byte, 44)
	copy(h, "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+len(data)))
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1)
	binary.LittleEndian.PutUint16(h[22:], 2)
	binary.LittleEndian.PutUint32(h[24:], uint32(rate))
	binary.LittleEndian.PutUint32(h[28:], uint32(rate*2*bytesPer))
	binary.LittleEndian.PutUint16(h[32:], uint16(2*bytesPer))
	binary.LittleEndian.PutUint16(h[34:], uint16(bits))
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(len(data)))
	if err := os.WriteFile(path, append(h, data...), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAnalysisOfA24AndA16BitWAV(t *testing.T) {
	dir := t.TempDir()
	for _, bits := range []int{16, 24} {
		p := filepath.Join(dir, "t.wav")
		writeWAV(t, p, bits, 2, 0.5)
		a, err := Analyze(p)
		if err != nil {
			t.Fatal(err)
		}
		if a.Frames < 58 || a.Frames > 61 || a.Duration < 1.99 || a.Duration > 2.01 {
			t.Fatalf("%d bit: two seconds at 30 a second: %d frames, %.2f s", bits, a.Frames, a.Duration)
		}
		l := binary.LittleEndian.Uint16(a.Peak[40:])
		if l < 31000 || l > 34500 {
			t.Fatalf("%d bit: the peak is half of full scale: %d", bits, l)
		}
		// 1 kHz is in the band that holds it, and loud there
		loud, at := byte(0), 0
		for i := 0; i < a.Bands; i++ {
			if v := a.Spec[20*a.Bands+i]; v > loud {
				loud, at = v, i
			}
		}
		if loud < 150 || at < 20 || at > 40 {
			t.Fatalf("%d bit: the tone is in band %d at %d/255", bits, at, loud)
		}
	}
}

func TestReplayServesThePageTheEventsAndTheAnalysis(t *testing.T) {
	dir := t.TempDir()
	wav := filepath.Join(dir, "take.wav")
	writeWAV(t, wav, 24, 1, 0.3)
	start := time.Now()
	l, err := perflog.Create(perflog.PathFor(wav), wav, start)
	if err != nil {
		t.Fatal(err)
	}
	l.Add(start.Add(100*time.Millisecond), "on", perflog.P("bank", "A"), perflog.P("pad", 1))
	l.Close()
	r, err := Load(wav, "")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(r.Handler())
	defer srv.Close()
	get := func(path string) (int, []byte) {
		resp, err := srv.Client().Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, b
	}
	if code, b := get("/"); code != 200 || len(b) < 1000 {
		t.Fatalf("the page: %d %d bytes", code, len(b))
	}
	if code, b := get("/events.json"); code != 200 {
		t.Fatal("events")
	} else {
		var ev struct {
			File   string
			Events []map[string]any
		}
		if json.Unmarshal(b, &ev) != nil || ev.File != "take.wav" || len(ev.Events) != 3 { // start, on, end? (start, on)
			if len(ev.Events) < 2 {
				t.Fatalf("the events: %s", b)
			}
		}
	}
	if code, b := get("/analysis.json"); code != 200 || len(b) < 100 {
		t.Fatal("analysis")
	}
	if code, _ := get("/fxdata.js"); code != 200 {
		t.Fatal("the names of the effects")
	}
	if code, b := get("/audio"); code != 200 || len(b) < 100000 {
		t.Fatalf("the audio: %d %d", code, len(b))
	}
	if _, err := Load(filepath.Join(dir, "none.wav"), ""); err == nil {
		t.Fatal("a missing file is an error")
	}
}

func TestCuesAreKept(t *testing.T) {
	r := &Replay{CuesPath: filepath.Join(t.TempDir(), "x.cues.json")}
	if err := r.SetCues([]Cue{{T: 6.3, Label: "A2"}}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(r.CuesPath)
	if !strings.Contains(string(b), "A2") {
		t.Fatalf("not kept: %s", b)
	}
	if err := r.SetCues(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(r.CuesPath); err == nil {
		t.Fatal("empty cues should remove the file")
	}
	if CuesPathFor("/a/b.events.jsonl") != "/a/b.events.cues.json" {
		t.Fatal(CuesPathFor("/a/b.events.jsonl"))
	}
}

func TestTitleAndOldCuesFile(t *testing.T) {
	dir := t.TempDir()
	r := &Replay{CuesPath: filepath.Join(dir, "x.cues.json")}
	if err := r.SetTitle("  Friday set "); err != nil {
		t.Fatal(err)
	}
	if r.Title() != "Friday set" {
		t.Fatal(r.Title())
	}
	if err := r.SetCues([]Cue{{T: 1, Label: "A1"}}); err != nil {
		t.Fatal(err)
	}
	var m meta
	b, _ := os.ReadFile(r.CuesPath)
	if json.Unmarshal(b, &m) != nil || m.Title != "Friday set" || len(m.Cues) != 1 {
		t.Fatalf("file: %s", b)
	}
	// the first form of the file was only the list of cues
	old := filepath.Join(dir, "old.cues.json")
	os.WriteFile(old, []byte(`[{"t":2,"label":"B1"}]`), 0o644)
	var l []Cue
	if b, _ := os.ReadFile(old); json.Unmarshal(b, &m) == nil {
		t.Fatal("an array must not parse as the object")
	} else {
		json.Unmarshal(b, &l)
	}
	if len(l) != 1 {
		t.Fatal(l)
	}
}

func TestStyleIsKept(t *testing.T) {
	r := &Replay{CuesPath: filepath.Join(t.TempDir(), "x.cues.json")}
	if err := r.SetStyle([]byte(`{"accent":"#ff3b30","handle":"@moahbeatlab"}`)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(r.Style()), "@moahbeatlab") {
		t.Fatalf("style %s", r.Style())
	}
	if r.SetStyle([]byte(`[1]`)) == nil {
		t.Error("a list is no style")
	}
	if err := r.SetStyle([]byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(r.CuesPath); err == nil {
		t.Error("an empty style, no title and no cues should leave no file")
	}
}

func TestCaptionsAreKept(t *testing.T) {
	r := &Replay{CuesPath: filepath.Join(t.TempDir(), "x.cues.json")}
	if err := r.SetCaptions([]Caption{{From: 1, To: 4.5, Text: "the kick is muted here"}}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(r.CuesPath)
	if !strings.Contains(string(b), "the kick is muted here") {
		t.Fatalf("not kept: %s", b)
	}
	if got := r.Captions(); len(got) != 1 || got[0].To != 4.5 {
		t.Fatalf("%v", got)
	}
}

func TestPatternsCanBeAdopted(t *testing.T) {
	r := &Replay{CuesPath: filepath.Join(t.TempDir(), "x.cues.json")}
	if err := r.SetPatterns([]Cue{{T: 3, Label: "B1"}}, true); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(r.CuesPath)
	if !strings.Contains(string(b), "patterns_adopted") {
		t.Fatalf("not kept: %s", b)
	}
	r2 := &Replay{CuesPath: r.CuesPath}
	var m meta
	json.Unmarshal(b, &m)
	r2.title, r2.cues, r2.adopted = m.Title, m.Cues, m.PatternsAdopted
	if !r2.Adopted() || len(r2.Cues()) != 1 {
		t.Fatal("reading back")
	}
}

func TestChaptersAreTextForYouTube(t *testing.T) {
	got := ChaptersText([]Chapter{{T: 75.4, Title: " Drums "}, {T: 0, Title: "Intro"}, {T: 3725, Title: "Outro"}})
	want := "0:00 Intro\n1:15 Drums\n1:02:05 Outro\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := ChaptersText([]Chapter{{T: 30, Title: "A"}}); !strings.HasPrefix(got, "0:00 Intro\n0:30 A") {
		t.Errorf("YouTube needs a chapter at 0:00: %q", got)
	}
	r := &Replay{CuesPath: filepath.Join(t.TempDir(), "x.cues.json")}
	if err := r.SetChapters([]Chapter{{T: 1, Title: "x"}}); err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters()) != 1 {
		t.Fatal("not kept")
	}
	if err := r.SetCaptions([]Caption{{From: 1, To: 2, Text: "t", Focus: "pad:A3"}}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(r.CuesPath)
	if !strings.Contains(string(b), "pad:A3") || !strings.Contains(string(b), `"chapters"`) {
		t.Fatalf("file: %s", b)
	}
}

func TestBusNotesAreKept(t *testing.T) {
	r := &Replay{CuesPath: filepath.Join(t.TempDir(), "x.cues.json")}
	if err := r.SetBusNotes([]BusNote{{Bus: 1, T: 0, Text: "Filter+Drive"}, {Bus: 1, T: 62.5, Text: "Delay"}}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(r.CuesPath)
	if !strings.Contains(string(b), "bus_notes") || len(r.BusNotes()) != 2 {
		t.Fatalf("not kept: %s", b)
	}
}

func TestTheCameraVideoIsServedAndKept(t *testing.T) {
	dir := t.TempDir()
	vid := filepath.Join(dir, "cam.mp4")
	os.WriteFile(vid, []byte("not really a video, but bytes"), 0o644)
	r := &Replay{CuesPath: filepath.Join(dir, "x.cues.json")}
	if err := r.SetCameraFile("/no/such/file.mp4"); err == nil {
		t.Fatal("a missing file was taken")
	}
	if err := r.SetCameraFile(vid); err != nil {
		t.Fatal(err)
	}
	if r.cameraFile != "cam.mp4" || r.CameraPath() != vid {
		t.Fatalf("kept as %q, path %q", r.cameraFile, r.CameraPath())
	}
	if err := r.SetCamera([]byte(`{"sync":1.5,"layer":"front"}`)); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(r.Handler())
	defer srv.Close()
	req, _ := http.NewRequest("GET", srv.URL+"/camera", nil)
	req.Header.Set("Range", "bytes=4-8")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 206 || string(b) != "reall" {
		t.Fatalf("range: %d %q", resp.StatusCode, b)
	}
	data, _ := os.ReadFile(r.CuesPath)
	if !strings.Contains(string(data), `"camera_file": "cam.mp4"`) || !strings.Contains(string(data), "sync") {
		t.Fatalf("file: %s", data)
	}
}

func TestTemplatesAreSavedListedAndUsedForNewTakes(t *testing.T) {
	r := &Replay{CuesPath: filepath.Join(t.TempDir(), "x.cues.json"), TemplatesDir: t.TempDir(), fresh: true}
	if err := r.WriteTemplate("../evil", []byte(`{}`)); err == nil {
		t.Fatal("a path was taken as a name")
	}
	body := `{"name":"Reel look","parts":{"look":{"accent":"#ff3b30"},"scenes":[{"rel":0,"layout":"top"}],"chapters":[]}}`
	if err := r.WriteTemplate("Reel look", []byte(body)); err != nil {
		t.Fatal(err)
	}
	l, err := r.ListTemplates()
	if err != nil || len(l) != 1 || l[0].Name != "Reel look" || len(l[0].Parts) != 2 || l[0].Default {
		t.Fatalf("list: %+v %v (empty parts are not counted)", l, err)
	}
	if err := r.SetDefaultTemplate("nope"); err == nil {
		t.Fatal("a template that is not there was made the default")
	}
	if err := r.SetDefaultTemplate("Reel look"); err != nil {
		t.Fatal(err)
	}
	if l, _ := r.ListTemplates(); !l[0].Default {
		t.Fatal("not the default")
	}
	if got := r.freshTemplate(); !strings.Contains(string(got), "ff3b30") {
		t.Fatalf("a fresh take should get the default: %s", got)
	}
	r.SetTitle("x") // something is kept now: the take is not fresh any more
	if r.freshTemplate() != nil {
		t.Fatal("a take with saved things must not get the template again")
	}
	srv := httptest.NewServer(r.Handler())
	defer srv.Close()
	resp, _ := http.Get(srv.URL + "/templates")
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), "Reel look") {
		t.Fatalf("/templates: %s", b)
	}
	req, _ := http.NewRequest("DELETE", srv.URL+"/templates/Reel%20look", nil)
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != 204 {
		t.Fatalf("delete: %v %v", resp, err)
	}
	if l, _ := r.ListTemplates(); len(l) != 0 || r.DefaultTemplate() != nil {
		t.Fatal("a deleted template (and its default mark) is gone")
	}
}

func TestNoCameraIsNoName(t *testing.T) {
	r := &Replay{CuesPath: filepath.Join(t.TempDir(), "x.cues.json")}
	if r.CameraName() != "" {
		t.Fatalf("a take without a camera has the name %q", r.CameraName())
	}
}

// The page is one big script: a mistake that stops it from parsing (a comment that swallows the rest of a line, a missing bracket) makes the whole
// page dead, and nothing in Go notices. When node is there, the script is parsed with it.
func TestThePageScriptParses(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("no node to parse the page script with")
	}
	page, err := assets.ReadFile("static/replay.html")
	if err != nil {
		t.Fatal(err)
	}
	s := string(page)
	i := strings.Index(s, "<script>\n\"use strict\"")
	j := strings.LastIndex(s, "</script>")
	if i < 0 || j < i {
		t.Fatal("the script of the page was not found")
	}
	cmd := exec.Command(node, "-e", `const vm = require("vm"), fs = require("fs"); new vm.Script(fs.readFileSync(0, "utf8"), { filename: "replay.html" });`)
	cmd.Stdin = strings.NewReader(s[i+len("<script>") : j])
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("the script of the page does not parse: %v\n%s", err, out)
	}
}
