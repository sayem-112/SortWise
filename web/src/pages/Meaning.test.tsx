import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Collection } from "../lib/api";
import { BookmarksPage } from "./BookmarksPage";

const post = { id: 7, postId: "1001", author: "Ada", username: "ada", text: "My automobile", url: "https://x.com/ada/status/1001", postedAt: "2025-01-02T12:00:00Z", importedAt: "2026-07-18T12:00:00Z", visibleContext: {}, media: [], summary: "", mediaDescription: "", processingStatus: "completed", archived: false, categories: [], tags: [], lists: [] };

// Answers like the server: a search by meaning adds a post the words do not match.
function server(options: { notice?: string } = {}) {
  return vi.fn(async (input: string, init?: RequestInit) => {
    const url = new URL(input, "http://localhost");
    if (init?.method === "PATCH") return { ok: true, json: async () => ({}) };
    if (url.pathname.endsWith("/settings/ai")) return { ok: true, json: async () => ({ meaning: { enabled: true, available: true, indexed: 5, total: 5 }, providers: [], resting: [] }) };
    if (url.pathname.endsWith("/bookmarks")) {
      const meaning = url.searchParams.get("mode") === "meaning";
      const found = meaning && !options.notice;
      return {
        ok: true,
        json: async () => ({
          page: 1,
          pageSize: 50,
          total: found ? 1 : 0,
          items: found ? [{ ...post, match: "meaning" }] : [],
          ...(meaning ? { search: options.notice ? { meaning: false, notice: options.notice } : { meaning: true } } : {}),
        }),
      };
    }
    return { ok: true, json: async () => ({ items: [] }) };
  });
}

function show(page: React.ReactNode, path = "/bookmarks") {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>{page}</MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("search by meaning", () => {
  beforeEach(() => localStorage.clear());

  it("asks for posts about the same thing once Meaning is on, and marks them", async () => {
    const fetchMock = server();
    vi.stubGlobal("fetch", fetchMock);
    show(<BookmarksPage />, "/bookmarks?q=car");

    expect(await screen.findByText("No matching bookmarks")).toBeInTheDocument();
    const toggle = screen.getByRole("button", { name: "Meaning" });
    expect(toggle).toHaveAttribute("aria-pressed", "false");
    fireEvent.click(toggle);

    expect(await screen.findByText("My automobile")).toBeInTheDocument();
    expect(toggle).toHaveAttribute("aria-pressed", "true");
    expect(fetchMock.mock.calls.some(([url]) => String(url).includes("q=car") && String(url).includes("mode=meaning"))).toBe(true);
    expect(screen.getByText(/and posts about the same thing/)).toBeInTheDocument();
    expect(screen.getAllByRole("img", { name: /Found by meaning/ }).length).toBeGreaterThan(0);
    // The choice is kept for the next search.
    expect(localStorage.getItem("bw.searchMode")).toBe("meaning");
  });

  it("says why when only word matches can be shown", async () => {
    vi.stubGlobal("fetch", server({ notice: "Search by meaning needs a Gemini API key. Add one in Settings." }));
    show(<BookmarksPage />, "/bookmarks?q=car&mode=meaning");

    expect(await screen.findByText(/Showing posts with these words only/)).toBeInTheDocument();
    expect(screen.getByText(/needs a Gemini API key/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open Settings" })).toHaveAttribute("href", "/settings");
  });
});

describe("a smart collection's page", () => {
  beforeEach(() => localStorage.clear());

  const collection: Collection = { id: 3, name: "Cars", color: "blue", count: 1, query: { q: "car", mode: "meaning", categories: [], tags: [], status: "", list: "", author: "", sort: "relevance" } };

  it("opens on the saved search and offers to save a change to it", async () => {
    const fetchMock = server();
    vi.stubGlobal("fetch", fetchMock);
    show(<BookmarksPage collection={collection} />, "/collections/3");

    expect(await screen.findByRole("heading", { name: "Cars" })).toBeInTheDocument();
    expect(await screen.findByText("My automobile")).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "Search bookmarks" })).toHaveValue("car");
    expect(screen.getByRole("button", { name: "Meaning" })).toHaveAttribute("aria-pressed", "true");
    // Untouched, there is nothing to save, and no second "Save search".
    expect(screen.queryByRole("button", { name: "Save changes" })).toBeNull();
    expect(screen.queryByRole("button", { name: /Save as a smart collection/ })).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Meaning" }));
    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));
    await waitFor(() => {
      const sent = fetchMock.mock.calls.find(([url, init]) => init?.method === "PATCH" && String(url).endsWith("/collections/3"));
      expect(JSON.parse(String(sent?.[1]?.body)).query).toMatchObject({ q: "car", mode: "" });
    });

    // Discard goes back to what was saved.
    fireEvent.click(screen.getByRole("button", { name: "Discard" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Meaning" })).toHaveAttribute("aria-pressed", "true"));
    expect(screen.queryByRole("button", { name: "Save changes" })).toBeNull();
  });
});
