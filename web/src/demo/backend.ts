/* The demo's stand-in for the Sortwise server. The website demo has no
   server and no AI service, so every request the app makes is answered
   here, in the browser, from a library of real public posts. What a visitor
   changes (stars, lists, collections, edits) is kept in this browser only. */
import {
  APIError,
  type AISettings,
  type Bookmark,
  type Category,
  type Collection,
  type CollectionQuery,
  type List,
  type Tag,
  type TagKind,
} from "../lib/api";
import type { OptionColor } from "../lib/format";
import { hubness, MATCH_START, matches, nearest, rankWords, snippet, terms, type Vectors } from "./search";
import source from "./library.json";

type Assignment = { id: number; confidence: number | null; state: "automatic" | "added" | "removed" };
type Post = Omit<Bookmark, "categories" | "tags" | "lists" | "authorAvatar" | "processingStatus" | "archived" | "summary" | "match"> & {
  aiSummary: string;
  manualSummary: string | null;
  avatar: string;
  archived: boolean;
  categories: Assignment[];
  tags: Assignment[];
};
type State = {
  posts: Post[];
  categories: { id: number; name: string; description: string; parentId?: number; active: boolean }[];
  tags: { id: number; name: string; kind: TagKind; aliases: string[] }[];
  lists: { id: number; name: string; kind: "favorites" | "custom"; icon: string; color: OptionColor; pinnedAt: number | null }[];
  items: { listId: number; bookmarkId: number; addedAt: number }[];
  collections: { id: number; name: string; color: OptionColor; query: CollectionQuery }[];
  settings: { paused: boolean; backup: boolean; language: string; meaning: boolean };
  lastIds: { list: number; collection: number; category: number; tag: number };
};

type SourcePost = (typeof source.posts)[number];

export const LIST_ICONS = ["list", "bookmark", "book-open", "lightbulb", "code", "palette", "chef-hat", "dumbbell", "wallet", "briefcase", "graduation-cap", "flask", "music", "film", "plane", "heart", "rocket", "target", "newspaper", "sparkles"];
export const COLORS: OptionColor[] = ["purple", "blue", "green", "orange", "pink", "brown", "red", "gray", "yellow"];
// Saved demo state is tied to this exact library, so a changed library
// starts fresh instead of mixing old copies of posts with new vectors.
function fingerprint() {
  let hash = 0;
  for (const post of source.posts) {
    for (const char of `${post.id}|${post.postId}|${post.summary}|${post.text.length}`) hash = (Math.imul(hash, 31) + char.charCodeAt(0)) | 0;
  }
  return (hash >>> 0).toString(36);
}
const STORAGE = `sortwise-demo:${fingerprint()}:3`;

/* ---------- State ---------- */

// SQLite's CURRENT_TIMESTAMP form, which the app reads as UTC.
function timestamp(date = new Date()) {
  return date.toISOString().replace("T", " ").slice(0, 19);
}

function parse(value: string) {
  return new Date(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(value) ? `${value.replace(" ", "T")}Z` : value).getTime();
}

/* A fresh library. Each post counts as saved a few hours after it was
   posted, and the dates are moved so the newest was saved two hours ago,
   keeping their spacing, so the demo never looks stale. */
function initial(): State {
  const saved = (post: SourcePost) => (post.postedAt ? parse(post.postedAt) + (3 + (post.id % 9)) * 3600_000 : parse(post.importedAt));
  const newest = Math.max(...source.posts.map(saved));
  const shift = Date.now() - 2 * 3600_000 - newest;
  const posts = source.posts.map((post: SourcePost): Post => {
    const { vector: _vector, summary, categories, tags, ...rest } = post;
    void _vector;
    return {
      ...(rest as unknown as Post),
      importedAt: timestamp(new Date(saved(post) + shift)),
      aiSummary: summary,
      manualSummary: null,
      archived: false,
      categories: categories.map((item) => ({ ...item, state: "automatic" as const })),
      tags: tags.map((item) => ({ ...item, state: "automatic" as const })),
    };
  });
  return {
    posts,
    categories: source.categories.map((item) => ({ ...item, description: "", active: true })),
    tags: source.tags.map((item) => ({ ...item, kind: item.kind as TagKind, aliases: [] })),
    // A little already set up, so the sidebar shows what lists are for.
    lists: [
      { id: 1, name: "Favorites", kind: "favorites", icon: "star", color: "yellow", pinnedAt: null },
      { id: 2, name: "Read this weekend", kind: "custom", icon: "book-open", color: "blue", pinnedAt: null },
      { id: 3, name: "Career ideas", kind: "custom", icon: "briefcase", color: "green", pinnedAt: null },
    ],
    items: [],
    collections: [],
    settings: { paused: false, backup: true, language: "English", meaning: true },
    lastIds: { list: 3, collection: 0, category: source.categories.length, tag: source.tags.length },
  };
}

let state: State = load();

