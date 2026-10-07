import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import { lemonsshBridge } from '../../infrastructure/services/lemonsshBridge';
import { createPluginContributionRefreshGuard } from './usePluginContributions';

export interface InstalledPluginDraft {
  id: string;
  displayName: string;
  version: string;
  enabled: boolean;
  runtimeStatus: string;
}

export function describeInstalledPlugin(plugin: LemonSSHInstalledPlugin): InstalledPluginDraft {
  const manifest = plugin.manifest && typeof plugin.manifest === 'object'
    ? plugin.manifest as Record<string, unknown>
    : {};
  const declaredName = typeof manifest.displayName === 'string' ? manifest.displayName.trim() : '';
  return {
    id: plugin.id,
    displayName: declaredName || plugin.id,
    version: plugin.activeVersion ?? '',
    enabled: plugin.enabled,
    runtimeStatus: plugin.runtime?.status ?? 'unknown',
  };
}

export interface UseInstalledPluginsResult {
  available: boolean;
  loading: boolean;
  error: Error | null;
  plugins: InstalledPluginDraft[];
  clearError(): void;
  pickPackageArchive(title: string): Promise<string | null>;
  installPackage(archivePath: string, options?: { enable?: boolean }): Promise<void>;
  setEnabled(pluginId: string, enabled: boolean): Promise<void>;
  restart(pluginId: string): Promise<void>;
  uninstall(pluginId: string): Promise<void>;
}

/**
 * Application hook for installed-plugin lifecycle management. All native work
 * goes through the runtime bridge; components never touch Wails bindings.
 */
export function useInstalledPlugins(options: { enabled?: boolean } = {}): UseInstalledPluginsResult {
  const enabled = options.enabled !== false;
  const bridge = typeof window === 'undefined' ? undefined : lemonsshBridge.get();
  const [plugins, setPlugins] = useState<InstalledPluginDraft[]>([]);
  const [available, setAvailable] = useState(false);
  const [loading, setLoading] = useState(enabled);
  const [error, setError] = useState<Error | null>(null);
  const refreshGuard = useRef(createPluginContributionRefreshGuard());

  const refresh = useCallback(async () => {
    const isCurrent = refreshGuard.current.begin();
    if (!enabled || !bridge?.listPlugins) {
      if (!isCurrent()) return;
      setAvailable(false);
      setPlugins([]);
      setLoading(false);
      return;
    }
    try {
      const records = await bridge.listPlugins();
      if (!isCurrent()) return;
      setPlugins(records.filter(Boolean).map(describeInstalledPlugin));
      setAvailable(true);
      setError(null);
    } catch (cause) {
      if (!isCurrent()) return;
      setAvailable(false);
      setPlugins([]);
      setError(cause instanceof Error ? cause : new Error(String(cause)));
    } finally {
      if (isCurrent()) setLoading(false);
    }
  }, [bridge, enabled]);

  useEffect(() => {
    const guard = refreshGuard.current;
    if (!enabled) return () => guard.invalidate();
    void refresh();
    // Lifecycle mutations emit contribution-change events; keep the list fresh.
    const unsubscribe = bridge?.onPluginContributionsChanged?.(() => {
      void refresh();
    });
    return () => {
      guard.invalidate();
      unsubscribe?.();
    };
  }, [bridge, enabled, refresh]);

  const run = useCallback(async (action: () => Promise<unknown>) => {
    try {
      await action();
    } catch (cause) {
      const failure = cause instanceof Error ? cause : new Error(String(cause));
      // Reconcile the list with the native inventory first, then record the
      // failure so the refresh cannot wipe it.
      await refresh();
      setError(failure);
      throw failure;
    }
    await refresh();
    setError(null);
  }, [refresh]);

  const installPackage = useCallback(async (archivePath: string, installOptions?: { enable?: boolean }) => {
    if (!bridge?.installPluginPackage) throw new Error('Plugin package installation is unavailable');
    await run(() => bridge.installPluginPackage!(archivePath, installOptions));
  }, [bridge, run]);

  const setEnabled = useCallback(async (pluginId: string, next: boolean) => {
    if (!bridge?.setPluginEnabled) throw new Error('Plugin lifecycle control is unavailable');
    await run(() => bridge.setPluginEnabled!(pluginId, next));
  }, [bridge, run]);

  const restart = useCallback(async (pluginId: string) => {
    if (!bridge?.restartPlugin) throw new Error('Plugin restart is unavailable');
    await run(() => bridge.restartPlugin!(pluginId));
  }, [bridge, run]);

  const uninstall = useCallback(async (pluginId: string) => {
    if (!bridge?.uninstallPlugin) throw new Error('Plugin uninstall is unavailable');
    await run(() => bridge.uninstallPlugin!(pluginId));
  }, [bridge, run]);

  const clearError = useCallback(() => setError(null), []);

  const pickPackageArchive = useCallback(async (title: string) => {
    if (!bridge?.selectFile) throw new Error('Plugin package selection is unavailable');
    return bridge.selectFile(
      title,
      undefined,
      [{ name: 'LemonSSH plugin package', extensions: ['ncpkg'] }],
    );
  }, [bridge]);

  return useMemo(() => ({
    available,
    loading,
    error,
    plugins,
    clearError,
    pickPackageArchive,
    installPackage,
    setEnabled,
    restart,
    uninstall,
  }), [available, clearError, error, installPackage, loading, pickPackageArchive, plugins, restart, setEnabled, uninstall]);
}
