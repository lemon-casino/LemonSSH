#!/usr/bin/env node
// Installer packaging for the unsigned Wails artifacts (P6 follow-up).
// Consumes the dist/wails output of scripts/package-wails.mjs and produces
// installer formats from the packaged binary. Project policy: NO signing and
// NO fake success. A format whose packaging tool is missing is an honest skip
// recorded in installers.json as { format, ok: false, reason: "..." } and is
// never reported as built.
import { existsSync } from "node:fs";
import { chmod, copyFile, mkdir, readdir, rm, writeFile } from "node:fs/promises";
import { spawnSync } from "node:child_process";
import path from "node:path";
import process from "node:process";
import { pathToFileURL } from "node:url";

const APP_NAME = "LemonSSH";
const MAINTAINER = "Netcatty Maintainers";
const DESCRIPTION =
  "Netcatty is a modern SSH manager and terminal app with host grouping, SFTP, keychain, port forwarding, and a rich UI.";
const LICENSE = "GPL-3.0-or-later";
const LINUX_BINARY_DIR = "usr/local/bin";

// Matches the artifactBasename layout of scripts/package-wails.mjs.
const BINARY_PATTERN = /^LemonSSH-(.+)-(windows|linux|darwin)-(amd64|arm64)(\.exe)?$/;
const HELPER_NAMES = new Set(["mosh-client", "mosh-client.exe", "et", "et.exe"]);
const CHECKSUM_FILE = "checksums.txt";
const ICON_CANDIDATES = ["build/appicon.png", "build/icons/256x256.png", "build/icons/512x512.png"];

// Probe commands only check availability; they must exit 0 when installed.
const TOOL_PROBES = {
  nsis: "makensis -VERSION",
  deb: "dpkg-deb --version",
  rpm: "rpmbuild --version",
  appimage: "appimagetool --version",
  zip: "zip -v",
  powershell: 'powershell -NoProfile -Command "$PSVersionTable.PSVersion.Major"',
};

export function archMapping(goarch) {
  switch (goarch) {
    case "amd64":
      return { nsis: "x64", deb: "amd64", rpm: "x86_64", appimage: "x86_64" };
    case "arm64":
      return { nsis: "arm64", deb: "arm64", rpm: "aarch64", appimage: "aarch64" };
    default:
      throw new Error(`unsupported goarch ${goarch}`);
  }
}

export function nsisScript({ name, version, exeFile, outFile }) {
  if (!name || !version || !exeFile || !outFile) {
    throw new Error("name, version, exeFile and outFile are required");
  }
  const exeName = path.basename(exeFile);
  return [
    `; ${name} ${version} Windows installer (unsigned, no Authenticode)`,
    `Name "${name} ${version}"`,
    `OutFile "${outFile}"`,
    `InstallDir "$PROGRAMFILES64\\${name}"`,
    "SetCompressor lzma",
    "",
    'Section "Install"',
    '  SetOutPath "$INSTDIR"',
    `  File "${exeFile}"`,
    `  CreateDirectory "$SMPROGRAMS\\${name}"`,
    `  CreateShortCut "$SMPROGRAMS\\${name}\\${name}.lnk" "$INSTDIR\\${exeName}"`,
    `  CreateShortCut "$DESKTOP\\${name}.lnk" "$INSTDIR\\${exeName}"`,
    `  WriteUninstaller "$INSTDIR\\Uninstall ${name}.exe"`,
    "SectionEnd",
    "",
    'Section "Uninstall"',
    `  Delete "$INSTDIR\\${exeName}"`,
    `  Delete "$INSTDIR\\Uninstall ${name}.exe"`,
    `  Delete "$SMPROGRAMS\\${name}\\${name}.lnk"`,
    `  Delete "$DESKTOP\\${name}.lnk"`,
    `  RMDir "$SMPROGRAMS\\${name}"`,
    '  RMDir "$INSTDIR"',
    "SectionEnd",
    "",
  ].join("\n");
}

export function debControl({ name, version, arch, maintainer, description }) {
  if (!name || !version || !maintainer || !description) {
    throw new Error("name, version, maintainer and description are required");
  }
  // Debian policy: package names are lowercase; amd64/arm64 map through as-is.
  const debArch = archMapping(arch).deb;
  return [
    `Package: ${name.toLowerCase()}`,
    `Version: ${version}`,
    "Section: net",
    "Priority: optional",
    `Architecture: ${debArch}`,
    "Depends:",
    `Maintainer: ${maintainer}`,
    `Description: ${description}`,
    "",
  ].join("\n");
}

