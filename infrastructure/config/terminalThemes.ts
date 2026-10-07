import type { TerminalTheme } from '../../domain/models';
import { classicTerminalThemes } from './terminalThemes/classic';
import { systemPresetTerminalThemes } from './terminalThemes/systemPresets';
import { coreTerminalThemes } from './terminalThemes/core';
import { extraTerminalThemes } from './terminalThemes/extra';
import { modernTerminalThemes } from './terminalThemes/modern';
import { uiMatchTerminalThemes } from './terminalThemes/uiMatch';

// Re-export for convenience
export type TerminalThemeConfig = TerminalTheme;

const UI_MATCH_TERMINAL_THEME_IDS = new Set([
  'ui-snow',
  'ui-pure-white',
  'ui-ivory',
  'ui-mist',
  'ui-mint',
  'ui-sand',
  'ui-lavender',
  'ui-pure-black',
  'ui-midnight',
  'ui-deep-blue',
  'ui-vscode',
  'ui-graphite',
  'ui-obsidian',
  'ui-forest',
]);

export const TERMINAL_THEMES: TerminalTheme[] = [
  ...coreTerminalThemes,
  ...uiMatchTerminalThemes,
  ...modernTerminalThemes,
  ...classicTerminalThemes,
  ...systemPresetTerminalThemes,
  ...extraTerminalThemes,
];

const TERMINAL_THEME_BY_ID = new Map(TERMINAL_THEMES.map((theme) => [theme.id, theme]));

/**
 * compat#6: the two builtin theme ids carried the old brand spelling. Every
 * read side (theme resolution, settings reads, sync payload apply) maps them
 * to the new ids so saved settings and cloud payloads keep resolving; unknown
 * ids (user-created themes) pass through untouched and the next save persists
 * the new value.
 */
export const LEGACY_TERMINAL_THEME_IDS: Readonly<Record<string, string>> = {
  'netcatty-dark': 'lemonssh-dark',
  'netcatty-light': 'lemonssh-light',
};

export const normalizeLegacyTerminalThemeId = (themeId: string | null | undefined): string | null => {
  if (typeof themeId !== 'string') return null;
  return LEGACY_TERMINAL_THEME_IDS[themeId] ?? themeId;
};

export const getBuiltinTerminalThemeById = (themeId: string): TerminalTheme | undefined =>
  TERMINAL_THEME_BY_ID.get(normalizeLegacyTerminalThemeId(themeId) ?? themeId);

export const isUiMatchTerminalThemeId = (themeId: string): boolean =>
  UI_MATCH_TERMINAL_THEME_IDS.has(themeId);

export const USER_VISIBLE_TERMINAL_THEMES: TerminalTheme[] = TERMINAL_THEMES.filter(
  (theme) => !isUiMatchTerminalThemeId(theme.id),
);
