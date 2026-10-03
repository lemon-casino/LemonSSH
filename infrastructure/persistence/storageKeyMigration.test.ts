import assert from "node:assert/strict";
import { test } from "node:test";

import {
  runStoragePrefixMigration,
  renamedLegacyStorageKey,
  STORAGE_PREFIX_MIGRATION_MARKER,
  type StoragePrefixMigrationStore,
} from "./storageKeyMigration";

function makeStore(initial: Record<string, string> = {}) {
  const map = new Map(Object.entries(initial));
  let failWrites = false;
  const store: StoragePrefixMigrationStore & { writes: Array<[string, string]> } = {
    keys: (): string[] => [...map.keys()],
    readString: (key: string): string | null => map.get(key) ?? null,
    writeString: (key: string, value: string): boolean => {
      if (failWrites) return false;
      map.set(key, value);
      store.writes.push([key, value]);
      return true;
    },
    writes: [],
  };
  return { store, map, setFailWrites: (value: boolean): void => { failWrites = value; } };
}

test('renamedLegacyStorageKey swaps only the first netcatty occurrence', () => {
  assert.equal(renamedLegacyStorageKey('netcatty_hosts_v1'), 'lemonssh_hosts_v1');
  assert.equal(renamedLegacyStorageKey('__netcatty_pf_cancel_reconnect'), '__lemonssh_pf_cancel_reconnect');
  assert.equal(renamedLegacyStorageKey('netcatty.aiDebug.hide'), 'lemonssh.aiDebug.hide');
  assert.equal(
    renamedLegacyStorageKey('netcatty_terminal_encoding_by_host_v1:host-1'),
    'lemonssh_terminal_encoding_by_host_v1:host-1',
  );
  // Non-brand keys are untouched.
  assert.equal(renamedLegacyStorageKey('debug.hotkeys'), null);
  assert.equal(renamedLegacyStorageKey('lemonssh_close_behavior_v1'), null);
  assert.equal(renamedLegacyStorageKey('theme'), null);
});

test('migration copies legacy keys to the new prefix and keeps the old keys', () => {
  const { store, map } = makeStore({
    'netcatty_hosts_v1': '[{"id":"h1"}]',
    '__netcatty_pf_cancel_reconnect': '1',
    'netcatty.aiDebug.hide': '["panel"]',
    'netcatty_terminal_encoding_by_host_v1:host-1': 'utf-8',
    'debug.hotkeys': '1',
    'lemonssh_close_behavior_v1': 'ask',
  });

  const result = runStoragePrefixMigration(store);

  assert.equal(result.ran, true);
  assert.deepEqual(result.failed, []);
  assert.equal(map.get('lemonssh_hosts_v1'), '[{"id":"h1"}]');
  assert.equal(map.get('__lemonssh_pf_cancel_reconnect'), '1');
  assert.equal(map.get('lemonssh.aiDebug.hide'), '["panel"]');
  assert.equal(map.get('lemonssh_terminal_encoding_by_host_v1:host-1'), 'utf-8');
  // Copy, not rename: legacy keys survive the migration itself.
  assert.equal(map.get('netcatty_hosts_v1'), '[{"id":"h1"}]');
  assert.equal(map.get('__netcatty_pf_cancel_reconnect'), '1');
  // Non-brand keys are neither copied nor touched.
  assert.equal(map.get('debug.hotkeys'), '1');
  assert.equal(map.get('lemonssh_close_behavior_v1'), 'ask');
  assert.ok(!result.copied.includes('debug.hotkeys'));
  // Completion marker lands with a versioned JSON payload.
  const marker = map.get(STORAGE_PREFIX_MIGRATION_MARKER);
  assert.ok(marker);
  assert.equal((JSON.parse(marker ?? '{}') as { v?: number }).v, 1);
});

