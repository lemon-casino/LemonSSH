"use strict";

const fs = require("node:fs");
const path = require("node:path");
const { spawn } = require("node:child_process");

const {
  FrameReader,
  METADATA_SOURCE_FORMAT,
  PROTOCOL_VERSION,
  createClientHandshake,
  metadataPurpose,
  writeFrame,
} = require("./channel.cjs");

const MAX_STDERR_BYTES = 32 * 1024;
const DEFAULT_TIMEOUT_MS = 30_000;

function validateBinaryPath(binaryPath) {
  if (typeof binaryPath !== "string" || !path.isAbsolute(binaryPath)) throw new Error("binary path must be absolute");
  const stat = fs.statSync(binaryPath);
  if (!stat.isFile()) throw new Error("binary path must name a file");
  return binaryPath;
}

async function runInteroperability({ binaryPath, fixtures, prepareFixture, metadata = [], prepareMetadata = null, timeoutMs = DEFAULT_TIMEOUT_MS, interruptAt = null }) {
  validateBinaryPath(binaryPath);
  if (!Array.isArray(fixtures) || fixtures.length === 0 || fixtures.length > 64) throw new Error("invalid fixture set");
  if (!Array.isArray(metadata) || metadata.length > 16) throw new Error("invalid metadata set");
  if (metadata.length > 0 && typeof prepareMetadata !== "function") throw new Error("missing metadata preparer");
  const startedAt = Date.now();
  const remaining = () => Math.max(1, timeoutMs - (Date.now() - startedAt));
  const capturedStdout = [];
  const capturedStderr = [];
  let stderrBytes = 0;
  let childClosed = false;
  let channel = null;
  let cleanupPromise = null;
  let parentExitHandler;

  const child = spawn(binaryPath, [], {
    shell: false,
    windowsHide: true,
    stdio: ["pipe", "pipe", "pipe"],
    env: { ...process.env },
  });
  const closePromise = new Promise((resolve) => {
    child.once("close", (code, signal) => {
      childClosed = true;
      resolve({ code, signal });
    });
  });
  const spawnErrorPromise = new Promise((_, reject) => {
    child.once("error", () => reject(new Error("child process failed")));
  });
  child.stderr.on("data", (chunk) => {
    const data = Buffer.from(chunk);
    stderrBytes += data.length;
    if (stderrBytes <= MAX_STDERR_BYTES) capturedStderr.push(data);
    if (stderrBytes > MAX_STDERR_BYTES && !childClosed) child.kill();
  });
  const reader = new FrameReader(child.stdout, (chunk) => capturedStdout.push(chunk));
  parentExitHandler = () => {
    if (!childClosed) child.kill();
  };
  process.once("exit", parentExitHandler);

  const cleanup = async () => {
    if (cleanupPromise) return cleanupPromise;
    cleanupPromise = (async () => {
      channel?.destroy();
      if (!child.stdin.destroyed) child.stdin.destroy();
      if (!childClosed) child.kill();
      const boundedClose = Promise.race([
        closePromise,
        new Promise((_, reject) => {
          const timer = setTimeout(() => reject(new Error("child reap deadline exceeded")), 3_000);
          timer.unref?.();
        }),
      ]);
      await boundedClose;
      process.removeListener("exit", parentExitHandler);
      return true;
    })();
    return cleanupPromise;
  };

  const interrupt = async (phase) => {
    if (interruptAt !== phase) return;
    child.kill();
    await closePromise;
    throw new Error("injected child interruption");
  };

  try {
    await interrupt("after-spawn");
    const serverHello = await Promise.race([reader.readJson(remaining()), spawnErrorPromise]);
    channel = createClientHandshake(serverHello);
    await writeFrame(child.stdin, channel.hello);
    const serverConfirmation = await reader.readJson(remaining());
    channel.verifyServerConfirmation(serverConfirmation);
    await writeFrame(child.stdin, channel.clientConfirmation);
    await interrupt("after-handshake");

    let sequence = 1;
    for (const fixture of fixtures) {
      const plaintext = prepareFixture(fixture);
      const payload = Buffer.from(JSON.stringify({ plaintext }), "utf8");
      const ciphertext = channel.seal(sequence, fixture.id, fixture.sourceFormat, fixture.purpose, payload);
      payload.fill(0);
      await writeFrame(child.stdin, {
        v: PROTOCOL_VERSION,
        type: "secret",
        seq: sequence,
        fixtureId: fixture.id,
        sourceFormat: fixture.sourceFormat,
        purpose: fixture.purpose,
        ciphertext,
      });
      if (sequence === 1) await interrupt("after-first-secret");
      const receiptFrame = await reader.readJson(remaining());
      channel.openReceipt(receiptFrame, {
        sequence,
        fixtureId: fixture.id,
        sourceFormat: fixture.sourceFormat,
        purpose: fixture.purpose,
      });
      sequence++;
    }
    for (const entry of metadata) {
      const data = prepareMetadata(entry);
      if (typeof data !== "string" || Buffer.byteLength(data, "utf8") > 64 * 1024) throw new Error("invalid metadata payload");
      const payload = Buffer.from(JSON.stringify({ data }), "utf8");
      const purpose = metadataPurpose(entry.classification);
      const ciphertext = channel.seal(sequence, entry.id, METADATA_SOURCE_FORMAT, purpose, payload);
      payload.fill(0);
      await writeFrame(child.stdin, {
        v: PROTOCOL_VERSION,
        type: "metadata",
        seq: sequence,
        metadataId: entry.id,
        classification: entry.classification,
        ciphertext,
      });
      const receiptFrame = await reader.readJson(remaining());
      const ack = channel.openMetadataReceipt(receiptFrame, {
        sequence,
        metadataId: entry.id,
        classification: entry.classification,
        sourceFormat: METADATA_SOURCE_FORMAT,
        purpose,
      });
      if (ack.data !== data) throw new Error("metadata roundtrip mismatch");
      sequence++;
    }
    await writeFrame(child.stdin, { v: PROTOCOL_VERSION, type: "finish", count: fixtures.length + metadata.length });
    child.stdin.end();
    const status = await Promise.race([
      closePromise,
      new Promise((_, reject) => {
        const timer = setTimeout(() => reject(new Error("child exit deadline exceeded")), remaining());
        timer.unref?.();
      }),
    ]);
    if (status.code !== 0 || status.signal != null || stderrBytes !== 0) throw new Error("child failed");
    channel.destroy();
    channel = null;
    process.removeListener("exit", parentExitHandler);
    return {
      receipt: { count: fixtures.length, passed: true },
      capturedStdout,
      capturedStderr,
      cleanup: true,
      argv: [binaryPath],
      envValues: Object.values(process.env),
    };
  } catch (error) {
    await cleanup();
    error.probeEvidence = {
      capturedStdout,
      capturedStderr,
      cleanup: childClosed,
      argv: [binaryPath],
      envValues: Object.values(process.env),
    };
    throw error;
  }
}

module.exports = {
  DEFAULT_TIMEOUT_MS,
  MAX_STDERR_BYTES,
  runInteroperability,
  validateBinaryPath,
};
