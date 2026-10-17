package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"excelsior/internal/chat"

	"excelsior/pkg/protocol"
	"excelsior/pkg/session"
)

// Conn owns socket I/O and immutable, workspace-bound application subscriptions.
type Conn struct {
	hub         *Hub
	ws          *websocket.Conn
	send        chan []byte
	workspace   string
	mu          sync.RWMutex
	subs        map[string]*chat.Subscription
	queuedBytes atomic.Int64
	done        chan struct{}
	closeOnce   sync.Once
}

func newConn(hub *Hub, ws *websocket.Conn) *Conn {
	return &Conn{
		hub:       hub,
		ws:        ws,
		send:      make(chan []byte, 128),
		done:      make(chan struct{}),
		workspace: hub.Workspace(),
		subs:      make(map[string]*chat.Subscription),
	}
}

func (c *Conn) isClosed() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

func (c *Conn) currentWorkspace() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.workspace
}

func (c *Conn) sendEnvelope(env protocol.Envelope) {
	if !c.trySendEnvelope(env) {
		c.close()
	}
}

func (c *Conn) trySendEnvelope(env protocol.Envelope) bool {
	if env.Workspace == "" {
		env.Workspace = c.currentWorkspace()
	}
	if c.isClosed() {
		return false
	}
	b, err := json.Marshal(env)
	if err != nil {
		return false
	}
	if c.queuedBytes.Add(int64(len(b))) > chat.MaxSubscriptionBytes {
		c.queuedBytes.Add(-int64(len(b)))
		return false
	}
	select {
	case <-c.done:
		c.queuedBytes.Add(-int64(len(b)))
		return false
	case c.send <- b:
	default:
		c.queuedBytes.Add(-int64(len(b)))
		return false
	}
	return true
}

func (c *Conn) sendError(id, msg string) {
	c.sendEnvelope(protocol.NewEnvelopeWithID(id, protocol.TypeError, map[string]string{"error": msg, "code": "command_failed"}))
}

func (c *Conn) close() {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		subs := c.subs
		c.subs = make(map[string]*chat.Subscription)
		c.mu.Unlock()
		for _, sub := range subs {
			sub.Close()
		}
		close(c.done)
		if c.ws != nil {
			_ = c.ws.Close()
		}
	})
}

func (c *Conn) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.close()
	}()
	for {
		select {
		case <-c.done:
			if c.ws != nil {
				_ = c.ws.WriteMessage(websocket.CloseMessage, []byte{})
			}
			return
		case msg := <-c.send:
			c.queuedBytes.Add(-int64(len(msg)))
			if c.ws != nil {
				_ = c.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := c.ws.WriteMessage(websocket.TextMessage, msg); err != nil {
					return
				}
			}
		case <-ticker.C:
			if c.ws != nil {
				_ = c.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := c.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}
}

func (c *Conn) readPump(ctx context.Context) {
	defer c.close()
	if c.ws == nil {
		return
	}
	c.ws.SetReadLimit(1 << 20)
	_ = c.ws.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.ws.SetPongHandler(func(string) error {
		_ = c.ws.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, data, err := c.ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				c.hub.logger().Warn("ws read error", "err", err)
			}
			return
		}
		_ = c.ws.SetReadDeadline(time.Now().Add(60 * time.Second))

		var env protocol.Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			c.sendError("", fmt.Sprintf("bad envelope: %v", err))
			continue
		}
		if env.Ver != "" && env.Ver != protocol.Ver {
			c.sendError(env.ID, fmt.Sprintf("unsupported ver %q, want %q", env.Ver, protocol.Ver))
			continue
		}

		c.dispatchEnvelope(ctx, env)
	}
}

