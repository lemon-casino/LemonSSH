"use strict";

const crypto = require("node:crypto");

const ENC_V1_IDS = [
  "host.password", "host.telnet-password", "host.proxy-password",
  "key.private-key", "key.passphrase", "identity.password", "proxy-profile.password",
  "group.password", "group.telnet-password", "group.proxy-password", "default-key.passphrase",
  "ai.provider-api-key", "ai.external-agent-api-key", "ai.web-search-api-key",
  "sync.oauth-access-token", "sync.oauth-refresh-token", "sync.webdav-password",
  "sync.webdav-token", "sync.s3-secret", "sync.s3-session-token", "legacy.github-token",
  "legacy.gist-token", "plugin.sync-opaque-config", "plugin.durable-credential-ref",
];

const RAW_IDS = [
  "raw.cloud-master-password",
  "raw.local-backup-payload",
  "raw.plugin-secret-store-value",
];

const EDGE_IDS = [
  "edge.unicode",
  "edge.multiline-private-key",
  "edge.max-plaintext",
  "edge.enc-v1-prefix",
];

const REQUIRED_FIXTURE_IDS = [...ENC_V1_IDS, ...RAW_IDS, ...EDGE_IDS];
const RESIDUAL_INVENTORY_GAPS = ["custom headers", "environment secrets", "plugin writeOnly fields"];

function randomToken(bytes = 32) {
  return crypto.randomBytes(bytes).toString("base64url");
}

function buildSyntheticCorpus() {
  const fixtures = [];
  for (const id of ENC_V1_IDS) {
    fixtures.push(makeFixture(id, "enc:v1", `synthetic:${id}:${randomToken()}`));
  }
  for (const id of RAW_IDS) {
    fixtures.push(makeFixture(id, "safeStorage-raw", `synthetic:${id}:${randomToken(48)}`));
  }
  fixtures.push(makeFixture("edge.unicode", "enc:v1", `秘密-🔐-данные-${randomToken(48)}`));
  fixtures.push(makeFixture(
    "edge.multiline-private-key",
    "enc:v1",
    `-----BEGIN SYNTHETIC PRIVATE KEY-----\n${crypto.randomBytes(512).toString("base64").match(/.{1,64}/g).join("\n")}\n-----END SYNTHETIC PRIVATE KEY-----`,
  ));
  fixtures.push(makeFixture("edge.max-plaintext", "enc:v1", crypto.randomBytes(64 * 1024).toString("hex")));
  fixtures.push(makeFixture("edge.enc-v1-prefix", "enc:v1", `enc:v1:not-ciphertext-${randomToken(48)}`));
  const metadata = [
    { id: "metadata.app-lock-verifier", classification: "verifier", data: `synthetic:app-lock-verifier:${randomToken(24)}` },
    { id: "metadata.sync-master-key-config", classification: "key-derivation-config", data: `synthetic:kdf-config:${randomToken(24)}` },
    { id: "metadata.empty-absent-optional", classification: "empty-or-absent", data: "" },
  ];
  validateCorpus({ fixtures, metadata });
  return { fixtures, metadata, residualInventoryGaps: [...RESIDUAL_INVENTORY_GAPS] };
}

function makeFixture(id, sourceFormat, plaintext) {
  return {
    id,
    sourceFormat,
    purpose: `profile-secret-migration/${id}`,
    plaintext,
  };
}

function validateCorpus(corpus) {
  if (!corpus || !Array.isArray(corpus.fixtures) || !Array.isArray(corpus.metadata)) {
    throw new Error("invalid corpus");
  }
  if (corpus.fixtures.length !== REQUIRED_FIXTURE_IDS.length || corpus.metadata.length !== 3) {
    throw new Error("incomplete corpus");
  }
  const seenValues = new Set();
  corpus.fixtures.forEach((fixture, index) => {
    if (fixture.id !== REQUIRED_FIXTURE_IDS[index]) throw new Error("fixture order mismatch");
    if (fixture.purpose !== `profile-secret-migration/${fixture.id}`) throw new Error("fixture purpose mismatch");
    if (fixture.sourceFormat !== (RAW_IDS.includes(fixture.id) ? "safeStorage-raw" : "enc:v1")) {
      throw new Error("fixture source format mismatch");
    }
    const size = Buffer.byteLength(fixture.plaintext, "utf8");
    if (size === 0 || size > 128 * 1024 || seenValues.has(fixture.plaintext)) throw new Error("invalid fixture plaintext");
    seenValues.add(fixture.plaintext);
  });
  if (Buffer.byteLength(corpus.fixtures.at(-2).plaintext, "utf8") !== 128 * 1024) {
    throw new Error("bounded fixture size mismatch");
  }
  corpus.metadata.forEach((entry) => {
    if (typeof entry.data !== "string" || Buffer.byteLength(entry.data, "utf8") > 64 * 1024) {
      throw new Error("invalid metadata payload");
    }
  });
  if (corpus.metadata.at(-1).data !== "") throw new Error("empty-absent metadata must carry empty payload");
  return true;
}

