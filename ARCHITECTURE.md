# Architecture

One owner runs one Go engine on their computer. Desktop and future mobile clients control that engine; files, credentials for DeepSeek, tools, and chat history stay on the computer.

```text
Desktop / mobile / CLI -- WebSocket --> Go engine
                                         |-- GoAI / DeepSeek
                                         |-- workspace tools
                                         |-- atomic JSON sessions
```

## Ownership

- The transport-neutral internal/chat.Coordinator owns runs, keyed by canonical workspace path and session ID. There is one active turn per session across all connections. Different sessions may run concurrently.
- Connections authenticate, select a workspace, subscribe, and forward commands. Disconnecting does not cancel work. The engine captures the workspace at run creation, so later client navigation cannot redirect tools.
- One mutex orders registration, projection updates, snapshot enqueueing, events, terminal outcomes and interaction replies. Each subscriber has an immutable workspace and a bounded queue; callbacks never execute under this mutex. Tools run outside that mutex. One JSON store is reused per workspace. Rename/delete reject active sessions.
- The chat coordinator owns successful-turn persistence. Missing sessions start empty; corrupted or unreadable sessions fail without overwrite. Sessions are stored as atomic JSON files in the workspace.
- Session stores derive display titles from each loaded record while building list metadata, avoiding a second read of untitled conversations.
- GoAI owns provider HTTP, streaming, retries, and provider-specific message formats. `pkg/agent` owns the fixed ReAct loop so planning, action-space selection, context management, safety checks, diagnostics, and stuck detection are applied consistently.

## Protocol

The versioned envelope includes the canonical `workspace` on workspace-scoped responses and events. Every connection must authenticate before issuing commands; the handshake advertises the run lifecycle capability.

1. Within five seconds of connecting, send `auth` with `{token}`. No other operation is accepted first. The response includes `{ok:true,workspace,capabilities:["run-lifecycle-v1"]}`; clients require that capability. Tokens are never placed in URLs.
2. `workspace.set` acknowledges the canonical workspace, followed by `session.list`. This clears the connection's old subscriptions. Clients ignore queued events from another workspace.
3. `session.data` or `session.subscribe` returns a `session.data` snapshot and subscribes atomically. The snapshot contains `id`, `messages`, `running`, optional `runId`, current-turn `events`, optional `pending` interaction envelope, terminal `outcome`, `unsavedAvailable` and `projectionUnavailable`. Apply the snapshot before subsequent live events.
4. `chat.req` starts a run and broadcasts its initial snapshot, then deltas. Its request ID correlates the starting snapshot. Terminal outcome enqueueing and reservation release share one ordering boundary after persistence. Concurrent starts return a session-busy error.
5. Approval/question requests and replies carry `sessionId`, `runId`, and `interactionId`. First valid reply wins. `interaction.done` clears the prompt on every subscribed client. Stale replies fail.
6. `chat.cancel` takes `{sessionId,runId}`. Cancellation is separate from disconnecting. Desktop Escape cancels when no interaction dialog is open.

Slow subscribers are disconnected rather than losing events silently. Reconnect and request a snapshot to recover. Current-turn text fragments are coalesced in memory; no event database or durable replay log is introduced.

## Running and connecting

`excelsior engine` listens on `127.0.0.1:17812` by default. It creates a random 256-bit owner token in the OS user-config directory under `excelsior/owner-token`. The directory is restricted to the owner (Windows ACL / Unix permissions). `EXCELSIOR_TOKEN_FILE` can select a token file inside a dedicated configuration directory.

- `excelsior engine token` prints the token for manual pairing. Keep it private.
- `excelsior engine rotate-token` replaces it. Restart the engine and update clients to revoke previous access.
- Desktop obtains the token through its preload bridge only for its configured engine URL. Remote clients can paste a token into Settings; it is kept in browser session storage per URL.
- `excelsior --engine ws://localhost:17812/v1/ws --session my-chat "hello"` uses the daemon. `EXCELSIOR_ENGINE_TOKEN` selects a remote engine's token. Without `--engine`, CLI execution remains standalone. Engine configuration and permission policy govern remote execution.
- `--origin` sets exact allowed browser origins. Defaults cover packaged Electron (`null`) and development (`http://localhost:3000`). Native clients omit Origin but still authenticate.
- For remote access, run the engine independently and use an encrypted private network or a private TLS gateway with `wss://`. Bind an explicit private interface when needed. The engine itself serves HTTP/WebSocket; public exposure, TLS certificate management, and relay hosting are outside this change.

## Limits

OS leases enforce one process per workspace store, including reads and preparation. Shutdown cancels pre-commit work, drains commits, then closes subscriptions and leases; a timeout retains leases while workers remain. Runs survive client disconnects, not process restarts. Completed history survives restart; failed/canceled partial turns are not saved. There is no automatic run recovery, account system, or mobile UI.

For remote control, use an externally started engine. Closing Electron stops an engine it spawned, but never stops an external engine. Frontend assets remain separate from the Go binary.

## Operational budgets and desktop

Application subscriptions allow 128 updates and 16 MiB; transport output also allows 128 envelopes and 16 MiB. A run snapshot is capped at 4 MiB. Terminal retention allows 64 sessions and 32 MiB per coordinator. Eviction loses unsaved output; there is no durable partial-turn log. Every assembled provider request, including tools and arguments, is checked against a 600,000-byte budget before network execution.

Context windows count input plus reserved output tokens. The harness meters complete prompts with the configured model's published DeepSeek V4/V4.1 template and tokenizer, then compacts against the remaining input allowance. Summary calls reserve their own output space. The client repeats the input-plus-output check before generation. Unsupported model formats require an explicit matching token encoding or a custom provider counter; they do not use character estimates. See [token budgets](docs/harness.md#token-budgets).

Desktop builds the Go binary for each packaging target and loads it only from packaged resources. The default owned workspace is under userData; EXCELSIOR_WORKSPACE overrides it. EXCELSIOR_ENGINE_ADDR controls both listener and client endpoint; EXCELSIOR_ENGINE selects an external engine. Readiness requires authenticated capability negotiation. Native IPC checks the exact renderer URL and main frame. Shell cancellation cleans up descendants; edit reads are bounded and preserve file permissions.

Store I/O still uses the coordinator ordering mutex. Lock splitting, caches and frontend batching remain measurement-driven follow-up work. See [runtime flows](FLOWS.md) and [session runs](docs/runs.md).
