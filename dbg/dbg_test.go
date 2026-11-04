package dbg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheLogIsOffUntilItIsTurnedOnAndWritesToTheFile(t *testing.T) {
	Logger.Info("nobody hears this")
	if Enabled() || Path() != "" {
		t.Fatal("the log starts off")
	}
	path := filepath.Join(t.TempDir(), "sub", "debug.log")
	got, err := Set(Basic, path)
	if err != nil || got != path || !Enabled() || Mode() != Basic {
		t.Fatalf("on: %q %v", got, err)
	}
	Logger.Info("hello", "n", 3)
	Logger.With("area", "x").Debug("with attributes")
	Logger.Log(nil, -8, "a wire byte") // trace: below the level of the basic log
	Set(Off, "")
	Logger.Info("not written")
	b, _ := os.ReadFile(path)
	txt := string(b)
	for _, want := range []string{"404tui debug log", "version=", "hello", "n=3", "with attributes", "area=x"} {
		if !strings.Contains(txt, want) {
			t.Errorf("the log has %q:\n%s", want, txt)
		}
	}
	for _, not := range []string{"nobody hears", "not written", "wire byte"} {
		if strings.Contains(txt, not) {
			t.Errorf("the log has %q", not)
		}
	}
	// trace adds the wire
	Set(Trace, path)
	Logger.Log(nil, -8, "a wire byte")
	Set(Off, "")
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), "TRACE") {
		t.Error("the trace level writes the wire")
	}
}
