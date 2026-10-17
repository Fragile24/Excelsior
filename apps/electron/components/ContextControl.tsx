import React, { useEffect, useId, useRef, useState } from "react";
import { Check, ChevronDown, LoaderCircle, Minimize2 } from "./phosphor-icons";
import type { ContextStatus } from "../lib/protocol";

type ContextControlProps = {
  context: ContextStatus | null;
  running: boolean;
  queued: boolean;
  compactNext: boolean;
  hasHistory: boolean;
  connected: boolean;
  onCompact: () => Promise<void>;
};

const number = (value: number) => value.toLocaleString("en-US");
const breakdownLabels = [
  { key: "system", label: "System prompt", color: "system" },
  { key: "tools", label: "Tool calls", color: "tools" },
  { key: "user", label: "User messages", color: "user" },
  { key: "assistant", label: "Assistant messages", color: "assistant" },
] as const;

function policy(context: ContextStatus): string {
  const soft = Math.round(context.softLimit / context.inputBudget * 100);
  const hard = Math.round(context.hardLimit / context.inputBudget * 100);
  switch (context.tier) {
    case 0: return "Automatic compaction is off for this engine.";
    case 1: return `Shorten older tool output at ${soft}% of the input allowance.`;
    case 2: return `Shorten older tool output at ${soft}%. Originals can be recalled during the run.`;
    case 3: return `Summarize older turns at ${hard}% of the input allowance.`;
    default: return `Shorten older tool output at ${soft}%; summarize older turns at ${hard}%.`;
  }
}