function load(): State {
  try {
    for (const key of Object.keys(localStorage)) {
      if (key.startsWith("sortwise-demo:") && key !== STORAGE) localStorage.removeItem(key);
    }
    const saved = localStorage.getItem(STORAGE);
    const parsed = saved ? (JSON.parse(saved) as State) : null;
    if (parsed && Array.isArray(parsed.posts) && Array.isArray(parsed.lists) && parsed.settings && parsed.lastIds) return parsed;
  } catch {
    /* storage unavailable or damaged: start fresh */
  }
  const fresh = initial();
  seed(fresh);
  return fresh;
}

/* Starts the lists off with a few posts, chosen by topic. */
function seed(fresh: State) {
  const byCategory = (name: string) => {
    const id = fresh.categories.find((item) => item.name === name)?.id;
    return fresh.posts.filter((post) => post.categories.some((item) => item.id === id)).map((post) => post.id);
  };
  const now = Date.now();
  const add = (listId: number, ids: number[]) => ids.forEach((bookmarkId, index) => fresh.items.push({ listId, bookmarkId, addedAt: now - (index + 1) * 3600_000 }));
  add(1, [...byCategory("AI").slice(0, 3), ...byCategory("Design").slice(0, 1), ...byCategory("Backend").slice(0, 2)]);
  add(2, [...byCategory("Learning").slice(0, 3), ...byCategory("Research").slice(0, 2)]);
  add(3, byCategory("Career").slice(0, 4));
}

function save() {
  try {
    localStorage.setItem(STORAGE, JSON.stringify(state));
  } catch {
    /* storage full or unavailable: changes last until the page reloads */
  }
}

/* Puts the demo back as it was first opened. */
export function resetDemo() {
  try {
    localStorage.removeItem(STORAGE);
  } catch {
    /* nothing saved */
  }
  state = load();
}

/* ---------- Meaning ---------- */

let vectors: Vectors | null = null;
let hubs: Map<number, number> | null = null;

function meaningHubs() {
  if (!hubs) hubs = hubness(meaningVectors());
  return hubs;
}

function meaningVectors(): Vectors {
  if (!vectors) {
    vectors = new Map();
    for (const post of source.posts) {
      const bytes = Uint8Array.from(atob(post.vector), (char) => char.charCodeAt(0));
      vectors.set(post.id, new Int8Array(bytes.buffer));
    }
  }
  const live = new Set(state.posts.map((post) => post.id));
  return new Map([...vectors].filter(([id]) => live.has(id)));
}

/* ---------- Views ---------- */

const visible = (item: Assignment) => item.state !== "removed" && (item.state === "added" || item.confidence !== null);

function view(post: Post): Bookmark {
  const name = (list: { id: number; name: string }[], id: number) => list.find((item) => item.id === id)?.name || "";
  return {
    id: post.id,
    postId: post.postId,
    author: post.author,
    username: post.username,
    text: post.text,
    url: post.url,
    postedAt: post.postedAt,
    importedAt: post.importedAt,
    language: post.language,
    visibleContext: post.visibleContext,
    media: post.media,
    summary: post.manualSummary ?? post.aiSummary,
    mediaDescription: post.mediaDescription,
    processingStatus: "completed",
    archived: post.archived,
    authorAvatar: post.avatar || undefined,
    categories: post.categories
      .filter(visible)
      .map((item) => ({ id: item.id, name: name(state.categories, item.id), confidence: item.confidence ?? undefined, manual: item.state !== "automatic" }))
      .sort((a, b) => a.name.localeCompare(b.name)),
    tags: post.tags
      .filter(visible)
      .map((item) => ({ id: item.id, name: name(state.tags, item.id), confidence: item.confidence ?? undefined, manual: item.state !== "automatic" }))
      .sort((a, b) => a.name.localeCompare(b.name)),
    lists: state.items.filter((item) => item.bookmarkId === post.id).map((item) => item.listId).sort((a, b) => a - b),
  };
}

/* The text a search looks through, field by field, as the app's index does. */
function fields(post: Post) {
  const bookmark = view(post);
  const context = post.visibleContext;
  return {
    text: [post.text, context.quotedPost?.text, context.card?.title, context.card?.description].filter(Boolean).join(" \n "),
    summary: bookmark.summary,
    author: post.author,
    username: post.username,
    media: post.mediaDescription,
    taxonomy: [
      ...bookmark.categories.map((item) => item.name),
      ...bookmark.tags.flatMap((item) => [item.name, ...(state.tags.find((tag) => tag.id === item.id)?.aliases || [])]),
    ].join(", "),
  };
}

const searchable = (post: Post) => Object.values(fields(post)).join(" \n ");

const needsReview = (post: Post) => !post.categories.some((item) => item.state !== "removed" && (item.state === "added" || (item.confidence ?? 0) >= 0.5));

type Query = {
  text: string;
  meaning: boolean;
  status: string;
  sort: string;
  archived: boolean;
  categories: number[];
  tags: number[];
  author: string;
  list: string;
};

