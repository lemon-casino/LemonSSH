// Wails RuntimeClient adapter (P1-02). This module is the only frontend file
// allowed to import the Wails runtime and the generated bindings (enforced by
// ESLint). Ports without a Go owner reject every call fail-closed instead of
// pretending parity; they are implemented domain by domain from P2 onward.

import { Dialogs, Events, Window as wailsWindow } from "@wailsio/runtime";
import * as netcattyService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/netcattyservice";
import * as terminalService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/terminalservice";
import * as sftpService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/sftpservice";
import * as settingsWindowService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/settingswindowservice";
import * as forwardService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/forwardservice";
import * as appLockService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/applockservice";
import * as pluginService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/pluginservice";
import * as deepLinkService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/deeplinkservice";
import * as filesystemService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/filesystemservice";
import * as transferService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/transferservice";
import * as popupWindowService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/popupwindowservice";
import {
  buildTerminalSocketUrl,
  bytesToBase64,
  entryToRemoteFile,
  pickSSHConnectArgs,
  statToSftpStatResult,
  terminalSocketSubprotocols,
} from "./terminalRoute";
import type {
  WailsRouteBootstrap,
  WailsSftpEntry,
  WailsSftpFileInfo,
} from "./terminalRoute";
import { setActiveRuntimeClient } from "../runtimeClient";
import type { RuntimeClient } from "../runtimeClient";
import type { RemoteFile } from "../../../domain/models/workspace";
import { openDataPlaneSession } from "./dataPlaneSession";
import type { DataPlaneSessionHandle } from "./dataPlaneSession";

export function isWailsRuntime(): boolean {
  return typeof window !== "undefined" && "_wails" in window;
}

/**
 * Builds a port whose implemented methods are backed by generated bindings
 * and whose remaining methods reject with an explicit migration error. The
 * rejection is the honest state during the migration: the Electron bridge
 * still owns those capabilities.
 */
function portWith<T extends object>(portName: string, implemented: object): T {
  return new Proxy(implemented, {
    get(target, property, receiver) {
      if (property in target) return Reflect.get(target, property, receiver);
      if (property === "__wails__") return undefined;
      throw new Error(
        `Netcatty.${portName}.${String(property)} is not migrated to the Wails runtime yet`,
      );
    },
  }) as T;
}

function missingBridgeMethod(property: string | symbol): never {
  throw new Error(
    `Netcatty bridge method ${String(property)} is not available under the Wails runtime yet`,
  );
}

