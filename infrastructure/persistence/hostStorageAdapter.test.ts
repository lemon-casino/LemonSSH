import assert from "node:assert/strict";
import { test } from "node:test";
import * as storageModule from "./hostStorageAdapter";
import type { ProfileClient, ProfileMutation } from "../runtime/profile/profileClient";
import { SYNC_STORAGE_KEYS } from '../../domain/sync';

function fixture(initial: Record<string, string> = {}) {
  const data = new Map(Object.entries(initial).map(([key, value]) => [key, Buffer.from(value).toString("base64")]));
  let revision = 0;
  let failure: Error | undefined;
  const client: ProfileClient = {
    revision: async () => revision,
    domains: async () => ["settings", "vault", "sessions"],
    domainKeys: async (domain) => [...data.keys()].filter(key => key.startsWith(`${domain}/`)).map(key => key.slice(domain.length + 1)),
    getRawBase64: async (domain, key) => data.get(`${domain}/${key}`),
    setRawBase64: async () => { throw new Error("non-CAS write"); },
    deleteRaw: async () => { throw new Error("non-CAS delete"); },
    write: async (expected, mutations: ProfileMutation[]) => {
      if (failure) throw failure;
      if (expected !== revision) throw new Error("profile revision conflict");
      for (const mutation of mutations) {
        const key = `${mutation.domain}/${mutation.key}`;
        if (mutation.delete) data.delete(key);
        else data.set(key, mutation.valueBase64!);
      }
      return { revision: ++revision };
    },
  };
  return { client, data, fail: (error?: Error) => { failure = error; } };
}

test('remote empty values remain authoritative and remote-only keys survive legacy import', async () => {
  const go = fixture({ 'settings/empty': '', 'vault/netcatty_hosts_v1': '[]', 'settings/remoteOnly': 'retained' });
  const adapter = storageModule.createCanonicalStorage(go.client, localFixture({ empty: 'stale', netcatty_hosts_v1: '[stale]', legacy: 'imported' }).local);
  await adapter.hydrate();
  assert.equal(adapter.readString('empty'), '');
  assert.equal(adapter.readString('netcatty_hosts_v1'), '[]');
  assert.equal(adapter.readString('remoteOnly'), 'retained');
  assert.equal(adapter.readString('legacy'), 'imported');
});

test('concurrent same-key CAS writers preserve one winner and roll back the loser', async () => {
  const go = fixture({ 'settings/theme': 'initial' });
  const a = storageModule.createCanonicalStorage(go.client, localFixture().local, () => {});
  const b = storageModule.createCanonicalStorage(go.client, localFixture().local, () => {});
  await a.hydrate();
  await b.hydrate();
  a.writeString('theme', 'A');
  b.writeString('theme', 'B');
  const results = await Promise.allSettled([a.flush(), b.flush()]);
  assert.equal(results.filter(result => result.status === 'fulfilled').length, 1);
  assert.equal(results.filter(result => result.status === 'rejected').length, 1);
  await Promise.all([a.refresh(), b.refresh()]);
  const durable = Buffer.from(go.data.get('settings/theme')!, 'base64').toString();
  assert.ok(durable === 'A' || durable === 'B');
  assert.equal(a.readString('theme'), durable);
  assert.equal(b.readString('theme'), durable);
});

test('later optimistic write survives refresh while an earlier same-key commit waits', async () => {
  const go = fixture();
  const adapter = storageModule.createCanonicalStorage(go.client, localFixture().local);
  await adapter.hydrate();
  const write = go.client.write;
  let release!: () => void;
  let started!: () => void;
  const began = new Promise<void>(resolve => { started = resolve; });
  const hold = new Promise<void>(resolve => { release = resolve; });
  let first = true;
  go.client.write = async (revision, mutations) => {
    if (first) { first = false; started(); await hold; }
    return write(revision, mutations);
  };
  adapter.writeString('sequence', 'first');
  await began;
  adapter.writeString('sequence', 'second');
  const refreshed = adapter.refresh();
  assert.equal(adapter.readString('sequence'), 'second');
  release();
  await Promise.all([adapter.flush(), refreshed]);
  assert.equal(adapter.readString('sequence'), 'second');
  assert.equal(Buffer.from(go.data.get('settings/sequence')!, 'base64').toString(), 'second');
});

function localFixture(initial: Record<string, string> = {}) {
  const data = new Map(Object.entries(initial));
  const local = {
    keys: () => [...data.keys()],
    readString: (key: string) => data.get(key) ?? null,
    writeString: (key: string, value: string) => { data.set(key, value); return true; },
    remove: (key: string) => { data.delete(key); },
  };
  return { data, local };
}

