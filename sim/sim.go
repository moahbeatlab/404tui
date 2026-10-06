// Package sim is an in-memory stand-in for an SP-404MKII: it speaks the serial protocol
// over a device.Port, so the layers above can run and be tested without the hardware.
//
// It follows docs/re/ as far as the host can tell; timing and anything the specs leave
// open are simplified.
package sim

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/moahbeatlab/404tui/proto"
	"github.com/moahbeatlab/404tui/proto/fileapi"
	"github.com/moahbeatlab/404tui/proto/septet"
)

const (
	blockLen   = 327
	nameOffset = 175
	nameLen    = 24
	chopOffset = 199

	// Device error "not found", as a 32 code:i32 report.
	errNotFound int32 = 14 - 130
)

type handle struct {
	path string
	pos  int
	// Directory handles list entries instead of bytes.
	entries [][]byte
	// Bytes written since the last acknowledged batch.
	batch int
}

// Sim is the simulated device. Read and Write are the host's end of the serial link.
type Sim struct {
	mu     sync.Mutex
	wake   chan struct{}
	out    []byte
	closed bool
	dec    proto.Decoder

	project  uint8
	bank     uint8
	pad      uint8
	names    [16]string
	blocks   [proto.PadCount][]byte
	patterns map[uint16][]proto.PadIndex
	files    map[string][]byte
	dirs     map[string]bool
	handles  map[int32]*handle
	next     int32

	// Requests holds every control payload the host sent, in order.
	Requests [][]byte
	// HostStatuses holds the results the host reported for the device's own file requests.
	HostStatuses []int32
	// RenderedWAV is what a bounce or pad render writes into the host's file.
	RenderedWAV []byte
}

func New() *Sim {
	s := &Sim{
		wake:     make(chan struct{}, 1),
		patterns: map[uint16][]proto.PadIndex{},
		files:    map[string][]byte{},
		dirs:     map[string]bool{},
		handles:  map[int32]*handle{},
		next:     0x2b,
	}
	for i := range s.names {
		s.names[i] = fmt.Sprintf("PROJECT_%02d", i+1)
	}
	for i := range s.blocks {
		s.blocks[i] = emptyBlock(uint16(i))
	}
	s.RenderedWAV = append([]byte("RIFF\x00\x00\x00\x00WAVEfmt "), make([]byte, 28)...)
	s.RenderedWAV = append(s.RenderedWAV, 1, 2, 3, 4, 5, 6, 7, 8)
	return s
}

func emptyBlock(pad uint16) []byte {
	b := make([]byte, blockLen)
	b[0] = proto.ReplyPadBlock
	binary.LittleEndian.PutUint16(b[1:], pad)
	copy(b[nameOffset:nameOffset+nameLen-1], bytes.Repeat([]byte{' '}, nameLen-1))
	copy(b[chopOffset:chopOffset+proto.PadBlockChopPoints*4], bytes.Repeat([]byte{0xff}, proto.PadBlockChopPoints*4))
	return b
}

// ---- setup and inspection, for whoever drives the simulation ----

// AddFile puts a file on the device, creating its parent directories.
func (s *Sim) AddFile(path string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.put(remote(path), data)
}

func (s *Sim) put(remote string, data []byte) {
	s.files[remote] = data
	for dir := parent(remote); dir != "" && !s.dirs[dir]; dir = parent(dir) {
		s.dirs[dir] = true
	}
}

// File returns a file's contents.
func (s *Sim) File(path string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.files[remote(path)]
	return data, ok
}

// SetSample makes a pad of the current project hold a sample of size bytes.
func (s *Sim) SetSample(pad proto.PadIndex, name string, size uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.blocks[pad.Index()]
	binary.LittleEndian.PutUint32(b[3:], size)
	setName(b, name)
	s.put(remote(pad.SamplePath(s.project+1)), make([]byte, size))
}

// SetPattern makes a pattern slot exist, playing pads.
func (s *Sim) SetPattern(slot proto.PadIndex, pads ...proto.PadIndex) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.patterns[slot.Index()] = pads
}

// Selected is the bank and pad last selected.
func (s *Sim) Selected() (bank, pad uint8) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bank, s.pad
}

// Notify sends an unsolicited control message, as the device does when something changes
// on its own panel.
func (s *Sim) Notify(payload []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.control(payload)
	s.signal()
}

