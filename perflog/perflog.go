// Package perflog writes the events of a performance next to its audio recording: one JSON object
// per line, each with its time in seconds from the start of the recording. The first line says what
// the file belongs to. Replaying the file with the audio shows what was pressed and when.
package perflog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Version is the version of the line format; it is on the first line.
const Version = 1

// Pair is one field of a line.
type Pair struct {
	K string
	V any
}

// P makes a Pair.
func P(k string, v any) Pair { return Pair{k, v} }

// PathFor is the log that goes with an audio file: the same name with .events.jsonl.
func PathFor(audio string) string {
	return strings.TrimSuffix(audio, filepath.Ext(audio)) + ".events.jsonl"
}

// Log is a file being written. It is safe to use from several goroutines.
type Log struct {
	Path  string
	start time.Time
	mu    sync.Mutex
	f     *os.File
	n     int
}

// Create opens the log of the recording that began at start (the time its audio counts from).
func Create(path, audio string, start time.Time) (*Log, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	l := &Log{Path: path, start: start, f: f}
	l.write(fmt.Sprintf(`{"k":"start","version":%d,"audio":%s,"started":%s}`,
		Version, quote(filepath.Base(audio)), quote(start.Format(time.RFC3339Nano))))
	return l, nil
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func (l *Log) write(line string) {
	if _, err := l.f.WriteString(line + "\n"); err == nil {
		l.n++
	}
}

// Add writes an event that happened at the given time.
func (l *Log) Add(at time.Time, kind string, fields ...Pair) {
	if l == nil {
		return
	}
	var b strings.Builder
	t := at.Sub(l.start).Seconds()
	fmt.Fprintf(&b, `{"t":%.3f,"k":%s`, t, quote(kind))
	for _, p := range fields {
		b.WriteString(",")
		b.WriteString(quote(p.K))
		b.WriteString(":")
		switch v := p.V.(type) {
		case int:
			b.WriteString(strconv.Itoa(v))
		case float64:
			b.WriteString(strconv.FormatFloat(v, 'f', 2, 64))
		case bool:
			b.WriteString(strconv.FormatBool(v))
		case string:
			b.WriteString(quote(v))
		default:
			j, _ := json.Marshal(v)
			b.Write(j)
		}
	}
	b.WriteString("}")
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f != nil {
		l.write(b.String())
	}
}

// Count is how many lines have been written, the first one included.
func (l *Log) Count() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.n
}

// Close finishes the file.
func (l *Log) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}

// Entry is a line read back.
type Entry map[string]any

// Read reads a log file.
func Read(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []Entry
	for i, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		out = append(out, e)
	}
	return out, nil
}
