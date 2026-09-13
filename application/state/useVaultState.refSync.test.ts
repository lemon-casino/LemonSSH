/**
 * Regression test for the workbench group-delete no-op.
 *
 * Root cause: `useVaultState` wrote its refs from the render closure:
 *
 *   customGroupsRef.current = customGroups;
 *   hostsRef.current = hosts;
 *   ...
 *
 * Any re-render between an imperative `ref.current = ...` write and the read
 * inside `commitVaultGroupMutation` rewinds the ref to the render-time value.
 * `stateMatchesLiveSnapshot` then compares the freshly persisted vault against
 * a stale ref and rejects the transaction as "superseded". `useVaultGroupDeletion`
 * retries forever, so the delete dialog closes and nothing happens.
 *
 * These assertions pin the fix: refs must be seeded from an effect, so the
 * render closure can never rewind an imperative write.
 */
import assert from 'node:assert/strict';
import test from 'node:test';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';

const USE_VAULT_STATE = fileURLToPath(
  new URL('./useVaultState.ts', import.meta.url),
);

const REF_STATE_PAIRS = [
  ['customGroupsRef', 'customGroups'],
  ['managedSourcesRef', 'managedSources'],
  ['hostsRef', 'hosts'],
  ['snippetsRef', 'snippets'],
  ['groupConfigsRef', 'groupConfigs'],
] as const;

test('useVaultState never re-syncs vault refs from the render closure', async () => {
  const source = await readFile(USE_VAULT_STATE, 'utf8');

  // Every ref-sync statement must live inside a hook callback (effect) or an
  // update handler. A bare statement in the component body is the regression.
  // Count total occurrences and assert none of them sit in the component body:
  // the body is the region between the ref declarations and the first
  // `useCallback`/`useLayoutEffect` that follows them.
  const refDeclarationIndex = source.indexOf('const notesPersistFailureNotifiedAtRef = useRef(0);');
  assert.notEqual(refDeclarationIndex, -1, 'ref declarations must be present');

  const firstHookAfterRefs = source.indexOf('useLayoutEffect(() => {', refDeclarationIndex);
  assert.notEqual(firstHookAfterRefs, -1, 'a layout effect must follow the ref declarations');

  const bodySlice = source.slice(refDeclarationIndex, firstHookAfterRefs);
  for (const [refName, stateName] of REF_STATE_PAIRS) {
    assert.doesNotMatch(
      bodySlice,
      new RegExp(`${refName}\\.current = ${stateName};`),
      `${refName}.current must not be assigned in the render body`,
    );
  }
});

test('each vault ref is seeded from its own layout effect', async () => {
  const source = await readFile(USE_VAULT_STATE, 'utf8');

  for (const [refName, stateName] of REF_STATE_PAIRS) {
    const pattern = new RegExp(
      `useLayoutEffect\\(\\(\\) => \\{\\s*${refName}\\.current = ${stateName};\\s*\\}, \\[${stateName}\\]\\);`,
    );
    assert.match(
      source,
      pattern,
      `${refName} must be synced in a dedicated layout effect keyed only on ${stateName}`,
    );
  }
});
