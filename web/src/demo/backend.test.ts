import { beforeEach, describe, expect, it } from "vitest";
import type { Bookmark, BookmarkPage, Category, Collection, List, QuickResults, RelatedBookmark, Tag } from "../lib/api";
import { demoApi, resetDemo } from "./backend";
import { hubness, nearest, rankWords, snippet, terms } from "./search";

const get = <T>(path: string) => demoApi<T>(path);
const send = <T>(path: string, method: string, body?: unknown) => demoApi<T>(path, { method, body: body === undefined ? undefined : JSON.stringify(body) });

describe("the demo's stand-in server", () => {
  beforeEach(() => {
    localStorage.clear();
    resetDemo();
  });

  it("serves the library newest first, in pages, with every field the app reads", async () => {
    const first = await get<BookmarkPage>("/bookmarks?page=1&pageSize=10");
    expect(first.total).toBeGreaterThan(50);
    expect(first.items).toHaveLength(10);
    const post = first.items[0];
    expect(post).toMatchObject({ processingStatus: "completed", archived: false });
    expect(post.summary.length).toBeGreaterThan(10);
    expect(post.categories.length + post.tags.length).toBeGreaterThan(0);
    expect(post.authorAvatar).toMatch(/^https:\/\/pbs\.twimg\.com\/profile_images\//);
    const saved = first.items.map((item) => item.importedAt);
    expect([...saved].sort().reverse()).toEqual(saved);
    // The newest post was saved recently, so the demo never looks stale.
    expect(Date.now() - new Date(`${saved[0].replace(" ", "T")}Z`).getTime()).toBeLessThan(3 * 3600_000);

    const second = await get<BookmarkPage>("/bookmarks?page=2&pageSize=10");
    expect(second.items.map((item) => item.id)).not.toContain(post.id);
  });

  it("searches words the way the app does, and widens a search by meaning", async () => {
    const words = await get<BookmarkPage>("/bookmarks?q=habits");
    expect(words.total).toBeGreaterThan(0);
    expect(words.search).toBeUndefined();
    expect(words.items.some((item) => item.match)).toBe(false);

    const meaning = await get<BookmarkPage>("/bookmarks?q=habits&mode=meaning");
    expect(meaning.search).toEqual({ meaning: true, notice: undefined });
    expect(meaning.total).toBeGreaterThan(words.total);
    const extra = meaning.items.filter((item) => item.match === "meaning");
    expect(extra.length).toBe(meaning.total - words.total);
    for (const item of extra) expect(words.items.map((other) => other.id)).not.toContain(item.id);

    // Words that match nothing cannot be widened here, and it says why.
    const nothing = await get<BookmarkPage>("/bookmarks?q=zzzqqq&mode=meaning");
    expect(nothing.total).toBe(0);
    expect(nothing.search?.meaning).toBe(false);
    expect(nothing.search?.notice).toMatch(/demo/);

    // Turned off in Settings, it says so.
    await send("/settings/ai", "PATCH", { meaning: false });
    const off = await get<BookmarkPage>("/bookmarks?q=habits&mode=meaning");
    expect(off.search).toEqual({ meaning: false, notice: "Search by meaning is turned off in Settings." });
    expect((await get<{ items: RelatedBookmark[] }>(`/bookmarks/${words.items[0].id}/related`)).items).toEqual([]);
  });

  it("finds related posts, never the post itself", async () => {
    const page = await get<BookmarkPage>("/bookmarks?q=simulators");
    const post = page.items[0];
    const related = await get<{ items: RelatedBookmark[] }>(`/bookmarks/${post.id}/related`);
    expect(related.items.length).toBeGreaterThan(0);
    expect(related.items.length).toBeLessThanOrEqual(5);
    expect(related.items.map((item) => item.id)).not.toContain(post.id);
    expect(related.items[0].text.length).toBeGreaterThan(0);
  });

  it("answers the finder with marked excerpts and people", async () => {
    const results = await get<QuickResults>("/search/quick?q=karpathy&limit=8");
    expect(results.authors[0]?.username.toLowerCase()).toBe("karpathy");
    expect(results.bookmarks.length).toBeGreaterThan(0);
    const marked = results.bookmarks.find((item) => item.authorMatch || item.usernameMatch);
    expect(marked?.authorMatch ?? marked?.usernameMatch).toContain(String.fromCharCode(2));
  });

  it("keeps lists, stars, edits, and collections in this browser, until reset", async () => {
    const lists = await get<{ items: List[] }>("/lists");
    const favorites = lists.items.find((item) => item.kind === "favorites")!;
    expect(favorites.count).toBeGreaterThan(0);
    const post = (await get<BookmarkPage>("/bookmarks?list=none&pageSize=1")).items[0];

    const created = await send<List>("/lists", "POST", { name: "Weekend" });
    await send(`/lists/${created.id}/bookmarks`, "POST", { bookmarkIds: [post.id], inList: true });
    await send<Bookmark>(`/bookmarks/${post.id}`, "PATCH", { summary: "My own words" });
    const collection = await send<Collection>("/collections", "POST", { name: "Habits", query: { q: "habits", mode: "meaning", categories: [], tags: [], status: "", list: "", author: "", sort: "" } });
    expect(collection.count).toBeGreaterThan(0);
    await expect(send("/collections", "POST", { name: "habits", query: { q: "x" } })).rejects.toMatchObject({ status: 409 });
    await expect(send("/collections", "POST", { name: "All", query: {} })).rejects.toMatchObject({ status: 422 });

    // Survives a reload: the state is read back from this browser.
    const reloaded = await get<Bookmark>(`/bookmarks/${post.id}`);
    expect(reloaded.summary).toBe("My own words");
    expect(reloaded.lists).toContain(created.id);
    expect((await get<BookmarkPage>(`/bookmarks?list=${created.id}`)).total).toBe(1);

    resetDemo();
    expect((await get<Bookmark>(`/bookmarks/${post.id}`)).summary).not.toBe("My own words");
    expect((await get<{ items: Collection[] }>("/collections")).items).toEqual([]);
  });

  it("keeps saved searches and categories consistent through merges, deletes, and renames", async () => {
    const tags = await get<{ items: Tag[] }>("/tags");
    const [kept, merged] = tags.items.filter((item) => item.count > 0);
    const both = await send<Collection>("/collections", "POST", { name: "Two tags", query: { q: "", mode: "", categories: [], tags: [merged.id], status: "", list: "", author: "", sort: "" } });
    await send(`/tags/${merged.id}/merge`, "POST", { targetId: kept.id });
    const after = (await get<{ items: Collection[] }>("/collections")).items.find((item) => item.id === both.id)!;
    expect(after.query.tags).toEqual([kept.id]);
    expect(after.count).toBeGreaterThan(0);

    // A renamed tag is still found by its old name.
    const before = (await get<BookmarkPage>(`/bookmarks?q=${encodeURIComponent(kept.name)}`)).total;
    await send(`/tags/${kept.id}`, "PATCH", { name: "zz renamed", kind: kept.kind });
    expect((await get<BookmarkPage>(`/bookmarks?q=${encodeURIComponent(kept.name)}`)).total).toBe(before);

    // A category cannot end up inside itself.
    const categories = (await get<{ items: Category[] }>("/categories")).items;
    const [a, b] = categories;
    await send(`/categories/${b.id}`, "PATCH", { name: b.name, description: "", parentId: a.id });
    await expect(send(`/categories/${a.id}`, "PATCH", { name: a.name, description: "", parentId: b.id })).rejects.toMatchObject({ status: 422 });
    await expect(send(`/categories/${a.id}`, "PATCH", { name: a.name, description: "", parentId: a.id })).rejects.toMatchObject({ status: 422 });
    // Merging a parent moves its children to the target.
    const c = categories[2];
    await send(`/categories/${a.id}/merge`, "POST", { targetId: c.id });
    expect((await get<{ items: Category[] }>("/categories")).items.find((item) => item.id === b.id)?.parentId).toBe(c.id);

    // Saved searches are checked as the app checks them.
    await expect(send("/collections", "POST", { name: "Odd", query: { q: "x", status: "bogus" } })).rejects.toMatchObject({ status: 422 });
    await expect(send("/collections", "POST", { name: "Odd", query: { q: "x", sort: "added_desc" } })).rejects.toMatchObject({ status: 422 });
  });

  it("explains what only the app can do", async () => {
    await expect(send("/settings/ai/keys/gemini", "PUT", { apiKey: "x".repeat(20) })).rejects.toMatchObject({ status: 403 });
    await expect(send("/extension/pairing-codes", "POST", {})).rejects.toMatchObject({ status: 403 });
    await expect(send("/backups", "POST", {})).rejects.toMatchObject({ status: 403 });
    await expect(get("/nope")).rejects.toMatchObject({ status: 404 });
  });
});

describe("demo search helpers", () => {
  it("matches the start of words, and one-letter words whole", () => {
    expect(terms("  Next.js  c++ ")).toEqual(["next", "js", "c"]);
    const posts = [
      { id: 1, text: "Testing in Go" },
      { id: 2, text: "A test of patience, test test" },
      { id: 3, text: "Cooking" },
    ];
    expect(rankWords(posts, "test", (post) => post.text).map((post) => post.id)).toEqual([2, 1]);
    expect(rankWords(posts, "c", (post) => post.text)).toEqual([]);
  });

  it("marks matches in an excerpt", () => {
    const excerpt = snippet("One two three four five six seven eight nine ten eleven twelve", "seven", 4);
    expect(excerpt).toBe(`…six ${String.fromCharCode(2)}seven${String.fromCharCode(3)} eight nine…`);
  });

  it("finds only what stands out, and holds back posts close to everything", () => {
    const vector = (...values: number[]) => Int8Array.from([...values, ...new Array(8 - values.length).fill(0)]);
    const vectors = new Map<number, Int8Array>([
      [1, vector(100, 0, 0, 0, 0, 0, 1, 0)],
      [2, vector(96, 10, 0, 0, 0, 0, 1, 0)],
      [3, vector(0, 0, 100, 0, 0, 0, 1, 0)],
      [4, vector(0, 0, 0, 100, 0, 0, 1, 0)],
      [5, vector(0, 0, 0, 0, 100, 0, 1, 0)],
      [6, vector(0, 0, 0, 0, 0, 100, 1, 0)],
      [7, vector(0, 100, 0, 0, 0, 0, 1, 0)],
      [8, vector(0, 0, 0, 0, 0, 0, 1, 100)],
      [9, vector(40, 40, 40, 40, 40, 40, 1, 40)],
    ]);
    const query = Float32Array.from([1, 0, 0, 0, 0, 0, 0, 0]);
    expect(nearest(vectors, query, 5, new Set(), 0.2).map((hit) => hit.id)).toEqual([1, 2]);
    expect(nearest(vectors, query, 5, new Set([1]), 0.2).map((hit) => hit.id)).toEqual([2]);
    // Post 9 is a little close to everything; held back, it is not offered.
    const hubs = hubness(vectors, 3);
    expect(hubs.get(9)).toBeGreaterThan(hubs.get(3)!);
  });
});
