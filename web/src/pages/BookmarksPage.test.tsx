import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { BookmarksPage } from "./BookmarksPage";

const post = { id: 7, postId: "1001", author: "Ada", username: "ada", text: "Reliable queues", url: "https://x.com/ada/status/1001", postedAt: "2025-01-02T12:00:00Z", importedAt: "2026-07-18T12:00:00Z", visibleContext: {}, media: [], summary: "", mediaDescription: "", processingStatus: "pending", archived: false, categories: [], tags: [], lists: [] };

describe("Library browsing", () => {
  it("sorts by posted date and deletes directly from a card", async () => {
    const fetchMock = vi.fn(async (input: string, init?: RequestInit) => {
      if (init?.method === "DELETE") return { ok: true, json: async () => ({}) };
      if (input.includes("/categories") || input.includes("/tags")) return { ok: true, json: async () => ({ items: [] }) };
      return {
        ok: true,
        json: async () => ({
          page: 1,
          pageSize: 36,
          total: 1,
          items: [{ id: 7, postId: "1001", author: "Ada", username: "ada", text: "Reliable queues", url: "https://x.com/ada/status/1001", postedAt: "2025-01-02T12:00:00Z", importedAt: "2026-07-18T12:00:00Z", visibleContext: {}, media: [], summary: "", mediaDescription: "", processingStatus: "pending", archived: false, categories: [], tags: [], lists: [] }],
        }),
      };
    });
    vi.stubGlobal("fetch", fetchMock);
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<QueryClientProvider client={client}><MemoryRouter><BookmarksPage /><ConfirmDialog /></MemoryRouter></QueryClientProvider>);

    expect(await screen.findByText("Jan 2, 2025")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("combobox", { name: "Sort bookmarks" }));
    fireEvent.click(screen.getByRole("option", { name: "Newest post date" }));
    await waitFor(() => expect(fetchMock.mock.calls.some(([url]) => String(url).includes("sort=posted_desc"))).toBe(true));

    fireEvent.click(await screen.findByRole("button", { name: "Delete bookmark by Ada" }));
    // Nothing is deleted until the in-app dialog is confirmed.
    expect(await screen.findByRole("alertdialog", { name: "Delete this bookmark?" })).toBeInTheDocument();
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === "DELETE")).toBe(false);
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(fetchMock.mock.calls.some(([, init]) => init?.method === "DELETE")).toBe(true));
  });

  it("stars a bookmark into Favorites and shows a list's own page", async () => {
    const favorites = { id: 1, name: "Favorites", kind: "favorites", count: 0 };
    let lists: number[] = [];
    const fetchMock = vi.fn(async (input: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        lists = [1];
        return { ok: true, json: async () => ({}) };
      }
      if (input.includes("/lists")) return { ok: true, json: async () => ({ items: [favorites] }) };
      if (input.includes("/categories") || input.includes("/tags")) return { ok: true, json: async () => ({ items: [] }) };
      return { ok: true, json: async () => ({ page: 1, pageSize: 50, total: 1, items: [{ ...post, lists }] }) };
    });
    vi.stubGlobal("fetch", fetchMock);
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<QueryClientProvider client={client}><MemoryRouter><BookmarksPage list={favorites as never} /></MemoryRouter></QueryClientProvider>);

    expect(await screen.findByRole("heading", { name: "Favorites" })).toBeInTheDocument();
    await waitFor(() => expect(fetchMock.mock.calls.some(([url]) => String(url).includes("list=1") && String(url).includes("sort=added_desc"))).toBe(true));

    const star = await screen.findByRole("button", { name: "Add to Favorites" });
    await waitFor(() => expect(star).toBeEnabled());
    fireEvent.click(star);
    await waitFor(() => expect(star).toHaveAttribute("aria-pressed", "true"));
    const sent = fetchMock.mock.calls.find(([url, init]) => init?.method === "POST" && String(url).endsWith("/lists/1/bookmarks"));
    expect(JSON.parse(String(sent?.[1]?.body))).toEqual({ bookmarkIds: [7], inList: true });
  });

  it("adds every selected bookmark to a list in one go", async () => {
    const reading = { id: 2, name: "Read later", kind: "custom", icon: "list", color: "purple", count: 0 };
    const second = { ...post, id: 8, postId: "1002", author: "Grace" };
    // Remembers what was filed, like the real app.
    const filed = new Set<number>();
    const fetchMock = vi.fn(async (input: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        const body = JSON.parse(String(init.body));
        body.bookmarkIds.forEach((id: number) => (body.inList ? filed.add(id) : filed.delete(id)));
        return { ok: true, json: async () => ({}) };
      }
      if (input.includes("/lists")) return { ok: true, json: async () => ({ items: [{ id: 1, name: "Favorites", kind: "favorites", icon: "star", color: "yellow", count: 0 }, reading] }) };
      if (input.includes("/categories") || input.includes("/tags")) return { ok: true, json: async () => ({ items: [] }) };
      return { ok: true, json: async () => ({ page: 1, pageSize: 50, total: 2, items: [post, second].map((item) => ({ ...item, lists: filed.has(item.id) ? [2] : [] })) }) };
    });
    vi.stubGlobal("fetch", fetchMock);
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<QueryClientProvider client={client}><MemoryRouter><BookmarksPage /></MemoryRouter></QueryClientProvider>);

    fireEvent.click(await screen.findByRole("checkbox", { name: "Select post by Ada" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Select post by Grace" }));
    expect(screen.getByText("2 selected")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Add to list" }));
    // Favorites is the star, not a list in the picker.
    expect(screen.queryByRole("checkbox", { name: /Favorites/ })).not.toBeInTheDocument();
    const field = await screen.findByRole("textbox", { name: "Find or create a list" });
    fireEvent.change(field, { target: { value: "read" } });
    // Enter adds; pressing it again must not take the posts back out.
    fireEvent.keyDown(field, { key: "Enter" });
    await waitFor(() => expect(screen.getByRole("checkbox", { name: /Read later/ })).toHaveAttribute("aria-checked", "true"));
    fireEvent.keyDown(field, { key: "Enter" });
    await waitFor(() => {
      const sent = fetchMock.mock.calls.filter(([url, init]) => init?.method === "POST" && String(url).endsWith("/lists/2/bookmarks"));
      expect(sent.map(([, init]) => JSON.parse(String(init?.body)))).toEqual([{ bookmarkIds: [7, 8], inList: true }]);
    });
  });

  it("files posts from the keyboard: J/K move, X selects, F stars", async () => {
    const second = { ...post, id: 8, postId: "1002", author: "Grace" };
    const fetchMock = vi.fn(async (input: string, init?: RequestInit) => {
      if (init?.method === "POST") return { ok: true, json: async () => ({}) };
      if (input.includes("/lists")) return { ok: true, json: async () => ({ items: [{ id: 1, name: "Favorites", kind: "favorites", icon: "star", color: "yellow", count: 0 }] }) };
      if (input.includes("/categories") || input.includes("/tags")) return { ok: true, json: async () => ({ items: [] }) };
      return { ok: true, json: async () => ({ page: 1, pageSize: 50, total: 2, items: [post, second] }) };
    });
    vi.stubGlobal("fetch", fetchMock);
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<QueryClientProvider client={client}><MemoryRouter><BookmarksPage /></MemoryRouter></QueryClientProvider>);
    await screen.findByRole("checkbox", { name: "Select post by Ada" });
    await waitFor(() => expect(fetchMock.mock.calls.some(([url]) => String(url).includes("/lists"))).toBe(true));

    fireEvent.keyDown(window, { key: "j" });
    fireEvent.keyDown(window, { key: "x" });
    fireEvent.keyDown(window, { key: "j" });
    fireEvent.keyDown(window, { key: "x" });
    expect(await screen.findByText("2 selected")).toBeInTheDocument();
    await waitFor(() => {
      fireEvent.keyDown(window, { key: "f" });
      const sent = fetchMock.mock.calls.find(([url, init]) => init?.method === "POST" && String(url).endsWith("/lists/1/bookmarks"));
      expect(JSON.parse(String(sent?.[1]?.body))).toEqual({ bookmarkIds: [7, 8], inList: true });
    });
  });
});
