import { describe, expect, it } from "vitest";
import type { CollectionQuery } from "./api";
import { collectionParams, hasCriteria, sameQuery } from "./collections";

const empty: CollectionQuery = { q: "", mode: "", categories: [], tags: [], status: "", list: "", author: "", sort: "" };

describe("smart collection searches", () => {
  it("opens the library on exactly what was saved", () => {
    const params = collectionParams({ ...empty, q: "rust async", mode: "meaning", categories: [3, 1], tags: [9], status: "review", list: "none", sort: "posted_desc" });
    expect(params.get("q")).toBe("rust async");
    expect(params.get("mode")).toBe("meaning");
    expect(params.getAll("category")).toEqual(["3", "1"]);
    expect(params.getAll("tag")).toEqual(["9"]);
    expect(params.get("status")).toBe("review");
    expect(params.get("list")).toBe("none");
    expect(params.get("sort")).toBe("posted_desc");
    // A word search says so, whatever the person last used elsewhere.
    expect(collectionParams({ ...empty, q: "rust" }).get("mode")).toBe("words");
  });

  it("tells a changed search from the same one written differently", () => {
    const saved: CollectionQuery = { ...empty, q: "rust async", mode: "meaning", categories: [1, 3], tags: [9], author: "Ada", sort: "relevance" };
    expect(sameQuery({ ...saved, q: " rust  async ", categories: [3, 1], author: "ada", sort: "" }, saved)).toBe(true);
    expect(sameQuery({ ...saved, mode: "" }, saved)).toBe(false);
    expect(sameQuery({ ...saved, tags: [] }, saved)).toBe(false);
    expect(sameQuery({ ...saved, sort: "posted_desc" }, saved)).toBe(false);
    expect(sameQuery({ ...saved, list: "2" }, saved)).toBe(false);
    // With no words there is no mode to differ in.
    expect(sameQuery({ ...empty, status: "review", mode: "meaning" }, { ...empty, status: "review" })).toBe(true);
  });

  it("needs a search or a filter", () => {
    expect(hasCriteria(empty)).toBe(false);
    expect(hasCriteria({ ...empty, q: "  " })).toBe(false);
    expect(hasCriteria({ ...empty, sort: "posted_desc" })).toBe(false);
    expect(hasCriteria({ ...empty, tags: [1] })).toBe(true);
    expect(hasCriteria({ ...empty, author: "ada" })).toBe(true);
  });
});
