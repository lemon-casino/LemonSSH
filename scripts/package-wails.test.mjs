import assert from "node:assert/strict";
import { test } from "node:test";
import { mkdtemp, mkdir, readdir, readFile, rm, writeFile, realpath } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";

// macOS /var is a system symlink; tests use the physical temp directory so
// helper path checks can continue rejecting symlink ancestors.
const tempRoot = await realpath(tmpdir());

import {
  artifactBasename,
  buildLdflags,
  checksumEntries,
  helperResourcePath,
  normalizeUpdatePublicKey,
  parseArgs,
  purityInventory,
  shouldUseShell,
  stampWindowsVersionResource,
  windowsGuiLdflags,
  verifyHelper,
  writeProtocolResources,
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

test("buildLdflags injects the update public key only when provided", () => {
  // Without a key (the common no-secret environment) behavior is unchanged
  // and the runtime keeps checksums.txt-only verification.
  const key = "a".repeat(64);
  assert.equal(buildLdflags("1.2.3", ""), "-s -w -X main.version=1.2.3");
  assert.equal(buildLdflags("1.2.3"), "-s -w -X main.version=1.2.3");
  assert.equal(
    buildLdflags("1.2.3", key),
    `-s -w -X main.version=1.2.3 -X main.updatePublicKey=${key}`,
  );
});

test("normalizeUpdatePublicKey enforces the ed25519 hex shape", () => {
  assert.equal(normalizeUpdatePublicKey(undefined), "");
  assert.equal(normalizeUpdatePublicKey(""), "");
  assert.equal(normalizeUpdatePublicKey(null), "");
  const key = "D75A980182B10AB7D54BFED3C964073A0EE172F3DAA62325AF021A68F707511A";
  assert.equal(normalizeUpdatePublicKey(` ${key.toLowerCase()} `), key.toLowerCase(), "trims and lowercases");
  assert.throws(() => normalizeUpdatePublicKey("short"), /64 hex chars/);
  assert.throws(() => normalizeUpdatePublicKey("z".repeat(64)), /64 hex chars/);
  assert.throws(() => normalizeUpdatePublicKey("a".repeat(65)), /64 hex chars/);
});

test("windowsGuiLdflags hides the console on Windows GUI builds", () => {
  assert.equal(windowsGuiLdflags("windows"), " -H windowsgui");
  assert.equal(windowsGuiLdflags("linux"), "");
  assert.equal(windowsGuiLdflags("darwin"), "");
});

test("parseArgs accepts the documented flags", () => {
  const args = parseArgs(["--version", "9.9.9", "--goos", "linux", "--goarch", "arm64", "--skip-frontend", "--out-dir", "out", "--update-public-key", "a".repeat(64)]);
  assert.deepEqual(args, {
    version: "9.9.9",
    goos: "linux",
    goarch: "arm64",
    skipFrontend: true,
    outDir: "out",
    updatePublicKey: "a".repeat(64),
  });
  assert.throws(() => parseArgs(["--nonsense"]), /unknown argument/);
});

test("checksumEntries hashes files deterministically", async () => {
  const dir = await mkdtemp(path.join(tempRoot, "pkg-wails-"));
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
  assert.ok(inventory.forbiddenRuntimeMarkers.includes("electron"));
  assert.match(inventory.notes.join("\n"), /not a release gate|optional/i);
});

test("shouldUseShell runs npm through the shell on Windows", () => {
  assert.equal(shouldUseShell("npm", "win32"), true);
  assert.equal(shouldUseShell("npm", "linux"), true);
  assert.equal(shouldUseShell("go", "linux"), false);
  assert.equal(shouldUseShell("go", "win32"), true);
});

test("helper verification rejects altered content and wrong machine architecture", async () => {
  const { createHash } = await import("node:crypto");
  const pe = Buffer.alloc(128);
  pe.write("MZ"); pe.writeUInt32LE(64, 60); pe.write("PE\0\0", 64); pe.writeUInt16LE(0x8664, 68);
  const pin = { os: "windows", arch: "amd64", sha256: createHash("sha256").update(pe).digest("hex") };
  assert.doesNotThrow(() => verifyHelper(pe, pin, "windows", "amd64"));
  assert.throws(() => verifyHelper(Buffer.from("tampered"), pin, "windows", "amd64"), /hash/);
  assert.throws(() => verifyHelper(pe, pin, "windows", "arm64"), /target/);
  const wrong = { ...pin, arch: "arm64" };
  assert.throws(() => verifyHelper(pe, wrong, "windows", "arm64"), /architecture/);
});

test("ad-hoc helper installation stays removed; the lock gates all provisioning", async () => {
  // Ad-hoc installs were removed: provisioning goes exclusively through the
  // reviewed lock flow (scripts/fetch-wails-helpers.mjs), whose digest and
  // provenance gates run before any byte reaches the cache or resources.
  const mod = await import("./package-wails.mjs");
  assert.ok(!("installHelper" in mod), "legacy ad-hoc helper install must stay removed");
  const { validateLock } = await import("./fetch-wails-helpers.mjs");
  // A lock without trusted digests and provenance is rejected before any
  // download or install can start.
  assert.throws(
    () => validateLock({ schemaVersion: 1, releases: {}, assets: [{}] }),
    /invalid helper release|trusted sha256|provenance/i,
  );
});

test("protocol resources register all supported schemes", async () => {
  const dir = await mkdtemp(path.join(tempRoot, "protocol-wails-"));
  const linux = await writeProtocolResources(dir, "linux", "LemonSSH");
  const desktop = await readFile(linux[0], "utf8");
  assert.match(desktop, /Exec=LemonSSH %u/);
  for (const scheme of ["ssh", "telnet", "lemonssh", "netcatty"]) assert.ok(desktop.includes(`x-scheme-handler/${scheme};`));
  const mac = await writeProtocolResources(dir, "darwin", "LemonSSH");
  const plist = await readFile(mac[0], "utf8");
  assert.match(plist, /<key>CFBundleIdentifier<\/key><string>app\.lemonssh\.desktop<\/string>/);
  for (const scheme of ["ssh", "telnet", "lemonssh", "netcatty"]) assert.ok(plist.includes(`<string>${scheme}</string>`));
});

test("helperResourcePath follows the fetch-mosh layout", () => {
  assert.equal(helperResourcePath("windows", "amd64", "mosh"), path.join("resources", "mosh", "win32-x64", "mosh-client.exe"));
  assert.equal(helperResourcePath("darwin", "arm64", "mosh"), path.join("resources", "mosh", "darwin-universal", "mosh-client"));
  assert.equal(helperResourcePath("linux", "arm64", "et"), path.join("resources", "et", "linux-arm64", "et"));
});

test("versionParts extracts the numeric file version from package versions", async () => {
  const { versionParts } = await import("./windows-version-info.mjs");
  assert.deepEqual(versionParts("0.0.1"), { major: 0, minor: 0, patch: 1, build: 0 });
  assert.deepEqual(versionParts("1.2.3"), { major: 1, minor: 2, patch: 3, build: 0 });
  assert.deepEqual(versionParts("v1.2"), { major: 1, minor: 2, patch: 0, build: 0 });
  // Prerelease/build suffixes stay out of the numeric quadruple.
  assert.deepEqual(versionParts("0.0.1-beta.1"), { major: 0, minor: 0, patch: 1, build: 0 });
  assert.deepEqual(versionParts("2.0.0+build.5"), { major: 2, minor: 0, patch: 0, build: 0 });
  assert.throws(() => versionParts("banana"), /unsupported version string/);
  assert.throws(() => versionParts(""), /unsupported version string/);
});

test("winresVersionInfo renders the winres info.json shape", async () => {
  const { lemonsshWinresInfo } = await import("./windows-version-info.mjs");
  const info = lemonsshWinresInfo("1.2.3");
  assert.deepEqual(info.fixed, { file_version: "1.2.3.0", product_version: "1.2.3.0" });
  assert.deepEqual(info.info["0409"], {
    Comments: "",
    CompanyName: "LemonSSH",
    FileDescription: "LemonSSH",
    FileVersion: "1.2.3",
    InternalName: "LemonSSH",
    LegalCopyright: "",
    OriginalFilename: "LemonSSH.exe",
    ProductName: "LemonSSH",
    ProductVersion: "1.2.3",
  });
  assert.throws(() => lemonsshWinresInfo(""), /version is required/);
});

test("generateWindowsSyso runs the pinned tool against a stamped temp info.json", async () => {
  const { generateWindowsSyso, buildSysoCommand } = await import("./winres.mjs");
  const commandTempRoot = await mkdtemp(path.join(tempRoot, "winres-cmd-"));
  const commands = [];
  const result = await generateWindowsSyso({
    version: "9.9.9",
    arch: "amd64",
    out: path.join("cmd", "lemonssh", "rsrc_windows_amd64.syso"),
    run: async (command) => commands.push(command),
    tempRoot: commandTempRoot,
  });
  assert.equal(result.version, "9.9.9");
  assert.equal(commands.length, 1);
  assert.match(commands[0], /go run github\.com\/wailsapp\/wails\/v3\/cmd\/wails3@v3\.0\.0-beta\.12 generate syso/);
  assert.match(commands[0], /-arch=amd64/);
  assert.match(commands[0], /-info="[^"]+info\.json"/);
  // The stamped info file is temporary: its directory is removed even when
  // the tool invocation fails.
  const infoPath = commands[0].match(/-info="([^"]+info\.json)"/)[1];
  const tempDir = path.dirname(infoPath);
  await assert.rejects(() => readdir(tempDir), /ENOENT/);
  // A failing tool invocation still cleans its stamped info file up.
  await assert.rejects(
    () => generateWindowsSyso({ version: "1.0.0", run: async () => { throw new Error("tool boom"); }, tempRoot: commandTempRoot }),
    /tool boom/,
  );
  const remaining = [];
  for (const entry of await readdir(commandTempRoot)) remaining.push(entry);
  assert.deepEqual(remaining, []);
  await rm(commandTempRoot, { recursive: true, force: true });

  const command = buildSysoCommand({ arch: "arm64", icon: "i.ico", manifest: "m.manifest", info: "some info.json", out: "out.syso" });
  assert.match(command, /-arch=arm64/);
  assert.match(command, /-icon="i\.ico"/);
  assert.match(command, /-info="some info\.json"/);
});

