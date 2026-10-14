package formats

import (
	"bytes"
	"encoding/binary"
	"math"
	"slices"
	"testing"

	"github.com/moahbeatlab/404tui/proto"
)

func TestSMPHeaderMatchesTheSpec(t *testing.T) {
	// 144000 stereo frames, as in the captured import of a 3 s test tone.
	h := header(576_000, 2)
	want := []byte{'R', 'F', 'W', 'V', 0x00, 0x08, 0xcb, 0xf8, 0x00, 0x00, 0xbb, 0x80, 0, 0, 0, 2, 0, 0, 0, 0x10}
	if !bytes.Equal(h[:20], want) {
		t.Errorf("fields = % x", h[:20])
	}
	if !bytes.Equal(h[0x20:0x24], []byte{0x29, 0x74, 0xff, 0x85}) {
		t.Errorf("digest bytes = % x, want the ones seen in the capture", h[0x20:0x24])
	}
	if !digestMatches(h[:]) {
		t.Error("digest does not match its own header")
	}
}

func TestSMPRoundTripAndCorruption(t *testing.T) {
	s := Sample{Channels: 2, Samples: []int16{0, 1, -1, math.MaxInt16, math.MinInt16, 1234}}
	b := s.SMP()
	if len(b) != 512+12 || !bytes.Equal(b[512:516], []byte{0, 0, 0, 1}) {
		t.Fatalf("encoded %d bytes, pcm starts % x", len(b), b[512:516])
	}
	got, err := ParseSMP(b)
	if err != nil || got.Channels != 2 || !slices.Equal(got.Samples, s.Samples) {
		t.Errorf("round trip = %+v (%v)", got, err)
	}
	info, _ := ParseHeader(b)
	if info.Frames() != 3 || info.FrameAt(512+8) != 2 || info.ByteAt(2) != 520 {
		t.Errorf("frames %d, frame at %d, byte at %d", info.Frames(), info.FrameAt(520), info.ByteAt(2))
	}

	bad := slices.Clone(b)
	bad[0x21] ^= 1
	if _, err := ParseSMP(bad); err != ErrDigestMismatch {
		t.Errorf("flipped digest: %v", err)
	}
	if _, err := ParseSMP(b[:len(b)-1]); err == nil {
		t.Error("truncated file accepted")
	}
	bad = slices.Clone(b)
	bad[0] = 'X'
	if _, err := ParseSMP(bad); err != ErrBadMagic {
		t.Errorf("bad magic: %v", err)
	}
}

func tone(n, rate int, hz, amp float64) []int16 {
	out := make([]int16, n)
	for i := range out {
		out[i] = int16(math.Sin(2*math.Pi*hz*float64(i)/float64(rate)) * amp)
	}
	return out
}

// wavFile builds a WAV of any depth for the decoder.
func wavFile(rate, channels, depth, format int, data []byte) []byte {
	var b []byte
	b = append(b, "RIFF"...)
	b = binary.LittleEndian.AppendUint32(b, uint32(36+len(data)))
	b = append(b, "WAVEfmt "...)
	b = binary.LittleEndian.AppendUint32(b, 16)
	b = binary.LittleEndian.AppendUint16(b, uint16(format))
	b = binary.LittleEndian.AppendUint16(b, uint16(channels))
	b = binary.LittleEndian.AppendUint32(b, uint32(rate))
	b = binary.LittleEndian.AppendUint32(b, uint32(rate*channels*depth/8))
	b = binary.LittleEndian.AppendUint16(b, uint16(channels*depth/8))
	b = binary.LittleEndian.AppendUint16(b, uint16(depth))
	b = append(b, "data"...)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(data)))
	return append(b, data...)
}

