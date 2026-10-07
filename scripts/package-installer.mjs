#!/usr/bin/env node
// Installer packaging for the unsigned Wails artifacts (P6 follow-up).
// Consumes the dist/wails output of scripts/package-wails.mjs and produces
// installer formats from the packaged binary. The installer-resources.json
// contract written by package-wails (helper inventory with macOS bundle
// destinations, CLI/MCP tool names, protocol resources) drives what each
// format installs: URL-protocol registration for NSIS, the .desktop file and
// PATH-resident helpers for deb/rpm, MimeType + helpers for the AppImage, and
// a real Contents/-shaped bundle for macOS. Project policy: NO signing and
// NO fake success. A format whose packaging tool is missing is an honest skip
// recorded in installers.json as { format, ok: false, reason: "..." } and is
// never reported as built.
import { existsSync } from "node:fs";
import { chmod, copyFile, mkdir, readFile, readdir, rm, writeFile } from "node:fs/promises";
import { spawnSync } from "node:child_process";
import path from "node:path";
import process from "node:process";
import { pathToFileURL } from "node:url";

const APP_NAME = "LemonSSH";
const MAINTAINER = "LemonSSH Maintainers";
const DESCRIPTION =
  "LemonSSH is a modern SSH manager and terminal app with host grouping, SFTP, keychain, port forwarding, and a rich UI.";
const LICENSE = "GPL-3.0-or-later";
const LINUX_BINARY_DIR = "usr/local/bin";

// Matches the artifactBasename layout of scripts/package-wails.mjs.
const BINARY_PATTERN = /^LemonSSH-(.+)-(windows|linux|darwin)-(amd64|arm64)(\.exe)?$/;
const HELPER_NAMES = new Set(["mosh-client", "mosh-client.exe", "et", "et.exe", "LemonSSH-tool", "LemonSSH-tool.exe", "LemonSSH-mcp", "LemonSSH-mcp.exe"]);
const CHECKSUM_FILE = "checksums.txt";
const ICON_CANDIDATES = ["build/appicon.png", "build/icons/256x256.png", "build/icons/512x512.png"];

// URL schemes LemonSSH owns; mirrors deeplink.ProtocolSchemes plus the legacy
// "netcatty" scheme in internal/platform/deeplink/protocolreg.go and the
// scheme declarations that scripts/package-wails.mjs writes into
// lemonssh.desktop / Info.plist. The legacy netcatty:// scheme stays
// registered so pre-rename deep links keep opening the app.
export const PROTOCOL_SCHEMES = ["ssh", "telnet", "lemonssh", "netcatty"];
// .desktop MimeType value for those schemes (trailing ";" per the spec).
const SCHEME_MIME_TYPES = `${PROTOCOL_SCHEMES.map((scheme) => `x-scheme-handler/${scheme}`).join(";")};`;
// The package-wails -> installer contract file in the dist/wails input.
const INSTALLER_RESOURCES_FILE = "installer-resources.json";

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

// NSIS scripts are generated for Windows targets, so every path they embed
// must use backslashes no matter which host OS assembles the script (the unit
// tests run on Linux CI). Host paths may arrive with either separator
// (path.join output differs per platform), so normalize them here and derive
// basenames from both separators instead of path.basename, which only splits
// on "\" on Windows and therefore keeps "C:\build\x.exe" whole on POSIX.
const toNsisPath = (value) => String(value).replaceAll("/", "\\");
const nsisBasename = (value) => {
  const normalized = toNsisPath(value);
  return normalized.split("\\").filter(Boolean).pop() ?? normalized;
};

