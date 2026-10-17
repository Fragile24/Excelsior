package agent

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"unicode/utf8"

	"excelsior/pkg/llm"
	"excelsior/pkg/tools"
)

// Runner defines the execution interface used by the chat coordinator.
type Runner interface {
	RunWithHistory(ctx context.Context, opts RunOptions) (*RunResult, error)
}

// Agent runs a fixed ReAct loop and applies planning, action-space, context,
// and safety policies around each provider call.
type Agent struct {
	LLM     llm.ChatProvider
	Tools   []tools.Tool
	System  string
	Model   string
	Logger  *slog.Logger
	Harness *HarnessConfig
}

var _ Runner = (*Agent)(nil)

// StreamEvent is one event emitted while a run is executing.
type StreamEvent struct {
	Type         string // text | reasoning | tool_start | tool_result | context | done | error
	Text         string
	Reasoning    string
	ToolName     string
	ToolCallID   string
	ToolArgs     string
	ToolResult   string
	FinishReason string
	Usage        *llm.Usage
	Context      *ContextStatus
}

// RunOptions controls one agent run.
type RunOptions struct {
	// Messages is the conversation history excluding the configured system prompt.
	// It must contain at least one user message.
	Messages        []llm.Message
	ReasoningEffort llm.ReasoningEffort
	// OnEvent receives streaming and tool lifecycle events synchronously.
	OnEvent func(StreamEvent)
	// Compact requests compaction before the first model step. Further requests
	// are coalesced by the coordinator and consumed between complete tool turns.
	Compact         bool
	CompactRequests <-chan struct{}
}

// RunResult is the full, un-compacted transcript, including partial failed runs.
// Context management only changes the private working copy sent to the model.
type RunResult struct {
	FinalMessage *llm.Message
	Messages     []llm.Message
	TotalUsage   *llm.Usage
}

const maxToolResultChars = tools.MaxObservationChars

func (a *Agent) logger() *slog.Logger {
	if a.Logger != nil {
		return a.Logger
	}
	return slog.Default()
}

func (a *Agent) resolveModel() string {
	if strings.TrimSpace(a.Model) != "" {
		return strings.TrimSpace(a.Model)
	}
	if a.LLM != nil {
		return a.LLM.ModelName()
	}
	return ""
}

// Run executes a conversation and returns its final assistant message.
func (a *Agent) Run(ctx context.Context, opts RunOptions) (*llm.Message, error) {
	result, err := a.RunWithHistory(ctx, opts)
	if err != nil {
		return nil, err
	}
	return result.FinalMessage, nil
}

// RunWithHistory executes a harness-controlled ReAct loop until the model
// returns a final answer or the configured step budget is exhausted.
func (a *Agent) RunWithHistory(ctx context.Context, opts RunOptions) (*RunResult, error) {
	if a.LLM == nil {
		return nil, fmt.Errorf("agent: %w", ErrLLMNotConfigured)
	}
	if len(opts.Messages) == 0 {
		return nil, fmt.Errorf("agent: %w", ErrEmptyMessages)
	}
	if !hasUserMessage(opts.Messages) {
		return nil, fmt.Errorf("agent: %w", ErrEmptyMessages)
	}

	cfg, err := normalizeHarnessConfig(a.Harness)
	if err != nil {
		return nil, err
	}
	messages := a.prepareMessages(opts.Messages)
	emit := opts.OnEvent
	if emit == nil {
		emit = func(StreamEvent) {}
	}
	model := a.resolveModel()
	a.logger().Info("agent run start", "model", model, "messages", len(messages), "maxSteps", cfg.MaxSteps, "contextTier", cfg.ContextTier)

	var usage usageAcc
	result, err := a.runHarness(ctx, model, cfg, messages, emit, &usage, opts)
	if err != nil {
		emit(StreamEvent{Type: "error", Text: err.Error(), Usage: usage.Sum()})
		return result, err
	}
	return result, nil
}

func hasUserMessage(messages []llm.Message) bool {
	for _, message := range messages {
		if message.Role == "user" {
			return true
		}
	}
	return false
}

func (a *Agent) prepareMessages(incoming []llm.Message) []llm.Message {
	messages := append([]llm.Message(nil), incoming...)
	if strings.TrimSpace(a.System) != "" && (len(messages) == 0 || messages[0].Role != "system") {
		messages = append([]llm.Message{{Role: "system", Content: a.System}}, messages...)
	}
	return sanitizeToolCalls(messages)
}

// sanitizeToolCalls closes dangling calls left by older or interrupted history
// before a provider sees it.
func sanitizeToolCalls(messages []llm.Message) []llm.Message {
	out := make([]llm.Message, 0, len(messages))
	pending := map[string]bool{}
	flush := func() {
		for id := range pending {
			out = append(out, llm.Message{Role: "tool", ToolCallID: id, Content: "(tool call cancelled)"})
		}
		pending = map[string]bool{}
	}
	for _, message := range messages {
		if message.Role != "tool" {
			flush()
		}
		if message.Role == "assistant" {
			for _, call := range message.ToolCalls {
				if call.ID != "" {
					pending[call.ID] = true
				}
			}
		}
		if message.Role == "tool" && message.ToolCallID != "" {
			delete(pending, message.ToolCallID)
		}
		out = append(out, message)
	}
	flush()
	return out
}

type usageAcc struct {
	prompt, completion int
	seen               bool
}

func (u *usageAcc) Add(usage llm.Usage) {
	u.prompt += usage.PromptTokens
	u.completion += usage.CompletionTokens
	u.seen = true
}

func (u *usageAcc) Sum() *llm.Usage {
	if u == nil || !u.seen {
		return nil
	}
	return &llm.Usage{PromptTokens: u.prompt, CompletionTokens: u.completion, TotalTokens: u.prompt + u.completion}
}

func truncateRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	const suffix = "\n[truncated]"
	runes := []rune(value)
	if limit <= len(suffix) {
		return string(runes[:max(0, limit)])
	}
	return string(runes[:limit-len(suffix)]) + suffix
}

func toLLMTools(registered map[string]tools.Tool) []llm.ToolDefinition {
	definitions := make([]llm.ToolDefinition, 0, len(registered))
	for _, tool := range registered {
		definitions = append(definitions, llm.ToolDefinition{
			Type: "function",
			Function: llm.FuncDef{
				Name:        tool.Name(),
				Description: tool.Description(),
				Parameters:  tool.Parameters(),
			},
		})
	}
	slices.SortFunc(definitions, func(a, b llm.ToolDefinition) int { return cmp.Compare(a.Function.Name, b.Function.Name) })
	return definitions
}

func jsonSchemaMap(properties map[string]any, required []string) map[string]any {
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "properties": properties, "required": required}
}

// DefaultSystemPrompt is intentionally concise; harness-specific protocols are
// injected only when their corresponding component is enabled.
const DefaultSystemPrompt = `You are Excelsior, a senior coding agent working in the user's workspace.

Use tools to inspect files before changing them. Make focused, correct changes that match the project style. Run relevant verification when useful, and report what changed and what you verified. Treat tool errors as useful feedback and change approach when an action fails.`
