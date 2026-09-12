import type { ProfileClient, ProfileMutation } from "../runtime/profile/profileClient";
import { CANONICAL_PROFILE_DOMAINS, isAIManagedStorageKey, profileDomainForKey } from "./profileDomain";
import type { ProfileTextReader, LocalTextStore } from "./hostStorageHydrate";

// This marker belongs to Go, not browser storage. Import and marker are one CAS
// transaction, so a restart can never re-import a deleted legacy value.
const IMPORT_DOMAIN = "device";
const IMPORT_KEY = "canonical-v1";
export type LegacyTextStore = LocalTextStore & { keys(): string[]; remove(key: string): void };
export type ProfileSnapshot = { revision: number; values: Map<string, string> };
export const encodeProfileText = (value: string): string => {
  const bytes = new TextEncoder().encode(value);
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
};
const decodeProfileText = (value: string): string =>
  new TextDecoder().decode(Uint8Array.from(atob(value), char => char.charCodeAt(0)));
export const isProfileConflict = (error: unknown): boolean => String(error).includes("revision conflict");

export async function readCanonicalSnapshot(client: ProfileClient): Promise<ProfileSnapshot> {
  if (!client.domainKeys) throw new Error("Canonical hydration requires profile key enumeration");
  for (let attempt = 0; attempt < 8; attempt++) {
    const revision = await client.revision();
    const values = new Map<string, string>();
    for (const domain of CANONICAL_PROFILE_DOMAINS) {
      for (const key of await client.domainKeys(domain)) {
        if (isAIManagedStorageKey(key) || profileDomainForKey(key) !== domain) continue;
        const value = await client.getRawBase64(domain, key);
        if (value !== undefined) values.set(key, decodeProfileText(value));
      }
    }
    if (await client.revision() === revision) return { revision, values };
  }
  throw new Error("Profile changed repeatedly during hydration");
}

export async function hydrateCanonicalProfile(client: ProfileClient, local: LegacyTextStore): Promise<ProfileSnapshot> {
  for (let attempt = 0; attempt < 8; attempt++) {
    const snapshot = await readCanonicalSnapshot(client);
    // Go reserves expectedRevision=0 for unconditional writes. Establish a
    // nonzero revision using an inert device key before any import CAS. Racing
    // initializers may both write this same seed, but cannot overwrite data.
    if (snapshot.revision === 0) {
      await client.write(0, [{ domain: IMPORT_DOMAIN, key: "canonical-cas-seed", valueBase64: encodeProfileText("1") }]);
      continue;
    }
    const imported = await client.getRawBase64(IMPORT_DOMAIN, IMPORT_KEY);
    if (await client.revision() !== snapshot.revision) continue;
    if (imported !== undefined) return snapshot;
    const mutations: ProfileMutation[] = [];
    // Union: snapshot already contains all remote keys; only absent local keys
    // are imported. Existing Go values always win, including empty strings.
    for (const key of local.keys()) {
      if (isAIManagedStorageKey(key) || snapshot.values.has(key)) continue;
      const value = local.readString(key);
      if (value !== null) mutations.push({ domain: profileDomainForKey(key), key, valueBase64: encodeProfileText(value) });
    }
    mutations.push({ domain: IMPORT_DOMAIN, key: IMPORT_KEY, valueBase64: encodeProfileText("1") });
    try {
      await client.write(snapshot.revision, mutations);
      return await readCanonicalSnapshot(client);
    } catch (error) {
      if (!isProfileConflict(error)) throw error;
    }
  }
  throw new Error("Profile revision conflict during legacy import");
}

export type CanonicalDiffEntry = {
  domain: string; key: string; inLocal: boolean; inProfile: boolean; mismatch: boolean;
};
export async function diffCanonicalSources(reader: ProfileTextReader, local: LocalTextStore, domain: string, keys: string[]): Promise<CanonicalDiffEntry[]> {
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
