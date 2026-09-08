"use strict";

const assert = require("node:assert/strict");
const crypto = require("node:crypto");
const test = require("node:test");

const { buildMigrationRecords, scanBundleForLeaks } = require("./profileMigrationSourceReader.cjs");
const { buildBundle } = require("./profileMigrationBroker.cjs");

test("source reader exports only canonical keys with inventory classifications", () => {
  const storage = new Map([
    ["netcatty_hosts_v1", JSON.stringify({ hosts: [{ id: "h1", password: "enc:v1:REALSECRET" }] })],
    ["netcatty_theme_v1", "dark"],
    ["netcatty_vault_hosts_view_mode_v1", "tree"], // device-local: skipped
    ["netcatty_sftp_folder_prescan_v1", "true"], // retired: skipped
    ["netcatty_debug_hotkeys", "1"], // transient: skipped
  ]);
  const records = buildMigrationRecords({ getItem: (key) => storage.get(key) ?? null });
  const keys = records.map((record) => record.key);
  assert.ok(keys.includes("netcatty_hosts_v1"));
  assert.ok(keys.includes("netcatty_theme_v1"));
  assert.equal(keys.includes("netcatty_vault_hosts_view_mode_v1"), false);
  assert.equal(keys.includes("netcatty_sftp_folder_prescan_v1"), false);
  assert.equal(keys.includes("netcatty_debug_hotkeys"), false);
  const hosts = records.find((record) => record.key === "netcatty_hosts_v1");
  assert.equal(hosts.classification, "re-seal-secrets");
  const theme = records.find((record) => record.key === "netcatty_theme_v1");
  assert.equal(theme.classification, "canonical-migrated");
});

test("leak scan rejects plaintext and base64 canaries in bundles", () => {
  const target = crypto.generateKeyPairSync("x25519");
  const records = [
    { domain: "settings", key: "theme", classification: "canonical-migrated", valueBase64: Buffer.from("plain-theme", "utf8").toString("base64") },
    { domain: "vault", key: "host.password", classification: "re-seal-secrets", plaintext: "SUPER-CANARY-SECRET-9999" },
  ];
  const bundle = buildBundle({
    targetPublicKey: target.publicKey.export({ format: "der", type: "spki" }).toString("base64"),
    sourceFingerprint: "fingerprint-abcdef123456",
    backupManifest: { backupPath: "backup.db" },
    records,
  });
  // A correctly encrypted bundle contains neither form of the canary.
  assert.equal(scanBundleForLeaks(bundle, ["SUPER-CANARY-SECRET-9999", "plain-theme-long"]), 2);
  // Regression guard: any accidental plaintext/base64 inclusion must throw.
  assert.throws(() => scanBundleForLeaks({ ciphertext: "xSUPER-CANARY-SECRET-9999y" }, ["SUPER-CANARY-SECRET-9999"]), /plaintext leak/);
  assert.throws(() => scanBundleForLeaks({ ciphertext: Buffer.from("plain-theme-long").toString("base64") }, ["plain-theme-long"]), /base64 leak/);
});
