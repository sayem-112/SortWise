import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  ArrowUpRight,
  CalendarDays,
  CircleDot,
  FolderTree,
  Tags,
  Trash2,
  Type,
} from "lucide-react";
import { Link } from "react-router-dom";
import { deleteBookmark, type Bookmark } from "../lib/api";
import { StatusPill } from "./StatusPill";
import { Option } from "./ui";
import { colorFor, formatDate, initials, postPreview } from "../lib/format";

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
      if (window.confirm("Permanently delete this bookmark and its analysis?")) remove.mutate();
    },
  };
}

function dateLabel(bookmark: Bookmark) {
  return `${bookmark.postedAt ? "Posted" : "Imported"} ${formatDate(bookmark.postedAt || bookmark.importedAt)}`;
}

function RowActions({ bookmark }: { bookmark: Bookmark }) {
  const remove = useDeleteBookmark(bookmark);
  return (
    <span className="row-actions">
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
}

/* Gallery view card: a cover (media, or the post's opening lines like Notion's page preview) plus properties. */
export function BookmarkCard({ bookmark, onOpenDrawer }: BookmarkCardProps) {
  const preview = bookmark.media[0];
  const text = postPreview(bookmark);

  function open(event: React.MouseEvent) {
    if (!onOpenDrawer) return;
    event.preventDefault();
    onOpenDrawer(bookmark.id);
  }

  return (
    <article className="gallery-card">
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
      <RowActions bookmark={bookmark} />
    </article>
  );
}

/* Table view: Notion's database table with a title column and typed property columns. */
export function BookmarkTable({
  bookmarks,
  onOpenDrawer,
}: {
  bookmarks: Bookmark[];
  onOpenDrawer: (id: number) => void;
}) {
  return (
    <div className="table-scroll">
      <table className="db-table library-table">
        <thead>
          <tr>
            <th className="col-title"><span><Type size={14} />Post</span></th>
            <th className="col-status"><span><CircleDot size={14} />Status</span></th>
            <th className="col-cats"><span><FolderTree size={14} />Categories</span></th>
            <th className="col-tags"><span><Tags size={14} />Tags</span></th>
            <th className="col-date"><span><CalendarDays size={14} />Posted</span></th>
            <th className="col-actions"><span className="visually-hidden">Actions</span></th>
          </tr>
        </thead>
        <tbody>
          {bookmarks.map((bookmark) => (
            <tr key={bookmark.id} onClick={() => onOpenDrawer(bookmark.id)}>
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
              <td className="col-date">
                <time title={bookmark.postedAt ? "Posted on X" : "Imported into Sortwise; post date unknown"}>
                  {formatDate(bookmark.postedAt || bookmark.importedAt)}
                </time>
                {!bookmark.postedAt && <span className="date-note"> imported</span>}
              </td>
              <td className="col-actions">
                <RowActions bookmark={bookmark} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
