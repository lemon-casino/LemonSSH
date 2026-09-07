# Wails v3 and Go Target Architecture

Status: approved design

## 1. Scope

This document defines the target runtime architecture and migration boundaries.
It does not claim that Wails or Go implementations already exist.

In scope:

- Wails v3 application and window lifecycle;
- Go runtime services for terminal, SSH, SFTP, transfers, forwarding, storage,
  credentials, sync, AI, plugins, OS integration, packaging, and updates;
- reuse of the React/TypeScript UI, xterm.js, Monaco, domain logic, and
  presentation stores where their behavior remains shell-neutral;
- lossless profile migration and controlled Electron retirement.

Out of scope:

- replacing React, TypeScript, xterm.js, Monaco, or Vite;
- preserving runtime compatibility with unpublished JavaScript/Node plugins;
- long-term Electron/Node sidecars;
- reducing product scope without an explicit product decision;
- allowing Wails and Electron to concurrently mutate one profile.

## 2. First Principles

The migration is governed by these principles:

1. User outcomes and stored data matter more than implementation-language
   parity.
2. A replacement is complete only when behavior, safety, performance, and
   lifecycle ownership are replaced together.
3. There is one canonical owner for each mutable resource and policy decision.
4. Adapters translate transports; they do not own business policy.
5. Security-sensitive operations fail closed when identity, permission,
   credential protection, or containment cannot be established.
6. The terminal hot path is a data-plane problem, not ordinary UI RPC.
7. Old runtime paths retire as soon as their replacements pass the declared
   gates. Compatibility code requires a concrete removal trigger.

## 3. Target Layers

```text
React / TypeScript UI
  - App shell, xterm.js, Monaco, views, presentation state
                         |
Shell-neutral frontend ports and generated clients
  - typed requests, events, subscriptions, cancellation, window identity
                         |
Go application services
  - sessions, vault, settings, sync, agents, plugins, approvals
                         |
Go domain and policy
  - models, capability catalog, migration rules, security decisions
                         |
Infrastructure and platform adapters
  - PTY, SSH, SFTP, filesystem, keyring, process, HTTP, updater, packaging
                         |
Windows / macOS / Linux
```

Wails belongs at the outer UI/host boundary. Core Go packages must not import
Wails. This permits headless tests, CLI/MCP adapters, and platform services to
reuse the same application owners.

The Wails facade lives under `cmd/netcatty` or a dedicated outer adapter package
owned by that command. It may import `internal/app`; `internal/app` must never
import Wails.

## 4. Proposed Go Repository Shape

The exact names may change during planning, but ownership must follow this
shape:

```text
cmd/
  netcatty/                 Wails application
  netcatty-tool/            native internal CLI
  netcatty-mcp/             stdio MCP adapter
  netcatty-schema/          contract/code generation checks
  netcatty-plugin-pack/     plugin validation and packaging

internal/
  app/                      shell-neutral startup, shutdown and use cases
  capability/               one catalog, policy, scope, dispatch, projections
  terminal/                 sessions, PTY, transport pool, SFTP, transfers
  agent/                    turns, context, tools, providers, external adapters
  plugin/                   v2 contract, packages, runtimes, permissions
  profile/                  durable store, migration, backup, writer lease
  sync/                     encryption, CRDT, providers, recovery
  rpc/                      bounded local protocols, MCP/CLI transport
  platform/                 OS keyring, process, window, tray, paths, updater
```

Do not create one package per Electron bridge mechanically. Group code by the
resource or policy it owns.

Plugin and Agent policy do not share a principal/grant owner. Plugin runtime
identity, grants, secret leases, quotas and brokers live under
`internal/plugin/permissions`. Agent capability identity, chat permission mode,
MCP/CLI and tool projections live under deferred `internal/capability`. Both
invoke the same shell-neutral Go application services; neither imports the
other's authorization model.

The plugin owner is completed before the Agent capability catalog exists. It
must therefore authorize plugin principals directly through
`internal/plugin/permissions`, then invoke shared Go application services for
operations. It must not depend on a provisional or future Agent catalog.

