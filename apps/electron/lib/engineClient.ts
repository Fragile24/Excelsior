import {
  decodeMessage,
  RUN_CAPABILITY,
  type Commands,
  type ReasoningEffort,
  type ReplyType,
  type Responses,
  type WireMessage,
  type WorktreeInfo,
} from "./protocol";

export type ConnectionStatus =
  | "connecting"
  | "authenticating"
  | "synchronizing"
  | "connected"
  | "disconnected"
  | "error";
export class CommandError extends Error {
  constructor(
    message: string,
    public delivery: "unsent" | "uncertain" | "rejected",
    public code?: string,
  ) {
    super(message);
  }
}
type Pending = {
  resolve: (msg: WireMessage) => void;
  reject: (e: Error) => void;
  timer: ReturnType<typeof setTimeout>;
  expected: string;
};
export type EngineClientHandlers = {
  onMessage: (msg: WireMessage) => void;
  onSelection?: (id: string | null) => void;
  onStatus: (status: ConnectionStatus) => void;
  onError: (message: string) => void;
  getToken: (url: string) => Promise<string>;
  createSocket?: (url: string) => WebSocket;
};

// Socket mechanics and correlated commands; no React, browser storage or Electron APIs.
export class EngineClient {
  private ws: WebSocket | null = null;
  private attempts = 0;
  private timer: ReturnType<typeof setTimeout> | null = null;
  private handshakeTimer: ReturnType<typeof setTimeout> | null = null;
  private dead = false;
  private authed = false;
  private ready = false;
  private generation = 0;
  private sequence = 0;
  private scopeVersion = 0;
  private scopeGuard() {
    const generation = this.generation,
      scope = this.scopeVersion;
    return () => {
      if (
        this.dead ||
        generation !== this.generation ||
        scope !== this.scopeVersion
      )
        throw new CommandError(
          "Workspace or connection changed; review the session before continuing.",
          "unsent",
        );
    };
  }
  private pending = new Map<string, Pending>();
  private desiredWorkspace = "";
  private confirmedWorkspace = "";
  private subscriptions = new Map<string, Set<string>>();
  private activeSessions = new Map<string, string>();
  private synchronizing = false;

