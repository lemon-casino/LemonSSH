# P0-04 Electron Secret Unseal and Go Re-Seal Probe

Status: `needs-verification`

This is disposable feasibility evidence for lossless secret migration between
the Electron `safeStorage` baseline and Go platform keyring providers. It is
not the production migration broker, does not touch real user profiles, and
must not be imported by production React or Go code.

## Probe Boundary

- Source: `experiments/profile-secret-migration/`
- Electron driver: `experiments/profile-secret-migration/electron/*.cjs`
  (corpus, length-prefixed JSONL channel, interop runner, leak scanner)
- Go process: `experiments/profile-secret-migration/` module plus
  `cmd/profile-secret-migration-probe` (30 s bounded stdin/stdout protocol)
- Providers: Windows DPAPI user-range seal/open with purpose-bound entropy
  (`provider_windows.go`); non-Windows providers are explicit fail-closed
  stubs (`provider_stub.go`) that make the Go endpoint refuse to run
- Excluded: production profile store, real user data, writer lease, profile
  promotion, Electron bridge changes, and non-Windows live keyrings

## Protocol

The Electron parent and Go child establish an authenticated ephemeral channel
before any secret byte is exchanged:

- X25519 ephemeral key exchange with a 16-byte run ID and 32-byte server
  challenge; both sides derive a transcript hash over the canonical
  length-prefixed transcript.
- Channel key: HKDF-SHA256(secret=shared, salt=challenge,
  info=`netcatty-profile-secret-migration-v1/channel-key/<transcript>`).
- Mutual HMAC-SHA256 confirmation proofs with distinct roles (`go`,
  `electron`); the Go child seals every receipt back to the parent.
- Payloads: AES-256-GCM with direction-separated nonces (`E2G1`, `G2E1`) and a
  canonical AAD binding run ID, direction, sequence, record ID, source format,
  and purpose. Wrong direction, sequence, ID, source format, or purpose fails
  authentication.
- Framing: 4-byte big-endian length-prefixed strict JSON. Stderr must stay
  empty; any stderr byte fails the run.

## Synthetic Corpus

The corpus covers every secret-bearing field class with synthetic values only:
24 `enc:v1:` credentials (host/key/identity/proxy/group/AI/sync/legacy/plugin
classes), 3 `safeStorage-raw` values (cloud master password, local backup
payload, plugin secret store value), and 4 edge cases (Unicode, multiline
synthetic private key, 128 KiB max plaintext, `enc:v1:`-prefixed plaintext).
Three classified metadata records (app-lock verifier, sync KDF config,
empty-or-absent optional) complete the 34-record inventory.

`verifyNegativeSources` rejects corrupt, truncated, header-only, foreign, and
encryption-unavailable `enc:v1:` sources before any interop traffic. The Go
side independently verifies each seal with an in-process open round-trip
before emitting a receipt.

## Close-Out Corrections (2026-09-08)

Bringing the probe to a passing state surfaced and fixed two defects:

1. Stale Go golden test vectors: the KDF and canonical AAD goldens were
   computed against an earlier, longer protocol name. Both were recomputed for
   the current `netcatty-profile-secret-migration-v1` (36-byte) name and the
   KDF value was independently reproduced with Node `crypto.hkdfSync` before
   being pinned.
2. Electron driver metadata gap: the Go corpus contract requires 31 secret
   fixtures plus 3 metadata records, but the driver never sent `metadata`
   frames, so every run failed at `finish` inventory validation. The driver now
   seals metadata with the `classified-metadata` source format, opens
   `metadata_receipt` frames via a dedicated receipt verifier, checks the
   echoed metadata round-trip, and sends the full 34-record finish count.

## Automated Evidence

```bash
go -C experiments/profile-secret-migration test ./...
go -C experiments/profile-secret-migration vet ./...
```

Go suites cover KDF/nonce/AAD determinism and direction separation, mutual
handshake confirmation and transcript binding, tampered-hello rejection, and
AES-GCM rejection of wrong direction/sequence/ID/format/purpose plus
ciphertext tampering.

Live interop (requires a Windows host with DPAPI):

```bash
go -C experiments/profile-secret-migration build -o bin/profile-secret-migration-probe.exe ./cmd/profile-secret-migration-probe
./node_modules/.bin/electron experiments/profile-secret-migration/electron/runner.cjs <absolute-binary-path>
```

The runner isolates an Electron `userData` under the dedicated Netcatty temp
directory, round-trips every fixture through `safeStorage`, drives the Go
child, scans captured stdout/stderr/argv/env and the isolated root for all 31
plaintext canaries, and deletes the isolated root before reporting.

## Windows DPAPI Smoke

Recorded on 2026-09-08 using Windows 10 22H2 x64 build 19045 and the repo
Electron runtime:

```json
{
  "formatVersion": 1,
  "platform": "win32",
  "provider": "windows-dpapi-user",
  "protocolVersion": 1,
  "fixtureCount": 31,
  "metadataCount": 3,
  "passed": true,
  "cleanup": true,
  "leakScan": true
}
```

Interrupt injection (`--interrupt=after-spawn`, `--interrupt=after-handshake`,
`--interrupt=after-first-secret`) reports `passed: false` with `cleanup: true`
and `leakScan: true`: the child is killed and reaped, no plaintext canary
appears in captured evidence, and the parent never emits a passing receipt
after an interrupted record.

## Exit Assessment

The Electron-to-Go secret handoff is feasible on this Windows host and
advances `FND-03` to `probe`. P0-04 remains `needs-verification` because the
following evidence is absent:

- macOS Keychain and Linux Secret Service live provider re-seal runs;
- non-Windows leak scans through the real keyring paths (CI currently covers
  only the fail-closed stub and Go contract suites);
- a closed P0-01A release target matrix and the Phase 0 exit gate.

The production replacement path is P2-04 (platform credential providers),
P2-05 (Electron export broker), and P2-06 (Wails import/promote/rollback);
this probe must be deleted or reduced to a harness once those owners land.
