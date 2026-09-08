"use strict";

// P2-05 Electron-side migration export broker core. The broker accepts only
// injected, already-classified profile records and a target Wails X25519 public
// key. It never writes plaintext temporary files: the bundle is assembled in
// memory, encrypted with an ephemeral key, and returned to the caller.

const crypto = require("node:crypto");

const BUNDLE_VERSION = 1;
const MAX_BUNDLE_BYTES = 32 * 1024 * 1024;
const PURPOSE = "netcatty/profile-migration-bundle/v1";
const SECRET_CLASSIFICATIONS = new Set(["secret", "re-seal-secrets"]);
const RAW_CLASSIFICATIONS = new Set(["canonical-migrated", "preserve-opaque", "preserve-metadata", "device-local", "transient-cache", "retired"]);

function assertPurpose(purpose) {
  if (typeof purpose !== "string" || purpose.length === 0 || purpose.length > 256 || purpose.trim() !== purpose || /[^\x21-\x7e]/.test(purpose)) {
    throw new Error("invalid migration purpose");
  }
}

function decodeX25519PublicKey(value) {
  if (typeof value !== "string" || value.length > 512) throw new Error("invalid target public key");
  const der = Buffer.from(value, "base64");
  if (der.length === 0 || der.length > 256) throw new Error("invalid target public key");
  const key = crypto.createPublicKey({ key: der, format: "der", type: "spki" });
  if (key.asymmetricKeyType !== "x25519") throw new Error("target key must be x25519");
  return { der, key };
}

function canonicalManifest(records) {
  return records.map((record) => ({
    domain: record.domain,
    key: record.key,
    classification: record.classification,
    secret: SECRET_CLASSIFICATIONS.has(record.classification),
  })).sort((a, b) => `${a.domain}/${a.key}`.localeCompare(`${b.domain}/${b.key}`));
}

function validateRecord(record) {
  if (!record || typeof record !== "object" || Array.isArray(record)) throw new Error("invalid migration record");
  if (typeof record.domain !== "string" || !/^[a-z0-9-]{1,64}$/.test(record.domain)) throw new Error("invalid migration domain");
  if (typeof record.key !== "string" || record.key.length === 0 || record.key.length > 256) throw new Error("invalid migration key");
  if (!RAW_CLASSIFICATIONS.has(record.classification) && !SECRET_CLASSIFICATIONS.has(record.classification)) throw new Error("invalid migration classification");
  if (!SECRET_CLASSIFICATIONS.has(record.classification) && typeof record.valueBase64 !== "string") throw new Error("raw record must be base64");
  if (SECRET_CLASSIFICATIONS.has(record.classification) && typeof record.plaintext !== "string") throw new Error("secret record must be unsealed in memory");
}

function deriveBundleKey(shared, purpose) {
  assertPurpose(purpose);
  return Buffer.from(crypto.hkdfSync("sha256", shared, Buffer.from(purpose), Buffer.from(`${PURPOSE}/${purpose}`), 32));
}

function encryptBundle(targetPublicKey, plaintext, purpose) {
  const target = decodeX25519PublicKey(targetPublicKey);
  const ephemeral = crypto.generateKeyPairSync("x25519");
  const ephemeralDer = ephemeral.publicKey.export({ format: "der", type: "spki" });
  const shared = crypto.diffieHellman({ privateKey: ephemeral.privateKey, publicKey: target.key });
  const key = deriveBundleKey(shared, purpose);
  shared.fill(0);
  const nonce = crypto.randomBytes(12);
  const aad = Buffer.from(`${PURPOSE}/${purpose}`, "utf8");
  const cipher = crypto.createCipheriv("aes-256-gcm", key, nonce);
  cipher.setAAD(aad);
  const ciphertext = Buffer.concat([cipher.update(plaintext), cipher.final(), cipher.getAuthTag()]);
  key.fill(0);
  aad.fill(0);
  return {
    version: BUNDLE_VERSION,
    algorithm: "X25519-HKDF-SHA256-AES-256-GCM",
    purpose,
    ephemeralPublicKey: ephemeralDer.toString("base64"),
    nonce: nonce.toString("base64"),
    ciphertext: ciphertext.toString("base64"),
  };
}

function buildBundle({ records, sourceFingerprint, backupManifest, targetPublicKey, purpose = "profile-cutover" }) {
  assertPurpose(purpose);
  if (!Array.isArray(records) || records.length === 0 || records.length > 10000) throw new Error("invalid migration records");
  if (typeof sourceFingerprint !== "string" || sourceFingerprint.length < 16 || sourceFingerprint.length > 128) throw new Error("invalid source fingerprint");
  if (!backupManifest || typeof backupManifest !== "object") throw new Error("protective backup is required");
  records.forEach(validateRecord);

  const payload = {
    version: BUNDLE_VERSION,
    sourceFingerprint,
    records: records.map((record) => {
      if (SECRET_CLASSIFICATIONS.has(record.classification)) {
        return { domain: record.domain, key: record.key, classification: record.classification, plaintext: record.plaintext };
      }
      return { domain: record.domain, key: record.key, classification: record.classification, valueBase64: record.valueBase64 };
    }),
    manifest: canonicalManifest(records),
  };
  const plaintext = Buffer.from(JSON.stringify(payload), "utf8");
  const encrypted = encryptBundle(targetPublicKey, plaintext, purpose);
  plaintext.fill(0);
  const result = {
    bundleVersion: BUNDLE_VERSION,
    sourceFingerprint,
    backupManifest,
    recordCount: records.length,
    secretCount: records.filter((record) => SECRET_CLASSIFICATIONS.has(record.classification)).length,
    encrypted,
  };
  const encoded = Buffer.from(JSON.stringify(result), "utf8");
  if (encoded.length > MAX_BUNDLE_BYTES) throw new Error("migration bundle exceeds bound");
  encoded.fill(0);
  return result;
}

function createMigrationBroker({ writerLease, backup, readRecords, targetPublicKey, sourceFingerprint, purpose }) {
  if (!writerLease || typeof writerLease.acquire !== "function" || typeof writerLease.release !== "function") throw new Error("writer lease is required");
  if (!backup || typeof backup.create !== "function") throw new Error("protective backup is required");
  if (typeof readRecords !== "function") throw new Error("record reader is required");
  let exported = false;
  return {
    async export({ holder, ttlMs = 30000 } = {}) {
      if (exported) throw new Error("migration export already completed");
      const lease = await writerLease.acquire(holder, ttlMs);
      try {
        const backupManifest = await backup.create();
        const records = await readRecords();
        const bundle = buildBundle({ records, sourceFingerprint, backupManifest, targetPublicKey, purpose });
        exported = true;
        return { bundle, leaseEpoch: lease.epoch };
      } finally {
        await writerLease.release();
      }
    },
  };
}

module.exports = {
  BUNDLE_VERSION,
  MAX_BUNDLE_BYTES,
  PURPOSE,
  buildBundle,
  createMigrationBroker,
  decodeX25519PublicKey,
  encryptBundle,
  validateRecord,
};
