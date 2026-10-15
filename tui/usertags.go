package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/proto"
)

// userTagsMsg is the end of a change of the tags the user gives files.
type userTagsMsg struct {
	tags    *app.TagIndex
	message string
	// reopen is the pad of the assign dialog the change was made from; it opens again.
	reopen *proto.PadIndex
	// applied is set when the change was made on the screen's side already.
	applied bool
	err     error
}

// tagSet is what tags are given to: audio files, and folders, whose tags every file in them
// has too.
type tagSet struct {
	files, folders []string
}

func (ts tagSet) empty() bool { return len(ts.files)+len(ts.folders) == 0 }

// count is how many things are tagged.
func (ts tagSet) count() int { return len(ts.files) + len(ts.folders) }

// taggable sorts paths (known from rows) into the audio files and the folders among them; any
// other file is left out.
func taggable(rows []app.Node, paths []string) tagSet {
	isDir := map[string]bool{}
	for _, r := range rows {
		isDir[r.Path] = r.Dir
	}
	var ts tagSet
	for _, p := range paths {
		switch {
		case isDir[p]:
			ts.folders = append(ts.folders, p)
		case isAudio(app.Base(p)):
			ts.files = append(ts.files, p)
		}
	}
	return ts
}

// filesToTag is what the Files view tags: the marked files and folders, or the one in front.
func (m Model) filesToTag() tagSet {
	if m.tree == nil {
		return tagSet{}
	}
	return taggable(m.tree.Rows(), m.fileTargets())
}

// dialogFilesToTag is what the assign dialog tags: the marked files and folders, or the one
// in front.
func (m Model) dialogFilesToTag(b *browser) tagSet {
	if b.tree == nil {
		return tagSet{}
	}
	if len(b.marks) > 0 {
		var paths []string
		for p := range b.marks {
			paths = append(paths, p)
		}
		return taggable(b.tree.Rows(), paths)
	}
	if b.at < len(b.tree.Rows()) {
		return taggable(b.tree.Rows(), []string{b.tree.Rows()[b.at].Path})
	}
	return tagSet{}
}

// tagFiles gives files tags, and takes some off, in the index kept on the card. The tags
// are read from the card first when this run has not read them. It reports how many files
// changed.
func (m Model) tagFiles(ts tagSet, add, remove []string, reopen *proto.PadIndex) tea.Cmd {
	unit, ops, known := m.unit, m.ops, m.tags
	return func() tea.Msg {
		ops.Lock()
		defer ops.Unlock()
		// The index on show is read by the screen while this runs, so the change is made on a
		// copy, which replaces it when the message arrives.
		var ti *app.TagIndex
		if known != nil {
			ti = known.Clone()
		} else {
			var err error
			if ti, err = app.LoadTagIndex(unit); err != nil {
				return userTagsMsg{err: err, reopen: reopen}
			}
		}
		changed := changeTags(ti, ts, add, remove)
		if changed == 0 {
			return userTagsMsg{tags: ti, message: "nothing to change: the tags are as asked already", reopen: reopen}
		}
		if err := ti.Save(unit); err != nil {
			return userTagsMsg{tags: ti, err: err, reopen: reopen}
		}
		what := strings.Join(append(prefixAll("+", add), prefixAll("-", remove)...), " ")
		return userTagsMsg{tags: ti, message: fmt.Sprintf("%s: %s", plural(changed, "file"), what), reopen: reopen}
	}
}

// changeTags adds and removes tags on an index, for the files and for the folders, and returns
// how many of them changed.
func changeTags(ti *app.TagIndex, ts tagSet, add, remove []string) int {
	changed := 0
	if len(add) > 0 {
		changed += ti.AddUserTags(ts.files, add) + ti.AddFolderTags(ts.folders, add)
	}
	if len(remove) > 0 {
		changed += ti.RemoveUserTags(ts.files, remove) + ti.RemoveFolderTags(ts.folders, remove)
	}
	return changed
}

func prefixAll(prefix string, words []string) []string {
	out := make([]string, len(words))
	for i, w := range words {
		out[i] = prefix + w
	}
	return out
}

