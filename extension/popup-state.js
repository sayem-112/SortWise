(function (root, factory) {
  const api = factory();
  root.SortwisePopupState = api;
  if (typeof module === "object" && module.exports) module.exports = api;
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";

  function isBookmarksURL(value) {
    try {
      const url = new URL(value);
      return url.protocol === "https:" && url.hostname === "x.com" && (url.pathname === "/i/bookmarks" || url.pathname.startsWith("/i/bookmarks/"));
    } catch {
      return false;
    }
  }

  function count(value) {
    return Math.max(0, Number(value) || 0);
  }

  function normalizeImportState(value) {
    const input = value && typeof value === "object" ? value : {};
    return {
      running: input.running === true,
      mode: input.mode === "full" ? "full" : "new",
      auto: input.auto === true,
      phase: typeof input.phase === "string" ? input.phase : "idle",
      status: typeof input.status === "string" ? input.status : "Ready to import.",
      discovered: count(input.discovered),
      // Older saved states used "imported" (new + updated) and "duplicates".
      inserted: count(input.inserted ?? input.imported),
      updated: count(input.updated),
      unchanged: count(input.unchanged ?? input.duplicates),
      failed: count(input.failed),
      startedAt: typeof input.startedAt === "string" ? input.startedAt : null,
      finishedAt: typeof input.finishedAt === "string" ? input.finishedAt : null,
    };
  }

  // "3 minutes ago", "yesterday", or a date, for the last-import line.
  function relativeTime(value, now = Date.now()) {
    // SQLite stores UTC as "YYYY-MM-DD HH:MM:SS" with no zone; read it as UTC.
    const text = String(value || "");
    const time = Date.parse(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(text) ? `${text.replace(" ", "T")}Z` : text);
    if (Number.isNaN(time)) return "";
    const minutes = Math.round((now - time) / 60000);
    if (minutes < 1) return "just now";
    if (minutes < 60) return `${minutes} minute${minutes === 1 ? "" : "s"} ago`;
    const hours = Math.round(minutes / 60);
    if (hours < 24) return `${hours} hour${hours === 1 ? "" : "s"} ago`;
    const days = Math.round(hours / 24);
    if (days === 1) return "yesterday";
    if (days < 7) return `${days} days ago`;
    return new Date(time).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
  }

  return { isBookmarksURL, normalizeImportState, relativeTime };
});
