import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, ListPlus, Plus, Star } from "lucide-react";
import { useId, useState, type FormEvent, type KeyboardEvent } from "react";
import { createList, updateList, type Bookmark, type List } from "../lib/api";
import { LIST_ICONS, listIcon, listsQuery, listTone, useListChange } from "../lib/lists";
import { Popover } from "./ui";

/* The star: puts a bookmark in Favorites or takes it out. */
export function FavoriteButton({ bookmark }: { bookmark: Bookmark }) {
  const lists = useQuery(listsQuery);
  const change = useListChange();
  const favorites = lists.data?.items.find((item) => item.kind === "favorites");
  const on = Boolean(favorites && bookmark.lists.includes(favorites.id));
  const label = on ? "Remove from Favorites" : "Add to Favorites";
  return (
    <button
      type="button"
      className={`icon-button favorite-button ${on ? "on" : ""}`}
      disabled={!favorites}
      aria-pressed={on}
      aria-label={label}
      title={`${label} (F)`}
      onClick={(event) => {
        event.preventDefault();
        event.stopPropagation();
        if (favorites) change.apply({ list: favorites, ids: [bookmark.id], inList: !on });
      }}
    >
      <Star size={14} fill={on ? "currentColor" : "none"} />
    </button>
  );
}

/* One field that both finds a list and makes a new one, like Notion's select
   menus: type to filter, Enter adds to the highlighted list (or creates the
   name typed), Shift+Enter takes it out. Favorites is the star, not a list
   here. `selected` lists are checked; `partial` hold only some of a selection. */
export function ListPicker({
  selected,
  partial = [],
  onToggle,
  onCreated,
}: {
  selected: number[];
  partial?: number[];
  onToggle: (list: List, inList: boolean) => void;
  onCreated: (list: List) => void;
}) {
  const client = useQueryClient();
  const lists = useQuery(listsQuery);
  const [text, setText] = useState("");
  const [active, setActive] = useState(0);
  const id = useId();
  const create = useMutation({
    mutationFn: (name: string) => createList(name),
    onSuccess: (list) => {
      client.invalidateQueries({ queryKey: ["lists"] });
      setText("");
      onCreated(list);
    },
  });

  const items = (lists.data?.items || []).filter((item) => item.kind === "custom");
  const needle = text.trim().toLowerCase();
  const visible = items.filter((item) => item.name.toLowerCase().includes(needle));
  const exact = items.some((item) => item.name.toLowerCase() === needle);
  const canCreate = needle !== "" && !exact;
  const count = visible.length + (canCreate ? 1 : 0);
  const current = Math.min(active, Math.max(count - 1, 0));

  // A click toggles. From the keyboard Enter only adds, so pressing it twice
  // never takes back what was just filed; Shift+Enter removes.
  function choose(index: number, mode: "toggle" | "add" | "remove" = "toggle") {
    if (index < visible.length) {
      const list = visible[index];
      const inList = mode === "toggle" ? !selected.includes(list.id) : mode === "add";
      if (inList !== selected.includes(list.id) || partial.includes(list.id)) onToggle(list, inList);
      if (mode !== "toggle") setText("");
    } else if (canCreate && !create.isPending) {
      create.mutate(text.trim());
    }
  }

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const step = event.key === "ArrowDown" ? 1 : -1;
      setActive((current + step + count) % Math.max(count, 1));
    } else if (event.key === "Enter") {
      event.preventDefault();
      if (count) choose(current, event.shiftKey ? "remove" : "add");
    }
  }

  return (
    <div className="list-picker">
      <label className="menu-search">
        <ListPlus size={14} aria-hidden="true" />
        <input
          autoFocus
          value={text}
          onChange={(event) => {
            setText(event.target.value);
            setActive(0);
          }}
          onKeyDown={onKeyDown}
          placeholder="Find or create a list…"
          aria-label="Find or create a list"
          aria-controls={`${id}-items`}
          aria-activedescendant={count ? `${id}-${current}` : undefined}
          maxLength={80}
        />
      </label>
      <div className="option-list-items" role="group" aria-label="Lists" id={`${id}-items`}>
        {visible.map((list, index) => {
          const Icon = listIcon(list);
          const checked = selected.includes(list.id);
          const mixed = !checked && partial.includes(list.id);
          return (
            <button
              type="button"
              key={list.id}
              id={`${id}-${index}`}
              role="checkbox"
              aria-checked={checked ? true : mixed ? "mixed" : false}
              className={`menu-item ${checked ? "checked" : ""} ${index === current ? "active" : ""}`}
              onMouseEnter={() => setActive(index)}
              onClick={() => choose(index)}
            >
              <Icon size={14} className={`tone-${listTone(list)}`} aria-hidden="true" />
              <span className="list-picker-name">{list.name}</span>
              {mixed && <span className="menu-item-meta">Some</span>}
              <Check size={14} className="menu-check" aria-hidden="true" />
            </button>
          );
        })}
        {canCreate && (
          <button
            type="button"
            id={`${id}-${visible.length}`}
            className={`menu-item list-picker-create ${current === visible.length ? "active" : ""}`}
            onMouseEnter={() => setActive(visible.length)}
            onClick={() => choose(visible.length)}
            disabled={create.isPending}
          >
            <Plus size={14} aria-hidden="true" />
            <span>
              Create <strong>{text.trim()}</strong>
            </span>
          </button>
        )}
        {!visible.length && !canCreate && <p className="menu-empty">{lists.isLoading ? "Loading…" : "Type a name to make your first list"}</p>}
      </div>
      {create.error && (
        <p className="form-error" role="alert">
          {create.error.message}
        </p>
      )}
      {visible.length > 0 && <p className="list-picker-hint">Enter adds · Shift+Enter removes</p>}
    </div>
  );
}