func TestWAVRoundTripIsBitExact(t *testing.T) {
	s := Sample{Channels: 2, Samples: []int16{0, 1, -1, 32767, -32768, 1234, -4321, 7}}
	for _, ext := range []string{".wav", ""} {
		got, err := DecodeAudio(s.WAV(), ext)
		if err != nil || got.Resampled() || got.SourceChannels != 2 || !slices.Equal(got.Sample.Samples, s.Samples) {
			t.Errorf("ext %q: %+v (%v)", ext, got, err)
		}
	}
}

func TestMono24BitStaysMonoAndSixteenBit(t *testing.T) {
	var data []byte
	for _, v := range []int32{0, 0x100, -0x800000, 0x7fffff} {
		data = append(data, byte(v), byte(v>>8), byte(v>>16))
	}
	got, err := DecodeAudio(wavFile(48000, 1, 24, 1, data), "wav")
	if err != nil || got.Sample.Channels != 1 || !slices.Equal(got.Sample.Samples, []int16{0, 1, math.MinInt16, math.MaxInt16}) {
		t.Errorf("got %+v (%v)", got, err)
	}
}

func TestFloatAndEightBitWAV(t *testing.T) {
	var f []byte
	for _, v := range []float32{0, 0.5, -1} {
		f = binary.LittleEndian.AppendUint32(f, math.Float32bits(v))
	}
	got, err := DecodeAudio(wavFile(48000, 1, 32, 3, f), "wav")
	if err != nil || !slices.Equal(got.Sample.Samples, []int16{0, 16384, -32768}) {
		t.Errorf("float: %+v (%v)", got, err)
	}
	got, err = DecodeAudio(wavFile(48000, 1, 8, 1, []byte{128, 255, 0}), "wav")
	if err != nil || got.Sample.Samples[0] != 0 || got.Sample.Samples[2] != -32768 {
		t.Errorf("8-bit: %+v (%v)", got, err)
	}
}

func TestExtraChannelsAreDropped(t *testing.T) {
	var data []byte
	for _, v := range []int16{1, 2, 3, 4, 5, 6} {
		data = binary.LittleEndian.AppendUint16(data, uint16(v))
	}
	got, err := DecodeAudio(wavFile(48000, 3, 16, 1, data), "wav")
	if err != nil || got.SourceChannels != 3 || got.Sample.Channels != 2 || !slices.Equal(got.Sample.Samples, []int16{1, 2, 4, 5}) {
		t.Errorf("got %+v (%v)", got, err)
	}
}

func TestAIFF(t *testing.T) {
	// 16-bit stereo, 48 kHz: the rate is an 80-bit extended float.
	comm := []byte{0, 2, 0, 0, 0, 2, 0, 16, 0x40, 0x0E, 0xBB, 0x80, 0, 0, 0, 0, 0, 0}
	ssnd := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0x7F, 0xFF, 0x80, 0x00, 0x00, 0x01, 0xFF, 0xFF}
	var b []byte
	b = append(b, "FORM"...)
	b = binary.BigEndian.AppendUint32(b, uint32(4+8+len(comm)+8+len(ssnd)))
	b = append(b, "AIFFCOMM"...)
	b = binary.BigEndian.AppendUint32(b, uint32(len(comm)))
	b = append(b, comm...)
	b = append(b, "SSND"...)
	b = binary.BigEndian.AppendUint32(b, uint32(len(ssnd)))
	b = append(b, ssnd...)
	got, err := DecodeAudio(b, "aif")
	if err != nil || got.SourceRate != 48000 || !slices.Equal(got.Sample.Samples, []int16{32767, -32768, 1, -1}) {
		t.Errorf("got %+v (%v)", got, err)
	}
}