## 5. Frontend Reuse and Ports

The React routes in `index.tsx`, UI components, pure `domain/` logic, and most
presentation stores are reusable. Direct shell dependencies must move behind
capability-specific ports.

The current `netcattyBridge` is the transition seam, but the final frontend
must not depend on one unstructured global object. The target contract is a
composed runtime client:

```ts
interface RuntimeClient {
  app: AppClient;
  windows: WindowClient;
  profile: ProfileClient;
  credentials: CredentialClient;
  terminal: TerminalClient;
  sftp: SftpClient;
  sync: SyncClient;
  agent: AgentClient;
  plugins: PluginClient;
}
```

Rules:

- Application and domain modules do not import generated Wails bindings.
- Only the Wails adapter imports generated Wails code.
- Window identities are opaque Go-issued tokens, not WebView implementation
  IDs.
- Events include source instance and monotonic revision where ordering matters.
- Cancellation and unsubscribe are explicit for every long-lived operation.

## 6. Control Plane and Terminal Data Plane

### 6.1 Control plane

Typed Wails bindings and bounded events are suitable for:

- connect, close, resize, configuration and metadata;
- file and SFTP operations;
- transfer lifecycle and aggregated progress;
- forwarding, dialogs, settings and window actions;
- approvals, authentication challenges and agent events.

### 6.2 Terminal data plane

PTY, SSH, Mosh, Telnet and serial bytes must not be sent as one JSON RPC/event
per chunk. The data plane must provide:

- binary frames;
- per-session ordering and generation fencing;
- bounded queues and explicit credit/backpressure;
- a separate urgent-input/control path;
- reconnection and window-rebind semantics;
- no byte loss or duplication during renderer mount, reload, move, or stall;
- observable queue, latency, pause and resume metrics.

The first implementation candidate is an authenticated loopback WebSocket with
binary frames and one-use renderer tokens. It remains a probe until it meets
the performance gates. If it fails, use a Wails-native binary bridge or shared
memory/ring-buffer design. The architecture does not promise a WebSocket
fallback as a permanent second path.

## 7. Terminal and Network Services

Go becomes the owner of long-lived terminal resources:

- OS-specific PTY adapters: ConPTY on Windows and Unix PTY on macOS/Linux;
- SSH authentication, host keys, agents, jump hosts, proxies and certificates;
- a shared typed SSH transport pool for shell, SFTP, transfer and forwarding
  leases;
- SFTP browsing and dedicated transfer channels;
- resumable transfer scheduling and global progress;
- local, remote and dynamic port forwarding;
- Mosh and Eternal Terminal as supervised external binaries until a justified
  replacement exists;
- Telnet and serial as explicit transport implementations.

Go contexts, supervisors and bounded channels own cancellation and shutdown.
If goroutine isolation cannot contain a failure class, use a supervised Go
helper process rather than reintroducing Node.

## 8. Profile, Storage and Credentials

### 8.1 Canonical store

Renderer `localStorage` is not a portable profile format and cannot remain the
final source of truth. The target is a Go-owned transactional profile store
with:

- raw compatibility values for staged frontend migration;
- typed tables/documents for stable owners;
- per-record revisions and compare-and-swap;
- multi-record transactions;
- one cross-process profile writer lease;
- durable commit notifications;
- schema versions, migration receipts and backup manifests.

The storage engine decision belongs to implementation planning. It must support
atomic promotion and crash recovery on all target platforms.

### 8.2 Lossless cutover

Electron is the only safe initial reader of existing `enc:v1:` ciphertext. The
migration sequence is:

1. Acquire a profile-wide writer lease and suspend mutations/auto-sync.
2. Drain pending Vault and settings writes.
3. Create a required encrypted backup and source fingerprint.
4. Export all classified renderer keys and Electron-main durable files.
5. Decrypt existing credentials inside Electron memory.
6. Transfer a versioned, authenticated migration bundle without persisting
   plaintext.
