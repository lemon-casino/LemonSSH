"use strict";

const crypto = require("node:crypto");

const PROTOCOL_VERSION = 1;
const PROTOCOL_NAME = "netcatty-profile-secret-migration-v1";
const MAX_FRAME_BYTES = 192 * 1024;
const METADATA_SOURCE_FORMAT = "classified-metadata";

function metadataPurpose(classification) {
  return `profile-secret-migration/metadata/${classification}`;
}

function appendBytes(parts, value) {
  const data = Buffer.isBuffer(value) ? value : Buffer.from(value, "utf8");
  const size = Buffer.allocUnsafe(4);
  size.writeUInt32BE(data.length);
  parts.push(size, data);
}

function canonicalTranscript(runId, challenge, serverSpki, clientSpki) {
  const parts = [];
  appendBytes(parts, PROTOCOL_NAME);
  appendBytes(parts, runId);
  appendBytes(parts, challenge);
  appendBytes(parts, serverSpki);
  appendBytes(parts, clientSpki);
  return Buffer.concat(parts);
}

function canonicalAad(runId, direction, sequence, fixtureId, sourceFormat, purpose) {
  const parts = [];
  appendBytes(parts, PROTOCOL_NAME);
  appendBytes(parts, runId);
  appendBytes(parts, direction);
  const sequenceBytes = Buffer.allocUnsafe(8);
  sequenceBytes.writeBigUInt64BE(BigInt(sequence));
  parts.push(sequenceBytes);
  appendBytes(parts, fixtureId);
  appendBytes(parts, sourceFormat);
  appendBytes(parts, purpose);
  return Buffer.concat(parts);
}

function nonce(direction, sequence) {
  const result = Buffer.alloc(12);
  if (direction === "electron-to-go") result.write("E2G1", 0, "ascii");
  else if (direction === "go-to-electron") result.write("G2E1", 0, "ascii");
  else throw new Error("invalid direction");
  result.writeBigUInt64BE(BigInt(sequence), 4);
  return result;
}

function confirmationProof(key, transcriptHash, role) {
  return crypto.createHmac("sha256", key)
    .update(PROTOCOL_NAME)
    .update(Buffer.from([0]))
    .update(transcriptHash)
    .update(Buffer.from([0]))
    .update(role)
    .digest();
}

function decodeBase64(value, expectedLength, label) {
  if (typeof value !== "string" || !/^[A-Za-z0-9+/]+={0,2}$/.test(value)) {
    throw new Error(`invalid ${label}`);
  }
  const decoded = Buffer.from(value, "base64");
  if (decoded.toString("base64") !== value || (expectedLength != null && decoded.length !== expectedLength)) {
    throw new Error(`invalid ${label}`);
  }
  return decoded;
}