export function nsisScript({ name, version, exeFile, outFile, helperFiles = [], schemes = PROTOCOL_SCHEMES }) {
  if (!name || !version || !exeFile || !outFile) {
    throw new Error("name, version, exeFile and outFile are required");
  }
  const exeName = nsisBasename(exeFile);
  const installFiles = [`  File "${toNsisPath(exeFile)}"`];
  const uninstallFiles = [`  Delete "$INSTDIR\\${exeName}"`];
  for (const helperFile of helperFiles) {
    const helperName = nsisBasename(helperFile);
    installFiles.push(`  File "${toNsisPath(helperFile)}"`);
    uninstallFiles.push(`  Delete "$INSTDIR\\${helperName}"`);
  }
  // URL-scheme handoff written exactly like internal/platform/deeplink/
  // protocolreg.go ProtocolSpecs (HKCU\Software\Classes\<scheme> with the
  // "URL Protocol" marker and shell\open\command = '"<exe>" "%1"') so
  // DeepLinkService.GetOSProtocolStatus recognizes the installer-owned
  // registration. HKCU needs no elevation.
  const registryInstall = [];
  const registryUninstall = [];
  for (const scheme of schemes) {
    registryInstall.push(
      `  WriteRegStr HKCU "Software\\Classes\\${scheme}" "" "URL:${name} ${scheme} Protocol"`,
      `  WriteRegStr HKCU "Software\\Classes\\${scheme}" "URL Protocol" ""`,
      `  WriteRegStr HKCU "Software\\Classes\\${scheme}\\shell\\open\\command" "" '"$INSTDIR\\${exeName}" "%1"'`,
    );
    // Without /ifempty DeleteRegKey removes the whole scheme subtree.
    registryUninstall.push(`  DeleteRegKey HKCU "Software\\Classes\\${scheme}"`);
  }
  return [
    `; ${name} ${version} Windows installer (unsigned, no Authenticode)`,
    `Name "${name} ${version}"`,
    `OutFile "${toNsisPath(outFile)}"`,
    `InstallDir "$PROGRAMFILES64\\${name}"`,
    "SetCompressor lzma",
    "",
    'Section "Install"',
    '  SetOutPath "$INSTDIR"',
    ...installFiles,
    `  CreateDirectory "$SMPROGRAMS\\${name}"`,
    `  CreateShortCut "$SMPROGRAMS\\${name}\\${name}.lnk" "$INSTDIR\\${exeName}"`,
    `  CreateShortCut "$DESKTOP\\${name}.lnk" "$INSTDIR\\${exeName}"`,
    `  WriteUninstaller "$INSTDIR\\Uninstall ${name}.exe"`,
    "SectionEnd",
    "",
    'Section "Register URL protocols (ssh://, telnet://, lemonssh://, netcatty://)"',
    ...registryInstall,
    "SectionEnd",
    "",
    'Section "Uninstall"',
    ...uninstallFiles,
    `  Delete "$INSTDIR\\Uninstall ${name}.exe"`,
    `  Delete "$SMPROGRAMS\\${name}\\${name}.lnk"`,
    `  Delete "$DESKTOP\\${name}.lnk"`,
    `  RMDir "$SMPROGRAMS\\${name}"`,
    ...registryUninstall,
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

export function rpmSpec({ name, version, arch, exeFile, helperFiles = [], toolFiles = [], desktopFile = null }) {
  if (!name || !version || !exeFile) {
    throw new Error("name, version and exeFile are required");
  }
  const rpmArch = archMapping(arch).rpm;
  const rpmName = name.toLowerCase();
  const install = [
    "mkdir -p %{buildroot}%{_bindir}",
    `install -m 0755 "${exeFile}" %{buildroot}%{_bindir}/${rpmName}`,
  ];
  const files = [`%attr(0755,root,root) %{_bindir}/${rpmName}`];
  // Helpers sit in the nested kind layout beside the main binary
  // (helperpaths.go resolves exeDir/<kind>/<name>); the kind subdirectory also
  // keeps the pinned mosh-client from clobbering a distro /usr/bin/mosh-client.
  for (const helper of helperFiles) {
    install.push(
      `mkdir -p %{buildroot}%{_bindir}/${helper.directory}`,
      `install -m 0755 "${helper.source}" %{buildroot}%{_bindir}/${helper.directory}/${helper.name}`,
    );
    files.push(`%attr(0755,root,root) %{_bindir}/${helper.directory}/${helper.name}`);
  }
  for (const tool of toolFiles) {
    install.push(`install -m 0755 "${tool.source}" %{buildroot}%{_bindir}/${tool.name}`);
    files.push(`%attr(0755,root,root) %{_bindir}/${tool.name}`);
  }
  if (desktopFile) {
    install.push(
      "mkdir -p %{buildroot}%{_datadir}/applications",
      `install -m 0644 "${desktopFile.source}" %{buildroot}%{_datadir}/applications/${desktopFile.name}`,
    );
    files.push(`%attr(0644,root,root) %{_datadir}/applications/${desktopFile.name}`);
  }
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
    ...install,
    "",
    "%files",
    ...files,
    "",
  ].join("\n");
}

export function appImageDesktop({ name, exec, icon, mimeTypes = SCHEME_MIME_TYPES }) {
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
    `MimeType=${mimeTypes}`,
    "",
  ].join("\n");
}