export interface WailsBindingDeps {
  terminal: {
    Connect: (request: unknown) => Promise<string>;
    RespondKeyboardInteractive?: (requestID: string, responses: string[], cancelled: boolean) => Promise<unknown>;
    StartLocal?: (shell: string, cwd: string, cols: number, rows: number) => Promise<string>;
    StartTelnet?: (host: string, port: number, cols: number, rows: number) => Promise<string>;
    StartSerial?: (path: string, baudRate: number) => Promise<string>;
    ListSerialPorts?: () => Promise<Array<{ name: string }>>;
    Write: (...args: unknown[]) => unknown;
    Resize: (...args: unknown[]) => unknown;
    Signal: (...args: unknown[]) => unknown;
    Close: (sessionID: string) => Promise<unknown>;
    Bootstrap: (sessionID: string) => Promise<WailsRouteBootstrap>;
    ListenAddr: () => Promise<string> | string;
  };
  sftp: {
    Open: (request: unknown) => Promise<string>;
    Download?: (sftpID: string, remotePath: string, localPath: string) => Promise<number>;
    Upload?: (sftpID: string, localPath: string, remotePath: string) => Promise<number>;
    List: (sftpID: string, path: string) => Promise<WailsSftpEntry[]>;
    Mkdir: (sftpID: string, path: string) => Promise<unknown>;
    Remove: (sftpID: string, path: string) => Promise<unknown>;
    Rename: (sftpID: string, oldPath: string, newPath: string) => Promise<unknown>;
    Stat: (sftpID: string, path: string) => Promise<WailsSftpFileInfo>;
    Close: (sftpID: string) => Promise<unknown>;
    Read?: (sftpID: string, path: string) => Promise<string>;
    WriteText?: (sftpID: string, path: string, content: string) => Promise<unknown>;
    HomeDir?: (sftpID: string) => Promise<string>;
  };
  window?: {
    Minimise: () => Promise<void>;
    ToggleMaximise: () => Promise<void>;
    Close: () => Promise<void>;
    IsMaximised: () => Promise<boolean>;
    IsFullscreen: () => Promise<boolean>;
  };
  settings?: {
    Open: () => Promise<boolean>;
    Show?: () => Promise<unknown>;
    PaintReady?: () => Promise<boolean>;
    Close: () => Promise<unknown>;
  };
  forward?: {
    Start: (id: string, kind: string, bindHost: string, bindPort: number, targetHost: string, targetPort: number, request: unknown) => Promise<unknown>;
    Stop: (id: string) => Promise<unknown>;
    List: () => Promise<unknown>;
    Snapshot: (id: string) => Promise<unknown>;
  };
  dialogs?: {
    OpenFile: (options: Record<string, unknown>) => Promise<string | string[]>;
    SaveFile: (options: Record<string, unknown>) => Promise<string>;
  };
  appLock?: {
    GetRuntimeState: () => Promise<{
      initialized: boolean;
      locked: boolean;
      reason: string | null;
      version: number;
      lastLockedAt: number | null;
      lastUnlockedAt: number | null;
      lastActivityAt: number | null;
    }>;
    ReportActivity?: () => Promise<unknown>;
    Enable?: (password: string) => Promise<unknown>;
    Unlock?: (password: string) => Promise<unknown>;
    Disable?: (password: string) => Promise<unknown>;
    SetRuntimeLocked?: (reason: string) => Promise<unknown>;
  };
  plugins?: {
    List: () => Promise<unknown[]>;
    Install?: (pluginID: string, version: string, sha256Hex: string, manifestJSON: string) => Promise<unknown>;
    SetEnabled?: (pluginID: string, enabled: boolean) => Promise<unknown>;
  };
  deepLink?: {
    Parse?: (rawURL: string) => Promise<unknown>;
    Enqueue?: (rawURL: string) => Promise<unknown>;
    Pending?: () => Promise<number>;
    Ready?: () => Promise<unknown>;
    Drain?: () => Promise<unknown[]>;
  };
  filesystem?: {
    ExtractArchive?: (archivePath: string, destinationRoot: string) => Promise<number>;
  };
  transfer?: {
    Enqueue?: (spec: unknown) => Promise<unknown>;
    Pause?: (taskID: string) => Promise<unknown>;
    Resume?: (taskID: string) => Promise<unknown>;
    Cancel?: (taskID: string) => Promise<unknown>;
  };
  events?: {
    On: (name: string, callback: (event: { data?: unknown }) => void) => () => void;
  };
  popup?: {
    Open: (payload: unknown) => Promise<{ success: boolean; popupId?: string; error?: string }>;
  };
  openDataPlane?: typeof openDataPlaneSession;
}

  const defaultBindings: WailsBindingDeps = {
    terminal: terminalService as unknown as WailsBindingDeps["terminal"],
    sftp: sftpService as unknown as WailsBindingDeps["sftp"],
    window: wailsWindow,
    settings: settingsWindowService as unknown as WailsBindingDeps["settings"],
    forward: forwardService as unknown as WailsBindingDeps["forward"],
    dialogs: Dialogs as unknown as WailsBindingDeps["dialogs"],
    appLock: appLockService as unknown as WailsBindingDeps["appLock"],
    plugins: pluginService as unknown as WailsBindingDeps["plugins"],
    events: Events as unknown as WailsBindingDeps["events"],
    deepLink: deepLinkService as unknown as WailsBindingDeps["deepLink"],
    filesystem: filesystemService as unknown as WailsBindingDeps["filesystem"],
    transfer: transferService as unknown as WailsBindingDeps["transfer"],
    popup: popupWindowService as unknown as WailsBindingDeps["popup"],
  };

type SessionDataCallback = Parameters<NetcattyBridge["onSessionData"]>[1];
type SessionExitCallback = (evt: { sessionId: string; code?: number }) => void;

