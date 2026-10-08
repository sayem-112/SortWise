(function () {
  "use strict";

  const $ = (id) => document.getElementById(id);
  const { normalizeImportState, relativeTime } = SortwisePopupState;
  const DEFAULT_API_BASE = "http://127.0.0.1:8787/api/v1";
  const panels = ["loadingPanel", "offlinePanel", "pairingPanel", "scopePanel", "readyPanel", "runningPanel", "donePanel"];

  let paired = false;
  let busy = false;
  let summary = null;
  let importState = normalizeImportState();

  function api(path, method = "GET", body) {
    return chrome.runtime.sendMessage({ type: "api", path, method, body });
  }

  function appURL() {
    return ($("apiBase").value.trim() || DEFAULT_API_BASE).replace(/\/api\/v1\/?$/, "");
  }

  function show(panel) {
    for (const id of panels) $(id).hidden = id !== panel;
    $("footer").hidden = panel === "loadingPanel" || panel === "runningPanel";
    $("disconnect").hidden = !paired;
  }

  function badge(text, tone) {
    $("connectionBadge").textContent = text;
    $("connectionBadge").dataset.tone = tone;
  }

  function message(text) {
    $("message").textContent = text || "";
    $("message").hidden = !text;
  }

  function setCounts(prefix, state) {
    const ids = prefix ? ["doneDiscovered", "doneInserted", "doneUpdated", "doneUnchanged"] : ["discovered", "inserted", "updated", "unchanged"];
    [state.discovered, state.inserted, state.updated, state.unchanged].forEach((value, index) => {
      $(ids[index]).textContent = value.toLocaleString();
    });
  }

  function renderSummary() {
    if (!summary) return;
    const total = Number(summary.bookmarks) || 0;
    $("libraryCount").textContent = total.toLocaleString();
    $("libraryLabel").textContent = total === 1 ? "bookmark saved" : "bookmarks saved";
    const when = relativeTime(summary.lastImportAt);
    const added = Number(summary.lastImportInserted) || 0;
    $("lastImport").textContent = when || "Never";
    $("lastImportLabel").textContent = when && added ? `last saved · ${added.toLocaleString()} new` : "last saved";
  }

  function renderImport() {
    if (importState.running || importState.phase === "stopping") {
      badge(importState.auto ? "Checking" : "Syncing", "blue");
      $("runningTitle").textContent = importState.auto ? "Checking for new bookmarks" : "Syncing";
      $("runningStatus").textContent = importState.status;
      $("stop").disabled = busy || importState.phase === "stopping";
      setCounts("", importState);
      show("runningPanel");
      return true;
    }
    if (["complete", "stopped", "error"].includes(importState.phase) && !importState.acknowledged) {
      const tone = importState.phase === "error" ? "red" : importState.phase === "stopped" ? "gray" : "green";
      $("doneTitle").textContent = importState.phase === "error" ? "Sync failed" : importState.phase === "stopped" ? "Sync stopped" : "Sync finished";
      $("doneCallout").dataset.tone = tone;
      $("doneStatus").textContent = importState.status;
      $("openLibraryDone").hidden = importState.phase === "error";
      setCounts("done", importState);
      badge("Paired", "green");
      show("donePanel");
      return true;
    }
    return false;
  }

  // The first time (and whenever the user changes it): what syncs bring in.
  function renderScope(changing = false) {
    badge("Paired", "green");
    for (const input of document.querySelectorAll('input[name="scope"]')) input.checked = changing && input.value === summary?.scope;
    $("saveScope").disabled = !changing;
    $("saveScope").textContent = "Continue";
    $("cancelScope").hidden = !changing;
    show("scopePanel");
  }

  function renderReady() {
    badge("Paired", "green");
    renderSummary();
    $("scopeLabel").textContent = summary?.scope === "new" ? "only bookmarks from now on" : "all your X bookmarks";
    // "Everything" means everything since the starting line when scope is "new".
    document.querySelector(".note-full").textContent =
      summary?.scope === "new"
        ? "Reads everything you've bookmarked since you started and refreshes it."
        : "Reads your whole list and refreshes saved posts. Resumes if interrupted.";
    $("start").disabled = busy;
    $("start").textContent = busy ? "Starting…" : "Sync now";
    show("readyPanel");
  }

  async function refreshSummary() {
    const result = await api("/extension/summary");
    if (result?.ok) {
      summary = result.data;
      return "ok";
    }
    if (result?.status === 401) return "unpaired";
    if (result?.status === 0) return "offline";
    return "error";
  }

  async function initialize() {
    chrome.runtime.sendMessage({ type: "clear-badge" }).catch(() => {});
    const stored = await chrome.storage.local.get(["extensionToken", "apiBase", "importState", "importMode", "autoSave"]);
    if (stored.apiBase) $("apiBase").value = stored.apiBase;
    $("autoSave").checked = stored.autoSave !== false;
    if (stored.importMode === "full") document.querySelector('input[name="mode"][value="full"]').checked = true;
    importState = { ...normalizeImportState(stored.importState), acknowledged: stored.importState?.acknowledged === true };
    paired = Boolean(stored.extensionToken);
    await refresh();
  }

  async function refresh() {
    message("");
    const health = await api("/health");
    if (!health?.ok) {
      badge("App offline", "orange");
      show("offlinePanel");
      return;
    }
    if (!paired) {
      badge("Not paired", "gray");
      show("pairingPanel");
      return;
    }
    const status = await refreshSummary();
    if (status === "unpaired") {
      // The app revoked this token (for example, "Disconnect all" in Settings).
      paired = false;
      await chrome.storage.local.remove("extensionToken");
      badge("Not paired", "gray");
      show("pairingPanel");
      message("This extension was disconnected from the app. Pair it again to keep saving bookmarks.");
      return;
    }
    if (status === "offline") {
      badge("App offline", "orange");
      show("offlinePanel");
      return;
    }
    if (renderImport()) return;
    if (!summary?.scope) renderScope();
    else renderReady();
  }

  for (const input of document.querySelectorAll('input[name="scope"]')) {
    input.addEventListener("change", () => ($("saveScope").disabled = false));
  }

  $("changeScope").addEventListener("click", () => renderScope(true));
  $("cancelScope").addEventListener("click", () => renderReady());

  $("saveScope").addEventListener("click", async () => {
    const scope = document.querySelector('input[name="scope"]:checked')?.value;
    if (!scope || busy) return;
    busy = true;
    $("saveScope").disabled = true;
    $("saveScope").textContent = scope === "new" ? "Marking your starting point…" : "Saving…";
    message("");
    const response = await chrome.runtime.sendMessage({ type: "set-scope", scope });
    busy = false;
    if (!response?.ok) {
      $("saveScope").disabled = false;
      $("saveScope").textContent = "Continue";
      message(response?.error || "Could not save your choice.");
      return;
    }
    await refreshSummary();
    if (scope === "all") {
      // Bring the whole list in now, the way the user just asked.
      document.querySelector('input[name="mode"][value="full"]').checked = true;
      const started = await chrome.runtime.sendMessage({ type: "start-import", mode: "full" });
      if (started?.ok) {
        importState = normalizeImportState(started.state);
        renderImport();
        return;
      }
    }
    renderReady();
  });

  $("retry").addEventListener("click", () => {
    show("loadingPanel");
    void refresh();
  });

  $("openPairing").addEventListener("click", () => {
    chrome.tabs.create({ url: `${appURL()}/settings#extension` });
  });

  for (const id of ["openLibrary", "openLibraryDone"]) {
    $(id).addEventListener("click", () => chrome.tabs.create({ url: `${appURL()}/bookmarks` }));
  }

  $("pairingCode").addEventListener("keydown", (event) => {
    if (event.key === "Enter") $("pair").click();
  });

  $("pair").addEventListener("click", async () => {
    const code = $("pairingCode").value.trim();
    if (!/^\d{6,8}$/.test(code)) {
      message("Enter the 6-digit code shown in Sortwise's Settings.");
      return;
    }
    const apiBase = $("apiBase").value.trim().replace(/\/$/, "");
    if (!/^http:\/\/127\.0\.0\.1:\d+\/api\/v1$/.test(apiBase)) {
      message("Use a loopback address such as http://127.0.0.1:8787/api/v1.");
      return;
    }
    busy = true;
    $("pair").disabled = true;
    message("");
    try {
      await chrome.storage.local.set({ apiBase });
      const result = await api("/extension/pair", "POST", { code });
      if (!result?.ok || !result.data?.token) {
        throw new Error(result?.status === 0 ? "Sortwise isn't running." : result?.data?.error?.message || "Pairing failed.");
      }
      await chrome.storage.local.set({ extensionToken: result.data.token });
      paired = true;
      $("pairingCode").value = "";
      await refresh();
    } catch (error) {
      message(error instanceof Error ? error.message : "Pairing failed.");
    } finally {
      busy = false;
      $("pair").disabled = false;
    }
  });

  $("autoSave").addEventListener("change", () => chrome.storage.local.set({ autoSave: $("autoSave").checked }));

  for (const input of document.querySelectorAll('input[name="mode"]')) {
    input.addEventListener("change", () => chrome.storage.local.set({ importMode: input.value }));
  }

  $("start").addEventListener("click", async () => {
    if (busy) return;
    busy = true;
    renderReady();
    message("");
    const mode = document.querySelector('input[name="mode"]:checked')?.value === "full" ? "full" : "new";
    // The service worker runs the sync, so it keeps going after this popup closes.
    const response = await chrome.runtime.sendMessage({ type: "start-import", mode });
    busy = false;
    if (!response?.ok) {
      renderReady();
      message(response?.error || "The sync could not start.");
      return;
    }
    importState = normalizeImportState(response.state);
    renderImport();
  });

  $("stop").addEventListener("click", async () => {
    busy = true;
    renderImport();
    const response = await chrome.runtime.sendMessage({ type: "stop-import" });
    busy = false;
    if (response?.state) importState = normalizeImportState(response.state);
    renderImport();
  });

  $("again").addEventListener("click", async () => {
    importState = { ...importState, acknowledged: true };
    await chrome.storage.local.set({ importState });
    await refresh();
  });

  $("disconnect").addEventListener("click", async () => {
    if (importState.running) return;
    await api("/extension/pairing", "DELETE").catch(() => {});
    await chrome.storage.local.remove(["extensionToken", "importState"]);
    paired = false;
    importState = normalizeImportState();
    await refresh();
    message("Disconnected. Create a new code in the app to pair again.");
  });

  chrome.runtime.onMessage.addListener((incoming) => {
    if (incoming?.type !== "import-progress") return;
    importState = { ...normalizeImportState(incoming), acknowledged: incoming.acknowledged === true };
    if (!importState.running) void refreshSummary().then(renderSummary);
    if (!renderImport()) renderReady();
  });

  void initialize();
})();
