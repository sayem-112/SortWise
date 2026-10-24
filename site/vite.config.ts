import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// The landing page. The live demo is the web app, built separately and
// placed under /demo/ by scripts/build-site.mjs.
export default defineConfig({
  plugins: [react()],
  build: { outDir: "dist", emptyOutDir: true },
});
