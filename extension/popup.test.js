import { readFileSync } from "node:fs";
import { JSDOM } from "jsdom";
import { afterEach, describe, expect, it, vi } from "vitest";
import "./popup-state.js";

const html = readFileSync(new URL("./popup.html", import.meta.url), "utf8");

afterEach(() => {
  delete globalThis.chrome;
  delete globalThis.document;
  delete globalThis.window;
  vi.resetModules();
});

// A fake chrome API: `responses` maps "METHOD path" (or a message type) to a reply.
function setup({ stored = {}, responses = {} }) {
  const dom = new JSDOM(html, { url: "chrome-extension://sortwise/popup.html" });
  globalThis.window = dom.window;
  globalThis.document = dom.window.document;
  const listeners = [];
  const sent = [];
  const storage = { ...stored };
  globalThis.chrome = {
    tabs: { create: vi.fn() },
    storage: {
      local: {
        get: vi.fn(async (keys) => Object.fromEntries([keys].flat().map((key) => [key, storage[key]]))),
        set: vi.fn(async (values) => Object.assign(storage, values)),
        remove: vi.fn(async (keys) => [keys].flat().forEach((key) => delete storage[key])),
      },
    },
    runtime: {
      sendMessage: vi.fn(async (message) => {
        sent.push(message);
        const key = message.type === "api" ? `${message.method || "GET"} ${message.path}` : message.type;
        const reply = responses[key];
        return typeof reply === "function" ? reply(message) : reply;
      }),
      onMessage: { addListener: (fn) => listeners.push(fn) },
    },
  };
  return { sent, storage, emit: (message) => listeners.forEach((fn) => fn(message)) };
}

const $ = (id) => document.getElementById(id);
const visible = (id) => !$(id).hidden;

describe("popup", () => {
  it("says when the local app is not running", async () => {
    setup({ responses: { "GET /health": { ok: false, status: 0, data: {} } } });
    await import("./popup.js");
    await vi.waitFor(() => expect(visible("offlinePanel")).toBe(true));
    expect($("connectionBadge").textContent).toBe("App offline");
  });

  it("asks to pair when no token is stored, and pairs with a code", async () => {
    const { storage } = setup({
      responses: {
        "GET /health": { ok: true, status: 200, data: { status: "ok" } },
        "POST /extension/pair": { ok: true, status: 201, data: { token: "new-token" } },
        "GET /extension/summary": { ok: true, status: 200, data: { bookmarks: 854, lastImportAt: null, scope: "all" } },
      },
    });
    await import("./popup.js");
    await vi.waitFor(() => expect(visible("pairingPanel")).toBe(true));
    $("pairingCode").value = "123456";
    $("pair").click();
    await vi.waitFor(() => expect(visible("readyPanel")).toBe(true));
    expect(storage.extensionToken).toBe("new-token");
    expect($("libraryCount").textContent).toBe("854");
    expect($("libraryLabel").textContent).toBe("bookmarks saved");
  });

  it("returns to pairing when the app has revoked the token", async () => {
    const { storage } = setup({
      stored: { extensionToken: "old" },
      responses: {
        "GET /health": { ok: true, status: 200, data: {} },
        "GET /extension/summary": { ok: false, status: 401, data: {} },
      },
    });
    await import("./popup.js");
    await vi.waitFor(() => expect(visible("pairingPanel")).toBe(true));
    expect(storage.extensionToken).toBeUndefined();
    expect($("message").textContent).toContain("disconnected");
  });

  it("starts a sync in the chosen mode, follows its progress, and saves the auto-save choice", async () => {
    const { sent, emit, storage } = setup({
      stored: { extensionToken: "token" },
      responses: {
        "GET /health": { ok: true, status: 200, data: {} },
        "GET /extension/summary": { ok: true, status: 200, data: { bookmarks: 10, lastImportAt: "2026-09-30 10:00:00", lastImportInserted: 3, scope: "all" } },
        "start-import": { ok: true, state: { running: true, phase: "syncing", status: "Connecting to X…" } },
      },
    });
    await import("./popup.js");
    await vi.waitFor(() => expect(visible("readyPanel")).toBe(true));
    expect($("autoSave").checked).toBe(true);
    $("autoSave").click();
    expect(storage.autoSave).toBe(false);
    document.querySelector('input[name="mode"][value="full"]').checked = true;
    $("start").click();
    await vi.waitFor(() => expect(visible("runningPanel")).toBe(true));
    expect(sent.find((message) => message.type === "start-import")).toMatchObject({ mode: "full" });

    emit({ type: "import-progress", running: true, phase: "syncing", status: "Checked 40 bookmarks, 3 new…", discovered: 40, inserted: 3, unchanged: 37 });
    expect($("inserted").textContent).toBe("3");
    expect($("runningStatus").textContent).toBe("Checked 40 bookmarks, 3 new…");

    emit({ type: "import-progress", running: false, phase: "complete", status: "Done: 3 new.", discovered: 40, inserted: 3, unchanged: 37 });
    await vi.waitFor(() => expect(visible("donePanel")).toBe(true));
    expect($("doneStatus").textContent).toBe("Done: 3 new.");
    expect($("doneTitle").textContent).toBe("Sync finished");
  });
});

describe("popup: what Sortwise saves", () => {
  it("asks before anything is synced, and only from now on marks a starting point", async () => {
    let scope = "";
    const { sent } = setup({
      stored: { extensionToken: "token" },
      responses: {
        "GET /health": { ok: true, status: 200, data: {} },
        "GET /extension/summary": () => ({ ok: true, status: 200, data: { bookmarks: 0, lastImportAt: null, scope } }),
        "set-scope": (message) => {
          scope = message.scope;
          return { ok: true, data: { scope } };
        },
      },
    });
    await import("./popup.js");
    await vi.waitFor(() => expect(visible("scopePanel")).toBe(true));
    expect($("saveScope").disabled).toBe(true);

    const fromNow = document.querySelector('input[name="scope"][value="new"]');
    fromNow.checked = true;
    fromNow.dispatchEvent(new window.Event("change"));
    $("saveScope").click();
    await vi.waitFor(() => expect(visible("readyPanel")).toBe(true));
    expect(sent.find((m) => m.type === "set-scope")).toMatchObject({ scope: "new" });
    expect(sent.find((m) => m.type === "start-import")).toBeUndefined();
    expect($("scopeLabel").textContent).toBe("only bookmarks from now on");
  });
});
