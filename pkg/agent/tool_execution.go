package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"excelsior/pkg/llm"
	"excelsior/pkg/tools"
)

type stuckTracker struct {
	lastSignature string
	lastFailure   string
	repeated      int
	failures      int
	reminded      bool
}

type toolOutcome struct {
	call   llm.ToolCall
	output string
	err    error
}

func executeToolCalls(ctx context.Context, calls []llm.ToolCall, registered map[string]tools.Tool, manager *contextManager, plan *string, planning bool, emit func(StreamEvent), tracker *stuckTracker) ([]llm.Message, string, error) {
	observations := make([]llm.Message, 0, len(calls))
	reminder := ""
	readOnly := func(call llm.ToolCall) bool {
		if call.Function.Name == "update_plan" {
			return false
		}
		if call.Function.Name == "recall_event" {
			return manager.store != nil
		}
		tool, ok := registered[call.Function.Name]
		return ok && toolReadOnly(tool)
	}
	for index := 0; index < len(calls); {
		end := index + 1
		if readOnly(calls[index]) {
			for end < len(calls) && end-index < maxConcurrentReadTools && readOnly(calls[end]) {
				end++
			}
		}
		outcomes := make([]toolOutcome, end-index)
		for _, call := range calls[index:end] {
			emit(StreamEvent{Type: "tool_start", ToolName: call.Function.Name, ToolCallID: call.ID, ToolArgs: call.Function.Arguments})
		}
		var group sync.WaitGroup
		for offset, call := range calls[index:end] {
			execute := func() { outcomes[offset] = executeToolCall(ctx, call, registered, manager, plan, planning) }
			if end-index == 1 {
				execute()
			} else {
				group.Go(execute)
			}
		}
		group.Wait()
		var stopErr error
		for _, outcome := range outcomes {
			observations = append(observations, resultMessage(outcome))
			emit(StreamEvent{Type: "tool_result", ToolName: outcome.call.Function.Name, ToolCallID: outcome.call.ID, ToolResult: outcomeMessageContent(outcome)})
			if stopErr == nil {
				next, err := tracker.observe(outcome)
				stopErr = err
				if next != "" {
					reminder = next
				}
			}
		}
		if stopErr != nil {
			return observations, "", stopErr
		}
		index = end
	}
	return observations, reminder, nil
}

func executeToolCall(ctx context.Context, call llm.ToolCall, registered map[string]tools.Tool, manager *contextManager, plan *string, planning bool) (outcome toolOutcome) {
	outcome.call = call
	// A tool failure is an observation the model can recover from.
	defer func() {
		if failure := recover(); failure != nil {
			outcome.err = fmt.Errorf("tool %s panicked: %v", call.Function.Name, failure)
		}
	}()
	if err := ctx.Err(); err != nil {
		outcome.err = err
		return
	}
	switch call.Function.Name {
	case "update_plan":
		if !planning {
			outcome.err = errors.New("planning is disabled")
			return
		}
		outcome.output, outcome.err = validatedPlan(call.Function.Arguments)
		if outcome.err == nil {
			*plan = outcome.output
			outcome.output = "Plan updated."
		}
	case "recall_event":
		var args struct {
			ID string `json:"id"`
		}
		if manager.store == nil {
			outcome.err = errors.New("recall is disabled for this context tier")
		} else if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil || strings.TrimSpace(args.ID) == "" {
			outcome.err = errors.New("recall_event requires an event id")
		} else {
			outcome.output, outcome.err = manager.store.load(args.ID)
		}
	default:
		if tool, ok := registered[call.Function.Name]; ok {
			outcome.output, outcome.err = tool.Execute(ctx, json.RawMessage(call.Function.Arguments))
		} else {
			outcome.err = fmt.Errorf("unknown tool %q", call.Function.Name)
		}
	}
	return
}

func validatedPlan(arguments string) (string, error) {
	var args struct {
		Plan []struct {
			Content    string `json:"content"`
			Status     string `json:"status"`
			ActiveForm string `json:"activeForm"`
		} `json:"plan"`
	}
	if len(arguments) > maxPlanChars {
		return "", fmt.Errorf("update_plan exceeds %d bytes", maxPlanChars)
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil || len(args.Plan) == 0 {
		return "", errors.New("update_plan requires a non-empty plan array with content, status, and activeForm per task")
	}
	active, unfinished := 0, 0
	for _, task := range args.Plan {
		if strings.TrimSpace(task.Content) == "" || strings.TrimSpace(task.ActiveForm) == "" {
			return "", errors.New("each plan task needs content and activeForm")
		}
		switch task.Status {
		case "in_progress":
			active++
			unfinished++
		case "pending":
			unfinished++
		case "completed":
		default:
			return "", fmt.Errorf("invalid plan status %q; use pending, in_progress, or completed", task.Status)
		}
	}
	if unfinished > 0 && active != 1 {
		return "", errors.New("keep exactly one task in_progress until all tasks are completed")
	}
	encoded, err := json.Marshal(args.Plan)
	return string(encoded), err
}

func toolReadOnly(tool tools.Tool) bool {
	readOnly, ok := tool.(interface{ ReadOnly() bool })
	return ok && readOnly.ReadOnly()
}

func outcomeMessageContent(outcome toolOutcome) string {
	content := outcome.output
	if outcome.err != nil {
		content = "error: " + outcome.err.Error() + "\n" + content
	}
	// Recall returns archived content exactly; ordinary observations were already
	// bounded when generated. A summarized turn may contain several observations.
	if outcome.call.Function.Name == "recall_event" && outcome.err == nil {
		return content
	}
	return truncateRunes(strings.TrimSpace(content), maxToolResultChars)
}

func resultMessage(outcome toolOutcome) llm.Message {
	return llm.Message{Role: "tool", ToolCallID: outcome.call.ID, Name: outcome.call.Function.Name, Content: outcomeMessageContent(outcome)}
}

func (tracker *stuckTracker) observe(outcome toolOutcome) (string, error) {
	signature := outcome.call.Function.Name + "\x00" + outcome.call.Function.Arguments
	if signature != tracker.lastSignature {
		*tracker = stuckTracker{lastSignature: signature}
	}
	tracker.repeated++
	failure := ""
	if outcome.err != nil && !errors.Is(outcome.err, tools.ErrPermissionDenied) {
		failure = outcome.err.Error()
	}
	if failure != "" && failure == tracker.lastFailure {
		tracker.failures++
	} else {
		tracker.failures = 0
		if failure != "" {
			tracker.failures = 1
		}
	}
	tracker.lastFailure = failure
	if tracker.failures >= 8 {
		return "", fmt.Errorf("%w: terminated after eight consecutive identical failing tool calls", ErrStuck)
	}
	if tracker.repeated >= 5 && !tracker.reminded {
		tracker.reminded = true
		if tracker.failures >= 5 {
			return fmt.Sprintf("<system-reminder>You called %s with identical arguments %d times and it keeps failing. Inspect the error and change approach before acting again.</system-reminder>", outcome.call.Function.Name, tracker.repeated), nil
		}
		return fmt.Sprintf("<system-reminder>You called %s with identical arguments %d times. Use the result and move to the next concrete step.</system-reminder>", outcome.call.Function.Name, tracker.repeated), nil
	}
	return "", nil
}
