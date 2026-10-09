import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  ArrowUpRight,
  CalendarDays,
  CircleDot,
  FolderTree,
  List as ListIcon,
  ListMinus,
  Undo2,
  Tags,
  Trash2,
  Type,
} from "lucide-react";
import { Link } from "react-router-dom";
import { deleteBookmark, type Bookmark, type List } from "../lib/api";
import { useListChange } from "../lib/lists";
import { FavoriteButton, ListsMenu } from "./Lists";
import { StatusPill } from "./StatusPill";
import { Option } from "./ui";
import { colorFor, formatDate, initials, postPreview } from "../lib/format";
import { confirmAction } from "../lib/confirm";

function useDeleteBookmark(bookmark: Bookmark) {
  const queryClient = useQueryClient();
  const remove = useMutation({
    mutationFn: () => deleteBookmark(bookmark.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["bookmarks"] });
      queryClient.invalidateQueries({ queryKey: ["dashboard"] });
      queryClient.invalidateQueries({ queryKey: ["categories"] });
      queryClient.invalidateQueries({ queryKey: ["tags"] });
      queryClient.invalidateQueries({ queryKey: ["setup"] });
    },
  });
  return {
    pending: remove.isPending,
    confirm: () => {
      void confirmAction({ title: "Delete this bookmark?", message: "The post and its AI analysis are removed from your library. A sync won't bring it back unless you bookmark it on X again.", confirmLabel: "Delete", danger: true }).then((ok) => ok && remove.mutate());
    },
  };
}

function dateLabel(bookmark: Bookmark) {
  return `${bookmark.postedAt ? "Posted" : "Imported"} ${formatDate(bookmark.postedAt || bookmark.importedAt)}`;
}

/* What a list page does with rows taken out of it: they stay in place,
   faded, with their own Undo, until the view changes. */
export type ListRemoval = {
  ghosts: Set<number>;
  onRemoved: (bookmark: Bookmark) => void;
};

/* Row actions. Inside a list the remove button takes the post out of that
   list (with undo) instead of deleting it from the library. A row already
   taken out offers Undo in their place. */
function RowActions({ bookmark, list, menu = false, removal }: { bookmark: Bookmark; list?: List; menu?: boolean; removal?: ListRemoval }) {
  const remove = useDeleteBookmark(bookmark);
  const change = useListChange();
  if (list && removal?.ghosts.has(bookmark.id)) {
    return (
      <span className="row-actions ghost-actions">
        <button
          type="button"
          className="ghost-button"
          onClick={(event) => {
            event.stopPropagation();
            change.apply({ list, ids: [bookmark.id], inList: true, quiet: true });
          }}
        >
          <Undo2 size={14} aria-hidden="true" />
          Undo
        </button>
      </span>
    );
  }
  return (
    <span className="row-actions">
      <FavoriteButton bookmark={bookmark} />
      {menu && <ListsMenu bookmark={bookmark} variant="icon" />}
      {list ? (
        <button
          type="button"
          className="icon-button"
          aria-label={`Remove post by ${bookmark.author} from ${list.name}`}
          title={`Remove from ${list.name}`}
          onClick={(event) => {
            event.stopPropagation();
            change.apply({ list, ids: [bookmark.id], inList: false });
            removal?.onRemoved(bookmark);
          }}
        >
          <ListMinus size={14} />
        </button>
      ) : (
        <button
          type="button"
          className="icon-button danger"
          disabled={remove.pending}
          aria-label={`Delete bookmark by ${bookmark.author}`}
          title="Delete bookmark"
          onClick={(event) => {
            event.stopPropagation();
            remove.confirm();
          }}
        >
          <Trash2 size={14} />
        </button>
      )}
      <a
        href={bookmark.url}
        target="_blank"
        rel="noreferrer"
        aria-label={`View post by ${bookmark.author} on X`}
        title="View on X"
        className="icon-button"
        onClick={(event) => event.stopPropagation()}
      >
        <ArrowUpRight size={14} />
      </a>
    </span>
  );
}

interface BookmarkCardProps {
  bookmark: Bookmark;
  onOpenDrawer?: (id: number) => void;
  list?: List;
  removal?: ListRemoval;
}

/* Gallery view card: a cover (media, or the post's opening lines like Notion's page preview) plus properties. */
export function BookmarkCard({ bookmark, onOpenDrawer, list, removal }: BookmarkCardProps) {
  const ghost = Boolean(list && removal?.ghosts.has(bookmark.id));
  const preview = bookmark.media[0];
  const text = postPreview(bookmark);

  function open(event: React.MouseEvent) {
    if (!onOpenDrawer) return;
    event.preventDefault();
    onOpenDrawer(bookmark.id);
  }

  return (
    <article className={`gallery-card ${ghost ? "ghost" : ""}`}>
      <Link to={`/bookmarks/${bookmark.id}`} className="gallery-link" onClick={open}>
        <div className={`gallery-cover ${preview ? "has-media" : ""}`}>
          {preview ? (
            <>
              <img src={preview.previewUrl || preview.url} alt={preview.altText || "Post media"} loading="lazy" />
              {preview.kind === "video_poster" && <span className="cover-badge">Video</span>}
            </>
          ) : (
            <p>{text}</p>
          )}
        </div>
        <div className="gallery-body">
          <div className="gallery-title">
            <span className="avatar" aria-hidden="true">
              {initials(bookmark.author)}
            </span>
            <strong>{bookmark.author}</strong>
            <span className="handle">@{bookmark.username}</span>
          </div>
          {preview && <p className="gallery-text">{text}</p>}
          {bookmark.summary && <p className="gallery-summary">{bookmark.summary}</p>}
          <div className="gallery-properties">
            <StatusPill status={bookmark.processingStatus} />
            {bookmark.categories.map((item) => (
              <Option key={`c-${item.id}`} color={colorFor(item.id)}>
                {item.name}
              </Option>
            ))}
            {bookmark.tags.slice(0, 3).map((item) => (
              <Option key={`t-${item.id}`}>{item.name}</Option>
            ))}
            {bookmark.tags.length > 3 && <span className="more-count">+{bookmark.tags.length - 3}</span>}
          </div>
          <time className="gallery-date" title={bookmark.postedAt ? "Posted on X" : "Imported into Sortwise"}>
            {dateLabel(bookmark)}
          </time>
        </div>
      </Link>
      <RowActions bookmark={bookmark} list={list} menu removal={removal} />
    </article>
  );
}

