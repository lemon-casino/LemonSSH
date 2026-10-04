import { Puzzle } from 'lucide-react';
import React, { useCallback, useState } from 'react';

import { useI18n } from '../../application/i18n/I18nProvider';
import {
  usePluginMenuItems,
  type PluginMenuItem,
} from '../../application/state/usePluginMenuItems';
import { PluginContributionIcon } from './PluginContributionIcon';
import { Button } from '../ui/button';
import { Popover, PopoverContent, PopoverTrigger } from '../ui/popover';
import { Tooltip, TooltipContent, TooltipTrigger } from '../ui/tooltip';
import { cn } from '../../lib/utils';

/** Stable query: application-surface context only, so the bridge query key never churns. */
const APPLICATION_MENU_QUERY: LemonSSHPluginContributionQuery = Object.freeze({
  context: Object.freeze({ 'lemonssh.surface': 'application' }),
});

export function shouldRenderPluginApplicationMenu(items: ReadonlyArray<PluginMenuItem>): boolean {
  return items.length > 0;
}

interface PluginApplicationMenuItemsProps {
  items: ReadonlyArray<PluginMenuItem>;
  label: string;
  onExecute: (menu: PluginMenuItem, useAlternate: boolean) => void;
}

/** The plain item list inside the popover; exported so tests can render it without the Radix portal. */
export const PluginApplicationMenuItems: React.FC<PluginApplicationMenuItemsProps> = ({
  items,
  label,
  onExecute,
}) => (
  <div role="menu" aria-label={label}>
    {items.map((menu) => (
      <button
        key={menu.id}
        type="button"
        role="menuitem"
        disabled={!menu.enabled}
        className={cn(
          'flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-xs transition-colors',
          menu.enabled ? 'hover:bg-secondary' : 'cursor-not-allowed opacity-50',
        )}
        data-plugin-menu-item={menu.id}
        onClick={(event) => {
          if (!menu.enabled) return;
          onExecute(menu, event.altKey);
        }}
      >
        <PluginContributionIcon pluginId={menu.pluginId} icon={menu.icon} size={14} className="shrink-0" />
        <span className="min-w-0 flex-1 truncate">{menu.title}</span>
        {menu.checked && <span className="shrink-0 pl-2" aria-hidden="true">✓</span>}
        {menu.shortcut && <span className="shrink-0 pl-2 text-[10px] text-muted-foreground">{menu.shortcut}</span>}
      </button>
    ))}
  </div>
);

interface PluginApplicationMenuContentProps {
  items: ReadonlyArray<PluginMenuItem>;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  label: string;
  onExecute: (menu: PluginMenuItem, useAlternate: boolean) => void;
}

/**
 * Presentational popover menu for application-surface plugin commands. The
 * same Popover+Tooltip composition the neighboring transfer-center button
 * uses; exports the pure rendering half for tests. The empty case renders
 * nothing so the top bar never keeps a shell around when no plugin
 * contributes here.
 */
export const PluginApplicationMenuContent: React.FC<PluginApplicationMenuContentProps> = ({
  items,
  open,
  onOpenChange,
  label,
  onExecute,
}) => {
  if (!shouldRenderPluginApplicationMenu(items)) return null;
  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <Tooltip>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="h-7 w-7 shrink-0 app-no-drag top-tab-utility-btn"
              style={{ color: 'var(--top-tabs-muted, hsl(var(--muted-foreground)))' }}
              aria-label={label}
              data-section="plugin-application-menu-trigger"
            >
              <Puzzle size={16} />
            </Button>
          </PopoverTrigger>
        </TooltipTrigger>
        <TooltipContent>{label}</TooltipContent>
      </Tooltip>
      <PopoverContent
        align="end"
        sideOffset={6}
        className="w-56 p-1 app-no-drag"
        data-section="plugin-application-menu"
      >
        <PluginApplicationMenuItems items={items} label={label} onExecute={onExecute} />
      </PopoverContent>
    </Popover>
  );
};

/**
 * Application-surface plugin menu: the workbench/classic top bars double as
 * the application menu bar, so a puzzle-piece button appears there whenever
 * an enabled plugin contributes `application` menus and disappears (no empty
 * shell) otherwise. Clicks reuse the same executePluginCommand chain as the
 * command palette.
 */
const PluginApplicationMenuInner: React.FC = () => {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const { items, executeCommand } = usePluginMenuItems('application', APPLICATION_MENU_QUERY);
  const label = t('settings.tab.plugins');

  const handleExecute = useCallback((menu: PluginMenuItem, useAlternate: boolean) => {
    setOpen(false);
    void executeCommand(
      useAlternate && menu.alt ? menu.alt : menu.command,
      undefined,
      { 'lemonssh.surface': 'application' },
    ).catch(() => {});
  }, [executeCommand]);

  if (!shouldRenderPluginApplicationMenu(items)) return null;

  return (
    <PluginApplicationMenuContent
      items={items}
      open={open}
      onOpenChange={setOpen}
      label={label}
      onExecute={handleExecute}
    />
  );
};

export const PluginApplicationMenu = React.memo(PluginApplicationMenuInner);
PluginApplicationMenu.displayName = 'PluginApplicationMenu';
