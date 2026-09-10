import { useEffect, useState } from "react";
import { hostStorageAdapter as localStorageAdapter } from "../../infrastructure/persistence/hostStorageAdapter";

export type ViewMode = "grid" | "list" | "tree";

const isViewMode = (value: string | null): value is ViewMode =>
  value === "grid" || value === "list" || value === "tree";

export const useStoredViewMode = (
  storageKey: string,
  fallback: ViewMode = "grid",
) => {
  const [viewMode, setViewMode] = useState<ViewMode>(() => {
    const stored = localStorageAdapter.readString(storageKey);
    return isViewMode(stored) ? stored : fallback;
  });

  useEffect(() => {
    localStorageAdapter.writeString(storageKey, viewMode);
  }, [storageKey, viewMode]);

  return [viewMode, setViewMode] as const;
};
