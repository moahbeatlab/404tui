package playback

import (
	"bytes"
	"testing"
)

func TestTheLoopTakesTheNextSoundAtTheEndOfARound(t *testing.T) {
	r := &loopReader{pcm: []byte{1, 1, 1, 1, 2, 2, 2, 2}}
	buf := make([]byte, 6)
	if _, err := r.Read(buf); err != nil || !bytes.Equal(buf, []byte{1, 1, 1, 1, 2, 2}) {
		t.Fatalf("first read %v %v", buf, err)
	}
	// The next sound waits: the round that plays goes on to its end, then the new one starts.
	r.mu.Lock()
	r.next = []byte{9, 9, 9, 9}
	r.mu.Unlock()
	buf = make([]byte, 10)
	if _, err := r.Read(buf); err != nil {
		t.Fatal(err)
	}
	if want := []byte{2, 2, 9, 9, 9, 9, 9, 9, 9, 9}; !bytes.Equal(buf, want) {
		t.Errorf("read %v, want %v", buf, want)
	}
	// Without a new one it goes round and round.
	r2 := &loopReader{pcm: []byte{1, 2, 3, 4}}
	buf = make([]byte, 10)
	r2.Read(buf)
	if want := []byte{1, 2, 3, 4, 1, 2, 3, 4, 1, 2}; !bytes.Equal(buf, want) {
		t.Errorf("read %v, want %v", buf, want)
	}
}

func TestAReplacedLoopGoesOnFromTheSamePlace(t *testing.T) {
	r := &loopReader{pcm: []byte{1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3, 3}}
	buf := make([]byte, 4)
	r.Read(buf)
	r.mu.Lock()
	r.now = []byte{7, 7, 7, 7, 8, 8, 8, 8, 9, 9, 9, 9}
	r.mu.Unlock()
	buf = make([]byte, 8)
	r.Read(buf)
	// It was at the second frame: the new sound goes on from its second frame.
	if want := []byte{8, 8, 8, 8, 9, 9, 9, 9}; !bytes.Equal(buf, want) {
		t.Errorf("read %v, want %v", buf, want)
	}
}
