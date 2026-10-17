package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/zendev-sh/goai"
	"github.com/zendev-sh/goai/provider"
	"github.com/zendev-sh/goai/provider/deepseek"
)

// Client adapts GoAI's DeepSeek provider to the one-step generation interface.
// GoAI owns HTTP, SSE parsing, retries, and provider-specific message handling.
type Client struct {
	APIKey        string
	BaseURL       string // Defaults to https://api.deepseek.com when empty.
	Model         string
	HTTPClient    *http.Client
	TokenEncoding string // Empty selects the known model's published prompt/tokenizer profile.
}

// ModelName returns the configured model name.
func (c *Client) ModelName() string { return c.Model }

func (c *Client) baseURL() string {
	const defaultURL = "https://api.deepseek.com"
	base := strings.TrimSpace(c.BaseURL)
	// GoAI also applies its environment override to an explicitly supplied
	// default URL. Counting and generation must resolve that route identically.
	if base == "" || base == defaultURL {
		if configured := strings.TrimSpace(os.Getenv("DEEPSEEK_BASE_URL")); configured != "" {
			return configured
		}
		return defaultURL
	}
	return base
}

func (c *Client) model(reqModel string) provider.LanguageModel {
	model := strings.TrimSpace(reqModel)
	opts := make([]deepseek.Option, 0, 3)
	if key := strings.TrimSpace(c.APIKey); key != "" {
		opts = append(opts, deepseek.WithAPIKey(key))
	}
	opts = append(opts, deepseek.WithBaseURL(c.baseURL()))
	if c.HTTPClient != nil {
		opts = append(opts, deepseek.WithHTTPClient(c.HTTPClient))
	}
	return budgetedModel{LanguageModel: deepseek.Chat(model, opts...)}
}

// StreamChat runs a single streaming generation step (no automatic tool loop).
// The caller executes tools and manages history/context.
func (c *Client) StreamChat(
	ctx context.Context,
	req ChatRequest,
	onDelta func(Delta) error,
) (*Message, *Usage, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if strings.TrimSpace(req.Model) == "" {
		req.Model = c.Model
	}
	if req.ReasoningEffort != "" && !req.ReasoningEffort.Valid() {
		return nil, nil, fmt.Errorf("unsupported reasoning effort %q", req.ReasoningEffort)
	}
	if req.ContextWindowTokens < 0 {
		return nil, nil, fmt.Errorf("context window must be positive")
	}
	if req.ContextWindowTokens > 0 {
		if req.MaxTokens == nil || *req.MaxTokens <= 0 {
			return nil, nil, fmt.Errorf("a budgeted request requires an explicit positive output limit")
		}
		input, err := c.CountTokens(ctx, req)
		if err != nil {
			return nil, nil, err
		}
		if *req.MaxTokens >= req.ContextWindowTokens || input > req.ContextWindowTokens-*req.MaxTokens {
			return nil, nil, fmt.Errorf("%w: %d prompt tokens plus %d output tokens exceed the %d-token context window", ErrInvalidRequest, input, *req.MaxTokens, req.ContextWindowTokens)
		}
	}

	messages, err := toProviderMessages(req.Messages)
	if err != nil {
		return nil, nil, err
	}

	goaiTools := make([]goai.Tool, 0, len(req.Tools))
	for _, definition := range req.Tools {
		schema, err := json.Marshal(definition.Function.Parameters)
		if err != nil {
			return nil, nil, fmt.Errorf("llm: marshal tool schema %q: %w", definition.Function.Name, err)
		}
		name := definition.Function.Name
		goaiTools = append(goaiTools, goai.Tool{
			Name:        name,
			Description: definition.Function.Description,
			InputSchema: schema,
		})
	}

	options := []goai.Option{
		goai.WithMessages(messages...),
		goai.WithTools(goaiTools...),
		goai.WithMaxSteps(1),
	}
	if req.ReasoningEffort != "" {
		options = append(options, goai.WithProviderOptions(map[string]any{"reasoning_effort": string(req.ReasoningEffort)}))
	}
	if req.Temperature != nil {
		options = append(options, goai.WithTemperature(*req.Temperature))
	}
	if req.MaxTokens != nil {
		options = append(options, goai.WithMaxOutputTokens(*req.MaxTokens))
	}
	if req.TopP != nil {
		options = append(options, goai.WithTopP(*req.TopP))
	}

	stream, err := goai.StreamText(ctx, c.model(req.Model), options...)
	if err != nil {
		return nil, nil, newGoAIError(req.Model, err)
	}
	for chunk := range stream.Stream() {
		if err := forwardChunk(chunk, onDelta); err != nil {
			return nil, nil, err
		}
	}
	if err := stream.Err(); err != nil {
		return nil, nil, newGoAIError(req.Model, err)
	}
	result := stream.Result()
	msg := &Message{Role: "assistant", Content: result.Text, ReasoningContent: result.Reasoning, ToolCalls: fromGoAIToolCalls(result.ToolCalls)}
	var usage *Usage
	if result.TotalUsage.TotalTokens > 0 || result.TotalUsage.InputTokens > 0 || result.TotalUsage.OutputTokens > 0 {
		usage = &Usage{
			PromptTokens:     result.TotalUsage.InputTokens,
			CompletionTokens: result.TotalUsage.OutputTokens,
			TotalTokens:      result.TotalUsage.TotalTokens,
		}
	}
	return msg, usage, nil
}