export default function ContextControl({ context, running, queued, compactNext, hasHistory, connected, onCompact }: ContextControlProps) {
  const root = useRef<HTMLDetailsElement>(null);
  const heading = useId();
  const [submitting, setSubmitting] = useState(false);
  const compacting = running && context?.state === "compacting";
  const pending = running ? queued : compactNext;
  const ratio = context ? context.inputTokens / context.inputBudget : null;
  const percent = ratio === null ? null : Math.round(ratio * 100);
  const tone = context && context.inputTokens > context.inputBudget ? "danger"
    : context && context.inputTokens >= context.hardLimit && context.tier !== 0 ? "warning" : "normal";
  const disabled = !connected || submitting || compacting || (running ? queued || !context?.canCompact : !hasHistory || context?.tier === 0);
  const status = compacting ? "Compacting context…" : queued && running ? "Compaction queued for the next model step."
    : compactNext && !running ? "Compaction queued for your next message."
    : context?.state === "compacted" ? `Context compacted from ${number(context.beforeTokens)} to ${number(context.inputTokens)} input tokens.`
    : context?.state === "unchanged" ? "Context unchanged. No older turns were eligible to compact." : "";

  useEffect(() => {
    const close = (event: PointerEvent) => {
      if (root.current && !root.current.contains(event.target as Node)) root.current.open = false;
    };
    document.addEventListener("pointerdown", close);
    return () => document.removeEventListener("pointerdown", close);
  }, []);

  async function compact() {
    if (disabled) return;
    setSubmitting(true);
    try { await onCompact(); } finally { setSubmitting(false); }
  }

  return (
    <>
    <details ref={root} className="context-control" data-tone={tone} onKeyDown={(event) => {
      if (event.key === "Escape" && root.current?.open) {
        event.preventDefault();
        event.stopPropagation();
        root.current.open = false;
        root.current.querySelector("summary")?.focus();
      }
    }}>
      <summary className="context-trigger" aria-label={`Context details${percent === null ? "" : `, ${percent}% of input allowance used`}`}>
        {compacting || submitting ? <LoaderCircle size={13} className="context-spin" aria-hidden="true" /> : (
          <svg width="15" height="15" viewBox="0 0 20 20" className="context-ring" aria-hidden="true">
            <circle cx="10" cy="10" r="7" className="context-ring-track" />
            {ratio !== null && <circle cx="10" cy="10" r="7" pathLength="100" strokeDasharray={`${Math.min(100, ratio * 100)} 100`} className="context-ring-value" />}
          </svg>
        )}
        <span>Context</span>
        <span className="context-trigger-value">{compacting ? "Compacting" : pending ? "Queued" : percent === null ? "—" : `${percent}%`}</span>
        <ChevronDown size={11} aria-hidden="true" />
      </summary>

      <div className="context-panel" role="group" aria-labelledby={heading}>
        <div className="context-panel-heading">
          <h2 id={heading}>Context window</h2>
          {context && <span className="context-policy-badge">Auto {context.tier === 0 ? "off" : "on"}</span>}
        </div>
        {context ? <>
          <div className="context-input-label">
            <span>{running ? "Model input" : "Last model input"}</span>
            <span>{percent}% used</span>
          </div>
          <div className="context-count">{number(context.inputTokens)} <span>/ {number(context.inputBudget)} tokens</span></div>
          {context.breakdown ? <>
            <div className="context-breakdown-bar" role="meter" aria-label="Estimated token breakdown by role"
              aria-valuemin={0} aria-valuemax={context.inputBudget} aria-valuenow={Math.min(context.inputTokens, context.inputBudget)}
              aria-valuetext={`${number(context.inputTokens)} of ${number(context.inputBudget)} input tokens; system ${number(context.breakdown.system)}, tools ${number(context.breakdown.tools)}, user ${number(context.breakdown.user)}, assistant ${number(context.breakdown.assistant)}`}>
              {breakdownLabels.map(({ key, label, color }) => <span key={key} className={`context-breakdown-segment is-${color}`}
                style={{ width: `${context.breakdown![key] / context.inputBudget * 100}%` }} title={`${label}: ${number(context.breakdown![key])} tokens`} />)}
            </div>
            <div className="context-breakdown-list">
              {breakdownLabels.map(({ key, label, color }) => <div key={key}>
                <span className={`context-breakdown-dot is-${color}`} aria-hidden="true" />
                <span>{label}</span><strong>{number(context.breakdown![key])}</strong>
              </div>)}
            </div>
            <p className="context-breakdown-note">Role counts are estimated; total prompt count is exact.</p>
          </> : <meter className="context-meter" min={0} max={context.inputBudget} value={Math.min(context.inputTokens, context.inputBudget)}
            aria-label="Input context used" aria-valuetext={`${number(context.inputTokens)} of ${number(context.inputBudget)} input tokens, ${percent}%`} />}
          <dl className="context-budget">
            <div><dt>Input remaining</dt><dd>{number(Math.max(0, context.inputBudget - context.inputTokens))}</dd></div>
            <div><dt>Reserved for output</dt><dd>{number(context.outputReserve)}</dd></div>
            <div><dt>Total window</dt><dd>{number(context.windowTokens)}</dd></div>
          </dl>
          {context.inputTokens > context.inputBudget && <p className="context-alert">Input exceeds the allowance. Compaction may help; the engine will keep the original task and recent turns.</p>}
          {context.error && <p className="context-alert">{context.error}</p>}
          <div className="context-policy">
            <strong>Automatic compaction</strong>
            <p>{policy(context)}</p>
          </div>
          <p className="context-measurement" title={context.model}>Exact input count for {context.model}, including instructions, tools, and reasoning. Measured before a model call.</p>
        </> : <div className="context-empty">
          <Minimize2 size={22} strokeWidth={1.4} aria-hidden="true" />
          <strong>{running ? "Preparing context" : "Context is measured when you send"}</strong>
          <p>The engine counts the full model input and reserves space for its response.</p>
        </div>}

        <div className="context-action">
          <button type="button" onClick={() => { void compact(); }} disabled={disabled}
            aria-pressed={!running ? compactNext : undefined} aria-busy={submitting || compacting}>
            {compacting || submitting ? <LoaderCircle size={14} className="context-spin" aria-hidden="true" />
              : pending ? <Check size={14} aria-hidden="true" /> : <Minimize2 size={14} aria-hidden="true" />}
            {compacting ? "Compacting…" : submitting ? "Requesting…" : pending ? running ? "Queued for next step" : "Cancel queued compaction"
              : running ? "Compact context" : "Compact on next message"}
          </button>
          <p>{!connected ? "Connect the engine to manage context." : context?.tier === 0 ? "Compaction is disabled by the engine’s context policy."
            : !hasHistory ? "Start a conversation to compact its older context."
            : running && !context?.canCompact && !compacting ? "No older context to compact yet. The latest two turns are kept."
            : running ? "Runs before the next model step. The original task and latest two turns are kept."
            : "Applies to the next run. Your full conversation stays in the transcript."}</p>
        </div>
      </div>
    </details>
    <span className="sr-only" role="status">{status}</span>
    </>
  );
}
