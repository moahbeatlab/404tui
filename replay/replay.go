// Package replay shows a recorded performance: the WAV and the event log beside it (see core/eventlog.go), in a page that draws what the
// unit did while the sound plays. It serves the page, the audio, the events and the analysis of the audio (the level and the spectrum of
// every frame) to a browser; the page can also be driven frame by frame, to be rendered into a video.
package replay

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/moahbeatlab/404tui/perflog"
	"github.com/moahbeatlab/404tui/web"
)

//go:embed static
var assets embed.FS

// Replay is a performance ready to be shown.
type Replay struct {
	AudioPath, EventsPath string
	Events                []perflog.Entry
	Analysis              *Analysis
	CuesPath              string // the title and the patterns that were set by hand, beside the event log
	mu                    sync.Mutex
	title                 string
	cues                  []Cue
}

// meta is what is kept beside the event log: the title of the video and the cues (an older file is just the list of cues).
type meta struct {
	Title string `json:"title,omitempty"`
	Cues  []Cue  `json:"cues"`
}

// Cue says which pattern plays from a time of the log on: the unit does not tell when the pattern is changed.
type Cue struct {
	T     float64 `json:"t"`
	Label string  `json:"label"`
	// Pattern and Pads keep what the pattern and its pads were, so that the cues stay a whole picture when the project has changed since
	// (the page fills them in; the server keeps them as they are).
	Pattern json.RawMessage `json:"pattern,omitempty"`
	Pads    json.RawMessage `json:"pads,omitempty"`
}

// CuesPathFor is where the cues of an event log are kept.
func CuesPathFor(events string) string { return strings.TrimSuffix(events, ".jsonl") + ".cues.json" }

// Cues are the patterns set by hand.
func (r *Replay) Cues() []Cue {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Cue{}, r.cues...)
}

// SetCues replaces the cues and keeps them in the file.
func (r *Replay) SetCues(c []Cue) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cues = append([]Cue{}, c...)
	return r.save()
}

// Title is the title of the video ("" when none was set).
func (r *Replay) Title() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.title
}

// SetTitle changes the title and keeps it in the file.
func (r *Replay) SetTitle(t string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.title = strings.TrimSpace(t)
	return r.save()
}

func (r *Replay) save() error {
	if r.title == "" && len(r.cues) == 0 {
		if err := os.Remove(r.CuesPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	b, _ := json.MarshalIndent(meta{Title: r.title, Cues: r.cues}, "", " ")
	return os.WriteFile(r.CuesPath, b, 0o644)
}

// Load reads the event log (the file beside the audio when eventsPath is empty) and analyses the audio.
func Load(audioPath, eventsPath string) (*Replay, error) {
	if _, err := os.Stat(audioPath); err != nil {
		return nil, err
	}
	if eventsPath == "" {
		eventsPath = perflog.PathFor(audioPath)
	}
	ev, err := perflog.Read(eventsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("no event log %s: pass it as the second argument", eventsPath)
		}
		return nil, err
	}
	an, err := Analyze(audioPath)
	if err != nil {
		return nil, err
	}
	r := &Replay{AudioPath: audioPath, EventsPath: eventsPath, Events: ev, Analysis: an, CuesPath: CuesPathFor(eventsPath)}
	if b, err := os.ReadFile(r.CuesPath); err == nil {
		var m meta
		if json.Unmarshal(b, &m) == nil {
			r.title, r.cues = m.Title, m.Cues
		} else {
			json.Unmarshal(b, &r.cues) // the first form of the file: the list of cues
		}
	}
	return r, nil
}

// Handler is the web side: the page and what it asks for.
func (r *Replay) Handler() http.Handler {
	mux := http.NewServeMux()
	page, _ := assets.ReadFile("static/replay.html")
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/" {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(page)
	})
	mux.HandleFunc("/fxdata.js", func(w http.ResponseWriter, req *http.Request) {
		b, err := web.Asset("fxdata.js")
		if err != nil {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Write(b)
	})
	mux.HandleFunc("/events.json", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"file": filepath.Base(r.AudioPath), "events": r.Events, "cues": r.Cues(), "title": r.Title()})
	})
	mux.HandleFunc("/title", func(w http.ResponseWriter, req *http.Request) {
		var t struct {
			Title string `json:"title"`
		}
		if req.Method != http.MethodPost || json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<16)).Decode(&t) != nil {
			http.Error(w, "POST {\"title\": ...}", http.StatusBadRequest)
			return
		}
		if err := r.SetTitle(t.Title); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/cues", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "POST the cues", http.StatusMethodNotAllowed)
			return
		}
		var c []Cue
		if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<20)).Decode(&c); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := r.SetCues(c); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/analysis.json", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(r.Analysis)
	})
	mux.HandleFunc("/audio", func(w http.ResponseWriter, req *http.Request) {
		f, err := os.Open(r.AudioPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		defer f.Close()
		st, _ := f.Stat()
		w.Header().Set("Content-Type", "audio/wav")
		http.ServeContent(w, req, filepath.Base(r.AudioPath), st.ModTime(), f) // Range requests: the page can seek
	})
	return mux
}

// Serve serves the replay on addr (":0" for any port) and returns the address to open and a function that stops it.
func (r *Replay) Serve(addr string) (url string, stop func(), err error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return "", nil, err
	}
	srv := &http.Server{Handler: r.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(l)
	host := l.Addr().String()
	if tcp, ok := l.Addr().(*net.TCPAddr); ok && (tcp.IP.IsUnspecified() || tcp.IP == nil) {
		host = fmt.Sprintf("localhost:%d", tcp.Port)
	}
	return "http://" + host + "/", func() { srv.Close() }, nil
}
