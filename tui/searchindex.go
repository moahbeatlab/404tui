package tui

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

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

// scanOrigins looks for the files the samples of this project came from, for the pads that
// have no origin noted, by matching their names and lengths against the search index of the
// card; what is certain is noted and saved. Pads that fit several files, or none, are told.
func (m Model) scanOrigins() (tea.Model, tea.Cmd) {
	return m.askScanFolder(false)
}

// scanFolderUses finds the origins of the pads like scanOrigins, then notes, for each bank
// of the project, the folder most of its samples came from.
func (m Model) scanFolderUses() (tea.Model, tea.Cmd) {
	return m.askScanFolder(true)
}

// askScanFolder opens the folder dialog where the scan is to look, on the folder it looked in
// last time.
func (m Model) askScanFolder(banks bool) (tea.Model, tea.Cmd) {
	root, dir := app.CardRoot, m.mem.ScanDir
	if strings.HasPrefix(dir, "/") {
		root = localRoot()
	}
	var expanded []string
	if dir != "" {
		expanded = app.Ancestors(root, dir)
	}
	next, cmd := m.openBrowser(browseScan, root, expanded, dir)
	next.(Model).browser.scanBanks = banks
	return next, cmd
}

// scanOriginsFor looks for where the pads' samples came from among the indexed files under
// folder (all of them when it is empty).
func (m Model) scanOriginsFor(banks bool, folder string) (tea.Model, tea.Cmd) {
	rows, project, unit := m.snap.Pads, m.snap.Project, m.unit
	withSample := 0
	for _, r := range rows {
		if r.HasSample {
			withSample++
		}
	}
	prog := m.prog
	prog.begin(withSample)
	m.busy = "finding where the samples came from"
	return m, m.edit(func() tea.Msg {
		defer prog.begin(0)
		// The saved index is read: it holds the files of every source that was indexed, the card
		// and this computer alike.
		saved, err := app.LoadTagIndex(unit)
		if err != nil {
			return doneMsg{message: "reading the saved index failed: " + err.Error()}
		}
		ix := app.IndexOfAll(saved)
		if folder != "" {
			under := folder + "/"
			if folder == app.CardRoot || folder == app.StorageRoot {
				under = folder
			}
			var files []app.IndexedFile
			for _, f := range ix.Files {
				if strings.HasPrefix(f.Path, under) {
					files = append(files, f)
				}
			}
			ix.Files = files
		}
		if len(ix.Files) == 0 {
			where := ""
			if folder != "" {
				where = " under " + display(folder)
			}
			return doneMsg{message: "no indexed files" + where + ": open the Files view and press i on the folders your samples are in"}
		}
		// Files on this computer are read from it, the pads from the unit.
		reader := routedUnit{unit}
		have, err := app.LoadOrigins(unit)
		if err != nil {
			return doneMsg{message: "reading the origins failed: " + err.Error()}
		}
		lengths := map[string]float64{}
		seconds := func(path string) (float64, bool) {
			if s, ok := lengths[path]; ok {
				return s, s > 0
			}
			head, exists, err := reader.ReadPrefix(path, 512)
			s := 0.0
			if err == nil && exists {
				s, _ = app.WAVSeconds(head)
			}
			lengths[path] = s
			return s, s > 0
		}
		padSeconds := func(row app.PadRow) (float64, bool) {
			facts, err := app.ReadPadFacts(unit, row)
			return facts.Seconds, err == nil && facts.Seconds > 0
		}
		scan := app.ScanOrigins(rows, project, have, ix.Files, seconds, padSeconds, func(row app.PadRow, cands []string) []string {
			prog.step(int(row.Pad.Index()), "comparing the audio of "+row.Pad.String()+" with "+plural(len(cands), "file"))
			return app.MatchByContent(reader, row, cands)
		}, func(done, total int, line string) {
			prog.step(done, line)
			prog.logLine(line)
		})
		message := fmt.Sprintf("origins: %s found", plural(len(scan.Found), "pad"))
		if scan.Known > 0 {
			message += fmt.Sprintf(", %d already known", scan.Known)
		}
		if n := len(scan.Ambiguous); n > 0 {
			var which []string
			for _, a := range scan.Ambiguous[:min(n, 4)] {
				which = append(which, fmt.Sprintf("%s (%d files)", a.Pad, len(a.Candidates)))
			}
			message += fmt.Sprintf("; %d fit several files and are left: %s", n, strings.Join(which, ", "))
			if n > 4 {
				message += ", …"
			}
		}
		if n := len(scan.Unknown); n > 0 {
			message += fmt.Sprintf("; %d are not in the index", n)
		}
		origins := have
		if len(scan.Found) > 0 {
			var err error
			if origins, err = app.RecordOrigins(unit, project, scan.Found); err != nil {
				return doneMsg{message: message + "; saving them failed: " + err.Error()}
			}
		}
		if !banks {
			if len(scan.Found) == 0 {
				return doneMsg{message: message}
			}
			return doneMsg{message: message, origins: origins}
		}
		found := app.FolderUsesFromOrigins(origins, project, rows)
		if len(found) == 0 {
			return doneMsg{message: message + "; no bank has most of its samples from one folder", origins: origins}
		}
		uses, err := app.RecordFolderUses(unit, project, found)
		if err != nil {
			return doneMsg{message: message + "; saving the folders failed: " + err.Error(), origins: origins}
		}
		var banksSeen []string
		for _, f := range found {
			banksSeen = append(banksSeen, fmt.Sprintf("%c", 'A'+f.Bank))
		}
		return doneMsg{message: fmt.Sprintf("%s; folders found for bank %s", message, strings.Join(banksSeen, ", ")), origins: origins, folderUses: uses}
	})
}

