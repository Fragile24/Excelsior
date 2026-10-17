package permissions

import (
	"regexp"
	"strings"

	"excelsior/pkg/config"
	"excelsior/pkg/tools"
)

// Resolve applies workspace settings and explicit runtime overrides.
// Allow and deny runtime modes are authoritative; ask leaves workspace settings active.
func Resolve(override config.PermissionMode, settings config.Settings) config.PermissionMode {
	mode := settings.EffectivePermission(config.PermissionAsk)
	if override == config.PermissionAllow || override == config.PermissionDeny {
		mode = override
	}
	if mode == "" {
		mode = config.PermissionAsk
	}
	return mode
}

var destructiveShell = regexp.MustCompile(`(?i)(^|[\s;&|])(sudo\b|rm\s+-[a-z]*r[a-z]*f|rm\s+-[a-z]*f[a-z]*r|git\s+(push\b|reset\s+--hard\b)|chmod\s+-R\b|chown\s+-R\b|remove-item\b[^;\r\n]*-recurse\b)`)

// DefaultDecision supplies the paper's allow/ask/deny substrate in ask mode.
// Complex shell syntax still goes to the existing approval handler. This is a
// command policy, not an OS sandbox; explicit allow/deny modes remain authoritative.
func DefaultDecision(request tools.PermissionRequest) config.PermissionMode {
	if request.Tool != "bash" {
		return config.PermissionAsk
	}
	command := strings.TrimSpace(request.Command)
	if destructiveShell.MatchString(command) {
		return config.PermissionDeny
	}
	if strings.ContainsAny(command, ";&|`$<>()'\"\\\r\n") {
		return config.PermissionAsk
	}
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return config.PermissionAsk
	}
	// These options can turn an otherwise read-only command into execution or writes.
	for _, argument := range parts[1:] {
		for _, forbidden := range []string{"--pre", "--hostname-bin", "--search-zip", "--ext-diff", "--textconv", "--output", "--exec", "--paginate"} {
			if strings.HasPrefix(argument, forbidden) {
				return config.PermissionAsk
			}
		}
		if strings.HasPrefix(argument, "-") && !strings.HasPrefix(argument, "--") && strings.Contains(argument, "z") {
			return config.PermissionAsk
		}
	}
	switch parts[0] {
	case "ls", "pwd", "rg", "cat", "head", "tail", "wc":
		return config.PermissionAllow
	case "file":
		for _, argument := range parts[1:] {
			if argument == "--compile" || (strings.HasPrefix(argument, "-") && strings.Contains(argument, "C")) {
				return config.PermissionAsk
			}
		}
		return config.PermissionAllow
	case "git":
		if len(parts) < 2 {
			break
		}
		switch parts[1] {
		case "status", "diff", "log", "show":
			return config.PermissionAllow
		case "branch":
			for _, argument := range parts[2:] {
				switch argument {
				case "--list", "--all", "--remotes", "--show-current", "-a", "-r", "-v", "-vv":
				default:
					return config.PermissionAsk
				}
			}
			return config.PermissionAllow
		}
	}
	return config.PermissionAsk
}