function readQuery(params: URLSearchParams): Query {
  const ids = (key: string) => params.getAll(key).flatMap((value) => value.split(",")).map(Number).filter((id) => id > 0);
  return {
    text: (params.get("q") || "").trim(),
    meaning: params.get("mode") === "meaning",
    status: params.get("status") || "",
    sort: params.get("sort") || "",
    archived: params.get("archived") === "true",
    categories: ids("category"),
    tags: ids("tag"),
    author: (params.get("author") || "").replace(/^@/, "").trim(),
    list: params.get("list") || "",
  };
}

/* The posts that pass every filter except the words searched for. */
function filtered(query: Query) {
  const listId = Number(query.list) || 0;
  const custom = new Set(state.lists.filter((item) => item.kind === "custom").map((item) => item.id));
  return state.posts.filter((post) => {
    if (!query.archived && post.archived) return false;
    if (query.author && post.username.toLowerCase() !== query.author.toLowerCase()) return false;
    if (listId && !state.items.some((item) => item.listId === listId && item.bookmarkId === post.id)) return false;
    if (query.list === "none" && state.items.some((item) => item.bookmarkId === post.id && custom.has(item.listId))) return false;
    if (query.status === "review" && needsReview(post) === false) return false;
    if (query.status && query.status !== "review" && query.status !== "completed") return false;
    if (query.categories.some((id) => !post.categories.some((item) => item.id === id && visible(item)))) return false;
    if (query.tags.some((id) => !post.tags.some((item) => item.id === id && visible(item)))) return false;
    return true;
  });
}

function order(posts: Post[], query: Query, listId: number) {
  const time = (value?: string) => (value ? parse(value) : null);
  const byImported = (a: Post, b: Post) => parse(b.importedAt) - parse(a.importedAt) || b.id - a.id;
  const sorted = [...posts];
  switch (query.sort) {
    case "posted_desc":
    case "posted_asc": {
      const sign = query.sort === "posted_desc" ? -1 : 1;
      return sorted.sort((a, b) => {
        const [x, y] = [time(a.postedAt), time(b.postedAt)];
        if (x === null || y === null) return x === y ? 0 : x === null ? 1 : -1;
        return sign * (x - y) || sign * (a.id - b.id);
      });
    }
    case "imported_asc":
      return sorted.sort((a, b) => -byImported(a, b));
    case "added_desc":
      if (listId) {
        const added = (post: Post) => state.items.find((item) => item.listId === listId && item.bookmarkId === post.id)?.addedAt || 0;
        return sorted.sort((a, b) => added(b) - added(a) || b.id - a.id);
      }
  }
  return sorted.sort(byImported);
}

const RANK = 60;

/* The library's search. With words, posts are found by them; asked to look
   by meaning, the posts closest in meaning to the best word matches join
   them, ranked together as the app ranks words and meaning. */
function search(query: Query) {
  const pool = filtered(query);
  const listId = Number(query.list) || 0;
  if (!terms(query.text).length) return { posts: order(pool, query, listId), byMeaning: new Set<number>(), notice: undefined as string | undefined, meaning: false };
  const words = rankWords(pool, query.text, searchable);
  const ranked = query.sort === "" || query.sort === "relevance";
  if (!query.meaning) return { posts: ranked ? words : order(words, query, listId), byMeaning: new Set<number>(), notice: undefined, meaning: false };
  if (!state.settings.meaning) return { posts: ranked ? words : order(words, query, listId), byMeaning: new Set<number>(), notice: "Search by meaning is turned off in Settings.", meaning: false };
  // The real app asks Gemini what the words mean. With no AI service here,
  // the demo takes its sense of them from the posts that use them.
  const examples = rankWords(state.posts.filter((post) => !post.archived), query.text, searchable).slice(0, 5);
  if (!examples.length) {
    return {
      posts: words,
      byMeaning: new Set<number>(),
      notice: "The demo can only widen searches that match some words. The app understands any words, using Gemini.",
      meaning: false,
    };
  }
  const all = meaningVectors();
  const centre = new Float32Array(768);
  for (const post of examples) {
    const vector = all.get(post.id);
    if (!vector) continue;
    let size = 0;
    for (const value of vector) size += value * value;
    size = Math.sqrt(size) || 1;
    vector.forEach((value, index) => (centre[index] += value / size));
  }
  const allowed = new Set(pool.map((post) => post.id));
  // The examples are the closest to their own centre; they are already found
  // by their words, and would set the bar for everything else too high.
  const close = nearest(all, centre, 50, new Set(examples.map((post) => post.id)), 0.1, meaningHubs()).filter((hit) => allowed.has(hit.id));
  const score = new Map<number, number>();
  words.forEach((post, index) => score.set(post.id, 1 / (RANK + index + 1)));
  const byMeaning = new Set<number>();
  close.forEach((hit, index) => {
    if (!score.has(hit.id)) byMeaning.add(hit.id);
    score.set(hit.id, (score.get(hit.id) || 0) + 1 / (RANK + index + 1));
  });
  const found = pool.filter((post) => score.has(post.id));
  // Equal scores: a post with the words comes before one found only by meaning.
  const posts = ranked ? found.sort((a, b) => score.get(b.id)! - score.get(a.id)! || Number(byMeaning.has(a.id)) - Number(byMeaning.has(b.id)) || b.id - a.id) : order(found, query, listId);
  return { posts, byMeaning, notice: undefined, meaning: true };
}

