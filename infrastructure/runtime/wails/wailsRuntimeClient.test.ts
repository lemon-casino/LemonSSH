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
    bindings.sftp.Upload = async (_sessionId, path) => { uploads.push(path); return 7; };
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

test("optional-chain startup calls do not throw on missing bridge methods", () => {
  const client = createWailsRuntimeClient(stubBindings());
  const bridge = client.transitionBridge;
  assert.doesNotThrow(() => {
    bridge.setLanguage?.("en");
    void bridge.getAppLockSettings?.();
    void bridge.rendererReady?.();
    void bridge.notifySettingsChanged?.({ key: "x", value: "y" });
  });
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
  bindings.sftp.Upload = async (id, source, target) => {
    assert.equal(id, "original-sftp");
    uploaded.push(`${source} -> ${target}`);
    return 3;
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
  try {
    setActiveRuntimeClient(createWailsRuntimeClient(bindings));
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

test("startCompressedUpload calls UploadCompressedFolder", async () => {
  const seen: string[] = [];
  const bindings = stubBindings();
  bindings.sftp.UploadCompressedFolder = async (sftpID, localFolder, remoteZipPath) => {
    seen.push(sftpID, localFolder, remoteZipPath);
    return 12;
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
