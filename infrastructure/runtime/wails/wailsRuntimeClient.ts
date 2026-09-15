// Wails RuntimeClient adapter (P1-02). This module is the only frontend file
// allowed to import the Wails runtime and the generated bindings (enforced by
// ESLint). Ports without a Go owner reject every call fail-closed instead of
// pretending parity; they are implemented domain by domain from P2 onward.

import { Clipboard, Dialogs, Events, Window as wailsWindow } from "@wailsio/runtime";
import * as netcattyService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/netcattyservice";
import * as terminalService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/terminalservice";
import * as sftpService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/sftpservice";
import * as settingsWindowService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/settingswindowservice";
import * as forwardService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/forwardservice";
import * as appLockService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/applockservice";
import * as credentialService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/credentialservice";
import * as pluginService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/pluginservice";
import * as deepLinkService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/deeplinkservice";
import * as filesystemService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/filesystemservice";
import * as transferService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/transferservice";
import * as popupWindowService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/popupwindowservice";
import * as shortcutService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/shortcutservice";
import * as scriptService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/scriptservice";
import * as diagnosticLogService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/diagnosticlogservice";
import * as syncServiceBinding from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/syncservice";
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
import { createLocalShellBridge, type NativeLocalShellBindings } from './localShellBridge';
import { createCloudOAuthFacade, type CloudOAuthBindings } from '../../services/cloudSync/cloudSyncFacade';
import { createMonitoringBridge, type MonitoringBindings } from './monitoringBridge';
import { readLocalTree } from "./localTree";
import { createTransferBridge, type TransferBindings } from "./transferBridge";
import { createNativeFileActions, type NativeFileBindings } from "./nativeFileActions";
import * as profileBindings from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/profileservice";
import { createZmodemBridge } from './zmodemBridge';
import { subscribePopupConfig } from './popupConfigSubscription';
import { configureProfileBindings } from "../profile/profileClient";

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

function normalizePortForwardResult(tunnelId: string, result: unknown): PortForwardResult {
  const record = result && typeof result === "object" ? result as Record<string, unknown> : {};
  const success = record.success === true || record.Success === true;
  const status = String(record.status ?? record.Status ?? (success ? "active" : "error"));
  return {
    tunnelId: String(record.tunnelId ?? record.TunnelID ?? tunnelId),
    success,
    cancelled: Boolean(record.cancelled ?? record.Cancelled),
    blockedByCleanup: Boolean(record.blockedByCleanup ?? record.BlockedByCleanup),
    reused: Boolean(record.reused ?? record.Reused),
    status: status as PortForwardResult["status"],
    error: typeof record.error === "string" ? record.error : typeof record.Error === "string" ? record.Error : undefined,
  };
}

