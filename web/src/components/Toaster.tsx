import { X } from "lucide-react";
import { useEffect, useState } from "react";
import { dismissToast, subscribeToast, type Toast } from "../lib/toast";

const VISIBLE_MS = 6000;

function typing(target: EventTarget | null) {
  return target instanceof Element && Boolean(target.closest("input, textarea, [contenteditable='true']"));
}

/* The one toast at a time, bottom center. It stays while hovered or focused
   so there is always time to press Undo, and Ctrl+Z presses it too. */
export function Toaster() {
  const [toast, setToast] = useState<Toast | null>(null);
  const [held, setHeld] = useState(false);

  useEffect(() => subscribeToast(setToast), []);

  useEffect(() => {
    if (!toast || held) return;
    const timer = window.setTimeout(() => dismissToast(toast.id), VISIBLE_MS);
    return () => window.clearTimeout(timer);
  }, [toast, held]);

  useEffect(() => {
    const action = toast?.action;
    if (!toast || !action) return;
    const id = toast.id;
    function onKey(event: KeyboardEvent) {
      if ((event.ctrlKey || event.metaKey) && !event.shiftKey && event.key.toLowerCase() === "z" && !typing(event.target)) {
        event.preventDefault();
        action!.run();
        dismissToast(id);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [toast]);

  return (
    <div className="toaster" role="status" aria-live="polite">
      {toast && (
        <div
          key={toast.id}
          className={`toast ${toast.tone === "error" ? "error" : ""}`}
          onMouseEnter={() => setHeld(true)}
          onMouseLeave={() => setHeld(false)}
          onFocus={() => setHeld(true)}
          onBlur={() => setHeld(false)}
        >
          <span className="toast-message">{toast.message}</span>
          {toast.action && (
            <button
              type="button"
              className="toast-action"
              title="Undo (Ctrl+Z)"
              onClick={() => {
                toast.action!.run();
                dismissToast(toast.id);
              }}
            >
              {toast.action.label}
            </button>
          )}
          <button type="button" className="icon-button toast-close" aria-label="Dismiss" onClick={() => dismissToast(toast.id)}>
            <X size={14} />
          </button>
        </div>
      )}
    </div>
  );
}
