# Bundled `mosh-client` (MoshCatty)

This directory holds the pure Rust `mosh-client` from
[binaricat/MoshCatty](https://github.com/binaricat/MoshCatty).

Netcatty runs SSH + `mosh-server` bootstrap itself, then launches this binary
(see `electron/bridges/moshHandshake.cjs` and `terminalBridge/moshSession.cjs`).

## Layout

| Target | Release asset | Local path |
|--------|---------------|------------|
| Linux x64 | `mosh-client-linux-x64.tar.gz` | `linux-x64/mosh-client` |
| Linux arm64 | `mosh-client-linux-arm64.tar.gz` | `linux-arm64/mosh-client` |
| macOS universal | `mosh-client-darwin-universal.tar.gz` | `darwin-universal/mosh-client` |
| Windows x64 | `mosh-client-win32-x64.tar.gz` | `win32-x64/mosh-client.exe` |

Each tarball contains **only** the client binary (no Cygwin DLLs, no terminfo).
Windows builds static-link the MSVC CRT (`moshcatty-0.1.1+`).

Release tags: `moshcatty-*` (require `moshcatty-0.1.8+`) from
`binaricat/MoshCatty`, with `SHA256SUMS`.

### Linux glibc floors

Linux assets must start on the same distros Netcatty packages for. From
`moshcatty-0.1.2`, MoshCatty builds Linux clients on:

| Target | Build image | Max required GLIBC |
|--------|-------------|--------------------|
| `linux-x64` | AlmaLinux 8 | 2.28 |
| `linux-arm64` | Debian bullseye | 2.31 |

Do **not** pin packaging to `moshcatty-0.1.0` / `0.1.1` Linux binaries: those
were built on Ubuntu runners and require GLIBC 2.34.

## Fetch

```sh
# Optional pin (0.1.8+ disables unsafe local backspace prediction)
export MOSH_BIN_RELEASE=moshcatty-0.1.8
npm run fetch:mosh

# Dev: host platform; resolves latest moshcatty-* if unset
npm run fetch:mosh:dev
```

Env: `MOSH_BIN_OWNER` / `MOSH_BIN_REPO` (default `binaricat` / `MoshCatty`),
`MOSH_BIN_BASE_URL` for mirrors.

`electron-builder` packages `Resources/mosh/mosh-client[.exe]` only.

### Wails locked supply

Wails packaging uses [fetch-wails-helpers.lock.json](../../scripts/fetch-wails-helpers.lock.json)
and [fetch-wails-helpers.mjs](../../scripts/fetch-wails-helpers.mjs). The lock pins
`binaricat/MoshCatty` release `moshcatty-0.1.8`, source commit
`554b9d305e7ac4b11de740d764bbc3e05f816d7b`, archive and executable SHA256 values,
GitHub asset IDs, and the
[release workflow](https://github.com/binaricat/MoshCatty/actions/runs/29559896639).
Archive digests were checked against both GitHub asset metadata and the pinned
upstream `SHA256SUMS`. Executable digests were derived from those verified
archives, never from an unknown local binary.

```sh
# Fetch both locked Mosh and ET for the native host; Node 22+ and tar required.
node scripts/fetch-wails-helpers.mjs

# Fetch all eight release assets, including both macOS universal executables.
node scripts/fetch-wails-helpers.mjs --all
node scripts/fetch-wails-helpers.mjs --all --verify-only

# Target-specific supply; run the Darwin package command on macOS.
node scripts/fetch-wails-helpers.mjs --goos darwin --goarch arm64
node scripts/package-wails.mjs --goos darwin --goarch arm64 --out-dir dist/wails-darwin-arm64
```

`package-wails.mjs` fetches locked supply automatically before building. It emits
each executable beside its `.manifest.json`, copies licenses and a copy of the
lock, and includes them in recursive checksums and `installer-resources.json`.
The macOS executable stays universal; the packaged runtime sidecar uses the
application's exact GOARCH (`amd64` or `arm64`). Both Mach-O slice headers,
bounds, CPU types, and hashes are checked. CI additionally runs `lipo`.
Wails' macOS application build requires the native macOS CGO toolchain; the
helper validation can run on Windows, but `CGO_ENABLED=0` does not produce a
working macOS Wails application cross-build.

The resource sidecar uses the requested architecture when first created
(native architecture by default). Packaging generates its own target sidecar
without rewriting the resource sidecar. Do not commit generated binaries or
resource sidecars; the committed lock and fetch step reproduce them on each
runner. Existing matching files are reused without rewriting. Conflicting
files, forged sidecars, symlinks, hardlinks, unexpected archive members and hash
mismatches fail closed; use a separate `--resources-dir` for another supply
instead of overwriting an existing local helper.

Downloads are cached by SHA256 under `build/wails-helper-cache` and reverified on
every use. `--cache-dir` selects another cache. For hosts where Node HTTPS is
unavailable, explicitly set `WAILS_HELPER_DOWNLOAD=gh` to use an authenticated
GitHub CLI with the same pins. Neither transport honors the legacy
`MOSH_BIN_*`/`ET_BIN_*` overrides or unverified-download switches. The legacy
fetch commands above remain available for Electron/dev use; their output alone
is not Wails release provenance.

The cache stays outside Vite's `dist` directory. Helpers are copied to the
package output after the frontend build, so clearing frontend output cannot
remove the verified supply or silently omit it from the package.

Offline unit/regression checks:

```sh
node --test scripts/package-wails.test.mjs scripts/fetch-wails-helpers.test.mjs scripts/fetch-mosh-binaries.test.cjs scripts/fetch-et-binaries.test.cjs
```

After `--all`, set `WAILS_HELPER_TEST_LOCAL=1` and run that command again for real
binary packaging checks on all five native targets. Set
`WAILS_HELPER_TEST_HOST=1` to test only the fetched host target. These checks do
not establish signed/native session acceptance; Mosh/ET connection, input,
resize and reconnect acceptance remains manual.

## Licenses

- MoshCatty client: **GPL-3.0-or-later**
- Upstream Mosh protocol reference: **GPL-3.0**
- Netcatty is **GPL-3.0**
