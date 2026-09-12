#!/usr/bin/env node
// Wails qualification packaging (P6-02). Builds the Wails executable with a
// stamped version, produces a checksum file and an artifact manifest for the
// release-evidence workflow. Native builds keep the platform default CGO
// setting; cross builds are qualification binaries only (CGO disabled).
import { createHash } from "node:crypto";
import { existsSync } from "node:fs";
import { copyFile, readFile, writeFile, mkdir, readdir, chmod } from "node:fs/promises";
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

export function verifyHelper(data, pin, goos, goarch) {
  if (pin.os !== goos || pin.arch !== goarch) throw new Error("helper target does not match pin");
  if (createHash("sha256").update(data).digest("hex") !== pin.sha256) throw new Error("helper hash mismatch");
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
      for (let i = 0; i < count; i++) if (data.readUInt32BE(8 + i * 20) === cpu) matches = true;
    }
  } else if (goos === "linux" && data.length >= 20 && data.toString("hex", 0, 4) === "7f454c46") {
    matches = data[4] === 2 && data[5] === 1 && data.readUInt16LE(18) === (goarch === "amd64" ? 62 : 183);
  }
  if (!matches) throw new Error("helper executable architecture mismatch");
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

// Provision a release helper only with an independently supplied digest.
// Usage: node scripts/package-wails.mjs --install-helper mosh --source FILE_OR_HTTPS_URL --sha256 DIGEST --goos windows --goarch amd64
export async function installHelper({ source, destination, sha256, goos, goarch, kind }) {
 if (!["mosh","et"].includes(kind)) throw new Error("unknown helper kind");
 if (!/^[a-f0-9]{64}$/.test(sha256 ?? "")) throw new Error("trusted sha256 pin required");
 let data;
 if (source.startsWith("https://")) {
  const response=await fetch(source,{signal:AbortSignal.timeout(60000)});
  if (!response.ok) throw new Error(`helper download failed: ${response.status}`);
  data=Buffer.from(await response.arrayBuffer());
 } else { data=await readFile(source); }
 const pin={name:kind,path:path.basename(destination),os:goos,arch:goarch,sha256};
 verifyHelper(data,pin,goos,goarch);
 await mkdir(path.dirname(destination),{recursive:true});
 await writeFile(destination,data);
 if (goos!=="windows") await chmod(destination,0o755);
 await writeFile(`${destination}.manifest.json`,JSON.stringify(pin,null,2)+"\n");
 return pin;
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

  const helpers = [];
  for (const kind of ["mosh", "et"]) {
    const helper = helperResourcePath(target.goos, target.goarch, kind);
    if (!existsSync(helper)) {
      throw new Error(`required ${kind} helper missing: ${helper}`);
    }
    const pinPath = `${helper}.manifest.json`;
    const pin = JSON.parse(await readFile(pinPath, "utf8"));
    verifyHelper(await readFile(helper), pin, target.goos, target.goarch);
    const dest = path.join(args.outDir, path.basename(helper));
    await copyFile(helper, dest);
    await copyFile(pinPath, `${dest}.manifest.json`);
    helpers.push({ kind, ...pin, path: path.basename(helper), destination: target.goos === "darwin" ? `Contents/MacOS/${path.basename(helper)}` : path.basename(helper) });
  }
  await writeProtocolResources(args.outDir, target.goos, path.basename(artifact));
  await writeFile(path.join(args.outDir, "installer-resources.json"), JSON.stringify({
    helpers,
    protocolResources: target.goos === "darwin" ? [{ source: "Info.plist", destination: "Contents/Info.plist" }] : target.goos === "linux" ? [{ source: "lemonssh.desktop", destination: "share/applications/lemonssh.desktop" }] : [],
  }, null, 2));

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
  if (process.argv[2] === "--install-helper") {
    const options=Object.fromEntries(Array.from({length:Math.ceil((process.argv.length-2)/2)},(_,index)=>[process.argv[2+index*2]?.replace(/^--/,""),process.argv[3+index*2]]));
    await installHelper({kind:options["install-helper"],source:options.source,sha256:options.sha256,goos:options.goos,goarch:options.goarch,destination:helperResourcePath(options.goos,options.goarch,options["install-helper"])});
  } else { await main(); }
}
