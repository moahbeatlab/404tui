package tui

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// routedUnit reads the files on this computer from it and everything else from the unit, so a
// scan can look at the files of any indexed source.
type routedUnit struct{ app.Unit }

func (r routedUnit) ReadPrefix(path string, n uint32) ([]byte, bool, error) {
	if app.IsLocal(path) {
		return app.Local{}.ReadPrefix(path, n)
	}
	return r.Unit.ReadPrefix(path, n)
}

// folderUsed reports whether a folder, or one inside it, was put on a bank (restored), or has
// a sample on a pad that was assigned from it: it gets the ◆ in the trees.
func (m Model) folderUsed(path string) bool {
	if m.folderUses.Used(path) {
		return true
	}
	if m.origins == nil {
		return false
	}
	under := path + "/"
	for _, o := range m.origins.Pads {
		if strings.HasPrefix(o.Path, under) {
			return true
		}
	}
	return false
}

// usedMark is the icon in the gutter of the trees for a folder that was used: a diamond in the
// colour of conditions, which no other mark has.
func (m Model) usedMark() string {
	return lipgloss.NewStyle().Foreground(m.st.th.cond).Bold(true).Render("◆")
}

// folderUsage says, for a folder, whether it was used and where: onto which bank it was
// restored, which restored folders are inside it, and the pads of each project that hold a
// sample from it, bank by bank. It is empty for a folder nothing came from.
func (m Model) folderUsage(folder string, width int) []string {
	st := m.st
	own, inside := m.folderUses.Of(folder)
	byProject := m.padsFromFolder(folder)
	if len(own)+len(inside)+len(byProject) == 0 {
		return nil
	}
	lines := []string{m.usedMark() + " " + st.bold.Render("Used already")}
	row := func(indent int, text string) {
		lines = append(lines, strings.Repeat(" ", indent)+ansi.Truncate(text, width-indent, "…"))
	}
	for _, u := range own {
		how := "restored"
		if u.Matched {
			how = "found"
		}
		row(2, st.text.Render(fmt.Sprintf("%s onto project %d, bank %c", how, u.Project, 'A'+u.Bank))+st.dim.Render(fmt.Sprintf("  ·  %s  ·  %s", plural(u.Pads, "pad"), u.At)))
	}
	if len(inside) > 0 {
		banks := map[int][]string{}
		var order []int
		for _, u := range inside {
			if _, seen := banks[u.Project]; !seen {
				order = append(order, int(u.Project))
			}
			banks[int(u.Project)] = append(banks[int(u.Project)], fmt.Sprintf("%c", 'A'+u.Bank))
		}
		slices.Sort(order)
		var parts []string
		for _, p := range order {
			parts = append(parts, fmt.Sprintf("project %d bank %s", p, strings.Join(banks[p], " ")))
		}
		row(2, st.dim.Render("folders inside it were restored onto ")+st.text.Render(strings.Join(parts, ", ")))
	}
	for _, pr := range byProject {
		row(2, st.text.Render(fmt.Sprintf("project %d", pr.project))+st.dim.Render(fmt.Sprintf("  ·  %s with samples from it", plural(pr.n, "pad"))))
		for _, b := range pr.banks {
			shown := b.pads
			more := ""
			// A bank has 16 pads: the list fits on a line of any reasonable width.
			label := st.accent.Render(fmt.Sprintf("bank %s", b.bank)) + "  "
			if len(shown) > 12 {
				more = fmt.Sprintf(" and %d more", len(shown)-12)
				shown = shown[:12]
			}
			row(4, label+st.text.Render(strings.Join(shown, " "))+st.dim.Render(more))
		}
	}
	return append(lines, "")
}

// folderBank is the pads of one bank that hold samples from a folder, in pad order.
type folderBank struct {
	bank string
	pads []string
}

// folderProject is what a folder gave to one project.
type folderProject struct {
	project int
	n       int
	banks   []folderBank
}

// padsFromFolder reads the notes of where samples came from: the pads of each project, by bank,
// that were assigned from a file under the folder.
func (m Model) padsFromFolder(folder string) []folderProject {
	if m.origins == nil {
		return nil
	}
	type key struct {
		project int
		bank    string
	}
	pads := map[key][]string{}
	for k, o := range m.origins.Pads {
		if !strings.HasPrefix(o.Path, folder+"/") {
			continue
		}
		proj, pad, ok := strings.Cut(k, "/")
		n, err := strconv.Atoi(proj)
		if !ok || err != nil || len(pad) < 2 {
			continue
		}
		kk := key{n, pad[:1]}
		pads[kk] = append(pads[kk], pad)
	}
	padNumber := func(p string) int { n, _ := strconv.Atoi(p[1:]); return n }
	var keys []key
	for k, v := range pads {
		keys = append(keys, k)
		slices.SortFunc(v, func(a, b string) int { return cmp.Compare(padNumber(a), padNumber(b)) })
	}
	slices.SortFunc(keys, func(a, b key) int {
		return cmp.Or(cmp.Compare(a.project, b.project), cmp.Compare(a.bank, b.bank))
	})
	var out []folderProject
	for _, k := range keys {
		if len(out) == 0 || out[len(out)-1].project != k.project {
			out = append(out, folderProject{project: k.project})
		}
		cur := &out[len(out)-1]
		cur.banks = append(cur.banks, folderBank{bank: k.bank, pads: pads[k]})
		cur.n += len(pads[k])
	}
	return out
}