func setName(block []byte, name string) {
	field := block[nameOffset : nameOffset+nameLen]
	clear(field)
	copy(field[:nameLen-1], name)
}

// remote is a path as the simulated filesystem keys it: resolved, with single slashes and
// no trailing one.
func remote(path string) string {
	return clean(fileapi.RemotePath(path))
}

func clean(path string) string {
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	return strings.TrimRight(path, "/")
}

func parent(path string) string {
	path = strings.TrimRight(path, "/")
	return strings.TrimRight(path[:strings.LastIndexByte(path, '/')+1], "/")
}

// ---- the serial link ----

func (s *Sim) Read(p []byte) (int, error) {
	timeout := time.NewTimer(20 * time.Millisecond)
	defer timeout.Stop()
	for {
		s.mu.Lock()
		if len(s.out) > 0 {
			n := copy(p, s.out)
			s.out = s.out[n:]
			s.mu.Unlock()
			return n, nil
		}
		closed := s.closed
		s.mu.Unlock()
		if closed {
			return 0, io.EOF
		}
		select {
		case <-s.wake:
		case <-timeout.C:
			return 0, nil
		}
	}
}

func (s *Sim) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, io.ErrClosedPipe
	}
	s.dec.Push(p)
	for {
		m, ok := s.dec.Next()
		if !ok {
			break
		}
		switch {
		case m.Channel == proto.ChannelControl:
			s.Requests = append(s.Requests, bytes.Clone(m.Payload()))
			s.handleControl(m.Payload())
		case m.Channel == proto.ChannelFile && !m.IsShort():
			s.handleFile(m.Payload())
		}
	}
	s.signal()
	return len(p), nil
}

func (s *Sim) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.signal()
	return nil
}

func (s *Sim) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Sim) emit(m proto.Message) {
	m.Source, m.Destination = proto.AddrDevice, proto.AddrHost
	s.out = append(s.out, m.Encode()...)
}

// control sends a control payload, short when it fits.
func (s *Sim) control(payload []byte) {
	if len(payload) <= proto.ShortMax {
		s.emit(proto.Short(proto.ChannelControl, payload))
	} else {
		s.emit(proto.Long(proto.ChannelControl, payload))
	}
}

func (s *Sim) changed() {
	s.control([]byte{proto.ReplyChanged, s.project, 0, 0, 0})
}

func (s *Sim) deviceError(code int32) {
	s.control(binary.LittleEndian.AppendUint32([]byte{proto.ReplyError}, uint32(code)))
}

// ---- channel 5 ----

