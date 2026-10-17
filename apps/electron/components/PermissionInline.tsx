import React from "react";
import { Check, ShieldAlert, X } from "./phosphor-icons";
import type { PermissionReq } from "../lib/protocol";

type PermissionInlineProps = {
  permission: PermissionReq;
  onDecision: (approved: boolean) => void;
  onAllowAlways?: () => void;
};

function PermissionInline({ permission, onDecision, onAllowAlways }: PermissionInlineProps) {
  return (
    <div className="bg-[var(--bg-card)] border-subtle rounded-xl p-3.5 my-2.5 animate-appear">
      <div className="flex items-center justify-between pb-2 border-subtle-b mb-2.5">
        <div className="flex items-center gap-2">
          <div className="w-5 h-5 rounded bg-amber-500/15 flex items-center justify-center text-amber-400">
            <ShieldAlert className="w-3 h-3" />
          </div>
          <span className="text-[12.5px] font-medium text-[var(--text-main)]">
            Permission Request
          </span>
        </div>
        <span className="text-[11px] font-mono px-1.5 py-0.5 rounded bg-[var(--bg-input)] border-subtle text-[var(--text-muted)]">
          {permission.tool}
        </span>
      </div>

      {permission.filePath && (
        <div className="mb-2 text-xs">
          <span className="text-[var(--text-dim)] font-mono">File: </span>
          <code className="text-[var(--text-main)] font-mono bg-[var(--bg-input)] px-1.5 py-0.5 rounded border-subtle break-all">
            {permission.filePath}
          </code>
        </div>
      )}

      {permission.command && (
        <div className="mb-2 text-xs">
          <span className="text-[var(--text-dim)] font-mono">Command: </span>
          <code className="text-emerald-400 font-mono bg-[var(--bg-input)] px-2 py-1 rounded border-subtle block mt-1 break-all">
            $ {permission.command}
          </code>
        </div>
      )}

      {permission.preview && permission.tool !== "bash" && (
        <pre className="bg-[var(--bg-input)] border-subtle rounded-lg p-2.5 text-[11px] font-mono whitespace-pre-wrap break-words max-h-48 overflow-auto mb-2.5 selectable-text text-[var(--text-main)]">
          {permission.preview.slice(0, 1000)}
          {permission.preview.length > 1000 ? "\n… truncated" : ""}
        </pre>
      )}

      <div className="flex flex-wrap items-center justify-end gap-1.5 pt-1">
        <button
          type="button"
          onClick={() => onDecision(false)}
          className="flex items-center gap-1 px-2.5 py-1 rounded-md bg-[var(--bg-input)] hover:bg-[var(--bg-card-hover)] border-subtle text-[var(--text-muted)] hover:text-rose-400 text-xs font-medium transition-colors cursor-pointer"
        >
          <X className="w-3 h-3" />
          <span>Deny</span>
        </button>

        <button
          type="button"
          onClick={() => onDecision(true)}
          className="flex items-center gap-1 px-3 py-1 rounded-md bg-[var(--accent)] text-[var(--accent-ink)] text-xs font-medium hover:opacity-90 transition-all cursor-pointer"
        >
          <Check className="w-3 h-3" />
          <span>Allow</span>
        </button>

        {onAllowAlways && (
          <button
            type="button"
            onClick={onAllowAlways}
            className="flex items-center gap-1 px-2.5 py-1 rounded-md bg-[var(--bg-input)] hover:bg-amber-500/15 border-subtle text-amber-300 text-xs font-medium transition-colors cursor-pointer"
          >
            <span>Always Allow</span>
          </button>
        )}
      </div>
    </div>
  );
}

export default React.memo(PermissionInline);