export function rpmSpec({ name, version, arch, exeFile }) {
  if (!name || !version || !exeFile) {
    throw new Error("name, version and exeFile are required");
  }
  const rpmArch = archMapping(arch).rpm;
  const rpmName = name.toLowerCase();
  return [
    `Name:           ${rpmName}`,
    `Version:        ${version}`,
    "Release:        1%{?dist}",
    `Summary:        ${DESCRIPTION}`,
    `License:        ${LICENSE}`,
    `BuildArch:      ${rpmArch}`,
    "",
    "%description",
    DESCRIPTION,
    "",
    "%install",
    "mkdir -p %{buildroot}%{_bindir}",
    `install -m 0755 "${exeFile}" %{buildroot}%{_bindir}/${rpmName}`,
    "",
    "%files",
    `%attr(0755,root,root) %{_bindir}/${rpmName}`,
    "",
  ].join("\n");
}

export function appImageDesktop({ name, exec, icon }) {
  if (!name || !exec || !icon) {
    throw new Error("name, exec and icon are required");
  }
  return [
    "[Desktop Entry]",
    "Type=Application",
    `Name=${name}`,
    `Exec=${exec}`,
    `Icon=${icon}`,
    "Terminal=false",
    "Categories=Network;",
    "",
  ].join("\n");
}

export function parseInstallerArgs(argv) {
  const args = { inDir: path.join("dist", "wails"), outDir: path.join("dist", "wails") };
  for (let index = 0; index < argv.length; index++) {
    const arg = argv[index];
    if (arg === "--in") args.inDir = argv[++index];
    else if (arg === "--out") args.outDir = argv[++index];
    else throw new Error(`unknown argument ${arg}`);
  }
  return args;
}

function toolAvailable(command) {
  const result = spawnSync(command, { shell: true, encoding: "utf8" });
  return !result.error && result.status === 0;
}

function tail(text) {
  if (!text) return "no output";
  const flat = String(text).trim().replace(/\s+/g, " ");
  return flat.length > 240 ? `...${flat.slice(-240)}` : flat;
}

async function discoverPlatforms(inDir) {
  const entries = await readdir(inDir, { withFileTypes: true });
  const platforms = new Map();
  const helpers = [];
  let checksums = null;
  for (const entry of entries) {
    if (!entry.isFile()) continue;
    if (entry.name === CHECKSUM_FILE) {
      checksums = path.join(inDir, entry.name);
      continue;
    }
    const match = BINARY_PATTERN.exec(entry.name);
    if (match) {
      const [, version, goos, goarch] = match;
      platforms.set(`${goos}-${goarch}`, {
        goos,
        goarch,
        version,
        binary: path.join(inDir, entry.name),
      });
      continue;
    }
    if (HELPER_NAMES.has(entry.name)) helpers.push(path.join(inDir, entry.name));
  }
  return { platforms: [...platforms.values()], helpers, checksums };
}

// Helpers follow the fetch-mosh/fetch-et layout: ".exe" on Windows, bare names elsewhere.
function helpersFor(helpers, goos) {
  const matches = goos === "windows"
    ? (name) => name.endsWith(".exe")
    : (name) => !name.endsWith(".exe");
  return helpers.map((helper) => path.basename(helper)).filter(matches);
}

function artifactBase(platform) {
  const extension = platform.goos === "windows" ? ".exe" : "";
  return `LemonSSH-${platform.version}-${platform.goos}-${platform.goarch}${extension}`;
}

// Always attempted: a portable zip of the already packaged files (binary,
// optional mosh/et helpers, checksums.txt). Reuses existing files only.
async function attemptZip({ platform, helpers, checksums, inDir, outDir }) {
  const files = [artifactBase(platform), ...helpersFor(helpers, platform.goos)];
  if (checksums) files.push(path.basename(checksums));
  const outZip = path.join(outDir, `${artifactBase(platform)}.zip`);
  let command;
  if (process.platform === "win32") {
    if (!toolAvailable(TOOL_PROBES.powershell)) {
      return { format: "zip", ok: false, reason: "powershell (Compress-Archive) not available" };
    }
    const list = files.map((name) => `'${name}'`).join(",");
    command = `powershell -NoProfile -Command "Compress-Archive -Path ${list} -DestinationPath '${outZip}' -Force"`;
  } else {
    if (!toolAvailable(TOOL_PROBES.zip)) {
      return { format: "zip", ok: false, reason: "zip not installed" };
    }
    const list = files.map((name) => `'${name}'`).join(" ");
    command = `zip -q -j '${outZip}' ${list}`;
  }
  const result = spawnSync(command, { shell: true, cwd: inDir, encoding: "utf8" });
  if (result.status !== 0 || !existsSync(outZip)) {
    return {
      format: "zip",
      ok: false,
      reason: `zip packaging failed with status ${result.status}: ${tail(result.stderr)}`,
    };
  }
  return { format: "zip", ok: true, file: outZip };
}

