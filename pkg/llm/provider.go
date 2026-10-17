package llm

import "context"

// ChatProvider performs exactly one generation step. The agent owns tool
// execution and history management so the harness policy remains provider-
// independent and consistent across models.
type ChatProvider interface {
	ModelName() string
	// CountTokens returns the exact size of this provider's complete input prompt.
	// Unsupported models or prompt formats must return an error, not an estimate.
	CountTokens(ctx context.Context, req ChatRequest) (int, error)
	StreamChat(ctx context.Context, req ChatRequest, onDelta func(Delta) error) (*Message, *Usage, error)
}

var _ ChatProvider = (*Client)(nil)
