import type { QueryClient } from "@tanstack/react-query";

/* Everything derived from the set of bookmarks: lists of posts, counts,
   authors, and the finder's results. Call it after a bookmark is deleted,
   archived, or re-filed, so no page keeps describing posts as they were. */
export function refreshLibrary(client: QueryClient) {
  for (const key of ["bookmarks", "dashboard", "setup", "authors", "author", "quick-search"]) {
    client.invalidateQueries({ queryKey: [key] });
  }
}
