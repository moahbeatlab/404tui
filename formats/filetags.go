package formats

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf16"
)

// FileTags is what a track says about itself: the ID3 tags of an MP3, the Vorbis comments of a FLAC, the INFO list of a
// WAV, and how its sound is stored. They are read from the head (and an MP3's tail) of the file, never by decoding it.
type FileTags struct {
	Title, Artist, Album, Year, Genre, BPM, Key, Label, Remixer, Comment string
	Seconds                                                              float64 // 0: not known
	Kbps, Rate, Channels                                                 int
	VBR                                                                  bool
}

// Empty says whether the file had none of the words.
func (t FileTags) Empty() bool {
	return t.Title+t.Artist+t.Album+t.Year+t.Genre+t.BPM+t.Key+t.Label+t.Remixer+t.Comment == ""
}

// maxTagRead is how much of a file's head is read at most: tags with large covers are cut there (the words come first
// in almost every file).
const maxTagRead = 4 << 20

// ReadFileTags reads the tags of the file at path.
func ReadFileTags(path string) (FileTags, error) {
	f, err := os.Open(path)
	if err != nil {
		return FileTags{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return FileTags{}, err
	}
	head := make([]byte, 12)
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	switch {
	case bytes.HasPrefix(head, []byte("fLaC")):
		b, _ := readHead(f, maxTagRead)
		return flacTags(b), nil
	case bytes.HasPrefix(head, []byte("RIFF")) && len(head) >= 12 && string(head[8:12]) == "WAVE":
		b, _ := readHead(f, maxTagRead)
		return wavTags(b), nil
	}
	var t FileTags
	audioAt := int64(0)
	if bytes.HasPrefix(head, []byte("ID3")) && len(head) >= 10 {
		size := int64(syncsafe(head[6:10])) + 10
		if head[5]&0x10 != 0 {
			size += 10 // a footer
		}
		b, _ := readHead(f, min(size, maxTagRead))
		t = id3v2(b)
		audioAt = size
	}
	// the first frame of the sound: how it is stored and how long it lasts
	if _, err := f.Seek(audioAt, io.SeekStart); err == nil {
		b := make([]byte, 64<<10)
		n, _ := io.ReadFull(f, b)
		mp3Info(&t, b[:n], st.Size()-audioAt)
	}
	if t.Empty() && st.Size() >= 128 { // an old tag at the end
		b := make([]byte, 128)
		if _, err := f.ReadAt(b, st.Size()-128); err == nil && string(b[:3]) == "TAG" {
			v1 := id3v1(b)
			v1.Seconds, v1.Kbps, v1.Rate, v1.Channels, v1.VBR = t.Seconds, t.Kbps, t.Rate, t.Channels, t.VBR
			t = v1
		}
	}
	return t, nil
}

func readHead(f *os.File, n int64) ([]byte, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	b := make([]byte, n)
	m, err := io.ReadFull(f, b)
	if err == io.ErrUnexpectedEOF {
		err = nil
	}
	return b[:m], err
}

func syncsafe(b []byte) int {
	return int(b[0]&0x7f)<<21 | int(b[1]&0x7f)<<14 | int(b[2]&0x7f)<<7 | int(b[3]&0x7f)
}

// unsync takes out the zero bytes the unsynchronisation put after each 0xFF.
func unsync(b []byte) []byte {
	if !bytes.Contains(b, []byte{0xff, 0}) {
		return b
	}
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		out = append(out, b[i])
		if b[i] == 0xff && i+1 < len(b) && b[i+1] == 0 {
			i++
		}
	}
	return out
}

