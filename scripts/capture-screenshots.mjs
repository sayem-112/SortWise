// Captures the website's product screenshots from the demo, so they always
// show the app as it is. It drives a browser already on this computer
// (Microsoft Edge by default); nothing is downloaded.
//
//   1. npm run build:demo --workspace web
//   2. npm --prefix web exec -- vite preview web --mode demo --port 8792
//   3. node scripts/capture-screenshots.mjs [demo URL] [browser path]
import { mkdir, readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import puppeteer from "puppeteer-core";

const root = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const out = path.join(root, "site", "public", "images");
const base = (process.argv[2] || "http://localhost:8792/demo/").replace(/\/?$/, "/");
const browserPath = process.argv[3] || "C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe";

// The screenshots show the app, not the demo: no banner, no scrollbars.
const CLEAN = `.demo-banner{display:none!important} ::-webkit-scrollbar{display:none} *{scrollbar-width:none!important; caret-color:transparent!important}`;

const browser = await puppeteer.launch({
  executablePath: browserPath,
  headless: true,
  args: ["--hide-scrollbars", "--force-color-profile=srgb", "--lang=en-US"],
  // A laptop-sized window at twice the pixels, so text in the app reads at
  // the size the shots are shown on the page.
  defaultViewport: { width: 1280, height: 800, deviceScaleFactor: 2 },
});
await mkdir(out, { recursive: true });
// Whatever happens below, the browser is closed at the end.
process.on("exit", () => browser.process()?.kill());

async function open(route, setup) {
  const page = await browser.newPage();
  await page.emulateMediaFeatures([{ name: "prefers-reduced-motion", value: "reduce" }]);
  if (setup) await page.evaluateOnNewDocument(setup);
  await page.goto(new URL(route, base).href, { waitUntil: "networkidle0" });
  await page.addStyleTag({ content: CLEAN });
  await settle(page);
  return page;
}

// Waits for fonts, pictures, and any animation to finish. The sidebar's
// footer says what the installed app says, not what the demo says.
async function settle(page, extra = 400) {
  await page.evaluate(async () => {
    const [ai, library] = document.querySelectorAll(".sidebar-status span:last-child");
    if (ai) ai.textContent = "Organizing with Gemini";
    if (library) library.textContent = library.textContent.replace("sample posts in this demo", "bookmarks on this PC");
    await document.fonts.ready;
    // Only pictures on screen: those further down load lazily, maybe never.
    const onScreen = [...document.images].filter((image) => !image.complete && image.getBoundingClientRect().top < innerHeight);
    const loaded = Promise.all(onScreen.map((image) => new Promise((done) => ((image.onload = done), (image.onerror = done)))));
    await Promise.race([loaded, new Promise((done) => setTimeout(done, 8000))]);
    document.getAnimations().forEach((animation) => animation.finish());
  });
  await new Promise((done) => setTimeout(done, extra));
}

// height, when given, crops a page that does not fill the window.
async function shoot(page, name, height) {
  const file = path.join(out, `${name}.webp`);
  await page.screenshot({ path: file, type: "webp", quality: 82, ...(height ? { clip: { x: 0, y: 0, width: 1280, height } } : {}) });
  console.log("saved", path.relative(root, file));
  await page.close();
}

// A close-up: the part of the page between two elements, with some room around.
async function shootRegion(page, name, top, bottom, room = 32) {
  const clip = await page.evaluate(
    (topSelector, bottomSelector, pad) => {
      const first = document.querySelector(topSelector).getBoundingClientRect();
      const last = document.querySelector(bottomSelector).getBoundingClientRect();
      const left = Math.max(0, Math.min(first.left, last.left) - pad);
      const right = Math.min(innerWidth, Math.max(first.right, last.right) + pad);
      const y = Math.max(0, first.top - pad);
      // Page coordinates, not window ones: the page may have scrolled.
      return { x: left + scrollX, y: y + scrollY, width: right - left, height: Math.min(innerHeight, last.bottom + pad) - y };
    },
    top,
    bottom,
    room,
  );
  const file = path.join(out, `${name}.webp`);
  await page.screenshot({ path: file, type: "webp", quality: 82, clip });
  console.log("saved", path.relative(root, file), `${Math.round(clip.width * 2)}x${Math.round(clip.height * 2)}`);
  await page.close();
}

const clickRow = (text) => (page) =>
  page.evaluate((needle) => {
    const row = [...document.querySelectorAll(".library-table tbody tr")].find((item) => item.innerText.includes(needle));
    row?.click();
    return Boolean(row);
  }, text);

// 1. The library, as it first opens.
await shoot(await open("bookmarks"), "library");

// 2. A post open over the library: its AI summary, tags, and related posts.
{
  const page = await open("bookmarks", () => localStorage.setItem("bw.pageSize", "100"));
  if (!(await clickRow("Don't think of LLMs")(page))) throw new Error("post for the preview not found");
  await page.waitForSelector(".related-item");
  await settle(page, 700);
  await shoot(page, "preview");
}

// 2b. A close-up of one post, for "Every post, explained".
{
  const page = await open("bookmarks", () => localStorage.setItem("bw.pageSize", "100"));
  if (!(await clickRow("game management")(page))) throw new Error("post for the close-up not found");
  await page.waitForSelector(".related-item");
  await settle(page, 700);
  await shootRegion(page, "post", ".center-peek", ".center-peek", 0);
}

// 3. A search by meaning, with posts found without the words.
{
  const page = await open("bookmarks?q=habits&mode=meaning");
  await page.waitForSelector(".match-chip");
  await settle(page);
  await shootRegion(page, "meaning", ".view-bar", ".db-footer", 16);
}

// 4. The Ctrl+K finder with highlighted matches.
{
  const page = await open("bookmarks");
  await page.keyboard.down("Control");
  await page.keyboard.press("k");
  await page.keyboard.up("Control");
  await page.keyboard.type("llm", { delay: 60 });
  await page.waitForFunction(() => document.querySelectorAll(".command-label").length > 3);
  await settle(page, 600);
  await shootRegion(page, "finder", ".command-modal", ".command-modal", 0);
}

// 5. A smart collection saved from that search.
{
  const page = await open("bookmarks?q=habits&mode=meaning");
  await page.waitForSelector(".match-chip");
  await page.click(".save-collection > button");
  await page.waitForSelector(".save-collection-form input");
  await page.evaluate(() => {
    const input = document.querySelector(".save-collection-form input");
    input.select();
  });
  await page.keyboard.type("Healthy habits");
  await page.keyboard.press("Enter");
  await page.waitForFunction(() => location.pathname.includes("/collections/"));
  await page.waitForSelector(".library-table tbody tr");
  await page.evaluate(() => document.querySelectorAll(".toast").forEach((toast) => toast.remove()));
  await settle(page, 800);
  await page.evaluate(() => document.querySelectorAll(".toast, .toaster").forEach((toast) => toast.remove()));
  await shoot(page, "collection", 720);
}

// 6. Everyone you saved from, and one author's page.
await shoot(await open("authors"), "authors");
await shoot(await open("authors/karpathy"), "author");

// 7. The gallery view.
await shoot(await open("bookmarks", () => localStorage.setItem("bw.libraryView", "gallery")), "gallery");

// 8. The overview.
await shoot(await open(""), "overview");

// 9. The picture shown when the website is shared: the headline beside the app.
{
  const asData = async (file, type) => `data:${type};base64,${(await readFile(file)).toString("base64")}`;
  const font = (weight) => asData(path.join(root, "node_modules", "@fontsource", "chakra-petch", "files", `chakra-petch-latin-${weight}-normal.woff2`), "font/woff2");
  const page = await browser.newPage();
  await page.setViewport({ width: 1200, height: 630, deviceScaleFactor: 1 });
  await page.setContent(`<!doctype html><html><head><style>
    @font-face { font-family: Chakra; font-weight: 600; src: url(${await font(600)}) format("woff2"); }
    @font-face { font-family: Chakra; font-weight: 700; src: url(${await font(700)}) format("woff2"); }
    body { margin: 0; width: 1200px; height: 630px; overflow: hidden; background: #F8E9D1; font-family: Chakra; position: relative; }
    .sun { position: absolute; right: -120px; top: 60px; width: 640px; height: 640px; border-radius: 50%; background: #CC8800; }
    .stripe { position: absolute; right: 0; width: 620px; background: #F8E9D1; }
    .ground { position: absolute; left: 0; right: 0; bottom: 0; height: 70px; background: #9E4A2E; }
    .copy { position: absolute; left: 64px; top: 72px; width: 520px; }
    .brand { display: flex; align-items: center; gap: 14px; color: #1A0F05; font-size: 32px; font-weight: 700; }
    h1 { margin: 40px 0 0; color: #1A0F05; font-size: 68px; line-height: 1; letter-spacing: -1px; font-weight: 700; }
    p { margin: 24px 0 0; color: #4F3220; font-size: 26px; line-height: 1.35; font-weight: 600; }
    .shot { position: absolute; left: 640px; top: 118px; width: 640px; border: 4px solid #1A0F05; background: #191919; }
    .shot::before { content: ""; position: absolute; inset: 14px -14px -14px 14px; background: #1A0F05; z-index: -1; }
    .shot img { display: block; width: 100%; }
  </style></head><body>
    <div class="sun"></div>
    <div class="stripe" style="top:420px;height:10px"></div><div class="stripe" style="top:446px;height:14px"></div><div class="stripe" style="top:478px;height:18px"></div>
    <div class="ground"></div>
    <div class="copy">
      <div class="brand"><img src="${await asData(path.join(root, "site", "public", "favicon.svg"), "image/svg+xml")}" width="44" height="44">Sortwise</div>
      <h1>Save it on X.<br>Find it in seconds.</h1>
      <p>Your X bookmarks, saved on your PC and organized by AI.</p>
    </div>
    <div class="shot"><img src="${await asData(path.join(out, "preview.webp"), "image/webp")}"></div>
  </body></html>`, { waitUntil: "load" });
  await page.evaluate(() => document.fonts.ready);
  await page.screenshot({ path: path.join(out, "og.png"), type: "png" });
  console.log("saved", path.relative(root, path.join(out, "og.png")));
  await page.close();
}

await browser.close();
