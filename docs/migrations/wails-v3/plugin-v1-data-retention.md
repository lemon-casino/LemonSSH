# P2-01A Plugin v1 User-Data Retention Contract

Status: frozen. This contract binds Phase 2 profile cutover (P2-05/P2-06) and
the Phase 5 plugin v2 runtime (P5-02/P5-06).

Per the approved decisions, plugin v2 (`WV3-005`) breaks runtime
compatibility: unpublished JavaScript/Node plugin entrypoints (`main.browser`,
`main.node`) never execute in the target runtime. This contract defines how
every byte of existing plugin data survives that break.

## Source of truth

- v1 host: `electron/plugins/` (PackageStore, PluginManager, Database,
  SecretStore), SQLite database at `plugins.sqlite`, schema version 3
- Fixture: `testdata/migration/electron/plugin-v1-data-retention.json`
- Check: `npm run check:plugin-retention` fails when the database schema gains
  a table that this contract does not classify

## Table classification

Every database table has exactly one disposition:

| Disposition | Meaning |
| --- | --- |
| `preserve-metadata` | Retained verbatim as disabled metadata; never executed |
| `preserve-opaque` | Retained inside an opaque preservation envelope; Phase 2 does not interpret v2 semantics |
| `invalidate-grants` | Security state that defaults to invalid in the target |
| `re-seal-secrets` | Ciphertext moved only through the migration broker (P2-05) |
| `drop-runtime-state` | v1 runtime lifecycle state with no meaning in the target |

| Table | Kind | Disposition |
| --- | --- | --- |
| `plugins` | installed inventory | `preserve-metadata` (enabled flag forced false) |
| `plugin_versions` | installed inventory (`manifest_json`, `archive_sha256`) | `preserve-metadata` |
| `plugin_runtime_state` | v1 runtime lifecycle | `drop-runtime-state` |
| `plugin_crashes` | v1 runtime history | `drop-runtime-state` (disabled metadata implies non-runnable) |
| `plugin_kv` | user-owned state | `preserve-opaque` |
| `plugin_settings` | user-owned state | `preserve-opaque` |
| `plugin_view_state` | user-owned state | `preserve-opaque` |
| `plugin_permission_grants` | security state | `invalidate-grants` |
| `plugin_secrets` | security state (`safeStorage` ciphertext) | `re-seal-secrets` |
| `plugin_security_audit` | security history | `preserve-opaque` |
| `plugin_sync_sidecars` | sync sidecars (`settings`, `account_baseline`, `crdt_baseline`) | `preserve-opaque` |
| `plugin_sync_provider_bindings` | security state | `invalidate-grants` |

Package code (the immutable `staging/` + published archives behind
`PackageStore`) is never copied into a runnable location in the target; the
`plugin_versions` rows are enough to show the user what was installed.

## Preservation envelope

`preserve-opaque` tables migrate row-for-row into the Go profile store under
the namespace `plugin-v1/<plugin_id>/<table>/`, wrapped in a preservation
record that carries:

- `namespace` (above),
- `row_count`,
- `semantic_hash`: SHA-256 over the sorted canonical JSON of all rows,
- `schema_version` (3) the rows originated from.

Phase 2 equality checks (P2-06) compare these records before and after
promotion. Nothing in Phase 2 needs to understand what the rows mean for
plugin v2; the envelope is the interface.

## Security state

- **Grants are never inherited.** `plugin_permission_grants` and
  `plugin_sync_provider_bindings` do not carry authority into the target. A v2
  plugin re-requests permissions under the v2 principal model
  (`internal/plugin/permissions`, P5-02A/P5-06). Inheritance would require a
  byte-equal canonicalization of the v1 grant into the v2 resource model plus
  a proof that the security principal is the same — neither exists, so the
  default is deny.
- **Secrets move only through the broker.** `plugin_secrets.ciphertext` is
  sealed with Electron `safeStorage`; the P2-05 export broker unseals it in
  memory and the P2-04 Go providers re-seal it into the platform keyring. The
  re-sealed secrets are bound to the migration purpose and remain unusable by
  any v1 code (which no longer runs) and by v2 code until claimed.
- **Claim process.** A v2 plugin MAY claim preserved data after an explicit
  user approval scoped to `plugin_id` + claimed namespace; the approval is a
  v2 grant event recorded in the v2 audit log. Unclaimed preserved data stays
  in the envelope until the user deletes it.

## v1 code execution

The target runtime rejects v1 packages at install/import time with a clear
incompatibility result (`P5-01`). The only v1 artifacts carried forward are
the disabled metadata inventory and the opaque envelopes above. No shim,
adapter, or fallback executes `main.browser` / `main.node` entrypoints.

## Integration with Phase 2

- P2-05 (export broker) includes the classified tables and the preservation
  envelopes in the migration bundle.
- P2-06 (import/verify/promote) compares `semantic_hash` per envelope and
  fails cutover on mismatch.
- The `no-v1-execution` property is asserted by the retention check and, from
  Phase 5, by the v2 install/import rejection tests.
