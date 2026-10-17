# Excelsior — GoAI-powered coding agent (Go)

Library + CLI coding agent with first-class [DeepSeek](https://api.deepseek.com) support through [GoAI](https://goai.sh/). GoAI owns provider HTTP, SSE streaming, retry handling, and provider-specific message formats.

## Quick start

```bash
export DEEPSEEK_API_KEY=sk-...
go build -trimpath -ldflags "-s -w" -o excelsior ./cmd/excelsior
./excelsior "explain this repo"
./excelsior -m deepseek-v4-pro "refactor pkg/llm to add retries"
echo "add tests for pkg/tools" | ./excelsior
```

### Models

| model | usage |
|---|---|
| `deepseek-v4-flash` (default) | V4 Flash, fast, reasoning |
| `deepseek-v4-pro` | V4 Pro, reasoning |

Set via `-m` / `--model` or `DEEPSEEK_MODEL` env.

## Library

```go
import (
    "excelsior/pkg/agent"
    "excelsior/pkg/config"
    "excelsior/pkg/llm"
    "excelsior/pkg/tools"
)

cfg, err := config.FromEnv()
if err != nil {
    panic(err)
}
client := &llm.Client{APIKey: cfg.APIKey, Model: "deepseek-v4-flash"}
harness := agent.DefaultHarnessConfig()
ag := &agent.Agent{
    LLM:    client,
    Tools:  tools.WorkspaceTools(workspace),
    System: agent.DefaultSystemPrompt,
    Harness: &harness,
}
msg, err := ag.Run(ctx, agent.RunOptions{
    Messages: []llm.Message{{Role: "user", Content: "fix the bug in main.go"}},
    OnEvent: func(ev agent.StreamEvent) {
        // ev.Type: "text" | "reasoning" | "tool_start" | "tool_result" | "done"
    },
})
```

## Tools (core 8)

`read_file`, `write_file`, `edit_file`, `list_files`, `glob_files`, `grep_text`, `web_fetch`, and `bash` are the predefined workspace action set; `askQuestion` supports interactive runs. File operations are workspace-rooted, symlink-aware, and enforce read-before-write checks. The harness owns a fixed ReAct loop, limits tool observations to 24,000 characters, runs up to eight read-only tools concurrently, and diagnoses supported file edits.

The default harness profile implements T4/128k from [An Empirical Study of Harness Design for Coding Agents](https://arxiv.org/pdf/2609.20804): planning and predefined tools are enabled; stale tool observations are elided at 60% of the usable input allowance, and older complete model turns are summarized at 85% while the task preamble and a 30% recent window (at least two model turns) stay verbatim. T4 exposes `recall_event` for both elided observations and summarized turns. Full prompts are tokenized with pinned DeepSeek V4/V4.1 chat templates and BPE data, including tool and reasoning overhead. The total context window automatically reserves output space: by default, 128,000 total minus 16,384 output leaves 111,616 input tokens. See [the implementation map and research differences](docs/harness.md).

Choose another experimental profile with `--context-tier 0` through `--context-tier 4`, set the window with `--context-window`, disable planning with `--planning=false`, or use `--action-space bash`. The same settings can be supplied through `EXCELSIOR_CONTEXT_TIER`, `EXCELSIOR_CONTEXT_WINDOW`, `EXCELSIOR_PLANNING`, and `EXCELSIOR_ACTION_SPACE`.

Known DeepSeek models select their token encoding automatically. For a custom endpoint or model alias, set `--token-encoding deepseek-v4` or `--token-encoding deepseek-v4.1` to its actual prompt format (`EXCELSIOR_TOKEN_ENCODING` for the engine). Unknown formats return an error instead of falling back to estimates.

`update_plan` accepts a complete array of tasks with `content`, `status`, and `activeForm`. `bash` accepts `command`, `cwd`, and `timeout_seconds` (default 120, maximum 600), and reports exit status, captured streams, and timeouts. In ask mode, simple read commands are allowed, known destructive commands are denied, and other actions request approval; explicit allow/deny modes override this default policy.

## Architecture

One personal Go engine serves desktop and future mobile clients. Runs belong to the engine and survive client disconnects. Sessions use atomic workspace JSON files; access uses a single owner token. See [ARCHITECTURE.md](ARCHITECTURE.md) for protocol and lifecycle details.

```bash
./excelsior engine --workspace .
# In another terminal:
./excelsior --engine ws://localhost:17812/v1/ws --session my-chat "explain this repo"
```

Local desktop authentication is automatic. For another device, obtain the token with `excelsior engine token` and supply it in client Settings or `EXCELSIOR_ENGINE_TOKEN`. Use an encrypted private connection for remote access.

## Project layout

```text
cmd/excelsior   CLI and daemon startup
internal/app   shared agent construction
internal/chat  run ownership, ordered subscriptions and persistence
internal/permissions   permission policy
pkg/engine     authenticated WebSocket transport
pkg/protocol   shared message types
pkg/agent, pkg/llm, pkg/tools   agent, GoAI adapter, workspace tools
pkg/session    leased atomic JSON session storage
pkg/config, pkg/util   configuration and shared helpers
apps/electron  desktop shell and frontend
```

## Env

```
DEEPSEEK_API_KEY   required
DEEPSEEK_BASE_URL  default https://api.deepseek.com
DEEPSEEK_MODEL     default deepseek-v4-flash
EXCELSIOR_CONTEXT_TIER     default 4
EXCELSIOR_CONTEXT_WINDOW   default 128000
EXCELSIOR_TOKEN_ENCODING   infer known model; optional deepseek-v4 or deepseek-v4.1
EXCELSIOR_PLANNING         default true
EXCELSIOR_ACTION_SPACE     default predefined
EXCELSIOR_MAX_OUTPUT_TOKENS default 16384
EXCELSIOR_TEMPERATURE      default 0
EXCELSIOR_TOP_P             default 0.95
```

## Build

Requires Go 1.26 or newer. Tokenizer data is embedded; no Python, tokenizer service, or runtime download is needed.

```bash
go vet ./...
go build ./...
```

### Desktop verification

In apps/electron, run npm ci, npm test, npm run lint, and npm run pack. Packaging builds the matching native Go engine. Run node tests/installed-smoke.cjs with the unpacked executable path to exercise packaged startup, the mounted transcript, local test-provider persistence and shutdown. The smoke test uses temporary files and no real provider credentials.
