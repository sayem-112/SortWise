// Content script on x.com. It connects page-hook.js (inside X's page) with the
// extension's service worker, shows a small confirmation when a bookmark is
// saved, and scrolls the page for the fallback importer.
(() => {
  "use strict";

  const FROM_HOOK = "sortwise:hook";
  const TO_HOOK = "sortwise:bridge";
  const PAGE_TIMEOUT_MS = 30_000;
  const pending = new Map();
  let nextRequestId = 1;

  // After the extension is reloaded, scripts left in open tabs lose their
  // connection; they stop listening so only the new copy acts.
  const alive = () => Boolean(globalThis.chrome?.runtime?.id);

  // X's twid cookie holds the signed-in account ("u=<user id>").
  function account() {
    const match = document.cookie.match(/(?:^|; )twid=([^;]+)/);
    const value = match ? decodeURIComponent(match[1]) : "";
    return (value.match(/u=(\d+)/) || [])[1] || "";
  }

  function toWorker(message) {
    if (!alive()) return Promise.resolve(null);
    return chrome.runtime.sendMessage({ ...message, account: account() }).catch(() => null);
  }

  function onHookMessage(event) {
    if (!alive()) {
      window.removeEventListener("message", onHookMessage);
      return;
    }
    if (event.source !== window || event.data?.source !== FROM_HOOK) return;
    const message = event.data;
    switch (message.type) {
      case "page": {
        const resolve = pending.get(message.requestId);
        if (resolve) {
          pending.delete(message.requestId);
          resolve({ status: message.status, data: message.data, rateLimit: message.rateLimit, error: message.error, account: account() });
        }
        break;
      }
      case "template":
        void toWorker({ type: "x-template", url: message.url, authorization: message.authorization });
        break;
      case "timeline":
        void toWorker({ type: "x-timeline", data: message.data });
        break;
      case "bookmarked":
        void toWorker({ type: "x-bookmarked", tweetId: message.tweetId, tweet: message.tweet }).then(showSaved);
        break;
      case "unbookmarked":
        void toWorker({ type: "x-unbookmarked", tweetId: message.tweetId });
        break;
    }
  }
  window.addEventListener("message", onHookMessage);

  function fetchPage(url, authorization) {
    return new Promise((resolve) => {
      const requestId = `${Date.now()}-${nextRequestId++}`;
      const timer = setTimeout(() => {
        pending.delete(requestId);
        resolve({ status: 0, error: "X did not answer in time." });
      }, PAGE_TIMEOUT_MS);
      pending.set(requestId, (result) => {
        clearTimeout(timer);
        resolve(result);
      });
      window.postMessage({ source: TO_HOOK, type: "fetch-page", requestId, url, authorization }, location.origin);
    });
  }

  function atBottom() {
    return window.scrollY + window.innerHeight >= document.documentElement.scrollHeight - 8;
  }

  chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
    if (message?.type === "bw-ping") {
      sendResponse({ ok: true, url: location.href });
      return false;
    }
    if (message?.type === "fetch-page") {
      fetchPage(message.url, message.authorization).then(sendResponse);
      return true;
    }
    if (message?.type === "scroll-step") {
      if (message.top) window.scrollTo({ top: 0 });
      else window.scrollBy({ top: Math.round(window.innerHeight * 0.85) });
      sendResponse({ atBottom: atBottom() });
      return false;
    }
    return false;
  });

  // A quiet confirmation in the corner of X, in the app's Notion dark style.
  let toastHost = null;
  let toastTimer = null;
  function toast(text, tone) {
    if (!toastHost) {
      toastHost = document.createElement("div");
      toastHost.style.cssText = "position:fixed;left:20px;bottom:20px;z-index:2147483647;pointer-events:none";
      const shadow = toastHost.attachShadow({ mode: "open" });
      shadow.innerHTML = `<style>
        .toast{display:flex;align-items:center;gap:8px;max-width:320px;padding:9px 12px;border-radius:6px;background:#252525;
          box-shadow:0 0 0 1px rgba(255,255,255,.094),0 8px 24px rgba(0,0,0,.4);color:rgba(255,255,255,.81);
          font:500 13px/1.4 ui-sans-serif,-apple-system,"Segoe UI",sans-serif;opacity:0;transform:translateY(6px);transition:opacity .16s,transform .16s}
        .toast.show{opacity:1;transform:none}
        .dot{flex:none;width:7px;height:7px;border-radius:50%;background:#6cc393}
        .toast[data-tone=orange] .dot{background:#e59b62}
        @media (prefers-reduced-motion:reduce){.toast{transition:none}}
      </style><div class="toast" role="status" aria-live="polite"><span class="dot"></span><span class="text"></span></div>`;
      document.documentElement.appendChild(toastHost);
    }
    const box = toastHost.shadowRoot.querySelector(".toast");
    box.querySelector(".text").textContent = text;
    box.dataset.tone = tone;
    requestAnimationFrame(() => box.classList.add("show"));
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => box.classList.remove("show"), 2600);
  }

  function showSaved(result) {
    if (!result || result.disabled) return;
    if (result.saved) toast("Saved to Sortwise", "green");
    else if (result.offline) toast("Sortwise isn't running. It will save this later.", "orange");
    else if (result.queued) toast("Saving to Sortwise…", "green");
    else if (result.unpaired) toast("Sortwise is disconnected. Pair the extension to keep saving.", "orange");
    else if (result.otherAccount) toast("Not saved: this X account isn't the one your Sortwise library belongs to.", "orange");
  }

  const ready = () => void toWorker({ type: "x-ready" });
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", ready, { once: true });
  else ready();
})();
