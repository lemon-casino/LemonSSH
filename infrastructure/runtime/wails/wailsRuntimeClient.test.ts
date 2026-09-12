import assert from "node:assert/strict";
import { test } from "node:test";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";

import { useSftpDirectoryListing } from "../../../application/state/sftp/useSftpDirectoryListing";
import { getActiveRuntimeClient, setActiveRuntimeClient } from "../runtimeClient";
import { createElectronRuntimeClient } from "../electron/electronRuntimeClient";
import { useSftpExternalOperations } from "../../../application/state/sftp/useSftpExternalOperations";
import { sftpTransferCenterStore } from "../../../application/state/sftpTransferCenterStore";
import type { SftpPane } from "../../../application/state/sftp/types";
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
  const image = { path: "C:\\Netcatty\\temp\\shot.png", name: "shot.png", mediaType: "image/png", size: 123 };
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

    // An installed but incomplete desktop bridge must never enable preview data.
    setActiveRuntimeClient(createElectronRuntimeClient({} as NetcattyBridge));
    await assert.rejects(listing.getLocalHomeDir(), /getHomeDir unavailable/);
    await assert.rejects(listing.listLocalFiles(home), /listLocalDir unavailable/);
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
  listeners.get("netcatty:files-dropped")?.[0]({
    data: { filenames: ["C:\\a.txt"], x: 1, y: 2, elementDetails: { id: "pane" } },
  });
  listeners.get("netcatty:files-dropped")?.[0]({
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

test("drainDeepLinks Ready then Drain", async () => {
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
  assert.deepEqual(calls, ["ready", "drain"]);
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

test("cloud OAuth methods surface on the sync port and transition bridge", async () => {
  const calls: string[] = [];
  const bindings = stubBindings();
  bindings.sync = {
    GithubStartDeviceFlow: async (options: { clientId?: string; scope?: string }) => {
      calls.push("device-flow");
      assert.equal(options.clientId, "cid");
      return { deviceCode: "dc", userCode: "uc", verificationUri: "https://github.com/login/device", expiresAt: 1 };
    },
    GoogleGetUserInfo: async (options: { accessToken: string }) => ({ email: "me@example.com" }),
  } as never;
  const client = createWailsRuntimeClient(bindings);
  assert.equal(typeof client.sync.githubStartDeviceFlow, "function", "sync port must expose the camelCase facade");
  const device = await client.transitionBridge.githubStartDeviceFlow!({ clientId: "cid" });
  assert.equal(device.userCode, "uc");
  assert.equal(typeof client.transitionBridge.googleGetUserInfo, "function");
  assert.deepEqual(calls, ["device-flow"]);
});
