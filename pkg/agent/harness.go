package agent

import (
	"context"
	"fmt"
	"strings"

	"excelsior/pkg/llm"
	"excelsior/pkg/tools"
)

// ContextTier selects one of the paper's five context-management policies.
type ContextTier int

const (
	ContextT0 ContextTier = iota // no compaction
	ContextT1                    // elision
	ContextT2                    // elision with recall
	ContextT3                    // model summarization
	ContextT4                    // staged elision, recall, then summarization
)

// ActionSpace selects the workspace tools exposed to the model.
type ActionSpace string

const (
	ActionPredefined ActionSpace = "predefined"
	ActionBashOnly   ActionSpace = "bash"
)

const (
	defaultContextWindowTokens = 128_000
	defaultMaxSteps            = 300
	defaultMaxOutputTokens     = 16_384
	maxConcurrentReadTools     = 8
	maxPlanChars               = 20_000
	maxSummaryCalls            = 16
	softContextRatio           = 0.60
	hardContextRatio           = 0.85
	recentContextRatio         = 0.30
	minimumRecentTurns         = 2
)

// HarnessConfig controls the three components studied in the paper, plus the
// execution limits held fixed by its harness.
type HarnessConfig struct {
	ContextWindowTokens int // total prompt + reserved output tokens
	ContextTier         ContextTier
	Planning            bool
	ActionSpace         ActionSpace
	MaxSteps            int
	MaxOutputTokens     int
	Temperature         float64
	TopP                float64
}

// DefaultHarnessConfig is the paper-aligned operating profile: planning,
// predefined tools, staged T4 context management, and a 128k window.
func DefaultHarnessConfig() HarnessConfig {
	return HarnessConfig{
		ContextWindowTokens: defaultContextWindowTokens,
		ContextTier:         ContextT4,
		Planning:            true,
		ActionSpace:         ActionPredefined,
		MaxSteps:            defaultMaxSteps,
		MaxOutputTokens:     defaultMaxOutputTokens,
		Temperature:         0,
		TopP:                0.95,
	}
}

func normalizeHarnessConfig(input *HarnessConfig) (HarnessConfig, error) {
	cfg := DefaultHarnessConfig()
	if input != nil {
		cfg = *input
	}
	if cfg.ContextWindowTokens <= 0 {
		cfg.ContextWindowTokens = defaultContextWindowTokens
	}
	if cfg.ContextTier < ContextT0 || cfg.ContextTier > ContextT4 {
		return HarnessConfig{}, fmt.Errorf("agent: context tier must be 0..4, got %d", cfg.ContextTier)
	}
	if cfg.ActionSpace == "" {
		cfg.ActionSpace = ActionPredefined
	}
	if cfg.ActionSpace != ActionPredefined && cfg.ActionSpace != ActionBashOnly {
		return HarnessConfig{}, fmt.Errorf("agent: action space must be %q or %q, got %q", ActionPredefined, ActionBashOnly, cfg.ActionSpace)
	}
	if cfg.MaxSteps <= 0 {
		cfg.MaxSteps = defaultMaxSteps
	}
	if cfg.MaxOutputTokens <= 0 {
		cfg.MaxOutputTokens = defaultMaxOutputTokens
	}
	if cfg.MaxOutputTokens >= cfg.ContextWindowTokens {
		return HarnessConfig{}, fmt.Errorf("agent: output limit %d must be smaller than the total context window %d", cfg.MaxOutputTokens, cfg.ContextWindowTokens)
	}
	if cfg.Temperature < 0 || cfg.Temperature > 2 {
		return HarnessConfig{}, fmt.Errorf("agent: temperature must be 0..2, got %v", cfg.Temperature)
	}
	if cfg.TopP <= 0 {
		cfg.TopP = 0.95
	}
	if cfg.TopP > 1 {
		return HarnessConfig{}, fmt.Errorf("agent: top-p must be in (0,1], got %v", cfg.TopP)
	}
	return cfg, nil
}

