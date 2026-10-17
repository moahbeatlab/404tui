// Package web serves the Perform view to a browser: a page (embedded) and a WebSocket over which the
// program sends its state, as JSON, whenever it changes, and the browser sends its commands. It is
// meant for a phone on the same network as the computer that runs 404tui.
package web

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:embed static
var assets embed.FS

// Server is the web side of the program.
type Server struct {
	// Key is what a browser must give (as ?key=) to connect; empty lets everyone on the network in.
	Key string
	// OnCommand gets each command a browser sends, as the JSON text it sent. It must not block.
	OnCommand func(msg []byte)

	mu      sync.Mutex
	latest  []byte
	clients map[*client]struct{}
}

// client is a connected browser: it wakes when there is a newer state and is sent the latest one, so a
// slow phone skips states instead of falling behind.
type client struct {
	c    *conn
	wake chan struct{}
	sent []byte
}

// Publish makes state the one every browser is sent next. It does not wait for the browsers.
func (s *Server) Publish(state []byte) {
	s.mu.Lock()
	if bytes.Equal(state, s.latest) {
		s.mu.Unlock()
		return
	}
	s.latest = state
	for c := range s.clients {
		select {
		case c.wake <- struct{}{}:
		default:
		}
	}
	s.mu.Unlock()
}

// Clients is how many browsers are connected.
func (s *Server) Clients() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.clients)
}

// Handler is the page and the WebSocket (at /ws).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(assets, "static")
	files := http.FileServer(http.FS(static))
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	}))
	mux.HandleFunc("/ws", s.serveWS)
	return mux
}

func (s *Server) authorized(r *http.Request) bool {
	if s.Key != "" {
		return subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("key")), []byte(s.Key)) == 1
	}
	// Without a key a page of another site must not be able to drive the program through the
	// browser of someone on the network: the page that opens the socket has to be this one.
	if o := r.Header.Get("Origin"); o != "" {
		u, err := url.Parse(o)
		return err == nil && u.Host == r.Host
	}
	return true
}

func (s *Server) serveWS(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "wrong key", http.StatusForbidden)
		return
	}
	c, ok := upgrade(w, r)
	if !ok {
		return
	}
	cl := &client{c: c, wake: make(chan struct{}, 1)}
	s.mu.Lock()
	if s.clients == nil {
		s.clients = map[*client]struct{}{}
	}
	s.clients[cl] = struct{}{}
	s.mu.Unlock()
	cl.wake <- struct{}{} // the state as it is now
	done := make(chan struct{})
	go func() { // what the browser says
		defer close(done)
		for {
			msg, err := c.readText()
			if err != nil {
				return
			}
			if bytes.HasPrefix(msg, []byte(`{"a":"ping"`)) {
				c.writeText([]byte(`{"pong":1}`)) // for the browser to measure the round trip
				continue
			}
			if s.OnCommand != nil {
				s.OnCommand(msg)
			}
		}
	}()
	ping := time.NewTicker(20 * time.Second)
	defer func() {
		ping.Stop()
		s.mu.Lock()
		delete(s.clients, cl)
		s.mu.Unlock()
		c.close()
	}()
	for {
		select {
		case <-done:
			return
		case <-ping.C:
			if c.write(opPing, nil) != nil {
				return
			}
		case <-cl.wake:
			s.mu.Lock()
			state := s.latest
			s.mu.Unlock()
			if state == nil || string(state) == string(cl.sent) {
				continue
			}
			cl.sent = state
			if c.writeText(state) != nil {
				return
			}
		}
	}
}

// Listen starts serving on addr (":8404"), and returns once it is listening; the listener is returned
// for the port it got and to close it.
func (s *Server) Listen(addr string) (net.Listener, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(l)
	return l, nil
}

// URLs are the addresses to type into a phone, for the port a listener got: this computer's addresses on
// the networks it is on, the one in a private range first.
func (s *Server) URLs(l net.Listener) []string {
	port := ""
	if a, ok := l.Addr().(*net.TCPAddr); ok {
		port = ":" + itoa(a.Port)
	}
	var ips []string
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && !n.IP.IsLoopback() && !n.IP.IsLinkLocalUnicast() {
			ips = append(ips, n.IP.String())
		}
	}
	sort.SliceStable(ips, func(i, j int) bool { return isPrivate(ips[i]) && !isPrivate(ips[j]) })
	if len(ips) == 0 {
		ips = []string{"localhost"}
	}
	var out []string
	for _, ip := range ips {
		u := "http://" + ip + port + "/"
		if s.Key != "" {
			u += "?key=" + s.Key
		}
		out = append(out, u)
	}
	return out
}

func isPrivate(ip string) bool {
	p := net.ParseIP(ip)
	return p != nil && p.IsPrivate()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

// LoadKey reads the key kept for this user, or makes one and keeps it, so that the address on a phone
// stays the same from one run to the next.
func LoadKey() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return newKey()
	}
	path := filepath.Join(dir, "404tui", "web-key")
	if b, err := os.ReadFile(path); err == nil {
		if k := strings.TrimSpace(string(b)); len(k) >= 6 {
			return k
		}
	}
	k := newKey()
	if os.MkdirAll(filepath.Dir(path), 0o700) == nil {
		os.WriteFile(path, []byte(k+"\n"), 0o600)
	}
	return k
}

func newKey() string {
	const letters = "abcdefghjkmnpqrstuvwxyz23456789" // no letters that look like others
	b := make([]byte, 8)
	rand.Read(b)
	for i := range b {
		b[i] = letters[int(b[i])%len(letters)]
	}
	return string(b)
}
