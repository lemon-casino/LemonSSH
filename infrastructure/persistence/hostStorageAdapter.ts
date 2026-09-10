import { localStorageAdapter } from "./localStorageAdapter";
import type { ProfileClient } from "../runtime/profile/profileClient";
import { profileDomainForKey } from "./profileDomain";

// P2-07 transition adapter. Reads remain synchronous so existing hooks keep
// their render-time contract. The local cache is canonical during Electron
// and until Wails hydration completes; Wails writes are mirrored to the Go
// profile store and failures are surfaced through the existing boolean result
// rather than silently dropping the local write.

type HostStorageAdapter = {
  read<T>(key: string): T | null;
  write<T>(key: string, value: T): boolean;
  remove(key: string): void;
  readString(key: string): string | null;
  writeString(key: string, value: string): boolean;
};

let profileClient: ProfileClient | undefined;
const pendingWrites = new Map<string, Promise<void>>();

export function configureHostProfileClient(client: ProfileClient | undefined): void {
  profileClient = client;
}

function mirror(key: string, value: string | null): void {
  if (!profileClient) return;
  const domain = profileDomainForKey(key);
  const operation = value === null
    ? profileClient.deleteRaw(domain, key)
    : profileClient.setRawBase64(domain, key, encodeBase64(value));
  pendingWrites.set(key, operation.catch((error) => {
    console.warn(`[hostStorageAdapter] failed to persist ${key}:`, error);
  }).then(() => undefined));
}

function encodeBase64(value: string): string {
  if (typeof Buffer !== "undefined") return Buffer.from(value, "utf8").toString("base64");
  return btoa(unescape(encodeURIComponent(value)));
}

export const hostStorageAdapter: HostStorageAdapter & typeof localStorageAdapter = {
  ...localStorageAdapter,
  read<T>(key: string): T | null {
    return localStorageAdapter.read<T>(key);
  },
  write<T>(key: string, value: T): boolean {
    const result = localStorageAdapter.write(key, value);
    if (result) mirror(key, JSON.stringify(value));
    return result;
  },
  remove(key: string): void {
    localStorageAdapter.remove(key);
    mirror(key, null);
  },
  readString(key: string): string | null {
    return localStorageAdapter.readString(key);
  },
  writeString(key: string, value: string): boolean {
    const result = localStorageAdapter.writeString(key, value);
    if (result) mirror(key, value);
    return result;
  },
  writeBoolean(key: string, value: boolean): boolean {
    const result = localStorageAdapter.writeBoolean(key, value);
    if (result) mirror(key, value ? "true" : "false");
    return result;
  },
  writeNumber(key: string, value: number): boolean {
    const result = localStorageAdapter.writeNumber(key, value);
    if (result) mirror(key, String(value));
    return result;
  },
};

export async function flushHostProfileWrites(): Promise<void> {
  await Promise.all(pendingWrites.values());
  pendingWrites.clear();
}
