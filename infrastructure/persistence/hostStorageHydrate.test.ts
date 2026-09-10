import assert from "node:assert/strict";
import { test } from "node:test";

import { hydrateLocalStorageFromProfile, listHydrationKeys } from "./hostStorageHydrate";

test("hydrate skips keys that already exist locally", async () => {
  const local = new Map<string, string>([["theme", "dark"]]);
  const remote = new Map<string, string>([["theme", "light"], ["font", "mono"]]);
  const hydrated = await hydrateLocalStorageFromProfile(
    {
      getRawText: async (_domain, key) => remote.get(key),
    },
    {
      readString: (key) => local.get(key) ?? null,
      writeString: (key, value) => {
        local.set(key, value);
        return true;
      },
    },
    "settings",
    ["theme", "font"],
  );
  assert.deepEqual(hydrated, ["font"]);
  assert.equal(local.get("theme"), "dark");
  assert.equal(local.get("font"), "mono");
});

test("listHydrationKeys falls back when DomainKeys is empty or missing", async () => {
  assert.deepEqual(
    await listHydrationKeys({ getRawText: async () => undefined }, ["a"], "settings"),
    ["a"],
  );
  assert.deepEqual(
    await listHydrationKeys(
      { getRawText: async () => undefined, domainKeys: async () => [] },
      ["a"],
      "settings",
    ),
    ["a"],
  );
  assert.deepEqual(
    await listHydrationKeys(
      { getRawText: async () => undefined, domainKeys: async () => ["b"] },
      ["a"],
      "settings",
    ),
    ["b"],
  );
});
