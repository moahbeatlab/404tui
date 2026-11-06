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
	"sort"
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
	TemplatesDir          string // where the templates are (default: the configuration folder of the user)
	fresh                 bool   // the take has nothing saved yet: a template can be laid over it
	CuesPath              string // the title and the patterns that were set by hand, beside the event log
	mu                    sync.Mutex
	title                 string
	style                 json.RawMessage
	captions              []Caption
	chapters              []Chapter
	cuts                  []Cut
	busNotes              []BusNote
	cameraFile            string
	camera                json.RawMessage
	adopted               bool
	cues                  []Cue
}

// Caption is a line of text shown from one time of the audio to another (seconds from the start of the audio).
type Caption struct {
	From float64 `json:"from"`
	To   float64 `json:"to"`
	Text string  `json:"text"`
	// Focus is what the picture is turned to while the caption shows ("pads", "pad:A3", "pattern", "bus:2", "buses", "level", "events", "tempo"); the rest is
	// blurred and dimmed, the target stays sharp. Empty for none.
	Focus string `json:"focus,omitempty"`
}

// BusNote says what a bus is used for from a time of the audio on (seconds): the effect assigned to a bus can change in the course of a take.
type BusNote struct {
	Bus  int     `json:"bus"` // 1 to 4
	T    float64 `json:"t"`
	Text string  `json:"text"`
}

// Chapter starts a part of the video at a time of the audio (seconds): the chapters of the video on YouTube, and the label on the picture.
type Chapter struct {
	T     float64 `json:"t"`
	Title string  `json:"title"`
}

// ChaptersText is the list as YouTube wants it in a description: "0:00 Title" a line each. The first chapter has to start at 0:00, so one is put
// there when the list does not begin with it.
func ChaptersText(chs []Chapter, cuts []Cut) string {
	c := append([]Chapter{}, chs...)
	for i := range c {
		c[i].T = OutTime(cuts, c[i].T) // the times of the video: what is cut before a chapter is taken off
	}
	sort.Slice(c, func(i, j int) bool { return c[i].T < c[j].T })
	if len(c) == 0 {
		return ""
	}
	if c[0].T > 0.5 {
		c = append([]Chapter{{T: 0, Title: "Intro"}}, c...)
	}
	var b strings.Builder
	for _, ch := range c {
		t := int(ch.T + 0.0001)
		if t >= 3600 {
			fmt.Fprintf(&b, "%d:%02d:%02d %s\n", t/3600, t/60%60, t%60, strings.TrimSpace(ch.Title))
		} else {
			fmt.Fprintf(&b, "%d:%02d %s\n", t/60, t%60, strings.TrimSpace(ch.Title))
		}
	}
	return b.String()
}

// meta is what is kept beside the event log: the title of the video and the cues (an older file is just the list of cues).
type meta struct {
	Title string `json:"title,omitempty"`
	Cues  []Cue  `json:"cues"`
	// PatternsAdopted says the patterns of the event log were taken over as cues, to be edited: then the cues are the whole list.
	PatternsAdopted bool      `json:"patterns_adopted,omitempty"`
	Captions        []Caption `json:"captions,omitempty"`
	Chapters        []Chapter `json:"chapters,omitempty"`
	BusNotes        []BusNote `json:"bus_notes,omitempty"`
	Cuts            []Cut     `json:"cuts,omitempty"`
	// CameraFile is the video of the player (a path, relative to this file when it is beside it); Camera is the page's own object about it
	// (the sync, the crop, the scenes, the box on the picture), kept as it is.
	CameraFile string          `json:"camera_file,omitempty"`
	Camera     json.RawMessage `json:"camera,omitempty"`
	Style      json.RawMessage `json:"style,omitempty"` // colours, logo, handle, background: the page's own object, kept as it is
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

// freshTemplate is the default template, for a take that has nothing saved yet (nil otherwise).
func (r *Replay) freshTemplate() json.RawMessage {
	r.mu.Lock()
	fresh := r.fresh
	r.mu.Unlock()
	if !fresh {
		return nil
	}
	return r.DefaultTemplate()
}

// CameraName is the name of the camera video ("" when there is none).
func (r *Replay) CameraName() string {
	if p := r.CameraPath(); p != "" {
		return filepath.Base(p)
	}
	return ""
}

// CameraPath is the camera video as a path that can be opened ("" when there is none).
func (r *Replay) CameraPath() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.resolveCamera()
}

func (r *Replay) resolveCamera() string {
	if r.cameraFile == "" {
		return ""
	}
	if filepath.IsAbs(r.cameraFile) {
		return r.cameraFile
	}
	return filepath.Join(filepath.Dir(r.CuesPath), r.cameraFile)
}

// SetCameraFile chooses the camera video; it has to be a file that exists. A path under the folder of the take is kept relative to it.
func (r *Replay) SetCameraFile(path string) error {
	path = strings.TrimSpace(path)
	r.mu.Lock()
	defer r.mu.Unlock()
	if path == "" {
		r.cameraFile = ""
		return r.save()
	}
	if strings.HasPrefix(path, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(h, path[2:])
		}
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(r.CuesPath), path)
	}
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("no such file: %s", path)
	}
	if !st.Mode().IsRegular() {
		return fmt.Errorf("not a file: %s", path)
	}
	if rel, err := filepath.Rel(filepath.Dir(r.CuesPath), path); err == nil && !strings.HasPrefix(rel, "..") {
		path = rel
	}
	r.cameraFile = path
	return r.save()
}

