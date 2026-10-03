import { useCallback } from "react";
import { lemonsshBridge } from "../../infrastructure/services/lemonsshBridge";

export function subscribeWindowFullscreenChanged(
  cb: (isFullscreen: boolean) => void,
): () => void {
  try {
    return lemonsshBridge.get()?.onWindowFullScreenChanged?.(cb) ?? (() => {});
  } catch {
    return () => {};
  }
}

export const useWindowControls = () => {
  const notifyRendererReady = useCallback(() => {
    try {
      lemonsshBridge.get()?.rendererReady?.();
    } catch {
      // ignore
    }
  }, []);

  const notifySettingsPainted = useCallback(() => {
    try {
      void (lemonsshBridge.get() as { notifySettingsPainted?: () => Promise<unknown> } | undefined)
        ?.notifySettingsPainted?.();
    } catch {
      // ignore
    }
  }, []);

  const closeSettingsWindow = useCallback(async () => {
    const bridge = lemonsshBridge.get();
    await bridge?.closeSettingsWindow?.();
  }, []);

  const openSettingsWindow = useCallback(async () => {
    const bridge = lemonsshBridge.get();
    return bridge?.openSettingsWindow?.();
  }, []);

  const minimize = useCallback(async () => {
    const bridge = lemonsshBridge.get();
    await bridge?.windowMinimize?.();
  }, []);

  const maximize = useCallback(async () => {
    const bridge = lemonsshBridge.get();
    return bridge?.windowMaximize?.();
  }, []);

  const close = useCallback(async () => {
    const bridge = lemonsshBridge.get();
    await bridge?.windowClose?.();
  }, []);

  const quit = useCallback(async () => {
    const bridge = lemonsshBridge.get();
    await bridge?.quitApp?.();
  }, []);

  const isMaximized = useCallback(async () => {
    const bridge = lemonsshBridge.get();
    return bridge?.windowIsMaximized?.();
  }, []);

  const isFullscreen = useCallback(async () => {
    const bridge = lemonsshBridge.get();
    return bridge?.windowIsFullscreen?.() ?? false;
  }, []);

  const onFullscreenChanged = useCallback(subscribeWindowFullscreenChanged, []);

  const onWindowCommandCloseRequested = useCallback((cb: () => void) => {
    const bridge = lemonsshBridge.get();
    return bridge?.onWindowCommandCloseRequested?.(cb) ?? (() => {});
  }, []);

  return {
    notifyRendererReady,
    notifySettingsPainted,
    closeSettingsWindow,
    openSettingsWindow,
    minimize,
    maximize,
    close,
    quit,
    isMaximized,
    isFullscreen,
    onFullscreenChanged,
    onWindowCommandCloseRequested,
  };
};
