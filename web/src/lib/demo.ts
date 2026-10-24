/* The website demo: the app built to run in a browser with no server, on a
   library of public posts (see src/demo). Everything here is inert in the
   app itself, where DEMO is false and the demo code is left out of the build. */

export const DEMO = import.meta.env.MODE === "demo";

// Where "Download Sortwise" goes.
export const DOWNLOAD_URL = "https://github.com/sayem-112/SortWise/releases/latest";

/* Saves an export of the library. The app serves it; the demo builds it here. */
export async function downloadExport(file: "bookmarks.json" | "bookmarks.csv") {
  if (!DEMO) {
    window.location.href = `/api/v1/exports/${file}`;
    return;
  }
  const { exportLibrary } = await import("../demo/backend");
  const link = document.createElement("a");
  link.href = URL.createObjectURL(exportLibrary(file === "bookmarks.csv" ? "csv" : "json"));
  link.download = `sortwise-demo-${file}`;
  link.click();
  setTimeout(() => URL.revokeObjectURL(link.href), 1000);
}