func forwardChunk(chunk provider.StreamChunk, onDelta func(Delta) error) error {
	if chunk.Type == provider.ChunkError && chunk.Error != nil {
		return chunk.Error
	}
	if onDelta == nil {
		return nil
	}
	var delta Delta
	switch chunk.Type {
	case provider.ChunkText:
		delta.Content = chunk.Text
	case provider.ChunkReasoning:
		delta.ReasoningContent = chunk.Text
	default:
		return nil
	}
	if delta.Content == "" && delta.ReasoningContent == "" {
		return nil
	}
	return onDelta(delta)
}

// toProviderMessages converts app messages to provider messages.
func toProviderMessages(messages []Message) ([]provider.Message, error) {
	out := make([]provider.Message, 0, len(messages))
	for _, message := range messages {
		role, err := toProviderRole(message.Role)
		if err != nil {
			return nil, err
		}
		parts := make([]provider.Part, 0, 2+len(message.ToolCalls))
		if message.Content != "" {
			parts = append(parts, provider.Part{Type: provider.PartText, Text: message.Content})
		}
		if message.ReasoningContent != "" {
			parts = append(parts, provider.Part{Type: provider.PartReasoning, Text: message.ReasoningContent})
		}
		for _, call := range message.ToolCalls {
			parts = append(parts, provider.Part{Type: provider.PartToolCall, ToolCallID: call.ID, ToolName: call.Function.Name, ToolInput: json.RawMessage(call.Function.Arguments)})
		}
		// Tool messages carry only the tool result, replacing prior parts.
		if message.Role == "tool" {
			parts = []provider.Part{{Type: provider.PartToolResult, ToolCallID: message.ToolCallID, ToolName: message.Name, ToolOutput: message.Content}}
		}
		out = append(out, provider.Message{Role: role, Content: parts})
	}
	return out, nil
}

func toProviderRole(role string) (provider.Role, error) {
	switch role {
	case "system", "user", "assistant", "tool":
		return provider.Role(role), nil
	default:
		return "", fmt.Errorf("llm: unsupported message role %q", role)
	}
}

func fromGoAIToolCalls(calls []provider.ToolCall) []ToolCall {
	out := make([]ToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, ToolCall{ID: call.ID, Type: "function", Function: FuncCall{Name: call.Name, Arguments: string(call.Input)}})
	}
	return out
}

func newGoAIError(model string, err error) error {
	var apiErr *goai.APIError
	if errors.As(err, &apiErr) {
		return &LLMError{StatusCode: apiErr.StatusCode, Model: model, Body: apiErr.ResponseBody, Err: fmt.Errorf("%w: %v", classifyStatus(apiErr.StatusCode), err)}
	}
	var overflow *goai.ContextOverflowError
	if errors.As(err, &overflow) {
		return &LLMError{Model: model, Body: overflow.ResponseBody, Err: fmt.Errorf("%w: %v", ErrInvalidRequest, err)}
	}
	if errors.Is(err, context.Canceled) {
		return err
	}
	return &LLMError{Model: model, Err: fmt.Errorf("%w: %v", ErrStreamInterrupted, err)}
}

// ponytail: status already implies the class; Kind field was a third copy of the same fact
func classifyStatus(status int) error {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return ErrAuthFailed
	case status == http.StatusTooManyRequests:
		return ErrRateLimit
	case status == http.StatusBadRequest:
		return ErrInvalidRequest
	case status >= http.StatusInternalServerError:
		return ErrServerUnavailable
	default:
		return ErrStreamInterrupted
	}
}
