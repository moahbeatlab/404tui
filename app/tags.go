package app

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto/fileapi"
)

// TagIndexName is the file the index is kept in, at the root of the SD card: the files of the
// folders that were indexed, as compressed JSON, small enough to read over the serial link
// in a moment. Tags are not stored: they are worked out when it is read.
const TagIndexName = "404tui-index.json.gz"

// legacyTagIndexName is the plain JSON file earlier versions wrote; it is still read when
// there is no newer file, and removed once one is written.
const legacyTagIndexName = "404tui-index.json"

// TagIndexPath is where the index lives.
var TagIndexPath = Join(CardRoot, TagIndexName)

var legacyTagIndexPath = Join(CardRoot, legacyTagIndexName)

// tagIndexVersion changes when the file's layout does.
const tagIndexVersion = 1

// FavouriteTag is the tag the user gives the samples they like best.
const FavouriteTag = "favourite"

// TagIndex says what each indexed audio file is, from its name and the folders it is in:
// "SD:Synth/House/124_bass_wobble.wav" is synth, house, 124, bass and wobble. Only the
// folders the user chose are in it.
type TagIndex struct {
	Version int `json:"version"`
	// Folders are the folders that were indexed.
	Folders []string `json:"folders"`
	// Files maps a file's path to its tags, separated by spaces.
	Files map[string]string `json:"files"`
	// Updated is when the index was last written ("2026-10-14 17:02"); empty when it never was.
	Updated string `json:"-"`
	// legacy is set when the index was read from the older plain file.
	legacy bool
	// User are the tags the user gave files, by path: "favourite", "bass-wobbly", whatever was
	// typed. They are kept apart from the tags worked out from names, so that indexing again
	// never loses them.
	User map[string][]string `json:"user,omitempty"`
	// UserFolders are the tags the user gave folders. Every file in the folder, now or later,
	// has them too.
	UserFolders map[string][]string `json:"-"`
}

// NeedsUpgrade reports whether the index was read from the older plain file: saving it
// writes the small compressed file and removes the old one.
func (t *TagIndex) NeedsUpgrade() bool { return t != nil && t.legacy && len(t.Files) > 0 }

// indexFile is how the index is written: folder by folder, so a path is stored once.
type indexFile struct {
	Version int                 `json:"version"`
	Updated string              `json:"updated"`
	Folders []string            `json:"folders"`
	Dirs    map[string][]string `json:"dirs"`
	User    map[string][]string `json:"user,omitempty"`
	// UserFolders is what the user gave folders.
	UserFolders map[string][]string `json:"userFolders,omitempty"`
}

// NewTagIndex is an index with nothing in it.
func NewTagIndex() *TagIndex {
	return &TagIndex{Version: tagIndexVersion, Files: map[string]string{}}
}

var (
	// stopTags say nothing about the sound: file types, the folders everything lives in, and
	// the names sample packs put on every file ("h2" is a label, "vol2" a release).
	stopTags = map[string]bool{
		"wav": true, "wave": true, "aif": true, "aiff": true, "flac": true, "mp3": true, "ogg": true,
		"the": true, "and": true, "sd": true, "sample": true, "samples": true,
		"import": true, "export": true, "collections": true, "preview": true, "previews": true,
		"pack": true, "packs": true, "kit": true, "kits": true, "sp": true, "mini": true,
		"h2": true, "lm": true, "hdk": true, "edk": true,
	}
	stopPattern = regexp.MustCompile(`^(vol\d*|volume\d*|edk\d+|hdk\d+|kit\d+|ds\d+)$`)
	// sharpKeys are the note names a chord or bass one-shot starts with: Fs is F sharp.
	sharpKey = regexp.MustCompile(`^([a-g])s$`)
	// tagFold replaces short and plural forms by the one word they stand for.
	tagFold = map[string]string{
		"bd": "kick", "kck": "kick", "kicks": "kick", "sn": "snare", "snr": "snare", "snares": "snare",
		"hh": "hat", "hihat": "hat", "hihats": "hat", "hats": "hat", "clp": "clap", "claps": "clap",
		"bs": "bass", "basses": "bass", "syn": "synth", "synths": "synth", "vox": "vocal", "vocals": "vocal",
		"loops": "loop", "percs": "perc", "toms": "tom",
		"shot": "oneshot", "shots": "oneshot", "oneshots": "oneshot", "hit": "oneshot", "hits": "oneshot",
	}
	// tagAliases adds a tag next to the word: an open hat is also a hat.
	tagAliases = map[string]string{"chat": "hat", "ohat": "hat"}
	// tagSuffixes are words a compound ends in: "drumloop" is a loop.
	tagSuffixes = []string{"loop", "kick", "snare", "hat", "clap", "bass", "synth", "vocal", "perc"}
	keyTag      = regexp.MustCompile(`^[a-g][#b]?(m|min|maj|minor|major)$`)
	trailing    = regexp.MustCompile(`^([a-z]{3,})\d+$`)
)

