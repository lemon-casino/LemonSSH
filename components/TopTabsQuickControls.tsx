import React, { useState } from 'react';
import {
  Monitor,
  Moon,
  Plug,
  SlidersHorizontal,
  Sun,
} from 'lucide-react';
import { useI18n } from '../application/i18n/I18nProvider';
import { cn } from '../lib/utils';
import { Button } from './ui/button';
import { Popover, PopoverContent, PopoverTrigger } from './ui/popover';
import { Switch } from './ui/switch';
import { Tooltip, TooltipContent, TooltipTrigger } from './ui/tooltip';

export interface TopTabsQuickControlsProps {
  theme: 'dark' | 'light';
  themePreference: 'dark' | 'light' | 'system';
  onThemeChange: (theme: 'dark' | 'light' | 'system') => void;
  externalMcpEnabled: boolean;
  onToggleExternalMcp: (enabled: boolean) => void;
  showExternalMcpToggle?: boolean;
  className?: string;
  style?: React.CSSProperties;
}

export const TopTabsQuickControls: React.FC<TopTabsQuickControlsProps> = ({
  theme,
  themePreference,
  onThemeChange,
  externalMcpEnabled,
  onToggleExternalMcp,
  showExternalMcpToggle = true,
  className,
  style,
}) => {
  const { t } = useI18n();
  const [isOpen, setIsOpen] = useState(false);
  const isDark = theme === 'dark';
  const externalMcpLabelId = React.useId();
  const themeOptions = [
    { value: 'light' as const, label: t('topTabs.controlPanel.theme.light') },
    { value: 'dark' as const, label: t('topTabs.controlPanel.theme.dark') },
    { value: 'system' as const, label: t('topTabs.controlPanel.theme.system') },
  ];

  return (
    <Popover
      open={isOpen}
      onOpenChange={setIsOpen}
    >
      <Tooltip>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className={cn('h-7 w-7 shrink-0 app-no-drag top-tab-utility-btn', className)}
              style={{
                ...style,
                color: style?.color ?? 'var(--top-tabs-muted, hsl(var(--muted-foreground)))',
              }}
              aria-label={t('topTabs.controlPanel')}
              data-section="top-tabs-quick-controls"
            >
              <SlidersHorizontal size={16} />
            </Button>
          </PopoverTrigger>
        </TooltipTrigger>
        <TooltipContent>{t('topTabs.controlPanel')}</TooltipContent>
      </Tooltip>

      <PopoverContent
        className="w-72 p-0 app-no-drag"
        align="end"
        sideOffset={6}
      >
        <div className="px-3 py-2 border-b border-border/60">
          <div className="text-sm font-medium">{t('topTabs.controlPanel')}</div>
        </div>

        <div className="p-2">
          <div>
            <div className="flex items-center justify-between gap-3 rounded-md px-2 py-2 hover:bg-muted/40">
              <div className="flex min-w-0 items-center gap-2 text-sm">
                {themePreference === 'system'
                  ? <Monitor size={14} className="shrink-0 text-muted-foreground" />
                  : isDark
                    ? <Moon size={14} className="shrink-0 text-muted-foreground" />
                    : <Sun size={14} className="shrink-0 text-muted-foreground" />}
                <span className="truncate">{t('topTabs.controlPanel.theme')}</span>
              </div>
              <div className="flex shrink-0 items-center rounded-md bg-muted/50 p-0.5" role="group" aria-label={t('topTabs.controlPanel.theme')}>
                {themeOptions.map((option) => (
                  <button
                    key={option.value}
                    type="button"
                    aria-pressed={themePreference === option.value}
                    onClick={() => onThemeChange(option.value)}
                    className={cn(
                      'h-5 rounded px-1.5 text-[10px] font-medium transition-colors',
                      themePreference === option.value
                        ? 'bg-background text-foreground shadow-sm'
                        : 'text-muted-foreground hover:text-foreground',
                    )}
                  >
                    {option.label}
                  </button>
                ))}
              </div>
            </div>
          </div>

          {showExternalMcpToggle ? (
            <div className="mt-1 border-t border-border/60 pt-1">
              <div className="flex items-center justify-between gap-3 rounded-md px-2 py-2 hover:bg-muted/40">
                <div className="flex min-w-0 items-center gap-2 text-sm">
                  <Plug size={14} className="shrink-0 text-muted-foreground" />
                  <span id={externalMcpLabelId} className="truncate">{t('topTabs.controlPanel.externalMcp')}</span>
                </div>
                <Switch
                  checked={externalMcpEnabled}
                  onCheckedChange={onToggleExternalMcp}
                  aria-labelledby={externalMcpLabelId}
                />
              </div>
            </div>
          ) : null}
        </div>
      </PopoverContent>
    </Popover>
  );
};

export default TopTabsQuickControls;
