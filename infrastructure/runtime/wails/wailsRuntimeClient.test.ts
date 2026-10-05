import assert from "node:assert/strict";
import { test } from "node:test";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";

import { useSftpDirectoryListing } from "../../../application/state/sftp/useSftpDirectoryListing";
import { getActiveRuntimeClient, setActiveRuntimeClient } from "../runtimeClient";
import { useSftpExternalOperations } from "../../../application/state/sftp/useSftpExternalOperations";
import { sftpTransferCenterStore } from "../../../application/state/sftpTransferCenterStore";
import type { SftpPane } from "../../../application/state/sftp/types";
import type { AppLockSettings } from "../../../domain/appLock";
import { hostStorageAdapter } from "../../persistence/hostStorageAdapter";
import { createWailsRuntimeClient } from "./wailsRuntimeClient";
import type { WailsBindingDeps } from "./wailsRuntimeClient";
import { RECEIVE_WINDOW_BYTES } from "../../terminal/dataplane/frame";

function stubBindings(overrides: Partial<WailsBindingDeps["terminal"]> = {}): WailsBindingDeps {
  const attached: string[] = [];
  return {
    terminal: {
      Connect: async () => "term-1",
      StartLocal: async () => "local-1",
      Write: () => 0,
      Resize: () => undefined,
      Signal: () => undefined,
      Close: async () => undefined,
      Bootstrap: async (sessionID) => ({
        SessionID: sessionID,
        Generation: 1,
        DataToken: "d".repeat(64),
        UrgentToken: "u".repeat(64),
        WindowBytes: RECEIVE_WINDOW_BYTES,
      }),
      ListenAddr: () => "127.0.0.1:9",
      ...overrides,
    },
    sftp: {
      Open: async () => "sftp-1",
      List: async () => [],
      Mkdir: async () => undefined,
      Remove: async () => undefined,
      Rename: async () => undefined,
      Stat: async () => ({ path: "/", isDir: true, size: 0, mode: "drwxr-xr-x", modTime: "2026-09-10T00:00:00Z" }),
      Close: async () => undefined,
    },
    openDataPlane: ({ onComplete }) => {
      attached.push("open");
      return {
        dispose: () => {
          attached.push("dispose");
          onComplete?.();
        },
      };
    },
  };
}

test("agentRuntime relays the Go turn runtime DTOs", async () => {
  const calls: string[] = [];
  const bindings = stubBindings({
  }) as WailsBindingDeps;
  bindings.agentservice = {
    AgentPrepare: async (request) => {
      calls.push("prepare");
      return {
        TurnID: "turn_1",
        LeaseExpiresAtMS: 1,
        Cursor: "0",
        SnapshotRevision: "1",
        EffectiveScope: request.RequestedScope,
        EffectiveConfigRevision: "1",
        PolicyRevision: "1",
      };
    },
    AgentStart: async () => { calls.push("start"); },
    AgentStop: async () => { calls.push("stop"); return { Status: "stopped", Revision: "2", ThroughSequence: "3" }; },
    AgentReadEvents: async () => { calls.push("read"); return { Events: [] }; },
    AgentSnapshot: async () => { calls.push("snapshot"); return { Status: "running", Revision: "1", ThroughSequence: "2" }; },
  };
  const client = createWailsRuntimeClient(bindings);

  const prepared = await client.agentRuntime.agentPrepare({
    RequestID: "req_1",
    ChatSessionID: "chat_1",
    AgentID: "agnt_1",
    Input: { Text: "hello" },
    RequestedScope: { TerminalRead: true },
  } as never);
  assert.equal(prepared.TurnID, "turn_1");
  await client.agentRuntime.agentStart({ RequestID: "req_1", Kind: "start", TurnID: "turn_1" } as never);
  await client.agentRuntime.agentStop("turn_1", "test");
  await client.agentRuntime.agentReadEvents("turn_1", "0", 10);
  await client.agentRuntime.agentSnapshot("turn_1");
  assert.deepEqual(calls, ["prepare", "start", "stop", "read", "snapshot"]);
});

test("agentRuntime throws typed unavailable without bindings", () => {
  const bindings = stubBindings() as WailsBindingDeps;
  delete (bindings as Partial<WailsBindingDeps>).agentservice;
  const client = createWailsRuntimeClient(bindings);
  assert.throws(() => client.agentRuntime.agentPrepare({} as never), /agent runtime is unavailable/);
});

test("Complete resolves authoritative clean/error/closed exit metadata", async () => {
  for (const status of [{ reason: "exited" as const, exitCode: 0 }, { reason: "exited" as const, exitCode: 7 }, { reason: "closed" as const }, { reason: "error" as const, error: "transport lost" }]) {
    const bindings = stubBindings({ GetExitStatus: async () => ({ sessionId: "term-1", ...status }) });
    let complete: (() => void) | undefined;
    bindings.openDataPlane = options => { complete = options.onComplete; return { dispose() {} }; };
    const bridge = createWailsRuntimeClient(bindings).transitionBridge;
    await bridge.startSSHSession({ hostname: "host", username: "user" });
    const received = new Promise(resolve => bridge.onSessionExit("term-1", resolve));
    complete!();
    assert.deepEqual(await received, { sessionId: "term-1", ...status });
  }
});

test("native exit remains authoritative when Complete is lost with the socket", async () => {
  const bindings = stubBindings({ GetExitStatus: async () => ({ sessionId: "term-1", reason: "exited", exitCode: 0 }) });
  let disconnect: (() => void) | undefined;
  bindings.openDataPlane = options => { disconnect = options.onDisconnect; return { dispose() {} }; };
  const bridge = createWailsRuntimeClient(bindings).transitionBridge;
  await bridge.startSSHSession({ hostname: "host", username: "user" });
  const result = new Promise(resolve => bridge.onSessionExit("term-1", resolve));
  disconnect!();
  assert.deepEqual(await Promise.race([result, new Promise(resolve => setTimeout(() => resolve("missing exit"), 100))]), { sessionId: "term-1", reason: "exited", exitCode: 0 });
  await bridge.closeSession("term-1");
});

test("OSC notification bridge reports native delivery and failure", async () => {
  const bindings = stubBindings();
  bindings.settings = { Open: async () => true, Close: async () => {}, ShowSystemNotification: async payload => ({ shown: payload.body === "ready" }) };
  const bridge = createWailsRuntimeClient(bindings).transitionBridge;
  assert.deepEqual(await bridge.showSystemNotification!({ title: "Host", body: "ready" }), { shown: true });
  bindings.settings.ShowSystemNotification = async () => { throw new Error("unavailable"); };
  assert.deepEqual(await bridge.showSystemNotification!({ title: "Host", body: "ready" }), { shown: false, reason: "Error: unavailable" });
});

test("clipboard image bridge uses managed native files and exact terminal aliases", async () => {
  const bindings = stubBindings();
  const opened: string[] = [];
  bindings.sftp.OpenForTerminal = async id => { opened.push(id); return `sftp-${id}`; };
  const image = { path: "C:\\LemonSSH\\temp\\shot.png", name: "shot.png", mediaType: "image/png", size: 123 };
  bindings.filesystem = { ReadClipboardImage: async () => image };
  const bridge = createWailsRuntimeClient(bindings).transitionBridge;
  await bridge.startSSHSession({ sessionId: "ui-image", hostname: "host", username: "user" });
  assert.deepEqual(await bridge.readClipboardImage!(), image);
  assert.equal(await bridge.openSftpForSession!("ui-image"), "sftp-term-1");
  assert.equal(await bridge.openSftpForSession!("native-other"), "sftp-native-other");
  assert.deepEqual(opened, ["term-1", "native-other"]);
  bindings.filesystem.ReadClipboardImage = async () => null;
  assert.equal(await bridge.readClipboardImage!(), null);
  bindings.sftp.OpenForTerminal = async () => { throw new Error("closed terminal"); };
  await assert.rejects(bridge.openSftpForSession!("ui-image"), /closed terminal/);
});

test("autocomplete bridge lists the aliased terminal without PTY writes", async () => {
  const bindings = stubBindings({
    ListAutocompleteDirectory: async (id, directory, foldersOnly) => ({
      success: id === 'term-1' && directory === '/data' && foldersOnly,
      entries: [{ name: 'Mihomo', type: 'directory' }],
    }),
    Write: () => { throw new Error('completion must never write to PTY'); },
  });
  const bridge = createWailsRuntimeClient(bindings).transitionBridge;
  await bridge.startSSHSession({ sessionId: 'ui-completion', hostname: 'host', username: 'user' });
  const result = await bridge.listAutocompleteRemoteDir!('ui-completion', '/data', true);
  assert.equal(result.success, true);
  assert.equal(result.entries[0].name, 'Mihomo');
});

test("system unlock keeps unavailable native status and rejected authentication fail closed", async () => {
  const bindings = stubBindings();
  bindings.appLock = {
    GetRuntimeState: async () => ({initialized:true,locked:true,reason:"manual",version:1,lastLockedAt:1,lastUnlockedAt:null,lastActivityAt:null}),
    GetSystemUnlockStatus: async () => ({supported:true,available:false,enabled:false,platform:"win32",label:"Windows Hello",reason:"not configured"}),
    UnlockWithBiometrics: async () => ({success:false,error:"disabled"}),
  };
  const bridge = createWailsRuntimeClient(bindings).transitionBridge;
  assert.equal((await bridge.getAppLockSystemUnlockStatus!()).available, false);
  assert.deepEqual(await bridge.requestAppLockSystemUnlock!(), {ok:false,error:"disabled"});
  bindings.appLock.UnlockWithBiometrics = async () => ({success:true});
  assert.deepEqual(await bridge.requestAppLockSystemUnlock!(), {ok:true});
  assert.deepEqual(await createWailsRuntimeClient(stubBindings()).transitionBridge.requestAppLockSystemUnlock!(), {ok:false,error:"unsupported"});
});