// TagsFor is the tags of a file: the words of its name and of every folder from the folder
// that was chosen down. base is the folder above the chosen one, so the chosen folder's own
// name counts. Numbers that only count things are dropped; two or three digits from 40 up
// are kept, as they are usually a tempo (124) or a machine (808).
func TagsFor(path, base string) []string {
	rel := strings.TrimPrefix(strings.TrimPrefix(path, base), "/")
	rel = strings.TrimPrefix(rel, CardRoot)
	if at := strings.LastIndexByte(rel, '.'); at > strings.LastIndexByte(rel, '/') {
		rel = rel[:at]
	}
	seen := map[string]bool{}
	var tags []string
	add := func(t string) {
		if t != "" && !seen[t] && !stopTags[t] {
			seen[t] = true
			tags = append(tags, t)
		}
	}
	if strings.EqualFold(path[strings.LastIndexByte(path, '.')+1:], "mp3") && strings.LastIndexByte(rel, '/') >= 0 {
		// The name of a song is an artist and a title: only the folders say what it is.
		rel = rel[:strings.LastIndexByte(rel, '/')]
	}
	words := strings.FieldsFunc(strings.ToLower(rel), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '#'
	})
	for i := 0; i < len(words); i++ {
		word := words[i]
		// "one shot" and "one shots" are the one tag, oneshot.
		if word == "one" && i+1 < len(words) && (words[i+1] == "shot" || words[i+1] == "shots") {
			add("oneshot")
			i++
			continue
		}
		if isDigits(word) {
			if v, _ := strconv.Atoi(word); len(word) >= 2 && len(word) <= 3 && v >= 40 {
				add(word)
			}
			continue
		}
		if len(word) < 2 || stopPattern.MatchString(word) {
			continue
		}
		if m := sharpKey.FindStringSubmatch(word); m != nil {
			add(m[1] + "#")
			continue
		}
		if folded, ok := tagFold[word]; ok {
			add(folded)
			continue
		}
		add(word)
		if alias, ok := tagAliases[word]; ok {
			add(alias)
		}
		if m := trailing.FindStringSubmatch(word); m != nil {
			add(m[1])
		}
		for _, suffix := range tagSuffixes {
			if len(word) > len(suffix) && strings.HasSuffix(word, suffix) {
				add(suffix)
			}
		}
	}
	return tags
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// IsKeyTag reports whether a tag names a musical key, like cmin or f#m.
func IsKeyTag(t string) bool { return keyTag.MatchString(t) }

// readIfThere reads a file of the card, and says whether it exists.
func readIfThere(s Storage, path string) (data []byte, exists bool, err error) {
	if _, exists, err = s.Stat(path); err != nil || !exists {
		return nil, false, err
	}
	data, err = s.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	return data, err == nil, err
}

// LoadTagIndex reads the index from the SD card. A card without one gives an empty index.
func LoadTagIndex(s Storage) (*TagIndex, error) {
	t := NewTagIndex()
	data, exists, err := readIfThere(s, TagIndexPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", TagIndexPath, err)
	}
	if exists {
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err == nil {
			var f indexFile
			if err = json.NewDecoder(zr).Decode(&f); err == nil {
				t.Folders, t.Updated, t.User, t.UserFolders = f.Folders, f.Updated, f.User, f.UserFolders
				for dir, names := range f.Dirs {
					for _, name := range names {
						t.Files[Join(dir, name)] = ""
					}
				}
			}
		}
		if err != nil {
			return nil, fmt.Errorf("%s is not readable (%v): delete it to start again", TagIndexPath, err)
		}
	} else if data, exists, err = readIfThere(s, legacyTagIndexPath); err != nil {
		return nil, fmt.Errorf("reading %s: %w", legacyTagIndexPath, err)
	} else if exists {
		if err := json.Unmarshal(data, t); err != nil {
			return nil, fmt.Errorf("%s is not readable (%v): delete it to start again", legacyTagIndexPath, err)
		}
		if t.Files == nil {
			t.Files = map[string]string{}
		}
		t.legacy = true
	}
	t.Retag()
	return t, nil
}

