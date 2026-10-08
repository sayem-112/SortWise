importScripts("sync-core.js");

const {
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
} = SortwiseSync;

const DEFAULT_API_BASE = "http://127.0.0.1:8787/api/v1";
const BOOKMARKS_URL = "https://x.com/i/bookmarks";
// Opening X checks for new bookmarks at most this often.
const AUTO_SYNC_INTERVAL_MS = 60 * 60_000;
const TEMPLATE_TIMEOUT_MS = 25_000;
// The automatic check when X opens reads at most this many pages (about 20
// posts each); catching up on a large backlog is the user's explicit choice.
const AUTO_SYNC_MAX_PAGES = 5;
const PENDING_LIMIT = 200;
const BOTTOM_WAIT_STEPS = 8;
const SCROLL_STEP_MS = 1200;

// ---------------------------------------------------------------------------
// Local app

async function apiRequest(message) {
  const { extensionToken, apiBase } = await chrome.storage.local.get(["extensionToken", "apiBase"]);
  try {
    const response = await fetch(`${apiBase || DEFAULT_API_BASE}${message.path}`, {
      method: message.method || "GET",
      headers: {
        "Content-Type": "application/json",
        ...(extensionToken ? { Authorization: `Bearer ${extensionToken}` } : {}),
      },
      body: message.body ? JSON.stringify(message.body) : undefined,
    });
    const text = await response.text();
    let data = {};
    if (text) {
      try {
        data = JSON.parse(text);
      } catch {
        data = { error: { message: "The local app returned an unreadable response." } };
      }
    }
    return { ok: response.ok, status: response.status, data };
  } catch (error) {
    return { ok: false, status: 0, data: { error: { message: String(error) } } };
  }
}

function appError(response) {
  if (response?.status === 0) return new Error("Sortwise stopped responding. Start the app, then sync again.");
  if (response?.status === 401) return new Error("This extension is no longer paired. Pair it again from the popup.");
  return new Error(response?.data?.error?.message || "The local app could not save the bookmarks.");
}

// The signed-in X account (from X's twid cookie, via x-bridge.js), sent with
// every import so data from a second X account never mixes into the library.
let xAccount = "";
function noteAccount(account) {
  if (/^\d+$/.test(String(account || ""))) xAccount = String(account);
}

// What syncs bring in: "all", "new" (only from the moment it was chosen), or
// "" before the user has chosen. Read from the app, which enforces it.
async function importScope() {
  const response = await apiRequest({ path: "/extension/summary" });
  return response.ok ? response.data?.scope || "" : null;
}

async function settings() {
  const stored = await chrome.storage.local.get(["extensionToken", "autoSave", "xTemplate", "lastSyncAt"]);
  return { ...stored, autoSave: stored.autoSave !== false };
}

// ---------------------------------------------------------------------------
// Saving posts you bookmark or unbookmark on X, and anything queued while the app was closed

async function queuePending(key, value) {
  const stored = (await chrome.storage.local.get(key))[key] || [];
  stored.push(value);
  await chrome.storage.local.set({ [key]: stored.slice(-PENDING_LIMIT) });
}

async function flushPending() {
  const { pendingCaptures = [], pendingRemovals = [] } = await chrome.storage.local.get(["pendingCaptures", "pendingRemovals"]);
  if (pendingCaptures.length) {
    for (let start = 0; start < pendingCaptures.length; start += 50) {
      const response = await apiRequest({ path: "/imports/x", method: "POST", body: { source: "capture", tweets: pendingCaptures.slice(start, start + 50), account: xAccount } });
      if (!response.ok) return false;
    }
    await chrome.storage.local.remove("pendingCaptures");
  }
  if (pendingRemovals.length) {
    const response = await apiRequest({ path: "/imports/x/removed", method: "POST", body: { postIds: pendingRemovals } });
    if (!response.ok) return false;
    await chrome.storage.local.remove("pendingRemovals");
  }
  return true;
}

async function onBookmarked({ tweetId, tweet, account }, tabId) {
  noteAccount(account);
  const { extensionToken, autoSave } = await settings();
  if (!extensionToken || !autoSave) return { disabled: true };
  if (!tweet) {
    // The post was not on the page long enough to be seen; it is at the top of
    // the bookmarks list now, so a quick, limited check picks it up.
    if (await importScope()) setTimeout(() => void runSync("new", { tabId, quiet: true }), 3000);
    return { queued: true, tweetId };
  }
  const response = await apiRequest({ path: "/imports/x", method: "POST", body: { source: "capture", tweets: [tweet], account: xAccount } });
  if (response.ok) {
    void flushPending();
    flashBadge();
    return { saved: true };
  }
  if (response.status === 401) return { unpaired: true };
  if (response.status === 409) return { otherAccount: true };
  await queuePending("pendingCaptures", tweet);
  return { offline: true };
}

