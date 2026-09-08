"use strict";

// Thin trusted-origin adapter for P2-05. All security-sensitive work remains
// in profileMigrationBroker.cjs; this module only validates the renderer
// boundary and supplies the broker with injected backup/lease/read functions.

const { createMigrationBroker } = require("./profileMigrationBroker.cjs");

const TRUSTED_ORIGINS = new Set(["netcatty://app", "http://localhost"]);

function assertTrustedOrigin(origin) {
  if (typeof origin !== "string" || !TRUSTED_ORIGINS.has(origin)) {
    throw new Error("migration export requires trusted Netcatty origin");
  }
}

function createTrustedMigrationExport({ origin, writerLease, backup, readRecords, targetPublicKey, sourceFingerprint, purpose }) {
  assertTrustedOrigin(origin);
  const broker = createMigrationBroker({ writerLease, backup, readRecords, targetPublicKey, sourceFingerprint, purpose });
  return {
    async exportBundle(options) {
      assertTrustedOrigin(options?.origin ?? origin);
      return broker.export({ holder: options?.holder, ttlMs: options?.ttlMs });
    },
  };
}

module.exports = { TRUSTED_ORIGINS, assertTrustedOrigin, createTrustedMigrationExport };
