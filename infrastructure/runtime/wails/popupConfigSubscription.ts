type PopupLeaseBindings = {
  GetConfig?: (popupId: string, token: string) => Promise<unknown>;
  Heartbeat?: (popupId: string, token: string) => Promise<unknown>;
};

/** Config is fetched only for this URL's authenticated popup identity. */
export function subscribePopupConfig(
  search: string,
  bindings: PopupLeaseBindings | undefined,
  receive: (config: unknown) => void,
  report: (error: unknown) => void = error => console.error('Popup configuration lease failed', error),
): () => void {
  const params = new URLSearchParams(search);
  const popupId = params.get('popupId');
  const token = params.get('popupToken');
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
