import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdir, readdir, stat, writeFile } from "node:fs/promises";
import path from "node:path";

import wabtInit from "wabt";

import {
  buildPluginPackage,
  validatePluginDirectory,
  validatePluginPackage,
} from "./archive.js";
import { readAndValidateManifest, manifestIdentity } from "./manifest.js";

export interface InitPluginOptions {
  readonly id: string;
  readonly name?: string;
}

// The scaffolded entrypoint implements the lemonssh-wasm-abi v1 dispatch
// channel the Go host (internal/plugin/wasm) speaks: lemonssh_alloc /
// lemonssh_free manage linear memory for the host-written request envelope
// and lemonssh_dispatch answers every method with a fixed pong envelope
// behind a [u32 LE length] prefix. hello-lemonssh (examples/plugins) shows a
// full method dispatcher with broker-gated host imports. Compiling the WAT
// below with wabt keeps the scaffold deterministic and offline.
const SCAFFOLD_WAT = String.raw`(module
  ;; lemonssh-wasm-abi v1: alloc/free/dispatch exports, no host imports yet.
  ;; Add  (import "lemonssh" "lemonssh_host_log" (func (param i32 i32 i32) (result i32)))
  ;; and friends once the manifest declares the matching runtime permissions.
  (memory (export "memory") 1 4)
  (global $heap (mut i32) (i32.const 4096))

  ;; [u32 LE 27]{"ok":true,"result":"pong"}
  (data (i32.const 64) "\1b\00\00\00{\"ok\":true,\"result\":\"pong\"}")

  (func $ensure_capacity (param $need i32) (result i32)
    (if (result i32)
      (i32.gt_u (local.get $need) (i32.mul (memory.size) (i32.const 65536)))
      (then
        (i32.ne
          (memory.grow
            (i32.add
              (i32.div_u
                (i32.sub (local.get $need) (i32.mul (memory.size) (i32.const 65536)))
                (i32.const 65536))
              (i32.const 1)))
          (i32.const -1)))
      (else (i32.const 1))))

  (func $alloc (param $size i32) (result i32)
    (local $ptr i32)
    (local.set $ptr
      (i32.and (i32.add (global.get $heap) (i32.const 7)) (i32.const -8)))
    (if
      (call $ensure_capacity (i32.add (local.get $ptr) (local.get $size)))
      (then (global.set $heap (i32.add (local.get $ptr) (local.get $size))))
      (else (local.set $ptr (i32.const 0))))
    (local.get $ptr))

  (func $free (param $ptr i32) (param $len i32))

  ;; The scaffold answers every method with pong; parse the request envelope
  ;; {"method":"...","payload":...} and branch on $method to do real work.
  (func $dispatch (param $reqPtr i32) (param $reqLen i32) (result i32)
    (i32.const 64))

  (func $start)

  (export "lemonssh_alloc" (func $alloc))
  (export "lemonssh_free" (func $free))
  (export "lemonssh_dispatch" (func $dispatch))
  (export "_start" (func $start))
)
`;

let scaffoldWasmCache: Uint8Array | undefined;

async function compileScaffoldWasm(): Promise<Uint8Array> {
  if (scaffoldWasmCache) return scaffoldWasmCache;
  const wabt = await wabtInit();
  const parsed = wabt.parseWat("plugin.wat", SCAFFOLD_WAT, {});
  const binary = parsed.toBinary({ log: false, write_debug_names: false });
  scaffoldWasmCache = new Uint8Array(binary.buffer);
  return scaffoldWasmCache;
}

