#!/usr/bin/env node
// Wails release supply uses the reviewed lock, never a mutable latest release
// or a checksum calculated from an unproven local executable.
import { createHash } from "node:crypto";
import { execFile, execFileSync } from "node:child_process";
import { lstat, mkdir, readFile, writeFile, chmod } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { isDeepStrictEqual, promisify } from "node:util";
import tarChecks from "./archive-checks.cjs";
import tarMoshRelease from "./resolve-mosh-bin-release.cjs";

const ROOT = fileURLToPath(new URL("../", import.meta.url));
export const LOCK_PATH = fileURLToPath(new URL("./fetch-wails-helpers.lock.json", import.meta.url));
// Vite empties dist during package-wails' frontend build.
export const DEFAULT_CACHE = path.join(ROOT, "build", "wails-helper-cache");
export const DEFAULT_RESOURCES = path.join(ROOT, "resources");
export const sha256 = (data) => createHash("sha256").update(data).digest("hex");
const json = (value) => Buffer.from(`${JSON.stringify(value, null, 2)}\n`);
const hostArch = () => ({ x64: "amd64", arm64: "arm64" })[process.arch];
const execFileAsync = promisify(execFile);

function safeRelative(name) {
  if (!name || name.split("/").some((part) => !/^[A-Za-z0-9_-][A-Za-z0-9._-]*$/.test(part))) {
    throw new Error(`unsafe helper path: ${name}`);
  }
}

export function verifyHelper(data, pin, goos, goarch) {
  if (!["windows", "darwin", "linux"].includes(goos) || !["amd64", "arm64"].includes(goarch)) throw new Error("unsupported helper target");
  if (pin.os !== goos || pin.arch !== goarch) throw new Error("helper target does not match pin");
  if (sha256(data) !== pin.sha256) throw new Error("helper hash mismatch");
  const cpu = goarch === "amd64" ? 0x01000007 : 0x0100000c;
  let matches = false;
  if (goos === "windows" && data.length >= 64 && data.toString("ascii", 0, 2) === "MZ") {
    const offset = data.readUInt32LE(60);
    matches = offset + 6 <= data.length && data.toString("ascii", offset, offset + 4) === "PE\0\0" && data.readUInt16LE(offset + 4) === (goarch === "amd64" ? 0x8664 : 0xaa64);
  } else if (goos === "darwin" && data.length >= 8) {
    if (data.readUInt32LE(0) === 0xfeedfacf) matches = data.readUInt32LE(4) === cpu;
    else if (data.readUInt32BE(0) === 0xcafebabe) {
      const count = data.readUInt32BE(4);
      if (count > 32 || 8 + count * 20 > data.length) throw new Error("invalid universal helper architecture table");
      const slices = [];
      const cpus = new Set();
      for (let i = 0; i < count; i++) {
        const entry = 8 + i * 20;
        const sliceCpu = data.readUInt32BE(entry);
        const offset = data.readUInt32BE(entry + 8);
        const size = data.readUInt32BE(entry + 12);
        if (offset < 8 + count * 20 || size < 32 || offset + size > data.length ||
            data.readUInt32LE(offset) !== 0xfeedfacf || data.readUInt32LE(offset + 4) !== sliceCpu ||
            cpus.has(sliceCpu) || slices.some((slice) => offset < slice.end && offset + size > slice.start)) {
          throw new Error("invalid universal helper slice");
        }
        cpus.add(sliceCpu);
        slices.push({ start: offset, end: offset + size });
        if (sliceCpu === cpu) matches = true;
      }
    }
  } else if (goos === "linux" && data.length >= 20 && data.toString("hex", 0, 4) === "7f454c46") {
    matches = data[4] === 2 && data[5] === 1 && data.readUInt16LE(18) === (goarch === "amd64" ? 62 : 183);
  }
  if (!matches) throw new Error("helper executable architecture mismatch");
}

