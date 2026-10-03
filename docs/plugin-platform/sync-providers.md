# Sync providers (implemented over the v2 provider registry)

Status: `kind: "sync"` declarations are enumerable through the broker-gated
provider registry (`internal/plugin/providers`, see
[terminal-providers.md](./terminal-providers.md) — a plugin needs the
manifest-declared `{"kind": "provider", "resource": "sync", "mode": "read"}`
permission plus the trusted grant to appear in
`PluginService.ExtensionProviders("sync", …)`), and the sync data plane is
served by the Go extension host (`cmd/lemonssh/pluginExtensionService.go`)
over the same dispatch channel. The renderer adapter
(`infrastructure/services/adapters/pluginSyncIpcHost.ts`) drives it through
the typed bridge surface; `isPluginSyncIpcAvailable()` reports true only when
the bridge exposes the data plane **and** the registry currently enumerates at
least one sync provider, which re-opens the plugin sync cloud backup path
(`infrastructure/services/cloudSync/*`).

## Intent

LemonSSH cloud sync providers are dynamic and namespaced. Built-in providers
(`github`, `google`, `onedrive`, `webdav`, `s3`) stay first-party; plugins
register additional IDs under their plugin namespace with `kind: "sync"` and
permission `provider.sync`.

## Boundary

Plugins implement **encrypted object storage only**:

- `connect` / `disconnect` / `getAccount`
- `getCapabilities` (`revisions`, `conditionalWrites`, `atomicReplacement`,
  size limits)
- `readObject` / `writeObject` / `deleteObject`

LemonSSH owns encryption, the master key, CRDT merge, migrations, protection
snapshots, conflict handling, and read-merge-write-verify. Plugin providers
never receive the vault master key or plaintext sync payloads — callers hand
over already-encrypted object bytes. Sync connect secrets (`password`,
`token`, `secret`, `apiKey`, `accessToken`) are stripped from configuration
before entering any cloud payload, sealed with the platform credential
provider (AES-256-GCM, OS-keyring backed) and stored inside the owning
plugin's store record under a reserved `sync-secret/` key namespace that the
declarative settings surface never exposes; they are referenced only as
opaque `{ kind: "secret", id, key }` refs. A connect dispatch inlines the
resolved secret value — the sandboxed plugin can only ever receive its OWN
secret, and the WASM host imports deliberately have no secret channel.

## Wire protocol

Every sync operation is one `provider.invoke` dispatch with
`kind: "sync"` and `payload.operationId` equal to the request id (see
[terminal-providers.md](./terminal-providers.md) for the envelope). The
operations and payload shapes mirror the contract payloads
(`SyncConnectPayload`, `SyncReadObjectPayload`, …); object bytes cross the
1 MiB dispatch envelope as base64 (`encoding: "base64"`):

| operation        | payload → result |
|------------------|------------------|
| `connect`        | `{configuration, operationId, credential?}` → `{account}` |
| `disconnect`     | `{operationId}` → `null` |
| `getAccount`     | `{operationId}` → `{account: SyncAccount \| null}` |
| `getCapabilities`| `{operationId}` → `SyncCapabilitiesResult` |
| `readObject`     | `{key, operationId, streamed?}` → `{found, byteLength, encoding?, data?}` or `{found, streamed: true, byteLength}` |
| `readChunk`      | `{transferId, operationId, maxBytes}` → `{encoding: "base64", data, done}` |
| `writeObject`    | `{key, operationId, byteLength, encoding: "base64", data, expectedRevision?}` → `{created, revision?}` |
| `writeBegin`     | `{key, operationId, byteLength, expectedRevision?}` → `{windowBytes}` |
| `writeChunk`     | `{transferId, operationId, sequence, encoding: "base64", data}` → `{accepted}` |
| `writeCommit`    | `{transferId, operationId}` → `{created, revision?}` |
| `deleteObject`   | `{key, operationId, expectedRevision?}` → `{deleted}` |

The host clamps chunk windows to 192 KiB of raw bytes (base64 plus JSON must
fit the dispatch envelope) and every dispatch to the 10 s cap. Streamed reads
and begin/chunk/commit writes keep their cursor/buffer in the plugin's own
guest memory between dispatches; the host tracks the transfer id → provider
binding and expires it after 10 idle minutes, so a stale or spoofed
`transferId` resolves to nothing.

## Renderer flow

`createPluginSyncIpcHost()` (`infrastructure/services/adapters/
pluginSyncIpcHost.ts`) implements the `PluginSyncProviderHost` used by the
cloud sync adapters: small objects ride inline, everything above the
`SyncLimits.inlineObjectBytes` cutoff uses the pull/chunked transfers above.
`isPluginSyncIpcAvailable()` returns true only when the bridge exposes the
data plane methods **and** `pluginHostReady()` — fed by the bridge's cached
`ExtensionProviders("sync", …)` registry probe, refreshed on every plugin
lifecycle change — reports at least one enumerated sync provider.

## Current reality

- The data plane is served by `PluginService.PluginSync*` (14 methods,
  `cmd/lemonssh/pluginExtensionService.go`); chunked transfers and secret
  storage are covered by Go tests (`cmd/lemonssh/pluginExtensionService_test.go`).
- The `pluginSyncRestoreSecrets` stash is session-scoped (in-memory on the
  host): previous plaintext values captured on overwrite/delete can be
  re-applied or discarded, but do not survive an app restart.
- The built-in WebDAV/S3 sync adapters live in
  `infrastructure/services/adapters/` and are unrelated to the plugin platform.
- `collectPluginSyncSidecars` / `applyPluginSyncSidecars` remain bridge
  surface without a Go data plane under Wails; the renderer sidecar bridge
  falls back to its local last-known/pending-remote persistence, so plugin
  settings still ride the encrypted cloud payload.
