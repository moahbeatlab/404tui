package fileapi

import (
	"bytes"
	"testing"

	"github.com/moahbeatlab/404tui/proto/septet"
)

func TestRoutesPathsToTheRightVolume(t *testing.T) {
	for path, want := range map[string]string{
		"ROLAND/SP-404MKII": "/SP404REMOTE///ROLAND/SP-404MKII",
		// A leading slash still means the device's own storage.
		"/ROLAND":        "/SP404REMOTE///ROLAND",
		"SD:IMPORT/pack": "/IMPORT/pack",
		"SD:/IMPORT":     "/IMPORT",
		"SD:":            "/",
		// Already-resolved paths pass through.
		Root: Root,
	} {
		if got := RemotePath(path); got != want {
			t.Errorf("RemotePath(%q) = %q, want %q", path, got, want)
		}
	}
	if !OnCard("SD:IMPORT") || OnCard("ROLAND") {
		t.Error("OnCard misroutes")
	}
}

func TestHostRequestsFromCapture(t *testing.T) {
	write := []byte{
		0xF0, 0x41, 0x7A, 0x03, 0x06, 0, 0, 0, 0, 0, 0x40, 0, 0, 0, 0x0C, 'R', 'I', 'F',
		'F', 0x24, 0x40, 0x1F, 0x00, 'W', 'A', 'V', 'E', 0xF7,
	}
	req, err := ParseHostRequest(write)
	if w, ok := req.(HostWrite); err != nil || !ok || w.Handle != 0 || !bytes.Equal(w.Data, []byte("RIFF\x24\x40\x1f\x00WAVE")) {
		t.Errorf("write parsed as %+v (%v)", req, err)
	}
	seek := []byte{0xF0, 0x41, 0x7A, 0x03, 0x07, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x0C, 0x00, 0xF7}
	req, err = ParseHostRequest(seek)
	if req != (HostSeek{Handle: 0, Offset: 12}) || err != nil {
		t.Errorf("seek parsed as %+v (%v)", req, err)
	}
	want := []byte{0xF0, 0x41, 0x7A, 0x7A, 0, 0, 0, 0, 0x2C, 0x7F, 0x7F, 0x7F, 0x7F, 0x7F, 0xF7}
	if got := HostStatus(0x2C); !bytes.Equal(got, want) {
		t.Errorf("HostStatus = % x", got)
	}
}

func TestOpenRequestMatchesSpecLayout(t *testing.T) {
	req := Open("ROLAND/SP-404MKII/PROJECT_06/PADCONF.BIN", FlagReadOnly)
	path := []byte("/SP404REMOTE///ROLAND/SP-404MKII/PROJECT_06/PADCONF.BIN\x00")
	n := septet.Encode(int32(len(path)))
	if !bytes.Equal(req[:5], []byte{0xF0, 0x41, 0x7A, 0x03, 0x00}) ||
		!bytes.Equal(req[5:10], make([]byte, 5)) ||
		!bytes.Equal(req[10:15], n[:]) ||
		!bytes.Equal(req[15:15+len(path)], path) ||
		req[len(req)-1] != 0xF7 {
		t.Errorf("open request = % x", req)
	}
}

func TestReadAndSeekRequests(t *testing.T) {
	want := []byte{0xF0, 0x41, 0x7A, 3, 4, 0, 0, 0, 0, 0x2b, 0, 0, 0, 4, 0, 0, 0xF7}
	if got := Read(0x2b, 512); !bytes.Equal(got, want) {
		t.Errorf("Read = % x", got)
	}
	if got := Read(0x2c, BulkRead)[10:15]; !bytes.Equal(got, []byte{0x20, 0, 0x08, 0, 0}) {
		t.Errorf("bulk read length = % x", got)
	}
	want = []byte{0xF0, 0x41, 0x7A, 3, 7, 0, 0, 0, 0, 0x2c, 0, 0, 0, 0, 12, 0, 0xF7}
	if got := Seek(0x2c, 12); !bytes.Equal(got, want) {
		t.Errorf("Seek = % x", got)
	}
}

func TestWriteRequestFlagsLastChunk(t *testing.T) {
	req := Write(0x2b, []byte{1, 2, 3}, true)
	if !bytes.Equal(req[:15], []byte{0xF0, 0x41, 0x7A, 3, 6, 0, 0, 0, 0, 0x2b, 0x40, 0, 0, 0, 3}) {
		t.Errorf("head = % x", req[:15])
	}
	if !bytes.Equal(req[15:], []byte{1, 2, 3, 0xF7}) {
		t.Errorf("tail = % x", req[15:])
	}
}

func TestParsesStatusAndDataReplies(t *testing.T) {
	notFound := []byte{0xF0, 0x41, 0x7A, 0x7A, 0x7F, 0x7F, 0x7F, 0x7F, 0x7F, 0, 0, 0, 0, 2, 0xF7}
	reply, err := ParseReply(notFound)
	if reply != (StatusReply{Result: -1, Extra: 2}) || err != nil || !reply.IsFailure() {
		t.Errorf("status parsed as %+v (%v)", reply, err)
	}

	fstat := []byte{
		0xF0, 0x41, 0x7A, 0x02, 0, 0, 0, 0, 0, 0x40, 0, 0, 0, 8, 0xff, 0x81, 0, 0, 0x44, 0x8c,
		0, 0, 0xF7,
	}
	reply, err = ParseReply(fstat)
	data, ok := reply.(DataReply)
	if err != nil || !ok || !data.Last {
		t.Fatalf("data parsed as %+v (%v)", reply, err)
	}
	st, err := ParseStat(data.Data)
	if err != nil || st.Size != 35908 || st.IsDir() {
		t.Errorf("stat = %+v (%v)", st, err)
	}
}

func TestParsesDirEntries(t *testing.T) {
	e, err := ParseDirEntry([]byte{
		0, 0, 0, 0, 12, 8, 'B', 'A', 'N', 'K', '4', '-', '1', '3', '.', 'S', 'M', 'P', 0,
	})
	if err != nil || e.Name != "BANK4-13.SMP" || e.IsDir() {
		t.Errorf("entry = %+v (%v)", e, err)
	}
	dot, err := ParseDirEntry([]byte{0, 0, 0, 0, 1, 4, '.', 0})
	if err != nil || !dot.IsDot() {
		t.Errorf("dot entry = %+v (%v)", dot, err)
	}
}

func TestRenameUsesFixedSlots(t *testing.T) {
	req := Rename("a", "b")
	n := septet.Encode(400)
	if !bytes.Equal(req[10:15], n[:]) || len(req) != 15+400+1 {
		t.Errorf("rename is %d bytes, length field % x", len(req), req[10:15])
	}
	if !bytes.Equal(req[15:15+18], []byte("/SP404REMOTE///a\x00\x00")) {
		t.Errorf("first slot = %q", req[15:15+18])
	}
}
