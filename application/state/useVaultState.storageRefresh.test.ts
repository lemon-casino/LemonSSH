/**
 * Regression: workbench group/host delete flashes then restores the old tree
 * until restart.
 *
 * Browser `storage` events carry the value at fire time. Under the Wails
 * canonical profile that payload is a compatibility projection and can lag
 * behind the in-memory cache. Adopting `event.newValue` after a local delete
 * paints the new tree, then restores the pre-delete snapshot. Canonical mode
 * must treat native events as invalidations only; live HOSTS/GROUPS refresh
 * from `hostStorageAdapter.readString`. Same-content echoes must not
 * `setHosts` (that flash is the other half of the bug).
 */
import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

const source = readFileSync(new URL("./useVaultState.ts", import.meta.url), "utf8");

test("browser storage events are ignored when the canonical profile is live", () => {
  assert.match(
    source,
    /const isAuthoritativeBrowserStorageEvent[\s\S]*if \(hasHostProfileClient\(\)\) return false;[\s\S]*eventNewValue === localStorageAdapter\.readString\(key\)/,
  );
  assert.match(
    source,
    /if \(!isAuthoritativeBrowserStorageEvent\(key, event\.newValue\)\) return;/,
  );
});

test("hosts and groups refresh from the live adapter cache, not StorageEvent.newValue", () => {
  assert.match(
    source,
    /handleLocalStorageAdapterChanged[\s\S]*STORAGE_KEY_HOSTS[\s\S]*STORAGE_KEY_GROUPS[\s\S]*applyVaultStorageValue\(key, localStorageAdapter\.readString\(key\)\)/,
  );
  assert.doesNotMatch(
    source,
    /if \(key === STORAGE_KEY_HOSTS\) \{[\s\S]*safeParse<Host\[\]>\(event\.newValue\)/,
  );
  assert.doesNotMatch(
    source,
    /if \(key === STORAGE_KEY_GROUPS\) \{[\s\S]*safeParse<string\[\]>\(event\.newValue\)/,
  );
});

test("same-content host storage echoes do not setHosts", () => {
  // Decrypting the blob we just wrote allocates a new array. Applying it
  // flashes the tree even when nothing changed.
  assert.match(
    source,
    /if \(key === STORAGE_KEY_HOSTS\) \{[\s\S]*if \(vaultSnapshotsEqual\(sanitized, hostsRef\.current\)\) return;[\s\S]*setHosts\(sanitized\)/,
  );
  assert.match(
    source,
    /if \(key === STORAGE_KEY_GROUPS\) \{[\s\S]*if \(vaultSnapshotsEqual\(next, customGroupsRef\.current\)\) return;[\s\S]*setCustomGroups\(next\)/,
  );
});
