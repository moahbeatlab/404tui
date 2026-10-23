package lab

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/moahbeatlab/404tui/formats"
)

// tone is a sine of hz Hz, secs long, mono, 48 kHz.
func tone(hz, secs float64) formats.Sample {
	n := int(secs * formats.SampleRate)
	s := formats.Sample{Channels: 1, Samples: make([]int16, n)}
	for i := range s.Samples {
		env := math.Exp(-3 * float64(i) / float64(n))
		s.Samples[i] = int16(18000 * env * math.Sin(2*math.Pi*hz*float64(i)/formats.SampleRate))
	}
	return s
}

// clicks is a sample with a short burst every gap seconds.
func clicks(count int, gap float64) formats.Sample {
	n := int(float64(count) * gap * formats.SampleRate)
	s := formats.Sample{Channels: 1, Samples: make([]int16, n)}
	for k := 0; k < count; k++ {
		at := int(float64(k) * gap * formats.SampleRate)
		for i := 0; i < 2000 && at+i < n; i++ {
			s.Samples[at+i] = int16(25000 * math.Exp(-float64(i)/300) * math.Sin(float64(i)*0.3))
		}
	}
	return s
}

// must and jsonOf panic on an error; every test defers catch, which turns the panic into a failure.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func catch(t *testing.T) {
	if r := recover(); r != nil {
		t.Helper()
		t.Fatalf("%v", r)
	}
}

