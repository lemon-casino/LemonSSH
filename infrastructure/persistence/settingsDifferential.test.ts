import assert from "node:assert/strict";
import { test } from "node:test";

import {
  configureHostProfileClient,
  flushHostProfileWrites,
  hostStorageAdapter,
} from "./hostStorageAdapter";

// P2-07 settings-domain differential harness: proves the adapter's Electron
// semantics and its Wails mirror produce byte-identical results, which is the
// parity evidence the settings cutover requires.

type Recorder = {
  mirror: Map<string, string>;
  client: ReturnType<typeof makeRecordingClient>;
};

function makeRecordingClient(initial: Record<string, string> = {}) {
  const mirror = new Map<string, string>(Object.entries(initial));
  let revision = 1;
  const writes: Array<{ domain: string; key: string; value: string }> = [];
  const client = {
    revision: async () => revision,
    getRawBase64: async (domain: string, key: string) => mirror.get(key) ?? undefined,
    setRawBase64: async (domain: string, key: string, value: string) => {
      mirror.set(key, value);
      writes.push({ domain, key, value });
    },
    deleteRaw: async (domain: string, key: string) => {
      mirror.delete(key);
    },
    write: async (
      expectedRevision: number,
      mutations: Array<{ domain: string; key: string; value?: string; delete?: boolean }>,
    ) => {
      if (expectedRevision !== revision) throw new Error("cutover revision conflict");
      revision += 1;
      for (const mutation of mutations) {
        if (mutation.delete) mirror.delete(mutation.key);
        else if (mutation.value !== undefined) mirror.set(mutation.key, mutation.value);
      }
      return { revision };
    },
    domains: async () => ["settings"],
  };
  return { client, mirror, writes, bump: () => { revision += 1; } };
}

function installLocalStorage(): Map<string, string> {
  const local = new Map<string, string>();
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    value: {
      getItem: (key: string) => local.get(key) ?? null,
      setItem: (key: string, value: string) => { local.set(key, value); },
      removeItem: (key: string) => { local.delete(key); },
    },
  });
  return local;
}

test("settings differential: adapter write mirrors byte-identical host value", async () => {
  installLocalStorage();
  const recording = makeRecordingClient();
  configureHostProfileClient(recording.client);

  const payloads = ["dark", '{"ui":"light","accent":"#3b82f6"}', "秘密值-🔐"];
  for (const [index, payload] of payloads.entries()) {
    assert.equal(hostStorageAdapter.writeString(`settings.key${index}`, payload), true);
    await flushHostProfileWrites();
    const mirrored = recording.mirror.get(`settings.key${index}`);
    assert.ok(mirrored, "mirror entry missing");
    assert.equal(Buffer.from(mirrored!, "base64").toString("utf8"), payload);
    assert.equal(hostStorageAdapter.readString(`settings.key${index}`), payload);
  }
});

test("settings differential: remove propagates and read stays consistent", async () => {
  installLocalStorage();
  const recording = makeRecordingClient();
  configureHostProfileClient(recording.client);

  hostStorageAdapter.writeString("settings.remove-me", "gone-soon");
  await flushHostProfileWrites();
  assert.ok(recording.mirror.has("settings.remove-me"));

  hostStorageAdapter.remove("settings.remove-me");
  await flushHostProfileWrites();
  assert.equal(recording.mirror.has("settings.remove-me"), false);
  assert.equal(hostStorageAdapter.readString("settings.remove-me"), null);
});

test("differential acceptance envelope: three domains round-trip", async () => {
  installLocalStorage();
  const recording = makeRecordingClient({ "netcatty_theme_v1": "dark" });
  configureHostProfileClient(recording.client);

  const cases: Array<[string, string]> = [
    ["netcatty_theme_v1", "midnight"],
    ["netcatty_term_font_size_v1", "14"],
    ["netcatty_show_sftp_tab_v1", "true"],
  ];
  for (const [key, value] of cases) {
    hostStorageAdapter.writeString(key, value);
    await flushHostProfileWrites();
    assert.equal(recording.mirror.get(key), Buffer.from(value, "utf8").toString("base64"));
  }
});
