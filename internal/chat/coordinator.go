package chat

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"excelsior/internal/permissions"
	"excelsior/pkg/agent"
	"excelsior/pkg/config"
	"excelsior/pkg/llm"
	"excelsior/pkg/session"
	"excelsior/pkg/tools"
)

var (
	ErrSessionBusy      = errors.New("session busy")
	ErrEngineStopped    = errors.New("engine stopped")
	ErrRunNotFound      = errors.New("run no longer active")
	ErrStaleInteraction = errors.New("interaction no longer pending")
	ErrSessionNotFound  = session.ErrSessionNotFound
	ErrCorruptedSession = session.ErrCorruptedSession
	ErrProjectionLimit  = errors.New("run projection exceeded 4 MiB; start a shorter turn")
)

const MaxProjectionBytes = 4 << 20
const MaxRetainedBytes = 32 << 20
const MaxRetainedSessions = 64

func CanonicalWorkspace(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path
}
func sessionKey(workspace, id string) string { return CanonicalWorkspace(workspace) + "\x00" + id }
func newID() string                          { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }

type activeRun struct {
	id, workspace, sessionID, state string
	ctx                             context.Context
	cancel                          context.CancelFunc
	done                            chan struct{}
	messages                        []llm.Message
	record                          session.Record
	events                          []Event
	pending                         *PendingInteraction
	response                        chan interactionResponse
	projectionBytes                 int
	projectionErr                   error
	compactRequests                 chan struct{}
	compactPending                  bool
}
type interactionResponse struct {
	PermissionApproved  bool
	AskSelected         int
	AskAnswer, AskLabel string
}
type StartCommand struct {
	Workspace, SessionID, Model string
	ReasoningEffort             llm.ReasoningEffort
	Messages                    []llm.Message
	Compact                     bool
}
type Config struct {
	NewAgent               func(model, workspace string) (agent.Runner, error)
	SessionStore           session.Store
	StoreFactory           func(workspace string) session.Store
	PermissionOverrideFunc func() config.PermissionMode
	PermissionHandler      tools.PermissionHandler
	QuestionHandler        tools.QuestionHandler
	Ephemeral              bool
	Logger                 *slog.Logger
}

// Coordinator owns execution and ordered delivery. The registry mutex is also
// the ordering boundary. Keep it until measured contention justifies a split.
type Coordinator struct {
	listWorkers            sync.WaitGroup
	mu                     sync.Mutex
	newAgent               func(string, string) (agent.Runner, error)
	sessionStore           session.Store
	storeFactory           func(string) session.Store
	permissionOverrideFunc func() config.PermissionMode
	permissionHandler      tools.PermissionHandler
	questionHandler        tools.QuestionHandler
	ephemeral              bool
	logger                 *slog.Logger
	runs                   map[string]*activeRun
	subs                   map[string]map[*Subscription]struct{}
	stores                 map[string]session.Store
	terminal               map[string]Snapshot
	retainedOrder          []string
	retainedBytes          int
	stopped                bool
}
type RunHandle struct {
	ID, Workspace, SessionID string
	Context                  context.Context
	Cancel                   context.CancelFunc
	Done                     chan struct{}
	run                      *activeRun
}

