package core

import (
	"bytes"
	"fmt"
	"hash/maphash"
	"sync"

	"github.com/moahbeatlab/404tui/device"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// Live editing: the project in the editor is the unit's, and what is changed in the editor is written to the unit as it is
// changed. The editor stays the one place where everything is done (so every feature works the same, with undo); after each
// change what differs from what the unit was last given is worked out and sent in the background, one thing after the other:
// a pad's samples, its settings and name, a pattern.

type liveBase struct {
	params   [160]map[byte]int32
	names    [160]string
	hasFile  [160]bool
	fileHash [160]uint64
	fileKey  [160]fileKey // what the hash was worked out from: the same bytes in the same place need no new hash
	patterns map[int][]byte
}

type fileKey struct {
	p *byte
	n int
}

type liveChange struct {
	kind  string // "sample", "clear", "param", "name", "pattern", "unpattern"
	pad   int    // 0..159, or the pattern slot 1..160
	id    byte
	value int32
	name  string
	data  []byte
}

type liveState struct {
	dev     *device.Device
	project int
	base    liveBase
	seed    maphash.Seed

	mu      sync.Mutex
	queue   []liveChange
	running bool
	sent    int
	err     error
}

// goLive makes the project in the editor the live one: the unit's project n.
func (a *App) goLive(dev *device.Device, n int) {
	if err := dev.SelectProject(uint8(n)); err != nil {
		a.say(err.Error(), true)
		return
	}
	l := &liveState{dev: dev, project: n, seed: maphash.MakeSeed()}
	l.base = a.liveBaseNow(l)
	a.live = l
	a.say(fmt.Sprintf("live: the editor is unit project %d; what you change goes to the unit", n+1), false)
}

func (a *App) stopLive() {
	if a.live != nil {
		a.live = nil
		a.say("live editing stopped", false)
	}
}

// liveBaseNow reads what the editor holds, to compare with later.
func (a *App) liveBaseNow(l *liveState) liveBase {
	b := liveBase{patterns: map[int][]byte{}}
	old := l.base
	for n := 0; n < 160; n++ {
		p := a.info.Pads[n]
		b.names[n], b.hasFile[n] = p.Name, p.File
		if !p.File {
			continue
		}
		b.params[n] = a.lab.PadParams(n)
		if f, ok := a.lab.PadFile(n); ok && len(f) > 0 {
			k := fileKey{&f[0], len(f)}
			if old.fileKey[n] == k && old.hasFile[n] {
				b.fileKey[n], b.fileHash[n] = k, old.fileHash[n] // the sample is where it was
			} else {
				b.fileKey[n], b.fileHash[n] = k, maphash.Bytes(l.seed, f)
			}
		}
	}
	for s := 1; s <= 160; s++ {
		if pb, err := a.lab.PatternBytes(s); err == nil && len(pb) > 0 {
			b.patterns[s] = append([]byte(nil), pb...)
		}
	}
	return b
}

// liveSync works out what changed since the unit was last given something, and sends it.
func (a *App) liveSync() {
	l := a.live
	if l == nil {
		return
	}
	a.refresh() // what the editor holds now
	now := a.liveBaseNow(l)
	var ch []liveChange
	for n := 0; n < 160; n++ {
		switch {
		case !now.hasFile[n] && l.base.hasFile[n]:
			ch = append(ch, liveChange{kind: "clear", pad: n})
		case now.hasFile[n] && (!l.base.hasFile[n] || now.fileHash[n] != l.base.fileHash[n]):
			f, _ := a.lab.PadFile(n)
			ch = append(ch, liveChange{kind: "sample", pad: n, name: now.names[n], data: append([]byte(nil), f...)})
			for id, v := range now.params[n] { // the settings go with a new sample
				ch = append(ch, liveChange{kind: "param", pad: n, id: id, value: v})
			}
		case now.hasFile[n]:
			if now.names[n] != l.base.names[n] {
				ch = append(ch, liveChange{kind: "name", pad: n, name: now.names[n]})
			}
			for id, v := range now.params[n] {
				if old, ok := l.base.params[n][id]; !ok || old != v {
					ch = append(ch, liveChange{kind: "param", pad: n, id: id, value: v})
				}
			}
		}
	}
	for s := 1; s <= 160; s++ {
		was, had := l.base.patterns[s]
		cur, has := now.patterns[s]
		switch {
		case has && (!had || !bytes.Equal(was, cur)):
			ch = append(ch, liveChange{kind: "pattern", pad: s, data: cur})
		case !has && had:
			ch = append(ch, liveChange{kind: "unpattern", pad: s})
		}
	}
	l.base = now
	if len(ch) == 0 {
		return
	}
	l.mu.Lock()
	l.queue = coalesce(l.queue, ch)
	start := !l.running
	l.running = true
	l.mu.Unlock()
	if start {
		go l.run()
	}
}

// run sends the queue, one thing after the other, until it is empty.
func (l *liveState) run() {
	for {
		l.mu.Lock()
		if len(l.queue) == 0 {
			l.running = false
			l.mu.Unlock()
			return
		}
		c := l.queue[0]
		l.queue = l.queue[1:]
		l.mu.Unlock()
		err := l.send(c)
		l.mu.Lock()
		l.sent++
		if err != nil {
			l.err = fmt.Errorf("%s %s: %w", c.kind, padLabelOf(c), err)
		}
		l.mu.Unlock()
	}
}

func padLabelOf(c liveChange) string {
	if c.kind == "pattern" || c.kind == "unpattern" {
		return fmt.Sprintf("pattern %d", c.pad)
	}
	return fmt.Sprintf("pad %d", c.pad+1)
}

func (l *liveState) send(c liveChange) error {
	switch c.kind {
	case "sample":
		pad, _ := proto.NewPadIndex(uint16(c.pad))
		return l.dev.ImportSMPLive(uint8(l.project), pad, c.data, c.name)
	case "clear":
		pad, _ := proto.NewPadIndex(uint16(c.pad))
		return l.dev.PadOp(proto.PadOpDelete, pad)
	case "param":
		pad, _ := proto.NewPadIndex(uint16(c.pad))
		return l.dev.SetParam(c.id, proto.TargetPad(pad), c.value)
	case "name":
		pad, _ := proto.NewPadIndex(uint16(c.pad))
		return l.dev.SetSampleName(pad, c.name)
	case "pattern":
		slot, _ := proto.NewPadIndex(uint16(c.pad - 1))
		return l.dev.WriteFile(formats.PatternPath(uint8(l.project)+1, slot), c.data)
	case "unpattern":
		slot, _ := proto.NewPadIndex(uint16(c.pad - 1))
		return l.dev.Unlink(formats.PatternPath(uint8(l.project)+1, slot))
	}
	return nil
}

// liveState reports how many things are still on their way and the last error, for the panel.
func (a *App) liveStatus() (waiting int, sent int, err error) {
	l := a.live
	if l == nil {
		return 0, 0, nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	n := len(l.queue)
	if l.running {
		n++
	}
	return n, l.sent, l.err
}

// liveTick says what went wrong in the background.
func (a *App) liveTick() {
	l := a.live
	if l == nil {
		return
	}
	l.mu.Lock()
	err := l.err
	l.err = nil
	l.mu.Unlock()
	if err != nil {
		a.say("live: "+err.Error(), true)
	}
}

// coalesce adds the new changes to the queue: a change that is overtaken by a newer one while it waits (a setting held down goes through
// every value, a pattern is edited again) is not sent, only the last one is. A pad that is cleared takes its waiting changes with it.
func coalesce(queue, add []liveChange) []liveChange {
	for _, c := range add {
		keep := queue[:0:0]
		for _, q := range queue {
			switch {
			case c.kind == "param" && q.kind == "param" && q.pad == c.pad && q.id == c.id,
				c.kind == "name" && q.kind == "name" && q.pad == c.pad,
				(c.kind == "pattern" || c.kind == "unpattern") && (q.kind == "pattern" || q.kind == "unpattern") && q.pad == c.pad,
				c.kind == "sample" && q.kind == "sample" && q.pad == c.pad,
				c.kind == "clear" && q.pad == c.pad && q.kind != "pattern" && q.kind != "unpattern":
				continue
			}
			keep = append(keep, q)
		}
		queue = append(keep, c)
	}
	return queue
}