async function onUnbookmarked({ tweetId }) {
  const { extensionToken, autoSave } = await settings();
  if (!extensionToken || !autoSave || !/^\d+$/.test(String(tweetId))) return;
  const response = await apiRequest({ path: "/imports/x/removed", method: "POST", body: { postIds: [String(tweetId)] } });
  if (!response.ok && response.status !== 401) await queuePending("pendingRemovals", String(tweetId));
}

// ---------------------------------------------------------------------------
// The Bookmarks request template: learned from X's own request, never guessed

let templateWaiters = [];

async function saveTemplate(url, authorization) {
  const template = isBookmarksRequest(url) && /^Bearer /.test(authorization || "") ? { url: stripCursor(url), authorization } : null;
  if (!template?.url) return;
  await chrome.storage.local.set({ xTemplate: template });
  const waiters = templateWaiters;
  templateWaiters = [];
  waiters.forEach((resolve) => resolve(template));
}

function waitForTemplate() {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      templateWaiters = templateWaiters.filter((item) => item !== done);
      reject(new Error("X did not load your bookmarks. Make sure you're signed in to X in this browser."));
    }, TEMPLATE_TIMEOUT_MS);
    function done(template) {
      clearTimeout(timer);
      resolve(template);
    }
    templateWaiters.push(done);
  });
}

// ---------------------------------------------------------------------------
// Tabs

function waitForTabLoad(tabId, timeoutMs = 20000) {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      chrome.tabs.onUpdated.removeListener(listener);
      reject(new Error("X took too long to load. Check your connection and try again."));
    }, timeoutMs);
    function listener(id, info) {
      if (id === tabId && info.status === "complete") {
        clearTimeout(timer);
        chrome.tabs.onUpdated.removeListener(listener);
        resolve();
      }
    }
    chrome.tabs.onUpdated.addListener(listener);
    chrome.tabs.get(tabId).then((tab) => {
      if (tab.status === "complete") listener(tabId, { status: "complete" });
    }, () => {});
  });
}

async function sendToTab(tabId, message) {
  try {
    return await chrome.tabs.sendMessage(tabId, message);
  } catch {
    return null;
  }
}

async function bridgeReady(tabId) {
  return (await sendToTab(tabId, { type: "bw-ping" }))?.ok === true;
}

async function findXTab(preferred) {
  if (preferred && (await bridgeReady(preferred))) return preferred;
  const tabs = await chrome.tabs.query({ url: "https://x.com/*" });
  for (const tab of tabs) {
    if (!tab.discarded && (await bridgeReady(tab.id))) return tab.id;
  }
  return null;
}

// Opening the bookmarks page makes X request the list itself, which gives the
// sync a fresh template. The tab opens in the background unless asked.
async function openBookmarksTab(active = false) {
  const tab = await chrome.tabs.create({ url: BOOKMARKS_URL, active });
  const template = waitForTemplate();
  template.catch(() => {});
  await waitForTabLoad(tab.id);
  return { tabId: tab.id, template };
}

// Scripts declared in the manifest only reach tabs opened after install, so
// attach to X tabs that were already open.
async function attachToOpenTabs() {
  const tabs = await chrome.tabs.query({ url: "https://x.com/*" });
  for (const tab of tabs) {
    if (tab.discarded) continue;
    await chrome.scripting.executeScript({ target: { tabId: tab.id }, files: ["page-hook.js"], world: "MAIN" }).catch(() => {});
    await chrome.scripting.executeScript({ target: { tabId: tab.id }, files: ["x-bridge.js"] }).catch(() => {});
  }
}

// ---------------------------------------------------------------------------
// Sync: page through the bookmarks list with X's own request

let sync = null;

function freshState(mode, auto = false) {
  return {
    running: true,
    mode,
    // An automatic check started by opening X, rather than Sync now.
    auto,
    phase: "syncing",
    status: "Connecting to X…",
    discovered: 0,
    inserted: 0,
    updated: 0,
    unchanged: 0,
    failed: 0,
    startedAt: new Date().toISOString(),
    finishedAt: null,
  };
}