func NewCoordinator(cfg Config) *Coordinator {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Ephemeral {
		cfg.SessionStore = session.NewMemoryStore()
	}
	return &Coordinator{
		newAgent: cfg.NewAgent, sessionStore: cfg.SessionStore, storeFactory: cfg.StoreFactory,
		permissionOverrideFunc: cfg.PermissionOverrideFunc, permissionHandler: cfg.PermissionHandler,
		questionHandler: cfg.QuestionHandler, ephemeral: cfg.Ephemeral, logger: cfg.Logger,
		runs: map[string]*activeRun{}, subs: map[string]map[*Subscription]struct{}{},
		stores: map[string]session.Store{}, terminal: map[string]Snapshot{},
	}
}
func (c *Coordinator) effectivePermissionOverride() config.PermissionMode {
	if c.permissionOverrideFunc != nil {
		return c.permissionOverrideFunc()
	}
	return ""
}
func (c *Coordinator) Store(workspace string) session.Store {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.storeLocked(workspace)
}
func (c *Coordinator) storeLocked(workspace string) session.Store {
	workspace = CanonicalWorkspace(workspace)
	if c.sessionStore != nil {
		return c.sessionStore
	}
	if st := c.stores[workspace]; st != nil {
		return st
	}
	var st session.Store
	if c.storeFactory != nil {
		st = c.storeFactory(workspace)
	} else {
		st = session.NewDirStore(filepath.Join(workspace, ".excelsior", "sessions"))
	}
	c.stores[workspace] = st
	return st
}

// ReserveTurn atomically claims the single active run slot for
// (workspace, sessionID) and publishes its initial snapshot, without
// starting the agent. The caller must pass the returned handle to
// ExecuteTurn; if reservation fails no run is created.
//
// It validates incoming is non-empty, canonicalizes the workspace,
// rejects when stopped (ErrEngineStopped) or a run is already active
// (ErrSessionBusy), checks the store lease, loads or creates the
// session record, appends incoming to history (ErrProjectionLimit if
// over budget), inserts the run in "preparing" state, drops any stale
// terminal snapshot, and notifies existing subscribers before first delta.
func (c *Coordinator) ReserveTurn(workspace, sessionID string, incoming ...llm.Message) (*RunHandle, error) {
	if len(incoming) == 0 {
		return nil, errors.New("chat requires at least one message")
	}
	if sessionID == "" {
		sessionID = newID()
	}
	ws := CanonicalWorkspace(workspace)
	key := sessionKey(ws, sessionID)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return nil, ErrEngineStopped
	}
	if c.runs[key] != nil {
		return nil, ErrSessionBusy
	}
	st := c.storeLocked(ws)
	if err := session.CheckLease(st); err != nil {
		return nil, err
	}
	record, err := st.Load(sessionID)
	if err != nil && !errors.Is(err, session.ErrSessionNotFound) {
		return nil, err
	}
	if errors.Is(err, session.ErrSessionNotFound) {
		record = session.Record{ID: sessionID, CreatedAt: time.Now().UTC()}
		record.Title = (session.Record{Messages: incoming}).DisplayTitle()
		if err = st.Save(record); err != nil {
			return nil, err
		}
	}
	messages := cloneMessages(append(record.Messages, incoming...))
	if messageBytes(messages) > MaxProjectionBytes {
		return nil, ErrProjectionLimit
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := &activeRun{id: newID(), workspace: ws, sessionID: sessionID, ctx: ctx, cancel: cancel, done: make(chan struct{}), state: "preparing", record: record, messages: messages, projectionBytes: messageBytes(messages), compactRequests: make(chan struct{}, 1)}
	c.dropTerminalLocked(key)
	c.runs[key] = run
	// Existing subscribers must learn the new run before its first delta.
	snap := c.runSnapshot(run)
	c.publishLocked(key, Update{Snapshot: &snap})
	return &RunHandle{ID: run.id, Workspace: ws, SessionID: sessionID, Context: ctx, Cancel: func() { _ = c.Cancel(ws, sessionID, run.id) }, Done: run.done, run: run}, nil
}

