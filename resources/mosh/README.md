# Bundled `mosh-client` (MoshCatty)

LemonSSH packages the pure Rust `mosh-client` from [MoshCatty](https://github.com/binaricat/MoshCatty). The Go terminal service performs SSH and `mosh-server` bootstrap, then launches the verified helper.

The source of truth is [`scripts/fetch-wails-helpers.lock.json`](../../scripts/fetch-wails-helpers.lock.json). It pins release provenance, archive and executable hashes, asset IDs, architectures, and license files. [`scripts/fetch-wails-helpers.mjs`](../../scripts/fetch-wails-helpers.mjs) fetches and verifies the supply.

```sh
# Fetch the host target
node scripts/fetch-wails-helpers.mjs

# Fetch and verify all supported targets
node scripts/fetch-wails-helpers.mjs --all
node scripts/fetch-wails-helpers.mjs --all --verify-only

# Package a selected target
node scripts/package-wails.mjs --goos darwin --goarch arm64 --out-dir dist/wails-darwin-arm64
```

Supported layouts are Linux amd64/arm64, macOS universal, and Windows amd64. The helper cache lives under `build/wails-helper-cache`; every read is hash checked. Packaging copies the executable, runtime manifest, lock, and licenses into the Wails artifact. Symlinks, hardlinks, unexpected archive members, architecture mismatches, and digest mismatches fail closed.

MoshCatty and LemonSSH are GPL-3.0-or-later licensed.
