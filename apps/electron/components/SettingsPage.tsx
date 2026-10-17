import React, { useState } from "react";
import { ArrowLeft, Check, Layers, Palette, PanelLeft, PanelLeftClose, ShieldCheck, Wifi } from "./phosphor-icons";
import { ACCENT_COLORS, AVAILABLE_MODELS, AVAILABLE_THEMES } from "../lib/constants";
import type { PermissionMode } from "../lib/protocol";

type SettingsTab = "appearance" | "models" | "connection" | "security";

export type SettingsValues = { url: string; model: string; permission: PermissionMode; token: string };
type SettingsPageProps = {
  navOpen: boolean;
  onToggleNav: () => void;
  onClose: () => void;
  engineUrl: string;
  engineState: string;
  defaultModel: string;
  currentTheme: string;
  onSaveTheme: (theme: string) => void;
  accentColor: string | null;
  onSaveAccent: (color: string | null) => void;
  surfaceColor: string | null;
  onSaveSurface: (color: string | null) => void;
  transcriptSize: number;
  onSaveTranscriptSize: (size: number) => void;
  permissionMode: PermissionMode;
  onSave: (values: SettingsValues) => void;
};

const TABS: { id: SettingsTab; label: string; icon: React.ComponentType<{ className?: string }> }[] = [
  { id: "appearance", label: "Appearance", icon: Palette },
  { id: "models", label: "Models", icon: Layers },
  { id: "connection", label: "Connection", icon: Wifi },
  { id: "security", label: "Security", icon: ShieldCheck },
];

