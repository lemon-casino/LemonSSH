import test from "node:test";
import assert from "node:assert/strict";

import { createTerminalSessionStarters } from "./createTerminalSessionStarters";

const noop = () => undefined;

const useWails = (t: test.TestContext) => {
  const previous = Object.getOwnPropertyDescriptor(globalThis, "window");
  Object.defineProperty(globalThis, "window", { configurable: true, value: { _wails: {} } });
  t.after(() => {
    if (previous) Object.defineProperty(globalThis, "window", previous);
    else Reflect.deleteProperty(globalThis, "window");
  });
};

const armSudoPrompt = (
  autofill: { armForCommand: (command: string) => void } | null,
): string => {
  autofill?.armForCommand("sudo whoami");
  return "[sudo] password for alice: ";
};

const makeBackend = (
  onStartEt: (options: Record<string, unknown>) => void = noop,
) => ({
  backendAvailable: () => true,
  telnetAvailable: () => true,
  moshAvailable: () => true,
  etAvailable: () => true,
  localAvailable: () => true,
  serialAvailable: () => true,
  execAvailable: () => true,
  startSSHSession: async () => "ssh-session",
  startTelnetSession: async () => "telnet-session",
  startMoshSession: async () => "mosh-session",
  startEtSession: async (options: Record<string, unknown>) => {
    onStartEt(options);
    return "et-session";
  },
  startLocalSession: async () => "local-session",
  startSerialSession: async () => "serial-session",
  execCommand: async () => ({}),
  onSessionData: () => noop,
  onSessionExit: () => noop,
  onChainProgress: () => noop,
  writeToSession: noop,
  resizeSession: noop,
});

const makeCtx = (
  host: Record<string, unknown>,
  resolvedChainHosts: Array<Record<string, unknown>>,
  terminalBackend: ReturnType<typeof makeBackend>,
  sinks: { setError?: (m: string) => void } = {},
) => ({
  host: {
    id: "host-1",
    label: "Target",
    hostname: "target.example.test",
    username: "alice",
    etEnabled: true,
    ...host,
  },
  keys: [],
  identities: [],
  resolvedChainHosts,
  sessionId: "session-1",
  terminalSettings: {},
  terminalBackend,
  sessionRef: { current: null },
  hasConnectedRef: { current: false },
  hasRunStartupCommandRef: { current: false },
  disposeDataRef: { current: null },
  disposeExitRef: { current: null },
  fitAddonRef: { current: null },
  serializeAddonRef: { current: null },
  pendingAuthRef: { current: null },
  updateStatus: noop,
  setStatus: noop,
  setError: sinks.setError ?? noop,
  setNeedsAuth: noop,
  setAuthRetryMessage: noop,
  setAuthPassword: noop,
  setProgressLogs: noop,
  setProgressValue: noop,
  setChainProgress: noop,
});

const term = {
  cols: 120,
  rows: 32,
  write: noop,
  writeln: noop,
  scrollToBottom: noop,
};

test("startEt enables sudo autofill with the host saved password", async () => {
  let onData: ((data: string) => void) | null = null;
  const sent: string[] = [];
  const backend = {
    ...makeBackend(),
    onSessionData: (_id: string, cb: (data: string) => void) => {
      onData = cb;
      return noop;
    },
    writeToSession: (_id: string, data: string) => sent.push(data),
  };
  const sudoAutofillRef = { current: null };
  const ctx = {
    ...makeCtx({
      password: "saved-secret",
    }, [], backend),
    sudoAutofillRef,
    sudoAutofillPassword: "saved-secret",
    onSudoHint: () => true,
  };

  await createTerminalSessionStarters(ctx as never).startEt(term as never);
  onData?.(armSudoPrompt(sudoAutofillRef.current));
  sudoAutofillRef.current?.confirmFill();

  assert.deepEqual(sent, ["saved-secret\n"]);
});

test("startEt fails loudly when a configured jump host cannot be resolved", async () => {
  let started = false;
  let error = "";
  const backend = makeBackend(() => { started = true; });
  // hostChain references jump-1, but resolvedChainHosts is empty (missing).
  const ctx = makeCtx(
    { hostChain: { hostIds: ["jump-1"] } },
    [],
    backend,
    { setError: (m) => { error = m; } },
  );

  await createTerminalSessionStarters(ctx as never).startEt(term as never);

  // Must NOT silently fall back to a direct connection.
  assert.equal(started, false);
  assert.match(error, /jump host is missing/i);
  assert.match(error, /jump-1/);
});