// Run is the standalone CLI entry point. Fire-and-forget callers use
// ReserveTurn plus async ExecuteTurn instead.
func (c *Coordinator) Run(ctx context.Context, cmd StartCommand, onEvent func(Event)) (*agent.RunResult, Outcome, error) {
	h, err := c.ReserveTurn(cmd.Workspace, cmd.SessionID, cmd.Messages...)
	if err != nil {
		return nil, Outcome{}, err
	}
	sub, err := c.Subscribe(h.Workspace, h.SessionID, "")
	if err != nil {
		h.Cancel()
		_, o := c.ExecuteTurn(cmd, h)
		return nil, o, err
	}
	defer sub.Close()
	stop := context.AfterFunc(ctx, h.Cancel)
	defer stop()
	type completed struct {
		result  *agent.RunResult
		outcome Outcome
	}
	finished := make(chan completed, 1)
	go func() { r, o := c.ExecuteTurn(cmd, h); finished <- completed{r, o} }()
	for {
		u, err := sub.Next(context.Background())
		if err != nil {
			h.Cancel()
			return nil, Outcome{}, err
		}
		if u.Event != nil && onEvent != nil {
			onEvent(*u.Event)
		}
		if u.Outcome != nil {
			break
		}
	}
	result := <-finished
	if result.outcome.Status != OutcomeSucceeded {
		return result.result, result.outcome, fmt.Errorf("%s: %s", result.outcome.Code, result.outcome.Error)
	}
	return result.result, result.outcome, nil
}
func (c *Coordinator) ExecuteTurn(cmd StartCommand, h *RunHandle) (*agent.RunResult, Outcome) {
	run := h.run
	outcome := Outcome{SessionID: run.sessionID, RunID: run.id, Status: OutcomeFailed, Code: "runner_creation_failed"}
	var runner agent.Runner
	var err error
	if c.newAgent == nil {
		err = errors.New("no agent runner factory configured")
	} else {
		runner, err = c.newAgent(cmd.Model, run.workspace)
	}
	if err != nil {
		outcome.Error = err.Error()
		c.finish(run, outcome, nil)
		return nil, outcome
	}
	c.mu.Lock()
	run.state = "running"
	c.mu.Unlock()
	turnCtx := tools.WithPermissionHandler(run.ctx, func(ctx context.Context, rq tools.PermissionRequest) (tools.PermissionResponse, error) {
		if c.permissionHandler != nil {
			return c.permissionHandler(ctx, rq)
		}
		perm := permissions.Resolve(c.effectivePermissionOverride(), config.LoadSettings(run.workspace))
		if perm == config.PermissionAsk {
			perm = permissions.DefaultDecision(rq)
		}
		if perm == config.PermissionAllow {
			return tools.PermissionResponse{Approved: true}, nil
		}
		if perm == config.PermissionDeny {
			return tools.PermissionResponse{Approved: false}, nil
		}
		pi := PendingInteraction{ID: newID(), Kind: InteractionPermission, SessionID: run.sessionID, RunID: run.id, Permission: &PermissionData{Tool: rq.Tool, FilePath: rq.FilePath, Preview: rq.Preview, Command: rq.Command}}
		resp, err := c.waitInteraction(ctx, run, pi)
		return tools.PermissionResponse{Approved: resp.PermissionApproved}, err
	})
	turnCtx = tools.WithQuestionHandler(turnCtx, func(ctx context.Context, rq tools.AskRequest) (tools.AskResponse, error) {
		if c.questionHandler != nil {
			return c.questionHandler(ctx, rq)
		}
		pi := PendingInteraction{ID: newID(), Kind: InteractionAsk, SessionID: run.sessionID, RunID: run.id, Ask: &AskData{Question: rq.Question, Options: append([]string(nil), rq.Options...)}}
		resp, err := c.waitInteraction(ctx, run, pi)
		return tools.AskResponse{Selected: resp.AskSelected, Answer: resp.AskAnswer, Label: resp.AskLabel}, err
	})
	svc := Service{Runner: runner, Store: c.Store(run.workspace)}
	if c.ephemeral {
		svc.Store = nil
	}
	result, execErr := svc.RunPrepared(turnCtx, PreparedTurn{
		SessionID: run.sessionID, RunID: run.id, Messages: cloneMessages(run.messages), Record: run.record,
		ReasoningEffort: cmd.ReasoningEffort, Compact: cmd.Compact, CompactRequests: run.compactRequests,
		OnEvent: func(ev Event) { ev.SessionID = run.sessionID; ev.RunID = run.id; c.appendAndBroadcastEvent(run, ev) },
		OnPersist: func() error {
			c.mu.Lock()
			defer c.mu.Unlock()
			if err := run.ctx.Err(); err != nil {
				return err
			}
			run.state = "persisting"
			return nil
		},
	})
	c.mu.Lock()
	projectionErr := run.projectionErr
	persisting := run.state == "persisting"
	c.mu.Unlock()
	switch {
	case projectionErr != nil:
		outcome.Status = OutcomeFailed
		outcome.Code = "projection_limit"
		outcome.Error = projectionErr.Error()
	case errors.Is(execErr, ErrPersistenceFailed):
		outcome.Status = OutcomePersistenceFailed
		outcome.Code = "persistence_failed"
		outcome.Error = execErr.Error()
	case !persisting && run.ctx.Err() != nil:
		outcome.Status = OutcomeCanceled
		outcome.Code = "canceled"
		outcome.Error = "run canceled"
	case execErr != nil:
		outcome.Status = OutcomeFailed
		outcome.Code = "execution_failed"
		switch {
		case errors.Is(execErr, agent.ErrContextBudget):
			outcome.Code = "context_budget"
		case errors.Is(execErr, agent.ErrStepBudget):
			outcome.Code = "step_budget"
		case errors.Is(execErr, agent.ErrStuck):
			outcome.Code = "stuck"
		}
		outcome.Error = execErr.Error()
	default:
		outcome.Status = OutcomeSucceeded
		outcome.Persisted = !c.ephemeral
		outcome.Code = ""
	}
	c.finish(run, outcome, result)
	return result, outcome
}
func (c *Coordinator) waitInteraction(ctx context.Context, run *activeRun, pi PendingInteraction) (interactionResponse, error) {
	c.mu.Lock()
	if err := ctx.Err(); err != nil {
		c.mu.Unlock()
		return interactionResponse{}, err
	}
	run.state = "waiting_for_interaction"
	run.pending = &pi
	run.response = make(chan interactionResponse, 1)
	response := run.response
	c.publishLocked(sessionKey(run.workspace, run.sessionID), Update{Interaction: &pi})
	c.mu.Unlock()
	select {
	case resp := <-response:
		return resp, nil
	case <-ctx.Done():
		return interactionResponse{}, ctx.Err()
	}
}
func (c *Coordinator) appendAndBroadcastEvent(run *activeRun, ev Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.appendEventLocked(run, ev)
}

