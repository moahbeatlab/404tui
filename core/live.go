package core

import (
	"bytes"
	"fmt"
	"github.com/moahbeatlab/404tui/dbg"
	"hash/maphash"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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

	// what the unit is known to have (the editor as it was last sent), for noticing what is changed on the unit itself
	sig       liveBase
	watchBank atomic.Int32
	pulls     chan []pulled
	stop      chan struct{}
	// the unit is not looked at until this time: after a change of ours it takes a moment to show it
	quietUntil   time.Time
	reloaded     bool           // the unit finished a reload
	patternsSent bool           // a pattern was written to the unit's card and the unit was not told to reload (see App.reloadLive)
	seen         map[int]uint64 // pad -> the unit's block as last seen different from ours (it has to be seen twice to count)
}

// settleTime is how long the unit is left alone after something was sent to it.
const settleTime = 8 * time.Second

// pulled is a pad that was changed on the unit: its block and its sample.
type pulled struct {
	n     int
	block []byte
	smp   []byte
}

// goLive makes the project in the editor the live one: the unit's project n.
func (a *App) goLive(dev *device.Device, n int) {
	if err := dev.SelectProject(uint8(n)); err != nil {
		a.say(err.Error(), true)
		return
	}
	dbg.Logger.Info("live: start", "project", n+1)
	l := &liveState{dev: dev, project: n, seed: maphash.MakeSeed(), pulls: make(chan []pulled, 4), stop: make(chan struct{})}
	l.base = a.liveBaseNow(l)
	l.sig = l.base
	a.live = l
	go l.pollLoop()
	a.say(fmt.Sprintf("live: the editor is unit project %d; what you change goes to the unit", n+1), false)
}

