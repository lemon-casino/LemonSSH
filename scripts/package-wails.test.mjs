import assert from "node:assert/strict";
import { test } from "node:test";
import { mkdtemp, readFile, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";

import {
  artifactBasename,
  buildLdflags,
  checksumEntries,
  helperResourcePath,
  parseArgs,
  purityInventory,
  windowsGuiLdflags,
} from "./package-wails.mjs";

test("artifactBasename applies the platform executable suffix", () => {
  assert.equal(artifactBasename("1.2.3", "windows", "amd64"), "LemonSSH-1.2.3-windows-amd64.exe");
  assert.equal(artifactBasename("1.2.3", "darwin", "arm64"), "LemonSSH-1.2.3-darwin-arm64");
  assert.equal(artifactBasename("1.2.3", "linux", "amd64"), "LemonSSH-1.2.3-linux-amd64");
});

test("artifactBasename rejects unknown targets", () => {
  assert.throws(() => artifactBasename("1.2.3", "sunos", "amd64"), /unsupported GOOS/);
  assert.throws(() => artifactBasename("1.2.3", "linux", "386"), /unsupported GOARCH/);
  assert.throws(() => artifactBasename("", "linux", "amd64"), /version is required/);
});

test("buildLdflags strips the quote characters", () => {
  assert.equal(buildLdflags('1.2.3'), "-s -w -X main.version=1.2.3");
  assert.equal(buildLdflags('a"b'), "-s -w -X main.version=ab");
});

test("windowsGuiLdflags hides the console on Windows GUI builds", () => {
  assert.equal(windowsGuiLdflags("windows"), " -H windowsgui");
  assert.equal(windowsGuiLdflags("linux"), "");
  assert.equal(windowsGuiLdflags("darwin"), "");
});

test("parseArgs accepts the documented flags", () => {
  const args = parseArgs(["--version", "9.9.9", "--goos", "linux", "--goarch", "arm64", "--skip-frontend", "--out-dir", "out"]);
  assert.deepEqual(args, {
    version: "9.9.9",
    goos: "linux",
    goarch: "arm64",
    skipFrontend: true,
    outDir: "out",
  });
  assert.throws(() => parseArgs(["--nonsense"]), /unknown argument/);
});

test("checksumEntries hashes files deterministically", async () => {
  const dir = await mkdtemp(path.join(tmpdir(), "pkg-wails-"));
  const fileA = path.join(dir, "a.txt");
  const fileB = path.join(dir, "b.txt");
  await writeFile(fileA, "alpha");
  await writeFile(fileB, "beta");
  const entries = await checksumEntries([fileA, fileB]);
  assert.equal(entries.length, 2);
  assert.equal(entries[0].path, fileA);
  assert.equal(entries[0].bytes, 5);
  assert.match(entries[0].sha256, /^[0-9a-f]{64}$/);
  assert.notEqual(entries[0].sha256, entries[1].sha256);
  // Deterministic re-run.
  const again = await checksumEntries([fileA]);
  assert.equal(again[0].sha256, entries[0].sha256);
  await readFile(entries[0].path, "utf8"); // still readable after hashing
});

test("purityInventory never claims a signed installer", () => {
  const inventory = purityInventory({
    artifactName: "LemonSSH-1.0.0-windows-amd64.exe",
    sha256: "a".repeat(64),
    bytes: 12,
    goos: "windows",
    goarch: "amd64",
    cross: false,
  });
  assert.equal(inventory.signed, false);
  assert.deepEqual(inventory.installerFormats, []);
  assert.ok(inventory.electronMarkers.includes("electron"));
  assert.ok(inventory.notes.some((note) => note.includes("P8-01")));
});

test("helperResourcePath follows the fetch-mosh layout", () => {
  assert.equal(helperResourcePath("windows", "amd64", "mosh"), path.join("resources", "mosh", "win32-x64", "mosh-client.exe"));
  assert.equal(helperResourcePath("darwin", "arm64", "mosh"), path.join("resources", "mosh", "darwin-universal", "mosh-client"));
  assert.equal(helperResourcePath("linux", "arm64", "et"), path.join("resources", "et", "linux-arm64", "et"));
});
