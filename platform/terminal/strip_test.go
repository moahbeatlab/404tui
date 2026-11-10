package terminal

import (
	"strings"
	"testing"
)

// SEQ: p puts the arrows on the row of the bank's 16 patterns, so another pattern is chosen without the tools menu
func TestOnSeqPChoosesThePatternInTheRowOfBoxes(t *testing.T) {
	m := newModel(t)
	press(m, "x")
	press(m, "y")
	press(m, "l") // SEQ
	if !strings.Contains(m.text(), "PATTERN A1") {
		t.Fatalf("not on SEQ:\n%s", m.text())
	}
	press(m, "p")
	if txt := m.text(); !strings.Contains(txt, "choose the pattern") {
		t.Fatalf("the panel does not say what the arrows do:\n%s", txt)
	}
	press(m, "l")
	press(m, "l")
	if !strings.Contains(m.text(), "PATTERN A3") {
		t.Fatalf("l twice should show pattern A3:\n%s", m.text())
	}
	press(m, "h")
	if !strings.Contains(m.text(), "PATTERN A2") {
		t.Fatal("h goes back one pattern")
	}
	press(m, "]") // the next bank, at the same place
	if !strings.Contains(m.text(), "PATTERN B2") {
		t.Fatalf("] is the bank:\n%s", m.text())
	}
	press(m, "p") // back to the grid
	press(m, "l")
	if !strings.Contains(m.text(), "PATTERN B2") || strings.Contains(m.text(), "choose the pattern") {
		t.Fatal("after p the arrows move the cursor in the grid again")
	}
}
