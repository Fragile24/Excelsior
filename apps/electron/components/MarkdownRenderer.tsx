import React, { useCallback, useMemo, useState } from "react";
import {
  AlertCircle,
  AlertTriangle,
  CheckSquare,
  ChevronDown,
  ExternalLink,
  Info,
  Lightbulb,
  ShieldAlert,
  Square,
} from "./phosphor-icons";
import CodeBlock from "./CodeBlock";
import { BrandMark } from "./Icons";
import { parseChunks, parseMarkdownBlocks, type AlertType, type BlockToken, type TableAlign } from "../lib/markdown";

type Role = "user" | "assistant" | "system" | "tool" | "reason" | "error";

type MarkdownRendererProps = {
  content?: string;
  role: Role;
  meta?: string;
  args?: string;
  isStreaming?: boolean;
};

/* =========================================================================
   Inline Markdown Renderer
   ========================================================================= */

const Inline = React.memo(function Inline({ text }: { text: string }) {
  if (!text) return null;

  const parts = text.split(
    /(`[^`]+`|\*\*\*[^*]+\*\*\*|\*\*[^*]+\*\*|(?<!\w)__[^_]+__(?!\w)|~~[^~]+~~|\*[^*]+\*|(?<!\w)_[^_]+_(?!\w)|\[[^\]]+\]\([^)]+\)|<kbd>[^<]+<\/kbd>|https?:\/\/[^\s<)]+)/g
  );

  return (
    <>
      {parts.map((part, index) => {
        if (!part) return null;
        if (part.startsWith("`") && part.endsWith("`") && part.length > 2) {
          return (
            <code
              key={index}
              className="px-1.5 py-0.5 rounded bg-[var(--bg-input)] text-[var(--text-main)] font-mono text-[12px] border border-[var(--border-subtle)]"
            >
              {part.slice(1, -1)}
            </code>
          );
        }
        if (part.startsWith("***") && part.endsWith("***") && part.length > 6) {
          return (
            <strong key={index} className="font-semibold italic text-[var(--text-main)]">
              <Inline text={part.slice(3, -3)} />
            </strong>
          );
        }
        if (
          ((part.startsWith("**") && part.endsWith("**")) || (part.startsWith("__") && part.endsWith("__"))) &&
          part.length > 4
        ) {
          return (
            <strong key={index} className="font-semibold text-[var(--text-main)]">
              <Inline text={part.slice(2, -2)} />
            </strong>
          );
        }
        if (
          ((part.startsWith("*") && part.endsWith("*")) || (part.startsWith("_") && part.endsWith("_"))) &&
          part.length > 2
        ) {
          return (
            <em key={index} className="italic text-[var(--text-main)]">
              <Inline text={part.slice(1, -1)} />
            </em>
          );
        }
        if (part.startsWith("~~") && part.endsWith("~~") && part.length > 4) {
          return (
            <del key={index} className="line-through text-[var(--text-dim)]">
              <Inline text={part.slice(2, -2)} />
            </del>
          );
        }
        if (part.startsWith("<kbd>") && part.endsWith("</kbd>")) {
          return (
            <kbd
              key={index}
              className="px-1.5 py-0.5 rounded bg-[var(--bg-input)] border border-[var(--border-subtle)] text-[11px] font-mono text-[var(--text-dim)] shadow-xs"
            >
              {part.slice(5, -6)}
            </kbd>
          );
        }
        const linkMatch = part.match(/^\[([^\]]+)\]\(([^)]+)\)$/);
        if (linkMatch) {
          const isHttp = linkMatch[2].startsWith("http");
          return (
            <a
              key={index}
              href={linkMatch[2]}
              target="_blank"
              rel="noopener noreferrer"
              className="text-[var(--accent)] hover:underline inline-flex items-center gap-0.5 cursor-pointer font-medium"
            >
              <span><Inline text={linkMatch[1]} /></span>
              {isHttp && <ExternalLink className="w-2.5 h-2.5 opacity-60 inline shrink-0" />}
            </a>
          );
        }
        if (part.startsWith("http://") || part.startsWith("https://")) {
          let url = part;
          let trail = "";
          while (/[.,;:!?)]$/.test(url) && !(url.endsWith(")") && url.includes("("))) {
            trail = url.slice(-1) + trail;
            url = url.slice(0, -1);
          }
          return (
            <React.Fragment key={index}>
              <a
                href={url}
                target="_blank"
                rel="noopener noreferrer"
                className="text-[var(--accent)] hover:underline break-all"
              >
                {url}
              </a>
              {trail}
            </React.Fragment>
          );
        }
        return <span key={index}>{part}</span>;
      })}
    </>
  );
});

/* =========================================================================
   Table Renderer
   ========================================================================= */

const Table = React.memo(function Table({
  headers,
  aligns,
  rows,
}: {
  headers: string[];
  aligns: TableAlign[];
  rows: string[][];
}) {
  const getAlignClass = (align?: TableAlign) => {
    if (align === "center") return "text-center";
    if (align === "right") return "text-right";
    return "text-left";
  };

  return (
    <div className="my-3 overflow-x-auto rounded-xl border border-[var(--border-subtle)]">
      <table className="w-full text-left text-xs border-collapse">
        <thead>
          <tr className="bg-[var(--bg-input)] border-b border-[var(--border-subtle)] font-semibold">
            {headers.map((header, i) => (
              <th
                key={i}
                className={`px-3.5 py-2 text-[12px] font-medium text-[var(--text-muted)] uppercase tracking-wider ${getAlignClass(
                  aligns[i]
                )}`}
              >
                <Inline text={header} />
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-[var(--border-subtle)]">
          {rows.map((row, rowIndex) => (
            <tr key={rowIndex} className="hover:bg-[var(--bg-card-hover)] transition-colors">
              {row.map((cell, cellIndex) => (
                <td
                  key={cellIndex}
                  className={`px-3.5 py-2 text-[12.5px] text-[var(--text-main)] ${getAlignClass(
                    aligns[cellIndex]
                  )}`}
                >
                  <Inline text={cell} />
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
});

/* =========================================================================
   Block Elements Components
   ========================================================================= */

const ALERT_CONFIG: Record<
  AlertType,
  { icon: React.ComponentType<{ className?: string }>; border: string; bg: string; text: string }
> = {
  note: { icon: Info, border: "border-blue-500/30", bg: "bg-blue-500/10", text: "text-blue-400" },
  tip: { icon: Lightbulb, border: "border-emerald-500/30", bg: "bg-emerald-500/10", text: "text-emerald-400" },
  important: { icon: AlertCircle, border: "border-purple-500/30", bg: "bg-purple-500/10", text: "text-purple-400" },
  warning: { icon: AlertTriangle, border: "border-amber-500/30", bg: "bg-amber-500/10", text: "text-amber-400" },
  caution: { icon: ShieldAlert, border: "border-rose-500/30", bg: "bg-rose-500/10", text: "text-rose-400" },
};

function AlertBlock({
  alertType,
  title,
  content,
}: {
  alertType: AlertType;
  title: string;
  content: string;
}) {
  const config = ALERT_CONFIG[alertType] ?? ALERT_CONFIG.note;
  const Icon = config.icon;

  return (
    <div className={`my-3 p-3.5 rounded-xl border ${config.border} ${config.bg} text-[13px]`}>
      <div className={`flex items-center gap-2 font-semibold text-xs uppercase tracking-wider mb-1.5 ${config.text}`}>
        <Icon className="w-4 h-4 shrink-0" />
        <span>{title}</span>
      </div>
      <div className="text-[var(--text-main)] leading-relaxed pl-6 space-y-1">
        {content.split("\n").map((line, idx) => (
          <p key={idx}><Inline text={line} /></p>
        ))}
      </div>
    </div>
  );
}

function RenderBlocks({ blocks }: { blocks: BlockToken[] }) {
  return (
    <div className="space-y-2">
      {blocks.map((token, index) => {
        switch (token.type) {
          case "h": {
            if (token.level === 1) {
              return (
                <h1 key={index} className="text-[17px] font-bold mt-5 mb-2 pb-1 border-b border-[var(--border-subtle)] text-[var(--text-main)]">
                  <Inline text={token.text} />
                </h1>
              );
            }
            if (token.level === 2) {
              return (
                <h2 key={index} className="text-[15px] font-semibold mt-4 mb-1.5 text-[var(--text-main)]">
                  <Inline text={token.text} />
                </h2>
              );
            }
            if (token.level === 3) {
              return (
                <h3 key={index} className="text-[13.5px] font-semibold mt-3 mb-1 text-[var(--text-main)]">
                  <Inline text={token.text} />
                </h3>
              );
            }
            if (token.level === 4) {
              return (
                <h4 key={index} className="text-[12.5px] font-semibold mt-2.5 mb-1 text-[var(--text-main)]">
                  <Inline text={token.text} />
                </h4>
              );
            }
            return (
              <h5 key={index} className="text-[12px] font-semibold text-[var(--text-muted)] mt-2 mb-1">
                <Inline text={token.text} />
              </h5>
            );
          }
          case "hr":
            return <hr key={index} className="my-4 border-t border-[var(--border-subtle)]" />;
          case "alert":
            return (
              <AlertBlock
                key={index}
                alertType={token.alertType}
                title={token.title}
                content={token.content}
              />
            );
          case "quote":
            return (
              <blockquote
                key={index}
                className="border-l-2 border-[var(--accent)] bg-[var(--bg-input)]/30 rounded-r-lg px-3.5 py-2 my-2.5 text-[13px] text-[var(--text-muted)] italic leading-relaxed space-y-1"
              >
                {token.content.split("\n").map((line, lineIdx) => (
                  <p key={lineIdx}>
                    <Inline text={line} />
                  </p>
                ))}
              </blockquote>
            );
          case "table":
            return (
              <Table
                key={index}
                headers={token.headers}
                aligns={token.aligns}
                rows={token.rows}
              />
            );
          case "ul":
            return (
              <div key={index} className="space-y-1 my-2">
                {token.items.map((item, itemIdx) => (
                  <div
                    key={itemIdx}
                    style={{ paddingLeft: `${item.level * 16}px` }}
                    className="flex items-start gap-2 text-[13px] text-[var(--text-main)] leading-relaxed"
                  >
                    {item.checked !== undefined ? (
                      item.checked ? (
                        <CheckSquare className="w-3.5 h-3.5 text-emerald-500 shrink-0 mt-1" />
                      ) : (
                        <Square className="w-3.5 h-3.5 text-[var(--text-dim)] shrink-0 mt-1" />
                      )
                    ) : (
                      <span className="text-[var(--text-dim)] shrink-0 select-none mt-0.5">•</span>
                    )}
                    <span className={item.checked ? "line-through text-[var(--text-dim)]" : ""}>
                      <Inline text={item.text} />
                    </span>
                  </div>
                ))}
              </div>
            );
          case "ol":
            return (
              <div key={index} className="space-y-1 my-2">
                {token.items.map((item, itemIdx) => (
                  <div
                    key={itemIdx}
                    style={{ paddingLeft: `${item.level * 16}px` }}
                    className="flex items-start gap-2 text-[13px] text-[var(--text-main)] leading-relaxed"
                  >
                    <span className="font-mono text-[11.5px] text-[var(--text-dim)] shrink-0 w-4 text-right select-none mt-0.5">
                      {item.num}.
                    </span>
                    <span>
                      <Inline text={item.text} />
                    </span>
                  </div>
                ))}
              </div>
            );
          case "p":
            return (
              <p key={index} className="text-[13px] text-[var(--text-main)] leading-relaxed my-2">
                <Inline text={token.text} />
              </p>
            );
          default:
            return null;
        }
      })}
    </div>
  );
}

/* =========================================================================
   User, Tool, and Reason Blocks
   ========================================================================= */

function UserBubble({ text }: { text: string }) {
  return (
    <div className="user-message flex justify-end animate-appear">
      <div className="user-message-content selectable-text whitespace-pre-wrap">
        {text}
      </div>
    </div>
  );
}

function ToolBlock({ content, meta, args: rawArgs }: { content: string; meta?: string; args?: string }) {
  const [open, setOpen] = useState(false);
  const isResult = !!meta?.includes("→");
  const rawName = isResult ? meta!.replace(/\s*→$/, "").trim() : (meta || "tool").trim();
  const hasFence = content.includes("```");
  const isExecuting = !isResult && rawArgs === undefined && !content;
  const isBash = /bash|shell/.test(rawName.toLowerCase());

  const pickArg = useCallback((raw?: string | null): string | null => {
    if (!raw?.trim()) return null;
    try {
      const parsed = JSON.parse(raw);
      if (parsed && typeof parsed === "object") {
        const val =
          (parsed as Record<string, unknown>).command ??
          (parsed as Record<string, unknown>).cmd ??
          (parsed as Record<string, unknown>).filePath ??
          (parsed as Record<string, unknown>).TargetFile ??
          (parsed as Record<string, unknown>).AbsolutePath ??
          (parsed as Record<string, unknown>).FilePath ??
          (parsed as Record<string, unknown>).Query ??
          (parsed as Record<string, unknown>).Pattern ??
          (parsed as Record<string, unknown>).prompt ??
          (parsed as Record<string, unknown>).query ??
          Object.values(parsed)[0];
        if (typeof val === "string" && val.trim()) return val;
      }
    } catch {}
    return null;
  }, []);

  const preview = useMemo(() => {
    const line = (pickArg(rawArgs) ?? pickArg(content) ?? (rawArgs ?? content ?? "")).split("\n")[0].trim();
    return line.length > 80 ? `${line.slice(0, 80)}…` : line;
  }, [rawArgs, content, pickArg]);

  const rawArgsText = rawArgs !== undefined ? rawArgs : (!isResult && !hasFence ? content : null);

  const toolChunks = useMemo(() => {
    if (!hasFence) return null;
    return parseChunks(content);
  }, [content, hasFence]);

  return (
    <div className="my-1.5 text-xs font-mono animate-tool-reveal">
      {/* Header (borderless & backgroundless) */}
      <div
        role="button"
        tabIndex={0}
        onClick={() => setOpen((v) => !v)}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            setOpen((v) => !v);
          }
        }}
        className="w-full py-1 px-1.5 flex items-center gap-2 cursor-pointer select-none rounded hover:bg-[var(--bg-card-hover)] transition-colors"
      >
        <span className="font-medium text-[var(--text-main)] text-[11px] shrink-0">
          {isBash ? "shell" : rawName}
        </span>
        {preview && (
          <span className="text-[11px] text-[var(--text-dim)] truncate min-w-0 flex-1 selectable-text font-mono animate-bit-by-bit">
            {preview}
          </span>
        )}
        {isExecuting && (
          <span className="flex items-center gap-1.5 text-[10px] text-amber-400 font-medium">
            <span className="w-1.5 h-1.5 rounded-full bg-amber-400 studio-motion-loop" />
            running
          </span>
        )}
        <ChevronDown
          className={`w-3 h-3 text-[var(--text-dim)] transition-transform duration-150 ${open ? "" : "-rotate-90"}`}
          aria-hidden
        />
      </div>

      {/* Body */}
      {open && (
        <div className="mt-1 rounded-lg border-subtle bg-[var(--bg-input)] px-2.5 py-2 space-y-2 animate-appear">
          {content && content !== rawArgsText && (
            hasFence && toolChunks ? (
              <div className="space-y-2">
                {toolChunks.map((ch, i) =>
                  ch.type === "code" ? (
                    <div key={i} className="animate-bit-by-bit" style={{ animationDelay: `${Math.min(i * 50, 300)}ms` }}>
                      <CodeBlock language={ch.lang} code={ch.content} />
                    </div>
                  ) : ch.content.trim() ? (
                    <div
                      key={i}
                      className="rounded px-2 py-1.5 text-[var(--text-muted)] selectable-text whitespace-pre-wrap break-words animate-bit-by-bit"
                      style={{ animationDelay: `${Math.min(i * 50, 300)}ms` }}
                    >
                      {ch.content.trim()}
                    </div>
                  ) : null
                )}
              </div>
            ) : (
              <pre className="rounded px-2 py-1.5 text-[var(--text-muted)] max-h-80 overflow-y-auto whitespace-pre-wrap break-words selectable-text animate-bit-by-bit">
                {content}
              </pre>
            )
          )}
        </div>
      )}
    </div>
  );
}

