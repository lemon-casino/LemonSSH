import assert from "node:assert/strict";
import { test } from "node:test";
import { mkdtemp, mkdir, readFile, writeFile, rm, lstat, link, symlink } from "node:fs/promises";
import path from "node:path";
import { gzipSync } from "node:zlib";
import {
  checkDigest, downloadPinned, githubDownloadArgs, loadLock, packageHelpers,
  preserveFile, readSafeFile, runtimeManifest, selectAssets, sha256,
  validateLock, verifyArchiveFiles, verifyBuildProvenance, verifyInstalled,
  verifySidecar, DEFAULT_CACHE, DEFAULT_RESOURCES,
} from "./fetch-wails-helpers.mjs";
import { packageFiles, packageWails, checksumEntries, verifyHelper } from "./package-wails.mjs";

async function scratch(t) {
  const parent = path.join(DEFAULT_CACHE, "tests");
  await mkdir(parent, { recursive: true });
  const dir = await mkdtemp(path.join(parent, "supply-"));
  t.after(() => rm(dir, { recursive: true, force: true }));
  return dir;
}

function pe(machine = 0x8664) {
  const data = Buffer.alloc(128);
  data.write("MZ"); data.writeUInt32LE(64, 60);
  data.write("PE\0\0", 64); data.writeUInt16LE(machine, 68);
  return data;
}

function universal() {
  const data = Buffer.alloc(512);
  data.writeUInt32BE(0xcafebabe); data.writeUInt32BE(2, 4);
  for (const [i, cpu] of [0x01000007, 0x0100000c].entries()) {
    const entry = 8 + i * 20, offset = 128 + i * 128;
    data.writeUInt32BE(cpu, entry);
    data.writeUInt32BE(offset, entry + 8); data.writeUInt32BE(128, entry + 12);
    data.writeUInt32LE(0xfeedfacf, offset); data.writeUInt32LE(cpu, offset + 4);
  }
  return data;
}

// Build link/path fixtures without creating OS symlinks (no Windows privilege
// dependency), so archive rejection runs on every packaging runner.
function tar(entries) {
  const blocks = [];
  for (const { name, data = Buffer.alloc(0), type = "0", target = "" } of entries) {
    const header = Buffer.alloc(512);
    header.write(name, 0, 100); header.write("0000755\0", 100);
    header.write("0000000\0", 108); header.write("0000000\0", 116);
    header.write(`${data.length.toString(8).padStart(11, "0")}\0`, 124);
    header.write("00000000000\0", 136); header.fill(32, 148, 156);
    header.write(type, 156); header.write(target, 157, 100);
    header.write("ustar\0", 257); header.write("00", 263);
    const sum = header.reduce((a, b) => a + b, 0);
    header.write(`${sum.toString(8).padStart(6, "0")}\0 `, 148);
    blocks.push(header, data, Buffer.alloc((512 - data.length % 512) % 512));
  }
  return gzipSync(Buffer.concat([...blocks, Buffer.alloc(1024)]));
}

async function fixture(t, entries = [{ name: "et.exe", data: pe() }]) {
  const dir = await scratch(t);
  const lock = structuredClone(await loadLock());
  const asset = lock.assets.find((entry) => entry.kind === "et" && entry.os === "windows");
  const data = entries.find((entry) => entry.name === "et.exe")?.data ?? pe();
  asset.files = [{ path: "et.exe", sha256: sha256(data), executable: true }];
  const archive = tar(entries);
  asset.sha256 = sha256(archive);
  const archivePath = path.join(dir, "fixture.tar.gz");
  await writeFile(archivePath, archive);
  const resourcesDir = path.join(dir, "resources");
  const binaryPath = path.join(resourcesDir, "et", asset.directory, "et.exe");
  const pin = runtimeManifest(lock, asset, "amd64");
  await preserveFile(binaryPath, data);
  await preserveFile(`${binaryPath}.manifest.json`, Buffer.from(JSON.stringify(pin)));
  return { dir, lock, asset, archivePath, resourcesDir, binaryPath, pin };
}

test("committed lock covers both helpers for five native targets and rejects unsupported targets", async () => {
  const lock = await loadLock();
  for (const [os, arch] of [["windows", "amd64"], ["darwin", "amd64"], ["darwin", "arm64"], ["linux", "amd64"], ["linux", "arm64"]]) {
    assert.equal(selectAssets(lock, os, arch).length, 2);
  }
  assert.throws(() => selectAssets(lock, "windows", "arm64"), /no locked/);
  assert.throws(() => selectAssets(lock, "darwin", "universal"), /no locked/);
});

