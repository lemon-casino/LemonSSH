# Wails v3 Migration Capability Matrix

Status: initial inventory

This matrix and the append-only `migration-ledger.md` are the sole authority for
migration status and resume position. Update the relevant row in the same change
that advances a capability. A row cannot become `migrated` without verification
and a ledger entry. Plan status and next-action prose are navigation hints only
and cannot override the matrix or ledger.

## Status Summary

All rows begin as `not-started`. Existing Electron tests are behavioral evidence,
not proof that a Go replacement exists.

P0-01 已建立可重复的 Electron contract fixtures 和 Windows 本机观测，但由于
macOS/Linux、正式 30-round、remote SSH 和部分 Windows repaint/cleanup 证据缺失，
P0-01 本身不推进 capability 状态。见
`baselines/electron-runtime-baseline.md` 和 ledger `WV3-L001`。

P0-03 authenticated loopback WebSocket candidate 已通过 Windows 10 WebView2
canonical sustained workload、stall/urgent/rebind 和完整性 smoke；三平台、30-round、
paired Electron envelope、真实 popup movement 和 formal RSS/latency evidence 仍缺失，
因此 `TERM-01` 仅为 `probe`。见 `probes/terminal-data-plane.md` 和 ledger `WV3-L010`。

P0-05 已完成外部 Agent 静态协议分类，但三平台 executable/process-tree/handshake
证据缺失，`AI-04` 保持 `not-started`。见
`baselines/external-agent-protocols.md` 和 ledger `WV3-L004`。

WV3-009/WV3-010 已把 production AI 固定为 Non-AI Completion Gate 后的 Phase 7，
并把 plugin permissions 与 Agent capability policy 分为独立 owner。Phase 6 package、
updater 和 upgrade-bootstrap 只产生 qualification evidence，不推进 AI rows，也不构成
final RC。见 ledger `WV3-L005`。

