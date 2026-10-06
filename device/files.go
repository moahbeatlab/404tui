package device

import (
	"fmt"
	"time"

	"github.com/moahbeatlab/404tui/proto"
	"github.com/moahbeatlab/404tui/proto/fileapi"
)

// Remote file API on the device's SD card (channel 6).

const fileTimeout = 5 * time.Second

// Short messages on this channel are notifications, never replies.
func isFileReply(m *proto.Message) bool {
	return m.Channel == proto.ChannelFile && !m.IsShort()
}

func (d *Device) fileCall(payload []byte, what string) (fileapi.Reply, error) {
	reply, err := d.transact(proto.Long(proto.ChannelFile, payload), what, fileTimeout, isFileReply)
	if err != nil {
		return nil, err
	}
	return parseReply(reply)
}

func parseReply(m proto.Message) (fileapi.Reply, error) {
	reply, err := fileapi.ParseReply(m.Payload())
	if err != nil {
		return nil, fmt.Errorf("protocol: %w", err)
	}
	return reply, nil
}

func (d *Device) statusCall(payload []byte, what string) (int32, error) {
	reply, err := d.fileCall(payload, what)
	if err != nil {
		return 0, err
	}
	status, ok := reply.(fileapi.StatusReply)
	switch {
	case !ok:
		return 0, UnexpectedError{what}
	case status.Result == -1:
		return 0, RefusedError{what}
	}
	return status.Result, nil
}

// dataCall returns ok false when the device answers with a failure status.
func (d *Device) dataCall(payload []byte, what string) (data []byte, ok bool, err error) {
	reply, err := d.fileCall(payload, what)
	if err != nil {
		return nil, false, err
	}
	switch r := reply.(type) {
	case fileapi.DataReply:
		return r.Data, true, nil
	case fileapi.StatusReply:
		if r.Result == -1 {
			return nil, false, nil
		}
	}
	return nil, false, UnexpectedError{what}
}

func (d *Device) OpenFile(path string, flags uint32) (int32, error) {
	return d.statusCall(fileapi.Open(path, flags), "open "+path)
}

func (d *Device) CloseFile(handle int32) error {
	_, err := d.statusCall(fileapi.Close(handle), "close")
	return err
}

func (d *Device) Seek(handle int32, offset uint32) (uint32, error) {
	r, err := d.statusCall(fileapi.Seek(handle, offset), "seek")
	return uint32(r), err
}

func (d *Device) FStat(handle int32) (fileapi.FileStat, error) {
	data, ok, err := d.dataCall(fileapi.FStat(handle), "fstat")
	if err != nil {
		return fileapi.FileStat{}, err
	}
	if !ok {
		return fileapi.FileStat{}, RefusedError{"fstat"}
	}
	return parseStat(data)
}

func parseStat(data []byte) (fileapi.FileStat, error) {
	st, err := fileapi.ParseStat(data)
	if err != nil {
		return st, fmt.Errorf("protocol: %w", err)
	}
	return st, nil
}

// Stat reports exists false when the path does not exist.
func (d *Device) Stat(path string) (st fileapi.FileStat, exists bool, err error) {
	data, ok, err := d.dataCall(fileapi.Stat(path), "stat "+path)
	if err != nil || !ok {
		return fileapi.FileStat{}, false, err
	}
	st, err = parseStat(data)
	return st, err == nil, err
}

func (d *Device) Unlink(path string) error {
	_, err := d.statusCall(fileapi.Unlink(path), "unlink "+path)
	return err
}

// Mkdir creates a directory; fails if the name is taken.
func (d *Device) Mkdir(path string) error {
	_, err := d.statusCall(fileapi.Mkdir(path), "mkdir "+path)
	return err
}

// Rmdir removes a directory; fails unless it is empty.
func (d *Device) Rmdir(path string) error {
	_, err := d.statusCall(fileapi.Rmdir(path), "rmdir "+path)
	return err
}

// Rename moves or renames a file or directory within the card.
func (d *Device) Rename(from, to string) error {
	_, err := d.statusCall(fileapi.Rename(from, to), "rename "+from)
	return err
}

func (d *Device) FreeKb() (uint32, error) {
	r, err := d.statusCall(fileapi.FreeKb(fileapi.Root), "free space")
	return uint32(r), err
}

// Read reads up to count bytes; the device answers in chunks of at most fileapi.Chunk.
func (d *Device) Read(handle int32, count uint32) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.send(proto.Long(proto.ChannelFile, fileapi.Read(handle, count))); err != nil {
		return nil, err
	}
	var out []byte
	for {
		m, err := d.wait("read", fileTimeout, isFileReply)
		if err != nil {
			return nil, err
		}
		reply, err := parseReply(m)
		if err != nil {
			return nil, err
		}
		switch r := reply.(type) {
		case fileapi.DataReply:
			out = append(out, r.Data...)
			if r.Last {
				return out, nil
			}
		case fileapi.StatusReply:
			if r.Result == -1 {
				return nil, RefusedError{"read"}
			}
			return out, nil
		}
	}
}

// Progress is called with the bytes done so far and the total.
type Progress func(done, total uint64)

