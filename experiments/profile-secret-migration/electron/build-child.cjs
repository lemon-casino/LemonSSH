"use strict";

const path = require("node:path");
const { spawnSync } = require("node:child_process");

function withMutedConsole(fn) {
  const saved = { ...console };
  console.log = () => {};
  console.error = () => {};
  console.warn = () => {};
  try {
    return fn();
  } finally {
    Object.assign(console, saved);
  }
}

function buildChild() {
  const probeRoot = path.resolve(__dirname, "..");
  const tempDirBridge = require(path.resolve(probeRoot, "..", "..", "electron", "bridges", "tempDirBridge.cjs"));
  const binaryPath = withMutedConsole(() => tempDirBridge.getTempFilePath("profile-secret-migration-probe.exe"));
  const result = spawnSync("go", ["build", "-o", binaryPath, "./cmd/profile-secret-migration-probe"], {
    cwd: probeRoot,
    shell: false,
    windowsHide: true,
    stdio: ["ignore", "pipe", "pipe"],
    encoding: "utf8",
    timeout: 120_000,
  });
  if (result.status !== 0 || result.error) throw new Error("Go child build failed");
  return binaryPath;
}

if (require.main === module) {
  try {
    process.stdout.write(`${buildChild()}\n`);
  } catch {
    process.stderr.write("profile secret migration child build failed\n");
    process.exitCode = 1;
  }
}

module.exports = { buildChild };
