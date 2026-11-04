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
	return &Replay{AudioPath: audioPath, EventsPath: eventsPath, Events: ev, Analysis: an}, nil
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
		json.NewEncoder(w).Encode(map[string]any{"file": filepath.Base(r.AudioPath), "events": r.Events})
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
