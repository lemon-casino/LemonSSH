import { useCallback } from 'react';

import { lemonsshBridge } from '../../infrastructure/services/lemonsshBridge';

/**
 * Opens a URL in the system browser (never the app WebView). Components use
 * this seam instead of importing the infrastructure bridge directly.
 */
export function useOpenExternal(): (url: string) => Promise<void> {
  return useCallback(async (url: string) => {
    const bridge = lemonsshBridge.get();
    const opener = bridge?.openExternal;
    if (!opener) {
      window.open(url, '_blank', 'noopener,noreferrer');
      return;
    }
    await opener(url);
  }, []);
}
