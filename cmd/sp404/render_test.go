package main

import (
	"strings"
	"testing"

	"github.com/moahbeatlab/404tui/replay"
)

func TestRenderRefusesWhatIsWrongBeforeTheBrowserStarts(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{nil, "usage"},
		{[]string{"a.wav", "b.jsonl", "c.wav"}, "usage"},
		{[]string{"a.wav", "-format", "square"}, "wide or vertical"},
		{[]string{"a.wav", "-fps", "0"}, "-fps"},
		{[]string{"a.wav", "-ffmpeg", "/no/such/ffmpeg"}, "ffmpeg not found"},
	} {
		err := runRender(c.args, options{})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: %v, want %q", c.args, err, c.want)
		}
	}
}

func TestTheBrowserIsFoundOrSaid(t *testing.T) {
	if _, err := findBrowser("/no/such/browser"); err == nil || !strings.Contains(err.Error(), "was not found") {
		t.Errorf("%v", err)
	}
}

func TestReplayHelpSaysEverything(t *testing.T) {
	for _, a := range []string{"-h", "--help", "help"} {
		if err := runReplay([]string{a}, options{}); err != nil {
			t.Errorf("replay %s: %v", a, err)
		}
		if err := runReplay([]string{"render", a}, options{}); err != nil {
			t.Errorf("replay render %s: %v", a, err)
		}
	}
	for _, want := range []string{"pattern from here", "render", "-format", "preset=", "cues.json", "-workers"} {
		if !strings.Contains(replayUsage, want) {
			t.Errorf("the help does not mention %q", want)
		}
	}
}

func TestPlanFramesLeavesTheCutsOut(t *testing.T) {
	cuts := []replay.Cut{{From: 2, To: 4}, {From: 7, To: 8.5}}
	spans, times := planFrames(cuts, 0, 10, 10)
	if len(spans) != 3 || spans[0].n != 20 || spans[1].n != 30 || spans[2].n != 15 || len(times) != 65 {
		t.Fatalf("spans %+v, %d frames", spans, len(times))
	}
	if spans[1].first != 20 || times[20] != 4 || times[49] < 6.89 || times[50] != 8.5 || spans[0].cutBefore || !spans[0].cutAfter || !spans[2].cutBefore || spans[2].cutAfter {
		t.Fatalf("spans %+v, times %v", spans, times[18:22])
	}
	if f := audioFilter(spans, 10); !strings.Contains(f, "concat=n=3") || !strings.Contains(f, "atrim=start=4.0000:duration=3.0000") {
		t.Fatal(f)
	}
	// nothing cut: one part, as before
	if sp, tm := planFrames(nil, 3, 5, 30); len(sp) != 1 || len(tm) != 60 || sp[0].cutBefore || sp[0].cutAfter {
		t.Fatalf("%+v %d", sp, len(tm))
	}
	// everything cut
	if sp, _ := planFrames([]replay.Cut{{From: 0, To: 20}}, 0, 10, 30); len(sp) != 0 {
		t.Fatalf("%+v", sp)
	}
}
