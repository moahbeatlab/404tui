package device

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/moahbeatlab/404tui/proto"
	"github.com/moahbeatlab/404tui/proto/fileapi"
)

// Parameters, pad blocks and pad operations (channel 5).

const (
	controlTimeout = 3 * time.Second
	// Projects is the number of project slots.
	Projects = 16
)

// Activity is what the device is doing, from the status poll alone: cheap enough to ask
// several times a second, unlike Status, which also reads the project settings.
type Activity struct {
	// Current project, 0-based.
	Project uint8
	// The pad selected on the device: hitting a pad selects it, and choosing a bank selects
	// its first pad. Only meaningful when HasSelected.
	Selected    proto.PadIndex
	HasSelected bool
	// Frames played so far of the pad sounding; only meaningful when Playing.
	Position uint32
	Playing  bool
}

// Status is a summary of the device state from the status poll and project settings.
type Status struct {
	// Raw 7E status payload.
	Raw []byte
	// Current project, 0-based (hypothesis: byte 1 of the status payload).
	Project uint8
	// Selected bank and pad (bytes 3 and 5 of the status payload); only meaningful when
	// HasSelected.
	Selected    proto.PadIndex
	HasSelected bool
	Settings    *proto.ProjectSettings
}

func short(payload []byte) proto.Message {
	return proto.Short(proto.ChannelControl, payload)
}

func long(payload []byte) proto.Message {
	return proto.Long(proto.ChannelControl, payload)
}

func refused(what string, code byte) error {
	return RefusedError{fmt.Sprintf("%s: status %d", what, code)}
}

func deviceError(what string, code int32) error {
	if text, ok := proto.ErrorMessage(code); ok {
		return RefusedError{fmt.Sprintf("%s: %s (error %d)", what, text, code)}
	}
	return RefusedError{fmt.Sprintf("%s: error %d", what, code)}
}

func isControl(m *proto.Message) bool {
	return m.Channel == proto.ChannelControl
}

// startsWith matches a payload of at least n bytes that begins with cmd.
func startsWith(cmd byte, n int) func([]byte) bool {
	return func(p []byte) bool { return len(p) >= n && p[0] == cmd }
}

// forPad matches a payload of at least n bytes that begins with cmd and the pad's index.
func forPad(cmd byte, pad proto.PadIndex, n int) func([]byte) bool {
	id := binary.LittleEndian.AppendUint16(nil, pad.Index())
	return func(p []byte) bool { return len(p) >= n && p[0] == cmd && bytes.Equal(p[1:3], id) }
}

func (d *Device) controlCall(msg proto.Message, what string, match func([]byte) bool) ([]byte, error) {
	reply, err := d.transact(msg, what, controlTimeout, func(m *proto.Message) bool {
		return isControl(m) && match(m.Payload())
	})
	if err != nil {
		return nil, err
	}
	return bytes.Clone(reply.Payload()), nil
}

// command sends a short command whose reply is `cmd status`; fails unless status is 0.
func (d *Device) command(payload []byte, expect byte, what string) error {
	reply, err := d.controlCall(short(payload), what, startsWith(expect, 1))
	if err != nil {
		return err
	}
	if len(reply) > 1 && reply[1] != 0 {
		return refused(what, reply[1])
	}
	return nil
}

func (d *Device) statusPoll() ([]byte, error) {
	return d.controlCall(short(proto.StatusRequest()), "status", startsWith(proto.ReplyStatus, 1))
}

func byteAt(raw []byte, i int) byte {
	if i < len(raw) {
		return raw[i]
	}
	return 0
}

func selectedPad(raw []byte) (proto.PadIndex, bool) {
	if len(raw) <= 5 {
		return proto.PadIndex{}, false
	}
	return proto.PadFromBankPad(uint16(raw[3]), uint16(raw[5]))
}

func (d *Device) Status() (*Status, error) {
	raw, err := d.statusPoll()
	if err != nil {
		return nil, err
	}
	project := byteAt(raw, 1)
	settings, err := d.ProjectSettings(project)
	if err != nil {
		return nil, err
	}
	selected, ok := selectedPad(raw)
	return &Status{Raw: raw, Project: project, Selected: selected, HasSelected: ok, Settings: settings}, nil
}