test("app lock lock/timeout calls and cross-window event subscriptions map onto the Go services", async () => {
  const calls: string[] = [];
  const subscribers: Array<{ name: string; cb: (event: { data?: unknown }) => void }> = [];
  const bindings = stubBindings();
  bindings.events = {
    On: (name, cb) => {
      subscribers.push({ name, cb });
      return () => { calls.push(`unsubscribe:${name}`); };
    },
  };
  bindings.appLock = {
    GetRuntimeState: async () => ({initialized:true,locked:false,reason:null,version:1,lastLockedAt:null,lastUnlockedAt:null,lastActivityAt:null}),
    SetRuntimeLocked: async (reason: string) => {
      calls.push(`lock:${reason}`);
      return {initialized:true,locked:true,reason,version:2,lastLockedAt:1,lastUnlockedAt:null,lastActivityAt:null};
    },
    SetTimeoutMinutes: async (minutes: number) => {
      calls.push(`timeout:${minutes}`);
      return { enabled: true, timeoutMinutes: minutes, systemUnlockEnabled: false, systemUnlockAutoPromptEnabled: false, passwordVerifier: null };
    },
  };
  const bridge = createWailsRuntimeClient(bindings).transitionBridge;

  const locked = await bridge.setAppLockRuntimeLocked!("manual");
  assert.equal(locked.locked, true);
  assert.equal(locked.reason, "manual");
  assert.deepEqual(await bridge.setAppLockTimeoutMinutes!(30), {
    enabled: true, timeoutMinutes: 30, systemUnlockEnabled: false, systemUnlockAutoPromptEnabled: false, passwordVerifier: null,
  });
  assert.deepEqual(calls, ["lock:manual", "timeout:30"]);

  const received: unknown[] = [];
  const unsubscribers = [
    bridge.onAppLockRuntimeStateChanged!((state) => received.push(state)),
    bridge.onAppLockSettingsChanged!((settings) => received.push(settings)),
    bridge.onAppLockReopen!(() => received.push("reopen")),
    bridge.onVaultBackupsChanged!(() => received.push("backups")),
  ];
  assert.deepEqual(subscribers.map((subscriber) => subscriber.name), [
    "app-lock:runtime-state-changed",
    "app-lock:settings-changed",
    "app-lock:reopen",
    "vault-backups:changed",
  ]);
  subscribers[0].cb({ data: { locked: true, reason: "idle" } });
  subscribers[1].cb({ data: { timeoutMinutes: 30 } });
  subscribers[2].cb({});
  subscribers[3].cb({ data: null });
  assert.deepEqual(received, [
    { locked: true, reason: "idle" },
    { timeoutMinutes: 30 },
    "reopen",
    "backups",
  ]);
  unsubscribers.forEach((unsubscribe) => unsubscribe());
  assert.deepEqual(calls, [
    "lock:manual",
    "timeout:30",
    "unsubscribe:app-lock:runtime-state-changed",
    "unsubscribe:app-lock:settings-changed",
    "unsubscribe:app-lock:reopen",
    "unsubscribe:vault-backups:changed",
  ]);
});

test("app lock timeout setting fails closed without the Go method", async () => {
  const bridge = createWailsRuntimeClient(stubBindings()).transitionBridge;
  await assert.rejects(bridge.setAppLockTimeoutMinutes!(5), /App lock timeout setting unavailable/);
});

test("app lock reset maps onto Go Reset and reports authoritative settings", async () => {
  const seen: string[] = [];
  const disabledSettings: AppLockSettings = { enabled: false, timeoutMinutes: 15, systemUnlockEnabled: false, systemUnlockAutoPromptEnabled: false, passwordVerifier: null };
  const bindings = stubBindings();
  bindings.appLock = {
    GetRuntimeState: async () => ({ initialized: true, locked: true, reason: "startup", version: 1, lastLockedAt: 1, lastUnlockedAt: null, lastActivityAt: null }),
    Reset: async (password: string) => {
      seen.push(`reset:${password}`);
      if (password === "") throw new Error("empty-current");
      if (password !== "secret") throw new Error("incorrect");
      return { initialized: true, locked: false, reason: null, version: 2, lastLockedAt: null, lastUnlockedAt: 5, lastActivityAt: 5 };
    },
    GetSettings: async () => {
      seen.push("settings");
      return disabledSettings;
    },
  };
  const bridge = createWailsRuntimeClient(bindings).transitionBridge;

  assert.deepEqual(await bridge.requestAppLockReset!("secret"), disabledSettings);
  assert.deepEqual(seen, ["reset:secret", "settings"]);
  assert.deepEqual(await bridge.requestAppLockReset!(""), { ok: false, error: "empty-current" });
  assert.deepEqual(await bridge.requestAppLockReset!("wrong"), { ok: false, error: "incorrect" });

  // Without the Go Reset binding the mapping fails closed with a typed code.
  const degraded = createWailsRuntimeClient(stubBindings()).transitionBridge;
  assert.deepEqual(await degraded.requestAppLockReset!("secret"), { ok: false, error: "incorrect" });
});

test("app lock enable maps onto Go Enable with the new password", async () => {
  const seen: string[] = [];
  const enabledSettings: AppLockSettings = { enabled: true, timeoutMinutes: 15, systemUnlockEnabled: false, systemUnlockAutoPromptEnabled: false, passwordVerifier: { version: 1, algorithm: "PBKDF2-SHA256", iterations: 210000, salt: "s", hash: "h" } };
  const bindings = stubBindings();
  bindings.appLock = {
    GetRuntimeState: async () => ({ initialized: true, locked: false, reason: null, version: 1, lastLockedAt: null, lastUnlockedAt: 1, lastActivityAt: 1 }),
    Enable: async (password: string) => {
      seen.push(`enable:${password}`);
      if (password === "short") throw new Error("app lock: password too short: 5 < 4");
      return { initialized: true, locked: true, reason: "password", version: 2, lastLockedAt: 5, lastUnlockedAt: null, lastActivityAt: null };
    },
    GetSettings: async () => {
      seen.push("settings");
      return enabledSettings;
    },
  };
  const bridge = createWailsRuntimeClient(bindings).transitionBridge;

  assert.deepEqual(await bridge.requestAppLockEnable!("new-pass"), enabledSettings);
  assert.deepEqual(seen, ["enable:new-pass", "settings"]);
  assert.deepEqual(await bridge.requestAppLockEnable!(""), { ok: false, error: "empty-next" });
  assert.deepEqual(await bridge.requestAppLockEnable!("short"), { ok: false, error: "incorrect" });

  // Without the Go Enable binding the mapping fails closed with a typed code.
  const degraded = createWailsRuntimeClient(stubBindings()).transitionBridge;
  assert.deepEqual(await degraded.requestAppLockEnable!("new-pass"), { ok: false, error: "incorrect" });
});

test("local browsing uses native paths through the bridge and fails without filesystem bindings", async () => {
  let listing!: ReturnType<typeof useSftpDirectoryListing>;
  function Probe() {
    listing = useSftpDirectoryListing();
    return null;
  }
  renderToStaticMarkup(createElement(Probe));
  const previousClient = getActiveRuntimeClient();
  try {
    const bindings = stubBindings();
    const home = "C:\\Users\\actual-user";
    const uploads: string[] = [];
    bindings.filesystem = {
      HomeDir: async () => home,
      ListDir: async (path) => {
        assert.equal(path, home);
        return [{ name: "real.pdf", type: "file", size: "7", lastModified: "2026-09-11T00:00:00Z" }];
      },
    };
    bindings.transfer = {
      Start: async request => { uploads.push(request.sourcePath); return { taskId: request.taskId, state: 'completed', totalBytes: 7, doneBytes: 7 }; },
      Progress: async () => { throw new Error('Already completed'); },
    };
    const client = createWailsRuntimeClient(bindings);
    setActiveRuntimeClient(client);
    assert.equal(await client.sftp.getHomeDir!(), home);
    assert.equal((await client.sftp.listLocalDir!(home))[0].name, "real.pdf");
    const localHome = await listing.getLocalHomeDir();
    assert.equal(localHome, home);
    const files = await listing.listLocalFiles(localHome);
    assert.equal(files.length, 1);
    assert.equal(files[0].size, 7);
    await client.transitionBridge.startStreamTransfer!({
      transferId: "upload-1", sourceType: "local", targetType: "sftp",
      sourcePath: `${localHome}\\${files[0].name}`, targetPath: "/real.pdf", targetSftpId: "sftp-1",
    });
    assert.deepEqual(uploads, [`${home}\\real.pdf`]);

    bindings.filesystem.ListDir = async () => [];
    assert.deepEqual(await listing.listLocalFiles(home), []);
    bindings.filesystem.ListDir = async () => { throw new Error("directory inaccessible"); };
    await assert.rejects(listing.listLocalFiles(home), /directory inaccessible/);
    bindings.filesystem.HomeDir = async () => "";
    await assert.rejects(listing.getLocalHomeDir(), /home directory unavailable/);

    setActiveRuntimeClient(createWailsRuntimeClient(stubBindings()));
    await assert.rejects(listing.getLocalHomeDir(), /getHomeDir.*not available/);
    await assert.rejects(listing.listLocalFiles(home), /listLocalDir.*not available/);

    setActiveRuntimeClient(undefined);
    assert.ok((await listing.listLocalFiles("C:/Users/damao/Documents")).some((file) => file.name === "report.pdf"));
  } finally {
    setActiveRuntimeClient(previousClient);
  }
});

test("SFTP terminal actions resolve UI IDs and keep native clipboard text intact", async () => {
  const writes: unknown[][] = [];
  const bindings = stubBindings({
    Connect: async () => "native-a",
    Write: (...args) => { writes.push(args); },
    GetSessionPwd: async (id, options) => {
      assert.deepEqual(options, { allowHomeFallback: false, allowLoginShellFallback: false, timeoutMs: 350 });
      return { success: id === "native-a", cwd: "/srv/a b'\u76ee\u5f55" };
    },
    GetSessionRemoteInfo: async () => ({ success: true, remoteSshVersion: "OpenSSH_9" }),
  });
  let clipboard = "";
  bindings.clipboard = { SetText: async text => { clipboard = text; return true; }, Text: async () => clipboard };
  const client = createWailsRuntimeClient(bindings);
  await client.transitionBridge.startSSHSession({ sessionId: "ui-a", hostname: "h", username: "u" });
  client.transitionBridge.writeToSession("ui-a", "cd '/srv/a b'\r");
  assert.equal(writes[0][0], "native-a");
  assert.equal((await client.terminal.getSessionPwd!("ui-a", { allowHomeFallback: false, timeoutMs: 350 })).cwd, "/srv/a b'\u76ee\u5f55");
  await client.transitionBridge.writeClipboardText!("/srv/a b'\u76ee\u5f55");
  assert.equal(await client.transitionBridge.readClipboardText!(), "/srv/a b'\u76ee\u5f55");
});

test("transitionBridge startSSHSession attaches the data plane", async () => {
  const bindings = stubBindings();
  const client = createWailsRuntimeClient(bindings);
  const chunks: string[] = [];
  client.transitionBridge.onSessionData("term-1", (data) => chunks.push(data));
  const id = await client.transitionBridge.startSSHSession({ hostname: "h", username: "u" });
  assert.equal(id, "term-1");
});

test("transitionBridge closeSession disposes the data plane", async () => {
  let disposed = false;
  const bindings = stubBindings();
  bindings.openDataPlane = () => ({
    dispose: () => {
      disposed = true;
    },
  });
  const client = createWailsRuntimeClient(bindings);
  await client.transitionBridge.startSSHSession({ hostname: "h", username: "u" });
  await client.transitionBridge.closeSession("term-1");
  assert.equal(disposed, true);
});

test("optional startup methods remain safe and missing native settings reject explicitly", async () => {
  const client = createWailsRuntimeClient(stubBindings());
  const bridge = client.transitionBridge;
  assert.doesNotThrow(() => {
    void bridge.rendererReady?.();
    void bridge.notifySettingsChanged?.({ key: "x", value: "y" });
  });
  await bridge.setLanguage?.("en");
  await assert.rejects(bridge.getAppLockSettings!(), /App lock settings unavailable/);
});

test("openSettingsWindow creates or focuses a dedicated settings window", async () => {
  const bindings = stubBindings();
  const calls: string[] = [];
  bindings.settings = {
    Open: async () => {
      calls.push("open");
      return true;
    },
    Close: async () => {
      calls.push("close");
    },
  };
  const bridge = createWailsRuntimeClient(bindings).transitionBridge;
  assert.equal(await bridge.openSettingsWindow?.(), true);
  await bridge.closeSettingsWindow?.();
  assert.deepEqual(calls, ["open", "close"]);
});

