import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { CommandPalette } from "./CommandPalette";

const START = String.fromCharCode(2);
const END = String.fromCharCode(3);

function Where() {
  return <output data-testid="where">{useLocation().pathname}</output>;
}

function setup() {
  const fetchMock = vi.fn(async (input: string) => {
    if (input.includes("/search/quick")) {
      return {
        ok: true,
        json: async () => ({
          total: 12,
          bookmarks: [
            { id: 7, author: "Ada Lovelace", username: "ada", postedAt: "2025-01-02T12:00:00Z", importedAt: "2026-07-18T12:00:00Z", text: `The analytical ${START}engine${END} weaves patterns` },
            { id: 8, author: "Grace Hopper", username: "grace", importedAt: "2026-07-18T12:00:00Z", text: "A ship in port is safe", taxonomy: `${START}engine${END} compilers` },
          ],
          authors: [{ username: "Babbage", name: "Charles Babbage", count: 3, latestSavedAt: "2026-07-18 12:00:00" }],
        }),
      };
    }
    return { ok: true, json: async () => ({ items: [] }) };
  });
  vi.stubGlobal("fetch", fetchMock);
  const onOpenBookmark = vi.fn();
  const onClose = vi.fn();
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <CommandPalette open onClose={onClose} onNewList={() => undefined} onOpenBookmark={onOpenBookmark} />
        <Where />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { fetchMock, onOpenBookmark, onClose };
}

describe("finder", () => {
  it("shows why each post matched and previews it on Enter", async () => {
    const { onOpenBookmark, onClose } = setup();
    const field = screen.getByRole("combobox");
    fireEvent.change(field, { target: { value: "engine" } });

    // The matched word is highlighted in the post, and in the tags when that is where it matched.
    const first = await screen.findByRole("option", { name: /Ada Lovelace/ });
    expect(first.querySelector("mark")).toHaveTextContent("engine");
    expect(screen.getByRole("option", { name: /Grace Hopper/ })).toHaveTextContent("Tagged");
    expect(screen.getByRole("option", { name: /Charles Babbage/ })).toHaveTextContent("3 saved");
    expect(screen.getByRole("option", { name: "See all 12 results in the library" })).toBeInTheDocument();

    fireEvent.keyDown(field, { key: "Enter" });
    expect(onOpenBookmark).toHaveBeenCalledWith(7, [7, 8]);
    expect(onClose).toHaveBeenCalled();
    expect(screen.getByTestId("where")).toHaveTextContent("/");
  });

  it("opens a post as a full page with Ctrl+Enter, and a person's page from their row", async () => {
    const { onOpenBookmark } = setup();
    const field = screen.getByRole("combobox");
    fireEvent.change(field, { target: { value: "engine" } });
    await screen.findByRole("option", { name: /Ada Lovelace/ });

    fireEvent.keyDown(field, { key: "Enter", ctrlKey: true });
    expect(onOpenBookmark).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.getByTestId("where")).toHaveTextContent("/bookmarks/7"));

    fireEvent.click(screen.getByRole("option", { name: /Charles Babbage/ }));
    await waitFor(() => expect(screen.getByTestId("where")).toHaveTextContent("/authors/babbage"));
  });

  it("does not search until two characters are typed", async () => {
    const { fetchMock } = setup();
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "e" } });
    expect(screen.getByText("Keep typing to search your posts.")).toBeInTheDocument();
    // Long enough for a search to have started, had one been due.
    await new Promise((resolve) => setTimeout(resolve, 400));
    expect(fetchMock.mock.calls.some(([url]) => String(url).includes("/search/quick"))).toBe(false);
  });

  it("holds an early Enter until the results for those words arrive", async () => {
    const { onOpenBookmark } = setup();
    const field = screen.getByRole("combobox");
    fireEvent.change(field, { target: { value: "engine" } });
    // Pressed at once, before any search has run: nothing may open yet.
    fireEvent.keyDown(field, { key: "Enter" });
    expect(onOpenBookmark).not.toHaveBeenCalled();
    expect(screen.getByTestId("where")).toHaveTextContent("/");
    // Then the top result for "engine" opens, not a library search.
    await waitFor(() => expect(onOpenBookmark).toHaveBeenCalledWith(7, [7, 8]));
  });

  it("keeps focus in the search box and walks results with Tab", async () => {
    setup();
    const field = screen.getByRole("combobox");
    fireEvent.change(field, { target: { value: "engine" } });
    const first = await screen.findByRole("option", { name: /Ada Lovelace/ });
    expect(first).toHaveAttribute("aria-selected", "true");
    expect(first).toHaveAttribute("tabindex", "-1");
    fireEvent.keyDown(field, { key: "Tab" });
    expect(screen.getByRole("option", { name: /Grace Hopper/ })).toHaveAttribute("aria-selected", "true");
    fireEvent.keyDown(field, { key: "Tab", shiftKey: true });
    expect(first).toHaveAttribute("aria-selected", "true");
  });

  it("leads with a person when their name is what was typed", async () => {
    setup();
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "babb" } });
    const person = await screen.findByRole("option", { name: /Charles Babbage/ });
    expect(person).toHaveAttribute("aria-selected", "true");
  });
});