func (s *Sim) handleControl(p []byte) {
	if len(p) == 0 {
		return
	}
	u16 := func(at int) uint16 {
		if at+2 > len(p) {
			return 0
		}
		return binary.LittleEndian.Uint16(p[at:])
	}
	pad := u16(1)
	padReply := func(cmd byte, status ...byte) {
		s.control(append([]byte{cmd, p[1], p[2]}, status...))
	}
	// Commands addressed to a pad need a valid one; the rest need at least one argument.
	forPad := bytes.IndexByte([]byte{0x8E, 0x8F, 0x95, 0x9E, 0xAB, 0x98, 0xA3, 0x91, 0x9F, 0xB7, 0xB8, 0x02, 0x1E}, p[0]) >= 0
	if forPad && (len(p) < 3 || pad >= proto.PadCount) {
		return
	}
	if len(p) < 2 && p[0] != 0x9D {
		return
	}
	switch p[0] {
	case 0xFE:
		status := []byte{proto.ReplyStatus, s.project, 0, s.bank, 0, s.pad, 0, 0, 0xff, 0xff, 0xff, 0xff}
		s.emit(proto.Long(proto.ChannelControl, status))
	case 0xFD:
		s.control(s.settings(p[1]))
		for i, name := range s.names {
			s.emit(proto.Long(proto.ChannelControl, append([]byte{proto.ReplyProjectName, byte(i)}, name...)))
		}
	case 0x9A:
		if p[1] >= 16 {
			s.control([]byte{proto.ReplyChanged, 1})
			return
		}
		s.control([]byte{proto.ReplyChanged, 0})
		s.saveProject()
		s.project = p[1]
		s.loadProject()
		s.changed()
	case 0x8E:
		padReply(proto.ReplyPreviewStart, 0)
	case 0x8F:
		padReply(proto.ReplyPreviewStop, 0)
	case 0x95:
		s.control([]byte{proto.ReplyMoveSample, 0})
		s.moveSample(pad, u16(3), u16(5))
	case 0x9E:
		s.control(s.blocks[pad])
	case 0xAB:
		_, exists := s.patterns[pad]
		padReply(proto.ReplyPatternExists, boolByte(exists))
	case 0x9D:
		s.control([]byte{proto.ReplyStop, 0})
	case proto.CmdSetParam:
		s.setParam(p)
	case 0x9C:
		s.peaks(p)
	case proto.CmdSampleName:
		setName(s.blocks[pad], string(p[3:]))
		s.control(p)
	case proto.CmdProjectName:
		// The device renames the current project whatever number it is sent.
		s.names[s.project] = string(p[2:])
		s.control(p)
	case proto.CmdPadBlock:
		copy(s.blocks[pad], p[:min(len(p), proto.PadBlockWriteLen)])
		s.control([]byte{proto.ReplyPadBlock, 0})
	case 0x98:
		s.padEdit(pad, proto.ReplyTruncate, false)
	case 0xA3:
		s.padEdit(pad, proto.ReplyNormalize, false)
	case 0x91:
		// 91 pad 01 00 only frees the slot for an import; 91 pad 00 00 deletes the sample.
		if len(p) > 3 && p[3] == 1 {
			s.control([]byte{proto.ReplyPadSlot, 0})
			return
		}
		s.padEdit(pad, proto.ReplyPadSlot, true)
	case 0x9F:
		padReply(proto.ReplyPadCommit, 0)
		s.changed()
	case 0x92:
		s.init(u16(1), u16(3), u16(5))
	case 0xB1:
		s.control([]byte{proto.RestoreReloadReply, 0})
		s.loadProject()
		s.control([]byte{proto.RestoreReloaded, 0, 0, 0, 0})
	case 0xB7:
		bitmap := make([]byte, proto.PadCount/8)
		for _, used := range s.patterns[pad] {
			bitmap[used.Index()/8] |= 1 << (used.Index() % 8)
		}
		head := []byte{proto.RenderPadsUsedReply, 0, byte(len(s.patterns[pad])), byte(len(bitmap))}
		s.control(append(head, bitmap...))
	case 0xB8:
		s.render(p, u16(3), u16(5))
	}
}

func boolByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}

func (s *Sim) settings(project byte) []byte {
	words := make([]uint32, proto.SettingsWords)
	words[0] = 12_000
	for i := 12; i < 22; i++ {
		words[i] = 12_000 << 1
	}
	p := []byte{proto.ReplyProjectSettings, project, 0}
	for _, w := range words {
		p = binary.LittleEndian.AppendUint32(p, w)
	}
	name := make([]byte, proto.SettingsNameLen)
	if int(project) < len(s.names) {
		copy(name, s.names[project])
	}
	return append(p, name...)
}

func (s *Sim) hasSample(pad uint16) bool {
	return binary.LittleEndian.Uint32(s.blocks[pad][3:]) != 0
}

func (s *Sim) samplePath(pad uint16) string {
	p, _ := proto.NewPadIndex(pad)
	return remote(p.SamplePath(s.project + 1))
}

func (s *Sim) clearPad(pad uint16) {
	s.blocks[pad] = emptyBlock(pad)
	delete(s.files, s.samplePath(pad))
}

func (s *Sim) padEdit(pad uint16, reply byte, remove bool) {
	s.control([]byte{reply, 0})
	if !s.hasSample(pad) {
		s.deviceError(errNotFound)
		return
	}
	if remove {
		s.clearPad(pad)
	}
	s.changed()
}

func (s *Sim) moveSample(from, to, mode uint16) {
	if to >= proto.PadCount || !s.hasSample(from) {
		s.deviceError(errNotFound)
		return
	}
	a, b := s.blocks[from], s.blocks[to]
	fa, fb := s.files[s.samplePath(from)], s.files[s.samplePath(to)]
	hadB := s.hasSample(to)
	s.clearPad(from)
	s.clearPad(to)
	place := func(pad uint16, block, file []byte) {
		binary.LittleEndian.PutUint16(block[1:], pad)
		s.blocks[pad] = block
		s.files[s.samplePath(pad)] = file
	}
	place(to, a, fa)
	if mode == uint16(proto.MoveExchange) && hadB {
		place(from, b, fb)
	}
	s.changed()
}

