import { useEffect } from 'react';
import { ScriptDialogHost } from '@/components/scripts/ScriptDialogHost.tsx';
import { captureScreenSnapshot } from '@/infrastructure/scripts/screenSnapshotRegistry.ts';
import { setupScriptBridgeListeners } from '@/application/state/useOutputTriggers.ts';
import { lemonsshBridge } from '@/infrastructure/services/lemonsshBridge.ts';
import { setScriptRuns } from '@/application/state/scriptAutomationCoordinator.ts';
import type { Snippet } from '@/domain/models';

export function ScriptAutomationRoot() {
  useEffect(() => {
    const disposeBridge = setupScriptBridgeListeners(captureScreenSnapshot);
    const bridge = lemonsshBridge.get();
    bridge?.scriptGetRuns?.().then(setScriptRuns).catch(() => {});
    const disposeRuns = bridge?.onScriptRunsUpdated?.(({ runs }) => {
      setScriptRuns(runs);
    });
    return () => {
      disposeBridge();
      disposeRuns?.();
    };
  }, []);

  useEffect(() => {
    const handler = (event: Event) => {
      const snippet = (event as CustomEvent<{ snippet: Snippet }>).detail?.snippet;
      if (!snippet) return;
      window.dispatchEvent(new CustomEvent('lemonssh:scripts:run-on-focused', { detail: { snippet } }));
    };
    window.addEventListener('lemonssh:scripts:run-now', handler);
    return () => window.removeEventListener('lemonssh:scripts:run-now', handler);
  }, []);

  return <ScriptDialogHost />;
}
