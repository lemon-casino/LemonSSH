# Hello LemonSSH

This package is the runnable internal example for the **manifest v2 / WASM**
plugin platform that ships in the Go host (`internal/plugin`). It exercises
the surface the host implements today: a validated `apiVersion: 2` manifest, a
sandboxed WASM entrypoint (wazero, WASI disabled, memory capped by
`entrypoint.memoryMB`) speaking the **lemonssh-wasm-abi v1 dispatch channel**
(canonical `lemonssh` module and `lemonssh_*` exports; the Go host keeps the
legacy `netcatty` module and `netcatty_*` export resolution for pre-rename
binaries),
a declarative settings UI rendered by the host from the `ui` schema, and
declarative UI contributions (`ui.views` card bound to the greeting setting,
a `commandPalette` menu and a `ctrl+alt+h` keybinding for the declared
`hello.ping` command — see `docs/plugin-platform/ui-contributions.md`).

Legacy v1 packages (`main.browser` / `main.node`) are rejected at install time
— rebuild for WASM instead of expecting a shim.

## What the dispatch demo shows

The host writes a JSON request envelope `{"method":"<name>","payload":…}` into
the module's linear memory and calls `lemonssh_dispatch`. `hello.wat` (the
readable source of truth; compiled with the offline `wabt` npm package):

- `lemonssh_alloc` / `lemonssh_free` — 8-byte aligned bump allocator backing
  the request buffer and the `[u32 LE length]`-prefixed response region;
- `lemonssh_dispatch` — logs one best-effort info line, then routes by
  method:
  - `ping` → `{"ok":true,"result":{"pong":true,"greeting":"<value>"}}`;
  - `command.execute` for its declared `hello.ping` command →
    `{"ok":true,"result":{"command":"hello.ping","handled":true}}` (any other
    command gets the structured `not_found` envelope — the renderer surfaces
    it as `Plugin command … failed: unknown method`);
  - `providers.list` → one `terminal.theme` provider declaration
    (`com.lemonssh.hello.accent`, label `Hello Accent`);
  - `provider.invoke` → `{"ok":true,"result":{"colors":{"cursor":"#34d399"}}}`
    (the theme colors the terminal merges over its base theme);
  - `provider.sessionEvent` → a minimal `{"ok":true}` ack;
  - anything else → `{"ok":false,"error":{"code":"not_found",…}}`;
- `lemonssh_host_log` / `lemonssh_host_setting_get` — the only host imports,
  both **broker-gated**: without the manifest-declared `runtime` grants the
  log line is dropped and the greeting degrades to `""`, while the call still
  succeeds. Grants are recorded only through the trusted
  `PluginService.GrantPermission(pluginID, "runtime", "log"|"settings", …)`
  path. The provider declaration surfaces only after the same trusted path
  grants the manifest-declared
  `GrantPermission("hello-lemonssh", "provider", "terminal.theme", "read", "session")`
  — until then the host never even asks the plugin for providers.

`TestExamplePluginInstallsThroughService` in `cmd/lemonssh/pluginService_test.go`
drives the install → enable → `CallPlugin("ping")` → grant → greeting flow;
`TestPluginCommandExecutesOverWasmDispatch` and `TestTerminalProviderLifecycle`
in `cmd/lemonssh/pluginProviders_test.go` drive the command channel and the
provider lifecycle (enumerate → grant → provide → session event → disable) on
this exact package, so the example, the host and the published contract cannot
drift apart.

## Build and validate

From the repository root:

```bash
npm run build:plugin-packages
npm exec -- lemonssh-plugin validate examples/plugins/hello-lemonssh
npm exec -- lemonssh-plugin pack examples/plugins/hello-lemonssh --out hello-lemonssh.ncpkg
```

`npm run build` inside this directory recompiles `hello.wat` with wabt and
fails if `lemonssh.plugin.json` no longer matches its checksum.

## Install and try it

Open **Settings → Plugins → Installed plugins** and install the packaged
`hello-lemonssh.ncpkg`. The plugin is enabled on install; its **Greeting**
setting appears under **Settings → Plugins** and is stored by the Go host
(`PluginService.SetSetting`). `Restart` re-instantiates the WASM module in the
sandbox; `Uninstall` removes the stored package and its settings. Once the
host UI calls `GrantPermission("hello-lemonssh", "runtime", "settings",
"read", "session")`, dispatches return the configured greeting.

## Scope

`packages/plugin-sdk`'s plugin classes still target the legacy JavaScript
runtime and are not loadable by this host; its `WASM_ABI` /
`parseWasmDispatchResponse` helpers describe this channel's encoding, and
`buildPluginViewDataRequest` / `parsePluginViewDataResult` document the
canonical `view.data` binding pull. See `docs/plugin-platform/` for the
implemented boundary and open gaps.