func (s *Sim) setParam(p []byte) {
	if len(p) < 9 {
		return
	}
	id, target := p[1], binary.LittleEndian.Uint16(p[3:])
	value := binary.LittleEndian.Uint32(p[5:])
	switch {
	case target == uint16(proto.TargetGlobal) && id == 0x00:
		s.bank, s.pad = byte(value), 0
	case target == uint16(proto.TargetGlobal) && id == 0x01:
		s.pad = byte(value)
	case target < proto.PadCount && id >= proto.ChopPointFirst && id < proto.ChopPointFirst+proto.PadBlockChopPoints:
		binary.LittleEndian.PutUint32(s.blocks[target][chopOffset+4*int(id-proto.ChopPointFirst):], value)
	case target < proto.PadCount && id >= 0x67 && id <= 0x7B:
		binary.LittleEndian.PutUint32(s.blocks[target][3+4*int(id-0x66):], value)
	case target < proto.PadCount && id >= 0x7D && id <= 0x7F:
		binary.LittleEndian.PutUint32(s.blocks[target][3+4*int(id-0x67):], value)
	}
	s.control(p)
}

func (s *Sim) peaks(p []byte) {
	if len(p) < 17 {
		return
	}
	points := binary.LittleEndian.Uint32(p[9:])
	reply := make([]byte, 16, 16+4*points)
	reply[0] = proto.ReplyPeaks
	copy(reply[1:4], p[2:5])
	for i := uint32(0); i < points; i++ {
		reply = binary.LittleEndian.AppendUint16(reply, uint16(-int16(i)))
		reply = binary.LittleEndian.AppendUint16(reply, uint16(i))
	}
	s.control(reply)
}

func (s *Sim) init(mode, option, bank uint16) {
	s.control([]byte{proto.ReplyInit, 0})
	if mode == 100 {
		// Import mode: erase the current project before a restore writes its files.
		s.eraseFolder()
		for i := range s.blocks {
			s.blocks[i] = emptyBlock(uint16(i))
		}
		return
	}
	inScope := func(pad uint16, all, oneBank uint16) bool {
		return option == 0 || option == all || (option == oneBank && pad/proto.PadsPerBank == bank)
	}
	for pad := uint16(0); pad < proto.PadCount; pad++ {
		if inScope(pad, 1, 2) {
			s.clearPad(pad)
		}
		if inScope(pad, 3, 4) {
			delete(s.patterns, pad)
		}
	}
	s.changed()
}

func (s *Sim) folder() string {
	return remote(fmt.Sprintf("ROLAND/SP-404MKII/PROJECT_%02d", s.project+1))
}

func (s *Sim) eraseFolder() {
	prefix := s.folder() + "/"
	for path := range s.files {
		if strings.HasPrefix(path, prefix) {
			delete(s.files, path)
		}
	}
}

// saveProject and loadProject keep each project's pad blocks in a file of its folder, so
// that switching projects switches pads.
func (s *Sim) saveProject() {
	s.put(s.folder()+"/.sim-blocks", bytes.Join(s.blocks[:], nil))
}

func (s *Sim) loadProject() {
	saved := s.files[s.folder()+"/.sim-blocks"]
	for i := range s.blocks {
		if len(saved) >= (i+1)*blockLen {
			s.blocks[i] = bytes.Clone(saved[i*blockLen : (i+1)*blockLen])
		} else {
			s.blocks[i] = emptyBlock(uint16(i))
		}
	}
}

// render answers a render control message; for a bounce or a pad render it then writes
// the WAV into the host's file, as the device does once it has finished rendering.
func (s *Sim) render(p []byte, op, hostHandle uint16) {
	if len(p) < 7 {
		return
	}
	s.control(append([]byte{proto.RenderControlReply}, p[1:5]...))
	if op == proto.RenderBegin || op == proto.RenderEnd {
		return
	}
	wav := s.RenderedWAV
	size := binary.LittleEndian.AppendUint32(nil, uint32(len(wav)-8))
	h := int32(hostHandle)
	s.hostRequest(fileapi.OpWrite, h, septet.EncodeLen(uint32(len(wav)), septet.FlagLast), wav)
	s.hostRequest(fileapi.OpSeek, h, septet.Encode(4), []byte{0})
	s.hostRequest(fileapi.OpWrite, h, septet.EncodeLen(4, septet.FlagLast), size)
	done := append([]byte{proto.RenderDoneReply}, p[1:7]...)
	s.control(append(done, 0, 0))
}

