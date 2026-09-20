# Agent tools

The Go catalog in `internal/capability/` defines 71 native capabilities and
six renderer-local harness tools. Sidebar placement excludes `host_open`;
MCP and the global tool set retain it. Web search is exposed only when configured.

`AgentHost.dispatch` is the shared native entry point for Wails, CLI, MCP and
the Go provider driver. It owns permissions, grants, chat cancellation and
session scope checks. Each builtin, global and public RPC alias is registered.

- Terminal execution and background jobs use the terminal service. Local
  execution preserves the configured shell, working directory and environment.
  Job polling uses bounded UTF-16 pages and returns a continuation offset.
- SFTP tools lease a subsystem on the scoped terminal's existing SSH transport.
  Writes, transfers, rename, delete, mkdir and chmod use the existing SFTP owner.
- Hosts, notes, groups, snippets, scripts and port-forwarding rule operations
  reach the application vault handler through correlated Wails events. Queued
  requests recheck cancellation before changing application state.
- Attachment registration carries inline content as well as file metadata.
- Observer denies writes; confirm requests one host approval; auto executes
  within scope. Stopping a chat cancels pending approvals, foreground commands
  and that chat's background jobs.

The six harness tools (workspace information, session information, terminal
context, saved output, web search and URL fetch) execute in the renderer.

Wails packages include `netcatty-tool` and `netcatty-mcp` (with `.exe` on
Windows). First-party launchers pass `NETCATTY_TOOL_CLI_DISCOVERY_FILE` pointing
to the running application's discovery file. These binaries contain no
Electron or Node runtime.

Settings can enable external MCP with a separate revocable token and discovery
file. Client configuration uses `NETCATTY_EXTERNAL_MCP_DISCOVERY_FILE`.
Disabling external access cancels that scope without affecting first-party
tools. Temporary access and agent-created sessions expire after their configured
idle periods; the user's existing terminals are retained.

Run the handler/surface coverage check with:

```sh
go test ./cmd/netcatty -run TestEveryAdvertisedNativeToolHasAllRPCSurfaces -v
```

Behavior tests cover scope isolation, native session mapping, vault dispatch,
write denial, cancellation, Unicode polling, CLI arguments and MCP errors.
Adding a catalog entry requires both its handler and all declared aliases.