function SettingsPage({
  navOpen,
  onToggleNav,
  onClose,
  engineUrl,
  engineState,
  defaultModel,
  currentTheme,
  onSaveTheme,
  accentColor,
  onSaveAccent,
  surfaceColor,
  onSaveSurface,
  transcriptSize,
  onSaveTranscriptSize,
  permissionMode,
  onSave,
}: SettingsPageProps) {
  const [activeTab, setActiveTab] = useState<SettingsTab>("appearance");
  const [draftToken, setDraftToken] = useState("");
  const [draftUrl, setDraftUrl] = useState(engineUrl);
  const [draftModel, setDraftModel] = useState(defaultModel);
  const [draftPermissionMode, setDraftPermissionMode] = useState(permissionMode);
  const themeSurface = AVAILABLE_THEMES.find((theme) => theme.id === currentTheme)?.surface ?? "#e9ecf0";
  const surfaceFallback = currentTheme === "system" && typeof window !== "undefined" && window.matchMedia("(prefers-color-scheme: dark)").matches
    ? "#1c1e22"
    : themeSurface;

  const handleSave = () => onSave({ url: draftUrl.trim(), model: draftModel, permission: draftPermissionMode, token: draftToken.trim() });

  const statusBg =
    engineState === "connected"
      ? "text-[var(--text-main)] bg-emerald-500/10"
      : engineState === "error"
        ? "text-[var(--text-main)] bg-rose-500/10"
        : "text-[var(--text-main)] bg-amber-500/10";

  return (
    <div className="preferences-page animate-appear">
      {navOpen && <aside className="preferences-sidebar">
        <button type="button" className="preferences-back" onClick={onClose}>
          <ArrowLeft size={15} aria-hidden="true" /><span>Back to workspace</span>
        </button>
        <div className="preferences-sidebar-title">
          <h2>Preferences</h2>
          <p>Configure your workspace</p>
        </div>
        <nav aria-label="Settings categories" className="settings-navigation">
            {TABS.map((tab) => {
              const Icon = tab.icon;
              const isActive = activeTab === tab.id;
              return (
                <button
                  key={tab.id}
                  type="button"
                  aria-current={isActive ? "page" : undefined}
                  onClick={() => setActiveTab(tab.id)}
                  className={`settings-navigation-item w-full flex items-center gap-3 px-3 py-2 rounded-lg text-[13px] font-medium transition-colors text-left cursor-pointer ${
                    isActive
                      ? "bg-[var(--bg-card)] text-[var(--text-main)]"
                      : "text-[var(--text-muted)] hover:bg-[var(--bg-card-hover)] hover:text-[var(--text-main)]"
                  }`}
                >
                  <Icon className={`w-4 h-4 ${isActive ? "text-[var(--accent)]" : "text-[var(--text-dim)]"}`} />
                  <span>{tab.label}</span>
                </button>
              );
            })}
        </nav>
        <div className="preferences-sidebar-footer">Excelsior v0.1.0</div>
      </aside>}

      <main className="preferences-main" aria-labelledby="settings-title">
        <header className="preferences-header">
          <div className="preferences-header-title">
            <button type="button" className="preferences-nav-toggle" onClick={onToggleNav}
              aria-label={navOpen ? "Collapse settings sidebar" : "Open settings sidebar"}
              title={navOpen ? "Collapse settings sidebar" : "Open settings sidebar"}>
              {navOpen ? <PanelLeftClose size={18} aria-hidden="true" /> : <PanelLeft size={18} aria-hidden="true" />}
            </button>
            <div className="preferences-heading-copy">
              <h1 id="settings-title">Preferences</h1>
              <p>Manage appearance, models, and workspace behavior.</p>
            </div>
          </div>
          <div className="preferences-actions">
            <button type="button" className="preferences-cancel" onClick={onClose}>Cancel</button>
            <button type="button" className="preferences-save" onClick={handleSave}>Save changes</button>
          </div>
        </header>

        <div className="settings-content">
            {activeTab === "appearance" && (
              <div className="space-y-4 animate-appear">
                <div>
                  <h3 className="text-[15px] font-semibold text-[var(--text-main)]">Appearance</h3>
                  <p className="text-[12px] text-[var(--text-dim)] mt-0.5">Customize the visual theme and contrast of the workspace.</p>
                </div>
                <div className="grid grid-cols-2 gap-3 pt-1">
                  {AVAILABLE_THEMES.map((t) => (
                    <button
                      key={t.id}
                      type="button"
                      onClick={() => onSaveTheme(t.id)}
                      aria-pressed={currentTheme === t.id}
                      className={`p-3 rounded-xl text-left border-subtle transition-all cursor-pointer ${
                        currentTheme === t.id
                          ? "bg-[var(--bg-card-hover)] font-medium ring-1.5 ring-[var(--accent)]"
                          : "bg-[var(--bg-input)] hover:bg-[var(--bg-card-hover)] text-[var(--text-main)]"
                      }`}
                    >
                      <span className="theme-preview !h-12" data-theme={t.id} aria-hidden="true" />
                      <div className="flex items-center justify-between mt-2">
                        <span className="text-[12.5px] font-medium">{t.name}</span>
                        {currentTheme === t.id && <Check className="w-3.5 h-3.5 text-[var(--accent)]" />}
                      </div>
                    </button>
                  ))}
                </div>
                <section className="border-subtle-t pt-4" aria-labelledby="surface-heading">
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <h4 id="surface-heading" className="text-[13px] font-medium">Workspace surfaces</h4>
                      <p className="text-[11px] text-[var(--text-dim)] mt-0.5">Set a base tone; panels and text adapt to it.</p>
                    </div>
                    <button
                      type="button"
                      onClick={() => onSaveSurface(null)}
                      disabled={!surfaceColor}
                      className="text-[11px] text-[var(--text-muted)] hover:text-[var(--text-main)] disabled:opacity-40 disabled:cursor-default"
                    >
                      Reset
                    </button>
                  </div>
                  <label className="relative mt-3 flex cursor-pointer items-center gap-3 overflow-hidden rounded-xl border-subtle bg-[var(--bg-input)] px-3 py-2.5">
                    <span className="h-8 w-8 shrink-0 rounded-lg border-subtle shadow-sm" style={{ backgroundColor: surfaceColor ?? surfaceFallback }} aria-hidden="true" />
                    <span className="flex-1 text-[12px]">Choose a surface tone</span>
                    <span className="font-mono text-[10px] text-[var(--text-dim)]">{(surfaceColor ?? surfaceFallback).toUpperCase()}</span>
                    <input
                      type="color"
                      aria-label="Customize workspace surface color"
                      value={surfaceColor ?? surfaceFallback}
                      onChange={(event) => onSaveSurface(event.target.value)}
                      className="absolute inset-0 h-full w-full cursor-pointer opacity-0"
                    />
                  </label>
                </section>
                <section className="border-subtle-t pt-4" aria-labelledby="accent-heading">
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <h4 id="accent-heading" className="text-[13px] font-medium">Accent color</h4>
                      <p className="text-[11px] text-[var(--text-dim)] mt-0.5">Personalize buttons, focus states, and highlights.</p>
                    </div>
                    <button
                      type="button"
                      onClick={() => onSaveAccent(null)}
                      disabled={!accentColor}
                      className="text-[11px] text-[var(--text-muted)] hover:text-[var(--text-main)] disabled:opacity-40 disabled:cursor-default"
                    >
                      Use theme color
                    </button>
                  </div>
                  <div className="flex flex-wrap items-center gap-2 mt-3" role="group" aria-label="Accent color presets">
                    {ACCENT_COLORS.map((color) => (
                      <button
                        key={color.value}
                        type="button"
                        aria-label={`${color.name} accent`}
                        aria-pressed={accentColor === color.value}
                        onClick={() => onSaveAccent(color.value)}
                        className={`h-7 w-7 rounded-full border border-[var(--border-hover)] transition-transform hover:scale-110 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--accent)] ${accentColor === color.value ? "ring-2 ring-[var(--text-main)] ring-offset-2 ring-offset-[var(--bg-card)]" : ""}`}
                        style={{ backgroundColor: color.value }}
                      />
                    ))}
                    <label className="grid h-7 w-7 cursor-pointer place-items-center overflow-hidden rounded-full border border-[var(--border-hover)] bg-[var(--bg-input)]" title="Choose a custom accent color">
                      <input
                        type="color"
                        aria-label="Choose a custom accent color"
                        value={accentColor ?? (AVAILABLE_THEMES.find((theme) => theme.id === currentTheme)?.accent ?? "#202020")}
                        onChange={(event) => onSaveAccent(event.target.value)}
                        className="h-10 w-10 cursor-pointer border-0 bg-transparent p-0"
                      />
                    </label>
                  </div>
                </section>
                <section className="border-subtle-t pt-4" aria-labelledby="transcript-size-heading">
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <h4 id="transcript-size-heading" className="text-[13px] font-medium">Transcript text size</h4>
                      <p className="text-[11px] text-[var(--text-dim)] mt-0.5">Adjust reading size without changing the interface.</p>
                    </div>
                    <output htmlFor="transcript-size" className="text-[11px] font-mono text-[var(--text-muted)]">{transcriptSize}px</output>
                  </div>
                  <input
                    id="transcript-size"
                    type="range"
                    min={12}
                    max={18}
                    step={1}
                    value={transcriptSize}
                    onChange={(event) => onSaveTranscriptSize(Number(event.target.value))}
                    aria-label="Transcript text size"
                    aria-valuetext={`${transcriptSize} pixels`}
                    className="mt-3 w-full cursor-pointer"
                    style={{ accentColor: "var(--accent)" }}
                  />
                  <div className="flex justify-between text-[10px] text-[var(--text-dim)]">
                    <span>Compact</span><span>Large</span>
                  </div>
                </section>
              </div>
            )}

            {activeTab === "models" && (
              <div className="space-y-4 animate-appear">
                <div>
                  <h3 className="text-[15px] font-semibold text-[var(--text-main)]">Model Selection</h3>
                  <p className="text-[12px] text-[var(--text-dim)] mt-0.5">Choose the default model used for code assistance and reasoning.</p>
                </div>
                <div className="space-y-1.5 pt-1">
                  <label htmlFor="default-model" className="text-[12.5px] text-[var(--text-muted)] font-medium">Default Model</label>
                  <select
                    id="default-model"
                    value={draftModel}
                    onChange={(e) => setDraftModel(e.target.value)}
                    className="w-full bg-[var(--bg-input)] border-subtle rounded-xl px-3.5 py-2 text-[13px] outline-none text-[var(--text-main)] transition-colors"
                  >
                    {AVAILABLE_MODELS.map((m) => (
                      <option key={m.id} value={m.id} className="bg-[var(--bg-card)] text-[var(--text-main)]">
                        {m.name} ({m.badge})
                      </option>
                    ))}
                  </select>
                </div>
                <div className="space-y-2 pt-2">
                  {AVAILABLE_MODELS.map((m) => (
                    <button
                      key={m.id}
                      type="button"
                      aria-pressed={draftModel === m.id}
                      onClick={() => setDraftModel(m.id)}
                      className={`w-full text-left p-3.5 rounded-xl border-subtle cursor-pointer transition-colors ${
                        draftModel === m.id ? "bg-[var(--bg-input)] ring-1.5 ring-[var(--accent)]" : "bg-[var(--bg-card)] hover:bg-[var(--bg-card-hover)]"
                      }`}
                    >
                      <div className="flex items-center justify-between mb-1">
                        <span className="text-[13px] font-medium">{m.name}</span>
                        <span className="text-[10px] px-1.5 py-0.5 rounded bg-[var(--bg-card)] border-subtle text-[var(--text-dim)] font-medium">{m.badge}</span>
                      </div>
                      <p className="text-[11.5px] text-[var(--text-dim)] leading-relaxed">{m.desc}</p>
                    </button>
                  ))}
                </div>
              </div>
            )}

            {activeTab === "connection" && (
              <div className="space-y-4 animate-appear">
                <div>
                  <h3 className="text-[15px] font-semibold text-[var(--text-main)]">Engine Connection</h3>
                  <p className="text-[12px] text-[var(--text-dim)] mt-0.5">Configure connection to the Excelsior background engine.</p>
                </div>
                <div className="space-y-1.5 pt-1">
                  <div className="flex items-center justify-between">
                    <label htmlFor="engine-url" className="text-[12.5px] text-[var(--text-muted)] font-medium">WebSocket URL</label>
                    <span className={`px-2 py-0.5 rounded-full font-mono text-[10.5px] font-medium ${statusBg}`}>
                      ● {engineState}
                    </span>
                  </div>
                  <input
                    id="engine-url"
                    value={draftUrl}
                    onChange={(e) => setDraftUrl(e.target.value)}
                    placeholder="ws://localhost:17812/v1/ws"
                    className="w-full bg-[var(--bg-input)] border-subtle rounded-xl px-3.5 py-2 font-mono text-[12.5px] outline-none transition-colors text-[var(--text-main)]"
                  />
                  <p className="text-[11px] text-[var(--text-dim)]">Default local engine runs on port 17812.</p>
                </div>
                <div className="space-y-1.5 pt-2">
                  <label htmlFor="engine-token" className="text-[12.5px] text-[var(--text-muted)] font-medium">Owner Token</label>
                  <input
                    id="engine-token"
                    type="password"
                    autoComplete="off"
                    value={draftToken}
                    onChange={(e) => setDraftToken(e.target.value)}
                    placeholder="Automatic for local; enter for remote"
                    className="w-full bg-[var(--bg-input)] border-subtle rounded-xl px-3.5 py-2 text-[12.5px] outline-none text-[var(--text-main)]"
                  />
                  <p className="text-[11px] text-[var(--text-dim)]">Stored for this browser session only.</p>
                </div>
              </div>
            )}

            {activeTab === "security" && (
              <div className="space-y-4 animate-appear">
                <div>
                  <h3 className="text-[15px] font-semibold text-[var(--text-main)]">Security & Approvals</h3>
                  <p className="text-[12px] text-[var(--text-dim)] mt-0.5">Manage execution safety and permission requirements.</p>
                </div>
                <label className="block mt-2">
                  <span className="font-medium text-[var(--text-main)] text-[13px] block mb-2">
                    Permission mode
                  </span>
                  <select
                    value={draftPermissionMode}
                    onChange={(event) => setDraftPermissionMode(event.target.value as PermissionMode)}
                    className="w-full bg-[var(--bg-input)] border-subtle rounded-xl px-3.5 py-2 text-[12.5px] outline-none text-[var(--text-main)]"
                  >
                    <option value="ask">Ask before actions</option>
                    <option value="allow">Allow actions automatically</option>
                    <option value="deny">Deny actions</option>
                  </select>
                  <span className="block text-[12px] text-[var(--text-dim)] mt-1.5 leading-relaxed">
                    Controls whether file edits and terminal commands need approval.
                  </span>
                </label>
              </div>
            )}
          </div>
        </main>
      </div>
  );
}

export default React.memo(SettingsPage);
