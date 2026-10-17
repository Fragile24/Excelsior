"use client";

import React, { useCallback, useEffect, useState } from "react";
import { ArrowRight, ArrowUpRight, MessageSquare, PanelLeft } from "../components/phosphor-icons";
import Sidebar from "../components/Sidebar";
import Composer from "../components/Composer";
import ContextControl from "../components/ContextControl";
import SettingsPage, { type SettingsValues } from "../components/SettingsPage";
import MenuBar from "../components/MenuBar";
import AskDialog from "../components/AskDialog";
import Transcript from "../components/Transcript";
import { useEngine } from "../lib/useEngine";
import { usePreferences } from "../lib/usePreferences";
import { formatTimeAgo, workspaceName } from "../lib/format";
import { DEFAULT_MODEL, ENGINE_URL_FALLBACK } from "../lib/constants";
import { ErrorBoundary } from "../components/ErrorBoundary";
import type { PermissionMode, WorktreeInfo } from "../lib/protocol";

export default function Page() {
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [settingsNavOpen, setSettingsNavOpen] = useState(true);
  const [model, setModel] = useState(DEFAULT_MODEL);
  const [engineUrl, setEngineUrl] = useState("");
  const [worktrees, setWorktrees] = useState<WorktreeInfo[]>([]);
  const { state, conversation, run, reconnect, reportError, setCompactNext } = useEngine(engineUrl);
  const { theme, setTheme, accentColor, setAccentColor, surfaceColor, setSurfaceColor, transcriptSize, setTranscriptSize, reasoningEffort, setReasoningEffort, workspaces } = usePreferences(state.workspace);
  const { blocks, running: streaming, pending, usage } = conversation;
  const ask = pending?.type === "ask.req" ? pending.payload : null;
  const permission = pending?.type === "permission.req" ? pending.payload : null;
  const activeSession = state.sessions.find((s) => s.id === state.activeId);
  const openSettings = useCallback(() => setSettingsOpen(true), []);
  const toggleSidebar = useCallback(() => setSidebarOpen((open) => !open), []);
  const toggleSettingsNav = useCallback(() => setSettingsNavOpen((open) => !open), []);

  useEffect(() => {
    if (window.matchMedia("(max-width: 720px)").matches) setSidebarOpen(false);
  }, []);

  useEffect(() => {
    let alive = true;
    const url = window.electronAPI?.getEngineUrl?.() ?? Promise.resolve(ENGINE_URL_FALLBACK);
    void url.then((value) => { if (alive) setEngineUrl(value || ENGINE_URL_FALLBACK); })
      .catch((error) => { if (alive) reportError(String(error)); });
    return () => { alive = false; };
  }, [reportError]);

  useEffect(() => {
    if (!state.workspace || state.status !== "connected") {
      setWorktrees([]);
      return;
    }
    let alive = true;
    void run((client) => client.listWorktrees()).then((items) => {
      if (alive && items) setWorktrees(items);
    });
    return () => { alive = false; };
  }, [state.workspace, state.status, run]);

  const handleNewChat = useCallback((path?: string) => run(async (client) => {
    if (path && path !== state.workspace) await client.setWorkspace(path);
    return client.createSession();
  }), [run, state.workspace]);

  const handleSelectSession = useCallback((path: string, id: string) => run(async (client) => {
    if (window.matchMedia("(max-width: 720px)").matches) setSidebarOpen(false);
    if (path !== state.workspace) await client.setWorkspace(path);
    return client.selectSession(id);
  }), [run, state.workspace]);

  const handleOpenFolder = useCallback(async () => {
    if (!window.electronAPI?.openFolderDialog) {
      reportError("Open folder is available in the desktop app.");
      return;
    }
    try {
      const picked = await window.electronAPI.openFolderDialog();
      if (picked) await run(async (client) => {
        const confirmed = await client.setWorkspace(picked);
        return client.createSession(workspaceName(confirmed) + " session");
      });
    } catch (error) { reportError(String(error)); }
  }, [run, reportError]);

  const handleSendPrompt = useCallback(async (raw: string) => {
    const text = raw.trim();
    return !!text && !!(await run((client) => client.startChat(text, model, state.activeId, conversation.compactNext, reasoningEffort)));
  }, [run, model, state.activeId, conversation.compactNext, reasoningEffort]);

  const handleCompact = useCallback(async () => {
    const { activeId } = state;
    const { runId } = conversation;
    if (streaming && activeId && runId) {
      await run((client) => client.compactRun(activeId, runId));
    } else {
      setCompactNext(!conversation.compactNext);
    }
  }, [run, streaming, state.activeId, conversation.runId, conversation.compactNext, setCompactNext]);

  const cancelRun = useCallback(() => {
    const { activeId } = state;
    const { runId } = conversation;
    if (activeId && runId) void run((client) => client.cancelRun(activeId, runId));
  }, [run, state.activeId, conversation.runId]);

  const handleDeleteSession = useCallback((id: string) => { void run((client) => client.deleteSession(id)); }, [run]);
  const handleRenameSession = useCallback((id: string, title: string) => {
    if (title.trim()) void run((client) => client.renameSession(id, title.trim()));
  }, [run]);

  const handleCreateWorktree = useCallback(async (branch: string) => {
    const created = await run((client) => client.createWorktree(branch));
    if (!created) return null;
    setWorktrees((items) => items.some((item) => item.path === created.path) ? items : [...items, created]);
    return created;
  }, [run]);

  const handleSelectWorktree = useCallback((path: string) => {
    if (path !== state.workspace) void run((client) => client.setWorkspace(path));
  }, [run, state.workspace]);

  const handleSelectPermission = useCallback(async (mode: PermissionMode) => {
    await run((client) => client.setSettings({ permission: mode }));
  }, [run]);

  const handleAnswerAsk = useCallback((selected: number, label: string, input: string) => {
    if (!ask) return;
    const answer = (selected === -1 ? input : label).trim();
    if (selected !== -1 && !answer) return;
    const { sessionId, runId, interactionId } = ask;
    void run((client) => client.replyAsk({ sessionId, runId, interactionId, selected, answer, label: answer }));
  }, [run, ask]);

  const handlePermissionDecision = useCallback((approved: boolean) => {
    if (!permission) return;
    const { sessionId, runId, interactionId } = permission;
    void run((client) => client.replyPermission({ sessionId, runId, interactionId, approved }));
  }, [run, permission]);

  async function saveSettings(values: SettingsValues) {
    if (values.url === engineUrl && state.status === "connected" && values.permission !== state.permissionMode) {
      await run((client) => client.setSettings({ permission: values.permission }));
    }
    if (values.token) {
      try { sessionStorage.setItem("engine-token:" + values.url, values.token); }
      catch (error) { reportError(String(error)); return; }
    }
    setModel(values.model);
    setEngineUrl(values.url);
    if (values.url === engineUrl && values.token) reconnect();
    setSettingsOpen(false);
  }

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if (event.defaultPrevented) return;
      if (settingsOpen) {
        if (event.key === "Escape") { event.preventDefault(); setSettingsOpen(false); return; }
        if (!(event.ctrlKey || event.metaKey)) return;
        if (event.key.toLowerCase() === "b") { event.preventDefault(); toggleSettingsNav(); }
        if (event.key === ",") { event.preventDefault(); setSettingsOpen(false); }
        return;
      }
      if (event.key === "Escape") {
        if (sidebarOpen && window.matchMedia("(max-width: 720px)").matches) { setSidebarOpen(false); return; }
        if (ask) handleAnswerAsk(-1, "", "");
        else if (permission) handlePermissionDecision(false);
        else if (streaming) cancelRun();
      }
      if (!(event.ctrlKey || event.metaKey)) return;
      switch (event.key.toLowerCase()) {
        case "b": event.preventDefault(); toggleSidebar(); break;
        case "n": event.preventDefault(); void handleNewChat(); break;
        case "o": event.preventDefault(); void handleOpenFolder(); break;
        case ",": event.preventDefault(); openSettings(); break;
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [settingsOpen, sidebarOpen, ask, permission, streaming, cancelRun, handleAnswerAsk, handlePermissionDecision, handleNewChat, handleOpenFolder, toggleSidebar, toggleSettingsNav, openSettings]);

  const isLanding = blocks.length === 0;
  const composer = (mode: "centered" | "docked") => (
    <Composer mode={mode} selectedModel={model} onSelectModel={setModel} reasoningEffort={reasoningEffort} onSelectReasoningEffort={setReasoningEffort}
      permissionMode={state.permissionMode} onSelectPermission={handleSelectPermission}
      onSend={handleSendPrompt} onStop={cancelRun} isStreaming={streaming}
      disabled={state.status !== "connected"}
      contextControl={<ContextControl key={state.workspace + ":" + state.activeId}
        context={conversation.context} running={streaming} queued={conversation.compactQueued}
        compactNext={conversation.compactNext} hasHistory={blocks.length > 0}
        connected={state.status === "connected"} onCompact={handleCompact} />} />
  );

  return (
    <ErrorBoundary>
      <div className="studio-app animate-appear">
        <MenuBar
          onNewChat={handleNewChat}
          onOpenFolder={handleOpenFolder}
          onOpenSettings={openSettings}
          onToggleSidebar={settingsOpen ? toggleSettingsNav : toggleSidebar}
          currentTheme={theme}
          sessionTokens={usage.total}
          onSaveTheme={setTheme}
          engineState={state.status}
          projectName={workspaceName(state.workspace)}
          sessionTitle={activeSession?.title ?? null}
        />
        <div className="studio-workspace">
          {settingsOpen ? <SettingsPage
            navOpen={settingsNavOpen}
            onToggleNav={toggleSettingsNav}
            onClose={() => setSettingsOpen(false)}
            engineUrl={engineUrl}
            engineState={state.status}
            defaultModel={model}
            currentTheme={theme}
            onSaveTheme={setTheme}
            accentColor={accentColor}
            onSaveAccent={setAccentColor}
            surfaceColor={surfaceColor}
            onSaveSurface={setSurfaceColor}
            transcriptSize={transcriptSize}
            onSaveTranscriptSize={setTranscriptSize}
            permissionMode={state.permissionMode}
            onSave={saveSettings}
          /> : <>
          {sidebarOpen && <button type="button" className="sidebar-backdrop" aria-label="Close sidebar" onClick={toggleSidebar} />}
          <Sidebar
            isOpen={sidebarOpen}
            connected={state.status === "connected"}
            onToggleSidebar={toggleSidebar}
            workspaces={workspaces}
            workspace={state.workspace}
            worktrees={worktrees}
            sessions={state.sessions}
            activeSessionId={state.activeId}
            onSelectSession={handleSelectSession}
            onNewSession={handleNewChat}
            onDeleteSession={handleDeleteSession}
            onRenameSession={handleRenameSession}
            onCreateWorktree={handleCreateWorktree}
            onSelectWorktree={handleSelectWorktree}
            onOpenFolder={handleOpenFolder}
            onOpenSettings={openSettings}
          />

          <main className="studio-canvas">
            {!sidebarOpen && <button type="button" className="sidebar-reopen" onClick={toggleSidebar}
              aria-label="Open sidebar" title="Open sidebar (Ctrl+B)"><PanelLeft size={18} strokeWidth={1.6} aria-hidden="true" /></button>}
            {state.error && <div role="alert" className="workspace-error"><span>{state.error}</span><button type="button" onClick={openSettings}>Preferences <ArrowUpRight size={13} /></button></div>}
            {isLanding ? (
              <div className="landing-scroll">
              <div className="studio-landing animate-appear">
                <div className="landing-heading">
                  <h1>What are we building?</h1>
                </div>
                <div className="w-full">
                  {composer("centered")}
                </div>
                {state.workspace && state.sessions.some((session) => session.count > 0) && <section className="recent-sessions" aria-label="Recent sessions">
                  <div className="recent-heading"><h2>Recent sessions</h2><button type="button" onClick={() => setSidebarOpen(true)}>View all <ArrowRight size={12} /></button></div>
                  {state.sessions.filter((session) => session.count > 0).slice(0, 3).map((session) => <button type="button" className="recent-session" key={session.id} onClick={() => handleSelectSession(state.workspace!, session.id)}>
                    <MessageSquare size={15} strokeWidth={1.5} aria-hidden="true" /><span>{session.title}</span><time>{formatTimeAgo(session.updatedAt, session.id)}</time><ArrowUpRight size={14} aria-hidden="true" />
                  </button>)}
                </section>}
              </div>
              </div>
            ) : (
              <div key={state.activeId ?? "session"} className="flex-1 flex flex-col h-full min-h-0 animate-appear">
                <Transcript
                  blocks={blocks}
                  streaming={streaming}
                  permission={permission}
                  onPermissionDecision={handlePermissionDecision}
                  onAllowAlways={() => { void run((client) => client.setSettings({ permission: "allow" })); handlePermissionDecision(true); }}
                />
                <div className="composer-dock">
                  {ask ? <AskDialog key={ask.interactionId} ask={ask} onAnswer={handleAnswerAsk} /> : composer("docked")}
                </div>
              </div>
            )}
          </main>
          </>}
        </div>
      </div>
    </ErrorBoundary>
  );
}
