import { useCallback, useSyncExternalStore } from 'react';
import type { ScriptRunParams } from '@/types/global/lemonssh-bridge-script.d.ts';
import { lemonsshBridge } from '@/infrastructure/services/lemonsshBridge.ts';
import {
  getScriptRunsSnapshot,
  subscribeScriptRuns as subscribeScriptRunsStore,
} from './scriptRunsStore.ts';

/**
 * Script run list for UI. Bridge → coordinator.setScriptRuns → scriptRunsStore
 * (ScriptAutomationRoot owns the IPC bind). This hook only subscribes so Scripts
 * side panel re-renders without TerminalLayerInner.
 *
 * Pass `{ enabled: false }` when the Scripts panel is not visible so retained
 * slots do not re-render on every automation log tick.
 */
export function useScriptExecution(options?: { enabled?: boolean }) {
  const enabled = options?.enabled ?? true;
  const runs = useSyncExternalStore(
    enabled ? subscribeScriptRunsStore : () => () => {},
    getScriptRunsSnapshot,
    getScriptRunsSnapshot,
  );

  const runScript = useCallback(async (params: ScriptRunParams) => {
    const bridge = lemonsshBridge.get();
    if (!bridge?.scriptRun) {
      throw new Error('Script bridge unavailable');
    }
    return bridge.scriptRun(params);
  }, []);

  const stopRun = useCallback(async (runId: string) => {
    await lemonsshBridge.get()?.scriptStop?.(runId);
  }, []);

  const pauseRun = useCallback(async (runId: string) => {
    await lemonsshBridge.get()?.scriptPause?.(runId);
  }, []);

  const resumeRun = useCallback(async (runId: string) => {
    await lemonsshBridge.get()?.scriptResume?.(runId);
  }, []);

  const getRunsForSession = useCallback((sessionId: string) => {
    return getScriptRunsSnapshot().filter((run) => run.sessionId === sessionId);
  }, []);

  return {
    runs,
    runScript,
    stopRun,
    pauseRun,
    resumeRun,
    getRunsForSession,
  };
}
