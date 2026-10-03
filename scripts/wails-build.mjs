#!/usr/bin/env node
// Wails production build for the host platform. Stamps the package.json
// version into the binary so the window title and Health/Version report the
// release, not the skeleton placeholder. Accepts the same optional
// --update-public-key injection as package-wails.mjs so a host build can
// verify signed release manifests; without the flag the ldflags are identical
// to the historical output.
import { spawnSync } from "node:child_process";
import { readFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { pathToFileURL } from "node:url";
import { buildLdflags } from "./package-wails.mjs";

export function hostGoos(platform = process.platform) {
  const goos = { win32: "windows", darwin: "darwin", linux: "linux" }[platform];
  if (!goos) throw new Error(`unsupported host platform ${platform}`);
  return goos;
}

// Same gating as package-wails.mjs windowsGuiLdflags: -H windowsgui is a
// Windows-only linker flag; attaching it on macOS/Linux would emit PE
// binaries the host OS cannot run.
export function windowsGuiLdflags(goos) {
  return goos === "windows" ? " -H windowsgui" : "";
}

export function artifactPath(goos) {
  return `bin/LemonSSH${goos === "windows" ? ".exe" : ""}`;
}

// The exact go build invocation for a host build, split out so tests can pin
// the flag composition (version stamp, optional update public key, windowsgui
// gating) without linking a binary.
export function buildCommand(version, goos, updatePublicKey = "") {
  const output = artifactPath(goos);
  return `go build -trimpath "-ldflags=${buildLdflags(version, updatePublicKey)}${windowsGuiLdflags(goos)}" -o ${output} ./cmd/lemonssh`;
}

function parseArgs(argv) {
  const args = { updatePublicKey: "" };
  for (let index = 0; index < argv.length; index++) {
    const arg = argv[index];
    if (arg === "--update-public-key") args.updatePublicKey = argv[++index] ?? "";
    else throw new Error(`unknown argument ${arg}`);
  }
  return args;
}

function run(command) {
  // npm is npm.cmd on Windows; shell keeps resolution and quoting uniform.
  const result = spawnSync(command, { stdio: "inherit", shell: true });
  if (result.status !== 0 || result.error) {
    throw new Error(`${command} failed with status ${result.status}`);
  }
}

export async function wailsBuild({ version, goos = hostGoos(), updatePublicKey = "" } = {}) {
  const output = artifactPath(goos);
  run("npm run build:native-tools");
  run("npm run build");
  run("node scripts/wails-prepare-frontend.mjs");
  run(buildCommand(version, goos, updatePublicKey));
  console.log(`[wails-build] built ${output} (version ${version})`);
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  const pkg = JSON.parse(await readFile("package.json", "utf8"));
  const version = String(pkg.version ?? "0.0.0").replace(/"/g, "");
  const args = parseArgs(process.argv.slice(2));
  await wailsBuild({ version, updatePublicKey: args.updatePublicKey });
}