| ID | Capability | Scope | Current owner | Target owner | Initial blocker | Required completion evidence | Status |
| --- | --- | --- | --- | --- | --- | --- | --- |
| FND-01 | Wails shell and typed frontend ports | required | `electron/main.cjs`, preload, `netcattyBridge` | `cmd/netcatty`, shell-neutral runtime client | Production skeleton owns health/version/window-role with generated bindings; the Wails transitionBridge now exposes the migrated terminal/sftp subset (SSH connect/write/resize/signal/close plus onSessionData/onSessionExit over the loopback data plane, SFTP open/list/mkdir/delete/rename/stat/close) so existing UI callers work without Electron IPC; remaining ~450 port methods still fail closed; three-platform launch evidence absent | Wails launch and contract tests on three platforms; application/domain imports remain shell-neutral | implemented |
| FND-02 | Profile store and coordination | required | renderer `localStorage`, storage events, Web Locks | Go transactional profile service | Transactional bbolt store plus OS-flock writer lease with persistent epoch fencing implemented and tested; single-platform evidence only, renderer persistence not wired until P2-07 | key inventory, transaction/crash tests, revisions, notifications and single writer lease | implemented |
| FND-03 | Credential protection and profile migration | required | Electron `safeStorage`, credential/vault backup bridges | Go platform keyring plus Electron migration broker | P2-04 provider implemented and P2-05/P2-06 bundle/import facades now exist: purpose-bound AES-GCM over OS keyring, encrypted X25519 bundle verification, secret re-seal, staging/promote; live cross-platform keyring and full source-profile broker wiring pending | all secret fields decrypt/re-seal; no plaintext leak; atomic cutover and rollback crash matrix | implemented |
| FND-04 | Window, single-instance and app lifecycle | required | Electron `BrowserWindow`, window manager, app lifecycle | Go window/lifecycle coordinator with Wails adapter | Go window registry implemented: role single-instance, opaque tokens, close veto, dirty-editor guard, crash cleanup, session/popup role fencing; Wails window wiring and multi-monitor/crash live matrix pending | multi-monitor, dirty-editor, popup/rebind, second-instance and crash tests on three platforms | probe |
| TERM-01 | Terminal binary data plane | required | MessagePort output/urgent input and flow control | Go data-plane service plus WebView transport | Production bounded v2 binary frame codec differential-checked against the P0-03 probe; loopback WebSocket route orchestration wired into TerminalService and now consumed by the renderer: onSessionData opens the authenticated data socket, grants the receive window, and fans UTF-8 output to xterm.js; three-platform paired benchmark evidence remains pending | zero loss/duplication, bounded RSS/queue, latency and rebind benchmarks on WebView2/WKWebView/WebKitGTK | implemented |
| TERM-02 | Local PTY | required | `node-pty`, terminal worker | Go ConPTY/Unix PTY adapters | Windows ConPTY adapter live-proven on this host (cmd.exe banner, echo, resize, clean close via UserExistsError/conpty v0.1.4) plus Unix creack/pty backend and Wails PTYService facade; shell matrix breadth and reload/Unicode sweep pending | shell matrix, resize flood, Ctrl-C, Unicode, reload and child cleanup tests | implemented |
| TERM-03 | Telnet, serial, Mosh and ET | required | terminal bridge and packaged binaries | Go transport services and supervised binaries | composite: implementation requires the TERM-03 child rows (telnet, serial, mosh/et, zmodem) per the decomposition gate | protocol/device tests, helper hash/arch checks, lifecycle cleanup | not-started |
| TERM-03.1 | Telnet protocol owner | required | terminal bridge telnet paths | Go telnet service (TCP + NAWS/echo negotiation) | IAC codec, WILL/DO negotiation, NAWS, echo-mode tracking and auto-login implemented with in-process server tests; live device matrix and reconnect evidence pending | protocol tests, echo/NAWS and reconnect tests | probe |
| TERM-03.2 | Serial protocol owner | required | terminal bridge serial paths | Go serial service | go.bug.st/serial v1.6.4 backend with validated config, bounded session lifecycle, close-all teardown; real-device matrix pending hardware | device matrix, disconnect and buffer tests | probe |
| TERM-03.3 | Mosh and ET supervised binaries | required | packaged mosh/et binaries + bridges | Go supervised binary runner | Runner implemented (sha256/arch pinned manifest, restart budget, stdin-held live process, fail-fast on immediate death, clean teardown) with live host-binary tests; mosh/et reconnect protocol parity pending | helper hash/arch checks, reconnect and cleanup tests | probe |
| TERM-03.4 | ZMODEM/YMODEM file transfer | required | `zmodemHelper.cjs` event bridge | Go ZMODEM/YMODEM service | CRC-16 framing, ZFILE metadata safety (traversal/reserved/size-cap), cancellation implemented; full rz/sz session engine pending | event tests, file safety and cancellation tests | probe |
| SSH-01 | SSH authentication and sessions | required | `sshBridge.cjs`, ssh2 helpers and patches | Go SSH session service | Dial core implemented: host-key accept-new/reject-changed policy with known-hosts file, password/key+passphrase/keyboard-interactive auth, jump chain transport with keepalive and unified close; now exposed end-to-end by the Wails TerminalService (dial → auth → PTY shell → data plane streaming, resize/signal/close, urgent→stdin); live MFA/proxy/jump evidence and legacy algorithm decisions pending | compatibility lab, host-key fail-closed, cancellation and live MFA tests | probe |
| SSH-02 | Shared SSH transport pool | required | `sshConnectionPool.cjs` | Go typed lease pool | Reference-counted pool with compatibility key (endpoint/auth fingerprint/jump chain/forwarding), single-flight dial, healthy return vs discard, idle TTL/LRU, shutdown and agent-forwarding single-use policy; live one-auth network evidence pending | concurrency/property tests and one-auth network evidence | implemented |
| SFTP-01 | SFTP browsing | required | `sftpBridge.cjs` | Go SFTP service | Browsing core implemented on pkg/sftp v1.13.9 (list/stat/mkdir/rename/remove/read/write via transport-neutral RemoteFS); production ClientFS adapter now wire-protocol tested against a real pkg/sftp server over net.Pipe and exposed by the Wails SFTPService over the shared SSH pool (pool lease + per-session bounded concurrency + download/upload streaming); sudo SFTP and non-UTF-8/raw path live matrix pending | real-server matrix, cancellation, encoding, symlink and reconnect tests | probe |
| SFTP-02 | Transfer scheduler and compressed upload | required | transfer/compress bridges and renderer scheduler | Go transfer service | Chunk scheduler implemented (task identity, checkpoints, pause/resume/cancel, per-host concurrency, aggregated snapshots); compressed upload and high-RTT/corruption lab pending | corruption/hash tests, resume matrix, renderer-close survival and throughput | probe |
| NET-01 | Port forwarding | required | `portForwardingBridge.cjs` | Go tunnel manager on shared SSH pool | Forward manager implemented (local/remote/dynamic SOCKS5, monotonic revision, subscribe+snapshot, injectable tunnel); SSH-pool tunnel wiring and IPv6/collision live tests pending | local/remote/SOCKS, half-close, transport-loss and epoch tests | probe |
| SYS-01 | Local filesystem, dialogs and temp files | required | local FS/temp bridges | Go filesystem and platform adapters | Dedicated temp service, symlink-safe resolution, zip-slip-hardened extraction implemented and tested (Windows live); native dialogs and UNC/long-path live matrix pending | traversal attack corpus, UNC/Unicode, temp substitution and platform tests | probe |
| SYS-02 | Tray, shortcuts and OS menus | required | global shortcut/window bridges | Go platform shell services | Shortcut registry (conflict detection, case-insensitive lookup) and tray menu state generator implemented; native tray/shortcut registration and three-platform behavior unverified | native smoke matrix including lock redaction and cleanup | probe |
| SYS-03 | Deep links, file association and context menu | required | deep-link/main/installer code | Go intent queue plus platform registration | Strict intent parser (ssh/telnet/netcatty schemes, password params rejected) and pre-ready at-most-once queue implemented; installed registration and three-platform delivery unverified | cold/warm start, disabled preference, malformed URL and installed-package tests | probe |
| SYS-04 | App lock and biometric auth | required | Electron settings/runtime and native helper | Go app-lock owner plus platform auth | PBKDF2 verifier core implemented over the P2-04 provider (password sealed via keyring, verifier record password-free); Windows Hello/Touch ID and lock lifecycle wiring pending | verifier migration, timeout/cancel, Touch ID/Hello and lock lifecycle tests | probe |
| SYNC-01 | Vault/settings/session restore persistence | required | React hooks plus localStorage | Go profile services with frontend state adapters | hostStorageAdapter transition layer mirrors Wails writes to Go store; renderer canonical cutover and domain differential/concurrency evidence pending | differential behavior, lossless data equality and restore invariants | probe |
| SYNC-02 | Cloud and convergent sync | required | renderer CloudSyncManager and Electron cloud bridges | Go sync service | CRDT lineage, OAuth/provider and key rotation migration | encrypted fixture parity, CRDT/baseline equality, interruption and rotation rollback tests | not-started |
| AI-01 | Capability catalog and policy | required | CJS catalog, codegen and RPC dispatch | deferred `internal/capability` typed Agent catalog/policy and generated projections | hard-blocked by Non-AI Completion Gate; multiple synchronized current surfaces | Phase 7 projection drift checks and full capability x surface x permission matrix | not-started |
| AI-02 | Native MCP and CLI | required | Node MCP server and tool CLI | `netcatty-mcp`, `netcatty-tool` Go binaries | hard-blocked by Non-AI Completion Gate; auth/discovery/scope and output compatibility | Phase 7 protocol fuzzing, JSON golden tests, token rotation and process cleanup | not-started |
| AI-03 | Catty runtime and providers | required | renderer AgentRuntime, Vercel AI SDK, Electron HTTP proxy | Go agent runtime and HTTP/SSE providers | hard-blocked by Non-AI Completion Gate; tool-loop/context/streaming parity | Phase 7 canonical event traces, compaction, 413, stop, redaction and provider tests | not-started |
| AI-04 | External agent adapters | required | Node SDK drivers and Codex app-server bridge | Go HTTP/CLI/stdio protocol adapters | hard-blocked by Non-AI Completion Gate and P0-05 Bun/runtime/retirement decisions | per-agent Phase 7 protocol parity or explicit retirement decision | not-started |
| PLUG-01 | Plugin v2 contract and package store | required | internal plugin schema/packages and Electron host | Go schema/validator/package manager plus `internal/plugin/permissions` | Manifest v2 contract, permission broker, package store and declarative UI schema implemented (schema validation, fail-closed authorization, lifecycle states, injection-proof UI); codegen drift, atomic install/recovery and v1 rejection UX pending | attack corpus, codegen drift, atomic install/recovery, broker authorization and v1 rejection UX | probe |
| PLUG-02 | WASM runtime and declarative UI | required | sandboxed BrowserWindow runtime and custom views | wazero runtime plus host-rendered UI schema and plugin permission brokers | resource limits and UI contribution model unimplemented; must not depend on Agent catalog | memory/time/quota/trap tests; no DOM/network/filesystem escape; shared service dispatch | not-started |
| PLUG-03 | Native plugin process runtime | required | Electron utility process and companions | Go supervised child-process runtime plus plugin permission brokers | OS sandbox and descendant containment; must not depend on Agent catalog | hash/arch checks, malformed RPC, job/process group cleanup, broker policy and quarantine | not-started |
| REL-01 | Packaging, signing and native resources | required | electron-builder and Electron scripts | Wails build plus platform package pipelines | Qualification packaging pipeline implemented (`scripts/package-wails.mjs`: version ldflags stamp, artifact manifest + SHA-256 checksums, cross-build CGO guard) and Windows artifact launch-smoked; signed packages and required macOS/Linux targets remain absent | Phase 6 focused package evidence plus P8-01 final signed clean-machine/package smoke on all targets | probe |
| REL-02 | Auto-update and rollback | required | electron-updater bridge | signed Go/platform updater | Probed infrastructure exists: ed25519 signed manifest verification and the P6-04 upgrade bootstrap chain (detect→backup→migrate→verify→activate) now persist crash-safely and are exposed as an UpgradeService; no production updater feed, signing key management, or final RC qualification | focused infrastructure evidence plus P8-01 final signed N-1 to N, tamper, interruption, elevation, tray and rollback tests | probe |
| REL-03 | Electron and runtime Node retirement | aggregate | Electron entry, CJS runtime, runtime node_modules | no replacement owner; split purity/deletion target | `REL-03.1` precedes cutover; `REL-03.2` follows rollback closure | child-row artifact purity and repository retirement evidence | not-started |
| REL-03.1 | Wails release artifact runtime purity | required | Electron frozen outside Wails artifact as release carrier | final signed Wails artifact with no runtime Electron/Node | depends on non-AI and AI target owners, not Electron repository deletion | P8-01 artifact SBOM/content/process-tree checks before cutover | not-started |
| REL-03.2 | Electron release-carrier repository retirement | required | frozen Electron rollback carrier including plugin runtime | deletion target | depends on P8-02 cutover and rollback-window closure | P9-01 deletion plus P9-02 repository/dependency scan and final Gate 14 reproof | not-started |