export interface WailsBindingDeps {
  terminal: NativeLocalShellBindings & MonitoringBindings & {
    ListAutocompleteDirectory?: (sessionID: string, directory: string, foldersOnly: boolean, prefix: string, limit: number) => Promise<{ success: boolean; entries: Array<{ name: string; type: 'file' | 'directory' | 'symlink' }>; error?: string }>;
    GetSessionPwd?: (sessionID: string, options: { allowHomeFallback: boolean; allowLoginShellFallback: boolean; timeoutMs: number }) => Promise<{ success: boolean; cwd?: string; error?: string }>;
    GetSessionRemoteInfo?: (sessionID: string) => Promise<{ success: boolean; remoteSshVersion?: string; error?: string }>;
    Connect: (request: unknown) => Promise<string>;
    TestProxy?: (request: {
      kind: string;
      host?: string;
      port?: number;
      username?: string;
      password?: string;
      command?: string;
      targetHost?: string;
      targetPort?: number;
    }) => Promise<{ ok: boolean; latencyMs: number; error?: string }>;
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
    SendZmodem?: (sessionID: string, filePath: string, remoteName: string, command: string) => Promise<unknown>;
    ReceiveZmodem?: (sessionID: string, destinationDir: string) => Promise<unknown>;
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
    RestartHelper?: (sessionID: string) => Promise<NetcattyHelperSessionState>;
    Write: (...args: unknown[]) => unknown;
    Resize: (...args: unknown[]) => unknown;
    Signal: (...args: unknown[]) => unknown;
    GetExitStatus?: (sessionID: string) => Promise<SessionExitEvent | null>;
    Close: (sessionID: string) => Promise<unknown>;
    Bootstrap: (sessionID: string) => Promise<WailsRouteBootstrap>;
    Reconnect?: (sessionID: string) => Promise<WailsRouteBootstrap>;
    ListenAddr: () => Promise<string> | string;
  };
  sftp: {
    OpenForTerminal?: (sessionId: string) => Promise<string>;
    Open: (request: unknown) => Promise<string>;
    Download?: (sftpID: string, remotePath: string, localPath: string) => Promise<number>;
    Upload?: (sftpID: string, localPath: string, remotePath: string) => Promise<number>;
    List: (sftpID: string, path: string) => Promise<WailsSftpEntry[]>;
    Mkdir: (sftpID: string, path: string) => Promise<unknown>;
    Remove: (sftpID: string, path: string) => Promise<unknown>;
    Rename: (sftpID: string, oldPath: string, newPath: string) => Promise<unknown>;
    Stat: (sftpID: string, path: string) => Promise<WailsSftpFileInfo>;
    Close: (sftpID: string) => Promise<unknown>;
    Chmod?: (sftpID: string, path: string, mode: string) => Promise<unknown>;
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
    ShowSystemNotification?: NonNullable<NetcattyBridge["showSystemNotification"]>;
    Open: () => Promise<boolean>;
    Show?: () => Promise<unknown>;
    PaintReady?: () => Promise<boolean>;
    Close: () => Promise<unknown>;
  };
  forward?: {
    Start: (id: string, kind: string, bindHost: string, bindPort: number, targetHost: string, targetPort: number, request: unknown) => Promise<unknown>;
    Stop: (id: string) => Promise<unknown>;
    StopByRuleId?: (ruleId: string) => Promise<unknown>;
    List: () => Promise<unknown>;
    Snapshot: (id: string) => Promise<unknown>;
    RuntimeSnapshot?: () => Promise<unknown>;
  };
  dialogs?: {
    OpenFile: (options: Record<string, unknown>) => Promise<string | string[]>;
    SaveFile: (options: Record<string, unknown>) => Promise<string>;
  };
  credential?: {
    Available?: () => Promise<boolean>;
    Seal?: (plaintextBase64: string, purpose: string) => Promise<string>;
    Open?: (envelopeBase64: string, purpose: string) => Promise<string>;
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
    UnlockWithBiometrics?: () => Promise<{ success: boolean; error?: string }>;
    GetSettings?: () => ReturnType<NonNullable<NetcattyBridge["getAppLockSettings"]>>;
    GetSystemUnlockStatus?: () => ReturnType<NonNullable<NetcattyBridge["getAppLockSystemUnlockStatus"]>>;
    SetSystemUnlockEnabled?: (enabled: boolean, password: string, autoPrompt: boolean) => Promise<{ systemUnlockEnabled: boolean; systemUnlockAutoPromptEnabled: boolean }>;
    SetRuntimeLocked?: (reason: string) => Promise<unknown>;
  };
  plugins?: {
    List: () => Promise<unknown[]>;
    Install?: (pluginID: string, version: string, sha256Hex: string, manifestJSON: string) => Promise<unknown>;
    SetEnabled?: (pluginID: string, enabled: boolean) => Promise<unknown>;
    UISchema?: (pluginID: string) => Promise<unknown>;
    Settings?: (pluginID: string) => Promise<Record<string, string | number | boolean>>;
    SetSetting?: (pluginID: string, settingID: string, valueJSON: string) => Promise<unknown>;
    GrantPermission?: (pluginID: string, kind: string, resource: string, mode: string, lifetime: string) => Promise<unknown>;
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
  filesystem?: NativeFileBindings & {
    ReadClipboardImage?: () => Promise<{ path: string; name: string; mediaType: string; size?: number } | null>;
    TempInfo?: () => Promise<{ path: string; fileCount: number; totalSize: number }>;
    TempFilePath?: (name: string) => Promise<string>;
    ClearTemp?: () => Promise<{ success: boolean; deletedCount: number }>;
    HomeDir?: () => Promise<string>;
    ListDir?: (path: string) => Promise<RemoteFile[]>;
    ExtractArchive?: (archivePath: string, destinationRoot: string) => Promise<number>;
    StatPath?: (path: string) => Promise<{ name: string; isDir: boolean; size: number }>;
    StageFromLocalPath?: (path: string) => Promise<{ stagedPath: string; name: string; size: number }>;
    StageBegin?: (fileName: string) => Promise<string>;
    StageAppend?: (tempPath: string, offset: number, data: string) => Promise<unknown>;
    StageDiscard?: (tempPath: string) => Promise<unknown>;
  };
  sync?: {
    CloudSyncSetSessionPassword?: (password: string) => Promise<boolean>;
    CloudSyncGetSessionPassword?: () => Promise<{ password?: string; found?: boolean }>;
    CloudSyncClearSessionPassword?: () => Promise<{ success?: boolean }>;
    CloudSyncResetEverything?: () => Promise<string[]>;
    GetVaultBackupCapabilities?: () => Promise<{ encryptionAvailable?: boolean }>;
    CreateVaultBackup?: (payload: unknown) => Promise<{ created?: boolean; backup?: unknown }>;
    ListVaultBackups?: () => Promise<{ backups?: unknown[] } | unknown[]>;
    ReadVaultBackup?: (payload: unknown) => Promise<unknown>;
    TrimVaultBackups?: (payload: unknown) => Promise<{ deletedCount?: number; keptCount?: number }>;
    OpenVaultBackupDir?: () => Promise<{ success?: boolean; path?: string }>;
    PrepareOAuthCallback?: () => Promise<{ sessionId: string; port: number; redirectUri: string }>;
    OpenProviderConsole?: (provider: 'github' | 'google' | 'onedrive') => Promise<void>;
    GithubStartDeviceFlow?: (options: { clientId?: string; scope?: string }) => Promise<unknown>;
    GithubGetUserInfo?: (options: unknown) => Promise<unknown>;
    GithubFindSyncFile?: (options: unknown) => Promise<unknown>;
    GoogleGetUserInfo?: (options: unknown) => Promise<unknown>;
    GoogleExchangeCodeForTokens?: (options: unknown) => Promise<unknown>;
    GoogleDriveGetRevisionHistory?: (options: unknown) => Promise<{ revisions?: Array<{ version: string; date: string }> } | Array<{ version: string; date: string }>>;
    OnedriveGetUserInfo?: (options: unknown) => Promise<unknown>;
    CloudSyncWebdavInitialize?: (config: unknown) => Promise<{ resourceId: string | null }>;
    CloudSyncWebdavUpload?: (config: unknown, syncedFile: unknown) => Promise<{ resourceId: string }>;
    CloudSyncWebdavDownload?: (config: unknown) => Promise<{ syncedFile: unknown | null }>;
    CloudSyncWebdavDelete?: (config: unknown) => Promise<{ ok: true }>;
    CloudSyncS3Initialize?: (config: unknown) => Promise<{ resourceId: string | null }>;
    CloudSyncS3Upload?: (config: unknown, syncedFile: unknown) => Promise<{ resourceId: string }>;
    CloudSyncS3Download?: (config: unknown) => Promise<{ syncedFile: unknown | null }>;
    CloudSyncS3Delete?: (config: unknown) => Promise<{ ok: true }>;
  };
  transfer?: Partial<TransferBindings> & {
    Enqueue?: (spec: unknown) => Promise<unknown>;
  };
  events?: {
    On: (name: string, callback: (event: { data?: unknown }) => void) => () => void;
  };
  popup?: {
    Open: (payload: unknown) => Promise<{ success: boolean; popupId?: string; error?: string }>;
    GetConfig?: (popupId: string, token: string) => Promise<unknown>;
    Heartbeat?: (popupId: string, token: string) => Promise<unknown>;
  };
  shortcuts?: {
    Register?: (raw: string) => Promise<{ success: boolean; enabled?: boolean; error?: string; accelerator?: string }>;
    Unregister?: () => Promise<{ success: boolean }>;
    Status?: () => Promise<{ enabled: boolean; hotkey: string | null }>;
  };
  script?: {
    StartRecording?: (sessionID: string) => Promise<{ ok?: boolean; error?: string }>;
    StopRecording?: (sessionID: string) => Promise<{ steps?: unknown[]; code?: string }>;
    AppendRecordingStep?: (sessionID: string, step: unknown) => Promise<{
      ok?: boolean;
      stopped?: boolean;
      reason?: string;
      error?: string;
      steps?: unknown[];
      code?: string;
    }>;
    ResolveDialog?: (requestId: string, value: string, cancelled: boolean) => Promise<boolean>;
    Run?: (request: {
      runId?: string;
      scriptId?: string;
      scriptLabel?: string;
      sessionId: string;
      content: string;
    }) => Promise<{ ok?: boolean; error?: string; runId?: string; runIds?: string[] }>;
    Stop?: (runID: string) => Promise<{ ok?: boolean }>;
    Pause?: (runID: string) => Promise<{ ok?: boolean }>;
    Resume?: (runID: string) => Promise<{ ok?: boolean }>;
    GetRuns?: (sessionID?: string) => Promise<unknown[]>;
  };
  diagnosticLog?: {
    Append?: (line: string) => Promise<unknown>;
  };
  tray?: {
    SetLanguage?: (language: string) => Promise<boolean>;
    Quit?: () => Promise<void>;
  };
  clipboard?: { SetText: (text: string) => Promise<boolean>; Text: () => Promise<string> };
  openDataPlane?: typeof openDataPlaneSession;
}