function publish(state, status, phase) {
  if (status) state.status = status;
  if (phase) state.phase = phase;
  const current = { ...state, acknowledged: sync?.quiet && !state.running ? true : undefined };
  chrome.storage.local.set({ importState: current }).catch(() => {});
  chrome.runtime.sendMessage({ type: "import-progress", ...current }).catch(() => {});
  if (!sync?.quiet || (!state.running && state.inserted > 0)) updateBadge(state);
}

function describe(state) {
  const parts = [];
  if (state.inserted) parts.push(`${state.inserted} new`);
  if (state.updated) parts.push(`${state.updated} updated`);
  return parts.length ? parts.join(", ") : "nothing new yet";
}

// Waits in short steps so the service worker stays awake and Stop works.
async function pause(ms) {
  const until = Date.now() + ms;
  while (Date.now() < until && !sync?.stop) {
    await new Promise((resolve) => setTimeout(resolve, Math.min(15_000, until - Date.now())));
    await chrome.runtime.getPlatformInfo().catch(() => {});
  }
}

function addCounts(state, result) {
  state.discovered += Number(result.received || 0);
  state.inserted += Number(result.inserted || 0);
  state.updated += Number(result.updated || 0) + Number(result.upgraded || 0);
  state.unchanged += Number(result.unchanged || 0) + Number(result.deleted || 0);
  state.failed += Number(result.skipped || 0);
}

async function importTimeline(state, data, source) {
  const response = await apiRequest({ path: "/imports/x", method: "POST", body: { source, timeline: data, account: xAccount } });
  if (!response.ok) throw appError(response);
  addCounts(state, response.data);
  return response.data;
}

async function runSync(mode, { tabId, quiet = false } = {}) {
  if (sync) return { ok: false, error: "A sync is already running.", state: null };
  const { extensionToken } = await settings();
  if (!extensionToken) return { ok: false, error: "Pair the extension first." };
  sync = { stop: false, quiet, mode: "direct" };
  // Automatic checks only look for new bookmarks, never the whole list.
  const state = freshState(mode === "full" && !quiet ? "full" : "new", quiet);
  publish(state);
  void syncLoop(state, tabId);
  return { ok: true, state: { ...state } };
}

async function syncLoop(state, preferredTab) {
  let opened = null;
  let completed = false;
  try {
    await flushPending();
    let { xTemplate: template } = await chrome.storage.local.get("xTemplate");
    let tabId = await findXTab(preferredTab);
    if (!tabId || !template) {
      publish(state, "Opening your X bookmarks…");
      opened = await openBookmarksTab();
      tabId = opened.tabId;
      template = template || (await opened.template);
    }

    let { resumeCursor: cursor = null } = state.mode === "full" ? await chrome.storage.local.get("resumeCursor") : {};
    if (cursor) publish(state, "Resuming where the last full sync stopped…");
    let streak = 0;
    let retries = 0;
    let refreshed = false;
    let pages = 0;

    while (!sync.stop) {
      const page = await sendToTab(tabId, { type: "fetch-page", url: pageURL(template.url, cursor), authorization: template.authorization });
      if (!page) throw new Error("The X tab was closed. The next sync will pick up from here.");
      noteAccount(page.account);
      if (page.status === 429) {
        if (retries >= MAX_RATE_LIMIT_RETRIES) throw new Error("X is limiting requests right now. Try again later.");
        const wait = rateLimitWait(page.rateLimit, retries++);
        const at = new Date(Date.now() + wait).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
        publish(state, `X asked to slow down. Resuming at ${at}…`);
        await pause(wait);
        continue;
      }
      if (page.status === 401 || page.status === 403) throw new Error("Sign in to X in this browser, then sync again.");
      if (page.status !== 200 || !page.data?.data) {
        // X may have changed the request; load the bookmarks page once to learn
        // the new one, then fall back to scrolling.
        if (!refreshed) {
          refreshed = true;
          await chrome.storage.local.remove("xTemplate");
          publish(state, "Refreshing the connection to X…");
          let fresh;
          if (!opened) {
            opened = await openBookmarksTab();
            tabId = opened.tabId;
            fresh = opened.template;
          } else {
            fresh = waitForTemplate();
            await chrome.tabs.reload(opened.tabId);
          }
          template = await fresh.catch(() => null);
          if (template) continue;
        }
        await scrollSync(state, opened);
        completed = true;
        break;
      }
      retries = 0;

      const posts = countTimelineTweets(page.data);
      if (posts === 0) {
        completed = true;
        break;
      }
      const result = await importTimeline(state, page.data, "sync");
      pages += 1;
      // Reached bookmarks from before "only from now on" was chosen.
      if (result.reachedBoundary) {
        completed = true;
        break;
      }
      streak = nextStreak(streak, result);
      if (state.mode === "new" && streak >= KNOWN_STREAK_LIMIT) {
        completed = true;
        break;
      }
      if (state.auto && pages >= AUTO_SYNC_MAX_PAGES) {
        completed = true;
        break;
      }
      const next = findBottomCursor(page.data);
      if (!next || next === cursor) {
        completed = true;
        break;
      }
      cursor = next;
      if (state.mode === "full") await chrome.storage.local.set({ resumeCursor: cursor });
      publish(state, `Checked ${state.discovered.toLocaleString()} bookmarks, ${describe(state)}…`);
      await pause(pageDelay());
    }

    state.running = false;
    state.finishedAt = new Date().toISOString();
    if (completed) {
      if (state.mode === "full") await chrome.storage.local.remove("resumeCursor");
      await chrome.storage.local.set({ lastSyncAt: Date.now() });
      const status = state.inserted || state.updated ? `Done: ${describe(state)}.` : "You're up to date. No new bookmarks.";
      publish(state, status, "complete");
    } else {
      publish(state, `Stopped: checked ${state.discovered.toLocaleString()} bookmarks, ${describe(state)}.`, "stopped");
    }
  } catch (error) {
    state.running = false;
    state.finishedAt = new Date().toISOString();
    publish(state, error instanceof Error ? error.message : "Sync failed.", "error");
  } finally {
    if (opened) chrome.tabs.remove(opened.tabId).catch(() => {});
    sync = null;
  }
}