func TestResamplesAToneTo48k(t *testing.T) {
	for _, rate := range []int{44_100, 22_050, 96_000, 32_000} {
		src := tone(rate, rate, 1000, 16_000)
		var data []byte
		for _, v := range src {
			data = binary.LittleEndian.AppendUint16(data, uint16(v))
		}
		got, err := DecodeAudio(wavFile(rate, 1, 16, 1, data), "wav")
		if err != nil || !got.Resampled() {
			t.Fatalf("%d Hz: %+v (%v)", rate, got, err)
		}
		s := got.Sample
		if s.Channels != 1 || s.Frames() != 48_000 {
			t.Fatalf("%d Hz: %d channels, %d frames", rate, s.Channels, s.Frames())
		}
		// Compare the middle against an ideal 1 kHz tone at 48 kHz.
		var errEnergy, refEnergy float64
		for n := 4_800; n < 43_200; n++ {
			ideal := math.Sin(2*math.Pi*1000*float64(n)/48_000) * 16_000
			errEnergy += math.Pow(float64(s.Samples[n])-ideal, 2)
			refEnergy += ideal * ideal
		}
		if snr := 10 * math.Log10(refEnergy/errEnergy); snr < 60 {
			t.Errorf("%d Hz: SNR %.1f dB", rate, snr)
		}
	}
}

func TestRejectsNonAudio(t *testing.T) {
	for _, junk := range [][]byte{[]byte("definitely not audio"), {}, []byte("RIFF")} {
		if _, err := DecodeAudio(junk, "wav"); err == nil {
			t.Errorf("%q accepted", junk)
		}
	}
	if !IsAudioFile("Kick.WAV") || !IsAudioFile("a.flac") || IsAudioFile("notes.txt") || IsAudioFile("samples.json") {
		t.Error("IsAudioFile misjudges extensions")
	}
}

func note(delta byte, pad proto.PadIndex, extra byte) []byte {
	group, offset := pad.Index()/groupSize, pad.Index()%groupSize
	return []byte{delta, noteFirst + byte(offset), byte(group) | extra, 0x8D, 0x7F, 0x40, 0x77, 0x00}
}

func TestPatternDeltasAndTrailer(t *testing.T) {
	c8, d14, f1 := mustPad("C8"), mustPad("D14"), mustPad("F1")
	b := slices.Concat(
		note(0x10, c8, 0),
		[]byte{0xFF, 0x80, 0, 0, 0, 0, 0, 0}, // filler, +255
		note(0x05, d14, 0x40),
		[]byte{0x00, noteFirst, 0x01, 0x00, 0x64, 0x40, 0x10, 0x01},
		[]byte{0x00, endRecord, 0, 0, 0, 0, 0, 0},
		[]byte{0x04, 0, 0, 0, 0x03, 0x80, 0x04, 0x01},
	)
	p, err := ParsePattern(b)
	if err != nil {
		t.Fatal(err)
	}
	if p.Length != 276 || p.BeatsPerBar() != 3 || len(p.Events) != 3 {
		t.Fatalf("length %d, beats %d, %d events", p.Length, p.BeatsPerBar(), len(p.Events))
	}
	ticks := []uint32{p.Events[0].Tick, p.Events[1].Tick, p.Events[2].Tick}
	if !slices.Equal(ticks, []uint32{0, 271, 276}) {
		t.Errorf("ticks = %v", ticks)
	}
	if p.Events[0].Pad != c8 || p.Events[0].Pitch != 0x8D || p.Events[0].Length != 0x77 {
		t.Errorf("first note = %+v", p.Events[0])
	}
	if p.Events[1].Pad != d14 || p.Events[1].Flags != 0x40 {
		t.Errorf("second note = %+v", p.Events[1])
	}
	if p.Events[2].Pad != f1 || p.Events[2].Velocity != 100 || p.Events[2].Length != 272 {
		t.Errorf("third note = %+v", p.Events[2])
	}
	if _, err := ParsePattern(make([]byte, 7)); err == nil {
		t.Error("ragged file accepted")
	}
}

