#!/usr/bin/env node
// Wails qualification packaging (P6-02). Builds the Wails executable with a
// stamped version, produces a checksum file and an artifact manifest for the
// release-evidence workflow. Native builds keep the platform default CGO
// setting; cross builds are qualification binaries only (CGO disabled).
import { createHash } from "node:crypto";
import { existsSync } from "node:fs";
import { copyFile, readFile, writeFile, mkdir, readdir } from "node:fs/promises";
import { spawnSync } from "node:child_process";
import path from "node:path";
import process from "node:process";
import { pathToFileURL } from "node:url";

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

const ELECTRON_MARKERS = Object.freeze(["electron", "node.exe", ".asar", "runtime/node_modules"]);

// purityInventory records what a qualification artifact is, without claiming a
// signed installer or a Node-free final RC. REL-03.1 still needs P8-01.
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
    electronMarkers: [...ELECTRON_MARKERS],
    notes: [
      "bare Go binary only; no msi, pkg, AppImage, deb, or rpm",
      "Authenticode/codesign not invoked",
      "REL-03.1 artifact purity waits on P8-01 signed RC",
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
    const content = await readFile(filePath);
    entries.push({
      path: filePath,
      sha256: createHash("sha256").update(content).digest("hex"),
      bytes: content.length,
    });
  }
  return entries;
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

export function hostTarget() {
  const osMap = { win32: "windows", darwin: "darwin", linux: "linux" };
  const goos = osMap[process.platform];
  if (!goos) throw new Error(`unsupported host platform ${process.platform}`);
  const goarch = process.arch === "x64" ? "amd64" : process.arch === "arm64" ? "arm64" : process.arch;
  return { goos, goarch };
}

function run(command, args, options = {}) {
  // npm needs the shell on Windows (npm.cmd); string commands are always run
  // through the shell so quoting stays explicit and verbatim.
  const { shell = command !== "npm", ...rest } = options;
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

async function main() {
  const args = parseArgs(process.argv.slice(2));
  const pkg = JSON.parse(await readFile("package.json", "utf8"));
  const version = args.version ?? pkg.version;
  const { goos, goarch } = hostTarget();
  const target = {
    goos: args.goos ?? goos,
    goarch: args.goarch ?? goarch,
  };
  const cross = target.goos !== goos || target.goarch !== goarch;

  if (!args.skipFrontend) {
    run("npm", ["run", "build"]);
    run("node", ["scripts/wails-prepare-frontend.mjs"]);
  }

  await mkdir(args.outDir, { recursive: true });
  const artifact = path.join(args.outDir, artifactBasename(version, target.goos, target.goarch));
  const env = { ...process.env, GOOS: target.goos, GOARCH: target.goarch };
  if (cross) {
    env.CGO_ENABLED = "0";
    console.warn(`[package-wails] cross build for ${target.goos}/${target.goarch}: CGO disabled (qualification binary only)`);
  }
  run(`go build -trimpath "-ldflags=${buildLdflags(version)}${windowsGuiLdflags(target.goos)}" -o "${artifact}" ./cmd/netcatty`, null, { env });

  for (const kind of ["mosh", "et"]) {
    const helper = helperResourcePath(target.goos, target.goarch, kind);
    if (existsSync(helper)) {
      const dest = path.join(args.outDir, path.basename(helper));
      await copyFile(helper, dest);
    }
  }

  const files = (await readdir(args.outDir))
    .filter((name) => name !== "checksums.txt" && name !== "artifact-manifest.json")
    .map((name) => path.join(args.outDir, name));
  const entries = await checksumEntries(files);

  const checksumLines = entries.map((entry) => `${entry.sha256}  ${path.basename(entry.path)}`);
  await writeFile(path.join(args.outDir, "checksums.txt"), `${checksumLines.join("\n")}\n`, "utf8");

  const manifest = {
    version,
    commit: gitCommit(),
    goos: target.goos,
    goarch: target.goarch,
    cross,
    builtAt: new Date().toISOString(),
    artifacts: entries.map((entry) => ({ name: path.basename(entry.path), ...entry })),
    purity: entries.map((entry) => purityInventory({
      artifactName: path.basename(entry.path),
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
  await main();
}
