import { ChevronDown, ChevronUp } from "lucide-react";
import { useEffect, useMemo } from "react";
import { useLocation, useNavigate, useParams } from "react-router-dom";
import { BookmarkDocument } from "../components/BookmarkDocument";

// What a page opened from a list carries along: the list's posts, in order.
export type PostTrail = { ids: number[] };

function typing(target: EventTarget | null) {
  return target instanceof Element && Boolean(target.closest("input, textarea, [contenteditable='true'], .menu"));
}

export function BookmarkDetailPage() {
  const { id = "" } = useParams();
  const navigate = useNavigate();
  const location = useLocation();
  const trail = useMemo(() => (location.state as PostTrail | null)?.ids ?? [], [location.state]);
  const index = trail.indexOf(Number(id));
  const previous = index > 0 ? trail[index - 1] : undefined;
  const next = index >= 0 && index < trail.length - 1 ? trail[index + 1] : undefined;

  // Each post starts at its top, however far down the last one was read.
  useEffect(() => {
    window.scrollTo(0, 0);
  }, [id]);

  useEffect(() => {
    // Moving along the list replaces the page, so Back still returns to the list.
    const go = (target: number) => navigate(`/bookmarks/${target}`, { replace: true, state: { ids: trail } });
    function onKey(event: KeyboardEvent) {
      if (event.defaultPrevented || event.metaKey || event.ctrlKey || event.altKey || typing(event.target)) return;
      // Anything open on top of the page keeps the keys for itself.
      if (document.querySelector(".center-peek, .command-backdrop, .confirm-backdrop, .menu")) return;
      if ((event.key === "ArrowUp" || event.key === "k") && previous) {
        event.preventDefault();
        go(previous);
      } else if ((event.key === "ArrowDown" || event.key === "j") && next) {
        event.preventDefault();
        go(next);
      } else if (event.key === "Escape") {
        event.preventDefault();
        // Back to wherever the page was opened from; straight to the library when it was opened directly.
        if (location.key !== "default") navigate(-1);
        else navigate("/bookmarks");
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [navigate, location.key, trail, previous, next]);

  return (
    <main className="page bookmark-page">
      {index >= 0 && (
        <div className="page-trail">
          <button
            type="button"
            className="icon-button"
            disabled={!previous}
            onClick={() => previous && navigate(`/bookmarks/${previous}`, { replace: true, state: { ids: trail } })}
            aria-label="Previous bookmark"
            title="Previous (↑ or K)"
          >
            <ChevronUp size={17} />
          </button>
          <button
            type="button"
            className="icon-button"
            disabled={!next}
            onClick={() => next && navigate(`/bookmarks/${next}`, { replace: true, state: { ids: trail } })}
            aria-label="Next bookmark"
            title="Next (↓ or J)"
          >
            <ChevronDown size={17} />
          </button>
          <span className="peek-position">
            {index + 1} of {trail.length}
          </span>
        </div>
      )}
      <BookmarkDocument key={id} bookmarkId={id} variant="page" onDeleted={() => navigate("/bookmarks")} remember />
    </main>
  );
}