// Write writes data at the current position in batches, checking the device's byte counts.
func (d *Device) Write(handle int32, data []byte) error {
	return d.WriteWith(handle, data, nil)
}

// WriteWith is like Write, calling progress after each batch.
func (d *Device) WriteWith(handle int32, data []byte, progress Progress) error {
	total := uint64(len(data))
	const batch = fileapi.Chunk * fileapi.WriteBatch
	for done := 0; done < len(data); {
		part := data[done:min(done+batch, len(data))]
		if err := d.writeBatch(handle, part); err != nil {
			return err
		}
		done += len(part)
		if progress != nil {
			progress(uint64(done), total)
		}
	}
	return nil
}

func (d *Device) writeBatch(handle int32, part []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for at := 0; at < len(part); at += fileapi.Chunk {
		end := min(at+fileapi.Chunk, len(part))
		msg := proto.Long(proto.ChannelFile, fileapi.Write(handle, part[at:end], end == len(part)))
		if err := d.send(msg); err != nil {
			return err
		}
	}
	m, err := d.wait("write", fileTimeout, isFileReply)
	if err != nil {
		return err
	}
	reply, err := parseReply(m)
	if err != nil {
		return err
	}
	status, ok := reply.(fileapi.StatusReply)
	if !ok {
		return UnexpectedError{"write"}
	}
	if int64(status.Result) != int64(len(part)) {
		return RefusedError{fmt.Sprintf("write (%d of %d bytes accepted)", status.Result, len(part))}
	}
	return nil
}

// ReadFile reads a whole file.
func (d *Device) ReadFile(path string) ([]byte, error) {
	return d.ReadFileWith(path, nil)
}

// ReadFileWith is like ReadFile, calling progress as blocks arrive.
func (d *Device) ReadFileWith(path string, progress Progress) ([]byte, error) {
	handle, err := d.OpenFile(path, fileapi.FlagReadOnly)
	if err != nil {
		return nil, err
	}
	data, err := d.readAll(handle, progress)
	closeErr := d.CloseFile(handle)
	if err != nil {
		return nil, err
	}
	return data, closeErr
}

func (d *Device) readAll(handle int32, progress Progress) ([]byte, error) {
	st, err := d.FStat(handle)
	if err != nil {
		return nil, err
	}
	size := int(st.Size)
	out := make([]byte, 0, size)
	for len(out) < size {
		chunk, err := d.Read(handle, uint32(min(size-len(out), int(fileapi.BulkRead))))
		if err != nil {
			return nil, err
		}
		if len(chunk) == 0 {
			break
		}
		out = append(out, chunk...)
		if progress != nil {
			progress(uint64(len(out)), uint64(size))
		}
	}
	return out, nil
}

// WriteFile creates or replaces a file with data.
func (d *Device) WriteFile(path string, data []byte) error {
	return d.WriteFileWith(path, data, nil)
}

// WriteFileWith is like WriteFile, calling progress as batches go out.
func (d *Device) WriteFileWith(path string, data []byte, progress Progress) error {
	handle, err := d.OpenFile(path, fileapi.FlagCreateTruncateReadWrite)
	if err != nil {
		return err
	}
	err = d.WriteWith(handle, data, progress)
	closeErr := d.CloseFile(handle)
	if err != nil {
		return err
	}
	return closeErr
}

// ReadPrefix reads the first n bytes of a file; exists is false if it does not exist.
func (d *Device) ReadPrefix(path string, n uint32) (data []byte, exists bool, err error) {
	if _, exists, err := d.Stat(path); err != nil || !exists {
		return nil, false, err
	}
	handle, err := d.OpenFile(path, fileapi.FlagReadOnly)
	if err != nil {
		return nil, false, err
	}
	data, err = d.Read(handle, n)
	closeErr := d.CloseFile(handle)
	if err != nil {
		return nil, false, err
	}
	if closeErr != nil {
		return nil, false, closeErr
	}
	return data, true, nil
}

// ListDir lists a directory, without . and ..
func (d *Device) ListDir(path string) ([]fileapi.DirEntry, error) {
	dir, err := d.statusCall(fileapi.OpenDir(path), "opendir "+path)
	if err != nil {
		return nil, err
	}
	entries := []fileapi.DirEntry{}
	var result error
	for {
		data, ok, err := d.dataCall(fileapi.ReadDir(dir), "readdir")
		if err != nil {
			result = err
			break
		}
		if !ok {
			break
		}
		entry, err := fileapi.ParseDirEntry(data)
		if err != nil {
			return nil, fmt.Errorf("protocol: %w", err)
		}
		if !entry.IsDot() {
			entries = append(entries, entry)
		}
	}
	// The SD card's filesystem closes the handle itself once readdir runs out, and then
	// refuses closedir. The entries are already in hand, so that is not a failure; a
	// genuine one would only leak a handle until the device restarts.
	if _, err := d.statusCall(fileapi.CloseDir(dir), "closedir"); err != nil {
		d.log.Debug("closedir", "path", path, "err", err)
	}
	if result != nil {
		return nil, result
	}
	return entries, nil
}
