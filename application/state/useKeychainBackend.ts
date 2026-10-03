import { useCallback } from "react";
import { lemonsshBridge } from "../../infrastructure/services/lemonsshBridge";

export const useKeychainBackend = () => {
  const generateKeyPair = useCallback(async (options: { type: "RSA" | "ECDSA" | "ED25519"; bits?: number; comment?: string }) => {
    const bridge = lemonsshBridge.get();
    return bridge?.generateKeyPair?.(options);
  }, []);

  const execCommand = useCallback(async (options: Parameters<LemonSSHBridge["execCommand"]>[0]) => {
    const bridge = lemonsshBridge.get();
    if (!bridge?.execCommand) throw new Error("execCommand unavailable");
    return bridge.execCommand(options);
  }, []);

  return { generateKeyPair, execCommand };
};
