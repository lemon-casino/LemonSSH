type LeaseBindings = {
  GetConfig?: (id: string, token: string) => Promise<unknown>;
  Heartbeat?: (id: string, token: string) => Promise<unknown>;
};

/** URL query parameter names carrying the window identity. */
export interface LeaseParams {
  id: string;
  token: string;
}

/** The terminal popup's identity parameters (popupWindowService.go). */
export const POPUP_LEASE_PARAMS: LeaseParams = { id: 'popupId', token: 'popupToken' };

/**
 * Config is fetched only for this URL's authenticated window identity, and a
 * periodic heartbeat keeps the Go-side lease alive (a renderer that stops
 * answering lets its window be closed as crashed). Shared by the terminal
 * popup (#/terminal-popup) and the peer session windows (#/session-window);
 * each route passes its own identity parameter names.
 */
export function subscribePopupConfig(
  search: string,
  bindings: LeaseBindings | undefined,
  receive: (config: unknown) => void,
  report: (error: unknown) => void = error => console.error('Popup configuration lease failed', error),
  params: LeaseParams = POPUP_LEASE_PARAMS,
): () => void {
  const searchParams = new URLSearchParams(search);
  const popupId = searchParams.get(params.id);
  const token = searchParams.get(params.token);
  if (!popupId || !token || !bindings?.GetConfig || !bindings.Heartbeat) return () => undefined;
  let disposed = false;
  let timer: ReturnType<typeof setInterval> | undefined;
  let heartbeatPending = false;
  const dispose = () => {
    disposed = true;
    if (timer !== undefined) clearInterval(timer);
    timer = undefined;
  };
  const fail = (error: unknown) => {
    if (disposed) return;
    dispose();
    report(error);
  };
  const heartbeat = () => {
    if (disposed || heartbeatPending) return;
    heartbeatPending = true;
    try {
      void bindings.Heartbeat!(popupId, token).catch(fail).finally(() => { heartbeatPending = false; });
    } catch (error) { heartbeatPending = false; fail(error); }
  };
  timer = setInterval(heartbeat, 5000);
  heartbeat();
  if (!disposed) {
    try {
      void bindings.GetConfig(popupId, token).then(config => {
        if (!disposed) receive(config);
      }).catch(fail);
    } catch (error) { fail(error); }
  }
  return dispose;
}