test("startEt rejects a configured chain with more than one jump host even if under-resolved", async () => {
  let started = false;
  let error = "";
  const backend = makeBackend(() => { started = true; });
  // Two configured hops but only one resolved — a resolved-length check alone
  // would wrongly let this through.
  const ctx = makeCtx(
    { hostChain: { hostIds: ["jump-1", "jump-2"] } },
    [{
      id: "jump-1",
      label: "Jump",
      hostname: "jump.example.test",
      username: "jumper",
    }],
    backend,
    { setError: (m) => { error = m; } },
  );

  await createTerminalSessionStarters(ctx as never).startEt(term as never);

  assert.equal(started, false);
  assert.match(error, /at most one jump host/i);
});

test("startEt rejects missing proxy identities on the target host before unsupported proxy", async () => {
  let started = false;
  let error = "";
  const backend = makeBackend(() => { started = true; });
  const ctx = makeCtx(
    {
      proxyConfig: {
        type: "http",
        host: "proxy.example.test",
        port: 3128,
        identityId: "missing-identity",
      },
    },
    [],
    backend,
    { setError: (m) => { error = m; } },
  );

  await createTerminalSessionStarters(ctx as never).startEt(term as never);

  assert.equal(started, false);
  assert.match(error, /Proxy identity for "Target" is missing/);
});

test("startEt rejects incomplete proxy identities on the target host before unsupported proxy", async () => {
  let started = false;
  let error = "";
  const backend = makeBackend(() => { started = true; });
  const ctx = {
    ...makeCtx(
      {
        proxyConfig: {
          type: "http",
          host: "proxy.example.test",
          port: 3128,
          identityId: "identity-1",
        },
      },
      [],
      backend,
      { setError: (m) => { error = m; } },
    ),
    identities: [{
      id: "identity-1",
      label: "Proxy login",
      username: "proxy-user",
      authMethod: "password",
      created: 1,
    }],
  };

  await createTerminalSessionStarters(ctx as never).startEt(term as never);

  assert.equal(started, false);
  assert.match(error, /Proxy identity for "Target" is incomplete/);
});

test("startEt rejects missing saved proxy profiles on jump hosts", async () => {
  let started = false;
  let error = "";
  const backend = makeBackend(() => { started = true; });
  const ctx = makeCtx(
    { hostChain: { hostIds: ["jump-1"] } },
    [{
      id: "jump-1",
      label: "Jump",
      hostname: "jump.example.test",
      username: "jumper",
      proxyProfileId: "missing-proxy",
    }],
    backend,
    { setError: (m) => { error = m; } },
  );

  await createTerminalSessionStarters(ctx as never).startEt(term as never);

  assert.equal(started, false);
  assert.match(error, /Saved proxy for jump host "Jump" is missing/);
});

test("startEt rejects missing proxy identities on jump hosts", async () => {
  let started = false;
  let error = "";
  const backend = makeBackend(() => { started = true; });
  const ctx = makeCtx(
    { hostChain: { hostIds: ["jump-1"] } },
    [{
      id: "jump-1",
      label: "Jump",
      hostname: "jump.example.test",
      username: "jumper",
      proxyConfig: {
        type: "http",
        host: "proxy.example.test",
        port: 3128,
        identityId: "missing-identity",
      },
    }],
    backend,
    { setError: (m) => { error = m; } },
  );

  await createTerminalSessionStarters(ctx as never).startEt(term as never);

  assert.equal(started, false);
  assert.match(error, /Proxy identity for "Jump" is missing/);
});

test("startEt rejects incomplete proxy identities on jump hosts", async () => {
  let started = false;
  let error = "";
  const backend = makeBackend(() => { started = true; });
  const ctx = {
    ...makeCtx(
      { hostChain: { hostIds: ["jump-1"] } },
      [{
        id: "jump-1",
        label: "Jump",
        hostname: "jump.example.test",
        username: "jumper",
        proxyConfig: {
          type: "http",
          host: "proxy.example.test",
          port: 3128,
          identityId: "identity-1",
        },
      }],
      backend,
      { setError: (m) => { error = m; } },
    ),
    identities: [{
      id: "identity-1",
      label: "Proxy login",
      username: "proxy-user",
      authMethod: "password",
      created: 1,
    }],
  };

  await createTerminalSessionStarters(ctx as never).startEt(term as never);

  assert.equal(started, false);
  assert.match(error, /Proxy identity for "Jump" is incomplete/);
});