func (c *Coordinator) appendEventLocked(run *activeRun, ev Event) {
	if run.projectionErr != nil {
		return
	}
	ev = cloneEvent(ev)
	size := eventBytes(ev)
	if run.projectionBytes+size > MaxProjectionBytes {
		run.projectionErr = ErrProjectionLimit
		run.cancel()
		return
	}
	run.projectionBytes += size
	if ev.Context != nil && ev.Context.Reason == "manual" && ev.Context.State != "compacting" {
		run.compactPending = false
	}
	n := len(run.events)
	if n > 0 && (ev.Type == "text" || ev.Type == "reasoning") && run.events[n-1].Type == ev.Type {
		run.events[n-1].Text += ev.Text
		run.events[n-1].Reasoning += ev.Reasoning
	} else {
		run.events = append(run.events, ev)
	}
	c.publishLocked(sessionKey(run.workspace, run.sessionID), Update{Event: &ev})
}

// publishLocked fans one update out to every Subscription on the session key,
// copying into each queue. A slow client's full queue evicts only that client
// (ErrSlowSubscriber; it must Resnapshot) so one bad network never stalls the
// rest. Caller must hold c.mu: the mutex is the delivery ordering boundary.
func (c *Coordinator) publishLocked(key string, u Update) {
	for sub := range c.subs[key] {
		if !sub.enqueue(u) {
			delete(c.subs[key], sub)
			c.logger.Warn("subscriber overflow", "workspace", sub.Workspace, "session", sub.SessionID)
		}
	}
}
func (c *Coordinator) finish(run *activeRun, outcome Outcome, result *agent.RunResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := sessionKey(run.workspace, run.sessionID)
	snap := c.runSnapshot(run)
	snap.Running = false
	snap.Pending = nil
	snap.Outcome = &outcome
	snap.UnsavedAvailable = outcome.Status != OutcomeSucceeded
	if result != nil && (outcome.Status == OutcomeSucceeded || outcome.Status == OutcomePersistenceFailed) {
		snap.Messages = cloneMessages(withoutSystemMessages(result.Messages))
		// The transcript already contains generated text and tools. Keep context
		// telemetry so reconnects retain the last measured input and compactions.
		snap.Events = nil
		for _, event := range run.events {
			if event.Type == "context" || event.Type == "compact_queued" || event.Type == "generation" {
				if event.Type == "generation" {
					event.Text = "" // usage is retained; text is already in Messages
				}
				snap.Events = append(snap.Events, cloneEvent(event))
			}
		}
	}
	c.retainLocked(key, snap)
	c.publishLocked(key, Update{Outcome: &outcome})
	delete(c.runs, key)
	run.cancel()
	close(run.done)
	c.logger.Info("run finished", "workspace", run.workspace, "session", run.sessionID, "run", run.id, "status", outcome.Status, "persisted", outcome.Persisted)
}
func (c *Coordinator) Cancel(workspace, sessionID, runID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	run := c.runs[sessionKey(workspace, sessionID)]
	if run == nil || runID == "" || run.id != runID {
		return ErrRunNotFound
	}
	if run.state != "persisting" {
		run.cancel()
	}
	return nil
}

