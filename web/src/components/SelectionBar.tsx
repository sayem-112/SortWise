import { useQuery } from "@tanstack/react-query";
import { ListMinus, ListPlus, Star, X } from "lucide-react";
import { useEffect } from "react";
import type { Bookmark, List } from "../lib/api";
import { listsQuery, useListChange } from "../lib/lists";
import { showToast } from "../lib/toast";
import { ListPicker } from "./Lists";
import { Popover } from "./ui";

/* Appears while rows are selected: add them all to a list, star or unstar
   them, or take them out of the list being viewed. Each action only touches
   the posts it actually changes, so its message counts real changes. */
export function SelectionBar({
  selected,
  list,
  onClear,
  onRemoved,
}: {
  selected: Bookmark[];
  list?: List;
  onClear: () => void;
  // Called with the posts taken out of the list being viewed.
  onRemoved: (bookmarks: Bookmark[]) => void;
}) {
  const lists = useQuery(listsQuery);
  const change = useListChange();
  const favorites = lists.data?.items.find((item) => item.kind === "favorites");
  const custom = (lists.data?.items || []).filter((item) => item.kind === "custom");
  // Lists every selected bookmark is in are checked; lists only some are in are marked.
  const all = custom.filter((item) => selected.every((bookmark) => bookmark.lists.includes(item.id))).map((item) => item.id);
  const some = custom.filter((item) => selected.some((bookmark) => bookmark.lists.includes(item.id))).map((item) => item.id);
  const allFavorites = Boolean(favorites && selected.every((bookmark) => bookmark.lists.includes(favorites.id)));

  // The toast moves up while this bar is on screen, so the two never overlap.
  useEffect(() => {
    document.body.dataset.selecting = "true";
    return () => {
      delete document.body.dataset.selecting;
    };
  }, []);

  function apply(target: Pick<List, "id" | "name">, inList: boolean) {
    const ids = selected.filter((bookmark) => bookmark.lists.includes(target.id) !== inList).map((bookmark) => bookmark.id);
    if (ids.length) change.apply({ list: target, ids, inList, bulk: true });
    else showToast({ message: `Already ${inList ? "in" : "out of"} ${target.name}` });
    return ids;
  }

  return (
    <div className="selection-bar" role="toolbar" aria-label="Selected bookmarks">
      <span className="selection-count">{selected.length} selected</span>
      <Popover
        label="Add to list"
        title="Add to list (L)"
        triggerClassName="ghost-button"
        className="selection-menu"
        trigger={
          <>
            <ListPlus size={15} aria-hidden="true" />
            Add to list
          </>
        }
      >
        {() => (
          <ListPicker
            selected={all}
            partial={some}
            onToggle={(target, inList) => apply(target, inList)}
            onCreated={(target) => change.apply({ list: target, ids: selected.map((bookmark) => bookmark.id), inList: true, bulk: true })}
          />
        )}
      </Popover>
      {favorites && list?.kind !== "favorites" && (
        <button type="button" className="ghost-button" title={`${allFavorites ? "Unfavorite" : "Favorite"} (F)`} onClick={() => apply(favorites, !allFavorites)}>
          <Star size={15} fill={allFavorites ? "currentColor" : "none"} aria-hidden="true" />
          {allFavorites ? "Unfavorite" : "Favorite"}
        </button>
      )}
      {list && (
        <button
          type="button"
          className="ghost-button"
          onClick={() => {
            const ids = apply(list, false);
            onRemoved(selected.filter((bookmark) => ids.includes(bookmark.id)));
            onClear();
          }}
        >
          <ListMinus size={15} aria-hidden="true" />
          Remove from {list.name}
        </button>
      )}
      <button type="button" className="icon-button selection-clear" onClick={onClear} aria-label="Clear selection" title="Clear selection (Esc)">
        <X size={15} />
      </button>
    </div>
  );
}
