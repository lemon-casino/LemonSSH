"use strict";

const assert = require("node:assert/strict");
const crypto = require("node:crypto");
const test = require("node:test");

const {
  TERMINAL_SUSTAINED_OUTPUT_WORKLOAD,
  makeTerminalSustainedOutputChunk,
  makeTerminalSustainedOutputChunks,
} = require("./terminal-sustained-output-workload.cjs");

function summarize(chunks) {
  const hash = crypto.createHash("sha256");
  const chunkSizes = new Map();
  let totalBytes = 0;
  let totalChars = 0;
  for (const chunk of chunks) {
    const bytes = Buffer.byteLength(chunk, TERMINAL_SUSTAINED_OUTPUT_WORKLOAD.encoding);
    totalBytes += bytes;
    totalChars += chunk.length;
    hash.update(chunk, TERMINAL_SUSTAINED_OUTPUT_WORKLOAD.encoding);
    chunkSizes.set(bytes, (chunkSizes.get(bytes) ?? 0) + 1);
  }
  return {
    chunkCount: chunks.length,
    totalBytes,
    totalChars,
    chunkSizeCount: chunkSizes.size,
    minChunkBytes: Math.min(...chunkSizes.keys()),
    maxChunkBytes: Math.max(...chunkSizes.keys()),
    payloadSha256: hash.digest("hex"),
    chunkSizes,
  };
}

test("default sustained-output workload preserves the Electron payload", () => {
  const summary = summarize(makeTerminalSustainedOutputChunks());

  assert.equal(summary.chunkCount, 1600);
  assert.equal(summary.totalBytes, 9_708_106);
  assert.equal(summary.totalChars, 9_708_106);
  assert.equal(summary.payloadSha256, "a72132b19b586c26f76ee1c635870b5dddb6727f744cd2099e9104745114c0e1");
  assert.equal(summary.chunkSizeCount, 117);
  assert.equal(summary.minChunkBytes, 5976);
  assert.equal(summary.maxChunkBytes, 6124);
  assert.equal(summary.chunkSizes.get(6060), 163);
  assert.equal(summary.chunkSizes.get(6124), 168);
  assert.equal([...summary.chunkSizes.values()].reduce((total, count) => total + count, 0), 1600);
});

test("selected chunks preserve line and modulo boundaries", () => {
  const firstLines = makeTerminalSustainedOutputChunk(0).split("\r\n");
  assert.equal(firstLines.pop(), "");
  assert.equal(firstLines.length, 64);
  assert.equal(
    firstLines[0],
    "2026-08-13 INFO worker=0 WARN ERROR failed from 10.2.0.0 payload=xxxxxxxxxxxxxxxxxxxxxxxx",
  );
  assert.equal(
    firstLines[63],
    "2026-08-13 INFO worker=31 WARN ERROR failed from 10.2.63.63 payload=xxxxxxxxxxxxxxxxxxxxxxxx",
  );

  assert.equal(
    makeTerminalSustainedOutputChunk(255).split("\r\n")[0],
    "2026-08-13 INFO worker=0 WARN ERROR failed from 10.2.0.0 payload=xxxxxxxxxxxxxxxxxxxxxxxx",
  );

  const finalLines = makeTerminalSustainedOutputChunk(1599).split("\r\n");
  assert.equal(
    finalLines[63],
    "2026-08-13 INFO worker=31 WARN ERROR failed from 10.2.132.36 payload=xxxxxxxxxxxxxxxxxxxxxxxx",
  );
  assert.deepEqual(
    makeTerminalSustainedOutputChunks(2),
    [makeTerminalSustainedOutputChunk(0), makeTerminalSustainedOutputChunk(1)],
  );
});
