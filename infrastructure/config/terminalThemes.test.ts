import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  getBuiltinTerminalThemeById,
  LEGACY_TERMINAL_THEME_IDS,
  normalizeLegacyTerminalThemeId,
} from './terminalThemes.ts';
import { coreTerminalThemes } from './terminalThemes/core.ts';

test('normalizeLegacyTerminalThemeId maps the two legacy builtin ids', () => {
  assert.equal(normalizeLegacyTerminalThemeId('netcatty-dark'), 'lemonssh-dark');
  assert.equal(normalizeLegacyTerminalThemeId('netcatty-light'), 'lemonssh-light');
  assert.deepEqual({ ...LEGACY_TERMINAL_THEME_IDS }, {
    'netcatty-dark': 'lemonssh-dark',
    'netcatty-light': 'lemonssh-light',
  });
});

test('normalizeLegacyTerminalThemeId keeps unknown and current ids untouched', () => {
  assert.equal(normalizeLegacyTerminalThemeId('lemonssh-dark'), 'lemonssh-dark');
  assert.equal(normalizeLegacyTerminalThemeId('my-custom-theme'), 'my-custom-theme');
  assert.equal(normalizeLegacyTerminalThemeId('ui-midnight'), 'ui-midnight');
  assert.equal(normalizeLegacyTerminalThemeId(''), '');
  assert.equal(normalizeLegacyTerminalThemeId(null), null);
  assert.equal(normalizeLegacyTerminalThemeId(undefined), null);
  assert.equal(normalizeLegacyTerminalThemeId(42 as unknown as string), null);
});

test('builtin theme resolution accepts the legacy ids via the alias', () => {
  const dark = getBuiltinTerminalThemeById('netcatty-dark');
  assert.ok(dark);
  assert.equal(dark?.id, 'lemonssh-dark');
  const light = getBuiltinTerminalThemeById('netcatty-light');
  assert.ok(light);
  assert.equal(light?.id, 'lemonssh-light');
  // Current ids keep resolving; unknown ids (user-created themes) do not.
  assert.equal(getBuiltinTerminalThemeById('lemonssh-dark')?.id, 'lemonssh-dark');
  assert.equal(getBuiltinTerminalThemeById('definitely-not-builtin'), undefined);
});

test('core terminal themes carry the new ids only', () => {
  const ids = coreTerminalThemes.map((theme) => theme.id);
  assert.deepEqual(ids, ['lemonssh-dark', 'lemonssh-light']);
});
