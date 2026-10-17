package app

import (
	"log/slog"

	"excelsior/pkg/agent"
	"excelsior/pkg/config"
	"excelsior/pkg/llm"
	"excelsior/pkg/tools"
)

// NewAgent is the shared composition point for CLI and engine runs.
func NewAgent(cfg config.Config, workspace, model, system string, logger *slog.Logger) *agent.Agent {
	if logger == nil {
		logger = slog.Default()
	}
	harness := agent.DefaultHarnessConfig()
	if cfg.Harness != nil {
		harness.ContextWindowTokens = cfg.Harness.ContextWindowTokens
		harness.ContextTier = agent.ContextTier(cfg.Harness.ContextTier)
		harness.Planning = cfg.Harness.Planning
		harness.ActionSpace = agent.ActionSpace(cfg.Harness.ActionSpace)
	}
	if cfg.MaxTokens > 0 {
		harness.MaxOutputTokens = cfg.MaxTokens
	}
	harness.Temperature = cfg.Temperature
	if cfg.TopP > 0 {
		harness.TopP = cfg.TopP
	}
	return &agent.Agent{
		LLM: &llm.Client{
			APIKey:        cfg.APIKey,
			BaseURL:       cfg.BaseURL,
			Model:         model,
			TokenEncoding: cfg.TokenEncoding,
		},
		Tools:   tools.WorkspaceTools(workspace),
		System:  system,
		Logger:  logger,
		Harness: &harness,
	}
}
