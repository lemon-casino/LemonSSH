import assert from "node:assert/strict";
import { test } from "node:test";
import { configureHostProfileClient, hydrateHostProfile, hostStorageAdapter, flushHostProfileWrites } from "./hostStorageAdapter";
import { LOCAL_STORAGE_ADAPTER_CHANGED_EVENT } from "./localStorageAdapter";
import type { ProfileClient } from "../runtime/profile/profileClient";

test("browser storage invalidation reloads Go before existing storage subscribers receive updated values", async () => {
  const target = new EventTarget();
  const originals = new Map<string, PropertyDescriptor | undefined>();
  const install = (key: string, value: unknown) => {
    originals.set(key, Object.getOwnPropertyDescriptor(globalThis, key));
    Object.defineProperty(globalThis, key, { configurable: true, value });
  };
  const local = new Map<string, string>();
  install("addEventListener", target.addEventListener.bind(target));
  install("removeEventListener", target.removeEventListener.bind(target));
  install("dispatchEvent", target.dispatchEvent.bind(target));
  install("localStorage", {
    get length() { return local.size; }, key: (index: number) => [...local.keys()][index] ?? null,
    getItem: (key: string) => local.get(key) ?? null,
    setItem: (key: string, value: string) => { local.set(key, value); }, removeItem: (key: string) => { local.delete(key); },
  });
  let revision = 1;
  const remote = new Map([["settings/theme", Buffer.from("Go dark").toString("base64")]]);
  const client: ProfileClient = {
    revision: async () => revision, domains: async () => ["settings"],
    domainKeys: async domain => [...remote.keys()].filter(key => key.startsWith(`${domain}/`)).map(key => key.slice(domain.length + 1)),
    getRawBase64: async (domain, key) => remote.get(`${domain}/${key}`),
    setRawBase64: async () => { throw new Error("unexpected non-CAS write"); }, deleteRaw: async () => { throw new Error("unexpected non-CAS delete"); },
    write: async (expected, mutations) => {
      if (expected !== revision) throw new Error("profile revision conflict");
      for (const edit of mutations) {
        if (edit.delete) remote.delete(`${edit.domain}/${edit.key}`);
        else remote.set(`${edit.domain}/${edit.key}`, edit.valueBase64!);
      }
      return { revision: ++revision };
    },
  };
  const observed: Array<string | null> = [];
  const onChange = (event: Event) => {
    if ((event as CustomEvent<{ key: string }>).detail.key === "theme") observed.push(hostStorageAdapter.readString("theme"));
  };
  try {
    configureHostProfileClient(client);
    await hydrateHostProfile();
    target.addEventListener(LOCAL_STORAGE_ADAPTER_CHANGED_EVENT, onChange);
    remote.set("settings/theme", Buffer.from("Go light").toString("base64")); revision++;
    local.set("theme", "stale browser event");
    const event = new Event("storage");
    Object.defineProperty(event, "key", { value: "theme" });
    target.dispatchEvent(event);
    await flushHostProfileWrites();
    await new Promise(resolve => setTimeout(resolve, 10));
    assert.equal(hostStorageAdapter.readString("theme"), "Go light");
    assert.ok(observed.includes("Go light"));
    assert.equal(observed.includes("stale browser event"), false);
    remote.delete("settings/theme"); revision++;
    target.dispatchEvent(event);
    await flushHostProfileWrites();
    await new Promise(resolve => setTimeout(resolve, 10));
    assert.equal(hostStorageAdapter.readString("theme"), null);
    assert.ok(observed.includes(null));
  } finally {
    target.removeEventListener(LOCAL_STORAGE_ADAPTER_CHANGED_EVENT, onChange);
    configureHostProfileClient(undefined);
    for (const [key, descriptor] of originals) {
      if (descriptor) Object.defineProperty(globalThis, key, descriptor);
      else Reflect.deleteProperty(globalThis, key);
    }
  }
});
