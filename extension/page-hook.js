// Runs inside X's page (the "main world") from the moment it starts loading.
// It watches the API calls X's own code makes, which is the only place the full
// data lives, and passes what Sortwise needs to x-bridge.js:
//   - the Bookmarks list request, used as a template to page through the list;
//   - pages of the Bookmarks list as X loads them;
//   - the post you just bookmarked or unbookmarked.
// It also fetches pages of the list when asked. Those requests run here, as part
// of X's page with your X login, exactly like X's own requests.
(() => {
  "use strict";
  if (window.__sortwiseHook) return;
  window.__sortwiseHook = true;

  const TO_BRIDGE = "sortwise:hook";
  const FROM_BRIDGE = "sortwise:bridge";
  const GRAPHQL = /^https:\/\/x\.com\/i\/api\/graphql\/[\w-]+\/(\w+)/;
  // Posts seen anywhere on X recently, so a bookmarked post can be saved whole.
  const CACHE_LIMIT = 1000;
  const cache = new Map();
  const nativeFetch = window.fetch.bind(window);

  function send(type, payload) {
    window.postMessage({ source: TO_BRIDGE, type, ...payload }, location.origin);
  }

  function operation(url) {
    const match = String(url).match(GRAPHQL);
    return match ? match[1] : null;
  }

  function postId(result) {
    if (!result || typeof result !== "object") return null;
    if (result.__typename === "TweetWithVisibilityResults") return result.tweet?.rest_id || null;
    if (result.__typename === "Tweet" || (result.rest_id && result.legacy)) return result.rest_id || null;
    return null;
  }

  // Remember every post result in a response, including quoted posts.
  function remember(node, depth = 0) {
    if (!node || typeof node !== "object" || depth > 40) return;
    if (Array.isArray(node)) {
      for (const item of node) remember(item, depth + 1);
      return;
    }
    const id = postId(node);
    if (id) {
      cache.delete(id);
      cache.set(id, node);
      if (cache.size > CACHE_LIMIT) cache.delete(cache.keys().next().value);
    }
    for (const value of Object.values(node)) remember(value, depth + 1);
  }

  function bookmarkedId(body) {
    try {
      const id = JSON.parse(body || "{}")?.variables?.tweet_id;
      return /^\d+$/.test(String(id)) ? String(id) : null;
    } catch {
      return null;
    }
  }

  function observe(request, status, data) {
    const op = operation(request.url);
    if (!op || status !== 200) return;
    if (request.method === "GET" && data) remember(data);
    if (op === "Bookmarks" && request.method === "GET" && data) {
      send("template", { url: request.url, authorization: request.headers.authorization || "" });
      send("timeline", { data });
    } else if (op === "CreateBookmark") {
      const id = bookmarkedId(request.body);
      if (id) send("bookmarked", { tweetId: id, tweet: cache.get(id) || null });
    } else if (op === "DeleteBookmark") {
      const id = bookmarkedId(request.body);
      if (id) send("unbookmarked", { tweetId: id });
    }
  }

  // X makes its API calls with XMLHttpRequest.
  const xhr = XMLHttpRequest.prototype;
  const open = xhr.open;
  const setRequestHeader = xhr.setRequestHeader;
  const sendRequest = xhr.send;
  xhr.open = function (method, url) {
    this.__sortwise = { method: String(method).toUpperCase(), url: String(url), headers: {} };
    return open.apply(this, arguments);
  };
  xhr.setRequestHeader = function (name, value) {
    if (this.__sortwise) this.__sortwise.headers[String(name).toLowerCase()] = String(value);
    return setRequestHeader.apply(this, arguments);
  };
  xhr.send = function (body) {
    const request = this.__sortwise;
    if (request && operation(request.url)) {
      request.body = typeof body === "string" ? body : null;
      this.addEventListener("load", () => {
        let data = null;
        try {
          data = this.responseType === "json" ? this.response : this.responseType === "" || this.responseType === "text" ? JSON.parse(this.responseText) : null;
        } catch {
          data = null;
        }
        observe(request, this.status, data);
      });
    }
    return sendRequest.apply(this, arguments);
  };

  // In case X moves some calls to fetch.
  window.fetch = async function (input, init) {
    const response = await nativeFetch(input, init);
    try {
      const url = typeof input === "string" ? input : input?.url;
      if (operation(url)) {
        const headers = {};
        new Headers(init?.headers || (input instanceof Request ? input.headers : undefined)).forEach((value, key) => (headers[key] = value));
        const request = { method: String(init?.method || (input instanceof Request ? input.method : "GET")).toUpperCase(), url, headers, body: typeof init?.body === "string" ? init.body : null };
        response.clone().json().then((data) => observe(request, response.status, data), () => {});
      }
    } catch {
      /* observing must never break X */
    }
    return response;
  };

  function csrfToken() {
    return (document.cookie.match(/(?:^|; )ct0=([^;]+)/) || [])[1] || "";
  }

  // Fetch one page of the bookmarks list for a sync.
  async function fetchPage({ requestId, url, authorization }) {
    const reply = (payload) => send("page", { requestId, ...payload });
    if (!/^https:\/\/x\.com\/i\/api\/graphql\/[\w-]+\/Bookmarks\?/.test(String(url)) || !/^Bearer /.test(String(authorization))) {
      reply({ status: 0, error: "invalid request" });
      return;
    }
    try {
      const response = await nativeFetch(url, {
        method: "GET",
        credentials: "include",
        headers: {
          authorization,
          "x-csrf-token": csrfToken(),
          "x-twitter-auth-type": "OAuth2Session",
          "x-twitter-active-user": "yes",
          "content-type": "application/json",
        },
      });
      let data = null;
      try {
        data = await response.json();
      } catch {
        data = null;
      }
      reply({
        status: response.status,
        data,
        rateLimit: { remaining: response.headers.get("x-rate-limit-remaining"), reset: response.headers.get("x-rate-limit-reset") },
      });
    } catch (error) {
      reply({ status: 0, error: String(error) });
    }
  }

  window.addEventListener("message", (event) => {
    if (event.source !== window || event.data?.source !== FROM_BRIDGE) return;
    if (event.data.type === "fetch-page") void fetchPage(event.data);
  });
})();
