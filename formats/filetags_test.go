package formats

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"
)

func id3Frame(ver byte, id string, body []byte) []byte {
	b := []byte(id)
	var sz [4]byte
	if ver == 4 {
		n := len(body)
		sz = [4]byte{byte(n >> 21 & 0x7f), byte(n >> 14 & 0x7f), byte(n >> 7 & 0x7f), byte(n & 0x7f)}
	} else {
		binary.BigEndian.PutUint32(sz[:], uint32(len(body)))
	}
	return append(append(append(b, sz[:]...), 0, 0), body...)
}

func utf16Text(s string) []byte {
	out := []byte{1, 0xff, 0xfe}
	for _, u := range utf16.Encode([]rune(s)) {
		out = append(out, byte(u), byte(u>>8))
	}
	return out
}

func id3Tag(ver byte, frames ...[]byte) []byte {
	var body []byte
	for _, f := range frames {
		body = append(body, f...)
	}
	n := len(body)
	return append([]byte{'I', 'D', '3', ver, 0, 0, byte(n >> 21 & 0x7f), byte(n >> 14 & 0x7f), byte(n >> 7 & 0x7f), byte(n & 0x7f)}, body...)
}

// mp3Frames is n MPEG-1 layer III frames at 128 kbps, 44.1 kHz, stereo (417 bytes each).
func mp3Frames(n int) []byte {
	var out []byte
	for range n {
		f := make([]byte, 417)
		f[0], f[1], f[2], f[3] = 0xff, 0xfb, 0x90, 0x00
		out = append(out, f...)
	}
	return out
}

func TestATrackSaysWhatItIs(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, b []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	// ID3v2.4: UTF-16 and UTF-8 text, a numbered genre, the key and the comment in their frames, a named text
	tag := id3Tag(4,
		id3Frame(4, "TIT2", utf16Text("Give Me Something")),
		id3Frame(4, "TPE1", append([]byte{3}, "The Black 80s"...)),
		id3Frame(4, "TCON", append([]byte{0}, "(35)"...)),
		id3Frame(4, "TBPM", append([]byte{0}, "124"...)),
		id3Frame(4, "TKEY", append([]byte{0}, "Am"...)),
		id3Frame(4, "TDRC", append([]byte{0}, "2019-05-01"...)),
		id3Frame(4, "COMM", append([]byte{0, 'e', 'n', 'g', 0}, "Hollis P Monroe Mix"...)),
		id3Frame(4, "TXXX", append([]byte{0}, "LABEL\x00Defected"...)),
	)
	p := write("a.mp3", append(tag, mp3Frames(100)...))
	got, err := ReadFileTags(p)
	if err != nil {
		t.Fatal(err)
	}
	want := FileTags{Title: "Give Me Something", Artist: "The Black 80s", Genre: "House", BPM: "124", Key: "Am", Year: "2019",
		Comment: "Hollis P Monroe Mix", Label: "Defected", Kbps: 128, Rate: 44100, Channels: 2}
	secs := got.Seconds
	got.Seconds = 0
	if got != want {
		t.Fatalf("v2.4:\n got %+v\nwant %+v", got, want)
	}
	if secs < 2.55 || secs > 2.65 { // 100 frames of 1152 samples at 44.1 kHz
		t.Errorf("length %.3f s, want about 2.61", secs)
	}

	// ID3v2.3, and an old tag at the end of a file without one
	p = write("b.mp3", append(id3Tag(3, id3Frame(3, "TALB", append([]byte{0}, "Defected Classics"...))), mp3Frames(10)...))
	if got, _ := ReadFileTags(p); got.Album != "Defected Classics" {
		t.Errorf("v2.3 album %q", got.Album)
	}
	v1 := make([]byte, 128)
	copy(v1, "TAG")
	copy(v1[3:], "Old Title")
	copy(v1[33:], "Old Artist")
	v1[127] = 35
	p = write("c.mp3", append(mp3Frames(10), v1...))
	if got, _ := ReadFileTags(p); got.Title != "Old Title" || got.Artist != "Old Artist" || got.Genre != "House" || got.Kbps != 128 {
		t.Errorf("v1: %+v", got)
	}

	// a WAV's INFO list
	info := []byte("INFO")
	for _, kv := range [][2]string{{"INAM", "Loop 1"}, {"IART", "moahbeatlab"}} {
		v := kv[1] + "\x00"
		if len(v)%2 == 1 {
			v += "\x00"
		}
		info = binary.LittleEndian.AppendUint32(append(info, kv[0]...), uint32(len(v)))
		info = append(info, v...)
	}
	var w []byte
	w = append(w, "RIFF\x00\x00\x00\x00WAVEfmt "...)
	w = binary.LittleEndian.AppendUint32(w, 16)
	w = binary.LittleEndian.AppendUint16(w, 1)
	w = binary.LittleEndian.AppendUint16(w, 2)
	w = binary.LittleEndian.AppendUint32(w, 48000)
	w = binary.LittleEndian.AppendUint32(w, 48000*4)
	w = binary.LittleEndian.AppendUint16(w, 4)
	w = binary.LittleEndian.AppendUint16(w, 16)
	w = binary.LittleEndian.AppendUint32(append(w, "LIST"...), uint32(len(info)))
	w = append(w, info...)
	w = binary.LittleEndian.AppendUint32(append(w, "data"...), 48000*4)
	w = append(w, make([]byte, 48000*4)...)
	p = write("d.wav", w)
	if got, _ := ReadFileTags(p); got.Title != "Loop 1" || got.Artist != "moahbeatlab" || got.Seconds != 1 || got.Rate != 48000 || got.Channels != 2 {
		t.Errorf("wav: %+v", got)
	}
}

