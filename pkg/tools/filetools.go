package tools

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"excelsior/pkg/util"
)

// Resource limits.
const (
	MaxFileReadSize = 5 << 20 // 5 MB
	MaxReadLines    = 2000
	MaxListEntries  = 2000
	MaxGrepMatches  = 1000
	MaxGlobMatches  = 1000
)

type fileRead struct {
	Full bool
	Hash string
}

type FileState struct {
	mu    sync.Mutex
	reads map[string]fileRead
}

func NewFileState() *FileState { return &FileState{reads: map[string]fileRead{}} }

func (s *FileState) Record(path, content string, full bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reads == nil {
		s.reads = make(map[string]fileRead)
	}
	hash := contentHash(content)
	previous := s.reads[path]
	s.reads[path] = fileRead{Full: full || (previous.Full && previous.Hash == hash), Hash: hash}
}

func (s *FileState) Get(path string) (fileRead, bool) {
	if s == nil {
		return fileRead{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.reads[path]
	return r, ok
}

func contentHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func isIgnored(rel string) bool {
	for _, segment := range strings.Split(rel, string(os.PathSeparator)) {
		switch segment {
		case ".git", "__pycache__", ".venv", "node_modules":
			return true
		}
	}
	return false
}

type ReadFileTool struct {
	Root  string
	State *FileState
}

func (t *ReadFileTool) Name() string   { return "read_file" }
func (t *ReadFileTool) ReadOnly() bool { return true }
func (t *ReadFileTool) Description() string {
	return "Read UTF-8 text with line numbers, range, and full_read status. offset is 1-based; limit defaults to 2000. Output is bounded to 24000 characters and files to 5MB. Only a complete delivered read enables edit_file or overwrite; empty files qualify. Sensitive files require permission."
}
func (t *ReadFileTool) Parameters() any {
	return jsonSchema(map[string]any{
		"path":   map[string]any{"type": "string"},
		"offset": map[string]any{"type": "integer", "description": "1-based starting line (default 1)"},
		"limit":  map[string]any{"type": "integer", "description": "max lines (default 2000)"},
	}, []string{"path"})
}
func (t *ReadFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Path   string `json:"path"`
		Offset *int   `json:"offset"`
		Limit  *int   `json:"limit"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", errf("read_file", "validate", "", fmt.Errorf("%w: %v", ErrInvalidArguments, err))
	}
	p, err := secureJoin(t.Root, a.Path)
	if err != nil {
		return "", err
	}
	name := strings.ToLower(filepath.Base(p))
	if name == ".env" || strings.HasPrefix(name, ".env.") || strings.Contains(name, "secret") || name == "id_rsa" || name == "id_ed25519" || strings.HasSuffix(name, ".pem") {
		if err := checkPermission(ctx, "read_file", PermissionRequest{Tool: "read_file", FilePath: a.Path}); err != nil {
			return "", err
		}
	}
	f, err := os.Open(p)
	if err != nil {
		return "", errf("read_file", "read", a.Path, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", errf("read_file", "read", a.Path, fmt.Errorf("path is a directory (use list_files)"))
	}
	if info.Size() > MaxFileReadSize {
		return "", errf("read_file", "read", a.Path, fmt.Errorf("%w: %d > %d", ErrFileTooLarge, info.Size(), MaxFileReadSize))
	}
	b, err := io.ReadAll(io.LimitReader(f, MaxFileReadSize+1))
	if err != nil {
		return "", errf("read_file", "read", a.Path, err)
	}
	if len(b) > MaxFileReadSize {
		return "", errf("read_file", "read", a.Path, ErrFileTooLarge)
	}
	if bytes.IndexByte(b[:min(len(b), 8192)], 0) >= 0 {
		return "", errf("read_file", "read", a.Path, fmt.Errorf("binary file"))
	}
	text := string(b)
	if !utf8.Valid(b) {
		return "", errf("read_file", "read", a.Path, fmt.Errorf("file is not UTF-8 text"))
	}
	lines := strings.Split(text, "\n")
	if text == "" {
		lines = []string{}
	}
	offset := 1
	if a.Offset != nil {
		if *a.Offset < 1 {
			return "", errf("read_file", "validate", a.Path, fmt.Errorf("offset must be at least 1"))
		}
		offset = *a.Offset
	}
	limit := MaxReadLines
	if a.Limit != nil {
		if *a.Limit < 1 {
			return "", errf("read_file", "validate", a.Path, fmt.Errorf("limit must be positive"))
		}
		limit = *a.Limit
	}
	if offset > max(1, len(lines)) {
		return "", errf("read_file", "read", a.Path, fmt.Errorf("offset past EOF"))
	}
	end := offset - 1 + min(limit, len(lines)-(offset-1))
	var sb strings.Builder
	used := utf8.RuneCountInString(a.Path) + 200 // room for range header and continuation notice
	for i := offset - 1; i < end; i++ {
		line := fmt.Sprintf("%d: %s\n", i+1, lines[i])
		cost := utf8.RuneCountInString(line)
		if used+cost > MaxObservationChars {
			end = i
			if i == offset-1 {
				t.State.Record(p, text, false)
				return "", errf("read_file", "read", a.Path, fmt.Errorf("line exceeds observation limit; use bash for bounded inspection"))
			}
			break
		}
		sb.WriteString(line)
		used += cost
	}
	full := offset == 1 && end >= len(lines)
	t.State.Record(p, text, full)
	result := fmt.Sprintf("file: %s lines=%d range=%d..%d full_read=%t\n", a.Path, len(lines), offset, end, full) + sb.String()
	if end < len(lines) {
		result += fmt.Sprintf("[more lines available; continue at offset=%d]\n", end+1)
	}
	return result, nil
}

type WriteFileTool struct {
	Root  string
	State *FileState
}

func (t *WriteFileTool) Name() string { return "write_file" }
func (t *WriteFileTool) Description() string {
	return "Create a file or replace it entirely, returning a unified diff and diagnostics. Existing files require overwrite=true and a full recorded read. Rejects external changes since the read. Shows the diff for permission before writing and updates read state after success. Prefer edit_file for targeted changes."
}
func (t *WriteFileTool) Parameters() any {
	return jsonSchema(map[string]any{
		"path":      map[string]any{"type": "string"},
		"content":   map[string]any{"type": "string"},
		"overwrite": map[string]any{"type": "boolean"},
	}, []string{"path", "content"})
}
func (t *WriteFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Path      string `json:"path"`
		Content   string `json:"content"`
		Overwrite *bool  `json:"overwrite"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", errf("write_file", "validate", "", fmt.Errorf("%w: %v", ErrInvalidArguments, err))
	}
	if len(a.Content) > MaxWriteSize {
		return "", errf("write_file", "validate", a.Path, fmt.Errorf("%w: content too large", ErrFileTooLarge))
	}
	p, err := secureJoin(t.Root, a.Path)
	if err != nil {
		return "", err
	}
	overwrite := false
	if a.Overwrite != nil {
		overwrite = *a.Overwrite
	}
	_, statErr := os.Stat(p)
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return "", errf("write_file", "stat", a.Path, statErr)
	}
	exists := statErr == nil
	before := ""
	if exists && !overwrite {
		return "", errf("write_file", "write", a.Path, fmt.Errorf("file exists; pass overwrite=true"))
	}
	if exists {
		rec, ok := t.State.Get(p)
		if !ok || !rec.Full {
			return "", errf("write_file", "write", a.Path, fmt.Errorf("file must be fully read before overwrite"))
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return "", errf("write_file", "read", a.Path, err)
		}
		if contentHash(string(b)) != rec.Hash {
			return "", errf("write_file", "write", a.Path, fmt.Errorf("file modified externally since read"))
		}
		before = string(b)
	}
	diff := fileDiff(a.Path, before, a.Content)
	if err := checkPermission(ctx, "write", PermissionRequest{Tool: "write", FilePath: a.Path, Preview: util.Truncate(diff, MaxObservationChars)}); err != nil {
		return "", err
	}
	if err := ensureUnchanged(t.Root, a.Path, before, exists); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", errf("write_file", "write", a.Path, err)
	}
	mode := os.FileMode(0o644)
	if exists {
		mode = recMode(p)
	}
	if err := util.WriteAtomic(p, []byte(a.Content), mode); err != nil {
		return "", errf("write_file", "write", a.Path, err)
	}
	t.State.Record(p, a.Content, true)
	result := fmt.Sprintf("Wrote %s\n%s", a.Path, diff)
	if diag := postEditDiagnostics(ctx, t.Root, a.Path); diag != "" {
		result += "\n" + diag
	}
	return result, nil
}