func (c *Conn) dispatchEnvelope(ctx context.Context, env protocol.Envelope) {
	switch env.Type {
	case protocol.TypeChatCancel:
		c.cancelTurn(env)
	case protocol.TypeChatCompact:
		c.compactTurn(env)
	case protocol.TypeChatReq:
		c.dispatchChat(ctx, env)
	case protocol.TypeAskResp:
		c.handleAskResp(env)
	case protocol.TypePermissionResp:
		c.handlePermissionResp(env)
	case protocol.TypeSessionList:
		c.handleSessionList(ctx, env)
	case protocol.TypeSessionData:
		c.handleSessionData(ctx, env)
	case protocol.TypeSessionCreate:
		c.handleSessionCreate(ctx, env)
	case protocol.TypeSessionDelete:
		c.handleSessionDelete(ctx, env)
	case protocol.TypeSessionRename:
		c.handleSessionRename(ctx, env)
	case protocol.TypeSessionSubscribe:
		c.handleSessionSubscribe(env, true)
	case protocol.TypeSessionUnsubscribe:
		c.handleSessionSubscribe(env, false)
	case "workspace.status":
		c.sendEnvelope(protocol.NewEnvelopeWithID(env.ID, "workspace.status", map[string]string{"branch": branchOf(ctx, c.currentWorkspace())}))
	case protocol.TypeWorkspaceSet:
		c.handleWorkspaceSet(ctx, env)
	case protocol.TypeWorktreeList:
		c.handleWorktreeList(ctx, env)
	case protocol.TypeWorktreeCreate:
		c.handleWorktreeCreate(ctx, env)
	case protocol.TypeSettingsGet:
		c.handleSettingsGet(ctx, env)
	case protocol.TypeSettingsSet:
		c.handleSettingsSet(ctx, env)
	case protocol.TypePing:
		c.sendEnvelope(protocol.NewEnvelope(protocol.TypePong, nil))
	default:
		c.sendError(env.ID, fmt.Sprintf("unknown type %q", env.Type))
	}
}

// dispatchChat reserves the session, subscribes the connection atomically, then
// launches execution — so no event (including fast failures) is ever missed.
func (c *Conn) dispatchChat(_ context.Context, env protocol.Envelope) {
	var req protocol.ChatReq
	if !c.decodePayload(env, &req, "chat.req") {
		return
	}
	if req.ReasoningEffort != "" && !req.ReasoningEffort.Valid() {
		c.sendCommandError(env.ID, fmt.Errorf("invalid reasoning effort %q", req.ReasoningEffort))
		return
	}
	if req.SessionID == "" {
		req.SessionID = newID()
	}
	workspace := c.currentWorkspace()
	coord := c.hub.Coordinator()

	handle, err := coord.ReserveTurn(workspace, req.SessionID, req.Messages...)
	if err != nil {
		c.sendCommandError(env.ID, err)
		return
	}

	c.snapshot(env, req.SessionID)

	go coord.ExecuteTurn(chat.StartCommand{
		Workspace:       workspace,
		SessionID:       req.SessionID,
		Model:           req.Model,
		ReasoningEffort: req.ReasoningEffort,
		Messages:        req.Messages,
		Compact:         req.Compact,
	}, handle)
}

func (c *Conn) cancelTurn(env protocol.Envelope) {
	var req struct {
		SessionID string `json:"sessionId"`
		RunID     string `json:"runId"`
	}
	if !c.decodePayload(env, &req, "chat.cancel") {
		return
	}
	if err := c.hub.Coordinator().Cancel(c.currentWorkspace(), req.SessionID, req.RunID); err != nil {
		c.sendCommandError(env.ID, err)
		return
	}
	c.sendEnvelope(protocol.NewEnvelopeWithID(env.ID, protocol.TypeChatCancel, map[string]any{"sessionId": req.SessionID, "runId": req.RunID, "accepted": true}))
}

func (c *Conn) compactTurn(env protocol.Envelope) {
	var req struct {
		SessionID string `json:"sessionId"`
		RunID     string `json:"runId"`
	}
	if !c.decodePayload(env, &req, "chat.compact") {
		return
	}
	if err := c.hub.Coordinator().Compact(c.currentWorkspace(), req.SessionID, req.RunID); err != nil {
		c.sendCommandError(env.ID, err)
		return
	}
	c.sendEnvelope(protocol.NewEnvelopeWithID(env.ID, protocol.TypeChatCompact, map[string]any{"sessionId": req.SessionID, "runId": req.RunID, "accepted": true}))
}