export function createWailsRuntimeClient(bindings: WailsBindingDeps = defaultBindings): RuntimeClient {
  const unimplemented = <T extends object>(portName: string): T => portWith<T>(portName, {});
  const dataListeners = new Map<string, Set<SessionDataCallback>>();
  const exitListeners = new Map<string, Set<SessionExitCallback>>();
  const planes = new Map<string, DataPlaneSessionHandle>();

  function emitData(sessionID: string, chunk: string): void {
    for (const listener of dataListeners.get(sessionID) ?? []) listener(chunk);
  }

  function emitExit(sessionID: string): void {
    for (const listener of exitListeners.get(sessionID) ?? []) listener({ sessionId: sessionID });
  }

  async function attachDataPlane(sessionID: string): Promise<void> {
    planes.get(sessionID)?.dispose();
    const [bootstrap, listenAddr] = await Promise.all([
      bindings.terminal.Bootstrap(sessionID),
      Promise.resolve(bindings.terminal.ListenAddr()),
    ]);
    planes.set(sessionID, (bindings.openDataPlane ?? openDataPlaneSession)({
      listenAddr,
      bootstrap,
      onData: (chunk) => emitData(sessionID, chunk),
      onComplete: () => emitExit(sessionID),
    }));
  }

  const startSSHSession = (options: Parameters<NetcattyBridge["startSSHSession"]>[0]) => {
    const args = pickSSHConnectArgs(options);
    return bindings.terminal.Connect(args).then(async (sessionID) => {
      await attachDataPlane(sessionID);
      return sessionID;
    });
  };
  const startLocalSession = async (options: {
    shell?: string;
    cwd?: string;
    cols?: number;
    rows?: number;
  } = {}) => {
    if (!bindings.terminal.StartLocal) missingBridgeMethod("startLocalSession");
    const sessionID = await bindings.terminal.StartLocal(
      options.shell ?? "",
      options.cwd ?? "",
      options.cols ?? 80,
      options.rows ?? 24,
    );
    await attachDataPlane(sessionID);
    return sessionID;
  };
  const startTelnetSession = async (options: {
    hostname: string;
    port?: number;
    cols?: number;
    rows?: number;
  }) => {
    if (!bindings.terminal.StartTelnet) missingBridgeMethod("startTelnetSession");
    const sessionID = await bindings.terminal.StartTelnet(
      options.hostname,
      options.port ?? 23,
      options.cols ?? 80,
      options.rows ?? 24,
    );
    await attachDataPlane(sessionID);
    return sessionID;
  };
  const startSerialSession = async (options: { path: string; baudRate?: number }) => {
    if (!bindings.terminal.StartSerial) missingBridgeMethod("startSerialSession");
    const sessionID = await bindings.terminal.StartSerial(options.path, options.baudRate ?? 115200);
    await attachDataPlane(sessionID);
    return sessionID;
  };
  const listSerialPorts = async () => {
    const ports = await bindings.terminal.ListSerialPorts?.() ?? [];
    return ports.map((port) => ({
      path: port.name,
      manufacturer: "",
      serialNumber: "",
      vendorId: "",
      productId: "",
      pnpId: "",
    }));
  };
  const writeToSession = (sessionID: string, data: string) =>
    bindings.terminal.Write(sessionID, bytesToBase64(new TextEncoder().encode(data))) as unknown as void;
  const resizeSession = (sessionID: string, cols: number, rows: number) =>
    bindings.terminal.Resize(sessionID, cols, rows) as unknown as void;
  const interruptSession = (sessionID: string) =>
    bindings.terminal.Signal(sessionID, "INT") as unknown as void;
  const closeSession = async (sessionID: string) => {
    planes.get(sessionID)?.dispose();
    planes.delete(sessionID);
    await bindings.terminal.Close(sessionID);
  };
  const onSessionData = (
    sessionID: string,
    cb: SessionDataCallback,
  ) => {
    let set = dataListeners.get(sessionID);
    if (!set) {
      set = new Set();
      dataListeners.set(sessionID, set);
    }
    set.add(cb);
    return () => {
      set.delete(cb);
      if (set.size === 0) dataListeners.delete(sessionID);
    };
  };
  const onSessionExit = (sessionID: string, cb: SessionExitCallback) => {
    let set = exitListeners.get(sessionID);
    if (!set) {
      set = new Set();
      exitListeners.set(sessionID, set);
    }
    set.add(cb);
    return () => {
      set.delete(cb);
      if (set.size === 0) exitListeners.delete(sessionID);
    };
  };

  const openSftp = (options: Parameters<NetcattyBridge["openSftp"]>[0]) => {
    const args = pickSSHConnectArgs(options);
    return bindings.sftp.Open(args);
  };
  const downloadSftp = (sftpID: string, remotePath: string, localPath: string) =>
    bindings.sftp.Download?.(sftpID, remotePath, localPath) as Promise<number>;
  const uploadSftp = (sftpID: string, localPath: string, remotePath: string) =>
    bindings.sftp.Upload?.(sftpID, localPath, remotePath) as Promise<number>;
  const listSftp = async (sftpID: string, path: string): Promise<RemoteFile[]> => {
    const entries = await bindings.sftp.List(sftpID, path);
    return entries.map(entryToRemoteFile);
  };
  const mkdirSftp = (sftpID: string, path: string) =>
    bindings.sftp.Mkdir(sftpID, path) as Promise<void>;
  const deleteSftp = (sftpID: string, path: string) =>
    bindings.sftp.Remove(sftpID, path) as Promise<void>;
  const renameSftp = (sftpID: string, oldPath: string, newPath: string) =>
    bindings.sftp.Rename(sftpID, oldPath, newPath) as Promise<void>;
  const statSftp = async (sftpID: string, path: string) => {
    const stat = await bindings.sftp.Stat(sftpID, path);
    return statToSftpStatResult(stat);
  };
  const closeSftp = (sftpID: string) =>
    bindings.sftp.Close(sftpID) as Promise<void>;
  const readSftp = (sftpID: string, path: string) => bindings.sftp.Read?.(sftpID, path) as Promise<string>;
  const writeSftp = (sftpID: string, path: string, content: string) => bindings.sftp.WriteText?.(sftpID, path, content) as Promise<void>;
  const getSftpHomeDir = (sftpID: string) => bindings.sftp.HomeDir?.(sftpID) as Promise<string>;

  const windowMinimize = () => bindings.window?.Minimise();
  const windowMaximize = async () => {
    await bindings.window?.ToggleMaximise();
    return bindings.window?.IsMaximised() ?? false;
  };
  const windowClose = () => bindings.window?.Close();
  const windowIsMaximized = () => bindings.window?.IsMaximised() ?? Promise.resolve(false);
  const windowIsFullscreen = () => bindings.window?.IsFullscreen() ?? Promise.resolve(false);
  const openSettingsWindow = () => bindings.settings?.Open() ?? Promise.resolve(false);
  const showSettingsWindow = () => bindings.settings?.Show?.();
  const notifySettingsPainted = () => bindings.settings?.PaintReady?.();
  const closeSettingsWindow = () => bindings.settings?.Close();
  const selectFile = async () => {
    const selected = await bindings.dialogs?.OpenFile({ CanChooseFiles: true, CanChooseDirectories: false });
    return typeof selected === "string" ? selected : selected?.[0] ?? "";
  };
  const selectDirectory = async () => {
    const selected = await bindings.dialogs?.OpenFile({ CanChooseFiles: false, CanChooseDirectories: true });
    return typeof selected === "string" ? selected : selected?.[0] ?? "";
  };
  const showSaveDialog = async () => bindings.dialogs?.SaveFile({}) ?? "";
  const startPortForward = (options: {
    tunnelId: string;
    type: string;
    bindAddress?: string;
    localPort: number;
    remoteHost?: string;
    remotePort?: number;
    hostname: string;
    port?: number;
    username: string;
    password?: string;
    privateKey?: string;
    passphrase?: string;
    requiresMfa?: boolean;
    jumpHosts?: unknown[];
    proxy?: unknown;
  }) => {
    const request = pickSSHConnectArgs(options as Parameters<NetcattyBridge["startSSHSession"]>[0]);
    return bindings.forward?.Start(
      options.tunnelId,
      options.type,
      options.bindAddress ?? "127.0.0.1",
      options.localPort,
      options.remoteHost ?? "",
      options.remotePort ?? 0,
      request,
    );
  };
  const stopPortForward = (id: string) => bindings.forward?.Stop(id);
  const listPortForwards = () => bindings.forward?.List();
  const getPortForwardSnapshot = (id: string) => bindings.forward?.Snapshot(id);
  const getAppLockRuntimeState = () => bindings.appLock?.GetRuntimeState();
  const reportAppLockActivity = () => bindings.appLock?.ReportActivity?.();
  const listPlugins = () => bindings.plugins?.List() ?? Promise.resolve([]);
  type KeyboardInteractiveCallback = Parameters<NonNullable<NetcattyBridge["onKeyboardInteractive"]>>[0];
  const keyboardListeners = new Set<KeyboardInteractiveCallback>();
  let keyboardSubscribed = false;
  const subscribeKeyboardEvents = () => {
    if (keyboardSubscribed) return;
    const eventsOn = bindings.events?.On ?? Events.On;
    if (typeof eventsOn !== "function") return;
    keyboardSubscribed = true;
    eventsOn("ssh:keyboard-interactive", (event) => {
      const challenge = (event?.data ?? event) as {
        requestId?: string;
        hostname?: string;
        name?: string;
        instructions?: string;
        prompts?: Array<{ prompt: string; echo: boolean }>;
      };
      const request = {
        requestId: challenge.requestId ?? "",
        sessionId: "",
        name: challenge.name ?? "",
        instructions: challenge.instructions ?? "",
        prompts: challenge.prompts ?? [],
        hostname: challenge.hostname ?? "",
        scope: "external" as const,
      };
      for (const listener of keyboardListeners) listener(request);
    });
  };
  const onKeyboardInteractive = (cb: KeyboardInteractiveCallback) => {
    subscribeKeyboardEvents();
    keyboardListeners.add(cb);
    return () => {
      keyboardListeners.delete(cb);
    };
  };
  const respondKeyboardInteractive = async (requestId: string, responses: string[], cancelled?: boolean) => {
    try {
      await bindings.terminal.RespondKeyboardInteractive?.(requestId, responses, Boolean(cancelled));
      return { success: true };
    } catch (error) {
      return { success: false, error: error instanceof Error ? error.message : String(error) };
    }
  };

  const implementedBridge: Partial<NetcattyBridge> = {
    startSSHSession,
    startLocalSession,
    startTelnetSession,
    startSerialSession,
    listSerialPorts,
    writeToSession,
    resizeSession,
    interruptSession,
    closeSession,
    onSessionData: onSessionData as NetcattyBridge["onSessionData"],
    onSessionExit: onSessionExit as NetcattyBridge["onSessionExit"],
    openSftp,
    listSftp,
    mkdirSftp,
    deleteSftp,
    renameSftp,
    statSftp,
    closeSftp,
    readSftp,
    writeSftp,
    // The Go bindings return Wails-native shapes; cast until the shared
    // port types gain Wails-specific variants.
    getSftpHomeDir: ((sftpID: string) =>
      bindings.sftp.HomeDir?.(sftpID).then((homeDir) => ({ success: true, homeDir }))) as unknown as NetcattyBridge["getSftpHomeDir"],
    startStreamTransfer: (async (options: {
      sourceType: "local" | "sftp";
      targetType: "local" | "sftp";
      sourcePath: string;
      targetPath: string;
      sourceSftpId?: string;
      targetSftpId?: string;
      transferId: string;
    }) => {
      if (options.sourceType === "sftp" && options.targetType === "local" && options.sourceSftpId) {
        await downloadSftp(options.sourceSftpId, options.sourcePath, options.targetPath);
        return { transferId: options.transferId };
      }
      if (options.sourceType === "local" && options.targetType === "sftp" && options.targetSftpId) {
        await uploadSftp(options.targetSftpId, options.sourcePath, options.targetPath);
        return { transferId: options.transferId };
      }
      return { transferId: options.transferId, error: "stream transfer shape not migrated yet" };
    }) as unknown as NetcattyBridge["startStreamTransfer"],
    extractLocalArchive: (async (archivePath: string) => {
      if (!bindings.filesystem?.ExtractArchive) return { success: false };
      const parent = archivePath.replace(/[\\/][^\\/]+$/, "") || ".";
      try {
        await bindings.filesystem.ExtractArchive(archivePath, parent);
        return { success: true };
      } catch {
        return { success: false };
      }
    }) as unknown as NetcattyBridge["extractLocalArchive"],
    drainDeepLinks: (async () => {
      await bindings.deepLink?.Ready?.();
      return bindings.deepLink?.Drain?.() ?? [];
    }) as unknown as NetcattyBridge["drainDeepLinks"],
    onSshDeepLink: ((cb: (payload: { url?: string }) => void) => {
      const eventsOn = bindings.events?.On ?? Events.On;
      if (typeof eventsOn !== "function") return () => undefined;
      return eventsOn("deeplink:ssh", (event) => {
        const data = (event?.data ?? event) as { url?: string; Kind?: string; Host?: string; Port?: string; Username?: string };
        if (data.url) {
          cb({ url: data.url });
          return;
        }
        if (!data.Host) return;
        const user = data.Username ? `${data.Username}@` : "";
        const port = data.Port ? `:${data.Port}` : "";
        cb({ url: `ssh://${user}${data.Host}${port}` });
      });
    }) as unknown as NetcattyBridge["onSshDeepLink"],
    openTerminalPopup: (async (payload) => {
      if (!bindings.popup?.Open) return { success: false, error: "openTerminalPopup unavailable" };
      return bindings.popup.Open(payload);
    }) as unknown as NetcattyBridge["openTerminalPopup"],
    onTerminalPopupConfig: ((cb) => {
      const eventsOn = bindings.events?.On ?? Events.On;
      if (typeof eventsOn !== "function") return () => undefined;
      return eventsOn("terminal:popup-config", (event) => {
        cb((event?.data ?? event) as import("../../../domain/systemManager/types").TerminalPopupPayload);
      });
    }) as unknown as NetcattyBridge["onTerminalPopupConfig"],
    onKeyboardInteractive,
    respondKeyboardInteractive,
    selectFile,
    selectDirectory,
    showSaveDialog,
    startPortForward: startPortForward as unknown as NetcattyBridge["startPortForward"],
    stopPortForward: ((id: string) =>
      bindings.forward?.Stop(id)) as unknown as NetcattyBridge["stopPortForward"],
    listPortForwards: (() =>
      Promise.resolve(bindings.forward?.List() ?? [])) as unknown as NetcattyBridge["listPortForwards"],
    getPortForwardSnapshot: ((id: string) =>
      bindings.forward?.Snapshot(id)) as unknown as NetcattyBridge["getPortForwardSnapshot"],
    windowMinimize,
    windowMaximize,
    windowClose,
    windowIsMaximized,
    windowIsFullscreen,
    openSettingsWindow,
    notifySettingsPainted,
    closeSettingsWindow: closeSettingsWindow as unknown as NetcattyBridge["closeSettingsWindow"],
    getAppLockRuntimeState: (() =>
      bindings.appLock?.GetRuntimeState()) as unknown as NetcattyBridge["getAppLockRuntimeState"],
    reportAppLockActivity: (() =>
      bindings.appLock?.ReportActivity?.()) as unknown as NetcattyBridge["reportAppLockActivity"],
    listPlugins: (() =>
      Promise.resolve(bindings.plugins?.List() ?? [])) as unknown as NetcattyBridge["listPlugins"],
    requestAppLockPasswordChange: (async (input: { currentPassword?: string; nextPassword: string }) => {
      if (!input.nextPassword) return { ok: false, error: "empty-next" };
      if (input.currentPassword) {
        try {
          await bindings.appLock?.Disable?.(input.currentPassword);
        } catch {
          return { ok: false, error: "incorrect" };
        }
      }
      try {
        await bindings.appLock?.Enable?.(input.nextPassword);
        return { enabled: true, timeoutMinutes: 15, systemUnlockEnabled: false, systemUnlockAutoPromptEnabled: false, passwordVerifier: { version: 1, algorithm: "PBKDF2-SHA256", iterations: 210000, salt: "", hash: "" } };
      } catch {
        return { ok: false, error: "incorrect" };
      }
    }) as unknown as NetcattyBridge["requestAppLockPasswordChange"],
    requestAppLockUnlock: (async (password: string) => {
      if (!password) return { ok: false, error: "empty" };
      try {
        await bindings.appLock?.Unlock?.(password);
        return { ok: true };
      } catch {
        return { ok: false, error: "incorrect" };
      }
    }) as unknown as NetcattyBridge["requestAppLockUnlock"],
    requestAppLockDisable: (async (currentPassword: string) => {
      try {
        await bindings.appLock?.Disable?.(currentPassword);
        return { enabled: false, timeoutMinutes: 15, systemUnlockEnabled: false, systemUnlockAutoPromptEnabled: false, passwordVerifier: null };
      } catch {
        return { ok: false, error: "incorrect" };
      }
    }) as unknown as NetcattyBridge["requestAppLockDisable"],
    pauseTransfer: ((transferId: string) =>
      bindings.transfer?.Pause?.(transferId)) as unknown as NetcattyBridge["pauseTransfer"],
    resumeTransfer: ((transferId: string) =>
      bindings.transfer?.Resume?.(transferId)) as unknown as NetcattyBridge["resumeTransfer"],
    cancelTransfer: ((transferId: string) =>
      bindings.transfer?.Cancel?.(transferId)) as unknown as NetcattyBridge["cancelTransfer"],
    cancelZmodem: (async () => ({ success: false, error: "zmodem session engine is not wired yet" })) as unknown as NetcattyBridge["cancelZmodem"],
    startMoshSession: (async () => {
      throw new Error("mosh reconnect protocol is not wired on the Wails data plane yet");
    }) as unknown as NetcattyBridge["startMoshSession"],
    startEtSession: (async () => {
      throw new Error("eternal terminal reconnect protocol is not wired on the Wails data plane yet");
    }) as unknown as NetcattyBridge["startEtSession"],
  };
  const transitionBridge = new Proxy(implementedBridge, {
    get(target, property, receiver) {
      if (property in target) return Reflect.get(target, property, receiver);
      // Optional-chain callers (bridge?.setLanguage?.()) must see undefined,
      // not a throwing function. A throwing stub crashes first paint.
      return undefined;
    },
  }) as NetcattyBridge;

  return {
    app: portWith("app", {
      quitApp: () => {
        throw new Error("quitApp is not migrated to the Wails runtime yet");
      },
    }),
    agent: unimplemented("agent"),
    files: unimplemented("files"),
    script: unimplemented("script"),
    terminal: portWith("terminal", {
      startSSHSession,
      startLocalSession,
      startTelnetSession,
      startSerialSession,
      listSerialPorts,
      writeToSession,
      resizeSession,
      interruptSession,
      closeSession,
      onSessionData,
      onSessionExit,
    }),
    sftp: portWith("sftp", {
      openSftp,
      listSftp,
      mkdirSftp,
      deleteSftp,
      renameSftp,
      statSftp,
      closeSftp,
      readSftp,
      writeSftp,
      getSftpHomeDir,
    }),
    sync: unimplemented("sync"),
    system: unimplemented("system"),
    plugin: unimplemented("plugin"),
    transitionBridge,
  };
}

