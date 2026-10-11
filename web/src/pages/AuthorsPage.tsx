import { keepPreviousData, useInfiniteQuery } from "@tanstack/react-query";
import { ArrowUpDown, CalendarDays, Hash, LoaderCircle, Search, SearchX, Type, Users, X } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { AuthorAvatar, AuthorLink } from "../components/Author";
import { Select } from "../components/Select";
import { EmptyState, PageHeader } from "../components/ui";
import { getAuthors } from "../lib/api";
import { authorPath } from "../lib/authors";
import { formatDate } from "../lib/format";

const PAGE_SIZE = 100;

const SORTS = ["count", "recent", "name"] as const;
type Sort = (typeof SORTS)[number];

/* Everyone the library has posts from, as a database of people: who, how
   many posts, and when the last one was saved. */
export function AuthorsPage() {
  const navigate = useNavigate();
  // The find text and sort live in the address, so Back from an author
  // returns to the same list.
  const [search, setSearch] = useSearchParams();
  const text = (search.get("q") || "").trim();
  const sort: Sort = SORTS.find((value) => value === search.get("sort")) || "count";
  const [draft, setDraft] = useState(text);

  function update(next: { q?: string; sort?: Sort }) {
    const value = new URLSearchParams();
    const q = next.q ?? text;
    const order = next.sort ?? sort;
    if (q) value.set("q", q);
    if (order !== "count") value.set("sort", order);
    setSearch(value, { replace: true });
  }

  // The list filters as you type, a moment after the last keystroke.
  useEffect(() => {
    if (draft.trim() === text) return;
    const timer = window.setTimeout(() => update({ q: draft.trim() }), 200);
    return () => window.clearTimeout(timer);
    // `update` is rebuilt every render; only the typed text should restart the wait.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [draft, text]);

  const params = useMemo(() => {
    const value = new URLSearchParams({ sort, pageSize: String(PAGE_SIZE) });
    if (text) value.set("q", text);
    return value.toString();
  }, [sort, text]);

  const authors = useInfiniteQuery({
    queryKey: ["authors", params],
    queryFn: ({ pageParam }) => getAuthors(`${params}&page=${pageParam}`),
    initialPageParam: 1,
    getNextPageParam: (last) => (last.page * last.pageSize < last.total ? last.page + 1 : undefined),
    placeholderData: keepPreviousData,
  });

  const items = authors.data?.pages.flatMap((page) => page.items) ?? [];
  const total = authors.data?.pages[0]?.total ?? 0;

  return (
    <main className="page full">
      <PageHeader
        icon={Users}
        tone="pink"
        title="Authors"
        description={
          authors.data
            ? total === 0
              ? "Everyone you've saved posts from."
              : `${total.toLocaleString()} ${text ? "matching " : ""}${total === 1 ? "person" : "people"} you've saved posts from. Click one to see everything of theirs.`
            : "Everyone you've saved posts from."
        }
      />

      <div className="view-bar authors-bar">
        <form className="db-search" role="search" onSubmit={(event) => event.preventDefault()}>
          <Search size={15} aria-hidden="true" />
          <input value={draft} onChange={(event) => setDraft(event.target.value)} placeholder="Find a name or @handle…" aria-label="Find an author" />
          {draft && (
            <button type="button" className="icon-button" aria-label="Clear" onClick={() => {
                setDraft("");
                update({ q: "" });
              }}
            >
              <X size={14} />
            </button>
          )}
        </form>
        <div className="view-bar-tools">
          <Select
            variant="tool"
            icon={ArrowUpDown}
            label="Sort authors"
            align="end"
            value={sort}
            onChange={(value) => update({ sort: value as Sort })}
            options={[
              { value: "count", label: "Most saved" },
              { value: "recent", label: "Recently saved" },
              { value: "name", label: "Name" },
            ]}
          />
        </div>
      </div>

      {authors.isLoading && (
        <div className="skeleton-table" aria-busy="true" aria-label="Loading authors">
          {Array.from({ length: 8 }).map((_, index) => (
            <div key={index} className="skeleton skeleton-row" />
          ))}
        </div>
      )}

      {authors.isError && (
        <EmptyState icon={SearchX} title="Authors could not be loaded">
          The local server did not answer. Check that Sortwise is still running.
        </EmptyState>
      )}

      {authors.data && items.length === 0 && (
        <EmptyState
          icon={text ? SearchX : Users}
          title={text ? `Nobody matches “${text}”` : "No authors yet"}
          action={
            text && (
              <button
                className="button secondary"
                onClick={() => {
                  setDraft("");
                  update({ q: "" });
                }}
              >
                Clear search
              </button>
            )
          }
        >
          {text ? "Try part of their name or handle." : "Authors appear here as soon as you save posts from X."}
        </EmptyState>
      )}

      {items.length > 0 && (
        <div className="table-scroll">
          <table className="db-table authors-table">
            <thead>
              <tr>
                <th className="col-author"><span><Type size={14} />Author</span></th>
                <th className="col-count"><span><Hash size={14} />Posts</span></th>
                <th className="col-date"><span><CalendarDays size={14} />Last saved</span></th>
              </tr>
            </thead>
            <tbody>
              {items.map((author) => (
                <tr key={author.username.toLowerCase()} onClick={() => navigate(authorPath(author.username))}>
                  <td className="col-author">
                    <div className="person-cell">
                      <AuthorAvatar name={author.name} username={author.username} />
                      <AuthorLink username={author.username} className="person-name">
                        {author.name || `@${author.username}`}
                      </AuthorLink>
                      <span className="handle">@{author.username}</span>
                      <span className="open-chip" aria-hidden="true">Open</span>
                    </div>
                  </td>
                  <td className="col-count">{author.count.toLocaleString()}</td>
                  <td className="col-date">
                    <time>{formatDate(author.latestSavedAt)}</time>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {total > 0 && (
        <div className="db-footer">
          <span className="db-count">
            Count <strong>{total.toLocaleString()}</strong>
            {items.length < total && <span className="muted"> · showing {items.length.toLocaleString()}</span>}
          </span>
          {authors.hasNextPage && (
            <button type="button" className="ghost-button" disabled={authors.isFetchingNextPage} onClick={() => void authors.fetchNextPage()}>
              {authors.isFetchingNextPage && <LoaderCircle size={13} className="spin" aria-hidden="true" />}
              Show more
            </button>
          )}
        </div>
      )}
    </main>
  );
}
