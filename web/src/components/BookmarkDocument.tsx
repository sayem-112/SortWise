import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Archive,
  ArrowUpRight,
  CalendarDays,
  CircleDot,
  Download,
  FileText,
  FolderTree,
  Link2,
  ListPlus,
  MoreHorizontal,
  Pencil,
  RefreshCw,
  Sparkles,
  Tags,
  Trash2,
  Undo2,
  type LucideIcon,
} from "lucide-react";
import { Fragment, useEffect, useRef, useState, type ReactNode } from "react";
import { Link } from "react-router-dom";
import {
  deleteBookmark,
  getBookmark,
  getCategories,
  getRelated,
  getTags,
  reprocessBookmark,
  setArchived,
  updateBookmarkMetadata,
  type Bookmark,
} from "../lib/api";
import { colorFor, colorForText, formatDate, formatDateTime, initials, postPreview } from "../lib/format";
import { isFrontmost, usePeek } from "../lib/peek";
import { forgetOpened, rememberOpened } from "../lib/recent";
import { refreshLibrary } from "../lib/refresh";
import { AuthorAvatar, AuthorLink } from "./Author";
import { listsQuery, useListChange } from "../lib/lists";
import { FavoriteButton, ListsMenu } from "./Lists";
import { StatusPill } from "./StatusPill";
import { Callout, Option, OptionList, Popover } from "./ui";
import { confirmAction } from "../lib/confirm";

interface BookmarkDocumentProps {
  bookmarkId: string;
  variant: "peek" | "page";
  onDeleted: () => void;
  // Add this post to the finder's "recently opened".
  remember?: boolean;
}

const LINK = /(https?:\/\/[^\s]+|@\w{1,15})/g;

/* Post text with web links and @mentions clickable, long URLs shortened. */
function RichText({ text }: { text: string }) {
  return (
    <>
      {text.split(LINK).map((part, index) => {
        if (index % 2 === 0) return <Fragment key={index}>{part}</Fragment>;
        const mention = part.startsWith("@");
        const href = mention ? `https://x.com/${part.slice(1)}` : part;
        const label = mention ? part : part.replace(/^https?:\/\/(www\.)?/, "").replace(/\/$/, "");
        return (
          <a key={index} href={href} target="_blank" rel="noreferrer" className="post-link">
            {label.length > 42 ? `${label.slice(0, 41)}…` : label}
          </a>
        );
      })}
    </>
  );
}

/* Colored by handle, so an author looks the same here as in every list. */
function Avatar({ name, username, small = false }: { name: string; username: string; small?: boolean }) {
  return (
    <span className={`post-avatar opt-${colorForText(username || name)} ${small ? "small" : ""}`} aria-hidden="true">
      {initials(name)}
    </span>
  );
}

function host(url: string) {
  return url.replace(/^https?:\/\/(www\.)?/, "").split("/")[0];
}

/* The saved post, written on the page itself: byline, text, then its media,
   link card, and quoted post as Notion-style embedded blocks. */
