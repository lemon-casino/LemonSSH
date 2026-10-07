import { useCallback, useEffect, useRef, useState } from 'react';

import { lemonsshBridge } from '../../infrastructure/services/lemonsshBridge';

export interface PluginViewDataRequest {
  pluginId: string;
  viewId: string;
  bindings: ReadonlyArray<string>;
}

export interface PluginViewDataEntry {
  source: 'plugin' | 'settings';
  data: Record<string, unknown>;
}

/** pluginId → viewId → resolved data (plugin dispatch merged over settings values). */
export type PluginViewDataSnapshot = Record<string, Record<string, PluginViewDataEntry>>;

const EMPTY_SNAPSHOT: PluginViewDataSnapshot = Object.freeze({});

// Deduplicates work items and drops binding-less requests: the dispatch channel
// has nothing to fetch when a view declares no bindings.
export function collectPluginViewDataRequests(
  requests: ReadonlyArray<PluginViewDataRequest>,
): PluginViewDataRequest[] {
  const seen = new Set<string>();
  const unique: PluginViewDataRequest[] = [];
  for (const request of requests) {
    if (!request.bindings.length) continue;
    const key = `${request.pluginId}::${request.viewId}`;
    if (seen.has(key)) continue;
    seen.add(key);
    unique.push({ pluginId: request.pluginId, viewId: request.viewId, bindings: [...request.bindings] });
  }
  return unique;
}

export function serializePluginViewDataRequests(
  requests: ReadonlyArray<PluginViewDataRequest>,
): string {
  return JSON.stringify(collectPluginViewDataRequests(requests));
}

export interface UsePluginViewDataResult {
  data: PluginViewDataSnapshot;
  loading: boolean;
  refresh(): Promise<void>;
}

/**
 * Pulls declarative view data through the plugin bridge. Each entry merges the
 * plugin's `view.data` dispatch result over its declared non-secret settings
 * values (the bridge owns that merge and its settings fallback).
 */
export function usePluginViewData(
  requests: ReadonlyArray<PluginViewDataRequest>,
  refreshKey?: unknown,
): UsePluginViewDataResult {
  const bridge = typeof window === 'undefined' ? undefined : lemonsshBridge.get();
  const requestsKey = serializePluginViewDataRequests(requests);
  const [data, setData] = useState<PluginViewDataSnapshot>(EMPTY_SNAPSHOT);
  const [loading, setLoading] = useState(() => collectPluginViewDataRequests(requests).length > 0);
  const guard = useRef(0);

  const load = useCallback(async () => {
    const token = ++guard.current;
    const pending = collectPluginViewDataRequests(JSON.parse(requestsKey) as PluginViewDataRequest[]);
    if (!bridge?.getPluginViewData || pending.length === 0) {
      setData(EMPTY_SNAPSHOT);
      setLoading(false);
      return;
    }
    setLoading(true);
    const entries = await Promise.all(pending.map(async request => {
      try {
        const result = await bridge.getPluginViewData!(request.pluginId, request.viewId, request.bindings);
        return [request.pluginId, request.viewId, result] as const;
      } catch {
        // Fail soft per view: a broken plugin must not blank the other views.
        return [request.pluginId, request.viewId, null] as const;
      }
    }));
    if (token !== guard.current) return;
    const next: PluginViewDataSnapshot = {};
    for (const [pluginId, viewId, result] of entries) {
      if (!result) continue;
      (next[pluginId] ??= {})[viewId] = { source: result.source, data: result.data };
    }
    setData(next);
    setLoading(false);
  }, [bridge, requestsKey]);

  useEffect(() => {
    void load();
    return () => { guard.current += 1; };
  }, [load, refreshKey]);

  return { data, loading, refresh: load };
}