test("window controls call the Wails native window API", async () => {
  const calls: string[] = [];
  const bindings = stubBindings();
  bindings.window = {
    Minimise: async () => { calls.push("minimize"); },
    ToggleMaximise: async () => { calls.push("maximize"); },
    Close: async () => { calls.push("close"); },
    IsMaximised: async () => true,
    IsFullscreen: async () => false,
  };
  const bridge = createWailsRuntimeClient(bindings).transitionBridge;
  await bridge.windowMinimize?.();
  assert.equal(await bridge.windowMaximize?.(), true);
  assert.equal(await bridge.windowIsMaximized?.(), true);
  assert.equal(await bridge.windowIsFullscreen?.(), false);
  await bridge.windowClose?.();
  assert.deepEqual(calls, ["minimize", "maximize", "close"]);
});

test("windowClose routes through the lifecycle quit guard when available", async () => {
  const calls: string[] = [];
  const bindings = stubBindings();
  bindings.windowLifecycle = {
    RequestClose: async (name) => {
      calls.push(`requestClose:${name}`);
      return { success: true, guarded: true };
    },
  };
  bindings.window = {
    Minimise: async () => { calls.push("minimize"); },
    ToggleMaximise: async () => undefined,
    Hide: async () => { calls.push("hide"); },
    Close: async () => { calls.push("close"); },
    IsMaximised: async () => false,
    IsFullscreen: async () => false,
  };
  const bridge = createWailsRuntimeClient(bindings).transitionBridge;
  await bridge.windowClose?.();
  // Outside a browser location (Node tests) the caller resolves to the main
  // window, which must reach the Go-side guard instead of hiding silently.
  assert.deepEqual(calls, ["requestClose:main"]);
});

test("windowClose falls back to the legacy hide when RequestClose is missing", async () => {
  const calls: string[] = [];
  const bindings = stubBindings();
  bindings.window = {
    Minimise: async () => undefined,
    ToggleMaximise: async () => undefined,
    Hide: async () => { calls.push("hide"); },
    Close: async () => { calls.push("close"); },
    IsMaximised: async () => false,
    IsFullscreen: async () => false,
  };
  const bridge = createWailsRuntimeClient(bindings).transitionBridge;
  await bridge.windowClose?.();
  assert.deepEqual(calls, ["hide"]);
});

test("startLocalSession attaches the data plane", async () => {
  const bindings = stubBindings();
  const client = createWailsRuntimeClient(bindings);
  const id = await client.transitionBridge.startLocalSession?.({ shell: "cmd.exe" });
  assert.equal(id, "local-1");
});

test("startMoshSession and startEtSession forward proxy and jump hosts to the Go bridge", async () => {
  const seen: Array<Record<string, unknown>> = [];
  const bindings = stubBindings({
    StartMosh: async (request) => {
      seen.push(request as Record<string, unknown>);
      return "mosh-1";
    },
    StartEt: async (request) => {
      seen.push(request as Record<string, unknown>);
      return "et-1";
    },
  });
  const client = createWailsRuntimeClient(bindings);
  const options = {
    hostname: "h",
    username: "u",
    proxy: { type: "socks5", host: "127.0.0.1", port: 1080, username: "p", password: "s" },
    jumpHosts: [{ hostname: "jump", username: "bastion", port: 2222 }],
  };
  await client.transitionBridge.startMoshSession!(options);
  await client.transitionBridge.startEtSession!(options);
  assert.equal(seen.length, 2);
  for (const request of seen) {
    assert.equal(request.proxyUrl, "socks5://p:s@127.0.0.1:1080");
    assert.deepEqual(request.proxyCommand, "");
    assert.equal((request.jumpHosts as Array<Record<string, unknown>>)[0].hostname, "jump");
    assert.equal((request.jumpHosts as Array<Record<string, unknown>>)[0].port, 2222);
  }
});

test("onHelperLifecycle fans mosh/et lifecycle events and restartHelperSession maps aliases", async () => {
  const listeners = new Map<string, Array<(event: { data?: unknown }) => void>>();
  const restarted: string[] = [];
  const bindings = stubBindings({
    StartMosh: async () => "mosh-1",
    RestartHelper: async (sessionID) => {
      if (sessionID !== "mosh-1") throw new Error("helper session not found");
      restarted.push(sessionID);
      return { state: "running", attempt: 0, sessionId: sessionID };
    },
  });
  bindings.events = {
    On: (name, callback) => {
      const set = listeners.get(name) ?? [];
      set.push(callback);
      listeners.set(name, set);
      return () => undefined;
    },
  };
  const client = createWailsRuntimeClient(bindings);
  const seen: Array<{ state: string; sessionId: string }> = [];
  const dispose = client.transitionBridge.onHelperLifecycle!("ui-mosh", (evt) => seen.push({ state: evt.state, sessionId: evt.sessionId }));
  // Listeners may also register under the native session id directly.
  const etSeen: Array<{ state: string; sessionId: string }> = [];
  client.transitionBridge.onHelperLifecycle!("et-1", (evt) => etSeen.push({ state: evt.state, sessionId: evt.sessionId }));
  await client.transitionBridge.startMoshSession!({ sessionId: "ui-mosh", hostname: "h", username: "u" });
  listeners.get("mosh:lifecycle")?.[0]({ data: { state: "failed", attempt: 3, sessionId: "mosh-1", kind: "mosh" } });
  listeners.get("et:lifecycle")?.[0]({ data: { state: "running", attempt: 0, sessionId: "et-1" } });
  // The mosh listener matches via its alias and must not see other sessions.
  assert.deepEqual(seen, [{ state: "failed", sessionId: "mosh-1" }]);
  assert.deepEqual(etSeen, [{ state: "running", sessionId: "et-1" }]);
  dispose();
  listeners.get("mosh:lifecycle")?.[0]({ data: { state: "failed", sessionId: "mosh-1" } });
  assert.equal(seen.length, 1);

  assert.deepEqual(
    await client.transitionBridge.restartHelperSession!("ui-mosh"),
    { success: true, state: { state: "running", attempt: 0, sessionId: "mosh-1" } },
  );
  assert.deepEqual(restarted, ["mosh-1"]);
  assert.deepEqual(
    await client.transitionBridge.restartHelperSession!("gone"),
    { success: false, error: "helper session not found" },
  );
});

test("startSSHSession maps jump and MFA onto one Connect payload", async () => {
  const seen: unknown[] = [];
  const bindings = stubBindings({
    Connect: async (request) => {
      seen.push(request);
      return "term-1";
    },
  });
  const client = createWailsRuntimeClient(bindings);
  await client.transitionBridge.startSSHSession({
    hostname: "h",
    username: "u",
    requiresMfa: true,
    jumpHosts: [{ hostname: "jump", username: "bastion", port: 2222 }],
  });
  assert.equal((seen[0] as { enableMfa: boolean }).enableMfa, true);
  assert.equal((seen[0] as { jumpHosts: Array<{ hostname: string }> }).jumpHosts[0].hostname, "jump");
});

test("onKeyboardInteractive fans Wails events to the existing modal queue", async () => {
  const listeners = new Map<string, Array<(event: { data?: unknown }) => void>>();
  const bindings = stubBindings();
  bindings.events = {
    On: (name, callback) => {
      const set = listeners.get(name) ?? [];
      set.push(callback);
      listeners.set(name, set);
      return () => undefined;
    },
  };
  bindings.terminal.RespondKeyboardInteractive = async () => undefined;
  const client = createWailsRuntimeClient(bindings);
  const seen: Array<{ requestId: string }> = [];
  client.transitionBridge.onKeyboardInteractive?.((request) => seen.push({ requestId: request.requestId }));
  listeners.get("ssh:keyboard-interactive")?.[0]({ data: { requestId: "kbd-1", hostname: "h", prompts: [{ prompt: "PIN:", echo: false }] } });
  assert.deepEqual(seen, [{ requestId: "kbd-1" }]);
  const result = await client.transitionBridge.respondKeyboardInteractive?.("kbd-1", ["1234"], false);
  assert.equal(result?.success, true);
});

test("statSftp returns null for missing upload targets instead of throwing", async () => {
  const bindings = stubBindings();
  bindings.sftp.Stat = async () => {
    throw new Error('sftp: "file does not exist" (SSH_FX_NO_SUCH_FILE)');
  };
  const client = createWailsRuntimeClient(bindings);
  const stat = await client.transitionBridge.statSftp?.("sftp-1", "/root/new.png");
  assert.equal(stat, null);
  bindings.sftp.Stat = async () => {
    throw new Error("connection lost");
  };
  await assert.rejects(
    () => client.transitionBridge.statSftp?.("sftp-1", "/root/x"),
    /connection lost/,
  );
});

test("onFilesDropped fans the Wails drop event to listeners", async () => {
  const listeners = new Map<string, Array<(event: { data?: unknown }) => void>>();
  const bindings = stubBindings();
  bindings.events = {
    On: (name, callback) => {
      const set = listeners.get(name) ?? [];
      set.push(callback);
      listeners.set(name, set);
      return () => undefined;
    },
  };
  const client = createWailsRuntimeClient(bindings);
  const seen: Array<{ filenames: string[] }> = [];
  client.transitionBridge.onFilesDropped?.((payload) => seen.push({ filenames: payload.filenames }));
  listeners.get("lemonssh:files-dropped")?.[0]({
    data: { filenames: ["C:\\a.txt"], x: 1, y: 2, elementDetails: { id: "pane" } },
  });
  listeners.get("lemonssh:files-dropped")?.[0]({
    data: [{ Filenames: ["C:\\b.txt"], X: 3, Y: 4 }],
  });
  assert.deepEqual(seen, [{ filenames: ["C:\\a.txt"] }, { filenames: ["C:\\b.txt"] }]);
});

