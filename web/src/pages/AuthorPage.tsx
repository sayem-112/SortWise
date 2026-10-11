import { useQuery } from "@tanstack/react-query";
import { ArrowUpRight, Check, UserX } from "lucide-react";
import { Link, Navigate, useLocation, useParams } from "react-router-dom";
import { AuthorAvatar } from "../components/Author";
import { EmptyState } from "../components/ui";
import { APIError, getAuthor, type AuthorProfile } from "../lib/api";
import { authorPath } from "../lib/authors";
import { colorFor, formatDate, parseTime } from "../lib/format";
import { BookmarksPage, type LibraryFilters } from "./BookmarksPage";

/* One author: who they are and what their saved posts are about, then the
   library showing only their posts. */
export function AuthorPage() {
  const { username = "" } = useParams();
  const location = useLocation();
  const handle = username.replace(/^@/, "");
  const profile = useQuery({
    queryKey: ["author", handle.toLowerCase()],
    queryFn: () => getAuthor(handle),
    retry: (count, error) => !(error instanceof APIError && error.status === 404) && count < 2,
  });

  // One address per person: /authors/@Ada and /authors/ADA both become /authors/ada.
  if (location.pathname !== authorPath(handle)) {
    return <Navigate to={{ pathname: authorPath(handle), search: location.search }} replace />;
  }

  if (profile.isLoading) {
    return (
      <main className="page full" aria-busy="true">
        <div className="skeleton skeleton-title" />
      </main>
    );
  }
  // Gone means gone: after their last post is deleted, an earlier answer is not shown.
  const missing = profile.error instanceof APIError && profile.error.status === 404;
  if (missing || !profile.data) {
    return (
      <main className="page full">
        <EmptyState
          icon={UserX}
          title={`No saved posts from @${handle}`}
          action={
            <Link to="/authors" className="button secondary">
              See all authors
            </Link>
          }
        >
          {missing
            ? "You haven't saved anything from this account, or those posts were deleted or archived."
            : "The local server did not answer. Check that Sortwise is still running."}
        </EmptyState>
      </main>
    );
  }

  const author = profile.data;
  return (
    <BookmarksPage
      author={author.username}
      icon={
        <div className="page-icon author-page-icon">
          <AuthorAvatar name={author.name} username={author.username} src={author.avatar} size="page" />
        </div>
      }
      title={author.name || `@${author.username}`}
      actions={(filters) => <AuthorFacts author={author} filters={filters} />}
    />
  );
}

function monthYear(value: string) {
  return parseTime(value).toLocaleDateString(undefined, { month: "short", year: "numeric" });
}

/* Under the title: the handle with a way out to X, when their posts are
   from, and the topics they are saved for. A topic is a filter on this page:
   click to narrow to it, click again to clear, and they combine with the
   search and the filter row. */
function AuthorFacts({ author, filters }: { author: AuthorProfile; filters: LibraryFilters }) {
  const from = author.firstPostedAt && monthYear(author.firstPostedAt);
  const to = author.latestPostedAt && monthYear(author.latestPostedAt);
  const topics = [
    ...author.categories.map((item) => ({ ...item, key: `c-${item.id}`, kind: "category", on: filters.categories.includes(item.id), toggle: () => filters.toggleCategory(item.id), color: colorFor(item.id) })),
    ...author.topTags.map((item) => ({ ...item, key: `t-${item.id}`, kind: "tag", on: filters.tags.includes(item.id), toggle: () => filters.toggleTag(item.id), color: "default" as const })),
  ];
  return (
    <div className="author-facts">
      <p className="author-line">
        <a className="author-handle" href={`https://x.com/${author.username}`} target="_blank" rel="noreferrer" title="Open their profile on X">
          @{author.username}
          <ArrowUpRight size={13} aria-hidden="true" />
        </a>
        {from && (
          <>
            <span aria-hidden="true">·</span>
            <span>{from === to ? `Posted ${from}` : `Posted ${from} to ${to}`}</span>
          </>
        )}
        <span aria-hidden="true">·</span>
        <span>Last saved {formatDate(author.latestSavedAt)}</span>
      </p>
      {topics.length > 0 && (
        <div className="author-topics" role="group" aria-label="Filter by what they are saved for">
          <span className="author-topics-label">Saved for</span>
          {topics.map((item) => (
            <button
              key={item.key}
              type="button"
              className={`topic-chip opt-${item.color} ${item.on ? "on" : ""}`}
              aria-pressed={item.on}
              title={item.on ? `Showing only ${item.name}. Click to clear.` : `Show only their ${item.count} post${item.count === 1 ? "" : "s"} in ${item.name}`}
              onClick={item.toggle}
            >
              {item.on && <Check size={12} strokeWidth={2.5} aria-hidden="true" />}
              <span className="topic-name">{item.name}</span>
              {item.count > 1 && <span className="topic-count">{item.count}</span>}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
