import type { LocalTextStore } from "./hostStorageHydrate";

/**
 * One-shot localStorage key-prefix migration (compat#3).
 *
 * Older releases stored every setting under a `netcatty` prefix (`netcatty_*`,
 * `netcatty.aiDebug.*`, `__netcatty_*`, dynamic suffixes like
 * `netcatty_terminal_encoding_by_host_v1:<hostId>`). The brand rename moves
 * all of them to the `lemonssh` spelling. Because old keys are plain
 * localStorage data, the migration is a copy, not a rename:
 *
 * - every key matching /^(?:__)?netcatty/ gets its FIRST `netcatty` swapped
 *   for `lemonssh`;
 * - the value is copied to the new name only when the new name does not
 *   already exist (never overwriting newer data);
 * - legacy keys are kept during the migration (the canonical apply projection
 *   cleans them up afterwards; the durable source of truth is the Go profile
 *   store, whose own rename keeps old rows — compat#4);
 * - a marker key records completion so later startups skip the scan
 *   (idempotent). The marker is only written when every copy succeeded, so a
 *   quota/IO failure retries on the next launch without duplicating work.
 *
 * Must run before the canonical hydrate (bootstrap.ts) and before any
 * settings read so consumers only ever see the new names.
 */

export type StoragePrefixMigrationStore = LocalTextStore & {
  keys(): string[];
};

/** Marker key proving the one-shot prefix migration has completed. */
export const STORAGE_PREFIX_MIGRATION_MARKER = "lemonssh_storage_prefix_migration_v1";

/** Legacy brand prefix, matched with an optional leading double underscore. */
const LEGACY_PREFIX_PATTERN = /^(?:__)?netcatty/;

/**
 * New key for a legacy `netcatty`-prefixed key, or null when the key is not
 * brand-prefixed. Only the first occurrence is swapped so dynamic suffixes
 * (`netcatty_terminal_encoding_by_host_v1:<hostId>`) keep their payload.
 */
export function renamedLegacyStorageKey(key: string): string | null {
  if (!LEGACY_PREFIX_PATTERN.test(key)) return null;
  return key.replace("netcatty", "lemonssh");
}

export type StoragePrefixMigrationResult = {
  /** False when the marker was already present (idempotent skip). */
  ran: boolean;
  /** Legacy keys whose values were copied to the new names this run. */
  copied: string[];
  /** Legacy keys skipped because the new name already held data. */
  skippedExisting: string[];
  /** Legacy keys whose copy failed (quota/IO); the marker is then withheld. */
  failed: string[];
};

export function runStoragePrefixMigration(
  local: StoragePrefixMigrationStore,
  timestamp: () => string = (): string => new Date().toISOString(),
): StoragePrefixMigrationResult {
  if (local.readString(STORAGE_PREFIX_MIGRATION_MARKER) !== null) {
    return { ran: false, copied: [], skippedExisting: [], failed: [] };
  }
  const copied: string[] = [];
  const skippedExisting: string[] = [];
  const failed: string[] = [];
  for (const key of local.keys()) {
    const renamed = renamedLegacyStorageKey(key);
    if (renamed === null) continue;
    if (local.readString(renamed) !== null) {
      skippedExisting.push(key);
      continue;
    }
    const value = local.readString(key);
    if (value === null) continue;
    if (local.writeString(renamed, value)) {
      copied.push(key);
    } else {
      failed.push(key);
    }
  }
  // Withhold the completion marker on partial failure so the next launch
  // retries exactly the keys that did not land (copied ones are skipped).
  if (failed.length === 0) {
    local.writeString(
      STORAGE_PREFIX_MIGRATION_MARKER,
      JSON.stringify({ v: 1, at: timestamp() }),
    );
  }
  return { ran: failed.length === 0, copied, skippedExisting, failed };
}
