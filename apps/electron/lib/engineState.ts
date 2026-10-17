import type {
  ContextStatus, Delta, Interaction, Message, Outcome, PermissionMode, SessionInfo,
  SessionUsage, Snapshot, WireMessage,
} from "./protocol";
import type { ConnectionStatus } from "./engineClient";

export type Block = {
  role: "user" | "assistant" | "reason" | "tool" | "error" | "context";
  content: string;
  meta?: string;
  args?: string;
  callId?: string;
  runId?: string;
  context?: ContextStatus;
};
export type BlockRole = Block["role"];
export type EngineStatus = ConnectionStatus;
export const NO_USAGE: SessionUsage = { prompt: 0, completion: 0, total: 0 };
export type Conversation = {
  blocks: Block[];
  running: boolean;
  runId: string | null;
  usage: SessionUsage;
  pending: Interaction | null;
  outcome: Outcome | null;
  context: ContextStatus | null;
  compactQueued: boolean;
  compactNext: boolean;
};
const EMPTY_CONVERSATION: Conversation = {
  blocks: [], running: false, runId: null, usage: NO_USAGE, pending: null, outcome: null,
  context: null, compactQueued: false, compactNext: false,
};
export type EngineState = {
  status: ConnectionStatus;
  workspace: string | null;
  sessions: SessionInfo[];
  conversations: Record<string, Conversation>;
  activeId: string | null;
  permissionMode: PermissionMode;
  error: string | null;
};
export function initialEngineState(): EngineState {
  return {
    status: "disconnected", workspace: null, sessions: [], conversations: {},
    activeId: null, permissionMode: "ask", error: null,
  };
}
export function activeConversation(state: EngineState): Conversation {
  return state.conversations[state.activeId ?? ""] ?? EMPTY_CONVERSATION;
}
export type EngineAction =
  | { type: "reset" }
  | { type: "status"; status: ConnectionStatus }
  | { type: "active"; id: string | null }
  | { type: "error"; message: string | null }
  | { type: "compact-next"; workspace: string | null; id: string; enabled: boolean }
  | { type: "server"; msg: WireMessage };

export function reduce(state: EngineState, action: EngineAction): EngineState {
  switch (action.type) {
    case "reset": return initialEngineState();
    case "status": return { ...state, status: action.status };
    case "active": return { ...state, activeId: action.id };
    case "error": return { ...state, error: action.message };
    case "compact-next": {
      const current = state.conversations[action.id];
      return action.workspace === state.workspace && current && !current.running
        ? withConversation(state, action.id, { ...current, compactNext: action.enabled }) : state;
    }
    case "server": return reduceServer(state, action.msg);
  }
}

function withConversation(state: EngineState, id: string, value: Conversation): EngineState {
  if (state.conversations[id] === value) return state;
  return { ...state, conversations: { ...state.conversations, [id]: value } };
}

function reduceServer(state: EngineState, msg: WireMessage): EngineState {
  if (msg.type === "auth") return state;
  if (msg.type === "workspace.set") {
    return msg.payload.workspace === state.workspace ? state
      : { ...initialEngineState(), status: state.status, workspace: msg.payload.workspace };
  }
  if (msg.workspace !== state.workspace) return state;
  switch (msg.type) {
    case "settings.get":
    case "settings.set":
      return { ...state, permissionMode: msg.payload.permission };
    case "session.list":
      return { ...state, sessions: msg.payload.sessions };
    case "session.data": {
      const next = fromSnapshot(msg.payload);
      next.compactNext = !next.running && (state.conversations[msg.payload.id]?.compactNext ?? false);
      return withConversation(state, msg.payload.id, next);
    }
    case "session.delete": {
      const id = msg.payload.deleted;
      const conversations = { ...state.conversations };
      delete conversations[id];
      return {
        ...state, conversations,
        sessions: state.sessions.filter((s) => s.id !== id),
        activeId: state.activeId === id ? null : state.activeId,
      };
    }
    case "error":
      return { ...state, error: msg.payload.error };
    case "delta":
    case "done":
    case "ask.req":
    case "permission.req":
    case "interaction.done": {
      const p = msg.payload;
      const current = state.conversations[p.sessionId];
      if (!current || current.runId !== p.runId) return state;
      switch (msg.type) {
        case "delta":
          return current.running ? withConversation(state, p.sessionId, applyDelta(current, msg.payload)) : state;
        case "done":
          return withConversation(state, p.sessionId, applyOutcome(current, msg.payload));
        case "ask.req":
        case "permission.req":
          return current.running ? withConversation(state, p.sessionId, { ...current, pending: msg }) : state;
        case "interaction.done":
          return current.pending?.payload.interactionId === msg.payload.interactionId
            ? withConversation(state, p.sessionId, { ...current, pending: null }) : state;
      }
    }
    default:
      return state;
  }
}

