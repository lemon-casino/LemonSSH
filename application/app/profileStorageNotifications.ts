import { HOST_PROFILE_ERROR_EVENT } from '../../infrastructure/persistence/hostStorageAdapter';

/** The adapter logs diagnostic details; user notifications never read them. */
export function subscribeProfileStorageErrors(target: EventTarget, notify: () => void): () => void {
  let lastShown = -Infinity;
  const handler = () => {
    const now = Date.now();
    if (now - lastShown < 5000) return;
    lastShown = now;
    notify();
  };
  target.addEventListener(HOST_PROFILE_ERROR_EVENT, handler);
  return () => target.removeEventListener(HOST_PROFILE_ERROR_EVENT, handler);
}