// Compact queues one request at a complete model/tool boundary. It never
// interrupts a provider call or mutates the persisted conversation transcript.
func (c *Coordinator) Compact(workspace, sessionID, runID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	run := c.runs[sessionKey(workspace, sessionID)]
	if run == nil || runID == "" || run.id != runID || run.ctx.Err() != nil || run.state == "persisting" {
		return ErrRunNotFound
	}
	if run.compactPending {
		return nil
	}
	var current *agent.ContextStatus
	for i := len(run.events) - 1; i >= 0; i-- {
		if run.events[i].Context != nil {
			current = run.events[i].Context
			break
		}
	}
	if current == nil || !current.CanCompact {
		return errors.New("no older context is available to compact yet")
	}
	if current.State == "compacting" {
		return errors.New("context compaction is already in progress")
	}
	run.compactPending = true
	run.compactRequests <- struct{}{}
	c.appendEventLocked(run, Event{SessionID: sessionID, RunID: runID, Type: "compact_queued"})
	return nil
}

// Subscribe registers one connection's cursor on a session. Many connections
// (desktop, mobile, CLI) share one session key; each gets its own Subscription
// with its own bounded queue, fed snapshot-first so late joiners and
// reconnects see current state before live deltas.
func (c *Coordinator) Subscribe(workspace, sessionID, requestID string) (*Subscription, error) {
	ws := CanonicalWorkspace(workspace)
	key := sessionKey(ws, sessionID)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return nil, ErrEngineStopped
	}
	snap, err := c.snapshotLocked(ws, sessionID)
	if err != nil {
		return nil, err
	}
	sub := &Subscription{Workspace: ws, SessionID: sessionID, queue: make(chan queuedUpdate, 128)}
	sub.unsubscribe = func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		delete(c.subs[key], sub)
		if len(c.subs[key]) == 0 {
			delete(c.subs, key)
		}
		sub.closeQueue()
	}
	if c.subs[key] == nil {
		c.subs[key] = map[*Subscription]struct{}{}
	}
	c.subs[key][sub] = struct{}{}
	if !sub.enqueue(Update{RequestID: requestID, Snapshot: &snap}) {
		delete(c.subs[key], sub)
		return nil, ErrSlowSubscriber
	}
	return sub, nil
}

