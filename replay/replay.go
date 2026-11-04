// Package replay shows a recorded performance: the WAV and the event log beside it (see core/eventlog.go), in a page that draws what the
// unit did while the sound plays. It serves the page, the audio, the events and the analysis of the audio (the level and the spectrum of
// every frame) to a browser; the page can also be driven frame by frame, to be rendered into a video.
package replay

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	style                 json.RawMessage
	captions              []Caption
	adopted               bool
	cues                  []Cue
}

// Caption is a line of text shown from one time of the audio to another (seconds from the start of the audio).
type Caption struct {
	From float64 `json:"from"`
	To   float64 `json:"to"`
	Text string  `json:"text"`
}

// meta is what is kept beside the event log: the title of the video and the cues (an older file is just the list of cues).
type meta struct {
	Title string `json:"title,omitempty"`
	Cues  []Cue  `json:"cues"`
	// PatternsAdopted says the patterns of the event log were taken over as cues, to be edited: then the cues are the whole list.
	PatternsAdopted bool            `json:"patterns_adopted,omitempty"`
	Captions        []Caption       `json:"captions,omitempty"`
	Style           json.RawMessage `json:"style,omitempty"` // colours, logo, handle, background: the page's own object, kept as it is
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

// Adopted says the cues are the whole list of patterns (the log's own are left out).
func (r *Replay) Adopted() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.adopted
}

// SetPatterns replaces the cues and says whether they are the whole list of patterns.
func (r *Replay) SetPatterns(c []Cue, adopted bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cues, r.adopted = append([]Cue{}, c...), adopted
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

// Captions are the lines of text shown at times.
func (r *Replay) Captions() []Caption {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Caption{}, r.captions...)
}

// SetCaptions replaces the captions and keeps them in the file.
func (r *Replay) SetCaptions(c []Caption) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.captions = append([]Caption{}, c...)
	return r.save()
}

// Style is the look of the video as the page saved it.
func (r *Replay) Style() json.RawMessage {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.style) == 0 {
		return nil // null in the answer
	}
	return append(json.RawMessage{}, r.style...)
}

// SetStyle keeps the look of the video (any JSON object; "{}" or empty clears it).
func (r *Replay) SetStyle(b []byte) error {
	var probe map[string]any
	if len(b) > 0 && json.Unmarshal(b, &probe) != nil {
		return errors.New("the style is not a JSON object")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(probe) == 0 {
		r.style = nil
	} else {
		r.style = append(json.RawMessage{}, b...)
	}
	return r.save()
}

func (r *Replay) save() error {
	if r.title == "" && len(r.cues) == 0 && len(r.style) == 0 && len(r.captions) == 0 && !r.adopted {
		if err := os.Remove(r.CuesPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	b, _ := json.MarshalIndent(meta{Title: r.title, Cues: r.cues, PatternsAdopted: r.adopted, Captions: r.captions, Style: r.style}, "", " ")
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
			r.title, r.cues, r.style, r.captions, r.adopted = m.Title, m.Cues, m.Style, m.Captions, m.PatternsAdopted
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
		json.NewEncoder(w).Encode(map[string]any{"file": filepath.Base(r.AudioPath), "events": r.Events, "cues": r.Cues(), "title": r.Title(), "style": r.Style(), "captions": r.Captions(), "adopted": r.Adopted()})
	})
	mux.HandleFunc("/captions", func(w http.ResponseWriter, req *http.Request) {
		var c []Caption
		if req.Method != http.MethodPost || json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<20)).Decode(&c) != nil {
			http.Error(w, "POST the captions", http.StatusBadRequest)
			return
		}
		if err := r.SetCaptions(c); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/style", func(w http.ResponseWriter, req *http.Request) {
		b, err := io.ReadAll(http.MaxBytesReader(w, req.Body, 8<<20)) // a logo and a background picture are in it
		if req.Method != http.MethodPost || err != nil {
			http.Error(w, "POST the style", http.StatusBadRequest)
			return
		}
		if err := r.SetStyle(b); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
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
		// the list of cues, or {"cues": [...], "adopted": true}
		raw, err := io.ReadAll(http.MaxBytesReader(w, req.Body, 4<<20))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var c []Cue
		adopted := r.Adopted()
		if t := bytes.TrimSpace(raw); len(t) > 0 && t[0] == '{' {
			var o struct {
				Cues    []Cue `json:"cues"`
				Adopted bool  `json:"adopted"`
			}
			if err := json.Unmarshal(t, &o); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			c, adopted = o.Cues, o.Adopted
		} else if err := json.Unmarshal(raw, &c); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := r.SetPatterns(c, adopted); err != nil {
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
