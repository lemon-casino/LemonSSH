# Isolated plugin host runtime (Go / Wails)

Status: implemented for the manifest v2 declarative surface, the
lemonssh-wasm-abi v1 dispatch channel and the provider registry served over
that channel (see terminal-providers.md). Declarative `ui.views` render
host-side for the `settings` location — see ui-contributions.md.

The plugin host lives in the Go backend, not in the renderer and not in a
JavaScript main process. LemonSSH is a Wails v3 desktop application: the
frontend talks to the plugin host only through the generated Wails bindings for
`PluginService` (`cmd/lemonssh/pluginService.go`), surfaced in the renderer by
the typed bridge in `infrastructure/runtime/wails/pluginBridge.ts`. There is no
`LEMONSSH_PLUGIN_DEV` gate, no Electron BrowserWindow/utilityProcess runtime,
no `node:sqlite` database and no Electron `safeStorage`; those belonged to the
retired Electron shell.

## Where the host lives

- `internal/plugin/manifest` — manifest v2 parsing/validation. `apiVersion`
  must be `2`, the entrypoint must be an `entrypoint.wasm` with a matching
  `entrypoint.sha256` (64 hex chars), and the memory cap is optional
  (`entrypoint.memoryMB`, 16–512 MiB).
- `internal/plugin/v1reject` — legacy v1 packages (`main.browser` /
  `main.node`) are rejected at install time with
  `plugin v1 packages are not supported by this runtime`. There is no shim, no
  adapter and no fallback.
- `internal/plugin/store` — the installed inventory. An atomic JSON snapshot
  (temp file + rename, `.bak` kept) plus a `packages/` directory holding the
  exact installed `.ncpkg` bytes. Two-phase installs (`StageInstall` /
  `CommitStaged`) never run while staged; `RecoverStaged` drops unpublished
  stages at startup.
- `internal/plugin/wasm` — the WASM runtime, built on wazero. WASI is disabled;
  modules compile, instantiate and run `_start` inside a sandbox with
  `WithCloseOnContextDone(true)`. A plugin whose manifest declares
  `entrypoint.memoryMB` gets its own wazero runtime with
  `WithMemoryLimitPages` applied. The runtime also owns the
  lemonssh-wasm-abi v1 dispatch channel described below, including the
  broker-gated `lemonssh` host module.
- `internal/plugin/native` — hash-pinned, broker-authorized supervised native
  companion processes (process-group/job-object containment, length-prefixed
  framed RPC, stdout flood bounds, quarantine on containment failure). Native
  plugin code never executes inside the Go host process.
- `internal/plugin/permissions` — the permission broker (see
  [security-and-permissions.md](./security-and-permissions.md)).
- `internal/plugin/ui` — declarative UI schema validation (see
  [ui-contributions.md](./ui-contributions.md)).
- `internal/plugin/host` — connects installed manifests to the broker: the
  gate for settings, declarative UI and permission grants.

## Inventory location

`PluginService` is created in `cmd/lemonssh/main.go` with its inventory at
`<profile directory>/plugins/inventory.json`; installed archives are copied to
`<profile directory>/plugins/packages/<pluginID>-<version>.ncpkg`. The
inventory records plugin ID, version, state (`installed` / `enabled` /
`disabled` / `staged`), the archive SHA-256 and the validated manifest
snapshot, plus labels such as `packagePath`.

## Installation transaction

`PluginService.InstallPackage(archivePath, options)` performs these steps:

1. open the `.ncpkg` (ZIP) and reject archives with more than 512 entries,
   unsafe paths (`..`, absolute, backslashes) and single files over 64 MiB;
2. read the manifest (`lemonssh.plugin.json`, the pre-rename
   `netcatty.plugin.json`, or `manifest.json`), reject v1 documents via
   `v1reject`, then parse and fully validate the v2 manifest;
3. locate `entrypoint.wasm` inside the archive and verify its SHA-256 against
   `entrypoint.sha256`;
4. register the plugin in the inventory with the archive digest and validated
   manifest snapshot, copy the exact `.ncpkg` bytes into `packages/`, and roll
   the install back if any write fails;
5. when `options.enable` is set, instantiate the WASM module in the sandbox
   (memory capped from the manifest) and flip the state to `enabled`; a failed
   instantiation rolls the install back.

On service startup, `restoreEnabledPackages` re-reads every enabled plugin's
stored package, re-validates it and re-instantiates it; a package that no
longer parses is disabled rather than executed.

## Lifecycle

- `SetEnabled(id, true)` reads the stored package, re-instantiates WASM and
  enables the record; `SetEnabled(id, false)` closes the WASM module and
  revokes all broker grants for that plugin.
- `Restart(id)` stops any native process, closes the WASM module and re-runs
  the enable path for enabled plugins.
- `Uninstall(id)` stops native processes, closes the WASM module, revokes
  grants, deletes the inventory record and removes the stored `.ncpkg`.

The renderer plugin manager (Settings → Plugins → Installed plugins) calls
`InstallPackage`, `SetEnabled`, `Restart` and `Uninstall` through the Wails
bindings via `useInstalledPlugins`; there is no direct renderer access to the
inventory.

## WASM dispatch channel (lemonssh-wasm-abi v1)

