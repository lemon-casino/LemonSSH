# Wails v3 Migration Decisions

Status: active decision authority

This file records approved product and architecture decisions for the migration.
Matrix rows, plan tasks and ledger entries must cite a decision ID when they
change support scope, compatibility, performance thresholds, security providers,
capability retirement, rollback policy or canonical ownership.

Every accepted decision uses a human-readable `### WV3-NNN - Title` heading and
exact `Categories` metadata. The title is review context; categories are the
mechanical authorization boundary.

Do not rewrite accepted entries. Supersede them with a new decision that names
the old ID and explains the changed evidence.

## Accepted Decisions

### WV3-001 - Controlled dual-shell migration

- Categories: migration-strategy:controlled-dual-shell, retirement:bounded-rollback
- Decision: Electron remains a frozen stable release carrier while Go becomes
  the target owner capability by capability.
- Retirement: Electron becomes read-only/export-only at Wails cutover and is
  deleted after the bounded rollback window.

### WV3-002 - Final runtime excludes Electron and Node

- Categories: runtime-boundary:node-free
- Decision: Node may be used to build React assets but may not be packaged,
  directly or transitively launched, embedded by a retained integration, or
  required by a plugin/Agent/helper at runtime.

### WV3-003 - Three-platform simultaneous cutover

- Categories: release-policy:simultaneous-three-platform
- Decision: Windows, macOS and Linux must all meet the frozen release target
  matrix before Wails becomes the default release.

### WV3-004 - Lossless profile migration and one writer

- Categories: data-migration:lossless, writer-model:single
- Decision: existing data migrates with backup, semantic verification, atomic
  promotion and tested rollback. Electron and Wails never concurrently write
  one profile.

### WV3-005 - Plugin v2 breaks runtime compatibility

- Categories: plugin-contract:v2-break, plugin-runtime:wasm-declarative-native
- Decision: unpublished JavaScript/Node plugin entrypoints are not executed by
  the target runtime. Ordinary plugins use WASM and declarative UI; exceptional
  native code runs only in supervised child processes.

### WV3-006 - Dedicated terminal data plane

- Categories: terminal-data-plane:dedicated
- Decision: terminal bytes do not use per-chunk Wails JSON calls/events. One
  bounded binary transport is selected by evidence and failed candidates retire.

### WV3-007 - Linux secure-storage baseline

- Categories: credential-provider:linux-secret-service
- Decision: lossless Linux migration and persisted credentials require a
  working, unlocked Secret Service implementation. Environments without secure
  storage are outside that support boundary until another provider is approved.

### WV3-008 - Current architecture and artifact baseline

- Categories: platform-baseline
- Decision: Wails cutover inherits the architectures and artifact classes that
  the official Electron release workflow actually builds: Windows x64;
  macOS x64/arm64; Linux x64/arm64; Linux AppImage/deb/rpm/pacman and Nix wrappers.
- Boundary: exact minimum OS/WebView versions and the Linux GTK compatibility
  conflict still require the decisions listed in `release-target-matrix.md`.
- Exclusion: Windows ARM64 remains unsupported until a separate decision adds
  it with native helper, installer, updater and clean-machine evidence.

### WV3-009 - AI implementation is the final migration domain

- Categories: sequencing:ai-last
- Decision: P0-05 remains a read-only protocol audit, but production capability
  catalog/MCP/CLI/Catty/external-Agent implementation starts only after every
  non-AI FND/TERM/SSH/SFTP/NET/SYS/SYNC/PLUG target owner and non-AI release
  infrastructure slice reaches its declared verification gate.
- Boundary: final signed RC, full N-1 to N qualification and Wails default
  cutover occur only after AI exits its gate.
- Non-goal: this does not remove AI from the final Wails product.

### WV3-010 - Plugin and Agent policy owners are separate principals

- Categories: policy-owner:plugin-agent-split
- Decision: `internal/plugin/permissions` owns plugin runtime identity,
  principals, grants, secret leases, quotas and broker authorization. Deferred
  `internal/capability` owns Agent-facing capability identity, Agent placement,
  Observer/Confirm/Auto projection, MCP/CLI and Catty/global tool metadata.
