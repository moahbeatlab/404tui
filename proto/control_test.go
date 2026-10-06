package proto

import (
	"bytes"
	"slices"
	"testing"
)

func mustPad(t *testing.T, name string) PadIndex {
	t.Helper()
	p, err := ParsePad(name)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSpecPayloads(t *testing.T) {
	b1, b2, b3 := mustPad(t, "B1"), mustPad(t, "B2"), mustPad(t, "B3")
	d13, j15, j16 := mustPad(t, "D13"), mustPad(t, "J15"), mustPad(t, "J16")
	cases := []struct {
		name      string
		got, want []byte
	}{
		{"set pad param", SetParam(0x69, TargetPad(b1), 100), []byte{0x01, 0x69, 0x00, 0x10, 0x00, 0x64, 0, 0, 0}},
		{"set global param", SetParam(0x2A, TargetGlobal, 12000), []byte{0x01, 0x2A, 0x00, 0xFF, 0xFF, 0xE0, 0x2E, 0, 0}},
		{"pad block", PadBlockRequest(b1), []byte{0x9E, 0x10, 0x00}},
		{"project settings", ProjectSettingsRequest(11), []byte{0xFD, 0x0B, 0x00}},
		{"preview start", PreviewStart(d13), []byte{0x8E, 0x3C, 0x00, 0x7F, 0xFF}},
		{"preview stop", PreviewStop(d13), []byte{0x8F, 0x3C, 0x00, 0x00}},
		{"peaks", PeaksRequest(b1, 1, 0x2000, 32, 256), []byte{
			0x9C, 0x05, 0x10, 0x00, 0x01, 0x00, 0x20, 0, 0, 0x20, 0, 0, 0, 0x00, 0x01, 0, 0,
		}},
		{"delete", PadOpDelete.Payload(j16), []byte{0x91, 0x9F, 0x00, 0x00, 0x00}},
		{"release slot", ImportReleaseSlot(j16), []byte{0x91, 0x9F, 0x00, 0x01, 0x00}},
		{"project name", ProjectName(5, "PROJECT_06"), []byte("\x35\x05PROJECT_06")},
		{"sample name", SampleName(j16, "TEST"), []byte("\x02\x9f\x00TEST")},
		{"move overwrite", MoveSample(j16, j15, MoveOverwrite), []byte{0x95, 0x9F, 0x00, 0x9E, 0x00, 0x00, 0x00}},
		{"move exchange", MoveSample(b2, b3, MoveExchange), []byte{0x95, 0x11, 0x00, 0x12, 0x00, 0x01, 0x00}},
		{"init samples bank", InitProject(InitSamplesBank(2)), []byte{0x92, 0x00, 0x00, 0x02, 0x00, 0x02, 0x00}},
		{"init patterns bank", InitProject(InitPatternsBank(2)), []byte{0x92, 0x00, 0x00, 0x04, 0x00, 0x02, 0x00}},
		{"restore prepare", RestorePrepare(), []byte{0x92, 0x64, 0x00, 0x00, 0x00, 0x00, 0x00}},
	}
	for _, c := range cases {
		if !bytes.Equal(c.got, c.want) {
			t.Errorf("%s: got % x, want % x", c.name, c.got, c.want)
		}
	}

	if code, ok := ParseError([]byte{0x32, 0x89, 0xFF, 0xFF, 0xFF}); !ok || code != -119 {
		t.Errorf("ParseError = %d, %v", code, ok)
	}
	for code, want := range map[int32]string{-119: "invalid format", -92: "bank protected", -111: "no empty pad"} {
		if got, ok := ErrorMessage(code); !ok || got != want {
			t.Errorf("ErrorMessage(%d) = %q, %v; want %q", code, got, ok, want)
		}
	}
	if _, ok := ErrorMessage(-5); ok {
		t.Error("ErrorMessage(-5) should be unknown")
	}
}

func TestRenderMessagesFromCapture(t *testing.T) {
	a1, _ := NewPadIndex(0)
	if got, want := RenderControl(a1, RenderBounce, 0), []byte{0xB8, 0x00, 0x00, 0xEB, 0x03, 0x00, 0x00}; !bytes.Equal(got, want) {
		t.Errorf("RenderControl = % x, want % x", got, want)
	}
	used := make([]byte, 4+20)
	copy(used, []byte{0x37, 0x00, 0x01, 0x14, 0, 0, 0, 0, 0x80})
	if pads, ok := ParseRenderPadsUsed(used); !ok || !slices.Equal(pads, []PadIndex{mustPad(t, "C8")}) {
		t.Errorf("ParseRenderPadsUsed = %v, %v", pads, ok)
	}
	done, ok := ParseRenderDone([]byte{0x39, 0, 0, 0xEB, 0x03, 0, 0, 0, 0})
	if want := (RenderDone{Op: RenderBounce}); !ok || done != want {
		t.Errorf("ParseRenderDone = %+v, %v", done, ok)
	}
}

func TestParsesProjectName(t *testing.T) {
	if project, name, ok := ParseProjectName([]byte("\x35\x02PROJECT_03")); !ok || project != 2 || name != "PROJECT_03" {
		t.Errorf("got %d %q %v", project, name, ok)
	}
	if project, name, ok := ParseProjectName([]byte("\x35\x00      ")); !ok || project != 0 || name != "" {
		t.Errorf("got %d %q %v", project, name, ok)
	}
}
