package sim

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
	"github.com/moahbeatlab/404tui/proto/fileapi"
)

// A Folder is a set of projects on this computer that the simulated unit works on in place
// of a real one, for working offline. Each folder holding a PADCONF.BIN (a project as the
// unit stores it, or as 404tui exports it) is one project slot, in the order of their names;
// a new project becomes a new folder. The SD card is the folder SD next to them. Every
// change is written to these folders as it is made.
type folder struct {
	// lib holds the projects; sd is the SD card.
	lib, sd string
	// slots are the project folders, "" for a slot with no project yet.
	slots [16]string
	// conf is each project's PADCONF.BIN as read, the base its file is written from;
	// written is what was last written, so that only a change writes it again.
	conf    [16]*formats.Padconf
	written [16][]byte
	words   [16][proto.SettingsWords]uint32
	// template is a project read from a real unit, that new projects copy their unknown
	// settings from; nil when the folder has none.
	template *formats.Padconf
	// dirty are paths (as the simulation keys them) changed since they were written.
	dirty map[string]bool
	dirs  map[string]bool
	// lazy are files that are on disk and not read yet, with their sizes.
	lazy  map[string]string
	sizes map[string]int
}

// Folder is where an offline simulation keeps its projects, and the projects it found.
type Folder struct {
	Dir      string
	Projects []string
	// Template says whether new projects can copy a project made by the unit; without one
	// they get the documented defaults, which are not all known.
	Template bool
}

var storagePrefix = clean(fileapi.Root)

// projectPath matches a path in a project folder of the unit's storage.
var projectPath = regexp.MustCompile(`^` + regexp.QuoteMeta(storagePrefix) + `/ROLAND/SP-404MKII/PROJECT_(\d\d)(?:/(.*))?$`)

// OpenFolder is a simulated unit working on the projects in dir. dir may also be one project
// folder: then the folder it is in holds the projects, and that one is selected.
func OpenFolder(dir string) (*Sim, Folder, error) {
	dir = filepath.Clean(dir)
	info, err := os.Stat(dir)
	if err != nil {
		return nil, Folder{}, err
	}
	if !info.IsDir() {
		return nil, Folder{}, fmt.Errorf("%s is not a folder", dir)
	}
	lib, start := dir, ""
	if isProject(dir) {
		lib, start = filepath.Dir(dir), dir
	}
	entries, err := os.ReadDir(lib)
	if err != nil {
		return nil, Folder{}, err
	}
	var found []string
	for _, e := range entries {
		if p := filepath.Join(lib, e.Name()); e.IsDir() && isProject(p) {
			found = append(found, p)
		}
	}
	sort.Strings(found)
	if start != "" && !contains(found, start) {
		found = append([]string{start}, found...)
	}
	slots := placeProjects(found)
	// A folder is named for its project: the number of the slot and the project's name. A folder
	// whose project was renamed meanwhile, or was exported under another name, is renamed now,
	// before anything of it is read.
	for i, p := range slots {
		if p == "" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(p, "PADCONF.BIN"))
		if err != nil {
			continue
		}
		conf, err := formats.ParsePadconf(raw)
		if err != nil || conf.Name == "" {
			continue
		}
		if now := renameProjectFolder(p, i, conf.Name); now != p {
			if p == start {
				start = now
			}
			slots[i] = now
		}
	}

	s := New()
	d := &folder{lib: lib, sd: filepath.Join(lib, "SD"), dirty: map[string]bool{}, dirs: map[string]bool{},
		lazy: map[string]string{}, sizes: map[string]int{}}
	s.disk = d
	out := Folder{Dir: lib}
	for i := range s.names {
		d.words[i] = formats.NewPadconf(nil, "").Settings
	}
	for i, p := range slots {
		if p == "" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(p, "PADCONF.BIN"))
		if err != nil {
			return nil, Folder{}, err
		}
		conf, err := formats.ParsePadconf(raw)
		if err != nil {
			return nil, Folder{}, fmt.Errorf("%s: %w", p, err)
		}
		d.slots[i], d.conf[i], d.words[i] = p, conf, conf.Settings
		d.written[i] = conf.Bytes()
		if d.template == nil && raw[8] == 3 {
			d.template = conf
		}
		s.names[i] = conf.Name
		if conf.Name == "" {
			s.names[i] = filepath.Base(p)
		}
		prefix := fmt.Sprintf("%s/ROLAND/SP-404MKII/PROJECT_%02d", storagePrefix, i+1)
		if err := s.load(p, prefix); err != nil {
			return nil, Folder{}, err
		}
		s.files[prefix+"/.sim-blocks"] = bytes.Join(conf.Blocks[:], nil)
		if p == start {
			s.project = uint8(i)
		}
		out.Projects = append(out.Projects, p)
	}
	if err := s.load(d.sd, ""); err != nil && !os.IsNotExist(err) {
		return nil, Folder{}, err
	}
	out.Template = d.template != nil
	s.loadProject()
	return s, out, nil
}

// slotPrefix is the number a project's folder starts with ("02 " in "02 Flavors"), 1 to 16, or 0.
var slotPrefix = regexp.MustCompile(`^(\d\d) `)

