import { isAIManagedStorageKey } from "./profileDomain";
import type { ProfileTextReader, LocalTextStore } from "./hostStorageHydrate";

// SYNC-01 canonical cutover. The Go profile store is the durable owner of the
// non-AI domains; localStorage becomes a derived read cache that existing
// synchronous hooks keep consuming. One hydration pass before React mounts
// (boot already awaits hydrateReady) converges both sources:
//
// - Go value, no local value  -> hydrate the local cache from Go.
// - Local value, no Go value  -> promote the legacy local value into Go
//   (first-run import / rollback path for profiles written before cutover).
// - Both values, different    -> conflict. The local value wins and Go is
//   healed, because the renderer is the only writer during a session and a
//   boot-time divergence almost always means a failed best-effort mirror;
//   the divergence is reported so acceptance evidence can show the count.
// - Both values, equal        -> converged, no-op.
//
// AI-managed keys are excluded in every branch: they stay localStorage-
// canonical until P6-05 and must never cross into the Go store.

export type ProfileTextWriter = {
  setRawText(domain: string, key: string, value: string): Promise<void>;
};

export type CanonicalDivergence = {
  domain: string;
  key: string;
  /** True when both sources had values that differed (healed toward local). */
  conflict: boolean;
};

export type CanonicalHydrationOutcome = {
  hydratedFromProfile: string[];
  promotedToProfile: string[];
  healedToProfile: string[];
  divergences: CanonicalDivergence[];
};

export async function hydrateCanonicalProfile(
  reader: ProfileTextReader,
  writer: ProfileTextWriter,
  local: LocalTextStore,
  domain: string,
  keys: string[],
): Promise<CanonicalHydrationOutcome> {
  const outcome: CanonicalHydrationOutcome = {
    hydratedFromProfile: [],
    promotedToProfile: [],
    healedToProfile: [],
    divergences: [],
  };
  for (const key of keys) {
    if (isAIManagedStorageKey(key)) continue;
    const localValue = local.readString(key);
    const remoteValue = await reader.getRawText(domain, key);
    if (remoteValue === undefined && localValue === null) continue;
    if (remoteValue !== undefined && localValue === null) {
      if (local.writeString(key, remoteValue)) outcome.hydratedFromProfile.push(key);
      continue;
    }
    if (remoteValue === undefined || remoteValue !== localValue) {
      await writer.setRawText(domain, key, localValue as string);
      outcome.healedToProfile.push(key);
      outcome.divergences.push({ domain, key, conflict: remoteValue !== undefined });
      if (remoteValue === undefined) outcome.promotedToProfile.push(key);
    }
  }
  return outcome;
}

export type CanonicalDiffEntry = {
  domain: string;
  key: string;
  inLocal: boolean;
  inProfile: boolean;
  /** Values exist in both sources but differ. Values are never reported. */
  mismatch: boolean;
};

/**
 * Differential comparison across localStorage and the Go profile store during
 * the cutover window. Read-only: it never writes either source. AI-managed
 * keys are skipped, matching the hydration boundary.
 */
export async function diffCanonicalSources(
  reader: ProfileTextReader,
  local: LocalTextStore,
  domain: string,
  keys: string[],
): Promise<CanonicalDiffEntry[]> {
  const entries: CanonicalDiffEntry[] = [];
  for (const key of keys) {
    if (isAIManagedStorageKey(key)) continue;
    const localValue = local.readString(key);
    const remoteValue = await reader.getRawText(domain, key);
    const inLocal = localValue !== null;
    const inProfile = remoteValue !== undefined;
    entries.push({ domain, key, inLocal, inProfile, mismatch: inLocal && inProfile && localValue !== remoteValue });
  }
  return entries;
}