func TestStartPatternStartsEveryPadTogether(t *testing.T) {
	a1, a5, g16 := mustPad("A1"), mustPad("A5"), mustPad("G16")
	b, err := StartPattern([]proto.PadIndex{a1, a5, g16}, 2)
	if err != nil {
		t.Fatal(err)
	}
	// Three notes, 3840 ticks of filler (15 x 255 + 15), the end record and the trailer.
	if len(b) != (3+16+2)*record {
		t.Fatalf("%d bytes", len(b))
	}
	if !slices.Equal(b[:record], []byte{0, noteFirst, 0, 0, 127, 64, 0x00, 0x0F}) {
		t.Errorf("first record = % x", b[:record])
	}
	if !slices.Equal(b[len(b)-record:], []byte{2, 0, 0, 0, 0, 0x80, 0x04, 0x01}) {
		t.Errorf("trailer = % x", b[len(b)-record:])
	}
	p, err := ParsePattern(b)
	if err != nil {
		t.Fatal(err)
	}
	if p.Length != 2*4*PPQ || p.BeatsPerBar() != 4 || len(p.Events) != 3 {
		t.Fatalf("length %d, beats %d, %d events", p.Length, p.BeatsPerBar(), len(p.Events))
	}
	for i, want := range []proto.PadIndex{a1, a5, g16} {
		if e := p.Events[i]; e.Pad != want || e.Tick != 0 || e.Length != 3840 || e.Velocity != 127 || e.Pitch != 0 {
			t.Errorf("event %d = %+v", i, e)
		}
	}
	for _, bars := range []int{0, MaxStartBars + 1} {
		if _, err := StartPattern([]proto.PadIndex{a1}, bars); err == nil {
			t.Errorf("%d bars accepted", bars)
		}
	}
	if _, err := StartPattern(nil, 2); err == nil {
		t.Error("a pattern without pads accepted")
	}
}

func mustPad(name string) proto.PadIndex {
	p, err := proto.ParsePad(name)
	if err != nil {
		panic(err)
	}
	return p
}

func TestRetargetBankLeavesTheRest(t *testing.T) {
	a1, b1 := mustPad("A1"), mustPad("B1")
	control := []byte{0x05, control, 0x40, 0x00, 0x7F, 0x40, 0x23, 0x00}
	b := slices.Concat(note(0x10, a1, 0), control, note(0x01, b1, 0), []byte{0, endRecord, 0, 0, 0, 0, 0, 0})
	before := slices.Clone(b)

	if n := RetargetBank(b, 0, 2); n != 1 {
		t.Fatalf("changed %d notes", n)
	}
	p, _ := ParsePattern(b)
	if p.Events[0].Pad != mustPad("C1") || p.Events[2].Pad != b1 {
		t.Errorf("notes now play %s and %s", p.Events[0].Pad, p.Events[2].Pad)
	}
	// Everything but the one note's two pad bytes is as it was.
	for i := range b {
		if b[i] != before[i] && i != 1 && i != 2 {
			t.Errorf("byte %d changed", i)
		}
	}
	if RetargetBank(b, 0, 2) != 0 {
		t.Error("retargeting bank A again found notes")
	}
	// A pad in the later groups survives the move to the first one and back.
	j16 := mustPad("J16")
	c := note(0, j16, 0)
	RetargetBank(c, 9, 0)
	if p, _ := ParsePattern(append(c, 0, endRecord, 0, 0, 0, 0, 0, 0)); p.Events[0].Pad != mustPad("A16") {
		t.Errorf("J16 retargeted to %s", p.Events[0].Pad)
	}
	if got := PatternPath(6, mustPad("B2")); got != "ROLAND/SP-404MKII/PROJECT_06/PTN/PTN00018.BIN" {
		t.Errorf("path = %s", got)
	}
}

func TestSliceIsAPartOfThePatternFromZero(t *testing.T) {
	p := Pattern{Length: 7680, Events: []Event{{Tick: 0}, {Tick: 1920}, {Tick: 3900}, {Tick: 5760}}}
	part := p.Slice(3840, 5760)
	if part.Length != 1920 || len(part.Events) != 1 || part.Events[0].Tick != 60 {
		t.Errorf("slice of bar 3 = length %d, events %+v; want 1920 and one event at 60", part.Length, part.Events)
	}
}
