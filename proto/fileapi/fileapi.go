// Package fileapi implements channel 6: the remote file API on the SD card.
//
// Payloads are SysEx-shaped (F0 41 7A <cmd> … F7). Requests use command 0x03; the device
// answers with a status (0x7A) or a data reply (0x02).
package fileapi

import (
	"bytes"
	"encoding/binary"
	"strings"

	"github.com/moahbeatlab/404tui/proto"
	"github.com/moahbeatlab/404tui/proto/septet"
)

const (
	// Root is the prefix that routes a path to the device's own storage, where projects,
	// samples and the factory library live.
	Root = "/SP404REMOTE//"

	// Card marks a path as living on the SD card instead: SD:IMPORT/pack/kick.wav. The card
	// is a separate filesystem at the device's real root, holding IMPORT, EXPORT, BKUP and
	// the firmware images. The device's own IMPORT browser reads IMPORT from here.
	Card = "SD:"

	// Chunk is the largest data block in one write request or one read reply.
	Chunk = 0x5000

	// WriteBatch is the number of write chunks sent before waiting for the device's byte
	// count.
	WriteBatch = 15

	// BulkRead is the read size the official app uses for bulk reads.
	BulkRead uint32 = 0x20000

	end byte = 0xF7

	cmdRequest byte = 0x03
	cmdStatus  byte = 0x7A
	cmdData    byte = 0x02
)

var head = []byte{0xF0, 0x41, 0x7A}

type Op byte

const (
	OpOpen     Op = 0x00
	OpClose    Op = 0x03
	OpRead     Op = 0x04
	OpWrite    Op = 0x06
	OpSeek     Op = 0x07
	OpMkdir    Op = 0x09
	OpUnlink   Op = 0x0A
	OpRmdir    Op = 0x0B
	OpOpenDir  Op = 0x0C
	OpCloseDir Op = 0x0D
	OpReadDir  Op = 0x0E
	OpStat     Op = 0x12
	OpFStat    Op = 0x13
	OpRename   Op = 0x17
	OpFreeKb   Op = 0x18
)

// Open flags (newlib values).
const (
	FlagReadOnly                uint32 = 0
	FlagCreateTruncateReadWrite uint32 = 0x602
)

// Mode bits returned by stat.
const (
	ModeTypeMask uint32 = 0xF000
	ModeDir      uint32 = 0x4000
	ModeFile     uint32 = 0x8000
)

// Directory entry type values (Linux d_type).
const (
	DtDir byte = 4
	DtReg byte = 8
)

