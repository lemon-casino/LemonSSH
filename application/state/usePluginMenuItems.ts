import { useMemo } from 'react';

import {
  collectOwnedPluginMenus,
  comparePluginMenus,
  usePluginContributions,
} from './usePluginContributions';

/**
 * Menu locations a plugin manifest may target (mirrors the Go host's
 * validMenuLocations in internal/plugin/ui/schema.go). commandPalette has its
 * own search-integrated builder inside QuickSwitcher; every other location is
 * consumed through this hook.
 */
export const PLUGIN_MENU_LOCATIONS = [
  'commandPalette',
  'application',
  'host/context',
  'terminal/context',
  'terminal/toolbar',
  'statusBar',
] as const;

export type PluginMenuLocation = (typeof PLUGIN_MENU_LOCATIONS)[number];

/** One owned plugin menu entry: menu fields plus the owning pluginId and the command-inherited icon. */
export type PluginMenuItem = ReturnType<typeof collectOwnedPluginMenus>[number];

/**
 * Pure selector shared by every menu surface: keeps only visible menus whose
 * location matches (a single location or a union such as
 * ['terminal/toolbar', 'statusBar']), inherits the referenced command's icon,
 * and sorts by group/order/id so placements stay stable across surfaces.
 */
export function selectPluginMenuItems(
  plugins: LemonSSHPluginContributionSnapshot['plugins'],
  location: PluginMenuLocation | readonly PluginMenuLocation[],
): PluginMenuItem[] {
  const wanted = new Set<string>(
    typeof location === 'string' ? [location] : [...location],
  );
  return collectOwnedPluginMenus(plugins)
    .filter((menu) => wanted.has(menu.location) && menu.visible)
    .sort(comparePluginMenus);
}

export interface UsePluginMenuItemsResult {
  /** Visible plugin menus for the requested location(s); empty when no plugin contributes there. */
  items: PluginMenuItem[];
  available: boolean;
  loading: boolean;
  /** Same execution chain the command palette uses (bridge.executePluginCommand). */
  executeCommand: (
    command: string,
    args?: unknown,
    context?: Record<string, unknown>,
  ) => Promise<unknown>;
}

/**
 * Application-layer gate for plugin menu contributions at one (or several)
 * surface locations. Components consume `items` and `executeCommand` only;
 * filtering, icon inheritance, ordering and the plugin-lifecycle subscription
 * (enable/disable/restart refresh via onPluginContributionsChanged) stay here.
 */
export function usePluginMenuItems(
  location: PluginMenuLocation | readonly PluginMenuLocation[],
  query: LemonSSHPluginContributionQuery = {},
  options: { enabled?: boolean } = {},
): UsePluginMenuItemsResult {
  const contributions = usePluginContributions(query, options);
  const items = useMemo(
    () => selectPluginMenuItems(contributions.snapshot.plugins, location),
    [contributions.snapshot.plugins, location],
  );
  return {
    items,
    available: contributions.available,
    loading: contributions.loading,
    executeCommand: contributions.executeCommand,
  };
}
