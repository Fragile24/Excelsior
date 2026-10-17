import React, { useCallback, useEffect, useRef, useState } from "react";
import { ArrowUp, BrainCircuit, Cpu, LoaderCircle, ShieldCheck, ShieldOff, ShieldQuestionMark, Square, Zap } from "./phosphor-icons";
import { AVAILABLE_MODELS } from "../lib/constants";
import type { PermissionMode, ReasoningEffort } from "../lib/protocol";
import ComposerPicker from "./ComposerPicker";

const MODEL_OPTIONS = AVAILABLE_MODELS.map((model) => ({ value: model.id, label: model.name, description: model.desc, icon: model.badge === "Flash" ? Zap : BrainCircuit }));
const PERMISSION_OPTIONS = [
  { value: "ask", label: "Ask first", description: "Request approval for changes and commands.", icon: ShieldQuestionMark },
  { value: "allow", label: "Allow actions", description: "Make changes and run commands automatically.", icon: ShieldCheck },
  { value: "deny", label: "Deny actions", description: "Block actions that need permission.", icon: ShieldOff },
] as const;
const REASONING_OPTIONS = [
  { value: "none", label: "Off", description: "Skip extended reasoning.", icon: BrainCircuit },
  { value: "low", label: "Low", description: "Quick reasoning for straightforward tasks.", icon: BrainCircuit },
  { value: "high", label: "High", description: "Balanced reasoning for everyday coding.", icon: BrainCircuit },
  { value: "max", label: "Max", description: "Deep reasoning for complex work.", icon: BrainCircuit },
] as const;

export type ComposerMode = "centered" | "docked";

type ComposerProps = {
  mode: ComposerMode;
  selectedModel: string;
  onSelectModel: (id: string) => void;
  reasoningEffort: ReasoningEffort;
  onSelectReasoningEffort: (effort: ReasoningEffort) => void;
  permissionMode: PermissionMode;
  onSelectPermission: (mode: PermissionMode) => Promise<void>;
  onSend: (text: string) => Promise<boolean>;
  disabled?: boolean;
  isStreaming?: boolean;
  onStop: () => void;
  contextControl?: React.ReactNode;
};

function Composer({ mode, selectedModel, onSelectModel, reasoningEffort, onSelectReasoningEffort, permissionMode, onSelectPermission, onSend, disabled, isStreaming, onStop, contextControl }: ComposerProps) {
  const [text, setText] = useState("");
  const [sending, setSending] = useState(false);
  const [changingPermissions, setChangingPermissions] = useState(false);
  const permissionRequest = useRef(false);
  const sendingRef = useRef(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const activeModel = AVAILABLE_MODELS.find((model) => model.id === selectedModel) ?? AVAILABLE_MODELS[0];
  const activePermission = PERMISSION_OPTIONS.find((option) => option.value === permissionMode) ?? PERMISSION_OPTIONS[0];
  const canSend = text.trim().length > 0 && !disabled && !isStreaming && !sending && !changingPermissions;

  async function selectPermission(value: PermissionMode) {
    if (permissionRequest.current || disabled || isStreaming || value === permissionMode) return;
    permissionRequest.current = true;
    setChangingPermissions(true);
    try { await onSelectPermission(value); }
    finally { permissionRequest.current = false; setChangingPermissions(false); }
  }

  useEffect(() => {
    const input = textareaRef.current;
    if (!input) return;
    input.style.height = "auto";
    input.style.height = `${Math.min(input.scrollHeight, 200)}px`;
  }, [text]);

  const handleSend = useCallback(async () => {
    const trimmed = text.trim();
    if (!trimmed || !canSend || sendingRef.current) return;
    sendingRef.current = true;
    setSending(true);
    try {
      if (await onSend(trimmed)) setText((current) => current === text ? "" : current);
    } finally {
      sendingRef.current = false;
      setSending(false);
    }
  }, [canSend, onSend, text]);

  return (
    <div className={`composer-wrap composer-${mode}`}>
      <div className="studio-composer">
        <textarea
          ref={textareaRef}
          rows={1}
          value={text}
          onChange={(event) => setText(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter" && !event.shiftKey && !event.nativeEvent.isComposing) {
              event.preventDefault();
              void handleSend();
            }
          }}
          placeholder="Ask Excelsior…"
          disabled={isStreaming}
          aria-label="Composer input"
          aria-describedby="composer-hint"
          className="selectable-text"
        />
        <div className="composer-tools">
          <div className="composer-options">
            <ComposerPicker label="Model" value={activeModel.id} text={activeModel.name.replace("DeepSeek ", "")}
              icon={Cpu} options={MODEL_OPTIONS} onSelect={onSelectModel} disabled={isStreaming || sending} title={activeModel.name} />
            <ComposerPicker label="Reasoning" value={reasoningEffort} text={REASONING_OPTIONS.find((option) => option.value === reasoningEffort)?.label ?? "High"}
              icon={BrainCircuit} options={REASONING_OPTIONS} onSelect={onSelectReasoningEffort} disabled={isStreaming || sending} title="Reasoning effort" />
            <ComposerPicker label="Permissions" value={permissionMode} text={changingPermissions ? "Saving…" : activePermission.label}
              icon={changingPermissions ? LoaderCircle : activePermission.icon} options={PERMISSION_OPTIONS}
              onSelect={(value) => { void selectPermission(value); }} disabled={disabled || isStreaming || sending} busy={changingPermissions}
              title={disabled ? "Manage the engine connection in Preferences to change permissions" : "Workspace permissions"} />
            {contextControl}
          </div>
          <button
            type="button"
            onClick={isStreaming ? onStop : handleSend}
            disabled={!isStreaming && !canSend}
            aria-label={isStreaming ? "Stop generation" : "Send message"}
            title={isStreaming ? "Stop generation (Esc)" : disabled ? "Connect your engine to send" : "Send message (Enter)"}
            className="composer-send"
          >
            {isStreaming ? <Square size={13} fill="currentColor" /> : <ArrowUp size={19} strokeWidth={1.8} />}
          </button>
        </div>
      </div>
      <div id="composer-hint" className={isStreaming ? "composer-hint" : "sr-only"}>
        {isStreaming ? <span className="working-indicator"><span className="status-dot online" /> Working <span>· Esc to stop</span></span>
          : <span><kbd>Enter</kbd> to send <span className="hint-separator">·</span> <kbd>Shift Enter</kbd> for a new line</span>}
      </div>
    </div>
  );
}

export default React.memo(Composer);
