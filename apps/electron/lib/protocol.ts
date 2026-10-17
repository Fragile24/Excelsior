// Wire contract: pkg/protocol. Go fixtures exercise this decoder in npm test.
export const RUN_CAPABILITY = "run-lifecycle-v1";
export type Envelope<T = unknown> = {
  ver: "v1";
  workspace?: string;
  id?: string;
  type: string;
  payload: T;
};
export type Message = {
  role: string;
  content?: string;
  reasoning_content?: string;
  tool_call_id?: string;
  name?: string;
  tool_calls?: {
    id: string;
    type: string;
    function: { name: string; arguments: string };
  }[];
};
export type ReasoningEffort = "none" | "low" | "high" | "max";
export type ChatReq = {
  sessionId?: string;
  model: string;
  reasoningEffort?: ReasoningEffort;
  messages: Message[];
  compact?: boolean;
};
export type ContextStatus = {
  model: string;
  state: "ready" | "compacting" | "compacted" | "unchanged" | "error";
  reason: "automatic" | "manual";
  inputTokens: number;
  inputBudget: number;
  outputReserve: number;
  windowTokens: number;
  softLimit: number;
  hardLimit: number;
  tier: number;
  canCompact: boolean;
  beforeTokens: number;
  compactions: number;
  breakdown?: {
    system: number;
    tools: number;
    user: number;
    assistant: number;
  };
  error?: string;
};
export type Delta = {
  runId: string;
  sessionId: string;
  type: string;
  text?: string;
  reasoning?: string;
  toolName?: string;
  toolCallID?: string;
  toolArgs?: string;
  toolResult?: string;
  finishReason?: string;
  promptTokens?: number;
  completionTokens?: number;
  totalTokens?: number;
  context?: ContextStatus;
};
export type SessionUsage = {
  prompt: number;
  completion: number;
  total: number;
};
export type AskReq = {
  runId: string;
  interactionId: string;
  sessionId: string;
  question: string;
  options: string[];
};
export type AskResp = Pick<AskReq, "runId" | "interactionId" | "sessionId"> & {
  selected: number;
  answer: string;
  label: string;
};
export type PermissionReq = {
  runId: string;
  interactionId: string;
  sessionId: string;
  tool: string;
  filePath?: string;
  preview?: string;
  command?: string;
};
export type PermissionResp = Pick<
  PermissionReq,
  "runId" | "interactionId" | "sessionId"
> & { approved: boolean };
export type SessionInfo = {
  id: string;
  title: string;
  count: number;
  updatedAt?: string;
  branch?: string;
  added?: number;
  deleted?: number;
};
export type WorktreeInfo = {
  path: string;
  branch?: string;
  head?: string;
  main: boolean;
  current: boolean;
};
export type PermissionMode = "ask" | "allow" | "deny";
export type SettingsGetResp = { permission: PermissionMode };
export type SettingsSetResp = SettingsGetResp;
export type Outcome = {
  sessionId: string;
  runId: string;
  status: "succeeded" | "failed" | "canceled" | "persistence_failed";
  persisted: boolean;
  error?: string;
  code?: string;
};
export type Interaction =
  | (Envelope<AskReq> & { type: "ask.req" })
  | (Envelope<PermissionReq> & { type: "permission.req" });
export type Snapshot = {
  id: string;
  runId?: string;
  running: boolean;
  messages: Message[];
  events?: Delta[];
  pending?: Interaction;
  outcome?: Outcome;
  unsavedAvailable: boolean;
  projectionUnavailable: boolean;
};
export type Responses = {
  auth: { ok: boolean; workspace: string; capabilities: string[] };
  "workspace.set": { workspace: string };
  "workspace.status": { branch: string };
  "worktree.list": { worktrees: WorktreeInfo[] };
  "worktree.create": WorktreeInfo;
  "session.list": { sessions: SessionInfo[] };
  "session.data": Snapshot;
  "session.create": { id: string };
  "session.delete": { deleted: string };
  "session.rename": { id: string; title: string };
  "session.unsubscribe": { id: string };
  "chat.cancel": { sessionId: string; runId: string; accepted: boolean };
  "chat.compact": { sessionId: string; runId: string; accepted: boolean };
  "ask.resp": { accepted: boolean };
  "permission.resp": { accepted: boolean };
  "settings.get": SettingsGetResp;
  "settings.set": SettingsSetResp;
  delta: Delta;
  done: Outcome;
  "ask.req": AskReq;
  "permission.req": PermissionReq;
  "interaction.done": {
    sessionId: string;
    runId: string;
    interactionId: string;
  };
  error: { error: string; code: string; sessionId?: string; runId?: string };
  pong: null;
};
export type WireMessage = {
  [K in keyof Responses]: Envelope<Responses[K]> & { type: K };
}[keyof Responses];
export type Commands = {
  "workspace.set": { workspace: string };
  "workspace.status": Record<string, never>;
  "worktree.list": Record<string, never>;
  "worktree.create": { branch: string };
  "session.list": Record<string, never>;
  "settings.get": Record<string, never>;
  "settings.set": { permission?: PermissionMode };
  "session.create": { title?: string };
  "session.data": { id: string };
  "session.delete": { id: string };
  "session.rename": { id: string; title: string };
  "session.subscribe": { id: string };
  "session.unsubscribe": { id: string };
  "chat.req": ChatReq;
  "chat.cancel": { sessionId: string; runId: string };
  "chat.compact": { sessionId: string; runId: string };
  "ask.resp": AskResp;
  "permission.resp": PermissionResp;
};
export type ReplyType<K extends keyof Commands> = K extends
  "chat.req" | "session.subscribe"
  ? "session.data"
  : K extends keyof Responses
    ? K
    : never;

