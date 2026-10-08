import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { App } from "./App";

vi.stubGlobal("fetch", vi.fn(async (input: string) => ({
  ok: true,
  json: async () =>
    input.includes("setup")
      ? { bookmarks: 0, pendingJobs: 0, aiConfigured: false }
      : input.includes("settings/ai")
        ? { provider: "gemini", backup: true, paused: false, language: "English", providers: [], active: "", resting: [] }
        : { items: [], page: 1, pageSize: 4, total: 0 },
})));

describe("App", () => {
  it("renders the dashboard and onboarding", async () => {
    render(<QueryClientProvider client={new QueryClient()}><MemoryRouter><App /></MemoryRouter></QueryClientProvider>);
    expect(screen.getByRole("link", { name: "Sortwise home" })).toBeInTheDocument();
    expect(await screen.findByText("Connect the browser extension")).toBeInTheDocument();
  });
});
