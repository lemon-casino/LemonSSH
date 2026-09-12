import React, { memo, useCallback, useEffect, useState } from 'react';
import { Lock, Plus, Settings, Sparkles } from 'lucide-react';

import { useI18n } from '../../application/i18n/I18nProvider';
import {
  setVaultNavSection,
  useVaultNavState,
} from '../../application/state/vaultNavStore';
import type { VaultSection } from '../VaultNavItems';
import { VaultNavItems } from '../VaultNavItems';
import { AppLogo } from '../AppLogo';
import { Button } from '../ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '../ui/tooltip';
import { SyncStatusButton } from '../SyncStatusButton';
import { GlobalSftpTransferCenter } from '../GlobalSftpTransferCenter';
import { TopTabsQuickControls } from '../TopTabsQuickControls';
import { WindowControls } from '../top-tabs/TopTabItems';
import { useWindowControls } from '../../application/state/useWindowControls';

const dragRegionStyle = { WebkitAppRegion: 'drag' } as React.CSSProperties;

interface WorkbenchChromeProps {
  theme: 'dark' | 'light';
  themePreference: 'dark' | 'light' | 'system';
  onThemeChange: (theme: 'dark' | 'light' | 'system') => void;
  isMacClient: boolean;
  showWindowControls: boolean;
  onSelectVaultSection: (section: VaultSection) => void;
  onOpenQuickSwitcher: () => void;
  onOpenSettings: () => void;
  onSyncNow?: () => Promise<void>;
  onLockApp?: () => void;
  appLockEnabled?: boolean;
  externalMcpEnabled: boolean;
  onToggleExternalMcp: (enabled: boolean) => void;
  showExternalMcpToggle?: boolean;
}

/**
 * Workbench top bar: the title bar doubles as the business menu bar
 * (hosts / keys / proxies / ... on the left, utility + window controls on the
 * right). Session tabs live in the left session tree, not here.
 *
 * Right-side controls reuse the same leaf components as TopTabs; callbacks
 * come from AppView so both shells share one action surface.
 */
