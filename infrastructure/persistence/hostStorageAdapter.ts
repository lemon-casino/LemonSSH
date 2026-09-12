import { emitLocalStorageAdapterChanged, localStorageAdapter } from "./localStorageAdapter";
import type { ProfileClient } from "../runtime/profile/profileClient";
import { isAIManagedStorageKey, profileDomainForKey } from "./profileDomain";
import { encodeProfileText, hydrateCanonicalProfile, isProfileConflict, readCanonicalSnapshot, type LegacyTextStore, type ProfileSnapshot } from "./canonicalHydration";

export const HOST_PROFILE_ERROR_EVENT = "netcatty:profile-storage-error";
const CHANNEL_NAME = "netcatty:canonical-profile";

function reportFailure(error: unknown): void {
  console.error("[hostStorageAdapter] canonical persistence failed:", error);
  if (typeof globalThis.dispatchEvent === "function" && typeof CustomEvent === "function") {
    globalThis.dispatchEvent(new CustomEvent(HOST_PROFILE_ERROR_EVENT, { detail: { error } }));
  }
}

export function createCanonicalStorage(
  client: ProfileClient,
  local: LegacyTextStore,
  onError: (error: unknown) => void = reportFailure,
  onCommit: () => void = () => undefined,
) {
  let ready = false;
  let appliedRevision = -1;
  let committed = new Map<string, string>();
  let cache = new Map<string, string>();
  const pending: Array<{ key: string; value: string | null }> = [];
  let queue: Promise<void> = Promise.resolve();
  const failures: unknown[] = [];

  function requireReady() {
    if (!ready) throw new Error("Canonical profile has not hydrated");
  }
  function apply(snapshot: ProfileSnapshot) {
    appliedRevision = snapshot.revision;
    committed = snapshot.values;
    const next = new Map(committed);
    for (const edit of pending) {
      if (edit.value === null) next.delete(edit.key);
      else next.set(edit.key, edit.value);
    }
    const previous = cache;
    cache = next;
    for (const key of new Set([...previous.keys(), ...next.keys(), ...local.keys()])) {
      if (isAIManagedStorageKey(key)) continue;
      const value = next.get(key) ?? null;
      // Compatibility projection only; all canonical reads use memory. Quota
      // errors here cannot turn a successful Go commit into a failed write.
      try {
        if (value === null) local.remove(key);
        else local.writeString(key, value);
      } catch (error) { onError(error); }
      if ((previous.get(key) ?? null) !== value) emitLocalStorageAdapterChanged(key);
    }
  }
  function enqueue(operation: () => Promise<void>): Promise<void> {
    const result = queue.then(operation);
    queue = result.catch(error => { failures.push(error); onError(error); });
    return result;
  }
  function readString(key: string): string | null {
    if (isAIManagedStorageKey(key)) return local.readString(key);
    requireReady();
    return cache.get(key) ?? null;
  }
  function change(key: string, value: string | null): boolean {
    if (isAIManagedStorageKey(key)) {
      if (value === null) { local.remove(key); return true; }
      return local.writeString(key, value);
    }
    requireReady();
    const expected = cache.get(key) ?? null;
    if (expected === value) return true;
    const edit = { key, value };
    pending.push(edit);
    if (value === null) cache.delete(key);
    else cache.set(key, value);
    emitLocalStorageAdapterChanged(key);
    // Boolean means accepted into the queue. flush() is the durable result;
    // failures are also emitted for callers using the legacy sync API.
    void enqueue(async () => {
      try {
        for (let attempt = 0; attempt < 8; attempt++) {
          const snapshot = await readCanonicalSnapshot(client);
          const actual = snapshot.values.get(key) ?? null;
          if (actual !== expected && actual !== value) throw new Error(`Profile revision conflict for ${key}`);
          try {
            if (actual !== value) await client.write(snapshot.revision, [{ domain: profileDomainForKey(key), key, delete: value === null, valueBase64: value === null ? undefined : encodeProfileText(value) }]);
            onCommit();
            return;
          } catch (error) {
            if (!isProfileConflict(error) || attempt === 7) throw error;
          }
        }
      } finally {
        pending.splice(pending.indexOf(edit), 1);
        // Refresh both success and failure paths, rolling back failed optimistic
        // edits while retaining later queued edits in the synchronous cache.
        try { apply(await readCanonicalSnapshot(client)); }
        catch (error) { apply({ revision: 0, values: committed }); onError(error); }
      }
    }).catch(() => undefined);
    return true;
  }
  return {
    async hydrate() {
      const snapshot = await hydrateCanonicalProfile(client, local);
      apply(snapshot);
      ready = true;
    },
    refresh() { return enqueue(async () => {
      if (!ready) return;
      if (await client.revision() !== appliedRevision) apply(await readCanonicalSnapshot(client));
    }); },
    async flush() {
      let observed: Promise<void>;
      do { observed = queue; await observed; } while (observed !== queue);
      if (failures.length) {
        const errors = failures.splice(0);
        throw new AggregateError(errors, errors.map(String).join("; "));
      }
    },
    keys() { requireReady(); return [...new Set([...cache.keys(), ...local.keys().filter(isAIManagedStorageKey)])]; },
    readString,
    read<T>(key: string): T | null {
      const value = readString(key);
      try { return value === null ? null : JSON.parse(value) as T; } catch { return null; }
    },
    readBoolean(key: string): boolean | null { const value = readString(key); return value === "true" ? true : value === "false" ? false : null; },
    readNumber(key: string): number | null { const value = readString(key); const number = value ? parseInt(value, 10) : NaN; return Number.isNaN(number) ? null : number; },
    write<T>(key: string, value: T): boolean { const text = JSON.stringify(value); if (text === undefined) return false; return change(key, text); },
    writeString: (key: string, value: string) => change(key, value),
    writeBoolean: (key: string, value: boolean) => change(key, String(value)),
    writeNumber: (key: string, value: number) => change(key, String(value)),
    remove: (key: string) => { change(key, null); },
  };
}