// RemotePath turns a path into one the device understands. Plain paths
// (ROLAND/SP-404MKII/PROJECT_06) address the device's own storage; a Card prefix addresses
// the SD card.
func RemotePath(path string) string {
	if strings.HasPrefix(path, Root) {
		return path
	}
	if rel, ok := strings.CutPrefix(path, Card); ok {
		return "/" + strings.TrimLeft(rel, `/\`)
	}
	return Root + "/" + strings.TrimLeft(path, `/\`)
}

// OnCard reports whether a path addresses the SD card.
func OnCard(path string) bool {
	return strings.HasPrefix(path, Card)
}

func request(op Op, first, second [septet.Len]byte, tail []byte) []byte {
	out := make([]byte, 0, 3+12+len(tail)+1)
	out = append(out, head...)
	out = append(out, cmdRequest, byte(op))
	out = append(out, first[:]...)
	out = append(out, second[:]...)
	out = append(out, tail...)
	return append(out, end)
}

func pathRequest(op Op, first int32, path string) []byte {
	b := append([]byte(RemotePath(path)), 0)
	return request(op, septet.Encode(first), septet.Encode(int32(len(b))), b)
}

func handleRequest(op Op, handle int32, arg [septet.Len]byte) []byte {
	return request(op, septet.Encode(handle), arg, []byte{0})
}

func Open(path string, flags uint32) []byte {
	return pathRequest(OpOpen, int32(flags), path)
}

func Close(handle int32) []byte {
	return handleRequest(OpClose, handle, septet.Encode(0))
}

func Read(handle int32, count uint32) []byte {
	var flags byte
	if count > Chunk {
		flags = septet.FlagMulti
	}
	return handleRequest(OpRead, handle, septet.EncodeLen(count, flags))
}

// Write builds one write chunk (at most Chunk bytes). lastInBatch asks the device to reply.
func Write(handle int32, data []byte, lastInBatch bool) []byte {
	var flags byte
	if lastInBatch {
		flags = septet.FlagLast
	}
	return request(OpWrite, septet.Encode(handle), septet.EncodeLen(uint32(len(data)), flags), data)
}

// Seek seeks to an absolute offset. The status result is the new offset.
func Seek(handle int32, offset uint32) []byte {
	return handleRequest(OpSeek, handle, septet.Encode(int32(offset)))
}

func FStat(handle int32) []byte {
	return handleRequest(OpFStat, handle, septet.Encode(0))
}

func Stat(path string) []byte {
	return pathRequest(OpStat, 0, path)
}

func Unlink(path string) []byte {
	return pathRequest(OpUnlink, 0, path)
}

// Mkdir creates a directory. Fails with −1 (extra 17, EEXIST) when the name is taken.
func Mkdir(path string) []byte {
	return pathRequest(OpMkdir, 0, path)
}

// Rmdir removes an empty directory.
func Rmdir(path string) []byte {
	return pathRequest(OpRmdir, 0, path)
}

func OpenDir(path string) []byte {
	return pathRequest(OpOpenDir, 0, path)
}

func ReadDir(dir int32) []byte {
	return handleRequest(OpReadDir, dir, septet.Encode(0))
}

func CloseDir(dir int32) []byte {
	return handleRequest(OpCloseDir, dir, septet.Encode(0))
}

func FreeKb(path string) []byte {
	return pathRequest(OpFreeKb, 0, path)
}

// Rename takes two paths, each NUL-padded into a 200-byte slot.
func Rename(from, to string) []byte {
	block := make([]byte, 400)
	for i, path := range []string{from, to} {
		b := RemotePath(path)
		copy(block[i*200:i*200+199], b)
	}
	return request(OpRename, septet.Encode(0), septet.Encode(400), block)
}

// Reply is a device answer: a StatusReply or a DataReply.
type Reply interface {
	IsFailure() bool
}

// StatusReply is the result of an operation: a handle, a new offset, a byte count, 0, or
// −1 on failure.
type StatusReply struct {
	Result, Extra int32
}

// DataReply is a block of data (read, stat, readdir).
type DataReply struct {
	Handle int32
	Last   bool
	Data   []byte
}

func (r StatusReply) IsFailure() bool { return r.Result == -1 }
func (r DataReply) IsFailure() bool   { return false }

// sysexBody strips the F0 41 7A … F7 envelope.
func sysexBody(payload []byte, need int) ([]byte, error) {
	if len(payload) < 5 || !bytes.HasPrefix(payload, head) || payload[len(payload)-1] != end {
		return nil, proto.ErrNotSysex
	}
	body := payload[3 : len(payload)-1]
	if len(body) < need {
		return nil, proto.TruncatedError{Need: need, Have: len(body)}
	}
	return body, nil
}

// tail returns the n data bytes that follow the fixed part of body.
func tail(body []byte, fixed int, n uint32) ([]byte, error) {
	if uint64(len(body)-fixed) < uint64(n) {
		return nil, proto.TruncatedError{Need: fixed + int(n), Have: len(body)}
	}
	return bytes.Clone(body[fixed : fixed+int(n)]), nil
}

func ParseReply(payload []byte) (Reply, error) {
	const need = 1 + 2*septet.Len
	body, err := sysexBody(payload, need)
	if err != nil {
		return nil, err
	}
	first, err := septet.Decode(body[1:])
	if err != nil {
		return nil, err
	}
	second, err := septet.Decode(body[1+septet.Len:])
	if err != nil {
		return nil, err
	}
	switch body[0] {
	case cmdStatus:
		return StatusReply{Result: first.Int32(), Extra: second.Int32()}, nil
	case cmdData:
		data, err := tail(body, need, second.Uint32())
		if err != nil {
			return nil, err
		}
		return DataReply{Handle: first.Int32(), Last: second.IsLast(), Data: data}, nil
	}
	return nil, proto.UnexpectedCommandError(body[0])
}

// HostRequest is a request the device sends to the host while it writes rendered audio
// into a host file (pattern Bounce/MULTIPAD export): a HostWrite or a HostSeek. Same
// layout as host requests, opposite direction.
type HostRequest interface {
	hostRequest()
}

type HostWrite struct {
	Handle int32
	Data   []byte
}

type HostSeek struct {
	Handle int32
	Offset uint32
}

func (HostWrite) hostRequest() {}
func (HostSeek) hostRequest()  {}

func ParseHostRequest(payload []byte) (HostRequest, error) {
	const need = 2 + 2*septet.Len
	body, err := sysexBody(payload, need)
	if err != nil {
		return nil, err
	}
	if body[0] != cmdRequest {
		return nil, proto.UnexpectedCommandError(body[0])
	}
	handle, err := septet.Decode(body[2:])
	if err != nil {
		return nil, err
	}
	arg, err := septet.Decode(body[2+septet.Len:])
	if err != nil {
		return nil, err
	}
	switch Op(body[1]) {
	case OpWrite:
		data, err := tail(body, need, arg.Uint32())
		if err != nil {
			return nil, err
		}
		return HostWrite{Handle: handle.Int32(), Data: data}, nil
	case OpSeek:
		return HostSeek{Handle: handle.Int32(), Offset: arg.Uint32()}, nil
	}
	return nil, proto.UnexpectedCommandError(body[1])
}

// HostAck is the first answer to a HostRequest (sent as a short message).
func HostAck() []byte {
	return []byte{0xF0, 0x41, 0x7A, 0x7C, 0x00, end}
}

// HostStatus is the second answer to a HostRequest: bytes written or the new offset (−1 on
// failure).
func HostStatus(result int32) []byte {
	out := make([]byte, 0, 15)
	out = append(out, head...)
	out = append(out, cmdStatus)
	r, m := septet.Encode(result), septet.Encode(-1)
	out = append(out, r[:]...)
	out = append(out, m[:]...)
	return append(out, end)
}

type FileStat struct {
	Mode uint32
	Size uint32
}

func ParseStat(data []byte) (FileStat, error) {
	if len(data) < 8 {
		return FileStat{}, proto.TruncatedError{Need: 8, Have: len(data)}
	}
	return FileStat{
		Mode: binary.LittleEndian.Uint32(data),
		Size: binary.LittleEndian.Uint32(data[4:]),
	}, nil
}

func (s FileStat) IsDir() bool {
	return s.Mode&ModeTypeMask == ModeDir
}

type DirEntry struct {
	Name string
	Kind byte
}

func ParseDirEntry(data []byte) (DirEntry, error) {
	if len(data) < 6 {
		return DirEntry{}, proto.TruncatedError{Need: 6, Have: len(data)}
	}
	n := int(data[4])
	if len(data) < 6+n {
		return DirEntry{}, proto.TruncatedError{Need: 6 + n, Have: len(data)}
	}
	return DirEntry{
		Name: strings.ToValidUTF8(string(data[6:6+n]), "�"),
		Kind: data[5],
	}, nil
}

func (e DirEntry) IsDir() bool {
	return e.Kind == DtDir
}

func (e DirEntry) IsDot() bool {
	return e.Name == "." || e.Name == ".."
}