// Resnapshot uses the existing queue so queued terminal events cannot be
// overtaken by a replacement subscription's initial snapshot.
func (c *Coordinator) Resnapshot(sub *Subscription, requestID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := sessionKey(sub.Workspace, sub.SessionID)
	if _, ok := c.subs[key][sub]; !ok {
		return ErrSlowSubscriber
	}
	snap, err := c.snapshotLocked(sub.Workspace, sub.SessionID)
	if err != nil {
		return err
	}
	if !sub.enqueue(Update{RequestID: requestID, Snapshot: &snap}) {
		delete(c.subs[key], sub)
		return ErrSlowSubscriber
	}
	return nil
}
func (c *Coordinator) Snapshot(workspace, sessionID string) (Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snapshotLocked(CanonicalWorkspace(workspace), sessionID)
}
func (c *Coordinator) runSnapshot(run *activeRun) Snapshot {
	snap := Snapshot{SessionID: run.sessionID, RunID: run.id, Running: true, Messages: cloneMessages(run.messages), Events: cloneEvents(run.events)}
	if run.pending != nil {
		p := *run.pending
		if p.Ask != nil {
			a := *p.Ask
			a.Options = append([]string(nil), a.Options...)
			p.Ask = &a
		}
		if p.Permission != nil {
			v := *p.Permission
			p.Permission = &v
		}
		snap.Pending = &p
	}
	return snap
}
func (c *Coordinator) snapshotLocked(ws, id string) (Snapshot, error) {
	key := sessionKey(ws, id)
	if run := c.runs[key]; run != nil {
		return c.runSnapshot(run), nil
	}
	if snap, ok := c.terminal[key]; ok {
		return cloneSnapshot(snap), nil
	}
	st := c.storeLocked(ws)
	if err := session.CheckLease(st); err != nil {
		return Snapshot{}, err
	}
	record, err := st.Load(id)
	if err != nil && !errors.Is(err, session.ErrSessionNotFound) {
		return Snapshot{}, err
	}
	// A missing terminal record means prior unsaved output is unavailable.
	return Snapshot{SessionID: id, Messages: cloneMessages(record.Messages)}, nil
}
func (c *Coordinator) dropTerminalLocked(key string) {
	if snap, ok := c.terminal[key]; ok {
		c.retainedBytes -= snapshotBytes(snap)
		delete(c.terminal, key)
	}
	for i, k := range c.retainedOrder {
		if k == key {
			c.retainedOrder = append(c.retainedOrder[:i], c.retainedOrder[i+1:]...)
			break
		}
	}
}
func (c *Coordinator) retainLocked(key string, snap Snapshot) {
	c.dropTerminalLocked(key)
	size := snapshotBytes(snap)
	if size > MaxProjectionBytes {
		snap.Messages = nil
		snap.Events = nil
		snap.UnsavedAvailable = false
		snap.ProjectionUnavailable = true
		size = snapshotBytes(snap)
	}
	for len(c.retainedOrder) > 0 && (len(c.retainedOrder) >= MaxRetainedSessions || c.retainedBytes+size > MaxRetainedBytes) {
		c.dropTerminalLocked(c.retainedOrder[0])
	}
	c.terminal[key] = snap
	c.retainedOrder = append(c.retainedOrder, key)
	c.retainedBytes += size
}
func (c *Coordinator) ReplyPermission(ws, id, runID, interactionID string, approved bool) error {
	return c.reply(ws, id, runID, interactionID, InteractionPermission, interactionResponse{PermissionApproved: approved})
}
func (c *Coordinator) ReplyAsk(ws, id, runID, interactionID string, selected int, answer, label string) error {
	return c.reply(ws, id, runID, interactionID, InteractionAsk, interactionResponse{AskSelected: selected, AskAnswer: answer, AskLabel: label})
}
func (c *Coordinator) reply(ws, id, runID, interactionID, kind string, resp interactionResponse) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := sessionKey(ws, id)
	run := c.runs[key]
	if run == nil || run.id != runID || run.pending == nil || run.pending.ID != interactionID || run.pending.Kind != kind || run.ctx.Err() != nil {
		return ErrStaleInteraction
	}
	if kind == InteractionAsk && (resp.AskSelected < -1 || resp.AskSelected >= len(run.pending.Ask.Options)) {
		return errors.New("invalid question selection")
	}
	pi := *run.pending
	run.pending = nil
	run.state = "running"
	c.publishLocked(key, Update{Resolved: &pi})
	run.response <- resp
	return nil
}
func (c *Coordinator) ListSessions(ws string) ([]session.SessionMeta, error) {
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return nil, ErrEngineStopped
	}
	st := c.storeLocked(ws)
	c.listWorkers.Add(1)
	c.mu.Unlock()
	defer c.listWorkers.Done()
	if err := session.CheckLease(st); err != nil {
		return nil, err
	}
	return st.List()
}
func (c *Coordinator) mutate(ws, id string, discardProjection bool, fn func(session.Store) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return ErrEngineStopped
	}
	key := sessionKey(ws, id)
	if c.runs[key] != nil {
		return ErrSessionBusy
	}
	st := c.storeLocked(ws)
	if err := session.CheckLease(st); err != nil {
		return err
	}
	if err := fn(st); err != nil {
		return err
	}
	if discardProjection {
		c.dropTerminalLocked(key)
	}
	return nil
}
func (c *Coordinator) CreateSession(ws, id, title string) error {
	return c.mutate(ws, id, true, func(s session.Store) error {
		return s.Save(session.Record{ID: id, Title: title})
	})
}
func (c *Coordinator) DeleteSession(ws, id string) error {
	return c.mutate(ws, id, true, func(s session.Store) error { return s.Delete(id) })
}
func (c *Coordinator) RenameSession(ws, id, title string) error {
	return c.mutate(ws, id, false, func(s session.Store) error {
		record, err := s.Load(id)
		if err != nil {
			return err
		}
		record.Title = title
		return s.Save(record)
	})
}