// toggleFavourite gives files the favourite tag, or takes it off when they all have it.
func (m Model) toggleFavourite(ts tagSet, reopen *proto.PadIndex) (tea.Model, tea.Cmd) {
	if ts.empty() {
		m.message = "no audio file or folder to tag: mark some with v, or put the cursor on one"
		return m, nil
	}
	all := m.tags != nil
	for _, p := range append(slices.Clone(ts.files), ts.folders...) {
		all = all && m.tags.HasUserTag(p, app.FavouriteTag)
	}
	if all {
		return m.applyTags(ts, nil, []string{app.FavouriteTag}, reopen)
	}
	return m.applyTags(ts, []string{app.FavouriteTag}, nil, reopen)
}

// tagsSavedMsg ends the writing of the tag index to the card.
type tagsSavedMsg struct{ err error }

// applyTags changes the tags at once on a copy of the index that is on show, so that the
// screen and the next key already see it, and writes the index to the card on another
// goroutine. When the tags have not been read from the card yet, the change waits for them.
func (m Model) applyTags(ts tagSet, add, remove []string, reopen *proto.PadIndex) (tea.Model, tea.Cmd) {
	if m.tags == nil {
		return m, m.tagFiles(ts, add, remove, reopen)
	}
	next := m.tags.Clone()
	changed := changeTags(next, ts, add, remove)
	if changed == 0 {
		m.message = "nothing to change: the tags are as asked already"
		if reopen != nil {
			return m.openAssign(*reopen)
		}
		return m, nil
	}
	what := strings.Join(append(prefixAll("+", add), prefixAll("-", remove)...), " ")
	model, _ := m.userTagged(userTagsMsg{tags: next, message: fmt.Sprintf("%s: %s", plural(changed, "file"), what), reopen: reopen, applied: true})
	m = model.(Model)
	unit, ops := m.unit, m.ops
	// What is written is a copy of its own: saving changes the index it is called on.
	toSave := next.Clone()
	save := func() tea.Msg {
		ops.Lock()
		defer ops.Unlock()
		return tagsSavedMsg{err: toSave.Save(unit)}
	}
	if reopen != nil {
		model, cmd := m.openAssign(*reopen)
		return model, tea.Batch(cmd, save)
	}
	return m, save
}

// tagsWritten handles the end of the writing of the index.
func (m Model) tagsWritten(msg tagsSavedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.message = "the tags are not saved on the card: " + msg.err.Error()
	}
	return m, nil
}

// askTags asks which tags to add to files: words add, and a word with - in front takes it off.
// The question lists the tags the files have, so the tags are read from the card first when
// this run has not read them.
func (m Model) askTags(ts tagSet, reopen *proto.PadIndex) (tea.Model, tea.Cmd) {
	if ts.empty() {
		m.message = "no audio file or folder to tag: mark some with v, or put the cursor on one"
		return m, nil
	}
	if m.tags == nil && m.unit != nil {
		unit, ops := m.unit, m.ops
		return m, func() tea.Msg {
			ops.Lock()
			defer ops.Unlock()
			tags, err := app.LoadTagIndex(unit)
			return askTagsMsg{tags: tags, err: err, ts: ts, reopen: reopen}
		}
	}
	m.prompt = &prompt{kind: promptTags, paths: ts.files, folders: ts.folders, reopen: reopen}
	return m, nil
}

// askTagsMsg is the tags read from the card for the question about tags.
type askTagsMsg struct {
	tags   *app.TagIndex
	err    error
	ts     tagSet
	reopen *proto.PadIndex
}

func (m Model) tagsRead(msg askTagsMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.message = "reading the tags failed: " + msg.err.Error()
		if msg.reopen != nil {
			return m.openAssign(*msg.reopen)
		}
		return m, nil
	}
	m.tags = msg.tags
	m.prompt = &prompt{kind: promptTags, paths: msg.ts.files, folders: msg.ts.folders, reopen: msg.reopen}
	return m, nil
}