test("native file and folder drops upload to the original tab and complete without lifecycle events", async (t) => {
  t.mock.method(hostStorageAdapter, "readNumber", () => null);
  const previous = getActiveRuntimeClient();
  const bindings = stubBindings();
  const uploaded: string[] = [];
  const directories: string[] = [];
  const pane = { id: "drop-tab", connection: {
    id: "drop-connection", isLocal: false, hostId: "drop-host", hostLabel: "Drop", currentPath: "/srv",
  } } as SftpPane;
  let activePane = pane;
  bindings.filesystem = {
    StatPath: async (path) => {
      // Simulate a tab switch during native metadata lookup.
      activePane = { ...pane, id: "other-tab", connection: { ...pane.connection!, id: "other-connection", currentPath: "/other" } };
      return { name: path.split("\\").pop()!, isDir: path.endsWith("folder"), size: 3 };
    },
    ListDir: async (path) => path.endsWith("empty") ? [] : [
      { name: "nested.txt", type: "file", size: "3", lastModified: "2026-09-11T00:00:00Z" },
      { name: "empty", type: "directory", size: "0", lastModified: "2026-09-11T00:00:00Z" },
    ],
  };
  bindings.sftp.Stat = async () => { throw new Error("no such file"); };
  bindings.transfer = {
    Start: async request => {
      assert.equal(request.targetSessionId, 'original-sftp');
      uploaded.push(`${request.sourcePath} -> ${request.targetPath}`);
      return { taskId: request.taskId, state: 'completed', totalBytes: 3, doneBytes: 3 };
    },
    Progress: async () => { throw new Error('Already completed'); },
  };
  bindings.sftp.Mkdir = async (_id, path) => { directories.push(path); };
  let operations!: ReturnType<typeof useSftpExternalOperations>;
  function Probe() {
    operations = useSftpExternalOperations({
      ownerId: "native-drop-regression", getActivePane: () => activePane,
      getPaneByTabId: (id) => id === pane.id ? pane : activePane,
      getPaneByConnectionId: () => pane, refresh: async () => {},
      sftpSessionsRef: { current: new Map([["drop-connection", "original-sftp"]]) },
      connectionCacheKeyMapRef: { current: new Map([["drop-connection", "original-endpoint"]]) },
    });
    return null;
  }
  let unsubscribeTransferEvents: (() => void) | undefined;
  try {
    const client = createWailsRuntimeClient(bindings);
    setActiveRuntimeClient(client);
    unsubscribeTransferEvents = client.transitionBridge.onGlobalSftpTransferEvent?.(event => {
      sftpTransferCenterStore.ingestBackgroundEvent(event);
    });
    renderToStaticMarkup(createElement(Probe));
    const results = await operations.uploadExternalPaths("left", ["C:\\drop\\one.txt", "C:\\drop\\folder"]);
    assert.ok(results.length >= 2 && results.every((result) => result.success), JSON.stringify(results));
    assert.deepEqual(uploaded, [
      "C:\\drop\\one.txt -> /srv/one.txt",
      "C:\\drop\\folder\\nested.txt -> /srv/folder/nested.txt",
    ]);
    assert.ok(directories.includes("/srv/folder/empty"));
    const tasks = sftpTransferCenterStore.getSnapshot().tasks.filter((task) => task.ownerId === "native-drop-regression");
    assert.ok(tasks.length >= 2);
    assert.ok(tasks.every((task) => task.status === "completed"), JSON.stringify(tasks.map((task) => [task.fileName, task.status])));
  } finally {
    unsubscribeTransferEvents?.();
    for (const task of sftpTransferCenterStore.getSnapshot().tasks) {
      if (task.ownerId === "native-drop-regression") sftpTransferCenterStore.dismiss(task.id);
    }
    setActiveRuntimeClient(previous);
  }
});

test("native folder scanning preserves empty folders, skips directory links, and enforces cancellation and limits", async () => {
  const bindings = stubBindings();
  const reads: string[] = [];
  bindings.filesystem = {
    ListDir: async (path) => {
      reads.push(path);
      return path === "C:\\folder" ? [
        { name: "empty", type: "directory", size: "0", lastModified: "2026-09-11T00:00:00Z" },
        { name: "loop", type: "symlink", linkTarget: "directory", size: "0", lastModified: "2026-09-11T00:00:00Z" },
      ] : [];
    },
  };
  const client = createWailsRuntimeClient(bindings);
  const tree = await client.sftp.listLocalTree!("C:\\folder");
  assert.deepEqual(tree.map((row) => row.relativePath), ["folder", "folder/empty"]);
  assert.deepEqual(reads, ["C:\\folder", "C:\\folder\\empty"]);
  await assert.rejects(client.sftp.listLocalTree!("C:\\folder", { limits: { maxEntries: 1 } }), /limit exceeded/);
  await assert.rejects(client.sftp.listLocalTree!("C:\\folder", {
    scanId: "cancel-scan", onEntries: () => { void client.sftp.cancelLocalTreeScan!("cancel-scan"); },
  }), /Drop scan cancelled/);
  assert.equal((await client.sftp.listLocalTree!("C:\\folder", { scanId: "cancel-scan" })).length, 2);
});

test("getPathForFile ignores the WebView2 path property", () => {
  const client = createWailsRuntimeClient(stubBindings());
  const file = { name: "a.txt", path: "C:\\Users\\Lemon\\a.txt" } as File & { path: string };
  assert.equal(client.transitionBridge.getPathForFile?.(file), undefined);
});

test("extractLocalArchive fails closed when filesystem ExtractArchive is missing", async () => {
  const client = createWailsRuntimeClient(stubBindings());
  const result = await client.transitionBridge.extractLocalArchive?.("/tmp/a.zip");
  assert.equal(result?.success, false);
});

test("extractLocalArchive calls filesystem ExtractArchive", async () => {
  const seen: string[] = [];
  const bindings = stubBindings();
  bindings.filesystem = {
    ExtractArchive: async (archivePath, destinationRoot) => {
      seen.push(archivePath, destinationRoot);
      return 2;
    },
  };
  const client = createWailsRuntimeClient(bindings);
  const result = await client.transitionBridge.extractLocalArchive?.("/tmp/dir/a.zip");
  assert.equal(result?.success, true);
  assert.equal(seen[0], "/tmp/dir/a.zip");
  assert.match(seen[1], /[/\\]tmp[/\\]dir$/);
});

test("pauseTransfer reaches the Go transfer service", async () => {
  const paused: string[] = [];
  const bindings = stubBindings();
  bindings.transfer = {
    Start: async () => ({ taskId: 't-1', state: 'running', totalBytes: 10, doneBytes: 2 }),
    Progress: async () => ({ taskId: 't-1', state: 'paused', totalBytes: 10, doneBytes: 2 }),
    Pause: async (taskID) => {
      paused.push(taskID);
    },
  };
  const client = createWailsRuntimeClient(bindings);
  await client.transitionBridge.pauseTransfer?.("t-1");
  assert.deepEqual(paused, ["t-1"]);
});

test("drainDeepLinks drains cold-start intents before enabling live delivery", async () => {
  const calls: string[] = [];
  const bindings = stubBindings();
  bindings.deepLink = {
    Ready: async () => {
      calls.push("ready");
    },
    Drain: async () => {
      calls.push("drain");
      return [{ Kind: "ssh", Host: "lab", Port: "22", Username: "root" }];
    },
  };
  const client = createWailsRuntimeClient(bindings);
  const actions = await client.transitionBridge.drainDeepLinks?.();
  assert.deepEqual(calls, ["drain", "ready"]);
  assert.equal((actions?.[0] as { Host?: string }).Host, "lab");
});

test("startCompressedUpload fails closed until a compressed-upload owner exists", async () => {
  const client = createWailsRuntimeClient(stubBindings());
  const result = await client.transitionBridge.startCompressedUpload?.({
    compressionId: "c1",
    folderPath: "/tmp/dir",
    targetPath: "/remote",
    sftpId: "sftp-1",
    folderName: "dir",
    totalBytes: 1,
  });
  assert.equal(result?.success, false);
});

test("startCompressedUpload calls the scheduler compressed owner", async () => {
  const seen: string[] = [];
  const bindings = stubBindings();
  bindings.transfer = {
    Start: async () => { throw new Error('wrong start'); },
    StartCompressed: async request => { seen.push(request.targetSessionId, request.sourcePath, request.targetPath); return { taskId:request.taskId,state:'completed',totalBytes:12,doneBytes:12 }; },
    Progress: async () => ({taskId:'c1',state:'completed',totalBytes:12,doneBytes:12}),
    Pause:async()=>{},Resume:async()=>{},Cancel:async()=>{},
  };
  const client = createWailsRuntimeClient(bindings);
  const result = await client.transitionBridge.startCompressedUpload?.({
    compressionId: "c1",
    folderPath: "/tmp/dir",
    targetPath: "/remote",
    sftpId: "sftp-1",
    folderName: "dir",
    totalBytes: 1,
  });
  assert.equal(result?.success, true);
  assert.deepEqual(seen, ["sftp-1", "/tmp/dir", "/remote/dir.zip"]);
});

test("extractSftpArchive fails closed until a remote extract owner exists", async () => {
  const client = createWailsRuntimeClient(stubBindings());
  const result = await client.transitionBridge.extractSftpArchive?.("sftp-1", "/tmp/a.zip");
  assert.equal(result?.success, false);
});

test("extractSftpArchive calls SFTP ExtractArchive", async () => {
  const seen: string[] = [];
  const bindings = stubBindings();
  bindings.sftp.ExtractArchive = async (sftpID, remotePath) => {
    seen.push(sftpID, remotePath);
    return 1;
  };
  const client = createWailsRuntimeClient(bindings);
  const result = await client.transitionBridge.extractSftpArchive?.("sftp-1", "/opt/a.zip");
  assert.equal(result?.success, true);
  assert.deepEqual(seen, ["sftp-1", "/opt/a.zip"]);
});

test("extractSftpArchive forwards the filename encoding", async () => {
  const seen: unknown[] = [];
  const bindings = stubBindings();
  bindings.sftp.ExtractArchive = async (sftpID, remotePath, encoding) => {
    seen.push(sftpID, remotePath, encoding);
    return 1;
  };
  const client = createWailsRuntimeClient(bindings);
  const result = await client.transitionBridge.extractSftpArchive?.("sftp-1", "/opt/a.zip", "gb18030");
  assert.equal(result?.success, true);
  assert.deepEqual(seen, ["sftp-1", "/opt/a.zip", "gb18030"]);
});

test("listSftp forwards the filename encoding to the Go binding", async () => {
  const seen: unknown[] = [];
  const bindings = stubBindings();
  bindings.sftp.List = async (sftpID, path, encoding) => {
    seen.push(sftpID, path, encoding);
    return [];
  };
  const client = createWailsRuntimeClient(bindings);
  await client.transitionBridge.listSftp?.("sftp-1", "/data", "gb18030");
  await client.transitionBridge.listSftp?.("sftp-1", "/data");
  assert.deepEqual(seen, ["sftp-1", "/data", "gb18030", "sftp-1", "/data", undefined]);
});

test("setSessionEncoding pins the Go input charset and swaps the live data-plane decoder", async () => {
  const pinned: Array<{ sessionID: string; encoding: string }> = [];
  let planeDecoder: { decode: (bytes: Uint8Array) => string } | undefined;
  const bindings = stubBindings({
    SetSessionEncoding: async (sessionID, encoding) => {
      pinned.push({ sessionID, encoding });
      return { ok: true, encoding };
    },
  });
  const openDataPlane = bindings.openDataPlane!;
  bindings.openDataPlane = (options) => {
    planeDecoder = options.decoder;
    return openDataPlane(options);
  };
  const client = createWailsRuntimeClient(bindings);
  await client.transitionBridge.startSSHSession?.({ sessionId: "ui-1", hostname: "h" } as never);
  assert.ok(planeDecoder);
  assert.equal(planeDecoder!.decode(new TextEncoder().encode("ok")), "ok");

  const result = await client.transitionBridge.setSessionEncoding?.("ui-1", "gb18030");
  assert.deepEqual(result, { ok: true, encoding: "gb18030" });
  // The Go input charset is pinned on the native session id.
  assert.deepEqual(pinned, [{ sessionID: "term-1", encoding: "gb18030" }]);
  // The live plane's decoder now decodes GB18030 output.
  assert.equal(planeDecoder!.decode(new Uint8Array([0xca, 0xfd, 0xbe, 0xdd])), "数据");
});

test("setSessionEncoding without the Go method still swaps the decoder and reports ok:false", async () => {
  let planeDecoder: { decode: (bytes: Uint8Array) => string } | undefined;
  const bindings = stubBindings();
  const openDataPlane = bindings.openDataPlane!;
  bindings.openDataPlane = (options) => {
    planeDecoder = options.decoder;
    return openDataPlane(options);
  };
  const client = createWailsRuntimeClient(bindings);
  await client.transitionBridge.startSSHSession?.({ sessionId: "ui-2", hostname: "h" } as never);
  const result = await client.transitionBridge.setSessionEncoding?.("ui-2", "gb18030");
  assert.deepEqual(result, { ok: false, encoding: "gb18030" });
  assert.equal(planeDecoder!.decode(new Uint8Array([0xca, 0xfd, 0xbe, 0xdd])), "数据");
});