async function setScope(scope) {
  if (scope === "all") {
    const response = await apiRequest({ path: "/imports/x/scope", method: "POST", body: { scope: "all", account: xAccount } });
    if (!response.ok) throw appError(response);
    return response.data;
  }
  let opened = null;
  try {
    let { xTemplate: template } = await chrome.storage.local.get("xTemplate");
    let tabId = await findXTab();
    if (!tabId || !template) {
      opened = await openBookmarksTab();
      tabId = opened.tabId;
      template = template || (await opened.template);
    }
    const page = await sendToTab(tabId, { type: "fetch-page", url: pageURL(template.url, null), authorization: template.authorization });
    if (!page) throw new Error("The X tab was closed. Try again.");
    if (page.status === 401 || page.status === 403) throw new Error("Sign in to X in this browser, then try again.");
    if (page.status !== 200 || !page.data?.data) throw new Error("X did not return your bookmarks. Try again in a moment.");
    noteAccount(page.account);
    const response = await apiRequest({ path: "/imports/x/scope", method: "POST", body: { scope: "new", timeline: page.data, account: xAccount } });
    if (!response.ok) throw appError(response);
    // The list at this moment is the starting line; nothing to sync yet.
    await chrome.storage.local.set({ lastSyncAt: Date.now() });
    return response.data;
  } finally {
    if (opened) chrome.tabs.remove(opened.tabId).catch(() => {});
  }
}

// Fallback when X's request cannot be repeated: scroll the bookmarks page and
// save each page X loads by itself. The tab must be in front for X to load more.
async function scrollSync(state, opened) {
  sync.mode = "scroll";
  sync.scrollState = state;
  sync.streak = 0;
  publish(state, "Reading your bookmarks by scrolling…");
  let tabId = opened?.tabId;
  if (tabId) await chrome.tabs.update(tabId, { active: true });
  else tabId = (await chrome.tabs.create({ url: BOOKMARKS_URL, active: true })).id;
  await waitForTabLoad(tabId);
  let bottomSteps = 0;
  let lastSeen = -1;
  await sendToTab(tabId, { type: "scroll-step", top: true });
  while (!sync.stop) {
    if (state.mode === "new" && sync.streak >= KNOWN_STREAK_LIMIT) return;
    const step = await sendToTab(tabId, { type: "scroll-step" });
    if (!step) throw new Error("The X tab was closed.");
    if (step.atBottom && state.discovered === lastSeen) {
      if (++bottomSteps >= BOTTOM_WAIT_STEPS) return;
    } else {
      bottomSteps = 0;
    }
    lastSeen = state.discovered;
    publish(state, `Checked ${state.discovered.toLocaleString()} bookmarks, ${describe(state)}…`);
    await pause(SCROLL_STEP_MS);
  }
}

