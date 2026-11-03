package lab

import "github.com/moahbeatlab/404tui/formats"

// Snap is the project as it was at a moment, to go back to: undo. The lab replaces a project file with a new buffer when it
// changes one and never edits a buffer in place, so a snapshot shares the file buffers (cheap, however big the samples) and
// copies only the pad settings.
type Snap struct {
	files map[string][]byte
	pad   *formats.Padconf
}

// Snapshot takes the project as it is.
func (l *Lab) Snapshot() *Snap {
	s := &Snap{files: make(map[string][]byte, len(l.files))}
	for k, v := range l.files {
		s.files[k] = v
	}
	if l.pad != nil {
		s.pad = l.pad.Clone()
	}
	return s
}

// Restore puts the project back as the snapshot has it. The samples being worked on (the decoded ones) are not part of it.
func (l *Lab) Restore(s *Snap) {
	l.files = make(map[string][]byte, len(s.files))
	for k, v := range s.files {
		l.files[k] = v
	}
	l.pad = nil
	if s.pad != nil {
		l.pad = s.pad.Clone()
	}
}

func sameBuf(a, b []byte) bool { return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0]) }

// Changed says whether the project differs from the snapshot.
func (l *Lab) Changed(s *Snap) bool {
	if len(l.files) != len(s.files) {
		return true
	}
	for k, v := range l.files {
		if o, ok := s.files[k]; !ok || !sameBuf(v, o) {
			return true
		}
	}
	return !l.pad.Equal(s.pad)
}

// HeldBytes is how much file data the snapshot holds that the newer state (a snapshot, or nil for the project as it is now)
// does not: what keeping it costs.
func (s *Snap) HeldBytes(newer *Snap) int {
	n := 0
	for k, v := range s.files {
		if newer != nil {
			if o, ok := newer.files[k]; ok && sameBuf(v, o) {
				continue
			}
		}
		n += len(v)
	}
	return n
}

// HeldBytesNow is HeldBytes against the project as it is.
func (l *Lab) HeldBytesNow(s *Snap) int {
	n := 0
	for k, v := range s.files {
		if o, ok := l.files[k]; ok && sameBuf(v, o) {
			continue
		}
		n += len(v)
	}
	return n
}
