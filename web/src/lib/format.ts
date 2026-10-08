/* Notion's nine select-option colors. Categories and tags get a stable color from their id. */
const OPTION_COLORS = ["gray", "brown", "orange", "yellow", "green", "blue", "purple", "pink", "red"] as const;
export type OptionColor = (typeof OPTION_COLORS)[number] | "default";

export function colorFor(id: number): OptionColor {
  return OPTION_COLORS[Math.abs(id) % OPTION_COLORS.length];
}

export const isMac = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform);
export const modKey = isMac ? "⌘" : "Ctrl+";

export function initials(value: string) {
  return value.split(/\s+/).slice(0, 2).map((part) => part[0]).join("").toUpperCase() || "X";
}

/* The server stores times in UTC as "YYYY-MM-DD HH:MM:SS" with no zone, which
   browsers would read as local time. Read that form as UTC; full ISO times
   (with Z or an offset) pass through unchanged. */
export function parseTime(value: string) {
  return new Date(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(value) ? `${value.replace(" ", "T")}Z` : value);
}

export function formatDate(value: string) {
  return parseTime(value).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
}

export function formatDateTime(value: string) {
  return parseTime(value).toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
}

const linkOnly = /^(\s*https:\/\/t\.co\/\w+\s*)*$/;

/* The best one-line description of a post: its text, or, for X Articles and
   link-only posts, the card title, then quoted text or media description. */
export function postPreview(bookmark: {
  text: string;
  mediaDescription: string;
  summary?: string;
  visibleContext: { card?: { title: string }; quotedPost?: { text: string } };
}) {
  if (bookmark.text && !linkOnly.test(bookmark.text)) return bookmark.text;
  return (
    bookmark.visibleContext.card?.title ||
    bookmark.visibleContext.quotedPost?.text ||
    bookmark.mediaDescription ||
    bookmark.summary ||
    bookmark.text ||
    "Media bookmark"
  );
}
