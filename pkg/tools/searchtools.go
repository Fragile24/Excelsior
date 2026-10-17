package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

type ListFilesTool struct{ Root string }

func (t *ListFilesTool) Name() string   { return "list_files" }
func (t *ListFilesTool) ReadOnly() bool { return true }
func (t *ListFilesTool) Description() string {
	return "List a workspace directory as a bounded, alphabetically ordered tree with file sizes. recursive defaults to false; max_entries defaults to 200, maximum 2000. Ignores .git, __pycache__, .venv, and node_modules. Read-only."
}
func (t *ListFilesTool) Parameters() any {
	return jsonSchema(map[string]any{
		"path":        map[string]any{"type": "string"},
		"recursive":   map[string]any{"type": "boolean"},
		"max_entries": map[string]any{"type": "integer", "minimum": 1, "maximum": MaxListEntries, "default": 200},
	}, nil)
}
func (t *ListFilesTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Path       string `json:"path"`
		Recursive  bool   `json:"recursive"`
		MaxEntries int    `json:"max_entries"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", err
	}
	limit, err := searchLimit(a.MaxEntries, 200, MaxListEntries)
	if err != nil {
		return "", err
	}
	base, err := workspaceDirectory(t.Root, a.Path)
	if err != nil {
		return "", err
	}
	var entries []string
	err = walkWorkspace(ctx, t.Root, base, func(full, rel string, entry fs.DirEntry) error {
		local, _ := filepath.Rel(base, full)
		if !a.Recursive && filepath.Dir(local) != "." {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if len(entries) >= limit {
			entries = append(entries, "[entry limit reached]")
			return filepath.SkipAll
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		indent := strings.Repeat("  ", strings.Count(filepath.ToSlash(local), "/"))
		if entry.IsDir() {
			entries = append(entries, indent+rel+"/")
			if !a.Recursive {
				return filepath.SkipDir
			}
		} else {
			entries = append(entries, fmt.Sprintf("%s%s (%d bytes)", indent, rel, info.Size()))
		}
		return nil
	})
	return strings.Join(entries, "\n"), err
}

type GlobFilesTool struct{ Root string }

func (t *GlobFilesTool) Name() string   { return "glob_files" }
func (t *GlobFilesTool) ReadOnly() bool { return true }
func (t *GlobFilesTool) Description() string {
	return "Find files below path (default workspace root). Patterns support ** for zero or more directories, *, ?, and [abc]. Results are workspace-relative; max_matches defaults to 200, maximum 1000. Workspace ignores apply; directory symlinks are not traversed. Read-only."
}
func (t *GlobFilesTool) Parameters() any {
	return jsonSchema(map[string]any{
		"pattern":     map[string]any{"type": "string"},
		"path":        map[string]any{"type": "string"},
		"max_matches": map[string]any{"type": "integer", "minimum": 1, "maximum": MaxGlobMatches, "default": 200},
	}, []string{"pattern"})
}
func (t *GlobFilesTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Pattern    string `json:"pattern"`
		Path       string `json:"path"`
		MaxMatches int    `json:"max_matches"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", err
	}
	if err := validateGlob(a.Pattern); err != nil {
		return "", err
	}
	limit, err := searchLimit(a.MaxMatches, 200, MaxGlobMatches)
	if err != nil {
		return "", err
	}
	base, err := workspaceDirectory(t.Root, a.Path)
	if err != nil {
		return "", err
	}
	var matches []string
	err = walkWorkspace(ctx, t.Root, base, func(full, rel string, entry fs.DirEntry) error {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		local, _ := filepath.Rel(base, full)
		if matchGlob(a.Pattern, filepath.ToSlash(local)) {
			if len(matches) >= limit {
				matches = append(matches, "[match limit reached]")
				return filepath.SkipAll
			}
			matches = append(matches, rel)
		}
		return nil
	})
	return strings.Join(matches, "\n"), err
}

func validateGlob(pattern string) error {
	if strings.TrimSpace(pattern) == "" {
		return fmt.Errorf("glob pattern is required")
	}
	for _, part := range strings.Split(filepath.ToSlash(pattern), "/") {
		if _, err := path.Match(part, ""); err != nil {
			return fmt.Errorf("invalid glob %q: %w", pattern, err)
		}
	}
	return nil
}

