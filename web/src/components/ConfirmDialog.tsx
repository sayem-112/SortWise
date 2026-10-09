import { useEffect, useRef, useState } from "react";
import { answerConfirm, subscribeConfirm, type ConfirmRequest } from "../lib/confirm";

/* The app's confirmation dialog. Escape or a click outside cancels; Tab stays
   inside the two buttons. */
export function ConfirmDialog() {
  const [request, setRequest] = useState<ConfirmRequest | null>(null);
  const cancel = useRef<HTMLButtonElement>(null);
  const confirm = useRef<HTMLButtonElement>(null);
  const opener = useRef<Element | null>(null);

  useEffect(
    () =>
      subscribeConfirm((next) => {
        if (next) opener.current = document.activeElement;
        setRequest(next);
      }),
    [],
  );

  useEffect(() => {
    if (!request) return;
    (request.danger ? cancel : confirm).current?.focus();
    function onKey(event: KeyboardEvent) {
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        answerConfirm(false);
      }
      if (event.key === "Tab") {
        event.preventDefault();
        (document.activeElement === cancel.current ? confirm : cancel).current?.focus();
      }
    }
    document.addEventListener("keydown", onKey, true);
    return () => {
      document.removeEventListener("keydown", onKey, true);
      if (opener.current instanceof HTMLElement && opener.current.isConnected) opener.current.focus();
    };
  }, [request]);

  if (!request) return null;
  return (
    <div className="confirm-backdrop" onPointerDown={(event) => event.target === event.currentTarget && answerConfirm(false)}>
      <div className="confirm-dialog" role="alertdialog" aria-modal="true" aria-labelledby="confirm-title" aria-describedby={request.message ? "confirm-message" : undefined}>
        <h2 id="confirm-title">{request.title}</h2>
        {request.message && <p id="confirm-message">{request.message}</p>}
        <div className="confirm-actions">
          <button ref={cancel} type="button" className="button secondary" onClick={() => answerConfirm(false)}>
            Cancel
          </button>
          <button ref={confirm} type="button" className={`button ${request.danger ? "secondary danger" : "primary"}`} onClick={() => answerConfirm(true)}>
            {request.confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
