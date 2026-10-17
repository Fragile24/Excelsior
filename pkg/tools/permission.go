package tools

import (
	"context"
	"errors"
	"fmt"
)

// ErrPermissionDenied covers user denials and policy denials.
var ErrPermissionDenied = errors.New("permission denied")

// PermissionRequest is input to a PermissionHandler.
type PermissionRequest struct {
	Tool     string `json:"tool"`               // "write" | "edit" | "bash" | "read_file"
	FilePath string `json:"filePath,omitempty"` // for write/edit
	Preview  string `json:"preview,omitempty"`  // truncated content/command preview
	Command  string `json:"command,omitempty"`  // for bash
}

// PermissionResponse is the user's decision.
type PermissionResponse struct {
	Approved bool `json:"approved"`
}

// PermissionHandler is called by mutating tools to request user approval.
type PermissionHandler func(ctx context.Context, req PermissionRequest) (PermissionResponse, error)

type permissionHandlerKey struct{}

// WithPermissionHandler returns a context carrying h.
func WithPermissionHandler(ctx context.Context, h PermissionHandler) context.Context {
	return context.WithValue(ctx, permissionHandlerKey{}, h)
}

// GetPermissionHandler retrieves handler installed by WithPermissionHandler.
func GetPermissionHandler(ctx context.Context) (PermissionHandler, bool) {
	h, ok := ctx.Value(permissionHandlerKey{}).(PermissionHandler)
	return h, ok
}

// checkPermission invokes handler if present. If no handler, it auto-allows.
// Caller should provide mode-specific handler (e.g. allow/deny) for headless runs.
func checkPermission(ctx context.Context, tool string, req PermissionRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	h, ok := GetPermissionHandler(ctx)
	if !ok || h == nil {
		return nil
	}
	resp, err := h(ctx, req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return errf(tool, "permission", "", fmt.Errorf("%w: %v", ErrPermissionDenied, err))
		}
		return errf(tool, "permission", "", err)
	}
	if !resp.Approved {
		return errf(tool, "permission", "", ErrPermissionDenied)
	}
	return nil
}