## Update Rules

For every row update, include:

- previous and new status;
- target owner paths;
- exact tests/benchmarks and platforms covered;
- unresolved platform or behavior gaps;
- Electron files deleted or their retirement trigger;
- ledger entry ID.

Scope is canonical and uses only `required`, `removed`, or `aggregate`:

- `required` is an implementable root or child and cannot have `retired` status;
- `removed` preserves the stable ID after an accepted capability-specific
  `scope-removal:<CAPABILITY-ID>` decision, must have `retired` status, and must
  retain chronological `removed:` or `deleted:` evidence from that same ledger entry;
- `aggregate` is a non-implementable roll-up, remains `not-started`, and is
  excluded from completion and gate status calculations.

Replay starts every leaf row at `required` and `REL-03` at `aggregate`, then
applies each ledger `Scope change` chronologically. Final replay scope must match
this matrix. Completion evaluates required leaf rows only. NONAI evaluates its
chronological replay point: required non-AI leaf rows plus `REL-01` and `REL-02`,
excluding AI rows, aggregates, `REL-03.1`, `REL-03.2`, and rows already removed
at that gate. A later removal cannot retroactively satisfy an earlier gate.

The latest valid NONAI gate epoch controls AI advancement and production AI
paths. A required non-AI row falling from `verified` or `migrated` below a
completed state invalidates that epoch; successful `verified -> migrated`
advancement does not. An approved post-gate scope removal still invalidates the
epoch that counted the row as required; a later gate may exclude it. Recovery
does not reopen AI until another complete gate is recorded. Canonical non-`none`
Electron retirement evidence is replay state: later `none:` records do not erase
it, and every final required leaf at `verified` or `migrated` must retain one.

If one capability is too broad for a safe slice, add child rows with stable IDs
instead of hiding partial progress in prose.