test("lock requires source/build/archive/binary provenance and safe names", async () => {
  const original = await loadLock();
  for (const change of [
    (lock) => { delete lock.releases.mosh.source.commit; },
    (lock) => { delete lock.releases.et.build; },
    (lock) => { lock.assets[0].sha256 = ""; },
    (lock) => { lock.assets[0].files[0].sha256 = ""; },
    (lock) => { lock.assets[0].files[0].path = "../escape.exe"; },
    (lock) => { lock.releases.mosh.tag = "moshcatty-0.1.7"; },
    (lock) => { lock.releases.et.licenses[0].url = "https://raw.githubusercontent.com/owner/repo/main/LICENSE"; },
  ]) {
    const lock = structuredClone(original); change(lock);
    assert.throws(() => validateLock(lock));
  }
});

test("archive and executable hashes are independent checks", async (t) => {
  const { archivePath, asset } = await fixture(t);
  assert.equal((await verifyArchiveFiles(archivePath, asset)).length, 1);
  await assert.rejects(verifyArchiveFiles(archivePath, { ...asset, sha256: "0".repeat(64) }), /SHA256 mismatch/);
  await assert.rejects(verifyArchiveFiles(archivePath, { ...asset, files: [{ ...asset.files[0], sha256: "0".repeat(64) }] }), /et.exe SHA256 mismatch/);
});

test("correct archive and binary hashes cannot bless a wrong PE architecture", async (t) => {
  const { archivePath, asset } = await fixture(t, [{ name: "et.exe", data: pe(0xaa64) }]);
  await assert.rejects(verifyArchiveFiles(archivePath, asset), /architecture/);
});

test("universal verification checks real slices, offsets, overlap and CPU identity", () => {
  const data = universal();
  for (const arch of ["amd64", "arm64"]) {
    assert.doesNotThrow(() => verifyHelper(data, { os: "darwin", arch, sha256: sha256(data) }, "darwin", arch));
  }
  for (const mutate of [
    (value) => value.writeUInt32BE(999999, 36),
    (value) => value.writeUInt32LE(0x01000007, 260),
    (value) => value.writeUInt32BE(128, 36),
    (value) => value.writeUInt32BE(999, 4),
  ]) {
    const invalid = Buffer.from(data); mutate(invalid);
    assert.throws(() => verifyHelper(invalid, { os: "darwin", arch: "arm64", sha256: sha256(invalid) }, "darwin", "arm64"), /universal helper/);
  }
});

for (const type of ["1", "2", "3", "6"]) {
  test(`archive rejects tar type ${type} before filesystem extraction`, async (t) => {
    const { archivePath, asset } = await fixture(t, [{ name: "et.exe", type, target: "outside" }]);
    await assert.rejects(verifyArchiveFiles(archivePath, asset), /link or special file/);
  });
}

test("archive rejects traversal, duplicate members and unlocked extras", async (t) => {
  for (const name of ["../outside", "/absolute", "C:/escape", "et.exe", "extra.dll"]) {
    const { archivePath, asset } = await fixture(t, [{ name: "et.exe", data: pe() }, { name, data: pe() }]);
    await assert.rejects(verifyArchiveFiles(archivePath, asset), /path|inventory/);
  }
});

test("installed helper needs sidecar, matching provenance and locked bytes", async (t) => {
  const { lock, asset, resourcesDir, binaryPath, pin } = await fixture(t);
  await verifyInstalled(lock, asset, resourcesDir);
  const forged = structuredClone(pin);
  forged.provenance.source.commit = "0".repeat(40);
  await writeFile(`${binaryPath}.manifest.json`, JSON.stringify(forged));
  await assert.rejects(verifyInstalled(lock, asset, resourcesDir), /sidecar\/provenance/);
  await writeFile(`${binaryPath}.manifest.json`, JSON.stringify(pin));
  await writeFile(binaryPath, "tampered");
  await assert.rejects(verifyInstalled(lock, asset, resourcesDir), /SHA256 mismatch/);
  await rm(`${binaryPath}.manifest.json`);
  await assert.rejects(verifyInstalled(lock, asset, resourcesDir), /ENOENT/);
});

test("self-computed sidecar digest cannot replace trusted binary digest", async (t) => {
  const { pin, lock, asset } = await fixture(t);
  assert.throws(() => verifySidecar({ ...pin, sha256: sha256(Buffer.from("unknown local binary")) }, lock, asset), /sidecar\/provenance/);
  assert.throws(() => verifySidecar({ ...pin, arch: "arm64" }, lock, asset), /sidecar\/provenance/);
});