// The AppDir desktop entry. Exec carries the %u field code so ssh://,
// telnet://, lemonssh:// and netcatty:// deep links hand the URL to the
// AppImage: the
// runtime passes it to AppRun ("$@") and the binary opens the session
// directly instead of a bare launch.
export function appImageDesktopEntry(name = APP_NAME) {
  return appImageDesktop({ name, exec: `${name} %u`, icon: name });
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

// Reads the installer-resources.json contract written by
// scripts/package-wails.mjs: the helper inventory (with macOS bundle
// destinations), the CLI/MCP tool names, and the protocol resources
// (lemonssh.desktop / Info.plist) with their target destinations. A missing
// file means older dist output; the installers then fall back to HELPER_NAMES
// discovery and skip the desktop/plist resources instead of inventing them.
export async function readInstallerResources(inDir) {
  const manifestPath = path.join(inDir, INSTALLER_RESOURCES_FILE);
  if (!existsSync(manifestPath)) return { helpers: [], tools: [], protocolResources: [] };
  const parsed = JSON.parse(await readFile(manifestPath, "utf8"));
  return {
    helpers: Array.isArray(parsed.helpers) ? parsed.helpers : [],
    tools: Array.isArray(parsed.tools) ? parsed.tools : [],
    protocolResources: Array.isArray(parsed.protocolResources) ? parsed.protocolResources : [],
  };
}

// installer-resources.json is a build-time contract but still parsed JSON:
// reject source/destination segments that would escape the staging roots
// instead of trusting the file blindly.
function safeResourceSegments(value, label) {
  const segments = String(value ?? "").split("/");
  if (!value || segments.some((segment) => !segment || segment === "." || segment === ".." || segment.includes("\\") || segment.includes(":"))) {
    throw new Error(`unsafe installer resource ${label}: ${value}`);
  }
  return segments;
}

// Rewrites a .desktop Exec line so the launcher points at the command the
// package actually installs (deb: /usr/local/bin/lemonssh, rpm: /usr/bin/lemonssh
// — both plain "lemonssh") instead of the versioned artifact basename that
// scripts/package-wails.mjs writes ("Exec=LemonSSH-<version>-linux-<arch> %u",
// a file that no longer exists after install). Everything after the executable
// token — e.g. the %u field code for ssh:// deep links — is preserved; a
// desktop file without an Exec line comes back unchanged. Idempotent.
export function normalizeDesktopExec(desktop, command) {
  if (!/^\s*Exec=/m.test(desktop)) return desktop;
  return desktop.replace(/^Exec=\s*\S+(.*)$/m, `Exec=${command}$1`);
}

// Copies a protocol resource into a staging tree. .desktop sources get their
// Exec token normalized to `command` (the installed binary name); anything
// else is copied verbatim. Used by the deb staging tree and the rpm spec
// staging so both package formats ship a working menu entry.
export async function stageDesktopResource(source, destination, command) {
  if (/\.desktop$/i.test(path.basename(source))) {
    const desktop = await readFile(source, "utf8");
    await writeFile(destination, normalizeDesktopExec(desktop, command), "utf8");
  } else {
    await copyFile(source, destination);
  }
  await chmod(destination, 0o644).catch(() => {});
}

// Files (basenames inside the dist/wails input) that a Windows installer
// installs beside the main executable: the mosh/et helpers plus any DLL that
// ships beside et.exe for the loader, and the CLI/MCP tools. Helper pin
// manifests and third-party licenses stay out of the installer (the portable
// zip and the macOS bundle carry them); without installer-resources.json the
// discovery falls back to the .exe helper names.
export function windowsInstallerFiles({ resources, discoveredHelpers = [] } = {}) {
  const names = new Set();
  for (const helper of resources?.helpers ?? []) {
    // installer-resources.json is written per packaging target, but filter by
    // the declared os anyway so a mixed dist dir never Files a foreign helper.
    if (helper.os && helper.os !== "windows") continue;
    for (const file of helper.packagedFiles ?? []) {
      const source = String(file.destination ?? file.path ?? "");
      const name = path.basename(source);
      if (!name) continue;
      if (/\.manifest\.json$/i.test(name)) continue;
      if (/^licenses\//i.test(source)) continue;
      names.add(name);
    }
    if (helper.destination) names.add(path.basename(helper.destination));
  }
  if (names.size === 0) {
    for (const discovered of discoveredHelpers) {
      if (path.basename(discovered).endsWith(".exe")) names.add(path.basename(discovered));
    }
  }
  for (const tool of resources?.tools ?? []) names.add(path.basename(tool));
  return [...names].sort();
}

// Linux install plan for helpers/tools relative to the bin directory.
// mosh/et use the nested kind layout (<binDir>/<kind>/<name>) that
// internal/app/terminaluse/helperpaths.go resolves beside the executable
// (exeDir/<kind>/<name>); the subdirectory also keeps the pinned mosh-client
// from shadowing a distro /usr/bin/mosh-client on PATH. The CLI/MCP tools are
// user-facing commands and sit flat in the same bin directory.
// kindLayout=false flattens the helpers (isolated AppDir usr/bin).
export function linuxHelperTargets({ resources, discoveredHelpers = [], kindLayout = true } = {}) {
  const targets = [];
  const seen = new Set();
  const push = (source, directory) => {
    const name = path.basename(String(source));
    if (!name || name.endsWith(".exe")) return; // wrong-platform helper name
    const key = `${directory}/${name}`;
    if (seen.has(key)) return;
    seen.add(key);
    targets.push({ source: name, directory, name });
  };
  for (const helper of resources?.helpers ?? []) {
    const source = helper.destination ?? helper.path ?? "";
    if (!source) continue;
    push(source, kindLayout ? (helper.kind === "et" ? "et" : "mosh") : ".");
  }
  if (!(resources?.helpers ?? []).length) {
    // Legacy fallback (no installer-resources.json helpers): use the same
    // nested kind layout as the contract path so a discovered mosh-client
    // installed flat next to /usr/local/bin/lemonssh cannot shadow a distro
    // /usr/bin/mosh-client on PATH (helperpaths.go resolves both layouts).
    // The CLI/MCP tools are user-facing commands and stay flat; an isolated
    // AppDir usr/bin (kindLayout=false) stays flat like the portable zip.
    for (const discovered of discoveredHelpers) {
      const name = path.basename(discovered);
      const kind = name === "et" ? "et" : name === "mosh-client" ? "mosh" : null;
      push(discovered, kindLayout && kind ? kind : ".");
    }
  }
  for (const tool of resources?.tools ?? []) push(tool, ".");
  return targets;
}

function artifactBase(platform) {
  const extension = platform.goos === "windows" ? ".exe" : "";
  return `LemonSSH-${platform.version}-${platform.goos}-${platform.goarch}${extension}`;
}

// Builds LemonSSH.app/Contents/{Info.plist,MacOS,Resources} from the flat
// scripts/package-wails.mjs output. installer-resources.json declares the
// bundle destinations (Contents/Info.plist for the protocol plist,
// Contents/MacOS for helpers/tools, Contents/Resources/licenses for
// third-party licenses); this staging is what finally assembles a real bundle
// instead of shipping a flat Mach-O. The plist's protocol declarations and
// CFBundleExecutable are verified before the bundle is zipped.
export async function stageDarwinAppBundle({ inDir, stageDir, platform, resources, discoveredHelpers = [] }) {
  const bundle = path.join(stageDir, `${APP_NAME}.app`);
  const contents = path.join(bundle, "Contents");
  const macosDir = path.join(contents, "MacOS");
  await mkdir(macosDir, { recursive: true });
  await writeFile(path.join(contents, "PkgInfo"), "APPL????", "utf8");
  const executableName = path.basename(platform.binary);
  // The main executable keeps the packaged name; the staged Info.plist's
  // CFBundleExecutable declares exactly that name (verified below).
  const bundleBinary = path.join(macosDir, executableName);
  await copyFile(platform.binary, bundleBinary);
  await chmod(bundleBinary, 0o755).catch(() => {});
  // Info.plist must sit at Contents/Info.plist — the destination
  // installer-resources.json declares. scripts/package-wails.mjs writes it
  // flat beside the binary; this mapping fixes the bundle structure.
  const protocolResources = [...(resources?.protocolResources ?? [])];
  if (!protocolResources.some((resource) => resource.source === "Info.plist") && existsSync(path.join(inDir, "Info.plist"))) {
    protocolResources.push({ source: "Info.plist", destination: "Contents/Info.plist" });
  }
  for (const resource of protocolResources) {
    const source = path.join(inDir, ...safeResourceSegments(resource.source, "source"));
    if (!existsSync(source)) continue;
    const destination = path.join(bundle, ...safeResourceSegments(resource.destination, "destination"));
    await mkdir(path.dirname(destination), { recursive: true });
    await copyFile(source, destination);
  }
  const plistPath = path.join(contents, "Info.plist");
  if (!existsSync(plistPath)) {
    throw new Error("macOS bundle staging requires Info.plist (missing from installer-resources.json and the dist input)");
  }
  const plist = await readFile(plistPath, "utf8");
  for (const scheme of PROTOCOL_SCHEMES) {
    if (!plist.includes(`<string>${scheme}</string>`)) {
      throw new Error(`macOS bundle Info.plist does not declare the ${scheme}:// URL scheme`);
    }
  }
  const executable = plist.match(/<key>CFBundleExecutable<\/key><string>([^<]+)<\/string>/);
  if (executable && executable[1] !== executableName) {
    throw new Error(`macOS bundle Info.plist CFBundleExecutable ${executable[1]} does not match the staged executable ${executableName}`);
  }
  // Helpers follow the destinations packageHelpers declared (Contents/MacOS
  // executables plus pin manifests, Contents/Resources/licenses); without
  // installer-resources.json the flat non-.exe discovery feeds MacOS/ directly.
  const helperEntries = (resources?.helpers ?? []).length
    ? resources.helpers.flatMap((helper) => (helper.packagedFiles ?? []).map((file) => ({
        source: file.path,
        destination: file.destination ?? file.path,
        executable: file.path === helper.path,
      })))
    : discoveredHelpers
        .filter((helper) => !path.basename(helper).endsWith(".exe"))
        .map((helper) => ({ source: path.basename(helper), destination: path.basename(helper), executable: true }));
  for (const entry of helperEntries) {
    if (!entry.source) continue;
    const source = path.join(inDir, ...safeResourceSegments(entry.source, "helper source"));
    if (!existsSync(source)) continue;
    const destination = path.join(bundle, ...safeResourceSegments(entry.destination, "helper destination"));
    await mkdir(path.dirname(destination), { recursive: true });
    await copyFile(source, destination);
    await chmod(destination, entry.executable ? 0o755 : 0o644).catch(() => {});
  }
  // CLI/MCP tools beside the executable so the exeDir lookup finds them.
  for (const tool of resources?.tools ?? []) {
    const source = path.join(inDir, ...safeResourceSegments(tool, "tool source"));
    if (!existsSync(source)) continue;
    const destination = path.join(macosDir, path.basename(tool));
    await copyFile(source, destination);
    await chmod(destination, 0o755).catch(() => {});
  }
  return bundle;
}

// Always attempted: a portable zip of the already packaged files (binary,
// optional mosh/et helpers, checksums.txt). Reuses existing files only.
// darwin instead zips a real LemonSSH.app bundle; a staging failure is an
// honest skip, never a flat-file fallback.
async function attemptZip({ platform, helpers, checksums, resources, inDir, outDir, workDir }) {
  const outZip = path.join(outDir, `${artifactBase(platform)}.zip`);
  let command;
  let cwd = inDir;
  if (platform.goos === "darwin") {
    if (!toolAvailable(TOOL_PROBES.zip)) {
      return { format: "zip", ok: false, reason: "zip not installed" };
    }
    const stageDir = path.join(workDir, `macbundle-${platform.goarch}`);
    let bundle;
    try {
      bundle = await stageDarwinAppBundle({ inDir, stageDir, platform, resources, discoveredHelpers: helpers });
    } catch (error) {
      return { format: "zip", ok: false, reason: `macOS bundle staging failed: ${error.message}` };
    }
    const members = [path.basename(bundle)];
    if (checksums) {
      // checksums.txt lives in the dist input, but the zip runs inside the
      // bundle staging directory; copy it in so the member actually exists.
      await copyFile(checksums, path.join(stageDir, path.basename(checksums)));
      members.push(path.basename(checksums));
    }
    const list = members.map((name) => `'${name}'`).join(" ");
    command = `zip -q -r '${outZip}' ${list}`;
    cwd = stageDir;
  } else {
    const files = [artifactBase(platform), ...helpersFor(helpers, platform.goos)];
    if (checksums) files.push(path.basename(checksums));
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
  }
  const result = spawnSync(command, { shell: true, cwd, encoding: "utf8" });
  if (result.status !== 0 || !existsSync(outZip)) {
    return {
      format: "zip",
      ok: false,
      reason: `zip packaging failed with status ${result.status}: ${tail(result.stderr)}`,
    };
  }
  return { format: "zip", ok: true, file: outZip };
}

async function attemptNsis({ platform, outDir, workDir, inDir, resources, helpers }) {
  if (platform.goos !== "windows") {
    return { format: "nsis", ok: false, reason: `nsis not applicable for ${platform.goos} artifacts` };
  }
  if (!toolAvailable(TOOL_PROBES.nsis)) {
    return { format: "nsis", ok: false, reason: "makensis not installed" };
  }
  const base = artifactBase(platform).replace(/\.exe$/, "");
  const outFile = path.join(outDir, `${base}-setup.exe`);
  const scriptPath = path.join(workDir, `${base}.nsi`);
  // Helpers and CLI/MCP tools install beside the main executable.
  const helperFiles = windowsInstallerFiles({ resources, discoveredHelpers: helpers })
    .map((name) => path.join(inDir, name))
    .filter((file) => existsSync(file));
  await writeFile(
    scriptPath,
    nsisScript({ name: APP_NAME, version: platform.version, exeFile: platform.binary, outFile, helperFiles }),
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

// Maps the flat scripts/package-wails.mjs output into a deb filesystem tree:
// DEBIAN/control, the main binary under LINUX_BINARY_DIR, helpers/tools beside
// it, and the .desktop file at the prefix-relative destination that
// installer-resources.json declares (share/applications/lemonssh.desktop ->
// /usr/share/applications).
export async function stageDebTree({ inDir, staging, platform, resources, discoveredHelpers = [] }) {
  const controlDir = path.join(staging, "DEBIAN");
  const binaryDir = path.join(staging, ...LINUX_BINARY_DIR.split("/"));
  await mkdir(controlDir, { recursive: true });
  await mkdir(binaryDir, { recursive: true });
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
  // Protocol resources (the desktop entry with the
  // ssh/telnet/lemonssh/netcatty MimeType line) land at their declared
  // prefix-relative destinations; the
  // .desktop Exec token is normalized to the installed command name so the
  // menu entry launches /usr/local/bin/lemonssh, not the versioned artifact.
  const installedCommand = APP_NAME.toLowerCase();
  for (const resource of resources?.protocolResources ?? []) {
    const source = path.join(inDir, ...safeResourceSegments(resource.source, "source"));
    if (!existsSync(source)) continue;
    const destination = path.join(staging, ...safeResourceSegments(resource.destination, "destination"));
    await mkdir(path.dirname(destination), { recursive: true });
    await stageDesktopResource(source, destination, installedCommand);
  }
  for (const target of linuxHelperTargets({ resources, discoveredHelpers, kindLayout: true })) {
    const source = path.join(inDir, target.source);
    if (!existsSync(source)) continue;
    const destination = path.join(binaryDir, ...target.directory.split("/"), target.name);
    await mkdir(path.dirname(destination), { recursive: true });
    await copyFile(source, destination);
    await chmod(destination, 0o755).catch(() => {});
  }
  return staging;
}

async function attemptDeb({ platform, outDir, workDir, inDir, resources, helpers }) {
  if (platform.goos !== "linux") {
    return { format: "deb", ok: false, reason: `deb not applicable for ${platform.goos} artifacts` };
  }
  if (!toolAvailable(TOOL_PROBES.deb)) {
    return { format: "deb", ok: false, reason: "dpkg-deb not installed" };
  }
  const staging = path.join(workDir, `deb-${platform.goarch}`);
  await stageDebTree({ inDir, staging, platform, resources, discoveredHelpers: helpers });
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

async function attemptRpm({ platform, outDir, workDir, inDir, resources, helpers }) {
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
  // Helpers/tools/desktop come from the same plan as the deb staging; sources
  // must exist or the spec shrinks honestly instead of failing mid-rpmbuild.
  const installTargets = linuxHelperTargets({ resources, discoveredHelpers: helpers, kindLayout: true })
    .map((target) => ({ ...target, source: path.join(inDir, target.source) }))
    .filter((target) => existsSync(target.source));
  const desktopResource = (resources?.protocolResources ?? []).find((resource) =>
    String(resource.source).endsWith(".desktop"),
  );
  const desktopSource = desktopResource ? path.join(inDir, ...safeResourceSegments(desktopResource.source, "source")) : null;
  let desktopFile = null;
  if (desktopSource && existsSync(desktopSource)) {
    // The %install section installs this file verbatim, so normalize the Exec
    // token into a staged copy: the spec must ship "Exec=lemonssh" (installed
    // at %{_bindir}/lemonssh), not the versioned artifact basename.
    const stagedDesktop = path.join(topdir, "SPECS", path.basename(desktopSource));
    await stageDesktopResource(desktopSource, stagedDesktop, APP_NAME.toLowerCase());
    desktopFile = {
      source: stagedDesktop,
      name: path.basename(desktopResource.destination ?? desktopResource.source),
    };
  }
  await writeFile(
    specPath,
    rpmSpec({
      name: APP_NAME,
      version: platform.version,
      arch: platform.goarch,
      exeFile: platform.binary,
      helperFiles: installTargets.filter((target) => target.directory !== "."),
      toolFiles: installTargets.filter((target) => target.directory === "."),
      desktopFile,
    }),
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

async function attemptAppImage({ platform, outDir, workDir, repoRoot, inDir, resources, helpers }) {
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
    appImageDesktopEntry(APP_NAME),
    "utf8",
  );
  const appImageBinary = path.join(binDir, APP_NAME);
  await copyFile(platform.binary, appImageBinary);
  await chmod(appImageBinary, 0o755).catch(() => {});
  // Helpers and CLI/MCP tools beside the executable inside the isolated
  // AppDir: usr/bin is the exeDir the AppRun execs from, so
  // helperpaths.go resolves them flat, like the portable zip layout.
  for (const target of linuxHelperTargets({ resources, discoveredHelpers: helpers, kindLayout: false })) {
    const source = path.join(inDir, target.source);
    if (!existsSync(source)) continue;
    const destination = path.join(binDir, target.name);
    await copyFile(source, destination);
    await chmod(destination, 0o755).catch(() => {});
  }
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

// One installer format attempt. A staging crash (copy failure, unsafe
// resource path, ...) is wrapped into the same honest ok:false shape as the
// tool-missing skips, with the underlying message kept in `reason`, so a
// single broken format neither aborts the remaining formats nor the
// installers.json summary report.
export async function runAttempt(format, attempt, context) {
  try {
    return await attempt(context);
  } catch (error) {
    return { format, ok: false, reason: `staging failed: ${error.message}` };
  }
}

const FORMAT_ATTEMPTS = [
  ["zip", attemptZip],
  ["nsis", attemptNsis],
  ["deb", attemptDeb],
  ["rpm", attemptRpm],
  ["appimage", attemptAppImage],
];

// Runs every installer format for one platform, in order, even when an
// earlier attempt throws; each entry is reported through onAttempt as soon
// as it completes.
export async function attemptInstallerFormats(context, attempts = FORMAT_ATTEMPTS, onAttempt = logAttempt) {
  const formats = [];
  for (const [format, attempt] of attempts) {
    const entry = await runAttempt(format, attempt, context);
    formats.push(entry);
    onAttempt(entry);
  }
  return formats;
}

export async function runInstaller(argv = process.argv.slice(2)) {
  const args = parseInstallerArgs(argv);
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
  let resources;
  try {
    resources = await readInstallerResources(inDir);
  } catch (error) {
    console.error(`[package-installer] ${error.message}`);
    process.exitCode = 1;
    return;
  }
  console.log(
    `[package-installer] installer resources: ${resources.helpers.length} helper(s), ${resources.tools.length} tool(s), ${resources.protocolResources.length} protocol resource(s)`,
  );
  await mkdir(outDir, { recursive: true });
  const workDir = path.join(outDir, ".installer-work");
  await mkdir(workDir, { recursive: true });
  const repoRoot = process.cwd();
  const results = [];
  // Symmetric cleanup: the work dir (deb/rpm staging trees, the AppDir, the
  // macOS bundle stage) is removed whether the run completes, records honest
  // per-format failures, or dies midway — without this finally a staging
  // failure leaks half-built staging trees next to the release artifacts.
  try {
    for (const platform of platforms) {
      console.log(`[package-installer] packaging ${platform.goos}/${platform.goarch} (version ${platform.version})`);
      const context = { platform, helpers, checksums, resources, inDir, outDir, workDir, repoRoot };
      const formats = await attemptInstallerFormats(context);
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
    const skipped = results.reduce(
      (count, platform) => count + platform.formats.filter((format) => !format.ok).length,
      0,
    );
    console.log(
      `[package-installer] summary written to ${summaryPath} (${skipped} skipped/failed format(s); skips are honest, never success)`,
    );
  } finally {
    await rm(workDir, { recursive: true, force: true });
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  await runInstaller();
}
