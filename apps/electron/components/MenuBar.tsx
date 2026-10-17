import React, { useCallback, useEffect, useRef, useState } from "react";
import { WindowCloseIcon, WindowMaximizeIcon, WindowMinimizeIcon } from "./Icons";
import { AVAILABLE_THEMES } from "../lib/constants";

type MenuId = "file" | "view" | "window" | null;

type MenuBarProps = {
  onNewChat: () => void;
  onOpenFolder: () => void;
  onOpenSettings: () => void;
  onToggleSidebar: () => void;
  currentTheme: string;
  onSaveTheme: (theme: string) => void;
  sessionTokens?: number;
  engineState?: string;
  projectName?: string;
  sessionTitle?: string | null;
};

export function formatTokens(n: number): string {
  return n >= 1000 ? `${(n / 1000).toFixed(1)}k` : `${n}`;
}

type MenuItemProps = {
  onClick: () => void;
  children: React.ReactNode;
  kbd?: string;
  danger?: boolean;
};

const MenuItem = React.memo(function MenuItem({ onClick, children, kbd, danger }: MenuItemProps) {
  return (
    <button
      type="button"
      role="menuitem"
      onClick={() => onClick()}
      className={`w-full text-left px-3 py-1.5 rounded-md hover:bg-[var(--bg-card-hover)] cursor-pointer flex justify-between items-center transition-colors ${danger ? "text-rose-400 hover:text-rose-300 hover:bg-rose-500/10" : "text-[var(--text-main)]"}`}
    >
      <span className="text-[12px] font-medium">{children}</span>
      {kbd && <span className="text-[10px] text-[var(--text-dim)] font-mono bg-[var(--bg-input)] px-1.5 py-0.5 rounded border-subtle">{kbd}</span>}
    </button>
  );
});

