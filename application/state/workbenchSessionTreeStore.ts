import { useSyncExternalStore } from "react";

import { useTreeExpandedState } from "./useTreeExpandedState";
import { STORAGE_KEY_WORKBENCH_SESSION_TREE_EXPANDED } from "../../infrastructure/config/storageKeys";

type WorkbenchTreeState = {
  expandedPaths: Set<string>;
};

const listeners = new Set<() => void>();
let expandedPaths = new Set<string>();

function notify() {
  for (const listener of listeners) listener();
}

export function useWorkbenchTreeExpanded(): {
  expandedPaths: Set<string>;
  toggle: (path: string) => void;
  expand: (path: string) => void;
} {
  const tree = useTreeExpandedState(STORAGE_KEY_WORKBENCH_SESSION_TREE_EXPANDED);
  return tree;
}

export const STORAGE_KEY = STORAGE_KEY_WORKBENCH_SESSION_TREE_EXPANDED;
