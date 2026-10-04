# Plugin host extension boundaries (Go / Wails)

Status: architecture notes for the shipped Go host.

This document records the layering that future plugin-platform work must keep.
Everything here describes code that exists in the tree today; it replaces the
Electron-era boundary notes about main-process registries and renderer
bridges.

## Package boundaries

Dependency direction is one-way, matching the repository-wide rule in
`AGENTS.md`:

```
cmd/lemonssh (Wails services)
    └─ internal/plugin/host        glue: manifests → broker/UI/store
         ├─ internal/plugin/manifest   v2 validation (rejects v1 via v1reject)
         ├─ internal/plugin/ui         declarative settings/view schema
         ├─ internal/plugin/permissions fail-closed capability broker
         ├─ internal/plugin/store      durable inventory (atomic JSON snapshot)
         ├─ internal/plugin/wasm       wazero sandbox (WASI off, memory caps)
         └─ internal/plugin/native     contained companion processes
```

Rules that keep the seams stable:

- `internal/plugin/*` packages never import `cmd/lemonssh` or Wails. The Wails
  surface is a thin adapter (`PluginService`) that constructs the store,
  runtimes and broker and delegates everything else.
- Frontend code reaches the host only through
  `infrastructure/runtime/wails/pluginBridge.ts` and the generated bindings.
  Components never call Wails directly; application hooks
  (`useInstalledPlugins`, `usePluginContributions`, `useDeclarativePlugins`)
  are the only callers.
- The manifest is the contract: an invalid v2 document never enters the store,
  so the runtimes and broker can trust the stored snapshot without re-parsing
  user input. `restoreEnabledPackages` re-validates on startup and disables a
  package that no longer parses instead of executing it.

## Extension points for future phases

- **Plugin ABI**: the lemonssh-wasm-abi v1 dispatch channel
  (`internal/plugin/wasm`) and the framed native companion RPC
  (`internal/plugin/native`) are the two shipped extension surfaces. New
  dispatch methods (today `command.execute`, `view.data` and the provider
  registry triple) attach in the guests and hosts that speak the channel, and
  any new host import must route every privileged call through
  `internal/plugin/permissions`, backed by a manifest-declared permission —
  do not add capability bypasses (direct filesystem/env access from runtimes)
  to "make plugins useful".
- **Providers** (terminal, connection, authentication, importer, sync):
  implemented in Go (`internal/plugin/providers`) behind
  `PluginService.TerminalProviders` / `ProvideTerminal` /
  `CancelTerminalRequest` / `PublishTerminalSessionEvent` /
  `ExtensionProviders`, with the broker gating every enumeration and
  invocation — see terminal-providers.md. Future provider kinds follow the
  same shape: declarations validated against the manifest and served through
  the registry, never renderer-side process spawning.
- **Plugin views**: `ui.views` (`list`/`card`) are validated today; hosting a
  view means rendering host-managed documents from validated data. Arbitrary
  plugin HTML/JS must never be loaded into the renderer; if a rich view runtime
  is ever added it needs its own isolated surface and threat-model review.

## CI contract

- `npm run test:plugin-runtime` — Go suites for every `internal/plugin`
  package plus the Wails service layer.
- `npm run check:plugin-contract` — generated schema artifacts plus the
  host-level install test against `examples/plugins/hello-lemonssh`.
- `go test ./cmd/lemonssh -run TestEveryAdvertisedNativeToolHasAllRPCSurfaces`
  must pass whenever the capability catalog changes (see `AGENTS.md`).
