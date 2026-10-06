package proto

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// Channel 5: control, parameters and pad operations.
//
// A payload is a command byte followed by little-endian arguments. Requests often set
// bit 7 of the command; replies clear it. Short messages carry up to 7 bytes; most short
// replies are `cmd status` with status 0 on success.

const (
	RequestBit   byte   = 0x80
	SMPHeaderLen uint32 = 512
)

// Commands sent without the request bit and echoed back unchanged.
const (
	CmdSetParam    byte = 0x01
	CmdSampleName  byte = 0x02
	CmdProjectName byte = 0x35
	CmdPadBlock    byte = 0x1E
)

// Reply command bytes (request command with bit 7 cleared).
const (
	ReplyPadBlock        byte = 0x1E
	ReplyPatternExists   byte = 0x2B
	ReplyProjectName     byte = 0x35
	ReplyStatus          byte = 0x7E
	ReplyProjectSettings byte = 0x7D
	ReplyPeaks           byte = 0x1C
	ReplyPadSlot         byte = 0x11
	ReplyPadCommit       byte = 0x1F
	ReplyStop            byte = 0x1D
	ReplyTruncate        byte = 0x18
	ReplyNormalize       byte = 0x23
	ReplyPreviewStart    byte = 0x0E
	ReplyPreviewStop     byte = 0x0F
	ReplyMoveSample      byte = 0x15
	// ReplyError is an unsolicited error report: 32 code:i32 (see ErrorMessage).
	ReplyError byte = 0x32
	// ReplyInit answers Init (92) and the first step of a restore.
	ReplyInit byte = 0x12
	// ReplyChanged: short 1A 00 answers a project select. Also sent unsolicited as
	// 1A proj 00 00 00 when project data changed on the device.
	ReplyChanged byte = 0x1A
)

// Message table the official app shows for device errors, in its order.
var errorMessages = [46]string{
	"success",
	"error",
	"media full",
	"media protected",
	"media error",
	"unsupported format",
	"media unformatted",
	"media damaged",
	"media ejected",
	"media busy",
	"recording too long",
	"invalid format",
	"invalid path",
	"cancelled",
	"not found",
	"open error",
	"read error",
	"write error",
	"duplicate name",
	"no empty pad",
	"invalid filename",
	"already installed",
	"update wave info error",
	"update exp info error",
	"exp check wave error",
	"exp check exp error",
	"exp slot full",
	"memory full",
	"memory fragmentation",
	"hash check error",
	"error",
	"sample too short",
	"license error",
	"sample too long",
	"keyboard sample full",
	"no chop points",
	"BPM detect error",
	"key detect error",
	"bank protected",
	"one or more banks protected",
	"some import errors",
	"error",
	"import error",
	"sampling error",
	"BPM detect error",
	"process completed",
}

// ErrorMessage is the text for a device error code from a 32 code:i32 message, as the
// official app maps it: −128..=−90 index the message table at code + 130, −1 is a generic
// error.
func ErrorMessage(code int32) (string, bool) {
	switch {
	case code >= -128 && code <= -90:
		return errorMessages[code+130], true
	case code == -1:
		return errorMessages[1], true
	}
	return "", false
}

// ParseError parses an unsolicited device error message 32 code:i32.
func ParseError(payload []byte) (int32, bool) {
	if len(payload) < 5 || payload[0] != ReplyError {
		return 0, false
	}
	return int32(binary.LittleEndian.Uint32(payload[1:5])), true
}

func withPad(command byte, pad PadIndex, extra ...byte) []byte {
	p := binary.LittleEndian.AppendUint16([]byte{command}, pad.Index())
	return append(p, extra...)
}

func SetParam(id byte, target Target, value int32) []byte {
	p := []byte{CmdSetParam, id, 0}
	p = binary.LittleEndian.AppendUint16(p, uint16(target))
	return binary.LittleEndian.AppendUint32(p, uint32(value))
}

// ---- short requests ----

func StatusRequest() []byte {
	return []byte{0xFE, 0x66, 0x00}
}

// ProjectSettingsRequest asks for the settings of project (0-based). Reply: long
// 7D proj 00 …, followed by 16 long 35 nn name project names.
func ProjectSettingsRequest(project uint8) []byte {
	return []byte{0xFD, project, 0x00}
}

// SelectProject makes project (0-based) the current project. Reply: short 1A 00.
func SelectProject(project uint8) []byte {
	return []byte{0x9A, project, 0x00}
}

// MkiiExit leaves the remote screen on the device ("MKII EXIT"). No reply.
func MkiiExit() []byte {
	return []byte{0x3A, 0x00, 0x00, 0x00}
}