test("startEt connects with a single resolved jump host", async () => {
  let captured: Record<string, unknown> | null = null;
  let error = "";
  const backend = makeBackend((options) => { captured = options; });
  const ctx = makeCtx(
    { hostChain: { hostIds: ["jump-1"] } },
    [{
      id: "jump-1",
      label: "Jump",
      hostname: "jump.example.test",
      username: "jumper",
      // key auth with no saved key reference → local identity file fallback
      authMethod: "key",
      identityFilePaths: ["/Users/alice/.ssh/jump_ed25519"],
    }],
    backend,
    { setError: (m) => { error = m; } },
  );

  await createTerminalSessionStarters(ctx as never).startEt(term as never);

  assert.equal(error, "");
  assert.ok(captured);
  const jumpHosts = captured.jumpHosts as Array<Record<string, unknown>>;
  assert.equal(jumpHosts.length, 1);
  assert.equal(jumpHosts[0]?.hostname, "jump.example.test");
  // Local identity file fallback is forwarded for the hop.
  assert.deepEqual(jumpHosts[0]?.identityFilePaths, ["/Users/alice/.ssh/jump_ed25519"]);
});

test("startEt forwards a jump host's custom ET port", async () => {
  let captured: Record<string, unknown> | null = null;
  const backend = makeBackend((options) => { captured = options; });
  const ctx = makeCtx(
    { hostChain: { hostIds: ["jump-1"] } },
    [{
      id: "jump-1",
      label: "Jump",
      hostname: "jump.example.test",
      username: "jumper",
      etPort: 9022,
    }],
    backend,
  );

  await createTerminalSessionStarters(ctx as never).startEt(term as never);

  const jumpHosts = (captured as Record<string, unknown>).jumpHosts as Array<Record<string, unknown>>;
  assert.equal(jumpHosts[0]?.etPort, 9022);
});

test("startEt forwards a jump host reference key path as an identity file", async () => {
  let captured: Record<string, unknown> | null = null;
  const backend = makeBackend((options) => { captured = options; });
  const ctx = {
    ...makeCtx(
      { hostChain: { hostIds: ["jump-1"] } },
      [{
        id: "jump-1",
        label: "Jump",
        hostname: "jump.example.test",
        username: "jumper",
        authMethod: "key",
        identityFileId: "ref-key",
      }],
      backend,
    ),
    keys: [{
      id: "ref-key",
      label: "Reference key",
      source: "reference",
      filePath: "/Users/alice/.ssh/jump_reference_ed25519",
      // reference keys carry no inline privateKey material
    }],
  };

  await createTerminalSessionStarters(ctx as never).startEt(term as never);

  const jumpHosts = (captured as Record<string, unknown>).jumpHosts as Array<Record<string, unknown>>;
  // privateKey must be omitted for a reference key, and the on-disk path
  // forwarded as an IdentityFile instead of being dropped.
  assert.equal(jumpHosts[0]?.privateKey, undefined);
  assert.deepEqual(jumpHosts[0]?.identityFilePaths, ["/Users/alice/.ssh/jump_reference_ed25519"]);
});

test("startEt keeps a jump private key when agent filtering is unavailable", async () => {
  let captured: Record<string, unknown> | null = null;
  const backend = makeBackend((options) => { captured = options; });
  const ctx = {
    ...makeCtx(
      { hostChain: { hostIds: ["jump-1"] } },
      [{
        id: "jump-1",
        label: "Jump",
        hostname: "jump.example.test",
        username: "jumper",
        authMethod: "key",
        identityFileId: "jump-key",
        useSshAgent: true,
      }],
      backend,
    ),
    keys: [{
      id: "jump-key",
      label: "Jump key",
      type: "ED25519",
      category: "key",
      source: "imported",
      created: 1,
      privateKey: "JUMP PRIVATE KEY",
      passphrase: "jump-passphrase",
    }],
  };

  await createTerminalSessionStarters(ctx as never).startEt(term as never);

  const jumpHosts = (captured as Record<string, unknown>).jumpHosts as Array<Record<string, unknown>>;
  assert.equal(jumpHosts[0]?.useSshAgent, false);
  assert.equal(jumpHosts[0]?.privateKey, "JUMP PRIVATE KEY");
  assert.equal(jumpHosts[0]?.passphrase, "jump-passphrase");
});

test("startEt connects directly when no jump host is configured", async () => {
  let captured: Record<string, unknown> | null = null;
  let error = "";
  const backend = makeBackend((options) => { captured = options; });
  const ctx = makeCtx({}, [], backend, { setError: (m) => { error = m; } });

  await createTerminalSessionStarters(ctx as never).startEt(term as never);

  assert.equal(error, "");
  assert.ok(captured);
  assert.equal(captured.jumpHosts, undefined);
});

