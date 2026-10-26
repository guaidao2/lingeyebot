// Package knowledge stores per-role methodology files. Knowledge is separate
// from shared Skills: each agent role only sees its own directory, and the
// tools are scoped by the role carried in the tool environment.
package knowledge

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	// MaxBody bounds a knowledge document. Keep methodology, not logs.
	MaxBody = 200_000
	dirName = "knowledges"
)

type Meta struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	UpdatedAt   string `json:"updatedAt"`
}

type Doc struct {
	Meta
	Body string `json:"body"`
}

type Store struct {
	root string
}

// New creates a store rooted at <dataDir>/agents.
func New(root string) *Store { return &Store{root: root} }

func (s *Store) List(role string) ([]Meta, error) {
	dir, err := s.roleDir(role)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Meta{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Meta{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		doc, err := readDoc(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		out = append(out, doc.Meta)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *Store) Get(role, name string) (Doc, error) {
	path, err := s.docPath(role, name)
	if err != nil {
		return Doc{}, err
	}
	return readDoc(path)
}

func (s *Store) Exists(role, name string) bool {
	path, err := s.docPath(role, name)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

func (s *Store) Create(role, name, description, body string) (Doc, error) {
	if s.Exists(role, name) {
		return Doc{}, fmt.Errorf("knowledge %q already exists for role %q", name, role)
	}
	return s.Save(role, name, description, body)
}

func (s *Store) Update(role, name, description, body string) (Doc, error) {
	if !s.Exists(role, name) {
		return Doc{}, fmt.Errorf("knowledge %q does not exist for role %q", name, role)
	}
	return s.Save(role, name, description, body)
}

func (s *Store) Save(role, name, description, body string) (Doc, error) {
	path, err := s.docPath(role, name)
	if err != nil {
		return Doc{}, err
	}
	description = strings.TrimSpace(description)
	if description == "" {
		return Doc{}, errors.New("knowledge description is required")
	}
	if len(description) > 512 {
		return Doc{}, errors.New("knowledge description is too long")
	}
	if len(body) > MaxBody {
		return Doc{}, fmt.Errorf("knowledge body exceeds %d bytes", MaxBody)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return Doc{}, err
	}
	doc := Doc{Meta: Meta{Name: strings.TrimSpace(name), Description: description, UpdatedAt: time.Now().UTC().Format(time.RFC3339)}, Body: strings.TrimRight(body, "\n")}
	content := "---\nname: " + doc.Name + "\ndescription: " + escapeScalar(doc.Description) + "\n---\n\n" + doc.Body + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return Doc{}, err
	}
	return doc, nil
}

// Index returns the metadata index injected into a role prompt.
func (s *Store) Index(role string) string {
	docs, err := s.List(role)
	if err != nil || len(docs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, doc := range docs {
		fmt.Fprintf(&b, "- %s: %s\n", doc.Name, doc.Description)
	}
	return b.String()
}

func (s *Store) roleDir(role string) (string, error) {
	if err := validRole(role); err != nil {
		return "", err
	}
	return filepath.Join(s.root, role, dirName), nil
}

func (s *Store) docPath(role, name string) (string, error) {
	dir, err := s.roleDir(role)
	if err != nil {
		return "", err
	}
	if err := validName(name); err != nil {
		return "", err
	}
	return filepath.Join(dir, strings.TrimSpace(name)+".md"), nil
}

func validRole(role string) error {
	role = strings.TrimSpace(role)
	if role == "" || role == "." || role == ".." {
		return errors.New("invalid role")
	}
	for _, r := range role {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			continue
		}
		return fmt.Errorf("invalid role %q", role)
	}
	return nil
}

func validName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 128 || name == "." || name == ".." {
		return errors.New("invalid knowledge name")
	}
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			continue
		}
		return fmt.Errorf("invalid knowledge name %q", name)
	}
	return nil
}

func readDoc(path string) (Doc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Doc{}, err
	}
	name, description, body, err := parse(string(data))
	if err != nil {
		return Doc{}, err
	}
	info, _ := os.Stat(path)
	updated := ""
	if info != nil {
		updated = info.ModTime().UTC().Format(time.RFC3339)
	}
	return Doc{Meta: Meta{Name: name, Description: description, UpdatedAt: updated}, Body: body}, nil
}

func parse(text string) (string, string, string, error) {
	text = strings.TrimPrefix(text, "\ufeff")
	if !strings.HasPrefix(text, "---") {
		return "", "", "", errors.New("knowledge frontmatter missing")
	}
	parts := strings.SplitN(text, "---", 3)
	if len(parts) != 3 {
		return "", "", "", errors.New("knowledge frontmatter malformed")
	}
	var name, description string
	for _, line := range strings.Split(parts[1], "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "name":
			name = strings.TrimSpace(value)
		case "description":
			description = unescapeScalar(strings.TrimSpace(value))
		}
	}
	if name == "" || description == "" {
		return "", "", "", errors.New("knowledge frontmatter requires name and description")
	}
	return name, description, strings.TrimLeft(parts[2], "\r\n"), nil
}

func escapeScalar(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	return strings.ReplaceAll(value, "\n", " ")
}

func unescapeScalar(value string) string {
	return strings.ReplaceAll(value, "\\\\", "\\")
}