// PreviewStart starts playing a pad, as the app's Preview button does on mouse down.
// Reply 0E pad 00.
func PreviewStart(pad PadIndex) []byte {
	return withPad(0x8E, pad, 0x7F, 0xFF)
}

// PreviewStop stops a preview (mouse up). Reply 0F pad 00.
func PreviewStop(pad PadIndex) []byte {
	return withPad(0x8F, pad, 0x00)
}

// MoveMode is how MoveSample treats the destination pad.
type MoveMode uint16

const (
	// MoveOverwrite replaces the destination; the source pad becomes empty (the app's
	// "Overwrite").
	MoveOverwrite MoveMode = 0
	// MoveExchange swaps the two pads.
	MoveExchange MoveMode = 1
)

// MoveSample moves or exchanges a pad's sample. Reply 15 00, then a 1A proj notification.
func MoveSample(from, to PadIndex, mode MoveMode) []byte {
	p := binary.LittleEndian.AppendUint16(withPad(0x95, from), to.Index())
	return binary.LittleEndian.AppendUint16(p, uint16(mode))
}

func PadBlockRequest(pad PadIndex) []byte {
	return withPad(0x9E, pad)
}

func PatternExistsRequest(slot PadIndex) []byte {
	return withPad(0xAB, slot)
}

// Stop is sent by the official app on Disconnect and before an import (reply 1D 00).
func Stop() []byte {
	return []byte{0x9D}
}

// ---- long requests ----

// PeaksRequest asks for waveform peaks: points (min, max) pairs, each over samplesPerPoint
// frames.
func PeaksRequest(pad PadIndex, channel uint8, startFrame, points, samplesPerPoint uint32) []byte {
	p := binary.LittleEndian.AppendUint16([]byte{0x9C, 0x05}, pad.Index())
	p = append(p, channel)
	p = binary.LittleEndian.AppendUint32(p, startFrame)
	p = binary.LittleEndian.AppendUint32(p, points)
	return binary.LittleEndian.AppendUint32(p, samplesPerPoint)
}

// Peak is one waveform point.
type Peak struct {
	Min, Max int16
}

// ParsePeaks parses a peaks reply into (min, max) pairs.
func ParsePeaks(payload []byte) []Peak {
	if len(payload) < 16 {
		return nil
	}
	data := payload[16:]
	out := make([]Peak, 0, len(data)/4)
	for ; len(data) >= 4; data = data[4:] {
		out = append(out, Peak{
			Min: int16(binary.LittleEndian.Uint16(data)),
			Max: int16(binary.LittleEndian.Uint16(data[2:])),
		})
	}
	return out
}

func SampleName(pad PadIndex, name string) []byte {
	b := printable(name)
	return append(withPad(CmdSampleName, pad), b[:min(len(b), 23)]...)
}

// ProjectName takes a 0-based project (0 = project 1).
func ProjectName(project uint8, name string) []byte {
	return append([]byte{CmdProjectName, project}, printable(name)...)
}

// ParseProjectName parses a 35 nn name project-name reply.
func ParseProjectName(payload []byte) (project uint8, name string, ok bool) {
	if len(payload) < 2 || payload[0] != ReplyProjectName {
		return 0, "", false
	}
	name = strings.Trim(strings.ToValidUTF8(string(payload[2:]), "�"), " \x00")
	return payload[1], name, true
}

// PadOp is a device-side operation on one pad's sample.
type PadOp int

const (
	PadOpTruncate PadOp = iota
	PadOpNormalize
	PadOpDelete
)

func (op PadOp) Payload(pad PadIndex) []byte {
	switch op {
	case PadOpTruncate:
		return withPad(0x98, pad)
	case PadOpNormalize:
		return withPad(0xA3, pad)
	default:
		return withPad(0x91, pad, 0x00, 0x00)
	}
}

func (op PadOp) Reply() byte {
	switch op {
	case PadOpTruncate:
		return ReplyTruncate
	case PadOpNormalize:
		return ReplyNormalize
	default:
		return ReplyPadSlot
	}
}

// Bracketed reports whether the official app brackets the operation with EditBegin /
// EditEnd.
func (op PadOp) Bracketed() bool {
	return op == PadOpNormalize
}

func EditBegin() []byte {
	return []byte{0xFB, 0x00, 0x00}
}

func EditEnd() []byte {
	return []byte{0xFC, 0x00, 0x00}
}

// Pattern rendering on the device (Bounce and MULTIPAD export). The device writes the
// WAV into a host file handle through fileapi host requests.
const (
	RenderBegin  uint16 = 1000
	RenderEnd    uint16 = 1001
	RenderBounce uint16 = 1003
	// RenderFailed is the result code of a failed render.
	RenderFailed uint16 = 1002

	RenderPadsUsedReply byte = 0x37
	RenderControlReply  byte = 0x38
	RenderDoneReply     byte = 0x39
)