func (a *App) stopLive() {
	if a.live != nil {
		close(a.live.stop)
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
	if dbg.Enabled() && len(ch) > 0 {
		kinds := map[string]int{}
		var pads []string
		for _, c := range ch {
			kinds[c.kind]++
			if c.kind != "param" && len(pads) < 12 {
				pads = append(pads, padLabelOf(c))
			}
		}
		dbg.Logger.Info("live: changes found", "n", len(ch), "kinds", kinds, "pads", pads, "queued before", len(l.queue))
	}
	l.mu.Lock()
	l.sig = now
	start := false
	if len(ch) > 0 { // what is on its way and what the unit is expected to have change together: the poll never sees one without the other
		l.queue = coalesce(l.queue, ch)
		start = !l.running
		l.running = true
		l.quietUntil = time.Now().Add(settleTime)
	}
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
		st := time.Now()
		err := l.send(c)
		if dbg.Enabled() {
			size := len(c.data)
			if err != nil {
				dbg.Logger.Warn("live: send failed", "kind", c.kind, "what", padLabelOf(c), "bytes", size, "ms", time.Since(st).Milliseconds(), "err", err)
			} else {
				dbg.Logger.Debug("live: sent", "kind", c.kind, "what", padLabelOf(c), "bytes", size, "ms", time.Since(st).Milliseconds())
			}
		}
		l.mu.Lock()
		if err == nil && (c.kind == "pattern" || c.kind == "unpattern") {
			l.patternsSent = true
		}
		l.quietUntil = time.Now().Add(settleTime) // the unit may need a moment to show what it was given
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
		return l.dev.ImportSMPLive(uint8(l.project)+1, pad, c.data, c.name) // the import takes the project number from 1
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
	tell := l.patternsSent && !l.running && len(l.queue) == 0
	if tell {
		l.patternsSent = false
	}
	l.mu.Unlock()
	if tell { // the pattern is in the unit's files: the unit shows it when it loads them
		a.say("the pattern is written to the unit: it shows there after a reload (PROJECTS, this project, Reload on the unit)", false)
	}
	l.watchBank.Store(int32(a.bank))
	select {
	case ps := <-l.pulls:
		a.applyPulls(ps)
	default:
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

// ---- what is changed on the unit itself (a pad sampled or edited there) comes into the editor

// pollLoop looks at the 16 pads of the bank in view every few seconds, while nothing is on its way to the unit.
func (l *liveState) pollLoop() {
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-t.C:
		}
		if ps := l.poll(); len(ps) > 0 {
			select {
			case l.pulls <- ps:
			case <-l.stop:
				return
			}
		}
	}
}

// poll reads the pads of the watched bank from the unit and returns those that differ from what the unit was last given: a sample
// that is there or gone, its name, its settings.
func (l *liveState) poll() []pulled {
	l.mu.Lock()
	sig, busy := l.sig, l.running || len(l.queue) > 0 || time.Now().Before(l.quietUntil)
	l.mu.Unlock()
	if busy {
		l.seen = nil
		return nil
	}
	pollStart := time.Now()
	if l.seen == nil {
		l.seen = map[int]uint64{}
	}
	bank := int(l.watchBank.Load())
	var out []pulled
	for i := 0; i < 16; i++ {
		n := bank*16 + i
		pad, ok := proto.NewPadIndex(uint16(n))
		if !ok {
			continue
		}
		blk, err := l.dev.PadBlock(pad)
		if err != nil {
			return out
		}
		if !padDiffers(blk, sig, n) {
			delete(l.seen, n)
			continue
		}
		// a difference counts when the unit shows the same thing twice: it is not a state on the way to what we sent
		if h := maphash.Bytes(l.seed, blk.Raw()); l.seen[n] != h {
			l.seen[n] = h
			continue
		}
		delete(l.seen, n)
		p := pulled{n: n, block: blk.Raw()}
		if blk.HasSample() {
			smp, err := l.dev.ReadFile(pad.SamplePath(uint8(l.project) + 1))
			if err != nil {
				continue
			}
			p.smp = smp
		}
		out = append(out, p)
	}
	if dbg.Enabled() {
		dbg.Logger.Debug("live: poll", "bank", bank, "pulled", len(out), "waiting for a second look", len(l.seen), "ms", time.Since(pollStart).Milliseconds())
	}
	return out
}

func padDiffers(blk *proto.PadBlock, sig liveBase, n int) bool {
	if blk.HasSample() != sig.hasFile[n] {
		return true
	}
	if !blk.HasSample() {
		return false
	}
	if blk.Name() != sig.names[n] {
		return true
	}
	for _, pi := range proto.Params {
		if pi.Scope != proto.ScopePad {
			continue
		}
		if want, ok := sig.params[n][pi.ID]; ok {
			if v, ok := blk.Param(pi.ID); ok && v != want {
				return true
			}
		}
	}
	return false
}

// applyPulls puts the pads that were changed on the unit into the editor, and tells the live state they are the unit's now, so that
// they are not sent back.
func (a *App) applyPulls(ps []pulled) {
	l := a.live
	if l == nil {
		return
	}
	var names []string
	for _, p := range ps {
		dbg.Logger.Info("live: pad pulled from the unit", "pad", a.label(p.n), "sample bytes", len(p.smp), "had sample", a.info.Pads[p.n].File)
		if err := a.lab.PullPad(p.n, p.block, p.smp); err != nil {
			a.say("live: "+err.Error(), true)
			continue
		}
		delete(a.pcm, p.n)
		names = append(names, a.label(p.n))
	}
	if len(names) == 0 {
		return
	}
	a.sl = nil
	a.refresh()
	l.base = a.liveBaseNow(l)
	l.mu.Lock()
	l.sig = l.base
	l.mu.Unlock()
	a.say("changed on the unit: "+strings.Join(names, ", "), false)
}

// liveHave says that the sample file of pad n, just fetched from the unit, is what the unit has: it is not a change to send.
func (a *App) liveHave(n int, data []byte) {
	l := a.live
	if l == nil || len(data) == 0 {
		return
	}
	h := maphash.Bytes(l.seed, data)
	l.base.fileHash[n], l.base.fileKey[n] = h, fileKey{&data[0], len(data)}
	l.mu.Lock()
	l.sig.fileHash[n], l.sig.fileKey[n] = h, l.base.fileKey[n]
	l.mu.Unlock()
}

// ---- the samples of a project that was loaded without them come when they are needed

type padFetch struct {
	n    int
	data []byte
	err  error
}

// padSampleSoft is padSample for drawing: a sample that is still on the unit is asked for in the background and not waited for.
func (a *App) padSampleSoft(n int) (int, bool) {
	if a.live != nil && a.lab.IsRemote(n) {
		if _, ok := a.pcm[n]; !ok {
			a.wantPad(n)
			return 0, false
		}
	}
	return a.padSample(n)
}

// wantPad puts a pad first among those that are fetched in the background.
func (a *App) wantPad(n int) {
	if a.live == nil || !a.lab.IsRemote(n) {
		return
	}
	for _, w := range a.fetchWant {
		if w == n {
			return
		}
	}
	a.fetchWant = append(a.fetchWant, n)
}

// fetchTick takes in the sample that arrived, and starts the next one: the pads that are wanted, else the pad under the cursor, else the
// others of the bank in view, one at a time (the unit is a slow cable).
func (a *App) fetchTick() {
	if a.fetchCh == nil {
		a.fetchCh = make(chan padFetch, 2)
	}
	select {
	case f := <-a.fetchCh:
		a.fetchBusy = -1
		if f.err == nil && a.live != nil && a.lab.IsRemote(f.n) {
			a.lab.PutLoaded(f.n, f.data)
			a.liveHave(f.n, f.data)
		}
	default:
	}
	l := a.live
	if l == nil || a.fetchBusy >= 0 || a.lab.RemoteCount() == 0 {
		return
	}
	next := -1
	for len(a.fetchWant) > 0 && next < 0 {
		if n := a.fetchWant[0]; a.lab.IsRemote(n) {
			next = n
		}
		a.fetchWant = a.fetchWant[1:]
	}
	if next < 0 && a.scr == Pads && a.lab.IsRemote(a.padN()) {
		next = a.padN()
	}
	for i := 0; next < 0 && i < 16; i++ {
		if n := a.bank*16 + i; a.lab.IsRemote(n) {
			next = n
		}
	}
	if next < 0 {
		return
	}
	pi, ok := proto.NewPadIndex(uint16(next))
	if !ok {
		return
	}
	a.fetchBusy = next
	dev, project, ch := l.dev, l.project, a.fetchCh
	go func() {
		st := time.Now()
		data, err := dev.ReadFile(pi.SamplePath(uint8(project) + 1))
		dbg.Logger.Debug("live: sample fetched in the background", "pad", next+1, "bytes", len(data), "ms", time.Since(st).Milliseconds(), "err", err)
		ch <- padFetch{next, data, err}
	}()
}

// reloadLive makes the unit read the project that is edited live from its card again, so that the patterns written to it show on the unit.
// It stops the sound there; settings the unit holds in memory only are replaced by what is on its card.
func (a *App) reloadLive() {
	l := a.live
	if l == nil {
		return
	}
	a.say("reloading the project on the unit...", false)
	dev, project := l.dev, l.project
	st := time.Now()
	go func() {
		err := dev.ReloadProject(uint8(project))
		dbg.Logger.Info("live: the unit reloaded the project", "project", project+1, "ms", time.Since(st).Milliseconds(), "err", err)
		l.mu.Lock()
		if err != nil {
			l.err = fmt.Errorf("reload: %w", err)
		}
		l.reloaded = err == nil
		l.mu.Unlock()
	}()
}
