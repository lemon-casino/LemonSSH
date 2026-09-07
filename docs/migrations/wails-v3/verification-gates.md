# Wails v3 Migration Verification Gates

Status: approved gate model; thresholds may only be tightened or changed by an
explicit design decision with recorded evidence.

Passing a unit test is not enough to declare a capability migrated. Apply the
gates relevant to each slice and all global gates before release cutover.

## Chronological Governance Gates

`capability-matrix.md` plus the append-only ledger are the sole status and
resume authority. Plan prose is a navigation hint. The checker replays status,
scope, retirement evidence and gates in ledger order.

P6-05 `NONAI-COMPLETE` evaluates required non-AI leaf rows in `FND`, `TERM`,
`SSH`, `SFTP`, `NET`, `SYS`, `SYNC` and `PLUG`, plus `REL-01` and `REL-02`.
It excludes AI rows, aggregate rows, `REL-03.1`, `REL-03.2`, and rows already
removed at that chronological point. Its accepted decision references must
collectively carry all exact categories:

- `release-target:windows`
- `release-target:macos`
- `release-target:linux`
- `agent-runtime:cursor-bun`
- `agent-runtime:opencode-bun`
- `agent-disposition:copilot`
- `agent-disposition:codebuddy`
- `agent-disposition:cursor-cli`

Arbitrary accepted decisions and verification prose are insufficient. A later
scope removal cannot repair an earlier premature gate. After a valid gate, a
required non-AI leaf falling from `verified` or `migrated` below completed status
invalidates that epoch; successful `verified -> migrated` advancement does not.
An approved post-gate scope removal still invalidates the epoch that counted the
row as required; the next gate evaluates the new scope. Recovery requires another
valid P6-05 gate before more AI advancement.
The canonical ordered evidence parser is:
`nonAiRows=verified; releaseTargets=<accepted release-target decision IDs>;
agentDecisions=<accepted Agent decision IDs>;
qualification=P6-02,P6-03,P6-04; authority=<nonempty>`. ID lists use
comma-separated `WV3-NNN` values without spaces or duplicates; every ID must be
accepted, category-relevant and repeated in the ledger `Decision references`.
When an epoch is invalid, production `internal/capability`, `internal/agent`,
`cmd/netcatty-mcp` and `cmd/netcatty-tool` paths are forbidden again.

P8-02 `WAILS-CUTOVER` requires grade A, `REL-03.1` already `verified` or
`migrated`, all required Phase 7 AI leaf rows `verified` or removed by approved
capability-specific decisions, and structured cutover verification:
`rc=<nonempty>; platforms=<nonempty>; migration=<nonempty>; rollback=<nonempty>;
authority=<nonempty>`.

P8-03 `ROLLBACK-CLOSED` follows a valid cutover, requires grade A and an accepted
decision carrying `rollback-window-closure`, and records exact closure evidence:
`thresholds=<nonempty>; sample=<nonempty>; blockers=<nonempty>;
authority=<nonempty>`. Only after both release gates may `REL-03.2` advance.

## Gate 1: Architecture and Ownership

- Core Go services do not import Wails.
- Application/domain TypeScript does not import generated Wails bindings.
- Every implemented capability has one canonical handler and policy owner.
- Wails, MCP, CLI, Agent and plugin adapters call the same application service.
- No permanent Electron fallback remains after migration of a capability.
- Any temporary compatibility carrier has one owner, reason, deadline or
  falsifiable retirement trigger.
- Architecture tests prevent reintroduction of shell-specific imports into
  shell-neutral layers.

## Gate 2: Frontend Contract Parity

- Electron and Wails adapters pass the same request, error, cancellation,
  subscription and lifecycle contract suites during dual-shell development.
- Generated TypeScript contracts are reproducible and checked for drift.
- Long-lived subscriptions explicitly unsubscribe and do not leak across
  React StrictMode remounts.
- Window and session identities are opaque and cannot be forged by frontend
  payloads.

## Gate 3: Terminal Data Plane

Required workloads:

- sustained high-rate output;
- millions of short lines;
- long unbroken lines;
- multiple concurrent sessions;
- hidden/stalled/reloaded WebView;
- urgent input and Ctrl-C during output flood;
- route movement between main and popup windows;
- metadata-only plugin pipeline output.

Required results:

- zero byte loss, duplication or reordering;
- stale generations cannot affect replacement sessions;
- bounded process and WebView memory;
- bounded backend and frontend queues;
- demonstrated producer pause/resume and returned credit;
- no starvation of urgent input;
- latency and throughput are no worse than the accepted Electron baseline, or
  a separately approved product threshold backed by measurements;