async function onTimeline(data) {
  if (sync?.mode === "scroll") {
    const result = await importTimeline(sync.scrollState, data, "scroll").catch(() => null);
    if (result) sync.streak = result.reachedBoundary ? KNOWN_STREAK_LIMIT : nextStreak(sync.streak, result);
    return;
  }
  // Browsing your bookmarks on X saves what you scroll past, outside a sync,
  // once the user has chosen what Sortwise should bring in.
  if (sync) return;
  const { extensionToken, autoSave } = await settings();
  if (!extensionToken || !autoSave || !(await importScope())) return;
  await apiRequest({ path: "/imports/x", method: "POST", body: { source: "scroll", timeline: data, account: xAccount } });
}

// Opening X checks for new bookmarks in the background, at most once an hour,
// and only when the app is running (tries are spaced out while it is not).
let lastAutoAttempt = 0;
async function onXReady(tabId) {
  const { extensionToken, autoSave, xTemplate, lastSyncAt } = await settings();
  if (!extensionToken || !autoSave || !xTemplate || sync) return;
  if (Date.now() - Number(lastSyncAt || 0) < AUTO_SYNC_INTERVAL_MS || Date.now() - lastAutoAttempt < 10 * 60_000) return;
  lastAutoAttempt = Date.now();
  // Nothing happens automatically until the user has chosen what to bring in.
  if (!(await importScope())) return;
  await runSync("new", { tabId, quiet: true });
}

// ---------------------------------------------------------------------------
// Toolbar badge

function updateBadge(state) {
  if (state.running) {
    chrome.action.setBadgeBackgroundColor({ color: "#1b73cf" });
    chrome.action.setBadgeText({ text: state.inserted > 0 ? String(Math.min(state.inserted, 999)) : "…" });
  } else if (state.phase === "error") {
    chrome.action.setBadgeBackgroundColor({ color: "#e2625f" });
    chrome.action.setBadgeText({ text: "!" });
  } else if (state.phase === "complete") {
    chrome.action.setBadgeBackgroundColor({ color: "#2b593f" });
    chrome.action.setBadgeText({ text: state.inserted > 0 ? `+${Math.min(state.inserted, 99)}` : "✓" });
  } else {
    chrome.action.setBadgeText({ text: "" });
  }
}

let flashTimer = null;
function flashBadge() {
  if (sync) return;
  chrome.action.setBadgeBackgroundColor({ color: "#2b593f" });
  chrome.action.setBadgeText({ text: "+1" });
  clearTimeout(flashTimer);
  flashTimer = setTimeout(() => chrome.action.setBadgeText({ text: "" }), 4000);
}

// ---------------------------------------------------------------------------

chrome.runtime.onInstalled.addListener(() => void attachToOpenTabs());

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  const tabId = sender.tab?.id;
  switch (message?.type) {
    case "api":
      apiRequest(message).then(sendResponse);
      return true;
    case "start-import":
      runSync(message.mode === "full" ? "full" : "new").then(sendResponse);
      return true;
    case "stop-import":
      if (sync) {
        sync.stop = true;
        chrome.storage.local.get("importState").then(({ importState }) => sendResponse({ ok: true, state: { ...importState, phase: "stopping", status: "Stopping after this page…" } }));
        return true;
      }
      sendResponse({ ok: true, state: null });
      return false;
    case "clear-badge":
      if (!sync) chrome.action.setBadgeText({ text: "" });
      return false;
    case "set-scope":
      setScope(message.scope === "new" ? "new" : "all")
        .then((data) => sendResponse({ ok: true, data }))
        .catch((error) => sendResponse({ ok: false, error: error instanceof Error ? error.message : String(error) }));
      return true;
    case "x-template":
      noteAccount(message.account);
      void saveTemplate(message.url, message.authorization);
      return false;
    case "x-timeline":
      noteAccount(message.account);
      void onTimeline(message.data);
      return false;
    case "x-bookmarked":
      onBookmarked(message, tabId).then(sendResponse);
      return true;
    case "x-unbookmarked":
      noteAccount(message.account);
      void onUnbookmarked(message);
      return false;
    case "x-ready":
      noteAccount(message.account);
      if (tabId !== undefined) void onXReady(tabId);
      return false;
  }
  return false;
});
