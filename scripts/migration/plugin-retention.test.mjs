"use strict";

// P2-01A retention check: every table in the plugin v1 SQLite schema must be
// classified in the retention fixture, and the contract properties (no v1
// execution, no grant inheritance, broker-only secrets) stay asserted.

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import path from "node:path";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "..");
const databaseSource = readFileSync(path.join(repoRoot, "electron", "plugins", "database.cjs"), "utf8");
const fixture = JSON.parse(
  readFileSync(path.join(repoRoot, "testdata", "migration", "electron", "plugin-v1-data-retention.json"), "utf8"),
);

function schemaTables() {
  const tables = new Set();
  for (const match of databaseSource.matchAll(/CREATE TABLE (?:IF NOT EXISTS )?([a-z_]+)\s*\(/g)) {
    tables.add(match[1]);
  }
  return tables;
}

test("every plugin database table has a retention disposition", () => {
  const classified = new Set(Object.keys(fixture.tables));
  const unclassified = [...schemaTables()].filter((table) => !classified.has(table));
  assert.deepEqual(unclassified, [], "unclassified plugin tables; update plugin-v1-data-retention.json");
});

test("schema version matches the fixture snapshot", () => {
  const version = databaseSource.match(/const SCHEMA_VERSION = (\d+)/)?.[1];
  assert.equal(version, String(fixture.schemaVersion), "SCHEMA_VERSION drifted; re-verify the retention contract");
});

test("contract properties stay fail-closed", () => {
  assert.equal(fixture.v1Execution, "rejected");
  assert.deepEqual(fixture.v1EntryPoints, ["main.browser", "main.node"]);
  assert.equal(fixture.grantsInherited, false);
  assert.ok(fixture.secretPath.includes("go-keyring-reseal (P2-04)"));
  assert.ok(fixture.preservationNamespacePrefix.startsWith("plugin-v1/"));
});

test("secrets and grants never land in executable or inherited buckets", () => {
  assert.equal(fixture.tables.plugin_secrets, "re-seal-secrets");
  assert.equal(fixture.tables.plugin_permission_grants, "invalidate-grants");
  assert.equal(fixture.tables.plugin_sync_provider_bindings, "invalidate-grants");
  for (const [table, disposition] of Object.entries(fixture.tables)) {
    if (table.endsWith("_versions") || table === "plugins") {
      assert.equal(disposition, "preserve-metadata", `${table} must be metadata-only`);
    }
  }
});