func (cfg HarnessConfig) inputBudget() int { return cfg.ContextWindowTokens - cfg.MaxOutputTokens }

const planningInstruction = `# Task planning

You have an update_plan tool. The harness keeps the plan separately and shows it before each model turn.

- Before taking action on a non-trivial task, create a short plan with update_plan.
- Send the complete task list on every update; each task has content, status (pending, in_progress, completed), and activeForm.
- Keep one unfinished task in_progress. Mark each task completed as soon as it is done.
- Skip planning for a trivial, single-step task or a purely informational request.`

const planningReminderInitial = `<system-reminder>

Create a plan with update_plan before taking action on this non-trivial task. Skip this only for a trivial, single-step or informational request.

</system-reminder>`

func (a *Agent) runHarness(ctx context.Context, model string, cfg HarnessConfig, original []llm.Message, emit func(StreamEvent), usage *usageAcc, opts RunOptions) (result *RunResult, runErr error) {
	complete := append([]llm.Message(nil), original...)
	defer func() {
		if result == nil {
			result = &RunResult{Messages: complete, TotalUsage: usage.Sum()}
		}
	}()
	toolByName := make(map[string]tools.Tool, len(a.Tools))
	for _, tool := range a.Tools {
		if tool.Name() == "update_plan" || tool.Name() == "recall_event" {
			continue // component tools are owned by the harness
		}
		if cfg.ActionSpace == ActionBashOnly && tool.Name() != "bash" {
			continue
		}
		toolByName[tool.Name()] = tool
	}
	toolDefs := toLLMTools(toolByName)

	plan := ""
	if cfg.Planning {
		toolDefs = append(toolDefs, llm.ToolDefinition{
			Type: "function",
			Function: llm.FuncDef{
				Name:        "update_plan",
				Description: "Replace the complete task plan. Keep one task in_progress until all tasks are completed. The plan is held in memory and shown before every model turn.",
				Parameters: jsonSchemaMap(map[string]any{"plan": map[string]any{
					"type": "array", "minItems": 1,
					"items": jsonSchemaMap(map[string]any{
						"content":    map[string]any{"type": "string"},
						"status":     map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed"}},
						"activeForm": map[string]any{"type": "string"},
					}, []string{"content", "status", "activeForm"}),
				}}, []string{"plan"}),
			},
		})
	}
	manager, err := newContextManager(a.LLM, model, cfg, opts.ReasoningEffort, usage)
	if err != nil {
		return nil, err
	}
	defer manager.store.close()
	manager.emit = emit
	if usesRecall(cfg.ContextTier) {
		toolDefs = append(toolDefs, llm.ToolDefinition{
			Type: "function",
			Function: llm.FuncDef{
				Name:        "recall_event",
				Description: "Retrieve an archived observation or summarized model turn by its event id. Returns the original content, including tool results. Prefer re-reading or re-running when easily reproducible.",
				Parameters:  jsonSchemaMap(map[string]any{"id": map[string]any{"type": "string", "description": "event id shown in the elision stub"}}, []string{"id"}),
			},
		})
	}

	working := append([]llm.Message(nil), original...)
	protocol := actionInstructions(cfg.ActionSpace, toolByName)
	if cfg.Planning {
		protocol += "\n\n" + planningInstruction
	}
	if len(working) > 0 && working[0].Role == "system" {
		working[0].Content = strings.TrimSpace(working[0].Content) + "\n\n" + protocol
	} else {
		working = append([]llm.Message{{Role: "system", Content: protocol}}, working...)
	}

	tracker := stuckTracker{}
	reminder := ""
	for step := 0; step < cfg.MaxSteps; step++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		extra := make([]llm.Message, 0, 2)
		if cfg.Planning {
			if strings.TrimSpace(plan) == "" {
				extra = append(extra, llm.Message{Role: "system", Content: planningReminderInitial})
			} else {
				extra = append(extra, llm.Message{Role: "system", Content: "<system-reminder>\nCurrent plan (update with update_plan; keep one task in_progress until all are completed):\n" + plan + "\n</system-reminder>"})
			}
		}
		if reminder != "" {
			extra = append(extra, llm.Message{Role: "system", Content: reminder})
			reminder = ""
		}
		compact := step == 0 && opts.Compact
		select {
		case <-opts.CompactRequests:
			compact = true
		default:
		}
		working, err = manager.manage(ctx, working, extra, toolDefs, compact)
		if err != nil {
			return nil, err
		}

		requestMessages := assembleMessages(working, manager.summaryForInput(), extra)
		request := llm.ChatRequest{
			ContextWindowTokens: cfg.ContextWindowTokens,
			Model:               model,
			ReasoningEffort:     opts.ReasoningEffort,
			Messages:            requestMessages,
			Tools:               toolDefs,
			Temperature:         floatPointer(cfg.Temperature),
			MaxTokens:           intPointer(cfg.MaxOutputTokens),
			TopP:                floatPointer(cfg.TopP),
		}
		var streamedUsage usageAcc
		assistant, reportedUsage, err := a.LLM.StreamChat(ctx, request, func(delta llm.Delta) error {
			if delta.Usage != nil {
				streamedUsage.Add(*delta.Usage)
			}
			if delta.ReasoningContent != "" {
				emit(StreamEvent{Type: "reasoning", Reasoning: delta.ReasoningContent})
			}
			if delta.Content != "" {
				emit(StreamEvent{Type: "text", Text: delta.Content})
			}
			return nil
		})
		if reportedUsage != nil {
			usage.Add(*reportedUsage)
		} else if streamedUsage.seen {
			usage.Add(*streamedUsage.Sum())
		}
		if err != nil {
			return nil, fmt.Errorf("agent model step: %w", err)
		}
		if assistant == nil {
			return nil, ErrNilLLMMessage
		}
		assistant.Role = "assistant"
		working = append(working, *assistant)
		complete = append(complete, *assistant)
		if len(assistant.ToolCalls) == 0 {
			emit(StreamEvent{Type: "done", Text: assistant.Content, FinishReason: "stop", Usage: usage.Sum()})
			return &RunResult{FinalMessage: assistant, Messages: complete, TotalUsage: usage.Sum()}, nil
		}

		observations, nextReminder, err := executeToolCalls(ctx, assistant.ToolCalls, toolByName, manager, &plan, cfg.Planning, emit, &tracker)
		complete = append(complete, observations...)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			return nil, err
		}
		working = append(working, observations...)
		reminder = nextReminder
	}
	return nil, fmt.Errorf("%w after %d steps", ErrStepBudget, cfg.MaxSteps)
}

