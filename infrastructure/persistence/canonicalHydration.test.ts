import assert from "node:assert/strict";
import { test } from "node:test";
import { spawn } from "node:child_process";
import { createInterface } from "node:readline";
import { mkdtemp, rm } from "node:fs/promises";
import { resolve } from "node:path";
import { createCanonicalStorage } from "./hostStorageAdapter";
import type { ProfileClient } from "../runtime/profile/profileClient";

// Runs the actual bbolt store through a test-only JSONL transport. This catches
// invalid domains, base64 mismatches, zero-revision CAS semantics and restart loss.
test("real Go adapter boot, concurrent import, serialized writes, conflicts, restart, closed-store errors and AI isolation", async () => {
  const directory = await mkdtemp(resolve("infrastructure/persistence/.profile-test-"));
  const db = resolve(directory, "profile.db");
  const processes: ReturnType<typeof spawn>[] = [];
  async function start() {
    const child = spawn("go", ["run", "./infrastructure/persistence/profileStoreHarness.go", db], { stdio: ["pipe", "pipe", "pipe"] });
    processes.push(child);
    const lines = createInterface({ input: child.stdout! });
    const waiting: Array<{ resolve(value: unknown): void; reject(error: Error): void }> = [];
    let stderr = "";
    child.stderr!.on("data", chunk => { stderr += String(chunk); });
    child.on("exit", () => { for (const waiter of waiting.splice(0)) waiter.reject(new Error(stderr || "Go exited")); });
    lines.on("line", line => {
      const response = JSON.parse(line);
      const waiter = waiting.shift()!;
      if (response.error) waiter.reject(new Error(response.error));
      else waiter.resolve(response.value);
    });
    const call = (request: object): Promise<unknown> => new Promise((resolve, reject) => {
      waiting.push({ resolve, reject });
      child.stdin!.write(`${JSON.stringify(request)}\n`);
    });
    const client: ProfileClient = {
      revision: async () => Number(await call({ Method: "revision" })),
      domains: async () => ["settings", "vault", "sessions"],
      domainKeys: async domain => (await call({ Method: "keys", Domain: domain }) as string[] | null) ?? [],
      getRawBase64: async (domain, key) => (await call({ Method: "get", Domain: domain, Key: key }) as string | null) ?? undefined,
      setRawBase64: async () => { throw new Error("non-CAS write"); },
      deleteRaw: async () => { throw new Error("non-CAS delete"); },
      write: async (revision, mutations) => {
        const result = await call({ Method: "write", Revision: revision, Mutations: mutations.map(m => ({ Domain: m.domain, Key: m.key, Value: m.valueBase64, Delete: m.delete })) }) as { Revision: number };
        return { revision: result.Revision };
      },
    };
    return { client, close: async () => { await call({ Method: "close" }); } };
  }
  const localData = new Map([["theme", "legacy"], ["netcatty_hosts_v1", "hosts"], ["netcatty_ai_sessions_v1", "private"]]);
  const local = {
    keys: () => [...localData.keys()], readString: (key: string) => localData.get(key) ?? null,
    writeString: (key: string, value: string) => { localData.set(key, value); return true; },
    remove: (key: string) => { localData.delete(key); },
  };
  const errors: unknown[] = [];
  try {
    const host = await start();
    const a = createCanonicalStorage(host.client, local, error => errors.push(error));
    const b = createCanonicalStorage(host.client, local, error => errors.push(error));
    await Promise.all([a.hydrate(), b.hydrate()]);
    assert.equal(a.readString("theme"), "legacy");
    assert.equal(b.readString("netcatty_hosts_v1"), "hosts");
    a.writeString("theme", "first");
    a.writeString("theme", "Go canonical");
    await a.flush();
    b.writeString("theme", "stale overwrite");
    await assert.rejects(b.flush(), /conflict/);
    assert.equal(b.readString("theme"), "Go canonical");
    a.remove("netcatty_hosts_v1");
    await a.flush();
    await b.refresh();
    assert.equal(b.readString("netcatty_hosts_v1"), null);
    assert.equal(await host.client.getRawBase64("settings", "netcatty_ai_sessions_v1"), undefined);
    await host.close();
    const restarted = await start();
    localData.set("theme", "stale local");
    localData.set("netcatty_hosts_v1", "resurrected");
    const c = createCanonicalStorage(restarted.client, local, error => errors.push(error));
    await c.hydrate();
    assert.equal(c.readString("theme"), "Go canonical");
    assert.equal(c.readString("netcatty_hosts_v1"), null);
    assert.equal(c.readString("netcatty_ai_sessions_v1"), "private");
    await restarted.close();
    c.writeString("theme", "cannot persist");
    await assert.rejects(c.flush(), /closed/);
    assert.equal(c.readString("theme"), "Go canonical");
    assert.ok(errors.some(error => String(error).includes("closed")));
  } finally {
    for (const child of processes) if (child.exitCode === null) child.kill();
    await rm(directory, { recursive: true, force: true, maxRetries: 20, retryDelay: 100 });
  }
});
