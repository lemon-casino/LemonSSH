"use strict";

// P2-01 drift test: every storage key in storageKeys.ts must exist in the
// frozen data inventory with a valid classification. Adding a new key without
// regenerating the inventory (npm run generate:data-inventory) fails here and
// in CI, keeping Gate 6 "no unclassified data" machine-enforced.

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import path from "node:path";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "..");
const storageKeysSource = readFileSync(path.join(repoRoot, "infrastructure", "config", "storageKeys.ts"), "utf8");
const inventory = JSON.parse(
  readFileSync(path.join(repoRoot, "testdata", "migration", "electron", "data-inventory.json"), "utf8"),
);

const CLASSIFICATIONS = new Set(["canonical-migrated", "device-local", "transient-cache", "retired"]);

function sourceKeyValues() {
  const values = new Set();
  const pattern = /'((?:netcatty|debug|__netcatty)[A-Za-z0-9_.:]*)'/g;
  for (const match of storageKeysSource.matchAll(pattern)) {
    values.add(match[1]);
  }
  return values;
}

test("every storage key is classified in the data inventory", () => {
  const inventoryValues = new Set(inventory.entries.map((entry) => entry.value));
  const missing = [...sourceKeyValues()].filter((value) => !inventoryValues.has(value));
  assert.deepEqual(missing, [], "unclassified storage keys; run npm run generate:data-inventory");
});

test("every inventory classification is valid and consistent", () => {
  for (const entry of inventory.entries) {
    assert.ok(
      CLASSIFICATIONS.has(entry.classification),
      `${entry.value} has invalid classification ${entry.classification}`,
    );
    assert.equal(typeof entry.secretBearing, "boolean", entry.value);
    assert.equal(typeof entry.syncRelation, "string", entry.value);
  }
});

test("retired and transient keys never enter the sync payload candidates", () => {
  for (const entry of inventory.entries) {
    if (entry.classification === "device-local" || entry.classification === "transient-cache" || entry.classification === "retired") {
      assert.match(entry.syncRelation, /^never synced$/, entry.value);
    }
  }
});

test("secret-bearing keys are explicit and expected", () => {
  const secrets = inventory.entries.filter((entry) => entry.secretBearing).map((entry) => entry.value);
  for (const expected of [
    "netcatty_hosts_v1",
    "netcatty_keys_v1",
    "netcatty_default_key_passphrases_v1",
    "netcatty_ai_providers_v1",
  ]) {
    assert.ok(secrets.includes(expected), `${expected} must stay marked secret-bearing`);
  }
});