export async function initPlugin(
  targetDirectory: string,
  options: InitPluginOptions,
): Promise<string> {
  const directory = path.resolve(targetDirectory);
  await mkdir(directory, { recursive: true });
  const existingEntries = await readdir(directory);
  if (existingEntries.length > 0) {
    throw new Error(`Target directory is not empty: ${directory}`);
  }
  const displayName = options.name?.trim() || options.id.split(".").at(-1) || options.id;
  // Manifest v2 (Go host) names are package identifiers without dots.
  const pluginName = options.id.toLowerCase().replaceAll(/[^a-z0-9-]+/g, "-")
    .replaceAll(/^-+|-+$/g, "");
  if (!/^[a-z][a-z0-9-]{1,63}$/.test(pluginName)) {
    throw new Error(`Plugin id ${options.id} cannot be turned into a manifest v2 name`);
  }
  const scaffoldWasm = await compileScaffoldWasm();
  const entrypointSha256 = createHash("sha256").update(scaffoldWasm).digest("hex");
  const manifest = {
    apiVersion: 2,
    name: pluginName,
    version: "0.1.0",
    displayName,
    description: "A LemonSSH plugin",
    entrypoint: {
      wasm: "plugin.wasm",
      sha256: entrypointSha256,
      memoryMB: 16,
    },
    permissions: [],
    ui: {
      settings: [
        {
          id: `${options.id}.greeting`,
          type: "text",
          label: "Greeting",
          description: "Greeting text managed by the host settings page.",
          default: `Hello from ${displayName}`,
        },
      ],
    },
  };
  await Promise.all([
    writeFile(
      path.join(directory, "lemonssh.plugin.json"),
      `${JSON.stringify(manifest, null, 2)}\n`,
      "utf8",
    ),
    writeFile(path.join(directory, "plugin.wat"), SCAFFOLD_WAT, "utf8"),
    writeFile(path.join(directory, "plugin.wasm"), scaffoldWasm),
    writeFile(
      path.join(directory, "build.mjs"),
      [
        `// Compiles plugin.wat with wabt and verifies the manifest checksum stays in sync.`,
        `import { createHash } from "node:crypto";`,
        `import { readFile, writeFile } from "node:fs/promises";`,
        `import wabtInit from "wabt";`,
        ``,
        `const wabt = await wabtInit();`,
        `const wat = await readFile("plugin.wat", "utf8");`,
        `const binary = wabt.parseWat("plugin.wat", wat, {}).toBinary({ log: false, write_debug_names: false });`,
        `const wasm = new Uint8Array(binary.buffer);`,
        `await writeFile("plugin.wasm", wasm);`,
        `const sha256 = createHash("sha256").update(wasm).digest("hex");`,
        `const manifest = JSON.parse(await readFile("lemonssh.plugin.json", "utf8"));`,
        `if (manifest.entrypoint?.sha256 !== sha256) {`,
        `  console.error(\`Stale manifest: expected entrypoint.sha256 \${sha256}\`);`,
        `  process.exit(1);`,
        `}`,
        ``,
      ].join("\n"),
      "utf8",
    ),
    writeFile(
      path.join(directory, "package.json"),
      `${JSON.stringify({
        name: options.id.replaceAll(".", "-"),
        version: "0.1.0",
        private: true,
        type: "module",
        scripts: { build: "node build.mjs" },
        devDependencies: { wabt: "^1.0.39" },
      }, null, 2)}\n`,
      "utf8",
    ),
    writeFile(
      path.join(directory, "README.md"),
      [
        `# ${displayName}`,
        ``,
        `A LemonSSH plugin scaffold for the manifest v2 / WASM runtime.`,
        ``,
        "- `lemonssh.plugin.json` declares the plugin (apiVersion 2) and its declarative settings UI.",
        "- `plugin.wat` is the sandboxed entrypoint source the Go host instantiates (wazero, WASI disabled).",
        "- `plugin.wasm` implements the lemonssh-wasm-abi v1 dispatch channel:",
        "  `lemonssh_alloc` / `lemonssh_free` manage the request buffer the host writes and",
        "  `lemonssh_dispatch` answers one JSON envelope per call (the scaffold always replies pong).",
        "- `npm install && npm run build` recompiles `plugin.wat` and verifies the manifest checksum.",
        ``,
        "Host imports (`lemonssh_host_log`, `lemonssh_host_setting_get`) are available once the",
        "manifest declares the matching `runtime` permissions; every import is broker-gated and",
        "returns a structured status code instead of trapping when a grant is missing. See",
        "`docs/plugin-platform/isolated-runtime.md` and `examples/plugins/hello-lemonssh`.",
        ``,
        "Package it with `npm exec -- lemonssh-plugin pack <this directory>` and install the",
        "resulting `.ncpkg` from LemonSSH's Settings → Plugins page.",
        ``,
      ].join("\n"),
      "utf8",
    ),
  ]);
  await readAndValidateManifest(directory);
  return directory;
}

export async function validateTarget(target: string) {
  const resolved = path.resolve(target);
  const targetStats = await stat(resolved);
  if (targetStats.isDirectory()) {
    const result = await validatePluginDirectory(resolved);
    return { kind: "directory" as const, ...result };
  }
  if (targetStats.isFile() && resolved.endsWith(".ncpkg")) {
    const result = await validatePluginPackage(resolved);
    return { kind: "package" as const, ...result };
  }
  throw new Error("Validation target must be a plugin directory or .ncpkg file");
}

export async function buildPlugin(pluginDirectory: string): Promise<void> {
  const directory = path.resolve(pluginDirectory);
  await readAndValidateManifest(directory);
  const npmCommand = process.platform === "win32" ? "npm.cmd" : "npm";
  await new Promise<void>((resolve, reject) => {
    const child = spawn(npmCommand, ["run", "build", "--if-present"], {
      cwd: directory,
      env: process.env,
      shell: false,
      stdio: "inherit",
      windowsHide: true,
    });
    child.once("error", reject);
    child.once("exit", (code, signal) => {
      if (code === 0) resolve();
      else reject(new Error(`Plugin build failed (${signal ?? `exit ${String(code)}`})`));
    });
  });
  await validatePluginDirectory(directory);
}

export async function packPlugin(pluginDirectory: string, outputPath?: string) {
  const directory = path.resolve(pluginDirectory);
  const manifest = await readAndValidateManifest(directory);
  const identity = manifestIdentity(manifest);
  const resolvedOutput = outputPath
    ? path.resolve(outputPath)
    : path.join(path.dirname(directory), `${identity.id}-${identity.version}.ncpkg`);
  return buildPluginPackage(directory, resolvedOutput);
}