// Save writes the index to the root of the SD card, and removes the older plain file.
func (t *TagIndex) Save(s Storage) error {
	sort.Strings(t.Folders)
	t.Updated = time.Now().Format("2006-01-02 15:04")
	f := indexFile{Version: tagIndexVersion, Updated: t.Updated, Folders: t.Folders, Dirs: map[string][]string{}, User: t.User, UserFolders: t.UserFolders}
	for path := range t.Files {
		dir := Dir(path)
		f.Dirs[dir] = append(f.Dirs[dir], Base(path))
	}
	for _, names := range f.Dirs {
		sort.Strings(names)
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if err := json.NewEncoder(zw).Encode(f); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	if err := s.WriteFile(TagIndexPath, buf.Bytes()); err != nil {
		return fmt.Errorf("writing %s: %w", TagIndexPath, err)
	}
	if _, old, err := s.Stat(legacyTagIndexPath); err == nil && old {
		_ = s.Unlink(legacyTagIndexPath)
	}
	return nil
}

// IndexFromTags is the search index of a source made from the saved index, without
// walking the card: every indexed file under root. Its Saved says when it was written.
func IndexFromTags(t *TagIndex, root string) *Index {
	ix := &Index{Root: root}
	if t == nil {
		return ix
	}
	dirs := map[string]bool{}
	for path, tags := range t.Files {
		if root != StorageRoot && !strings.HasPrefix(path, root) {
			continue
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(path, root), "/")
		ix.Files = append(ix.Files, IndexedFile{Path: path, Rel: rel, lower: strings.ToLower(rel) + " " + tags})
		dirs[Dir(path)] = true
	}
	sort.Slice(ix.Files, func(i, j int) bool { return NaturalLess(ix.Files[i].lower, ix.Files[j].lower) })
	ix.Folders, ix.Saved = len(dirs), t.Updated
	return ix
}

// Retag works the tags out again from each file's path, with the rules of this version, so
// that a better rule applies to what was indexed before without walking the card again.
// A file is read as part of the indexed folder it is in (the deepest one). A placeholder is
// dropped.
func (t *TagIndex) Retag() {
	for path := range t.Files {
		best := ""
		for _, f := range t.Folders {
			prefix := f
			if f != CardRoot && f != StorageRoot && !strings.HasSuffix(f, "/") {
				prefix += "/"
			}
			if strings.HasPrefix(path, prefix) && len(f) > len(best) {
				best = f
			}
		}
		if best == "" {
			continue
		}
		if isPlaceholder(path) {
			delete(t.Files, path)
			continue
		}
		t.Files[path] = strings.Join(TagsFor(path, Dir(best)), " ")
	}
	t.applyUser()
}

// applyUser adds the user's tags to the tags of the files that have them. A file the user
// tagged that is not in an indexed folder gets an entry too, so that it can be found by them.
func (t *TagIndex) applyUser() {
	for path, mine := range t.User {
		own, ok := t.Files[path]
		if !ok {
			own = strings.Join(TagsFor(path, Dir(path)), " ")
		}
		have := map[string]bool{}
		for _, tag := range strings.Fields(own) {
			have[tag] = true
		}
		for _, tag := range mine {
			if !have[tag] {
				own += " " + tag
				have[tag] = true
			}
		}
		t.Files[path] = strings.TrimSpace(own)
	}
	if len(t.UserFolders) == 0 {
		return
	}
	// A file has the tags of the folders above it, as well as its own.
	for path, own := range t.Files {
		have := map[string]bool{}
		for _, tag := range strings.Fields(own) {
			have[tag] = true
		}
		added := false
		for dir := Dir(path); dir != "" && dir != "/" && dir != CardRoot; dir = Dir(dir) {
			for _, tag := range t.UserFolders[dir] {
				if !have[tag] {
					own += " " + tag
					have[tag] = true
					added = true
				}
			}
		}
		if added {
			t.Files[path] = strings.TrimSpace(own)
		}
	}
}

// CleanTag turns what was typed into a tag: lower case, one word, only letters, digits,
// "-", "_" and "#". It is empty when nothing is left.
func CleanTag(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '#':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-_")
}

// AddUserTags gives every file in paths the tags, and returns how many files changed.
func (t *TagIndex) AddUserTags(paths, tags []string) int {
	if t.User == nil {
		t.User = map[string][]string{}
	}
	changed := 0
	for _, path := range paths {
		mine := t.User[path]
		added := false
		for _, tag := range tags {
			if tag = CleanTag(tag); tag != "" && !slices.Contains(mine, tag) {
				mine = append(mine, tag)
				added = true
			}
		}
		if added {
			sort.Strings(mine)
			t.User[path] = mine
			changed++
		}
	}
	t.applyUser()
	return changed
}

// RemoveUserTags takes the tags off every file in paths, and returns how many files changed.
// Only tags the user gave can be removed: the ones worked out from names come back.
func (t *TagIndex) RemoveUserTags(paths, tags []string) int {
	changed := 0
	for _, path := range paths {
		mine := t.User[path]
		keep := mine[:0:0]
		for _, tag := range mine {
			if !slices.Contains(tags, tag) {
				keep = append(keep, tag)
			}
		}
		if len(keep) != len(mine) {
			changed++
			if len(keep) == 0 {
				delete(t.User, path)
			} else {
				t.User[path] = keep
			}
			// The file's own tags are worked out again without the removed ones.
			delete(t.Files, path)
			t.restore(path)
		}
	}
	t.applyUser()
	return changed
}

// restore puts the file's own tags back, from its name and the indexed folder it is in.
func (t *TagIndex) restore(path string) {
	best := ""
	for _, f := range t.Folders {
		prefix := f
		if f != CardRoot && f != StorageRoot && !strings.HasSuffix(f, "/") {
			prefix += "/"
		}
		if strings.HasPrefix(path, prefix) && len(f) > len(best) {
			best = f
		}
	}
	if best != "" {
		t.Files[path] = strings.Join(TagsFor(path, Dir(best)), " ")
	}
}

// AddFolderTags gives every folder in dirs the tags: each file in them has the tags too, now
// and for the files that come later. It returns how many folders changed.
func (t *TagIndex) AddFolderTags(dirs, tags []string) int {
	if t.UserFolders == nil {
		t.UserFolders = map[string][]string{}
	}
	changed := 0
	for _, dir := range dirs {
		mine := t.UserFolders[dir]
		added := false
		for _, tag := range tags {
			if tag = CleanTag(tag); tag != "" && !slices.Contains(mine, tag) {
				mine = append(mine, tag)
				added = true
			}
		}
		if added {
			sort.Strings(mine)
			t.UserFolders[dir] = mine
			changed++
		}
	}
	t.applyUser()
	return changed
}

// RemoveFolderTags takes the tags off every folder in dirs, and the files in them lose them
// too (unless they have them of their own). It returns how many folders changed.
func (t *TagIndex) RemoveFolderTags(dirs, tags []string) int {
	changed := 0
	for _, dir := range dirs {
		mine := t.UserFolders[dir]
		keep := mine[:0:0]
		for _, tag := range mine {
			if !slices.Contains(tags, tag) {
				keep = append(keep, tag)
			}
		}
		if len(keep) == len(mine) {
			continue
		}
		changed++
		if len(keep) == 0 {
			delete(t.UserFolders, dir)
		} else {
			t.UserFolders[dir] = keep
		}
	}
	if changed > 0 {
		// The files below worked their tags out with the folder's: work them out again.
		t.Retag()
	}
	return changed
}

// HasUserTag says whether the user gave a file the tag.
func (t *TagIndex) HasUserTag(path, tag string) bool {
	return t != nil && (slices.Contains(t.User[path], tag) || slices.Contains(t.UserFolders[path], tag))
}

// Tags is a file's tags, or nil when it is not indexed.
func (t *TagIndex) Tags(path string) []string {
	if t == nil {
		return nil
	}
	return strings.Fields(t.Files[path])
}

// Search lists the indexed files that have a tag starting with each word of query.
func (t *TagIndex) Search(query string) []string {
	words := strings.Fields(strings.ToLower(query))
	if t == nil || len(words) == 0 {
		return nil
	}
	var out []string
files:
	for path, tags := range t.Files {
		fields := strings.Fields(tags)
		for _, w := range words {
			ok := false
			for _, tag := range fields {
				if strings.HasPrefix(tag, w) {
					ok = true
					break
				}
			}
			if !ok {
				continue files
			}
		}
		out = append(out, path)
	}
	sort.Slice(out, func(i, j int) bool { return NaturalLess(strings.ToLower(out[i]), strings.ToLower(out[j])) })
	return out
}

// AttachTags lets an index's search match tags too: a search for "hat" then finds "chat".
func (ix *Index) AttachTags(t *TagIndex) {
	if t == nil {
		return
	}
	for i, f := range ix.Files {
		if tags, ok := t.Files[f.Path]; ok {
			ix.Files[i].lower = strings.ToLower(f.Rel) + " " + tags
		}
	}
}

// TagProgress is how far an indexing run has come. Folders is how many have been read and
// Queued how many are known and still to read, so the share done is Folders/(Folders+Queued).
type TagProgress struct {
	Folders, Queued, Files int
	// Unreadable counts folders the unit would not list; their files are missing.
	Unreadable int
	Current    string
}

// IndexTags walks folder and tags every audio file in it, adding to t. list reads one
// folder; the caller can take a lock in it, or wait, so that other work on the unit goes
// on. cancel, when it returns true, ends the walk: what was found so far is kept. A walk
// that finishes also removes what t held for folder that is no longer there.
func IndexTags(list func(dir string) ([]fileapi.DirEntry, error), folder string, t *TagIndex, lim IndexLimits,
	progress func(TagProgress), cancel func() bool) (files int, completed bool, err error) {
	base := Dir(folder)
	found := map[string]string{}
	type item struct {
		path  string
		depth int
	}
	queue := []item{{folder, 0}}
	prog := TagProgress{}
	report := func(cur string) {
		prog.Queued, prog.Current = len(queue), cur
		if progress != nil {
			progress(prog)
		}
	}
	completed = true
	var unreadable []string
	for len(queue) > 0 {
		if cancel != nil && cancel() {
			completed = false
			break
		}
		if prog.Folders >= lim.MaxFolders || len(found) >= lim.MaxFiles {
			completed = false
			break
		}
		cur := queue[0]
		queue = queue[1:]
		entries, lerr := ListDirRetry(list, cur.path)
		if lerr != nil {
			if prog.Folders == 0 {
				return 0, false, lerr
			}
			prog.Unreadable++
			unreadable = append(unreadable, cur.path)
			report(cur.path)
			continue
		}
		prog.Folders++
		for _, e := range entries {
			path := Join(cur.path, e.Name)
			switch {
			case e.IsDir():
				if cur.depth+1 <= lim.MaxDepth && !skipFolders[e.Name] {
					queue = append(queue, item{path, cur.depth + 1})
				}
			case formats.IsAudioFile(e.Name):
				if isPlaceholder(path) {
					continue
				}
				found[path] = strings.Join(TagsFor(path, base), " ")
				prog.Files++
			}
		}
		report(cur.path)
	}
	prefix := folder
	if folder != CardRoot && folder != StorageRoot && !strings.HasSuffix(folder, "/") {
		prefix += "/"
	}
	if completed {
		for path := range t.Files {
			if !strings.HasPrefix(path, prefix) {
				continue
			}
			// What was known of a folder the unit would not list this time is kept.
			keep := false
			for _, bad := range unreadable {
				if strings.HasPrefix(path, bad+"/") {
					keep = true
					break
				}
			}
			if !keep {
				delete(t.Files, path)
			}
		}
	}
	for path, tags := range found {
		t.Files[path] = tags
	}
	t.applyUser()
	for _, f := range t.Folders {
		if f == folder {
			return len(found), completed, nil
		}
	}
	t.Folders = append(t.Folders, folder)
	return len(found), completed, nil
}

// Matches lists the indexed files under root that have a tag starting with each word of
// query, as search results.
func (t *TagIndex) Matches(query, root string, limit int) []IndexedFile {
	if t == nil {
		return nil
	}
	var out []IndexedFile
	for _, path := range t.Search(query) {
		if root != StorageRoot && !strings.HasPrefix(path, root) {
			continue
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(path, root), "/")
		out = append(out, IndexedFile{Path: path, Rel: rel, lower: strings.ToLower(rel)})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

// TagCount is a tag and how many files have it.
type TagCount struct {
	Tag   string
	Count int
}

// Facets counts the tags of the indexed files under root that match query, most files
// first. The words of the query itself are left out, as every match has them: what is left
// is what could narrow the search further. With an empty query it counts every file.
func (t *TagIndex) Facets(query, root string, limit int) []TagCount {
	if t == nil {
		return nil
	}
	words := strings.Fields(strings.ToLower(query))
	var paths []string
	if len(words) == 0 {
		for path := range t.Files {
			paths = append(paths, path)
		}
	} else {
		paths = t.Search(query)
	}
	counts := map[string]int{}
	for _, path := range paths {
		if root != StorageRoot && !strings.HasPrefix(path, root) {
			continue
		}
		for _, tag := range strings.Fields(t.Files[path]) {
			counts[tag]++
		}
	}
	for _, w := range words {
		delete(counts, w)
	}
	out := make([]TagCount, 0, len(counts))
	for tag, n := range counts {
		out = append(out, TagCount{tag, n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Tag < out[j].Tag
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// UserTagCounts are the tags the user gave, with how many files carry each, most first.
func (t *TagIndex) UserTagCounts() []TagCount {
	if t == nil {
		return nil
	}
	counts := map[string]int{}
	for _, mine := range t.User {
		for _, tag := range mine {
			counts[tag]++
		}
	}
	for _, mine := range t.UserFolders {
		for _, tag := range mine {
			counts[tag]++
		}
	}
	return SortedCounts(counts)
}

// OwnTags are the tags a file has from its name and folders, without the ones the user gave.
func (t *TagIndex) OwnTags(path string) []string {
	if t == nil {
		return nil
	}
	var out []string
	for _, tag := range strings.Fields(t.Files[path]) {
		if !slices.Contains(t.User[path], tag) {
			out = append(out, tag)
		}
	}
	return out
}

// UserTagsOf are the tags the user gave a file.
func (t *TagIndex) UserTagsOf(path string) []string {
	if t == nil {
		return nil
	}
	return append(slices.Clone(t.User[path]), t.UserFolders[path]...)
}

// SortedCounts lists counts of tags, most first, then by name.
func SortedCounts(counts map[string]int) []TagCount {
	out := make([]TagCount, 0, len(counts))
	for tag, n := range counts {
		out = append(out, TagCount{tag, n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Tag < out[j].Tag
	})
	return out
}

// Clone is a copy of the index that shares nothing with it. The screen reads the index
// while a command on another goroutine changes it, so a change is made on a copy, and the
// copy replaces the index when it is done: an index that is on show is never written to.
func (t *TagIndex) Clone() *TagIndex {
	c := &TagIndex{
		Version: t.Version, Updated: t.Updated, legacy: t.legacy,
		Folders:     slices.Clone(t.Folders),
		Files:       make(map[string]string, len(t.Files)),
		User:        make(map[string][]string, len(t.User)),
		UserFolders: make(map[string][]string, len(t.UserFolders)),
	}
	for path, mine := range t.UserFolders {
		c.UserFolders[path] = slices.Clone(mine)
	}
	for path, tags := range t.Files {
		c.Files[path] = tags
	}
	for path, mine := range t.User {
		c.User[path] = slices.Clone(mine)
	}
	return c
}

// UseUserTagsOf takes the tags the user gave from another copy of the index, in place of its
// own. A run that walks the card for a long time ends by doing this with the newest copy, so
// that the tags given while it ran are not lost.
func (t *TagIndex) UseUserTagsOf(from *TagIndex) {
	if from == nil {
		return
	}
	old := t.User
	t.User = make(map[string][]string, len(from.User))
	for path, mine := range from.User {
		t.User[path] = slices.Clone(mine)
	}
	t.UserFolders = make(map[string][]string, len(from.UserFolders))
	for path, mine := range from.UserFolders {
		t.UserFolders[path] = slices.Clone(mine)
	}
	// A folder's tags reach many files: they are all worked out again.
	defer t.Retag()
	// What the files had from the old tags is worked out again, so a tag taken off is gone.
	for path := range old {
		delete(t.Files, path)
		t.restore(path)
	}
	t.applyUser()
}

// FolderTagged are the folders the user gave the tag.
func (t *TagIndex) FolderTagged(tag string) map[string]bool {
	out := map[string]bool{}
	if t == nil {
		return out
	}
	for dir, mine := range t.UserFolders {
		if slices.Contains(mine, tag) {
			out[dir] = true
		}
	}
	return out
}
