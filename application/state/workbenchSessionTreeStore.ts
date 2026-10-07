import { useCallback, useState } from 'react';
import { hostStorageAdapter } from '../../infrastructure/persistence/hostStorageAdapter';
import { STORAGE_KEY_WORKBENCH_SESSION_TREE_WIDTH } from '../../infrastructure/config/storageKeys';
import { useTreeExpandedState } from "./useTreeExpandedState";

export function useWorkbenchTreeWidth() {
  const [width, setWidth] = useState(() => {
    const stored = Number(hostStorageAdapter.readString(STORAGE_KEY_WORKBENCH_SESSION_TREE_WIDTH));
    return Number.isFinite(stored) && stored >= 180 && stored <= 480 ? stored : 240;
  });
  const resize = useCallback((value: number) => {
    if (!Number.isFinite(value)) return;
    const next = Math.max(180, Math.min(480, value));
    setWidth(next);
    hostStorageAdapter.writeString(STORAGE_KEY_WORKBENCH_SESSION_TREE_WIDTH, String(next));
  }, []);
  return { width, resize };
}
import { STORAGE_KEY_WORKBENCH_SESSION_TREE_EXPANDED } from "../../infrastructure/config/storageKeys";

/**
 * Workbench session-tree view state: which group/host branches are expanded.
 * Persisted via useTreeExpandedState (hostStorageAdapter-backed) under a key
 * that is independent from the vault host tree (plan §3.7.7).
 */
export function useWorkbenchTreeExpanded(): ReturnType<typeof useTreeExpandedState> {
  return useTreeExpandedState(STORAGE_KEY_WORKBENCH_SESSION_TREE_EXPANDED);
}

export const STORAGE_KEY = STORAGE_KEY_WORKBENCH_SESSION_TREE_EXPANDED;
