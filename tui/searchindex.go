package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/proto/fileapi"
)

// startIndex makes the file index a search uses for a source. When the index saved on the
// card (the folders you indexed with i) has files under the source, the search index is made
// from it at once: nothing is walked, so it costs a read of one small file, not minutes
// of listing folders. With none saved the card is walked only when walk is true (a search
// asked for it), on another goroutine that keeps the unit free for other work between two
// folders; what it has read shows in the Details panel and in the search dialog.
func (m Model) startIndex(root string, walk bool) tea.Cmd {
	if m.unit == nil || m.indexes[root] != nil {
		return nil
	}
	// A build that is going on is left alone, and one that failed is not started again by
	// itself, which would loop: only a search asks again.
	if job := m.searchJobs[root]; job != nil && (job.active() || (job.failed.Load() && !walk) || (job.nothing.Load() && !walk)) {
		return nil
	}
	job := &tagJob{folder: root}
	m.searchJobs[root] = job
	unit, ops, known := m.unit, m.ops, m.tags
	return func() tea.Msg {
		defer job.done.Store(true)
		if known == nil {
			ops.Lock()
			loaded, err := app.LoadTagIndex(unit)
			ops.Unlock()
			if err != nil {
				job.failed.Store(true)
				return indexMsg{root: root, err: err}
			}
			known = loaded
			if known.NeedsUpgrade() {
				// An index from an older version is written again in the small format.
				ops.Lock()
				_ = known.Save(unit)
				ops.Unlock()
			}
		}
		if saved := app.IndexFromTags(known, root); len(saved.Files) > 0 {
			return indexMsg{root: root, ix: saved, tags: known}
		}
		if !walk {
			job.nothing.Store(true)
			return indexMsg{root: root, tags: known, none: true}
		}
		list := func(dir string) ([]fileapi.DirEntry, error) {
			ops.Lock()
			entries, err := unit.ListDir(dir)
			ops.Unlock()
			time.Sleep(4 * time.Millisecond)
			return entries, err
		}
		ix, _, err := app.BuildIndexWith(list, root, app.DefaultLimits, func(p app.IndexProgress) {
			job.folders.Store(int64(p.Folders))
			job.queued.Store(int64(p.Queued))
			job.files.Store(int64(p.Files))
			job.unreadable.Store(int64(p.Unreadable))
			job.current.Store(p.Current)
		}, nil)
		if err != nil {
			job.failed.Store(true)
			return indexMsg{root: root, err: err}
		}
		ix.AttachTags(known)
		return indexMsg{root: root, ix: ix, tags: known}
	}
}

// autoIndex starts building the card's search index in the background the first time the
// Files view is on show, so that the first search does not have to wait for it.
func (m Model) autoIndex(cmd tea.Cmd) (Model, tea.Cmd) {
	if m.section != secFiles || m.root != app.CardRoot || m.snap == nil {
		return m, cmd
	}
	if start := m.startIndex(m.root, false); start != nil {
		return m, tea.Batch(cmd, start)
	}
	return m, cmd
}

// indexLines is what the Details panel says about the index of the source on show: how far
// a walk has come, or where the index it has comes from. It is empty when there is nothing
// to say.
func (m Model) indexLines(width int) []string {
	st := m.st
	job := m.searchJobs[m.root]
	if job.active() {
		if job.folders.Load() == 0 && job.queued.Load() == 0 {
			return []string{st.accent.Render(spinner[m.frame%len(spinner)]) + " " + st.text.Render("Reading the saved index…"), ""}
		}
		return append([]string{st.accent.Render(spinner[m.frame%len(spinner)]) + " " + st.text.Render("Indexing "+rootName(m.root)+" for search")},
			append(indexBarLines(st, job, width), "")...)
	}
	if ix := m.indexes[m.root]; ix != nil {
		note := fmt.Sprintf("Search index  %s", plural(len(ix.Files), "file"))
		switch {
		case ix.Saved != "":
			note += "  ·  saved " + ix.Saved + "  ·  i indexes again"
		case ix.Truncated:
			note += "  (partial: it hit a limit)"
		}
		lines := []string{st.dim.Render(note)}
		if n := len(ix.Unreadable); n > 0 {
			lines = append(lines, st.warn.Render(plural(n, "folder")+" could not be read: r reads them again"))
		}
		return append(lines, "")
	}
	if job != nil && job.nothing.Load() {
		return []string{st.warn.Render("No search index yet"), st.dim.Render("i indexes the folder in front, once (saved on the card)"), ""}
	}
	return nil
}

// indexBarLines draws a bar and the counts of a running job.
func indexBarLines(st styles, job *tagJob, width int) []string {
	folders, queued := int(job.folders.Load()), int(job.queued.Load())
	cells := min(max(width-2, 10), 40)
	done := 0
	if total := folders + queued; total > 0 {
		done = cells * folders / total
	}
	bar := st.accent.Render(strings.Repeat("█", done)) + st.faint.Render(strings.Repeat("░", cells-done))
	counts := fmt.Sprintf("%d/%d folders · %s", folders, folders+queued, plural(int(job.files.Load()), "file"))
	if n := job.unreadable.Load(); n > 0 {
		counts += fmt.Sprintf(" · %d unreadable", n)
	}
	return []string{bar, st.dim.Render(counts)}
}