func mp4Box(typ string, parts ...[]byte) []byte {
	var body []byte
	for _, p := range parts {
		body = append(body, p...)
	}
	return append(binary.BigEndian.AppendUint32(nil, uint32(8+len(body))), append([]byte(typ), body...)...)
}

func mp4Data(typ uint32, v []byte) []byte {
	return mp4Box("data", binary.BigEndian.AppendUint32(nil, typ), make([]byte, 4), v)
}

func TestAnM4aSaysWhatItIs(t *testing.T) {
	mvhd := make([]byte, 100)
	binary.BigEndian.PutUint32(mvhd[12:], 1000)   // time scale
	binary.BigEndian.PutUint32(mvhd[16:], 355000) // 355 s
	entry := make([]byte, 28)
	binary.BigEndian.PutUint16(entry[16:], 2)
	binary.BigEndian.PutUint32(entry[24:], 44100<<16)
	stsd := mp4Box("stsd", make([]byte, 8), mp4Box("mp4a", entry))
	ilst := mp4Box("ilst",
		mp4Box("\xa9nam", mp4Data(1, []byte("Give Me Something"))),
		mp4Box("\xa9ART", mp4Data(1, []byte("The Black 80s"))),
		mp4Box("tmpo", mp4Data(21, []byte{0, 124})),
		mp4Box("----", mp4Box("mean", make([]byte, 4), []byte("com.apple.iTunes")), mp4Box("name", make([]byte, 4), []byte("initialkey")), mp4Data(1, []byte("8A"))),
	)
	moov := mp4Box("moov", mp4Box("mvhd", mvhd), mp4Box("trak", mp4Box("mdia", mp4Box("minf", mp4Box("stbl", stsd)))),
		mp4Box("udta", mp4Box("meta", make([]byte, 4), ilst)))
	// the sound first, its index (moov) at the end, as many programs write it
	file := append(append(mp4Box("ftyp", []byte("M4A \x00\x00\x00\x00")), mp4Box("mdat", make([]byte, 5000))...), moov...)
	p := filepath.Join(t.TempDir(), "a.m4a")
	os.WriteFile(p, file, 0o644)
	got, err := ReadFileTags(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Give Me Something" || got.Artist != "The Black 80s" || got.BPM != "124" || got.Key != "8A" ||
		got.Seconds != 355 || got.Rate != 44100 || got.Channels != 2 {
		t.Errorf("m4a: %+v", got)
	}
	if !IsAudioFile("Track.M4A") {
		t.Error("an .m4a is not taken as audio")
	}
}
