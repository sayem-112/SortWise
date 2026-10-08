import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { BookmarksPage } from "./BookmarksPage";

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
          items: [{ id: 7, postId: "1001", author: "Ada", username: "ada", text: "Reliable queues", url: "https://x.com/ada/status/1001", postedAt: "2025-01-02T12:00:00Z", importedAt: "2026-07-18T12:00:00Z", visibleContext: {}, media: [], summary: "", mediaDescription: "", processingStatus: "pending", archived: false, categories: [], tags: [] }],
        }),
      };
    });
    vi.stubGlobal("fetch", fetchMock);
    vi.spyOn(window, "confirm").mockReturnValue(true);
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<QueryClientProvider client={client}><MemoryRouter><BookmarksPage /></MemoryRouter></QueryClientProvider>);

    expect(await screen.findByText("Jan 2, 2025")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("combobox", { name: "Sort bookmarks" }));
    fireEvent.click(screen.getByRole("option", { name: "Newest post date" }));
    await waitFor(() => expect(fetchMock.mock.calls.some(([url]) => String(url).includes("sort=posted_desc"))).toBe(true));

    fireEvent.click(await screen.findByRole("button", { name: "Delete bookmark by Ada" }));
    await waitFor(() => expect(fetchMock.mock.calls.some(([, init]) => init?.method === "DELETE")).toBe(true));
  });
});
