package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func jsonUnmarshal(raw []byte, v any) error { return json.Unmarshal(raw, v) }

func fileTools(env Env) []Spec {
	return []Spec{
		readFileTool(env),
		writeFileTool(env),
		editFileTool(env),
		listDirTool(env),
		grepTool(env),
	}
}

func readFileTool(env Env) Spec {
	return Spec{
		Name:     "read_file",
		ReadOnly: true,
		Description: "Read a text file. Paths are relative to the workspace unless absolute. " +
			"Use offset/limit for large files.",
		Parameters: obj(map[string]any{
			"path":   str("File path."),
			"offset": integer("1-based start line (default 1)."),
			"limit":  integer("Maximum lines to return (default 400, max 2000)."),
		}, "path"),
		Fn: func(_ context.Context, raw anyArgs) (string, error) {
			var args struct {
				Path   string `json:"path"`
				Offset int    `json:"offset"`
				Limit  int    `json:"limit"`
			}
			if err := decodeInto(raw, &args); err != nil {
				return "", err
			}
			full := env.resolve(args.Path)
			info, err := os.Stat(full)
			if err != nil {
				return "", err
			}
			if info.Size() > 16<<20 {
				return "", fmt.Errorf("file is too large (%d bytes); use grep or a smaller range", info.Size())
			}
			data, err := os.ReadFile(full)
			if err != nil {
				return "", err
			}
			if strings.ContainsRune(string(data), 0) {
				return "", errors.New("file appears to be binary")
			}
			lines := strings.Split(string(data), "\n")
			start := args.Offset
			if start <= 0 {
				start = 1
			}
			limit := args.Limit
			if limit <= 0 {
				limit = 400
			}
			if limit > 2000 {
				limit = 2000
			}
			if start > len(lines) {
				return "", fmt.Errorf("offset %d beyond end of file (%d lines)", start, len(lines))
			}
			end := start - 1 + limit
			if end > len(lines) {
				end = len(lines)
			}
			var b strings.Builder
			for i := start - 1; i < end; i++ {
				fmt.Fprintf(&b, "%d\t%s\n", i+1, lines[i])
			}
			if end < len(lines) {
				fmt.Fprintf(&b, "...(%d more lines)\n", len(lines)-end)
			}
			return b.String(), nil
		},
	}
}

func writeFileTool(env Env) Spec {
	return Spec{
		Name: "write_file",
		Description: "Create or overwrite a text file. Parent directories are created automatically. " +
			"Use edit_file for targeted changes instead of rewriting large files.",
		Parameters: obj(map[string]any{
			"path":    str("File path."),
			"content": str("Full file content."),
		}, "path", "content"),
		Fn: func(_ context.Context, raw anyArgs) (string, error) {
			var args struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			}
			if err := decodeInto(raw, &args); err != nil {
				return "", err
			}
			full := env.resolve(args.Path)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				return "", err
			}
			if err := os.WriteFile(full, []byte(args.Content), 0o644); err != nil {
				return "", err
			}
			return fmt.Sprintf("wrote %d bytes to %s", len(args.Content), full), nil
		},
	}
}

func editFileTool(env Env) Spec {
	return Spec{
		Name: "edit_file",
		Description: "Replace one exact occurrence of old_string with new_string. " +
			"The old string must occur exactly once; add context to disambiguate.",
		Parameters: obj(map[string]any{
			"path":       str("File path."),
			"old_string": str("Exact text to replace."),
			"new_string": str("Replacement text."),
		}, "path", "old_string", "new_string"),
		Fn: func(_ context.Context, raw anyArgs) (string, error) {
			var args struct {
				Path      string `json:"path"`
				OldString string `json:"old_string"`
				NewString string `json:"new_string"`
			}
			if err := decodeInto(raw, &args); err != nil {
				return "", err
			}
			full := env.resolve(args.Path)
			data, err := os.ReadFile(full)
			if err != nil {
				return "", err
			}
			count := strings.Count(string(data), args.OldString)
			if count == 0 {
				return "", errors.New("old_string not found")
			}
			if count > 1 {
				return "", fmt.Errorf("old_string occurs %d times; add context", count)
			}
			updated := strings.Replace(string(data), args.OldString, args.NewString, 1)
			if err := os.WriteFile(full, []byte(updated), 0o644); err != nil {
				return "", err
			}
			return fmt.Sprintf("edited %s", full), nil
		},
	}
}