test("registerGlobalHotkey reaches ShortcutService", async () => {
  const seen: string[] = [];
  const bindings = stubBindings();
  bindings.shortcuts = {
    Register: async (raw) => {
      seen.push(raw);
      return { success: true, enabled: true, accelerator: raw };
    },
  };
  const client = createWailsRuntimeClient(bindings);
  const result = await client.transitionBridge.registerGlobalHotkey?.("CmdOrCtrl+Shift+K");
  assert.equal(result?.success, true);
  assert.deepEqual(seen, ["CmdOrCtrl+Shift+K"]);
});

test("openTerminalPopup calls the popup window service", async () => {
  const seen: unknown[] = [];
  const bindings = stubBindings();
  bindings.popup = {
    Open: async (payload) => {
      seen.push(payload);
      return { success: true, popupId: "popup-1" };
    },
  };
  const client = createWailsRuntimeClient(bindings);
  const result = await client.transitionBridge.openTerminalPopup?.({ sessionId: "s1" } as never);
  assert.equal(result?.success, true);
  assert.equal(result?.popupId, "popup-1");
  assert.equal((seen[0] as { sessionId: string }).sessionId, "s1");
});

test("onSessionData fans out chunks from the data plane", async () => {
  let deliver: ((chunk: string) => void) | undefined;
  const bindings = stubBindings();
  bindings.openDataPlane = ({ onData }) => {
    deliver = onData;
    return { dispose: () => undefined };
  };
  const client = createWailsRuntimeClient(bindings);
  const seen: string[] = [];
  const dispose = client.transitionBridge.onSessionData("term-1", (data) => seen.push(data));
  await client.transitionBridge.startSSHSession({ hostname: "h", username: "u" });
  deliver?.("prompt$ ");
  assert.deepEqual(seen, ["prompt$ "]);
  dispose();
  deliver?.("ignored");
  assert.deepEqual(seen, ["prompt$ "]);
});

test("script recording methods reach the Go recorder and other script methods stay unmigrated", async () => {
  const started: string[] = [];
  const bindings = stubBindings();
  bindings.script = {
    StartRecording: async (sessionID: string) => {
      started.push(sessionID);
      return { ok: true };
    },
    StopRecording: async () => ({ steps: [], code: "" }),
    AppendRecordingStep: async () => ({ ok: true }),
    Run: async (request: { sessionId: string; content: string }) => {
      assert.equal(request.sessionId, "s1");
      return { ok: true, runId: "run-1", runIds: ["run-1"] };
    },
    Stop: async () => ({ ok: true }),
    GetRuns: async () => [],
  };
  const client = createWailsRuntimeClient(bindings);
  assert.deepEqual(await client.script.scriptRecordingStart("s1"), { ok: true });
  assert.deepEqual(started, ["s1"]);
  assert.equal(typeof client.transitionBridge.scriptRecordingStart, "function");
  assert.deepEqual(await client.transitionBridge.scriptRecordingStart!("s1"), { ok: true });
  assert.deepEqual(started, ["s1", "s1"]);
  assert.deepEqual(await client.script.scriptRecordingStop("s1"), { steps: [], code: "" });
  assert.deepEqual(await client.script.scriptRecordingAppendStep("s1", { type: "send", value: "ls" }), { ok: true });
  const ran = await client.transitionBridge.scriptRun!({ sessionId: "s1", content: "await nct.session.sleep(1);\nawait main();" });
  assert.deepEqual(ran, { runId: "run-1", runIds: ["run-1"] });
});

test("script dialog response JSON-encodes form objects", async () => {
  const seen: Array<{ requestId: string; value: string; cancelled: boolean }> = [];
  const bindings = stubBindings();
  bindings.script = {
    ResolveDialog: async (requestId: string, value: string, cancelled: boolean) => {
      seen.push({ requestId, value, cancelled });
      return true;
    },
  };
  const client = createWailsRuntimeClient(bindings);
  const responded = await client.script.scriptDialogResponse!("dlg-1", { env: "prod", restart: true });
  assert.deepEqual(responded, { ok: true });
  assert.deepEqual(seen, [{
    requestId: "dlg-1",
    value: JSON.stringify({ env: "prod", restart: true }),
    cancelled: false,
  }]);
});

test("script run maps renderer session aliases onto native terminal ids", async () => {
  const seen: string[] = [];
  const bindings = stubBindings();
  bindings.script = {
    Run: async (request: { sessionId: string }) => {
      seen.push(request.sessionId);
      return { ok: true, runId: "run-2", runIds: ["run-2"] };
    },
    GetRuns: async () => [{ runId: "run-2", sessionId: "term-1", status: "running", startedAt: 1, logs: [] }],
  };
  const client = createWailsRuntimeClient(bindings);
  await client.transitionBridge.startSSHSession({ sessionId: "ui-session", hostname: "host", username: "user" });
  const ran = await client.transitionBridge.scriptRun!({ sessionId: "ui-session", content: "await nct.session.sleep(1);\nawait main();" });
  assert.deepEqual(ran, { runId: "run-2", runIds: ["run-2"] });
  assert.deepEqual(seen, ["term-1"]);
  const runs = await client.transitionBridge.scriptGetRuns!("ui-session");
  assert.equal(runs[0]?.sessionId, "ui-session");
});

test("script execution forwards permissions, metadata and screen snapshots", async () => {
  const bindings = stubBindings();
  const handlers = new Map<string, (event: { data?: unknown }) => void>();
  bindings.events = { On: (name, callback) => { handlers.set(name, callback); return () => { handlers.delete(name); }; } };
  let request: Record<string, unknown> | undefined;
  let snapshot: unknown;
  bindings.script = {
    Run: async value => { request = value; return { ok: true, runId: 'script' }; },
    ResolveScreenSnapshot: async (_id, value) => { snapshot = value; return true; },
  };
  const bridge = createWailsRuntimeClient(bindings).transitionBridge;
  await bridge.startSSHSession({ sessionId: 'ui-script', hostname: 'host', username: 'user' });
  await bridge.scriptRun({ sessionId: 'ui-script', content: 'nct.log(1)', permissionMode: 'observer', sessionMeta: { name: 'Prod' } });
  assert.equal(request?.permissionMode, 'observer');
  assert.deepEqual(request?.sessionMeta, { name: 'Prod' });
  assert.equal(request?.sessionId, 'term-1');
  let received = '';
  bridge.onScriptScreenSnapshotRequest(({ sessionId }) => { received = sessionId; });
  handlers.get('lemonssh:script:screen-snapshot-request')?.({ data: { requestId: 'screen', sessionId: 'term-1' } });
  assert.equal(received, 'ui-script');
  const screen = { rows: 24, cols: 80, currentRow: 2, lines: ['prompt'] };
  assert.deepEqual(await bridge.scriptScreenSnapshotResponse('screen', screen), { ok: true });
  assert.deepEqual(snapshot, screen);
});

test("cloud OAuth methods surface on the sync port and transition bridge", async () => {
  const calls: string[] = [];
  const bindings = stubBindings();
  bindings.sync = {
    GithubStartDeviceFlow: async (options: { clientId?: string; scope?: string }) => {
      calls.push("device-flow");
      assert.equal(options.clientId, "cid");
      return { deviceCode: "dc", userCode: "uc", verificationUri: "https://github.com/login/device", expiresAt: 1 };
    },
    GoogleGetUserInfo: async (_options: { accessToken: string }) => ({ email: "me@example.com" }),
  } as never;
  const client = createWailsRuntimeClient(bindings);
  assert.equal(typeof client.sync.githubStartDeviceFlow, "function", "sync port must expose the camelCase facade");
  const device = await client.transitionBridge.githubStartDeviceFlow!({ clientId: "cid" });
  assert.equal(device.userCode, "uc");
  assert.equal(typeof client.transitionBridge.googleGetUserInfo, "function");
  assert.deepEqual(calls, ["device-flow"]);
});

test("openProviderConsole forwards the allow-listed provider to the Go bridge", async () => {
  const requested: string[] = [];
  const bindings = stubBindings();
  bindings.sync = {
    OpenProviderConsole: async (provider: 'github' | 'google' | 'onedrive') => {
      requested.push(provider);
    },
  } as never;
  const client = createWailsRuntimeClient(bindings);
  assert.equal(typeof client.transitionBridge.openProviderConsole, "function");
  await client.transitionBridge.openProviderConsole!('github');
  assert.deepEqual(requested, ['github']);
  await client.sync.openProviderConsole!('google');
  assert.deepEqual(requested, ['github', 'google']);
});

test("port forward start normalizes Go result into the renderer contract", async () => {
  const bindings = stubBindings();
  const started: unknown[] = [];
  bindings.forward = {
    Start: async (...args: unknown[]) => {
      started.push(args);
      return { TunnelID: args[0], Success: true, Status: "active" };
    },
    Stop: async (id: string) => ({ TunnelID: id, Success: true, Status: "inactive" }),
    StopByRuleId: async () => ({ stopped: 1 }),
    List: async () => [{ ruleId: "rule-1", tunnelId: "pf-rule-1-1", type: "local", status: "active" }],
    Snapshot: async (id: string) => ({ TunnelID: id, Success: true, Status: "active" }),
  };
  const client = createWailsRuntimeClient(bindings);
  const result = await client.transitionBridge.startPortForward!({
    tunnelId: "pf-rule-1-1",
    type: "local",
    localPort: 18080,
    remoteHost: "127.0.0.1",
    remotePort: 80,
    hostname: "lab",
    username: "root",
  });
  assert.deepEqual(result, { tunnelId: "pf-rule-1-1", success: true, cancelled: false, blockedByCleanup: false, reused: false, status: "active", error: undefined });
  assert.equal(started.length, 1);
  assert.equal(await client.transitionBridge.stopPortForwardByRuleId?.("rule-1").then((value) => value.stopped), 1);
});

const stubForwardRuntimeEvents = (
  bindings: WailsBindingDeps,
) => {
  const listeners = new Map<string, Array<(event: { data?: unknown }) => void>>();
  const unsubscribed: string[] = [];
  bindings.events = {
    On: (name, callback) => {
      const set = listeners.get(name) ?? [];
      set.push(callback);
      listeners.set(name, set);
      return () => {
        unsubscribed.push(name);
      };
    },
  };
  return {
    listeners,
    unsubscribed,
    feed: (name: string) => listeners.get(name)!,
  };
};

