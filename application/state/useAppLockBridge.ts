import { useCallback } from 'react';

import { lemonsshBridge } from '../../infrastructure/services/lemonsshBridge';
import type { RuntimeAppLockReason } from './useAppLockRuntime';

let rendererReadySent = false;

export function useAppLockBridge() {
  const getRuntimeState = useCallback(async () => {
    return lemonsshBridge.get()?.getAppLockRuntimeState?.();
  }, []);

  const getSettings = useCallback(async () => {
    return lemonsshBridge.get()?.getAppLockSettings?.();
  }, []);

  const setRuntimeLocked = useCallback(async (reason: Exclude<RuntimeAppLockReason, null>) => {
    return lemonsshBridge.get()?.setAppLockRuntimeLocked?.(reason);
  }, []);

  const requestUnlock = useCallback(async (password: string) => {
    return lemonsshBridge.get()?.requestAppLockUnlock?.(password) ?? { ok: false as const, error: 'incorrect' as const };
  }, []);

  const requestReset = useCallback(async (currentPassword: string) => {
    return lemonsshBridge.get()?.requestAppLockReset?.(currentPassword);
  }, []);

  const reportActivity = useCallback(async () => {
    await lemonsshBridge.get()?.reportAppLockActivity?.();
  }, []);

  const onRuntimeStateChanged = useCallback((listener: (state: unknown) => void) => {
    try {
      return lemonsshBridge.get()?.onAppLockRuntimeStateChanged?.(listener) ?? (() => {});
    } catch {
      return () => {};
    }
  }, []);

  const onSettingsChanged = useCallback((listener: (settings: unknown) => void) => {
    try {
      return lemonsshBridge.get()?.onAppLockSettingsChanged?.(listener) ?? (() => {});
    } catch {
      return () => {};
    }
  }, []);

  const notifyRendererReady = useCallback(() => {
    if (rendererReadySent) return;
    rendererReadySent = true;
    try {
      lemonsshBridge.get()?.rendererReady?.();
    } catch {
      // ignore
    }
  }, []);

  const onAppLockReopen = useCallback((listener: () => void) => {
    try {
      return lemonsshBridge.get()?.onAppLockReopen?.(listener) ?? (() => {});
    } catch {
      return () => {};
    }
  }, []);

  return {
    getRuntimeState,
    getSettings,
    setRuntimeLocked,
    requestUnlock,
    requestReset,
    reportActivity,
    onRuntimeStateChanged,
    onSettingsChanged,
    notifyRendererReady,
    onAppLockReopen,
  };
}
