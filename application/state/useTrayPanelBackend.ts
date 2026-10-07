import { useCallback } from "react";
import { lemonsshBridge } from "../../infrastructure/services/lemonsshBridge";

export const useTrayPanelBackend = () => {
  const hideTrayPanel = useCallback(async () => {
    const bridge = lemonsshBridge.get();
    await bridge?.hideTrayPanel?.();
  }, []);

  const openMainWindow = useCallback(async () => {
    const bridge = lemonsshBridge.get();
    await bridge?.openMainWindow?.();
  }, []);

  const quitApp = useCallback(async () => {
    const bridge = lemonsshBridge.get();
    await bridge?.quitApp?.();
  }, []);

  const jumpToSession = useCallback(async (sessionId: string) => {
    const bridge = lemonsshBridge.get();
    await bridge?.jumpToSessionFromTrayPanel?.(sessionId);
  }, []);

  const closeSessionFromTrayPanel = useCallback(async (sessionId: string) => {
    const bridge = lemonsshBridge.get();
    await bridge?.closeSessionFromTrayPanel?.(sessionId);
  }, []);

  const connectToHostFromTrayPanel = useCallback(async (hostId: string) => {
    const bridge = lemonsshBridge.get();
    await bridge?.connectToHostFromTrayPanel?.(hostId);
  }, []);

  const onTrayPanelCloseRequest = useCallback((callback: () => void) => {
    const bridge = lemonsshBridge.get();
    return bridge?.onTrayPanelCloseRequest?.(callback);
  }, []);

  const onTrayPanelRefresh = useCallback((callback: () => void) => {
    const bridge = lemonsshBridge.get();
    return bridge?.onTrayPanelRefresh?.(callback);
  }, []);

  const onTrayPanelMenuData = useCallback(
    (
      callback: (data: {
        sessions?: Array<{ id: string; label: string; hostLabel: string; status: "connecting" | "connected" | "disconnected"; workspaceId?: string; workspaceTitle?: string }>;
        portForwardRules?: Array<{
          id: string;
          label: string;
          type: "local" | "remote" | "dynamic";
          localPort: number;
          remoteHost?: string;
          remotePort?: number;
          status: "inactive" | "connecting" | "active" | "error";
          hostId?: string;
          canStop?: boolean;
        }>;
      }) => void,
    ) => {
      const bridge = lemonsshBridge.get();
      return bridge?.onTrayPanelMenuData?.(callback);
    },
    [],
  );

  return {
    hideTrayPanel,
    openMainWindow,
    quitApp,
    jumpToSession,
    closeSessionFromTrayPanel,
    connectToHostFromTrayPanel,
    onTrayPanelCloseRequest,
    onTrayPanelRefresh,
    onTrayPanelMenuData,
  };
};
