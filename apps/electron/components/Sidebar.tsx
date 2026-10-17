import React, { useCallback, useState } from "react";
import { ChevronRight, Folder, FolderOpen, FolderPlus, GitBranch, MessageSquare, PanelLeftClose, Plus, Search, Settings2, X } from "./phosphor-icons";
import { BrandMark, PencilIcon, TrashIcon } from "./Icons";
import { cleanTitle, formatTimeAgo, workspaceName } from "../lib/format";
import type { SessionInfo, WorktreeInfo } from "../lib/protocol";

type SidebarProps = {
  isOpen: boolean;
  connected: boolean;
  workspaces: string[];
  workspace: string | null;
  worktrees: WorktreeInfo[];
  sessions: SessionInfo[];
  activeSessionId: string | null;
  onSelectSession: (folderId: string, sessionId: string) => void;
  onNewSession?: (folderId?: string) => void;
  onDeleteSession?: (id: string) => void;
  onRenameSession?: (id: string, title: string) => void;
  onCreateWorktree?: (branch: string) => Promise<WorktreeInfo | null>;
  onSelectWorktree?: (path: string) => void;
  onOpenFolder: () => void;
  onOpenSettings: () => void;
  onToggleSidebar: () => void;
};

const SessionRow = React.memo(function SessionRow({ folderId, session, isActive, onSelectSession, onDeleteSession, onRenameSession }: {
  folderId: string;
  session: SessionInfo;
  isActive: boolean;
  onSelectSession: SidebarProps["onSelectSession"];
  onDeleteSession?: SidebarProps["onDeleteSession"];
  onRenameSession?: SidebarProps["onRenameSession"];
}) {
  const [editing, setEditing] = useState(false);
  const editingRef = React.useRef(false);
  const [draft, setDraft] = useState(session.title);
  const startEdit = () => { setDraft(session.title); editingRef.current = true; setEditing(true); };
  const commit = () => {
    if (!editingRef.current) return;
    editingRef.current = false;
    setEditing(false);
    if (draft.trim() && draft.trim() !== session.title) onRenameSession?.(session.id, draft.trim());
  };

  return (
    <div className={`session-row ${isActive ? "is-active" : ""}`}>
      {editing ? (
        <input
          autoFocus
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          onBlur={commit}
          onKeyDown={(event) => {
            if (event.key === "Enter") { event.preventDefault(); commit(); }
            if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); editingRef.current = false; setEditing(false); }
          }}
          aria-label={`Rename ${session.title}`}
          className="session-rename"
        />
      ) : (
        <button type="button" className="session-select" onClick={() => onSelectSession(folderId, session.id)}
          onDoubleClick={onRenameSession ? startEdit : undefined} aria-current={isActive ? "page" : undefined}>
          <MessageSquare size={17} strokeWidth={1.5} aria-hidden="true" />
          <span className="session-info">
            <span className="session-title">{cleanTitle(session.title)}</span>
            <span className="session-meta">
              <span>{formatTimeAgo(session.updatedAt, session.id)}</span>
              {!!session.added && <span className="text-emerald-500">+{session.added}</span>}
              {!!session.deleted && <span className="text-rose-500">−{session.deleted}</span>}
              {session.branch && <span className="session-branch"><GitBranch size={12} aria-hidden="true" />{session.branch}</span>}
            </span>
          </span>
        </button>
      )}
      {!editing && <div className="session-actions">
        {onRenameSession && <button type="button" onClick={startEdit} aria-label={`Rename ${session.title}`} title="Rename session"><PencilIcon className="w-3.5 h-3.5" /></button>}
        {onDeleteSession && <button type="button" onClick={() => onDeleteSession(session.id)} aria-label={`Delete ${session.title}`} title="Delete session"><TrashIcon className="w-3.5 h-3.5" /></button>}
      </div>}
    </div>
  );
});

