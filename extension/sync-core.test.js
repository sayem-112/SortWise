import { describe, expect, it } from "vitest";
import sync from "./sync-core.js";

const page = (ids, cursor) => ({
  data: {
    bookmark_timeline_v2: {
      timeline: {
        instructions: [
          {
            entries: [
              ...ids.map((id) => ({ entryId: `tweet-${id}`, content: { itemContent: { __typename: "TimelineTweet", tweet_results: { result: { rest_id: id } } } } })),
              { entryId: "cursor-top", content: { cursorType: "Top", value: "TOP" } },
              ...(cursor ? [{ entryId: "cursor-bottom", content: { cursorType: "Bottom", value: cursor } }] : []),
            ],
          },
        ],
      },
    },
  },
});

describe("sync helpers", () => {
  it("finds the next-page cursor and counts posts", () => {
    expect(sync.findBottomCursor(page(["1", "2"], "NEXT"))).toBe("NEXT");
    expect(sync.findBottomCursor(page(["1"]))).toBeNull();
    expect(sync.countTimelineTweets(page(["1", "2", "3"], "NEXT"))).toBe(3);
  });

  it("builds page URLs from X's own request", () => {
    const original = "https://x.com/i/api/graphql/abc/Bookmarks?variables=%7B%22count%22%3A20%2C%22cursor%22%3A%22OLD%22%7D&features=%7B%7D";
    const template = sync.stripCursor(original);
    expect(JSON.parse(new URL(template).searchParams.get("variables"))).toEqual({ count: 20 });
    expect(JSON.parse(new URL(sync.pageURL(template, "NEXT")).searchParams.get("variables"))).toEqual({ count: 20, cursor: "NEXT" });
    expect(new URL(sync.pageURL(template, "NEXT")).searchParams.get("features")).toBe("{}");
  });

  it("only accepts X's Bookmarks request as a template", () => {
    expect(sync.isBookmarksRequest("https://x.com/i/api/graphql/abc-1/Bookmarks?variables=%7B%7D")).toBe(true);
    expect(sync.isBookmarksRequest("https://x.com/i/api/graphql/abc/HomeTimeline?variables=%7B%7D")).toBe(false);
    expect(sync.isBookmarksRequest("https://evil.example/i/api/graphql/abc/Bookmarks")).toBe(false);
  });

  it("counts a run of already-saved posts, reset by anything new", () => {
    let streak = sync.nextStreak(0, { unchanged: 20 });
    expect(streak).toBe(20);
    streak = sync.nextStreak(streak, { unchanged: 19, inserted: 1 });
    expect(streak).toBe(0);
    // Posts only refreshed with fuller data, or deleted in Sortwise, are known.
    expect(sync.nextStreak(10, { unchanged: 5, upgraded: 15 })).toBe(30);
    expect(sync.nextStreak(10, { deleted: 2 })).toBe(12);
  });

  it("waits for X's rate-limit reset, within bounds", () => {
    const now = 1_000_000_000_000;
    expect(sync.rateLimitWait({ reset: String(now / 1000 + 120) }, 0, now)).toBe(122_000);
    expect(sync.rateLimitWait({ reset: String(now / 1000 + 1) }, 0, now)).toBe(30_000);
    expect(sync.rateLimitWait({}, 0, now)).toBe(60_000);
    expect(sync.rateLimitWait({}, 10, now)).toBe(15 * 60_000);
    expect(sync.pageDelay(() => 0.5)).toBe(3000);
  });
});