test("port forward runtime subscription returns the snapshot and fans ordered Go events", async () => {
  const bindings = stubBindings();
  const { listeners, unsubscribed, feed } = stubForwardRuntimeEvents(bindings);
  bindings.forward = {
    Start: async (...args: unknown[]) => ({ TunnelID: args[0], Success: true, Status: "active" }),
    Stop: async (id: string) => ({ TunnelID: id, Success: true, Status: "inactive" }),
    List: async () => [],
    Snapshot: async (id: string) => ({ TunnelID: id, Success: true, Status: "inactive" }),
    RuntimeSnapshot: async () => ({
      epoch: "wails",
      revision: 7,
      records: [{ ruleId: "rule-1", tunnelId: "pf-rule-1-1", phase: "active", revision: 7, updatedAt: 1 }],
    }),
  };
  const client = createWailsRuntimeClient(bindings);

  // subscribePortForwardRuntime hands out the authoritative snapshot that
  // seeds the renderer's epoch/revision tracking.
  const snapshot = await client.transitionBridge.subscribePortForwardRuntime!();
  assert.deepEqual(snapshot, {
    epoch: "wails",
    revision: 7,
    records: [{ ruleId: "rule-1", tunnelId: "pf-rule-1-1", phase: "active", revision: 7, updatedAt: 1 }],
  });

  // onPortForwardRuntime arms exactly one native subscription and delivers
  // the Go payloads in arrival order.
  const seen: Array<{ epoch: string; revision: number; kind: string }> = [];
  const unsubscribe = client.transitionBridge.onPortForwardRuntime!((event) =>
    seen.push({ epoch: event.epoch, revision: event.revision, kind: event.kind }),
  );
  assert.equal(listeners.get("lemonssh:port-forward:runtime")?.length, 1);
  feed("lemonssh:port-forward:runtime")[0]({
    data: { epoch: "wails", revision: 8, kind: "upsert", record: { ruleId: "rule-2", tunnelId: "pf-rule-2-2", phase: "active", revision: 8, updatedAt: 2 } },
  });
  feed("lemonssh:port-forward:runtime")[0]({
    data: { epoch: "wails", revision: 9, kind: "remove", tunnelId: "pf-rule-2-2", ruleId: "rule-2" },
  });
  assert.deepEqual(seen, [
    { epoch: "wails", revision: 8, kind: "upsert" },
    { epoch: "wails", revision: 9, kind: "remove" },
  ]);

  unsubscribe();
  assert.deepEqual(unsubscribed, ["lemonssh:port-forward:runtime"]);
  await assert.deepEqual(await client.transitionBridge.unsubscribePortForwardRuntime!(), { success: true });
});

test("onPortForwardStatus filters the runtime feed down to one tunnel", async () => {
  const bindings = stubBindings();
  const { feed } = stubForwardRuntimeEvents(bindings);
  bindings.forward = {
    Start: async (...args: unknown[]) => ({ TunnelID: args[0], Success: true, Status: "active" }),
    Stop: async (id: string) => ({ TunnelID: id, Success: true, Status: "inactive" }),
    List: async () => [],
    Snapshot: async (id: string) => ({ TunnelID: id, Success: true, Status: "active" }),
  };
  const client = createWailsRuntimeClient(bindings);

  const seen: Array<{ status: string; error?: string }> = [];
  const unsubscribe = client.transitionBridge.onPortForwardStatus!("pf-rule-1-1", (status, error) =>
    seen.push({ status, error }),
  );
  const runtime = feed("lemonssh:port-forward:runtime");
  runtime[0]({
    data: { epoch: "wails", revision: 2, kind: "upsert", record: { ruleId: "rule-1", tunnelId: "pf-rule-1-1", phase: "error", error: "bind failed", revision: 2, updatedAt: 2 } },
  });
  runtime[0]({
    data: { epoch: "wails", revision: 3, kind: "upsert", record: { ruleId: "rule-9", tunnelId: "pf-rule-9-9", phase: "active", revision: 3, updatedAt: 3 } },
  });
  runtime[0]({
    data: { epoch: "wails", revision: 4, kind: "remove", tunnelId: "pf-rule-1-1", ruleId: "rule-1" },
  });
  assert.deepEqual(seen, [
    { status: "error", error: "bind failed" },
    { status: "inactive", error: undefined },
  ]);
  unsubscribe();

  // subscribePortForward snapshots one tunnel's current status.
  const status = await client.transitionBridge.subscribePortForward!("pf-rule-1-1");
  assert.deepEqual(status, { tunnelId: "pf-rule-1-1", status: "active", error: undefined });
});

test("credentials round-trip through the Go credential provider with the enc:v1 contract", async () => {
  const sealed: string[] = [];
  const bindings = stubBindings();
  bindings.credential = {
    Available: async () => true,
    Seal: async (plaintextBase64: string, purpose: string) => {
      assert.equal(purpose, "cloud-sync-credentials");
      sealed.push(plaintextBase64);
      return Buffer.from(`sealed(${Buffer.from(plaintextBase64, "base64").toString("utf8")})`).toString("base64");
    },
    Open: async (envelopeBase64: string, purpose: string) => {
      assert.equal(purpose, "cloud-sync-credentials");
      const decoded = Buffer.from(envelopeBase64, "base64").toString("utf8");
      const inner = decoded.startsWith("sealed(") ? decoded.slice(7, -1) : decoded;
      return Buffer.from(inner, "utf8").toString("base64");
    },
  } as never;
  const client = createWailsRuntimeClient(bindings);
  assert.equal(await client.files.credentialsAvailable?.(), true);
  const envelope = await client.files.credentialsEncrypt?.("github-token");
  assert.match(envelope, /^enc:v1:/);
  const storedSealed = sealed[0];
  assert.equal(await client.files.credentialsDecrypt?.(envelope), "github-token");
  assert.equal(sealed.length, 1);
  // A plaintext passthrough value decrypts unchanged, matching the Electron contract.
  assert.equal(await client.files.credentialsDecrypt?.("plain-value"), "plain-value");
  void storedSealed;
});

test("cloud sync session and reset methods are reachable on both surfaces", async () => {
  const calls: string[] = [];
  const bindings = stubBindings();
  bindings.sync = {
    // Mirror the generated wire shape: []string arrives wrapped.
    CloudSyncResetEverything: async () => { calls.push("reset"); return { removedKeys: ["key-a"] }; },
    CloudSyncSetSessionPassword: async () => { calls.push("set"); return true; },
    CloudSyncGetSessionPassword: async () => { calls.push("get"); return { password: "p", found: true }; },
    CloudSyncClearSessionPassword: async () => { calls.push("clear"); return { success: true }; },
  } as never;
  const client = createWailsRuntimeClient(bindings);
  assert.deepEqual(await client.sync.cloudSyncResetEverything(), ["key-a"]);
  await client.sync.cloudSyncSetSessionPassword!("p");
  assert.equal(await client.sync.cloudSyncGetSessionPassword!(), "p");
  assert.deepEqual(await client.sync.cloudSyncClearSessionPassword!(), { success: true });
  assert.deepEqual(calls, ["reset", "set", "get", "clear"]);
  assert.equal(typeof client.transitionBridge.cloudSyncResetEverything, "function");
  assert.equal(typeof client.transitionBridge.cloudSyncSetSessionPassword, "function");
  assert.equal(typeof client.transitionBridge.cloudSyncGetSessionPassword, "function");
  assert.equal(typeof client.transitionBridge.cloudSyncClearSessionPassword, "function");
});

test("vault backup methods are reachable on both surfaces", async () => {
  const calls: string[] = [];
  const bindings = stubBindings();
  bindings.sync = {
    GetVaultBackupCapabilities: async () => { calls.push("caps"); return { encryptionAvailable: true }; },
    CreateVaultBackup: async () => { calls.push("create"); return { created: true, backup: { id: "b1" } }; },
    ListVaultBackups: async () => { calls.push("list"); return { backups: [{ id: "b1" }] }; },
    ReadVaultBackup: async () => { calls.push("read"); return { backup: { id: "b1" }, payload: { hosts: [] } }; },
    TrimVaultBackups: async () => { calls.push("trim"); return { deletedCount: 0, keptCount: 1 }; },
    OpenVaultBackupDir: async () => { calls.push("open"); return { success: true, path: "/tmp" }; },
  } as never;
  const client = createWailsRuntimeClient(bindings);
  assert.deepEqual(await client.sync.getVaultBackupCapabilities(), { encryptionAvailable: true });
  assert.equal((await client.sync.createVaultBackup({ payload: { hosts: [] }, reason: "before_restore" })).created, true);
  assert.equal((await client.sync.listVaultBackups()).length, 1);
  assert.equal((await client.sync.readVaultBackup({ id: "b1" })).backup.id, "b1");
  await client.sync.trimVaultBackups({ maxCount: 20 });
  await client.sync.openVaultBackupDir();
  assert.deepEqual(calls, ["caps", "create", "list", "read", "trim", "open"]);
  assert.equal(typeof client.transitionBridge.createVaultBackup, "function");
  assert.equal(typeof client.transitionBridge.listVaultBackups, "function");
});

test("agent interaction bridge fans events lazily and maps decisions onto the Go gate", async () => {
  const listeners = new Map<string, Array<(event: { data?: unknown }) => void>>();
  const bindings = stubBindings();
  bindings.events = {
    On: (name, callback) => {
      const set = listeners.get(name) ?? [];
      set.push(callback);
      listeners.set(name, set);
      return () => undefined;
    },
  };
  const responded: Array<[string, boolean]> = [];
  bindings.agentservice = {
    AgentPendingInteractions: async () => [
      { interactionId: "ia_pending", capabilityId: "lemonssh.exec", summary: { method: "lemonssh/exec", command: "reboot" }, deadlineMs: 4102444800000 },
    ],
    AgentRespondInteraction: async (interactionID: string, approved: boolean) => {
      responded.push([interactionID, approved]);
      if (interactionID === "ia_gone") throw new Error('interaction "ia_gone" is not pending (already resolved or unknown)');
    },
  } as WailsBindingDeps["agentservice"];
  const client = createWailsRuntimeClient(bindings);

  // The "agent:interaction" subscription stays lazy until a listener arrives.
  assert.equal(listeners.has("agent:interaction"), false);

  const seen: Array<{ interactionId: string; capabilityId: string }> = [];
  const dispose = client.transitionBridge.onAgentInteraction!((payload) => seen.push({ interactionId: payload.interactionId, capabilityId: payload.capabilityId }));
  listeners.get("agent:interaction")?.[0]({ data: { interactionId: "ia_1", capabilityId: "lemonssh.exec", description: "Run a command", summary: { command: "reboot" }, deadlineMs: 4102444800000 } });
  // The same event without the Wails data envelope must still reach listeners.
  listeners.get("agent:interaction")?.[0]({ interactionId: "ia_2", capabilityId: "lemonssh.sftp.write" });
  assert.deepEqual(seen, [
    { interactionId: "ia_1", capabilityId: "lemonssh.exec" },
    { interactionId: "ia_2", capabilityId: "lemonssh.sftp.write" },
  ]);
  dispose();
  listeners.get("agent:interaction")?.[0]({ data: { interactionId: "ia_3", capabilityId: "lemonssh.x" } });
  assert.deepEqual(seen.map((entry) => entry.interactionId), ["ia_1", "ia_2"]);

  assert.deepEqual(await client.transitionBridge.agentPendingInteractions!(), [
    { interactionId: "ia_pending", capabilityId: "lemonssh.exec", summary: { method: "lemonssh/exec", command: "reboot" }, deadlineMs: 4102444800000 },
  ]);
  await client.transitionBridge.agentRespondInteraction!("ia_1", true);
  // Go's typed double-response guard rejects with its own message, untouched.
  await assert.rejects(client.transitionBridge.agentRespondInteraction!("ia_gone", false), /is not pending/);
  assert.deepEqual(responded, [["ia_1", true], ["ia_gone", false]]);
});