- evidence exists for WebView2, WKWebView and WebKitGTK.

The data-plane implementation remains `probe` until these results are measured.

## Gate 4: Terminal and SSH Compatibility

Verify on supported OS/architecture targets:

- local shells, resize, signals, child trees and working directories;
- UTF-8 and configured legacy encodings;
- OpenSSH versions and representative legacy/network appliances;
- password, key, encrypted key, keyboard-interactive/MFA partial success,
  certificates and agents;
- agent forwarding, jump chains, SOCKS/HTTP/proxy command paths;
- host-key accept, persistence, mismatch and rejection on every hop;
- keepalive, timeout, disconnect, reconnect and cancellation races;
- shared transport reuse and unhealthy transport eviction.

Unsupported legacy behavior must be an explicit product decision, not a silent
library limitation.

## Gate 5: SFTP, Transfer and Forwarding

- directory listing, stat, symlink and broken-link semantics match;
- sudo SFTP and non-UTF-8 filename policy is explicitly verified or retired;
- transfer progress survives UI unmount and window closure;
- pause/resume/cancel/checkpoint tests cover arbitrary interruption points;
- source mutation, disconnect, retry and final byte/hash verification cannot
  produce silent corruption;
- local, remote and dynamic forwarding cover IPv4/IPv6, half-close, collisions,
  concurrent clients and shared transport loss;
- no orphan transfer, channel, listener or socket survives shutdown.

## Gate 6: Profile Inventory and Data Equality

Before profile migration implementation is accepted:

- every key in `infrastructure/config/storageKeys.ts` is classified as migrated,
  device-local, transient/cache, or explicitly retired;
- every Electron-main durable file is classified;
- raw string, boolean, number and JSON encodings are preserved where required;
- normalized source and target entity counts, stable IDs, ordering, groups,
  references, settings and session/workspace layouts match;
- CRDT vectors, dots, tombstones, conflicts and provider baselines match;
- only documented nondeterministic fields are excluded from semantic hashes.

No unclassified `netcatty_*` data may be ignored.

## Gate 7: Secret Migration

Use fixtures containing every secret-bearing field.

- Electron decrypts all usable `enc:v1:` values before cutover.
- Go re-seals each value with the correct purpose and platform provider.
- no plaintext appears in logs, events, receipts, temporary files, database
  diagnostics or backup manifests;
- no Electron ciphertext survives as a valid Wails credential;
- connection/authentication consumers receive the original plaintext only at
  the final privileged boundary;
- unavailable secure storage fails closed;
- an unreadable meaningful secret blocks the lossless migration unless the user
  makes a separate explicit destructive decision.

## Gate 8: Crash Consistency and Writer Ownership

Inject process termination before and after every export, decrypt, staging,
re-seal, validation, promotion, receipt and writer-lease step.

After each crash, one of these must hold:

- Electron is intact and remains the only writer;
- Wails is fully verified and remains the only writer;
- both profiles are intact but only one writer lease exists;
- automatic sync is blocked pending explicit recovery.

Test forward cutover and reverse rollback. Never validate rollback by simply
launching Electron against stale pre-cutover data.

## Gate 9: Sync and Backup Safety

- v1 cloud payloads remain readable where currently supported;
- convergent schema mismatch/corruption fails closed;
- provider baselines and canonical replicas migrate together;
- master-key rotation rollback restores exact prior ciphertext;
- destructive remote apply creates the required encrypted protective backup;
- interrupted apply blocks automatic upload;
- plugin sync sidecars and user-owned data survive missing plugin code;
- legacy Electron backups are re-encrypted or retained with an explicit access
  plan before Electron removal.

## Gate 10: Capability Policy, MCP and CLI

This gate is deferred until P6-05 records `Gate: NONAI-COMPLETE` after every
required non-AI leaf target owner plus `REL-01` and `REL-02` is verified and the
Phase 6 focused packaging/updater/upgrade-bootstrap evidence passes. P0-05
read-only probes do not open this gate.

Run a complete capability x surface x permission-mode matrix:

- observer;
- confirm denied, approved and grant match/mismatch;
- auto;
- missing/cancelled chat session;
- in-scope and out-of-scope terminal session;
- read, write and sensitive read;
- planned/unsupported capability;
- documented polling/control exceptions.

Also verify:

- authentication precedes dispatch;
- tokens rotate/revoke and discovery files have restrictive ACL/mode;
- malformed/oversized/partial protocol frames are bounded;
- CLI JSON output and command quoting remain compatible where required;
- cancellation clears approvals, jobs, processes, transfers and locks;
- shutdown removes sockets/discovery files and reaps children.