function Post({ bookmark, actions }: { bookmark: Bookmark; actions: ReactNode }) {
  const { quotedPost, card, article } = bookmark.visibleContext;
  const text = article ? "" : bookmark.text;
  return (
    <section className="post" aria-label={`Post by ${bookmark.author || bookmark.username}`}>
      <header className="post-byline">
        <AuthorAvatar name={bookmark.author || bookmark.username} username={bookmark.username} src={bookmark.authorAvatar} size="post" />
        <div className="post-who">
          {/* The name opens everything saved from them; the handle goes to X. */}
          <AuthorLink username={bookmark.username} className="post-author">
            {bookmark.author || bookmark.username}
          </AuthorLink>
          <span className="post-meta">
            <a className="post-handle" href={`https://x.com/${bookmark.username}`} target="_blank" rel="noreferrer" title="Open their profile on X">
              @{bookmark.username}
            </a>
            {bookmark.postedAt && (
              <>
                <span aria-hidden="true"> · </span>
                <time dateTime={bookmark.postedAt} title={formatDateTime(bookmark.postedAt)}>
                  {formatDate(bookmark.postedAt)}
                </time>
              </>
            )}
          </span>
        </div>
        <div className="post-actions">{actions}</div>
      </header>

      {text && (
        <p className="post-text">
          <RichText text={text} />
        </p>
      )}

      {bookmark.media.length > 0 && (
        <div className={`post-media count-${Math.min(bookmark.media.length, 4)}`}>
          {bookmark.media.map((media) => (
            <figure key={media.url}>
              {media.videoUrl ? (
                <video src={media.videoUrl} poster={media.previewUrl || media.url} controls preload="none" aria-label={media.altText || "Post video"} />
              ) : (
                <>
                  <img src={media.previewUrl || media.url} alt={media.altText || "Post image"} loading="lazy" />
                  {media.kind === "video_poster" && <figcaption>Video</figcaption>}
                </>
              )}
            </figure>
          ))}
        </div>
      )}

      {card && (
        <a className={`embed-block link-block ${article ? "article" : ""}`} href={card.url || bookmark.url} target="_blank" rel="noreferrer">
          <strong>{card.title}</strong>
          {card.description && <span className="embed-text">{card.description}</span>}
          {(article || (card.url && host(card.url) !== "t.co")) && (
            <span className="embed-source">
              {article ? <FileText size={13} aria-hidden="true" /> : <Link2 size={13} aria-hidden="true" />}
              {article ? "Article on X" : host(card.url)}
            </span>
          )}
        </a>
      )}

      {quotedPost && (
        <a className="embed-block quote-block" href={quotedPost.url} target="_blank" rel="noreferrer">
          <span className="quote-byline">
            <Avatar name={quotedPost.username || "?"} username={quotedPost.username || ""} small />
            <strong>@{quotedPost.username || "unknown"}</strong>
            <span>quoted post</span>
          </span>
          {quotedPost.text ? <span className="embed-text quote-text">{quotedPost.text}</span> : <span className="embed-text muted">Open it on X to read the quoted post.</span>}
        </a>
      )}
    </section>
  );
}

/* Posts about the same thing as this one, closest first. Worked out on this
   computer from what is already saved. In a preview they open in place,
   with this post and the others a keypress away; on a page they are links. */
