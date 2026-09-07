# Netcatty Wails Shell Probe

Disposable P0-02 feasibility probe for Wails v3 shell behavior. Nothing under
this directory is a production runtime owner, and production React code must not
import it.

The module pins Wails `v3.0.0-beta.12` and requires Go 1.25 or newer. Use the
version-pinned CLI so a globally installed alpha CLI cannot alter the result:

```bash
go test ./...
npm --prefix frontend ci
npm --prefix frontend run build
go run github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.12 task build
```

The Taskfiles route every nested Wails command through that same pinned module
version; a globally installed `wails3` binary is not used.

Run the application with the same pinned CLI and complete the manual matrix in
`docs/migrations/wails-v3/probes/wails-shell.md` on each target platform.