function snapshotBlocks(messages: Message[]): Block[] {
  const args = new Map<string, string>();
  return messages.flatMap((m): Block[] => {
    for (const call of m.tool_calls ?? []) args.set(call.id, call.function.arguments);
    if (m.role === "system") return [];
    if (m.role === "tool") {
      return [{ role: "tool", content: m.content ?? "", meta: m.name, args: args.get(m.tool_call_id ?? ""), callId: m.tool_call_id }];
    }
    if (m.role === "user") return [{ role: "user", content: m.content ?? "" }];
    const blocks: Block[] = [];
    if (m.reasoning_content) blocks.push({ role: "reason", content: m.reasoning_content });
    if (m.content) blocks.push({ role: "assistant", content: m.content });
    return blocks;
  });
}

function fromSnapshot(p: Snapshot): Conversation {
  let current: Conversation = {
    blocks: snapshotBlocks(p.messages), running: p.running, runId: p.runId ?? null,
    usage: NO_USAGE, pending: p.pending ?? null, outcome: null,
    context: null, compactQueued: false, compactNext: false,
  };
  for (const event of p.events ?? []) current = applyDelta(current, event);
  if (p.outcome) current = applyOutcome(current, p.outcome);
  if (p.projectionUnavailable) {
    current = { ...current, blocks: [...current.blocks, { role: "error", content: "Unsaved output is no longer available." }] };
  }
  return current;
}

function applyOutcome(current: Conversation, outcome: Outcome): Conversation {
  if (current.outcome?.runId === outcome.runId) return current;
  let blocks: Block[] = outcome.status === "succeeded" ? current.blocks : [
    ...current.blocks,
    {
      role: "error",
      content: outcome.status === "persistence_failed"
        ? "History was not saved. " + (outcome.error ?? "") : (outcome.error ?? outcome.status),
    },
  ];
  if (current.compactQueued) blocks = [...blocks, { role: "context", content: "Compaction was not applied", meta: "The run ended before the next model step. You can queue it for your next message." }];
  return { ...current, blocks, running: false, pending: null, compactQueued: false, outcome };
}

function appendBlock(blocks: Block[], block: Block): Block[] {
  const last = blocks[blocks.length - 1];
  return last?.role === block.role && last.meta === block.meta
    ? [...blocks.slice(0, -1), { ...last, content: last.content + block.content }]
    : [...blocks, block];
}

function applyDelta(current: Conversation, d: Delta): Conversation {
  if (d.type === "compact_queued") return { ...current, compactQueued: true };
  if (d.type === "context" && d.context) {
    const context = d.context;
    const finished = context.state === "compacted" || (context.state === "unchanged" && context.reason === "manual");
    const block: Block = {
      role: "context", content: context.state === "compacted" ? "Context compacted" : "Context unchanged",
      context, runId: d.runId,
    };
    return {
      ...current, context,
      compactQueued: context.reason === "manual" && context.state !== "compacting" ? false : current.compactQueued,
      blocks: finished ? [...current.blocks, block] : current.blocks,
    };
  }
  if (d.type === "generation" || d.type === "done") {
    const u = current.usage;
    return {
      ...current,
      usage: {
        prompt: u.prompt + (d.promptTokens ?? 0),
        completion: u.completion + (d.completionTokens ?? 0),
        total: u.total + (d.totalTokens ?? (d.promptTokens ?? 0) + (d.completionTokens ?? 0)),
      },
    };
  }
  if (d.type === "tool_start" || d.type === "tool_result") {
    const blocks = [...current.blocks];
    const index = d.toolCallID
      ? blocks.findIndex((b) => b.role === "tool" && b.runId === d.runId && b.callId === d.toolCallID)
      : blocks.findLastIndex((b) => b.role === "tool" && b.runId === d.runId && b.meta === d.toolName && b.args === undefined);
    const previous = blocks[index];
    const block: Block = d.type === "tool_start"
      ? { role: "tool", meta: d.toolName, callId: d.toolCallID, runId: d.runId, content: (previous?.content ?? "") + (d.toolArgs ?? "") }
      : { role: "tool", meta: d.toolName, callId: d.toolCallID, runId: d.runId, args: previous?.content ?? "", content: d.toolResult ?? "" };
    if (index < 0) blocks.push(block);
    else blocks[index] = block;
    return { ...current, blocks };
  }
  let block: Block;
  switch (d.type) {
    case "text": block = { role: "assistant", content: d.text ?? "" }; break;
    case "reasoning": block = { role: "reason", content: d.reasoning ?? "" }; break;
    case "error": block = { role: "error", content: d.text ?? "" }; break;
    default: return current;
  }
  return block.content ? { ...current, blocks: appendBlock(current.blocks, block) } : current;
}
