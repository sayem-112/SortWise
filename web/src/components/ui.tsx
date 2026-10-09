import { Check, ChevronDown, ChevronRight, Search, X, type LucideIcon } from "lucide-react";
import {
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import type { OptionColor } from "../lib/format";

/* ---------- Page scaffolding ---------- */

export function PageHeader({
  icon: Icon,
  tone = "gray",
  title,
  description,
  children,
  iconSlot,
}: {
  icon: LucideIcon;
  tone?: OptionColor;
  title: ReactNode;
  description?: ReactNode;
  children?: ReactNode;
  // Replaces the page icon, e.g. with a button that changes it.
  iconSlot?: ReactNode;
}) {
  return (
    <header className="page-header">
      {iconSlot || (
        <div className={`page-icon tone-${tone}`} aria-hidden="true">
          <Icon size={64} strokeWidth={1.5} />
        </div>
      )}
      <h1 className="page-title">{title}</h1>
      {description && <p className="page-description">{description}</p>}
      {children && <div className="page-header-actions">{children}</div>}
    </header>
  );
}

export function Callout({
  icon: Icon,
  tone = "default",
  children,
  role,
}: {
  icon: LucideIcon;
  tone?: OptionColor;
  children: ReactNode;
  role?: string;
}) {
  return (
    <div className={`callout callout-${tone}`} role={role}>
      <Icon size={18} className="callout-icon" aria-hidden="true" />
      <div className="callout-body">{children}</div>
    </div>
  );
}

export function Option({
  color = "default",
  children,
  onRemove,
  removeLabel,
}: {
  color?: OptionColor;
  children: ReactNode;
  onRemove?: () => void;
  removeLabel?: string;
}) {
  return (
    <span className={`option opt-${color}`}>
      <span className="option-label">{children}</span>
      {onRemove && (
        <button type="button" className="option-remove" aria-label={removeLabel} onClick={onRemove}>
          <X size={11} />
        </button>
      )}
    </span>
  );
}

export function PropertyRow({
  icon: Icon,
  name,
  children,
}: {
  icon: LucideIcon;
  name: string;
  children: ReactNode;
}) {
  return (
    <div className="property-row">
      <div className="property-name">
        <Icon size={15} aria-hidden="true" />
        <span>{name}</span>
      </div>
      <div className="property-value">{children}</div>
    </div>
  );
}

export function EmptyState({
  icon: Icon,
  title,
  children,
  action,
}: {
  icon: LucideIcon;
  title: string;
  children?: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div className="empty-state">
      <Icon size={28} strokeWidth={1.5} aria-hidden="true" />
      <h3>{title}</h3>
      {children && <p>{children}</p>}
      {action}
    </div>
  );
}

export function ViewTabs<T extends string>({
  tabs,
  value,
  onChange,
  label,
}: {
  tabs: { value: T; label: string; icon: LucideIcon; count?: number }[];
  value: T;
  onChange: (value: T) => void;
  label: string;
}) {
  return (
    <div className="view-tabs" role="tablist" aria-label={label}>
      {tabs.map(({ value: tab, label: text, icon: Icon, count }) => (
        <button
          key={tab}
          role="tab"
          type="button"
          aria-selected={tab === value}
          className={tab === value ? "view-tab active" : "view-tab"}
          onClick={() => onChange(tab)}
        >
          <Icon size={15} aria-hidden="true" />
          <span>{text}</span>
          {count !== undefined && <span className="view-tab-count">{count}</span>}
        </button>
      ))}
    </div>
  );
}

/* ---------- Popover (Notion menus) ---------- */

export function Popover({
  trigger,
  children,
  align = "start",
  label,
  active = false,
  className = "",
  triggerClassName = "pill-button",
  title,
}: {
  trigger: ReactNode;
  children: (close: () => void) => ReactNode;
  align?: "start" | "end";
  label: string;
  active?: boolean;
  className?: string;
  triggerClassName?: string;
  title?: string;
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const button = useRef<HTMLButtonElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const id = useId();

  // Closing from inside the menu or with Escape puts focus back on the trigger.
  const close = useCallback(() => {
    setOpen(false);
    button.current?.focus({ preventScroll: true });
  }, []);

  // The menu lives on <body> so scrolling containers such as the table never
  // clip it. It sits under its trigger, or above when there is more room
  // there, and always inside the window.
  useLayoutEffect(() => {
    if (!open) return;
    function place() {
      const anchor = button.current?.getBoundingClientRect();
      const element = menu.current;
      if (!anchor || !element) return;
      const gap = 6;
      const edge = 8;
      element.style.maxHeight = "";
      const width = element.offsetWidth;
      const height = element.offsetHeight;
      const below = window.innerHeight - anchor.bottom - gap - edge;
      const above = anchor.top - gap - edge;
      const left = align === "end" ? anchor.right - width : anchor.left;
      element.style.left = `${Math.max(edge, Math.min(left, window.innerWidth - width - edge))}px`;
      if (height <= below || below >= above) {
        element.style.top = `${anchor.bottom + gap}px`;
        element.style.maxHeight = `${Math.max(below, 120)}px`;
        element.classList.remove("open-up");
      } else {
        element.style.top = `${Math.max(edge, anchor.top - gap - Math.min(height, above))}px`;
        element.style.maxHeight = `${above}px`;
        element.classList.add("open-up");
      }
    }
    place();
    window.addEventListener("resize", place);
    window.addEventListener("scroll", place, true);
    return () => {
      window.removeEventListener("resize", place);
      window.removeEventListener("scroll", place, true);
    };
  }, [open, align]);

  useEffect(() => {
    if (!open) return;
    function onPointer(event: PointerEvent) {
      const target = event.target as Node;
      if (!ref.current?.contains(target) && !menu.current?.contains(target)) setOpen(false);
    }
    function onKey(event: KeyboardEvent) {
      if (event.key === "Escape") {
        event.stopPropagation();
        close();
      }
    }
    document.addEventListener("pointerdown", onPointer);
    document.addEventListener("keydown", onKey, true);
    return () => {
      document.removeEventListener("pointerdown", onPointer);
      document.removeEventListener("keydown", onKey, true);
    };
  }, [open, close]);

  return (
    <div className={`popover-anchor ${className}`} ref={ref}>
      <button
        ref={button}
        type="button"
        className={`${triggerClassName} ${active ? "active" : ""} ${open ? "open" : ""}`}
        aria-expanded={open}
        aria-controls={id}
        aria-label={label}
        title={title}
        onClick={(event) => {
          event.stopPropagation();
          setOpen((value) => !value);
        }}
      >
        {trigger}
      </button>
      {open &&
        createPortal(
          <div
            ref={menu}
            className={`menu popover floating ${className ? `${className.split(" ")[0]}-menu` : ""}`}
            id={id}
            role="dialog"
            aria-label={label}
            onClick={(event) => event.stopPropagation()}
          >
            {children(close)}
          </div>,
          document.body,
        )}
    </div>
  );
}

/* A searchable checklist, the body of Notion's select and multi-select menus. */
export function OptionList({
  options,
  selected,
  onToggle,
  placeholder = "Search for an option…",
  emptyText = "No options",
  single = false,
}: {
  options: { id: number | string; name: string; color?: OptionColor; meta?: ReactNode; icon?: LucideIcon; iconClass?: string }[];
  selected: (number | string)[];
  onToggle: (id: number | string) => void;
  placeholder?: string;
  emptyText?: string;
  single?: boolean;
}) {
  const [filter, setFilter] = useState("");
  const visible = options.filter((option) => option.name.toLowerCase().includes(filter.trim().toLowerCase()));
  return (
    <div className="option-list">
      {options.length > 7 && (
        <label className="menu-search">
          <Search size={14} aria-hidden="true" />
          <input
            autoFocus
            value={filter}
            onChange={(event) => setFilter(event.target.value)}
            placeholder={placeholder}
            aria-label={placeholder}
          />
        </label>
      )}
      <div className="option-list-items" role={single ? "radiogroup" : "group"}>
        {visible.map((option) => {
          const checked = selected.includes(option.id);
          return (
            <button
              type="button"
              key={option.id}
              role={single ? "radio" : "checkbox"}
              aria-checked={checked}
              className={`menu-item ${checked ? "checked" : ""}`}
              onClick={() => onToggle(option.id)}
            >
              {option.icon && <option.icon size={14} className={option.iconClass} aria-hidden="true" />}
              {option.color ? <Option color={option.color}>{option.name}</Option> : <span>{option.name}</span>}
              {option.meta !== undefined && <span className="menu-item-meta">{option.meta}</span>}
              <Check size={14} className="menu-check" aria-hidden="true" />
            </button>
          );
        })}
        {visible.length === 0 && <p className="menu-empty">{emptyText}</p>}
      </div>
    </div>
  );
}

export function SelectTrigger({ children }: { children: ReactNode }) {
  return (
    <>
      {children}
      <ChevronDown size={13} aria-hidden="true" className="pill-chevron" />
    </>
  );
}

/* ---------- Side peek (Notion's right-hand page preview) ---------- */

export function SidePeek({
  open,
  onClose,
  label,
  toolbar,
  children,
}: {
  open: boolean;
  onClose: () => void;
  label: string;
  toolbar?: ReactNode;
  children: ReactNode;
}) {
  const panel = useRef<HTMLDivElement>(null);
  const closeRef = useRef(onClose);
  closeRef.current = onClose;

  useEffect(() => {
    if (!open) return;
    const previous = document.activeElement as HTMLElement | null;
    panel.current?.focus();
    function onKey(event: KeyboardEvent) {
      if (event.key === "Escape") closeRef.current();
    }
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("keydown", onKey);
      previous?.focus?.();
    };
  }, [open]);

  if (!open) return null;
  return (
    <div className="peek-backdrop" onClick={onClose}>
      <div
        className="peek"
        role="dialog"
        aria-label={label}
        tabIndex={-1}
        ref={panel}
        onClick={(event) => event.stopPropagation()}
      >
        <div className="peek-toolbar">
          <button type="button" className="icon-button" onClick={onClose} aria-label="Close preview" title="Close (Esc)">
            <X size={17} />
          </button>
          <div className="peek-toolbar-actions">{toolbar}</div>
        </div>
        <div className="peek-body">{children}</div>
      </div>
    </div>
  );
}

/* ---------- Center peek (Notion's centered page preview) ---------- */

export function CenterPeek({
  open,
  onClose,
  label,
  toolbar,
  children,
}: {
  open: boolean;
  onClose: () => void;
  label: string;
  toolbar?: ReactNode;
  children: ReactNode;
}) {
  const panel = useRef<HTMLDivElement>(null);
  const closeRef = useRef(onClose);
  closeRef.current = onClose;

  useEffect(() => {
    if (!open) return;
    const previous = document.activeElement as HTMLElement | null;
    panel.current?.focus({ preventScroll: true });
    const overflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    function onKey(event: KeyboardEvent) {
      if (event.key === "Escape" && !event.defaultPrevented) closeRef.current();
      // Keep keyboard focus inside the dialog.
      if (event.key === "Tab" && panel.current) {
        const focusable = panel.current.querySelectorAll<HTMLElement>(
          'a[href], button:not([disabled]), input, textarea, select, [tabindex]:not([tabindex="-1"])'
        );
        if (!focusable.length) return;
        const first = focusable[0];
        const last = focusable[focusable.length - 1];
        if (event.shiftKey && (document.activeElement === first || document.activeElement === panel.current)) {
          event.preventDefault();
          last.focus();
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault();
          first.focus();
        }
      }
    }
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("keydown", onKey);
      document.body.style.overflow = overflow;
      previous?.focus?.({ preventScroll: true });
    };
  }, [open]);

  if (!open) return null;
  return (
    <div className="center-peek-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <div className="center-peek" role="dialog" aria-modal="true" aria-label={label} tabIndex={-1} ref={panel}>
        <div className="center-peek-bar">
          <button type="button" className="icon-button" onClick={onClose} aria-label="Close" title="Close (Esc)">
            <X size={17} />
          </button>
          {toolbar}
        </div>
        <div className="center-peek-body">{children}</div>
      </div>
    </div>
  );
}

export function Switch({
  checked,
  onChange,
  label,
  disabled,
}: {
  checked: boolean;
  onChange: (value: boolean) => void;
  label: string;
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      className={`switch ${checked ? "on" : ""}`}
      onClick={() => onChange(!checked)}
    >
      <span className="switch-thumb" />
    </button>
  );
}

/* Notion's toggle block: a disclosure triangle that reveals nested content. */
export function Toggle({ summary, children }: { summary: ReactNode; children: ReactNode }) {
  return (
    <details className="toggle-block">
      <summary>
        <ChevronRight size={16} className="toggle-caret" aria-hidden="true" />
        <span>{summary}</span>
      </summary>
      <div className="toggle-content">{children}</div>
    </details>
  );
}

/* Read-only Notion checkbox property. */
export function CheckboxValue({ checked, label }: { checked: boolean; label: string }) {
  return (
    <span
      className={`checkbox-value ${checked ? "checked" : ""}`}
      role="img"
      aria-label={`${label}: ${checked ? "yes" : "no"}`}
    >
      {checked && <Check size={12} strokeWidth={3} aria-hidden="true" />}
    </span>
  );
}