7. Import into a fresh Go staging profile.
8. Re-seal credentials through platform-native protection.
9. Validate schemas, counts, IDs, relations, CRDT lineage and semantic hashes.
10. Atomically promote staging and write a cutover receipt.
11. Allow Wails to become the only writer.

If any meaningful secret cannot be decrypted, lossless migration is blocked.
It must not silently produce unusable credential placeholders.

### 8.3 Platform credential providers

- Windows: DPAPI/Credential Manager or another reviewed user-bound provider.
- macOS: Keychain Services.
- Linux: Secret Service with fail-closed handling when secure storage is
  unavailable.

The initial Linux support contract requires a working, unlocked Secret Service
implementation for profile migration and persisted credentials. Environments
without secure storage may run only workflows that do not persist or migrate
secrets; they are not supported targets for lossless profile migration. Adding
another Linux secret provider requires a security decision recorded in
`decisions.md`.

New envelopes identify format version, provider and purpose. Existing
`enc:v1:` values remain legacy Electron ciphertext and are never reinterpreted
as a new format.

## 9. Sync and Backups

The Go sync owner preserves:

- zero-knowledge encrypted cloud payloads;
- current CRDT replica, tombstones, dots, vectors, conflicts and per-provider
  baselines;
- prepare/check/commit/rollback master-key rotation;
- required protective backups before destructive remote apply;
- interrupted-apply detection that blocks unsafe upload;
- device-local settings that must not enter cloud sync;
- local-only known-host trust data.

Materialized Vault data alone is insufficient for migration. The canonical CRDT
lineage and provider baselines migrate as one consistency unit.

## 10. Capability Catalog, MCP and CLI

This entire section is part of the deferred AI migration domain. It begins only
after the non-AI completion gate.

One typed Go capability catalog owns:

- stable capability identity and description;
- input schema;
- read/write/sensitive policy;
- required approval mode;
- agent placement;
- Wails, MCP and CLI surface metadata;
- handler registration.

Generated projections include Wails metadata/TypeScript clients, Catty tools,
global agent tools, MCP schemas, CLI commands and policy fixtures. No adapter
maintains a separate manually synchronized handler table.

`netcatty-mcp` and `netcatty-tool` are native Go binaries. Authentication,
scope, approval, cancellation and dispatch remain host-owned. MCP and CLI are
transport adapters, not alternate policy authorities.

## 11. Agent Runtime

Go becomes the sole owner of active agent turns, events, tool execution,
context management, cancellation, traces and output handles.

Preserved invariants include:

- at most one active turn per chat session;
- one canonical event protocol across backends;
- one stop path for UI, slash, MCP and shutdown;
- tool argument/result secret redaction;
- bounded recoverable tool-output handles;
- compaction with permission/session/goal reinjection;
- runtime-qualified external session identity;
- no duplicate Netcatty and external-agent approval prompts.

Catty providers migrate to direct Go HTTP/SSE adapters. External integrations
must expose a stable documented protocol:

- Codex: app-server JSONL protocol;
- ACP-capable agents: ACP/stdio;
- other agents: reviewed HTTP, CLI or stdio adapters.

If Claude, Copilot, Cursor, OpenCode, CodeBuddy or another integration has no
stable non-Node protocol by the release gate, that integration is disabled or
retired through an explicit product decision. A hidden permanent Node sidecar
is forbidden.

## 12. Plugin Platform v2

### 12.1 Contract

Plugin v2 is a new contract. The old `main.browser` and `main.node` entrypoints
are rejected with a clear incompatibility result. Source-level migration tools
may be provided, but no runtime compatibility host is shipped.

Keep the strongest existing contract properties:

- schema-first generated types and validators;
- immutable package snapshots and atomic activation;
- host-generated runtime identity;
- bounded RPC, deadlines, cancellation and streams;
- canonical resource permissions and fail-closed brokers;
- opaque secret references and one-use operation-bound leases;
- serialized lifecycle mutation, crash quarantine and audit;
- user-owned data that does not cascade-delete with package code.