// The action ablation changes instructions together with the exposed tools.
func actionInstructions(space ActionSpace, registered map[string]tools.Tool) string {
	if space == ActionBashOnly {
		return `# Workspace protocol
Use bash for all repository inspection, search, and changes. Read existing files before modifying them and prefer targeted edits. Shell changes do not update file state or run automatic diagnostics: read changes back and validate them explicitly. Report actual results and errors. Stay inside the workspace.`
	}
	rules := []string{"# Workspace protocol", "Inspect the repository before modifying it. Stay inside the workspace and report actual results. Independent read-only calls may run concurrently. Read each result and correct failed inputs before retrying."}
	for _, entry := range []struct{ name, rule string }{
		{"read_file", "Use read_file before editing existing files. A full read is required for mutation; partial reads do not qualify."},
		{"edit_file", "Use edit_file for targeted replacements. old_text must be unique unless replace_all is set."},
		{"write_file", "Use write_file for new files or full replacements. Existing files require overwrite=true and a recorded full read."},
		{"bash", "Use bash for external programs and verification. Shell changes bypass tracked file state and automatic diagnostics."},
		{"web_fetch", "Use web_fetch for concrete documentation URLs. Avoid repeatedly fetching the same URL."},
	} {
		if _, ok := registered[entry.name]; ok {
			rules = append(rules, entry.rule)
		}
	}
	return strings.Join(rules, "\n")
}
