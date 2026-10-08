import { readFileSync } from "node:fs";
import { JSDOM } from "jsdom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const source = readFileSync(new URL("./page-hook.js", import.meta.url), "utf8");

// A stand-in for the XMLHttpRequest X's code uses.
class FakeXHR {
  constructor() {
    this.listeners = {};
    this.responseType = "";
  }
  open() {}
  setRequestHeader() {}
  send() {}
  addEventListener(type, fn) {
    this.listeners[type] = fn;
  }
  respond(status, data) {
    this.status = status;
    this.responseText = JSON.stringify(data);
    this.listeners.load?.();
  }
}

let window;
let sent;

beforeEach(() => {
  const dom = new JSDOM("<!doctype html><html><body></body></html>", { url: "https://x.com/home", runScripts: "outside-only" });
  window = dom.window;
  window.XMLHttpRequest = class extends FakeXHR {};
  window.fetch = vi.fn();
  sent = [];
  window.postMessage = (message) => sent.push(message);
  window.eval(source);
});

afterEach(() => window.close());

function call(method, url, body, response, headers = {}) {
  const request = new window.XMLHttpRequest();
  request.open(method, url);
  for (const [name, value] of Object.entries(headers)) request.setRequestHeader(name, value);
  request.send(body);
  request.respond(200, response);
}

const tweet = (id) => ({ __typename: "Tweet", rest_id: id, legacy: { full_text: `post ${id}` } });
const timeline = (...ids) => ({ data: { home: { instructions: [{ entries: ids.map((id) => ({ content: { itemContent: { __typename: "TimelineTweet", tweet_results: { result: tweet(id) } } } })) }] } } });

describe("page hook", () => {
  it("passes on the Bookmarks request as a template and its page of posts", () => {
    const url = "https://x.com/i/api/graphql/q1/Bookmarks?variables=%7B%22count%22%3A20%7D";
    call("GET", url, null, timeline("1"), { Authorization: "Bearer public" });
    expect(sent.find((m) => m.type === "template")).toMatchObject({ url, authorization: "Bearer public" });
    expect(sent.find((m) => m.type === "timeline").data.data.home).toBeTruthy();
  });

  it("sends the whole post when you bookmark something already on screen", () => {
    call("GET", "https://x.com/i/api/graphql/h1/HomeTimeline?variables=%7B%7D", null, timeline("41", "42"));
    call("POST", "https://x.com/i/api/graphql/c1/CreateBookmark", JSON.stringify({ variables: { tweet_id: "42" } }), { data: { tweet_bookmark_put: "Done" } });
    const bookmarked = sent.find((m) => m.type === "bookmarked");
    expect(bookmarked.tweetId).toBe("42");
    expect(bookmarked.tweet.legacy.full_text).toBe("post 42");

    call("POST", "https://x.com/i/api/graphql/c1/CreateBookmark", JSON.stringify({ variables: { tweet_id: "99" } }), { data: {} });
    expect(sent.filter((m) => m.type === "bookmarked")[1]).toMatchObject({ tweetId: "99", tweet: null });
  });

  it("reports unbookmarking", () => {
    call("POST", "https://x.com/i/api/graphql/d1/DeleteBookmark", JSON.stringify({ variables: { tweet_id: "42" } }), { data: { tweet_bookmark_delete: "Done" } });
    expect(sent.find((m) => m.type === "unbookmarked")).toMatchObject({ tweetId: "42" });
  });

  it("only fetches X's Bookmarks list when asked", async () => {
    window.dispatchEvent(new window.MessageEvent("message", { source: window, data: { source: "sortwise:bridge", type: "fetch-page", requestId: "r1", url: "https://x.com/i/api/graphql/q/DeleteBookmark?x", authorization: "Bearer t" } }));
    await vi.waitFor(() => expect(sent.find((m) => m.type === "page")).toMatchObject({ requestId: "r1", status: 0 }));
  });
});
