import React, { useEffect, useId, useRef, useState } from "react";
import { Check, ChevronDown, type PhosphorIcon } from "./phosphor-icons";

type PickerOption<T extends string> = {
  value: T;
  label: string;
  description: string;
  icon: PhosphorIcon;
};

export default function ComposerPicker<T extends string>({ label, value, text, icon: Icon, options, disabled, busy, title, onSelect }: {
  label: string;
  value: T;
  text: string;
  icon: PhosphorIcon;
  options: readonly PickerOption<T>[];
  disabled?: boolean;
  busy?: boolean;
  title?: string;
  onSelect: (value: T) => void;
}) {
  const id = useId();
  const trigger = useRef<HTMLButtonElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);

  useEffect(() => {
    const menu = panel.current;
    const toggle = () => setOpen(!!menu?.matches(":popover-open"));
    menu?.addEventListener("toggle", toggle);
    return () => menu?.removeEventListener("toggle", toggle);
  }, []);

  useEffect(() => {
    if (disabled || busy) panel.current?.hidePopover();
  }, [disabled, busy]);

  useEffect(() => {
    if (!open) return;
    const close = () => panel.current?.hidePopover();
    window.addEventListener("resize", close);
    return () => window.removeEventListener("resize", close);
  }, [open]);

  const close = () => {
    panel.current?.hidePopover();
    trigger.current?.focus();
  };

  const show = () => {
    const menu = panel.current;
    const button = trigger.current;
    if (!menu || !button || disabled || busy) return;
    if (menu.matches(":popover-open")) { close(); return; }
    menu.showPopover();
    const anchor = button.getBoundingClientRect();
    const { width, height } = menu.getBoundingClientRect();
    menu.style.left = `${Math.max(12, Math.min(anchor.left, window.innerWidth - width - 12))}px`;
    const top = anchor.top >= height + 12 ? anchor.top - height - 8 : anchor.bottom + 8;
    menu.style.top = `${Math.max(12, Math.min(top, window.innerHeight - height - 12))}px`;
    menu.querySelector<HTMLButtonElement>('[aria-checked="true"]')?.focus();
  };

  return (
    <div className="composer-picker" onBlur={(event) => {
      if (!event.currentTarget.contains(event.relatedTarget)) panel.current?.hidePopover();
    }}>
      <button ref={trigger} type="button" className="composer-picker-trigger" disabled={disabled} title={title}
        aria-label={`${label}: ${options.find((option) => option.value === value)?.label ?? text}`}
        aria-haspopup="menu" aria-expanded={open} aria-controls={id} aria-busy={busy} aria-disabled={disabled || busy} onClick={show}
        onKeyDown={(event) => {
          if (event.key === "ArrowDown" || event.key === "ArrowUp") { event.preventDefault(); show(); }
        }}>
        <Icon size={15} strokeWidth={1.6} aria-hidden="true" />
        <span>{text}</span><ChevronDown size={12} aria-hidden="true" />
      </button>
      <div ref={panel} id={id} {...{ popover: "auto" }} role="menu" aria-label={label} className="composer-picker-panel"
        onKeyDown={(event) => {
          if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); close(); return; }
          const buttons = Array.from(event.currentTarget.querySelectorAll<HTMLButtonElement>('[role="menuitemradio"]'));
          const index = buttons.indexOf(document.activeElement as HTMLButtonElement);
          const next = event.key === "Home" ? 0 : event.key === "End" ? buttons.length - 1
            : event.key === "ArrowDown" ? (index + 1) % buttons.length
            : event.key === "ArrowUp" ? (index + buttons.length - 1) % buttons.length : -1;
          if (next !== -1) { event.preventDefault(); buttons[next]?.focus(); }
        }}>
        <div className="composer-picker-heading">{label}</div>
        {options.map((option) => <button key={option.value} type="button" role="menuitemradio" aria-checked={value === option.value}
          className="composer-picker-option" tabIndex={value === option.value ? 0 : -1}
          onClick={() => { onSelect(option.value); close(); }}>
          <span className="picker-option-icon"><option.icon size={16} strokeWidth={1.5} aria-hidden="true" /></span>
          <span className="picker-option-copy"><span className="picker-option-label">{option.label}</span><span className="picker-option-description">{option.description}</span></span>
          {value === option.value && <Check size={15} strokeWidth={2} aria-hidden="true" />}
        </button>)}
      </div>
    </div>
  );
}
