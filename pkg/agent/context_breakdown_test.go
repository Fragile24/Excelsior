package agent

import (
	"context"
	"testing"

	"excelsior/pkg/llm"
)

type contextBreakdownProvider struct{}

func (*contextBreakdownProvider) ModelName() string { return "test-model" }
func (*contextBreakdownProvider) CountTokens(_ context.Context, req llm.ChatRequest) (int, error) {
	tokens := 3 // Prompt framing and generation prefix.
	for _, message := range req.Messages {
		tokens += 2 + len(message.Content) + len(message.ReasoningContent)
		for _, call := range message.ToolCalls {
			tokens += 5 + len(call.Function.Name) + len(call.Function.Arguments)
		}
	}
	for _, definition := range req.Tools {
		tokens += 20 + len(definition.Function.Name) + len(definition.Function.Description)
	}
	return tokens, nil
}
func (*contextBreakdownProvider) StreamChat(context.Context, llm.ChatRequest, func(llm.Delta) error) (*llm.Message, *llm.Usage, error) {
	return nil, nil, nil
}

func TestContextTokenBreakdownPartitionsExactInputTotal(t *testing.T) {
	provider := &contextBreakdownProvider{}
	messages := []llm.Message{
		{Role: "system", Content: "Follow the workspace instructions."},
		{Role: "user", Content: "Fix the parser."},
		{Role: "assistant", Content: "I will inspect the parser."},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call-1", Type: "function", Function: llm.FuncCall{Name: "read_file", Arguments: `{"path":"parser.go"}`}}}},
		{Role: "tool", Content: "Parser source contents."},
		{Role: "assistant", Content: "The parser now handles empty input."},
	}
	definitions := []llm.ToolDefinition{{Type: "function", Function: llm.FuncDef{Name: "read_file", Description: "Read a workspace file."}}}
	fullCount, err := provider.CountTokens(context.Background(), llm.ChatRequest{Messages: messages, Tools: definitions})
	if err != nil {
		t.Fatal(err)
	}
	manager := &contextManager{provider: provider, model: "test-model", config: DefaultHarnessConfig()}
	breakdown := manager.tokenBreakdown(context.Background(), messages, definitions, fullCount)
	if breakdown == nil {
		t.Fatal("expected role token breakdown")
	}
	if got := breakdown.System + breakdown.Tools + breakdown.User + breakdown.Assistant; got != fullCount {
		t.Fatalf("breakdown sum = %d; want exact input total %d", got, fullCount)
	}
	for role, count := range map[string]int{
		"system": breakdown.System, "tools": breakdown.Tools,
		"user": breakdown.User, "assistant": breakdown.Assistant,
	} {
		if count <= 0 {
			t.Errorf("%s token estimate = %d; want positive count", role, count)
		}
	}
}

func TestApportionTokensUsesStableLargestRemainders(t *testing.T) {
	got := apportionTokens(7, [4]int{1, 1, 1, 1})
	if *got != (TokenBreakdown{System: 2, Tools: 2, User: 2, Assistant: 1}) {
		t.Fatalf("apportionTokens() = %+v; want stable split 2, 2, 2, 1", *got)
	}
}
