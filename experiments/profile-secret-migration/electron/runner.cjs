"use strict";

const fs = require("node:fs");
const path = require("node:path");
const crypto = require("node:crypto");

const RESULT_PREFIX = "PROFILE_SECRET_MIGRATION_PROBE_RESULT ";
const startedAt = Date.now();
const capturedConsole = [];
for (const method of ["log", "warn", "error", "info", "debug"]) {
  console[method] = (...values) => capturedConsole.push(Buffer.from(values.map((value) => String(value?.message || value)).join(" "), "utf8"));
}

let electron;
let isolatedRoot = null;
let corpus = null;
let evidence = null;
let resultWritten = false;

function resultShape(passed, cleanup, leakScan) {
  return {
    formatVersion: 1,
    platform: process.platform,
    provider: process.platform === "win32" ? "windows-dpapi-user" : "unavailable",
    protocolVersion: 1,
    fixtureCount: corpus?.fixtures?.length || 0,
    metadataCount: corpus?.metadata?.length || 0,
    passed,
    cleanup,
    leakScan,
    duration: Date.now() - startedAt,
  };
}

function writeResult(result, exitCode) {
  if (resultWritten) return;
  resultWritten = true;
  process.stdout.write(`${RESULT_PREFIX}${JSON.stringify(result)}\n`, () => electron.app.exit(exitCode));
}

async function main() {
  electron = require("electron");
  const args = process.argv.slice(2);
  const binaryPath = args[0];
  const mode = args[1] || "";
  if (args.length < 1 || args.length > 2 || (mode && !/^--interrupt=(after-spawn|after-handshake|after-first-secret)$/.test(mode))) {
    throw new Error("invalid runner arguments");
  }
  const interruptAt = mode ? mode.slice("--interrupt=".length) : null;
  const tempDirBridge = require(path.resolve(__dirname, "..", "..", "..", "electron", "bridges", "tempDirBridge.cjs"));
  const dedicatedTemp = tempDirBridge.getTempDir();
  isolatedRoot = fs.mkdtempSync(path.join(dedicatedTemp, "profile-secret-migration-"));
  electron.app.setPath("userData", path.join(isolatedRoot, "userData"));
  electron.app.commandLine.appendSwitch("disable-gpu");
  electron.app.commandLine.appendSwitch("disable-logging");
  electron.app.on("window-all-closed", () => {});
  await electron.app.whenReady();
  if (process.platform !== "win32") throw new Error("Windows evidence required");
  if (!electron.safeStorage.isEncryptionAvailable()) throw new Error("secure storage unavailable");

  const credentialBridge = require(path.resolve(__dirname, "..", "..", "..", "electron", "bridges", "credentialBridge.cjs"));
  const corpusModule = require("./corpus.cjs");
  const { runInteroperability } = require("./interop.cjs");
  const { runLeakScan } = require("./leakScanner.cjs");
  corpus = corpusModule.buildSyntheticCorpus();
  corpusModule.verifyNegativeSources(electron.safeStorage, credentialBridge);

  evidence = await runInteroperability({
    binaryPath,
    fixtures: corpus.fixtures,
    prepareFixture: (fixture) => corpusModule.prepareFixture(fixture, electron.safeStorage, credentialBridge),
    metadata: corpus.metadata,
    prepareMetadata: (entry) => entry.data,
    interruptAt,
  });
  const marker = resultShape(true, true, true);
  const canaries = corpus.fixtures.map((fixture) => fixture.plaintext);
  const leakScan = runLeakScan({
    canaries,
    capturedStdout: [...capturedConsole, ...evidence.capturedStdout],
    capturedStderr: evidence.capturedStderr,
    marker,
    receipt: evidence.receipt,
    root: isolatedRoot,
    argv: evidence.argv,
    envValues: evidence.envValues,
  });
  if (!leakScan) throw new Error("leak scan failed");
  fs.rmSync(isolatedRoot, { recursive: true, force: true });
  if (fs.existsSync(isolatedRoot)) throw new Error("isolated cleanup failed");
  isolatedRoot = null;
  writeResult(resultShape(true, true, true), 0);
}

void main().catch(async (error) => {
  let cleanup = Boolean(error?.probeEvidence?.cleanup || evidence?.cleanup);
  let leakScan = false;
  try {
    const canaries = corpus?.fixtures?.map((fixture) => fixture.plaintext) || [];
    if (canaries.length > 0 && isolatedRoot) {
      const failureEvidence = error?.probeEvidence || evidence || {};
      leakScan = require("./leakScanner.cjs").runLeakScan({
        canaries,
        capturedStdout: [...capturedConsole, ...(failureEvidence.capturedStdout || [])],
        capturedStderr: failureEvidence.capturedStderr || [],
        marker: resultShape(false, cleanup, false),
        receipt: failureEvidence.receipt || {},
        root: isolatedRoot,
        argv: failureEvidence.argv || process.argv.slice(2),
        envValues: failureEvidence.envValues || Object.values(process.env),
      });
    }
  } catch {
    leakScan = false;
  }
  try {
    if (isolatedRoot) fs.rmSync(isolatedRoot, { recursive: true, force: true });
    cleanup = cleanup && (!isolatedRoot || !fs.existsSync(isolatedRoot));
    isolatedRoot = null;
  } catch {
    cleanup = false;
  }
  if (!electron) {
    process.stdout.write(`${RESULT_PREFIX}${JSON.stringify(resultShape(false, cleanup, leakScan))}\n`);
    process.exitCode = 1;
    return;
  }
  writeResult(resultShape(false, cleanup, leakScan), 1);
});