// safeName makes a project's name fit a folder name: what a file name cannot hold becomes _.
func safeName(name string) string {
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < ' ' {
			return '_'
		}
		return r
	}, strings.TrimSpace(name))
	return strings.Trim(name, ". ")
}

// folderFor is what the folder of the project in slot (0-based) is called: the slot number and
// the project's name, so that the order of the folder names is the order of the slots whatever
// the projects are called. A project that still has its default name keeps PROJECT_NN.
func folderFor(slot int, name string) string {
	safe := safeName(name)
	if safe == "" || safe == fmt.Sprintf("PROJECT_%02d", slot+1) {
		return fmt.Sprintf("PROJECT_%02d", slot+1)
	}
	return fmt.Sprintf("%02d %s", slot+1, safe)
}

// placeProjects puts the project folders in their slots: one that starts with its number goes
// there, and the rest, in the order of their names, fill the slots that are free.
func placeProjects(found []string) [16]string {
	var slots [16]string
	var rest []string
	for _, p := range found {
		if m := slotPrefix.FindStringSubmatch(filepath.Base(p)); m != nil {
			if n, _ := strconv.Atoi(m[1]); n >= 1 && n <= 16 && slots[n-1] == "" {
				slots[n-1] = p
				continue
			}
		}
		rest = append(rest, p)
	}
	for _, p := range rest {
		for i := range slots {
			if slots[i] == "" {
				slots[i] = p
				break
			}
		}
	}
	return slots
}

// renameProjectFolder gives the folder of a project the name that goes with the project's name,
// when it has another and nothing is in the way. It returns where the folder is now.
func renameProjectFolder(dir string, slot int, name string) string {
	want := filepath.Join(filepath.Dir(dir), folderFor(slot, name))
	if want == dir {
		return dir
	}
	if _, err := os.Stat(want); err == nil {
		return dir
	}
	if os.Rename(dir, want) != nil {
		return dir
	}
	return want
}

func isProject(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "PADCONF.BIN"))
	return err == nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// load lists the files under local as files of the simulation under prefix, to be read
// when they are first asked for.
func (s *Sim) load(local, prefix string) error {
	return filepath.WalkDir(local, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(local, path)
		if rel == "." {
			if prefix != "" {
				s.dirs[prefix] = true
			}
			return nil
		}
		at := prefix + "/" + filepath.ToSlash(rel)
		if e.IsDir() {
			s.dirs[at] = true
			return nil
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		s.files[at] = nil
		s.disk.lazy[at], s.disk.sizes[at] = path, int(info.Size())
		return nil
	})
}

// content is a file's bytes, read from disk the first time.
func (s *Sim) content(path string) []byte {
	if d := s.disk; d != nil {
		if local, ok := d.lazy[path]; ok {
			if data, err := os.ReadFile(local); err == nil {
				s.files[path] = data
			}
			delete(d.lazy, path)
			delete(d.sizes, path)
		}
	}
	return s.files[path]
}

// size is a file's length, without reading it.
func (s *Sim) size(path string) int {
	if s.disk != nil {
		if n, ok := s.disk.sizes[path]; ok {
			return n
		}
	}
	return len(s.files[path])
}

// setFile stores a file's bytes, to be written to disk.
func (s *Sim) setFile(path string, data []byte) {
	if s.disk != nil {
		delete(s.disk.lazy, path)
		delete(s.disk.sizes, path)
		s.disk.dirty[path] = true
	}
	s.files[path] = data
}

// remove takes a file away, also from disk.
func (s *Sim) remove(path string) {
	if s.disk != nil {
		delete(s.disk.lazy, path)
		delete(s.disk.sizes, path)
		s.disk.dirty[path] = true
	}
	delete(s.files, path)
}

// dirChanged notes a folder made or removed, to be made or removed on disk.
func (s *Sim) dirChanged(path string) {
	if s.disk != nil {
		s.disk.dirs[path] = true
	}
}

// local is where a path of the simulation lives on disk; false for what is not kept there.
func (d *folder) local(path string, names [16]string) (string, bool) {
	if m := projectPath.FindStringSubmatch(path); m != nil {
		n, _ := strconv.Atoi(m[1])
		if n < 1 || n > 16 || m[2] == "" || strings.HasPrefix(m[2], ".sim-") {
			return "", false
		}
		dir, err := d.slot(n-1, names[n-1])
		if err != nil {
			return "", false
		}
		return filepath.Join(dir, filepath.FromSlash(m[2])), true
	}
	if path == "" || path == storagePrefix || strings.HasPrefix(path, storagePrefix+"/") {
		return "", false
	}
	return filepath.Join(d.sd, filepath.FromSlash(strings.TrimPrefix(path, "/"))), true
}

// slot is a project's folder, made when the project first has something to keep.
func (d *folder) slot(project int, name string) (string, error) {
	if d.slots[project] != "" {
		return d.slots[project], nil
	}
	base := fmt.Sprintf("PROJECT_%02d", project+1)
	dir := filepath.Join(d.lib, base)
	for i := 2; ; i++ {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			break
		}
		dir = filepath.Join(d.lib, fmt.Sprintf("%s (%d)", base, i))
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	d.slots[project] = dir
	return dir, nil
}

