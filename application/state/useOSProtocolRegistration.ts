import { useCallback, useEffect, useState } from "react";

import { netcattyBridge } from "../../infrastructure/services/netcattyBridge";

/**
 * OS protocol handoff (SYS-03): whether ssh://, telnet:// and netcatty://
 * currently open LemonSSH, plus the toggle that registers or removes the
 * schemes. Windows writes HKCU\Software\Classes (no elevation); other
 * platforms fail closed with the bridge's own message.
 */
export function useOSProtocolRegistration() {
  const [registered, setRegistered] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [supported, setSupported] = useState(true);

  const refresh = useCallback(async () => {
    const result = await netcattyBridge.get()?.getOSProtocolStatus?.();
    if (!result) {
      setSupported(false);
      return;
    }
    if (result.success) {
      setRegistered(Boolean(result.registered));
      setError(null);
    } else if (result.error) {
      setError(result.error);
    }
  }, []);

  const setEnabled = useCallback(async (enabled: boolean) => {
    setBusy(true);
    try {
      const result = await netcattyBridge.get()?.setOSProtocol?.(enabled);
      if (result?.success) {
        setRegistered(Boolean(result.registered));
        setError(null);
      } else {
        setError(result?.error ?? "unavailable");
      }
    } finally {
      setBusy(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  return { registered, busy, error, supported, setEnabled, refresh };
}