test("agent interaction bridge fails closed without the agent service", async () => {
  const bindings = stubBindings() as WailsBindingDeps;
  delete (bindings as Partial<WailsBindingDeps>).agentservice;
  const client = createWailsRuntimeClient(bindings);
  await assert.rejects(client.transitionBridge.agentPendingInteractions!(), /agentPendingInteractions is not available/);
  await assert.rejects(client.transitionBridge.agentRespondInteraction!("ia_1", true), /agentRespondInteraction is not available/);
});

test("passphrase prompts fan out and responses reach the Go broker", async () => {
  const listeners = new Map<string, Array<(event: { data?: unknown }) => void>>();
  const responses: Array<[string, string, boolean]> = [];
  const bindings = stubBindings({
    RespondPassphrase: async (requestId: string, passphrase: string, cancelled: boolean) => {
      responses.push([requestId, passphrase, cancelled]);
      return {};
    },
  });
  bindings.events = {
    On: (name, callback) => {
      const set = listeners.get(name) ?? [];
      set.push(callback);
      listeners.set(name, set);
      return () => undefined;
    },
  };
  const client = createWailsRuntimeClient(bindings);
  const seen: Array<{ requestId: string; keyPath: string; keyName: string; hostname?: string; sessionId?: string; bootEpoch?: number; passphraseInvalid?: boolean }> = [];
  const dispose = client.transitionBridge.onPassphraseRequest!((request) => seen.push(request));
  listeners.get("ssh:passphrase-request")?.[0]({
    data: {
      requestId: "pp-1",
      keyPath: "/home/u/id_ed25519",
      keyName: "id_ed25519",
      hostname: "host-a",
      sessionId: "term-1",
      bootEpoch: 4,
    },
  });
  assert.deepEqual(seen, [{
    requestId: "pp-1",
    keyPath: "/home/u/id_ed25519",
    keyName: "id_ed25519",
    hostname: "host-a",
    sessionId: "term-1",
    bootEpoch: 4,
    passphraseInvalid: false,
  }]);
  await client.transitionBridge.respondPassphrase!("pp-1", "phrase", false);
  assert.deepEqual(responses, [["pp-1", "phrase", false]]);
  // Skip abandons the key: the broker sees an empty cancelled answer.
  await client.transitionBridge.respondPassphraseSkip!("pp-1");
  assert.deepEqual(responses[1], ["pp-1", "", true]);
  dispose();
  listeners.get("ssh:passphrase-request")?.[0]({ data: { requestId: "pp-2" } });
  assert.equal(seen.length, 1);
});

test("passphrase timeout, cancel and auth-failed events map onto the bridge", async () => {
  const listeners = new Map<string, Array<(event: { data?: unknown }) => void>>();
  const bindings = stubBindings();
  bindings.events = {
    On: (name, callback) => {
      const set = listeners.get(name) ?? [];
      set.push(callback);
      listeners.set(name, set);
      return () => undefined;
    },
  };
  const client = createWailsRuntimeClient(bindings);
  const timedOut: string[] = [];
  const cancelled: string[] = [];
  const failed: Array<{ keyPaths: string[]; keyIds?: string[] }> = [];
  client.transitionBridge.onPassphraseTimeout!((event) => timedOut.push(event.requestId));
  client.transitionBridge.onPassphraseCancelled!((event) => cancelled.push(event.requestId));
  client.transitionBridge.onPassphraseAuthFailed!((event) => failed.push({ keyPaths: event.keyPaths, keyIds: event.keyIds }));
  listeners.get("ssh:passphrase-timeout")?.[0]({ data: { requestId: "pp-9" } });
  listeners.get("ssh:passphrase-cancelled")?.[0]({ data: { requestId: "pp-8" } });
  listeners.get("ssh:passphrase-auth-failed")?.[0]({ data: { keyPaths: ["/id"], keyIds: ["k1"] } });
  assert.deepEqual(timedOut, ["pp-9"]);
  assert.deepEqual(cancelled, ["pp-8"]);
  assert.deepEqual(failed, [{ keyPaths: ["/id"], keyIds: ["k1"] }]);
});

test("host key verification events and responses reach the Go broker", async () => {
  const listeners = new Map<string, Array<(event: { data?: unknown }) => void>>();
  const responses: Array<[string, boolean, boolean]> = [];
  const bindings = stubBindings({
    RespondHostKeyVerification: async (requestId: string, accept: boolean, addToKnownHosts: boolean) => {
      responses.push([requestId, accept, addToKnownHosts]);
      return {};
    },
  });
  bindings.events = {
    On: (name, callback) => {
      const set = listeners.get(name) ?? [];
      set.push(callback);
      listeners.set(name, set);
      return () => undefined;
    },
  };
  const client = createWailsRuntimeClient(bindings);
  const seen: Array<{ requestId: string; sessionId: string; hostname: string; port: number; status: string; keyType: string; fingerprint: string; knownFingerprint?: string }> = [];
  const dispose = client.transitionBridge.onHostKeyVerification!((request) => seen.push(request));
  listeners.get("ssh:host-key-verification")?.[0]({
    data: {
      requestId: "hk-1",
      sessionId: "term-2",
      hostname: "127.0.0.1",
      port: 2222,
      status: "changed",
      keyType: "ssh-ed25519",
      fingerprint: "SHA256:abc",
      knownFingerprint: "SHA256:old",
    },
  });
  assert.deepEqual(seen, [{
    requestId: "hk-1",
    sessionId: "term-2",
    hostname: "127.0.0.1",
    port: 2222,
    status: "changed",
    keyType: "ssh-ed25519",
    fingerprint: "SHA256:abc",
    publicKey: undefined,
    knownFingerprint: "SHA256:old",
    bootEpoch: undefined,
  }]);
  await client.transitionBridge.respondHostKeyVerification!("hk-1", true, true);
  assert.deepEqual(responses, [["hk-1", true, true]]);
  dispose();
  listeners.get("ssh:host-key-verification")?.[0]({ data: { requestId: "hk-2" } });
  assert.equal(seen.length, 1);
});

test("execCommand maps the renderer contract onto the Go one-shot exec", async () => {
  const seen: unknown[] = [];
  const bindings = stubBindings({
    ExecCommand: async (request) => {
      seen.push(request);
      return { stdout: "out", stderr: "err", code: 3 };
    },
  });
  const client = createWailsRuntimeClient(bindings);
  const result = await client.transitionBridge.execCommand!({
    hostname: "h",
    username: "root",
    port: 2222,
    password: "pw",
    requiresMfa: true,
    identityFilePaths: ["/id"],
    sessionId: "export-key:1",
    command: "mkdir -p ~/.ssh",
    timeout: 5000,
  });
  assert.deepEqual(result, { stdout: "out", stderr: "err", code: 3 });
  assert.deepEqual(seen, [{
    hostname: "h",
    username: "root",
    port: 2222,
    password: "pw",
    privateKey: "",
    passphrase: "",
    certificate: "",
    proxyUrl: "",
    proxyCommand: "",
    enableMfa: true,
    useAgent: false,
    agentForwarding: false,
    identityFilePaths: ["/id"],
    cols: 80,
    rows: 24,
    term: "xterm-256color",
    verifyHostKeys: true,
    keepaliveInterval: 30,
    keepaliveCountMax: 3,
    forwardX11: false,
    x11Display: "",
    sessionId: "export-key:1",
    bootEpoch: 0,
    sshDebugLogs: false,
    jumpHosts: [],
    command: "mkdir -p ~/.ssh",
    timeoutMs: 5000,
  }]);
});

test("readKnownHosts surfaces the system scan content or null", async () => {
  const bindings = stubBindings({ ReadKnownHosts: async () => "host ssh-ed25519 AAAA\n" } as Partial<WailsBindingDeps["terminal"]>);
  bindings.knownHosts = { ReadKnownHosts: async () => "host ssh-ed25519 AAAA\n" };
  const client = createWailsRuntimeClient(bindings);
  assert.equal(await client.transitionBridge.readKnownHosts?.(), "host ssh-ed25519 AAAA\n");

  const empty = stubBindings();
  empty.knownHosts = { ReadKnownHosts: async () => "" };
  const emptyClient = createWailsRuntimeClient(empty);
  assert.equal(await emptyClient.transitionBridge.readKnownHosts?.(), null);

  const missing = stubBindings();
  const missingClient = createWailsRuntimeClient(missing);
  await assert.rejects(missingClient.transitionBridge.readKnownHosts?.(), /readKnownHosts is not available/);
});

test("update bridge maps the Go UpdateService surface", async () => {
  const calls: string[] = [];
  const bindings = stubBindings() as WailsBindingDeps;
  bindings.lemonssh = {
    Version: async () => ({ name: "LemonSSH", version: "1.2.3", goos: "windows", goarch: "amd64", goVersion: "go1.26" }),
  };
  bindings.update = {
    CheckForUpdate: async () => {
      calls.push("check");
      return { available: true, supported: true, version: "1.3.0", releaseNotes: "notes", releaseDate: "2026-09-01T00:00:00Z" };
    },
    DownloadUpdate: async () => {
      calls.push("download");
      return { success: true };
    },
    InstallUpdate: async () => { calls.push("install"); },
    GetUpdateStatus: async () => ({ status: "ready", percent: 100, error: "", version: "1.3.0", isChecking: false }),
    GetAutoUpdate: async () => ({ enabled: false }),
    SetAutoUpdate: async (enabled) => {
      calls.push(`setAutoUpdate:${enabled}`);
      return { success: true };
    },
  };
  const client = createWailsRuntimeClient(bindings);
  const bridge = client.transitionBridge;

  assert.deepEqual(await bridge.getAppInfo?.(), { name: "LemonSSH", version: "1.2.3", platform: "windows" });
  assert.deepEqual(await bridge.checkForUpdate?.(), {
    available: true,
    supported: true,
    checking: undefined,
    ready: undefined,
    downloading: undefined,
    version: "1.3.0",
    releaseNotes: "notes",
    releaseDate: "2026-09-01T00:00:00Z",
    error: undefined,
  });
  assert.deepEqual(await bridge.downloadUpdate?.(), { success: true });
  assert.deepEqual(await bridge.getUpdateStatus?.(), { status: "ready", percent: 100, error: null, version: "1.3.0", isChecking: false });
  assert.deepEqual(await bridge.getAutoUpdate?.(), { enabled: false });
  await bridge.setAutoUpdate?.(true);
  bridge.installUpdate?.();
  assert.deepEqual(calls, ["check", "download", "setAutoUpdate:true", "install"]);
});

test("update bridge degrades without Go UpdateService", async () => {
  const bindings = stubBindings() as WailsBindingDeps;
  const bridge = createWailsRuntimeClient(bindings).transitionBridge;
  assert.deepEqual(await bridge.checkForUpdate?.(), { available: false, supported: false, error: "Update bridge unavailable" });
  assert.deepEqual(await bridge.downloadUpdate?.(), { success: false, error: "Update bridge unavailable" });
  assert.deepEqual(await bridge.getUpdateStatus?.(), { status: "idle", percent: 0, error: null, version: null });
  assert.deepEqual(await bridge.getAutoUpdate?.(), { enabled: true });
  assert.deepEqual(await bridge.setAutoUpdate?.(false), { success: false });
});