test('profile rotation transaction publishes every record only after durable commit', async () => {
  const go = fixture({ 'settings/config': 'old-config', 'settings/replica': 'old-ciphertext', 'settings/baseline': 'old-baseline' });
  const adapter = storageModule.createCanonicalStorage(go.client, localFixture().local);
  await adapter.hydrate();
  let release!: () => void;
  let began!: () => void;
  const started = new Promise<void>(resolve => { began = resolve; });
  const gate = new Promise<void>(resolve => { release = resolve; });
  const write = go.client.write;
  go.client.write = async (revision, mutations) => { began(); await gate; return write(revision, mutations); };
  const expected = new Map([['config', 'old-config'], ['replica', 'old-ciphertext'], ['baseline', 'old-baseline']]);
  const next = new Map([['config', 'new-config'], ['replica', 'new-ciphertext'], ['baseline', 'new-baseline']]);
  const transaction = adapter.transaction(expected, next);
  await started;
  assert.equal(adapter.readString('config'), 'old-config');
  assert.equal(adapter.readString('replica'), 'old-ciphertext');
  assert.throws(() => adapter.writeString('baseline', 'racing-write'), /transaction in progress/);
  release(); await transaction;
  for (const [key, value] of next) {
    assert.equal(adapter.readString(key), value);
    assert.equal(Buffer.from(go.data.get(`settings/${key}`)!, 'base64').toString(), value);
  }
});

test('failed profile rotation leaves original bytes and revision; stale preparation aborts', async () => {
  const go = fixture({ 'settings/config': 'old-config', 'settings/replica': 'old-ciphertext' });
  const adapter = storageModule.createCanonicalStorage(go.client, localFixture().local, () => {});
  await adapter.hydrate();
  const original = new Map(go.data);
  const revision = adapter.revision();
  const expected = new Map([['config', 'old-config'], ['replica', 'old-ciphertext']]);
  const next = new Map([['config', 'new-config'], ['replica', 'new-ciphertext']]);
  go.fail(new Error('disk failed'));
  await assert.rejects(adapter.transaction(expected, next, revision), /disk failed/);
  assert.deepEqual(go.data, original);
  assert.equal(adapter.revision(), revision);
  assert.equal(adapter.readString('replica'), 'old-ciphertext');
  go.fail();
  await go.client.write(revision, [{ domain: 'settings', key: 'new-provider-baseline', valueBase64: Buffer.from('old-key-ciphertext').toString('base64') }]);
  await assert.rejects(adapter.transaction(expected, next, revision), /Profile changed/);
  assert.equal(adapter.readString('config'), 'old-config');
});

test('rotation drains earlier failed writes and stale windows cannot append old-key records', async () => {
  const config = SYNC_STORAGE_KEYS.MASTER_KEY_CONFIG;
  const replica = SYNC_STORAGE_KEYS.CONVERGENT_REPLICA;
  const go = fixture({ [`settings/${config}`]: 'old-config' });
  const a = storageModule.createCanonicalStorage(go.client, localFixture().local, () => {});
  const b = storageModule.createCanonicalStorage(go.client, localFixture().local, () => {});
  await a.hydrate(); await b.hydrate();
  go.fail(new Error('queued disk failure'));
  a.writeString('queued', 'pending');
  await assert.rejects(a.transaction(new Map([[config, 'old-config']]), new Map([[config, 'new-config']])), /Pending profile writes failed/);
  go.fail();
  await a.transaction(new Map([[config, 'old-config']]), new Map([[config, 'new-config']]));
  b.writeString(replica, 'old-key-ciphertext');
  await assert.rejects(b.flush(), /Master key changed/);
  assert.equal(go.data.has(`settings/${replica}`), false);
});

