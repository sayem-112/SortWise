import { readFileSync } from "node:fs";
import vm from "node:vm";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const read = (name) => readFileSync(new URL(`./${name}`, import.meta.url), "utf8");
const TEMPLATE = { url: "https://x.com/i/api/graphql/q1/Bookmarks?variables=%7B%22count%22%3A20%7D&features=%7B%7D", authorization: "Bearer public" };

function page(ids, cursor) {
  return {
    data: {
      bookmark_timeline_v2: {
        timeline: {
          instructions: [
            {
              entries: [
                ...ids.map((id) => ({ content: { itemContent: { __typename: "TimelineTweet", tweet_results: { result: { rest_id: id } } } } })),
                ...(cursor ? [{ content: { cursorType: "Bottom", value: cursor } }] : []),
              ],
            },
          ],
        },
      },
    },
  };
}

// Loads service-worker.js with a fake chrome API, an open X tab that answers
// page requests from `xPages`, and a local app that answers from `app`.
function loadWorker({ stored = {}, xPages = {}, app = () => ({ status: 201, body: {} }) } = {}) {
  const storage = { extensionToken: "token", xTemplate: TEMPLATE, ...stored };
  const listeners = [];
  const tabMessages = [];
  const appCalls = [];
  const chrome = {
    storage: {
      local: {
        get: async (keys) => Object.fromEntries([keys].flat().filter((key) => key in storage).map((key) => [key, storage[key]])),
        set: async (values) => void Object.assign(storage, values),
        remove: async (keys) => void [keys].flat().forEach((key) => delete storage[key]),
      },
    },
    runtime: {
      sendMessage: async () => {},
      getPlatformInfo: async () => ({}),
      onMessage: { addListener: (fn) => listeners.push(fn) },
      onInstalled: { addListener() {} },
    },
    tabs: {
      query: async () => [{ id: 7, url: "https://x.com/home" }],
      sendMessage: async (tabId, message) => {
        tabMessages.push(message);
        if (message.type === "bw-ping") return { ok: true };
        if (message.type === "fetch-page") {
          const cursor = JSON.parse(new URL(message.url).searchParams.get("variables")).cursor || "start";
          const reply = xPages[cursor];
          return typeof reply === "function" ? reply() : reply;
        }
        return null;
      },
      create: vi.fn(async () => ({ id: 99 })),
      remove: vi.fn(async () => {}),
      get: async (id) => ({ id, status: "complete" }),
      update: async () => {},
      reload: async () => {},
      onUpdated: { addListener() {}, removeListener() {} },
    },
    action: { setBadgeText() {}, setBadgeBackgroundColor() {} },
    scripting: { executeScript: async () => {} },
  };
  const fetch = async (url, init) => {
    const call = { path: url.replace("http://127.0.0.1:8787/api/v1", ""), body: init.body ? JSON.parse(init.body) : null };
    appCalls.push(call);
    const reply = app(call);
    if (reply.offline) throw new TypeError("Failed to fetch");
    return { ok: reply.status < 300, status: reply.status, text: async () => JSON.stringify(reply.body || {}) };
  };
  const context = vm.createContext({ chrome, fetch, console, URL, Date, JSON, Math, Number, String, Promise, setTimeout, clearTimeout, Object, Array });
  context.importScripts = (...names) => names.forEach((name) => vm.runInContext(read(name), context));
  context.globalThis = context;
  vm.runInContext(read("service-worker.js"), context);
  const send = (message, sender = {}) =>
    new Promise((resolve) => {
      const async = listeners[0](message, sender, resolve);
      if (!async) resolve(undefined);
    });
  return { storage, tabMessages, appCalls, send, chrome };
}

async function settle(storage) {
  for (let i = 0; i < 200 && storage.importState?.running !== false; i++) await vi.advanceTimersByTimeAsync(1000);
}

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

const ids = (from) => Array.from({ length: 20 }, (_, i) => String(from + i));

