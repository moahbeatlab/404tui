package web

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// client is a bare WebSocket client for the tests: masked frames out, plain frames in.
type testClient struct {
	c net.Conn
	r *bufio.Reader
}

func dial(t *testing.T, addr, path string) (*testClient, string) {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef"))
	fmt.Fprintf(c, "GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n", path, addr, key)
	r := bufio.NewReader(c)
	line, _ := r.ReadString('\n')
	var head strings.Builder
	for {
		l, err := r.ReadString('\n')
		if err != nil || l == "\r\n" {
			break
		}
		head.WriteString(l)
	}
	if strings.Contains(line, "101") {
		sum := sha1.Sum([]byte(key + wsGUID))
		if !strings.Contains(head.String(), base64.StdEncoding.EncodeToString(sum[:])) {
			t.Fatalf("wrong Sec-WebSocket-Accept: %s", head.String())
		}
	}
	return &testClient{c: c, r: r}, line
}

func (c *testClient) send(op byte, p []byte) {
	mask := []byte{1, 2, 3, 4}
	f := []byte{0x80 | op}
	switch n := len(p); {
	case n < 126:
		f = append(f, 0x80|byte(n))
	default:
		f = append(f, 0x80|126, byte(n>>8), byte(n))
	}
	f = append(f, mask...)
	for i, b := range p {
		f = append(f, b^mask[i%4])
	}
	c.c.Write(f)
}

func (c *testClient) read(t *testing.T) (byte, string) {
	t.Helper()
	c.c.SetReadDeadline(time.Now().Add(2 * time.Second))
	var h [2]byte
	if _, err := io.ReadFull(c.r, h[:]); err != nil {
		t.Fatalf("read: %v", err)
	}
	n := int(h[1] & 0x7F)
	if n == 126 {
		var b [2]byte
		io.ReadFull(c.r, b[:])
		n = int(b[0])<<8 | int(b[1])
	}
	buf := make([]byte, n)
	io.ReadFull(c.r, buf)
	return h[0] & 0x0F, string(buf)
}

func start(t *testing.T, s *Server) string {
	t.Helper()
	l, err := s.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	return l.Addr().String()
}

func TestTheServerSendsTheLatestStateAtOnceAndEachNewOne(t *testing.T) {
	s := &Server{}
	addr := start(t, s)
	s.Publish([]byte(`{"n":1}`))
	c, line := dial(t, addr, "/ws")
	if !strings.Contains(line, "101") {
		t.Fatalf("no upgrade: %s", line)
	}
	if _, msg := c.read(t); msg != `{"n":1}` {
		t.Errorf("first state %q", msg)
	}
	s.Publish([]byte(`{"n":2}`))
	if _, msg := c.read(t); msg != `{"n":2}` {
		t.Errorf("second state %q", msg)
	}
	// The same state again is not sent again; a later one is.
	s.Publish([]byte(`{"n":2}`))
	s.Publish([]byte(`{"n":3}`))
	if _, msg := c.read(t); msg != `{"n":3}` {
		t.Errorf("third state %q", msg)
	}
	if s.Clients() != 1 {
		t.Errorf("%d clients", s.Clients())
	}
}

func TestACommandReachesTheProgramAndAPingIsAnswered(t *testing.T) {
	got := make(chan string, 4)
	s := &Server{OnCommand: func(m []byte) { got <- string(m) }}
	addr := start(t, s)
	c, _ := dial(t, addr, "/ws")
	c.send(opText, []byte(`{"a":"pad","i":3}`))
	select {
	case m := <-got:
		if m != `{"a":"pad","i":3}` {
			t.Errorf("command %q", m)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the command did not arrive")
	}
	c.send(opText, []byte(`{"a":"ping","t":1}`))
	if _, msg := c.read(t); !strings.Contains(msg, "pong") {
		t.Errorf("no pong: %q", msg)
	}
	select {
	case m := <-got:
		t.Errorf("a ping went to the program: %q", m)
	case <-time.After(100 * time.Millisecond):
	}
	// A ping frame is answered with a pong frame; a long message (two length bytes) comes through.
	c.send(opPing, []byte("x"))
	if op, _ := c.read(t); op != opPong {
		t.Errorf("a ping frame was answered with %d", op)
	}
	long := `{"a":"x","pad":"` + strings.Repeat("y", 300) + `"}`
	c.send(opText, []byte(long))
	if m := <-got; m != long {
		t.Error("a long message was changed")
	}
}

func TestOnlyABrowserWithTheKeyOrFromThisPageGetsIn(t *testing.T) {
	s := &Server{Key: "secret"}
	addr := start(t, s)
	if _, line := dial(t, addr, "/ws"); !strings.Contains(line, "403") {
		t.Errorf("no key: %s", line)
	}
	if _, line := dial(t, addr, "/ws?key=wrong"); !strings.Contains(line, "403") {
		t.Errorf("wrong key: %s", line)
	}
	if _, line := dial(t, addr, "/ws?key=secret"); !strings.Contains(line, "101") {
		t.Errorf("right key: %s", line)
	}
	// Without a key, a page of another site is turned away.
	open := &Server{}
	addr2 := start(t, open)
	req, _ := http.NewRequest("GET", "http://"+addr2+"/ws", nil)
	req.Header.Set("Origin", "http://evil.example")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Errorf("another site's page got status %d", resp.StatusCode)
	}
}

func TestThePageIsServedAndAPlainRequestToTheSocketIsRefused(t *testing.T) {
	addr := start(t, &Server{})
	for path, want := range map[string]string{"/": "<title>404tui</title>", "/app.js": "WebSocket", "/style.css": "--accent", "/manifest.webmanifest": "404tui"} {
		resp, err := http.Get("http://" + addr + path)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || !strings.Contains(string(b), want) {
			t.Errorf("%s: status %d, no %q", path, resp.StatusCode, want)
		}
	}
	resp, _ := http.Get("http://" + addr + "/ws")
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("a plain GET of /ws gave %d", resp.StatusCode)
	}
}

func TestASlowBrowserSkipsStatesAndAClosedOneIsForgotten(t *testing.T) {
	s := &Server{}
	addr := start(t, s)
	c, _ := dial(t, addr, "/ws")
	for i := range 200 {
		s.Publish([]byte(fmt.Sprintf(`{"n":%d}`, i)))
	}
	var last string
	for {
		c.c.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
		var h [2]byte
		if _, err := io.ReadFull(c.r, h[:]); err != nil {
			break
		}
		buf := make([]byte, h[1]&0x7F)
		io.ReadFull(c.r, buf)
		last = string(buf)
	}
	if last != `{"n":199}` {
		t.Errorf("the last state was %q", last)
	}
	c.send(opClose, nil)
	c.c.Close()
	for range 50 {
		if s.Clients() == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("a closed browser is still a client")
}

func TestTheKeyIsKeptAndTheAddressesAreListed(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	k1 := LoadKey()
	if len(k1) != 8 || k1 != LoadKey() {
		t.Errorf("the key %q is not kept", k1)
	}
	s := &Server{Key: "abc"}
	l, _ := s.Listen("127.0.0.1:0")
	defer l.Close()
	urls := s.URLs(l)
	if len(urls) == 0 || !strings.HasSuffix(urls[0], "/?key=abc") || !strings.HasPrefix(urls[0], "http://") {
		t.Errorf("urls %v", urls)
	}
}