const WorkbenchChromeInner: React.FC<WorkbenchChromeProps> = ({
  theme,
  themePreference,
  onThemeChange,
  isMacClient,
  showWindowControls,
  onSelectVaultSection,
  onOpenQuickSwitcher,
  onOpenSettings,
  onSyncNow,
  onLockApp,
  appLockEnabled,
  externalMcpEnabled,
  onToggleExternalMcp,
  showExternalMcpToggle,
}) => {
  const { t } = useI18n();
  const { currentSection } = useVaultNavState();
  const { maximize, isFullscreen, onFullscreenChanged } = useWindowControls();

  // macOS traffic lights need extra clearance only outside fullscreen.
  const [isWindowFullscreen, setIsWindowFullscreen] = useState(false);
  useEffect(() => {
    if (!isMacClient) return;
    let cancelled = false;
    isFullscreen().then((value) => {
      if (!cancelled) setIsWindowFullscreen(!!value);
    });
    const unsubscribe = onFullscreenChanged((value) => setIsWindowFullscreen(!!value));
    return () => {
      cancelled = true;
      unsubscribe();
    };
  }, [isFullscreen, isMacClient, onFullscreenChanged]);

  const handleSelectSection = useCallback((section: VaultSection) => {
    setVaultNavSection(section);
    onSelectVaultSection(section);
  }, [onSelectVaultSection]);

  // Double-click on the drag region (not on controls) maximizes the window.
  const handleTitleBarDoubleClick = useCallback((e: React.MouseEvent) => {
    if ((e.target as HTMLElement).closest('.app-no-drag')) return;
    if (!isMacClient) {
      maximize();
    }
  }, [isMacClient, maximize]);

  return (
    <div
      data-section="workbench-chrome"
      className="relative w-full bg-secondary app-drag"
      style={{
        ...dragRegionStyle,
        backgroundColor: 'var(--top-tabs-bg, hsl(var(--secondary)))',
        color: 'var(--top-tabs-fg, hsl(var(--foreground)))',
      }}
      onDoubleClick={handleTitleBarDoubleClick}
    >
      <div
        className="h-9 flex items-end gap-2 app-drag min-w-0"
        style={{
          paddingLeft: isMacClient && !isWindowFullscreen ? 76 : 12,
          paddingRight: showWindowControls ? 0 : 12,
        }}
      >
        <div className="flex items-center app-no-drag shrink-0 self-end h-7">
          <AppLogo className="h-6 w-6" />
        </div>
        <VaultNavItems
          orientation="horizontal"
          currentSection={currentSection}
          onSelectSection={handleSelectSection}
          sidebarCollapsed={false}
          t={t}
        />
        <div className="flex-1 min-w-4 app-drag" style={dragRegionStyle} />
        <div
          className="shrink-0 flex items-center gap-0.5 app-drag self-end h-7 overflow-visible"
          style={dragRegionStyle}
          data-section="workbench-chrome-actions"
        >
          <GlobalSftpTransferCenter />
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                className="h-7 w-7 shrink-0 app-no-drag top-tab-utility-btn"
                style={{ color: 'var(--top-tabs-muted, hsl(var(--muted-foreground)))' }}
                onClick={() => window.dispatchEvent(new CustomEvent('netcatty:toggle-ai-panel'))}
              >
                <Sparkles size={16} />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t('topTabs.aiAssistant')}</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                className="h-7 w-7 shrink-0 app-no-drag top-tab-utility-btn"
                style={{ color: 'var(--top-tabs-muted, hsl(var(--muted-foreground)))' }}
                onClick={onOpenQuickSwitcher}
              >
                <Plus size={16} />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t('topTabs.openQuickSwitcher')}</TooltipContent>
          </Tooltip>
          <SyncStatusButton
            onOpenSettings={onOpenSettings}
            onSyncNow={onSyncNow}
            className="h-7 w-7 shrink-0 top-tab-utility-btn"
            style={{ color: 'var(--top-tabs-muted, hsl(var(--muted-foreground)))' }}
          />
          {appLockEnabled && onLockApp && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-7 w-7 shrink-0 app-no-drag top-tab-utility-btn"
                  style={{ color: 'var(--top-tabs-muted, hsl(var(--muted-foreground)))' }}
                  onClick={onLockApp}
                >
                  <Lock size={16} />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{t('topTabs.lockApp')}</TooltipContent>
            </Tooltip>
          )}
          <TopTabsQuickControls
            theme={theme}
            themePreference={themePreference}
            onThemeChange={onThemeChange}
            externalMcpEnabled={externalMcpEnabled}
            onToggleExternalMcp={onToggleExternalMcp}
            showExternalMcpToggle={showExternalMcpToggle}
            style={{ color: 'var(--top-tabs-muted, hsl(var(--muted-foreground)))' }}
          />
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                className="h-7 w-7 shrink-0 app-no-drag top-tab-utility-btn"
                style={{ color: 'var(--top-tabs-muted, hsl(var(--muted-foreground)))' }}
                onClick={onOpenSettings}
              >
                <Settings size={16} />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t('topTabs.openSettings')}</TooltipContent>
          </Tooltip>
          {showWindowControls && <WindowControls />}
        </div>
      </div>
    </div>
  );
};

export const WorkbenchChrome = memo(
  WorkbenchChromeInner,
    (prev, next) =>
    prev.theme === next.theme &&
    prev.themePreference === next.themePreference &&
    prev.onThemeChange === next.onThemeChange &&
    prev.isMacClient === next.isMacClient &&
    prev.showWindowControls === next.showWindowControls &&
    prev.onSelectVaultSection === next.onSelectVaultSection &&
    prev.onOpenQuickSwitcher === next.onOpenQuickSwitcher &&
    prev.onOpenSettings === next.onOpenSettings &&
    prev.onSyncNow === next.onSyncNow &&
    prev.onLockApp === next.onLockApp &&
    prev.appLockEnabled === next.appLockEnabled &&
    prev.externalMcpEnabled === next.externalMcpEnabled &&
    prev.onToggleExternalMcp === next.onToggleExternalMcp &&
    prev.showExternalMcpToggle === next.showExternalMcpToggle,
);
WorkbenchChrome.displayName = 'WorkbenchChrome';
