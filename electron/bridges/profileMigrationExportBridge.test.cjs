"use strict";

const assert = require("node:assert/strict");
const test = require("node:test");
const { createTrustedMigrationExport } = require("./profileMigrationExportBridge.cjs");

test("trusted migration export rejects untrusted origin", () => {
  assert.throws(() => createTrustedMigrationExport({ origin: "http://evil.example" }), /trusted/);
});

test("trusted adapter validates origin again at call time", async () => {
  let released = false;
  const adapter = createTrustedMigrationExport({
    origin: "netcatty://app",
    writerLease: {
      async acquire() { return { epoch: 1 }; },
      async release() { released = true; },
    },
    backup: { async create() { return { backupPath: "backup.db" }; } },
    async readRecords() { return [{ domain: "settings", key: "theme", classification: "canonical-migrated", valueBase64: "ZGFyaw==" }]; },
    targetPublicKey: require("node:crypto").generateKeyPairSync("x25519").publicKey.export({ format: "der", type: "spki" }).toString("base64"),
    sourceFingerprint: "fingerprint-123456",
  });
  await assert.rejects(() => adapter.exportBundle({ origin: "http://evil.example" }), /trusted/);
  assert.equal(released, false);
});