## Gate 11: Agent Runtime

This gate is deferred until P6-05 records `Gate: NONAI-COMPLETE` after every
required non-AI leaf target owner plus `REL-01` and `REL-02` is verified and the
Phase 6 focused packaging/updater/upgrade-bootstrap evidence passes. P0-05
read-only probes do not open this gate.

- one active turn per chat session;
- turn start/end exactly once for success, error and cancellation;
- canonical events from every supported backend;
- secret redaction in calls, results, traces and errors;
- tool-result integrity, deduplication and scoped output handles;
- pre-turn/413 compaction and reinjection behavior;
- no unintended LLM summarization during step pruning;
- UI, slash, MCP and shutdown stop through one owner;
- runtime-qualified sessions cannot resume through another adapter;
- every retained external agent passes protocol fixtures without Node;
- race tests cover cancellation, steering, event fan-out and cleanup.

## Gate 12: Plugin v2 Security

Contract and package tests include:

- path traversal, aliases, reserved names, duplicate entries and symlinks;
- archive bombs, changing staging sources, oversized files/counts and hash
  mismatch;
- interruption between package publication/database operations;
- generated Go/TypeScript/guest binding drift;
- JSON depth/node/byte and safe-integer limits;
- stream sequence, credit, cancellation and late-response behavior;
- clear rejection and non-execution of v1 browser/Node packages.

WASM tests include memory/time/host-call limits, infinite loops, traps,
unauthorized imports, forbidden ambient I/O, stale identity and teardown.

Native-process tests include digest/architecture checks, symlink escape,
readiness timeout, malformed RPC, output flood, descendants, graceful/forced
termination and containment-failure quarantine.

## Gate 13: Platform and WebView Matrix

The exact supported OS versions, CPU architectures, WebView/runtime versions,
Linux distributions/desktops and package formats are owned by
`release-target-matrix.md` and must be frozen by `P0-01A` before production
implementation begins. Required release target classes include:

- Windows: supported architectures, WebView2, ConPTY, DPAPI/Hello, tray,
  protocols/context menu, installed and portable behavior;
- macOS: x64/arm64 as supported, WKWebView, PTY, Keychain/Touch ID, signing,
  notarization, URL/file events, tray/dock and update replacement;
- Linux: supported architectures, WebKitGTK, PTY, Secret Service, X11/Wayland
  documented behavior, tray, desktop handlers, AppImage/deb/rpm/pacman.

Lossless Linux profile migration requires a working, unlocked Secret Service.
The frozen matrix must identify supported keyring implementations and explicitly
exclude environments that cannot securely persist credentials.

Each target needs clean-machine install, first launch, profile migration,
terminal, SSH, SFTP, deep link, update and uninstall smoke evidence.

`*-latest` CI labels, artifact generation, or compile success do not define a
minimum supported OS. Evidence grade A requires every `required` row and artifact
class in `release-target-matrix.md`; `best-effort` rows cannot substitute for one.

## Gate 14: Packaging and Runtime Purity

Inspect every final artifact and SBOM:

- no Electron executable or library;
- no `.asar`;
- no runtime Node executable or `node_modules`;
- no `.cjs`/`.mjs` runtime bootstrap required by the application;
- no JavaScript/Node plugin runtime;
- no Node AI SDK import in shipped code;
- no child process command invokes `node`;
- no retained Agent/helper/plugin executable has a Node shebang, embeds a Node
  runtime, or launches Node anywhere in its recursive process tree;
- native plugin variants comply with the approved binary/dependency policy and
  include transitive SBOM evidence;
- native helpers are signed/hash-verified and match OS/architecture;
- installers, updaters and uninstallers cleanly terminate/reap owned processes.

P8-01 runs the artifact-purity subset before cutover for `REL-03.1`; it does not
require deletion of the frozen Electron carrier. P9-02 reruns the complete
repository/artifact/process proof after the rollback window for `REL-03.2`.
Phase 6 pre-AI package mechanics are focused qualification evidence, not an RC
and not satisfaction of this final gate.

## Evidence Grades

- `A`: direct target and relevant regression evidence on all applicable
  platforms; no meaningful unknown remains.
- `B`: direct evidence with a bounded platform or integration gap; capability
  cannot advance beyond `implemented` unless the matrix explicitly permits it.
- `C`: static analysis, unit-only, single-platform or probe evidence; capability
  remains `probe` or `implemented`.

Unless `decisions.md` contains an explicit exception referenced by the matrix
and ledger, `verified` and `migrated` require grade `A`; grade `B` cannot advance
beyond `implemented`.