  constructor(
    private url: string,
    private handlers: EngineClientHandlers,
  ) {
    this.connect();
  }
  private status(status: ConnectionStatus) {
    if (!this.dead) this.handlers.onStatus(status);
  }
  private desired(): Set<string> {
    let set = this.subscriptions.get(this.confirmedWorkspace);
    if (!set) {
      set = new Set();
      this.subscriptions.set(this.confirmedWorkspace, set);
    }
    return set;
  }
  private async request<K extends keyof Commands>(
    type: K,
    payload: Commands[K],
    bootstrap = false,
    timeoutMs = 10000,
  ): Promise<Responses[ReplyType<K>]> {
    const ws = this.ws;
    if ((!this.ready && !bootstrap) || !this.authed || ws?.readyState !== 1) {
      throw new CommandError(
        "Engine is not ready; your command was not sent.",
        "unsent",
      );
    }
    const id = this.generation + ":" + ++this.sequence;
    const expected =
      type === "chat.req" || type === "session.subscribe"
        ? "session.data"
        : type;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(
          new CommandError(
            "Acknowledgement lost. Reconnecting to inspect the session; do not resend until you review it.",
            "uncertain",
          ),
        );
        ws.close();
      }, timeoutMs);
      this.pending.set(id, {
        resolve: (msg) => resolve(msg.payload as Responses[ReplyType<K>]),
        reject,
        timer,
        expected,
      });
      try {
        ws.send(JSON.stringify({ ver: "v1", id, type, payload }));
      } catch {
        clearTimeout(timer);
        this.pending.delete(id);
        reject(
          new CommandError(
            "Socket send failed; your command was not sent.",
            "unsent",
          ),
        );
        ws.close();
      }
    });
  }
  private rejectPending() {
    for (const p of this.pending.values()) {
      clearTimeout(p.timer);
      p.reject(
        new CommandError(
          "Connection lost before acknowledgement. Inspect the restored session before retrying.",
          "uncertain",
        ),
      );
    }
    this.pending.clear();
  }
  close() {
    this.dead = true;
    ++this.generation;
    this.authed = this.ready = false;
    if (this.timer) clearTimeout(this.timer);
    if (this.handshakeTimer) clearTimeout(this.handshakeTimer);
    this.rejectPending();
    this.ws?.close();
    this.ws = null;
  }
  private connect() {
    if (this.dead) return;
    const generation = ++this.generation;
    this.ready = this.authed = false;
    this.status("connecting");
    let ws: WebSocket;
    try {
      ws = (this.handlers.createSocket ?? ((url) => new WebSocket(url)))(
        this.url,
      );
    } catch (e) {
      this.status("error");
      this.retry();
      return;
    }
    this.ws = ws;
    const current = () =>
      !this.dead && this.ws === ws && this.generation === generation;
    this.handshakeTimer = setTimeout(() => {
      if (current()) ws.close();
    }, 10000);
    ws.onopen = async () => {
      if (!current()) return;
      this.status("authenticating");
      try {
        const token = await this.handlers.getToken(this.url);
        if (current() && ws.readyState === 1)
          ws.send(
            JSON.stringify({ ver: "v1", type: "auth", payload: { token } }),
          );
      } catch (e) {
        if (current()) {
          this.handlers.onError(String(e));
          ws.close();
        }
      }
    };
    ws.onmessage = (e: MessageEvent<string>) => {
      if (!current()) return;
      try {
        const msg = decodeMessage(e.data);
        if (msg.type === "auth") {
          if (
            !msg.payload.ok ||
            !msg.payload.capabilities.includes(RUN_CAPABILITY)
          )
            throw new Error(
              "Engine lacks the required run lifecycle capability; update the engine.",
            );
          if (this.authed)
            throw new Error("Unexpected authentication response");
          this.authed = true;
          if (this.handshakeTimer) clearTimeout(this.handshakeTimer);
          void this.synchronize(
            this.desiredWorkspace || msg.payload.workspace,
          ).catch((e) => {
            if (current()) {
              this.handlers.onError(String(e));
              this.status("error");
              ws.close();
            }
          });
          return;
        }
        const pending = msg.id ? this.pending.get(msg.id) : undefined;
        // A correlated response is consumed exactly once. Late acknowledgements
        // cannot apply mutations to a newer view.
        if (msg.id && !pending) return;
        if (pending && msg.type !== "error" && msg.type !== pending.expected)
          throw new Error("Unexpected command response");
        if (msg.type !== "error") this.handlers.onMessage(msg);
        if (msg.type === "done" && msg.workspace === this.confirmedWorkspace) {
          void this.request("session.list", {}, true).catch((error) => {
            if (current() && msg.workspace === this.confirmedWorkspace) this.handlers.onError(String(error));
          });
        }
        if (pending) {
          clearTimeout(pending.timer);
          this.pending.delete(msg.id!);
          if (msg.type === "error")
            pending.reject(
              new CommandError(msg.payload.error, "rejected", msg.payload.code),
            );
          else pending.resolve(msg);
        } else if (msg.type === "error")
          this.handlers.onError(msg.payload.error);
      } catch (e) {
        this.handlers.onError(String(e));
        this.status("error");
        ws.close();
      }
    };
    ws.onerror = () => {
      if (current()) this.status("error");
    };
    ws.onclose = () => {
      if (!current()) return;
      if (this.handshakeTimer) clearTimeout(this.handshakeTimer);
      this.ready = this.authed = false;
      this.synchronizing = false;
      this.rejectPending();
      this.status("disconnected");
      this.retry();
    };
  }
  private retry() {
    if (this.dead) return;
    this.timer = setTimeout(
      () => this.connect(),
      Math.min(1000 * 2 ** this.attempts++, 10000),
    );
  }
  private async synchronize(workspace: string) {
    const generation = this.generation;
    ++this.scopeVersion;
    const check = this.scopeGuard();
    this.synchronizing = true;
    this.ready = false;
    this.status("synchronizing");
    const reply = await this.request("workspace.set", { workspace }, true);
    check();
    this.confirmedWorkspace = this.desiredWorkspace = reply.workspace;
    await this.request("settings.get", {}, true);
    check();
    const list = await this.request("session.list", {}, true);
    check();
    if (!this.activeSessions.has(reply.workspace) && list.sessions.length) {
      this.activeSessions.set(reply.workspace, list.sessions[0].id);
      this.desired().add(list.sessions[0].id);
    }
    const active = this.activeSessions.get(reply.workspace);
    const ids = new Set([...(active ? [active] : []), ...this.desired()]);
    for (const id of ids) {
      await this.request("session.data", { id }, true);
      check();
    }
    if (generation !== this.generation || this.dead) return;
    this.handlers.onSelection?.(active ?? null);
    this.attempts = 0;
    this.ready = true;
    this.synchronizing = false;
    this.status("connected");
  }
  async setWorkspace(workspace: string): Promise<string> {
    if (!this.ready || this.synchronizing)
      throw new CommandError("Wait for engine synchronization.", "unsent");
    try {
      await this.synchronize(workspace);
      return this.confirmedWorkspace;
    } catch (e) {
      // A rejected selection leaves the old confirmed scope authoritative.
      if (e instanceof CommandError && e.delivery === "rejected") {
        this.ready = true;
        this.synchronizing = false;
        this.status("connected");
      }
      throw e;
    }
  }
  async listWorktrees(): Promise<WorktreeInfo[]> {
    return (await this.request("worktree.list", {})).worktrees;
  }
  createWorktree(branch: string): Promise<WorktreeInfo> {
    return this.request("worktree.create", { branch }, false, 150000);
  }
  async selectSession(id: string) {
    const check = this.scopeGuard();
    this.desired().add(id);
    const snap = await this.request("session.data", { id });
    check();
    this.activeSessions.set(this.confirmedWorkspace, id);
    this.handlers.onSelection?.(id);
    return snap;
  }
  async createSession(title = "New session") {
    const check = this.scopeGuard();
    const created = await this.request("session.create", { title });
    check();
    this.desired().add(created.id);
    this.activeSessions.set(this.confirmedWorkspace, created.id);
    await this.selectSession(created.id);
    check();
    await this.request("session.list", {});
    check();
    return created.id;
  }
  async startChat(text: string, model: string, sessionId: string | null, compact = false, reasoningEffort?: ReasoningEffort) {
    // Acknowledge creation before sending the prompt. Never infer its target
    // from a later UI selection or retry a potentially accepted prompt.
    const check = this.scopeGuard();
    const id = sessionId ?? (await this.createSession());
    check();
    this.desired().add(id);
    const snap = await this.request("chat.req", {
      sessionId: id,
      model,
      ...(reasoningEffort ? { reasoningEffort } : {}),
      messages: [{ role: "user", content: text }],
      ...(compact ? { compact: true } : {}),
    });
    return snap;
  }
  cancelRun(sessionId: string, runId: string) {
    return this.request("chat.cancel", { sessionId, runId });
  }
  compactRun(sessionId: string, runId: string) {
    return this.request("chat.compact", { sessionId, runId });
  }
  replyAsk(payload: Commands["ask.resp"]) {
    return this.request("ask.resp", payload);
  }
  replyPermission(payload: Commands["permission.resp"]) {
    return this.request("permission.resp", payload);
  }
  async deleteSession(id: string) {
    const check = this.scopeGuard();
    await this.request("session.delete", { id });
    check();
    this.desired().delete(id);
    if (this.activeSessions.get(this.confirmedWorkspace) === id) {
      this.activeSessions.delete(this.confirmedWorkspace);
      this.handlers.onSelection?.(null);
    }
    await this.request("session.unsubscribe", { id });
    check();
    await this.request("session.list", {});
  }
  async renameSession(id: string, title: string) {
    const check = this.scopeGuard();
    await this.request("session.rename", { id, title });
    check();
    await this.request("session.list", {});
  }
  setSettings(payload: Commands["settings.set"]) {
    return this.request("settings.set", payload);
  }
}
