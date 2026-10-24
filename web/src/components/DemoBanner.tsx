import { ArrowLeft, ArrowUpRight, RotateCcw } from "lucide-react";
import { Link } from "react-router-dom";
import { DOWNLOAD_URL } from "../lib/demo";
import { confirmAction } from "../lib/confirm";
import { modKey } from "../lib/format";

/* Says, above every page of the website demo, what this is, where to try
   the things it does best, and where the real thing is. */
export function DemoBanner() {
  return (
    <div className="demo-banner" role="note">
      <a className="demo-banner-home" href="/" title="Back to the Sortwise website">
        <ArrowLeft size={13} aria-hidden="true" />
        <span>Website</span>
      </a>
      <span className="demo-banner-text">
        <strong>Live demo.</strong> Try <Link to="/bookmarks?q=habits&mode=meaning">a search by meaning</Link>, or press{" "}
        <kbd>{modKey}K</kbd>.
      </span>
      <span className="demo-banner-actions">
        <button
          type="button"
          className="demo-banner-reset"
          onClick={() => {
            void confirmAction({ title: "Reset the demo?", message: "Your stars, lists, collections, and edits are cleared, and the demo starts over.", confirmLabel: "Reset demo" }).then(async (ok) => {
              if (!ok) return;
              const { resetDemo } = await import("../demo/backend");
              resetDemo();
              // The app's own remembered choices start over too: recent posts, views, the Meaning switch.
              try {
                for (const key of Object.keys(localStorage)) if (key.startsWith("bw.")) localStorage.removeItem(key);
              } catch {
                /* storage unavailable: nothing was remembered */
              }
              window.location.assign(import.meta.env.BASE_URL);
            });
          }}
        >
          <RotateCcw size={13} aria-hidden="true" />
          Reset
        </button>
        <a className="demo-banner-download" href={DOWNLOAD_URL} target="_blank" rel="noreferrer">
          Download Sortwise
          <ArrowUpRight size={13} aria-hidden="true" />
        </a>
      </span>
    </div>
  );
}
