# CLAUDE.md

LemonSSH is a Go + Wails v3 desktop application with a React/TypeScript frontend.

## Commands

```bash
npm ci
npm run dev
npm run lint
npm test
go test ./internal/... ./cmd/...
npm run build
npm run package:wails
```

Run a single frontend test with:

```bash
node --test --import tsx path/to/file.test.ts
```

Regenerate native bindings after changing exported Wails services:

```bash
go run github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.12 generate bindings -d infrastructure/runtime/wails/bindings ./cmd/netcatty
```

## Architecture

- `cmd/netcatty/`: Wails entry point and frontend-facing Go services.
- `internal/`: SSH, SFTP, terminal, capability, plugin, credential, and profile implementations.
- `domain/`: pure TypeScript models and helpers.
- `application/` and `application/state/`: orchestration and React state hooks.
- `infrastructure/`: runtime, persistence, service, AI, and generated binding adapters.
- `components/`: presentation. `App.tsx` wires hooks to views.

Frontend code uses `infrastructure/runtime/` as its native boundary. Add native behavior in Go and expose it through Wails; do not introduce another desktop bridge.

Keep storage access behind `localStorageAdapter`, keys in `storageKeys.ts`, and network calls out of components. Use `components/ui/aside-panel.tsx` for Vault aside panels.

Issues must use the repository templates and the `[Bug]`, `[Feature]`, or `[Other]` title prefixes. Pull requests follow `.github/PULL_REQUEST_TEMPLATE.md`.
