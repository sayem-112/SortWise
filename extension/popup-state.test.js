import { describe, expect, it } from "vitest";
import "./popup-state.js";

const { isBookmarksURL, normalizeImportState } = globalThis.SortwisePopupState;

describe("popup state", () => {
  it("accepts only the X bookmarks route", () => {
    expect(isBookmarksURL("https://x.com/i/bookmarks")).toBe(true);
    expect(isBookmarksURL("https://x.com/i/bookmarks/folders/123")).toBe(true);
    expect(isBookmarksURL("https://x.com/home")).toBe(false);
    expect(isBookmarksURL("https://x.com.evil.example/i/bookmarks")).toBe(false);
    expect(isBookmarksURL("not a URL")).toBe(false);
  });

  it("normalizes persisted counters and running state", () => {
    expect(normalizeImportState({ running: true, discovered: "12", failed: -1 })).toMatchObject({
      running: true,
      discovered: 12,
      failed: 0,
      phase: "idle",
    });
  });
});

describe("import history", () => {
  const { relativeTime } = globalThis.SortwisePopupState;

  it("reads SQLite timestamps as UTC and describes them relatively", () => {
    const now = Date.parse("2026-09-30T12:00:00Z");
    expect(relativeTime("2026-09-30 10:00:00", now)).toBe("2 hours ago");
    expect(relativeTime("2026-09-30T11:59:40Z", now)).toBe("just now");
    expect(relativeTime("2026-09-29 11:00:00", now)).toBe("yesterday");
    expect(relativeTime("", now)).toBe("");
  });

  it("maps counters saved by the previous version", () => {
    expect(normalizeImportState({ imported: 5, duplicates: 2 })).toMatchObject({ inserted: 5, unchanged: 2, mode: "new" });
  });
});