function decryptEncV1OrThrow(value, plaintext, storage, credentialBridge) {
  if (typeof value !== "string" || !value.startsWith(credentialBridge.ENC_PREFIX)) {
    throw new Error("source encryption failed");
  }
  const decrypted = credentialBridge.decryptCredentialValue(value, storage);
  if (decrypted === value) throw new Error("source ciphertext unchanged");
  if (decrypted !== plaintext) throw new Error("source plaintext mismatch");
  return decrypted;
}

function decryptRawOrThrow(ciphertext, plaintext, storage) {
  if (!Buffer.isBuffer(ciphertext) || ciphertext.length === 0) throw new Error("source encryption failed");
  let decrypted;
  try {
    decrypted = storage.decryptString(ciphertext);
  } catch {
    throw new Error("source decrypt failed");
  }
  if (decrypted !== plaintext) throw new Error("source plaintext mismatch");
  return decrypted;
}

function prepareFixture(fixture, storage, credentialBridge) {
  if (fixture.sourceFormat === "enc:v1") {
    const ciphertext = credentialBridge.encryptCredentialValue(fixture.plaintext, storage);
    return decryptEncV1OrThrow(ciphertext, fixture.plaintext, storage, credentialBridge);
  }
  const ciphertext = storage.encryptString(fixture.plaintext);
  return decryptRawOrThrow(ciphertext, fixture.plaintext, storage);
}

function verifyNegativeSources(storage, credentialBridge) {
  const seed = `negative-seed:${randomToken(48)}`;
  const valid = credentialBridge.encryptCredentialValue(seed, storage);
  if (valid === seed || !valid.startsWith(credentialBridge.ENC_PREFIX)) throw new Error("negative setup failed");
  const payload = valid.slice(credentialBridge.ENC_PREFIX.length);
  const dpapiHeader = Buffer.from([
    0x01, 0x00, 0x00, 0x00, 0xd0, 0x8c, 0x9d, 0xdf, 0x01, 0x15,
    0xd1, 0x11, 0x8c, 0x7a, 0x00, 0xc0, 0x4f, 0xc2, 0x97, 0xeb,
  ]).toString("base64");
  const cases = [
    { id: "corrupt", value: "enc:v1:%%%" },
    { id: "truncated", value: `enc:v1:${payload.slice(0, Math.max(4, payload.length - 12))}` },
    { id: "header-only", value: `enc:v1:${dpapiHeader}` },
    { id: "foreign", value: `enc:v1:${Buffer.concat([Buffer.from(dpapiHeader, "base64"), crypto.randomBytes(64)]).toString("base64")}` },
  ];
  for (const item of cases) {
    let rejected = false;
    try {
      decryptEncV1OrThrow(item.value, seed, storage, credentialBridge);
    } catch {
      rejected = true;
    }
    if (!rejected) throw new Error("negative source accepted");
  }
  const unavailable = { isEncryptionAvailable: () => false, decryptString: storage.decryptString.bind(storage) };
  let unavailableRejected = false;
  try {
    decryptEncV1OrThrow(valid, seed, unavailable, credentialBridge);
  } catch {
    unavailableRejected = true;
  }
  if (!unavailableRejected) throw new Error("unavailable storage accepted");
  return true;
}

module.exports = {
  ENC_V1_IDS,
  RAW_IDS,
  REQUIRED_FIXTURE_IDS,
  RESIDUAL_INVENTORY_GAPS,
  buildSyntheticCorpus,
  decryptEncV1OrThrow,
  prepareFixture,
  validateCorpus,
  verifyNegativeSources,
};
