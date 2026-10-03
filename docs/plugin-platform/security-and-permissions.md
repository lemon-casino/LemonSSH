# Plugin security and permission boundary

Status: implemented for the declarative/native surfaces of the Go host and
for the provider registry; provider-era capability brokers beyond that
(network/filesystem RPC for sandboxed plugins) do not exist yet.

## Authority model

The broker is `internal/plugin/permissions` (`permissions.Broker`). Grants are
keyed by plugin ID plus an opaque capability key; every check is fail-closed:

- `Grant(id, key, lifetime, ttl)` records a grant with lifetime `once` or
  `session`. `once` grants are consumed by a single `Check`.
- `Check(id, key, mode)` denies anything that was not granted, and revokes on
  plugin disable/uninstall (`RevokeAll`).

Nothing in the renderer or in plugin data can create a capability key: keys are
derived from the validated manifest by `internal/plugin/host`:

```go
// internal/plugin/host — resource() refuses anything the manifest does not declare
for _, p := range m.Permissions {
    if p.Kind == kind && p.Resource == resource && p.Mode == mode { ... }
}
return "", fmt.Errorf("%w: permission is not declared by plugin", permissions.ErrNotGranted)
```

`Host.Grant` / `Host.Authorize` therefore cannot widen a grant beyond the
installed `apiVersion: 2` manifest, and disabling or uninstalling a plugin
drops its grants immediately (`PluginService.SetEnabled` / `Uninstall`).

`PluginService.GrantPermission` / `AuthorizePermission` are trusted host UI
entrypoints exposed over Wails for the declarative settings approval flow; they
are never callable by plugin code. Native companions additionally require a
session-scoped `companion.execute:write` grant (`PluginService.GrantNative`)
before `StartNative` will authorize a spawn.

## Provider authorization

The provider permission kind (`{"kind": "provider", "resource": "<ProviderKind>",
"mode": "read"}`) gates the terminal/extension provider registry
(`internal/plugin/providers`, see
[terminal-providers.md](./terminal-providers.md)). A plugin's declaration is
enumerated or invoked only while the broker holds the manifest-derived
`["provider","<kind>"]:read` grant; the registry re-checks the broker on every
call, so granting, revoking or disabling takes effect immediately and a
revoked plugin disappears from every provider surface.

## Secret settings

Declarative settings with `type: "password"` are sealed with the OS keyring
through `internal/platform/credentials` (`zalando/go-keyring`); the sealed blob
is stored in the plugin inventory as `{"sealed": "<base64>"}`. There is no
Electron `safeStorage` and no plaintext fallback: when the keyring is
unavailable, `Host.SetSetting` fails with
`secret settings require a credential provider; plaintext storage is disabled`.
Non-secret settings are validated against the declared type (`text`, `number`,
`boolean`, `select` options) before storage, and unknown setting IDs are
rejected.

## WASM and native containment

- WASM plugins run in wazero with WASI disabled and
  `WithCloseOnContextDone(true)`; the manifest's `entrypoint.memoryMB` becomes
  a hard linear-memory cap. The only RPC surface into WASM is the
  lemonssh-wasm-abi v1 dispatch channel
  ([isolated-runtime.md](./isolated-runtime.md)): every host import
  (`lemonssh_host_log`, `lemonssh_host_setting_get`) is gated per call by the
  fail-closed broker against the manifest's declared `runtime` permissions and
  returns a structured permission-denied status instead of trapping; dispatch
  itself is timeout-, size- and memory-capped with panic recovery.
- Native companions run as supervised child processes
  (`internal/plugin/native`): the binary path and SHA-256 must match a manifest
  variant, `node` interpreters and shebang wrappers are forbidden, stdout is
  flood-capped at 8 MiB, RPC frames at 1 MiB with a 10 s timeout, and process
  containment uses process groups (POSIX) and job objects / `CTRL_BREAK`
  (Windows). Containment failure quarantines the plugin.

## Not implemented

Renderer-side approval prompts beyond the declarative settings grant flow, the
network/filesystem brokers described in legacy design notes, and per-plugin
secret leases (`SecretLeaseRef`) have no Go implementation. The permission
broker and its fail-closed rules — now including the provider registry — are
the part of that design that shipped.