function ReasonBlock({ text }: { text: string }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="my-1 text-xs animate-appear">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="py-1 px-1.5 rounded flex items-center gap-1.5 text-[11px] text-[var(--text-dim)] hover:text-[var(--text-main)] hover:bg-[var(--bg-card-hover)] cursor-pointer transition-colors"
      >
        <ChevronDown className={`w-3 h-3 transition-transform duration-150 ${open ? "" : "-rotate-90"}`} />
        <span>Thinking process</span>
      </button>
      {open && (
        <div className="mt-1 rounded-lg border-subtle bg-[var(--bg-input)] px-3 py-2 text-[11px] text-[var(--text-muted)] font-mono leading-relaxed selectable-text whitespace-pre-wrap animate-appear">
          {text}
        </div>
      )}
    </div>
  );
}

/* =========================================================================
   Main MarkdownRenderer
   ========================================================================= */

function MarkdownRenderer({ content = "", role, meta, args, isStreaming }: MarkdownRendererProps) {
  const text = typeof content === "string" ? content : String(content ?? "");
  const chunks = useMemo(() => parseChunks(text), [text]);

  if (role === "user") return <UserBubble text={text} />;
  if (role === "tool") return <ToolBlock content={text} meta={meta} args={args} />;
  if (role === "reason") return <ReasonBlock text={text} />;
  if (role === "error") {
    if (text === "canceled") return <div className="run-stopped" role="status"><Square size={11} aria-hidden="true" />Stopped. Pick up where you left off.</div>;
    return (
      <div className="my-2.5 px-3.5 py-2.5 rounded-xl bg-rose-500/10 border-subtle text-rose-400 text-xs font-mono animate-appear">
        Error: {text}
      </div>
    );
  }

  return (
    <div className="assistant-message selectable-text animate-appear">
      {role === "assistant" && <div className="assistant-identity"><BrandMark /><span>Excelsior</span></div>}
      <div className="space-y-2">
        {chunks.map((chunk, index) => {
          if (chunk.type === "code") {
            return <CodeBlock key={index} language={chunk.lang} code={chunk.content} />;
          }
          const blocks = parseMarkdownBlocks(chunk.content);
          return <RenderBlocks key={index} blocks={blocks} />;
        })}
        {isStreaming && (
          <span className="inline-block w-2 h-4 bg-[var(--accent)] studio-motion-loop ml-1 align-middle" aria-hidden />
        )}
      </div>
    </div>
  );
}

export default React.memo(MarkdownRenderer);