function postOr404(id: number) {
  const post = state.posts.find((item) => item.id === id);
  if (!post) throw new APIError(404, "Bookmark not found");
  return post;
}

function counts<T extends { id: number; name: string }>(items: T[], key: "categories" | "tags", posts = state.posts.filter((post) => !post.archived)) {
  return items
    .map((item) => ({ id: item.id, name: item.name, count: posts.filter((post) => post[key].some((entry) => entry.id === item.id && visible(entry))).length }))
    .filter((item) => item.count > 0)
    .sort((a, b) => b.count - a.count || a.name.localeCompare(b.name));
}

function lists(): List[] {
  const live = new Set(state.posts.filter((post) => !post.archived).map((post) => post.id));
  return [...state.lists]
    .sort((a, b) => (a.kind === "favorites" ? -1 : b.kind === "favorites" ? 1 : 0) || (a.pinnedAt === null ? 1 : 0) - (b.pinnedAt === null ? 1 : 0) || (a.pinnedAt || 0) - (b.pinnedAt || 0) || a.name.localeCompare(b.name))
    .map((item) => ({ id: item.id, name: item.name, kind: item.kind, icon: item.icon, color: item.color, pinned: item.pinnedAt !== null, count: state.items.filter((entry) => entry.listId === item.id && live.has(entry.bookmarkId)).length }));
}

function authors(posts = state.posts.filter((post) => !post.archived)) {
  const groups = new Map<string, Post[]>();
  for (const post of posts) {
    const key = post.username.toLowerCase();
    groups.set(key, [...(groups.get(key) || []), post]);
  }
  return [...groups.values()].map((group) => {
    const latest = [...group].sort((a, b) => parse(b.importedAt) - parse(a.importedAt))[0];
    return { username: latest.username, name: latest.author, count: group.length, latestSavedAt: latest.importedAt, avatar: latest.avatar || undefined, posts: group };
  });
}

/* ---------- Saved searches ---------- */

const STATUSES = ["", "pending", "processing", "completed", "failed", "blocked", "review"];
const SORTS = ["", "relevance", "imported_desc", "imported_asc", "posted_desc", "posted_asc"];

function cleanQuery(input: Partial<CollectionQuery>): CollectionQuery {
  const ids = (values?: number[]) => [...new Set((values || []).filter((id) => id > 0))];
  const q = (input.q || "").trim().replace(/\s+/g, " ");
  const query: CollectionQuery = {
    q,
    mode: input.mode === "meaning" && q ? "meaning" : "",
    categories: ids(input.categories),
    tags: ids(input.tags),
    status: input.status || "",
    list: input.list || "",
    author: (input.author || "").replace(/^@/, "").trim(),
    sort: input.sort === "relevance" && !q ? "" : input.sort || "",
  };
  if (!STATUSES.includes(query.status)) throw new APIError(422, "unknown status");
  if (!SORTS.includes(query.sort)) throw new APIError(422, "unknown sort order");
  if (query.list && query.list !== "none" && !(Number(query.list) > 0)) throw new APIError(422, "unknown list");
  if (query.author && !/^[A-Za-z0-9_]{1,30}$/.test(query.author)) throw new APIError(422, "unknown author");
  if (!query.q && !query.categories.length && !query.tags.length && !query.status && !query.list && !query.author) throw new APIError(422, "add a search or a filter first");
  return query;
}

/* As the server does when a category or tag is merged (to the other one)
   or a tag deleted (to 0): saved searches follow, and one left with no
   search or filter at all goes. */
function repoint(kind: "categories" | "tags", from: number, to: number) {
  state.collections = state.collections.filter((collection) => {
    const ids = collection.query[kind];
    if (!ids.includes(from)) return true;
    collection.query[kind] = [...new Set(ids.map((id) => (id === from ? to : id)).filter((id) => id > 0))];
    const query = collection.query;
    return Boolean(query.q || query.categories.length || query.tags.length || query.status || query.list || query.author);
  });
}

function collectionCount(query: CollectionQuery) {
  const params = new URLSearchParams();
  if (query.q) params.set("q", query.q);
  if (query.mode) params.set("mode", query.mode);
  if (query.status) params.set("status", query.status);
  if (query.list) params.set("list", query.list);
  if (query.author) params.set("author", query.author);
  query.categories.forEach((id) => params.append("category", String(id)));
  query.tags.forEach((id) => params.append("tag", String(id)));
  return search(readQuery(params)).posts.length;
}

const collectionView = (item: State["collections"][number]): Collection => ({ ...item, count: collectionCount(item.query) });

