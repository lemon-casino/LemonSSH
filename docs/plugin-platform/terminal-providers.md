# Terminal Provider API

Status: implemented for enumeration, invocation, cancellation and session
events over the lemonssh-wasm-abi v1 dispatch channel. The registry lives in
`internal/plugin/providers`; the Wails surface is
`PluginService.TerminalProviders` / `ProvideTerminal` /
`CancelTerminalRequest` / `PublishTerminalSessionEvent` /
`ExtensionProviders` (`cmd/lemonssh/pluginService.go`), adapted by the plugin
bridge (`infrastructure/runtime/wails/pluginBridge.ts`) and consumed by the
renderer registry (`application/state/pluginTerminalProviderRegistry.ts`).
The extension kinds (`connection`, `authentication`, `importer`, `sync`) are
enumerated through the same registry and are served end to end by the
extension data plane (`cmd/lemonssh/pluginExtensionService.go`,
`pluginExtensionConnection.go`) — see
[sync-providers.md](./sync-providers.md) for the sync operations and below
for connection/importer/authentication.

## Intent

Terminal providers let a plugin contribute bounded behavior to the terminal:
completion items, decorations, link/hover matchers, prompt markers, and
themes. Extension providers (connection, authentication, importer, sync) are
enumerated through the same registry.

## Declaration and discovery

Plugins declare providers at runtime over the WASM dispatch channel — no
plugin code runs outside the sandbox and nothing is registered that the
manifest does not allow:

- `providers.list` — the host asks an enabled plugin for its declarations;
  the result is `{"providers": [{id, label, description?, kind,
  capabilities?, configurationSchema?}, …]}` (labels follow the contract
  `LocalizedText` shape: a string or a `{locale: text}` map, resolved by the
  host with the requested locale, then `"en"`, then the first key).
- `provider.invoke` — the host runs one operation:
  `{providerId, kind, operation, requestId, session, payload, deadlineMs}`;
  the result is the operation-specific JSON value.
- `provider.sessionEvent` — the host notifies the plugin about one terminal
  session lifecycle event (`{type, session, exitCode?}`); the result is only
  checked for `ok`.

The SDK ships the canonical constants and decoders
(`PROVIDERS_LIST_DISPATCH_METHOD`, `PROVIDER_INVOKE_DISPATCH_METHOD`,
`PROVIDER_SESSION_EVENT_DISPATCH_METHOD`, `parsePluginProviderListResult`,
`parsePluginProviderInvokeRequest`).

## Fail-closed authorization

A declaration is accepted only while **both** hold:

1. the plugin's stored manifest declares the permission
   `{"kind": "provider", "resource": "<kind>", "mode": "read"}` (the
   `provider` kind is validated by `internal/plugin/manifest` and mirrored by
   `@lemonssh/plugin-cli`); and
2. the fail-closed broker holds the grant for
   `["provider","<kind>"]:read` — recorded only through the trusted
   `PluginService.GrantPermission` approval path, re-checked by the registry
   on every enumeration and invocation, and revoked by
   `SetEnabled(false)` / `Uninstall`.

Disabling a plugin, revoking its grant, or breaking its WASM module removes
its providers from the registry without surfacing an error to other plugins.
Enumerating providers never instantiates or "starts" anything: the WASM
module already exists from enable time, and a module that cannot answer the
dispatch simply contributes nothing.

## Hard boundaries

- Providers receive immutable metadata snapshots only — session/host IDs,
  protocol, status, cwd, title, shell type, dimensions. Never raw xterm
  objects, backend handles, password/prompt content, or unbounded output
  streams.
- The privileged `terminal.interceptor.input/output` kinds are **not**
  registrable through this path; input/output interceptors remain a separate,
  explicit-grant fast path (`terminal.intercept.*`), and the registry rejects
  those kinds.
- Each `provider.invoke` dispatch is deadline-capped (requested
  `deadlineMs`, clamped to the host's 10 s dispatch cap) and can be aborted
  through `CancelTerminalRequest`, which surfaces as a `cancelled` result.
- Failures are structured: a plugin-declared error becomes a `failed` result
  carrying the plugin's code in `error.data.pluginCode`; transport failures
  map to the wire codes (`-32004` deadline, `-32013` internal).

## Renderer flow

`usePluginTerminalProviders` polls availability per kind, sends
`providePluginTerminal` requests with 1.5 s deadlines through the window
registry, and merges accepted results into decorations and the resolved
theme (`domain/pluginTerminalProviders.ts` normalizes and bounds every
result). The shipped `hello-lemonssh` example declares the
`terminal.theme` provider `com.lemonssh.hello.accent`; after the
manifest-declared `provider/terminal.theme/read` grant it answers
`provideTheme` with an accent cursor color.

## Extension provider data plane (connection / importer / authentication)

The extension kinds run their operations over the same `provider.invoke`
envelope, served by `PluginService.InvokePluginExtensionProvider` (generic
surface) plus the dedicated methods the renderer bridge maps
(`infrastructure/runtime/wails/pluginBridge.ts`):

- **connection** — `validateConfiguration` / `probe` through the generic
  surface (the `startPluginConnection` pre-flight in
  `application/state/useTerminalBackend.ts`), then `open` /
  `writeInput` / `readOutput` / `resize` / `signal` / `status` / `close` for
  the live connection. The Go host registers every opened connection as a
  first class terminal session (`internal/app/terminaluse/plugin_session.go`):
  renderer input, resize, signals, session logs and exit tracking flow
  through the ordinary terminal pipeline, and plugin output is published on
  the loopback data plane. A per-chunk `plugin:connection-data` renderer
  event exists for non-terminal consumers and stays gated off until a
  listener subscribes; connection closes always surface as
  `plugin:connection-closed`.
- **importer** — `detect` over a bounded base64 sample; `parseBegin` /
  `parseChunk` / `parseFinish` / `parseRecords` / `parseAbort` stream a file
  staged behind an opaque selection token (picked through the native file
  dialog, bounded by the contract `ImporterLimits`). Progress records are
  mirrored to the renderer as `plugin:importer-progress` events; drafts and
  warnings/errors return to the caller.
- **authentication** — `begin` results carrying
  `{status: "challenge", challenge}` are registered host-side and mirrored as
  `plugin:authentication-challenge` events; the renderer answers through
  `PluginService.RespondPluginAuthenticationChallenge` (`challengeResponse`
  operation). `authenticated` clears the pending entry, `cancelled`/`failed`
  emit a cancel event; unknown challenge ids fail closed.

Every operation re-resolves its `providerId` through the broker-checked
registry first, honors the request deadline (clamped to the 10 s dispatch
cap), and can be aborted through `PluginService.CancelPluginExtensionRequest`.
Cancellations mark the request id so late operations with the same id fail
fast instead of re-entering a plugin.
