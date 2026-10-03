# Plugin contracts, CLI and SDK

Status: internal (`0.1.0-internal`)

There are two plugin contracts in this repository. They are not compatible and
this document keeps the distinction explicit so nobody mistakes the legacy
JavaScript surface for something the Go host can load.

## Contract v2 — what the Go host actually accepts

`internal/plugin/manifest/v2.go` is the authority. A v2 manifest is a
`lemonssh.plugin.json` document with:

- `apiVersion: 2`;
- `name` (`^[a-z][a-z0-9-]{1,63}$`), semver `version`, non-empty `displayName`;
- `entrypoint: { wasm, sha256, memoryMB? }` — a relative `.wasm` path, a 64 hex
  character checksum, and an optional 16–512 MiB memory cap;
- `permissions`: resource-scoped tuples `{ kind, resource, mode }` with
  `kind` in `filesystem | network | terminal | secret | clipboard | runtime |
  provider`
  (`runtime` gates the WASM host imports: resource `log` write,
  resource `settings` read; `provider` registers terminal/extension
  providers, resource = the contract `ProviderKind`, e.g. `terminal.theme` —
  see [terminal-providers.md](./terminal-providers.md)) and `mode` in
  `read | write`;
- `contributions`: typed identifiers `{ type: command | setting | view, id }`;
- `ui` (optional): the declarative settings/view schema validated by
  `internal/plugin/ui` — see [ui-contributions.md](./ui-contributions.md);
- `minHostVersion` (optional semver floor).

Legacy v1 documents (`main.browser` / `main.node`) are rejected at install time
(`internal/plugin/v1reject`); adding v2 fields to a v1 document does not
upgrade it — the package must be rebuilt for WASM.

The shipped, installable reference package is
`examples/plugins/hello-lemonssh`: an `apiVersion: 2` manifest whose
`hello.wat` entrypoint implements the lemonssh-wasm-abi v1 dispatch channel
(compiled with the offline `wabt` npm package; `build.mjs` regenerates it and
enforces the manifest checksum) plus one declarative text setting.
`npm run check:plugin-contract` installs that exact package through
`PluginService.InstallPackage` and drives `PluginService.CallPlugin` over the
dispatch channel in the Go test suite
(`TestExamplePluginInstallsThroughService`), so the example can never silently
drift out of host compatibility again — the previous check only compared
generated schema artifacts and could not detect a host-level break.

### Tooling

`@lemonssh/plugin-cli` understands both contracts:

- `lemonssh-plugin init <dir> --id <id>` scaffolds a v2 WASM plugin
  (manifest, `plugin.wat` dispatch-ABI source, compiled `plugin.wasm`,
  checksum-verifying `build.mjs`);
- `lemonssh-plugin validate` / `pack` detect `apiVersion: 2` documents and
  enforce the same rules as the Go host (entrypoint suffix and traversal
  checks, checksum shape, memory bounds, permission kinds/modes, contribution
  and UI schema validation);
- `compatibility` checks `minHostVersion` for v2 packages and engine ranges for
  legacy v1 ones.

## Contract v1 — legacy JavaScript, not loadable

`packages/plugin-contract/schema/plugin-contract.schema.json` carries both
generations side by side. The legacy definitions (`manifestVersion: 1`,
browser/node entrypoints, JSON-RPC/stream frames, provider registries)
describe the retired Electron-era JavaScript runtime that no shipped runtime
loads. The `WasmAbi`, `WasmDispatchRequest/Response` and
`WasmHostImportStatus` definitions are the canonical v2 dispatch-channel
contract enforced by the Go host
([isolated-runtime.md](./isolated-runtime.md)); the generator validates their
constants and republishes them as `PLUGIN_WASM_*` limits.

`@lemonssh/plugin-sdk` still targets the legacy JavaScript runtime
(`definePlugin`, `context.providers.register`, …) — that surface is not
loadable by the Go host. Its `WASM_ABI`, `WASM_HOST_IMPORT_STATUS`,
`buildWasmDispatchRequest` and `parseWasmDispatchResponse` exports are the
typed encoding helpers for the v2 WASM dispatch channel, usable by tooling
and test harnesses on either side of the boundary. The provider protocol
constants and decoders (`PROVIDERS_LIST_DISPATCH_METHOD`,
`PROVIDER_INVOKE_DISPATCH_METHOD`,
`PROVIDER_SESSION_EVENT_DISPATCH_METHOD`,
`parsePluginProviderListResult`, `parsePluginProviderInvokeRequest`) document
the provider registry channel the Go host serves — see
[terminal-providers.md](./terminal-providers.md).

## Checks

- `npm run generate:plugin-contract` regenerates the committed TypeScript
  contract and JSON-limit artifacts from the schema.
- `npm run check:plugin-contract` compares those artifacts byte-for-byte and
  then runs the host-level Go test described above.
- `npm run test:plugin-runtime` runs the Go suites under `internal/plugin/...`
  and `cmd/lemonssh/...`, including manifest v2 validation, v1 rejection, the
  permission broker, store durability, WASM instantiation and the native
  containment runtime.