function createClientHandshake(serverHello) {
  if (!isExactObject(serverHello, ["v", "type", "runId", "challenge", "publicKey"])
      || serverHello.v !== PROTOCOL_VERSION || serverHello.type !== "server_hello") {
    throw new Error("invalid server hello");
  }
  const runId = decodeBase64(serverHello.runId, 16, "run id");
  const challenge = decodeBase64(serverHello.challenge, 32, "challenge");
  const serverSpki = decodeBase64(serverHello.publicKey, null, "server public key");
  if (serverSpki.length > 256) throw new Error("invalid server public key");
  const serverPublicKey = crypto.createPublicKey({ key: serverSpki, format: "der", type: "spki" });
  if (serverPublicKey.asymmetricKeyType !== "x25519") throw new Error("invalid server public key");
  const { privateKey, publicKey } = crypto.generateKeyPairSync("x25519");
  const clientSpki = publicKey.export({ format: "der", type: "spki" });
  const shared = crypto.diffieHellman({ privateKey, publicKey: serverPublicKey });
  const transcript = canonicalTranscript(runId, challenge, serverSpki, clientSpki);
  const transcriptHash = crypto.createHash("sha256").update(transcript).digest();
  transcript.fill(0);
  const info = Buffer.from(`${PROTOCOL_NAME}/channel-key/${transcriptHash.toString("base64")}`, "utf8");
  const key = Buffer.from(crypto.hkdfSync("sha256", shared, challenge, info, 32));
  shared.fill(0);
  const serverProof = confirmationProof(key, transcriptHash, "go");
  const clientProof = confirmationProof(key, transcriptHash, "electron");
  transcriptHash.fill(0);
  return {
    hello: {
      v: PROTOCOL_VERSION,
      type: "client_hello",
      runId: serverHello.runId,
      challenge: serverHello.challenge,
      publicKey: clientSpki.toString("base64"),
    },
    verifyServerConfirmation(frame) {
      if (!isExactObject(frame, ["v", "type", "proof"])
          || frame.v !== PROTOCOL_VERSION || frame.type !== "server_confirm") {
        throw new Error("invalid server confirmation");
      }
      const actual = decodeBase64(frame.proof, 32, "server proof");
      if (!crypto.timingSafeEqual(actual, serverProof)) throw new Error("server confirmation failed");
    },
    clientConfirmation: {
      v: PROTOCOL_VERSION,
      type: "client_confirm",
      proof: clientProof.toString("base64"),
    },
    seal(sequence, fixtureId, sourceFormat, purpose, value) {
      const aad = canonicalAad(runId, "electron-to-go", sequence, fixtureId, sourceFormat, purpose);
      const iv = nonce("electron-to-go", sequence);
      const cipher = crypto.createCipheriv("aes-256-gcm", key, iv);
      cipher.setAAD(aad);
      const ciphertext = Buffer.concat([cipher.update(value), cipher.final(), cipher.getAuthTag()]);
      aad.fill(0);
      return ciphertext.toString("base64");
    },
    openReceipt(frame, expected) {
      const keys = ["v", "type", "seq", "fixtureId", "sourceFormat", "purpose", "ciphertext"];
      if (!isExactObject(frame, keys) || frame.v !== PROTOCOL_VERSION || frame.type !== "receipt"
          || frame.seq !== expected.sequence || frame.fixtureId !== expected.fixtureId
          || frame.sourceFormat !== expected.sourceFormat || frame.purpose !== expected.purpose) {
        throw new Error("invalid receipt frame");
      }
      const ciphertext = decodeBase64(frame.ciphertext, null, "receipt ciphertext");
      if (ciphertext.length < 16 || ciphertext.length > MAX_FRAME_BYTES) throw new Error("invalid receipt ciphertext");
      const body = ciphertext.subarray(0, -16);
      const tag = ciphertext.subarray(-16);
      const aad = canonicalAad(runId, "go-to-electron", frame.seq, frame.fixtureId, frame.sourceFormat, frame.purpose);
      const decipher = crypto.createDecipheriv("aes-256-gcm", key, nonce("go-to-electron", frame.seq));
      decipher.setAAD(aad);
      decipher.setAuthTag(tag);
      const plaintext = Buffer.concat([decipher.update(body), decipher.final()]);
      aad.fill(0);
      let receipt;
      try {
        receipt = parseStrictJson(plaintext);
      } finally {
        plaintext.fill(0);
        ciphertext.fill(0);
      }
      if (!isExactObject(receipt, ["passed"]) || receipt.passed !== true) throw new Error("invalid receipt");
      return { passed: true };
    },
    openMetadataReceipt(frame, expected) {
      const keys = ["v", "type", "seq", "metadataId", "classification", "ciphertext"];
      if (!isExactObject(frame, keys) || frame.v !== PROTOCOL_VERSION || frame.type !== "metadata_receipt"
          || frame.seq !== expected.sequence || frame.metadataId !== expected.metadataId
          || frame.classification !== expected.classification) {
        throw new Error("invalid metadata receipt frame");
      }
      const ciphertext = decodeBase64(frame.ciphertext, null, "metadata receipt ciphertext");
      if (ciphertext.length < 16 || ciphertext.length > MAX_FRAME_BYTES) throw new Error("invalid metadata receipt ciphertext");
      const body = ciphertext.subarray(0, -16);
      const tag = ciphertext.subarray(-16);
      const aad = canonicalAad(runId, "go-to-electron", frame.seq, frame.metadataId, expected.sourceFormat, expected.purpose);
      const decipher = crypto.createDecipheriv("aes-256-gcm", key, nonce("go-to-electron", frame.seq));
      decipher.setAAD(aad);
      decipher.setAuthTag(tag);
      const plaintext = Buffer.concat([decipher.update(body), decipher.final()]);
      aad.fill(0);
      let receipt;
      try {
        receipt = parseStrictJson(plaintext);
      } finally {
        plaintext.fill(0);
        ciphertext.fill(0);
      }
      if (!isExactObject(receipt, ["passed", "data"]) || receipt.passed !== true || typeof receipt.data !== "string") {
        throw new Error("invalid metadata receipt");
      }
      return { passed: true, data: receipt.data };
    },
    destroy() {
      key.fill(0);
      serverProof.fill(0);
      clientProof.fill(0);
      runId.fill(0);
      challenge.fill(0);
    },
  };
}

