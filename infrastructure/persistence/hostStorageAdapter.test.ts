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
