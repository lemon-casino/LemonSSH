export type ProfileTextReader = {
  getRawText(domain: string, key: string): Promise<string | undefined>;
  domainKeys?(domain: string): Promise<string[]>;
};

export type LocalTextStore = {
  readString(key: string): string | null;
  writeString(key: string, value: string): boolean;
};

/**
 * Copies Go profile-store values into localStorage when the local key is empty.
 * Existing local values win so a mid-session renderer is not clobbered.
 */
export async function hydrateLocalStorageFromProfile(
  reader: ProfileTextReader,
  local: LocalTextStore,
  domain: string,
  keys: string[],
): Promise<string[]> {
  const hydrated: string[] = [];
  for (const key of keys) {
    if (local.readString(key) !== null) continue;
    const remote = await reader.getRawText(domain, key);
    if (remote === undefined) continue;
    if (local.writeString(key, remote)) hydrated.push(key);
  }
  return hydrated;
}

export async function listHydrationKeys(
  reader: ProfileTextReader,
  fallbackKeys: string[],
  domain: string,
): Promise<string[]> {
  if (!reader.domainKeys) return fallbackKeys;
  try {
    const listed = await reader.domainKeys(domain);
    if (listed.length === 0) return fallbackKeys;
    return listed;
  } catch {
    return fallbackKeys;
  }
}
