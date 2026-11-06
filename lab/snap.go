package lab

import "github.com/moahbeatlab/404tui/formats"

// Snap is the project as it was at a moment, to go back to: undo. The lab replaces a project file with a new buffer when it
// changes one and never edits a buffer in place, so a snapshot shares the file buffers (cheap, however big the samples) and
// copies only the pad settings.
type Snap struct {
	files  map[string][]byte
	remote map[int][]byte
	pad    *formats.Padconf
}

// Snapshot takes the project as it is.
func (l *Lab) Snapshot() *Snap {
	s := &Snap{files: make(map[string][]byte, len(l.files))}
	for k, v := range l.files {
		s.files[k] = v
	}
	if len(l.remote) > 0 {
		s.remote = make(map[int][]byte, len(l.remote))
		for k, v := range l.remote {
			s.remote[k] = v
		}
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
	l.remote = nil
	if len(s.remote) > 0 {
		l.remote = make(map[int][]byte, len(s.remote))
		for k, v := range s.remote {
			l.remote[k] = v
		}
	}
	l.pad = nil
	if s.pad != nil {
		l.pad = s.pad.Clone()
	}
}

func sameBuf(a, b []byte) bool { return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0]) }

// Changed says whether the project differs from the snapshot.
func (l *Lab) Changed(s *Snap) bool {
	for k, v := range l.files {
		o, ok := s.files[k]
		if !ok {
			if s.wasRemote(k) { // fetched from the unit since: the same sample, not a change
				continue
			}
			return true
		}
		if !sameBuf(v, o) {
			return true
		}
	}
	for k := range s.files {
		if _, ok := l.files[k]; !ok {
			return true
		}
	}
	for n := range s.remote { // a sample that was on the unit is still there, or was fetched: otherwise it is gone
		if _, ok := l.remote[n]; !ok {
			if _, loaded := l.files[padFile(n)]; !loaded {
				return true
			}
		}
	}
	for n := range l.remote {
		if _, ok := s.remote[n]; !ok {
			return true
		}
	}
	return !l.pad.Equal(s.pad)
}

// wasRemote says whether the file was a sample still on the unit when the snapshot was taken.
func (s *Snap) wasRemote(name string) bool {
	for n := range s.remote {
		if padFile(n) == name {
			return true
		}
	}
	return false
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