// These exercise the real adapter; only the external Go RPC and browser storage
// boundaries are replaced. Reverting to local reads or non-CAS writes breaks them.
test("boot unions legacy and remote keys, prefers Go, reads memory, and never resurrects deletions", async () => {
  const go = fixture({ "settings/theme": "remote", "vault/netcatty_hosts_v1": "[hosts]" });
  const legacy = localFixture({ theme: "stale", localOnly: "legacy", netcatty_ai_sessions_v1: "private" });
  const adapter = storageModule.createCanonicalStorage(go.client, legacy.local);
  await adapter.hydrate();
  assert.equal(adapter.readString("theme"), "remote");
  assert.equal(adapter.readString("localOnly"), "legacy");
  assert.equal(adapter.readString("netcatty_hosts_v1"), "[hosts]");
  legacy.data.set("theme", "tampered");
  assert.equal(adapter.readString("theme"), "remote");
  adapter.remove("localOnly");
  await adapter.flush();
  legacy.data.set("localOnly", "stale resurrection");
  const restarted = storageModule.createCanonicalStorage(go.client, legacy.local);
  await restarted.hydrate();
  assert.equal(restarted.readString("localOnly"), null);
  assert.equal(legacy.data.has("localOnly"), false);
  assert.equal(restarted.readString("netcatty_ai_sessions_v1"), "private");
  assert.equal([...go.data.keys()].some(key => key.includes("netcatty_ai_")), false);
});

test("all typed reads use cache, rapid writes and delete are serialized, AI stays local", async () => {
  const go = fixture();
  const legacy = localFixture();
  const adapter = storageModule.createCanonicalStorage(go.client, legacy.local);
  await adapter.hydrate();
  adapter.write("json", { a: 1 });
  adapter.writeBoolean("flag", true);
  adapter.writeNumber("number", 42);
  adapter.writeString("sequence", "first");
  adapter.writeString("sequence", "second");
  adapter.remove("sequence");
  adapter.writeString("netcatty_ai_providers_v1", "secret");
  adapter.writeString("netcatty.aiDebug.hide", "true");
  assert.deepEqual(adapter.read("json"), { a: 1 });
  assert.equal(adapter.readBoolean("flag"), true);
  assert.equal(adapter.readNumber("number"), 42);
  await adapter.flush();
  assert.equal(go.data.has("settings/sequence"), false);
  assert.equal(go.data.has("settings/netcatty_ai_providers_v1"), false);
  assert.equal(go.data.has("settings/netcatty.aiDebug.hide"), false);
  assert.equal(legacy.data.get("netcatty_ai_providers_v1"), "secret");
});

test("two actual adapters reject stale same-key edits but preserve unrelated writes", async () => {
  const go = fixture({ "settings/theme": "initial" });
  const a = storageModule.createCanonicalStorage(go.client, localFixture().local);
  const reported: unknown[] = [];
  const b = storageModule.createCanonicalStorage(go.client, localFixture().local, error => reported.push(error));
  await a.hydrate();
  await b.hydrate();
  a.writeString("theme", "A");
  await a.flush();
  b.writeString("theme", "B");
  await assert.rejects(b.flush(), /conflict/);
  assert.ok(reported.some(error => String(error).includes("conflict")));
  assert.equal(b.readString("theme"), "A");
  b.writeString("unrelated", "kept");
  await b.flush();
  await a.refresh();
  assert.equal(a.readString("unrelated"), "kept");
  assert.equal(a.readString("theme"), "A");
});

test("hydration and writes surface failures, rollback memory, and allow recovery", async () => {
  const go = fixture();
  const legacy = localFixture({ theme: "legacy" });
  const reported: unknown[] = [];
  const adapter = storageModule.createCanonicalStorage(go.client, legacy.local, error => reported.push(error));
  go.fail(new Error("disk unavailable"));
  await assert.rejects(adapter.hydrate(), /disk unavailable/);
  assert.throws(() => adapter.readString("theme"), /hydrat/);
  go.fail();
  await adapter.hydrate();
  go.fail(new Error("disk full"));
  adapter.writeString("theme", "lost");
  await assert.rejects(adapter.flush(), /disk full/);
  assert.ok(reported.some(error => String(error).includes("disk full")));
  assert.equal(adapter.readString("theme"), "legacy");
  assert.equal(legacy.data.get("theme"), "legacy");
  go.fail();
  adapter.writeString("theme", "recovered");
  await adapter.flush();
  assert.equal(adapter.readString("theme"), "recovered");
});

test("refresh observes remote deletion despite stale local and excludes remote AI", async () => {
  const go = fixture({ "settings/theme": "dark", "settings/netcatty_ai_sessions_v1": "remote secret" });
  const legacy = localFixture({ netcatty_ai_sessions_v1: "local secret" });
  const adapter = storageModule.createCanonicalStorage(go.client, legacy.local);
  await adapter.hydrate();
  await go.client.write(await go.client.revision(), [{ domain: "settings", key: "theme", delete: true }]);
  await adapter.refresh();
  assert.equal(adapter.readString("theme"), null);
  assert.equal(legacy.data.has("theme"), false);
  assert.equal(adapter.readString("netcatty_ai_sessions_v1"), "local secret");
});