// tagsPromptBox is the question about tags: what the files have already (the tags you gave,
// and for one file the ones its name and folders give), then the box to type in, and the
// tags used before to pick words from.
func (m Model) tagsPromptBox(p *prompt) string {
	st := m.st
	width := min(m.width-4, 96)
	all := append(slices.Clone(p.paths), p.folders...)
	title := "Tag " + plural(len(p.paths), "file")
	if len(p.folders) > 0 {
		title = "Tag " + plural(len(all), "item")
	}
	if len(all) == 1 {
		title = "Tag " + app.Base(all[0])
		if len(p.folders) == 1 {
			title += "/ and every file in it"
		}
	}
	words := func(items []string) string {
		if len(items) == 0 {
			return st.faint.Render("none")
		}
		return ansi.Truncate(strings.Join(items, "  "), width-18, "…")
	}
	// The tags given by the user, with how many of the files have each when there are several.
	mine := map[string]int{}
	for _, path := range all {
		for _, tag := range m.tags.UserTagsOf(path) {
			mine[tag]++
		}
	}
	var have []string
	for _, c := range app.SortedCounts(mine) {
		item := c.Tag
		if len(all) > 1 {
			item = fmt.Sprintf("%s (%d of %d)", c.Tag, c.Count, len(all))
		}
		have = append(have, item)
	}
	lines := []string{st.bold.Render(title), "", st.dim.Render(padTo("Your tags", 12)) + st.good.Render(words(have))}
	if len(p.paths) == 1 && len(p.folders) == 0 {
		lines = append(lines, st.dim.Render(padTo("From name", 12))+st.text.Render(words(m.tags.OwnTags(p.paths[0]))))
	}
	lines = append(lines, "", st.accent.Render("> ")+p.shown(), "")
	var used []string
	for i, c := range m.tags.UserTagCounts() {
		if i >= 14 {
			break
		}
		used = append(used, fmt.Sprintf("%s (%d)", c.Tag, c.Count))
	}
	if len(used) > 0 {
		lines = append(lines, st.dim.Render(padTo("Used before", 12))+st.text.Render(words(used)))
	}
	lines = append(lines, st.dim.Render("words add tags, -word takes one off  ·  Enter apply  ·  Esc cancel"))
	return st.modal.Width(width).Render(strings.Join(lines, "\n"))
}

// tagsAnswered carries out what was typed for promptTags.
func (m Model) tagsAnswered(p *prompt, text string) (tea.Model, tea.Cmd) {
	var add, remove []string
	for _, w := range strings.Fields(strings.NewReplacer(",", " ").Replace(text)) {
		if strings.HasPrefix(w, "-") {
			if t := app.CleanTag(w[1:]); t != "" {
				remove = append(remove, t)
			}
		} else if t := app.CleanTag(w); t != "" {
			add = append(add, t)
		}
	}
	if len(add)+len(remove) == 0 {
		m.message = "no tag typed"
		if p.reopen != nil {
			return m.openAssign(*p.reopen)
		}
		return m, nil
	}
	m.fileMarks = map[string]bool{}
	return m.applyTags(tagSet{p.paths, p.folders}, add, remove, p.reopen)
}

// userTagged handles the end of a change: the new index is kept, and the search indexes
// follow it.
func (m Model) userTagged(msg userTagsMsg) (tea.Model, tea.Cmd) {
	if msg.tags != nil {
		m.tags = msg.tags
		for root, ix := range m.indexes {
			if ix.Saved != "" {
				m.indexes[root] = app.IndexFromTags(m.tags, root)
			} else {
				ix.AttachTags(m.tags)
			}
		}
	}
	switch {
	case msg.err != nil:
		m.message = "tagging failed: " + msg.err.Error()
	default:
		m.message = "tagged " + msg.message
	}
	if msg.reopen != nil && !msg.applied {
		return m.openAssign(*msg.reopen)
	}
	return m, nil
}

// favouriteMark is the star shown in front of a favourite file.
func (m Model) favouriteMark(path string) string {
	if m.tags.HasUserTag(path, app.FavouriteTag) {
		return m.st.warn.Render("★")
	}
	return ""
}