test("update events subscribe to the Go update:* broadcasts", async () => {
  const bindings = stubBindings() as WailsBindingDeps;
  const seen: string[] = [];
  bindings.events = {
    On: (name, callback) => {
      seen.push(name);
      callback({ data: { version: "1.3.0", error: "boom" } });
      return () => undefined;
    },
  };
  const bridge = createWailsRuntimeClient(bindings).transitionBridge;

  let availableVersion = "";
  bridge.onUpdateAvailable?.((info) => { availableVersion = info.version; });
  let errorMessage = "";
  bridge.onUpdateError?.((payload) => { errorMessage = payload.error; });
  let progressNotified = false;
  bridge.onUpdateDownloadProgress?.(() => { progressNotified = true; });
  let downloadedNotified = false;
  bridge.onUpdateDownloaded?.(() => { downloadedNotified = true; });
  let notAvailableNotified = false;
  bridge.onUpdateNotAvailable?.(() => { notAvailableNotified = true; });

  assert.deepEqual(seen, ["update:available", "update:error", "update:download-progress", "update:downloaded", "update:not-available"]);
  assert.equal(availableVersion, "1.3.0");
  assert.equal(errorMessage, "boom");
  assert.equal(progressNotified, true);
  assert.equal(downloadedNotified, true);
  assert.equal(notAvailableNotified, true);
});

test("window lifecycle quit guard and focus recovery map onto the Go WindowLifecycleService", async () => {
  const bindings = stubBindings() as WailsBindingDeps;
  const subscribers: Array<{ name: string; cb: (event: { data?: unknown }) => void }> = [];
  bindings.events = {
    On: (name, callback) => {
      subscribers.push({ name, cb: callback });
      return () => undefined;
    },
  };
  const lifecycleCalls: string[] = [];
  bindings.windowLifecycle = {
    SetCloseToTray: async (enabled) => {
      lifecycleCalls.push(`setCloseToTray:${enabled}`);
      return { success: true, enabled };
    },
    IsCloseToTray: async () => {
      lifecycleCalls.push("isCloseToTray");
      return { success: true, enabled: true };
    },
    SetWindowOpacity: async (opacity) => {
      lifecycleCalls.push(`setWindowOpacity:${opacity}`);
      return opacity < 1;
    },
    ReportDirtyEditorsResult: async (hasDirty) => {
      lifecycleCalls.push(`reportDirtyEditorsResult:${hasDirty}`);
    },
  };
  let focusRequested = false;
  bindings.window = {
    Minimise: async () => undefined,
    ToggleMaximise: async () => undefined,
    Close: async () => undefined,
    IsMaximised: async () => false,
    IsFullscreen: async () => false,
    Focus: async () => { focusRequested = true; },
  };
  const bridge = createWailsRuntimeClient(bindings).transitionBridge;

  const shown: boolean[] = [];
  const willHide: boolean[] = [];
  const focusRequests: boolean[] = [];
  const dirtyChecks: boolean[] = [];
  bridge.onWindowShown?.(() => shown.push(true));
  bridge.onWindowWillHide?.(() => willHide.push(true));
  bridge.onWindowFocusRequested?.(() => focusRequests.push(true));
  bridge.onCheckDirtyEditors?.(() => dirtyChecks.push(true));

  assert.deepEqual(subscribers.map((subscriber) => subscriber.name), [
    "window:shown",
    "window:will-hide",
    "window:focus-requested",
    "window:check-dirty-editors",
  ]);
  subscribers.forEach((subscriber) => subscriber.cb({}));
  assert.deepEqual(shown, [true]);
  assert.deepEqual(willHide, [true]);
  assert.deepEqual(focusRequests, [true]);
  assert.deepEqual(dirtyChecks, [true]);

  assert.deepEqual(await bridge.setCloseToTray?.(true), { success: true, enabled: true });
  assert.deepEqual(await bridge.isCloseToTray?.(), { enabled: true });
  assert.equal(await bridge.setWindowOpacity?.(0.75), true);
  assert.equal(await bridge.setWindowOpacity?.(1), false);
  bridge.reportDirtyEditorsResult?.(true);
  assert.deepEqual(await bridge.windowFocus?.(), true);
  assert.equal(focusRequested, true);

  // Give the fire-and-forget report a tick to land.
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.deepEqual(lifecycleCalls, [
    "setCloseToTray:true",
    "isCloseToTray",
    "setWindowOpacity:0.75",
    "setWindowOpacity:1",
    "reportDirtyEditorsResult:true",
  ]);
});

test("window lifecycle bridge degrades without the Go service", async () => {
  const bindings = stubBindings() as WailsBindingDeps;
  const bridge = createWailsRuntimeClient(bindings).transitionBridge;
  assert.deepEqual(await bridge.setCloseToTray?.(false), { success: false, enabled: false });
  assert.deepEqual(await bridge.isCloseToTray?.(), { enabled: false });
  assert.equal(await bridge.setWindowOpacity?.(0.75), false);
  assert.equal(await bridge.windowFocus?.(), false);
  // Must not throw without a pending Go binding.
  bridge.reportDirtyEditorsResult?.(false);
});

test("tray menu and panel actions map onto the Go TrayService", async () => {
  const bindings = stubBindings() as WailsBindingDeps;
  const trayCalls: Array<{ method: string; arg?: unknown }> = [];
  bindings.tray = {
    SetLanguage: async () => false,
    Quit: async () => undefined,
    UpdateTrayMenuData: async (data) => {
      trayCalls.push({ method: "UpdateTrayMenuData", arg: data });
      return { success: true };
    },
    JumpToSessionFromPanel: async (sessionID) => {
      trayCalls.push({ method: "JumpToSessionFromPanel", arg: sessionID });
      return { success: true };
    },
    ConnectToHost: async (hostID) => {
      trayCalls.push({ method: "ConnectToHost", arg: hostID });
      return { success: true };
    },
    CloseSessionFromPanel: async (sessionID) => {
      trayCalls.push({ method: "CloseSessionFromPanel", arg: sessionID });
      return { success: true };
    },
    OpenMainWindow: async () => {
      trayCalls.push({ method: "OpenMainWindow" });
      return { success: true };
    },
  };
  bindings.trayPanel = {
    Hide: async () => {
      trayCalls.push({ method: "trayPanel.Hide" });
      return true;
    },
    PaintReady: async () => {
      trayCalls.push({ method: "trayPanel.PaintReady" });
      return true;
    },
  };
  const bridge = createWailsRuntimeClient(bindings).transitionBridge;

  assert.deepEqual(await bridge.updateTrayMenuData?.({ sessions: [] }), { success: true });
  assert.deepEqual(await bridge.jumpToSessionFromTrayPanel?.("s1"), { success: true });
  assert.deepEqual(await bridge.connectToHostFromTrayPanel?.("h1"), { success: true });
  assert.deepEqual(await bridge.closeSessionFromTrayPanel?.("s1"), { success: true });
  assert.deepEqual(await bridge.openMainWindow?.(), { success: true });
  assert.deepEqual(await bridge.hideTrayPanel?.(), { success: true });
  assert.equal(await bridge.notifyTrayPanelPaintReady?.(), true);

  // Without Go bindings every call degrades to a failed result instead of throwing.
  const bare = createWailsRuntimeClient(stubBindings() as WailsBindingDeps).transitionBridge;
  assert.deepEqual(await bare.updateTrayMenuData?.({}), { success: false });
  assert.deepEqual(await bare.jumpToSessionFromTrayPanel?.("s1"), { success: false });
  assert.deepEqual(await bare.connectToHostFromTrayPanel?.("h1"), { success: false });
  assert.deepEqual(await bare.closeSessionFromTrayPanel?.("s1"), { success: false });
  assert.deepEqual(await bare.openMainWindow?.(), { success: false });
  assert.deepEqual(await bare.hideTrayPanel?.(), { success: false });
  assert.equal(await bare.notifyTrayPanelPaintReady?.(), false);

  assert.deepEqual(trayCalls.map((call) => call.method), [
    "UpdateTrayMenuData",
    "JumpToSessionFromPanel",
    "ConnectToHost",
    "CloseSessionFromPanel",
    "OpenMainWindow",
    "trayPanel.Hide",
    "trayPanel.PaintReady",
  ]);
});

test("tray events subscribe to the Go tray:* broadcasts and normalize payloads", async () => {
  const bindings = stubBindings() as WailsBindingDeps;
  const subscribers: Array<{ name: string; cb: (event: { data?: unknown }) => void }> = [];
  bindings.events = {
    On: (name, callback) => {
      subscribers.push({ name, cb: callback });
      return () => undefined;
    },
  };
  const bridge = createWailsRuntimeClient(bindings).transitionBridge;

  const focused: string[] = [];
  bridge.onTrayFocusSession?.((sessionId) => focused.push(sessionId));
  const toggles: Array<{ ruleId: string; start: boolean }> = [];
  bridge.onTrayTogglePortForward?.((ruleId, start) => toggles.push({ ruleId, start }));
  const panelJumps: string[] = [];
  bridge.onTrayPanelJumpToSession?.((sessionId) => panelJumps.push(sessionId));
  const panelConnects: string[] = [];
  bridge.onTrayPanelConnectToHost?.((hostId) => panelConnects.push(hostId));
  const panelCloses: string[] = [];
  bridge.onTrayPanelCloseSession?.((sessionId) => panelCloses.push(sessionId));
  let menuData: unknown;
  bridge.onTrayPanelMenuData?.((data) => { menuData = data; });
  let refreshed = false;
  let closeRequested = false;
  bridge.onTrayPanelRefresh?.(() => { refreshed = true; });
  bridge.onTrayPanelCloseRequest?.(() => { closeRequested = true; });

  assert.deepEqual(subscribers.map((subscriber) => subscriber.name), [
    "tray:focus-session",
    "tray:toggle-port-forward",
    "tray:panel:jump-to-session",
    "tray:panel:connect-to-host",
    "tray:panel:close-session",
    "tray:panel:menu-data",
    "tray:panel:refresh",
    "tray:panel:close-request",
  ]);

  subscribers[0].cb({ data: "s1" });
  subscribers[1].cb({ data: { ruleId: "r1", start: true } });
  subscribers[2].cb({ data: "s2" });
  subscribers[3].cb({ data: "h1" });
  subscribers[4].cb({ data: "s3" });
  subscribers[5].cb({
    data: {
      sessions: [{ id: "s1", hostLabel: "AI Box", status: "connected" }],
      hosts: [{ id: "h1" }],
      portForwardRules: [{ id: "r1", type: "dynamic", localPort: 1080, status: "active" }],
    },
  });
  subscribers[6].cb({});
  subscribers[7].cb({});

  assert.deepEqual(focused, ["s1"]);
  assert.deepEqual(toggles, [{ ruleId: "r1", start: true }]);
  assert.deepEqual(panelJumps, ["s2"]);
  assert.deepEqual(panelConnects, ["h1"]);
  assert.deepEqual(panelCloses, ["s3"]);
  assert.deepEqual(menuData, {
    sessions: [{ id: "s1", label: "", hostLabel: "AI Box", status: "connected", workspaceId: undefined, workspaceTitle: undefined }],
    hosts: [{ id: "h1", label: undefined, hostname: undefined, group: undefined, pinned: false, lastConnectedAt: undefined, protocol: undefined }],
    portForwardRules: [{ id: "r1", label: undefined, type: "dynamic", localPort: 1080, remoteHost: undefined, remotePort: undefined, status: "active" }],
  });
  assert.equal(refreshed, true);
  assert.equal(closeRequested, true);
});
