# Wails v3 and Go Migration

Status: approved design and implementation plan, implementation not started

This directory is the canonical entry point for Netcatty's migration from
Electron and runtime Node.js to Wails v3 and Go. Future AI agents and human
contributors must read this file before changing either runtime.

## Goal

Ship one Wails v3 desktop application for Windows, macOS, and Linux whose
runtime services are implemented in Go. React, TypeScript, xterm.js, and Monaco
remain frontend technologies. Node.js may remain in the frontend build toolchain
but must not be packaged or launched at runtime.

The migration must preserve user-visible behavior, user data, security
boundaries, terminal performance, and supported desktop platforms. The final
release must not contain an Electron runtime, a Node sidecar, a JavaScript
plugin runtime, or a compatibility path that invokes Node.

## Approved Decisions

1. Use a controlled dual-shell migration. Electron remains the stable baseline
   while Wails capabilities are developed and verified.
2. Keep one canonical owner per capability. Dual shells do not justify two
   independent implementations of domain state, policy, persistence, or
   business rules.
3. Switch the default release only when Windows, macOS, and Linux all satisfy
   the release gates.
4. Migrate existing profiles in place without data loss. Electron and Wails
   must never write the same profile concurrently.
5. The final runtime contains no Node.js. External AI integrations survive only
   through stable HTTP, CLI, stdio, or other documented protocols.
6. The plugin platform does not preserve runtime compatibility with the
   unpublished `0.1.0-internal` JavaScript/Node plugin format.
7. Plugin v2 uses WASM for ordinary plugins, host-rendered declarative UI, and
   isolated native child processes only where WASM is insufficient.
8. Terminal byte traffic uses a dedicated bounded data plane. Wails JSON calls
   and events are control-plane mechanisms, not the terminal hot path.
9. Every completed replacement must update this documentation set. Code without
   the required documentation update is not considered migrated.
10. AI production implementation is last. P0-05 may audit protocols early, but
    Agent catalog/MCP/CLI/Catty/vendor adapters wait for the non-AI completion
    gate. Final RC and cutover wait for AI completion.

## Document Map

| Document | Authority |
| --- | --- |
| [architecture.md](./architecture.md) | Target architecture, ownership, invariants, compatibility and retirement rules |
| [capability-matrix.md](./capability-matrix.md) | Per-capability migration status, owner, blockers and completion evidence |
| [verification-gates.md](./verification-gates.md) | Required contract, data, security, performance, platform and release evidence |
| [release-target-matrix.md](./release-target-matrix.md) | Current Electron release facts, Wails required targets, exclusions and unresolved support decisions |
| [implementation-plan.md](./implementation-plan.md) | Ordered migration phases, executable work slices, dependencies and phase exit gates |
| [decisions.md](./decisions.md) | Approved product and architecture decisions with stable IDs for plans and ledger entries |
| [migration-ledger.md](./migration-ledger.md) | Append-only record of completed or blocked migration slices |
| [templates/slice-record.md](./templates/slice-record.md) | Required documentation record for each replacement slice |

The existing plugin documents under `docs/plugin-platform/` describe the
Electron implementation and the security properties that plugin v2 must
preserve. They remain current-state evidence, not the target runtime design.

`capability-matrix.md` and the append-only `migration-ledger.md` are the sole
status and resume authority. Status and next-action prose in the implementation
plan or other documents is a navigation hint only and cannot override them.

## Current State

- Current release shell: Electron.
- Target release shell: Wails v3.
- Current runtime languages: Node.js/CommonJS plus React/TypeScript.
- Target runtime languages: Go plus React/TypeScript frontend assets.
- Wails/Go production scaffold: not present; disposable P0-02 shell and P0-03
  terminal data-plane probes have Windows C-grade evidence.
- Active migration phase: Phase 0, baseline and feasibility probes.
- Next non-AI action: finish the P0-02/P0-03 three-platform and formal benchmark
  matrices; P0-04 may proceed independently as a disposable probe.
- Open Phase 0 boundaries: `P0-01` remains `needs-verification` and `P0-01A`
  remains `pause-for-user`; P0-02/P0-03 remain `needs-verification`, and P0-04
  has not started. No production Go owner may start until the Phase 0 exit gate
  closes.
- AI boundary: `P0-05` remains a read-only `needs-verification` audit and
  `AI-04` remains `not-started`. Production AI work is blocked until the
  P6-05 `NONAI-COMPLETE` ledger gate before Phase 7.

## Mandatory AI Workflow

Before starting a migration slice:

1. Read this file, `architecture.md`, the relevant rows in
   `capability-matrix.md`, and the applicable sections of
   `verification-gates.md`.
2. Read the current Electron owner and its tests. Do not infer behavior only
   from bridge method names.
3. Identify the target Go owner, frontend adapter, data compatibility boundary,
   and Electron retirement target.
4. Record a bounded slice with explicit non-goals and verification.
5. Do not introduce an unplanned fallback, duplicate owner, or permanent
   compatibility adapter.

After implementing a slice:

1. Run the required tests and platform checks.
2. Update the matching `capability-matrix.md` row.
3. Append a record to `migration-ledger.md` using
   `templates/slice-record.md`.
4. Update `implementation-plan.md` status and next action when that document
   exists.
5. Update `architecture.md` only when an approved architecture decision changes;
   never rewrite the baseline merely to match implementation drift.
6. Delete the replaced Electron owner, or record one precise retirement trigger
   and the reason immediate deletion is unsafe.

## Status Vocabulary

Use only these capability states:

- `not-started`: no maintained Go replacement exists.
- `probe`: disposable feasibility evidence exists; no production owner exists.
- `implemented`: a maintained Go implementation exists but parity evidence or
  retirement is incomplete.
- `verified`: required automated and platform evidence passes, but the old
  owner may still be active for controlled cutover.
- `migrated`: Go is the canonical owner, documentation is updated, and the
  corresponding Electron path is removed or disabled behind an approved,
  time-bounded rollback boundary.
- `blocked`: a documented dependency or failed gate prevents progress.
- `retired`: capability was deliberately removed from the target product with
  an approved product decision.

`implemented` is not `migrated`. A row cannot move to `migrated` without a
ledger record and retirement evidence.

During dual-shell development, distinguish these owner states:

- `target-owner`: the Go owner to which all new target-runtime behavior belongs.
- `release-carrier`: the frozen Electron implementation still serving the
  current stable release.
- `rollback-carrier`: a read-only or export-only old path retained for the
  bounded rollback window.

The release carrier is not allowed to become a second evolving source of truth.
After the Go target owner is verified, Electron receives only release-blocking
repairs, and each repair must include a Go parity assessment. Repository deletion
occurs when the release or rollback carrier reaches its recorded trigger.

## Global Completion Conditions

The migration is complete only when all of the following are true:

1. Every required leaf row in `capability-matrix.md` is `migrated`; aggregate
   rows are excluded. A capability may leave completion scope only through an
   earlier accepted decision and ledger record that changes its scope to
   `removed` while preserving the stable row at `retired`.
2. All gates in `verification-gates.md` pass on supported targets.
3. Existing profiles migrate with backup, verification, atomic cutover, and
   tested rollback.
4. The packaged application contains no Electron, Node runtime, runtime
   `node_modules`, `.asar`, or Node plugin bootstrap.
5. No child process command invokes `node`.
6. Windows, macOS, and Linux release artifacts pass install, launch, terminal,
   SSH, SFTP, credential, deep-link, update, and uninstall smoke tests.
7. Electron code, packaging, dependencies, and compatibility-only adapters are
   removed after the rollback window closes.