test('migration never overwrites an existing new-name value', () => {
  const { store, map } = makeStore({
    'netcatty_hosts_v1': '["legacy"]',
    'lemonssh_hosts_v1': '["fresh"]',
  });

  const result = runStoragePrefixMigration(store);

  assert.equal(result.ran, true);
  assert.deepEqual(result.skippedExisting, ['netcatty_hosts_v1']);
  assert.equal(map.get('lemonssh_hosts_v1'), '["fresh"]');
  assert.equal(map.get('netcatty_hosts_v1'), '["legacy"]');
});

test('marker makes the migration idempotent', () => {
  const { store, map, setFailWrites } = makeStore({ 'netcatty_theme_v1': 'dark' });
  runStoragePrefixMigration(store);
  assert.equal(map.get('lemonssh_theme_v1'), 'dark');
  const writesAfterFirst = store.writes.length;

  // A second startup sees the marker and does nothing.
  const second = runStoragePrefixMigration(store);
  assert.equal(second.ran, false);
  assert.deepEqual(second.copied, []);
  assert.equal(store.writes.length, writesAfterFirst);

  // Even new legacy-looking data is not copied once the marker exists.
  setFailWrites(false);
  map.set('netcatty_color_v1', 'accent');
  const third = runStoragePrefixMigration(store);
  assert.equal(third.ran, false);
  assert.equal(map.has('lemonssh_color_v1'), false);
});

test('failed copies withhold the marker so the next run retries', () => {
  const { store, map, setFailWrites } = makeStore({
    'netcatty_hosts_v1': '["hosts"]',
    'netcatty_notes_v1': '["notes"]',
  });

  setFailWrites(true);
  const failedRun = runStoragePrefixMigration(store);
  assert.equal(failedRun.ran, false);
  assert.deepEqual(failedRun.failed.sort(), ['netcatty_hosts_v1', 'netcatty_notes_v1']);
  assert.equal(map.has(STORAGE_PREFIX_MIGRATION_MARKER), false);

  // Storage recovers: the retry copies what was missing and finishes.
  setFailWrites(false);
  const retry = runStoragePrefixMigration(store);
  assert.equal(retry.ran, true);
  assert.deepEqual(retry.failed, []);
  assert.equal(map.get('lemonssh_hosts_v1'), '["hosts"]');
  assert.equal(map.get('lemonssh_notes_v1'), '["notes"]');
  assert.ok(map.has(STORAGE_PREFIX_MIGRATION_MARKER));
});

test('partial failure then retry only copies the missing keys', () => {
  // Simulate a store whose write hook fails for a single key until storage recovers.
  const map = new Map<string, string>([
    ['netcatty_hosts_v1', '["hosts"]'],
    ['netcatty_notes_v1', '["notes"]'],
  ]);
  let failingWrite: string | null = 'lemonssh_notes_v1';
  let writes = 0;
  const store: StoragePrefixMigrationStore = {
    keys: (): string[] => [...map.keys()],
    readString: (key: string): string | null => map.get(key) ?? null,
    writeString: (key: string, value: string): boolean => {
      if (key === failingWrite) return false;
      map.set(key, value);
      writes += 1;
      return true;
    },
  };

  const first = runStoragePrefixMigration(store);
  assert.equal(first.ran, false);
  assert.deepEqual(first.failed, ['netcatty_notes_v1']);
  assert.deepEqual(first.copied, ['netcatty_hosts_v1']);
  assert.equal(map.has(STORAGE_PREFIX_MIGRATION_MARKER), false);

  // Storage recovers: the retry skips the already-copied key and finishes.
  failingWrite = null;
  const beforeRetry = writes;
  const second = runStoragePrefixMigration(store);
  assert.equal(second.ran, true);
  assert.deepEqual(second.skippedExisting, ['netcatty_hosts_v1']);
  assert.deepEqual(second.copied, ['netcatty_notes_v1']);
  assert.equal(writes, beforeRetry + 2); // retried copy + marker
});