`internal/plugin/wasm` implements the only RPC path into a WASM module. The
canonical names, byte budgets and status codes are pinned as the `WasmAbi`,
`WasmDispatch*` and `WasmHostImportStatus` definitions of
`packages/plugin-contract` (surfaced again as `WASM_ABI` /
`WASM_HOST_IMPORT_STATUS` in `@lemonssh/plugin-sdk`); the Go constants in
`internal/plugin/wasm` must stay in sync.

**Guest exports.** A module that wants calls must export:

| export | signature | semantics |
| --- | --- | --- |
| `lemonssh_alloc` | `(size i32) -> ptr i32` | reserve `size` bytes of linear memory; `0` on failure |
| `lemonssh_free` | `(ptr i32, len i32)` | release a region from `alloc` or `dispatch` (bump allocators may no-op) |
| `lemonssh_dispatch` | `(reqPtr i32, reqLen i32) -> respPtr i32` | handle one request; return a guest-allocated `[u32 LE length][payload]` region, or `0` on failure |

The host writes the request envelope into the `alloc` region, copies the
response payload out, then calls `free` on both regions under the same
deadline. Modules without the full ABI instantiate fine and report
`ErrNoDispatchABI` on dispatch attempts.

**Envelopes.** Request: `{"method": string, "payload"?: JsonValue}`. Response:
`{"ok": true, "result"?: JsonValue}` or
`{"ok": false, "error": {"code", "message", "data"?}}` where `code` uses the
canonical `PluginErrorName` vocabulary. Both sides are capped at 1 MiB
(`RpcLimits.maxJsonBytes`).

**Host imports.** The host module `lemonssh` provides two functions; every
call is broker-gated with the plugin's manifest-declared `runtime`
permissions (`{"kind":"runtime","resource":"log","mode":"write"}` and
`{"kind":"runtime","resource":"settings","mode":"read"}`) and returns a
structured i32 status — denial is a value (`-1`), never a trap, panic or
process abort:

For compatibility with plugins packaged before the brand rename, the host
additionally registers the same two functions under the legacy host module
name `netcatty`, and guest export resolution falls back from the
`lemonssh_*` names to the legacy `netcatty_*` names when the new exports are
absent. New plugins must target the `lemonssh` names above.

| import | signature | gate | return |
| --- | --- | --- | --- |
| `lemonssh_host_log` | `(level i32, ptr i32, len i32) -> i32` | `runtime`/`log` write | `0` ok; level 0–3 (debug…error); lines are truncated at 8 KiB and kept in a 64-entry per-plugin ring (`Runtime.RecentLogs`) |
| `lemonssh_host_setting_get` | `(keyPtr, keyLen, bufPtr, bufCap i32) -> i32` | `runtime`/`settings` read | positive `ret` ≤ `bufCap`: value written; `ret > bufCap`: needs `ret` bytes, wrote nothing; `-1` denied, `-2` invalid/unknown key, `-3` unavailable |

`lemonssh_host_setting_get` reads only the plugin's own declared non-secret
settings — the exact view `PluginService.Settings` exposes; password settings
and other plugins' data are unreachable. Grants are recorded only through the
trusted `PluginService.GrantPermission` path (once/session lifetimes) after
`host.Host` matches the manifest declaration; there is no plugin-callable
grant path.

**Service entrypoint.** `PluginService.CallPlugin(pluginID, method,
payloadJSON)` fails closed unless the plugin is installed and enabled, then
forwards to `wasm.Runtime.Dispatch`. Transport failures (unknown/disabled
plugin, missing ABI, oversized request/response, allocation exhaustion,
guest trap, timeout) return a Go error; plugin-declared failures arrive
in-band as `result.error`. Each dispatch is serialized per module (guest
allocators are not re-entrant), bounded by a 10 s default deadline
(`WithCloseOnContextDone` aborts the guest), runs behind a panic-recovery
guard, and operates inside the manifest-capped linear memory.

**Offline fixtures and examples.** The Go tests assemble minimal dispatch
modules by hand (`internal/plugin/wasm/wasm_test.go`) so the suite never
needs a toolchain or network. `examples/plugins/hello-lemonssh` ships a
readable `hello.wat` compiled with the offline `wabt` npm package at build
time; it answers `ping -> {"ok":true,"result":{"pong":true,"greeting":…}}`
reading its own greeting setting (empty until the runtime/settings grant
exists), `command.execute` for its declared `hello.ping` command, the
provider registry methods (`providers.list`, `provider.invoke`,
`provider.sessionEvent`) for its `terminal.theme` provider, and logs one
best-effort info line per dispatch. `lemonssh-plugin init` scaffolds the same
ABI with a static pong.

## Not implemented

- Declarative `ui.views` entries with a location other than `settings` have no
  hosting runtime (the schema rejects them today); renderer requests to open
  such a view fail with a visible error instead of silently doing nothing.
  `settings` views render host-side through the plugin bridge — see
  ui-contributions.md.
- Provider families beyond the broker-gated registry (see
  terminal-providers.md) — session snapshot delivery is live and the
  extension data plane (connection/importer/authentication/sync) is served
  through the same dispatch channel, but privileged
  `terminal.interceptor.*` providers stay unimplemented.

This surface is contract-visible: `check:plugin-contract` installs the shipped
`examples/plugins/hello-lemonssh` package through
`PluginService.InstallPackage` and drives `PluginService.CallPlugin` over the
dispatch channel (see `TestExamplePluginInstallsThroughService` and
`TestCallPluginFailsClosed` in `cmd/lemonssh/pluginService_test.go`), so a
drift between the published contract and the host fails CI.