// How many list chips a table cell shows before "+N".
const CELL_CHIPS = 2;

/* The lists a bookmark is in, with the picker behind them. Shows as chips in
   a table cell or on the bookmark page, or as an icon button on gallery cards.
   Favorites shows as the star instead, and on a list's own page that list is
   left out of the cell, since every row is in it. */
export function ListsMenu({ bookmark, variant, viewing }: { bookmark: Bookmark; variant: "cell" | "rail" | "icon"; viewing?: List }) {
  const lists = useQuery(listsQuery);
  const change = useListChange();
  const current = (lists.data?.items || []).filter((item) => item.kind === "custom" && bookmark.lists.includes(item.id));
  const shown = variant === "cell" ? current.filter((item) => item.id !== viewing?.id) : current;
  const names = current.map((item) => item.name).join(", ");
  const label = current.length ? `Lists: ${names}. Change lists` : "Add to a list";
  const visibleChips = variant === "cell" ? shown.slice(0, CELL_CHIPS) : shown;
  const more = shown.length - visibleChips.length;

  const chips = shown.length ? (
    <>
      {visibleChips.map((item) => {
        const Icon = listIcon(item);
        return (
          <span key={item.id} className="list-chip" title={item.name}>
            <Icon size={12} className={`tone-${listTone(item)}`} aria-hidden="true" />
            <span className="list-chip-name">{item.name}</span>
          </span>
        );
      })}
      {more > 0 && <span className="more-count">+{more}</span>}
    </>
  ) : variant === "cell" ? (
    <span className="cell-add">
      <Plus size={13} aria-hidden="true" />
      Add
    </span>
  ) : (
    <span className="placeholder">Add to a list</span>
  );

  return (
    <Popover
      label={label}
      title={variant === "icon" ? "Add to a list" : "Lists (L)"}
      align="end"
      className={variant === "rail" ? "rail-editor lists-trigger" : variant === "cell" ? "cell-editor" : "icon-menu"}
      triggerClassName={variant === "icon" ? `icon-button ${current.length ? "has-lists" : ""}` : "pill-button"}
      trigger={variant === "icon" ? <ListPlus size={14} aria-hidden="true" /> : chips}
    >
      {() => (
        <ListPicker
          selected={bookmark.lists}
          onToggle={(list, inList) => change.apply({ list, ids: [bookmark.id], inList })}
          onCreated={(list) => change.apply({ list, ids: [bookmark.id], inList: true })}
        />
      )}
    </Popover>
  );
}

