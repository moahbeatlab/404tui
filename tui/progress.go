package tui

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

// progress is how far a long operation on the unit has come. The operation runs on another
// goroutine and the model is copied on every update, so the counters are shared through a
// pointer and read when a frame is drawn.
type progress struct {
	done, total atomic.Int64
	name        atomic.Value // string
	// sweep asks for a bar that moves without a position, for work that reports nothing.
	sweep atomic.Bool

	mu    sync.Mutex
	lines []string
}

// logLine adds a line to what the dialog lists under the bar, one per finished step.
func (p *progress) logLine(line string) {
	p.mu.Lock()
	p.lines = append(p.lines, line)
	p.mu.Unlock()
}

// recent is the last n lines logged, oldest first.
func (p *progress) recent(n int) []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.lines[max(len(p.lines)-n, 0):]...)
}

// begin starts counting toward total steps; with a total of 0 only the spinner shows.
func (p *progress) begin(total int) {
	p.done.Store(0)
	p.total.Store(int64(total))
	p.name.Store("")
	p.sweep.Store(false)
	p.mu.Lock()
	p.lines = nil
	p.mu.Unlock()
}

// beginSweep is for an operation that cannot say how far it has come.
func (p *progress) beginSweep() {
	p.begin(0)
	p.sweep.Store(true)
}

// sweepBar is a bar with a block running back and forth in it, frame by frame.
func (p *progress) sweepBar(width, frame int) string {
	if !p.sweep.Load() {
		return ""
	}
	const block = 6
	cells := max(width-2, block+2)
	span := cells - block
	at := frame / 2 % (2 * span)
	if at > span {
		at = 2*span - at
	}
	return "[" + strings.Repeat("░", at) + strings.Repeat("█", block) + strings.Repeat("░", span-at) + "]"
}

// step records that done steps are finished, and the name of the one in hand.
func (p *progress) step(done int, name string) {
	p.done.Store(int64(done))
	p.name.Store(name)
}

// stepTotal is step for callbacks that report the total as well.
func (p *progress) stepTotal(done, total int) {
	p.done.Store(int64(done))
	p.total.Store(int64(total))
}

// bar draws "[████░░░░] 12/40 name" in width cells, or "" when no total is known.
func (p *progress) bar(width int) string {
	total, done := int(p.total.Load()), int(p.done.Load())
	if total <= 0 {
		return ""
	}
	done = min(max(done, 0), total)
	count := fmt.Sprintf(" %d/%d", done, total)
	cells := max(width-len(count)-2, 4)
	filled := cells * done / total
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", cells-filled) + "]" + count
}

// file is the name of the step in hand, shortened to width.
func (p *progress) file(width int) string {
	name, _ := p.name.Load().(string)
	if r := []rune(name); len(r) > width && width > 1 {
		return "…" + string(r[len(r)-width+1:])
	}
	return name
}
