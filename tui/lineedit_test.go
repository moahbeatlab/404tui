package tui

import "testing"

func TestPromptEditing(t *testing.T) {
	p := &prompt{buffer: "kick drum"}
	for _, k := range []string{"ctrl+a", "ctrl+f", "ctrl+f", "delete"} {
		p.edit(k)
	}
	p.insert("X")
	if p.buffer != "kiXk drum" {
		t.Fatalf("after ctrl+a, 2×ctrl+f, delete, X: %q", p.buffer)
	}
	p.edit("ctrl+e")
	p.insert("!")
	p.edit("ctrl+b")
	p.edit("backspace")
	if p.buffer != "kiXk dru!" {
		t.Fatalf("after ctrl+e, !, ctrl+b, backspace: %q", p.buffer)
	}
	p.edit("ctrl+w")
	if p.buffer != "kiXk !" {
		t.Fatalf("ctrl+w: %q", p.buffer)
	}
	p.edit("ctrl+a")
	p.edit("ctrl+k")
	if p.buffer != "" || p.edit("nonsense") {
		t.Fatalf("ctrl+k: %q", p.buffer)
	}
	p = &prompt{buffer: "héllo"}
	p.edit("ctrl+a")
	p.edit("right")
	p.edit("delete")
	if p.buffer != "hllo" {
		t.Fatalf("multibyte: %q", p.buffer)
	}
}