function object(v: unknown): Record<string, unknown> {
  if (!v || typeof v !== "object" || Array.isArray(v))
    throw new Error("Invalid engine object");
  return v as Record<string, unknown>;
}
function strings(p: Record<string, unknown>, keys: string[], optional = false) {
  for (const key of keys)
    if (!(optional && p[key] === undefined) && typeof p[key] !== "string")
      throw new Error("Invalid engine field: " + key);
}
function bools(p: Record<string, unknown>, keys: string[]) {
  for (const key of keys)
    if (typeof p[key] !== "boolean")
      throw new Error("Invalid engine flag: " + key);
}
function array(v: unknown): unknown[] {
  if (!Array.isArray(v)) throw new Error("Invalid engine array");
  return v;
}
function identity(p: Record<string, unknown>) {
  strings(p, ["sessionId", "runId"]);
}
function outcome(v: unknown) {
  const p = object(v);
  identity(p);
  bools(p, ["persisted"]);
  strings(p, ["error", "code"], true);
  if (
    !["succeeded", "failed", "canceled", "persistence_failed"].includes(
      String(p.status),
    )
  )
    throw new Error("Invalid run outcome");
  if (p.status === "succeeded" && p.persisted !== true)
    throw new Error("Persistent engine success was not saved");
}
function delta(v: unknown) {
  const p = object(v);
  identity(p);
  strings(p, ["type"]);
  strings(
    p,
    [
      "text",
      "reasoning",
      "toolName",
      "toolCallID",
      "toolArgs",
      "toolResult",
      "finishReason",
    ],
    true,
  );
  for (const k of ["promptTokens", "completionTokens", "totalTokens"])
    if (
      p[k] !== undefined &&
      (typeof p[k] !== "number" || !Number.isFinite(p[k]) || p[k] < 0)
    )
      throw new Error("Invalid token usage");
  if (p.type === "context" || p.context !== undefined) {
    const c = object(p.context);
    strings(c, ["model", "state", "reason"]);
    strings(c, ["error"], true);
    bools(c, ["canCompact"]);
    for (const key of ["inputTokens", "inputBudget", "outputReserve", "windowTokens", "softLimit", "hardLimit", "tier", "beforeTokens", "compactions"])
      if (!Number.isSafeInteger(c[key]) || Number(c[key]) < 0) throw new Error("Invalid context count: " + key);
    if (c.breakdown !== undefined) {
      const breakdown = object(c.breakdown);
      for (const key of ["system", "tools", "user", "assistant"])
        if (!Number.isSafeInteger(breakdown[key]) || Number(breakdown[key]) < 0) throw new Error("Invalid context breakdown: " + key);
      if ([breakdown.system, breakdown.tools, breakdown.user, breakdown.assistant].reduce<number>((sum, value) => sum + Number(value), 0) !== Number(c.inputTokens))
        throw new Error("Context breakdown must match input tokens");
    }
    if (Number(c.inputBudget) <= 0 || Number(c.outputReserve) <= 0 ||
        Number(c.inputBudget) + Number(c.outputReserve) !== c.windowTokens ||
        Number(c.softLimit) > Number(c.hardLimit) || Number(c.hardLimit) > Number(c.inputBudget) ||
        Number(c.tier) > 4 ||
        !["ready", "compacting", "compacted", "unchanged", "error"].includes(String(c.state)) ||
        !["automatic", "manual"].includes(String(c.reason))) throw new Error("Invalid context budget");
  }
}
function interaction(type: string, v: unknown) {
  const p = object(v);
  identity(p);
  strings(p, ["interactionId"]);
  if (type === "ask.req") {
    strings(p, ["question"]);
    if (p.options === null) p.options = [];
    for (const x of array(p.options))
      if (typeof x !== "string") throw new Error("Invalid option");
  } else {
    strings(p, ["tool"]);
    strings(p, ["filePath", "preview", "command"], true);
  }
}
function snapshot(v: unknown) {
  const p = object(v);
  strings(p, ["id"]);
  strings(p, ["runId"], true);
  bools(p, ["running", "unsavedAvailable", "projectionUnavailable"]);
  if (p.running && !p.runId)
    throw new Error("Running snapshot lacks run identity");
  for (const v of array(p.messages)) {
    const m = object(v);
    strings(m, ["role"]);
    strings(m, ["content", "reasoning_content", "tool_call_id", "name"], true);
    if (!["user", "assistant", "system", "tool"].includes(String(m.role)))
      throw new Error("Invalid message role");
    for (const v of m.tool_calls === undefined ? [] : array(m.tool_calls)) {
      const t = object(v);
      strings(t, ["id", "type"]);
      strings(object(t.function), ["name", "arguments"]);
    }
  }
  for (const e of p.events === undefined ? [] : array(p.events)) {
    delta(e);
    const d = object(e);
    if (d.sessionId !== p.id || d.runId !== p.runId)
      throw new Error("Snapshot event scope mismatch");
  }
  if (p.outcome) {
    outcome(p.outcome);
    const o = object(p.outcome);
    if (o.sessionId !== p.id || o.runId !== p.runId)
      throw new Error("Outcome scope mismatch");
  }
  if (p.pending) {
    const e = object(p.pending);
    if (e.type !== "ask.req" && e.type !== "permission.req")
      throw new Error("Invalid pending interaction");
    interaction(e.type, e.payload);
    const i = object(e.payload);
    if (i.sessionId !== p.id || i.runId !== p.runId || !p.running)
      throw new Error("Interaction scope mismatch");
  }
}
export function decodeMessage(raw: string): WireMessage {
  if (raw.length > 32 * 1024 * 1024) throw new Error("Engine frame too large");
  const e = object(JSON.parse(raw));
  strings(e, ["type"]);
  strings(e, ["id", "workspace"], true);
  if (e.ver !== "v1") throw new Error("Unsupported engine protocol");
  if (e.type === "pong") return e as WireMessage;
  const p = object(e.payload);
  if (e.type !== "auth" && typeof e.workspace !== "string")
    throw new Error("Missing workspace scope");
  switch (e.type) {
    case "auth":
      bools(p, ["ok"]);
      strings(p, ["workspace"]);
      for (const c of array(p.capabilities))
        if (typeof c !== "string") throw new Error("Invalid capability");
      break;
    case "workspace.set":
      strings(p, ["workspace"]);
      break;
    case "workspace.status":
      strings(p, ["branch"]);
      break;
    case "worktree.list":
      for (const v of array(p.worktrees)) {
        const worktree = object(v);
        strings(worktree, ["path"]);
        strings(worktree, ["branch", "head"], true);
        bools(worktree, ["main", "current"]);
      }
      break;
    case "worktree.create":
      strings(p, ["path"]);
      strings(p, ["branch", "head"], true);
      bools(p, ["main", "current"]);
      break;
    case "session.list":
      for (const v of array(p.sessions)) {
        const s = object(v);
        strings(s, ["id", "title"]);
        strings(s, ["updatedAt", "branch"], true);
        if (typeof s.count !== "number") throw new Error("Invalid count");
      }
      break;
    case "session.data":
      snapshot(p);
      break;
    case "session.create":
    case "session.unsubscribe":
      strings(p, ["id"]);
      break;
    case "session.delete":
      strings(p, ["deleted"]);
      break;
    case "session.rename":
      strings(p, ["id", "title"]);
      break;
    case "settings.get":
    case "settings.set":
      strings(p, ["permission"]);
      break;
    case "chat.cancel":
    case "chat.compact":
      identity(p);
      bools(p, ["accepted"]);
      break;
    case "ask.resp":
    case "permission.resp":
      bools(p, ["accepted"]);
      break;
    case "delta":
      delta(p);
      break;
    case "done":
      outcome(p);
      break;
    case "ask.req":
    case "permission.req":
      interaction(e.type, p);
      break;
    case "interaction.done":
      identity(p);
      strings(p, ["interactionId"]);
      break;
    case "error":
      strings(p, ["error", "code"]);
      strings(p, ["sessionId", "runId"], true);
      break;
    default:
      throw new Error("Unknown engine message: " + e.type);
  }
  return e as WireMessage;
}
