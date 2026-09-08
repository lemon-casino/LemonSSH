"use strict";

// P2-05 source reader + leak scan. The default record reader enumerates every
// canonical storage key from the P2-01 data inventory through a storage
// accessor (Electron session localStorage in production, Map in tests) and
// classifies each record using the frozen inventory. The leak scan asserts the
// encrypted bundle never contains secret plaintext canaries.

const fs = require("node:fs");
const path = require("node:path");

const CANONICAL = "canonical-migrated";
const SECRET = "re-seal-secrets";

let cachedInventory = null;
function loadInventory() {
  if (cachedInventory) return cachedInventory;
  const inventoryPath = path.resolve(__dirname, "..", "..", "testdata", "migration", "electron", "data-inventory.json");
  cachedInventory = JSON.parse(fs.readFileSync(inventoryPath, "utf8"));
  return cachedInventory;
}

function classificationFor(value) {
  const inventory = loadInventory();
  const entry = inventory.entries.find((candidate) => candidate.value === value);
  if (!entry) throw new Error(`unclassified storage key: ${value}`);
  return entry.secretBearing ? SECRET : CANONICAL;
}

/**
 * Builds classified migration records from a storage accessor. Only
 * canonical-migrated keys are exported; device-local/transient/retired keys
 * are skipped by the P2-01 classification.
 */
function buildMigrationRecords(storage) {
  const inventory = loadInventory();
  const records = [];
  for (const entry of inventory.entries) {
    if (entry.classification !== CANONICAL) continue;
    const raw = storage.getItem(entry.value);
    if (raw === null || raw === undefined) continue;
    if (entry.secretBearing) {
      // The Electron export path unseals enc:v1 values in memory only; the
      // accessor here must already return the unsealed plaintext (the
      // credentialBridge decrypt step is applied by the caller).
      records.push({ domain: "settings", key: entry.value, classification: SECRET, plaintext: String(raw) });
    } else {
      records.push({
        domain: "settings",
        key: entry.value,
        classification: CANONICAL,
        valueBase64: Buffer.from(String(raw), "utf8").toString("base64"),
      });
    }
  }
  return records;
}

/**
 * Asserts no plaintext canary appears in the serialized bundle. Returns the
 * number of canaries checked; throws on the first leak.
 */
function scanBundleForLeaks(bundle, canaries) {
  const serialized = JSON.stringify(bundle);
  let checked = 0;
  for (const canary of canaries) {
    if (typeof canary !== "string" || canary.length < 4) continue;
    checked += 1;
    if (serialized.includes(canary)) {
      throw new Error(`plaintext leak detected in migration bundle (canary ${checked})`);
    }
    if (Buffer.from(canary, "utf8").toString("base64").length >= 8 && serialized.includes(Buffer.from(canary, "utf8").toString("base64"))) {
      throw new Error(`base64 leak detected in migration bundle (canary ${checked})`);
    }
  }
  return checked;
}

module.exports = { buildMigrationRecords, scanBundleForLeaks, classificationFor };
