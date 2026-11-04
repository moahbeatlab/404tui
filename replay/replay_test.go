package replay

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"net/http/httptest"
	"os"
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