- Shared boundary: both invoke the same Go application services for terminal,
  profile, filesystem, credential and network operations; neither policy owner
  imports or authorizes the other.

### WV3-011 - Windows release target floor

- Categories: release-target:windows
- Decision: the required Windows target is Windows 10 22H2 (build 19045) x64
  with the Evergreen WebView2 runtime. Installer, portable and ZIP artifact
  classes stay required. Windows 10 builds older than 22H2 are unsupported.
  Windows ARM64 and 32-bit remain unsupported, consistent with `WV3-008`.
- Recorded: 2026-09-08, product owner approval closing the `P0-01A`
  `pause-for-user` state.

### WV3-012 - macOS release target floor

- Categories: release-target:macos
- Decision: the required macOS targets are macOS 12 Monterey or newer on x64
  and arm64 with WKWebView. DMG and ZIP artifact classes stay required with
  signing/notarization when credentials are available.
- Recorded: 2026-09-08, product owner approval closing the `P0-01A`
  `pause-for-user` state.

### WV3-013 - Linux release target floor

- Categories: release-target:linux
- Decision: the required Linux targets follow the Wails v3 platform baseline:
  GTK 4.14+ / WebKitGTK with a matching glibc floor on x64 and arm64. The
  previous RHEL 8 / UOS / Deepin-era glibc 2.28 compatibility goal is retired;
  distros that cannot ship GTK 4.14+ are unsupported. AppImage, deb, rpm and
  pacman artifact classes stay required per `WV3-008`. Lossless migration and
  persisted credentials still require a working, unlocked Secret Service per
  `WV3-007`; X11 and Wayland sessions remain required desktop profiles.
- Boundary: exact minimum distro versions are certified in Phase 6 per
  `release-target-matrix.md` section 5 before any grade A claim.
- Recorded: 2026-09-08, product owner approval closing the `P0-01A`
  `pause-for-user` state.

## Required Future Decisions

P6-05 `NONAI-COMPLETE` requires accepted decisions that collectively carry every
exact category below. These categories are intentionally unresolved now:

- `release-target:windows`: freeze the Windows release target and support floor.
- `release-target:macos`: freeze the macOS release target and support floor.
- `release-target:linux`: freeze the Linux release target and support floor.
- `agent-runtime:cursor-bun`: accept or reject the Cursor embedded Bun runtime.
- `agent-runtime:opencode-bun`: accept or reject the OpenCode embedded Bun runtime.
- `agent-disposition:copilot`: retain or retire Copilot for the Wails release.
- `agent-disposition:codebuddy`: retain or retire CodeBuddy for the Wails release.
- `agent-disposition:cursor-cli`: retain or retire Cursor CLI login mode.

The gate must cite those accepted decisions in `Decision references` and bind
them into exact structured `Verification` fields:
`nonAiRows=verified; releaseTargets=<comma-separated decision IDs>;
agentDecisions=<comma-separated decision IDs>;
qualification=P6-02,P6-03,P6-04; authority=<nonempty>`. Decision ID lists use
canonical `WV3-NNN,WV3-NNN` form without spaces or duplicate IDs.
`releaseTargets` IDs must collectively carry the three `release-target:*`
categories; `agentDecisions` IDs must collectively carry all five Agent
runtime/disposition categories. Every listed ID must also appear in `Decision
references`. Loose prose, self-asserted IDs and irrelevant accepted decisions do
not satisfy the gate.

Other future category requirements:

- `rollback-window-closure`: approve actual closure evidence before P8-03 records
  `Gate: ROLLBACK-CLOSED`.
- `scope-removal:<CAPABILITY-ID>`: capability-specific approval required in the
  same ledger entry that changes that stable row from `required` to `removed`.

Create and approve a new decision before:

- reducing the platform/architecture/package target matrix;
- accepting a terminal performance regression or changing its benchmark envelope;
- retiring an existing user-visible capability or external Agent;
- adding another Linux credential provider;
- extending or closing the rollback window;
- changing the plugin execution model or runtime Node boundary.
