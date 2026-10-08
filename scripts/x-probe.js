/* Sortwise feasibility probe.
   Paste into the DevTools console on your X bookmarks page
   (https://x.com/i/history, Bookmarks tab, or https://x.com/i/bookmarks).
   It watches X's own API traffic, then (when you run __bwRun) replays the
   Bookmarks request a few ways to see what X accepts. Read-only: it never
   bookmarks, deletes, or posts anything, and it only talks to x.com.
   The report lists header NAMES only, never cookies, tokens, or post text. */
(() => {
  if (window.__bwProbe) return console.log("Sortwise probe is already running.");
  const P = (window.__bwProbe = { calls: [], seenIds: new Set(), fetch: window.fetch.bind(window) });
  const isGql = (u) => /\/i\/api\/graphql\//.test(String(u));
  const opOf = (u) => {
    const m = String(u).match(/\/graphql\/([^/]+)\/([^?]+)/);
    return m ? { queryId: m[1], op: m[2] } : {};
  };

  function save(req, status, data, transport) {
    const { queryId, op } = opOf(req.url);
    if (data) for (const m of JSON.stringify(data).matchAll(/"rest_id":"(\d{5,})"/g)) P.seenIds.add(m[1]);
    // Keep bookmark mutations and any paged timeline (the list request's name
    // may have changed with X's History redesign).
    const text = data ? JSON.stringify(data) : "";
    const keep = /Bookmark/i.test(op || "") || (text.includes('"TimelineTweet"') && text.includes('"Bottom"'));
    P.calls.push({ transport, method: req.method, op, queryId, url: req.url, headers: req.headers, body: req.body, status, data: keep ? data : null });
  }

  const X = XMLHttpRequest.prototype;
  const open = X.open, send = X.send, setHeader = X.setRequestHeader;
  X.open = function (method, url) {
    this.__bw = { method: String(method).toUpperCase(), url: String(url), headers: {} };
    return open.apply(this, arguments);
  };
  X.setRequestHeader = function (name, value) {
    if (this.__bw) this.__bw.headers[String(name).toLowerCase()] = value;
    return setHeader.apply(this, arguments);
  };
  X.send = function (body) {
    const req = this.__bw;
    if (req && isGql(req.url)) {
      req.body = typeof body === "string" ? body : null;
      this.addEventListener("loadend", () => {
        let data = null;
        try { data = this.responseType === "json" ? this.response : JSON.parse(this.responseText); } catch {}
        save(req, this.status, data, "xhr");
      });
    }
    return send.apply(this, arguments);
  };

  const nativeFetch = window.fetch;
  window.fetch = async function (input, init = {}) {
    const url = typeof input === "string" ? input : input?.url;
    const res = await nativeFetch.apply(this, arguments);
    if (isGql(url)) {
      const headers = {};
      new Headers(init.headers || (input instanceof Request ? input.headers : undefined)).forEach((v, k) => (headers[k] = v));
      const req = { method: (init.method || "GET").toUpperCase(), url, headers, body: typeof init.body === "string" ? init.body : null };
      res.clone().json().then((d) => save(req, res.status, d, "fetch")).catch(() => save(req, res.status, null, "fetch"));
    }
    return res;
  };

  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
  const findBottomCursor = (node) => {
    if (!node || typeof node !== "object") return null;
    if (node.cursorType === "Bottom" && typeof node.value === "string") return node.value;
    for (const v of Object.values(node)) {
      const c = findBottomCursor(v);
      if (c) return c;
    }
    return null;
  };
  const summarize = (status, data) => {
    const text = data ? JSON.stringify(data) : "";
    return {
      status,
      posts: (text.match(/"__typename":"TimelineTweet"/g) || []).length,
      hasNextCursor: Boolean(findBottomCursor(data)),
      error: data?.errors?.[0] ? `${data.errors[0].code ?? ""} ${String(data.errors[0].message).slice(0, 120)}` : null,
    };
  };
  const ct0 = () => (document.cookie.match(/(?:^|; )ct0=([^;]+)/) || [])[1];

  async function attempt(url, headers) {
    try {
      const res = await P.fetch(url, { method: "GET", headers, credentials: "include" });
      let data = null;
      try { data = await res.json(); } catch {}
      return { ...summarize(res.status, data), data };
    } catch (error) {
      return { status: "network error", error: String(error).slice(0, 120) };
    }
  }

  window.__bwRun = async function () {
    const report = { transportUsedByX: [...new Set(P.calls.map((c) => c.transport))], operationsSeen: [...new Set(P.calls.map((c) => c.op))] };
    // The saved-posts list request: prefer names mentioning bookmarks or
    // history, else any paged timeline requested while on the bookmarks tab.
    const timelines = P.calls.filter((c) => c.method === "GET" && c.status === 200 && c.data && findBottomCursor(c.data));
    report.pagedTimelinesSeen = [...new Set(timelines.map((c) => c.op))];
    const page = [...timelines].reverse().find((c) => /bookmark|history/i.test(c.op)) || null;
    if (!page) {
      report.problem = "No bookmarks list request recognized. Scroll your bookmarks until more posts load, then run __bwRun() again. (pagedTimelinesSeen lists the names that were captured.)";
    } else {
      report.bookmarksRequest = {
        op: page.op,
        page: location.pathname,
        transport: page.transport,
        headerNames: Object.keys(page.headers).sort(),
        hadTransactionId: "x-client-transaction-id" in page.headers,
      };
      const orig = page.headers;
      const noTxn = { ...orig };
      delete noTxn["x-client-transaction-id"];
      const minimal = {
        authorization: orig.authorization,
        "x-csrf-token": ct0(),
        "x-twitter-auth-type": "OAuth2Session",
        "x-twitter-active-user": "yes",
        "content-type": "application/json",
      };
      const nextUrl = (() => {
        const cursor = findBottomCursor(page.data);
        if (!cursor) return null;
        const u = new URL(page.url, location.origin);
        const vars = JSON.parse(u.searchParams.get("variables") || "{}");
        vars.cursor = cursor;
        u.searchParams.set("variables", JSON.stringify(vars));
        return u.toString();
      })();

      const tests = [
        ["1 exact replay (same transaction id)", page.url, orig],
        ["2 no transaction id", page.url, noTxn],
        ["3 minimal headers (what a background sync would send)", page.url, minimal],
        ["4 next page, no transaction id", nextUrl, noTxn],
        ["5 next page, minimal headers", nextUrl, minimal],
      ];
      report.replays = {};
      let sample = null;
      for (const [name, url, headers] of tests) {
        if (!url) { report.replays[name] = "skipped (no next-page cursor)"; continue; }
        const { data, ...result } = await attempt(url, headers);
        report.replays[name] = result;
        if (!sample && result.status === 200 && result.posts) sample = data;
        await sleep(2500);
      }
      const s = JSON.stringify(sample || page.data || {});
      report.dataIncludes = {
        longPostText: s.includes('"note_tweet"'),
        articles: s.includes('"article"'),
        quotedPosts: s.includes('"quoted_status_result"'),
        videoFiles: s.includes('"video_info"'),
        linkCards: s.includes('"card"'),
      };
    }

    const marks = P.calls.filter((c) => c.method === "POST" && /Bookmark/i.test(c.op || ""));
    report.bookmarkClicks = marks.length
      ? marks.map((c) => {
          let vars = {};
          try { vars = JSON.parse(c.body || "{}").variables || {}; } catch {}
          return {
            op: c.op,
            transport: c.transport,
            method: c.method,
            status: c.status,
            bodyVariableNames: Object.keys(vars),
            hasTweetId: Boolean(vars.tweet_id),
            postWasAlreadyLoadedOnPage: vars.tweet_id ? P.seenIds.has(String(vars.tweet_id)) : null,
            responseDataKeys: Object.keys(c.data?.data || {}),
          };
        })
      : "None captured. Open Home (click it in the sidebar, don't reload), bookmark a post, unbookmark it, then run __bwRun() again.";

    const json = JSON.stringify(report, null, 2);
    console.log(json);
    try { copy(json); console.log("Report copied to the clipboard."); } catch { console.log("Copy the report above."); }
    return "done";
  };

  console.log("Sortwise probe armed. Scroll your bookmarks, then bookmark and unbookmark a post from Home, then run: await __bwRun()");
})();