/* A short form for naming a new list, used in the sidebar. */
export function NewListForm({ onCreated, onCancel }: { onCreated: (list: List) => void; onCancel: () => void }) {
  const client = useQueryClient();
  const [name, setName] = useState("");
  const create = useMutation({
    mutationFn: () => createList(name),
    onSuccess: (list) => {
      client.invalidateQueries({ queryKey: ["lists"] });
      setName("");
      onCreated(list);
    },
  });
  function submit(event: FormEvent) {
    event.preventDefault();
    if (name.trim() && !create.isPending) create.mutate();
  }
  return (
    <form className="new-list-form" onSubmit={submit}>
      <label className="new-list-input">
        <Plus size={14} aria-hidden="true" />
        <input
          value={name}
          onChange={(event) => setName(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Escape") {
              event.preventDefault();
              onCancel();
            }
          }}
          onBlur={() => !name.trim() && onCancel()}
          placeholder="List name"
          aria-label="New list name"
          maxLength={80}
          autoFocus
        />
      </label>
      {create.error && (
        <p className="form-error" role="alert">
          {create.error.message}
        </p>
      )}
    </form>
  );
}

/* The list's page icon, which opens a picker for its color and icon, like
   choosing a Notion page icon. Saves on each click. */
export function ListLook({ list }: { list: List }) {
  const client = useQueryClient();
  const lists = useQuery(listsQuery);
  const save = useMutation({
    mutationFn: (input: { icon?: string; color?: string }) => updateList(list.id, input),
    onSuccess: () => client.invalidateQueries({ queryKey: ["lists"] }),
  });
  const appearance = lists.data?.appearance;
  const Icon = listIcon(list);
  const tone = listTone(list);
  return (
    <Popover
      label={`Change the icon and color of ${list.name}`}
      title="Change icon and color"
      className="list-look"
      triggerClassName={`page-icon page-icon-button tone-${tone}`}
      trigger={<Icon size={64} strokeWidth={1.5} aria-hidden="true" />}
    >
      {() => (
        <div className="look-picker">
          <p className="menu-caption">Color</p>
          <div className="look-colors" role="radiogroup" aria-label="Color">
            {(appearance?.colors || []).map((color) => (
              <button
                key={color}
                type="button"
                role="radio"
                aria-checked={color === tone}
                aria-label={color}
                title={color[0].toUpperCase() + color.slice(1)}
                className={`look-swatch swatch-${color} ${color === tone ? "on" : ""}`}
                onClick={() => save.mutate({ color })}
              />
            ))}
          </div>
          <p className="menu-caption">Icon</p>
          <div className="look-icons" role="radiogroup" aria-label="Icon">
            {(appearance?.icons || []).map((name) => {
              const Choice = LIST_ICONS[name];
              if (!Choice) return null;
              return (
                <button
                  key={name}
                  type="button"
                  role="radio"
                  aria-checked={name === list.icon}
                  aria-label={name.replace("-", " ")}
                  title={name.replace("-", " ")}
                  className={`icon-button look-icon ${name === list.icon ? "on" : ""}`}
                  style={{ color: `var(--${tone}-fg)` }}
                  onClick={() => save.mutate({ icon: name })}
                >
                  <Choice size={18} />
                </button>
              );
            })}
          </div>
          {save.error && (
            <p className="form-error" role="alert">
              {save.error.message}
            </p>
          )}
        </div>
      )}
    </Popover>
  );
}