function uniqueName(name: string, taken: string[], what: string) {
  const value = name.trim();
  if (!value || value.length > 80) throw new APIError(422, "name must contain 1–80 useful characters");
  if (taken.some((other) => other.toLowerCase() === value.toLowerCase())) throw new APIError(409, `You already have ${what} with this name`);
  return value;
}

/* ---------- Requests ---------- */

const unavailable = (message: string) => new APIError(403, message);

type Body = Record<string, unknown>;

function route(method: string, path: string, params: URLSearchParams, body: Body): unknown {
  const parts = path.split("/").filter(Boolean);
  const id = Number(parts[1]);
  const [resource] = parts;

  if (resource === "setup") return { bookmarks: state.posts.length, pendingJobs: 0, aiConfigured: true };
  if (resource === "health") return { status: "ok", version: "demo" };

  if (resource === "bookmarks") {
    if (parts.length === 1 && method === "GET") {
      const query = readQuery(params);
      const page = Math.max(1, Number(params.get("page")) || 1);
      const pageSize = Math.min(100, Math.max(1, Number(params.get("pageSize")) || 24));
      const result = search(query);
      return {
        items: result.posts.slice((page - 1) * pageSize, page * pageSize).map((post) => ({ ...view(post), ...(result.byMeaning.has(post.id) ? { match: "meaning" } : {}) })),
        page,
        pageSize,
        total: result.posts.length,
        ...(query.meaning && query.text ? { search: { meaning: result.meaning, notice: result.notice } } : {}),
      };
    }
    const post = postOr404(id);
    if (parts[2] === "related") {
      if (!state.settings.meaning) return { items: [] };
      const all = meaningVectors();
      const own = all.get(post.id);
      if (!own) return { items: [] };
      const archived = new Set(state.posts.filter((item) => item.archived).map((item) => item.id));
      const hits = nearest(all, Float32Array.from(own), 10, new Set([post.id]), 0.08, meaningHubs()).filter((hit) => !archived.has(hit.id)).slice(0, 5);
      return {
        items: hits.map((hit) => {
          const other = postOr404(hit.id);
          return { id: other.id, author: other.author, username: other.username, avatar: other.avatar || undefined, postedAt: other.postedAt, text: other.text || other.visibleContext.card?.title || other.mediaDescription };
        }),
      };
    }
    if (parts[2] === "reprocess") throw unavailable("Posts are analyzed again in the app you download. The demo's posts were analyzed in advance.");
    if (method === "GET") return view(post);
    if (method === "DELETE") {
      state.posts = state.posts.filter((item) => item.id !== post.id);
      state.items = state.items.filter((item) => item.bookmarkId !== post.id);
      save();
      return {};
    }
    if (method === "PATCH") {
      if (typeof body.archived === "boolean") post.archived = body.archived;
      if (body.resetSummary) post.manualSummary = null;
      else if (typeof body.summary === "string") post.manualSummary = body.summary.trim().slice(0, 800);
      const decide = (key: "categories" | "tags", decisions: unknown) => {
        for (const decision of (decisions as { id: number; state: Assignment["state"] }[]) || []) {
          const existing = post[key].find((item) => item.id === decision.id);
          if (existing) existing.state = decision.state;
          else post[key].push({ id: decision.id, confidence: null, state: decision.state });
        }
      };
      decide("categories", body.categoryDecisions);
      decide("tags", body.tagDecisions);
      save();
      return view(post);
    }
  }

  if (resource === "search" && parts[1] === "quick") {
    const text = (params.get("q") || "").trim();
    const limit = Math.min(20, Math.max(1, Number(params.get("limit")) || 8));
    const pool = state.posts.filter((post) => !post.archived);
    const found = terms(text).length ? rankWords(pool, text, searchable) : [];
    const people = authors()
      .filter((person) => {
        const needle = text.toLowerCase().replace(/^@/, "");
        return Boolean(needle) && (person.name.toLowerCase().includes(needle) || person.username.toLowerCase().includes(needle) || matches(`${person.name} ${person.username}`, text));
      })
      .sort((a, b) => b.count - a.count)
      .slice(0, 4)
      .map((person) => ({ username: person.username, name: person.name, count: person.count, latestSavedAt: person.latestSavedAt, avatar: person.avatar }));
    return {
      total: found.length,
      authors: people,
      bookmarks: found.slice(0, limit).map((post) => {
        const field = fields(post);
        const marked = (value: string, words: number) => {
          const excerpt = snippet(value, text, words);
          return excerpt.includes(MATCH_START) ? excerpt : undefined;
        };
        return {
          id: post.id,
          author: post.author,
          username: post.username,
          avatar: post.avatar || undefined,
          postedAt: post.postedAt,
          importedAt: post.importedAt,
          text: snippet(field.text, text, 16),
          summary: marked(field.summary, 14),
          media: marked(field.media, 14),
          taxonomy: marked(field.taxonomy, 8),
          authorMatch: marked(field.author, 8),
          usernameMatch: marked(field.username, 4),
        };
      }),
    };
  }

  if (resource === "categories") {
    if (parts.length === 1 && method === "GET") {
      return {
        items: state.categories
          .map((item): Category => ({ ...item, count: state.posts.filter((post) => post.categories.some((entry) => entry.id === item.id && visible(entry))).length }))
          .sort((a, b) => a.name.localeCompare(b.name)),
      };
    }
    if (parts.length === 1 && method === "POST") {
      const name = uniqueName(String(body.name || ""), state.categories.map((item) => item.name), "a category");
      const created = { id: ++state.lastIds.category, name, description: String(body.description || ""), parentId: (body.parentId as number) || undefined, active: true };
      state.categories.push(created);
      save();
      return { ...created, count: 0 };
    }
    const category = state.categories.find((item) => item.id === id);
    if (!category) throw new APIError(404, "Category not found");
    if (parts[2] === "merge") {
      const target = Number(body.targetId);
      if (!state.categories.some((item) => item.id === target) || target === id) throw new APIError(422, "Choose another category to merge into");
      for (const post of state.posts) {
        const from = post.categories.find((item) => item.id === id);
        if (!from) continue;
        post.categories = post.categories.filter((item) => item.id !== id);
        if (!post.categories.some((item) => item.id === target)) post.categories.push({ ...from, id: target });
      }
      for (const child of state.categories) if (child.parentId === id) child.parentId = target;
      state.categories = state.categories.filter((item) => item.id !== id);
      repoint("categories", id, target);
      save();
      return {};
    }
    if (method === "DELETE") category.active = false;
    if (method === "PATCH") {
      if (body.name !== undefined) category.name = uniqueName(String(body.name), state.categories.filter((item) => item.id !== id).map((item) => item.name), "a category");
      if (body.description !== undefined) category.description = String(body.description);
      if (typeof body.active === "boolean") category.active = body.active;
      const parent = Number(body.parentId) || undefined;
      for (let step = parent; step; step = state.categories.find((item) => item.id === step)?.parentId) {
        if (step === id) throw new APIError(422, "a category cannot be inside itself");
        if (!state.categories.some((item) => item.id === step)) throw new APIError(422, "parent category not found");
      }
      category.parentId = parent;
    }
    save();
    return { ...category, count: state.posts.filter((post) => post.categories.some((entry) => entry.id === id && visible(entry))).length };
  }

  if (resource === "tags") {
    const tagView = (item: State["tags"][number]): Tag => ({ ...item, count: state.posts.filter((post) => post.tags.some((entry) => entry.id === item.id && visible(entry))).length });
    if (parts.length === 1 && method === "GET") return { items: state.tags.map(tagView).sort((a, b) => b.count - a.count || a.name.localeCompare(b.name)) };
    if (parts.length === 1 && method === "POST") {
      const created = { id: ++state.lastIds.tag, name: uniqueName(String(body.name || ""), state.tags.map((item) => item.name), "a tag"), kind: (body.kind as TagKind) || "topic", aliases: [] };
      state.tags.push(created);
      save();
      return tagView(created);
    }
    const tag = state.tags.find((item) => item.id === id);
    if (!tag) throw new APIError(404, "Tag not found");
    if (parts[2] === "aliases") {
      if (method === "POST") tag.aliases = [...new Set([...tag.aliases, String(body.alias || "").trim()].filter(Boolean))];
      else tag.aliases = tag.aliases.filter((alias) => alias !== params.get("alias"));
      save();
      return tagView(tag);
    }
    if (parts[2] === "merge") {
      const target = state.tags.find((item) => item.id === Number(body.targetId));
      if (!target || target.id === id) throw new APIError(422, "Choose another tag to merge into");
      for (const post of state.posts) {
        const from = post.tags.find((item) => item.id === id);
        if (!from) continue;
        post.tags = post.tags.filter((item) => item.id !== id);
        if (!post.tags.some((item) => item.id === target.id)) post.tags.push({ ...from, id: target.id });
      }
      target.aliases = [...new Set([...target.aliases, tag.name, ...tag.aliases])];
      state.tags = state.tags.filter((item) => item.id !== id);
      repoint("tags", id, target.id);
      save();
      return {};
    }
    if (method === "DELETE") {
      if (state.posts.some((post) => post.tags.some((entry) => entry.id === id))) throw new APIError(409, "tag is in use; merge it instead");
      state.tags = state.tags.filter((item) => item.id !== id);
      repoint("tags", id, 0);
      save();
      return {};
    }
    if (method === "PATCH") {
      const renamed = uniqueName(String(body.name || ""), state.tags.filter((item) => item.id !== id).map((item) => item.name), "a tag");
      if (renamed.toLowerCase() !== tag.name.toLowerCase()) tag.aliases = [...new Set([...tag.aliases, tag.name])];
      tag.name = renamed;
      tag.kind = (body.kind as TagKind) || tag.kind;
      save();
      return tagView(tag);
    }
  }

  if (resource === "authors") {
    if (parts.length === 1) {
      const text = (params.get("q") || "").trim().toLowerCase().replace(/^@/, "");
      const sort = params.get("sort") || "count";
      const page = Math.max(1, Number(params.get("page")) || 1);
      const pageSize = Math.min(200, Math.max(1, Number(params.get("pageSize")) || 100));
      const all = authors()
        .filter((person) => !text || person.name.toLowerCase().includes(text) || person.username.toLowerCase().includes(text))
        .sort((a, b) =>
          sort === "name"
            ? a.name.localeCompare(b.name)
            : sort === "recent"
              ? parse(b.latestSavedAt) - parse(a.latestSavedAt)
              : b.count - a.count || a.name.localeCompare(b.name),
        );
      return {
        items: all.slice((page - 1) * pageSize, page * pageSize).map((person) => ({ username: person.username, name: person.name, count: person.count, latestSavedAt: person.latestSavedAt, avatar: person.avatar })),
        page,
        pageSize,
        total: all.length,
      };
    }
    const handle = decodeURIComponent(parts[1]).replace(/^@/, "").toLowerCase();
    const person = authors().find((item) => item.username.toLowerCase() === handle);
    if (!person) throw new APIError(404, "Author not found");
    const dates = person.posts.map((post) => post.postedAt).filter((value): value is string => Boolean(value)).sort((a, b) => parse(a) - parse(b));
    return {
      username: person.username,
      name: person.name,
      count: person.count,
      latestSavedAt: person.latestSavedAt,
      avatar: person.avatar,
      firstPostedAt: dates[0] || "",
      latestPostedAt: dates[dates.length - 1] || "",
      categories: counts(state.categories.filter((item) => item.active), "categories", person.posts).slice(0, 6),
      topTags: counts(state.tags, "tags", person.posts).slice(0, 6),
    };
  }

  if (resource === "lists") {
    if (parts.length === 1 && method === "GET") return { items: lists(), appearance: { icons: LIST_ICONS, colors: COLORS } };
    if (parts.length === 1 && method === "POST") {
      const name = uniqueName(String(body.name || ""), state.lists.map((item) => item.name), "a list");
      const created = { id: ++state.lastIds.list, name, kind: "custom" as const, icon: "list", color: COLORS[(state.lists.length - 1) % COLORS.length], pinnedAt: null };
      state.lists.push(created);
      save();
      return lists().find((item) => item.id === created.id);
    }
    const list = state.lists.find((item) => item.id === id);
    if (!list) throw new APIError(404, "List or bookmark not found");
    if (parts[2] === "bookmarks") {
      const ids = parts[3] ? [Number(parts[3])] : ((body.bookmarkIds as number[]) || []);
      const inList = parts[3] ? method === "PUT" : Boolean(body.inList);
      const now = Date.now();
      ids.forEach((bookmarkId, index) => {
        const present = state.items.some((item) => item.listId === id && item.bookmarkId === bookmarkId);
        if (inList && !present && state.posts.some((post) => post.id === bookmarkId)) state.items.push({ listId: id, bookmarkId, addedAt: now + index });
        if (!inList) state.items = state.items.filter((item) => !(item.listId === id && item.bookmarkId === bookmarkId));
      });
      save();
      return {};
    }
    if (list.kind === "favorites" && method !== "GET") throw new APIError(409, "Favorites can't be renamed or deleted");
    if (method === "DELETE") {
      const bookmarkIds = state.items.filter((item) => item.listId === id).sort((a, b) => a.addedAt - b.addedAt).map((item) => item.bookmarkId);
      state.lists = state.lists.filter((item) => item.id !== id);
      state.items = state.items.filter((item) => item.listId !== id);
      save();
      return { bookmarkIds };
    }
    if (method === "PATCH") {
      if (body.name) list.name = uniqueName(String(body.name), state.lists.filter((item) => item.id !== id).map((item) => item.name), "a list");
      if (body.icon && LIST_ICONS.includes(String(body.icon))) list.icon = String(body.icon);
      if (body.color && COLORS.includes(body.color as OptionColor)) list.color = body.color as OptionColor;
      if (typeof body.pinned === "boolean") list.pinnedAt = body.pinned ? list.pinnedAt || Date.now() : null;
      save();
      return lists().find((item) => item.id === id);
    }
  }

  if (resource === "collections") {
    if (parts.length === 1 && method === "GET") return { items: [...state.collections].sort((a, b) => a.name.localeCompare(b.name)).map(collectionView), colors: COLORS };
    if (parts.length === 1 && method === "POST") {
      const name = uniqueName(String(body.name || ""), state.collections.map((item) => item.name), "a smart collection");
      const created = { id: ++state.lastIds.collection, name, color: COLORS[(state.collections.length + 1) % COLORS.length], query: cleanQuery(body.query as CollectionQuery) };
      state.collections.push(created);
      save();
      return collectionView(created);
    }
    const collection = state.collections.find((item) => item.id === id);
    if (!collection) throw new APIError(404, "Smart collection not found");
    if (method === "DELETE") {
      state.collections = state.collections.filter((item) => item.id !== id);
      save();
      return {};
    }
    if (method === "PATCH") {
      const name = body.name !== undefined ? uniqueName(String(body.name), state.collections.filter((item) => item.id !== id).map((item) => item.name), "a smart collection") : collection.name;
      const color = body.color !== undefined ? (body.color as OptionColor) : collection.color;
      if (!COLORS.includes(color)) throw new APIError(422, "unknown color");
      const query = body.query !== undefined ? cleanQuery(body.query as CollectionQuery) : collection.query;
      Object.assign(collection, { name, color, query });
      save();
      return collectionView(collection);
    }
  }

  if (resource === "dashboard") {
    const live = state.posts.filter((post) => !post.archived);
    return {
      total: live.length,
      completed: live.length,
      pending: 0,
      failed: 0,
      needsReview: live.filter(needsReview).length,
      categories: counts(state.categories.filter((item) => item.active), "categories").slice(0, 12),
      topTags: counts(state.tags, "tags").slice(0, 16),
    };
  }

  if (resource === "settings" && parts[1] === "ai") {
    if (parts[2] === "keys") throw unavailable("You add your own API keys in the app you download. The demo needs none.");
    if (parts[2] === "test") throw unavailable("You test your own API keys in the app you download. The demo needs none.");
    if (method === "PATCH") {
      if (typeof body.paused === "boolean") state.settings.paused = body.paused;
      if (typeof body.backup === "boolean") state.settings.backup = body.backup;
      if (typeof body.meaning === "boolean") state.settings.meaning = body.meaning;
      if (typeof body.language === "string") state.settings.language = body.language;
      save();
    }
    const settings: AISettings = {
      provider: "gemini",
      backup: state.settings.backup,
      paused: state.settings.paused,
      language: state.settings.language,
      providers: [
        { id: "gemini", name: "Gemini", configured: true, source: "demo", model: "gemini-3.8-flash", keyUrl: "https://aistudio.google.com/apikey" },
        { id: "groq", name: "Groq", configured: false, source: "none", model: "qwen/qwen3.8-27b", keyUrl: "https://console.groq.com/keys" },
      ],
      active: "gemini",
      resting: [],
      meaning: { enabled: state.settings.meaning, available: true, indexed: state.posts.length, total: state.posts.length },
    };
    return settings;
  }

  if (resource === "jobs") {
    if (parts[2] === "retry") return {};
    const items = [...state.posts]
      .sort((a, b) => parse(b.importedAt) - parse(a.importedAt))
      .slice(0, 200)
      .map((post) => ({ id: post.id, bookmarkId: post.id, postId: post.postId, author: post.author, status: "completed", attempts: 1, maxAttempts: 5, lastError: "", updatedAt: post.importedAt }));
    const filter = (params.get("status") || "").split(",").filter(Boolean);
    return {
      items: filter.length && !filter.includes("completed") ? [] : items,
      counts: { pending: 0, processing: 0, completed: state.posts.length, failed: 0, blocked: 0, total: state.posts.length },
    };
  }

  if (resource === "taxonomy") {
    if (method === "GET") return { items: [] };
    throw unavailable("The AI suggests categories and merges in the app you download, with your own key.");
  }
  if (resource === "extension") {
    if (parts[1] === "connections") return method === "GET" ? { items: [] } : {};
    throw unavailable("You pair the browser extension with the app you download. The demo's posts are already saved.");
  }
  if (resource === "backups") throw unavailable("Backups are made in the app you download. The demo keeps everything in this browser.");
  throw new APIError(404, "API route not found");
}

