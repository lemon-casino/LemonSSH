import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';

const here = dirname(fileURLToPath(import.meta.url));
const source = readFileSync(join(here, 'AppSideEffects.tsx'), 'utf8');

/**
 * Diagnosis for: "workbench layout, left session tree, right-click delete does
 * not refresh until restart".
 *
 * `handleConfirmDeleteHost` builds the next host array from the `hosts`
 * captured by its own render closure instead of a functional update. The
 * confirm dialog lives in AppShell and is keyed on `deleteHostConfirm`, so
 * React re-renders it with a *fresh* `hosts`; but the callback registered into
 * the dialogs bag is rebuilt from the AppSideEffects render that produced the
 * confirm request. When any other host mutation lands in between (a terminal
 * renaming a host, an agent bridge write, a second window storage event), the
 * closure holds a pre-mutation array, and `updateHosts` sees an array that
 * still contains the deleted host -> the "unchanged"/superseded path keeps the
 * deleted row on screen until the next full reload.
 */
test('host deletion uses a functional update so a stale hosts closure cannot resurrect the row', () => {
  const start = source.indexOf('const handleConfirmDeleteHost = useCallback(');
  assert.notEqual(start, -1, 'handleConfirmDeleteHost must exist');
  const end = source.indexOf('const handleCancelDeleteHost', start);
  assert.notEqual(end, -1);
  const body = source.slice(start, end);

  // The functional form is the contract: it reads hostsRef.current at call
  // time instead of the render-time array captured by this closure.
  assert.match(
    body,
    /updateHosts\(\(prev\)\s*=>\s*prev\.filter\(/,
    'handleConfirmDeleteHost must delete via a functional updateHosts(prev => prev.filter(...)) '
    + 'so a stale render-time hosts array cannot keep the deleted host on screen',
  );
  assert.doesNotMatch(
    body,
    /updateHosts\(hosts\.filter\(/,
    'updateHosts(hosts.filter(...)) captures the render-time array and is the regression source',
  );
});

test('the confirm dialog closes in the same tick as the deletion is issued', () => {
  const start = source.indexOf('const handleConfirmDeleteHost = useCallback(');
  const end = source.indexOf('const handleCancelDeleteHost', start);
  const body = source.slice(start, end);

  const deleteAt = body.indexOf('updateHosts(');
  const closeAt = body.indexOf('setDeleteHostConfirm(null)');
  assert.notEqual(deleteAt, -1);
  assert.notEqual(closeAt, -1);
  assert.ok(
    closeAt > deleteAt,
    'the confirm state must clear after the delete is issued, never before',
  );
});
