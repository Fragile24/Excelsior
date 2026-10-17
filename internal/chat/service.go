package chat

import (
	"context"
	"errors"
	"fmt"
	"time"

	"excelsior/pkg/agent"
	"excelsior/pkg/llm"
	"excelsior/pkg/session"
)

// ErrPersistenceFailed indicates the model run generated a result but saving to disk failed.
var ErrPersistenceFailed = errors.New("persistence failed")

// Service owns one chat turn and its optional session persistence.
// Transport code supplies the runner and event sink; it does not save history.
type Service struct {
	Runner agent.Runner
	Store  session.Store
}

// PreparedTurn contains pre-loaded history and record for single-load execution.
type PreparedTurn struct {
	SessionID       string
	RunID           string
	Messages        []llm.Message
	Record          session.Record
	OnEvent         func(Event)
	OnPersist       func() error
	ReasoningEffort llm.ReasoningEffort
	Compact         bool
	CompactRequests <-chan struct{}
}

// RunPrepared executes the turn using already loaded history and updates the provided record.
func (s Service) RunPrepared(ctx context.Context, turn PreparedTurn) (*agent.RunResult, error) {
	result, err := s.Runner.RunWithHistory(ctx, agent.RunOptions{
		Messages:        turn.Messages,
		ReasoningEffort: turn.ReasoningEffort,
		Compact:         turn.Compact,
		CompactRequests: turn.CompactRequests,
		OnEvent: func(event agent.StreamEvent) {
			if turn.OnEvent != nil {
				if event.Type == "done" {
					event.Type = "generation"
				}
				turn.OnEvent(Event{
					SessionID:    turn.SessionID,
					RunID:        turn.RunID,
					Type:         event.Type,
					Text:         event.Text,
					Reasoning:    event.Reasoning,
					ToolName:     event.ToolName,
					ToolCallID:   event.ToolCallID,
					ToolArgs:     event.ToolArgs,
					ToolResult:   event.ToolResult,
					FinishReason: event.FinishReason,
					Usage:        event.Usage,
					Context:      event.Context,
				})
			}
		},
	})
	if err != nil {
		return result, err
	}
	if result == nil {
		return nil, errors.New("runner returned no result")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if s.Store == nil || turn.SessionID == "" {
		return result, nil
	}

	if turn.OnPersist != nil {
		if err := turn.OnPersist(); err != nil {
			return result, err
		}
	}

	rec := turn.Record
	if rec.ID == "" {
		rec.ID = turn.SessionID
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now().UTC()
	}
	rec.Messages = withoutSystemMessages(result.Messages)
	if saveErr := s.Store.Save(rec); saveErr != nil {
		return result, fmt.Errorf("%w: %v", ErrPersistenceFailed, saveErr)
	}
	return result, nil
}

func withoutSystemMessages(messages []llm.Message) []llm.Message {
	out := make([]llm.Message, 0, len(messages))
	for _, message := range messages {
		if message.Role != "system" {
			out = append(out, message)
		}
	}
	return out
}
