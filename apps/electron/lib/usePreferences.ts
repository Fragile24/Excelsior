"use client";

import { useEffect, useState } from "react";
import { AVAILABLE_THEMES, DEFAULT_REASONING_EFFORT, DEFAULT_THEME, STORAGE_KEYS } from "./constants";
import type { ReasoningEffort } from "./protocol";

export function readAccentColor(raw: string | null): string | null {
  return raw && /^#[\da-f]{6}$/i.test(raw) ? raw.toLowerCase() : null;
}

export function readTranscriptSize(raw: string | null): number {
  const size = Number(raw);
  return Number.isInteger(size) && size >= 12 && size <= 18 ? size : 14;
}

export function readReasoningEffort(raw: string | null): ReasoningEffort {
  return raw === "none" || raw === "low" || raw === "high" || raw === "max" ? raw : DEFAULT_REASONING_EFFORT;
}

export function accentInk(color: string): string {
  const channels = color.slice(1).match(/.{2}/g)?.map((channel) => parseInt(channel, 16) / 255);
  if (!channels || channels.length !== 3) return "#ffffff";
  const [r, g, b] = channels.map((value) => value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4);
  return 0.2126 * r + 0.7152 * g + 0.0722 * b > 0.179 ? "#202020" : "#ffffff";
}

export function readWorkspaces(raw: string | null): string[] {
  try {
    const items: unknown = JSON.parse(raw ?? "[]");
    if (!Array.isArray(items)) return [];
    const paths = items.map((item) => typeof item === "string" ? item : item?.path);
    return [...new Set(paths.filter((path): path is string => typeof path === "string" && !!path.trim()))];
  } catch {
    return [];
  }
}

// Local preferences belong to the shell; workspace paths are their identities.
export function usePreferences(workspace: string | null) {
  const [theme, setTheme] = useState<string>(DEFAULT_THEME);
  const [accentColor, setAccentColor] = useState<string | null>(null);
  const [surfaceColor, setSurfaceColor] = useState<string | null>(null);
  const [transcriptSize, setTranscriptSize] = useState(14);
  const [reasoningEffort, setReasoningEffort] = useState<ReasoningEffort>(DEFAULT_REASONING_EFFORT);
  const [workspaces, setWorkspaces] = useState<string[]>([]);
  const [loaded, setLoaded] = useState(false);
  useEffect(() => {
    try {
      const saved = localStorage.getItem(STORAGE_KEYS.theme);
      setTheme(AVAILABLE_THEMES.some((item) => item.id === saved) ? saved! : DEFAULT_THEME);
      setAccentColor(readAccentColor(localStorage.getItem(STORAGE_KEYS.accent)));
      setSurfaceColor(readAccentColor(localStorage.getItem(STORAGE_KEYS.surface)));
      setTranscriptSize(readTranscriptSize(localStorage.getItem(STORAGE_KEYS.transcriptSize)));
      setReasoningEffort(readReasoningEffort(localStorage.getItem(STORAGE_KEYS.reasoningEffort)));
      setWorkspaces(readWorkspaces(localStorage.getItem(STORAGE_KEYS.knownFolders)));
    } catch { /* Preferences are optional when storage is unavailable. */ }
    setLoaded(true);
  }, []);
  useEffect(() => {
    const root = document.documentElement;
    const systemTheme = window.matchMedia("(prefers-color-scheme: dark)");
    const applyTheme = () => {
      root.dataset.theme = theme === "system"
        ? (systemTheme.matches ? "default-dark" : "default-light")
        : theme;
    };
    applyTheme();
    if (theme === "system") systemTheme.addEventListener("change", applyTheme);
    if (accentColor) {
      root.style.setProperty("--accent", accentColor);
      root.style.setProperty("--accent-ink", accentInk(accentColor));
    } else {
      root.style.removeProperty("--accent");
      root.style.removeProperty("--accent-ink");
    }
    if (surfaceColor) {
      const ink = accentInk(surfaceColor);
      root.style.setProperty("--bg-canvas", surfaceColor);
      root.style.setProperty("--bg-sidebar", `color-mix(in srgb, ${surfaceColor} 94%, ${ink})`);
      root.style.setProperty("--bg-card", `color-mix(in srgb, ${surfaceColor} 88%, ${ink})`);
      root.style.setProperty("--bg-composer", `color-mix(in srgb, ${surfaceColor} 84%, ${ink})`);
      root.style.setProperty("--bg-input", `color-mix(in srgb, ${surfaceColor} 96%, ${ink})`);
      root.style.setProperty("--bg-header", `color-mix(in srgb, ${surfaceColor} 97%, ${ink})`);
      root.style.setProperty("--bg-card-hover", `color-mix(in srgb, ${surfaceColor} 80%, ${ink})`);
      root.style.setProperty("--text-main", ink);
      root.style.setProperty("--text-muted", `color-mix(in srgb, ${ink} 68%, ${surfaceColor})`);
      root.style.setProperty("--text-dim", `color-mix(in srgb, ${ink} 50%, ${surfaceColor})`);
      root.style.setProperty("--border-subtle", `color-mix(in srgb, ${ink} 11%, transparent)`);
      root.style.setProperty("--border-hover", `color-mix(in srgb, ${ink} 20%, transparent)`);
    } else {
      for (const property of ["--bg-canvas", "--bg-sidebar", "--bg-card", "--bg-composer", "--bg-input", "--bg-header", "--bg-card-hover", "--text-main", "--text-muted", "--text-dim", "--border-subtle", "--border-hover"]) {
        root.style.removeProperty(property);
      }
    }
    root.style.setProperty("--transcript-size", `${transcriptSize}px`);
    if (loaded) {
      try {
        localStorage.setItem(STORAGE_KEYS.theme, theme);
        if (accentColor) localStorage.setItem(STORAGE_KEYS.accent, accentColor);
        else localStorage.removeItem(STORAGE_KEYS.accent);
        if (surfaceColor) localStorage.setItem(STORAGE_KEYS.surface, surfaceColor);
        else localStorage.removeItem(STORAGE_KEYS.surface);
        localStorage.setItem(STORAGE_KEYS.transcriptSize, String(transcriptSize));
        localStorage.setItem(STORAGE_KEYS.reasoningEffort, reasoningEffort);
      } catch { /* Keep the in-memory preference. */ }
    }
    return () => systemTheme.removeEventListener("change", applyTheme);
  }, [theme, accentColor, surfaceColor, transcriptSize, reasoningEffort, loaded]);
  useEffect(() => {
    if (loaded && workspace) setWorkspaces((paths) => paths.includes(workspace) ? paths : [...paths, workspace]);
  }, [workspace, loaded]);
  useEffect(() => {
    if (loaded) {
      try { localStorage.setItem(STORAGE_KEYS.knownFolders, JSON.stringify(workspaces)); } catch { /* Keep the in-memory paths. */ }
    }
  }, [workspaces, loaded]);
  return { theme, setTheme, accentColor, setAccentColor, surfaceColor, setSurfaceColor, transcriptSize, setTranscriptSize, reasoningEffort, setReasoningEffort, workspaces };
}
