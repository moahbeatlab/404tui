package main

import (
	"strings"
	"testing"
)

func TestQRText(t *testing.T) {
	s, err := qrText("http://192.168.1.20:8404/?key=k7p2mdq4")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) < 10 || !strings.Contains(s, "█") {
		t.Fatalf("not a code:\n%s", s)
	}
}