async function attemptNsis({ platform, outDir, workDir }) {
  if (platform.goos !== "windows") {
    return { format: "nsis", ok: false, reason: `nsis not applicable for ${platform.goos} artifacts` };
  }
  if (!toolAvailable(TOOL_PROBES.nsis)) {
    return { format: "nsis", ok: false, reason: "makensis not installed" };
  }
  const base = artifactBase(platform).replace(/\.exe$/, "");
  const outFile = path.join(outDir, `${base}-setup.exe`);
  const scriptPath = path.join(workDir, `${base}.nsi`);
  await writeFile(
    scriptPath,
    nsisScript({ name: APP_NAME, version: platform.version, exeFile: platform.binary, outFile }),
    "utf8",
  );
  const result = spawnSync(`makensis "${scriptPath}"`, { shell: true, encoding: "utf8" });
  if (result.status !== 0 || !existsSync(outFile)) {
    return {
      format: "nsis",
      ok: false,
      reason: `makensis failed with status ${result.status}: ${tail(result.stderr)}`,
    };
  }
  return { format: "nsis", ok: true, file: outFile };
}

async function attemptDeb({ platform, outDir, workDir }) {
  if (platform.goos !== "linux") {
    return { format: "deb", ok: false, reason: `deb not applicable for ${platform.goos} artifacts` };
  }
  if (!toolAvailable(TOOL_PROBES.deb)) {
    return { format: "deb", ok: false, reason: "dpkg-deb not installed" };
  }
  const debArch = archMapping(platform.goarch).deb;
  const staging = path.join(workDir, `deb-${platform.goarch}`);
  const controlDir = path.join(staging, "DEBIAN");
  const binaryDir = path.join(staging, ...LINUX_BINARY_DIR.split("/"));
  await mkdir(controlDir, { recursive: true });
  await writeFile(
    path.join(controlDir, "control"),
    debControl({
      name: APP_NAME,
      version: platform.version,
      arch: platform.goarch,
      maintainer: MAINTAINER,
      description: DESCRIPTION,
    }),
    "utf8",
  );
  const installedBinary = path.join(binaryDir, APP_NAME.toLowerCase());
  await copyFile(platform.binary, installedBinary);
  await chmod(installedBinary, 0o755).catch(() => {});
  const outFile = path.join(outDir, `LemonSSH-${platform.version}-linux-${platform.goarch}.deb`);
  const result = spawnSync(
    `dpkg-deb --build --root-owner-group "${staging}" "${outFile}"`,
    { shell: true, encoding: "utf8" },
  );
  if (result.status !== 0 || !existsSync(outFile)) {
    return {
      format: "deb",
      ok: false,
      reason: `dpkg-deb failed with status ${result.status}: ${tail(result.stderr)}`,
    };
  }
  return { format: "deb", ok: true, file: outFile };
}

async function attemptRpm({ platform, outDir, workDir }) {
  if (platform.goos !== "linux") {
    return { format: "rpm", ok: false, reason: `rpm not applicable for ${platform.goos} artifacts` };
  }
  if (!toolAvailable(TOOL_PROBES.rpm)) {
    return { format: "rpm", ok: false, reason: "rpmbuild not installed" };
  }
  const rpmArch = archMapping(platform.goarch).rpm;
  const topdir = path.join(workDir, `rpm-${platform.goarch}`);
  for (const sub of ["BUILD", "BUILDROOT", "RPMS", "SOURCES", "SPECS", "SRPMS"]) {
    await mkdir(path.join(topdir, sub), { recursive: true });
  }
  const specPath = path.join(topdir, "SPECS", "lemonssh.spec");
  await writeFile(
    specPath,
    rpmSpec({ name: APP_NAME, version: platform.version, arch: platform.goarch, exeFile: platform.binary }),
    "utf8",
  );
  const result = spawnSync(
    `rpmbuild -bb --define "_topdir ${topdir}" --target ${rpmArch} "${specPath}"`,
    { shell: true, encoding: "utf8" },
  );
  const rpmDir = path.join(topdir, "RPMS", rpmArch);
  const built = result.status === 0 && existsSync(rpmDir)
    ? (await readdir(rpmDir)).filter((name) => name.endsWith(".rpm"))
    : [];
  if (built.length === 0) {
    return {
      format: "rpm",
      ok: false,
      reason: `rpmbuild failed with status ${result.status}: ${tail(result.stderr)}`,
    };
  }
  const outFile = path.join(outDir, `LemonSSH-${platform.version}-linux-${platform.goarch}.rpm`);
  await copyFile(path.join(rpmDir, built[0]), outFile);
  return { format: "rpm", ok: true, file: outFile };
}

