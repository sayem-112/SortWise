/* Asks the user to confirm an action in the app's own dialog, replacing the
   browser's confirm box. Resolves true when they confirm. */

export type ConfirmRequest = {
  title: string;
  message?: string;
  confirmLabel: string;
  // Destructive actions get red text and start with Cancel focused.
  danger?: boolean;
};

type Pending = ConfirmRequest & { resolve: (ok: boolean) => void };
type Listener = (request: Pending | null) => void;

let pending: Pending | null = null;
const listeners = new Set<Listener>();

function publish(next: Pending | null) {
  pending = next;
  listeners.forEach((listener) => listener(pending));
}

export function confirmAction(request: ConfirmRequest): Promise<boolean> {
  pending?.resolve(false);
  return new Promise((resolve) => publish({ ...request, resolve }));
}

export function answerConfirm(ok: boolean) {
  const current = pending;
  publish(null);
  current?.resolve(ok);
}

export function subscribeConfirm(listener: Listener) {
  listeners.add(listener);
  listener(pending);
  return () => {
    listeners.delete(listener);
  };
}