func (s *Sim) hostRequest(op fileapi.Op, h int32, arg [septet.Len]byte, tail []byte) {
	first := septet.Encode(h)
	body := append([]byte{0xF0, 0x41, 0x7A, 0x03, byte(op)}, first[:]...)
	body = append(body, arg[:]...)
	body = append(body, tail...)
	s.emit(proto.Long(proto.ChannelFile, append(body, 0xF7)))
}

// ---- channel 6 ----

func (s *Sim) status(result, extra int32) {
	r, e := septet.Encode(result), septet.Encode(extra)
	body := append([]byte{0xF0, 0x41, 0x7A, 0x7A}, r[:]...)
	body = append(body, e[:]...)
	s.emit(proto.Long(proto.ChannelFile, append(body, 0xF7)))
}

func (s *Sim) data(h int32, data []byte, last bool) {
	var flags byte
	if last {
		flags = septet.FlagLast
	}
	first, n := septet.Encode(h), septet.EncodeLen(uint32(len(data)), flags)
	body := append([]byte{0xF0, 0x41, 0x7A, 0x02}, first[:]...)
	body = append(body, n[:]...)
	body = append(body, data...)
	s.emit(proto.Long(proto.ChannelFile, append(body, 0xF7)))
}

func cstring(b []byte) string {
	if end := bytes.IndexByte(b, 0); end >= 0 {
		b = b[:end]
	}
	return string(b)
}

