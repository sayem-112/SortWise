/* A tiny store for the one toast the app shows at a time: a short
   confirmation, optionally with an Undo. */

export type Toast = {
  id: number;
  message: string;
  tone?: "error";
  action?: { label: string; run: () => void };
};

type Listener = (toast: Toast | null) => void;

let current: Toast | null = null;
let nextId = 1;
const listeners = new Set<Listener>();

export function showToast(toast: Omit<Toast, "id">) {
  current = { ...toast, id: nextId++ };
  listeners.forEach((listener) => listener(current));
}

export function dismissToast(id: number) {
  if (current?.id !== id) return;
  current = null;
  listeners.forEach((listener) => listener(null));
}

export function subscribeToast(listener: Listener) {
  listeners.add(listener);
  listener(current);
  return () => {
    listeners.delete(listener);
  };
}
