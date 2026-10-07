import { useCallback } from "react";
import { lemonsshBridge } from "../../infrastructure/services/lemonsshBridge";

export const useClipboardBackend = () => {
  const readClipboardText = useCallback(async (): Promise<string> => {
    const bridge = lemonsshBridge.get();
    if (!bridge?.readClipboardText) throw new Error("clipboard bridge unavailable");

    const text = await bridge.readClipboardText();
    return typeof text === "string" ? text : "";
  }, []);

  return { readClipboardText };
};