/**
 * Wails-native terminal surface for the renderer: the Electron ports cannot
 * express the loopback data plane, so the Wails-terminal renderer consumes
 * this typed surface directly (WS URLs + route bootstrap + streaming SFTP).
 */
export function goTerminalSurface() {
  const listenAddr = () => terminalService.ListenAddr();
  return {
    listenAddr,
    bootstrap: async (sessionID: string) => {
      const bootstrap = (await terminalService.Bootstrap(sessionID)) as unknown as WailsRouteBootstrap;
      const address = await listenAddr();
      return {
        ...bootstrap,
        dataSocketUrl: buildTerminalSocketUrl(address, bootstrap.SessionID, bootstrap.Generation, "data"),
        urgentSocketUrl: buildTerminalSocketUrl(address, bootstrap.SessionID, bootstrap.Generation, "urgent"),
        dataSubprotocols: terminalSocketSubprotocols(bootstrap.DataToken),
        urgentSubprotocols: terminalSocketSubprotocols(bootstrap.UrgentToken),
      };
    },
    write: (sessionID: string, bytes: Uint8Array) => terminalService.Write(sessionID, bytesToBase64(bytes)),
    signal: (sessionID: string, signal: string) => terminalService.Signal(sessionID, signal),
    close: (sessionID: string) => terminalService.Close(sessionID),
    sftp: {
      download: (sftpID: string, remotePath: string, localPath: string) =>
        sftpService.Download(sftpID, remotePath, localPath),
      upload: (sftpID: string, localPath: string, remotePath: string) =>
        sftpService.Upload(sftpID, localPath, remotePath),
    },
  };
}

export { netcattyService };

export function installWailsRuntimeClient(): boolean {
  if (!isWailsRuntime()) return false;
  setActiveRuntimeClient(createWailsRuntimeClient());
  return true;
}
