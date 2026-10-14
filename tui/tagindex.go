package tui

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/proto/fileapi"
)

// tagJob is an indexing run in the background. The run is on another goroutine and the
// model is copied on every update, so what the bottom bar shows is read from counters.
type tagJob struct {
	folder                             string
	folders, queued, files, unreadable atomic.Int64
	current                            atomic.Value // string
	saving, stopping, done, failed     atomic.Bool
	// nothing is set when the card has no saved index to make a search index from.
	nothing atomic.Bool
}

// tagIndexMsg ends a run.
type tagIndexMsg struct {
	index     *app.TagIndex
	folder    string
	files     int
	completed bool
	// unreadable is how many folders the unit would not list.
	unreadable int
	err        error
}

// active reports whether a run is going on.
func (j *tagJob) active() bool { return j != nil && !j.done.Load() }

// startTagIndex indexes a folder in the background, or stops the run that is going on. It
// keeps the unit free for other work between two folders it reads, and writes the index to
// the root of the SD card when it is done.
func (m Model) startTagIndex(folder string) (tea.Model, tea.Cmd) {
	if m.tagJob.active() {
		m.tagJob.stopping.Store(true)
		m.message = "stopping the indexing of " + display(m.tagJob.folder) + "…"
		return m, nil
	}
	if m.unit == nil {
		return m, nil
	}
	job := &tagJob{folder: folder}
	m.tagJob = job
	m.message = "indexing " + display(folder) + " in the background (i stops it)"
	unit, ops := m.unit, m.ops
	return m, func() tea.Msg {
		defer job.done.Store(true)
		ops.Lock()
		index, err := app.LoadTagIndex(unit)
		ops.Unlock()
		if err != nil {
			return tagIndexMsg{folder: folder, err: err}
		}
		list := func(dir string) ([]fileapi.DirEntry, error) {
			ops.Lock()
			entries, err := unit.ListDir(dir)
			ops.Unlock()
			// Whatever else waits for the unit gets its turn between two folders.
			time.Sleep(4 * time.Millisecond)
			return entries, err
		}
		files, completed, err := app.IndexTags(list, folder, index, app.DefaultLimits, func(p app.TagProgress) {
			job.folders.Store(int64(p.Folders))
			job.queued.Store(int64(p.Queued))
			job.files.Store(int64(p.Files))
			job.unreadable.Store(int64(p.Unreadable))
			job.current.Store(p.Current)
		}, job.stopping.Load)
		if err != nil {
			return tagIndexMsg{folder: folder, err: err}
		}
		job.saving.Store(true)
		ops.Lock()
		// Tags given while the card was walked are on the card by now: they are kept.
		if latest, lerr := app.LoadTagIndex(unit); lerr == nil {
			index.UseUserTagsOf(latest)
		}
		err = index.Save(unit)
		ops.Unlock()
		return tagIndexMsg{index: index, folder: folder, files: files, completed: completed, unreadable: int(job.unreadable.Load()), err: err}
	}
}

// tagIndexed handles the end of a run.
func (m Model) tagIndexed(msg tagIndexMsg) (tea.Model, tea.Cmd) {
	if msg.index != nil {
		// Tags given on the screen since are kept too.
		msg.index.UseUserTagsOf(m.tags)
		m.tags = msg.index
		// The search index of the card is made from what was just saved.
		m.indexes[app.CardRoot] = app.IndexFromTags(m.tags, app.CardRoot)
		delete(m.searchJobs, app.CardRoot)
	}
	switch {
	case msg.err != nil:
		m.message = "indexing " + display(msg.folder) + " failed: " + msg.err.Error()
	case !msg.completed:
		m.message = fmt.Sprintf("indexing stopped: %s from %s kept in %s", plural(msg.files, "file"), display(msg.folder), app.TagIndexPath)
	default:
		m.message = fmt.Sprintf("indexed %s in %s: saved to %s", plural(msg.files, "file"), display(msg.folder), app.TagIndexPath)
	}
	if msg.unreadable > 0 && msg.err == nil {
		m.message += fmt.Sprintf("  ·  %s could not be read, so their files are missing: press i again", plural(msg.unreadable, "folder"))
	}
	return m, nil
}

// indexBar is the line at the bottom while a run goes on: the folder, how far the walk has
// come (folders read of those known so far) and how many files are tagged.
func (m Model) indexBar() string {
	j, st := m.tagJob, m.st
	folders, queued := int(j.folders.Load()), int(j.queued.Load())
	label := st.accent.Render(spinner[m.frame%len(spinner)]) + " " + st.text.Render("indexing "+display(j.folder))
	tail := st.dim.Render(fmt.Sprintf("  %d/%d folders · %s  ·  i stops", folders, folders+queued, plural(int(j.files.Load()), "file")) + unreadableNote(j))
	if j.saving.Load() {
		tail = st.dim.Render("  saving " + app.TagIndexName + " to the SD card…")
	}
	room := m.width - ansi.StringWidth(label) - ansi.StringWidth(tail) - 4
	bar := ""
	if room >= 8 {
		cells := min(room, 40)
		done := cells
		if total := folders + queued; total > 0 && !j.saving.Load() {
			done = cells * folders / total
		}
		bar = "  " + st.accent.Render(strings.Repeat("█", done)) + st.faint.Render(strings.Repeat("░", cells-done))
	}
	return ansi.Truncate(" "+label+bar+tail, m.width, "…")
}

// unreadableNote is a warning for the bottom bar when folders could not be listed.
func unreadableNote(j *tagJob) string {
	if n := j.unreadable.Load(); n > 0 {
		return " · " + plural(int(n), "folder") + " unreadable"
	}
	return ""
}
