#!/usr/bin/env node
// Wails qualification packaging (P6-02). Builds the Wails executable with a
// stamped version, produces a checksum file and an artifact manifest for the
// release-evidence workflow. Native builds keep the platform default CGO
// setting; cross builds are qualification binaries only (CGO disabled).
import { createHash } from "node:crypto";
import { readFile, writeFile, mkdir, readdir, rm } from "node:fs/promises";
import { spawnSync } from "node:child_process";
import path from "node:path";
import process from "node:process";
import { pathToFileURL } from "node:url";
import { supplyHelpers, packageHelpers, preserveFile, readSafeFile, assertSafePath, verifyHelper } from "./fetch-wails-helpers.mjs";
export { verifyHelper } from "./fetch-wails-helpers.mjs";

const GOOS_EXTENSIONS = new Set(["windows", "darwin", "linux"]);
const ARCH_EXTENSIONS = new Set(["amd64", "arm64"]);

export function artifactBasename(version, goos, goarch) {
  if (!version) throw new Error("version is required");
  if (!GOOS_EXTENSIONS.has(goos)) throw new Error(`unsupported GOOS ${goos}`);
  if (!ARCH_EXTENSIONS.has(goarch)) throw new Error(`unsupported GOARCH ${goarch}`);
  const extension = goos === "windows" ? ".exe" : "";
  return `LemonSSH-${version}-${goos}-${goarch}${extension}`;
}