describe("service worker sync", () => {
  it("pages through bookmarks in an open X tab and stops at already-saved posts", async () => {
    const { storage, tabMessages, appCalls, send, chrome } = loadWorker({
      xPages: { start: { status: 200, data: page(ids(100), "C1") }, C1: { status: 200, data: page(ids(200), "C2") }, C2: { status: 200, data: page(ids(300), "C3") } },
      app: (call) => ({ status: 201, body: call.path === "/imports/x" ? { received: 20, unchanged: 20 } : {} }),
    });
    const started = await send({ type: "start-import", mode: "new" });
    expect(started.ok).toBe(true);
    await settle(storage);

    const pages = tabMessages.filter((m) => m.type === "fetch-page").map((m) => JSON.parse(new URL(m.url).searchParams.get("variables")).cursor || "start");
    expect(pages).toEqual(["start", "C1"]);
    expect(appCalls.filter((c) => c.path === "/imports/x").every((c) => c.body.source === "sync" && c.body.timeline)).toBe(true);
    expect(storage.importState).toMatchObject({ phase: "complete", discovered: 40, unchanged: 40 });
    expect(storage.importState.status).toContain("up to date");
    expect(storage.lastSyncAt).toBeGreaterThan(0);
    expect(chrome.tabs.create).not.toHaveBeenCalled();
  });

  it("waits when X rate-limits, and a full sync walks to the end and resumes cleanly", async () => {
    let limited = true;
    const { storage, send } = loadWorker({
      xPages: {
        start: () => (limited ? ((limited = false), { status: 429, rateLimit: {} }) : { status: 200, data: page(ids(100), "C1") }),
        C1: { status: 200, data: page(ids(200)) },
      },
      app: () => ({ status: 201, body: { received: 20, inserted: 20 } }),
    });
    await send({ type: "start-import", mode: "full" });
    await vi.advanceTimersByTimeAsync(1000);
    expect(storage.importState.status).toContain("X asked to slow down");
    await settle(storage);
    expect(storage.importState).toMatchObject({ phase: "complete", inserted: 40 });
    expect(storage.resumeCursor).toBeUndefined();
  });

  it("keeps the full-sync position when the app stops responding", async () => {
    const { storage, send } = loadWorker({
      xPages: { start: { status: 200, data: page(ids(100), "C1") }, C1: { status: 200, data: page(ids(200), "C2") } },
      app: (call) => (call.body?.timeline?.data && JSON.stringify(call.body).includes('"200"') ? { offline: true } : { status: 201, body: { received: 20, inserted: 20 } }),
    });
    await send({ type: "start-import", mode: "full" });
    await settle(storage);
    expect(storage.importState.phase).toBe("error");
    expect(storage.importState.status).toContain("stopped responding");
    expect(storage.resumeCursor).toBe("C1");
  });
});

describe("service worker automatic checks", () => {
  it("never walks the whole list when X opens, and stops at the from-now-on boundary", async () => {
    const xPages = {};
    for (let i = 0; i < 12; i++) xPages[i === 0 ? "start" : `C${i}`] = { status: 200, data: page(ids(1000 + i * 20), `C${i + 1}`) };
    const { storage, tabMessages, send } = loadWorker({
      xPages,
      app: (call) => ({ status: 201, body: call.path === "/extension/summary" ? { scope: "all" } : { received: 20, inserted: 20 } }),
    });
    await send({ type: "x-ready" }, { tab: { id: 7 } });
    await settle(storage);
    expect(tabMessages.filter((m) => m.type === "fetch-page")).toHaveLength(5);
    expect(storage.importState).toMatchObject({ auto: true, mode: "new", phase: "complete", inserted: 100 });
  });

  it("does nothing automatic until the user has chosen what to save", async () => {
    const { tabMessages, send } = loadWorker({ app: () => ({ status: 200, body: { scope: "" } }) });
    await send({ type: "x-ready" }, { tab: { id: 7 } });
    await vi.advanceTimersByTimeAsync(5000);
    expect(tabMessages.filter((m) => m.type === "fetch-page")).toHaveLength(0);
  });

  it("stops a sync as soon as the app says it reached the starting point", async () => {
    const { storage, tabMessages, send } = loadWorker({
      xPages: { start: { status: 200, data: page(ids(100), "C1") }, C1: { status: 200, data: page(ids(200), "C2") } },
      app: () => ({ status: 201, body: { received: 3, inserted: 3, reachedBoundary: true } }),
    });
    await send({ type: "start-import", mode: "full" });
    await settle(storage);
    expect(tabMessages.filter((m) => m.type === "fetch-page")).toHaveLength(1);
    expect(storage.importState).toMatchObject({ phase: "complete", inserted: 3 });
  });
});

describe("service worker capture", () => {
  it("saves a bookmarked post, or keeps it for later when the app is closed", async () => {
    let online = true;
    const { storage, appCalls, send } = loadWorker({ app: () => (online ? { status: 201, body: { inserted: 1 } } : { offline: true }) });
    const tweet = { __typename: "Tweet", rest_id: "42" };
    expect(await send({ type: "x-bookmarked", tweetId: "42", tweet }, { tab: { id: 7 } })).toEqual({ saved: true });
    expect(appCalls.at(-1)).toMatchObject({ path: "/imports/x", body: { source: "capture", tweets: [tweet] } });

    online = false;
    expect(await send({ type: "x-bookmarked", tweetId: "43", tweet: { rest_id: "43" } }, { tab: { id: 7 } })).toEqual({ offline: true });
    expect(storage.pendingCaptures).toEqual([{ rest_id: "43" }]);

    await send({ type: "x-unbookmarked", tweetId: "42" });
    await vi.advanceTimersByTimeAsync(0);
    expect(storage.pendingRemovals).toEqual(["42"]);
  });

  it("does nothing when automatic saving is off", async () => {
    const { appCalls, send } = loadWorker({ stored: { autoSave: false } });
    expect(await send({ type: "x-bookmarked", tweetId: "42", tweet: { rest_id: "42" } }, { tab: { id: 7 } })).toEqual({ disabled: true });
    expect(appCalls).toHaveLength(0);
  });
});
