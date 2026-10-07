/**
 * Regression test: the vault live-snapshot guard must compare *content*, not
 * property insertion order.
 *
 * `commitVaultGroupMutation` validates the state a transaction read back from
 * storage against the in-memory refs. The two sides go through different
 * builders:
 *
 *   storage : JSON → decrypt → sanitizeHost → normalizeVaultOrder
 *   memory  : updateHosts / buildVaultGroupDeletion (`{ ...host, group: "" }`)
 *
 * Domain helpers re-spread objects, which appends/pins keys in a different
 * order. The previous `JSON.stringify(a) === JSON.stringify(b)` guard treated
 * that as a mismatch, rejected the transaction as "superseded", and
 * `useVaultGroupDeletion` retried forever — the group-delete dialog closed and
 * nothing happened.
 */
import assert from 'node:assert/strict';
import test from 'node:test';

import {
  canonicalJsonString,
  jsonValuesEqual,
  normalizeJsonValue,
} from '../../domain/convergentSync/json.ts';

test('key order does not affect equality', () => {
  const fromStorage = { id: 'h1', label: 'web', group: 'Prod', order: 1000 };
  const fromMemory = { order: 1000, group: 'Prod', label: 'web', id: 'h1' };

  // The old guard would have failed here.
  assert.notEqual(JSON.stringify(fromStorage), JSON.stringify(fromMemory));
  assert.equal(jsonValuesEqual(fromStorage, fromMemory), true);
});

test('a re-spread host compares equal to its storage-round-tripped copy', () => {
  const storageHost = {
    id: 'h1', label: 'web', hostname: '10.0.0.1', username: 'root',
    port: 22, protocol: 'ssh', group: 'Prod', tags: [], order: 1000,
  };
  // What buildVaultGroupDeletion emits for a retained host.
  const flattened = { ...storageHost, group: '', managedSourceId: undefined };
  // The storage copy has no managedSourceId key at all.
  const fromStorage = { ...storageHost, group: '' };

  assert.equal(jsonValuesEqual(flattened as never, fromStorage as never), true);
});

test('real content differences still fail the guard', () => {
  const left = { id: 'h1', group: 'Prod' };
  const right = { id: 'h1', group: 'Dev' };

  assert.equal(jsonValuesEqual(left, right), false, 'a changed field must be detected');
  assert.notEqual(canonicalJsonString(left), canonicalJsonString(right));
});

test('array order is significant', () => {
  const left = [{ id: 'a' }, { id: 'b' }];
  const right = [{ id: 'b' }, { id: 'a' }];

  assert.equal(jsonValuesEqual(left, right), false, 'reordering hosts is a real change');
});

test('normalizeJsonValue drops undefined-valued keys consistently', () => {
  const withUndefined = { id: 'h1', managedSourceId: undefined };
  const withoutKey = { id: 'h1' };

  assert.deepEqual(normalizeJsonValue(withUndefined), normalizeJsonValue(withoutKey));
  assert.equal(
    jsonValuesEqual(normalizeJsonValue(withUndefined), normalizeJsonValue(withoutKey)),
    true,
  );
});
