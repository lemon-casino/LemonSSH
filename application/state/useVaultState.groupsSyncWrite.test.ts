import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';

const here = dirname(fileURLToPath(import.meta.url));
const vaultStateSource = readFileSync(join(here, 'useVaultState.ts'), 'utf8');
const inlineGroupActionsSource = readFileSync(
  join(here, '../../components/vault/useHostTreeInlineGroupActions.ts'),
  'utf8',
);

test('updateCustomGroups persists GROUPS synchronously, not behind configs encryption', () => {
  const start = vaultStateSource.indexOf('const updateCustomGroups = useCallback(');
  assert.notEqual(start, -1, 'updateCustomGroups must exist');
  const end = vaultStateSource.indexOf('const updateKnownHosts', start);
  const body = vaultStateSource.slice(start, end);

  const setGroupsAt = body.indexOf('setCustomGroups(next);');
  const syncWriteAt = body.indexOf("localStorageAdapter.write(STORAGE_KEY_GROUPS, next);");
  const encryptAt = body.indexOf('const encryptPromise = encryptGroupConfigs');
  assert.notEqual(setGroupsAt, -1);
  assert.notEqual(syncWriteAt, -1, 'GROUPS write must exist');
  assert.notEqual(encryptAt, -1);
  assert.ok(
    setGroupsAt < syncWriteAt && syncWriteAt < encryptAt,
    'GROUPS must be written before the async configs-encrypt chain, or vault group transactions race a stale storage snapshot',
  );
  // The write must sit at the callback top level, not inside a then() chain.
  const between = body.slice(setGroupsAt, syncWriteAt);
  assert.doesNotMatch(between, /\.then\(/, 'GROUPS write must not be deferred behind a promise chain');
});

test('inline new-group editor starts after the triggering menu unmounts', () => {
  const start = inlineGroupActionsSource.indexOf('const startInlineNewGroup = useCallback(');
  assert.notEqual(start, -1);
  const end = inlineGroupActionsSource.indexOf('const startInlineRenameGroup', start);
  const body = inlineGroupActionsSource.slice(start, end);

  assert.match(
    body,
    /window\.setTimeout\(\(\) => \{\s*hostTreeInlineGroupEditStore\.startEdit\(/,
    'startEdit must be deferred so the context menu focus restoration cannot blur the editor away in the same frame',
  );
  // The group data itself must still be committed synchronously.
  assert.match(body, /onUpdateCustomGroups\(Array\.from\(new Set\(\[\.\.\.customGroups, path\]\)\)\);/);
});
