// Package skill discovers SKILL.md playbooks from configured roots.
package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"
)

// Skill is one discovered playbook.
type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Body        string `json:"body"`
}

// Loader scans roots for SKILL.md files.
type Loader struct {
	roots []string

	mu     sync.RWMutex
	skills []Skill
	byName map[string]Skill
}

// New creates a loader over roots.
func New(roots []string) *Loader {
	l := &Loader{roots: roots, byName: map[string]Skill{}}
	_ = l.Reload()
	return l
}

// Reload rescans the roots. Missing roots are skipped.
func (l *Loader) Reload() error {
	found := map[string]Skill{}
	for _, root := range l.roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.EqualFold(d.Name(), "SKILL.md") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			s := parse(path, string(data))
			if _, exists := found[s.Name]; exists {
				return nil
			}
			found[s.Name] = s
			return nil
		})
	}
	names := make([]string, 0, len(found))
	for name := range found {
		names = append(names, name)
	}
	sort.Strings(names)
	list := make([]Skill, 0, len(names))
	for _, name := range names {
		list = append(list, found[name])
	}
	l.mu.Lock()
	l.skills = list
	l.byName = found
	l.mu.Unlock()
	return nil
}

// List returns skills sorted by name.
func (l *Loader) List() []Skill {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return append([]Skill(nil), l.skills...)
}

// Get returns one skill by name.
func (l *Loader) Get(name string) (Skill, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	s, ok := l.byName[name]
	return s, ok
}

// Index renders a compact list for the system prompt.
func (l *Loader) Index() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if len(l.skills) == 0 {
		return ""
	}
	var b strings.Builder
	for _, s := range l.skills {
		b.WriteString("- ")
		b.WriteString(s.Name)
		if s.Description != "" {
			b.WriteString(": ")
			b.WriteString(s.Description)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func parse(path, content string) Skill {
	name := strings.TrimSuffix(filepath.Base(filepath.Dir(path)), string(filepath.Separator))
	if strings.EqualFold(filepath.Base(filepath.Dir(path)), ".") {
		name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	body := content
	description := ""
	if strings.HasPrefix(content, "---") {
		parts := strings.SplitN(content, "---", 3)
		if len(parts) == 3 {
			front := parts[1]
			body = strings.TrimLeft(parts[2], "\r\n")
			for _, line := range strings.Split(front, "\n") {
				key, value, ok := strings.Cut(line, ":")
				if !ok {
					continue
				}
				key = strings.TrimSpace(strings.ToLower(key))
				value = strings.TrimSpace(strings.Trim(value, `"'`))
				switch key {
				case "name":
					if value != "" {
						name = value
					}
				case "description":
					description = value
				}
			}
		}
	}
	if description == "" {
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "<!--") {
				continue
			}
			description = trimRunes(line, 200)
			break
		}
	}
	if name == "" {
		name = "skill"
	}
	return Skill{Name: name, Description: description, Path: path, Body: body}
}

func trimRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// Save creates or updates a SKILL.md under the first configured root. The
// returned skill is re-read from disk, so the caller sees the canonical body.
func (l *Loader) Save(name, description, body string) (Skill, error) {
	name = strings.ReplaceAll(strings.TrimSpace(name), "\n", " ")
	if name == "" {
		return Skill{}, fmt.Errorf("skill name is required")
	}
	l.mu.RLock()
	existing, exists := l.byName[name]
	roots := append([]string(nil), l.roots...)
	l.mu.RUnlock()

	target := ""
	if exists {
		target = existing.Path
	} else {
		root := ""
		for _, candidate := range roots {
			if strings.TrimSpace(candidate) != "" {
				root = strings.TrimSpace(candidate)
				break
			}
		}
		if root == "" {
			return Skill{}, fmt.Errorf("no skills directory configured")
		}
		slug := skillSlug(name)
		if slug == "" {
			slug = "skill"
		}
		target = filepath.Join(root, slug, "SKILL.md")
		if _, err := os.Stat(target); err == nil {
			return Skill{}, fmt.Errorf("技能目录 %q 已存在，请换一个名称", slug)
		}
	}

	description = strings.ReplaceAll(strings.TrimSpace(description), "\n", " ")
	front := "name: " + name + "\ndescription: " + description
	if exists {
		if data, err := os.ReadFile(target); err == nil {
			front = preservedFrontmatter(string(data), name, description)
		}
	}
	content := "---\n" + front + "\n---\n\n" + strings.TrimSpace(body) + "\n"
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return Skill{}, err
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		return Skill{}, err
	}
	if err := l.Reload(); err != nil {
		return Skill{}, err
	}
	saved, ok := l.Get(name)
	if !ok {
		return Skill{}, fmt.Errorf("skill %q saved but not discovered", name)
	}
	return saved, nil
}

func skillSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == ' ' || r == '.':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func preservedFrontmatter(existing, name, description string) string {
	extras := []string{}
	if strings.HasPrefix(existing, "---") {
		parts := strings.SplitN(existing, "---", 3)
		if len(parts) == 3 {
			for _, line := range strings.Split(parts[1], "\n") {
				key, _, ok := strings.Cut(line, ":")
				if ok {
					switch strings.ToLower(strings.TrimSpace(key)) {
					case "name", "description":
						continue
					}
				}
				if strings.TrimSpace(line) != "" {
					extras = append(extras, strings.TrimRight(line, "\r"))
				}
			}
		}
	}
	lines := []string{"name: " + name, "description: " + description}
	lines = append(lines, extras...)
	return strings.Join(lines, "\n")
}
