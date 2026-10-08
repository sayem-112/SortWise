import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { BookmarkDrawer } from "./BookmarkDrawer";

const bookmark = (id: number, author: string) => ({
  id,
  postId: String(1000 + id),
  author,
  username: author.toLowerCase(),
  text: `Post ${id} about sourdough https://example.com/bread`,
  url: `https://x.com/${author}/status/${1000 + id}`,
  postedAt: "2026-09-01T10:00:00Z",
  importedAt: "2026-09-02 10:00:00",
  visibleContext: { quotedPost: { postId: "9", username: "baker", url: "https://x.com/baker/status/9", text: "Starter tips" } },
  media: [],
  summary: `Summary ${id}`,
  mediaDescription: "",
  processingStatus: "completed",
  archived: false,
  categories: [{ id: 1, name: "Food", manual: false }],
  tags: [{ id: 2, name: "fermentation", manual: false }],
});

vi.stubGlobal(
  "fetch",
  vi.fn(async (input: string) => ({
    ok: true,
    json: async () => {
      const match = input.match(/bookmarks\/(\d+)/);
      if (match) return bookmark(Number(match[1]), match[1] === "1" ? "Ada" : "Grace");
      return { items: [] };
    },
  })),
);

function Harness() {
  const [id, setId] = useState<number | null>(1);
  return <BookmarkDrawer bookmarkId={id} onClose={() => setId(null)} ids={[1, 2]} onNavigate={setId} />;
}

describe("bookmark peek", () => {
  it("shows the post, its AI summary, and moves through the list", async () => {
    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter>
          <Harness />
        </MemoryRouter>
      </QueryClientProvider>,
    );
    expect(await screen.findByRole("region", { name: "Post by Ada" })).toBeInTheDocument();
    expect(screen.getByText("Summary 1")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "AI summary" })).toBeInTheDocument();
    expect(screen.getByText("@baker")).toBeInTheDocument();
    expect(screen.getByText("Starter tips")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "example.com/bread" })).toHaveAttribute("href", "https://example.com/bread");
    expect(screen.getByText("1 of 2")).toBeInTheDocument();

    fireEvent.keyDown(window, { key: "j" });
    expect(await screen.findByRole("region", { name: "Post by Grace" })).toBeInTheDocument();

    fireEvent.keyDown(window, { key: "Escape" });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
});