/* Answers one request the way the server would, after a short pause so
   loading states look as they do in the app. */
export async function demoApi<T>(path: string, init?: RequestInit): Promise<T> {
  const url = new URL(path, "http://demo.local");
  const method = (init?.method || "GET").toUpperCase();
  let body: Body = {};
  if (typeof init?.body === "string" && init.body) {
    try {
      body = JSON.parse(init.body) as Body;
    } catch {
      throw new APIError(400, "request body must be JSON");
    }
  }
  await new Promise((resolve) => setTimeout(resolve, method === "GET" ? 60 : 120));
  return route(method, url.pathname, url.searchParams, body) as T;
}

/* The library as an export file, as the app's Settings page offers it. */
export function exportLibrary(kind: "json" | "csv") {
  const posts = state.posts.map(view);
  if (kind === "json") return new Blob([JSON.stringify({ bookmarks: posts }, null, 2)], { type: "application/json" });
  const cell = (value: unknown) => `"${String(value ?? "").replace(/"/g, '""')}"`;
  const rows = [
    ["id", "author", "username", "text", "url", "posted_at", "saved_at", "summary", "categories", "tags"],
    ...posts.map((post) => [post.postId, post.author, post.username, post.text, post.url, post.postedAt, post.importedAt, post.summary, post.categories.map((item) => item.name).join("; "), post.tags.map((item) => item.name).join("; ")]),
  ];
  return new Blob([rows.map((row) => row.map(cell).join(",")).join("\n")], { type: "text/csv" });
}