// Activity is the status poll on its own: bytes 1, 3 and 5 are the project, bank and pad,
// and bytes 8 to 11 count the frames a sounding pad has played (all FF when none is).
func (d *Device) Activity() (Activity, error) {
	raw, err := d.statusPoll()
	if err != nil {
		return Activity{}, err
	}
	a := Activity{Project: byteAt(raw, 1)}
	a.Selected, a.HasSelected = selectedPad(raw)
	if len(raw) >= 12 {
		if frames := binary.LittleEndian.Uint32(raw[8:12]); frames != 0xFFFFFFFF {
			a.Position, a.Playing = frames, true
		}
	}
	return a, nil
}

// SelectPad selects a pad on the device, bank first, as the app's pad grid does.
func (d *Device) SelectPad(pad proto.PadIndex) error {
	if err := d.SetParam(0x00, proto.TargetGlobal, int32(pad.Bank())); err != nil {
		return err
	}
	return d.SetParam(0x01, proto.TargetGlobal, int32(pad.Pad()))
}

// CurrentProject is the current project, 0-based (status poll byte 1).
func (d *Device) CurrentProject() (uint8, error) {
	raw, err := d.statusPoll()
	return byteAt(raw, 1), err
}

// ProjectSettings are the settings of project (0-based; 7D reply).
func (d *Device) ProjectSettings(project uint8) (*proto.ProjectSettings, error) {
	payload, err := d.controlCall(short(proto.ProjectSettingsRequest(project)), "project settings",
		func(p []byte) bool { return len(p) > 1 && p[0] == proto.ReplyProjectSettings && p[1] == project })
	if err != nil {
		return nil, err
	}
	return protoResult(proto.ParseProjectSettings(payload))
}

func protoResult[T any](v T, err error) (T, error) {
	if err != nil {
		return v, fmt.Errorf("protocol: %w", err)
	}
	return v, nil
}

// SelectProject makes project (0-based) the current project. It returns once the device
// has loaded it: settings requested before its 1A proj notification can still describe
// the old project.
func (d *Device) SelectProject(project uint8) error {
	var refusal byte
	_, err := d.transact(short(proto.SelectProject(project)), "select project", 10*time.Second,
		func(m *proto.Message) bool {
			p := m.Payload()
			if !isControl(m) || len(p) < 2 || p[0] != proto.ReplyChanged {
				return false
			}
			// Acknowledgement 1A status.
			if len(p) == 2 {
				if p[1] != 0 {
					refusal = p[1]
				}
				return refusal != 0
			}
			// Notification 1A proj 00 00 00.
			return p[1] == project
		})
	if err != nil {
		return err
	}
	if refusal != 0 {
		return refused("select project", refusal)
	}
	return nil
}

// MkiiExit leaves the remote screen on the device (the app's "MKII EXIT"). The link stays up.
func (d *Device) MkiiExit() error {
	return d.fire(short(proto.MkiiExit()))
}

// Preview plays a pad for duration, like holding the app's Preview button.
func (d *Device) Preview(pad proto.PadIndex, duration time.Duration) error {
	if err := d.PreviewStart(pad); err != nil {
		return err
	}
	time.Sleep(duration)
	return d.PreviewStop(pad)
}

// PreviewStart starts playing a pad, as holding the pad down would; it plays until
// PreviewStop.
func (d *Device) PreviewStart(pad proto.PadIndex) error {
	started, err := d.controlCall(short(proto.PreviewStart(pad)), "preview", forPad(proto.ReplyPreviewStart, pad, 4))
	if err != nil {
		return err
	}
	if started[3] != 0 {
		return refused("preview", started[3])
	}
	return nil
}

// PreviewStop stops a pad started with PreviewStart.
func (d *Device) PreviewStop(pad proto.PadIndex) error {
	_, err := d.controlCall(short(proto.PreviewStop(pad)), "stop preview", forPad(proto.ReplyPreviewStop, pad, 4))
	return err
}

// MoveSample moves a pad's sample onto another pad, replacing or exchanging with the
// destination.
func (d *Device) MoveSample(from, to proto.PadIndex, mode proto.MoveMode) error {
	what := fmt.Sprintf("%s %s to %s", mode, from, to)
	if err := d.command(proto.MoveSample(from, to, mode), proto.ReplyMoveSample, what); err != nil {
		return err
	}
	return d.checkDeviceError(what)
}

// ProjectNames are the names of the 16 project slots (empty string for unnamed slots). The
// device sends them after the project settings reply.
func (d *Device) ProjectNames() ([]string, error) {
	current, err := d.CurrentProject()
	if err != nil {
		return nil, err
	}
	names := make([]string, Projects)
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.send(short(proto.ProjectSettingsRequest(current))); err != nil {
		return nil, err
	}
	for seen := 0; seen < Projects; {
		m, err := d.wait("project names", controlTimeout, func(m *proto.Message) bool {
			return isControl(m) && startsWith(proto.ReplyProjectName, 1)(m.Payload())
		})
		if err != nil {
			return nil, err
		}
		if index, name, ok := proto.ParseProjectName(m.Payload()); ok && int(index) < Projects {
			names[index] = name
			seen++
		}
	}
	return names, nil
}