// id3v2 reads an ID3v2.2, 2.3 or 2.4 tag (b starts with its header).
func id3v2(b []byte) FileTags {
	var t FileTags
	if len(b) < 10 {
		return t
	}
	ver, flags := b[3], b[5]
	body := b[10:]
	if flags&0x80 != 0 && ver < 4 { // the whole tag is unsynchronised
		body = unsync(body)
	}
	if flags&0x40 != 0 && len(body) >= 4 { // an extended header
		n := int(binary.BigEndian.Uint32(body[:4]))
		if ver == 3 {
			n += 4
		} else {
			n = syncsafe(body[:4])
		}
		if n > len(body) {
			return t
		}
		body = body[n:]
	}
	idLen, hdr := 4, 10
	if ver == 2 {
		idLen, hdr = 3, 6
	}
	for len(body) >= hdr && body[0] != 0 {
		id := string(body[:idLen])
		var size int
		var fflags byte
		switch ver {
		case 2:
			size = int(body[3])<<16 | int(body[4])<<8 | int(body[5])
		case 3:
			size = int(binary.BigEndian.Uint32(body[4:8]))
			fflags = body[9]
		default:
			size = syncsafe(body[4:8])
			fflags = body[9]
		}
		if size < 0 || hdr+size > len(body) {
			break
		}
		data := body[hdr : hdr+size]
		body = body[hdr+size:]
		if ver == 4 {
			if fflags&0x01 != 0 && len(data) >= 4 { // a data length indicator
				data = data[4:]
			}
			if fflags&0x02 != 0 {
				data = unsync(data)
			}
			if fflags&0x0c != 0 { // compressed or encrypted: not read
				continue
			}
		} else if ver == 3 && fflags&0xc0 != 0 {
			continue
		}
		set := func(dst *string) {
			if *dst == "" {
				*dst = id3Text(data)
			}
		}
		switch id {
		case "TIT2", "TT2":
			set(&t.Title)
		case "TPE1", "TP1":
			set(&t.Artist)
		case "TALB", "TAL":
			set(&t.Album)
		case "TDRC", "TYER", "TYE", "TDRL", "TORY", "TDOR":
			set(&t.Year)
		case "TCON", "TCO":
			if t.Genre == "" {
				t.Genre = genreName(id3Text(data))
			}
		case "TBPM", "TBP":
			set(&t.BPM)
		case "TKEY", "TKE":
			set(&t.Key)
		case "TPUB", "TPB":
			set(&t.Label)
		case "TPE4", "TP4":
			set(&t.Remixer)
		case "COMM", "COM":
			if t.Comment == "" && len(data) > 4 { // encoding, language, a description, the text
				parts := splitText(data[0], data[4:])
				if len(parts) > 1 && !strings.HasPrefix(parts[0], "iTun") {
					t.Comment = strings.TrimSpace(parts[1])
				}
			}
		case "TXXX", "TXX": // a named text: some programs keep the comment, the key or the label in one
			if len(data) > 1 {
				if parts := splitText(data[0], data[1:]); len(parts) > 1 {
					setNamed(&t, strings.ToUpper(strings.TrimSpace(parts[0])), strings.TrimSpace(strings.Join(parts[1:], " ")))
				}
			}
		case "TLEN", "TLE":
			if ms, err := strconv.Atoi(id3Text(data)); err == nil && ms > 0 {
				t.Seconds = float64(ms) / 1000
			}
		}
	}
	if len(t.Year) > 4 {
		t.Year = t.Year[:4]
	}
	return t
}

// id3Text is the text of a text frame: its first value (several values are joined with " / ").
func id3Text(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	var vals []string
	for _, s := range splitText(data[0], data[1:]) {
		if s = strings.TrimSpace(s); s != "" {
			vals = append(vals, s)
		}
	}
	return strings.Join(vals, " / ")
}