test("universal runtime sidecars use exact GOARCH without changing executable hash", async () => {
  const lock = await loadLock();
  for (const asset of selectAssets(lock, "darwin", "arm64")) {
    const intel = runtimeManifest(lock, asset, "amd64"), arm = runtimeManifest(lock, asset, "arm64");
    assert.equal(intel.arch, "amd64"); assert.equal(arm.arch, "arm64");
    assert.equal(intel.sha256, arm.sha256);
    assert.doesNotThrow(() => verifySidecar(arm, lock, asset));
  }
});

test("upstream build provenance must agree with source, workflow and archive pins", async () => {
  const lock = await loadLock();
  const asset = lock.assets.find((entry) => entry.kind === "et");
  const release = structuredClone(lock.releases.et);
  const proof = {
    release: { repository: release.repository, tag: release.tag },
    upstream: { repository: release.source.repository, ref: release.source.tag, commit: release.source.commit },
    netcatty: { checkoutCommit: release.build.commit, workflowRun: release.build.run },
    artifacts: [{ name: asset.archive, sha256: asset.sha256 }],
  };
  const bytes = Buffer.from(JSON.stringify(proof));
  release.buildProvenance.sha256 = sha256(bytes);
  verifyBuildProvenance(bytes, release, asset);
  assert.throws(() => verifyBuildProvenance(Buffer.from("altered"), release, asset), /SHA256/);
  proof.upstream.commit = "0".repeat(40);
  const bad = Buffer.from(JSON.stringify(proof));
  release.buildProvenance.sha256 = sha256(bad);
  assert.throws(() => verifyBuildProvenance(bad, release, asset), /provenance does not match/);
});

test("fetch checks pinned bytes before publishing cache and refuses corrupt cache", async (t) => {
  const dir = await scratch(t), data = Buffer.from("asset"), digest = sha256(data);
  t.mock.method(globalThis, "fetch", async () => new Response(data));
  const result = await downloadPinned("https://example.test/asset", digest, dir);
  assert.deepEqual(result.data, data);
  await writeFile(result.file, "corrupt");
  await assert.rejects(downloadPinned("https://example.test/asset", digest, dir), /SHA256 mismatch/);
  await assert.rejects(downloadPinned("https://example.test/asset", "0".repeat(64), dir), /SHA256 mismatch/);
  await assert.rejects(lstat(path.join(dir, `${"0".repeat(64)}.asset`)), /ENOENT/);
  await assert.rejects(downloadPinned("http://example.test/asset", digest, dir), /HTTPS/);
});

test("gh transport preserves exact release tag, asset name and source commit", () => {
  assert.deepEqual(githubDownloadArgs("https://github.com/binaricat/MoshCatty/releases/download/moshcatty-0.1.8/SHA256SUMS"),
    ["release", "download", "moshcatty-0.1.8", "--repo", "binaricat/MoshCatty", "--pattern", "SHA256SUMS", "--output", "-"]);
  assert.throws(() => githubDownloadArgs("https://github.com/binaricat/MoshCatty/releases/latest"), /pinned GitHub URL/);
  assert.throws(() => githubDownloadArgs("https://raw.githubusercontent.com/owner/repo/main/LICENSE"), /pinned GitHub URL/);
});

test("download refuses HTTPS downgrade redirects and malformed digests", async (t) => {
  const dir = await scratch(t);
  t.mock.method(globalThis, "fetch", async () => new Response(null, { status: 302, headers: { location: "http://example.test/asset" } }));
  await assert.rejects(downloadPinned("https://example.test/asset", "0".repeat(64), dir), /redirects require HTTPS/);
  await assert.rejects(downloadPinned("https://example.test/asset", "../untrusted", dir), /trusted sha256 pin/);
});

test("installation preserves unknown files and does not rewrite matching binaries", async (t) => {
  const file = path.join(await scratch(t), "existing.exe");
  await writeFile(file, "original");
  const before = await lstat(file);
  await assert.rejects(preserveFile(file, Buffer.from("different")), /preserved/);
  await preserveFile(file, Buffer.from("original"));
  assert.equal((await lstat(file)).mtimeMs, before.mtimeMs);
  assert.equal(await readFile(file, "utf8"), "original");
});

