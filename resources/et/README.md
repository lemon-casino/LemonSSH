# Bundled EternalTerminal `et` client

This directory holds the EternalTerminal **client** binary (`et`) bundled
with the Netcatty installer. Netcatty launches this bundled `et` directly
(see `electron/bridges/terminalBridge/etSession.cjs`); `et` performs its
own SSH bootstrap and EternalTerminal protocol handshake against the remote
`etserver` / `etterminal`.

Like MoshCatty `mosh-client`, `et` is a pure network-transport client and does not
render a terminal locally, so there is **no terminfo bundle** here — only the
single `et` (`et.exe` on Windows) binary.

## How binaries land here

1. `.github/workflows/build-et-binaries.yml` builds `et` on relevant
   pushes/PRs, or on a manual `workflow_dispatch`. It uses
   `scripts/build-et/build-linux.sh` and `scripts/build-et/build-macos.sh`
   for Linux/macOS, and `scripts/build-et/build-windows.ps1` for Windows:

   | target            | provenance                                                       |
   |-------------------|------------------------------------------------------------------|
   | `linux-x64`       | upstream source, manylinux2014, vcpkg static deps + glibc        |
   | `linux-arm64`     | upstream source, manylinux2014, vcpkg static deps + glibc        |
   | `darwin-universal`| upstream source, lipo arm64 + x86_64, macOS system dylibs only   |
   | `win32-x64`       | upstream source, MSVC + vcpkg `x64-windows-static` (no DLLs)     |
   | `win32-arm64`     | (not built — add after a tested arm64 client is available)       |

   ET builds with CMake + Ninja + vcpkg
   (`cmake -DDISABLE_TELEMETRY=ON -GNinja -DCMAKE_BUILD_TYPE=RelWithDebInfo`).

2. When manually dispatched with `release_tag`, that workflow publishes the
   binaries to the dedicated `binaricat/Netcatty-et-bin` repository. The
   release gets a tag like `et-bin-6.2.10-1`, with `SHA256SUMS` attached.

3. Release packaging runs `scripts/resolve-et-bin-release.cjs` before
   `npm run fetch:et`. It uses an explicit workflow input first, then the
   `ET_BIN_RELEASE` repository variable, then the latest non-draft
   `et-bin-*` GitHub Release from the dedicated binary repository. The fetch
   step pulls the binaries into `resources/et/<platform-arch>/`. For local
   packaging, set `ET_BIN_RELEASE` yourself before running the same fetch
   command. Override `ET_BIN_OWNER` / `ET_BIN_REPO` only when testing a
   different binary repository. `electron-builder.config.cjs` then copies the
   matching binary into `Resources/et/et[.exe]`.

   Local dev uses the same binary path: `npm run dev` runs
   `npm run fetch:et:dev` first, which downloads the host platform's bundled
   `et` into this gitignored directory. Netcatty does not fall back to a
   system-installed `et`; if the bundled binary is missing, ET startup fails
   loudly instead of using whatever happens to be installed on the developer
   machine.

The directory is otherwise empty (binaries are gitignored).

### Wails locked supply

Wails uses the shared [helper lock](../../scripts/fetch-wails-helpers.lock.json)
and [supply script](../../scripts/fetch-wails-helpers.mjs), not latest-release
resolution. See [Mosh's Wails instructions](../mosh/README.md#wails-locked-supply)
for fetch, verification, transport and test commands.

The pin is `binaricat/Netcatty-et-bin` release `et-bin-6.2.10-1`, built from
`MisterTea/EternalTerminal` tag `et-v6.2.10` at
`f9a584ac06b2f1730b5bdf0a27150f28478368fb`. The published
[BUILD-PROVENANCE.json](https://github.com/binaricat/Netcatty-et-bin/releases/download/et-bin-6.2.10-1/BUILD-PROVENANCE.json)
links the [build run](https://github.com/binaricat/Netcatty/actions/runs/26945446872)
and Netcatty build-script checkout `c39793d592db59da68a1b4d6eaaf001620ad9464`.
The lock pins that provenance file, `SHA256SUMS`, every archive and each binary.
Fetching checks provenance fields against the lock as well as verifying hashes.
This is publisher provenance, not a signed attestation or proof of byte-identical
source rebuilds.

The actual Windows x64 release archive contains only `et.exe`. Its PE import
table references `WS2_32`, `SHLWAPI`, `dbghelp`, `CRYPT32`, `KERNEL32`, `USER32`,
`SHELL32`, `ole32` and `ADVAPI32` system DLLs. No VC++ redistributable or private
DLL bundle is needed for this pin. If a future reviewed archive includes DLLs,
each must appear in the locked file inventory with its own digest; Wails copies
them beside `et.exe` and validates their architecture. Unlisted members fail.

The macOS archive contains an actual universal Mach-O `et`, with x86_64 and
arm64 slices. Both slices link only `/usr/lib` and `/System/Library` libraries.
Wails keeps both slices and writes a runtime manifest for the application's
native GOARCH. Cross-packaging checks bytes and layout; macOS execution and
session acceptance still require a Mac.

The release archives omit license files. Wails therefore fetches 18 pinned
source/dependency license texts into its verified cache and packages them under
`licenses/et/`. URLs pin source commits; SHA256 values live in the shared lock.
The dependency versions come from upstream's vendored vcpkg ports, including
protobuf/utf8-range, OpenSSL, libsodium, abseil, zlib, cpp-httplib, cxxopts,
nlohmann-json, simpleini and brotli. Bundled source notices cover PlatformFolders,
ThreadPool, UniversalStacktrace, base64, easyloggingpp and sole. Package checksums
and installer resource mappings include all these files and the runtime sidecar.

## Licenses

- EternalTerminal is licensed under **Apache-2.0**
  (https://github.com/MisterTea/EternalTerminal).
- Netcatty is **GPL-3.0**; Apache-2.0 is one-way compatible with GPL-3.0, so
  redistribution as part of the installer is permitted.
- vcpkg-managed dependencies include libsodium (ISC), protobuf (BSD-3-Clause),
  OpenSSL/abseil (Apache-2.0), zlib (Zlib), and the MIT-licensed header libraries.
  Exact source versions and license-file digests for Wails are in the helper lock.

## Reproducible build

To reproduce the Linux binary locally:

```sh
docker run --rm -v $PWD:/workspace -w /workspace \
  -e ET_REF=et-v6.2.10 -e ARCH=x64 -e OUT_DIR=/workspace/out \
  quay.io/pypa/manylinux2014_x86_64 \
  bash scripts/build-et/build-linux.sh
```

For macOS the build needs an Xcode toolchain; see
`scripts/build-et/build-macos.sh`. For Windows see
`scripts/build-et/build-windows.ps1`.

## Roadmap

- Add Windows arm64 only after a tested standalone arm64 client is available.
- Make `ET_REF` track upstream release tags automatically.