async function attemptAppImage({ platform, outDir, workDir, repoRoot }) {
  if (platform.goos !== "linux") {
    return { format: "appimage", ok: false, reason: `appimage not applicable for ${platform.goos} artifacts` };
  }
  if (!toolAvailable(TOOL_PROBES.appimage)) {
    return { format: "appimage", ok: false, reason: "appimagetool not installed" };
  }
  const iconSource = ICON_CANDIDATES.map((candidate) => path.join(repoRoot, candidate)).find((candidate) => existsSync(candidate));
  if (!iconSource) {
    return { format: "appimage", ok: false, reason: "no PNG icon available for AppImage staging" };
  }
  const appDir = path.join(workDir, `AppDir-${platform.goarch}`);
  const binDir = path.join(appDir, "usr", "bin");
  await mkdir(binDir, { recursive: true });
  await writeFile(
    path.join(appDir, `${APP_NAME}.desktop`),
    appImageDesktop({ name: APP_NAME, exec: APP_NAME, icon: APP_NAME }),
    "utf8",
  );
  const appImageBinary = path.join(binDir, APP_NAME);
  await copyFile(platform.binary, appImageBinary);
  await chmod(appImageBinary, 0o755).catch(() => {});
  await copyFile(iconSource, path.join(appDir, ".DirIcon"));
  await copyFile(iconSource, path.join(appDir, `${APP_NAME}.png`));
  await writeFile(
    path.join(appDir, "AppRun"),
    [
      "#!/bin/sh",
      'HERE="$(dirname "$(readlink -f "$0")")"',
      `exec "$HERE/usr/bin/${APP_NAME}" "$@"`,
      "",
    ].join("\n"),
    { encoding: "utf8", mode: 0o755 },
  );
  const outFile = path.join(outDir, `LemonSSH-${platform.version}-linux-${platform.goarch}.AppImage`);
  const result = spawnSync(`appimagetool "${appDir}" "${outFile}"`, { shell: true, encoding: "utf8" });
  if (result.status !== 0 || !existsSync(outFile)) {
    return {
      format: "appimage",
      ok: false,
      reason: `appimagetool failed with status ${result.status}: ${tail(result.stderr)}`,
    };
  }
  return { format: "appimage", ok: true, file: outFile };
}

function logAttempt(entry) {
  const where = entry.file ? ` -> ${path.basename(entry.file)}` : "";
  const why = entry.reason ? ` (${entry.reason})` : "";
  console.log(`[package-installer]   ${entry.format}: ${entry.ok ? "ok" : "skipped/failed"}${where}${why}`);
}

async function main() {
  const args = parseInstallerArgs(process.argv.slice(2));
  const inDir = path.resolve(args.inDir);
  const outDir = path.resolve(args.outDir);
  if (!existsSync(inDir)) {
    console.error(`[package-installer] input directory not found: ${inDir}`);
    process.exitCode = 1;
    return;
  }
  const { platforms, helpers, checksums } = await discoverPlatforms(inDir);
  if (platforms.length === 0) {
    console.error(
      `[package-installer] no LemonSSH-<version>-<goos>-<goarch> binary found in ${inDir}; run "npm run package:wails" first`,
    );
    process.exitCode = 1;
    return;
  }
  await mkdir(outDir, { recursive: true });
  const workDir = path.join(outDir, ".installer-work");
  await mkdir(workDir, { recursive: true });
  const repoRoot = process.cwd();
  const results = [];
  for (const platform of platforms) {
    console.log(`[package-installer] packaging ${platform.goos}/${platform.goarch} (version ${platform.version})`);
    const context = { platform, helpers, checksums, inDir, outDir, workDir, repoRoot };
    const formats = [];
    for (const attempt of [attemptZip, attemptNsis, attemptDeb, attemptRpm, attemptAppImage]) {
      const entry = await attempt(context);
      formats.push(entry);
      logAttempt(entry);
    }
    results.push({
      goos: platform.goos,
      goarch: platform.goarch,
      version: platform.version,
      binary: platform.binary,
      formats,
    });
  }
  const summaryPath = path.join(outDir, "installers.json");
  await writeFile(
    summaryPath,
    `${JSON.stringify(
      {
        generatedAt: new Date().toISOString(),
        in: inDir,
        out: outDir,
        note: "Unsigned artifacts only. Skipped formats record ok:false with the tool-missing reason; never claimed as built.",
        platforms: results,
      },
      null,
      2,
    )}\n`,
    "utf8",
  );
  await rm(workDir, { recursive: true, force: true });
  const skipped = results.reduce(
    (count, platform) => count + platform.formats.filter((format) => !format.ok).length,
    0,
  );
  console.log(
    `[package-installer] summary written to ${summaryPath} (${skipped} skipped/failed format(s); skips are honest, never success)`,
  );
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  await main();
}
