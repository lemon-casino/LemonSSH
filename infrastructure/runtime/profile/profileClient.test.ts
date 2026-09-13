import assert from "node:assert/strict";
import { test } from "node:test";

import {
  createProfileClient,
  configureProfileBindings,
  getRawText,
  setRawText,
} from "./profileClient";

test("profile bindings are required at call time and injected CAS maps wire mutations", async () => {
  configureProfileBindings(undefined);
  const client = createProfileClient();
  await assert.rejects(client.revision(), /not configured/);
  let captured: unknown;
  configureProfileBindings({
    Revision: async () => 7,
    GetRaw: async () => { throw new Error("profile key not found"); },
    SetRaw: async () => undefined,
    DeleteRaw: async () => undefined,
    Write: async (revision, mutations) => { captured = { revision, mutations }; return { Revision: 8 }; },
    Domains: async () => ["vault"],
    DomainKeys: async () => ["hosts"],
  });
  try {
    assert.equal(await client.revision(), 7);
    assert.equal(await client.getRawBase64("vault", "missing"), undefined);
    assert.deepEqual(await client.write(7, [{ domain: "vault", key: "hosts", delete: true }]), { revision: 8 });
    assert.deepEqual(captured, { revision: 7, mutations: [{ Domain: "vault", Key: "hosts", Value: null, Delete: true }] });
    assert.deepEqual(await client.domainKeys!("vault"), ["hosts"]);
  } finally { configureProfileBindings(undefined); }
});

test("profile client surfaces the generated skeleton service surface", async () => {
  const client = createProfileClient();
  for (const method of [
    "revision", "getRawBase64", "setRawBase64", "deleteRaw", "write", "domains",
  ] as const) {
    assert.equal(typeof client[method], "function", method);
  }
});

test("absent keys resolve to undefined via the base64 accessor contract", async () => {
  // In plain Node there is no Wails bridge; the call rejects. We only assert
  // the mapping contract against a stub here.
  const stub = createProfileClient();
  const original = stub.getRawBase64;
  stub.getRawBase64 = async (domain, key) => {
    void domain;
    void key;
    throw new Error("profile key not found");
  };
  assert.equal(await getRawText(stub, "vault", "missing"), undefined);
  stub.getRawBase64 = original;
});

test("text helpers round-trip through base64", async () => {
  const calls: Array<{ domain: string; key: string; value: string }> = [];
  const stub = createProfileClient();
  const original = stub.setRawBase64;
  stub.setRawBase64 = async (domain, key, valueBase64) => {
    calls.push({ domain, key, value: valueBase64 });
  };
  await setRawText(stub, "vault", "netcatty_hosts_v1", '{"hosts":[]}');
  assert.equal(calls[0].value, Buffer.from('{"hosts":[]}', "utf8").toString("base64"));
  stub.setRawBase64 = original;
});
