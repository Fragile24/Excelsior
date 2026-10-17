import type { ReasoningEffort } from "./protocol";

export const ENGINE_URL_FALLBACK = "ws://localhost:17812/v1/ws";
export const STORAGE_KEYS = {
  theme: "excelsior-appearance",
  accent: "excelsior-accent",
  surface: "excelsior-surface",
  transcriptSize: "excelsior-transcript-size",
  knownFolders: "excelsior-known-folders",
  reasoningEffort: "excelsior-reasoning-effort",
} as const;

export const DEFAULT_MODEL = "deepseek-v4-flash";
export const DEFAULT_REASONING_EFFORT: ReasoningEffort = "high";
export const DEFAULT_PROJECT = "excelsior";
export const DEFAULT_THEME = "default-light";
export const AVAILABLE_THEMES = [
  { id: "system", name: "System", accent: "#202020", surface: "#e9ecf0" },
  { id: "default-light", name: "Studio light", accent: "#202020", surface: "#e9ecf0" },
  { id: "default-dark", name: "Studio dark", accent: "#ededed", surface: "#1c1e22" },
  { id: "rose-pine-dark", name: "Rosé at night", accent: "#ebbcba", surface: "#191724" },
  { id: "rose-pine-light", name: "Rosé dawn", accent: "#a85862", surface: "#eee6df" },
] as const;
export const ACCENT_COLORS = [
  { name: "Indigo", value: "#6366f1" },
  { name: "Blue", value: "#3b82f6" },
  { name: "Teal", value: "#0d9488" },
  { name: "Green", value: "#16a34a" },
  { name: "Amber", value: "#d97706" },
  { name: "Rose", value: "#e11d48" },
] as const;
export const AVAILABLE_MODELS = [
  { id: "deepseek-v4-flash", name: "DeepSeek V4 Flash", badge: "Flash", desc: "Fast and lightweight for everyday tasks" },
  { id: "deepseek-v4-pro", name: "DeepSeek V4 Pro", badge: "Pro", desc: "Deeper reasoning for complex work" },
] as const;
