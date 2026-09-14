import { netcattyBridge } from '../../infrastructure/services/netcattyBridge';

/**
 * Clipboard writer for components: prefers the host bridge so Wails gets the
 * native clipboard, falling back to the web clipboard API. Throws when every
 * path fails so callers can surface the error.
 */
export function useClipboardWrite(): {
  writeText: (text: string) => Promise<void>;
} {
  return {
    writeText: async (text) => {
      const bridge = netcattyBridge.get();
      if (bridge?.writeClipboardText) {
        if (!await bridge.writeClipboardText(text)) throw new Error('Clipboard write failed');
        return;
      }
      await navigator.clipboard.writeText(text);
    },
  };
}
