import { createContext, useContext } from "react";

/* The post preview. There is only ever one, owned by the app shell, so a
   post opened from the finder replaces whatever a page had open instead of
   stacking on it. Pages open it with the posts beside it, for moving between
   them, and read `current` to know one is showing. */
export type Peek = {
  current: number | null;
  open: (id: number, ids: number[]) => void;
  // Moves to a post reached from the one showing. If it is among the posts
  // already beside the preview, the place in them is kept; otherwise `ids`
  // become the posts to move between.
  follow: (id: number, ids: number[]) => void;
  close: () => void;
};

export const PeekContext = createContext<Peek>({ current: null, open: () => undefined, follow: () => undefined, close: () => undefined });

export function usePeek() {
  return useContext(PeekContext);
}

/* True when the element is in the front-most dialog, or no dialog is open.
   Page-level shortcuts use it so a post under a preview or a confirmation
   doesn't react to keys meant for what is on top. */
export function isFrontmost(element: Element | null) {
  const dialogs = document.querySelectorAll("[aria-modal='true']");
  const front = dialogs[dialogs.length - 1];
  return !front || Boolean(element && front.contains(element));
}
