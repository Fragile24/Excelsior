package agent

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"excelsior/pkg/llm"
	"excelsior/pkg/tools"
)

type actionTool struct{ name, output string }

func (t actionTool) Name() string        { return t.name }
func (t actionTool) Description() string { return t.name }
func (t actionTool) Parameters() any     { return map[string]any{"type": "object"} }
func (t actionTool) Execute(context.Context, json.RawMessage) (string, error) {
	return t.output, nil
}

type actionProvider struct {
	request llm.ChatRequest
	steps   int
}

func (*actionProvider) ModelName() string                                         { return "test-model" }
func (*actionProvider) CountTokens(context.Context, llm.ChatRequest) (int, error) { return 1, nil }
func (p *actionProvider) StreamChat(_ context.Context, req llm.ChatRequest, _ func(llm.Delta) error) (*llm.Message, *llm.Usage, error) {
	p.request = req
	p.steps++
	if p.steps == 1 {
		return &llm.Message{ToolCalls: []llm.ToolCall{{ID: "call-1", Type: "function", Function: llm.FuncCall{Name: "bash", Arguments: "{}"}}}}, nil, nil
	}
	return &llm.Message{Content: "done"}, nil, nil
}

func TestToolListsPreserveActionSpaceAndOverrides(t *testing.T) {
	for _, tc := range []struct {
		actions ActionSpace
		want    []string
	}{{ActionPredefined, []string{"askQuestion", "bash"}}, {ActionBashOnly, []string{"bash"}}} {
		t.Run(string(tc.actions), func(t *testing.T) {
			provider := &actionProvider{}
			cfg := DefaultHarnessConfig()
			cfg.ActionSpace, cfg.ContextTier, cfg.Planning = tc.actions, ContextT0, false
			a := &Agent{LLM: provider, Harness: &cfg, Tools: []tools.Tool{
				actionTool{"bash", "first"}, actionTool{"askQuestion", "unused"}, actionTool{"bash", "last"},
			}}
			result, err := a.RunWithHistory(context.Background(), RunOptions{Messages: []llm.Message{{Role: "user", Content: "Run a tool"}}})
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, tool := range provider.request.Tools {
				names = append(names, tool.Function.Name)
			}
			if !slices.Equal(names, tc.want) {
				t.Fatalf("model tools = %v; want %v", names, tc.want)
			}
			if len(result.Messages) != 4 || result.Messages[2].Role != "tool" || result.Messages[2].Content != "last" {
				t.Fatalf("tool override did not reach the model: %+v", result.Messages)
			}
		})
	}
}
