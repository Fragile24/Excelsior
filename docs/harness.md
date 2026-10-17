# Coding harness

This implements the harness methods in [An Empirical Study of Harness Design for Coding Agents, arXiv:2609.20804v1](https://arxiv.org/pdf/2609.20804), sections 2–3.1 and appendices A–B, inside Excelsior's Go engine. The study evaluates harness choices; it does not introduce a training algorithm or new model weights.

## Execution

`pkg/agent/harness.go` owns one ReAct loop. Each step assembles context, makes one provider call, executes the requested tools, and appends the assistant message and every observation to the working history. A model turn means one model invocation together with all its tool results. It does not mean a whole user request.

The default limits follow section 3.1: 300 steps, 16,384 output tokens, temperature 0, top-p 0.95, 24,000 characters per ordinary tool observation, and up to eight independent read-only calls concurrently. Mutating calls form ordering barriers. Errors become tool observations, including captured output for unsuccessful shell commands. Five repeated identical calls trigger one reminder per streak; eight identical consecutive failures stop the run. Permission denials do not count as failures.

The full transcript is separate from model context. `RunResult` retains original observations and token usage even when execution fails. Summary calls contribute to usage. Engine outcomes distinguish `context_budget`, `step_budget`, and `stuck`; only successful turns are persisted.

## Context policies

`pkg/agent/context.go` preserves the system/task preamble and a recent tail budgeted at 30% of the usable input allowance, with at least two complete model turns. Assistant tool calls and their results are never split during compaction. All percentage thresholds below apply after output reservation.

| Tier | Elision | Filesystem recall | Running summary | Trigger |
| --- | --- | --- | --- | --- |
| T0 | No | No | No | Stops if input exceeds the window |
| T1 | Yes | No | No | 85% |
| T2 | Yes | Yes | No | 85% |
| T3 | No | No | Yes | 85% |
| T4 | Yes | Yes | Yes | Elision at 60%, summary at 85% if still needed |

Elision replaces bulky middle-region tool bodies with size stubs. T2 and T4 stubs contain `event_N` identifiers. T4 also archives the complete turns removed by summarization, restoring any previously elided bodies before archival. `recall_event({"id":"event_N"})` returns the archived content. IDs and their private temporary filesystem store belong to one run; the store is deleted when the run ends.

Summarization uses a separate tool-free call to the same provider and model, with the original task, previous summary, and selected oldest middle turns. Its headings follow appendix A.3: Goal, Files touched, Done, Pending, Errors & fixes, Current state, Next step. The updated summary is injected immediately after the preserved preamble. Plan reminders and summaries are never appended to the saved transcript.

## Planning and actions

Planning is an independent switch. The harness keeps the current plan in memory and injects it before each model call. `update_plan` replaces the whole list:

```json
{"plan":[
  {"content":"Inspect the parser","status":"completed","activeForm":"Inspecting the parser"},
  {"content":"Fix the parser","status":"in_progress","activeForm":"Fixing the parser"},
  {"content":"Check the result","status":"pending","activeForm":"Checking the result"}
]}
```

Unfinished plans must have exactly one task in progress; completed plans have none. Disabling planning removes its tool, system instructions, and reminders.

The predefined action set contains `read_file`, `write_file`, `edit_file`, `list_files`, `glob_files`, `grep_text`, `web_fetch`, and `bash`. File reads report ranges and full-read eligibility. Existing-file mutations require a complete delivered read, check the content hash, preview and return a unified diff, and update file state. Files are rechecked after permission waits. Python edits run ruff, pyflakes, or an AST syntax fallback; Go edits run gofmt diagnostics.

Search applies common workspace exclusions, returns bounded results, and respects cancellation. Globs support recursive `**`, wildcards, and character classes. Web fetch accepts concrete HTTP(S) URLs, converts HTML to markdown or plain text, supports raw HTML, follows redirects, reports truncation, and preserves HTTP error previews. It provides no web search.

`bash` uses workspace-relative `cwd` and `timeout_seconds` (120 by default, at most 600). Results contain `exit_code`, `stdout`, `stderr`, and `timed_out`; nonzero exits and timeouts count as failures. Shell modifications bypass file state and automatic diagnostics. The bash-only profile removes the predefined file/search/web tools and supplies shell-specific instructions. Planning and recall remain independently controlled.

## Selecting the study's configurations

```sh
# Default: T4 / 128k / planning / predefined actions
excelsior "Fix the parser"

# Select any T0–T4 policy at 32000, 64000, 96000, or 128000
excelsior --context-tier 2 --context-window 32000 "Fix the parser"

# Additional component ablations at T4 / 128k
excelsior --planning=false "Fix the parser"
excelsior --action-space bash "Fix the parser"
```

The same flags configure `excelsior engine` at startup; remote clients use the engine's profile. The corresponding environment variables are `EXCELSIOR_CONTEXT_TIER`, `EXCELSIOR_CONTEXT_WINDOW`, `EXCELSIOR_PLANNING`, and `EXCELSIOR_ACTION_SPACE`.

## Token budgets

`--context-window` / `EXCELSIOR_CONTEXT_WINDOW` is the total input-plus-output window. Before main model calls, the input allowance is `context_window - EXCELSIOR_MAX_OUTPUT_TOKENS`. The default is `128000 - 16384 = 111616`. An output limit equal to or larger than the total window is rejected. Summary calls independently reserve their own output limit, at most 2,048 tokens and never more than the main output limit.

`pkg/llm/tokens.go` counts the complete rendered prompt using the pinned [dsr-go port](https://github.com/dandandujie/dsr-go/tree/3fd324c60f95aaf55c7b33c80eebfc243036f15b) of DeepSeek's published conversion, chat templates, and HuggingFace-compatible BPE tokenizer. This includes BOS/role/reasoning markers, the generation prefix, tool instructions and schemas, and historical tool calls/results. Tokenizer definitions are bundled in the binary and loaded once per profile. Repeated context measurements are cached by request digest within the run; no prompt text is retained by that cache.

The main loop, recent-tail selection, and summary selection use these token counts. A second client check enforces `input_tokens + max_tokens <= context_window` immediately before generation. The original byte-size safety limit remains independent of token limits.

Known V4 Pro and literal self-hosted V4 model names use `deepseek-v4`. Hosted Flash uses `deepseek-v4.1`, including the legacy `deepseek-v4-flash` alias that DeepSeek now routes to V4.1; [DeepSeek's model documentation](https://api-docs.deepseek.com/quick_start/pricing/) describes that routing. Set `EXCELSIOR_TOKEN_ENCODING` or `--token-encoding` to `deepseek-v4` or `deepseek-v4.1` for a custom alias or endpoint whose template differs from that inference. The selected profile must match the server's actual model template. Unknown models/formats stop with a configuration error; no heuristic fallback is used.

Custom library providers now implement `ChatProvider.CountTokens(context.Context, ChatRequest) (int, error)` for their own exact request format. Counts are exact for the bundled text-only prompt profiles. Provider-side changes to tokenizers, templates, default reasoning settings, or hidden instructions require updating/selecting the matching profile; reported API usage remains authoritative for billing. [Tokenizer provenance](../pkg/llm/tokenizers/README.md) records the pinned data and license.

## Desktop context controls

The composer’s **Context** control shows the exact last assembled model input, its input allowance, the separate output reserve, and the total window. The percentage uses the input allowance after output reservation. Counts update before main model calls; they do not represent cumulative billed usage or a live count of streamed output. Between runs the panel labels the reading **Last model input**. Before the first measurement it shows an empty state rather than an estimate.

**Compact context** queues manual compaction at the next complete model/tool boundary. **Compact on next message** requests it before the next run’s first model call and can be cancelled before sending. Manual compaction retains the original task and at least the latest two complete model turns. The configured tier still controls whether older observations are shortened, recalled, or summarized; T0 disables compaction. Automatic management continues to use its configured thresholds and 30% recent-context target.

Compaction notices report the before/after input counts, including no-op requests. Requests that miss a model boundary are reported as unapplied when the run ends. Live telemetry and requests are scoped to the workspace, session, and run, and are restored on reconnect while the engine retains the run snapshot. Measurements are not persisted across engine restarts. Compaction changes the run’s working input; saved conversation history remains complete. A later run rebuilds its working context from that history.

## Local adaptations and limits

- Tokenization covers the bundled DeepSeek text profiles, rather than the four models evaluated in the study. The existing serialized provider-request ceiling of 600,000 bytes also applies.
- Summary chunks target 40% of their input allowance and summaries request at most 2,048 output tokens. These are local implementation choices where the paper does not specify exact values. If the immutable preamble and recent tail exceed the input allowance after output reservation, the run stops without discarding them.
- Ordinary observations are capped at 24,000 characters. Recall preserves archived content without a second truncation, so a recalled multi-observation turn can exceed that size and still must fit the context budget.
- Prompts are concise functional equivalents of the paper's protocols. HTML conversion uses Go's HTML parser with documentation-oriented markdown formatting, rather than Python markdownify/BeautifulSoup. Search uses Go's RE2 regex syntax. Windows uses PowerShell with process-tree cancellation; Unix uses sh.
- The desktop profile retains the application's `askQuestion` auxiliary tool. File state, plan state, and recall storage live for one agent run. Approvals use the existing CLI/engine permission handlers; the default command policy is conservative about complex shell syntax. Explicit allow/deny modes remain authoritative. Shell commands are not an OS sandbox.
- This is an implementation of the harness, not a reproduction of the benchmark results. The project continues to use its configured provider/model; the paper's four models, SGLang serving, Harbor containers, benchmark verifiers, and 176 experimental runs have not been executed here.
