import { useState, type ReactNode } from "react";
import { Link } from "react-router-dom";
import { authorPath } from "../lib/authors";
import { colorForText, initials } from "../lib/format";

/* An author's saved profile picture, or, when there is none (or it fails to
   load), their initials on their own color. The color comes from the handle,
   so one person looks the same in every row, post, and search result. */
export function AuthorAvatar({
  name,
  username,
  src,
  size = "row",
}: {
  name: string;
  username: string;
  // Path of the saved picture, from the server.
  src?: string;
  size?: "row" | "post" | "page";
}) {
  // Remembers which address failed, so a new picture gets its own chance.
  const [failed, setFailed] = useState<string>();
  if (src && failed !== src) {
    // Pictures are small files on this computer: loading them right away
    // avoids a flash of empty circles in lists and in the finder.
    return <img key={src} className={`author-avatar size-${size}`} src={src} alt="" decoding="async" onError={() => setFailed(src)} />;
  }
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
