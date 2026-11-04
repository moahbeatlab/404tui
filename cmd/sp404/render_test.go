package main

import (
	"strings"
	"testing"
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
