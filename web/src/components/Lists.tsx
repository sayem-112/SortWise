import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Star } from "lucide-react";
import { useState, type FormEvent } from "react";
import { createList, type Bookmark, type List } from "../lib/api";
import { listIcon, listsQuery, useListMembership } from "../lib/lists";
import { OptionList, Popover } from "./ui";

/* The star: puts a bookmark in Favorites or takes it out. */
export function FavoriteButton({ bookmark }: { bookmark: Bookmark }) {
  const lists = useQuery(listsQuery);
  const membership = useListMembership(bookmark);
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
      title={label}
      onClick={(event) => {
        event.preventDefault();
        event.stopPropagation();
        if (favorites) membership.toggle(favorites.id);
      }}
    >
      <Star size={14} fill={on ? "currentColor" : "none"} />
    </button>
  );
}

/* A short form for naming a new list; calls back with the created list. */
export function NewListForm({ onCreated, onCancel, autoFocus = true }: { onCreated: (list: List) => void; onCancel?: () => void; autoFocus?: boolean }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const create = useMutation({
    mutationFn: () => createList(name),
    onSuccess: (list) => {
      queryClient.invalidateQueries({ queryKey: ["lists"] });
      setName("");
      onCreated(list);
    },
  });
  function submit(event: FormEvent) {
    event.preventDefault();
    event.stopPropagation();
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
            if (event.key === "Escape" && onCancel) {
              event.preventDefault();
              event.stopPropagation();
              onCancel();
            }
          }}
          onBlur={() => !name.trim() && onCancel?.()}
          placeholder="New list…"
          aria-label="New list name"
          maxLength={80}
          autoFocus={autoFocus}
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

/* The lists a bookmark is in, as a property on the bookmark page, with a
   menu to add it to others or make a new list for it. */
export function ListsProperty({ bookmark }: { bookmark: Bookmark }) {
  const lists = useQuery(listsQuery);
  const membership = useListMembership(bookmark);
  const items = lists.data?.items || [];
  const current = items.filter((item) => bookmark.lists.includes(item.id));
  return (
    <Popover
      label="Edit lists"
      align="end"
      className="rail-editor"
      trigger={
        current.length ? (
          current.map((item) => {
            const Icon = listIcon(item);
            return (
              <span key={item.id} className="list-chip">
                <Icon size={13} aria-hidden="true" />
                {item.name}
              </span>
            );
          })
        ) : (
          <span className="placeholder">Empty</span>
        )
      }
    >
      {() => (
        <>
          <OptionList
            options={items.map((item) => ({ id: item.id, name: item.name }))}
            selected={bookmark.lists}
            onToggle={(id) => membership.toggle(Number(id))}
            placeholder="Search lists…"
            emptyText={lists.isLoading ? "Loading…" : "No matching lists"}
          />
          <div className="menu-divider" />
          <NewListForm autoFocus={false} onCreated={(list) => membership.set(list.id, true)} />
        </>
      )}
    </Popover>
  );
}
