// Builds the hello-lemonssh WASM entrypoint and verifies the manifest stays
// in sync with it. The Go host (internal/plugin/wasm) instantiates the module
// with wazero: WASI is disabled; the module implements the lemonssh-wasm-abi
// v1 dispatch channel (lemonssh_alloc / lemonssh_dispatch / lemonssh_free)
// with broker-gated lemonssh_host_* imports — see hello.wat and
// docs/plugin-platform/isolated-runtime.md.
//
// hello.wat is compiled with the offline wabt (WebAssembly Binary Toolkit)
// npm package at build time; no network access happens during the build.
import console from "node:console";
import { createHash } from "node:crypto";
import { readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";
import wabtInit from "wabt";

const root = path.dirname(fileURLToPath(import.meta.url));

const watPath = path.join(root, "hello.wat");
const wat = await readFile(watPath, "utf8");
const wabt = await wabtInit();
const binary = wabt.parseWat("hello.wat", wat, {}).toBinary({
  log: false,
  write_debug_names: false,
});
const wasm = new Uint8Array(binary.buffer);

const target = path.join(root, "hello.wasm");
await writeFile(target, wasm);
const sha256 = createHash("sha256").update(wasm).digest("hex");

const manifestPath = path.join(root, "lemonssh.plugin.json");
const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
if (manifest.apiVersion !== 2) {
  console.error("hello-lemonssh: manifest must declare apiVersion 2 for the Go host");
  process.exitCode = 1;
} else if (manifest.entrypoint?.wasm !== "hello.wasm" || manifest.entrypoint?.sha256 !== sha256) {
  console.error(
    `hello-lemonssh: lemonssh.plugin.json entrypoint.sha256 is stale; expected ${sha256}`,
  );
  process.exitCode = 1;
} else {
  console.log(`hello.wasm written (${wasm.byteLength} bytes, sha256 ${sha256})`);
}
