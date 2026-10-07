import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

// fileURLToPath (not URL.pathname): pathname keeps a "/C:/" prefix on Windows,
// which path.join turns into an invalid "C:\C:\..." path.
const root = fileURLToPath(new URL('..', import.meta.url));

function readProjectFile(path: string): string {
  return readFileSync(join(root, path), 'utf8');
}

test('custom CSS helper uses a single stable style element id', () => {
  const source = readProjectFile('lib/customCss.ts');

  assert.match(source, /lemonssh-custom-css/);
  assert.match(source, /styleEl\.textContent = css/);
});

test('settings state applies custom CSS through the shared helper', () => {
  const source = readProjectFile('application/state/useSettingsState.ts');

  assert.match(source, /applyCustomCssToDocument\(customCSS\)/);
});
