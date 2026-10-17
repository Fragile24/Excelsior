package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

// BashTool executes a shell command in the workspace directory.
// On Windows it uses PowerShell, elsewhere sh. Both output streams are captured.
type BashTool struct{ Root string }

const maxShellOutput = 100_000

// shellOutput keeps draining a stream after the capture limit is reached.
type shellOutput struct {
	buf       bytes.Buffer
	truncated bool
}

func (b *shellOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := maxShellOutput - b.buf.Len()
	if len(p) > remaining {
		p = p[:remaining]
		b.truncated = true
	}
	_, _ = b.buf.Write(p)
	return n, nil
}

func (b *shellOutput) String() string {
	out := b.buf.String()
	if b.truncated {
		out += "\n[truncated]"
	}
	return out
}

func (t *BashTool) Name() string { return "bash" }
func (t *BashTool) Description() string {
	return "Run a command through /bin/sh -c (PowerShell on Windows). Returns exit_code, stdout, stderr, and timed_out; nonzero exits and timeouts are errors with captured output. cwd must be workspace-relative (default root). timeout_seconds defaults to 120, maximum 600. Shell changes bypass file-state tracking and automatic diagnostics."
}
func (t *BashTool) Parameters() any {
	return jsonSchema(map[string]any{
		"command":         map[string]any{"type": "string", "description": "Shell command"},
		"timeout_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 600, "default": 120},
		"cwd":             map[string]any{"type": "string", "description": "Working directory relative to workspace root"},
	}, []string{"command"})
}
func (t *BashTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", errf("bash", "exec", "", err)
	}
	var a struct {
		Command string `json:"command"`
		Timeout int    `json:"timeout_seconds"`
		CWD     string `json:"cwd"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", errf("bash", "validate", "", fmt.Errorf("%w: %v", ErrInvalidArguments, err))
	}
	a.Command = strings.TrimSpace(a.Command)
	if a.Command == "" {
		return "", errf("bash", "validate", "", fmt.Errorf("%w: command is required", ErrInvalidArguments))
	}
	if len(a.Command) > MaxCommandLength {
		return "", errf("bash", "validate", "", fmt.Errorf("%w: length %d exceeds max %d", ErrCommandTooLong, len(a.Command), MaxCommandLength))
	}
	if a.Timeout == 0 {
		a.Timeout = 120
	}
	if a.Timeout < 1 || a.Timeout > 600 {
		return "", errf("bash", "validate", "", fmt.Errorf("%w: timeout_seconds must be 1..600", ErrInvalidArguments))
	}
	dir, err := workspaceDirectory(t.Root, a.CWD)
	if err != nil {
		return "", errf("bash", "cwd", a.CWD, err)
	}
	if err := checkPermission(ctx, "bash", PermissionRequest{Tool: "bash", Command: a.Command}); err != nil {
		return "", err
	}
	// A permission wait can outlive a directory or symlink change.
	dir, err = workspaceDirectory(t.Root, a.CWD)
	if err != nil {
		return "", errf("bash", "cwd", a.CWD, err)
	}
	slog.Info("bash", "command", a.Command, "dir", dir)
	return runShell(ctx, dir, a.Command, time.Duration(a.Timeout)*time.Second)
}

func runShell(ctx context.Context, dir, command string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd, start, cleanup, err := shellCommand(ctx, command)
	if err != nil {
		return "", err
	}
	defer cleanup()
	cmd.WaitDelay = time.Second
	cmd.Dir = dir
	var stdout, stderr shellOutput
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := start(); err != nil {
		return "", err
	}
	waitErr := cmd.Wait()
	exitCode := 0
	if waitErr != nil {
		exitCode = -1
		if exit, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exit.ExitCode()
		}
	}
	encoded, _ := json.Marshal(struct {
		ExitCode int    `json:"exit_code"`
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
		TimedOut bool   `json:"timed_out"`
	}{exitCode, stdout.String(), stderr.String(), ctx.Err() == context.DeadlineExceeded})
	if ctx.Err() != nil {
		return string(encoded), ctx.Err()
	}
	return string(encoded), waitErr
}