// splitText decodes text in the frame's encoding, split at its zero terminators.
func splitText(enc byte, b []byte) []string {
	switch enc {
	case 1, 2: // UTF-16 with a byte order mark, UTF-16BE
		var out []string
		var cur []uint16
		be := enc == 2
		for i := 0; i+1 < len(b); i += 2 {
			if b[i] == 0xff && b[i+1] == 0xfe {
				be = false
				continue
			}
			if b[i] == 0xfe && b[i+1] == 0xff {
				be = true
				continue
			}
			u := binary.LittleEndian.Uint16(b[i:])
			if be {
				u = binary.BigEndian.Uint16(b[i:])
			}
			if u == 0 {
				out = append(out, string(utf16.Decode(cur)))
				cur = nil
				continue
			}
			cur = append(cur, u)
		}
		return append(out, string(utf16.Decode(cur)))
	case 3:
		return strings.Split(string(b), "\x00")
	}
	var out []string
	for _, p := range bytes.Split(b, []byte{0}) {
		out = append(out, latin1(p))
	}
	return out
}

func latin1(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

// genreName turns "(17)", "17" or "(17)Rock" into the genre's name.
func genreName(s string) string {
	g := s
	if strings.HasPrefix(g, "(") {
		if i := strings.IndexByte(g, ')'); i > 0 {
			if rest := strings.TrimSpace(g[i+1:]); rest != "" {
				return rest
			}
			g = g[1:i]
		}
	}
	if n, err := strconv.Atoi(g); err == nil {
		if n >= 0 && n < len(id3Genres) {
			return id3Genres[n]
		}
		return ""
	}
	return s
}

var id3Genres = []string{"Blues", "Classic Rock", "Country", "Dance", "Disco", "Funk", "Grunge", "Hip-Hop", "Jazz", "Metal",
	"New Age", "Oldies", "Other", "Pop", "R&B", "Rap", "Reggae", "Rock", "Techno", "Industrial", "Alternative", "Ska",
	"Death Metal", "Pranks", "Soundtrack", "Euro-Techno", "Ambient", "Trip-Hop", "Vocal", "Jazz+Funk", "Fusion", "Trance",
	"Classical", "Instrumental", "Acid", "House", "Game", "Sound Clip", "Gospel", "Noise", "Alternative Rock", "Bass", "Soul",
	"Punk", "Space", "Meditative", "Instrumental Pop", "Instrumental Rock", "Ethnic", "Gothic", "Darkwave",
	"Techno-Industrial", "Electronic", "Pop-Folk", "Eurodance", "Dream", "Southern Rock", "Comedy", "Cult", "Gangsta",
	"Top 40", "Christian Rap", "Pop/Funk", "Jungle", "Native American", "Cabaret", "New Wave", "Psychedelic", "Rave",
	"Showtunes", "Trailer", "Lo-Fi", "Tribal", "Acid Punk", "Acid Jazz", "Polka", "Retro", "Musical", "Rock & Roll",
	"Hard Rock"}

// id3v1 reads the 128-byte tag at the end of an old MP3.
func id3v1(b []byte) FileTags {
	field := func(p []byte) string {
		if i := bytes.IndexByte(p, 0); i >= 0 {
			p = p[:i]
		}
		return strings.TrimSpace(latin1(p))
	}
	t := FileTags{Title: field(b[3:33]), Artist: field(b[33:63]), Album: field(b[63:93]), Year: field(b[93:97]), Comment: field(b[97:127])}
	if int(b[127]) < len(id3Genres) {
		t.Genre = id3Genres[b[127]]
	}
	return t
}

var (
	mp3Kbps = [2][3][16]int{ // [MPEG-1, MPEG-2/2.5][layer I, II, III][index]
		{{0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448}, {0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384},
			{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}},
		{{0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256}, {0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160},
			{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160}},
	}
	mp3Rates = [4][3]int{{11025, 12000, 8000}, {0, 0, 0}, {22050, 24000, 16000}, {44100, 48000, 32000}} // by version bits
)

// mp3Info reads the first frame header of the sound in b (size bytes of sound in all): the bitrate, the rate, the channels,
// and the length (from a Xing or Info header's frame count, else from the bitrate).
func mp3Info(t *FileTags, b []byte, size int64) {
	for i := 0; i+4 <= len(b); i++ {
		if b[i] != 0xff || b[i+1]&0xe0 != 0xe0 {
			continue
		}
		verBits, layerBits := (b[i+1]>>3)&3, (b[i+1]>>1)&3
		bi, ri := b[i+2]>>4, (b[i+2]>>2)&3
		if verBits == 1 || layerBits == 0 || bi == 0 || bi == 15 || ri == 3 {
			continue
		}
		v := 1
		if verBits == 3 {
			v = 0
		}
		layer := 3 - int(layerBits) // 0: layer I
		kbps, rate := mp3Kbps[v][layer][bi], mp3Rates[verBits][ri]
		if kbps == 0 || rate == 0 {
			continue
		}
		mono := b[i+3]>>6 == 3
		t.Kbps, t.Rate, t.Channels = kbps, rate, 2
		if mono {
			t.Channels = 1
		}
		spf := 1152 // samples per frame
		if layer == 0 {
			spf = 384
		} else if layer == 2 && v == 1 {
			spf = 576
		}
		// the Xing/Info header sits after the side information
		side := 32
		switch {
		case v == 0 && mono:
			side = 17
		case v == 1 && !mono:
			side = 17
		case v == 1 && mono:
			side = 9
		}
		x := i + 4 + side
		if x+12 <= len(b) && (string(b[x:x+4]) == "Xing" || string(b[x:x+4]) == "Info") && b[x+7]&1 != 0 {
			frames := binary.BigEndian.Uint32(b[x+8:])
			if frames > 0 {
				secs := float64(frames) * float64(spf) / float64(rate)
				if t.Seconds == 0 {
					t.Seconds = secs
				}
				t.VBR = string(b[x:x+4]) == "Xing"
				if t.VBR && secs > 0 {
					t.Kbps = int(float64(size-int64(i)) * 8 / secs / 1000)
				}
				return
			}
		}
		if vb := i + 36; vb+18 <= len(b) && string(b[vb:vb+4]) == "VBRI" { // Fraunhofer's VBR header
			frames := binary.BigEndian.Uint32(b[vb+14:])
			if secs := float64(frames) * float64(spf) / float64(rate); secs > 0 {
				if t.Seconds == 0 {
					t.Seconds = secs
				}
				t.VBR = true
				t.Kbps = int(float64(size-int64(i)) * 8 / secs / 1000)
				return
			}
		}
		if t.Seconds == 0 {
			t.Seconds = float64(size-int64(i)) * 8 / float64(kbps*1000)
		}
		return
	}
}

// flacTags reads a FLAC's stream info and Vorbis comments.
func flacTags(b []byte) FileTags {
	var t FileTags
	p := 4
	for p+4 <= len(b) {
		last, typ := b[p]&0x80 != 0, b[p]&0x7f
		n := int(b[p+1])<<16 | int(b[p+2])<<8 | int(b[p+3])
		p += 4
		if p+n > len(b) {
			break
		}
		blk := b[p : p+n]
		switch typ {
		case 0:
			if len(blk) >= 18 {
				rate := int(blk[10])<<12 | int(blk[11])<<4 | int(blk[12])>>4
				t.Rate, t.Channels = rate, int(blk[12]>>1&7)+1
				total := uint64(blk[13]&0x0f)<<32 | uint64(binary.BigEndian.Uint32(blk[14:18]))
				if rate > 0 {
					t.Seconds = float64(total) / float64(rate)
				}
			}
		case 4:
			vorbisComments(&t, blk)
		}
		p += n
		if last {
			break
		}
	}
	return t
}

func vorbisComments(t *FileTags, b []byte) {
	if len(b) < 8 {
		return
	}
	n := int(binary.LittleEndian.Uint32(b))
	if 4+n+4 > len(b) {
		return
	}
	b = b[4+n:]
	count := int(binary.LittleEndian.Uint32(b))
	b = b[4:]
	for range count {
		if len(b) < 4 {
			return
		}
		l := int(binary.LittleEndian.Uint32(b))
		if 4+l > len(b) {
			return
		}
		k, v, ok := strings.Cut(string(b[4:4+l]), "=")
		b = b[4+l:]
		if ok {
			setNamed(t, strings.ToUpper(k), strings.TrimSpace(v))
		}
	}
}

// setNamed sets a tag by its Vorbis or RIFF name.
func setNamed(t *FileTags, k, v string) {
	var dst *string
	switch k {
	case "TITLE", "INAM":
		dst = &t.Title
	case "ARTIST", "IART":
		dst = &t.Artist
	case "ALBUM", "IPRD":
		dst = &t.Album
	case "DATE", "YEAR", "ICRD":
		dst = &t.Year
	case "GENRE", "IGNR":
		dst = &t.Genre
	case "BPM", "TEMPO":
		dst = &t.BPM
	case "INITIALKEY", "KEY":
		dst = &t.Key
	case "LABEL", "ORGANIZATION", "PUBLISHER":
		dst = &t.Label
	case "REMIXER", "MIXARTIST":
		dst = &t.Remixer
	case "COMMENT", "DESCRIPTION", "ICMT":
		dst = &t.Comment
	}
	if dst != nil && *dst == "" {
		*dst = v
		if dst == &t.Year && len(v) > 4 {
			*dst = v[:4]
		}
	}
}

// wavTags reads a WAV's format and its LIST INFO chunk (and an id3 chunk, which some programs write).
func wavTags(b []byte) FileTags {
	var t FileTags
	var byteRate, dataLen int
	p := 12
	for p+8 <= len(b) {
		id, n := string(b[p:p+4]), int(binary.LittleEndian.Uint32(b[p+4:]))
		body := b[p+8 : min(p+8+n, len(b))]
		switch {
		case id == "fmt " && len(body) >= 16:
			t.Channels = int(binary.LittleEndian.Uint16(body[2:]))
			t.Rate = int(binary.LittleEndian.Uint32(body[4:]))
			byteRate = int(binary.LittleEndian.Uint32(body[8:]))
		case id == "data":
			dataLen = n
		case id == "LIST" && len(body) >= 4 && string(body[:4]) == "INFO":
			q := 4
			for q+8 <= len(body) {
				k, l := string(body[q:q+4]), int(binary.LittleEndian.Uint32(body[q+4:]))
				if q+8+l > len(body) {
					break
				}
				setNamed(&t, k, strings.TrimSpace(strings.TrimRight(latin1(body[q+8:q+8+l]), "\x00")))
				q += 8 + l + l&1
			}
		case (id == "id3 " || id == "ID3 ") && bytes.HasPrefix(body, []byte("ID3")):
			it := id3v2(body)
			it.Rate, it.Channels = t.Rate, t.Channels
			if it.Empty() {
				break
			}
			t = mergeTags(t, it)
		}
		if id == "data" && p+8+n > len(b) {
			break // the sound is longer than what was read: the tags after it are not reached
		}
		p += 8 + n + n&1
	}
	if byteRate > 0 && dataLen > 0 {
		t.Seconds = float64(dataLen) / float64(byteRate)
		t.Kbps = byteRate * 8 / 1000
	}
	return t
}

// mergeTags fills what a lacks from b.
func mergeTags(a, b FileTags) FileTags {
	for _, f := range [][2]*string{{&a.Title, &b.Title}, {&a.Artist, &b.Artist}, {&a.Album, &b.Album}, {&a.Year, &b.Year},
		{&a.Genre, &b.Genre}, {&a.BPM, &b.BPM}, {&a.Key, &b.Key}, {&a.Label, &b.Label}, {&a.Remixer, &b.Remixer}, {&a.Comment, &b.Comment}} {
		if *f[0] == "" {
			*f[0] = *f[1]
		}
	}
	return a
}