func matchGlob(pattern, name string) bool {
	pattern, name = filepath.ToSlash(pattern), filepath.ToSlash(name)
	if !strings.Contains(pattern, "/") && pattern != "**" {
		ok, _ := path.Match(pattern, path.Base(name))
		return ok
	}
	var match func([]string, []string) bool
	match = func(pattern, parts []string) bool {
		if len(pattern) == 0 {
			return len(parts) == 0
		}
		if pattern[0] == "**" {
			for index := 0; index <= len(parts); index++ {
				if match(pattern[1:], parts[index:]) {
					return true
				}
			}
			return false
		}
		if len(parts) == 0 {
			return false
		}
		ok, _ := path.Match(pattern[0], parts[0])
		return ok && match(pattern[1:], parts[1:])
	}
	return match(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

type GrepTextTool struct{ Root string }

func (t *GrepTextTool) Name() string   { return "grep_text" }
func (t *GrepTextTool) ReadOnly() bool { return true }
func (t *GrepTextTool) Description() string {
	return "Search workspace files with a Go regular expression (RE2 syntax), returning path:line:text. include restricts a filename or path glob. case_sensitive defaults to true. max_matches defaults to 100, maximum 1000. Ignores workspace exclusions, symlinks, binary files, and files over 10MB. Read-only."
}
func (t *GrepTextTool) Parameters() any {
	return jsonSchema(map[string]any{
		"query":          map[string]any{"type": "string"},
		"path":           map[string]any{"type": "string"},
		"include":        map[string]any{"type": "string"},
		"case_sensitive": map[string]any{"type": "boolean", "default": true},
		"max_matches":    map[string]any{"type": "integer", "minimum": 1, "maximum": MaxGrepMatches, "default": 100},
	}, []string{"query"})
}
func (t *GrepTextTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Query         string `json:"query"`
		Path          string `json:"path"`
		Include       string `json:"include"`
		CaseSensitive *bool  `json:"case_sensitive"`
		MaxMatches    int    `json:"max_matches"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", err
	}
	if a.Include != "" {
		if err := validateGlob(a.Include); err != nil {
			return "", err
		}
	}
	limit, err := searchLimit(a.MaxMatches, 100, MaxGrepMatches)
	if err != nil {
		return "", err
	}
	pattern := a.Query
	if a.CaseSensitive != nil && !*a.CaseSensitive {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("invalid query: %w", err)
	}
	base, err := workspaceDirectory(t.Root, a.Path)
	if err != nil {
		return "", err
	}
	var results []string
	err = walkWorkspace(ctx, t.Root, base, func(full, rel string, entry fs.DirEntry) error {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if a.Include != "" && !matchGlob(a.Include, rel) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > 10<<20 {
			return nil
		}
		file, err := os.Open(full)
		if err != nil {
			return err
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, (10<<20)+1))
		if err != nil {
			return err
		}
		if len(data) > 10<<20 || bytes.IndexByte(data[:min(len(data), 8192)], 0) >= 0 {
			return nil
		}
		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Buffer(make([]byte, 4096), (10<<20)+1)
		line := 0
		for scanner.Scan() {
			if err := ctx.Err(); err != nil {
				return err
			}
			line++
			if re.MatchString(scanner.Text()) {
				if len(results) >= limit {
					results = append(results, "[match limit reached]")
					return filepath.SkipAll
				}
				results = append(results, fmt.Sprintf("%s:%d:%s", rel, line, scanner.Text()))
			}
		}
		return scanner.Err()
	})
	return strings.Join(results, "\n"), err
}

func searchLimit(value, fallback, maximum int) (int, error) {
	if value == 0 {
		return fallback, nil
	}
	if value < 1 || value > maximum {
		return 0, fmt.Errorf("limit must be 1..%d", maximum)
	}
	return value, nil
}

func workspaceDirectory(root, subdir string) (string, error) {
	if strings.TrimSpace(subdir) == "" {
		subdir = "."
	}
	full, err := secureJoin(resolveRoot(root), subdir)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", subdir)
	}
	return full, nil
}

func walkWorkspace(ctx context.Context, root, base string, visit func(string, string, fs.DirEntry) error) error {
	return filepath.WalkDir(base, func(full string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(resolveRoot(root), full)
		if err != nil {
			return err
		}
		if isIgnored(rel) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if full == base {
			return nil
		}
		return visit(full, filepath.ToSlash(rel), entry)
	})
}