export function validateLock(lock) {
  if (lock.schemaVersion !== 1 || !lock.assets?.length) throw new Error("invalid helper lock schema");
  const digest = (value) => {
    if (!/^[a-f0-9]{64}$/.test(value ?? "")) throw new Error("trusted sha256 pin required");
  };
  const identities = new Set();
  for (const asset of lock.assets) {
    const release = lock.releases[asset.kind];
    if (!release || !["mosh", "et"].includes(asset.kind)) throw new Error("invalid helper release");
    if (!/^[\w-]+\/[\w.-]+$/.test(release.repository) || !/^[\w.-]+$/.test(release.tag)) throw new Error("invalid release provenance");
    for (const origin of [release.source, release.build]) {
      if (!/^[\w-]+\/[\w.-]+$/.test(origin?.repository) || !/^[a-f0-9]{40}$/.test(origin?.commit)) throw new Error("missing source/build provenance");
    }
    if (!release.source.tag || !release.build.workflow || !release.build.run.startsWith(`https://github.com/${release.build.repository}/actions/runs/`)) throw new Error("missing build provenance");
    if (asset.kind === "mosh") {
      const { validateReleaseTag } = tarMoshRelease;
      validateReleaseTag(release.tag);
    }
    if (!["windows", "darwin", "linux"].includes(asset.os) || !asset.arches?.length || asset.arches.some((arch) => !["amd64", "arm64"].includes(arch))) throw new Error("invalid helper target");
    if (asset.os === "darwin" && !isDeepStrictEqual(asset.arches, ["amd64", "arm64"])) throw new Error("darwin helper must be universal");
    safeRelative(asset.directory);
    safeRelative(asset.archive);
    const identity = `${asset.kind}/${asset.directory}`;
    if (identities.has(identity)) throw new Error("duplicate helper target");
    identities.add(identity);
    if (!Number.isSafeInteger(asset.assetId) || asset.assetId <= 0) throw new Error("missing release asset provenance");
    digest(asset.sha256);
    digest(release.checksums?.sha256);
    if (release.buildProvenance) digest(release.buildProvenance.sha256);
    const names = new Set();
    for (const file of [...asset.files, ...release.licenses]) {
      safeRelative(file.path);
      if (names.has(file.path.toLowerCase())) throw new Error("duplicate helper file");
      names.add(file.path.toLowerCase());
      digest(file.sha256);
      if (file.url && !/^https:\/\/raw\.githubusercontent\.com\/[\w.-]+\/[\w.-]+\/[a-f0-9]{40}\//.test(file.url)) throw new Error("license source must pin a commit");
    }
    if (asset.files.filter((file) => file.executable).length !== 1) throw new Error("helper must have one executable");
  }
  return lock;
}

export async function loadLock() {
  return validateLock(JSON.parse(await readFile(LOCK_PATH, "utf8")));
}

export function selectAssets(lock, goos, goarch) {
  const assets = lock.assets.filter((asset) => asset.os === goos && asset.arches.includes(goarch));
  if (assets.length !== 2 || new Set(assets.map((asset) => asset.kind)).size !== 2) {
    throw new Error(`no locked Mosh/ET supply for ${goos}/${goarch}`);
  }
  return assets;
}

// Check every ancestor as well as the leaf: a directory junction can redirect
// otherwise safe filenames. Never overwrite unknown files or follow links.
export async function assertSafePath(file) {
  const absolute = path.resolve(file);
  const { root } = path.parse(absolute);
  let current = root;
  for (const part of absolute.slice(root.length).split(path.sep)) {
    current = path.join(current, part);
    let stat;
    try { stat = await lstat(current); } catch (error) {
      if (error.code === "ENOENT") continue;
      throw error;
    }
    if (stat.isSymbolicLink() || (!stat.isDirectory() && !stat.isFile()) || (stat.isFile() && stat.nlink !== 1)) {
      throw new Error(`unsafe helper path (link or special file): ${current}`);
    }
  }
}

export async function readSafeFile(file) {
  await assertSafePath(file);
  return readFile(file);
}

export async function preserveFile(file, data, executable = false) {
  await assertSafePath(file);
  let existing;
  try { existing = await readFile(file); } catch (error) {
    if (error.code !== "ENOENT") throw error;
  }
  if (existing) {
    if (!existing.equals(data)) throw new Error(`existing helper file differs; preserved: ${file}`);
    return;
  }
  await mkdir(path.dirname(file), { recursive: true });
  await writeFile(file, data, { flag: "wx", mode: executable ? 0o755 : 0o644 });
  if (executable && process.platform !== "win32") await chmod(file, 0o755);
}

export function checkDigest(data, expected, label) {
  if (!/^[a-f0-9]{64}$/.test(expected ?? "") || sha256(data) !== expected) throw new Error(`${label} SHA256 mismatch`);
  return data;
}

export function githubDownloadArgs(url) {
  const release = url.match(/^https:\/\/github\.com\/([\w.-]+\/[\w.-]+)\/releases\/download\/([\w.-]+)\/([\w.-]+)$/);
  if (release) return ["release", "download", release[2], "--repo", release[1], "--pattern", release[3], "--output", "-"];
  const raw = url.match(/^https:\/\/raw\.githubusercontent\.com\/([\w.-]+\/[\w.-]+)\/([a-f0-9]{40})\/(.+)$/);
  if (raw) return ["api", `repos/${raw[1]}/contents/${raw[3]}?ref=${raw[2]}`, "-H", "Accept: application/vnd.github.raw+json"];
  throw new Error("gh transport requires a pinned GitHub URL");
}

export async function downloadPinned(url, digest, cacheDir = DEFAULT_CACHE) {
  if (!url.startsWith("https://")) throw new Error("helper downloads require HTTPS");
  if (!/^[a-f0-9]{64}$/.test(digest ?? "")) throw new Error("trusted sha256 pin required");
  const cached = path.join(cacheDir, `${digest}.asset`);
  try { return { data: checkDigest(await readSafeFile(cached), digest, url), file: cached }; } catch (error) {
    if (error.code !== "ENOENT") throw error;
  }
  const transport = process.env.WAILS_HELPER_DOWNLOAD ?? "fetch";
  if (transport === "gh") {
    const { stdout } = await execFileAsync("gh", githubDownloadArgs(url), { encoding: "buffer", maxBuffer: 128 * 1024 * 1024, timeout: 60000 });
    const data = checkDigest(stdout, digest, url);
    await preserveFile(cached, data);
    return { data, file: cached };
  }
  if (transport !== "fetch") throw new Error(`unknown WAILS_HELPER_DOWNLOAD: ${transport}`);
  let response;
  // Retry only transport failures / transient upstream responses. Digest and
  // provenance failures are terminal and never fall back to another source.
  for (let attempt = 0; attempt < 3; attempt++) {
    try {
      let location = url;
      const signal = AbortSignal.timeout(60000);
      for (let redirects = 0; ; redirects++) {
        if (!location.startsWith("https://")) throw new Error("helper redirects require HTTPS");
        if (redirects > 5) throw new Error("too many helper redirects");
        response = await fetch(location, { signal, redirect: "manual" });
        if (response.status < 300 || response.status >= 400) break;
        const next = response.headers.get("location");
        await response.body?.cancel();
        if (!next) throw new Error("helper redirect missing location");
        location = new URL(next, location).href;
      }
      if (!response.ok) throw new Error(`HTTP ${response.status}: ${url}`);
      const data = Buffer.from(await response.arrayBuffer());
      checkDigest(data, digest, url);
      await preserveFile(cached, data);
      return { data, file: cached };
    } catch (error) {
      if (attempt === 2 || (response && response.status < 500 && response.status !== 429)) throw error;
    }
  }
}

export function releaseUrl(release, name) {
  return `https://github.com/${release.repository}/releases/download/${release.tag}/${name}`;
}

export function runtimeManifest(lock, asset, arch) {
  if (!asset.arches.includes(arch)) throw new Error("helper target does not match lock");
  const release = lock.releases[asset.kind];
  const binary = asset.files.find((file) => file.executable);
  return {
    name: asset.kind, path: binary.path, os: asset.os, arch, sha256: binary.sha256,
    provenance: {
      repository: release.repository, tag: release.tag, source: release.source, build: release.build,
      archive: { url: releaseUrl(release, asset.archive), assetId: asset.assetId, sha256: asset.sha256 },
      checksums: release.checksums, ...(release.buildProvenance ? { buildProvenance: release.buildProvenance } : {}),
    },
    files: asset.files, licenses: release.licenses,
  };
}

export function verifySidecar(pin, lock, asset) {
  if (!asset.arches.includes(pin.arch) || !isDeepStrictEqual(pin, runtimeManifest(lock, asset, pin.arch))) {
    throw new Error("helper sidecar/provenance does not match trusted lock");
  }
}

export async function verifyArchiveFiles(archivePath, asset) {
  checkDigest(await readSafeFile(archivePath), asset.sha256, asset.archive);
  const { cwd, archive } = tarChecks.resolveTarArchiveInvocation(archivePath);
  const options = { cwd, maxBuffer: 128 * 1024 * 1024, timeout: 60000 };
  const names = execFileSync("tar", ["-tzf", archive], { ...options, encoding: "utf8" }).trim().split(/\r?\n/);
  tarChecks.validateTarEntries(names);
  const listing = execFileSync("tar", ["-tvzf", archive], { ...options, encoding: "utf8" }).trim().split(/\r?\n/);
  if (listing.some((line) => !/^[d-]/.test(line))) throw new Error("helper archive contains a link or special file");
  const regular = names.filter((name) => !name.endsWith("/"));
  if (!isDeepStrictEqual([...regular].sort(), asset.files.map((file) => file.path).sort())) throw new Error("helper archive file inventory does not match lock");
  return asset.files.map((file) => {
    // Stream each validated regular member to memory, never extract links or
    // paths from the tarball into the filesystem.
    const data = execFileSync("tar", ["-xOzf", archive, "--", file.path], options);
    checkDigest(data, file.sha256, file.path);
    if (file.executable || file.path.toLowerCase().endsWith(".dll")) {
      for (const arch of asset.arches) verifyHelper(data, { os: asset.os, arch, sha256: file.sha256 }, asset.os, arch);
    }
    return { ...file, data };
  });
}

export async function verifyInstalled(lock, asset, resourcesDir = DEFAULT_RESOURCES) {
  const directory = path.join(resourcesDir, asset.kind, asset.directory);
  const binary = asset.files.find((file) => file.executable);
  const pin = JSON.parse(await readSafeFile(path.join(directory, `${binary.path}.manifest.json`)));
  verifySidecar(pin, lock, asset);
  for (const file of asset.files) {
    const data = checkDigest(await readSafeFile(path.join(directory, file.path)), file.sha256, file.path);
    if (file.executable || file.path.toLowerCase().endsWith(".dll")) {
      for (const arch of asset.arches) verifyHelper(data, { os: asset.os, arch, sha256: file.sha256 }, asset.os, arch);
    }
  }
  return pin;
}

export function verifyBuildProvenance(data, release, asset) {
  checkDigest(data, release.buildProvenance.sha256, "build provenance");
  const provenance = JSON.parse(data);
  if (provenance.release.repository !== release.repository || provenance.release.tag !== release.tag ||
      provenance.upstream.repository !== release.source.repository || provenance.upstream.ref !== release.source.tag ||
      provenance.upstream.commit !== release.source.commit || provenance.netcatty.checkoutCommit !== release.build.commit ||
      provenance.netcatty.workflowRun !== release.build.run ||
      !provenance.artifacts.some((entry) => entry.name === asset.archive && entry.sha256 === asset.sha256)) {
    throw new Error("upstream build provenance does not match lock");
  }
}

export async function supplyHelpers({ goos, goarch, all = false, verifyOnly = false, resourcesDir = DEFAULT_RESOURCES, cacheDir = DEFAULT_CACHE } = {}) {
  const lock = await loadLock();
  const assets = all ? lock.assets : selectAssets(lock, goos, goarch);
  for (const asset of assets) {
    if (!verifyOnly) {
      const release = lock.releases[asset.kind];
      const sums = await downloadPinned(releaseUrl(release, "SHA256SUMS"), release.checksums.sha256, cacheDir);
      if (tarChecks.parseSums(sums.data.toString()).get(asset.archive) !== asset.sha256) throw new Error("upstream SHA256SUMS does not match lock");
      if (release.buildProvenance) {
        const proof = await downloadPinned(releaseUrl(release, "BUILD-PROVENANCE.json"), release.buildProvenance.sha256, cacheDir);
        verifyBuildProvenance(proof.data, release, asset);
      }
      const archive = await downloadPinned(releaseUrl(release, asset.archive), asset.sha256, cacheDir);
      const files = await verifyArchiveFiles(archive.file, asset);
      for (const license of release.licenses) await downloadPinned(license.url, license.sha256, cacheDir);
      const directory = path.join(resourcesDir, asset.kind, asset.directory);
      const arch = asset.arches.includes(goarch) ? goarch : asset.arches.includes(hostArch()) ? hostArch() : asset.arches[0];
      const pin = runtimeManifest(lock, asset, arch);
      const sidecar = path.join(directory, `${pin.path}.manifest.json`);
      let existingPin;
      try { existingPin = JSON.parse(await readSafeFile(sidecar)); } catch (error) { if (error.code !== "ENOENT") throw error; }
      if (existingPin) verifySidecar(existingPin, lock, asset);
      for (const file of files) await preserveFile(path.join(directory, file.path), file.data, file.executable);
      if (!existingPin) await preserveFile(sidecar, json(pin));
    }
    const pin = await verifyInstalled(lock, asset, resourcesDir);
    console.log(`[wails-helpers] verified ${asset.kind}/${asset.directory} ${pin.sha256} (${asset.arches.join("+")})`);
  }
  return { lock, assets };
}

export async function packageHelpers(outDir, goos, goarch, { resourcesDir = DEFAULT_RESOURCES, cacheDir = DEFAULT_CACHE } = {}) {
  const lock = await loadLock();
  const helpers = [];
  for (const asset of selectAssets(lock, goos, goarch)) {
    await verifyInstalled(lock, asset, resourcesDir);
    const directory = path.join(resourcesDir, asset.kind, asset.directory);
    const pin = runtimeManifest(lock, asset, goarch);
    const files = [];
    for (const file of asset.files) {
      const data = checkDigest(await readSafeFile(path.join(directory, file.path)), file.sha256, file.path);
      // DLLs must sit beside et.exe for the Windows loader.
      const output = file.path.toLowerCase().endsWith(".dll") ? path.posix.basename(file.path) : file.path;
      await preserveFile(path.join(outDir, output), data, file.executable);
      files.push({ path: output, sha256: file.sha256, destination: goos === "darwin" ? `Contents/MacOS/${output}` : output });
    }
    for (const license of lock.releases[asset.kind].licenses) {
      const { data } = await downloadPinned(license.url, license.sha256, cacheDir);
      const output = `licenses/${asset.kind}/${license.path}`;
      await preserveFile(path.join(outDir, output), data);
      files.push({ path: output, sha256: license.sha256, destination: goos === "darwin" ? `Contents/Resources/${output}` : output });
    }
    await preserveFile(path.join(outDir, `${pin.path}.manifest.json`), json(pin));
    files.push({ path: `${pin.path}.manifest.json`, destination: goos === "darwin" ? `Contents/MacOS/${pin.path}.manifest.json` : `${pin.path}.manifest.json` });
    helpers.push({ kind: asset.kind, ...pin, destination: goos === "darwin" ? `Contents/MacOS/${pin.path}` : pin.path, packagedFiles: files });
  }
  await preserveFile(path.join(outDir, "helper-supply.lock.json"), json(lock));
  return helpers;
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  const options = { goos: ({ win32: "windows", darwin: "darwin", linux: "linux" })[process.platform], goarch: hostArch() };
  for (let i = 2; i < process.argv.length; i++) {
    const arg = process.argv[i];
    if (arg === "--all") options.all = true;
    else if (arg === "--verify-only") options.verifyOnly = true;
    else if (["--goos", "--goarch", "--resources-dir", "--cache-dir"].includes(arg)) {
      const key = { "--goos": "goos", "--goarch": "goarch", "--resources-dir": "resourcesDir", "--cache-dir": "cacheDir" }[arg];
      options[key] = process.argv[++i];
      if (!options[key] || options[key].startsWith("--")) throw new Error(`missing value for ${arg}`);
    } else throw new Error(`unknown argument ${arg}`);
  }
  await supplyHelpers(options).catch((error) => {
    console.error(`[wails-helpers] ${error.message}`);
    process.exitCode = 1;
  });
}
