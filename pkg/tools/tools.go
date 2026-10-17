package tools

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
)

// Resource limits for tool execution.
const (
	MaxWriteSize        = 10 << 20 // 10 MB per write
	MaxCommandLength    = 8 << 10  // 8 KB command length cap for bash
	MaxObservationChars = 24_000
)

// Tool is the interface all agent tools implement.
type Tool interface {
	Name() string
	Description() string
	Parameters() any // JSON Schema
	Execute(ctx context.Context, args json.RawMessage) (string, error)
}

// resolveRoot normalizes workspace to an absolute path, defaulting to cwd.
func resolveRoot(workspace string) string {
	if workspace == "" {
		if wd, err := os.Getwd(); err == nil && wd != "" {
			workspace = wd
		} else {
			slog.Warn("tools: Getwd failed, using '.'", "err", err)
			return "."
		}
	}
	if !filepath.IsAbs(workspace) {
		if abs, err := filepath.Abs(workspace); err == nil {
			workspace = abs
		}
	}
	return workspace
}

// WorkspaceTools returns the core tools rooted at workspace.
func WorkspaceTools(workspace string) []Tool {
	workspace = resolveRoot(workspace)
	state := NewFileState()
	return []Tool{
		&ReadFileTool{Root: workspace, State: state},
		&WriteFileTool{Root: workspace, State: state},
		&EditFileTool{Root: workspace, State: state},
		&ListFilesTool{Root: workspace},
		&GlobFilesTool{Root: workspace},
		&GrepTextTool{Root: workspace},
		&WebFetchTool{},
		&BashTool{Root: workspace},
		&AskTool{},
	}
}

// jsonSchema helper constructs standard JSON Schema objects for tool parameter definitions.
func jsonSchema(props map[string]any, required []string) map[string]any {
	if required == nil {
		required = []string{}
	}
	return map[string]any{
		"type":       "object",
		"properties": props,
		"required":   required,
	}
}
