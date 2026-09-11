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
import * as shortcutService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/shortcutservice";
import * as diagnosticLogService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/diagnosticlogservice";
import * as trayService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/trayservice";
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
import { readLocalTree } from "./localTree";

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
    StartTelnet?: (request: unknown) => Promise<string>;
    StartSerial?: (request: unknown) => Promise<string>;
    StartMosh?: (request: unknown) => Promise<string>;
    StartEt?: (request: unknown) => Promise<string>;
    ListSerialPorts?: () => Promise<Array<{
      name: string;
      manufacturer?: string;
      serialNumber?: string;
      vendorId?: string;
      productId?: string;
      pnpId?: string;
    }>>;
    CancelZmodem?: (sessionID: string) => Promise<unknown>;
    SendSerialYmodem?: (sessionID: string, filePath: string) => Promise<{
      fileName: string;
      totalBytes: number;
      writtenBytes: number;
    }>;
    ReceiveSerialYmodem?: (sessionID: string, destinationDir: string) => Promise<Array<{
      fileName: string;
      filePath: string;
      totalBytes: number;
      writtenBytes: number;
    }>>;
    GetTelnetEchoMode?: (sessionID: string) => Promise<{
      success: boolean;
      sessionId?: string;
      remoteEcho?: boolean;
      localEcho?: boolean;
    }>;
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
    ExtractArchive?: (sftpID: string, remotePath: string) => Promise<number>;
    UploadCompressedFolder?: (sftpID: string, localFolder: string, remoteZipPath: string) => Promise<number>;
  };
  window?: {
    Minimise: () => Promise<void>;
    ToggleMaximise: () => Promise<void>;
    Hide?: () => Promise<void>;
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
    GetOSProtocolStatus?: () => Promise<{ success: boolean; registered: boolean; error?: string }>;
    SetOSProtocol?: (enabled: boolean) => Promise<{ success: boolean; registered: boolean; error?: string }>;
  };
  filesystem?: {
    HomeDir?: () => Promise<string>;
    ListDir?: (path: string) => Promise<RemoteFile[]>;
    ExtractArchive?: (archivePath: string, destinationRoot: string) => Promise<number>;
    StatPath?: (path: string) => Promise<{ name: string; isDir: boolean; size: number }>;
    StageFromLocalPath?: (path: string) => Promise<{ stagedPath: string; name: string; size: number }>;
    StageBegin?: (fileName: string) => Promise<string>;
    StageAppend?: (tempPath: string, offset: number, data: string) => Promise<unknown>;
    StageDiscard?: (tempPath: string) => Promise<unknown>;
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
  shortcuts?: {
    Register?: (raw: string) => Promise<{ success: boolean; enabled?: boolean; error?: string; accelerator?: string }>;
    Unregister?: () => Promise<{ success: boolean }>;
    Status?: () => Promise<{ enabled: boolean; hotkey: string | null }>;
  };
  diagnosticLog?: {
    Append?: (line: string) => Promise<unknown>;
  };
  tray?: {
    SetLanguage?: (language: string) => Promise<boolean>;
    Quit?: () => Promise<void>;
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
    shortcuts: shortcutService as unknown as WailsBindingDeps["shortcuts"],
    diagnosticLog: diagnosticLogService as unknown as WailsBindingDeps["diagnosticLog"],
    tray: trayService as unknown as WailsBindingDeps["tray"],
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
    username?: string;
    password?: string;
    autoLogin?: boolean;
  }) => {
    if (!bindings.terminal.StartTelnet) missingBridgeMethod("startTelnetSession");
    const sessionID = await bindings.terminal.StartTelnet({
      hostname: options.hostname,
      port: options.port ?? 23,
      cols: options.cols ?? 80,
      rows: options.rows ?? 24,
      username: options.username ?? "",
      password: options.password ?? "",
      autoLogin: options.autoLogin ?? false,
      promptTimeoutSecs: 0,
    });
    await attachDataPlane(sessionID);
    return sessionID;
  };
  const startSerialSession = async (options: {
    path: string;
    baudRate?: number;
    dataBits?: number;
    stopBits?: string | number;
    parity?: string;
    flowControl?: string;
  }) => {
    if (!bindings.terminal.StartSerial) missingBridgeMethod("startSerialSession");
    const sessionID = await bindings.terminal.StartSerial({
      path: options.path,
      baudRate: options.baudRate ?? 115200,
      dataBits: options.dataBits ?? 8,
      stopBits: options.stopBits === undefined ? "1" : String(options.stopBits),
      parity: options.parity ?? "none",
      flowControl: options.flowControl ?? "none",
    });
    await attachDataPlane(sessionID);
    return sessionID;
  };
  const listSerialPorts = async () => {
    const ports = await bindings.terminal.ListSerialPorts?.() ?? [];
    return ports.map((port) => ({
      path: port.name,
      manufacturer: port.manufacturer ?? "",
      serialNumber: port.serialNumber ?? "",
      vendorId: port.vendorId ?? "",
      productId: port.productId ?? "",
      pnpId: port.pnpId ?? "",
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
    try {
      const stat = await bindings.sftp.Stat(sftpID, path);
      return statToSftpStatResult(stat);
    } catch (error) {
      // Conflict detection stats a not-yet-existing upload target; the
      // Electron bridge returns null there and so must this adapter.
      if (error instanceof Error && /does not exist|no such file/i.test(error.message)) {
        return null;
      }
      throw error;
    }
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
  const windowClose = () => bindings.window?.Hide?.() ?? bindings.window?.Close();
  const windowIsMaximized = () => bindings.window?.IsMaximised() ?? Promise.resolve(false);
  const windowIsFullscreen = () => bindings.window?.IsFullscreen() ?? Promise.resolve(false);
  const openSettingsWindow = () => bindings.settings?.Open() ?? Promise.resolve(false);
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
  const statLocalPath = (path: string) =>
    bindings.filesystem?.StatPath?.(path) as Promise<{ name: string; isDir: boolean; size: number }>;
  const getHomeDir = async () => {
    if (!bindings.filesystem?.HomeDir) missingBridgeMethod("getHomeDir");
    return bindings.filesystem.HomeDir();
  };
  const listLocalDir = async (path: string) => {
    if (!bindings.filesystem?.ListDir) missingBridgeMethod("listLocalDir");
    return bindings.filesystem.ListDir(path);
  };
  const localTreeScans = new Map<string, AbortController>();
  const listLocalTree: NonNullable<NetcattyBridge["listLocalTree"]> = async (path, options = {}) => {
    const scanId = options.scanId ?? crypto.randomUUID();
    if (localTreeScans.has(scanId)) throw new Error("Local tree scan already running");
    const controller = new AbortController();
    localTreeScans.set(scanId, controller);
    try {
      return await readLocalTree(path, listLocalDir, options, controller.signal);
    } finally {
      localTreeScans.delete(scanId);
    }
  };
  const cancelLocalTreeScan = async (scanId: string) => {
    const error = Object.assign(new Error("Drop scan cancelled"), { code: "ERR_DROP_SCAN_CANCELLED" });
    localTreeScans.get(scanId)?.abort(error);
  };
  const appendDiagnosticLog = (line: string) => {
    try {
      void bindings.diagnosticLog?.Append?.(line)?.catch(() => undefined);
    } catch {
      // diagnostics must never break the flow
    }
  };
  type FilesDroppedCallback = (payload: {
    filenames: string[];
    x: number;
    y: number;
    elementDetails?: { id?: string; classList?: string[]; attributes?: Record<string, string> };
  }) => void;
  const normalizeFilesDroppedPayload = (event: { data?: unknown } | unknown): Parameters<FilesDroppedCallback>[0] | null => {
    const raw = (event as { data?: unknown } | null)?.data ?? event;
    const candidate = Array.isArray(raw) ? raw[0] : raw;
    if (!candidate || typeof candidate !== "object") return null;
    const record = candidate as Record<string, unknown>;
    const filenames = (record.filenames ?? record.Filenames) as unknown;
    if (!Array.isArray(filenames) || filenames.length === 0) return null;
    const details = (record.elementDetails ?? record.ElementDetails ?? record) as Record<string, unknown>;
    const attributes = (details.attributes ?? details.Attributes) as Record<string, string> | undefined;
    return {
      filenames: filenames.map(String),
      x: Number(record.x ?? record.X ?? details.x ?? details.X ?? 0),
      y: Number(record.y ?? record.Y ?? details.y ?? details.Y ?? 0),
      elementDetails: {
        id: typeof details.id === "string" ? details.id : typeof details.ID === "string" ? details.ID : undefined,
        classList: Array.isArray(details.classList) ? details.classList as string[]
          : Array.isArray(details.ClassList) ? details.ClassList as string[] : undefined,
        attributes,
      },
    };
  };
  const filesDroppedListeners = new Set<FilesDroppedCallback>();
  let filesDroppedSubscribed = false;
  const subscribeFilesDropped = () => {
    if (filesDroppedSubscribed) return;
    const eventsOn = bindings.events?.On ?? Events.On;
    if (typeof eventsOn !== "function") return;
    filesDroppedSubscribed = true;
    eventsOn("netcatty:files-dropped", (event) => {
      const payload = normalizeFilesDroppedPayload(event);
      if (!payload) return;
      for (const listener of filesDroppedListeners) listener(payload);
    });
  };
  const onFilesDropped = (cb: FilesDroppedCallback) => {
    subscribeFilesDropped();
    filesDroppedListeners.add(cb);
    return () => {
      filesDroppedListeners.delete(cb);
    };
  };
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

  type TelnetEchoCallback = Parameters<NonNullable<NetcattyBridge["onTelnetEchoMode"]>>[1];
  type TelnetLoginCallback = Parameters<NonNullable<NetcattyBridge["onTelnetAutoLoginComplete"]>>[1];
  type MoshReadyCallback = Parameters<NonNullable<NetcattyBridge["onMoshSessionReady"]>>[1];
  const telnetEchoListeners = new Map<string, Set<TelnetEchoCallback>>();
  const telnetLoginListeners = new Map<string, Set<TelnetLoginCallback>>();
  const telnetCancelListeners = new Map<string, Set<TelnetLoginCallback>>();
  const moshReadyListeners = new Map<string, Set<MoshReadyCallback>>();
  let terminalEventsSubscribed = false;
  const subscribeTerminalEvents = () => {
    if (terminalEventsSubscribed) return;
    const eventsOn = bindings.events?.On ?? Events.On;
    if (typeof eventsOn !== "function") return;
    terminalEventsSubscribed = true;
    eventsOn("telnet:echo-mode", (event) => {
      const payload = (event?.data ?? event) as { sessionId?: string; remoteEcho?: boolean; localEcho?: boolean };
      const sessionId = payload.sessionId ?? "";
      for (const listener of telnetEchoListeners.get(sessionId) ?? []) {
        listener({ sessionId, remoteEcho: Boolean(payload.remoteEcho), localEcho: Boolean(payload.localEcho) });
      }
    });
    eventsOn("telnet:auto-login-complete", (event) => {
      const payload = (event?.data ?? event) as { sessionId?: string };
      const sessionId = payload.sessionId ?? "";
      for (const listener of telnetLoginListeners.get(sessionId) ?? []) listener({ sessionId });
    });
    eventsOn("telnet:auto-login-cancelled", (event) => {
      const payload = (event?.data ?? event) as { sessionId?: string };
      const sessionId = payload.sessionId ?? "";
      for (const listener of telnetCancelListeners.get(sessionId) ?? []) listener({ sessionId });
    });
    eventsOn("mosh:session-ready", (event) => {
      const payload = (event?.data ?? event) as { sessionId?: string };
      const sessionId = payload.sessionId ?? "";
      for (const listener of moshReadyListeners.get(sessionId) ?? []) listener({ sessionId });
    });
    eventsOn("et:session-ready", (event) => {
      const payload = (event?.data ?? event) as { sessionId?: string };
      const sessionId = payload.sessionId ?? "";
      for (const listener of moshReadyListeners.get(sessionId) ?? []) listener({ sessionId });
    });
  };
  const onTelnetEchoMode = ((sessionId: string, cb: TelnetEchoCallback) => {
    subscribeTerminalEvents();
    const set = telnetEchoListeners.get(sessionId) ?? new Set();
    set.add(cb);
    telnetEchoListeners.set(sessionId, set);
    return () => set.delete(cb);
  }) as unknown as NetcattyBridge["onTelnetEchoMode"];
  const onTelnetAutoLoginComplete = ((sessionId: string, cb: TelnetLoginCallback) => {
    subscribeTerminalEvents();
    const set = telnetLoginListeners.get(sessionId) ?? new Set();
    set.add(cb);
    telnetLoginListeners.set(sessionId, set);
    return () => set.delete(cb);
  }) as unknown as NetcattyBridge["onTelnetAutoLoginComplete"];
  const onTelnetAutoLoginCancelled = ((sessionId: string, cb: TelnetLoginCallback) => {
    subscribeTerminalEvents();
    const set = telnetCancelListeners.get(sessionId) ?? new Set();
    set.add(cb);
    telnetCancelListeners.set(sessionId, set);
    return () => set.delete(cb);
  }) as unknown as NetcattyBridge["onTelnetAutoLoginCancelled"];
  const onMoshSessionReady = ((sessionId: string, cb: MoshReadyCallback) => {
    subscribeTerminalEvents();
    const set = moshReadyListeners.get(sessionId) ?? new Set();
    set.add(cb);
    moshReadyListeners.set(sessionId, set);
    return () => set.delete(cb);
  }) as unknown as NetcattyBridge["onMoshSessionReady"];

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
    onTelnetEchoMode,
    onTelnetAutoLoginComplete,
    onTelnetAutoLoginCancelled,
    onMoshSessionReady,
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
    getPathForFile: (() => {
      // WebView2 File.path points at the dropped file but os.Open rejects it
      // with PATH_NOT_FOUND on some hosts while os.Stat succeeds. Return
      // undefined so the upload pipeline stages the File content instead.
      return undefined;
    }) as unknown as NetcattyBridge["getPathForFile"],
    statLocalPath: statLocalPath as unknown as NetcattyBridge["statLocalPath"],
    getHomeDir,
    listLocalDir,
    listLocalTree,
    cancelLocalTreeScan,
    stageFromLocalPath: ((path: string) =>
      bindings.filesystem?.StageFromLocalPath?.(path) as Promise<{ stagedPath: string; name: string; size: number }>) as unknown as NetcattyBridge["stageFromLocalPath"],
    appendDiagnosticLog: appendDiagnosticLog as unknown as NetcattyBridge["appendDiagnosticLog"],
    stageUploadFile: (async (file: File, _transferId: string) => {
      if (!bindings.filesystem?.StageBegin || !bindings.filesystem?.StageAppend || !bindings.filesystem?.StageDiscard) {
        throw new Error("staged uploads are not available");
      }
      const tempPath = await bindings.filesystem.StageBegin(file.name || "upload.bin");
      try {
        const chunkSize = 4 * 1024 * 1024;
        for (let offset = 0; offset < file.size; offset += chunkSize) {
          const slice = file.slice(offset, offset + chunkSize);
          const bytes = new Uint8Array(await slice.arrayBuffer());
          await bindings.filesystem.StageAppend(tempPath, offset, bytesToBase64(bytes));
        }
        return tempPath;
      } catch (error) {
        await bindings.filesystem.StageDiscard(tempPath).catch(() => undefined);
        throw error;
      }
    }) as unknown as NetcattyBridge["stageUploadFile"],
    deleteTempFile: (async (filePath: string) => {
      await bindings.filesystem?.StageDiscard?.(filePath);
      return { success: true };
    }) as unknown as NetcattyBridge["deleteTempFile"],
    onFilesDropped,
    setLanguage: (async (language: string) => {
      const changed = await bindings.tray?.SetLanguage?.(language);
      return changed ?? false;
    }) as unknown as NetcattyBridge["setLanguage"],
    quitApp: (async () => {
      await bindings.tray?.Quit?.();
    }) as unknown as NetcattyBridge["quitApp"],
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
    getOSProtocolStatus: (async () => {
      const result = await bindings.deepLink?.GetOSProtocolStatus?.();
      return result ?? { success: false, registered: false, error: "getOSProtocolStatus unavailable" };
    }) as unknown as NetcattyBridge["getOSProtocolStatus"],
    setOSProtocol: (async (enabled: boolean) => {
      const result = await bindings.deepLink?.SetOSProtocol?.(enabled);
      return result ?? { success: false, registered: false, error: "setOSProtocol unavailable" };
    }) as unknown as NetcattyBridge["setOSProtocol"],
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
    cancelZmodem: (async (sessionID: string) => {
      if (!bindings.terminal.CancelZmodem) return { success: false, error: "cancelZmodem unavailable" };
      await bindings.terminal.CancelZmodem(sessionID);
      return { success: true };
    }) as unknown as NetcattyBridge["cancelZmodem"],
    sendSerialYmodem: (async (sessionId: string, filePath: string) => {
      if (!bindings.terminal.SendSerialYmodem) return { success: false, error: "sendSerialYmodem unavailable" };
      try {
        const result = await bindings.terminal.SendSerialYmodem(sessionId, filePath);
        return { success: true, fileName: result.fileName, totalBytes: result.totalBytes, writtenBytes: result.writtenBytes };
      } catch (error) {
        return { success: false, error: error instanceof Error ? error.message : String(error) };
      }
    }) as unknown as NetcattyBridge["sendSerialYmodem"],
    receiveSerialYmodem: (async (sessionId: string, destinationDir: string) => {
      if (!bindings.terminal.ReceiveSerialYmodem) return { success: false, error: "receiveSerialYmodem unavailable" };
      try {
        const results = await bindings.terminal.ReceiveSerialYmodem(sessionId, destinationDir);
        return {
          success: true,
          files: results.map((file) => ({
            fileName: file.fileName,
            filePath: file.filePath,
            totalBytes: file.totalBytes,
            writtenBytes: file.writtenBytes,
          })),
          fileCount: results.length,
        };
      } catch (error) {
        return { success: false, error: error instanceof Error ? error.message : String(error) };
      }
    }) as unknown as NetcattyBridge["receiveSerialYmodem"],
    extractSftpArchive: (async (sftpId: string, remotePath: string) => {
      if (!bindings.sftp.ExtractArchive) return { success: false };
      await bindings.sftp.ExtractArchive(sftpId, remotePath);
      return { success: true };
    }) as unknown as NetcattyBridge["extractSftpArchive"],
    startCompressedUpload: (async (options: { sftpId: string; folderPath: string; targetPath: string; folderName: string; compressionId: string }) => {
      if (!bindings.sftp.UploadCompressedFolder) {
        return { success: false, error: "compressed upload is not wired on the Wails runtime yet", compressionId: options.compressionId };
      }
      const remoteZip = `${options.targetPath.replace(/\/$/, "")}/${options.folderName}.zip`;
      await bindings.sftp.UploadCompressedFolder(options.sftpId, options.folderPath, remoteZip);
      return { success: true, compressionId: options.compressionId };
    }) as unknown as NetcattyBridge["startCompressedUpload"],
    checkCompressedUploadSupport: (async () => ({ supported: Boolean(bindings.sftp.UploadCompressedFolder), localTar: false, remoteTar: false })) as unknown as NetcattyBridge["checkCompressedUploadSupport"],
    registerGlobalHotkey: (async (hotkey: string) => {
      if (!bindings.shortcuts?.Register) return { success: false, error: "registerGlobalHotkey unavailable" };
      return bindings.shortcuts.Register(hotkey);
    }) as unknown as NetcattyBridge["registerGlobalHotkey"],
    unregisterGlobalHotkey: (async () => {
      if (!bindings.shortcuts?.Unregister) return { success: false };
      return bindings.shortcuts.Unregister();
    }) as unknown as NetcattyBridge["unregisterGlobalHotkey"],
    getGlobalHotkeyStatus: (async () => {
      if (!bindings.shortcuts?.Status) return { enabled: false, hotkey: null };
      return bindings.shortcuts.Status();
    }) as unknown as NetcattyBridge["getGlobalHotkeyStatus"],
    startMoshSession: (async (options: Parameters<NonNullable<NetcattyBridge["startMoshSession"]>>[0]) => {
      if (!bindings.terminal.StartMosh) missingBridgeMethod("startMoshSession");
      const ssh = pickSSHConnectArgs({
        hostname: options.hostname,
        username: options.username ?? "",
        port: options.port,
        password: options.password,
        privateKey: options.privateKey,
        certificate: options.certificate,
        passphrase: options.passphrase,
        requiresMfa: options.requiresMfa,
        useSshAgent: options.useSshAgent,
        identityFilePaths: options.identityFilePaths,
        cols: options.cols,
        rows: options.rows,
      });
      const sessionID = await bindings.terminal.StartMosh({
        ...ssh,
        clientPath: options.moshClientPath ?? "",
        serverPath: options.moshServerPath ?? "",
        cols: options.cols ?? 80,
        rows: options.rows ?? 24,
      });
      await attachDataPlane(sessionID);
      return sessionID;
    }) as unknown as NetcattyBridge["startMoshSession"],
    startEtSession: (async (options: Parameters<NonNullable<NetcattyBridge["startEtSession"]>>[0]) => {
      if (!bindings.terminal.StartEt) missingBridgeMethod("startEtSession");
      const ssh = pickSSHConnectArgs({
        hostname: options.hostname,
        username: options.username ?? "",
        port: options.port,
        password: options.password,
        privateKey: options.privateKey,
        certificate: options.certificate,
        passphrase: options.passphrase,
        requiresMfa: options.requiresMfa,
        useSshAgent: options.useSshAgent,
        identityFilePaths: options.identityFilePaths,
        cols: options.cols,
        rows: options.rows,
      });
      const sessionID = await bindings.terminal.StartEt({
        ...ssh,
        clientPath: "",
        serverPath: "",
        cols: options.cols ?? 80,
        rows: options.rows ?? 24,
      });
      await attachDataPlane(sessionID);
      return sessionID;
    }) as unknown as NetcattyBridge["startEtSession"],
    getTelnetEchoMode: (async (sessionId: string) => {
      if (!bindings.terminal.GetTelnetEchoMode) {
        return { success: false, error: "getTelnetEchoMode unavailable" };
      }
      return bindings.terminal.GetTelnetEchoMode(sessionId);
    }) as unknown as NetcattyBridge["getTelnetEchoMode"],
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
      quitApp: async () => {
        await bindings.tray?.Quit?.();
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
      getHomeDir,
      listLocalDir,
      listLocalTree,
      cancelLocalTreeScan,
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
  // Go alpha.63 calls this global after resolving native file paths. The npm
  // runtime only installs _wails.handlePlatformFileDrop; reuse its Window here.
  // Remove the alias once Go uses that newer entry point too.
  const host = window as typeof window & { wails?: { Window?: typeof wailsWindow } };
  host.wails ??= {};
  host.wails.Window = wailsWindow;
  setActiveRuntimeClient(createWailsRuntimeClient());
  return true;
}
