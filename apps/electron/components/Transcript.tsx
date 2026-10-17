"use client";

import React, { useEffect, useRef, useState } from "react";
import { ArrowDown, Minimize2 } from "./phosphor-icons";
import MarkdownRenderer from "./MarkdownRenderer";
import PermissionInline from "./PermissionInline";
import type { PermissionReq } from "../lib/protocol";
import type { Block } from "../lib/engineState";

type TranscriptProps = {
  blocks: Block[];
  streaming: boolean;
  permission?: PermissionReq | null;
  onPermissionDecision?: (approved: boolean) => void;
  onAllowAlways?: () => void;
};

function Transcript({ blocks, streaming, permission, onPermissionDecision, onAllowAlways }: TranscriptProps) {
  const innerRef = useRef<HTMLDivElement | null>(null);
  const [showJump, setShowJump] = useState(false);

  useEffect(() => {
    const el = innerRef.current;
    if (el && !showJump) el.scrollTop = el.scrollHeight;
  }, [blocks, streaming, showJump]);

  const handleScroll = () => {
    const el = innerRef.current;
    if (!el) return;
    setShowJump(el.scrollHeight - el.scrollTop - el.clientHeight > 200);
  };

  const scrollToLatest = () => {
    const el = innerRef.current;
    if (el) el.scrollTo({ top: el.scrollHeight, behavior: "smooth" });
  };

  return (
    <div className="relative flex-1 min-h-0">
      <div ref={innerRef} onScroll={handleScroll} role="log" aria-label="Conversation" aria-live="polite" aria-busy={streaming} className="transcript-scroll h-full overflow-y-auto px-5 md:px-9 py-6 space-y-3">
        <div className="transcript-content mx-auto w-full">
          {blocks.map((block, index) => (
            block.role === "context" ? <div key={`context-${index}`} className="context-transcript-note">
              <Minimize2 size={13} aria-hidden="true" />
              <div>
                <span>{block.content}</span>
                {block.context ? <p>{block.context.state === "compacted"
                  ? `${block.context.beforeTokens.toLocaleString("en-US")} → ${block.context.inputTokens.toLocaleString("en-US")} input tokens · ${block.context.reason === "manual" ? "Requested" : "Automatic"}`
                  : block.context.tier === 0 ? "The engine’s context policy disables compaction."
                  : "No older turns were eligible. The original task and latest two turns were kept."}</p>
                  : block.meta && <p>{block.meta}</p>}
              </div>
            </div> : <MarkdownRenderer
              key={`${block.role}-${index}`}
              role={block.role}
              content={block.content}
              meta={block.meta}
              args={block.args}
              isStreaming={streaming && index === blocks.length - 1}
            />
          ))}
          {permission && onPermissionDecision && (
            <PermissionInline permission={permission} onDecision={onPermissionDecision} onAllowAlways={onAllowAlways} />
          )}
        </div>
      </div>
      {showJump && (
        <button
          type="button"
          onClick={scrollToLatest}
          aria-label="Scroll to latest"
          className="absolute bottom-4 right-6 w-7 h-7 rounded-full bg-[var(--bg-card)] border-subtle shadow-sm flex items-center justify-center text-[var(--text-muted)] hover:text-[var(--text-main)] hover:bg-[var(--bg-card-hover)] transition-colors cursor-pointer z-10 animate-appear"
        >
          <ArrowDown className="w-3.5 h-3.5" />
        </button>
      )}
    </div>
  );
}

export default React.memo(Transcript);
