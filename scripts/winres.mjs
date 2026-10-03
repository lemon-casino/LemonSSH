#!/usr/bin/env node
// Regenerates the Windows version resource (cmd/lemonssh/rsrc_windows_amd64.syso)
// with the version taken from package.json so file properties stop reporting
// 0.0.0. Uses the same pinned Wails CLI as the previous wails:winres one-liner,
// keeping the app icon and the PerMonitorV2 DPI manifest intact.
import { spawnSync } from "node:child_process";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import process from "node:process";
import { pathToFileURL } from "node:url";
import { lemonsshWinresInfo } from "./windows-version-info.mjs";

export const WAILS_SYSO_TOOL = "github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.12";
export const DEFAULT_ARCH = "amd64";
export const DEFAULT_ICON = "build/lemonssh.ico";
export const DEFAULT_MANIFEST = "build/windows/lemonssh.manifest";

export function defaultSysoPath(arch = DEFAULT_ARCH) {
  return path.join("cmd", "lemonssh", `rsrc_windows_${arch}.syso`);
}

export function parseWinresArgs(argv) {
  const args = { arch: DEFAULT_ARCH };
  for (let index = 0; index < argv.length; index++) {
    const arg = argv[index];
    if (arg === "--version") args.version = argv[++index];
    else if (arg === "--arch") args.arch = argv[++index];
    else if (arg === "--out") args.out = argv[++index];
    else if (arg === "--icon") args.icon = argv[++index];
    else if (arg === "--manifest") args.manifest = argv[++index];
    else throw new Error(`unknown argument ${arg}`);
  }
  return args;
}

export function buildSysoCommand({ tool = WAILS_SYSO_TOOL, arch, icon, manifest, info, out }) {
  return `go run ${tool} generate syso -arch=${arch} -icon="${icon}" -manifest="${manifest}" -info="${info}" -out="${out}"`;
}

function runShell(command) {
  const result = spawnSync(command, { stdio: "inherit", shell: true });
  if (result.status !== 0 || result.error) {
    throw new Error(`windows version resource generation failed with status ${result.status}`);
  }
}

// generateWindowsSyso writes the stamped winres info.json to a temporary
// directory and invokes the pinned Wails syso generator. `run` is injectable
// for tests; it receives the full shell command line.
export async function generateWindowsSyso({
  version,
  arch = DEFAULT_ARCH,
  out = defaultSysoPath(arch),
  icon = DEFAULT_ICON,
  manifest = DEFAULT_MANIFEST,
  run = runShell,
  tempRoot = tmpdir(),
}) {
  if (!version) throw new Error("version is required");
  const infoJson = `${JSON.stringify(lemonsshWinresInfo(version), null, 2)}\n`;
  const tempDir = await mkdtemp(path.join(tempRoot, "lemonssh-winres-"));
  try {
    const info = path.join(tempDir, "info.json");
    await writeFile(info, infoJson, "utf8");
    await run(buildSysoCommand({ arch, icon, manifest, info, out }));
  } finally {
    await rm(tempDir, { recursive: true, force: true });
  }
  return { out, arch, version };
}

async function main(argv = process.argv.slice(2)) {
  const args = parseWinresArgs(argv);
  const pkg = JSON.parse(await readFile("package.json", "utf8"));
  const version = args.version ?? pkg.version;
  if (!args.arch) throw new Error("--arch requires a value");
  await generateWindowsSyso({
    version,
    arch: args.arch,
    out: args.out,
    icon: args.icon,
    manifest: args.manifest,
  });
  console.log(`[winres] stamped ${path.basename(defaultSysoPath(args.arch))} with version ${version}`);
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  await main();
}