// folderUseLines say where a folder was put: which project and bank, and the folders inside it.
func (m Model) folderUseLines(folder string, width int) []string {
	st := m.st
	own, inside := m.folderUses.Of(folder)
	if len(own)+len(inside) == 0 {
		return nil
	}
	var lines []string
	for _, u := range own {
		how := "restored"
		if u.Matched {
			how = "found"
		}
		lines = append(lines, st.accent.Render("◆ ")+st.text.Render(fmt.Sprintf("Used in project %d, bank %c", u.Project, 'A'+u.Bank))+
			st.dim.Render(fmt.Sprintf("  ·  %s, %s, %s", plural(u.Pads, "pad"), how, u.At)))
	}
	if n := len(inside); n > 0 {
		var where []string
		for _, u := range inside[:min(n, 6)] {
			where = append(where, fmt.Sprintf("%d·%c", u.Project, 'A'+u.Bank))
		}
		text := "Folders inside it are on " + strings.Join(where, " ")
		if n > 6 {
			text += fmt.Sprintf(" and %d more", n-6)
		}
		lines = append(lines, st.dim.Render("◆ "+ansi.Truncate(text, width-2, "…")))
	}
	return append(lines, "")
}

// folderPadLines say which pads, in which projects, hold a sample that came from a folder: the
// notes of where the samples were assigned from, under the folder.
func (m Model) folderPadLines(folder string, width int) []string {
	if m.origins == nil {
		return nil
	}
	byProject := map[string][]string{}
	var order []string
	for key, o := range m.origins.Pads {
		if !strings.HasPrefix(o.Path, folder+"/") {
			continue
		}
		project, pad, ok := strings.Cut(key, "/")
		if !ok {
			continue
		}
		if _, seen := byProject[project]; !seen {
			order = append(order, project)
		}
		byProject[project] = append(byProject[project], pad)
	}
	if len(order) == 0 {
		return nil
	}
	slices.SortFunc(order, func(a, b string) int { return cmp.Compare(a, b) })
	st := m.st
	var lines []string
	for _, project := range order {
		pads := byProject[project]
		slices.Sort(pads)
		text := fmt.Sprintf("Project %s: %s", project, plural(len(pads), "pad")) + "  " + strings.Join(pads[:min(len(pads), 12)], " ")
		if len(pads) > 12 {
			text += fmt.Sprintf(" and %d more", len(pads)-12)
		}
		lines = append(lines, st.accent.Render("◆ ")+st.text.Render(ansi.Truncate(text, width-2, "…")))
	}
	return append(lines, "")
}

// routedUnit reads the files on this computer from it and everything else from the unit, so a
// scan can look at the files of any indexed source.
type routedUnit struct{ app.Unit }

func (r routedUnit) ReadPrefix(path string, n uint32) ([]byte, bool, error) {
	if app.IsLocal(path) {
		return app.Local{}.ReadPrefix(path, n)
	}
	return r.Unit.ReadPrefix(path, n)
}
