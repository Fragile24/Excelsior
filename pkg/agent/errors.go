package agent

import "errors"

var (
	ErrEmptyMessages    = errors.New("messages must contain a user message")
	ErrLLMNotConfigured = errors.New("LLM provider not configured")
	ErrNilLLMMessage    = errors.New("LLM provider returned nil message")
	ErrContextBudget    = errors.New("context budget exceeded")
	ErrStepBudget       = errors.New("agent step budget exhausted")
	ErrStuck            = errors.New("stuck detection")
)
