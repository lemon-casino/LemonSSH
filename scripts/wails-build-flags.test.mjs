import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { artifactPath, buildCommand, hostGoos, windowsGuiLdflags } from "./wails-build.mjs";
import { buildLdflags } from "./package-wails.mjs";

test("wails:build entry points at the build script", () => {
  const pkg = JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8"));
  assert.equal(pkg.scripts["wails:build"], "node scripts/wails-build.mjs");
});

test("-H windowsgui and the .exe artifact apply to windows only", () => {
  assert.equal(windowsGuiLdflags("windows"), " -H windowsgui");
  assert.equal(windowsGuiLdflags("darwin"), "");
  assert.equal(windowsGuiLdflags("linux"), "");
  assert.equal(artifactPath("windows"), "bin/LemonSSH.exe");
  assert.equal(artifactPath("darwin"), "bin/LemonSSH");
  assert.equal(artifactPath("linux"), "bin/LemonSSH");
});

test("host goos mapping covers the supported desktops and rejects the rest", () => {
  assert.equal(hostGoos("win32"), "windows");
  assert.equal(hostGoos("darwin"), "darwin");
  assert.equal(hostGoos("linux"), "linux");
  assert.throws(() => hostGoos("freebsd"));
});

test("buildCommand stamps the update public key only when provided, like package-wails", () => {
  const key = "ab".repeat(32);
  assert.equal(
    buildCommand("1.2.3", "windows", key),
    `go build -trimpath "-ldflags=-s -w -X main.version=1.2.3 -X main.updatePublicKey=${key} -H windowsgui" -o bin/LemonSSH.exe ./cmd/lemonssh`,
  );
  // No key: byte-identical to the historical ldflags, so keyless host builds
  // behave exactly as before.
  assert.equal(
    buildCommand("1.2.3", "windows"),
    'go build -trimpath "-ldflags=-s -w -X main.version=1.2.3 -H windowsgui" -o bin/LemonSSH.exe ./cmd/lemonssh',
  );
  assert.equal(
    buildCommand("1.2.3", "linux", key),
    `go build -trimpath "-ldflags=${buildLdflags("1.2.3", key)}" -o bin/LemonSSH ./cmd/lemonssh`,
  );
  // Invalid keys are rejected before the linker ever runs.
  assert.throws(() => buildCommand("1.2.3", "windows", "nothex"), /64 hex chars/);
});
