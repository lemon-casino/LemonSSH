import assert from "node:assert/strict";
import crypto from "node:crypto";
import fs from "node:fs";
import { createRequire } from "node:module";
import childProcess from "node:child_process";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import { sanitizeSessionRestorePayload } from "../../domain/sessionRestore.ts";

import {
  assertRpcMessage,
  createDefinitionValidator,
} from "../../electron/plugins/contractValidator.cjs";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const rootDir = path.resolve(scriptDir, "../..");
const fixtureDir = path.join(rootDir, "testdata/migration/electron");
const require = createRequire(import.meta.url);
const {
  TERMINAL_SUSTAINED_OUTPUT_WORKLOAD,
  makeTerminalSustainedOutputChunks,
} = require("../terminal-sustained-output-workload.cjs");

const readJson = (name) => JSON.parse(fs.readFileSync(path.join(fixtureDir, name), "utf8"));
const normalizeTextLineEndings = (value) => value.replace(/\r\n?/g, "\n");
const sha256 = (value) => crypto.createHash("sha256").update(value).digest("hex");

test("Electron migration fixture manifest covers every generated fixture", () => {
  const manifest = readJson("manifest.json");
  assert.equal(manifest.formatVersion, 1);
  assert.deepEqual(Object.keys(manifest.fixtures).sort(), [
    "agent-tool-specs.json",
    "bridge-contract-index.json",
    "capability-catalog.json",
    "cli-capabilities.json",
    "mcp-tool-specs.json",
    "plugin-contract-baseline.json",
    "plugin-contract-fixtures.json",
    "runtime-contract-fixtures.json",
    "runtime-contract-fixtures.typecheck.ts",
    "runtime-owner-index.json",
    "storage-key-candidates.json",
    "terminal-flow-constants.json",
    "terminal-sustained-output-workload.json",
  ]);
});

test("baseline check rejects an unexpected generated file", () => {
  const unexpectedPath = path.join(fixtureDir, "stale-generated-fixture.json");
  fs.writeFileSync(unexpectedPath, "{}\n", "utf8");
  try {
    const result = childProcess.spawnSync(
      process.execPath,
      [path.join(rootDir, "scripts/migration/export-electron-contract-fixtures.mjs"), "--check"],
      { cwd: rootDir, encoding: "utf8" },
    );
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /stale-generated-fixture\.json is an unexpected generated file/);
  } finally {
    fs.rmSync(unexpectedPath, { force: true });
  }
});

test("manifest hashes normalized sources and exact generated fixture bytes", () => {
  const manifest = readJson("manifest.json");
  for (const relativePath of [
    "scripts/terminal-sustained-output-workload.cjs",
    "scripts/xterm-keyword-highlight-throughput.live.test.cjs",
  ]) {
    const source = fs.readFileSync(path.join(rootDir, relativePath), "utf8");
    const normalized = normalizeTextLineEndings(source);
    const simulatedCrlf = normalized.replaceAll("\n", "\r\n");
    assert.equal(manifest.sourceFiles[relativePath], sha256(normalized), relativePath);
    assert.equal(
      sha256(normalizeTextLineEndings(simulatedCrlf)),
      manifest.sourceFiles[relativePath],
      `${relativePath} simulated CRLF`,
    );
  }

  for (const [name, expectedHash] of Object.entries(manifest.fixtures)) {
    assert.equal(
      sha256(fs.readFileSync(path.join(fixtureDir, name))),
      expectedHash,
      name,
    );
  }
});

test("terminal sustained-output fixture summarizes the canonical producer", () => {
  const fixture = readJson("terminal-sustained-output-workload.json");
  const manifest = readJson("manifest.json");
  const chunks = makeTerminalSustainedOutputChunks();
  const hash = crypto.createHash("sha256");
  const chunkSizeCounts = new Map();
  let totalBytes = 0;
  let totalChars = 0;

  for (const chunk of chunks) {
    const bytes = Buffer.byteLength(chunk, TERMINAL_SUSTAINED_OUTPUT_WORKLOAD.encoding);
    totalBytes += bytes;
    totalChars += chunk.length;
    hash.update(chunk, TERMINAL_SUSTAINED_OUTPUT_WORKLOAD.encoding);
    chunkSizeCounts.set(bytes, (chunkSizeCounts.get(bytes) ?? 0) + 1);
  }

  assert.equal(fixture.formatVersion, 1);
  assert.deepEqual(fixture.workload, TERMINAL_SUSTAINED_OUTPUT_WORKLOAD);
  assert.deepEqual(fixture.summary, {
    chunkCount: 1600,
    chunkSizeDistribution: [...chunkSizeCounts.entries()]
      .sort(([left], [right]) => left - right)
      .map(([bytes, count]) => ({ bytes, count })),
    payloadSha256: hash.digest("hex"),
    totalBytes,
    totalChars,
  });
  assert.equal(fixture.summary.totalBytes, 9_708_106);
  assert.equal(fixture.summary.totalChars, 9_708_106);
  assert.equal(fixture.summary.payloadSha256, "a72132b19b586c26f76ee1c635870b5dddb6727f744cd2099e9104745114c0e1");
  assert.equal(Object.hasOwn(fixture, "chunks"), false);
  assert.equal(JSON.stringify(fixture).includes("payload="), false);
  assert.equal(typeof manifest.sourceFiles[fixture.workload.generatorSource], "string");
  assert.equal(typeof manifest.sourceFiles[fixture.workload.canonicalBenchmarkSource], "string");
});