function RelatedPosts({ bookmarkId, variant }: { bookmarkId: number; variant: "peek" | "page" }) {
  const peek = usePeek();
  const related = useQuery({ queryKey: ["related", bookmarkId], queryFn: () => getRelated(bookmarkId), staleTime: 60_000 });
  const items = related.data?.items || [];
  if (!items.length) return null;
  return (
    <section className="related-block" aria-labelledby={`related-${bookmarkId}`}>
      <h2 id={`related-${bookmarkId}`}>Related posts</h2>
      <ul className="related-list">
        {items.map((item) => (
          <li key={item.id}>
            <Link
              to={`/bookmarks/${item.id}`}
              className="related-item"
              onClick={(event) => {
                if (variant !== "peek" || event.metaKey || event.ctrlKey || event.shiftKey) return;
                event.preventDefault();
                peek.follow(item.id, [bookmarkId, ...items.map((other) => other.id)]);
              }}
            >
              <AuthorAvatar name={item.author || item.username} username={item.username} src={item.avatar} />
              <span className="related-text">
                <span className="related-head">
                  <strong>{item.author || `@${item.username}`}</strong>
                  {item.postedAt && <time dateTime={item.postedAt}>{formatDate(item.postedAt)}</time>}
                </span>
                <span className="related-preview">{item.text}</span>
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}

function Property({ icon: Icon, name, children }: { icon: LucideIcon; name: string; children: ReactNode }) {
  return (
    <div className="rail-row">
      <span className="rail-name">
        <Icon size={14} aria-hidden="true" />
        {name}
      </span>
      <div className="rail-value">{children}</div>
    </div>
  );
}

export function BookmarkDocument({ bookmarkId, variant, onDeleted, remember = false }: BookmarkDocumentProps) {
  const queryClient = useQueryClient();
  const [editingSummary, setEditingSummary] = useState(false);
  const [summary, setSummary] = useState("");

  const result = useQuery({ queryKey: ["bookmark", bookmarkId], queryFn: () => getBookmark(bookmarkId), enabled: Boolean(bookmarkId) });
  const categories = useQuery({ queryKey: ["categories"], queryFn: getCategories });
  const lists = useQuery(listsQuery);
  const listChange = useListChange();
  const rail = useRef<HTMLElement>(null);
  const loadedBookmark = result.data;

  // The finder offers the posts opened last.
  useEffect(() => {
    if (remember && loadedBookmark) rememberOpened({ id: loadedBookmark.id, author: loadedBookmark.author, username: loadedBookmark.username, preview: postPreview(loadedBookmark) });
  }, [remember, loadedBookmark]);

  // F stars the open post; L opens its Lists menu. Ignored while typing or in a menu.
  useEffect(() => {
    if (!loadedBookmark) return;
    const bookmark = loadedBookmark;
    function onKey(event: KeyboardEvent) {
      if (event.metaKey || event.ctrlKey || event.altKey) return;
      if (event.target instanceof Element && event.target.closest("input, textarea, [contenteditable='true'], .menu")) return;
      // A post under a preview, the finder, or a confirmation leaves the keys to what is on top.
      if (!isFrontmost(rail.current) || document.querySelector(".command-backdrop")) return;
      const key = event.key.toLowerCase();
      if (key === "f") {
        const favorites = lists.data?.items.find((item) => item.kind === "favorites");
        if (!favorites) return;
        event.preventDefault();
        listChange.apply({ list: favorites, ids: [bookmark.id], inList: !bookmark.lists.includes(favorites.id) });
      } else if (key === "l") {
        event.preventDefault();
        rail.current?.querySelector<HTMLButtonElement>(".lists-trigger > button")?.click();
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [loadedBookmark, lists.data, listChange]);
  const tags = useQuery({ queryKey: ["tags"], queryFn: getTags });

  const invalidateLists = () => refreshLibrary(queryClient);
  const setBookmark = (value: Bookmark) => {
    queryClient.setQueryData(["bookmark", bookmarkId], value);
    invalidateLists();
  };

  const archive = useMutation({ mutationFn: (value: boolean) => setArchived(Number(bookmarkId), value), onSuccess: setBookmark });
  const remove = useMutation({
    mutationFn: () => deleteBookmark(Number(bookmarkId)),
    onSuccess: () => {
      forgetOpened(Number(bookmarkId));
      invalidateLists();
      onDeleted();
    },
  });
  const reprocess = useMutation({
    mutationFn: () => reprocessBookmark(Number(bookmarkId)),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["bookmark", bookmarkId] });
      queryClient.invalidateQueries({ queryKey: ["jobs"] });
    },
  });
  // Each change saves right away. Hand-made choices are kept when the AI runs again.
  const decide = useMutation({
    mutationFn: (input: Parameters<typeof updateBookmarkMetadata>[1]) => updateBookmarkMetadata(Number(bookmarkId), input),
    onSuccess: (value) => {
      setBookmark(value);
      queryClient.invalidateQueries({ queryKey: ["categories"] });
      queryClient.invalidateQueries({ queryKey: ["tags"] });
      setEditingSummary(false);
    },
  });

  if (result.isLoading) {
    return (
      <div className={`bookmark-doc doc-${variant}`} aria-busy="true">
        <div className="bookmark-grid">
          <div className="bookmark-main">
            <div className="skeleton skeleton-title" />
            <div className="skeleton skeleton-line" />
            <div className="skeleton skeleton-line short" />
          </div>
        </div>
      </div>
    );
  }
  if (!result.data) {
    return (
      <div className={`bookmark-doc doc-${variant}`}>
        <div className="bookmark-main">
          <Callout icon={Sparkles} tone="red" role="alert">
            This bookmark could not be found. It may have been deleted.
          </Callout>
        </div>
      </div>
    );
  }

  const bookmark = result.data;
  const pending = bookmark.processingStatus !== "completed";
  const categoryIds = bookmark.categories.map((item) => item.id);
  const tagIds = bookmark.tags.map((item) => item.id);
  const error = (decide.error || archive.error || remove.error || reprocess.error)?.message;

  function toggle(kind: "categoryDecisions" | "tagDecisions", selected: number[], id: number) {
    decide.mutate({ [kind]: [{ id, state: selected.includes(id) ? "removed" : "added" }] });
  }

  const actions = (
    <>
      <FavoriteButton bookmark={bookmark} />
      <a className="ghost-button open-on-x" href={bookmark.url} target="_blank" rel="noreferrer" aria-label="Open on X" title="Open on X">
        <span className="open-on-x-label">Open on X</span> <ArrowUpRight size={14} aria-hidden="true" />
      </a>
      <Popover label="More actions" align="end" className="more-menu" trigger={<MoreHorizontal size={16} aria-hidden="true" />}>
        {(close) => (
          <div className="menu-actions" role="menu">
            <button
              type="button"
              role="menuitem"
              className="menu-item"
              disabled={reprocess.isPending}
              onClick={() => {
                reprocess.mutate();
                close();
              }}
            >
              <RefreshCw size={14} aria-hidden="true" />
              Re-analyze with AI
            </button>
            <button
              type="button"
              role="menuitem"
              className="menu-item"
              onClick={() => {
                archive.mutate(!bookmark.archived);
                close();
              }}
            >
              {bookmark.archived ? <Undo2 size={14} aria-hidden="true" /> : <Archive size={14} aria-hidden="true" />}
              {bookmark.archived ? "Restore from archive" : "Archive"}
            </button>
            <button
              type="button"
              role="menuitem"
              className="menu-item danger"
              onClick={() => {
                close();
                void confirmAction({ title: "Delete this bookmark?", message: "The post and its AI analysis are removed from your library. A sync won't bring it back unless you bookmark it on X again.", confirmLabel: "Delete", danger: true }).then((ok) => ok && remove.mutate());
              }}
            >
              <Trash2 size={14} aria-hidden="true" />
              Delete
            </button>
          </div>
        )}
      </Popover>
    </>
  );

  return (
    <article className={`bookmark-doc doc-${variant}`}>
      <div className="bookmark-grid">
        <div className="bookmark-main">
          <Post bookmark={bookmark} actions={actions} />

          <section className="summary-block" aria-labelledby={`summary-${bookmark.id}`}>
            <Sparkles size={18} className="summary-icon" aria-hidden="true" />
            <div className="summary-body">
              <div className="summary-head">
                <h2 id={`summary-${bookmark.id}`}>AI summary</h2>
                {!editingSummary && !pending && (
                  <button
                    type="button"
                    className="ghost-button"
                    onClick={() => {
                      setSummary(bookmark.summary);
                      setEditingSummary(true);
                    }}
                  >
                    <Pencil size={13} aria-hidden="true" />
                    Edit
                  </button>
                )}
              </div>
              {editingSummary ? (
                <form
                  className="summary-editor"
                  onSubmit={(event) => {
                    event.preventDefault();
                    decide.mutate({ summary });
                  }}
                >
                  <textarea
                    value={summary}
                    onChange={(event) => setSummary(event.target.value)}
                    onKeyDown={(event) => {
                      if (event.key === "Escape") {
                        event.preventDefault();
                        event.stopPropagation();
                        setEditingSummary(false);
                      }
                    }}
                    maxLength={800}
                    rows={4}
                    autoFocus
                    aria-label="Summary"
                  />
                  <div className="summary-actions">
                    <span className="summary-count">{summary.length}/800 · kept when the AI runs again</span>
                    <button type="button" className="button secondary small" onClick={() => setEditingSummary(false)}>
                      Cancel
                    </button>
                    <button type="submit" className="button primary small" disabled={decide.isPending}>
                      Save
                    </button>
                  </div>
                </form>
              ) : pending ? (
                <p className="summary-empty">
                  {bookmark.processingStatus === "failed"
                    ? "The AI could not analyze this post. Choose Re-analyze with AI from the ⋯ menu to try again."
                    : "Waiting to be analyzed. The post is already saved and searchable."}
                </p>
              ) : bookmark.summary ? (
                <p className="summary-text">{bookmark.summary}</p>
              ) : (
                <p className="summary-empty">No summary yet: nothing readable was captured when this post was analyzed. Choose Re-analyze with AI from the ⋯ menu.</p>
              )}
              {bookmark.mediaDescription && !editingSummary && (
                <p className="summary-note">
                  <span>In the images</span> {bookmark.mediaDescription}
                </p>
              )}
            </div>
          </section>

          <RelatedPosts bookmarkId={bookmark.id} variant={variant} />
        </div>

        <aside className="bookmark-rail" aria-label="Properties" ref={rail}>
          <Property icon={CircleDot} name="Status">
            <StatusPill status={bookmark.processingStatus} />
            {reprocess.isSuccess && <span className="rail-note">Queued again</span>}
            {bookmark.archived && <Option color="gray">Archived</Option>}
            {bookmark.removedOnXAt && <Option color="orange">Unbookmarked on X</Option>}
          </Property>

          <Property icon={ListPlus} name="Lists">
            <ListsMenu bookmark={bookmark} variant="rail" />
          </Property>

          <Property icon={FolderTree} name="Categories">
            <Popover
              label="Edit categories"
              align="end"
              className="rail-editor"
              trigger={
                bookmark.categories.length ? (
                  bookmark.categories.map((item) => (
                    <Option key={item.id} color={colorFor(item.id)}>
                      {item.name}
                    </Option>
                  ))
                ) : (
                  <span className="placeholder">{pending ? "After analysis" : "Empty"}</span>
                )
              }
            >
              {() => (
                <OptionList
                  options={(categories.data?.items || [])
                    .filter((item) => item.active || categoryIds.includes(item.id))
                    .map((item) => ({ id: item.id, name: item.name, color: colorFor(item.id) }))}
                  selected={categoryIds}
                  onToggle={(id) => toggle("categoryDecisions", categoryIds, Number(id))}
                  placeholder="Search categories…"
                  emptyText={categories.isLoading ? "Loading…" : "No matching categories"}
                />
              )}
            </Popover>
          </Property>

          <Property icon={Tags} name="Tags">
            <Popover
              label="Edit tags"
              align="end"
              className="rail-editor"
              trigger={
                bookmark.tags.length ? (
                  bookmark.tags.map((item) => <Option key={item.id}>{item.name}</Option>)
                ) : (
                  <span className="placeholder">{pending ? "After analysis" : "Empty"}</span>
                )
              }
            >
              {() => (
                <OptionList
                  options={(tags.data?.items || []).map((item) => ({ id: item.id, name: item.name, meta: item.count }))}
                  selected={tagIds}
                  onToggle={(id) => toggle("tagDecisions", tagIds, Number(id))}
                  placeholder="Search tags…"
                  emptyText={tags.isLoading ? "Loading…" : "No matching tags"}
                />
              )}
            </Popover>
          </Property>


          {bookmark.postedAt && (
            <Property icon={CalendarDays} name="Posted">
              <span className="rail-date">{formatDate(bookmark.postedAt)}</span>
            </Property>
          )}
          <Property icon={Download} name="Saved">
            <span className="rail-date">{formatDate(bookmark.importedAt)}</span>
          </Property>

          {error && (
            <p className="form-error" role="alert">
              {error}
            </p>
          )}
        </aside>
      </div>
    </article>
  );
}
