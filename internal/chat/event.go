package chat

import (
	"encoding/json"
	"excelsior/pkg/agent"
	"excelsior/pkg/llm"
)

// Terminal outcome statuses.
const (
	OutcomeSucceeded         = "succeeded"
	OutcomeFailed            = "failed"
	OutcomeCanceled          = "canceled"
	OutcomePersistenceFailed = "persistence_failed"
)

// Interaction kinds.
const (
	InteractionPermission = "permission"
	InteractionAsk        = "ask"
)

// Event is the transport-neutral event emitted during a chat turn.
type Event struct {
	SessionID    string
	RunID        string
	Type         string
	Text         string
	Reasoning    string
	ToolName     string
	ToolCallID   string
	ToolArgs     string
	ToolResult   string
	FinishReason string
	Usage        *llm.Usage
	Context      *agent.ContextStatus
}

// Outcome represents the authoritative terminal state of a run.
type Outcome struct {
	SessionID string `json:"sessionId"`
	RunID     string `json:"runId"`
	Status    string `json:"status"` // succeeded | failed | canceled | persistence_failed
	Persisted bool   `json:"persisted"`
	Error     string `json:"error,omitempty"`
	Code      string `json:"code,omitempty"`
}

// PermissionData describes a tool action requiring approval.
type PermissionData struct {
	Tool     string `json:"tool"`
	FilePath string `json:"filePath,omitempty"`
	Preview  string `json:"preview,omitempty"`
	Command  string `json:"command,omitempty"`
}

// AskData describes a user question requested by an agent.
type AskData struct {
	Question string   `json:"question"`
	Options  []string `json:"options"`
}

// PendingInteraction represents an awaiting user decision.
type PendingInteraction struct {
	ID         string          `json:"id"`
	Kind       string          `json:"kind"` // "permission" | "ask"
	SessionID  string          `json:"sessionId"`
	RunID      string          `json:"runId"`
	Permission *PermissionData `json:"permission,omitempty"`
	Ask        *AskData        `json:"ask,omitempty"`
}

// Snapshot contains the authoritative state of a session at subscription time.
type Snapshot struct {
	SessionID             string              `json:"sessionId"`
	RunID                 string              `json:"runId,omitempty"`
	Running               bool                `json:"running"`
	Messages              []llm.Message       `json:"messages"`
	Events                []Event             `json:"events,omitempty"`
	Pending               *PendingInteraction `json:"pending,omitempty"`
	Outcome               *Outcome            `json:"outcome,omitempty"`
	UnsavedAvailable      bool                `json:"unsavedAvailable"`
	ProjectionUnavailable bool                `json:"projectionUnavailable"`
}

func cloneMessages(in []llm.Message) []llm.Message {
	out := append([]llm.Message{}, in...)
	for i := range out {
		out[i].ToolCalls = append([]llm.ToolCall(nil), out[i].ToolCalls...)
	}
	return out
}
func cloneEvent(e Event) Event {
	if e.Context != nil {
		value := *e.Context
		e.Context = &value
	}
	if e.Usage != nil {
		u := *e.Usage
		e.Usage = &u
	}
	return e
}
func cloneEvents(in []Event) []Event {
	out := append([]Event(nil), in...)
	for i := range out {
		out[i] = cloneEvent(out[i])
	}
	return out
}
func cloneSnapshot(s Snapshot) Snapshot {
	b, _ := json.Marshal(s)
	var out Snapshot
	_ = json.Unmarshal(b, &out)
	return out
}
func messageBytes(m []llm.Message) int { b, _ := json.Marshal(m); return len(b) }
func eventBytes(e Event) int           { b, _ := json.Marshal(e); return len(b) }
func snapshotBytes(s Snapshot) int     { b, _ := json.Marshal(s); return len(b) }
