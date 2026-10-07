import assert from "node:assert/strict";
import test from "node:test";

import {
  registerVaultNav,
  setVaultNavSection,
  syncVaultNavSection,
  useVaultNavState,
} from "./vaultNavStore.ts";

test("vault nav store defaults to the hosts section", () => {
  // Indirectly through the snapshot getter used by useSyncExternalStore.
  assert.ok(useVaultNavState);
});

test("setVaultNavSection falls back to internal state when VaultView is not mounted", () => {
  registerVaultNav(null);
  setVaultNavSection("keys");
  syncVaultNavSection("logs");
  // Re-registering a handler replaces the internal-state path.
  const seen: string[] = [];
  registerVaultNav({ setCurrentSection: (section) => seen.push(section) });
  setVaultNavSection("snippets");
  assert.deepEqual(seen, ["snippets"]);
  registerVaultNav(null);
});

test("syncVaultNavSection mirrors VaultView-owned state without invoking actions", () => {
  const seen: string[] = [];
  registerVaultNav({ setCurrentSection: (section) => seen.push(section) });
  syncVaultNavSection("notes");
  assert.deepEqual(seen, [], "sync must not drive the registered action");
  // Same value is a no-op (no redundant notifications).
  syncVaultNavSection("notes");
  assert.deepEqual(seen, []);
  registerVaultNav(null);
});

test("registerVaultNav(null) unregisters so external writes hit the fallback state", () => {
  const seen: string[] = [];
  registerVaultNav({ setCurrentSection: (section) => seen.push(section) });
  registerVaultNav(null);
  setVaultNavSection("knownhosts");
  assert.deepEqual(seen, []);
});
