package web

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// A WebSocket (RFC 6455) as far as this program needs it: text messages both ways, ping and close.
// Written here so that the program needs no library for it.

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

const (
	opText  = 0x1
	opClose = 0x8
	opPing  = 0x9
	opPong  = 0xA
)

// maxMessage is the longest message taken from a client: a command is a few dozen bytes.
const maxMessage = 1 << 16

// conn is one WebSocket on the server side.
type conn struct {
	c  net.Conn
	r  *bufio.Reader
	mu sync.Mutex // writes
}

// upgrade turns an HTTP request into a WebSocket, or answers with an error and returns false.
func upgrade(w http.ResponseWriter, r *http.Request) (*conn, bool) {
	if !headerHas(r.Header, "Connection", "upgrade") || !headerHas(r.Header, "Upgrade", "websocket") {
		http.Error(w, "a WebSocket is expected here", http.StatusBadRequest)
		return nil, false
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if r.Header.Get("Sec-WebSocket-Version") != "13" || key == "" {
		http.Error(w, "WebSocket version 13 is expected", http.StatusBadRequest)
		return nil, false
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "no WebSocket here", http.StatusInternalServerError)
		return nil, false
	}
	nc, rw, err := hj.Hijack()
	if err != nil {
		return nil, false
	}
	sum := sha1.Sum([]byte(key + wsGUID))
	accept := base64.StdEncoding.EncodeToString(sum[:])
	_, err = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + accept + "\r\n\r\n")
	if err == nil {
		err = rw.Flush()
	}
	if err != nil {
		nc.Close()
		return nil, false
	}
	if tc, ok := nc.(*net.TCPConn); ok {
		tc.SetNoDelay(true) // a command or a state is small and should not wait for company
	}
	return &conn{c: nc, r: rw.Reader}, true
}

func headerHas(h http.Header, name, token string) bool {
	for _, v := range h.Values(name) {
		for _, t := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(t), token) {
				return true
			}
		}
	}
	return false
}

// write sends one frame.
func (c *conn) write(op byte, p []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	hdr := []byte{0x80 | op}
	switch n := len(p); {
	case n < 126:
		hdr = append(hdr, byte(n))
	case n <= 0xFFFF:
		hdr = append(hdr, 126, byte(n>>8), byte(n))
	default:
		hdr = append(hdr, 127)
		hdr = binary.BigEndian.AppendUint64(hdr, uint64(n))
	}
	c.c.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if _, err := c.c.Write(append(hdr, p...)); err != nil {
		return err
	}
	return nil
}

// writeText sends a text message.
func (c *conn) writeText(p []byte) error { return c.write(opText, p) }

var errClosed = errors.New("websocket closed")

// readText waits for the next text message; pings are answered on the way and a close ends it.
// Messages in pieces (continuation frames) are put together.
func (c *conn) readText() ([]byte, error) {
	var msg []byte
	for {
		c.c.SetReadDeadline(time.Now().Add(60 * time.Second))
		var h [2]byte
		if _, err := io.ReadFull(c.r, h[:]); err != nil {
			return nil, err
		}
		fin, op := h[0]&0x80 != 0, h[0]&0x0F
		masked := h[1]&0x80 != 0
		n := uint64(h[1] & 0x7F)
		switch n {
		case 126:
			var b [2]byte
			if _, err := io.ReadFull(c.r, b[:]); err != nil {
				return nil, err
			}
			n = uint64(binary.BigEndian.Uint16(b[:]))
		case 127:
			var b [8]byte
			if _, err := io.ReadFull(c.r, b[:]); err != nil {
				return nil, err
			}
			n = binary.BigEndian.Uint64(b[:])
		}
		if !masked || n > maxMessage || uint64(len(msg))+n > maxMessage {
			return nil, errors.New("websocket frame not accepted")
		}
		var mask [4]byte
		if _, err := io.ReadFull(c.r, mask[:]); err != nil {
			return nil, err
		}
		data := make([]byte, n)
		if _, err := io.ReadFull(c.r, data); err != nil {
			return nil, err
		}
		for i := range data {
			data[i] ^= mask[i%4]
		}
		switch op {
		case opPing:
			c.write(opPong, data)
		case opPong:
		case opClose:
			c.write(opClose, nil)
			return nil, errClosed
		case opText, 0x0: // a text message, or the rest of one
			msg = append(msg, data...)
			if fin {
				return msg, nil
			}
		default:
			return nil, errors.New("websocket frame not accepted")
		}
	}
}

func (c *conn) close() { c.c.Close() }