/* Table view: Notion's database table with a title column and typed property columns. */
export function BookmarkTable({
  bookmarks,
  onOpenDrawer,
  list,
  selected,
  onSelect,
  onSelectAll,
  cursor,
  removal,
}: {
  bookmarks: Bookmark[];
  onOpenDrawer: (id: number) => void;
  list?: List;
  // Selection: shift-click selects the range from the last row clicked.
  selected: Set<number>;
  onSelect: (id: number, range: boolean) => void;
  onSelectAll: (all: boolean) => void;
  // The row the keyboard is on (J/K), if any.
  cursor: number | null;
  removal?: ListRemoval;
}) {
  const allSelected = bookmarks.length > 0 && bookmarks.every((item) => selected.has(item.id));
  const someSelected = !allSelected && bookmarks.some((item) => selected.has(item.id));
  return (
    <div className="table-scroll">
      <table className={`db-table library-table ${selected.size ? "selecting" : ""}`}>
        <thead>
          <tr>
            <th className="col-select">
              <input
                type="checkbox"
                className="row-check"
                checked={allSelected}
                ref={(input) => {
                  if (input) input.indeterminate = someSelected;
                }}
                onChange={() => onSelectAll(!allSelected)}
                aria-label={allSelected ? "Clear selection" : "Select all bookmarks on this page"}
              />
            </th>
            <th className="col-title"><span><Type size={14} />Post</span></th>
            <th className="col-status"><span><CircleDot size={14} />Status</span></th>
            <th className="col-cats"><span><FolderTree size={14} />Categories</span></th>
            <th className="col-tags"><span><Tags size={14} />Tags</span></th>
            <th className="col-lists"><span><ListIcon size={14} />Lists</span></th>
            <th className="col-date"><span><CalendarDays size={14} />Posted</span></th>
            <th className="col-actions"><span className="visually-hidden">Actions</span></th>
          </tr>
        </thead>
        <tbody>
          {bookmarks.map((bookmark, index) => {
            const ghost = Boolean(list && removal?.ghosts.has(bookmark.id));
            const classes = [selected.has(bookmark.id) && "selected", index === cursor && "cursor", ghost && "ghost"].filter(Boolean).join(" ");
            return (
            <tr key={bookmark.id} data-row={index} className={classes || undefined} onClick={() => !ghost && onOpenDrawer(bookmark.id)}>
              <td className="col-select" onClick={(event) => event.stopPropagation()}>
                <input
                  type="checkbox"
                  className="row-check"
                  checked={selected.has(bookmark.id)}
                  onChange={() => undefined}
                  onClick={(event) => onSelect(bookmark.id, event.shiftKey)}
                  aria-label={`Select post by ${bookmark.author}`}
                />
              </td>
              <td className="col-title">
                <div className="title-cell">
                  <span className="avatar" aria-hidden="true">{initials(bookmark.author)}</span>
                  <span className="title-author">{bookmark.author}</span>
                  <Link
                    to={`/bookmarks/${bookmark.id}`}
                    onClick={(event) => {
                      event.preventDefault();
                      event.stopPropagation();
                      onOpenDrawer(bookmark.id);
                    }}
                  >
                    {postPreview(bookmark)}
                  </Link>
                  <span className="open-chip" aria-hidden="true">Open</span>
                </div>
              </td>
              <td className="col-status">
                <StatusPill status={bookmark.processingStatus} />
              </td>
              <td className="col-cats">
                <span className="cell-options">
                  {bookmark.categories.map((item) => (
                    <Option key={item.id} color={colorFor(item.id)}>
                      {item.name}
                    </Option>
                  ))}
                </span>
              </td>
              <td className="col-tags">
                <span className="cell-options">
                  {bookmark.tags.slice(0, 3).map((item) => (
                    <Option key={item.id}>{item.name}</Option>
                  ))}
                  {bookmark.tags.length > 3 && <span className="more-count">+{bookmark.tags.length - 3}</span>}
                </span>
              </td>
              <td className="col-lists" onClick={(event) => event.stopPropagation()}>
                <ListsMenu bookmark={bookmark} variant="cell" viewing={list} />
              </td>
              <td className="col-date">
                <time title={bookmark.postedAt ? "Posted on X" : "Imported into Sortwise; post date unknown"}>
                  {formatDate(bookmark.postedAt || bookmark.importedAt)}
                </time>
                {!bookmark.postedAt && <span className="date-note"> imported</span>}
              </td>
              <td className="col-actions">
                <RowActions bookmark={bookmark} list={list} removal={removal} />
              </td>
            </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