function MenuBar({
  onNewChat,
  onOpenFolder,
  onOpenSettings,
  onToggleSidebar,
  currentTheme,
  onSaveTheme,
  sessionTokens,
  engineState = "connected",
  projectName,
  sessionTitle,
}: MenuBarProps) {
  const [openMenu, setOpenMenu] = useState<MenuId>(null);
  const [isDesktop, setIsDesktop] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    setIsDesktop(!!window.electronAPI);
    const handler = (event: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(event.target as Node)) setOpenMenu(null);
    };
    window.addEventListener("mousedown", handler);
    return () => window.removeEventListener("mousedown", handler);
  }, []);

  const handleWindowControl = useCallback((action: "minimize" | "maximize" | "close") => {
    window.electronAPI?.windowControl?.(action);
  }, []);

  const toggleMenu = useCallback((id: MenuId) => {
    setOpenMenu((prev) => (prev === id ? null : id));
  }, []);

  const closeMenu = useCallback(() => setOpenMenu(null), []);

  return (
    <header
      ref={rootRef}
      className={`desktop-menubar ${isDesktop ? "is-desktop" : ""} flex items-center justify-between text-[var(--text-muted)] text-[12px] shrink-0 z-40 select-none`}
      style={{ WebkitAppRegion: "drag" } as React.CSSProperties}
      onKeyDown={(event) => {
        if (event.key === "Escape" && openMenu) {
          event.preventDefault();
          event.stopPropagation();
          setOpenMenu(null);
          rootRef.current?.querySelector<HTMLButtonElement>(`button[data-menu="${openMenu}"]`)?.focus();
        }
      }}
    >
      <div className="flex items-center gap-2" style={{ WebkitAppRegion: "no-drag" } as React.CSSProperties}>
        <nav className="flex items-center gap-0.5" aria-label="Application menu">
          {(["file", "view", ...(isDesktop ? ["window" as const] : [])] as const).map((menu) => (
            <div key={menu} className="relative">
              <button
                type="button"
                data-menu={menu}
                onClick={() => toggleMenu(menu)}
                aria-expanded={openMenu === menu}
                aria-haspopup="menu"
                className={`px-2 py-0.5 rounded hover:bg-[var(--bg-card-hover)] hover:text-[var(--text-main)] capitalize transition-colors ${openMenu === menu ? "bg-[var(--bg-card-hover)] text-[var(--text-main)] font-semibold" : "text-[var(--text-muted)] font-medium"}`}
              >
                {menu}
              </button>

              {openMenu === menu && (
                <div
                  role="menu"
                  className="absolute left-0 top-full mt-1 w-48 bg-[var(--bg-card)] rounded-lg shadow-[var(--popover-shadow)] p-1 text-xs z-50 animate-appear border-subtle"
                  onClick={closeMenu}
                >
                  {menu === "file" && (
                    <>
                      <MenuItem onClick={onNewChat} kbd="Ctrl+N">New Chat</MenuItem>
                      <MenuItem onClick={onOpenFolder} kbd="Ctrl+O">Open Folder…</MenuItem>
                      <div className="my-1.5 h-px bg-[var(--border-subtle)]" />
                      <MenuItem onClick={onOpenSettings} kbd="Ctrl+,">Settings…</MenuItem>
                      {isDesktop && <><div className="my-1.5 h-px bg-[var(--border-subtle)]" />
                      <MenuItem onClick={() => handleWindowControl("close")} danger kbd="Alt+F4">Exit</MenuItem></>}
                    </>
                  )}

                  {menu === "view" && (
                    <>
                      <MenuItem onClick={onToggleSidebar} kbd="Ctrl+B">Toggle Sidebar</MenuItem>
                      <div className="my-1.5 h-px bg-[var(--border-subtle)]" />
                      <div className="px-2.5 py-1 text-[10px] text-[var(--text-dim)] font-semibold uppercase tracking-wider">Themes</div>
                      {AVAILABLE_THEMES.map((t) => (
                        <button
                          key={t.id}
                          type="button"
                          role="menuitemradio"
                          aria-checked={currentTheme === t.id}
                          onClick={() => onSaveTheme(t.id)}
                          className={`w-full text-left px-2.5 py-1.5 rounded-md hover:bg-[var(--bg-card-hover)] cursor-pointer flex justify-between items-center text-xs transition-colors ${currentTheme === t.id ? "text-[var(--text-main)] font-semibold bg-[var(--bg-card-hover)]" : "text-[var(--text-muted)]"}`}
                        >
                          <span>{t.name}</span>
                          {currentTheme === t.id && <span className="text-[11px] font-bold">✓</span>}
                        </button>
                      ))}
                      {isDesktop && <><div className="my-1.5 h-px bg-[var(--border-subtle)]" />
                      <MenuItem onClick={() => window.electronAPI?.toggleDevTools?.()} kbd="F12">Developer Tools</MenuItem></>}
                    </>
                  )}

                  {menu === "window" && (
                    <>
                      <MenuItem onClick={() => handleWindowControl("minimize")}>Minimize</MenuItem>
                      <MenuItem onClick={() => handleWindowControl("maximize")}>Maximize / Restore</MenuItem>
                      <div className="my-1.5 h-px bg-[var(--border-subtle)]" />
                      <MenuItem onClick={() => handleWindowControl("close")} danger>Close</MenuItem>
                    </>
                  )}
                </div>
              )}
            </div>
          ))}
        </nav>
      </div>

      <span className="window-title">Excelsior{sessionTitle && <><span>/</span>{sessionTitle}</>}</span>
      {/* Spacer */}
      <div className="flex-1" />

      {/* Right Controls: Tokens & Window Controls */}
      <div className="flex items-center gap-3 h-full" style={{ WebkitAppRegion: "no-drag" } as React.CSSProperties}>
        {/* Token Counter */}
        {(sessionTokens ?? 0) > 0 && (
          <span
            className="font-mono text-[10.5px] px-2 py-0.5 rounded-full bg-[var(--bg-input)] border-subtle text-[var(--text-dim)]"
            title="Tokens consumed in the last run, including context summaries"
            data-ui-content
          >
            {formatTokens(sessionTokens ?? 0)} consumed
          </span>
        )}

        {/* Window controls */}
        {isDesktop && <div className="flex items-center h-full ml-1">
          <button
            type="button"
            aria-label="Minimize window"
            aria-describedby="wc-min"
            onClick={() => handleWindowControl("minimize")}
            className="h-full px-3 text-[var(--text-dim)] hover:text-[var(--text-main)] hover:bg-[var(--bg-card-hover)] transition-colors"
          >
            <WindowMinimizeIcon className="w-3 h-3" />
          </button>
          <span id="wc-min" role="tooltip" className="sr-only">Minimize window</span>
          <button
            type="button"
            aria-label="Maximize window"
            aria-describedby="wc-max"
            onClick={() => handleWindowControl("maximize")}
            className="h-full px-3 text-[var(--text-dim)] hover:text-[var(--text-main)] hover:bg-[var(--bg-card-hover)] transition-colors"
          >
            <WindowMaximizeIcon className="w-3 h-3" />
          </button>
          <span id="wc-max" role="tooltip" className="sr-only">Maximize window</span>
          <button
            type="button"
            aria-label="Close window"
            aria-describedby="wc-close"
            onClick={() => handleWindowControl("close")}
            className="h-full px-3 text-[var(--text-dim)] hover:text-white hover:bg-rose-600 transition-colors"
          >
            <WindowCloseIcon className="w-3 h-3" />
          </button>
          <span id="wc-close" role="tooltip" className="sr-only">Close window</span>
        </div>}
      </div>
    </header>
  );
}

export default React.memo(MenuBar);
