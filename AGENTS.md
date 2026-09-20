# AGENTS.md

## Project Architecture

LemonSSH is a Go + Wails v3 desktop application with a React/TypeScript frontend.
Keep the dependency flow one way:

- `domain/`: pure models and helpers.
- `application/` and `application/state/`: orchestration, React hooks, and persistence boundaries.
- `infrastructure/`: adapters for Wails services, storage, networking, AI, and generated bindings.
- `components/` and `App.tsx`: presentation and view wiring.
- `cmd/netcatty/`: Wails application entry point and services exposed to the frontend.
- `internal/`: Go implementations for SSH, SFTP, terminal sessions, capabilities, plugins, credentials, and profile storage.

Components must not call native APIs, network services, or persistence directly. Add a typed adapter or application hook first. Keep storage keys in `infrastructure/config/storageKeys.ts` and use `infrastructure/persistence/localStorageAdapter.ts`.

## Runtime Boundary

The frontend uses the runtime client under `infrastructure/runtime/`. Wails is the only desktop runtime. Native capabilities belong in Go services under `cmd/netcatty/` or `internal/`; expose them through generated Wails bindings and the runtime adapter.

Run this after changing exported Wails service methods:

```bash
go run github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.12 generate bindings -d infrastructure/runtime/wails/bindings ./cmd/netcatty
```

Capability metadata is generated from the Go catalog:

```bash
npm run generate:capability-tools
```

Do not add a second native bridge shape when an existing Wails service or runtime port can be extended.

## AI Agent Harness

`infrastructure/ai/harness/` owns turn orchestration. `AgentRuntime` controls lifecycle, cancellation, trace fan-out, compaction, tool output storage, and deduplication. UI hooks manage view state and delegate turns to the runtime. Stop requests always go through `stopAgentTurn()`.

The capability catalog in `internal/capability/` is the source of truth for generated frontend tool specs. Observer mode blocks writes; confirm mode requests approval for write capabilities.

Tool dispatch, session scope, native approvals and cancellation flow through `AgentHost.dispatch`. Vault tools use the application hook through `AgentVaultRouter`; do not duplicate vault mutations in Go. See `docs/agent-tools.md`. Run `go test ./cmd/netcatty -run TestEveryAdvertisedNativeToolHasAllRPCSurfaces` when changing the catalog or registration.

## Plugin Runtime

The migrated plugin host lives under `internal/plugin/` and is exposed through Wails services. Contract and package types come from `packages/plugin-contract/`. Keep protocol changes compatible across the Go host, generated schema, SDK, and examples.

Use:

```bash
npm run check:plugin-contract
npm run test:plugin-runtime
```

## Temporary Files

Use Netcatty's dedicated temporary-file service. Do not write application temporary data directly to the operating system temp directory.

## Terminal Side Panels

- `domain/sidePanelLayout.ts` owns pure split-tree operations.
- `application/state/useTerminalSidePanelLayoutState.ts` owns per-terminal layouts.
- `TerminalLayerSidePanelSection.tsx` renders the shared toolbar and pane tree.
- Mounted tool panels use stable portal nodes and the hidden parking host.
- Closing a side panel clears that terminal's layout; switching terminals must not reuse another terminal's tree.

## Aside Panels

Vault subpages use `components/ui/aside-panel.tsx`. Pass `title` to `AsidePanel`; do not also render `AsidePanelHeader`. Render the panel at the section root inside a positioned parent because it uses absolute positioning. Use `SelectHostPanel` for host selection.

## Testing

```bash
npm run lint
npm test
go test ./internal/... ./cmd/...
npm run build
```

Add unit tests for domain logic and Go services when behavior changes. Add hook tests for stateful frontend behavior. When changing stored schemas, include backward-compatible handling.

## Packaging

```bash
npm run wails:helpers
npm run package:wails
```

Wails packages are the repository's release artifacts. Platform signing is optional distribution metadata and is not a source or release gate.

## Issues and Pull Requests

Issue titles must start with `[Bug]`, `[Feature]`, or `[Other]` and use the required template in `.github/ISSUE_TEMPLATE/`. Pull requests must follow `.github/PULL_REQUEST_TEMPLATE.md`. See `CONTRIBUTING.md` for the full workflow.