func (s *Sim) handleFile(p []byte) {
	if len(p) < 15 || p[len(p)-1] != 0xF7 {
		return
	}
	// The host's answer to one of the device's own requests.
	if p[3] == 0x7A {
		if result, err := septet.Decode(p[4:]); err == nil {
			s.HostStatuses = append(s.HostStatuses, result.Int32())
		}
		return
	}
	if len(p) < 16 {
		return
	}
	first, err1 := septet.Decode(p[5:])
	second, err2 := septet.Decode(p[10:])
	if err1 != nil || err2 != nil {
		return
	}
	if p[3] != 0x03 {
		return
	}
	tail := p[15 : len(p)-1]
	path := clean(cstring(tail))
	h := s.handles[first.Int32()]

	switch fileapi.Op(p[4]) {
	case fileapi.OpOpen:
		if uint32(first.Int32()) == fileapi.FlagCreateTruncateReadWrite {
			s.put(path, nil)
		} else if _, ok := s.files[path]; !ok {
			s.status(-1, 2)
			return
		}
		s.status(s.open(&handle{path: path}), 0)
	case fileapi.OpClose, fileapi.OpCloseDir:
		if h == nil {
			s.status(-1, 9)
			return
		}
		delete(s.handles, first.Int32())
		s.status(0, 0)
	case fileapi.OpRead:
		if h == nil {
			s.status(-1, 9)
			return
		}
		file := s.files[h.path]
		chunk := file[min(h.pos, len(file)):min(h.pos+int(second.Uint32()), len(file))]
		h.pos += len(chunk)
		for len(chunk) > fileapi.Chunk {
			s.data(first.Int32(), chunk[:fileapi.Chunk], false)
			chunk = chunk[fileapi.Chunk:]
		}
		s.data(first.Int32(), chunk, true)
	case fileapi.OpWrite:
		if h == nil {
			s.status(-1, 9)
			return
		}
		n := int(second.Uint32())
		if n > len(tail) {
			return
		}
		file := s.files[h.path]
		if grow := h.pos + n - len(file); grow > 0 {
			file = append(file, make([]byte, grow)...)
		}
		copy(file[h.pos:], tail[:n])
		s.files[h.path] = file
		h.pos += n
		h.batch += n
		if second.IsLast() {
			s.status(int32(h.batch), 0)
			h.batch = 0
		}
	case fileapi.OpSeek:
		if h == nil {
			s.status(-1, 9)
			return
		}
		h.pos = int(second.Uint32())
		s.status(int32(h.pos), 0)
	case fileapi.OpMkdir:
		if s.exists(path) {
			s.status(-1, 17)
			return
		}
		s.dirs[path] = true
		s.status(0, 0)
	case fileapi.OpUnlink:
		if _, ok := s.files[path]; !ok {
			s.status(-1, 2)
			return
		}
		delete(s.files, path)
		s.status(0, 0)
	case fileapi.OpRmdir:
		if !s.dirs[path] || len(s.children(path)) > 0 {
			s.status(-1, 39)
			return
		}
		delete(s.dirs, path)
		s.status(0, 0)
	case fileapi.OpOpenDir:
		if !s.dirs[path] && path != "" {
			s.status(-1, 2)
			return
		}
		entries := [][]byte{dirEntry(".", true), dirEntry("..", true)}
		for _, name := range s.children(path) {
			entries = append(entries, dirEntry(name, s.dirs[path+"/"+name]))
		}
		s.status(s.open(&handle{path: path, entries: entries}), 0)
	case fileapi.OpReadDir:
		if h == nil {
			s.status(-1, 9)
			return
		}
		if h.pos == len(h.entries) {
			// The card's filesystem closes the handle itself once readdir runs out.
			if !strings.HasPrefix(h.path+"/", clean(fileapi.Root)+"/") {
				delete(s.handles, first.Int32())
			}
			s.status(-1, 0)
			return
		}
		s.data(first.Int32(), h.entries[h.pos], true)
		h.pos++
	case fileapi.OpStat:
		file, isFile := s.files[path]
		switch {
		case isFile:
			s.data(0, stat(fileapi.ModeFile, len(file)), true)
		case s.dirs[path] || path == "":
			s.data(0, stat(fileapi.ModeDir, 0), true)
		default:
			s.status(-1, 2)
		}
	case fileapi.OpFStat:
		if h == nil {
			s.status(-1, 9)
			return
		}
		s.data(first.Int32(), stat(fileapi.ModeFile, len(s.files[h.path])), true)
	case fileapi.OpRename:
		if len(tail) < 400 {
			return
		}
		s.rename(clean(cstring(tail[:200])), clean(cstring(tail[200:400])))
	case fileapi.OpFreeKb:
		s.status(1_000_000, 0)
	}
}

func (s *Sim) open(h *handle) int32 {
	s.next++
	s.handles[s.next] = h
	return s.next
}

func (s *Sim) exists(path string) bool {
	_, isFile := s.files[path]
	return isFile || s.dirs[path]
}

// children lists the names directly inside a directory, sorted.
func (s *Sim) children(dir string) []string {
	seen := map[string]bool{}
	add := func(path string) {
		if rest, ok := strings.CutPrefix(path, dir+"/"); ok && rest != "" && !strings.Contains(rest, "/") {
			seen[rest] = true
		}
	}
	for path := range s.files {
		add(path)
	}
	for path := range s.dirs {
		add(path)
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (s *Sim) rename(from, to string) {
	if !s.exists(from) || s.exists(to) {
		s.status(-1, 2)
		return
	}
	if data, ok := s.files[from]; ok {
		delete(s.files, from)
		s.put(to, data)
	} else {
		for path, data := range s.files {
			if rest, ok := strings.CutPrefix(path, from+"/"); ok {
				delete(s.files, path)
				s.files[to+"/"+rest] = data
			}
		}
		for path := range s.dirs {
			if rest, ok := strings.CutPrefix(path, from+"/"); ok {
				delete(s.dirs, path)
				s.dirs[to+"/"+rest] = true
			}
		}
		delete(s.dirs, from)
		s.dirs[to] = true
	}
	s.status(0, 0)
}

func stat(mode uint32, size int) []byte {
	b := binary.LittleEndian.AppendUint32(nil, mode|0o777)
	return binary.LittleEndian.AppendUint32(b, uint32(size))
}

func dirEntry(name string, dir bool) []byte {
	kind := fileapi.DtReg
	if dir {
		kind = fileapi.DtDir
	}
	b := append([]byte{0, 0, 0, 0, byte(len(name)), kind}, name...)
	return append(b, 0)
}