for (const type of ["http", "socks5", "command"]) {
  test(`Wails ET forwards ${type} proxy and multiple SSH jump auth options`, async (t) => {
    useWails(t);
    let captured: Record<string, unknown> | undefined;
    let error = "";
    const proxy = { type, host: "proxy.test", port: 3128, command: "connect %h %p", username: "proxy-user", password: "proxy-secret" };
    const ctx = makeCtx({
      password: "target-secret",
      requiresMfa: true,
      proxyConfig: proxy,
      hostChain: { hostIds: ["one", "two"] },
    }, [
      { id: "one", hostname: "one.test", username: "first", password: "hop-secret", requiresMfa: true },
      { id: "two", hostname: "two.test", username: "second", authMethod: "key", identityFilePaths: ["/keys/hop"] },
    ], makeBackend((options) => { captured = options; }), { setError: (message) => { error = message; } });
    await createTerminalSessionStarters(ctx as never).startEt(term as never);
    assert.equal(error, "");
    assert.ok(captured);
    assert.deepEqual(captured.proxy, proxy);
    assert.equal(captured.password, "target-secret");
    assert.equal(captured.requiresMfa, true);
    const hops = captured.jumpHosts as Array<Record<string, unknown>>;
    assert.equal(hops.length, 2);
    assert.equal(hops[0].password, "hop-secret");
    assert.equal(hops[0].requiresMfa, true);
    assert.deepEqual(hops[1].identityFilePaths, ["/keys/hop"]);
  });
}

test("Wails ET validates missing hops and unreadable proxy credentials", async (t) => {
  useWails(t);
  let starts = 0;
  let error = "";
  const backend = makeBackend(() => { starts++; });
  const ctx = makeCtx({ hostChain: { hostIds: ["one", "missing"] } }, [{ id: "one", hostname: "one.test" }], backend,
    { setError: (value) => { error = value; } });
  await createTerminalSessionStarters(ctx as never).startEt(term as never);
  assert.match(error, /jump host is missing/i);
  assert.equal(starts, 0);
  const proxyCtx = makeCtx({ proxyConfig: { type: "http", host: "proxy.test", port: 80, username: "user", password: "enc:v1:djEwdGVzdAAAAAAAAAAAAAAAAA==" } }, [], backend,
    { setError: (value) => { error = value; } });
  await createTerminalSessionStarters(proxyCtx as never).startEt(term as never);
  assert.match(error, /cannot be decrypted/i);
  assert.equal(starts, 0);
});

test("Electron ET retains proxy rejection", async () => {
  let starts = 0;
  let error = "";
  const ctx = makeCtx({ proxyConfig: { type: "socks5", host: "proxy.test", port: 1080 } }, [], makeBackend(() => { starts++; }),
    { setError: (value) => { error = value; } });
  await createTerminalSessionStarters(ctx as never).startEt(term as never);
  assert.equal(starts, 0);
  assert.match(error, /does not currently support Netcatty proxy/);
});

test("Wails Mosh permits SSH bootstrap jump chains while retaining UDP proxy limit", async (t) => {
  useWails(t);
  let captured: Record<string, unknown> | undefined;
  let error = "";
  const backend = { ...makeBackend(), startMoshSession: async (options: Record<string, unknown>) => { captured = options; return "mosh-session"; } };
  const hops = [
    { id: "one", hostname: "one.test", password: "hop-secret" },
    { id: "two", hostname: "two.test", identityFilePaths: ["/keys/hop"], authMethod: "key" },
  ];
  const ctx = makeCtx({ hostChain: { hostIds: ["one", "two"] } }, hops, backend,
    { setError: (value) => { error = value; } });
  await createTerminalSessionStarters(ctx as never).startMosh(term as never);
  assert.equal(error, "");
  assert.equal((captured?.jumpHosts as unknown[]).length, 2);
  captured = undefined;
  const proxyCtx = makeCtx({ ...ctx.host, proxyConfig: { type: "socks5", host: "proxy.test", port: 1080 } }, hops, backend,
    { setError: (value) => { error = value; } });
  await createTerminalSessionStarters(proxyCtx as never).startMosh(term as never);
  assert.equal(captured, undefined);
  assert.match(error, /does not support proxy/);
});

test("startEt forwards known hosts and algorithm options for stats companion parity", async () => {
  let captured: Record<string, unknown> | null = null;
  const knownHosts = [{
    id: "kh-1",
    hostname: "target.example.test",
    port: 22,
    keyType: "ssh-ed25519",
    fingerprint: "SHA256:trusted",
    publicKey: "",
    discoveredAt: 1,
  }];
  const algorithms = { cipher: ["aes128-cbc"] };
  const backend = makeBackend((options) => { captured = options; });
  const ctx = {
    ...makeCtx(
      {
        legacyAlgorithms: true,
        skipEcdsaHostKey: true,
        algorithms,
      },
      [],
      backend,
    ),
    knownHosts,
  };

  await createTerminalSessionStarters(ctx as never).startEt(term as never);

  assert.ok(captured);
  assert.equal(captured.knownHosts, knownHosts);
  assert.equal(captured.legacyAlgorithms, true);
  assert.equal(captured.skipEcdsaHostKey, true);
  assert.equal(captured.algorithmOverrides, algorithms);
});
