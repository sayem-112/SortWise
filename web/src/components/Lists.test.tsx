import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { ConfirmDialog } from "./ConfirmDialog";
import { ListMenu, NewListForm } from "./Lists";

const recipes = { id: 4, name: "Recipes", kind: "custom", icon: "chef-hat", color: "orange", pinned: false, count: 2 } as const;

function setup(ui: React.ReactNode) {
  const fetchMock = vi.fn(async (input: string, init?: RequestInit) => {
    if (init?.method === "POST") return { ok: true, json: async () => ({ ...recipes, id: 9, name: JSON.parse(String(init.body)).name }) };
    if (init?.method === "PATCH") return { ok: true, json: async () => ({ ...recipes, ...JSON.parse(String(init.body)) }) };
    if (init?.method === "DELETE") return { ok: true, json: async () => ({ bookmarkIds: [1, 2] }) };
    return { ok: true, json: async () => ({ items: [recipes] }) };
  });
  vi.stubGlobal("fetch", fetchMock);
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        {ui}
        <ConfirmDialog />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return fetchMock;
}

describe("lists", () => {
  it("saves a new list with the check button as well as Enter", async () => {
    const created = vi.fn();
    const fetchMock = setup(<NewListForm onCreated={created} onCancel={() => undefined} />);
    const save = screen.getByRole("button", { name: "Create list" });
    expect(save).toBeDisabled();
    fireEvent.change(screen.getByRole("textbox", { name: "New list name" }), { target: { value: "Running" } });
    fireEvent.click(save);
    await waitFor(() => expect(created).toHaveBeenCalledWith(expect.objectContaining({ name: "Running" })));
    expect(fetchMock.mock.calls.filter(([, init]) => init?.method === "POST")).toHaveLength(1);
  });

  it("pins a list, and deletes it only after confirming", async () => {
    const fetchMock = setup(<ListMenu list={recipes} />);
    fireEvent.click(screen.getByRole("button", { name: "Recipes options" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Pin to top" }));
    await waitFor(() => {
      const sent = fetchMock.mock.calls.find(([, init]) => init?.method === "PATCH");
      expect(JSON.parse(String(sent?.[1]?.body))).toEqual({ pinned: true });
    });

    fireEvent.click(screen.getByRole("button", { name: "Recipes options" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Delete list" }));
    expect(await screen.findByRole("alertdialog", { name: "Delete Recipes?" })).toHaveTextContent("Its 2 bookmarks stay in your library.");
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === "DELETE")).toBe(false);
    fireEvent.click(screen.getByRole("button", { name: "Delete list" }));
    await waitFor(() => expect(fetchMock.mock.calls.some(([url, init]) => init?.method === "DELETE" && String(url).endsWith("/lists/4"))).toBe(true));
  });
});
