import { Download, RefreshCw, Trash2 } from 'lucide-react';
import React, { useState } from 'react';

import { useInstalledPlugins } from '../../../application/state/useInstalledPlugins';
import { useI18n } from '../../../application/i18n/I18nProvider';
import { Button } from '../../ui/button';
import { ConfirmDialog } from '../../ui/confirm-dialog';
import { Switch } from '../../ui/switch';

/**
 * Management surface for the native plugin inventory: install a packaged
 * .ncpkg, toggle enablement, restart or uninstall an installed plugin.
 * All native work is delegated to the useInstalledPlugins application hook.
 */
export function PluginManagerSection() {
  const { t } = useI18n();
  const manager = useInstalledPlugins();
  const [installing, setInstalling] = useState(false);
  const [pendingUninstall, setPendingUninstall] = useState<string | null>(null);
  const [uninstalling, setUninstalling] = useState(false);

  if (!manager.available && !manager.loading) return null;

  const pickAndInstall = async () => {
    if (!manager.available) return;
    setInstalling(true);
    manager.clearError();
    try {
      const archivePath = await manager.pickPackageArchive(t('settings.plugins.manager.pickPackage'));
      if (archivePath) await manager.installPackage(archivePath, { enable: true });
    } catch {
      // Failure is surfaced through the manager error banner.
    } finally {
      setInstalling(false);
    }
  };

  const confirmUninstall = async () => {
    if (!pendingUninstall) return;
    setUninstalling(true);
    try {
      await manager.uninstall(pendingUninstall);
      setPendingUninstall(null);
    } catch {
      // Failure is surfaced through the manager error banner.
    } finally {
      setUninstalling(false);
    }
  };

  const pendingPlugin = manager.plugins.find((plugin) => plugin.id === pendingUninstall);

  return (
    <section
      className="space-y-3 rounded-lg border border-border/70 bg-muted/10 p-4"
      aria-label={t('settings.plugins.manager.title')}
    >
      <div className="flex items-center justify-between gap-4">
        <div>
          <h3 className="text-base font-semibold">{t('settings.plugins.manager.title')}</h3>
          <p className="mt-1 text-xs text-muted-foreground">{t('settings.plugins.manager.description')}</p>
        </div>
        <Button
          type="button"
          size="sm"
          disabled={installing || !manager.available}
          onClick={() => void pickAndInstall()}
        >
          <Download size={14} className="mr-2" />
          {installing ? t('settings.plugins.manager.installing') : t('settings.plugins.manager.install')}
        </Button>
      </div>
      {manager.loading && <p className="text-sm text-muted-foreground">{t('settings.plugins.loading')}</p>}
      {manager.error && (
        <p role="alert" className="text-xs text-destructive">{manager.error.message}</p>
      )}
      {!manager.loading && manager.available && manager.plugins.length === 0 && (
        <p className="text-sm text-muted-foreground">{t('settings.plugins.manager.empty')}</p>
      )}
      <ul className="space-y-2">
        {manager.plugins.map((plugin) => {
          const busy = !manager.available;
          return (
            <li
              key={plugin.id}
              className="flex items-center justify-between gap-3 rounded-md border border-border/60 bg-background p-3"
            >
              <div className="min-w-0">
                <div className="flex min-w-0 items-center gap-2">
                  <span className="truncate text-sm font-medium">{plugin.displayName}</span>
                  {plugin.version && (
                    <span className="shrink-0 rounded bg-muted px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
                      v{plugin.version}
                    </span>
                  )}
                  {!plugin.enabled && (
                    <span className="shrink-0 rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">
                      {t('settings.plugins.manager.stateDisabled')}
                    </span>
                  )}
                  {plugin.enabled && plugin.runtimeStatus !== 'active' && (
                    <span className="shrink-0 rounded bg-amber-500/15 px-1.5 py-0.5 text-[10px] text-amber-700 dark:text-amber-300">
                      {plugin.runtimeStatus}
                    </span>
                  )}
                </div>
                <div className="truncate font-mono text-[10px] text-muted-foreground">{plugin.id}</div>
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="h-7 w-7"
                  disabled={busy}
                  aria-label={t('settings.plugins.manager.restart')}
                  title={t('settings.plugins.manager.restart')}
                  onClick={() => void manager.restart(plugin.id).catch(() => {})}
                >
                  <RefreshCw size={14} />
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="h-7 w-7"
                  disabled={busy}
                  aria-label={t('settings.plugins.manager.uninstall')}
                  title={t('settings.plugins.manager.uninstall')}
                  onClick={() => setPendingUninstall(plugin.id)}
                >
                  <Trash2 size={14} />
                </Button>
                <Switch
                  checked={plugin.enabled}
                  disabled={busy}
                  aria-label={t('settings.plugins.manager.toggle').replace('{name}', plugin.displayName)}
                  onCheckedChange={(checked) => void manager.setEnabled(plugin.id, checked).catch(() => {})}
                />
              </div>
            </li>
          );
        })}
      </ul>
      <ConfirmDialog
        open={pendingUninstall != null}
        title={t('settings.plugins.manager.uninstallConfirmTitle')}
        message={pendingPlugin
          ? t('settings.plugins.manager.uninstallConfirmMessage').replace('{name}', pendingPlugin.displayName)
          : undefined}
        confirmLabel={t('settings.plugins.manager.uninstall')}
        busy={uninstalling}
        destructive
        onOpenChange={(open) => { if (!open) setPendingUninstall(null); }}
        onConfirm={() => void confirmUninstall()}
      />
    </section>
  );
}
