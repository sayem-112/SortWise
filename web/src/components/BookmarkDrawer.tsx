import { ChevronDown, ChevronUp, Maximize2 } from "lucide-react";
import { useEffect } from "react";
import { Link } from "react-router-dom";
import { BookmarkDocument } from "./BookmarkDocument";
import { CenterPeek } from "./ui";

interface BookmarkDrawerProps {
  bookmarkId: number | null;
  onClose: () => void;
  // The bookmarks in the list the peek was opened from, for previous and next.
  ids?: number[];
  onNavigate?: (id: number) => void;
}

function typing(target: EventTarget | null) {
  return target instanceof Element && Boolean(target.closest("input, textarea, [contenteditable='true'], .menu"));
}

/* Notion's center peek: the bookmark opens over the list, which stays in place.
   Up and down (or K and J) move through the list without closing it. */
export function BookmarkDrawer({ bookmarkId, onClose, ids = [], onNavigate }: BookmarkDrawerProps) {
  const index = bookmarkId ? ids.indexOf(bookmarkId) : -1;
  const previous = index > 0 ? ids[index - 1] : undefined;
  const next = index >= 0 && index < ids.length - 1 ? ids[index + 1] : undefined;

  useEffect(() => {
    if (!bookmarkId || !onNavigate) return;
    function onKey(event: KeyboardEvent) {
      if (event.metaKey || event.ctrlKey || event.altKey || typing(event.target)) return;
      if ((event.key === "ArrowUp" || event.key === "k") && previous) {
        event.preventDefault();
        onNavigate!(previous);
      }
      if ((event.key === "ArrowDown" || event.key === "j") && next) {
        event.preventDefault();
        onNavigate!(next);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [bookmarkId, onNavigate, previous, next]);

  return (
    <CenterPeek
      open={Boolean(bookmarkId)}
      onClose={onClose}
      label="Bookmark"
      toolbar={
        <>
          {onNavigate && index >= 0 && (
            <div className="peek-nav">
              <button type="button" className="icon-button" disabled={!previous} onClick={() => previous && onNavigate(previous)} aria-label="Previous bookmark" title="Previous (↑ or K)">
                <ChevronUp size={17} />
              </button>
              <button type="button" className="icon-button" disabled={!next} onClick={() => next && onNavigate(next)} aria-label="Next bookmark" title="Next (↓ or J)">
                <ChevronDown size={17} />
              </button>
              <span className="peek-position">
                {index + 1} of {ids.length}
              </span>
            </div>
          )}
          <Link to={`/bookmarks/${bookmarkId}`} className="ghost-button push" title="Open as a full page">
            <Maximize2 size={14} />
            Open as page
          </Link>
        </>
      }
    >
      {bookmarkId && <BookmarkDocument key={bookmarkId} bookmarkId={String(bookmarkId)} variant="peek" onDeleted={onClose} />}
    </CenterPeek>
  );
}
