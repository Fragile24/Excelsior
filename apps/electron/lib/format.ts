import { DEFAULT_PROJECT } from "./constants";

export function workspaceName(path?: string | null): string {
  return path?.split(/[/\\]/).filter(Boolean).pop() ?? DEFAULT_PROJECT;
}

export function formatTimeAgo(updatedAt?: string, id?: string): string {
  const formatDiff = (ms: number): string => {
    const minutes = Math.floor(ms / 60000);
    if (minutes < 1) return "Just now";
    if (minutes < 60) return `${minutes}m`;
    const hours = Math.floor(minutes / 60);
    if (hours < 24) return `${hours}h`;
    return `${Math.floor(hours / 24)}d`;
  };

  if (updatedAt) {
    const date = new Date(updatedAt);
    if (!Number.isNaN(date.getTime()))
      return formatDiff(Date.now() - date.getTime());
  }

  if (id && /^\d+$/.test(id)) {
    const ts = Number.parseInt(id, 10);
    if (ts > 1e12) return formatDiff(Date.now() - ts);
  }

  return "";
}

export function cleanTitle(title?: string): string {
  if (!title || !title.trim() || title === "(empty)") return "New Chat";
  return title;
}