func (d *Device) PadBlock(pad proto.PadIndex) (*proto.PadBlock, error) {
	payload, err := d.controlCall(short(proto.PadBlockRequest(pad)), fmt.Sprintf("pad block %s", pad),
		forPad(proto.ReplyPadBlock, pad, 4))
	if err != nil {
		return nil, err
	}
	return protoResult(proto.ParsePadBlock(payload))
}

func (d *Device) PadBlocks() ([]*proto.PadBlock, error) {
	blocks := make([]*proto.PadBlock, 0, proto.PadCount)
	for _, pad := range proto.AllPads() {
		block, err := d.PadBlock(pad)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

func (d *Device) PatternExists(slot proto.PadIndex) (bool, error) {
	payload, err := d.controlCall(short(proto.PatternExistsRequest(slot)), fmt.Sprintf("pattern %s", slot),
		forPad(proto.ReplyPatternExists, slot, 4))
	if err != nil {
		return false, err
	}
	return payload[3] != 0, nil
}

func (d *Device) SetParam(id byte, target proto.Target, value int32) error {
	payload := proto.SetParam(id, target, value)
	head := payload[:5]
	_, err := d.controlCall(long(payload), fmt.Sprintf("set param 0x%02x", id), func(p []byte) bool {
		return len(p) >= 5 && bytes.Equal(p[:5], head)
	})
	return err
}

// SetChopPoints replaces a pad's chop points: slot n gets points[n] (sample frames), and
// slots past the end are cleared. The device keeps them by slot, without sorting.
func (d *Device) SetChopPoints(pad proto.PadIndex, points []uint32) error {
	if len(points) > proto.PadBlockChopPoints {
		return RefusedError{fmt.Sprintf("%s: %d chop points, at most %d fit", pad, len(points), proto.PadBlockChopPoints)}
	}
	for slot := 0; slot < proto.PadBlockChopPoints; slot++ {
		value := int32(-1)
		if slot < len(points) {
			value = int32(points[slot])
		}
		if err := d.SetParam(proto.ChopPointFirst+byte(slot), proto.TargetPad(pad), value); err != nil {
			return err
		}
	}
	return nil
}

// Peaks are waveform peaks (min, max) for one channel of a pad's sample.
func (d *Device) Peaks(pad proto.PadIndex, channel uint8, startFrame, points, samplesPerPoint uint32) ([]proto.Peak, error) {
	forThisPad := forPad(proto.ReplyPeaks, pad, 16)
	payload, err := d.controlCall(long(proto.PeaksRequest(pad, channel, startFrame, points, samplesPerPoint)), "peaks",
		func(p []byte) bool { return forThisPad(p) && p[3] == channel })
	if err != nil {
		return nil, err
	}
	return proto.ParsePeaks(payload), nil
}

func (d *Device) SetSampleName(pad proto.PadIndex, name string) error {
	_, err := d.controlCall(long(proto.SampleName(pad, name)), "sample name", forPad(proto.CmdSampleName, pad, 3))
	return err
}

func projectFolder(number uint8) string {
	return fmt.Sprintf("ROLAND/SP-404MKII/PROJECT_%02d", number)
}

// SetProjectName renames project (0-based), which must be the current project.
//
// The device renames whichever project is current and ignores the number it is sent, so
// renaming another one would rename the current one instead. It also keeps the name only
// in the project's PADCONF.BIN: an empty slot has none, and one written by older firmware
// has no room for a name, so a rename there is accepted and then lost when another project
// is selected. Both are refused with the reason.
func (d *Device) SetProjectName(project uint8, name string) error {
	number := project + 1
	current, err := d.CurrentProject()
	if err != nil {
		return err
	}
	if current != project {
		return UnsupportedError{fmt.Sprintf(
			"project %d is not the current project (project %d is); select it first", number, current+1)}
	}
	padconf := projectFolder(number) + "/PADCONF.BIN"
	if _, exists, err := d.Stat(padconf); err != nil {
		return err
	} else if !exists {
		return UnsupportedError{fmt.Sprintf(
			"project %d is empty, and the SP-404MKII only keeps a name for a project "+
				"with something saved in it; add a sample first, then rename it", number)}
	}
	handle, err := d.OpenFile(padconf, fileapi.FlagReadOnly)
	if err != nil {
		return err
	}
	header, readErr := d.Read(handle, 12)
	if err := d.CloseFile(handle); err != nil {
		return err
	}
	if readErr != nil {
		return readErr
	}
	// Byte 8 is the file's version; names arrived with version 3.
	if len(header) > 8 && header[8] < 3 {
		return UnsupportedError{fmt.Sprintf(
			"project %d was saved by older firmware, and its settings file has no room for a name", number)}
	}
	_, err = d.controlCall(long(proto.ProjectName(project, name)), "project name", func(p []byte) bool {
		return len(p) >= 2 && p[0] == proto.CmdProjectName && p[1] == project
	})
	return err
}

// checkDeviceError: after an acknowledged edit the device reports failure separately, as
// 32 code (for example on an empty pad). Wait briefly for that or for the 1A change
// notification that follows a successful edit.
func (d *Device) checkDeviceError(what string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	m, err := d.wait(what, 1500*time.Millisecond, func(m *proto.Message) bool {
		p := m.Payload()
		return isControl(m) && len(p) > 0 && (p[0] == proto.ReplyError || p[0] == proto.ReplyChanged)
	})
	var timeout TimeoutError
	switch {
	// Neither arrived: nothing went wrong that the device reported.
	case errors.As(err, &timeout):
		return nil
	case err != nil:
		return err
	}
	if code, ok := proto.ParseError(m.Payload()); ok {
		return deviceError(what, code)
	}
	return nil
}

func (d *Device) requireCurrent(project uint8, doing string) error {
	current, err := d.CurrentProject()
	if err != nil {
		return err
	}
	if current != project {
		return RefusedError{fmt.Sprintf("%s project %d: project %d is current (select it first)", doing, project+1, current+1)}
	}
	return nil
}

// InitProject erases data of the current project (all of it, all samples or patterns, or
// one bank's). project (0-based) must be the current project, as a guard against erasing
// the wrong one.
func (d *Device) InitProject(project uint8, scope proto.InitScope) error {
	if err := d.requireCurrent(project, "init"); err != nil {
		return err
	}
	what := fmt.Sprintf("init %s", scope)
	// The device answers 12 status and announces the reloaded project with 1A proj, in
	// either order (Patterns Bank sends the notification first). Wait for both, so the next
	// command doesn't reach the device while it is still reloading.
	var (
		status          byte
		acked, reloaded bool
		errCode         int32
		failed          bool
	)
	_, err := d.transact(short(proto.InitProject(scope)), what, 15*time.Second, func(m *proto.Message) bool {
		if !isControl(m) {
			return false
		}
		p := m.Payload()
		switch {
		case len(p) >= 2 && p[0] == proto.ReplyInit:
			status, acked = p[1], true
		case len(p) >= 3 && p[0] == proto.ReplyChanged:
			reloaded = reloaded || p[1] == project
		case !failed:
			errCode, failed = proto.ParseError(p)
		}
		return failed || (acked && status != 0) || (acked && reloaded)
	})
	var timeout TimeoutError
	switch {
	case failed:
		return deviceError(what, errCode)
	case acked && status != 0:
		return refused(what, status)
	case err == nil:
		return nil
	// Acknowledged, but the reload notification never came: the erase went through.
	case errors.As(err, &timeout) && acked:
		d.log.Warn("no reload notification from the device", "what", what)
		return nil
	}
	return err
}

// PadOp truncates, normalizes or deletes a pad's sample on the device.
func (d *Device) PadOp(op proto.PadOp, pad proto.PadIndex) error {
	if op.Bracketed() {
		if err := d.fire(short(proto.EditBegin())); err != nil {
			return err
		}
	}
	what := fmt.Sprintf("%s %s", op, pad)
	result := d.command(op.Payload(pad), op.Reply(), what)
	if result == nil {
		result = d.checkDeviceError(what)
	}
	if op.Bracketed() {
		if err := d.fire(short(proto.EditEnd())); err != nil {
			return err
		}
	}
	return result
}

// File is one file of a project backup: a path relative to the PROJECT_NN folder.
type File struct {
	Name string
	Data []byte
}

// RestoreProject replaces a project's files from a backup and makes the device reload it
// (the app's "Import to MKII"). project is 0-based and must be the current project: the
// first step erases the current project, on the card as well as in memory. files are
// written in order; progress, if not nil, is called per file.
func (d *Device) RestoreProject(project uint8, files []File, progress func(i int, name string)) error {
	if err := d.requireCurrent(project, "restore into"); err != nil {
		return err
	}
	if err := d.fire(short(proto.EditBegin())); err != nil {
		return err
	}
	result := d.restore(project, files, progress)
	if err := d.fire(short(proto.EditEnd())); err != nil {
		return err
	}
	return result
}

func (d *Device) restore(project uint8, files []File, progress func(i int, name string)) error {
	step := func(payload []byte, reply byte, what string) error {
		m, err := d.transact(short(payload), what, 15*time.Second, func(m *proto.Message) bool {
			return isControl(m) && startsWith(reply, 1)(m.Payload())
		})
		if err != nil {
			return err
		}
		if p := m.Payload(); len(p) > 1 && p[1] != 0 {
			return refused(what, p[1])
		}
		return nil
	}
	if err := step(proto.RestorePrepare(), proto.RestorePrepareReply, "prepare restore"); err != nil {
		return err
	}
	folder := projectFolder(project + 1)
	for i, f := range files {
		if progress != nil {
			progress(i, f.Name)
		}
		if err := d.WriteFile(folder+"/"+f.Name, f.Data); err != nil {
			return err
		}
	}
	if err := step(proto.RestoreReload(project), proto.RestoreReloadReply, "reload project"); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.wait("project reload", 15*time.Second, func(m *proto.Message) bool {
		return isControl(m) && startsWith(proto.RestoreReloaded, 1)(m.Payload())
	})
	return err
}

// ImportSMP writes a complete SMP file to a pad and points the pad at it, following the
// official app's import sequence. project is 1-based (the PROJECT_NN folder) and must be
// the current project: the pad block is written to the current project, so importing into
// another one would change a current pad while its file went elsewhere.
func (d *Device) ImportSMP(project uint8, pad proto.PadIndex, smp []byte, name string) error {
	current, err := d.CurrentProject()
	if err != nil {
		return err
	}
	if current+1 != project {
		return RefusedError{fmt.Sprintf("import into project %d: project %d is current (select it first)", project, current+1)}
	}
	if len(smp) < int(proto.SMPHeaderLen) {
		return UnexpectedError{"SMP shorter than its header"}
	}
	block, err := d.PadBlock(pad)
	if err != nil {
		return err
	}
	if err := d.fire(short(proto.ImportBegin())); err != nil {
		return err
	}
	result := d.importSMP(project, pad, block, smp, name)
	if err := d.fire(short(proto.ImportEnd())); err != nil {
		return err
	}
	return result
}

func (d *Device) importSMP(project uint8, pad proto.PadIndex, block *proto.PadBlock, smp []byte, name string) error {
	const header = int(proto.SMPHeaderLen)
	path := pad.SamplePath(project)
	tmp := strings.ReplaceAll(path, ".SMP", ".TMP")

	if err := d.command(proto.ImportReleaseSlot(pad), proto.ReplyPadSlot, "release slot"); err != nil {
		return err
	}
	if err := d.command(proto.Stop(), proto.ReplyStop, "stop"); err != nil {
		return err
	}
	_ = d.Unlink(tmp)
	_ = d.Unlink(path)

	handle, err := d.OpenFile(path, fileapi.FlagCreateTruncateReadWrite)
	if err != nil {
		return err
	}
	written := func() error {
		if _, err := d.Seek(handle, uint32(header)); err != nil {
			return err
		}
		if err := d.Write(handle, smp[header:]); err != nil {
			return err
		}
		if _, err := d.Seek(handle, 0); err != nil {
			return err
		}
		return d.Write(handle, smp[:header])
	}()
	if err := d.CloseFile(handle); err != nil {
		return err
	}
	if written != nil {
		return written
	}

	st, _, err := d.Stat(path)
	if err != nil {
		return err
	}
	if int(st.Size) != len(smp) {
		return RefusedError{fmt.Sprintf("write %s (size %d, expected %d)", path, st.Size, len(smp))}
	}

	block.AssignSample(uint32(len(smp)), name)
	forThisPad := forPad(proto.ReplyPadBlock, pad, 3)
	if _, err := d.controlCall(long(block.WritePayload()), "pad block write", func(p []byte) bool {
		return len(p) > 0 && p[0] == proto.ReplyPadBlock && (len(p) <= 2 || forThisPad(p))
	}); err != nil {
		return err
	}
	// Reply: 1F pad:u16 status.
	commit, err := d.controlCall(short(proto.ImportCommit(pad)), "commit import", forPad(proto.ReplyPadCommit, pad, 4))
	if err != nil {
		return err
	}
	if commit[3] != 0 {
		return refused("commit import", commit[3])
	}
	return nil
}
