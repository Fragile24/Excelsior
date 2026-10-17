import React from "react";

type IconProps = { className?: string };

export function BrandMark({ className = "brand-mark" }: IconProps) {
  return (
    <svg className={className} width="28" height="28" viewBox="0 0 32 32" fill="none" aria-hidden="true">
      <rect x="1" y="1" width="30" height="30" rx="8" fill="currentColor" />
      <path d="m9 10 6 6-6 6m10 0h5" stroke="var(--bg-card)" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

export const PlusIcon = React.memo(function PlusIcon({ className = "w-4 h-4" }: IconProps) {
  return (
    <svg width="16" height="16" className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
      <path d="M5 12h14" />
      <path d="M12 5v14" />
    </svg>
  );
});

export const PencilIcon = React.memo(function PencilIcon({ className = "w-4 h-4" }: IconProps) {
  return (
    <svg width="16" height="16" className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M17 3a2.85 2.83 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5Z" />
    </svg>
  );
});

export const TrashIcon = React.memo(function TrashIcon({ className = "w-4 h-4" }: IconProps) {
  return (
    <svg width="16" height="16" className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M3 6h18" />
      <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6" />
      <path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
    </svg>
  );
});

export const FolderIcon = React.memo(function FolderIcon({ className = "w-4 h-4" }: IconProps) {
  return (
    <svg width="16" height="16" className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z" />
    </svg>
  );
});

export const FolderPlusIcon = React.memo(function FolderPlusIcon({ className = "w-3.5 h-3.5" }: IconProps) {
  return (
    <svg width="14" height="14" className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round">
      <path d="M12 10v6" />
      <path d="M9 13h6" />
      <path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z" />
    </svg>
  );
});

export const ChevronDownIcon = React.memo(function ChevronDownIcon({ className = "w-3.5 h-3.5" }: IconProps) {
  return (
    <svg width="14" height="14" className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
      <path d="m6 9 6 6 6-6" />
    </svg>
  );
});

export const ChevronRightIcon = React.memo(function ChevronRightIcon({ className = "w-3.5 h-3.5" }: IconProps) {
  return (
    <svg width="14" height="14" className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
      <path d="m9 18 6-6-6-6" />
    </svg>
  );
});

export const SettingsIcon = React.memo(function SettingsIcon({ className = "w-4 h-4" }: IconProps) {
  return (
    <svg width="16" height="16" className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="3" />
      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z" />
    </svg>
  );
});

export const SendArrowIcon = React.memo(function SendArrowIcon({ className = "w-4 h-4" }: IconProps) {
  return (
    <svg width="16" height="16" className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round">
      <path d="M5 12h14" />
      <path d="m12 5 7 7-7 7" />
    </svg>
  );
});

export const CopyIcon = React.memo(function CopyIcon({ className = "w-3.5 h-3.5" }: IconProps) {
  return (
    <svg width="14" height="14" className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round">
      <rect width="14" height="14" x="8" y="8" rx="2" />
      <path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2" />
    </svg>
  );
});

export const CheckIcon = React.memo(function CheckIcon({ className = "w-3.5 h-3.5" }: IconProps) {
  return (
    <svg width="14" height="14" className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round">
      <polyline points="20 6 9 17 4 12" />
    </svg>
  );
});

export const WindowMinimizeIcon = React.memo(function WindowMinimizeIcon({ className = "w-3 h-3" }: IconProps) {
  return (
    <svg width="12" height="12" className={className} viewBox="0 0 24 24" fill="currentColor">
      <path d="M5 12h14v2H5z" />
    </svg>
  );
});

export const WindowMaximizeIcon = React.memo(function WindowMaximizeIcon({ className = "w-3 h-3" }: IconProps) {
  return (
    <svg width="12" height="12" className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
      <rect x="4" y="4" width="16" height="16" rx="1" />
    </svg>
  );
});

export const WindowCloseIcon = React.memo(function WindowCloseIcon({ className = "w-3 h-3" }: IconProps) {
  return (
    <svg width="12" height="12" className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
      <path d="M18 6 6 18" />
      <path d="m6 6 12 12" />
    </svg>
  );
});
