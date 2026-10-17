package engine

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"excelsior/internal/chat"
	"excelsior/internal/permissions"
	"excelsior/pkg/config"
	"excelsior/pkg/protocol"
)

// decodePayload unmarshals envelope payload into v, reporting error via sendError on failure.
func (c *Conn) decodePayload(env protocol.Envelope, v any, label string) bool {
	if err := env.Decode(v); err != nil {
		c.sendError(env.ID, fmt.Sprintf("bad %s: %v", label, err))
		return false
	}
	return true
}

// branchOf returns the current git branch of dir, or "" if not a git repo.
// ponytail: one branch per workspace (current HEAD), not the branch at session creation; persist per-session if that matters later.
func branchOf(ctx context.Context, dir string) string {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// handleSessionList returns basic session metadata plus the workspace's
// current branch. Working-tree analysis is deliberately not part of the list:
// it cannot establish which session caused an edit, and scanning histories
// for it costs a full read of every session file.
func (c *Conn) handleSessionList(ctx context.Context, env protocol.Envelope) {
	metas, err := c.hub.Coordinator().ListSessions(c.currentWorkspace())
	if err != nil {
		c.sendError(env.ID, fmt.Sprintf("list sessions: %v", err))
		return
	}

	sessionsList := make([]protocol.SessionInfo, 0, len(metas))
	for _, meta := range metas {
		sessionsList = append(sessionsList, protocol.SessionInfo{
			ID:    meta.ID,
			Title: meta.Title,
			Count: meta.MsgCount,
		})
	}
	c.sendEnvelope(protocol.NewEnvelopeWithID(env.ID, protocol.TypeSessionList, protocol.SessionListResp{Sessions: sessionsList}))
}

func (c *Conn) handleSessionData(ctx context.Context, env protocol.Envelope) {
	var req protocol.SessionDataReq
	if !c.decodePayload(env, &req, "session.data") {
		return
	}
	c.snapshot(env, req.ID)
}

func (c *Conn) handleSessionCreate(ctx context.Context, env protocol.Envelope) {
	var req protocol.SessionCreateReq
	if !c.decodePayload(env, &req, "session.create") {
		return
	}
	id := newID()
	if err := c.hub.Coordinator().CreateSession(c.currentWorkspace(), id, req.Title); err != nil {
		c.sendError(env.ID, fmt.Sprintf("create session: %v", err))
		return
	}
	c.sendEnvelope(protocol.NewEnvelopeWithID(env.ID, protocol.TypeSessionCreate, protocol.SessionCreateResp{ID: id}))
}

func (c *Conn) handleSessionDelete(ctx context.Context, env protocol.Envelope) {
	var req protocol.SessionDeleteReq
	if !c.decodePayload(env, &req, "session.delete") {
		return
	}
	if err := c.hub.Coordinator().DeleteSession(c.currentWorkspace(), req.ID); err != nil {
		if errors.Is(err, chat.ErrSessionBusy) {
			c.sendCommandError(env.ID, chat.ErrSessionBusy)
			return
		}
		c.sendError(env.ID, fmt.Sprintf("delete session: %v", err))
		return
	}
	c.sendEnvelope(protocol.NewEnvelopeWithID(env.ID, protocol.TypeSessionDelete, map[string]string{"deleted": req.ID}))
}

func (c *Conn) handleSessionRename(ctx context.Context, env protocol.Envelope) {
	var req protocol.SessionRenameReq
	if !c.decodePayload(env, &req, "session.rename") {
		return
	}
	if err := c.hub.Coordinator().RenameSession(c.currentWorkspace(), req.ID, req.Title); err != nil {
		if errors.Is(err, chat.ErrSessionBusy) {
			c.sendError(env.ID, "session busy")
			return
		}
		c.sendError(env.ID, fmt.Sprintf("rename session: %v", err))
		return
	}
	c.sendEnvelope(protocol.NewEnvelopeWithID(env.ID, protocol.TypeSessionRename, protocol.SessionInfo{ID: req.ID, Title: req.Title}))
}

func (c *Conn) handleSessionSubscribe(env protocol.Envelope, subscribe bool) {
	var req protocol.SessionSubscriptionReq
	if !c.decodePayload(env, &req, "session subscription") {
		return
	}
	if subscribe {
		c.snapshot(env, req.ID)
		return
	}
	c.unsubscribeSession(req.ID)
	c.sendEnvelope(protocol.NewEnvelopeWithID(env.ID, env.Type, map[string]string{"id": req.ID}))
}

func (c *Conn) handleWorkspaceSet(ctx context.Context, env protocol.Envelope) {
	var req protocol.WorkspaceSetReq
	if !c.decodePayload(env, &req, "workspace.set") {
		return
	}
	if target := strings.TrimSpace(req.Workspace); target != "" {
		resolved, err := config.ResolveWorkspace(target, "")
		if err != nil {
			c.sendError(env.ID, err.Error())
			return
		}
		c.mu.Lock()
		subs := c.subs
		c.subs = make(map[string]*chat.Subscription)
		c.workspace = canonicalWorkspace(resolved)
		c.mu.Unlock()
		for _, sub := range subs {
			sub.Close()
		}
		c.hub.logger().Info("switched workspace (per-conn)", "workspace", target)
	}
	c.sendEnvelope(protocol.NewEnvelopeWithID(env.ID, protocol.TypeWorkspaceSet, protocol.WorkspaceSetReq{Workspace: c.currentWorkspace()}))
	c.handleSessionList(ctx, env)
}

func (c *Conn) handleWorktreeList(ctx context.Context, env protocol.Envelope) {
	worktrees, err := listWorktrees(ctx, c.currentWorkspace())
	if err != nil {
		c.sendError(env.ID, fmt.Sprintf("list worktrees: %v", err))
		return
	}
	c.sendEnvelope(protocol.NewEnvelopeWithID(env.ID, protocol.TypeWorktreeList, protocol.WorktreeListResp{Worktrees: worktrees}))
}

func (c *Conn) handleWorktreeCreate(ctx context.Context, env protocol.Envelope) {
	var req protocol.WorktreeCreateReq
	if !c.decodePayload(env, &req, "worktree.create") {
		return
	}
	worktree, err := createWorktree(ctx, c.currentWorkspace(), req.Branch)
	if err != nil {
		c.sendError(env.ID, fmt.Sprintf("create worktree: %v", err))
		return
	}
	c.sendEnvelope(protocol.NewEnvelopeWithID(env.ID, protocol.TypeWorktreeCreate, worktree))
}

func (c *Conn) handleSettingsGet(ctx context.Context, env protocol.Envelope) {
	s := config.LoadSettings(c.currentWorkspace())
	perm := permissions.Resolve(c.hub.PermissionOverride, s)
	c.sendEnvelope(protocol.NewEnvelopeWithID(env.ID, protocol.TypeSettingsGet, protocol.SettingsGetResp{Permission: string(perm)}))
}

func (c *Conn) handleSettingsSet(ctx context.Context, env protocol.Envelope) {
	var req protocol.SettingsSetReq
	if !c.decodePayload(env, &req, "settings.set") {
		return
	}
	s := config.LoadSettings(c.currentWorkspace())
	updated := false
	if req.Permission != nil {
		pm, err := config.ParsePermissionMode(*req.Permission)
		if err != nil {
			c.sendError(env.ID, err.Error())
			return
		}
		s.Permission = pm
		updated = true
	}
	if updated {
		if err := config.SaveSettings(c.currentWorkspace(), s); err != nil {
			c.sendError(env.ID, fmt.Sprintf("save settings: %v", err))
			return
		}

		c.hub.logger().Info("settings updated", "permission", s.Permission)
	}
	perm := permissions.Resolve(c.hub.PermissionOverride, s)
	c.sendEnvelope(protocol.NewEnvelopeWithID(env.ID, protocol.TypeSettingsSet, protocol.SettingsSetResp{Permission: string(perm)}))
}