func listDirTool(env Env) Spec {
	return Spec{
		Name:     "list_dir",
		ReadOnly: true,
		Description: "List a directory (files and subdirectories), newest names sorted. " +
			"Defaults to the workspace.",
		Parameters: obj(map[string]any{
			"path": str("Directory path."),
		}),
		Fn: func(_ context.Context, raw anyArgs) (string, error) {
			var args struct {
				Path string `json:"path"`
			}
			if err := decodeInto(raw, &args); err != nil {
				return "", err
			}
			dir := env.resolve(args.Path)
			entries, err := os.ReadDir(dir)
			if err != nil {
				return "", err
			}
			sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
			var b strings.Builder
			fmt.Fprintf(&b, "%s\n", dir)
			for i, entry := range entries {
				if i >= 500 {
					fmt.Fprintf(&b, "...(%d more)\n", len(entries)-i)
					break
				}
				info, _ := entry.Info()
				size := int64(0)
				if info != nil {
					size = info.Size()
				}
				kind := "f"
				if entry.IsDir() {
					kind = "d"
				}
				fmt.Fprintf(&b, "%s %10d %s\n", kind, size, entry.Name())
			}
			return b.String(), nil
		},
	}
}

func grepTool(env Env) Spec {
	return Spec{
		Name:     "grep",
		ReadOnly: true,
		Description: "Search text files under a path with a Go regular expression. " +
			"Skips .git, node_modules and binary files. Returns path:line:text.",
		Parameters: obj(map[string]any{
			"pattern":    str("Regular expression."),
			"path":       str("File or directory (default workspace)."),
			"glob":       str("Optional filename glob, e.g. *.go."),
			"maxResults": integer("Maximum matches (default 200, max 1000)."),
		}, "pattern"),
		Fn: func(_ context.Context, raw anyArgs) (string, error) {
			var args struct {
				Pattern    string `json:"pattern"`
				Path       string `json:"path"`
				Glob       string `json:"glob"`
				MaxResults int    `json:"maxResults"`
			}
			if err := decodeInto(raw, &args); err != nil {
				return "", err
			}
			re, err := regexp.Compile(args.Pattern)
			if err != nil {
				return "", err
			}
			limit := args.MaxResults
			if limit <= 0 {
				limit = 200
			}
			if limit > 1000 {
				limit = 1000
			}
			root := env.resolve(args.Path)
			var b strings.Builder
			matches := 0
			err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				if matches >= limit {
					return filepath.SkipAll
				}
				name := d.Name()
				if d.IsDir() {
					if name == ".git" || name == "node_modules" || name == "dist" || name == ".idea" {
						return filepath.SkipDir
					}
					return nil
				}
				if args.Glob != "" {
					ok, _ := filepath.Match(args.Glob, name)
					if !ok {
						return nil
					}
				}
				info, err := d.Info()
				if err != nil || info.Size() > 2<<20 {
					return nil
				}
				data, err := os.ReadFile(path)
				if err != nil || strings.ContainsRune(string(data), 0) {
					return nil
				}
				for i, line := range strings.Split(string(data), "\n") {
					if re.MatchString(line) {
						fmt.Fprintf(&b, "%s:%d:%s\n", path, i+1, strings.TrimRight(line, "\r"))
						matches++
						if matches >= limit {
							break
						}
					}
				}
				return nil
			})
			if err != nil {
				return "", err
			}
			if matches == 0 {
				return "no matches", nil
			}
			return b.String(), nil
		},
	}
}