export function buildLdflags(version) {
  const value = String(version).replace(/"/g, "");
  return `-s -w -X main.version=${value}`;
}

export function windowsGuiLdflags(goos) {
  return goos === "windows" ? " -H windowsgui" : "";
}

const FORBIDDEN_RUNTIME_MARKERS = Object.freeze(["electron", "node.exe", ".asar", "runtime/node_modules"]);

// purityInventory records the runtime contents of the Wails artifact.
export function purityInventory({ artifactName, sha256, bytes, goos, goarch, cross }) {
  return {
    artifactName,
    sha256,
    bytes,
    goos,
    goarch,
    cross,
    signed: false,
    installerFormats: [],
    forbiddenRuntimeMarkers: [...FORBIDDEN_RUNTIME_MARKERS],
    notes: [
      "Wails/Go runtime artifact",
      "code signing is optional and is not a release gate",
    ],
  };
}

export function parseArgs(argv) {
  const args = {
    version: undefined,
    goos: undefined,
    goarch: undefined,
    skipFrontend: false,
    outDir: path.join("dist", "wails"),
  };
  for (let index = 0; index < argv.length; index++) {
    const arg = argv[index];
    if (arg === "--version") args.version = argv[++index];
    else if (arg === "--goos") args.goos = argv[++index];
    else if (arg === "--goarch") args.goarch = argv[++index];
    else if (arg === "--skip-frontend") args.skipFrontend = true;
    else if (arg === "--out-dir") args.outDir = argv[++index];
    else throw new Error(`unknown argument ${arg}`);
  }
  return args;
}

export async function checksumEntries(paths) {
  const entries = [];
  for (const filePath of paths) {
    const content = await readSafeFile(filePath);
    entries.push({
      path: filePath,
      sha256: createHash("sha256").update(content).digest("hex"),
      bytes: content.length,
    });
  }
  return entries;
}

export async function packageFiles(directory, relative = "") {
  await assertSafePath(path.join(directory, relative));
  const files = [];
  for (const entry of await readdir(path.join(directory, relative), { withFileTypes: true })) {
    if (!relative && ["checksums.txt", "artifact-manifest.json"].includes(entry.name)) continue;
    const name = path.join(relative, entry.name);
    if (entry.isDirectory()) files.push(...await packageFiles(directory, name));
    else if (entry.isFile()) files.push(path.join(directory, name));
    else throw new Error(`unsafe package entry: ${name}`);
  }
  return files.sort();
}

export function helperResourcePath(goos, goarch, kind = "mosh") {
  const name = kind === "et"
    ? (goos === "windows" ? "et.exe" : "et")
    : (goos === "windows" ? "mosh-client.exe" : "mosh-client");
  let platformDir;
  if (goos === "windows") platformDir = goarch === "arm64" ? "win32-arm64" : "win32-x64";
  else if (goos === "darwin") platformDir = "darwin-universal";
  else platformDir = goarch === "arm64" ? "linux-arm64" : "linux-x64";
  return path.join("resources", kind, platformDir, name);
}

export async function writeProtocolResources(outDir, goos, executable) {
  if (!/^[a-zA-Z0-9._-]+$/.test(executable)) throw new Error("invalid executable name");
  if (goos === "linux") {
    const target = path.join(outDir, "lemonssh.desktop");
    await writeFile(target, `[Desktop Entry]\nType=Application\nName=LemonSSH\nExec=${executable} %u\nTerminal=false\nMimeType=x-scheme-handler/ssh;x-scheme-handler/telnet;x-scheme-handler/netcatty;\n`);
    return [target];
  }
  if (goos === "darwin") {
    const target = path.join(outDir, "Info.plist");
    await writeFile(target, `<?xml version="1.0" encoding="UTF-8"?>\n<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">\n<plist version="1.0"><dict><key>CFBundleIdentifier</key><string>app.lemonssh.desktop</string><key>CFBundleExecutable</key><string>${executable}</string><key>CFBundlePackageType</key><string>APPL</string><key>CFBundleURLTypes</key><array><dict><key>CFBundleURLName</key><string>Netcatty sessions</string><key>CFBundleURLSchemes</key><array><string>ssh</string><string>telnet</string><string>netcatty</string></array></dict></array></dict></plist>\n`);
    return [target];
  }
  return [];
}

export function hostTarget() {
  const osMap = { win32: "windows", darwin: "darwin", linux: "linux" };
  const goos = osMap[process.platform];
  if (!goos) throw new Error(`unsupported host platform ${process.platform}`);
  const goarch = process.arch === "x64" ? "amd64" : process.arch === "arm64" ? "arm64" : process.arch;
  return { goos, goarch };
}

export function shouldUseShell(command, platform = process.platform) {
  // npm is npm.cmd on Windows; spawnSync without a shell returns status null.
  return command === "npm" || platform === "win32";
}

function run(command, args, options = {}) {
  // npm needs the shell on Windows (npm.cmd); string commands are always run
  // through the shell so quoting stays explicit and verbatim.
  const { shell = shouldUseShell(command), ...rest } = options;
  const result = Array.isArray(args)
    ? spawnSync(command, args, { stdio: "inherit", shell, ...rest })
    : spawnSync(command, { stdio: "inherit", shell: true, ...rest });
  if (result.status !== 0 || result.error) {
    const detail = args ? `${command} ${args.join(" ")}` : command;
    throw new Error(`${detail} failed with status ${result.status}`);
  }
}

function gitCommit() {
  const result = spawnSync("git", ["rev-parse", "HEAD"], { encoding: "utf8" });
  return result.status === 0 ? result.stdout.trim() : "unknown";
}

export async function packageWails(argv = process.argv.slice(2), runCommand = run) {
  const args = parseArgs(argv);
  const pkg = JSON.parse(await readFile("package.json", "utf8"));
  const version = args.version ?? pkg.version;
  const { goos, goarch } = hostTarget();
  const target = {
    goos: args.goos ?? goos,
    goarch: args.goarch ?? goarch,
  };
  const cross = target.goos !== goos || target.goarch !== goarch;

  // Fail supply verification before spending time on the frontend / Go build.
  await supplyHelpers(target);

  if (!args.skipFrontend) {
    await runCommand("npm", ["run", "build"]);
    await runCommand("node", ["scripts/wails-prepare-frontend.mjs"]);
  }

  // Vite clears dist, so stage helpers only after the frontend build.
  const helpers = await packageHelpers(args.outDir, target.goos, target.goarch);
  await mkdir(args.outDir, { recursive: true });
  const artifact = path.join(args.outDir, artifactBasename(version, target.goos, target.goarch));
  const env = { ...process.env, GOOS: target.goos, GOARCH: target.goarch };
  if (cross) {
    env.CGO_ENABLED = "0";
    console.warn(`[package-wails] cross build for ${target.goos}/${target.goarch}: CGO disabled (qualification binary only)`);
  }
  await runCommand(`go build -trimpath "-ldflags=${buildLdflags(version)}${windowsGuiLdflags(target.goos)}" -o "${artifact}" ./cmd/netcatty`, null, { env });

  // CLI/MCP retain the console subsystem for JSON and stdio transports.
  const tools = [];
  const nativeTools = [
    { command: 'netcatty-tool', output: 'LemonSSH-tool' },
    { command: 'netcatty-mcp', output: 'LemonSSH-mcp' },
  ];
  for (const legacy of ['netcatty-tool', 'netcatty-mcp']) {
    await rm(path.join(args.outDir, legacy + (target.goos === 'windows' ? '.exe' : '')), { force: true });
  }
  for (const { command, output } of nativeTools) {
    const name = output + (target.goos === 'windows' ? '.exe' : '');
    await runCommand(`go build -trimpath "-ldflags=-s -w" -o "${path.join(args.outDir, name)}" ./cmd/${command}`, null, { env });
    tools.push(name);
  }

  await writeProtocolResources(args.outDir, target.goos, path.basename(artifact));
  await writeFile(path.join(args.outDir, "installer-resources.json"), JSON.stringify({
    helpers,
    tools,
    protocolResources: target.goos === "darwin" ? [{ source: "Info.plist", destination: "Contents/Info.plist" }] : target.goos === "linux" ? [{ source: "lemonssh.desktop", destination: "share/applications/lemonssh.desktop" }] : [],
  }, null, 2));

  const files = await packageFiles(args.outDir);
  const entries = await checksumEntries(files);

  const relativeName = (file) => path.relative(args.outDir, file).split(path.sep).join("/");
  const checksumLines = entries.map((entry) => `${entry.sha256}  ${relativeName(entry.path)}`);
  await writeFile(path.join(args.outDir, "checksums.txt"), `${checksumLines.join("\n")}\n`, "utf8");

  const manifest = {
    version,
    commit: gitCommit(),
    goos: target.goos,
    goarch: target.goarch,
    cross,
    builtAt: new Date().toISOString(),
    // Helper pins bind this artifact to the exact locked Mosh/ET bytes and
    // their provenance (also detailed in installer-resources.json).
    helpers,
    tools,
    artifacts: entries.map((entry) => ({ name: relativeName(entry.path), ...entry })),
    purity: entries.map((entry) => purityInventory({
      artifactName: relativeName(entry.path),
      sha256: entry.sha256,
      bytes: entry.bytes,
      goos: target.goos,
      goarch: target.goarch,
      cross,
    })),
  };
  await writeFile(path.join(args.outDir, "artifact-manifest.json"), `${JSON.stringify(manifest, null, 2)}\n`, "utf8");
  console.log(`[package-wails] packaged ${manifest.artifacts.length} artifact(s) for ${target.goos}/${target.goarch} in ${args.outDir}`);
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  // Helper provisioning lives solely in the reviewed lock flow
  // (scripts/fetch-wails-helpers.mjs); ad-hoc installs would write sidecars
  // that the trusted lock refuses at packaging time.
  await packageWails();
}