test("Electron migration fixtures contain only synthetic runtime identities", () => {
  const runtime = readJson("runtime-contract-fixtures.json");
  assert.equal(runtime.sessionRestore.sessions[0].hostname, "example.invalid");
  assert.equal(runtime.agentEvents.at(0).sessionId, "chat-session-fixture-1");

  const serialized = JSON.stringify(runtime);
  for (const forbidden of [
    "password",
    "privateKey",
    "passphrase",
    "apiKey",
    "BEGIN OPENSSH PRIVATE KEY",
  ]) {
    assert.equal(serialized.includes(forbidden), false, forbidden);
  }

  assert.deepEqual(sanitizeSessionRestorePayload(runtime.sessionRestore), runtime.sessionRestore);
});

test("baseline fixtures preserve capability and protocol invariants", () => {
  const capabilities = readJson("capability-catalog.json");
  const mcp = readJson("mcp-tool-specs.json");
  const cli = readJson("cli-capabilities.json");
  const plugin = readJson("plugin-contract-baseline.json");
  const flow = readJson("terminal-flow-constants.json");

  assert.equal(new Set(capabilities.map((entry) => entry.id)).size, capabilities.length);
  assert.equal(mcp.some((entry) => entry.mcpTool === "terminal_execute"), true);
  assert.equal(cli.some((entry) => entry.command.join(" ") === "sftp read"), true);
  assert.equal(plugin.limits.RpcLimits.maxJsonBytes, 1024 * 1024);
  assert.equal(plugin.limits.StreamLimits.maxChunkBytes, 16 * 1024 * 1024);
  assert.equal(flow.FLOW_HIGH_WATER_MARK, 1024 * 1024);
  assert.ok(flow.FLOW_LOW_WATER_MARK < flow.FLOW_HIGH_WATER_MARK);
});

test("runtime owner index covers the migration risk centers", () => {
  const owners = readJson("runtime-owner-index.json");
  const areas = new Set(owners.map((entry) => entry.area));
  for (const required of [
    "terminal-data-plane",
    "ssh",
    "profile-persistence",
    "agent-runtime",
    "plugin-runtime",
  ]) {
    assert.equal(areas.has(required), true, required);
  }
  assert.equal(owners.every((entry) => entry.owners.length > 0 && entry.tests.length > 0), true);
  for (const entry of owners) {
    for (const relativePath of [...entry.owners, ...entry.tests]) {
      assert.equal(fs.existsSync(path.join(rootDir, relativePath)), true, relativePath);
    }
  }
});

test("bridge and storage candidates cover their canonical TypeScript sources", () => {
  const bridges = readJson("bridge-contract-index.json");
  const storageKeys = readJson("storage-key-candidates.json");
  assert.equal(bridges.length, 8);
  assert.equal(bridges.every((entry) => entry.methods.length > 0), true);
  for (const bridge of bridges) {
    assert.equal(
      bridge.declaration,
      normalizeTextLineEndings(fs.readFileSync(path.join(rootDir, bridge.source), "utf8")),
      bridge.source,
    );
    assert.equal(bridge.declarations.length > 0, true, bridge.source);
    for (const declaration of bridge.declarations) {
      assert.equal(
        declaration.declaration,
        normalizeTextLineEndings(fs.readFileSync(path.join(rootDir, declaration.source), "utf8")),
        declaration.source,
      );
    }
  }
  const importedSources = new Set(bridges.flatMap((bridge) => (
    bridge.declarations.map((declaration) => declaration.source)
  )));
  assert.equal(importedSources.has("domain/sync.ts"), true);
  assert.equal(importedSources.has("domain/appLock.ts"), true);
  assert.equal(importedSources.has("infrastructure/ai/types.ts"), true);
  assert.equal(storageKeys.some((entry) => entry.name === "STORAGE_KEY_HOSTS"), true);
  assert.equal(storageKeys.some((entry) => entry.name === "STORAGE_KEY_SESSION_RESTORE"), true);
  assert.equal(storageKeys.every((entry) => entry.value || entry.alias), true);
});

test("plugin fixtures pass and fail through production contract validators", () => {
  const fixtures = readJson("plugin-contract-fixtures.json");
  const validateManifest = createDefinitionValidator("PluginManifest");
  const validatePath = createDefinitionValidator("RelativePackagePath");

  assert.equal(validateManifest(fixtures.validManifest), true);
  assert.equal(validateManifest(fixtures.invalidManifestTraversal), false);
  assert.doesNotThrow(() => assertRpcMessage(fixtures.rpcRequest));
  assert.doesNotThrow(() => assertRpcMessage(fixtures.rpcSuccess));
  for (const rejectedPath of fixtures.rejectedPackagePaths) {
    assert.equal(validatePath(rejectedPath), false, rejectedPath);
  }
});
