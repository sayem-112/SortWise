import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// `vite build --mode demo` makes the website demo: the app with no server,
// served from /demo/ (see src/lib/demo.ts).
// The demo's page says what it is in search results and shared links.
const demoPage = {
  name: "demo-page",
  transformIndexHtml: (html: string) =>
    html
      .replace("<title>Sortwise</title>", "<title>Sortwise live demo</title>")
      .replace(
        "</head>",
        `    <meta name="description" content="Try Sortwise in your browser: 83 real public posts from X, summarized, tagged, and searchable by meaning." />\n  </head>`,
      ),
};

export default defineConfig(({ mode }) => ({
  plugins: [react(), tailwindcss(), ...(mode === "demo" ? [demoPage] : [])],
  base: mode === "demo" ? "/demo/" : "/",
  build: {
    outDir: mode === "demo" ? "dist-demo" : "dist",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/api": "http://127.0.0.1:8787",
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: "./src/test/setup.ts",
  },
}));
