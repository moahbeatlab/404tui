package proto

import "testing"

func TestLookups(t *testing.T) {
	if p, ok := ParamByName("Level"); !ok || p.ID != 0x69 {
		t.Errorf("ParamByName(Level) = %+v, %v", p, ok)
	}
	if id, inverted, ok := BankParam("bank-tempo-j"); !ok || id != 0x2A || inverted {
		t.Errorf("bank-tempo-j = %#x, %v, %v", id, inverted, ok)
	}
	if id, inverted, ok := BankParam("bank-volume-a"); !ok || id != 0x35 || !inverted {
		t.Errorf("bank-volume-a = %#x, %v, %v", id, inverted, ok)
	}
	if _, _, ok := BankParam("bank-tempo-k"); ok {
		t.Error("bank-tempo-k should not resolve")
	}
	for id, want := range map[byte]int{0x69: 3, 0x6F: 9, 0x7E: 23} {
		if got, ok := blockField(id); !ok || got != want {
			t.Errorf("blockField(%#x) = %d, %v; want %d", id, got, ok, want)
		}
	}
}
