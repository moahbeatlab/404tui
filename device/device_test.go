package device_test

import (
	"bytes"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/moahbeatlab/404tui/device"
	"github.com/moahbeatlab/404tui/proto"
	"github.com/moahbeatlab/404tui/proto/fileapi"
	"github.com/moahbeatlab/404tui/sim"
)

func connect(t *testing.T) (*device.Device, *sim.Sim) {
	t.Helper()
	s := sim.New()
	d := device.New(s, "sim")
	t.Cleanup(func() { d.Close() })
	return d, s
}

func pad(t *testing.T, name string) proto.PadIndex {
	t.Helper()
	p, err := proto.ParsePad(name)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func check(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestStatusAndProjects(t *testing.T) {
	d, s := connect(t)
	st, err := d.Status()
	check(t, err)
	if st.Project != 0 || !st.HasSelected || st.Selected.String() != "A1" || st.Settings.Name() != "PROJECT_01" {
		t.Errorf("status = %+v (%s)", st, st.Settings.Name())
	}
	names, err := d.ProjectNames()
	check(t, err)
	if len(names) != 16 || names[5] != "PROJECT_06" {
		t.Errorf("names = %v", names)
	}

	check(t, d.SelectProject(5))
	if current, err := d.CurrentProject(); err != nil || current != 5 {
		t.Errorf("current project = %d (%v)", current, err)
	}
	var refused device.RefusedError
	if err := d.SelectProject(20); !errors.As(err, &refused) {
		t.Errorf("selecting project 21: %v", err)
	}

	check(t, d.SelectPad(pad(t, "C7")))
	if bank, p := s.Selected(); bank != 2 || p != 6 {
		t.Errorf("device selected bank %d pad %d", bank, p)
	}
	a, err := d.Activity()
	check(t, err)
	if a.Project != 5 || a.Selected.String() != "C7" || a.Playing {
		t.Errorf("activity = %+v", a)
	}
}

func TestNotificationsBecomeEvents(t *testing.T) {
	d, s := connect(t)
	s.Notify([]byte{proto.ReplyChanged, 3, 0, 0, 0})
	// A transaction in between must set the notification aside, not lose it.
	_, err := d.CurrentProject()
	check(t, err)
	events := d.TakeEvents()
	if len(events) != 1 || !bytes.Equal(events[0].Payload(), []byte{proto.ReplyChanged, 3, 0, 0, 0}) {
		t.Errorf("events = %+v", events)
	}
	if again := d.TakeEvents(); len(again) != 0 {
		t.Errorf("events delivered twice: %+v", again)
	}
}

func TestPadsAndParams(t *testing.T) {
	d, s := connect(t)
	b3 := pad(t, "B3")
	s.SetSample(b3, "kick", 4096)

	block, err := d.PadBlock(b3)
	check(t, err)
	if !block.HasSample() || block.Name() != "kick" || block.FileSize() != 4096 {
		t.Errorf("block = %q, %d bytes", block.Name(), block.FileSize())
	}
	blocks, err := d.PadBlocks()
	check(t, err)
	if len(blocks) != proto.PadCount || blocks[b3.Index()].Name() != "kick" || blocks[0].HasSample() {
		t.Errorf("read %d blocks", len(blocks))
	}

	level, _ := proto.ParamByName("level")
	check(t, d.SetParam(level.ID, proto.TargetPad(b3), 99))
	check(t, d.SetSampleName(b3, "snare"))
	check(t, d.SetChopPoints(b3, []uint32{100, 200}))
	block, err = d.PadBlock(b3)
	check(t, err)
	if v, _ := block.Param(level.ID); v != 99 || block.Name() != "snare" {
		t.Errorf("level = %d, name = %q", v, block.Name())
	}
	if chops := block.ChopPoints(); chops[0] != 100 || chops[1] != 200 || chops[2] >= 0 {
		t.Errorf("chop points = %v", chops)
	}
	if err := d.SetChopPoints(b3, make([]uint32, 17)); err == nil {
		t.Error("17 chop points accepted")
	}

	peaks, err := d.Peaks(b3, 1, 0, 8, 256)
	check(t, err)
	if len(peaks) != 8 || peaks[3] != (proto.Peak{Min: -3, Max: 3}) {
		t.Errorf("peaks = %v", peaks)
	}
	check(t, d.Preview(b3, 0))
}

func TestPadOperations(t *testing.T) {
	d, s := connect(t)
	a1, a2, a3 := pad(t, "A1"), pad(t, "A2"), pad(t, "A3")
	s.SetSample(a1, "one", 1000)
	s.SetSample(a2, "two", 2000)

	check(t, d.MoveSample(a1, a2, proto.MoveExchange))
	one, _ := d.PadBlock(a2)
	two, _ := d.PadBlock(a1)
	if one.Name() != "one" || two.Name() != "two" {
		t.Errorf("after exchange: A1 %q, A2 %q", two.Name(), one.Name())
	}
	check(t, d.MoveSample(a2, a3, proto.MoveOverwrite))
	if b, _ := d.PadBlock(a2); b.HasSample() {
		t.Error("overwrite left the source in place")
	}

	// The device acknowledges an edit of an empty pad and then reports the failure.
	err := d.MoveSample(a2, a3, proto.MoveOverwrite)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("moving an empty pad: %v", err)
	}
	check(t, d.PadOp(proto.PadOpNormalize, a3))
	check(t, d.PadOp(proto.PadOpTruncate, a3))
	check(t, d.PadOp(proto.PadOpDelete, a3))
	if b, _ := d.PadBlock(a3); b.HasSample() {
		t.Error("delete left the sample")
	}
	if err := d.PadOp(proto.PadOpDelete, a3); err == nil {
		t.Error("deleting an empty pad succeeded")
	}
}

func TestFiles(t *testing.T) {
	d, s := connect(t)
	// Larger than one write batch and one bulk read, and not a multiple of the chunk size.
	big := make([]byte, fileapi.Chunk*fileapi.WriteBatch*2+1234)
	for i := range big {
		big[i] = byte(i * 7)
	}
	var steps []uint64
	check(t, d.Mkdir("SD:IMPORT"))
	check(t, d.WriteFileWith("SD:IMPORT/big.wav", big, func(done, total uint64) { steps = append(steps, done) }))
	if stored, _ := s.File("SD:IMPORT/big.wav"); !bytes.Equal(stored, big) {
		t.Fatalf("device holds %d bytes, wrote %d", len(stored), len(big))
	}
	if len(steps) != 3 || steps[2] != uint64(len(big)) {
		t.Errorf("write progress = %v", steps)
	}

	got, err := d.ReadFile("SD:IMPORT/big.wav")
	check(t, err)
	if !bytes.Equal(got, big) {
		t.Fatalf("read back %d bytes, want %d", len(got), len(big))
	}
	prefix, exists, err := d.ReadPrefix("SD:IMPORT/big.wav", 16)
	if err != nil || !exists || !bytes.Equal(prefix, big[:16]) {
		t.Errorf("prefix = % x (%v, %v)", prefix, exists, err)
	}
	if _, exists, err := d.ReadPrefix("SD:IMPORT/none.wav", 16); err != nil || exists {
		t.Errorf("prefix of a missing file: %v, %v", exists, err)
	}

	st, exists, err := d.Stat("SD:IMPORT/big.wav")
	if err != nil || !exists || int(st.Size) != len(big) || st.IsDir() {
		t.Errorf("stat = %+v (%v, %v)", st, exists, err)
	}
	if st, _, _ := d.Stat("SD:IMPORT"); !st.IsDir() {
		t.Error("IMPORT is not a directory")
	}

	check(t, d.Mkdir("SD:IMPORT/pack"))
	if err := d.Mkdir("SD:IMPORT/pack"); err == nil {
		t.Error("mkdir over an existing directory succeeded")
	}
	check(t, d.Rename("SD:IMPORT/big.wav", "SD:IMPORT/pack/kick.wav"))
	entries, err := d.ListDir("SD:IMPORT")
	check(t, err)
	if len(entries) != 1 || entries[0].Name != "pack" || !entries[0].IsDir() {
		t.Errorf("IMPORT holds %+v", entries)
	}
	entries, err = d.ListDir("SD:IMPORT/pack")
	check(t, err)
	if len(entries) != 1 || entries[0].Name != "kick.wav" || entries[0].IsDir() {
		t.Errorf("pack holds %+v", entries)
	}
	if err := d.Rmdir("SD:IMPORT/pack"); err == nil {
		t.Error("rmdir of a directory with files succeeded")
	}
	check(t, d.Unlink("SD:IMPORT/pack/kick.wav"))
	check(t, d.Rmdir("SD:IMPORT/pack"))
	if _, err := d.ListDir("SD:nowhere"); err == nil {
		t.Error("listing a missing directory succeeded")
	}
	if free, err := d.FreeKb(); err != nil || free == 0 {
		t.Errorf("free = %d (%v)", free, err)
	}

	// The device's own storage, where projects live.
	s.AddFile("ROLAND/SP-404MKII/PROJECT_01/PADCONF.BIN", []byte("x"))
	entries, err = d.ListDir("ROLAND/SP-404MKII")
	check(t, err)
	if len(entries) != 1 || entries[0].Name != "PROJECT_01" {
		t.Errorf("projects folder holds %+v", entries)
	}
}

func TestImportFollowsTheOfficialSequence(t *testing.T) {
	d, s := connect(t)
	j16 := pad(t, "J16")
	smp := make([]byte, 512+6000)
	for i := range smp {
		smp[i] = byte(i)
	}
	if err := d.ImportSMP(2, j16, smp, "hat"); err == nil {
		t.Fatal("import into a project that is not current succeeded")
	}
	if err := d.ImportSMP(1, j16, smp[:100], "hat"); err == nil {
		t.Fatal("import of a truncated SMP succeeded")
	}
	s.Requests = nil
	check(t, d.ImportSMP(1, j16, smp, "hat"))

	if stored, _ := s.File(j16.SamplePath(1)); !bytes.Equal(stored, smp) {
		t.Errorf("device holds %d bytes of sample, want %d", len(stored), len(smp))
	}
	block, err := d.PadBlock(j16)
	check(t, err)
	if start, end := block.StartEndBytes(); block.Name() != "hat" || start != 512 || end != uint32(len(smp)) {
		t.Errorf("pad block: %q %d..%d", block.Name(), start, end)
	}
	// Status poll, pad block read, begin, release slot, stop, pad block write, commit, end.
	var sequence []byte
	for _, p := range s.Requests[:8] {
		sequence = append(sequence, p[0])
	}
	if want := []byte{0xFE, 0x9E, 0xFB, 0x91, 0x9D, 0x1E, 0x9F, 0xFC}; !bytes.Equal(sequence, want) {
		t.Errorf("control sequence = % x, want % x", sequence, want)
	}
}

func TestInitAndRestore(t *testing.T) {
	d, s := connect(t)
	a1, b1 := pad(t, "A1"), pad(t, "B1")
	s.SetSample(a1, "one", 1000)
	s.SetSample(b1, "two", 1000)
	s.SetPattern(a1, b1)

	if err := d.InitProject(3, proto.InitAll); err == nil {
		t.Fatal("init of a project that is not current succeeded")
	}
	check(t, d.InitProject(0, proto.InitSamplesBank(1)))
	if a, _ := d.PadBlock(a1); !a.HasSample() {
		t.Error("init of bank B cleared bank A")
	}
	if b, _ := d.PadBlock(b1); b.HasSample() {
		t.Error("init of bank B left its sample")
	}
	if exists, _ := d.PatternExists(a1); !exists {
		t.Error("init of samples cleared a pattern")
	}
	check(t, d.InitProject(0, proto.InitAllPatterns))
	if exists, _ := d.PatternExists(a1); exists {
		t.Error("init of patterns left one")
	}

	files := []device.File{{Name: "PADCONF.BIN", Data: []byte("conf")}, {Name: "SMPL/BANK1-01.SMP", Data: []byte("smp")}}
	var seen []string
	if err := d.RestoreProject(3, files, nil); err == nil {
		t.Fatal("restore into a project that is not current succeeded")
	}
	check(t, d.RestoreProject(0, files, func(i int, name string) { seen = append(seen, name) }))
	if !slices.Equal(seen, []string{"PADCONF.BIN", "SMPL/BANK1-01.SMP"}) {
		t.Errorf("progress = %v", seen)
	}
	if data, _ := s.File("ROLAND/SP-404MKII/PROJECT_01/SMPL/BANK1-01.SMP"); string(data) != "smp" {
		t.Errorf("restored sample = %q", data)
	}
}

func TestProjectRename(t *testing.T) {
	d, s := connect(t)
	var unsupported device.UnsupportedError
	if err := d.SetProjectName(4, "NEW"); !errors.As(err, &unsupported) {
		t.Errorf("renaming another project: %v", err)
	}
	if err := d.SetProjectName(0, "NEW"); !errors.As(err, &unsupported) {
		t.Errorf("renaming an empty project: %v", err)
	}
	padconf := "ROLAND/SP-404MKII/PROJECT_01/PADCONF.BIN"
	s.AddFile(padconf, []byte{0, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0})
	if err := d.SetProjectName(0, "NEW"); !errors.As(err, &unsupported) {
		t.Errorf("renaming a project from older firmware: %v", err)
	}
	s.AddFile(padconf, []byte{0, 0, 0, 0, 0, 0, 0, 0, 3, 0, 0, 0})
	check(t, d.SetProjectName(0, "NEW"))
	if names, _ := d.ProjectNames(); names[0] != "NEW" {
		t.Errorf("names = %v", names)
	}
}

// buffer is a file in memory for the device to render into.
type buffer struct {
	data []byte
	pos  int
}

func (b *buffer) Write(p []byte) (int, error) {
	if grow := b.pos + len(p) - len(b.data); grow > 0 {
		b.data = append(b.data, make([]byte, grow)...)
	}
	copy(b.data[b.pos:], p)
	b.pos += len(p)
	return len(p), nil
}

func (b *buffer) Seek(offset int64, whence int) (int64, error) {
	if whence != io.SeekStart {
		return 0, errors.New("unsupported seek")
	}
	b.pos = int(offset)
	return offset, nil
}

func TestRenderServesTheDevicesFileRequests(t *testing.T) {
	d, s := connect(t)
	a1, c8 := pad(t, "A1"), pad(t, "C8")
	s.SetPattern(a1, c8)

	pads, err := d.PatternPads(a1)
	check(t, err)
	if !slices.Equal(pads, []proto.PadIndex{c8}) {
		t.Errorf("pattern pads = %v", pads)
	}

	for name, render := range map[string]func(io.WriteSeeker) error{
		"bounce": func(out io.WriteSeeker) error { return d.BouncePattern(a1, out) },
		"pad":    func(out io.WriteSeeker) error { return d.RenderPatternPad(a1, c8, out) },
	} {
		var out buffer
		s.HostStatuses = nil
		check(t, render(&out))
		want := bytes.Clone(s.RenderedWAV)
		copy(want[4:], []byte{byte(len(want) - 8), 0, 0, 0})
		if !bytes.Equal(out.data, want) {
			t.Errorf("%s: rendered % x, want % x", name, out.data, want)
		}
		// Bytes written, new offset, bytes written.
		if !slices.Equal(s.HostStatuses, []int32{int32(len(want)), 4, 4}) {
			t.Errorf("%s: host answered %v", name, s.HostStatuses)
		}
	}
}

func TestScreens(t *testing.T) {
	d, _ := connect(t)
	path, err := device.ScreenPath(6, "startup_2")
	if err != nil || path != "ROLAND/SP-404MKII/PROJECT_06/PICTURE/startup_2.bmp" {
		t.Errorf("path = %q (%v)", path, err)
	}
	if len(device.ScreenSlots()) != 6 {
		t.Errorf("slots = %v", device.ScreenSlots())
	}
	for _, slot := range []string{"../PADCONF.BIN", "startup_3"} {
		if _, err := device.ScreenPath(6, slot); err == nil {
			t.Errorf("slot %q accepted", slot)
		}
	}

	if _, exists, err := d.ReadScreen(1, "startup_1"); err != nil || exists {
		t.Errorf("missing screen: %v, %v", exists, err)
	}
	check(t, d.WriteScreen(1, "startup_1", []byte("BM...")))
	bmp, exists, err := d.ReadScreen(1, "startup_1")
	if err != nil || !exists || string(bmp) != "BM..." {
		t.Errorf("screen = %q (%v, %v)", bmp, exists, err)
	}
}

func TestClosedLinkIsReported(t *testing.T) {
	s := sim.New()
	d := device.New(s, "sim")
	if d.PortName() != "sim" {
		t.Errorf("port name = %q", d.PortName())
	}
	check(t, d.Close())
	if _, err := d.CurrentProject(); !errors.Is(err, device.ErrClosed) {
		t.Errorf("after close: %v", err)
	}
}

func TestSilenceTimesOut(t *testing.T) {
	d, _ := connect(t)
	// Channel 9 is nothing the device answers on.
	replies, err := d.RawExchange(proto.Short(proto.Channel(9), []byte{1}), 30_000_000)
	if err != nil || len(replies) != 0 {
		t.Errorf("raw exchange = %+v (%v)", replies, err)
	}
	replies, err = d.RawExchange(proto.Short(proto.ChannelControl, proto.StatusRequest()), 100_000_000)
	if err != nil || len(replies) != 1 || replies[0].Payload()[0] != proto.ReplyStatus {
		t.Errorf("raw status = %+v (%v)", replies, err)
	}
}
