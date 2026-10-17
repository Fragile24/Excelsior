package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// DefaultModel is used when DEEPSEEK_MODEL is unset.
	DefaultModel = "deepseek-v4-flash"
	// DefaultBaseURL is the DeepSeek API base URL.
	DefaultBaseURL = "https://api.deepseek.com"
)

// PermissionMode controls mutating tool approval.
type PermissionMode string

const (
	PermissionAsk   PermissionMode = "ask"
	PermissionAllow PermissionMode = "allow"
	PermissionDeny  PermissionMode = "deny"
)

// ParsePermissionMode validates a permission mode string (case-insensitive).
func ParsePermissionMode(s string) (PermissionMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "ask":
		return PermissionAsk, nil
	case "allow":
		return PermissionAllow, nil
	case "deny":
		return PermissionDeny, nil
	default:
		return "", fmt.Errorf("config Permission: invalid permission mode %q: must be ask|allow|deny: %w", s, ErrInvalidPermission)
	}
}

// Config holds DeepSeek-first settings. Env vars are the source of truth;
// flags override them. Permission is NOT here — it lives in Settings
// (workspace settings.json, env-seeded) with CLI flags as a runtime override.
type Config struct {
	APIKey        string
	BaseURL       string
	Model         string
	MaxTokens     int
	Temperature   float64
	TopP          float64
	TokenEncoding string
	Workspace     string
	Harness       *HarnessSettings
}

// HarnessSettings groups the three independently configurable components.
// A nil pointer selects the paper's default profile; a non-nil value is exact,
// including ContextTier 0 and Planning false.
type HarnessSettings struct {
	ContextWindowTokens int
	ContextTier         int
	Planning            bool
	ActionSpace         string
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// FromEnv reads configuration from environment variables. Its defaults follow
// the paper's T4/128k profile, temperature 0, top-p .95, and 16,384 output tokens.
func FromEnv() (Config, error) {
	tier, err := envInt("EXCELSIOR_CONTEXT_TIER", 4)
	if err != nil {
		return Config{}, err
	}
	window, err := envInt("EXCELSIOR_CONTEXT_WINDOW", 128000)
	if err != nil {
		return Config{}, err
	}
	planning, err := envBool("EXCELSIOR_PLANNING", true)
	if err != nil {
		return Config{}, err
	}
	maxTokens, err := envInt("EXCELSIOR_MAX_OUTPUT_TOKENS", 16384)
	if err != nil {
		return Config{}, err
	}
	temperature, err := envFloat("EXCELSIOR_TEMPERATURE", 0)
	if err != nil {
		return Config{}, err
	}
	topP, err := envFloat("EXCELSIOR_TOP_P", 0.95)
	if err != nil {
		return Config{}, err
	}
	actionSpace := strings.TrimSpace(os.Getenv("EXCELSIOR_ACTION_SPACE"))
	if actionSpace == "" {
		actionSpace = "predefined"
	}
	return Config{
		APIKey:        strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY")),
		BaseURL:       envOr("DEEPSEEK_BASE_URL", DefaultBaseURL),
		Model:         envOr("DEEPSEEK_MODEL", DefaultModel),
		MaxTokens:     maxTokens,
		Temperature:   temperature,
		TopP:          topP,
		TokenEncoding: strings.TrimSpace(os.Getenv("EXCELSIOR_TOKEN_ENCODING")),
		Workspace:     strings.TrimSpace(os.Getenv("EXCELSIOR_WORKSPACE")),
		Harness: &HarnessSettings{
			ContextWindowTokens: window,
			ContextTier:         tier,
			Planning:            planning,
			ActionSpace:         actionSpace,
		},
	}, nil
}

func envInt(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("config %s: expected an integer: %w", name, err)
	}
	return parsed, nil
}

func envFloat(name string, fallback float64) (float64, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("config %s: expected a number: %w", name, err)
	}
	return parsed, nil
}