// RenderPadsUsedRequest asks for the pads used by a pattern. Reply: long
// 37 ? count nbytes bitmap.
func RenderPadsUsedRequest(pattern PadIndex) []byte {
	return withPad(0xB7, pattern)
}

func ParseRenderPadsUsed(payload []byte) ([]PadIndex, bool) {
	if len(payload) < 4 || payload[0] != RenderPadsUsedReply {
		return nil, false
	}
	n := int(payload[3])
	if len(payload) < 4+n {
		return nil, false
	}
	bitmap := payload[4 : 4+n]
	pads := []PadIndex{}
	for _, p := range AllPads() {
		i := int(p.Index())
		if i/8 < len(bitmap) && bitmap[i/8]&(1<<(i%8)) != 0 {
			pads = append(pads, p)
		}
	}
	return pads, true
}

// RenderControl takes op RenderBegin, RenderEnd, RenderBounce, or a pad index 0..=159
// (MULTIPAD). Reply: 38 pattern op.
func RenderControl(pattern PadIndex, op, handle uint16) []byte {
	p := binary.LittleEndian.AppendUint16(withPad(0xB8, pattern), op)
	return binary.LittleEndian.AppendUint16(p, handle)
}

// RenderDone is the device's 39 pattern op handle result completion message.
type RenderDone struct {
	Pattern, Op, Handle, Result uint16
}

func ParseRenderDone(payload []byte) (RenderDone, bool) {
	if len(payload) < 9 || payload[0] != RenderDoneReply {
		return RenderDone{}, false
	}
	w := func(at int) uint16 { return binary.LittleEndian.Uint16(payload[at:]) }
	return RenderDone{Pattern: w(1), Op: w(3), Handle: w(5), Result: w(7)}, true
}

// InitScope is what Init clears in the current project.
type InitScope struct {
	option, bank uint16
}

var (
	InitAll         = InitScope{0, 0}
	InitAllSamples  = InitScope{1, 0}
	InitAllPatterns = InitScope{3, 0}
)

// InitSamplesBank is the samples of one bank, 0..=9 (A..J).
func InitSamplesBank(bank uint8) InitScope {
	return InitScope{2, uint16(bank)}
}

// InitPatternsBank is the patterns of one bank, 0..=9 (A..J).
func InitPatternsBank(bank uint8) InitScope {
	return InitScope{4, uint16(bank)}
}

// InitProject builds Init: 92 mode:u16 option:u16 bank:u16. Erases data of the current
// project, in memory and on the card. Reply 12 status, then a 1A proj notification.
func InitProject(scope InitScope) []byte {
	return initRequest(0, scope.option, scope.bank)
}

func initRequest(mode, option, bank uint16) []byte {
	p := []byte{0x92}
	for _, v := range []uint16{mode, option, bank} {
		p = binary.LittleEndian.AppendUint16(p, v)
	}
	return p
}

// Restoring a whole project from a PC backup (the app's "Import to MKII").
const (
	RestorePrepareReply byte = ReplyInit
	RestoreReloadReply  byte = 0x31
	RestoreReloaded     byte = 0x2C
)

// RestorePrepare is sent before any file is written: Init in import mode (100). Erases
// the current project, in memory and on the card. Reply 12 status after about a second.
func RestorePrepare() []byte {
	return initRequest(100, 0, 0)
}

// RestoreReload is sent after the files are written: reload project (0-based) from the
// card. Reply 31 status, later an unsolicited 2C 00 00 00 00.
func RestoreReload(project uint8) []byte {
	return []byte{0xB1, project, 0x00}
}

// The short messages the official app sends around a sample import, in order.

func ImportBegin() []byte {
	return []byte{0xFB, 0xA0, 0x00}
}

func ImportReleaseSlot(pad PadIndex) []byte {
	return withPad(0x91, pad, 0x01, 0x00)
}

func ImportCommit(pad PadIndex) []byte {
	return withPad(0x9F, pad)
}

func ImportEnd() []byte {
	return EditEnd()
}

func (m MoveMode) String() string {
	if m == MoveExchange {
		return "Exchange"
	}
	return "Overwrite"
}

func (op PadOp) String() string {
	switch op {
	case PadOpTruncate:
		return "Truncate"
	case PadOpNormalize:
		return "Normalize"
	default:
		return "Delete"
	}
}

func (s InitScope) String() string {
	switch s.option {
	case 0:
		return "All"
	case 1:
		return "AllSamples"
	case 2:
		return fmt.Sprintf("SamplesBank(%d)", s.bank)
	case 3:
		return "AllPatterns"
	default:
		return fmt.Sprintf("PatternsBank(%d)", s.bank)
	}
}