// flush writes what changed to disk: files and folders, and the current project's
// PADCONF.BIN when its pads, settings or name changed.
func (s *Sim) flush() {
	d := s.disk
	if d == nil {
		return
	}
	open := map[string]bool{}
	for _, h := range s.handles {
		open[h.path] = true
	}
	for path := range d.dirs {
		local, ok := d.local(path, s.names)
		if !ok {
			delete(d.dirs, path)
			continue
		}
		if s.dirs[path] {
			_ = os.MkdirAll(local, 0o755)
		} else {
			_ = os.Remove(local)
		}
		delete(d.dirs, path)
	}
	for path := range d.dirty {
		if open[path] {
			continue // still being written
		}
		delete(d.dirty, path)
		local, ok := d.local(path, s.names)
		if !ok {
			continue
		}
		data, exists := s.files[path]
		if !exists {
			_ = os.Remove(local)
			continue
		}
		if _, lazy := d.lazy[path]; lazy {
			continue
		}
		if os.MkdirAll(filepath.Dir(local), 0o755) == nil {
			_ = os.WriteFile(local, data, 0o644)
		}
	}
	s.flushPadconf()
}

// flushPadconf writes the current project's PADCONF.BIN when it changed. A slot that never
// held anything gets a file (and a folder) once it has a sample or a name of its own.
func (s *Sim) flushPadconf() {
	d, p := s.disk, int(s.project)
	if d == nil {
		return
	}
	d.words[p] = s.wordsOf(p)
	conf := d.conf[p]
	if conf == nil {
		used := s.names[p] != fmt.Sprintf("PROJECT_%02d", p+1)
		for i := range s.blocks {
			used = used || s.hasSample(uint16(i))
		}
		if !used && d.slots[p] == "" {
			return
		}
		conf = formats.NewPadconf(d.template, s.names[p])
		d.conf[p] = conf
	}
	conf.Settings, conf.Name = d.words[p], s.names[p]
	// A project that is renamed takes its folder's name along.
	if old := d.slots[p]; old != "" && s.names[p] != "" {
		if now := renameProjectFolder(old, p, s.names[p]); now != old {
			d.slots[p] = now
			for k, v := range d.lazy {
				if rel, ok := strings.CutPrefix(v, old+string(filepath.Separator)); ok {
					d.lazy[k] = filepath.Join(now, rel)
				}
			}
		}
	}
	for i := range s.blocks {
		conf.Blocks[i] = s.blocks[i]
	}
	out := conf.Bytes()
	if bytes.Equal(out, d.written[p]) {
		return
	}
	dir, err := d.slot(p, s.names[p])
	if err != nil {
		return
	}
	if os.WriteFile(filepath.Join(dir, "PADCONF.BIN"), out, 0o644) == nil {
		d.written[p] = out
		// The unit keeps the file in the project folder, where an export looks for it.
		path := fmt.Sprintf("%s/PADCONF.BIN", s.folder())
		s.files[path] = out
		delete(d.lazy, path)
		delete(d.sizes, path)
		s.dirs[s.folder()] = true
	}
}

// wordsOf is a project's settings words with the bank tempos and volumes in force.
func (s *Sim) wordsOf(project int) [proto.SettingsWords]uint32 {
	w := s.disk.words[project]
	if project != int(s.project) {
		return w
	}
	for b := 0; b < proto.Banks; b++ {
		w[12+b] = w[12+b]&1 | s.bankTempo[b]<<1
		shift := 16 * (b % 2)
		w[23+b/2] = w[23+b/2]&^(0xFFFF<<shift) | (s.bankVol[b]&0xFFFF)<<shift
	}
	return w
}

// useWords takes the bank tempos and volumes of a project's settings.
func (s *Sim) useWords(project int) {
	w := s.disk.words[project]
	for b := 0; b < proto.Banks; b++ {
		s.bankTempo[b] = w[12+b] >> 1 & 0xFFFF
		s.bankVol[b] = w[23+b/2] >> (16 * (b % 2)) & 0xFFFF
	}
}

// blocksFromPadconf reads the current project's pads from its PADCONF.BIN, as the unit does
// when it loads a project; false when there is none.
func (s *Sim) blocksFromPadconf() bool {
	raw := s.content(s.folder() + "/PADCONF.BIN")
	conf, err := formats.ParsePadconf(raw)
	if err != nil {
		return false
	}
	for i := range s.blocks {
		s.blocks[i] = bytes.Clone(conf.Blocks[i])
		binary.LittleEndian.PutUint16(s.blocks[i][1:], uint16(i))
	}
	if s.disk != nil {
		p := int(s.project)
		if s.disk.conf[p] == nil {
			s.disk.conf[p] = conf
		}
		s.disk.words[p] = conf.Settings
		if conf.Name != "" {
			s.names[p] = conf.Name
		}
	}
	return true
}
