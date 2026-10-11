import { Link } from "react-router-dom";
import type { ReactNode } from "react";
import { authorPath } from "../lib/authors";
import { colorForText, initials } from "../lib/format";

/* An author's initials on their own color. The color comes from the handle,
   so one person looks the same in every row, post, and search result. */
export function AuthorAvatar({ name, username, size = "row" }: { name: string; username: string; size?: "row" | "page" }) {
  return (
    <span className={`author-avatar size-${size} opt-${colorForText(username || name)}`} aria-hidden="true">
      {initials(name || username)}
    </span>
  );
}

/* A name that opens the author's page in Sortwise. In a table row, where the
   row itself opens the post, it lights up on hover to show it goes elsewhere. */
export function AuthorLink({ username, children, className = "" }: { username: string; children: ReactNode; className?: string }) {
  if (!username) return <span className={className}>{children}</span>;
  return (
    <Link to={authorPath(username)} className={`author-link ${className}`} title={`All saved posts from @${username}`} onClick={(event) => event.stopPropagation()}>
      {children}
    </Link>
  );
}
