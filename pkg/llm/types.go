package llm

// ReasoningEffort selects the provider's reasoning budget for one generation.
type ReasoningEffort string

const (
	ReasoningEffortNone ReasoningEffort = "none"
	ReasoningEffortLow  ReasoningEffort = "low"
	ReasoningEffortHigh ReasoningEffort = "high"
	ReasoningEffortMax  ReasoningEffort = "max"
)

func (e ReasoningEffort) Valid() bool {
	switch e {
	case ReasoningEffortNone, ReasoningEffortLow, ReasoningEffortHigh, ReasoningEffortMax:
		return true
	default:
		return false
	}
}

// Message is a chat message. ReasoningContent is DeepSeek-specific (R1/reasoner).
type Message struct {
	Role             string     `json:"role"`
	Content          string     `json:"content,omitempty"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
	Name             string     `json:"name,omitempty"`
}

// ToolCall is a model-requested function invocation in an assistant message.
type ToolCall struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"` // "function"
	Function FuncCall `json:"function"`
}

// FuncCall holds the function name and JSON-encoded arguments.
type FuncCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// ToolDefinition describes a callable tool for the model (OpenAI function calling).
type ToolDefinition struct {
	Type     string  `json:"type"` // "function"
	Function FuncDef `json:"function"`
}

// FuncDef is the function metadata inside a [ToolDefinition].
type FuncDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"` // JSON Schema
}

// ChatRequest contains one generation's input and sampling settings.
type ChatRequest struct {
	// ContextWindowTokens is the total input + output budget; zero disables the
	// client-side token guard for standalone callers. It is never sent on the wire.
	ContextWindowTokens int              `json:"-"`
	Model               string           `json:"model"`
	ReasoningEffort     ReasoningEffort  `json:"-"`
	Messages            []Message        `json:"messages"`
	Tools               []ToolDefinition `json:"tools,omitempty"`
	Temperature         *float64         `json:"temperature,omitempty"`
	MaxTokens           *int             `json:"max_tokens,omitempty"`
	TopP                *float64         `json:"top_p,omitempty"`
}

// Delta is a streaming fragment.
type Delta struct {
	Content          string
	ReasoningContent string
	Usage            *Usage
}

// Usage reports token counts when provided by the API.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
