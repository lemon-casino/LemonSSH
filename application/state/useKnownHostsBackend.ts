import { useCallback } from "react";
import { lemonsshBridge } from "../../infrastructure/services/lemonsshBridge";

export const useKnownHostsBackend = () => {
  const readKnownHosts = useCallback(async () => {
    const bridge = lemonsshBridge.get();
    return bridge?.readKnownHosts?.();
  }, []);

  return { readKnownHosts };
};

