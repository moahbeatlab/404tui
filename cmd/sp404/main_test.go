package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorOnTheSimulatorWritesADebugLog(t *testing.T) {
	log := filepath.Join(t.TempDir(), "sub", "debug.log")
	if err := run([]string{"doctor", "-preview"}, options{simulate: true, trace: true, logPath: log}); err != nil {
		t.Fatalf("doctor failed on the simulated unit: %v", err)
	}
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, want := range []string{
		"404tui debug log", "revision=", "os=", "unit opened",
		"reader started", "level=DEBUG msg=tx what=status", "level=TRACE msg=>", "level=TRACE msg=<",
		"doctor check", "name=\"pad blocks (160)\"", "msg=read", "reader stopped",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the log lacks %q", want)
		}
	}
}

func TestNoLoggingFlagsMeansNoLogFile(t *testing.T) {
	log := filepath.Join(t.TempDir(), "debug.log")
	if err := run([]string{"status"}, options{simulate: true, logPath: log}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(log); err == nil {
		t.Error("a log file was written without -v or -trace")
	}
}

func TestDebugLevelLeavesOutTheWireBytes(t *testing.T) {
	log := filepath.Join(t.TempDir(), "debug.log")
	if err := run([]string{"pads"}, options{simulate: true, verbose: true, logPath: log}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(log)
	if !strings.Contains(string(b), "msg=tx") || strings.Contains(string(b), "level=TRACE") {
		t.Errorf("-v should log transactions but not wire bytes:\n%s", b[:min(len(b), 600)])
	}
}
