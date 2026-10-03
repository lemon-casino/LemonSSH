import React, { useMemo } from 'react';
import type { DeclarativePluginView } from '@lemonssh/plugin-contract';

import { useDeclarativePlugins } from '../../application/state/useDeclarativePlugins';
import {
  usePluginViewData,
  type PluginViewDataRequest,
} from '../../application/state/usePluginViewData';
import { useI18n } from '../../application/i18n/I18nProvider';
import { DeclarativePluginHost, PluginInstallError } from './DeclarativePluginHost';

// Declarative views are rendered entirely by host-owned components: the
// plugin's schema supplies structure, its declared settings values are the
// base data layer, and the lemonssh-wasm-abi `view.data` dispatch (when the
// plugin implements it) overlays live data. No plugin HTML/JS/CSS is involved.
export function DeclarativePluginViewSurface({
  pluginId,
  viewId,
  location = 'settings',
}: {
  pluginId: string;
  viewId: string;
  location?: string;
}) {
  const { t } = useI18n();
  const declarative = useDeclarativePlugins();
  const entry = declarative.plugins.find(candidate => candidate.pluginId === pluginId);
  const schemaView: DeclarativePluginView | undefined = useMemo(
    () => entry?.schema.views?.find(view => view.id === viewId),
    [entry, viewId],
  );
  const requests = useMemo<PluginViewDataRequest[]>(
    () => (schemaView && location === 'settings' ? [{ pluginId, viewId, bindings: schemaView.bindings ?? [] }] : []),
    [location, pluginId, schemaView, viewId],
  );
  const viewData = usePluginViewData(requests, declarative.plugins);

  if (declarative.error != null) return <PluginInstallError error={declarative.error} />;
  if (!entry || !schemaView) {
    return <p className="p-4 text-sm text-muted-foreground">{t('settings.plugins.loading')}</p>;
  }
  return (
    <div className="min-h-0 overflow-auto p-4">
      <DeclarativePluginHost
        pluginId={pluginId}
        schema={{ views: [schemaView] }}
        values={entry.values}
        data={viewData.data[pluginId]?.[viewId]?.data ?? {}}
        permissions={entry.permissions}
        onGrantPermission={entry.onGrantPermission}
        onSettingChange={entry.onSettingChange}
      />
    </div>
  );
}
