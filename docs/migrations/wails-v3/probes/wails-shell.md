# P0-02 Wails v3 Shell Probe

Status: `needs-verification`

This is disposable feasibility evidence for Wails `v3.0.0-beta.12`. It is not
a production shell and does not create a Go runtime owner.

## Probe Boundary

- Source: `experiments/wails-shell-probe/`
- Frontend: independent vanilla TypeScript/Vite application; it does not import
  Netcatty production React modules.
- Backend: one probe-only Wails service for four window roles, lifecycle events,
  close veto, second-instance intent, deep links and native dialog parenting.
- Excluded: profile data, credentials, terminal transport, SSH/SFTP, plugins,
  sync, updater and AI functionality.
- Pinned framework: `github.com/wailsapp/wails/v3 v3.0.0-beta.12`.
- Required Go toolchain: Go 1.25 or newer. The Windows run used automatic
  toolchain `go1.26.7` from a `go1.24.1` host installation.

The frontend includes direct smoke surfaces for xterm.js, its WebGL addon,
Monaco, IME composition events, clipboard read/write and a minimal WASM module.
Window identity is a per-run opaque token attached to the `main`, `settings`,
`session` and `popup` roles.

## Automated Evidence

Recorded on 2026-08-28:

| Check | Result | Notes |
| --- | --- | --- |
| `go test ./...` | pass | role validation, opaque tokens, deep-link parsing and bounded event history |
| `go vet ./...` | pass | probe Go packages |
| `npm run build` | pass | xterm, WebGL addon, Monaco and WASM probe bundled by Vite |
| `npm run check:wails-shell-probe` | pass | clean frontend install/build followed by Go tests; wired into CI |
| pinned `wails3 task build` | pass | Windows amd64 executable generated with beta.12 |
| primary process launch | pass | remained alive for the bounded smoke interval |
| second-instance launch | pass | second process exited with code 0 while primary remained alive |

The Wails-generated Taskfile emitted missing `uname` and `tail` diagnostics in
PowerShell, but binding generation, frontend build, resource generation and the
native Windows build all completed. The probe must always run through the
version-pinned beta.12 CLI; the host's global CLI is an older alpha build.

## Platform Matrix

| Target | Build | Native launch | Interactive shell checks | Result |
| --- | --- | --- | --- | --- |
| Windows 10 22H2 x64, build 19045 | pass | bounded smoke pass | pending manual run | C-grade only |
| macOS x64/arm64 | not run | not run | not run | pending |
| Linux x64/arm64 | not run | not run | not run | pending |

Windows WebView2 version was not available from the checked EdgeUpdate registry
keys. A successful native launch proves a usable runtime was found, but does not
freeze the WebView2 support floor.

## Manual Checklist

Run this matrix on every required target and record exact OS, architecture and
WebView version:

- Open, focus, close and recreate all four window roles.
- Enable close veto, verify close is cancelled, then disable it and close.
- Reload each window and observe a new `runtime-ready` event with the same role
  token.
- Launch a second process with
  `netcatty-wails-probe://open/session?id=manual` and verify exactly one event in
  the primary process.
- Exercise tray show, quit and `CmdOrCtrl+Shift+F12`, including shortcut conflict.
- Install/package the probe, launch its URL scheme and verify malformed schemes
  are rejected.
- Open the native dialog for each role and verify correct parent ownership.
- Enter composed CJK text, perform the clipboard round-trip, and visually verify
  xterm and Monaco are nonblank.
- Verify WebGL and WASM report `PASS`; record any renderer fallback.
- Force renderer failure where the platform exposes a supported mechanism and
  record cleanup/recreation behavior.

## Known Wails v3 Boundaries

- beta.12 exposes renderer-process termination only on macOS. Windows WebView2
  and Linux WebKit process failure are not public `pkg/application` events.
  The probe records this as a platform gap and does not add a fallback.
- Installed deep-link registration is packaging behavior; `go run` cannot prove
  it. Windows and Linux second-instance URL delivery arrives in process args.
- Linux tray behavior depends on a StatusNotifierWatcher, and Wayland global
  shortcuts depend on the desktop portal/compositor.
- The standalone Monaco dependency currently reports moderate transitive
  DOMPurify advisories. The probe renders only trusted static content and is not
  a release artifact; production dependency selection remains a later gate.

## Sources

- <https://v3.wails.io/tutorials/03-notes-vanilla/>
- <https://v3.wails.io/tutorials/04-self-update-a-wails-app/>
- <https://github.com/wailsapp/wails/tree/v3.0.0-beta.12/v3/examples/single-instance-url-scheme>

## Exit Assessment

The Windows implementation path is feasible, but P0-02 remains
`needs-verification` until the interactive Windows checklist and the macOS/Linux
matrix are complete. No Electron owner is retired and Phase 1 production shell
work remains blocked by the full Phase 0 exit gate.