class FrameReader {
  constructor(stream, capture, maxFrameBytes = MAX_FRAME_BYTES) {
    this.stream = stream;
    this.capture = capture;
    this.maxFrameBytes = maxFrameBytes;
    this.buffer = Buffer.alloc(0);
    this.waiters = [];
    this.ended = false;
    this.error = null;
    stream.on("data", (chunk) => this.onData(chunk));
    stream.on("end", () => this.onEnd());
    stream.on("error", (error) => this.onError(error));
  }

  onData(chunk) {
    const data = Buffer.from(chunk);
    this.capture?.(data);
    this.buffer = Buffer.concat([this.buffer, data]);
    this.pump();
  }

  onEnd() {
    this.ended = true;
    this.pump();
  }

  onError(error) {
    this.error = error;
    this.pump();
  }

  pump() {
    while (this.waiters.length > 0) {
      const waiter = this.waiters[0];
      if (this.error) {
        this.waiters.shift();
        waiter.reject(new Error("protocol stream failed"));
        continue;
      }
      if (this.buffer.length >= 4) {
        const size = this.buffer.readUInt32BE(0);
        if (size === 0 || size > this.maxFrameBytes) {
          this.waiters.shift();
          waiter.reject(new Error("invalid frame size"));
          continue;
        }
        if (this.buffer.length >= size + 4) {
          const frame = this.buffer.subarray(4, size + 4);
          this.buffer = this.buffer.subarray(size + 4);
          this.waiters.shift();
          waiter.resolve(Buffer.from(frame));
          continue;
        }
      }
      if (this.ended) {
        this.waiters.shift();
        waiter.reject(new Error(this.buffer.length === 0 ? "protocol EOF" : "partial protocol frame"));
        continue;
      }
      break;
    }
  }

  read(timeoutMs) {
    return new Promise((resolve, reject) => {
      const waiter = {
        resolve: (value) => {
          clearTimeout(timer);
          resolve(value);
        },
        reject: (error) => {
          clearTimeout(timer);
          reject(error);
        },
      };
      const timer = setTimeout(() => {
        const index = this.waiters.indexOf(waiter);
        if (index >= 0) this.waiters.splice(index, 1);
        reject(new Error("protocol deadline exceeded"));
      }, timeoutMs);
      timer.unref?.();
      this.waiters.push(waiter);
      this.pump();
    });
  }

  async readJson(timeoutMs) {
    return parseStrictJson(await this.read(timeoutMs));
  }
}

function parseStrictJson(data) {
  const text = Buffer.isBuffer(data) ? data.toString("utf8") : String(data);
  if (Buffer.byteLength(text, "utf8") !== (Buffer.isBuffer(data) ? data.length : Buffer.byteLength(text))) {
    throw new Error("invalid UTF-8 frame");
  }
  let value;
  try {
    value = JSON.parse(text);
  } catch {
    throw new Error("invalid JSON frame");
  }
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error("invalid JSON frame");
  return value;
}

function isExactObject(value, keys) {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const actual = Object.keys(value).sort();
  const expected = [...keys].sort();
  return actual.length === expected.length && actual.every((key, index) => key === expected[index]);
}

function encodeFrame(value) {
  const body = Buffer.from(JSON.stringify(value), "utf8");
  if (body.length === 0 || body.length > MAX_FRAME_BYTES) throw new Error("invalid frame size");
  const header = Buffer.allocUnsafe(4);
  header.writeUInt32BE(body.length);
  return Buffer.concat([header, body]);
}

function writeFrame(stream, value) {
  return new Promise((resolve, reject) => {
    const frame = encodeFrame(value);
    stream.write(frame, (error) => error ? reject(new Error("protocol write failed")) : resolve());
  });
}

module.exports = {
  FrameReader,
  MAX_FRAME_BYTES,
  METADATA_SOURCE_FORMAT,
  PROTOCOL_VERSION,
  canonicalAad,
  createClientHandshake,
  encodeFrame,
  isExactObject,
  metadataPurpose,
  nonce,
  parseStrictJson,
  writeFrame,
};