test("installation and packaging reject directory links and hardlinked files", async (t) => {
  const dir = await scratch(t), original = path.join(dir, "original"), alias = path.join(dir, "alias");
  await mkdir(original);
  await symlink(original, alias, process.platform === "win32" ? "junction" : "dir");
  await assert.rejects(preserveFile(path.join(alias, "escape"), Buffer.from("data")), /unsafe helper path/);
  await assert.rejects(packageFiles(dir), /unsafe package entry/);
  const file = path.join(original, "binary");
  await writeFile(file, "data");
  await link(file, path.join(original, "hardlink"));
  await assert.rejects(readSafeFile(file), /unsafe helper path/);
});

test("recursive package checksums include licenses with stable relative paths", async (t) => {
  const dir = await scratch(t);
  await preserveFile(path.join(dir, "et.exe"), pe());
  await preserveFile(path.join(dir, "licenses/et/Apache.txt"), Buffer.from("license"));
  const entries = await checksumEntries(await packageFiles(dir));
  assert.equal(entries.length, 2);
  assert.ok(entries.some((entry) => entry.path.endsWith(path.join("licenses", "et", "Apache.txt"))));
});

// Opt-in real-supply checks run after fetching --all. Offline unit runs never
// fabricate provenance for the actual assets or silently hit the network.
const local = process.env.WAILS_HELPER_TEST_LOCAL === "1";
const hostOnly = process.env.WAILS_HELPER_TEST_HOST === "1";
const hostOS = ({ win32: "windows", darwin: "darwin", linux: "linux" })[process.platform];
const hostArch = ({ x64: "amd64", arm64: "arm64" })[process.arch];
test("real native packaging survives frontend output cleanup", { skip: !(local || hostOnly) }, async (t) => {
  const dir = await scratch(t);
  const outDir = path.join(dir, "dist", "wails");
  await packageWails(["--out-dir", outDir], async (command) => {
    if (command === "npm") await rm(path.join(dir, "dist"), { recursive: true, force: true });
    else if (command.startsWith("go build ")) {
      const artifact = command.match(/ -o "([^"]+)"/)[1];
      await writeFile(artifact, "fixture application binary");
    } else assert.equal(command, "node");
  });
  const manifest = JSON.parse(await readFile(path.join(outDir, "artifact-manifest.json")));
  assert.equal(manifest.artifacts.length, hostOS === "windows" ? 28 : 29);
  assert.equal(manifest.tools.length, 2);
  for (const tool of manifest.tools) assert.ok(manifest.artifacts.some(artifact => artifact.name === tool));
  for (const artifact of manifest.artifacts) {
    checkDigest(await readFile(path.join(outDir, artifact.name)), artifact.sha256, artifact.name);
  }
  const installer = JSON.parse(await readFile(path.join(outDir, "installer-resources.json")));
  assert.equal(installer.helpers.length, 2);
});

test("real locked supply verifies all eight downloaded binaries", { skip: !local }, async () => {
  const lock = await loadLock();
  assert.equal(lock.assets.length, 8);
  for (const asset of lock.assets) await verifyInstalled(lock, asset);
});

for (const [os, arch] of [["windows", "amd64"], ["darwin", "amd64"], ["darwin", "arm64"], ["linux", "amd64"], ["linux", "arm64"]]) {
  test(`real ${os}/${arch} packaging includes helpers, native manifests, licenses and lock`, { skip: !local && !(hostOnly && os === hostOS && arch === hostArch) }, async (t) => {
    const dir = await scratch(t);
    const helpers = await packageHelpers(dir, os, arch);
    assert.equal(helpers.length, 2);
    for (const helper of helpers) {
      const pin = JSON.parse(await readFile(path.join(dir, `${helper.path}.manifest.json`)));
      assert.equal(pin.arch, arch); assert.equal(pin.os, os);
      verifyHelper(await readFile(path.join(dir, helper.path)), pin, os, arch);
      const asset = (await loadLock()).assets.find((entry) => entry.kind === helper.kind && entry.os === os && entry.arches.includes(arch));
      assert.deepEqual(await readFile(path.join(dir, helper.path)), await readFile(path.join(DEFAULT_RESOURCES, helper.kind, asset.directory, helper.path)));
      for (const license of helper.licenses) {
        checkDigest(await readFile(path.join(dir, "licenses", helper.kind, license.path)), license.sha256, license.path);
      }
      assert.ok(helper.packagedFiles.some((file) => file.path.endsWith(".manifest.json")));
    }
    assert.equal(helpers.find((helper) => helper.kind === "et").licenses.length, 18);
    const entries = await checksumEntries(await packageFiles(dir));
    assert.equal(entries.length, 24);
    assert.equal(JSON.parse(await readFile(path.join(dir, "helper-supply.lock.json"))).schemaVersion, 1);
  });
}