### 12.2 WASM runtime

Ordinary plugins run through pure-Go `wazero`:

- no ambient filesystem, network or environment;
- WASI disabled by default;
- bounded memory, execution time and host calls;
- cancellable host imports;
- capability broker access only after policy evaluation;
- teardown on trap, protocol violation or quota exhaustion.

Suitable contributions include commands, importers, transformations, settings
logic, completion/decorations, and bounded sync/provider logic.

### 12.3 UI

Ordinary plugin UI is declarative data rendered by trusted Netcatty React
components. Arbitrary plugin HTML, CSS and JavaScript do not run in the main
Wails WebView. This preserves DOM, input, theme and host-binding isolation.

### 12.4 Native process runtime

Capabilities that cannot fit WASM use a native child process, never an in-process
Go plugin. Requirements include:

- signed or hash-pinned OS/architecture variants;
- package containment and symlink rejection;
- shell-free launch and minimal environment;
- private data directory;
- bounded framed RPC and streams;
- OS process-group/job-object containment;
- graceful then forced shutdown;
- quarantine on containment failure.

## 13. Windows, macOS and Linux

No platform is considered covered by another platform's result.

Windows requires ConPTY, process-tree cleanup, DPAPI/Hello, WebView2, frameless
chrome, registry protocols/context menu, tray, installed and portable packages.

macOS requires Unix PTY, WKWebView, Keychain/Touch ID, hardened runtime,
notarization, URL/file events, tray/dock behavior and safe bundle replacement.

Linux requires Unix PTY, WebKitGTK, Secret Service, X11/Wayland constraints,
desktop protocol registration, tray variants, and AppImage/deb/rpm/pacman
packaging behavior.

## 14. Cutover and Rollback

During dual-shell development:

- Electron is the default release and current behavior oracle.
- Wails uses test profiles until the migration broker is verified.
- only one shell can own a profile writer lease;
- no feature writes to both stores as a steady-state strategy.

The Go implementation is the `target-owner`; Electron remains a frozen
`release-carrier` until default-release cutover. Verification disconnects a
capability from the Wails-side legacy path and freezes the Electron behavior.
Deletion from the repository occurs at the row's cutover or rollback trigger.
This distinction permits a stable Electron release without granting two owners
authority over the target architecture.

At release cutover:

1. all pre-cutover required leaf capabilities are verified except `REL-03.2`;
   aggregate rows are excluded and removed rows require an earlier accepted
   scope decision;
2. all three platforms pass packaged smoke tests;
3. the profile migration and rollback crash matrix passes;
4. Wails becomes the default release;
5. Electron remains read-only/export-only for a bounded rollback window;
6. after the window, Electron code and compatibility adapters are deleted.

The sequence is non-AI target verification -> AI implementation/verification ->
final signed RC -> default Wails cutover -> bounded rollback observation ->
Electron/Node repository retirement. Pre-AI packaging artifacts are integration
artifacts, not release candidates.

Before cutover, runtime-purity evidence applies to the signed Wails artifact
(`REL-03.1`). Repository deletion of the frozen Electron release/rollback carrier
(`REL-03.2`) occurs only after the rollback observation window; it is not a
pre-cutover requirement.

Rollback after Wails has accepted writes requires a reverse export, verification
and writer-lease transfer. Launching old Electron directly against stale data
is not a rollback procedure.

## 15. Architecture Integrity Rules

Every migration review must answer:

1. Which resource or decision has one canonical owner?
2. Does the adapter contain policy that belongs in a higher-level owner?
3. Is an old Electron path still carrying live logic?
4. Did the change add a fallback or compatibility carrier?
5. What exact evidence permits old-path retirement?
6. Does the new path preserve failure, cancellation and shutdown semantics?
7. Did the required documentation and ledger update land with the code?

Any change to the owner model, plugin contract, data cutover, terminal data
plane, runtime Node boundary or three-platform release policy requires explicit
design review before implementation.
