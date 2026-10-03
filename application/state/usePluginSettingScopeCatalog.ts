import { useEffect, useState } from 'react';

import { lemonsshBridge } from '../../infrastructure/services/lemonsshBridge';

const EMPTY_SCOPE_CATALOG: LemonSSHPluginScopeCatalog = Object.freeze({
  workspace: Object.freeze([]),
  host: Object.freeze([]),
  session: Object.freeze([]),
  device: Object.freeze([{ id: 'device', label: 'This device' }]),
});

export function buildPluginSettingScopeCatalog({
  hosts,
  workspaces,
  sessions,
  deviceLabel,
}: {
  hosts: readonly { id: string; label?: string; hostname?: string }[];
  workspaces: readonly { id: string; title?: string }[];
  sessions: readonly { id: string; customName?: string; hostLabel?: string; hostname?: string }[];
  deviceLabel: string;
}): LemonSSHPluginScopeCatalog {
  return {
    host: hosts.map((host) => ({ id: host.id, label: host.label || host.hostname || host.id })),
    workspace: workspaces.map((workspace) => ({ id: workspace.id, label: workspace.title || workspace.id })),
    session: sessions.map((session) => ({
      id: session.id,
      label: session.customName || session.hostLabel || session.hostname || session.id,
    })),
    device: [{ id: 'device', label: deviceLabel }],
  };
}

export function resolvePluginSettingScopeSelection(
  catalog: LemonSSHPluginScopeCatalog,
  current: Partial<Record<LemonSSHPluginSettingScopeKind, string>>,
): Partial<Record<LemonSSHPluginSettingScopeKind, string>> {
  const next = { ...current };
  for (const kind of ['workspace', 'host', 'session', 'device'] as const) {
    const entries = catalog[kind];
    if (!entries.some((entry) => entry.id === current[kind])) next[kind] = entries[0]?.id;
  }
  return next;
}

export function usePluginSettingScopeCatalog(): LemonSSHPluginScopeCatalog {
  const [catalog, setCatalog] = useState<LemonSSHPluginScopeCatalog>(EMPTY_SCOPE_CATALOG);

  useEffect(() => {
    const bridge = lemonsshBridge.get();
    let cancelled = false;
    void bridge?.getPluginScopeCatalog?.().then((next) => {
      if (!cancelled) setCatalog(next);
    }).catch(() => {});
    const unsubscribe = bridge?.onPluginScopeCatalogChanged?.((next) => {
      if (!cancelled) setCatalog(next);
    });
    return () => {
      cancelled = true;
      unsubscribe?.();
    };
  }, []);

  return catalog;
}
