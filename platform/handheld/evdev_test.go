package handheld

import (
	"testing"
	"time"

	"github.com/moahbeatlab/404tui/core"
)

func TestTheR36SPadsCodes(t *testing.T) {
	if defaultMap[0x136] != core.L1 || defaultMap[0x137] != core.R1 {
		t.Fatal("0x136 and 0x137 are L1 and R1 on the R36S's pad")
	}
	if defaultMap[0x139] != core.R2 || defaultMap[0x138] != core.L2 {
		t.Fatal("R2 is 0x139 and L2 is 0x138")
	}
	for _, c := range []uint16{0x13a, 0x2c0, 0x13c} {
		if defaultMap[c] != core.Sel {
			t.Fatalf("0x%x is Select", c)
		}
	}
}

func TestTheChainCountsTheTimeAKeyIsHeld(t *testing.T) {
	// the chain counts the time held, whatever the pad sends (value 1 again and again, or value 2)
	c := newChain()
	t0 := time.Now()
	if r := c.down(core.Down, t0); r != 0 {
		t.Fatalf("the first press: %d", r)
	}
	var last int
	for i := 1; i <= 12; i++ { // a press every 170 ms, never let go
		last = c.down(core.Down, t0.Add(time.Duration(i)*170*time.Millisecond))
		if last < 1 {
			t.Fatalf("repeat %d is %d", i, last)
		}
	}
	if last < 8 {
		t.Fatalf("held for two seconds: repeat %d", last)
	}
	// a pause starts a new hold
	if r := c.down(core.Down, t0.Add(5*time.Second)); r != 0 {
		t.Fatalf("after a pause: %d", r)
	}
}