// snapshot sends a SessionData snapshot and registers the connection as a
// coordinator subscriber for the session.
func (c *Conn) snapshot(env protocol.Envelope, id string) {
	c.mu.RLock()
	existing := c.subs[id]
	c.mu.RUnlock()
	if existing != nil && existing.Workspace == c.currentWorkspace() {
		if err := c.hub.Coordinator().Resnapshot(existing, env.ID); err != nil {
			c.sendCommandError(env.ID, err)
		}
		return
	}
	sub, err := c.hub.Coordinator().Subscribe(c.currentWorkspace(), id, env.ID)
	if err != nil {
		c.sendCommandError(env.ID, err)
		return
	}
	c.mu.Lock()
	c.subs[id] = sub
	c.mu.Unlock()
	if c.isClosed() {
		sub.Close()
		return
	}
	go c.forwardSubscription(sub)
}
func (c *Conn) unsubscribeSession(id string) {
	c.mu.Lock()
	sub := c.subs[id]
	delete(c.subs, id)
	c.mu.Unlock()
	if sub != nil {
		sub.Close()
	}
}
func (c *Conn) handleAskResp(env protocol.Envelope) {
	var resp protocol.AskResp
	if !c.decodePayload(env, &resp, "ask.resp") {
		return
	}
	if err := c.hub.Coordinator().ReplyAsk(c.currentWorkspace(), resp.SessionID, resp.RunID, resp.InteractionID, resp.Selected, resp.Answer, resp.Label); err != nil {
		c.sendCommandError(env.ID, err)
		return
	}
	c.sendEnvelope(protocol.NewEnvelopeWithID(env.ID, protocol.TypeAskResp, map[string]bool{"accepted": true}))
}

func (c *Conn) handlePermissionResp(env protocol.Envelope) {
	var resp protocol.PermissionResp
	if !c.decodePayload(env, &resp, "permission.resp") {
		return
	}
	if err := c.hub.Coordinator().ReplyPermission(c.currentWorkspace(), resp.SessionID, resp.RunID, resp.InteractionID, resp.Approved); err != nil {
		c.sendCommandError(env.ID, err)
		return
	}
	c.sendEnvelope(protocol.NewEnvelopeWithID(env.ID, protocol.TypePermissionResp, map[string]bool{"accepted": true}))
}

func (c *Conn) forwardSubscription(sub *chat.Subscription) {
	for {
		u, err := sub.Next(context.Background())
		if err != nil {
			if errors.Is(err, chat.ErrSlowSubscriber) {
				c.close()
			}
			return
		}
		var env protocol.Envelope
		switch {
		case u.Snapshot != nil:
			env = protocol.NewEnvelopeWithID(u.RequestID, protocol.TypeSessionData, sessionDataFromSnapshot(*u.Snapshot))
		case u.Event != nil:
			env = protocol.NewEnvelope(protocol.TypeDelta, deltaFromEvent(*u.Event))
		case u.Interaction != nil:
			var ok bool
			env, ok = interactionEnvelope(u.Interaction)
			if !ok {
				continue
			}
		case u.Resolved != nil:
			env = protocol.NewEnvelope(protocol.TypeInteractionDone, map[string]string{"sessionId": u.Resolved.SessionID, "runId": u.Resolved.RunID, "interactionId": u.Resolved.ID})
		case u.Outcome != nil:
			env = protocol.NewEnvelope(protocol.TypeDone, u.Outcome)
		default:
			continue
		}
		env.Workspace = sub.Workspace
		c.mu.RLock()
		current := c.subs[sub.SessionID] == sub
		sent := true
		if current {
			sent = c.trySendEnvelope(env)
		}
		c.mu.RUnlock()
		if !sent {
			c.close()
			return
		}
	}
}
func (c *Conn) sendCommandError(id string, err error) {
	code := "command_failed"
	switch {
	case errors.Is(err, chat.ErrSessionBusy):
		code = "session_busy"
	case errors.Is(err, chat.ErrStaleInteraction):
		code = "stale_interaction"
	case errors.Is(err, chat.ErrRunNotFound):
		code = "run_not_found"
	case errors.Is(err, session.ErrStoreOwned):
		code = "store_owned"
	case errors.Is(err, chat.ErrEngineStopped):
		code = "engine_stopped"
	}
	c.sendEnvelope(protocol.NewEnvelopeWithID(id, protocol.TypeError, map[string]string{"code": code, "error": err.Error()}))
}
