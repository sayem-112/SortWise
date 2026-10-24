// Builds the website into site-dist/: the landing page (the React app in
// site/) and the live demo (the web app built with --mode demo) under /demo/.
//
//   npm run build:site
import { execSync } from "node:child_process";
import { cpSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const out = path.join(root, "site-dist");

execSync("npm run build --workspace site", { cwd: root, stdio: "inherit" });
execSync("npm run build:demo --workspace web", { cwd: root, stdio: "inherit" });

rmSync(out, { recursive: true, force: true });
cpSync(path.join(root, "site", "dist"), out, { recursive: true });
cpSync(path.join(root, "web", "dist-demo"), path.join(out, "demo"), { recursive: true });

// Sites that show a preview of a shared link need the picture's full
// address. Vercel says where the site lives while it builds.
const host = process.env.SITE_URL || (process.env.VERCEL_PROJECT_PRODUCTION_URL && `https://${process.env.VERCEL_PROJECT_PRODUCTION_URL}`);
if (host) {
  const page = path.join(out, "index.html");
  const html = readFileSync(page, "utf8")
    .replace('content="/images/og.png"', `content="${host.replace(/\/$/, "")}/images/og.png"`)
    .replace("<!-- og:url -->", `<meta property="og:url" content="${host.replace(/\/$/, "")}/" />`);
  writeFileSync(page, html);
}

console.log("Website built in site-dist/");