func envBool(name string, fallback bool) (bool, error) {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if value == "" {
		return fallback, nil
	}
	switch value {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("config %s: expected true or false", name)
	}
}

// Validate returns error if config is invalid for production use.
func (c Config) Validate() error {
	if strings.TrimSpace(c.APIKey) == "" {
		return fmt.Errorf("config APIKey: DEEPSEEK_API_KEY is required: %w", ErrMissingAPIKey)
	}
	if strings.TrimSpace(c.Model) == "" {
		return fmt.Errorf("config Model: model is required: %w", ErrMissingModel)
	}
	base := strings.TrimSpace(c.BaseURL)
	u, err := url.Parse(base)
	if err != nil {
		return fmt.Errorf("config BaseURL: invalid BaseURL %q: %w", c.BaseURL, ErrInvalidBaseURL)
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("config BaseURL: invalid BaseURL %q: scheme and host required: %w", c.BaseURL, ErrInvalidBaseURL)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("config BaseURL: scheme must be https or http, got %q: %w", u.Scheme, ErrInvalidBaseURL)
	}
	if c.Temperature < 0 || c.Temperature > 2 {
		return fmt.Errorf("config Temperature: must be 0..2, got %v: %w", c.Temperature, ErrInvalidTemperature)
	}
	if c.TopP < 0 || c.TopP > 1 {
		return fmt.Errorf("config TopP: must be 0..1, got %v", c.TopP)
	}
	if c.MaxTokens < 0 {
		return fmt.Errorf("config MaxTokens: must be positive")
	}
	if c.TokenEncoding != "" && c.TokenEncoding != "deepseek-v4" && c.TokenEncoding != "deepseek-v4.1" {
		return fmt.Errorf("config TokenEncoding: use deepseek-v4 or deepseek-v4.1")
	}
	window, output := 128000, c.MaxTokens
	if c.Harness != nil {
		window = c.Harness.ContextWindowTokens
	}
	if output == 0 {
		output = 16384
	}
	if window <= output {
		return fmt.Errorf("config: output limit %d must be smaller than total context window %d", output, window)
	}
	if c.Harness != nil {
		if c.Harness.ContextWindowTokens <= 0 {
			return fmt.Errorf("config ContextWindowTokens: must be positive")
		}
		if c.Harness.ContextTier < 0 || c.Harness.ContextTier > 4 {
			return fmt.Errorf("config ContextTier: must be 0..4, got %d", c.Harness.ContextTier)
		}
		if c.Harness.ActionSpace != "" && c.Harness.ActionSpace != "predefined" && c.Harness.ActionSpace != "bash" {
			return fmt.Errorf("config ActionSpace: must be predefined or bash, got %q", c.Harness.ActionSpace)
		}
	}
	return nil
}

// ResolveWorkspace returns absolute workspace path, preferring flag > config > cwd.
func ResolveWorkspace(flagWS, cfgWS string) (string, error) {
	ws := strings.TrimSpace(flagWS)
	if ws == "" {
		ws = strings.TrimSpace(cfgWS)
	}
	if ws == "" {
		var err error
		ws, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("config Workspace: failed to get current working directory: %w", ErrInvalidWorkspace)
		}
	}
	if !filepath.IsAbs(ws) {
		abs, err := filepath.Abs(ws)
		if err != nil {
			return "", fmt.Errorf("config Workspace: failed to resolve absolute path %q: %w", ws, ErrInvalidWorkspace)
		}
		ws = abs
	}
	if info, err := os.Stat(ws); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("config Workspace: workspace %q: %w", ws, ErrWorkspaceNotFound)
		}
		return "", fmt.Errorf("config Workspace: workspace %q: %w", ws, ErrInvalidWorkspace)
	} else if !info.IsDir() {
		return "", fmt.Errorf("config Workspace: workspace %q is not a directory: %w", ws, ErrWorkspaceNotDir)
	}
	return ws, nil
}
