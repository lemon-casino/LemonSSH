# Bundled EternalTerminal `et` client

LemonSSH packages a reviewed EternalTerminal client for each supported Wails target. The Go terminal service launches the packaged helper directly; it does not fall back to a system-installed executable.

The source of truth is [`scripts/fetch-wails-helpers.lock.json`](../../scripts/fetch-wails-helpers.lock.json). It pins the release, source and build provenance, archive hashes, individual file hashes, licenses, asset IDs, and supported architectures. [`scripts/fetch-wails-helpers.mjs`](../../scripts/fetch-wails-helpers.mjs) downloads and verifies the locked supply.

```sh
# Fetch the host target
node scripts/fetch-wails-helpers.mjs

# Fetch and verify every supported target
node scripts/fetch-wails-helpers.mjs --all
node scripts/fetch-wails-helpers.mjs --all --verify-only
```

The current ET pin is built from EternalTerminal `et-v6.2.10`. Windows packages include `et.exe` and any files explicitly listed in the lock. macOS uses a universal binary with x86_64 and arm64 slices. License files are fetched from commit-pinned URLs and packaged under `licenses/et/`.

The build workflow in `.github/workflows/build-et-binaries.yml` can reproduce and publish reviewed helper archives. Packaging consumes only the committed lock, never a mutable latest release.

EternalTerminal is Apache-2.0 licensed. Exact dependency notices and digests are recorded in the helper lock.