let canonical: ReturnType<typeof createCanonicalStorage> | undefined;
let disconnect: (() => void) | undefined;
export function configureHostProfileClient(client: ProfileClient | undefined): void {
  disconnect?.();
  disconnect = undefined;
  canonical = undefined;
  if (!client) return;
  // Browser notifications are invalidations only, never authoritative values.
  const channel = typeof window !== "undefined" && typeof BroadcastChannel !== "undefined" ? new BroadcastChannel(CHANNEL_NAME) : undefined;
  const instance = createCanonicalStorage(client, localStorageAdapter, reportFailure, () => channel?.postMessage("changed"));
  canonical = instance;
  const refresh = () => { void instance.refresh().catch(() => undefined); };
  const storage = (event: StorageEvent) => { if (event.key === null || !isAIManagedStorageKey(event.key)) refresh(); };
  if (channel) channel.onmessage = refresh;
  // Poll revisions too: Go sync/import writers need not share this webview's
  // origin or BroadcastChannel. Focus gives immediate catch-up after suspension.
  const timer = typeof window !== "undefined" ? setInterval(refresh, 2000) : undefined;
  globalThis.addEventListener?.("storage", storage);
  globalThis.addEventListener?.("focus", refresh);
  disconnect = () => {
    channel?.close();
    if (timer) clearInterval(timer);
    globalThis.removeEventListener?.("storage", storage);
    globalThis.removeEventListener?.("focus", refresh);
  };
}
export async function hydrateHostProfile(): Promise<void> { await canonical?.hydrate(); }
export async function flushHostProfileWrites(): Promise<void> { await canonical?.flush(); }

// Resolve on every call so consumers imported before bootstrap use the selected
// shell. Electron and AI retain the existing localStorage adapter behavior.
export const hostStorageAdapter: typeof localStorageAdapter = {
  keys: () => (canonical ?? localStorageAdapter).keys(),
  read: <T>(key: string) => (canonical ?? localStorageAdapter).read<T>(key),
  readString: key => (canonical ?? localStorageAdapter).readString(key),
  readBoolean: key => (canonical ?? localStorageAdapter).readBoolean(key),
  readNumber: key => (canonical ?? localStorageAdapter).readNumber(key),
  write: <T>(key: string, value: T) => (canonical ?? localStorageAdapter).write(key, value),
  writeString: (key, value) => (canonical ?? localStorageAdapter).writeString(key, value),
  writeBoolean: (key, value) => (canonical ?? localStorageAdapter).writeBoolean(key, value),
  writeNumber: (key, value) => (canonical ?? localStorageAdapter).writeNumber(key, value),
  remove: key => (canonical ?? localStorageAdapter).remove(key),
};
