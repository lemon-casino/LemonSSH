# P3-03 ssh2 Patch Compatibility Decisions

Status: active compatibility authority for the Go SSH owner (P3-03).

The Electron shell carries `patches/ssh2+1.17.0.patch` (904 lines) against the
Node ssh2 library. Each patched area needs an explicit Go-side decision before
`sshBridge.cjs` and the patches retire (P3-04A/Gate 4). This file records the
decisions; the checker does not validate it, but Gate 4 evidence must reference
every row.

## Patched areas and Go decisions

| # | Patch area (file) | What the patch does | Go decision | Status |
| --- | --- | --- | --- | --- |
| 1 | `agent.js` — RSA cert agent signing with sha2 flags | Extends agent RSA-signing flags to OpenSSH RSA certificate key types | x/crypto `ssh.PublicKeysCallback` over `agent.Client.Signers` signs RSA certs; hash selection is native. Pinned by `TestRSACertificateSignerNegotiation` | resolved natively (test-pinned) |
| 2 | `client.js` — `getKeyAlgos` RSA cert variants | Adds `ssh-rsa-cert-v01@openssh.com` / `rsa-sha2-256-cert` / `rsa-sha2-512-cert` cases preferring sha2 | x/crypto negotiates cert variants and rsa-sha2-256/512 natively during key exchange | resolved natively (test-pinned) |
| 3 | `protocol/Protocol.js` — token whitespace tolerance | Accepts legacy servers sending whitespace-padded identification strings | Go `x/crypto/ssh` trims per RFC 4253; behaviour differences must be proven against the compatibility lab before Gate 4 | pending compatibility lab |
| 4 | `protocol/SFTP.js` — packet header spanning buffer boundary | Keeps trailing 4 bytes so a valid SFTP header split across reads is not dropped | Go `github.com/pkg/sftp` owns framing; verify with a large-transfer fuzz/lab test at P3-05 | deferred to P3-05 |
| 5 | `protocol/constants.js` — Comware compat flag | Adds `COMPAT.COMWARE_DHGEX_1024` for `Comware-` server banners | x/crypto has no per-vendor compat flags. Legacy H3C/Comware switches requiring 1024-bit DH groups fail closed unless a kex override is approved; must be exercised in the Gate 4 lab | pending compatibility lab (fail-closed today) |
| 6 | `protocol/kex.js` — DHGEX size bugs | Works around large DHGEX and Comware 1024-bit group negotiation bugs | Same as #5: x/crypto negotiates modern groups; ancient buggy servers are unsupported until the lab proves otherwise | pending compatibility lab (fail-closed today) |
| 7 | `protocol/keyParser.js` — rsa-sha2 parse cases | Accepts `rsa-sha2-256/512` key formats in parser | x/crypto parses these natively | resolved natively |

## Rules

- A row may move to `resolved` only with a named Go test or live fixture.
- Rows marked `pending compatibility lab` keep the Electron/ssh2 path as the
  compatibility baseline; Gate 4 evidence must include at least one device or
  server per pending row before the ssh2 patches retire (P3-04A/Phase 3 exit).
- Fail-closed is the default: an unsupported legacy server produces a clear
  authentication/kex error, never a silent downgrade.