  const defaultBindings: WailsBindingDeps = {
    clipboard: Clipboard,
    terminal: terminalService as unknown as WailsBindingDeps["terminal"],
    sftp: sftpService as unknown as WailsBindingDeps["sftp"],
    window: wailsWindow,
    settings: settingsWindowService as unknown as WailsBindingDeps["settings"],
    forward: forwardService as unknown as WailsBindingDeps["forward"],
    dialogs: Dialogs as unknown as WailsBindingDeps["dialogs"],
    appLock: appLockService as unknown as WailsBindingDeps["appLock"],
    credential: credentialService as unknown as WailsBindingDeps["credential"],
    plugins: pluginService as unknown as WailsBindingDeps["plugins"],
    events: Events as unknown as WailsBindingDeps["events"],
    deepLink: deepLinkService as unknown as WailsBindingDeps["deepLink"],
    filesystem: filesystemService as unknown as WailsBindingDeps["filesystem"],
    transfer: transferService as unknown as WailsBindingDeps["transfer"],
    popup: popupWindowService as unknown as WailsBindingDeps["popup"],
    shortcuts: shortcutService as unknown as WailsBindingDeps["shortcuts"],
    script: scriptService as unknown as WailsBindingDeps["script"],
    diagnosticLog: diagnosticLogService as unknown as WailsBindingDeps["diagnosticLog"],
    tray: trayService as unknown as WailsBindingDeps["tray"],
    sync: syncServiceBinding as unknown as WailsBindingDeps["sync"],
  };

type SessionDataCallback = Parameters<NetcattyBridge["onSessionData"]>[1];
type SessionExitEvent = { sessionId: string; exitCode?: number; reason?: "exited" | "error" | "closed" | "timeout"; error?: string; intentional?: boolean };
type SessionExitCallback = (evt: SessionExitEvent) => void;
type HelperLifecycleCallback = Parameters<NonNullable<NetcattyBridge["onHelperLifecycle"]>>[1];

export function createWailsRuntimeClient(bindings: WailsBindingDeps = defaultBindings): RuntimeClient {
  // Field-level credential storage over the Go credential provider. The
  // envelope keeps the Electron enc:v1: sentinel so renderer-side detection
  // (no double encryption, migration) behaves identically.
  const CREDENTIAL_ENC_PREFIX = "enc:v1:";
  const CREDENTIAL_PURPOSE = "cloud-sync-credentials";
  const credentialsAvailable = async () => bindings.credential?.Available?.() ?? false;
  const credentialsEncrypt = async (plaintext: string) => {
    if (!bindings.credential?.Seal) missingBridgeMethod("credentialsEncrypt");
    if (plaintext.startsWith(CREDENTIAL_ENC_PREFIX)) return plaintext;
    const sealed = await bindings.credential.Seal(bytesToBase64(new TextEncoder().encode(plaintext)), CREDENTIAL_PURPOSE);
    return CREDENTIAL_ENC_PREFIX + sealed;
  };
  const credentialsDecrypt = async (value: string) => {
    if (typeof value !== "string" || value.length === 0 || !value.startsWith(CREDENTIAL_ENC_PREFIX)) return value;
    if (!bindings.credential?.Open) missingBridgeMethod("credentialsDecrypt");
    const opened = await bindings.credential.Open(value.slice(CREDENTIAL_ENC_PREFIX.length), CREDENTIAL_PURPOSE);
    const plainBytes = Uint8Array.from(atob(opened), (ch) => ch.charCodeAt(0));
    return new TextDecoder().decode(plainBytes);
  };
  const zmodem = createZmodemBridge(bindings.terminal, bindings.filesystem, (name, callback) =>
    (bindings.events?.On ?? Events.On)(name, callback), () => selectDirectory());
  const transfers = createTransferBridge(bindings.transfer as TransferBindings | undefined);
  const platform = typeof navigator === 'undefined' ? '' : navigator.platform;
  const nativeFileActions = createNativeFileActions(bindings.filesystem, bindings.dialogs, transfers.startStreamTransfer,
    /Win/i.test(platform) ? 'win32' : /Mac/i.test(platform) ? 'darwin' : 'linux');
  const unimplemented = <T extends object>(portName: string): T => portWith<T>(portName, {});
  const sessionAliases = new Map<string, string>();
  const nativeSessionId = (id: string) => sessionAliases.get(id) ?? id;
  const monitoring = createMonitoringBridge(bindings.terminal, nativeSessionId);
  // Cloud OAuth facade: maps the generated PascalCase sync bindings onto the
  // camelCase bridge surface the cloud sync adapters and UI already call.
  const cloudOAuth = createCloudOAuthFacade(bindings.sync as unknown as CloudOAuthBindings);
  const cloudSyncSetSessionPassword = (async (password: string) => {
    if (!bindings.sync?.CloudSyncSetSessionPassword) missingBridgeMethod("cloudSyncSetSessionPassword");
    return bindings.sync.CloudSyncSetSessionPassword(password);
  }) as unknown as NetcattyBridge["cloudSyncSetSessionPassword"];
  const cloudSyncGetSessionPassword = (async () => {
    const result = await bindings.sync?.CloudSyncGetSessionPassword?.();
    return result?.password ?? null;
  }) as unknown as NetcattyBridge["cloudSyncGetSessionPassword"];
  const cloudSyncClearSessionPassword = (async () => {
    return (await bindings.sync?.CloudSyncClearSessionPassword?.()) ?? { success: false };
  }) as unknown as NetcattyBridge["cloudSyncClearSessionPassword"];
  const cloudSyncResetEverything = (async () => {
    if (!bindings.sync?.CloudSyncResetEverything) missingBridgeMethod("cloudSyncResetEverything");
    // Wails wraps the []string return as { removedKeys: string[] }.
    const result = await bindings.sync.CloudSyncResetEverything();
    return result?.removedKeys ?? [];
  }) as unknown as NetcattyBridge["cloudSyncResetEverything"];
  const getVaultBackupCapabilities = (async () => {
    if (!bindings.sync?.GetVaultBackupCapabilities) missingBridgeMethod("getVaultBackupCapabilities");
    const result = await bindings.sync.GetVaultBackupCapabilities();
    return { encryptionAvailable: Boolean(result?.encryptionAvailable) };
  }) as unknown as NetcattyBridge["getVaultBackupCapabilities"];
  const createVaultBackup = (async (payload) => {
    if (!bindings.sync?.CreateVaultBackup) missingBridgeMethod("createVaultBackup");
    return bindings.sync.CreateVaultBackup(payload);
  }) as unknown as NetcattyBridge["createVaultBackup"];
  const listVaultBackups = (async () => {
    if (!bindings.sync?.ListVaultBackups) missingBridgeMethod("listVaultBackups");
    const result = await bindings.sync.ListVaultBackups();
    return Array.isArray(result) ? result : result?.backups ?? [];
  }) as unknown as NetcattyBridge["listVaultBackups"];
  const readVaultBackup = (async (payload) => {
    if (!bindings.sync?.ReadVaultBackup) missingBridgeMethod("readVaultBackup");
    return bindings.sync.ReadVaultBackup(payload);
  }) as unknown as NetcattyBridge["readVaultBackup"];
  const trimVaultBackups = (async (payload) => {
    if (!bindings.sync?.TrimVaultBackups) missingBridgeMethod("trimVaultBackups");
    return bindings.sync.TrimVaultBackups(payload);
  }) as unknown as NetcattyBridge["trimVaultBackups"];
  const openVaultBackupDir = (async () => {
    if (!bindings.sync?.OpenVaultBackupDir) missingBridgeMethod("openVaultBackupDir");
    return bindings.sync.OpenVaultBackupDir();
  }) as unknown as NetcattyBridge["openVaultBackupDir"];
  const openProviderConsole = (async (provider: 'github' | 'google' | 'onedrive') => {
    if (!bindings.sync?.OpenProviderConsole) missingBridgeMethod("openProviderConsole");
    await bindings.sync.OpenProviderConsole(provider);
  }) as unknown as NetcattyBridge["openProviderConsole"];
  const rememberSession = (uiId: string | undefined, nativeId: string) => {
    if (uiId) sessionAliases.set(uiId, nativeId);
  };
  const uiSessionId = (nativeId: string) => {
    for (const [alias, native] of sessionAliases) {
      if (native === nativeId) return alias;
    }
    return nativeId;
  };
  const getSessionPwd = async (id: string, options?: Parameters<NonNullable<NetcattyBridge["getSessionPwd"]>>[1]) => {
    try {
      return await bindings.terminal.GetSessionPwd?.(nativeSessionId(id), {
        allowHomeFallback: options?.allowHomeFallback ?? true,
        allowLoginShellFallback: options?.allowLoginShellFallback ?? options?.allowHomeFallback ?? true,
        timeoutMs: Number.isFinite(options?.timeoutMs) ? Math.min(5000, Math.max(100, options!.timeoutMs!)) : 2000,
      })
        ?? { success: false, error: "Terminal directory tracking unavailable" };
    } catch (error) { return { success: false, error: String(error) }; }
  };
  const getSessionRemoteInfo = async (id: string) =>
    await bindings.terminal.GetSessionRemoteInfo?.(nativeSessionId(id)) ?? { success: false };
  const writeClipboardText = (text: string) => (bindings.clipboard ?? Clipboard).SetText(text);
  const readClipboardText = () => (bindings.clipboard ?? Clipboard).Text();
  const readClipboardImage = () => bindings.filesystem?.ReadClipboardImage?.() ?? Promise.resolve(null);
  const openSftpForSession = (id: string) => {
    if (!bindings.sftp.OpenForTerminal) return Promise.reject(new Error("Terminal SFTP unavailable"));
    return bindings.sftp.OpenForTerminal(nativeSessionId(id));
  };
  const dataListeners = new Map<string, Set<SessionDataCallback>>();
  const exitListeners = new Map<string, Set<SessionExitCallback>>();
  const planes = new Map<string, DataPlaneSessionHandle>();
  const routeStates = new Map<string, { version: number; retries: number; timer?: ReturnType<typeof setTimeout>; closed: boolean }>();

  function emitData(sessionID: string, chunk: string): void {
    for (const listener of dataListeners.get(sessionID) ?? []) listener(chunk);
  }

  function emitExit(sessionID: string, status: SessionExitEvent = { sessionId: sessionID }): void {
    for (const listener of exitListeners.get(sessionID) ?? []) listener({ ...status, sessionId: sessionID });
  }

  async function emitNativeExit(sessionID: string): Promise<void> {
    try {
      const status = await bindings.terminal.GetExitStatus?.(sessionID);
      emitExit(sessionID, status ?? { sessionId: sessionID });
    } catch (error) {
      emitExit(sessionID, { sessionId: sessionID, reason: "error", error: String(error) });
    }
  }

  const showSystemNotification: NonNullable<NetcattyBridge["showSystemNotification"]> = async payload => {
    try {
      return await bindings.settings?.ShowSystemNotification?.(payload) ?? { shown: false, reason: "Native notifications unavailable" };
    } catch (error) { return { shown: false, reason: String(error) }; }
  };

  async function attachDataPlane(sessionID: string, reconnect = false): Promise<void> {
    let state = routeStates.get(sessionID);
    if (!state) {
      state = { version: 0, retries: 0, closed: false };
      routeStates.set(sessionID, state);
    }
    const owner = state;
    const version = ++owner.version;
    const current = () => !owner.closed && routeStates.get(sessionID) === owner && owner.version === version;
    planes.get(sessionID)?.dispose();
    const [bootstrap, listenAddr] = await Promise.all([
      reconnect
        ? (bindings.terminal.Reconnect ? bindings.terminal.Reconnect(sessionID) : Promise.reject(new Error('Terminal route reconnect unavailable')))
        : bindings.terminal.Bootstrap(sessionID),
      Promise.resolve(bindings.terminal.ListenAddr()),
    ]);
    if (!current()) return;
    const retry = () => {
      if (!current() || owner.timer) return;
      // Invalidate old callbacks before replacing the route. A dropped socket
      // is not a native process exit and must never restart Mosh/ET/SSH here.
      owner.version++;
      planes.get(sessionID)?.dispose();
      const schedule = () => {
        if (owner.closed || routeStates.get(sessionID) !== owner) return;
        if (owner.retries >= 6) {
          owner.closed = true;
          console.error('Terminal transport reconnect exhausted', sessionID);
          emitData(sessionID, '\r\n[Netcatty] Terminal connection could not be restored. Please reconnect.\r\n');
          emitExit(sessionID, { sessionId: sessionID, reason: "error", error: "Terminal transport reconnect exhausted" });
          return;
        }
        const delay = Math.min(1000 * 2 ** owner.retries++, 30000);
        owner.timer = setTimeout(() => {
          owner.timer = undefined;
          void attachDataPlane(sessionID, true).catch(error => {
            console.error('Terminal route reconnect failed', error);
            schedule();
          });
        }, delay);
      };
      schedule();
    };
    planes.set(sessionID, (bindings.openDataPlane ?? openDataPlaneSession)({
      listenAddr,
      bootstrap,
      onData: (chunk) => { if (current()) { owner.retries = 0; emitData(sessionID, chunk); } },
      onComplete: () => {
        if (!current()) return;
        owner.closed = true;
        if (owner.timer) clearTimeout(owner.timer);
        void emitNativeExit(sessionID);
      },
      onDisconnect: () => {
        if (!bindings.terminal.GetExitStatus) { retry(); return; }
        void bindings.terminal.GetExitStatus(sessionID).then(status => {
          if (!current()) return;
          if (!status) { retry(); return; }
          owner.closed = true;
          if (owner.timer) clearTimeout(owner.timer);
          emitExit(sessionID, status);
        }).catch(() => retry());
      },
    }));
  }

  const testProxy = (async (options: Parameters<NonNullable<NetcattyBridge["testProxy"]>>[0]) => {
    if (!bindings.terminal.TestProxy) missingBridgeMethod("testProxy");
    return bindings.terminal.TestProxy({
      kind: options.kind,
      host: options.host ?? "",
      port: options.port ?? 0,
      username: options.username ?? "",
      password: options.password ?? "",
      command: options.command ?? "",
      targetHost: options.targetHost ?? "",
      targetPort: options.targetPort ?? 0,
    });
  }) as NonNullable<NetcattyBridge["testProxy"]>;
  const startSSHSession = (options: Parameters<NetcattyBridge["startSSHSession"]>[0]) => {
    const args = pickSSHConnectArgs(options);
    return bindings.terminal.Connect(args).then(async (sessionID) => {
      rememberSession(options.sessionId, sessionID);
      await attachDataPlane(sessionID);
      return sessionID;
    });
  };
  const { startLocalSession, getDefaultShell, discoverShells, validatePath } = createLocalShellBridge(
    bindings.terminal,
    async (alias, id) => {
      rememberSession(alias, id);
      await attachDataPlane(id);
    },
  );
  const startTelnetSession = async (options: {
    sessionId?: string;
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
    rememberSession(options.sessionId, sessionID);
    await attachDataPlane(sessionID);
    return sessionID;
  };
  const startSerialSession = async (options: {
    sessionId?: string;
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
    rememberSession(options.sessionId, sessionID);
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
    bindings.terminal.Write(nativeSessionId(sessionID), bytesToBase64(new TextEncoder().encode(data))) as unknown as void;
  const resizeSession = (sessionID: string, cols: number, rows: number) =>
    bindings.terminal.Resize(nativeSessionId(sessionID), cols, rows) as unknown as void;
  const interruptSession = (sessionID: string) =>
    bindings.terminal.Signal(nativeSessionId(sessionID), "INT") as unknown as void;
  const closeSession = async (sessionID: string) => {
    sessionID = nativeSessionId(sessionID);
    for (const [alias, nativeId] of sessionAliases) {
      if (nativeId === sessionID) sessionAliases.delete(alias);
    }
    const route = routeStates.get(sessionID);
    if (route) {
      route.closed = true;
      if (route.timer) clearTimeout(route.timer);
      routeStates.delete(sessionID);
    }
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
    return bindings.sftp.Open({ ...args, sudo: options.sudo ?? false });
  };
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
  const { selectFile, selectDirectory, showSaveDialog } = nativeFileActions;
  const startPortForward = async (options: PortForwardOptions): Promise<PortForwardResult> => {
    if (!bindings.forward?.Start) missingBridgeMethod("startPortForward");
    const request = pickSSHConnectArgs(options as Parameters<NetcattyBridge["startSSHSession"]>[0]);
    const result = await bindings.forward.Start(
      options.tunnelId,
      options.type,
      options.bindAddress ?? "127.0.0.1",
      options.localPort,
      options.remoteHost ?? "",
      options.remotePort ?? 0,
      request,
    );
    return normalizePortForwardResult(options.tunnelId, result);
  };
  const stopPortForward = async (tunnelId: string): Promise<PortForwardResult> => {
    if (!bindings.forward?.Stop) missingBridgeMethod("stopPortForward");
    return normalizePortForwardResult(tunnelId, await bindings.forward.Stop(tunnelId));
  };
  const stopPortForwardByRuleId = async (ruleId: string) => {
    if (!bindings.forward?.StopByRuleId) missingBridgeMethod("stopPortForwardByRuleId");
    const result = await bindings.forward.StopByRuleId(ruleId) as { stopped?: number; failed?: number; errors?: string[] };
    return { stopped: result.stopped ?? 0, failed: result.failed, errors: result.errors };
  };
  const listPortForwards = async () => {
    if (!bindings.forward?.List) return [];
    const items = await bindings.forward.List() as Array<{ ruleId?: string; tunnelId?: string; type?: string; status?: string; error?: string }>;
    return (items ?? []).map((item) => ({
      ruleId: item.ruleId,
      tunnelId: item.tunnelId ?? "",
      type: item.type ?? "",
      status: item.status ?? "inactive",
      error: item.error,
    }));
  };
  const getPortForwardStatus = async (tunnelId: string): Promise<PortForwardStatusResult> => {
    if (!bindings.forward?.Snapshot) return { tunnelId, status: "inactive" };
    const result = normalizePortForwardResult(tunnelId, await bindings.forward.Snapshot(tunnelId));
    return { tunnelId, status: (result.status ?? "inactive") as PortForwardStatusResult["status"], error: result.error };
  };
  const getPortForwardSnapshot = async (): Promise<PortForwardRuntimeSnapshot> => {
    if (bindings.forward?.RuntimeSnapshot) {
      const snapshot = await bindings.forward.RuntimeSnapshot() as PortForwardRuntimeSnapshot;
      return snapshot ?? { epoch: "wails", revision: 0, records: [] };
    }
    const records = (await listPortForwards()).map((item) => ({
      ruleId: item.ruleId,
      tunnelId: item.tunnelId,
      phase: item.status,
      error: item.error,
      revision: 0,
      updatedAt: Date.now(),
    }));
    return { epoch: "wails", revision: 0, records };
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
  const helperLifecycleListeners = new Map<string, Set<HelperLifecycleCallback>>();
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
    // Supervised mosh/et helper lifecycle. The Go payload carries the native
    // session id; fans out to listeners registered under that id and to any
    // renderer alias mapped to it (mirrors writeToSession/closeSession).
    for (const name of ["mosh:lifecycle", "et:lifecycle"]) {
      eventsOn(name, (event) => {
        const payload = (event?.data ?? event) as NetcattyHelperSessionState | null;
        if (!payload || typeof payload.sessionId !== "string") return;
        const ids = new Set([payload.sessionId]);
        for (const [alias, native] of sessionAliases) {
          if (native === payload.sessionId) ids.add(alias);
        }
        for (const id of ids) {
          for (const listener of helperLifecycleListeners.get(id) ?? []) listener(payload);
        }
      });
    }
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
  const onHelperLifecycle = ((sessionId: string, cb: HelperLifecycleCallback) => {
    subscribeTerminalEvents();
    const set = helperLifecycleListeners.get(sessionId) ?? new Set();
    set.add(cb);
    helperLifecycleListeners.set(sessionId, set);
    return () => {
      set.delete(cb);
      if (set.size === 0) helperLifecycleListeners.delete(sessionId);
    };
  }) as unknown as NetcattyBridge["onHelperLifecycle"];
  const restartHelperSession = (async (sessionId: string) => {
    if (!bindings.terminal.RestartHelper) {
      return { success: false, error: "restartHelperSession unavailable" };
    }
    try {
      const state = await bindings.terminal.RestartHelper(nativeSessionId(sessionId));
      return { success: true, state };
    } catch (error) {
      return { success: false, error: error instanceof Error ? error.message : String(error) };
    }
  }) as unknown as NetcattyBridge["restartHelperSession"];

  const scriptRecordingStart = async (sessionId: string) => {
    if (!bindings.script?.StartRecording) missingBridgeMethod("scriptRecordingStart");
    const result = await bindings.script.StartRecording(nativeSessionId(sessionId));
    if (result?.error) throw new Error(result.error);
    return { ok: result?.ok !== false };
  };
  const scriptRecordingStop = async (sessionId: string) => {
    if (!bindings.script?.StopRecording) missingBridgeMethod("scriptRecordingStop");
    const result = await bindings.script.StopRecording(nativeSessionId(sessionId));
    return { steps: result?.steps ?? [], code: result?.code ?? "" };
  };
  const scriptRecordingAppendStep = (async (sessionId: string, step: unknown) => {
    if (!bindings.script?.AppendRecordingStep) missingBridgeMethod("scriptRecordingAppendStep");
    return bindings.script.AppendRecordingStep(nativeSessionId(sessionId), step);
  }) as unknown as NetcattyBridge["scriptRecordingAppendStep"];
  const scriptRun = (async (params: {
    runId?: string;
    scriptId?: string;
    scriptLabel?: string;
    content: string;
    sessionId?: string;
    sessionIds?: string[];
  }) => {
    if (!bindings.script?.Run) missingBridgeMethod("scriptRun");
    const sessionId = params.sessionId || params.sessionIds?.[0];
    if (!sessionId) throw new Error("sessionId required");
    const result = await bindings.script.Run({
      runId: params.runId,
      scriptId: params.scriptId,
      scriptLabel: params.scriptLabel,
      sessionId: nativeSessionId(sessionId),
      content: params.content,
    });
    if (result?.error) throw new Error(result.error);
    const runId = result?.runId || "";
    return { runId, runIds: result?.runIds ?? (runId ? [runId] : []) };
  }) as unknown as NetcattyBridge["scriptRun"];
  const scriptStop = async (runId: string) => {
    if (!bindings.script?.Stop) missingBridgeMethod("scriptStop");
    const result = await bindings.script.Stop(runId);
    return { ok: result?.ok !== false };
  };
  const scriptPause = async (runId: string) => {
    if (!bindings.script?.Pause) missingBridgeMethod("scriptPause");
    const result = await bindings.script.Pause(runId);
    return { ok: result?.ok !== false };
  };
  const scriptResume = async (runId: string) => {
    if (!bindings.script?.Resume) missingBridgeMethod("scriptResume");
    const result = await bindings.script.Resume(runId);
    return { ok: result?.ok !== false };
  };
  const scriptGetRuns = async (sessionId?: string) => {
    if (!bindings.script?.GetRuns) missingBridgeMethod("scriptGetRuns");
    const nativeId = sessionId ? nativeSessionId(sessionId) : "";
    const runs = await bindings.script.GetRuns(nativeId);
    if (!Array.isArray(runs)) return [];
    return runs.map((run) => {
      const entry = run as { sessionId?: string };
      return { ...entry, sessionId: entry.sessionId ? uiSessionId(entry.sessionId) : entry.sessionId };
    });
  };

  const scriptDialogRequestListeners = new Set<(payload: {
    requestId: string;
    type: string;
    message: string;
    defaultValue?: string;
    sensitive?: boolean;
  }) => void>();
  const scriptRunsUpdatedListeners = new Set<(payload: { runs: unknown[] }) => void>();
  let scriptEventsSubscribed = false;
  const subscribeScriptEvents = () => {
    if (scriptEventsSubscribed) return;
    const eventsOn = bindings.events?.On ?? Events.On;
    if (typeof eventsOn !== "function") return;
    scriptEventsSubscribed = true;
    eventsOn("netcatty:script:runs-updated", (event) => {
      const payload = ((event as { data?: unknown })?.data ?? event) as { runs?: unknown[] };
      const runs = Array.isArray(payload?.runs) ? payload.runs : [];
      for (const listener of scriptRunsUpdatedListeners) listener({ runs });
    });
    eventsOn("netcatty:script:dialog-request", (event) => {
      const payload = ((event as { data?: unknown })?.data ?? event) as {
        requestId?: string;
        type?: string;
        message?: string;
        defaultValue?: string;
        sensitive?: boolean;
      };
      if (!payload?.requestId) return;
      const request = {
        requestId: payload.requestId,
        type: payload.type ?? "prompt",
        message: payload.message ?? "",
        defaultValue: payload.defaultValue,
        sensitive: payload.sensitive,
      };
      for (const listener of scriptDialogRequestListeners) listener(request);
    });
  };
  const onScriptRunsUpdated = (cb: Parameters<NonNullable<NetcattyBridge["onScriptRunsUpdated"]>>[0]) => {
    subscribeScriptEvents();
    scriptRunsUpdatedListeners.add(cb);
    return () => {
      scriptRunsUpdatedListeners.delete(cb);
    };
  };
  const onScriptDialogRequest = (cb: Parameters<NonNullable<NetcattyBridge["onScriptDialogRequest"]>>[0]) => {
    subscribeScriptEvents();
    scriptDialogRequestListeners.add(cb);
    return () => {
      scriptDialogRequestListeners.delete(cb);
    };
  };
  const scriptDialogResponse = async (requestId: string, value?: unknown, cancelled?: boolean) => {
    if (!bindings.script?.ResolveDialog) missingBridgeMethod("scriptDialogResponse");
    const ok = await bindings.script.ResolveDialog(
      requestId,
      typeof value === "string" ? value : value === undefined || value === null ? "" : String(value),
      Boolean(cancelled),
    );
    return { ok };
  };

  const implementedBridge: Partial<NetcattyBridge> = {
    ...monitoring,
    ...cloudOAuth,
    openProviderConsole,
    scriptRecordingStart,
    scriptRecordingStop,
    scriptRecordingAppendStep,
    scriptRun,
    scriptStop,
    scriptPause,
    scriptResume,
    scriptGetRuns,
    onScriptRunsUpdated,
    onScriptDialogRequest,
    scriptDialogResponse,
    credentialsAvailable,
    credentialsEncrypt,
    credentialsDecrypt,
    cloudSyncSetSessionPassword,
    cloudSyncGetSessionPassword,
    cloudSyncClearSessionPassword,
    cloudSyncResetEverything,
    getVaultBackupCapabilities,
    createVaultBackup,
    listVaultBackups,
    readVaultBackup,
    trimVaultBackups,
    openVaultBackupDir,
    getDefaultShell,
    discoverShells,
    validatePath,
    getSessionPwd,
    getSessionRemoteInfo,
    showSystemNotification,
    writeClipboardText,
    readClipboardText,
    readClipboardImage,
    startSSHSession,
    testProxy,
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
    onHelperLifecycle,
    restartHelperSession,
    openSftp,
    openSftpForSession,
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
    ...nativeFileActions,
    listAutocompleteRemoteDir: async (id: string, directory: string, foldersOnly: boolean, prefix = '', limit = 100) => {
      try { return await bindings.terminal.ListAutocompleteDirectory?.(nativeSessionId(id), directory, foldersOnly, prefix, limit) ?? { success: false, entries: [] }; }
      catch { return { success: false, entries: [] }; }
    },
    listAutocompleteLocalDir: async (directory: string, foldersOnly: boolean, prefix = '', limit = 100) => {
      try { return await bindings.terminal.ListAutocompleteDirectory?.('', directory, foldersOnly, prefix, limit) ?? { success: false, entries: [] }; }
      catch { return { success: false, entries: [] }; }
    },
    chmodSftp: async (sftpID: string, path: string, mode: string) => {
      if (!bindings.sftp.Chmod) throw new Error('SFTP permissions unavailable');
      await bindings.sftp.Chmod(sftpID, path, mode);
    },
    onFilesDropped,
    setLanguage: (async (language: string) => {
      const changed = await bindings.tray?.SetLanguage?.(language);
      return changed ?? false;
    }) as unknown as NetcattyBridge["setLanguage"],
    quitApp: (async () => {
      await bindings.tray?.Quit?.();
    }) as unknown as NetcattyBridge["quitApp"],
    startStreamTransfer: transfers.startStreamTransfer,
    onGlobalSftpTransferEvent: transfers.onGlobalSftpTransferEvent,
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
    onTerminalPopupConfig: ((cb) => subscribePopupConfig(
      typeof window === 'undefined' ? '' : window.location.search,
      bindings.popup,
      config => cb(config as import("../../../domain/systemManager/types").TerminalPopupPayload),
    )) as unknown as NetcattyBridge["onTerminalPopupConfig"],
    onKeyboardInteractive,
    respondKeyboardInteractive,
    selectFile,
    selectDirectory,
    showSaveDialog,
    startPortForward,
    stopPortForward,
    stopPortForwardByRuleId,
    listPortForwards,
    getPortForwardStatus,
    getPortForwardSnapshot,
    windowMinimize,
    windowMaximize,
    windowClose,
    windowIsMaximized,
    windowIsFullscreen,
    openSettingsWindow,
    notifySettingsPainted,
    closeSettingsWindow: closeSettingsWindow as unknown as NetcattyBridge["closeSettingsWindow"],
    getAppLockSettings: async () => {
      if (!bindings.appLock?.GetSettings) throw new Error("App lock settings unavailable");
      return bindings.appLock.GetSettings();
    },
    getAppLockSystemUnlockStatus: async () => {
      if (!bindings.appLock?.GetSystemUnlockStatus) return { supported: false, available: false, enabled: false, platform: "unsupported", label: null, reason: "Native app lock unavailable" };
      return bindings.appLock.GetSystemUnlockStatus();
    },
    requestAppLockSystemUnlock: async () => {
      if (!bindings.appLock?.UnlockWithBiometrics) return { ok: false, error: "unsupported" };
      try {
        const result = await bindings.appLock.UnlockWithBiometrics();
        if (result.success) return { ok: true };
        return { ok: false, error: result.error === "disabled" || result.error === "not-locked" ? result.error : "failed" };
      } catch { return { ok: false, error: "failed" }; }
    },
    setAppLockSystemUnlockEnabled: async (input) => {
      if (!bindings.appLock?.SetSystemUnlockEnabled || !bindings.appLock.GetSettings) return { ok: false, error: "unsupported" };
      try {
        await bindings.appLock.SetSystemUnlockEnabled(input.enabled, input.currentPassword ?? "", input.autoPromptEnabled ?? false);
        return bindings.appLock.GetSettings();
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        const code = (["empty-current", "incorrect", "locked", "unsupported", "unavailable", "cancelled"] as const).find(value => message.includes(value));
        return { ok: false, error: code ?? "failed" };
      }
    },
    getAppLockRuntimeState: (() =>
      bindings.appLock?.GetRuntimeState()) as unknown as NetcattyBridge["getAppLockRuntimeState"],
    reportAppLockActivity: (() =>
      bindings.appLock?.ReportActivity?.()) as unknown as NetcattyBridge["reportAppLockActivity"],
    pluginV2: {
      list: async () => {
        if (!bindings.plugins?.List) throw new Error('Plugin service is not available');
        return bindings.plugins.List();
      },
      uiSchema: async (id: string) => {
        if (!bindings.plugins?.UISchema) throw new Error('Plugin UI is not available');
        return bindings.plugins.UISchema(id);
      },
      settings: async (id: string) => {
        if (!bindings.plugins?.Settings) throw new Error('Plugin settings are not available');
        return bindings.plugins.Settings(id);
      },
      setSetting: async (id: string, key: string, valueJSON: string) => {
        if (!bindings.plugins?.SetSetting) throw new Error('Plugin settings are not available');
        await bindings.plugins.SetSetting(id, key, valueJSON);
      },
      grantPermission: async (id: string, kind: string, resource: string, mode: string, lifetime: string) => {
        if (!bindings.plugins?.GrantPermission) throw new Error('Plugin permissions are not available');
        await bindings.plugins.GrantPermission(id, kind, resource, mode, lifetime);
      },
    },
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
        if (!bindings.appLock?.Enable || !bindings.appLock.GetSettings) throw new Error("App lock unavailable");
        await bindings.appLock.Enable(input.nextPassword);
        return bindings.appLock.GetSettings();
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
    pauseTransfer: transfers.pauseTransfer,
    resumeTransfer: transfers.resumeTransfer,
    cancelTransfer: transfers.cancelTransfer,
    startZmodemDragDropUpload: zmodem.startZmodemDragDropUpload,
    receiveZmodem: zmodem.receiveZmodem,
    onZmodemEvent: zmodem.onZmodemEvent,
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
    getTempDirInfo: async () => { if (!bindings.filesystem?.TempInfo) missingBridgeMethod('getTempDirInfo'); return bindings.filesystem.TempInfo(); },
    getTempDirPath: async () => { if (!bindings.filesystem?.TempInfo) missingBridgeMethod('getTempDirPath'); return (await bindings.filesystem.TempInfo()).path; },
    clearTempDir: async () => { if (!bindings.filesystem?.ClearTemp) missingBridgeMethod('clearTempDir'); const result=await bindings.filesystem.ClearTemp(); return { deletedCount:result.deletedCount, failedCount:0 }; },
    startCompressedUpload: transfers.startCompressedUpload,
    pauseCompressedUpload: transfers.pauseTransfer,
    resumeCompressedUpload: transfers.resumeTransfer,
    cancelCompressedUpload: async (id: string) => { await transfers.cancelTransfer(id); return { success: true }; },
    checkCompressedUploadSupport: (async () => ({ supported: Boolean((bindings.transfer as TransferBindings | undefined)?.StartCompressed), localTar: false, remoteTar: false })) as unknown as NetcattyBridge["checkCompressedUploadSupport"],
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
        proxy: options.proxy,
        jumpHosts: options.jumpHosts,
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
      rememberSession(options.sessionId, sessionID);
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
        proxy: options.proxy,
        jumpHosts: options.jumpHosts,
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
      rememberSession(options.sessionId, sessionID);
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
    files: portWith("files", { writeClipboardText, readClipboardText, readClipboardImage, credentialsAvailable, credentialsEncrypt, credentialsDecrypt }),
    script: portWith("script", {
      scriptRecordingStart,
      scriptRecordingStop,
      scriptRecordingAppendStep,
      scriptRun,
      scriptStop,
      scriptPause,
      scriptResume,
      scriptGetRuns,
      onScriptRunsUpdated,
    }),
    terminal: portWith("terminal", {
      getDefaultShell,
      discoverShells,
      validatePath,
      getServerStats: monitoring.getServerStats,
      getSessionPwd,
      getSessionRemoteInfo,
      startSSHSession,
      testProxy,
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
    openSftpForSession,
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
    sync: portWith("sync", {
      // Cloud OAuth surface (camelCase) mapped from the generated sync
      // bindings, so cloudSyncBridge.get() and the adapters reach the Go
      // OAuth/device-flow/file operations under Wails.
      ...cloudOAuth,
      openProviderConsole,
      cloudSyncSetSessionPassword,
      cloudSyncGetSessionPassword,
      cloudSyncClearSessionPassword,
      cloudSyncResetEverything,
      getVaultBackupCapabilities,
      createVaultBackup,
      listVaultBackups,
      readVaultBackup,
      trimVaultBackups,
      openVaultBackupDir,
      cloudSyncWebdavInitialize: (async (config: unknown) => {
        if (!bindings.sync?.CloudSyncWebdavInitialize) missingBridgeMethod("cloudSyncWebdavInitialize");
        return bindings.sync.CloudSyncWebdavInitialize(config);
      }) as unknown as NetcattyBridge["cloudSyncWebdavInitialize"],
      cloudSyncWebdavUpload: (async (config: unknown, syncedFile: unknown) => {
        if (!bindings.sync?.CloudSyncWebdavUpload) missingBridgeMethod("cloudSyncWebdavUpload");
        return bindings.sync.CloudSyncWebdavUpload(config, syncedFile);
      }) as unknown as NetcattyBridge["cloudSyncWebdavUpload"],
      cloudSyncWebdavDownload: (async (config: unknown) => {
        if (!bindings.sync?.CloudSyncWebdavDownload) missingBridgeMethod("cloudSyncWebdavDownload");
        return bindings.sync.CloudSyncWebdavDownload(config);
      }) as unknown as NetcattyBridge["cloudSyncWebdavDownload"],
      cloudSyncWebdavDelete: (async (config: unknown) => {
        if (!bindings.sync?.CloudSyncWebdavDelete) missingBridgeMethod("cloudSyncWebdavDelete");
        return bindings.sync.CloudSyncWebdavDelete(config);
      }) as unknown as NetcattyBridge["cloudSyncWebdavDelete"],
      cloudSyncS3Initialize: (async (config: unknown) => {
        if (!bindings.sync?.CloudSyncS3Initialize) missingBridgeMethod("cloudSyncS3Initialize");
        return bindings.sync.CloudSyncS3Initialize(config);
      }) as unknown as NetcattyBridge["cloudSyncS3Initialize"],
      cloudSyncS3Upload: (async (config: unknown, syncedFile: unknown) => {
        if (!bindings.sync?.CloudSyncS3Upload) missingBridgeMethod("cloudSyncS3Upload");
        return bindings.sync.CloudSyncS3Upload(config, syncedFile);
      }) as unknown as NetcattyBridge["cloudSyncS3Upload"],
      cloudSyncS3Download: (async (config: unknown) => {
        if (!bindings.sync?.CloudSyncS3Download) missingBridgeMethod("cloudSyncS3Download");
        return bindings.sync.CloudSyncS3Download(config);
      }) as unknown as NetcattyBridge["cloudSyncS3Download"],
      cloudSyncS3Delete: (async (config: unknown) => {
        if (!bindings.sync?.CloudSyncS3Delete) missingBridgeMethod("cloudSyncS3Delete");
        return bindings.sync.CloudSyncS3Delete(config);
      }) as unknown as NetcattyBridge["cloudSyncS3Delete"],
    }),
    system: portWith("system", monitoring),
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
  configureProfileBindings(profileBindings);
  setActiveRuntimeClient(createWailsRuntimeClient());
  return true;
}