type EditFileTool struct {
	Root  string
	State *FileState
}

func (t *EditFileTool) Name() string { return "edit_file" }
func (t *EditFileTool) Description() string {
	return "Replace exact nonempty old_text in an existing UTF-8 file. Requires a full recorded read; rejects external changes. Multiple matches require replace_all=true; unchanged text is rejected. Shows and returns a unified diff, updates read state, and runs diagnostics."
}
func (t *EditFileTool) Parameters() any {
	return jsonSchema(map[string]any{
		"path":        map[string]any{"type": "string"},
		"old_text":    map[string]any{"type": "string"},
		"new_text":    map[string]any{"type": "string"},
		"replace_all": map[string]any{"type": "boolean"},
	}, []string{"path", "old_text", "new_text"})
}
func (t *EditFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Path       string `json:"path"`
		OldText    string `json:"old_text"`
		NewText    string `json:"new_text"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", errf("edit_file", "validate", "", fmt.Errorf("%w: %v", ErrInvalidArguments, err))
	}
	if a.OldText == a.NewText {
		return "", errf("edit_file", "validate", a.Path, fmt.Errorf("%w: no-op edit", ErrInvalidArguments))
	}
	if a.OldText == "" {
		return "", errf("edit_file", "validate", a.Path, fmt.Errorf("old_text must not be empty"))
	}
	p, err := secureJoin(t.Root, a.Path)
	if err != nil {
		return "", err
	}
	rec, ok := t.State.Get(p)
	if !ok || !rec.Full {
		return "", errf("edit_file", "edit", a.Path, fmt.Errorf("file must be fully read before edit"))
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", errf("edit_file", "read", a.Path, err)
	}
	if contentHash(string(b)) != rec.Hash {
		return "", errf("edit_file", "edit", a.Path, fmt.Errorf("file modified externally since read"))
	}
	content := string(b)
	count := strings.Count(content, a.OldText)
	if count == 0 {
		return "", errf("edit_file", "edit", a.Path, ErrTextNotFound)
	}
	if count > 1 && !a.ReplaceAll {
		return "", errf("edit_file", "edit", a.Path, fmt.Errorf("%w: %d matches", ErrAmbiguousMatch, count))
	}
	newContent := content
	if a.ReplaceAll {
		newContent = strings.ReplaceAll(content, a.OldText, a.NewText)
	} else {
		newContent = strings.Replace(content, a.OldText, a.NewText, 1)
	}
	if len(newContent) > MaxWriteSize {
		return "", errf("edit_file", "edit", a.Path, fmt.Errorf("%w: result too large", ErrFileTooLarge))
	}
	diff := fileDiff(a.Path, content, newContent)
	if err := checkPermission(ctx, "edit", PermissionRequest{Tool: "edit", FilePath: a.Path, Preview: util.Truncate(diff, MaxObservationChars)}); err != nil {
		return "", err
	}
	if err := ensureUnchanged(t.Root, a.Path, content, true); err != nil {
		return "", err
	}
	if err := util.WriteAtomic(p, []byte(newContent), recMode(p)); err != nil {
		return "", errf("edit_file", "write", a.Path, err)
	}
	t.State.Record(p, newContent, true)
	result := fmt.Sprintf("Edited %s\n%s", a.Path, diff)
	if diag := postEditDiagnostics(ctx, t.Root, a.Path); diag != "" {
		result += "\n" + diag
	}
	return result, nil
}

// Recheck after a permission wait, since the user may have edited the file.
func ensureUnchanged(root, relative, before string, existed bool) error {
	p, err := secureJoin(root, relative)
	if err != nil {
		return err
	}
	current, err := os.ReadFile(p)
	if !existed && errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !existed || string(current) != before {
		return fmt.Errorf("file changed while awaiting permission; read it again")
	}
	return nil
}

func fileDiff(name, before, after string) string {
	if before == after {
		return "(no content change)"
	}
	lines := func(text string) []string {
		if text == "" {
			return nil
		}
		result := strings.SplitAfter(text, "\n")
		if result[len(result)-1] == "" {
			result = result[:len(result)-1]
		}
		return result
	}
	old, next := lines(before), lines(after)
	start := 0
	for start < len(old) && start < len(next) && old[start] == next[start] {
		start++
	}
	endOld, endNext := len(old), len(next)
	for endOld > start && endNext > start && old[endOld-1] == next[endNext-1] {
		endOld--
		endNext--
	}
	contextStart := max(0, start-3)
	tail := min(3, len(old)-endOld)
	oldCount, newCount := endOld+tail-contextStart, endNext+tail-contextStart
	oldStart, newStart := contextStart+1, contextStart+1
	if oldCount == 0 {
		oldStart = contextStart
	}
	if newCount == 0 {
		newStart = contextStart
	}
	var diff strings.Builder
	fmt.Fprintf(&diff, "--- a/%s\n+++ b/%s\n@@ -%d,%d +%d,%d @@\n", name, name, oldStart, oldCount, newStart, newCount)
	write := func(prefix string, entries []string) {
		for _, line := range entries {
			diff.WriteString(prefix + line)
			if !strings.HasSuffix(line, "\n") {
				diff.WriteString("\n\\ No newline at end of file\n")
			}
		}
	}
	write(" ", old[contextStart:start])
	write("-", old[start:endOld])
	write("+", next[start:endNext])
	write(" ", old[endOld:endOld+tail])
	return diff.String()
}

func recMode(p string) os.FileMode {
	info, err := os.Stat(p)
	if err == nil {
		return info.Mode().Perm()
	}
	return 0o644
}

func postEditDiagnostics(parent context.Context, root, filePath string) string {
	full, err := secureJoin(root, filePath)
	if err != nil {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(filePath))
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	if ext == ".py" {
		for _, candidate := range []struct {
			name string
			args []string
		}{{"ruff", []string{"check", "--output-format", "concise", full}}, {"pyflakes", []string{full}}} {
			if _, err := exec.LookPath(candidate.name); err != nil {
				continue
			}
			return runDiagnostic(ctx, candidate.name, candidate.args...)
		}
		for _, python := range []string{"python", "python3", "py"} {
			if _, err := exec.LookPath(python); err != nil {
				continue
			}
			return runDiagnostic(ctx, python, "-c", "import ast,pathlib,sys; p=pathlib.Path(sys.argv[1]); ast.parse(p.read_text(encoding='utf-8'), filename=str(p))", full)
		}
		return "diagnostics unavailable: install ruff or pyflakes, or make Python available for syntax checking"
	}
	if ext == ".go" {
		return runDiagnostic(ctx, "gofmt", "-l", full)
	}
	return ""
}

func runDiagnostic(ctx context.Context, name string, args ...string) string {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if ctx.Err() != nil {
		return "diagnostics timed out after 5 seconds"
	}
	if err != nil && text == "" {
		text = err.Error()
	}
	if text != "" {
		return "diagnostics:\n" + text
	}
	return ""
}