function Sidebar({ isOpen, connected, workspaces, workspace, worktrees, sessions, activeSessionId, onSelectSession, onNewSession, onDeleteSession, onRenameSession, onCreateWorktree, onSelectWorktree, onOpenFolder, onOpenSettings, onToggleSidebar }: SidebarProps) {
  const [collapsedFolders, setCollapsedFolders] = useState<Record<string, boolean>>({});
  const [query, setQuery] = useState("");
  const [creatingWorktree, setCreatingWorktree] = useState(false);
  const [worktreeBranch, setWorktreeBranch] = useState("");
  const [savingWorktree, setSavingWorktree] = useState(false);
  const [worktreeError, setWorktreeError] = useState("");
  const toggleFolder = useCallback((path: string) => setCollapsedFolders((previous) => ({ ...previous, [path]: !previous[path] })), []);
  const createWorktree = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const branch = worktreeBranch.trim();
    if (!branch || savingWorktree || !connected || !onCreateWorktree) return;
    setSavingWorktree(true);
    setWorktreeError("");
    try {
      const created = await onCreateWorktree(branch);
      if (!created) { setWorktreeError("Could not create branch"); return; }
      setWorktreeBranch("");
      setCreatingWorktree(false);
    } finally {
      setSavingWorktree(false);
    }
  };
  if (!isOpen) return null;

  return (
    <aside id="workspace-sidebar" className="studio-sidebar animate-appear" aria-label="Workspace navigation">
      <div className="sidebar-brand">
        <span className="brand-wordmark"><BrandMark />Excelsior</span>
        <button type="button" className="studio-icon" onClick={onToggleSidebar} aria-label="Collapse sidebar" title="Collapse sidebar (Ctrl+B)"><PanelLeftClose size={19} strokeWidth={1.5} /></button>
      </div>
      <div className="sidebar-primary">
        <button type="button" className="new-session-button" onClick={() => onNewSession?.()} disabled={!connected} title={connected ? "New session (Ctrl+N)" : "Connect your engine to start a session"}>
          <Plus size={19} strokeWidth={1.6} /><span>New session</span><kbd>Ctrl N</kbd>
        </button>
        <div className="session-search">
          <Search size={17} strokeWidth={1.6} aria-hidden="true" />
          <input type="search" placeholder="Search sessions" aria-label="Find a session" value={query} onChange={(event) => setQuery(event.target.value)} onKeyDown={(event) => {
            if (event.key === "Escape" && query) { event.preventDefault(); event.stopPropagation(); setQuery(""); }
          }} />
          {query && <button type="button" onClick={() => setQuery("")} aria-label="Clear session search"><X size={15} /></button>}
        </div>
      </div>
      <div className="sidebar-section-heading">
        <span>Projects</span>
        <button type="button" className="studio-icon" onClick={onOpenFolder} aria-label="Open folder" title="Open folder (Ctrl+O)"><FolderPlus size={18} strokeWidth={1.5} /></button>
      </div>
      <div className="sidebar-projects">
        {workspaces.length === 0 && <div className="sidebar-empty">
          <button type="button" onClick={onOpenFolder}><FolderOpen size={18} />Open folder</button>
        </div>}
        {workspaces.map((path) => {
          const name = workspaceName(path);
          const current = path === workspace;
          const items = current ? sessions.filter((session) => session.title.toLowerCase().includes(query.toLowerCase())) : [];
          const trackedWorktrees = current ? worktrees.filter((item) => !query || `${item.branch ?? ""} ${item.path}`.toLowerCase().includes(query.toLowerCase())) : [];
          const collapsed = !!collapsedFolders[path] && !query;
          return (
            <div key={path} className="project-group">
              <div className="project-heading">
                <button type="button" className="project-toggle" aria-expanded={!collapsed} onClick={() => toggleFolder(path)} title={path}>
                  <ChevronRight size={14} className={collapsed ? "" : "rotate-90"} aria-hidden="true" />
                  <Folder size={18} strokeWidth={1.5} aria-hidden="true" /><span>{name}</span>
                </button>
                {onNewSession && <button type="button" className="studio-icon" disabled={!connected} title={`New session in ${name}`} aria-label={`New session in ${name}`} onClick={() => onNewSession(path)}><Plus size={18} /></button>}
              </div>
              {!collapsed && <div className="project-sessions">
                {items.length === 0 ? <p className="sessions-empty">{query && current ? "No matching sessions" : current ? "No sessions yet" : "Start a session in this project"}</p>
                  : items.map((session) => <SessionRow key={session.id} folderId={path} session={session} isActive={current && activeSessionId === session.id}
                    onSelectSession={onSelectSession} onDeleteSession={onDeleteSession} onRenameSession={onRenameSession} />)}
                {current && worktrees.length > 0 && <section className="worktree-section" aria-label="Git worktrees">
                  <div className="worktree-heading"><span>Worktrees</span>
                    {onCreateWorktree && <button type="button" className="studio-icon" disabled={!connected || savingWorktree} onClick={() => { setCreatingWorktree((value) => !value); setWorktreeError(""); }} aria-label="Create worktree" title="Create worktree"><Plus size={15} /></button>}
                  </div>
                  {trackedWorktrees.map((item) => <button type="button" className={`worktree-row ${item.current ? "is-current" : ""}`} key={item.path} title={item.path} disabled={!connected || item.current} onClick={() => onSelectWorktree?.(item.path)} aria-current={item.current ? "page" : undefined}>
                    <GitBranch size={14} aria-hidden="true" />
                    <span>{item.branch || `Detached at ${item.head?.slice(0, 7) ?? "HEAD"}`}</span>
                    {item.current && <small>Current</small>}
                    {!item.current && item.main && <small>Main</small>}
                  </button>)}
                  {trackedWorktrees.length === 0 && query && <p className="worktree-empty">No matching worktrees</p>}
                  {creatingWorktree && <form className="worktree-create" onSubmit={(event) => void createWorktree(event)}>
                    <span className="worktree-origin">From current HEAD</span>
                    <input autoFocus value={worktreeBranch} onChange={(event) => setWorktreeBranch(event.target.value)} placeholder="New branch" aria-label="New worktree branch" onKeyDown={(event) => { if (event.key === "Escape") { setCreatingWorktree(false); setWorktreeBranch(""); } }} />
                    <button type="submit" disabled={!worktreeBranch.trim() || !connected || savingWorktree}>{savingWorktree ? "Creating…" : "Create"}</button>
                    {worktreeError && <span role="status">{worktreeError}</span>}
                  </form>}
                </section>}
              </div>}
            </div>
          );
        })}
      </div>
      <div className="sidebar-bottom">
        <button type="button" onClick={onOpenFolder}><FolderOpen size={18} strokeWidth={1.5} /><span>Open project</span><kbd>Ctrl O</kbd></button>
        <button type="button" onClick={onOpenSettings}><Settings2 size={18} strokeWidth={1.5} /><span>Preferences</span><kbd>Ctrl ,</kbd></button>
        <div className="sidebar-local"><span className={`status-dot ${connected ? "online" : ""}`} />{connected ? "Engine connected" : "Engine disconnected"}</div>
      </div>
    </aside>
  );
}

export default React.memo(Sidebar);
