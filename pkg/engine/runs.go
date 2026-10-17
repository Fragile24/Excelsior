package engine

import (
	"excelsior/internal/chat"
	"excelsior/pkg/protocol"
)

func canonicalWorkspace(path string) string {
	return chat.CanonicalWorkspace(path)
}

func deltaFromEvent(ev chat.Event) protocol.Delta {
	d := protocol.Delta{
		RunID:        ev.RunID,
		SessionID:    ev.SessionID,
		Type:         ev.Type,
		Text:         ev.Text,
		Reasoning:    ev.Reasoning,
		ToolName:     ev.ToolName,
		ToolCallID:   ev.ToolCallID,
		ToolArgs:     ev.ToolArgs,
		ToolResult:   ev.ToolResult,
		FinishReason: ev.FinishReason,
		Context:      ev.Context,
	}
	if ev.Usage != nil {
		d.PromptTokens = ev.Usage.PromptTokens
		d.CompletionTokens = ev.Usage.CompletionTokens
		d.TotalTokens = ev.Usage.TotalTokens
	}
	return d
}

func interactionEnvelope(pi *chat.PendingInteraction) (protocol.Envelope, bool) {
	switch {
	case pi.Kind == chat.InteractionPermission && pi.Permission != nil:
		return protocol.NewEnvelope(protocol.TypePermissionReq, protocol.PermissionReq{
			RunID: pi.RunID, InteractionID: pi.ID, SessionID: pi.SessionID,
			Tool: pi.Permission.Tool, FilePath: pi.Permission.FilePath, Preview: pi.Permission.Preview, Command: pi.Permission.Command,
		}), true
	case pi.Kind == chat.InteractionAsk && pi.Ask != nil:
		return protocol.NewEnvelope(protocol.TypeAskReq, protocol.AskReq{
			RunID: pi.RunID, InteractionID: pi.ID, SessionID: pi.SessionID,
			Question: pi.Ask.Question, Options: pi.Ask.Options,
		}), true
	}
	return protocol.Envelope{}, false
}

func sessionDataFromSnapshot(snap chat.Snapshot) protocol.SessionDataResp {
	data := protocol.SessionDataResp{
		ID:                    snap.SessionID,
		RunID:                 snap.RunID,
		Running:               snap.Running,
		Messages:              snap.Messages,
		Events:                make([]protocol.Delta, 0, len(snap.Events)),
		UnsavedAvailable:      snap.UnsavedAvailable,
		ProjectionUnavailable: snap.ProjectionUnavailable,
	}
	if o := snap.Outcome; o != nil {
		data.Outcome = &protocol.DoneResp{SessionID: o.SessionID, RunID: o.RunID, Status: o.Status, Persisted: o.Persisted, Error: o.Error, Code: o.Code}
	}
	for _, ev := range snap.Events {
		data.Events = append(data.Events, deltaFromEvent(ev))
	}
	if snap.Pending != nil {
		if env, ok := interactionEnvelope(snap.Pending); ok {
			data.Pending = &env
		}
	}
	return data
}
