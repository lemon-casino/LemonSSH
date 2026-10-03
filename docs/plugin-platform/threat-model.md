# Plugin platform threat model (Go / Wails host)

Plugins are untrusted code. A useful plugin may parse terminal output, display
content, or ship a native companion; none of those needs imply trust in the
author's code, update server, dependencies, or account.

This document records the properties the shipped Go host enforces today and
the properties that remain unenforced because the corresponding surfaces do
not exist yet.

## Protected assets

- passwords, private keys, API keys, OTP values, and secret-setting plaintext;
- terminal input while echo is disabled or authentication is in progress;
- host addresses, usernames, notes, command history, and terminal output;
- local files and filesystem metadata outside a plugin's data directory;
- LemonSSH renderer and Go-process authority;
- other plugins' packages, storage, settings, and runtime messages;
- cloud synchronization keys and provider credentials;
- the integrity and availability of terminal sessions and the LemonSSH process.

## Adversaries

Assumed hostile: a locally installed plugin package; a plugin dependency
compromised after publication; a companion executable; remote content parsed
by a plugin; a malformed or intentionally expensive package; an old package
crafted to exploit a newer installer; an update requesting broader permissions
than the installed version.

Trusted: the operating system, the Wails/Go application package, and the user
account. A machine already controlled by malware is outside the platform's
protection boundary.

## Enforced properties

**Package validation is the only way into the inventory.**
`PluginService.InstallPackage` (`cmd/lemonssh/pluginService.go`) rejects ZIPs
with more than 512 entries, unsafe entry paths (`..`, absolute, backslashes),
and single files over 64 MiB. The manifest must be valid v2 — v1 documents are
rejected by `internal/plugin/v1reject` with no shim. The WASM entrypoint's
SHA-256 must match `entrypoint.sha256` exactly.

**Checksum-pinned execution.** Enabling a plugin re-reads the stored `.ncpkg`,
re-validates it and re-instantiates WASM from those exact bytes
(`restoreEnabledPackages` disables a package that no longer parses instead of
executing it). The inventory stores the validated manifest snapshot; later
decisions (permissions, UI) read the snapshot, not user-controlled files.

**WASM sandbox with broker-gated imports.** `internal/plugin/wasm` runs
modules in wazero with WASI disabled and `WithCloseOnContextDone(true)`. The
manifest's `entrypoint.memoryMB` becomes a hard linear-memory cap in a
dedicated runtime. The only RPC path into a module is the lemonssh-wasm-abi
v1 dispatch channel: JSON envelopes capped at 1 MiB on both sides, a 10 s
default deadline that aborts the guest, per-module serialization, a panic
recovery guard, and allocation exhaustion surfaced as a structured error.
The two `lemonssh_host_*` imports (log, own-settings read) are the only
host-visible capabilities and every call passes the fail-closed broker with
a manifest-declared `runtime` permission; a missing grant returns the
structured permission-denied status to the guest.

**Fail-closed permissions.** `internal/plugin/permissions` grants are keyed by
plugin ID plus capability and exist only for resources the validated manifest
declares (`internal/plugin/host`). Disabling or uninstalling revokes every
grant. Native companion spawns additionally require a session-scoped
`companion.execute` grant and a manifest-matching, hash-verified binary
(`internal/plugin/native` forbids `node` interpreters/shebang wrappers, caps
stdout at 8 MiB and RPC frames at 1 MiB, and quarantines the plugin on
containment failure — process groups on POSIX, job objects on Windows).

**Secrets never stored in plaintext.** Password settings are sealed with the OS
keyring (`internal/platform/credentials`, go-keyring). When the keyring is
unavailable the write fails; there is no plaintext or localStorage fallback.

**Renderer isolation.** The renderer reaches the host only through generated
Wails bindings for `PluginService`; permission grants and native spawns are
trusted host UI entrypoints, never callable by plugin code. Plugin UI is
declarative and injection-checked (`internal/plugin/ui` rejects script/HTML
vectors in user-visible text) and rendered by LemonSSH's own components.

## Not yet enforceable (surface absent)

- CallPlugin accepts any enabled plugin's dispatch method without a
  user-facing approval; the privilege boundary is the broker-gated host
  imports inside the call. Future surfaces that carry user data beyond the
  provider session snapshot must add their own per-call approval flows.
- Publisher signatures / distribution trust are not implemented; today's trust
  anchor is the manifest checksum at install time.

## Provider surfaces (implemented, broker-gated)

Terminal and extension providers (terminal completion…theme, connection,
authentication, importer, sync) are live: a plugin declares providers over
the dispatch channel (`providers.list`) and the registry
(`internal/plugin/providers`) serves them to the renderer only while the
fail-closed broker holds the manifest-derived
`["provider","<kind>"]:read` grant (see
[terminal-providers.md](./terminal-providers.md)). The grant is created only
by the trusted `GrantPermission` approval path, re-checked on every
enumeration and invocation, and revoked on disable/uninstall — so a plugin
can neither claim a kind its manifest never declared nor keep contributing
after a revoke. Provider invocations deliver an immutable session metadata
snapshot (IDs, protocol, status, cwd, title, dimensions — never terminal
buffers, secrets or password content), are deadline-capped, and the
privileged `terminal.interceptor.*` kinds are rejected by the registry.

## Package attacks (reference)

The archive-level defenses still apply and are exercised by tests: path
traversal and aliasing are rejected (`safePluginArchivePath`), resource
exhaustion is bounded by entry/file/byte limits, and the CLI's package writer
(`@lemonssh/plugin-cli`) revalidates a deterministic ZIP whose content digest
binds every path, size, mode and file hash — so "validate one bytes, install
another" repacking fails checksum comparison at install time.