test("parseWinresArgs only accepts documented flags", async () => {
  const { parseWinresArgs } = await import("./winres.mjs");
  assert.deepEqual(parseWinresArgs(["--version", "1.2.3", "--arch", "arm64"]), { version: "1.2.3", arch: "arm64" });
  assert.deepEqual(parseWinresArgs([]), { arch: "amd64" });
  assert.throws(() => parseWinresArgs(["--nonsense"]), /unknown argument/);
});

test("stampWindowsVersionResource regenerates the syso and restores the committed bytes", async () => {
  const dir = await mkdtemp(path.join(tempRoot, "stamp-wails-"));
  process.chdir(dir);
  const syso = path.join("cmd", "lemonssh", "rsrc_windows_amd64.syso");
  await mkdir(path.dirname(syso), { recursive: true });
  await writeFile(syso, Buffer.from("committed-bytes"));

  const commands = [];
  let restore = await stampWindowsVersionResource({
    version: "4.5.6",
    goarch: "amd64",
    runCommand: async (command) => commands.push(command),
  });
  assert.match(commands[0], /generate syso/);
  assert.match(commands[0], /rsrc_windows_amd64\.syso/);
  await restore();
  assert.equal((await readFile(syso)).toString(), "committed-bytes");

  // A newly created syso (no committed bytes) is removed on restore.
  await rm(syso, { force: true });
  restore = await stampWindowsVersionResource({ version: "4.5.6", goarch: "amd64", runCommand: async () => {} });
  assert.ok(!(await readFile(syso).then(() => true, () => false)));
  await restore();
  await assert.rejects(() => readFile(syso), /ENOENT/);

  assert.rejects(() => stampWindowsVersionResource({ version: "4.5.6", goarch: "386", runCommand: async () => {} }), /unsupported windows GOARCH/);
  process.chdir(tempRoot);
  await rm(dir, { recursive: true, force: true });
});