// Shutdown retains ownership on timeout; the owning executable must then exit.
func (c *Coordinator) Shutdown(ctx context.Context) error {
	started := time.Now()
	c.mu.Lock()
	c.stopped = true
	runs := make([]*activeRun, 0, len(c.runs))
	for _, r := range c.runs {
		runs = append(runs, r)
		if r.state != "persisting" {
			r.cancel()
		}
	}
	c.mu.Unlock()
	for _, r := range runs {
		select {
		case <-r.done:
		case <-ctx.Done():
			c.logger.Error("shutdown incomplete", "elapsed", time.Since(started))
			return fmt.Errorf("shutdown incomplete: %w", ctx.Err())
		}
	}
	listsDone := make(chan struct{})
	go func() { c.listWorkers.Wait(); close(listsDone) }()
	select {
	case <-listsDone:
	case <-ctx.Done():
		return fmt.Errorf("shutdown incomplete: %w", ctx.Err())
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, subs := range c.subs {
		for sub := range subs {
			sub.closeQueue()
		}
		delete(c.subs, key)
	}
	for key, st := range c.stores {
		if closer, ok := st.(interface{ Close() }); ok {
			closer.Close()
		}
		delete(c.stores, key)
	}
	if closer, ok := c.sessionStore.(interface{ Close() }); ok {
		closer.Close()
	}
	c.logger.Info("shutdown drained", "elapsed", time.Since(started))
	return nil
}
func (c *Coordinator) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = c.Shutdown(ctx)
}