// Camera is the page's own object about the camera video.
func (r *Replay) Camera() json.RawMessage {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.camera) == 0 {
		return nil
	}
	return append(json.RawMessage{}, r.camera...)
}

// SetCamera keeps the page's object about the camera video (any JSON object; "{}" clears it).
func (r *Replay) SetCamera(b []byte) error {
	var probe map[string]any
	if len(b) > 0 && json.Unmarshal(b, &probe) != nil {
		return errors.New("the camera state is not a JSON object")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(probe) == 0 {
		r.camera = nil
	} else {
		r.camera = append(json.RawMessage{}, b...)
	}
	return r.save()
}

// BusNotes are the changes of what the buses are used for.
func (r *Replay) BusNotes() []BusNote {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]BusNote{}, r.busNotes...)
}

// SetBusNotes replaces the bus notes and keeps them in the file.
func (r *Replay) SetBusNotes(n []BusNote) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.busNotes = append([]BusNote{}, n...)
	return r.save()
}

// Chapters are the parts of the video.
func (r *Replay) Chapters() []Chapter {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Chapter{}, r.chapters...)
}

// SetChapters replaces the chapters and keeps them in the file.
func (r *Replay) SetChapters(c []Chapter) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.chapters = append([]Chapter{}, c...)
	return r.save()
}

// Cuts are the parts of the take that the video leaves out.
func (r *Replay) Cuts() []Cut {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Cut{}, r.cuts...)
}

// SetCuts replaces the cuts (sorted, clipped to the take, joined where they touch) and keeps them in the file.
func (r *Replay) SetCuts(c []Cut) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cuts = NormalizeCuts(c, r.Analysis.Duration)
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
	r.fresh = false
	if r.title == "" && len(r.cues) == 0 && len(r.style) == 0 && len(r.captions) == 0 && len(r.chapters) == 0 && len(r.cuts) == 0 && len(r.busNotes) == 0 && r.cameraFile == "" && len(r.camera) == 0 && !r.adopted {
		if err := os.Remove(r.CuesPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	b, _ := json.MarshalIndent(meta{Title: r.title, Cues: r.cues, PatternsAdopted: r.adopted, Captions: r.captions, Chapters: r.chapters, BusNotes: r.busNotes, Cuts: r.cuts, CameraFile: r.cameraFile, Camera: r.camera, Style: r.style}, "", " ")
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
			r.title, r.cues, r.style, r.captions, r.chapters, r.busNotes, r.adopted = m.Title, m.Cues, m.Style, m.Captions, m.Chapters, m.BusNotes, m.PatternsAdopted
			r.cuts = m.Cuts
			r.cameraFile, r.camera = m.CameraFile, m.Camera
		} else {
			json.Unmarshal(b, &r.cues) // the first form of the file: the list of cues
		}
	} else {
		r.fresh = true // nothing is kept for this take yet: the template new takes start with can be laid over it
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
		json.NewEncoder(w).Encode(map[string]any{"file": filepath.Base(r.AudioPath), "events": r.Events, "cues": r.Cues(), "title": r.Title(), "style": r.Style(), "captions": r.Captions(), "chapters": r.Chapters(), "cuts": r.Cuts(), "busnotes": r.BusNotes(), "adopted": r.Adopted(), "camera": r.Camera(), "camera_file": r.CameraName(), "default_template": r.freshTemplate()})
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
	mux.HandleFunc("/chapters", func(w http.ResponseWriter, req *http.Request) {
		var c []Chapter
		if req.Method != http.MethodPost || json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<20)).Decode(&c) != nil {
			http.Error(w, "POST the chapters", http.StatusBadRequest)
			return
		}
		if err := r.SetChapters(c); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/cuts", func(w http.ResponseWriter, req *http.Request) {
		var c []Cut
		if req.Method != http.MethodPost || json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<20)).Decode(&c) != nil {
			http.Error(w, "POST the cuts", http.StatusBadRequest)
			return
		}
		if err := r.SetCuts(c); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/templates", r.templateHandler)
	mux.HandleFunc("/templates/", r.templateHandler)
	mux.HandleFunc("/camera/file", func(w http.ResponseWriter, req *http.Request) {
		var f struct {
			Path string `json:"path"`
		}
		if req.Method != http.MethodPost || json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<16)).Decode(&f) != nil {
			http.Error(w, "POST {\"path\": ...}", http.StatusBadRequest)
			return
		}
		if err := r.SetCameraFile(f.Path); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"camera_file": r.CameraName()})
	})
	mux.HandleFunc("/camera", func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPost { // the state of the camera (sync, crop, scenes)
			b, err := io.ReadAll(http.MaxBytesReader(w, req.Body, 1<<20))
			if err == nil {
				err = r.SetCamera(b)
			}
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		path := r.CameraPath() // the video itself, with Range requests so that the page can seek
		f, err := os.Open(path)
		if path == "" || err != nil {
			http.NotFound(w, req)
			return
		}
		defer f.Close()
		st, _ := f.Stat()
		http.ServeContent(w, req, filepath.Base(path), st.ModTime(), f)
	})
	mux.HandleFunc("/busnotes", func(w http.ResponseWriter, req *http.Request) {
		var n []BusNote
		if req.Method != http.MethodPost || json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<20)).Decode(&n) != nil {
			http.Error(w, "POST the bus notes", http.StatusBadRequest)
			return
		}
		if err := r.SetBusNotes(n); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/chapters.txt", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, ChaptersText(r.Chapters(), r.Cuts()))
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
