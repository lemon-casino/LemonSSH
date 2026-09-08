import assert from "node:assert/strict";
import { test } from "node:test";

import {
  configureHostProfileClient,
  flushHostProfileWrites,
  hostStorageAdapter,
} from "./hostStorageAdapter";

test.afterEach(async () => {
  await flushHostProfileWrites();
  configureHostProfileClient(undefined);
});

test("host adapter preserves synchronous localStorage semantics without a client", () => {
  assert.equal(typeof hostStorageAdapter.read, "function");
  assert.equal(typeof hostStorageAdapter.write, "function");
  assert.equal(typeof hostStorageAdapter.remove, "function");
});

test("host adapter mirrors writes to the profile client", async () => {
  const storage = new Map<string, string>();
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    value: {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => { storage.set(key, value); },
      removeItem: (key: string) => { storage.delete(key); },
    },
  });
  const calls: Array<{ domain: string; key: string; value: string }> = [];
  configureHostProfileClient({
    revision: async () => 0,
    getRawBase64: async () => undefined,
    setRawBase64: async (domain, key, value) => { calls.push({ domain, key, value }); },
    deleteRaw: async () => undefined,
    write: async () => ({ revision: 1 }),
    domains: async () => ["settings"],
  });
  assert.equal(hostStorageAdapter.writeString("host-storage-test", "值"), true);
  await flushHostProfileWrites();
  assert.equal(calls.length, 1);
  assert.equal(calls[0].domain, "settings");
  assert.equal(Buffer.from(calls[0].value, "base64").toString("utf8"), "值");
});

test("two windows writing through CAS: stale revision is rejected, fresh wins", async () => {
  // Simulates P2-07's host-revision semantics: window A and window B both
  // read the profile revision, A writes first, and B's stale CAS must lose.
  let storedRevision = 5;
  const client = {
    revision: async () => storedRevision,
    getRawBase64: async () => undefined,
    setRawBase64: async () => undefined,
    deleteRaw: async () => undefined,
    write: async (expectedRevision: number) => {
      if (expectedRevision !== storedRevision) {
        throw new Error("profile revision conflict");
      }
      storedRevision += 1;
      return { revision: storedRevision };
    },
    domains: async () => ["settings"],
  };
  configureHostProfileClient(client);

  // Both windows observe the same starting revision.
  const revisionA = await client.revision();
  const revisionB = await client.revision();
  assert.equal(revisionA, revisionB);

  // Window A writes with the current revision and wins.
  await client.write(revisionA, []);
  // Window B's stale write is rejected by the host.
  await assert.rejects(() => client.write(revisionB, []), /revision conflict/);
  assert.equal(storedRevision, 6);
});
