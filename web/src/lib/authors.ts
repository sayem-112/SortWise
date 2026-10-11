/* An author's page. Handles are not case-sensitive, so the address is always
   lower case: every link to one person is the same link. */
export function authorPath(username: string) {
  return `/authors/${encodeURIComponent(username.replace(/^@/, "").toLowerCase())}`;
}
