import { useCallback } from 'react';
import { lemonsshBridge } from '../../infrastructure/services/lemonsshBridge';
import type { TerminalPopupPayload } from '../../domain/systemManager/types';

export function useTerminalPopupWindow() {
  const close = useCallback(async () => {
    await lemonsshBridge.get()?.windowClose?.();
  }, []);

  const setWindowTitle = useCallback(async (title: string) => {
    await lemonsshBridge.get()?.setWindowTitle?.(title);
  }, []);

  const onPopupConfig = useCallback((cb: (payload: TerminalPopupPayload) => void) => {
    const bridge = lemonsshBridge.get();
    if (!bridge?.onTerminalPopupConfig) return () => {};
    return bridge.onTerminalPopupConfig(cb);
  }, []);

  const markAttachClosePrepared = useCallback(async (sessionId: string, authorization: string) => {
    return lemonsshBridge.get()?.markAttachPopupClosePrepared?.(sessionId, authorization);
  }, []);

  const onPrepareClose = useCallback((cb: (payload: { sessionId: string; authorization: string }) => void) => {
    return lemonsshBridge.get()?.onTerminalPopupPrepareClose?.(cb) ?? (() => {});
  }, []);

  return { close, setWindowTitle, onPopupConfig, markAttachClosePrepared, onPrepareClose };
}
