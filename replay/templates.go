package replay

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Templates keep a setup of the replay (the look, the style of the captions, the camera and its scenes, the chapters, the bus descriptions) to
// use for other takes. They are files in one folder of the user (the configuration folder, 404tui/replay-templates), a file for each, and one of them
// can be the one that new takes start with. What is inside is the page's own object: the server keeps it as it is.

// TemplateInfo says what a template holds.
type TemplateInfo struct {
	Name     string    `json:"name"`
	Parts    []string  `json:"parts"`
	Default  bool      `json:"default"`
	Modified time.Time `json:"modified"`
}

var templateNameRe = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N} _.\-]{0,58}$`)

func (r *Replay) templatesDir() (string, error) {
	dir := r.TemplatesDir
	if dir == "" {
		if env := os.Getenv("SP404_TEMPLATES"); env != "" {
			dir = env
		} else {
			cfg, err := os.UserConfigDir()
			if err != nil {
				return "", err
			}
			dir = filepath.Join(cfg, "404tui", "replay-templates")
		}
	}
	return dir, os.MkdirAll(dir, 0o755)
}

func templateFile(dir, name string) (string, error) {
	name = strings.TrimSpace(name)
	if !templateNameRe.MatchString(name) {
		return "", errors.New("a template name has letters, digits, spaces, - _ . (up to 60)")
	}
	return filepath.Join(dir, name+".json"), nil
}

func (r *Replay) defaultTemplateName(dir string) string {
	b, _ := os.ReadFile(filepath.Join(dir, "default.txt"))
	return strings.TrimSpace(string(b))
}

// ListTemplates are the saved templates, by name.
func (r *Replay) ListTemplates() ([]TemplateInfo, error) {
	dir, err := r.templatesDir()
	if err != nil {
		return nil, err
	}
	def := r.defaultTemplateName(dir)
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := []TemplateInfo{}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		info := TemplateInfo{Name: name, Default: name == def, Parts: []string{}}
		if st, err := e.Info(); err == nil {
			info.Modified = st.ModTime()
		}
		if b, err := os.ReadFile(filepath.Join(dir, e.Name())); err == nil {
			var t struct {
				Parts map[string]json.RawMessage `json:"parts"`
			}
			if json.Unmarshal(b, &t) == nil {
				for k, v := range t.Parts {
					if s := strings.TrimSpace(string(v)); s != "" && s != "null" && s != "{}" && s != "[]" {
						info.Parts = append(info.Parts, k)
					}
				}
				sort.Strings(info.Parts)
			}
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

// ReadTemplate is the file of a template.
func (r *Replay) ReadTemplate(name string) (json.RawMessage, error) {
	dir, err := r.templatesDir()
	if err != nil {
		return nil, err
	}
	f, err := templateFile(dir, name)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(f)
	if err != nil {
		return nil, fmt.Errorf("no template %q", name)
	}
	return b, nil
}

// WriteTemplate saves a template (the page's object, a JSON object).
func (r *Replay) WriteTemplate(name string, raw []byte) error {
	var probe map[string]any
	if json.Unmarshal(raw, &probe) != nil {
		return errors.New("the template is not a JSON object")
	}
	dir, err := r.templatesDir()
	if err != nil {
		return err
	}
	f, err := templateFile(dir, name)
	if err != nil {
		return err
	}
	return os.WriteFile(f, raw, 0o644)
}

// DeleteTemplate removes a template (and the choice of it as the default).
func (r *Replay) DeleteTemplate(name string) error {
	dir, err := r.templatesDir()
	if err != nil {
		return err
	}
	f, err := templateFile(dir, name)
	if err != nil {
		return err
	}
	if err := os.Remove(f); err != nil {
		return fmt.Errorf("no template %q", name)
	}
	if r.defaultTemplateName(dir) == strings.TrimSpace(name) {
		os.Remove(filepath.Join(dir, "default.txt"))
	}
	return nil
}

// SetDefaultTemplate chooses the template that new takes start with ("" for none).
func (r *Replay) SetDefaultTemplate(name string) error {
	dir, err := r.templatesDir()
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		os.Remove(filepath.Join(dir, "default.txt"))
		return nil
	}
	if _, err := r.ReadTemplate(name); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "default.txt"), []byte(name), 0o644)
}

// DefaultTemplate is the template new takes start with, when there is one.
func (r *Replay) DefaultTemplate() json.RawMessage {
	dir, err := r.templatesDir()
	if err != nil {
		return nil
	}
	name := r.defaultTemplateName(dir)
	if name == "" {
		return nil
	}
	b, err := r.ReadTemplate(name)
	if err != nil {
		return nil
	}
	return b
}

func (r *Replay) templateHandler(w http.ResponseWriter, req *http.Request) {
	rest := strings.TrimPrefix(req.URL.Path, "/templates")
	rest = strings.TrimPrefix(rest, "/")
	fail := func(code int, err error) { http.Error(w, err.Error(), code) }
	switch {
	case rest == "" && req.Method == http.MethodGet:
		l, err := r.ListTemplates()
		if err != nil {
			fail(http.StatusInternalServerError, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(l)
	case rest == "default" && req.Method == http.MethodPost:
		var b struct {
			Name string `json:"name"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<16)).Decode(&b) != nil {
			fail(http.StatusBadRequest, errors.New(`POST {"name": ...}`))
			return
		}
		if err := r.SetDefaultTemplate(b.Name); err != nil {
			fail(http.StatusBadRequest, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case rest != "" && req.Method == http.MethodGet:
		b, err := r.ReadTemplate(rest)
		if err != nil {
			fail(http.StatusNotFound, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	case rest != "" && (req.Method == http.MethodPost || req.Method == http.MethodPut):
		buf := make([]byte, 0, 1<<16)
		tmp := make([]byte, 1<<16)
		body := http.MaxBytesReader(w, req.Body, 16<<20) // a logo and a picture can be in it
		for {
			n, err := body.Read(tmp)
			buf = append(buf, tmp[:n]...)
			if err != nil {
				break
			}
		}
		if err := r.WriteTemplate(rest, buf); err != nil {
			fail(http.StatusBadRequest, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case rest != "" && req.Method == http.MethodDelete:
		if err := r.DeleteTemplate(rest); err != nil {
			fail(http.StatusNotFound, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "not that", http.StatusMethodNotAllowed)
	}
}
