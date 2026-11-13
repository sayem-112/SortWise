import { keepPreviousData, useInfiniteQuery } from "@tanstack/react-query";
import { ArrowUpDown, LoaderCircle, Search, SearchX, Users, X } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { AuthorAvatar } from "../components/Author";
import { Select } from "../components/Select";
import { EmptyState, PageHeader } from "../components/ui";
import { getAuthors, type Author } from "../lib/api";
import { authorPath } from "../lib/authors";
import { colorFor, colorForText, formatDate } from "../lib/format";

const PAGE_SIZE = 100;

const SORTS = ["count", "recent", "name"] as const;
type Sort = (typeof SORTS)[number];

/* Everyone the library has posts from, as a database of people: who, how
   many posts, and when the last one was saved. */
export function AuthorsPage() {
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
  // Ranked by how much is saved, with nothing typed, the top three get the big cards.
  const ranked = sort === "count" && !text;
  const featured = ranked ? items.slice(0, 3) : [];
  const others = ranked ? items.slice(3) : items;

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
              : `${total.toLocaleString()} ${text ? "matching " : ""}${total === 1 ? "person" : "people"} you've saved posts from.`
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

      {featured.length > 0 && (
        <ol className="authors-featured" aria-label="Most saved">
          {featured.map((author, index) => (
            <li key={author.username.toLowerCase()}>
              <FeaturedAuthor author={author} rank={index + 1} />
            </li>
          ))}
        </ol>
      )}

      {others.length > 0 && (
        <ul className="authors-grid" aria-label={featured.length ? "Everyone else" : "Authors"}>
          {others.map((author) => (
            <li key={author.username.toLowerCase()}>
              <AuthorTile author={author} />
            </li>
          ))}
        </ul>
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

type Topic = NonNullable<Author["topics"]>[number];

/* What someone is saved for: a bar split by their top categories, and the
   names under it. */
function TopicMix({ topics }: { topics: Topic[] }) {
  const total = topics.reduce((sum, item) => sum + item.count, 0);
  return (
    <span className="topic-mix">
      <span className="topic-bar" role="img" aria-label={topics.map((item) => `${item.name}: ${item.count}`).join(", ")}>
        {topics.map((item) => (
          <span key={item.id} className={`opt-${colorFor(item.id)}`} style={{ flexGrow: item.count / total }} title={`${item.name}: ${item.count} post${item.count === 1 ? "" : "s"}`} />
        ))}
      </span>
      <span className="topic-legend">
        {topics.map((item) => (
          <span key={item.id} className={`opt-${colorFor(item.id)}`}>
            {item.name}
          </span>
        ))}
      </span>
    </span>
  );
}

/* One of the three most saved: a cover in the color of their main topic,
   their picture on its edge, the topic mix, and their latest post. */
function FeaturedAuthor({ author, rank }: { author: Author; rank: number }) {
  const main = author.topics?.[0];
  const cover = main ? colorFor(main.id) : colorForText(author.username);
  return (
    <Link to={authorPath(author.username)} className="author-feature">
      <span className={`author-cover cover-${cover}`} aria-hidden="true" />
      <span className="author-feature-body">
        <span className="author-feature-top">
          <AuthorAvatar name={author.name} username={author.username} src={author.avatar} size="post" />
          <span className="author-count" title={`Number ${rank} by posts saved`}>
            {author.count.toLocaleString()} post{author.count === 1 ? "" : "s"}
          </span>
        </span>
        <span className="author-names">
          <strong>{author.name || `@${author.username}`}</strong>
          <span className="handle">@{author.username}</span>
        </span>
        {author.topics?.length ? <TopicMix topics={author.topics} /> : null}
        {author.latest && <span className="author-latest">{author.latest}</span>}
      </span>
    </Link>
  );
}

/* Everyone else: picture, name, handle with a dot per top topic, and a count. */
function AuthorTile({ author }: { author: Author }) {
  return (
    <Link to={authorPath(author.username)} className="author-tile" title={`Last saved ${formatDate(author.latestSavedAt)}`}>
      <AuthorAvatar name={author.name} username={author.username} src={author.avatar} size="post" />
      <span className="author-tile-text">
        <strong>{author.name || `@${author.username}`}</strong>
        <span className="author-tile-meta">
          <span className="handle">@{author.username}</span>
          {author.topics?.map((item) => (
            <span key={item.id} className={`topic-dot opt-${colorFor(item.id)}`} title={item.name} />
          ))}
        </span>
      </span>
      <span className="author-tile-count">{author.count.toLocaleString()}</span>
    </Link>
  );
}
