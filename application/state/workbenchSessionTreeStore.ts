import { useTreeExpandedState } from "./useTreeExpandedState";
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
