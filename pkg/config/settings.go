package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"excelsior/pkg/util"
)

// Settings persists user preferences like permission mode.
// Stored at <workspace>/.excelsior/settings.json and fallback to user config dir.
type Settings struct {
	Permission PermissionMode `json:"permission,omitempty"` // ask|allow|deny
}

// SettingsPath returns the primary path for workspace settings.
func SettingsPath(workspace string) string {
	if workspace != "" {
		return filepath.Join(workspace, ".excelsior", "settings.json")
	}
	// fallback to cwd
	return filepath.Join(".excelsior", "settings.json")
}

// UserSettingsPath returns the global user config path.
func UserSettingsPath() string {
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return filepath.Join(dir, "excelsior", "settings.json")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".excelsior", "settings.json")
	}
	return ""
}

// LoadSettings reads settings from workspace path and user global path, merged
// with workspace taking precedence. EXCELSIOR_PERMISSION env seeds the base
// layer (lowest precedence). Missing file returns env-only settings.
func LoadSettings(workspace string) Settings {
	var merged Settings
	// env as base layer
	if pm, err := ParsePermissionMode(os.Getenv("EXCELSIOR_PERMISSION")); err == nil {
		merged.Permission = pm
	}
	// user global
	if p := UserSettingsPath(); p != "" {
		if s, err := loadSettingsFile(p); err == nil {
			merged = s
		}
	}
	// workspace overrides
	if p := SettingsPath(workspace); p != "" {
		if s, err := loadSettingsFile(p); err == nil {
			if s.Permission != "" {
				merged.Permission = s.Permission
			}
		}
	}
	return merged
}

func loadSettingsFile(path string) (Settings, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Settings{}, err
	}
	var s Settings
	if err := json.Unmarshal(b, &s); err != nil {
		return Settings{}, err
	}
	// normalize permission
	if s.Permission != "" {
		if pm, err := ParsePermissionMode(string(s.Permission)); err == nil {
			s.Permission = pm
		} else {
			s.Permission = ""
		}
	}
	return s, nil
}

// SaveSettings persists to workspace settings path atomically.
// If workspace is empty, falls back to user global path.
func SaveSettings(workspace string, s Settings) error {
	path := SettingsPath(workspace)
	if workspace == "" {
		if p := UserSettingsPath(); p != "" {
			path = p
		}
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	// use 0600
	return util.WriteAtomic(path, b, 0o600)
}

// EffectivePermission resolves the stored permission mode or its fallback.
func (s Settings) EffectivePermission(fallback PermissionMode) PermissionMode {
	if s.Permission != "" {
		return s.Permission
	}
	return fallback
}
