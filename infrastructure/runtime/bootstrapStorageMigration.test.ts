import assert from "node:assert/strict";
import { test } from "node:test";
import { readFile } from "node:fs/promises";
import vm from "node:vm";
import ts from "typescript";

import {
  runStoragePrefixMigration,
  STORAGE_PREFIX_MIGRATION_MARKER,
  type StoragePrefixMigrationStore,
} from "../persistence/storageKeyMigration";

type Fixture = {
  map: Map<string, string>;
  writes: string[];
  requireCalls: string[];
  hydrateCalls: () => number;
  keysAtHydrate: () => string[] | undefined;
  install: () => void;
  hydrateReady: () => Promise<void>;
  setWailsAvailable: (available: boolean) => void;
};

/**
 * Executes the real bootstrap.ts wiring in a sandbox. Native adapters
 * (Wails installer, profile client, canonical hydrate) are stubbed; the
 * storage-key migration is the real implementation, running against a
 * map-backed localStorageAdapter — exactly the object bootstrap hands it.
 */
async function bootFixture(initial: Record<string, string> = {}): Promise<Fixture> {
  const source = await readFile(new URL("./bootstrap.ts", import.meta.url), "utf8");
  const code = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  }).outputText;

  const map = new Map(Object.entries(initial));
  const writes: string[] = [];
  const requireCalls: string[] = [];
  const localStorageAdapter: StoragePrefixMigrationStore = {
    keys: (): string[] => [...map.keys()],
    readString: (key: string): string | null => map.get(key) ?? null,
    writeString: (key: string, value: string): boolean => {
      map.set(key, value);
      writes.push(key);
      return true;
    },
  };
  const wailsModule = {
    available: true,
    installWailsRuntimeClient: (): boolean => {
      requireCalls.push("wails");
      return wailsModule.available;
    },
  };
  let hydrateCalls = 0;
  let keysAtHydrate: string[] | undefined;
  const hostStorageModule = {
    configureHostProfileClient: (_client: unknown): void => {
      requireCalls.push("profile-client");
    },
    hydrateHostProfile: async (): Promise<void> => {
      hydrateCalls += 1;
      keysAtHydrate = [...map.keys()];
    },
  };

  const exports: Record<string, unknown> = {};
  const context = {
    exports,
    console: { error() {} },
    window: { addEventListener() {} },
    require: (name: string) => {
      if (name === "./wails/wailsRuntimeClient") return wailsModule;
      if (name === "./profile/profileClient") return { createProfileClient: () => ({}) };
      if (name === "./runtimeClient") return { getActiveRuntimeClient: () => undefined };
      if (name === "../persistence/hostStorageAdapter") return hostStorageModule;
      if (name === "../persistence/localStorageAdapter") return { localStorageAdapter };
      if (name === "../persistence/storageKeyMigration") return { runStoragePrefixMigration };
      throw new Error(`unexpected bootstrap import: ${name}`);
    },
  };
  vm.runInNewContext(code, context);
  return {
    map,
    writes,
    requireCalls,
    hydrateCalls: () => hydrateCalls,
    keysAtHydrate: () => keysAtHydrate,
    install: () => (exports.installRuntimeClient as () => void)(),
    hydrateReady: () => exports.hydrateReady as Promise<void>,
    setWailsAvailable: (available: boolean) => {
      wailsModule.available = available;
    },
  };
}

const legacySnapshot = {
  netcatty_hosts_v1: '[{"id":"legacy-host"}]',
  __netcatty_pf_cancel_reconnect: "1",
  "netcatty.aiDebug.hide": '["panel"]',
  "netcatty_terminal_encoding_by_host_v1:host-1": "utf-8",
  // Already-migrated and non-brand keys must come through untouched.
  lemonssh_close_behavior_v1: "ask",
  debug_hotkeys: "1",
};

test("installRuntimeClient copies the legacy netcatty snapshot before canonical hydration", async () => {
  const boot = await bootFixture(legacySnapshot);

  boot.install();
  await boot.hydrateReady();

  // Wiring: the Wails guard, the profile client and exactly one hydrate ran.
  assert.deepEqual(boot.requireCalls, ["wails", "profile-client"]);
  assert.equal(boot.hydrateCalls(), 1);

  // New names hold the legacy values after startup.
  assert.equal(boot.map.get("lemonssh_hosts_v1"), '[{"id":"legacy-host"}]');
  assert.equal(boot.map.get("__lemonssh_pf_cancel_reconnect"), "1");
  assert.equal(boot.map.get("lemonssh.aiDebug.hide"), '["panel"]');
  assert.equal(boot.map.get("lemonssh_terminal_encoding_by_host_v1:host-1"), "utf-8");
  // Copy, not rename: the old keys survive the startup migration itself.
  assert.equal(boot.map.get("netcatty_hosts_v1"), '[{"id":"legacy-host"}]');
  assert.equal(boot.map.get("__netcatty_pf_cancel_reconnect"), "1");
  // Non-brand and already-migrated keys are untouched.
  assert.equal(boot.map.get("lemonssh_close_behavior_v1"), "ask");
  assert.equal(boot.map.get("debug_hotkeys"), "1");
  // The completion marker lands so later startups skip the scan.
  assert.match(boot.map.get(STORAGE_PREFIX_MIGRATION_MARKER) ?? "", /"v":1/);

  // Ordering: every lemonssh_ key already existed when the canonical hydrate
  // started, so no consumer can observe the legacy-only state.
  const keysAtHydrate = boot.keysAtHydrate() ?? [];
  for (const key of ["lemonssh_hosts_v1", "__lemonssh_pf_cancel_reconnect", "lemonssh.aiDebug.hide"]) {
    assert.ok(keysAtHydrate.includes(key), `hydrate must see ${key}`);
  }
});

test("a second startup after the marker writes nothing", async () => {
  const boot = await bootFixture({ netcatty_theme_v1: "dark" });
  boot.install();
  await boot.hydrateReady();
  assert.equal(boot.map.get("lemonssh_theme_v1"), "dark");
  const writesAfterFirst = boot.writes.length;

  boot.install();
  await boot.hydrateReady();
  assert.equal(boot.writes.length, writesAfterFirst, "the marker must skip the copy on the next startup");
  assert.equal(boot.map.get("lemonssh_theme_v1"), "dark");
});

test("without the Wails host nothing runs and the snapshot stays legacy-only", async () => {
  const boot = await bootFixture(legacySnapshot);
  boot.setWailsAvailable(false);

  assert.throws(() => boot.install(), /requires the Wails runtime/);
  assert.equal(boot.hydrateCalls(), 0);
  assert.equal(boot.map.size, Object.keys(legacySnapshot).length);
  assert.equal(boot.map.get("lemonssh_hosts_v1"), undefined);
  assert.equal(boot.map.has(STORAGE_PREFIX_MIGRATION_MARKER), false);
});
