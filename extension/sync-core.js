(function (root, factory) {
  const api = factory();
  root.SortwiseSync = api;
  if (typeof module === "object" && module.exports) module.exports = api;
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";

  // Pause between pages of the bookmarks list, plus up to JITTER_MS at random,
  // so a sync reads like a person scrolling rather than a burst of requests.
  const PAGE_DELAY_MS = 2500;
  const JITTER_MS = 1000;
  // "New only" stops after this many consecutive posts that were already saved.
  const KNOWN_STREAK_LIMIT = 40;
  // When X says to slow down: wait, then retry, at most this many times in a row.
  const MAX_RATE_LIMIT_RETRIES = 5;
  const MIN_RATE_LIMIT_WAIT_MS = 30_000;
  const MAX_RATE_LIMIT_WAIT_MS = 15 * 60_000;

  function findBottomCursor(node) {
    if (!node || typeof node !== "object") return null;
    if (node.cursorType === "Bottom" && typeof node.value === "string") return node.value;
    for (const value of Object.values(node)) {
      const cursor = findBottomCursor(value);
      if (cursor) return cursor;
    }
    return null;
  }

  function countTimelineTweets(node) {
    if (!node || typeof node !== "object") return 0;
    if (node.__typename === "TimelineTweet" || node.itemType === "TimelineTweet") return 1;
    let total = 0;
    for (const value of Object.values(node)) total += countTimelineTweets(value);
    return total;
  }

  // The Bookmarks request X made, without its position in the list.
  function stripCursor(url) {
    try {
      const parsed = new URL(url);
      const variables = JSON.parse(parsed.searchParams.get("variables") || "{}");
      delete variables.cursor;
      parsed.searchParams.set("variables", JSON.stringify(variables));
      return parsed.toString();
    } catch {
      return null;
    }
  }

  function pageURL(templateURL, cursor) {
    const parsed = new URL(templateURL);
    const variables = JSON.parse(parsed.searchParams.get("variables") || "{}");
    if (cursor) variables.cursor = cursor;
    else delete variables.cursor;
    parsed.searchParams.set("variables", JSON.stringify(variables));
    return parsed.toString();
  }

  function isBookmarksRequest(url) {
    try {
      const parsed = new URL(url);
      return parsed.origin === "https://x.com" && /^\/i\/api\/graphql\/[\w-]+\/Bookmarks$/.test(parsed.pathname);
    } catch {
      return false;
    }
  }

  // Bookmarks are listed newest first, so a run of already-saved posts means the
  // rest of the list was saved before. Posts that were only refreshed with
  // X's fuller data, or deleted in Sortwise, count as already known.
  function nextStreak(streak, result) {
    const fresh = Number(result?.inserted || 0) + Number(result?.updated || 0);
    const known = Number(result?.unchanged || 0) + Number(result?.upgraded || 0) + Number(result?.deleted || 0);
    return fresh > 0 ? 0 : streak + known;
  }

  function rateLimitWait(rateLimit, attempt, now = Date.now()) {
    const reset = Number(rateLimit?.reset);
    if (Number.isFinite(reset) && reset > 0) {
      const untilReset = reset * 1000 - now + 2000;
      return Math.min(Math.max(untilReset, MIN_RATE_LIMIT_WAIT_MS), MAX_RATE_LIMIT_WAIT_MS);
    }
    return Math.min(60_000 * 2 ** attempt, MAX_RATE_LIMIT_WAIT_MS);
  }

  function pageDelay(random = Math.random) {
    return PAGE_DELAY_MS + Math.round(random() * JITTER_MS);
  }

  return {
    KNOWN_STREAK_LIMIT,
    MAX_RATE_LIMIT_RETRIES,
    findBottomCursor,
    countTimelineTweets,
    stripCursor,
    pageURL,
    isBookmarksRequest,
    nextStreak,
    rateLimitWait,
    pageDelay,
  };
});
