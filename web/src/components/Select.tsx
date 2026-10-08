import { Check, ChevronDown, Search, type LucideIcon } from "lucide-react";
import { createPortal } from "react-dom";
import { useEffect, useId, useRef, useState, type CSSProperties, type KeyboardEvent } from "react";
import type { OptionColor } from "../lib/format";
import { Option } from "./ui";

export type SelectOption<T extends string> = { value: T; label: string; hint?: string; color?: OptionColor };

/* A Notion-style dropdown replacing the native <select>, whose OS menu ignores the
   app's styling. Keyboard: arrows, Home/End, Enter or Space to choose, Escape to
   close. Long lists get a search box. Opens upward near the bottom of the screen. */
export function Select<T extends string>({
  value,
  options,
  onChange,
  label,
  icon: Icon,
  variant = "field",
  placeholder = "Choose…",
  align = "start",
  disabled,
}: {
  value: T;
  options: SelectOption<T>[];
  onChange: (value: T) => void;
  label: string;
  icon?: LucideIcon;
  variant?: "field" | "tool";
  placeholder?: string;
  align?: "start" | "end";
  disabled?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const [filter, setFilter] = useState("");
  // The menu is positioned against the viewport so scrolling panels (the side peek)
  // cannot clip it; it closes on scroll instead of drifting from its button.
  const [position, setPosition] = useState<CSSProperties>({});
  const anchor = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const list = useRef<HTMLDivElement>(null);
  const search = useRef<HTMLInputElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const inside = (target: EventTarget | null) =>
    Boolean(anchor.current?.contains(target as Node) || menu.current?.contains(target as Node));
  const id = useId();
  const selected = options.find((option) => option.value === value);
  const searchable = options.length > 10;
  const visible = searchable
    ? options.filter((option) => option.label.toLowerCase().includes(filter.trim().toLowerCase()))
    : options;

  useEffect(() => {
    if (!open) return;
    function onPointer(event: PointerEvent) {
      if (!inside(event.target)) setOpen(false);
    }
    function onScroll(event: Event) {
      if (!inside(event.target)) setOpen(false);
    }
    const onResize = () => setOpen(false);
    document.addEventListener("pointerdown", onPointer);
    window.addEventListener("scroll", onScroll, true);
    window.addEventListener("resize", onResize);
    return () => {
      document.removeEventListener("pointerdown", onPointer);
      window.removeEventListener("scroll", onScroll, true);
      window.removeEventListener("resize", onResize);
    };
  }, [open]);

  // Scroll only the option list; scrollIntoView could also scroll the panel behind.
  useEffect(() => {
    const container = list.current;
    const item = container?.querySelector<HTMLElement>(`[data-index="${active}"]`);
    if (!open || !container || !item) return;
    if (item.offsetTop < container.scrollTop) container.scrollTop = item.offsetTop;
    else if (item.offsetTop + item.offsetHeight > container.scrollTop + container.clientHeight)
      container.scrollTop = item.offsetTop + item.offsetHeight - container.clientHeight;
  }, [open, active]);

  useEffect(() => {
    if (open && searchable) search.current?.focus({ preventScroll: true });
  }, [open, searchable]);

  function show() {
    const rect = anchor.current?.getBoundingClientRect();
    if (rect) {
      const below = window.innerHeight - rect.bottom;
      const above = below < 300 && rect.top > below;
      const room = Math.max(160, (above ? rect.top : below) - 16);
      setPosition({
        position: "fixed",
        minWidth: rect.width,
        maxHeight: Math.min(360, room),
        ...(above ? { top: "auto", bottom: window.innerHeight - rect.top + 6 } : { top: rect.bottom + 6, bottom: "auto" }),
        ...(align === "end" ? { right: window.innerWidth - rect.right } : { left: rect.left }),
      });
    }
    setFilter("");
    setActive(Math.max(0, options.findIndex((option) => option.value === value)));
    setOpen(true);
  }

  function close(refocus = true) {
    setOpen(false);
    if (refocus) trigger.current?.focus({ preventScroll: true });
  }

  function choose(index: number) {
    const option = visible[index];
    if (!option) return;
    onChange(option.value);
    close();
  }

  function onKeyDown(event: KeyboardEvent) {
    if (!open) {
      if (["ArrowDown", "ArrowUp", "Enter", " "].includes(event.key)) {
        event.preventDefault();
        show();
      }
      return;
    }
    switch (event.key) {
      case "ArrowDown":
        event.preventDefault();
        setActive((index) => Math.min(visible.length - 1, index + 1));
        break;
      case "ArrowUp":
        event.preventDefault();
        setActive((index) => Math.max(0, index - 1));
        break;
      case "Home":
        event.preventDefault();
        setActive(0);
        break;
      case "End":
        event.preventDefault();
        setActive(visible.length - 1);
        break;
      case "Enter":
        event.preventDefault();
        choose(active);
        break;
      case " ":
        if (!searchable) {
          event.preventDefault();
          choose(active);
        }
        break;
      case "Escape":
        // Keep the side peek or palette underneath open.
        event.preventDefault();
        event.stopPropagation();
        close();
        break;
      case "Tab":
        close(false);
        break;
    }
  }

  return (
    <div className="select-anchor" ref={anchor} onKeyDown={onKeyDown}>
      <button
        ref={trigger}
        type="button"
        role="combobox"
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={open ? id : undefined}
        aria-label={label}
        aria-activedescendant={open && !searchable && visible[active] ? `${id}-${active}` : undefined}
        disabled={disabled}
        className={`select-trigger ${variant} ${open ? "open" : ""}`}
        onClick={() => (open ? close() : show())}
      >
        {Icon && <Icon size={15} aria-hidden="true" />}
        <span className="select-value">
          {selected ? (
            selected.color ? <Option color={selected.color}>{selected.label}</Option> : selected.label
          ) : (
            <span className="placeholder">{placeholder}</span>
          )}
        </span>
        <ChevronDown size={13} className="pill-chevron" aria-hidden="true" />
      </button>
      {open &&
        // Portaled to <body>: an animated or scrolling ancestor (the side peek) would
        // otherwise become the reference box for position: fixed and misplace the menu.
        createPortal(
        <div className="menu select-menu" style={position} ref={menu}>
          {searchable && (
            <label className="menu-search">
              <Search size={14} aria-hidden="true" />
              <input
                ref={search}
                value={filter}
                aria-label={`Search ${label.toLowerCase()}`}
                aria-controls={id}
                aria-activedescendant={visible[active] ? `${id}-${active}` : undefined}
                placeholder="Search…"
                onChange={(event) => {
                  setFilter(event.target.value);
                  setActive(0);
                }}
              />
            </label>
          )}
          <div className="option-list-items" role="listbox" id={id} aria-label={label} ref={list}>
            {visible.map((option, index) => (
              <div
                key={option.value}
                id={`${id}-${index}`}
                role="option"
                aria-selected={option.value === value}
                data-index={index}
                className={`menu-item ${index === active ? "active" : ""} ${option.value === value ? "checked" : ""}`}
                onMouseMove={() => index !== active && setActive(index)}
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => choose(index)}
              >
                {option.color ? <Option color={option.color}>{option.label}</Option> : <span>{option.label}</span>}
                {option.hint && <span className="menu-item-meta">{option.hint}</span>}
                <Check size={14} className="menu-check" aria-hidden="true" />
              </div>
            ))}
            {visible.length === 0 && <p className="menu-empty">No matches</p>}
          </div>
        </div>,
        document.body,
      )}
    </div>
  );
}