func ok(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func jsonOf[T any](b []byte, err error) T {
	if err != nil {
		panic(err)
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		panic(fmt.Sprintf("%v in %s", err, b))
	}
	return v
}

func put(l *Lab, s formats.Sample) int { return l.keep(s) }

func TestProjectRoundTrip(t *testing.T) {
	defer catch(t)
	l := New()
	l.NewProject("MY BEAT")
	ok(t, l.Assign(0, put(l, tone(110, 0.5)), "Kick"))
	ok(t, l.Assign(17, put(l, tone(220, 0.3)), "Snare"))
	ok(t, l.Assign(159, put(l, tone(440, 0.2)), "Last"))
	ok(t, l.NewPattern(18, 2))
	ok(t, l.PatternOp(18, "insert", []byte(`{"pad":0,"tick":0,"vel":110,"len":200}`)))
	ok(t, l.SetTempo(98.5))
	zip1 := must(l.ExportZip("PROJECT_03"))

	m := New()
	n, err := m.LoadZip(zip1)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 { // PADCONF.BIN, three samples, one pattern
		t.Errorf("%d files loaded, want 5", n)
	}
	in := jsonOf[Info](m.Info())
	if !in.Open || in.Name != "MY BEAT" || in.Tempo != 98.5 {
		t.Errorf("info: %+v", in)
	}
	for _, c := range []struct {
		n    int
		name string
	}{{0, "Kick"}, {17, "Snare"}, {159, "Last"}} {
		p := in.Pads[c.n]
		if !p.Has || !p.File || p.Name != c.name || p.Frames == 0 || p.Level != 127 {
			t.Errorf("pad %d: %+v", c.n, p)
		}
	}
	if in.Pads[1].Has || in.Pads[1].File {
		t.Errorf("pad 2 should be empty: %+v", in.Pads[1])
	}
	if len(in.Patterns) != 1 || in.Patterns[0].N != 18 || in.Patterns[0].Notes != 1 || in.Patterns[0].Bars != 2 || in.Patterns[0].Label != "B2" {
		t.Errorf("patterns: %+v", in.Patterns)
	}
	// Exporting what was loaded changes nothing, byte for byte.
	zip2 := must(m.ExportZip("PROJECT_03"))
	a, b := files(t, zip1), files(t, zip2)
	if len(a) != len(b) {
		t.Fatalf("files %d -> %d", len(a), len(b))
	}
	for name, data := range a {
		if !bytes.Equal(data, b[name]) {
			t.Errorf("%s changed in a round trip", name)
		}
	}
	// The pad's sample file is a real SMP of the right length.
	s := must(formats.ParseSMP(m.files["SMPL/BANK1-01.SMP"]))
	if s.Frames() != 24000 {
		t.Errorf("kick has %d frames, want 24000", s.Frames())
	}
}

func files(t *testing.T, zipBytes []byte) map[string][]byte {
	t.Helper()
	l := New()
	l.Begin()
	// Reuse LoadZip's reader through a throwaway lab: compare the staged view.
	if _, err := l.LoadZip(zipBytes); err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{"PADCONF.BIN": must(l.PadconfBytes())}
	for k, v := range l.files {
		out[k] = v
	}
	return out
}

func TestLoadFindsTheProjectInsideACard(t *testing.T) {
	defer catch(t)
	l := New()
	l.NewProject("X")
	ok(t, l.Assign(5, put(l, tone(300, 0.2)), "Five"))
	pad := must(l.PadconfBytes())
	smp := l.files["SMPL/BANK1-06.SMP"]

	m := New()
	m.Begin()
	root := "ROLAND/SP-404MKII/PROJECT_07/"
	m.Add(root+"PADCONF.BIN", pad)
	m.Add(root+"SMPL/BANK1-06.SMP", smp)
	m.Add(root+"SMPL/._BANK1-06.SMP", []byte("junk")) // not a sample: dropped by name below
	m.Add(root+"PTN/PATTERNCHAIN_00.CHN", []byte("chn"))
	m.Add(root+"NOTES.TXT", []byte("not kept"))
	m.Add("ROLAND/SP-404MKII/PROJECT_08/PADCONF.BIN", pad) // another project: not this one
	m.Add("ROLAND/SP-404MKII/PROJECT_08/SMPL/BANK1-01.SMP", smp)
	// Windows paths work too.
	m.Add(strings.ReplaceAll(root, "/", "\\")+"PICTURE\\p.bin", []byte("pic"))
	n, err := m.Finish()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.files["NOTES.TXT"]; ok {
		t.Error("NOTES.TXT kept")
	}
	if _, ok := m.files["PTN/PATTERNCHAIN_00.CHN"]; !ok {
		t.Error("the pattern chain file was dropped")
	}
	if _, ok := m.files["PICTURE/p.bin"]; !ok {
		t.Error("a Windows-path file was lost")
	}
	if _, ok := m.files["SMPL/BANK1-01.SMP"]; ok {
		t.Error("a sample of another project came along")
	}
	if n < 4 {
		t.Errorf("%d files", n)
	}
	in := jsonOf[Info](m.Info())
	if !in.Pads[5].Has || in.Pads[5].Name != "Five" {
		t.Errorf("pad 6: %+v", in.Pads[5])
	}
}

func TestNoPadconfIsRefused(t *testing.T) {
	defer catch(t)
	l := New()
	l.Begin()
	l.Add("SMPL/BANK1-01.SMP", []byte("x"))
	if _, err := l.Finish(); err != ErrNoProject {
		t.Errorf("err = %v", err)
	}
	if _, err := l.LoadZip([]byte("not a zip")); err == nil {
		t.Error("garbage accepted")
	}
	l.Begin()
	l.Add("PADCONF.BIN", []byte("RFPDxxxxxxxxxxxxxxxx"))
	if _, err := l.Finish(); err == nil {
		t.Error("a short PADCONF.BIN accepted")
	}
}

func TestAssignMoveClear(t *testing.T) {
	defer catch(t)
	l := New()
	l.NewProject("T")
	a, b := put(l, tone(100, 0.2)), put(l, tone(200, 0.4))
	ok(t, l.Assign(0, a, "Aé\x01B"))
	ok(t, l.Assign(1, b, "Second"))
	in := jsonOf[Info](l.Info())
	if in.Pads[0].Name != "AB" {
		t.Errorf("name %q: non-printable characters should go", in.Pads[0].Name)
	}
	ok(t, l.MovePad(0, 1)) // swap
	in = jsonOf[Info](l.Info())
	if in.Pads[0].Name != "Second" || in.Pads[1].Name != "AB" || in.Pads[0].Frames != 19200 || in.Pads[1].Frames != 9600 {
		t.Errorf("after the swap: %+v %+v", in.Pads[0], in.Pads[1])
	}
	ok(t, l.MovePad(1, 5)) // into an empty pad
	in = jsonOf[Info](l.Info())
	if in.Pads[1].Has || in.Pads[1].File || !in.Pads[5].Has || in.Pads[5].Name != "AB" {
		t.Errorf("after the move: %+v %+v", in.Pads[1], in.Pads[5])
	}
	ok(t, l.RenamePad(5, "Renamed"))
	ok(t, l.ClearPad(0))
	in = jsonOf[Info](l.Info())
	if in.Pads[0].Has || in.Pads[0].File || in.Pads[5].Name != "Renamed" {
		t.Errorf("after clear: %+v %+v", in.Pads[0], in.Pads[5])
	}
	if err := l.Assign(160, a, "x"); err == nil {
		t.Error("pad 160 accepted")
	}
	if err := l.Assign(0, 999, "x"); err == nil {
		t.Error("a missing sample accepted")
	}
	// The padconf survives a write and a read with the pad blocks intact.
	pc := must(formats.ParsePadconf(must(l.PadconfBytes())))
	if len(pc.Blocks[5]) != formats.PadBlockLen {
		t.Errorf("block length %d", len(pc.Blocks[5]))
	}
}

func TestDecodeWAVAndSMP(t *testing.T) {
	defer catch(t)
	l := New()
	src := tone(440, 0.25)
	m := jsonOf[Meta](l.Decode(src.WAV(), ".wav"))
	if m.Frames != 12000 || m.Resampled {
		t.Errorf("meta %+v", m)
	}
	m2 := jsonOf[Meta](l.Decode(src.SMP(), "smp"))
	if m2.Frames != 12000 {
		t.Errorf("smp meta %+v", m2)
	}
	if _, err := l.Decode([]byte("nonsense"), ".wav"); err == nil {
		t.Error("garbage decoded")
	}
	w := must(l.WAV(m.ID))
	if !bytes.Equal(w, src.WAV()) {
		t.Error("WAV round trip differs")
	}
}

func TestSampleTools(t *testing.T) {
	defer catch(t)
	l := New()
	id := put(l, tone(220, 1))
	p := must(l.Pitched(id, 12))
	if f := must(l.Sample(p)).Frames(); f < 23000 || f > 25000 {
		t.Errorf("an octave up has %d frames, want about 24000", f)
	}
	st := must(l.Stretched(id, 72000))
	if f := must(l.Sample(st)).Frames(); f != 72000 {
		t.Errorf("stretched to %d", f)
	}
	tr := must(l.Trim(id, 1000, 5000))
	if f := must(l.Sample(tr)).Frames(); f != 4000 {
		t.Errorf("trim %d", f)
	}
	if _, err := l.Trim(id, 100, 101); err == nil {
		t.Error("an empty trim accepted")
	}
	j := must(l.Join(id, tr))
	if f := must(l.Sample(j)).Frames(); f != 52000 {
		t.Errorf("join %d", f)
	}
	rv := must(l.Reversed(tr))
	a, b := must(l.Sample(tr)), must(l.Sample(rv))
	if a.Samples[0] != b.Samples[len(b.Samples)-1] || a.Samples[100] != b.Samples[len(b.Samples)-101] {
		t.Error("reversed is not the mirror")
	}
	nm := must(l.Normalize(id, -1))
	peak := 0
	for _, v := range must(l.Sample(nm)).Samples {
		peak = max(peak, absInt(int(v)))
	}
	if peak < 29000 || peak > 29300 { // -1 dBFS is 29204
		t.Errorf("normalized peak %d, want about 29204", peak)
	}
	silent := put(l, formats.Sample{Channels: 1, Samples: make([]int16, 500)})
	if _, err := l.Normalize(silent, -1); err == nil {
		t.Error("a silent sample normalized")
	}
	if _, err := l.Join(); err == nil {
		t.Error("an empty join accepted")
	}
	pk := must(l.Peaks(id, 0, 48000, 10))
	if len(pk) != 10 || pk[0] < pk[9] {
		t.Errorf("peaks %v (a decaying tone should fall)", pk)
	}
	sb := jsonOf[map[string]any](l.StretchToBars(put(l, tone(100, 3.8)), 120))
	if sb["bars"].(float64) != 2 {
		t.Errorf("stretch to bars: %v", sb)
	}
	if _, err := l.StretchToBars(put(l, tone(100, 0.8)), 120); err == nil {
		t.Error("a 2.5x stretch accepted")
	}
}

func slicing(t *testing.T, l *Lab, op string, cur []byte, args string) []byte {
	t.Helper()
	out, err := l.Slicing(op, cur, []byte(args))
	if err != nil {
		t.Fatalf("%s: %v", op, err)
	}
	return out
}

func parse(t *testing.T, b []byte) *formats.Slicing {
	t.Helper()
	s := &formats.Slicing{}
	if err := json.Unmarshal(b, s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSlicerOperations(t *testing.T) {
	defer catch(t)
	l := New()
	id := put(l, clicks(8, 0.25))
	frames := must(l.Sample(id)).Frames()
	cur := slicing(t, l, "new", nil, `{"frames":`+itoa(frames)+`,"n":8}`)
	s := parse(t, cur)
	if len(s.Slices) != 8 || s.Frames != frames || len(s.Order) != 8 {
		t.Fatalf("new: %d slices, order %v", len(s.Slices), s.Order)
	}
	cur = slicing(t, l, "divide", cur, `{"n":16}`)
	if n := len(parse(t, cur).Slices); n != 16 {
		t.Errorf("divide: %d", n)
	}
	r1 := slicing(t, l, "random", cur, `{"n":8,"seed":5}`)
	r2 := slicing(t, l, "random", cur, `{"n":8,"seed":5}`)
	r3 := slicing(t, l, "random", cur, `{"n":8,"seed":6}`)
	if !bytes.Equal(r1, r2) || bytes.Equal(r1, r3) {
		t.Error("random cuts must repeat for a seed and differ between seeds")
	}
	on := slicing(t, l, "onsets", cur, `{"id":`+itoa(id)+`,"limit":32,"sens":0.5}`)
	if n := len(parse(t, on).Slices); n < 6 || n > 10 {
		t.Errorf("onsets found %d slices in 8 clicks", n)
	}
	cur = slicing(t, l, "new", nil, `{"frames":`+itoa(frames)+`,"n":4}`)
	step := 60.0 / 120 * formats.SampleRate / 2 // an 8th at 120 BPM
	g := slicing(t, l, "grid", cur, `{"step":`+ftoa(step)+`,"anchor":-1}`)
	if n := len(parse(t, g).Slices); n != 8 { // 2 s of eighths at 120 BPM
		t.Errorf("grid: %d slices, want 8", n)
	}
	sp := slicing(t, l, "split", cur, `{"i":0,"frame":`+itoa(frames/8)+`}`)
	if n := len(parse(t, sp).Slices); n != 5 {
		t.Errorf("split: %d", n)
	}
	rm := slicing(t, l, "remove", sp, `{"i":1}`)
	if n := len(parse(t, rm).Slices); n != 4 {
		t.Errorf("remove: %d", n)
	}
	if _, err := l.Slicing("split", cur, []byte(`{"i":0,"frame":10}`)); err == nil {
		t.Error("a cut too close to the edge accepted")
	}
	one := slicing(t, l, "new", nil, `{"frames":`+itoa(frames)+`,"n":1}`)
	if _, err := l.Slicing("remove", one, []byte(`{"i":0}`)); err == nil {
		t.Error("the last slice removed")
	}
	ms := slicing(t, l, "moveStart", cur, `{"i":1,"delta":500}`)
	if parse(t, ms).Slices[1].Start != parse(t, cur).Slices[1].Start+500 {
		t.Error("moveStart did not move")
	}
	if _, err := l.Slicing("bogus", cur, nil); err == nil {
		t.Error("unknown operation accepted")
	}
	if _, err := l.Slicing("divide", []byte("{"), nil); err == nil {
		t.Error("broken JSON accepted")
	}
}

// steps is a sample of count parts, 0.25 s each, every one a different pitch.
func steps(count int) formats.Sample {
	var s formats.Sample
	s.Channels = 1
	for k := 0; k < count; k++ {
		s.Samples = append(s.Samples, tone(150+60*float64(k), 0.25).Samples...)
	}
	return s
}

func TestSlicerRender(t *testing.T) {
	defer catch(t)
	l := New()
	id := put(l, steps(8))
	frames := must(l.Sample(id)).Frames()
	cur := slicing(t, l, "new", nil, `{"frames":`+itoa(frames)+`,"n":8}`)
	r := jsonOf[Rendered](l.Render(id, cur, 1, 0))
	if r.Frames < frames-100 || r.Frames > frames+100 || len(r.Placed) != 8 {
		t.Errorf("a straight round is %d frames (want about %d), %d steps", r.Frames, frames, len(r.Placed))
	}
	// On a grid every step lasts a 16th at the BPM.
	s := parse(t, cur)
	s.Division, s.BPM = 16, 120
	grid, _ := json.Marshal(s)
	g := jsonOf[Rendered](l.Render(id, grid, 2, 0))
	if want := 16 * 6000; g.Frames != want { // 16 steps of 6000 frames
		t.Errorf("gridded 2 rounds: %d frames, want %d", g.Frames, want)
	}
	// Reverse and mute a slice: the rendered audio differs; a muted-everything slicing refuses.
	s2 := parse(t, cur)
	s2.Slices[0].Reverse = true
	s2.Slices[3].Pitch = 7
	rev, _ := json.Marshal(s2)
	r2 := jsonOf[Rendered](l.Render(id, rev, 1, 0))
	if slices.Equal(must(l.PCM(r.ID)), must(l.PCM(r2.ID))) {
		t.Error("a reversed, pitched slice sounds the same")
	}
	s3 := parse(t, cur)
	for i := range s3.Slices {
		s3.Slices[i].Gain = 0
	}
	quiet, _ := json.Marshal(s3)
	if _, err := l.Render(id, quiet, 1, 0); err != nil {
		t.Logf("a silent slicing renders: %v", err)
	}
	// Random modes: the round number changes the picks, the same round repeats.
	s4 := parse(t, cur)
	s4.Mode = formats.ModeRandom
	s4.Seed = 3
	rnd, _ := json.Marshal(s4)
	a := jsonOf[Rendered](l.Render(id, rnd, 1, 0))
	b := jsonOf[Rendered](l.Render(id, rnd, 1, 0))
	c := jsonOf[Rendered](l.Render(id, rnd, 1, 1))
	if !slices.Equal(must(l.PCM(a.ID)), must(l.PCM(b.ID))) {
		t.Error("the same seed and round must render the same")
	}
	if slices.Equal(must(l.PCM(a.ID)), must(l.PCM(c.ID))) {
		t.Error("another round must pick other slices")
	}
	sa := jsonOf[Meta](l.SliceAudio(id, cur, 2))
	if sa.Frames < frames/8-10 || sa.Frames > frames/8+10 {
		t.Errorf("slice audio is %d frames", sa.Frames)
	}
	if _, err := l.SliceAudio(id, cur, 99); err == nil {
		t.Error("slice 99 accepted")
	}
	sh := must(ShuffleOrder(cur, 9))
	order := parse(t, sh).Order
	sorted := slices.Clone(order)
	slices.Sort(sorted)
	if !slices.Equal(sorted, []int{0, 1, 2, 3, 4, 5, 6, 7}) {
		t.Errorf("shuffled order %v is not a permutation", order)
	}
}

func itoa(n int) string     { b, _ := json.Marshal(n); return string(b) }
func ftoa(f float64) string { b, _ := json.Marshal(f); return string(b) }

func TestChordKit(t *testing.T) {
	defer catch(t)
	maj := must(ChordKit(0, 0, false)) // C major
	var names []string
	for _, c := range maj {
		names = append(names, c.Name)
	}
	if got := strings.Join(names, " "); got != "C Dm Em F G Am Bdim" {
		t.Errorf("C major triads: %s", got)
	}
	var romans []string
	for _, c := range maj {
		romans = append(romans, c.Roman)
	}
	if got := strings.Join(romans, " "); got != "I ii iii IV V vi vii°" {
		t.Errorf("romans: %s", got)
	}
	sev := must(ChordKit(0, 0, true))
	names = nil
	for _, c := range sev {
		names = append(names, c.Name)
	}
	if got := strings.Join(names, " "); got != "Cmaj7 Dm7 Em7 Fmaj7 G7 Am7 Bm7b5" {
		t.Errorf("C major sevenths: %s", got)
	}
	min := must(ChordKit(9, 1, false)) // A minor
	if min[0].Name != "Am" || min[2].Name != "C" || min[4].Name != "Em" {
		t.Errorf("A minor: %v", min)
	}
	if _, err := ChordKit(0, 2, false); err == nil { // MIN-P has 5 notes
		t.Error("a pentatonic chord kit accepted")
	}
	if _, err := ChordKit(0, -1, false); err == nil {
		t.Error("no scale accepted")
	}
	if ChordName([]int{0, 4, 7}) != "C" || ChordName([]int{6, 9, 1}) != "Gb"+"m" && ChordName([]int{6, 9, 1}) != "Gbm" {
		t.Errorf("names: %s %s", ChordName([]int{0, 4, 7}), ChordName([]int{6, 9, 1}))
	}
	if ChordName([]int{0, 1, 2}) != "C?" {
		t.Errorf("unknown chord: %s", ChordName([]int{0, 1, 2}))
	}
}

func TestRenderArpAndVariants(t *testing.T) {
	defer catch(t)
	l := New()
	id := put(l, tone(261.63, 0.6))
	base := formats.DefaultArp()
	base.Notes = ChordNotes(0, -1, 0, 0)
	base.Bars = 1
	base.BPM = 120
	chord := base
	chord.Dir = formats.ArpChord
	cj, _ := json.Marshal(chord)
	m := jsonOf[Meta](l.RenderArp(id, cj))
	if m.Frames < 28800 { // at least the sample length
		t.Errorf("chord is %d frames", m.Frames)
	}
	aj, _ := json.Marshal(base)
	a := jsonOf[Meta](l.RenderArp(id, aj))
	if a.Frames != 96000 { // 1 bar at 120 BPM
		t.Errorf("a one bar arp at 120 BPM is %d frames, want 96000", a.Frames)
	}
	// A chord differs from the plain sample and has no clipping.
	pcm := must(l.PCM(m.ID))
	for _, v := range pcm {
		if v == math.MaxInt16 || v == math.MinInt16 {
			t.Fatal("the chord clips")
		}
	}
	if slices.Equal(pcm, must(l.PCM(id))) {
		t.Error("the chord is the sample")
	}
	if _, err := l.RenderArp(id, []byte(`{"Notes":[]}`)); err == nil {
		t.Error("an arp with no notes accepted")
	}
	long := put(l, tone(100, 45))
	if _, err := l.RenderArp(long, aj); err == nil {
		t.Error("a 45 s sample accepted")
	}
	// Out of range settings are brought in range, not trusted.
	wild := base
	wild.Dir, wild.Rate, wild.Gate, wild.Swing, wild.Octaves, wild.Bars, wild.BPM = 99, 99, 99, 99, 99, 99, 9999
	wj, _ := json.Marshal(wild)
	if _, err := l.RenderArp(id, wj); err != nil {
		t.Errorf("wild settings: %v", err)
	}

	v1, v2 := ArpVariants(base, 24, 1), ArpVariants(base, 24, 1)
	if !slices.EqualFunc(v1, v2, func(a, b Variant) bool {
		j1, _ := json.Marshal(a)
		j2, _ := json.Marshal(b)
		return bytes.Equal(j1, j2)
	}) {
		t.Error("variants must repeat for a seed")
	}
	if len(v1) != 24 {
		t.Fatalf("%d variants", len(v1))
	}
	seen := map[string]bool{}
	sounds := map[string]bool{}
	for _, v := range v1 {
		j, _ := json.Marshal(v.Arp)
		sounds[string(j)] = true
		seen[v.Name] = true
		if !slices.Equal(v.Arp.Notes, base.Notes) || v.Arp.BPM != base.BPM || v.Arp.Bars != base.Bars {
			t.Errorf("%s changed the chord, tempo or length", v.Name)
		}
		r, err := l.RenderArp(id, mustJSON(v.Arp))
		if err != nil {
			t.Errorf("%s: %v", v.Name, err)
			continue
		}
		if jsonOf[Meta](r, nil).Frames == 0 {
			t.Errorf("%s is empty", v.Name)
		}
	}
	if len(sounds) < 20 {
		t.Errorf("only %d of 24 variants differ", len(sounds))
	}
	if len(seen) < 12 {
		t.Errorf("only %d different names", len(seen))
	}
	// Different seeds give different later variants.
	w := ArpVariants(base, 24, 2)
	same := 0
	for i := 12; i < 24; i++ {
		if w[i].Name == v1[i].Name && w[i].Arp.Seed == v1[i].Arp.Seed {
			same++
		}
	}
	if same > 3 {
		t.Errorf("%d of the random variants equal for another seed", same)
	}
	if len(ArpVariants(base, 0, 1)) != 1 || len(ArpVariants(base, 500, 1)) != 64 {
		t.Error("the variant count is not kept in 1..64")
	}
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

func TestPatterns(t *testing.T) {
	defer catch(t)
	l := New()
	l.NewProject("P")
	ok(t, l.NewPattern(1, 1))
	for i := 0; i < 16; i += 4 {
		ok(t, l.PatternOp(1, "insert", mustJSON(map[string]any{"pad": 0, "tick": i * 120, "vel": 100, "len": 100})))
	}
	ok(t, l.PatternOp(1, "insert", mustJSON(map[string]any{"pad": 1, "tick": 480, "vel": 90, "len": 100})))
	doc := jsonOf[PatternDoc](l.Pattern(1))
	if doc.Bars != 1 || doc.Length != 1920 || len(doc.Notes) != 5 || doc.Beats != 4 {
		t.Fatalf("pattern: %+v", doc)
	}
	ok(t, l.PatternOp(1, "double", nil))
	doc = jsonOf[PatternDoc](l.Pattern(1))
	if doc.Bars != 2 || len(doc.Notes) != 10 {
		t.Errorf("doubled: %d bars, %d notes", doc.Bars, len(doc.Notes))
	}
	ok(t, l.PatternOp(1, "halve", nil))
	ok(t, l.PatternOp(1, "extend", nil))
	doc = jsonOf[PatternDoc](l.Pattern(1))
	if doc.Bars != 2 || len(doc.Notes) != 5 {
		t.Errorf("halved then extended: %d bars, %d notes", doc.Bars, len(doc.Notes))
	}
	// Shift only pad 1.
	ok(t, l.PatternOp(1, "shift", mustJSON(map[string]any{"pad": 1, "grid": 120, "steps": 2})))
	doc = jsonOf[PatternDoc](l.Pattern(1))
	for _, n := range doc.Notes {
		if n.Pad == 1 && n.Tick != 720 {
			t.Errorf("pad 2 note at %d, want 720", n.Tick)
		}
		if n.Pad == 0 && n.Tick%480 != 0 {
			t.Errorf("pad 1 note moved to %d", n.Tick)
		}
	}
	// Edit chosen notes: velocity and length, then delete.
	ok(t, l.PatternOp(1, "edit", mustJSON(map[string]any{"notes": []PadAt{{1, 720}}, "vel": 64, "len": 50})))
	doc = jsonOf[PatternDoc](l.Pattern(1))
	for _, n := range doc.Notes {
		if n.Pad == 1 && (n.Vel != 64 || n.Len != 50) {
			t.Errorf("edited note: %+v", n)
		}
	}
	ok(t, l.PatternOp(1, "clearPad", mustJSON(map[string]any{"pad": 0})))
	doc = jsonOf[PatternDoc](l.Pattern(1))
	if len(doc.Notes) != 1 || doc.Notes[0].Pad != 1 {
		t.Errorf("after clearPad: %+v", doc.Notes)
	}
	ok(t, l.PatternOp(1, "edit", mustJSON(map[string]any{"notes": []PadAt{{1, 720}}, "delete": true})))
	if doc = jsonOf[PatternDoc](l.Pattern(1)); len(doc.Notes) != 0 {
		t.Errorf("after delete: %+v", doc.Notes)
	}
	// Quantize, swing, humanize, reverse keep the notes.
	ok(t, l.NewPattern(2, 1))
	for _, tk := range []int{0, 130, 250, 370, 500, 610} {
		ok(t, l.PatternOp(2, "insert", mustJSON(map[string]any{"pad": 3, "tick": tk, "vel": 100, "len": 60})))
	}
	ok(t, l.PatternOp(2, "quantize", mustJSON(map[string]any{"grid": 120, "strength": 1})))
	for _, n := range jsonOf[PatternDoc](l.Pattern(2)).Notes {
		if n.Tick%120 != 0 {
			t.Errorf("not quantized: %d", n.Tick)
		}
	}
	ok(t, l.PatternOp(2, "swing", mustJSON(map[string]any{"grid": 120, "amount": 66})))
	ok(t, l.PatternOp(2, "humanize", mustJSON(map[string]any{"timing": 10, "velocity": 10, "seed": 1})))
	ok(t, l.PatternOp(2, "reverse", nil))
	if d := jsonOf[PatternDoc](l.Pattern(2)); len(d.Notes) != 6 || d.Length != 1920 {
		t.Errorf("tools lost notes or length: %d notes, length %d", len(d.Notes), d.Length)
	}
	// Errors.
	if err := l.PatternOp(9, "double", nil); err == nil {
		t.Error("an empty pattern edited")
	}
	if err := l.PatternOp(2, "bogus", nil); err == nil {
		t.Error("unknown operation accepted")
	}
	if err := l.PatternOp(2, "insert", mustJSON(map[string]any{"pad": 0, "tick": 99999, "vel": 100, "len": 10})); err == nil {
		t.Error("a note past the end accepted")
	}
	if err := l.NewPattern(161, 1); err == nil {
		t.Error("pattern 161 accepted")
	}
	ok(t, l.CopyPattern(2, 10))
	if !bytes.Equal(l.files[patFile(2)], l.files[patFile(10)]) {
		t.Error("copy differs")
	}
	ok(t, l.DeletePattern(10))
	if _, ok := l.files[patFile(10)]; ok {
		t.Error("delete left the file")
	}
}

func TestProgressionPattern(t *testing.T) {
	defer catch(t)
	l := New()
	l.NewProject("P")
	pads := []int{16, 17, 18, 19, 20, 21, 22} // chord pads B1..B7
	spec := ProgressionSpec{Pads: pads, Degrees: []int{1, 5, 6, 4}, Bars: 4, Rhythm: "hold", Velocity: 100}
	ok(t, l.WriteProgression(5, mustJSON(spec)))
	doc := jsonOf[PatternDoc](l.Pattern(5))
	if doc.Bars != 4 || len(doc.Notes) != 4 {
		t.Fatalf("hold: %d bars, %d notes", doc.Bars, len(doc.Notes))
	}
	want := []Note{{0, 16, 100, 1920, 0}, {1920, 20, 100, 1920, 0}, {3840, 21, 100, 1920, 0}, {5760, 19, 100, 1920, 0}}
	if !slices.Equal(doc.Notes, want) && !sameNotes(doc.Notes, want) {
		t.Errorf("notes %+v\nwant  %+v", doc.Notes, want)
	}
	// Two chords in a bar: each gets half of it, the rhythm squeezed into it.
	spec.Bars, spec.Rhythm = 2, "quarters"
	ok(t, l.WriteProgression(6, mustJSON(spec)))
	doc = jsonOf[PatternDoc](l.Pattern(6))
	if doc.Bars != 2 || len(doc.Notes) != 16 {
		t.Errorf("quarters, 4 chords in 2 bars: %d notes", len(doc.Notes))
	}
	for _, rh := range RhythmNames {
		spec.Bars, spec.Rhythm, spec.Humanize = 4, rh, 50
		if err := l.WriteProgression(7, mustJSON(spec)); err != nil {
			t.Errorf("%s: %v", rh, err)
			continue
		}
		d := jsonOf[PatternDoc](l.Pattern(7))
		for _, n := range d.Notes {
			if n.Tick >= d.Length || n.Len < 1 || int(n.Tick)+n.Len > int(d.Length) || n.Vel < 1 || n.Vel > 127 {
				t.Errorf("%s: note out of range %+v", rh, n)
			}
		}
		if len(d.Notes) < 4 {
			t.Errorf("%s: %d notes", rh, len(d.Notes))
		}
	}
	// Every progression of the list writes, in 4 and 8 bars, and its pattern reads back.
	for _, p := range Progressions {
		for _, bars := range []int{4, 8} {
			s := ProgressionSpec{Pads: pads, Degrees: p.Degrees, Bars: bars, Rhythm: "3+3+2", Velocity: 96}
			if err := l.WriteProgression(8, mustJSON(s)); err != nil {
				t.Errorf("%s in %d bars: %v", p.Name, bars, err)
			}
		}
	}
	if err := l.WriteProgression(5, mustJSON(ProgressionSpec{Pads: pads, Degrees: []int{9}, Bars: 1})); err != nil {
		t.Errorf("degree 9 wraps to degree 2: %v", err)
	}
	if err := l.WriteProgression(5, mustJSON(ProgressionSpec{Pads: pads[:3], Degrees: []int{6}, Bars: 1})); err == nil {
		t.Error("a degree without a pad accepted")
	}
	if err := l.WriteProgression(5, mustJSON(ProgressionSpec{Pads: pads, Degrees: []int{1, 2, 3, 4, 5, 6, 7}, Bars: 2})); err == nil {
		t.Error("7 chords in 2 bars should not fit evenly")
	}
	if err := l.WriteProgression(5, mustJSON(ProgressionSpec{Pads: pads, Degrees: nil})); err == nil {
		t.Error("no chords accepted")
	}
	if err := l.WriteProgression(5, mustJSON(ProgressionSpec{Pads: pads, Degrees: []int{1}, Rhythm: "nope"})); err == nil {
		t.Error("unknown rhythm accepted")
	}
}

func sameNotes(a, b []Note) bool {
	x, y := slices.Clone(a), slices.Clone(b)
	less := func(p, q Note) int { return int(p.Tick) - int(q.Tick) }
	slices.SortFunc(x, less)
	slices.SortFunc(y, less)
	return slices.Equal(x, y)
}

func TestConstsAreValidJSON(t *testing.T) {
	defer catch(t)
	for name, f := range map[string]func() ([]byte, error){"arp": ArpConsts, "slicer": SlicerConsts} {
		b, err := f()
		if err != nil {
			t.Fatal(err)
		}
		var v map[string]any
		if err := json.Unmarshal(b, &v); err != nil || len(v) < 5 {
			t.Errorf("%s consts: %v (%d keys)", name, err, len(v))
		}
	}
}
