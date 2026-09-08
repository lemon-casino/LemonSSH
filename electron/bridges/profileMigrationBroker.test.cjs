"use strict";

const assert = require("node:assert/strict");
const crypto = require("node:crypto");
const test = require("node:test");

const {
  BUNDLE_VERSION,
  buildBundle,
  createMigrationBroker,
  decodeX25519PublicKey,
} = require("./profileMigrationBroker.cjs");

function targetKeyPair() {
  const pair = crypto.generateKeyPairSync("x25519");
  return {
    pair,
    der: pair.publicKey.export({ format: "der", type: "spki" }).toString("base64"),
  };
}

function decryptBundle(bundle, targetPair, purpose = "profile-cutover") {
  const ephemeralDer = Buffer.from(bundle.encrypted.ephemeralPublicKey, "base64");
  const ephemeral = crypto.createPublicKey({ key: ephemeralDer, format: "der", type: "spki" });
  const shared = crypto.diffieHellman({ privateKey: targetPair.privateKey, publicKey: ephemeral });
  const key = Buffer.from(crypto.hkdfSync("sha256", shared, Buffer.from(purpose), Buffer.from(`netcatty/profile-migration-bundle/v1/${purpose}`), 32));
  const nonce = Buffer.from(bundle.encrypted.nonce, "base64");
  const ciphertext = Buffer.from(bundle.encrypted.ciphertext, "base64");
  const aad = Buffer.from(`netcatty/profile-migration-bundle/v1/${purpose}`, "utf8");
  const decipher = crypto.createDecipheriv("aes-256-gcm", key, nonce);
  decipher.setAAD(aad);
  decipher.setAuthTag(ciphertext.subarray(-16));
  const plaintext = Buffer.concat([decipher.update(ciphertext.subarray(0, -16)), decipher.final()]);
  return JSON.parse(plaintext.toString("utf8"));
}

test("buildBundle encrypts classified raw and secret records to the target", () => {
  const target = targetKeyPair();
  const bundle = buildBundle({
    targetPublicKey: target.der,
    sourceFingerprint: "source-fingerprint-1234",
    backupManifest: { backupPath: "backup.db", originalSha256: "abc" },
    records: [
      { domain: "vault", key: "netcatty_hosts_v1", classification: "canonical-migrated", valueBase64: "eyJob3N0cyI6W119" },
      { domain: "vault", key: "host.password", classification: "re-seal-secrets", plaintext: "not-in-output" },
    ],
  });
  assert.equal(bundle.bundleVersion, BUNDLE_VERSION);
  assert.equal(bundle.recordCount, 2);
  assert.equal(bundle.secretCount, 1);
  assert.equal(JSON.stringify(bundle).includes("not-in-output"), false);
  const payload = decryptBundle(bundle, target.pair);
  assert.equal(payload.sourceFingerprint, "source-fingerprint-1234");
  assert.equal(payload.records[1].plaintext, "not-in-output");
  assert.equal(payload.manifest.some((entry) => entry.secret === true), true);
});

test("bundle validation fails closed", () => {
  const target = targetKeyPair();
  const common = {
    targetPublicKey: target.der,
    sourceFingerprint: "source-fingerprint-1234",
    backupManifest: { backupPath: "backup.db" },
  };
  assert.throws(() => buildBundle({ ...common, records: [] }));
  assert.throws(() => buildBundle({ ...common, records: [{ domain: "vault", key: "x", classification: "secret" }] }));
  assert.throws(() => buildBundle({ ...common, records: [{ domain: "vault", key: "x", classification: "canonical-migrated", valueBase64: "AA==" }], targetPublicKey: "bad" }));
  assert.throws(() => decodeX25519PublicKey("bad"));
});

test("broker acquires backup and lease, exports once, and always releases", async () => {
  const calls = [];
  const target = targetKeyPair();
  const broker = createMigrationBroker({
    writerLease: {
      async acquire(holder, ttlMs) {
        calls.push(["acquire", holder, ttlMs]);
        return { epoch: 7 };
      },
      async release() {
        calls.push(["release"]);
      },
    },
    backup: { async create() { calls.push(["backup"]); return { backupPath: "backup.db" }; } },
    async readRecords() {
      calls.push(["records"]);
      return [{ domain: "settings", key: "theme", classification: "canonical-migrated", valueBase64: "ZGFyaw==" }];
    },
    targetPublicKey: target.der,
    sourceFingerprint: "source-fingerprint-1234",
  });
  const result = await broker.export({ holder: "electron", ttlMs: 1234 });
  assert.equal(result.leaseEpoch, 7);
  assert.deepEqual(calls.map((entry) => entry[0]), ["acquire", "backup", "records", "release"]);
  await assert.rejects(() => broker.export({ holder: "electron" }), /already completed/);
});

test("broker releases lease when export fails", async () => {
  let released = false;
  const target = targetKeyPair();
  const broker = createMigrationBroker({
    writerLease: {
      async acquire() { return { epoch: 1 }; },
      async release() { released = true; },
    },
    backup: { async create() { return { backupPath: "backup.db" }; } },
    async readRecords() { throw new Error("read failed"); },
    targetPublicKey: target.der,
    sourceFingerprint: "source-fingerprint-1234",
  });
  await assert.rejects(() => broker.export(), /read failed/);
  assert.equal(released, true);
});
