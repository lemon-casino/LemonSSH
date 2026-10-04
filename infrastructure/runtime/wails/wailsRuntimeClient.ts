// Wails RuntimeClient adapter. This module is the only frontend file
// allowed to import the Wails runtime and the generated bindings (enforced by
// ESLint). Missing Go capabilities fail closed instead of returning partial
// results.

import { Clipboard, Dialogs, Events, Window as wailsWindow } from "@wailsio/runtime";
import * as lemonsshService from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/lemonsshservice";
import * as agentServiceBinding from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/agentservice";
import * as agentCLIServiceBinding from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/agentcliservice";
import * as externalAgentServiceBinding from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/externalagentservice";
import * as userSkillsServiceBinding from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/userskillsservice";
import type {
  EventPage as AgentEventPage,
  PrepareTurnRequest as AgentPrepareTurnRequest,
  PreparedTurn as AgentPreparedTurn,
  TurnCommand as AgentTurnCommand,
  TurnSnapshot as AgentTurnSnapshot,
} from "./bindings/github.com/lemon-casino/lemonssh/internal/app/contracts/models";
import * as terminalService from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/terminalservice";
import * as knownHostsServiceBinding from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/knownhostsservice";
import * as sftpService from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/sftpservice";
import * as settingsWindowService from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/settingswindowservice";
import * as forwardService from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/forwardservice";
import * as appLockService from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/applockservice";
import * as credentialService from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/credentialservice";
import * as pluginService from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/pluginservice";
import * as deepLinkService from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/deeplinkservice";
import * as filesystemService from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/filesystemservice";
import * as transferService from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/transferservice";
import * as popupWindowService from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/popupwindowservice";
import * as sessionWindowServiceBinding from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/sessionwindowservice";
import * as shortcutService from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/shortcutservice";
import * as scriptService from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/scriptservice";
import * as diagnosticLogService from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/diagnosticlogservice";
import * as sessionLogServiceBinding from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/sessionlogservice";
import * as httpNetworkProxyServiceBinding from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/httpnetworkproxyservice";
import * as syncServiceBinding from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/syncservice";
import * as providerFetchService from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/providerfetchservice";
import * as trayService from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/trayservice";
import * as trayPanelWindowService from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/traypanelwindowservice";
import * as windowLifecycleService from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/windowlifecycleservice";
import * as lemonsshCoreService from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/lemonsshservice";
import * as updateServiceBinding from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/updateservice";
import {
  buildTerminalSocketUrl,
  bytesToBase64,
  entryToRemoteFile,
  pickSSHConnectArgs,
  callerWindowName,
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
import type { SftpFilenameEncoding } from "../../../domain/models/sftp";
import { createDataPlaneDecoder, openDataPlaneSession } from "./dataPlaneSession";
import type { DataPlaneSessionHandle } from "./dataPlaneSession";
import { createLocalShellBridge, type NativeLocalShellBindings } from './localShellBridge';
import { createCloudOAuthFacade, type CloudOAuthBindings } from '../../services/cloudSync/cloudSyncFacade';
import { createMonitoringBridge, type MonitoringBindings } from './monitoringBridge';
import { readLocalTree } from "./localTree";
import { createTransferBridge, type TransferBindings } from "./transferBridge";
import { createNativeFileActions, type NativeFileBindings } from "./nativeFileActions";
import * as profileBindings from "./bindings/github.com/lemon-casino/lemonssh/cmd/lemonssh/profileservice";
import { createZmodemBridge } from './zmodemBridge';
import { subscribePopupConfig, type LeaseParams } from './popupConfigSubscription';
import { configureProfileBindings } from "../profile/profileClient";
import { createAgentToolBridge, type NativeAgentToolBindings } from './agentToolBridge';
import { createAgentCliBridge, type NativeAgentCLIBindings } from './agentCliBridge';
import { createProviderBridge, type NativeProviderBindings } from './providerBridge';
import { createExternalAgentBridge } from './externalAgentBridge';
import { createUserSkillsBridge, type NativeUserSkillsBindings } from './userSkillsBridge';
import { createPluginBridge, type NativePluginBindings } from './pluginBridge';

export function isWailsRuntime(): boolean {
  return typeof window !== "undefined" && "_wails" in window;
}

/**
 * Builds a port whose implemented methods are backed by generated bindings.
 * Missing methods fail closed so unsupported calls cannot appear successful.
 */
function portWith<T extends object>(portName: string, implemented: object): T {
  return new Proxy(implemented, {
    get(target, property, receiver) {
      if (property in target) return Reflect.get(target, property, receiver);
      if (property === "__wails__") return undefined;
      throw new Error(
        `LemonSSH.${portName}.${String(property)} is unavailable in the Wails runtime`,
      );
    },
  }) as T;
}

function missingBridgeMethod(property: string | symbol): never {
  throw new Error(
    `LemonSSH bridge method ${String(property)} is not available under the Wails runtime yet`,
  );
}

// Wails event names broadcast by the Go services. These must stay in sync
// with the constants in cmd/lemonssh/appLockService.go,
// cmd/lemonssh/vaultBackupService.go and internal/app/updateuse/service.go.
const appLockRuntimeStateChangedEvent = "app-lock:runtime-state-changed";
const appLockSettingsChangedEvent = "app-lock:settings-changed";
const appLockReopenEvent = "app-lock:reopen";
const vaultBackupsChangedEvent = "vault-backups:changed";
const updateAvailableEvent = "update:available";
const updateNotAvailableEvent = "update:not-available";
const updateDownloadProgressEvent = "update:download-progress";
const updateDownloadedEvent = "update:downloaded";
const updateErrorEvent = "update:error";
// FileWatchService (cmd/lemonssh/fileWatchService.go) — external-editor
// auto-sync events. Keep in sync with the constants there.
const fileWatchSyncedEvent = "lemonssh:filewatch:synced";
const fileWatchErrorEvent = "lemonssh:filewatch:error";
const fileWatchStoppedEvent = "lemonssh:filewatch:stopped";
// WindowLifecycleService (cmd/lemonssh/windowLifecycleService.go) — main-window
// close semantics (quit guard / close-to-tray) and the focus-recovery events.
// Keep in sync with the constants there.
const windowShownEvent = "window:shown";
const windowWillHideEvent = "window:will-hide";
const windowFocusRequestedEvent = "window:focus-requested";
const windowCheckDirtyEditorsEvent = "window:check-dirty-editors";
// ForwardService (cmd/lemonssh/forwardService.go) — ordered tunnel-table
// events for the renderer's port-forward runtime subscription. Keep in sync
// with the constant there.
const portForwardRuntimeEvent = "lemonssh:port-forward:runtime";
// TrayService (cmd/lemonssh/trayService.go) — tray menu / tray panel actions.
// Main-window-directed events carry the session/host/rule id payload; the
// panel events target the #/tray window. Keep in sync with the constants
// there.
const trayFocusSessionEvent = "tray:focus-session";
const trayToggleForwardEvent = "tray:toggle-port-forward";
const trayPanelJumpSessionEvent = "tray:panel:jump-to-session";
const trayPanelConnectHostEvent = "tray:panel:connect-to-host";
const trayPanelCloseSessionEvent = "tray:panel:close-session";
const trayPanelMenuDataEvent = "tray:panel:menu-data";
const trayPanelRefreshEvent = "tray:panel:refresh";
const trayPanelCloseRequestEvent = "tray:panel:close-request";
// SessionWindowService (cmd/lemonssh/sessionWindowService.go) — peer session
// windows (#/session-window) carry their identity in these URL parameters.
// Keep in sync with sessionWindowURL there and callerWindowName in
// terminalRoute.ts.
const SESSION_WINDOW_LEASE_PARAMS: LeaseParams = { id: 'sessionWindowId', token: 'sessionWindowToken' };

/** Listener + payload extracted from the bridge's onTrayPanelMenuData contract. */
type TrayPanelMenuDataListener = NonNullable<LemonSSHBridge["onTrayPanelMenuData"]>;
type TrayPanelMenuDataPayload = Parameters<Parameters<TrayPanelMenuDataListener>[0]>[0];

/** Raw renderer-pushed tray menu payload (LemonSSHBridge.updateTrayMenuData). */
interface NativeTrayMenuData {
  sessions?: Array<{ id?: unknown; label?: unknown; hostLabel?: unknown; status?: unknown; workspaceId?: unknown; workspaceTitle?: unknown }>;
  hosts?: Array<{ id?: unknown; label?: unknown; hostname?: unknown; group?: unknown; pinned?: unknown; lastConnectedAt?: unknown; protocol?: unknown }>;
  portForwardRules?: Array<{ id?: unknown; label?: unknown; type?: unknown; localPort?: unknown; remoteHost?: unknown; remotePort?: unknown; status?: unknown }>;
}

/** Normalizes the Go-pushed tray panel snapshot onto the bridge contract. */
function normalizeTrayPanelMenuData(payload: unknown): TrayPanelMenuDataPayload {
  const data = (payload ?? {}) as NativeTrayMenuData;
  const text = (value: unknown): string => (typeof value === "string" ? value : "");
  return {
    sessions: (data.sessions ?? []).map((session) => ({
      id: text(session.id),
      label: text(session.label),
      hostLabel: text(session.hostLabel),
      status: (text(session.status) || "disconnected") as "connecting" | "connected" | "disconnected",
      workspaceId: text(session.workspaceId) || undefined,
      workspaceTitle: text(session.workspaceTitle) || undefined,
    })),
    hosts: (data.hosts ?? []).map((host) => ({
      id: text(host.id),
      label: text(host.label) || undefined,
      hostname: text(host.hostname) || undefined,
      group: text(host.group) || undefined,
      pinned: host.pinned === true,
      lastConnectedAt: typeof host.lastConnectedAt === "number" ? host.lastConnectedAt : undefined,
      protocol: text(host.protocol) || undefined,
    })),
    portForwardRules: (data.portForwardRules ?? []).map((rule) => ({
      id: text(rule.id),
      label: text(rule.label) || undefined,
      type: (text(rule.type) || "local") as "local" | "remote" | "dynamic",
      localPort: typeof rule.localPort === "number" ? rule.localPort : 0,
      remoteHost: text(rule.remoteHost) || undefined,
      remotePort: typeof rule.remotePort === "number" ? rule.remotePort : undefined,
      status: (text(rule.status) || "inactive") as "inactive" | "connecting" | "active" | "error",
    })),
  };
}

function base64ToArrayBuffer(value: string): ArrayBuffer {
  const binary = typeof atob === "function" ? atob(value) : Buffer.from(value, "base64").toString("binary");
  const bytes = Uint8Array.from(binary, character => character.charCodeAt(0));
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
}

function localStatToResult(stat: { path: string; isDir: boolean; isSymlink?: boolean; size: number; mode: string; modTime: string }): SftpStatResult {
  const segments = stat.path.split(/[\\/]/);
  return {
    name: segments[segments.length - 1] || stat.path,
    type: stat.isSymlink ? "symlink" : stat.isDir ? "directory" : "file",
    size: stat.size,
    lastModified: Date.parse(stat.modTime) || 0,
    permissions: stat.mode.length >= 9 ? stat.mode.slice(-9) : undefined,
  };
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
  provider?: NativeProviderBindings;
  terminal: NativeLocalShellBindings & MonitoringBindings & {
    ListAutocompleteDirectory?: (sessionID: string, directory: string, foldersOnly: boolean, prefix: string, limit: number) => Promise<{ success: boolean; entries: Array<{ name: string; type: 'file' | 'directory' | 'symlink' }>; error?: string }>;
    GetSessionPwd?: (sessionID: string, options: { allowHomeFallback: boolean; allowLoginShellFallback: boolean; timeoutMs: number }) => Promise<{ success: boolean; cwd?: string; error?: string }>;
    GetSessionRemoteInfo?: (sessionID: string) => Promise<{ success: boolean; remoteSshVersion?: string; error?: string }>;
    GetSessionDistroInfo?: (sessionID: string) => Promise<{ success: boolean; stdout?: string; stderr?: string; error?: string }>;
    ReadRemoteHistory?: (sessionID: string, limit: number) => Promise<{ success: boolean; pending?: boolean; error?: string; shell?: string; bash?: string; zsh?: string; fish?: string }>;
    PtyGetChildProcesses?: (sessionID: string) => Promise<Array<{ pid: number; command: string }>>;
    AcquireSessionFlowPauseLease?: (sessionID: string) => Promise<{ success: boolean; leaseId?: string; authorization?: string; error?: string }>;
    WaitSessionFlowPauseLease?: (sessionID: string, leaseId: string) => Promise<{ success: boolean; error?: string }>;
    ReleaseSessionFlowPauseLease?: (sessionID: string, leaseId: string, options?: { keepPaused?: boolean }) => Promise<{ success: boolean; error?: string }>;
    SetSessionFlowPaused?: (sessionID: string, paused: boolean) => Promise<void>;
    SetSessionFlowPausedAndWait?: (sessionID: string, paused: boolean) => Promise<{ success: boolean; error?: string }>;
    RequestSessionSnapshot?: (sessionID: string, authorization: string) => Promise<{
      success: boolean;
      snapshot?: string;
      kittyKeyboardModeState?: LemonSSHKittyKeyboardModeState;
      kittyKeyboardProtocolEnabled?: boolean;
      passwordPromptActive?: boolean;
      cwd?: string | null;
      title?: string | null;
      error?: string;
    }>;
    RespondSessionSnapshot?: (
      requestId: string,
      snapshot: string,
      kittyState: LemonSSHKittyKeyboardModeState | null,
      kittyEnabled: boolean | null,
      passwordPromptActive: boolean | null,
      cwd: string | null,
      title: string | null,
    ) => Promise<unknown>;
    ApplySessionSnapshot?: (sessionID: string, snapshot: string, context: unknown, authorization: string) => Promise<{ success: boolean; error?: string }>;
    RespondApplySnapshot?: (requestId: string, accepted: boolean) => Promise<unknown>;
    MarkAttachPopupClosePrepared?: (sessionID: string, authorization: string) => Promise<{ success: boolean; error?: string }>;
    RebindSessionOutput?: (sessionID: string, authorization: string) => Promise<{
      success: boolean;
      authorization?: string;
      route?: { sessionID: string; generation: number; dataToken: string; urgentToken: string; windowBytes: number };
      error?: string;
    }>;
    RestoreSessionOutput?: (sessionID: string, authorization: string) => Promise<{ success: boolean; error?: string }>;
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
    RespondPassphrase?: (requestID: string, passphrase: string, cancelled: boolean) => Promise<unknown>;
    RespondHostKeyVerification?: (requestID: string, accept: boolean, addToKnownHosts: boolean) => Promise<unknown>;
    ExecCommand?: (request: unknown) => Promise<{ stdout: string; stderr: string; code: number | null }>;
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
    GenerateKeyPair?: (options: { type: string; bits?: number; comment?: string }) => Promise<{ success: boolean; privateKey?: string; publicKey?: string; error?: string }>;
    CheckSshAgent?: (options: { identityAgent?: string; agentForwarding?: boolean; hostname?: string; port?: number; username?: string }) => Promise<{ running: boolean; startupType?: string | null; error?: string | null }>;
    GetDefaultKeys?: () => Promise<Array<{ name: string; path: string }>>;
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
    RestartHelper?: (sessionID: string) => Promise<LemonSSHHelperSessionState>;
    Write: (...args: unknown[]) => unknown;
    Resize: (...args: unknown[]) => unknown;
    Signal: (...args: unknown[]) => unknown;
    GetExitStatus?: (sessionID: string) => Promise<SessionExitEvent | null>;
    Close: (sessionID: string) => Promise<unknown>;
    Bootstrap: (sessionID: string) => Promise<WailsRouteBootstrap>;
    Reconnect?: (sessionID: string) => Promise<WailsRouteBootstrap>;
    ListenAddr: () => Promise<string> | string;
    SetSessionEncoding?: (sessionID: string, encoding: string) => Promise<{ ok: boolean; encoding: string }>;
    GetSessionEncoding?: (sessionID: string) => Promise<string> | string;
  };
  sftp: {
    OpenForTerminal?: (sessionId: string) => Promise<string>;
    Open: (request: unknown) => Promise<string>;
    Download?: (sftpID: string, remotePath: string, localPath: string, encoding?: string) => Promise<number>;
    Upload?: (sftpID: string, localPath: string, remotePath: string, encoding?: string) => Promise<number>;
    List: (sftpID: string, path: string, encoding?: string) => Promise<WailsSftpEntry[]>;
    Mkdir: (sftpID: string, path: string, encoding?: string) => Promise<unknown>;
    Remove: (sftpID: string, path: string, encoding?: string) => Promise<unknown>;
    Rename: (sftpID: string, oldPath: string, newPath: string, encoding?: string) => Promise<unknown>;
    Stat: (sftpID: string, path: string, encoding?: string) => Promise<WailsSftpFileInfo>;
    Lstat?: (sftpID: string, path: string, encoding?: string) => Promise<WailsSftpFileInfo & { isSymlink?: boolean }>;
    RealPath?: (sftpID: string, path: string, encoding?: string) => Promise<string>;
    Close: (sftpID: string) => Promise<unknown>;
    RetainTransfer?: (sftpID: string, leaseID: string) => Promise<unknown>;
    ReleaseTransfer?: (sftpID: string, leaseID: string) => Promise<unknown>;
    CopyDirectory?: (sftpID: string, sourcePath: string, targetPath: string, encoding?: string) => Promise<unknown>;
    Chmod?: (sftpID: string, path: string, mode: string, encoding?: string) => Promise<unknown>;
    Read?: (sftpID: string, path: string, encoding?: string) => Promise<string>;
    ReadBinary?: (sftpID: string, path: string, encoding?: string) => Promise<string>;
    WriteText?: (sftpID: string, path: string, content: string, encoding?: string) => Promise<unknown>;
    WriteBinary?: (sftpID: string, path: string, content: string, encoding?: string) => Promise<unknown>;
    HomeDir?: (sftpID: string) => Promise<string>;
    ExtractArchive?: (sftpID: string, remotePath: string, encoding?: string) => Promise<number>;
    UploadCompressedFolder?: (sftpID: string, localFolder: string, remoteZipPath: string, encoding?: string) => Promise<number>;
  };
  window?: {
    Minimise: () => Promise<void>;
    ToggleMaximise: () => Promise<void>;
    Hide?: () => Promise<void>;
    Close: () => Promise<void>;
    IsMaximised: () => Promise<boolean>;
    IsFullscreen: () => Promise<boolean>;
    Focus?: () => Promise<void>;
  };
  settings?: {
    ShowSystemNotification?: NonNullable<LemonSSHBridge["showSystemNotification"]>;
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
    Reset?: (password: string) => Promise<unknown>;
    UnlockWithBiometrics?: () => Promise<{ success: boolean; error?: string }>;
    GetSettings?: () => ReturnType<NonNullable<LemonSSHBridge["getAppLockSettings"]>>;
    GetSystemUnlockStatus?: () => ReturnType<NonNullable<LemonSSHBridge["getAppLockSystemUnlockStatus"]>>;
    SetSystemUnlockEnabled?: (enabled: boolean, password: string, autoPrompt: boolean) => Promise<{ systemUnlockEnabled: boolean; systemUnlockAutoPromptEnabled: boolean }>;
    SetRuntimeLocked?: (reason: string) => Promise<unknown>;
    SetTimeoutMinutes?: (minutes: number) => Promise<unknown>;
  };
  plugins?: NativePluginBindings & {
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
    SetSshDeepLinkEnabled?: (enabled: boolean) => Promise<{ success: boolean; enabled: boolean; supported?: boolean; error?: string }>;
    GetSshDeepLinkEnabled?: () => Promise<boolean>;
    SetJmsDeepLinkEnabled?: (enabled: boolean) => Promise<{ success: boolean; enabled: boolean; supported?: boolean; error?: string }>;
    GetJmsDeepLinkEnabled?: () => Promise<boolean>;
    SetExplorerContextMenuEnabled?: (enabled: boolean) => Promise<{ success: boolean; enabled: boolean; supported?: boolean; error?: string }>;
    GetExplorerContextMenuEnabled?: () => Promise<{ success: boolean; enabled: boolean; supported?: boolean; error?: string }>;
  };
  filesystem?: NativeFileBindings & {
    ReadClipboardImage?: () => Promise<{ path: string; name: string; mediaType: string; size?: number } | null>;
    TempInfo?: () => Promise<{ path: string; fileCount: number; totalSize: number }>;
    TempFilePath?: (name: string) => Promise<string>;
    ClearTemp?: () => Promise<{ success: boolean; deletedCount: number }>;
    // openPath accepts files AND directories (SFTP transfer reveal); the
    // OpenWithSystemDefault binding rejects non-regular files.
    OpenPath?: (path: string) => Promise<void>;
    // External-editor auto-sync (FileWatchService).
    StartFileWatch?: (localPath: string, remotePath: string, sftpId: string, encoding: string) => Promise<{ watchId: string; reused?: boolean }>;
    StopFileWatch?: (watchId: string, cleanupTempFile: boolean) => Promise<{ success: boolean }>;
    ListFileWatches?: () => Promise<Array<{ watchId: string; localPath: string; remotePath: string; sftpId: string }>>;
    RegisterTempFile?: (sftpId: string, localPath: string) => Promise<{ success: boolean }>;
    UnregisterTempFile?: (sftpId: string, localPath: string) => Promise<{ success: boolean; retained?: boolean; wasTracked?: boolean }>;
    HomeDir?: () => Promise<string>;
    ListDir?: (path: string) => Promise<RemoteFile[]>;
    ExtractArchive?: (archivePath: string, destinationRoot: string) => Promise<number>;
    StatPath?: (path: string) => Promise<{ name: string; isDir: boolean; size: number }>;
    ReadFile?: (path: string, maxBytes: number) => Promise<string>;
    WriteFile?: (path: string, data: string) => Promise<unknown>;
    DeletePath?: (path: string, expectedType: string) => Promise<unknown>;
    RenamePath?: (oldPath: string, newPath: string) => Promise<unknown>;
    Mkdir?: (path: string) => Promise<unknown>;
    Stat?: (path: string) => Promise<{ path: string; isDir: boolean; isSymlink?: boolean; size: number; mode: string; modTime: string }>;
    Lstat?: (path: string) => Promise<{ path: string; isDir: boolean; isSymlink?: boolean; size: number; mode: string; modTime: string }>;
    ListDrives?: () => Promise<string[]>;
    SystemInfo?: () => Promise<{ username: string; hostname: string }>;
    StageFromLocalPath?: (path: string) => Promise<[string, number] | { stagedPath: string; name: string; size: number }>;
    StageBegin?: (fileName: string) => Promise<string>;
    StageAppend?: (tempPath: string, offset: number, data: string) => Promise<unknown>;
    StageDiscard?: (tempPath: string) => Promise<unknown>;
    // Renderer tool-output spill persistence over the managed temp service
    // (consumed by cattyTurnDriver's toolOutputTemp bridge).
    ToolOutputPersistenceStatus?: () => Promise<{ durable: boolean; reason?: string }>;
    WriteToolOutputTemp?: (record: unknown, content: string) => Promise<{ ok: boolean; path?: string; error?: string }>;
    RestoreToolOutputTemp?: (handleId: string, chatSessionId: string) => Promise<{ path: string; record: unknown } | null>;
    ReadToolOutputTemp?: (filePath: string, request?: unknown) => Promise<unknown | null>;
    DeleteToolOutputTemp?: (filePath: string) => Promise<{ ok: boolean }>;
    DeleteChatToolOutputsTemp?: (chatSessionId: string) => Promise<{ deletedCount: number }>;
    DeleteTerminalToolOutputsTemp?: (chatSessionId: string, terminalSessionId: string) => Promise<{ deletedCount: number }>;
    DeleteTerminalToolOutputsEverywhereTemp?: (terminalSessionId: string) => Promise<{ deletedCount: number }>;
  };
  sync?: {
    CloudSyncSetSessionPassword?: (password: string) => Promise<boolean>;    CloudSyncGetSessionPassword?: () => Promise<{ password?: string; found?: boolean }>;
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
  knownHosts?: {
    ReadKnownHosts?: () => Promise<string>;
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
  sessionWindow?: {
    Open?: (payload: unknown) => Promise<{ success: boolean; windowId?: string; error?: string }>;
    GetConfig?: (windowId: string, token: string) => Promise<unknown>;
    Heartbeat?: (windowId: string, token: string) => Promise<unknown>;
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
    GetCrashLogs?: () => Promise<Array<{ fileName: string; date: string; size: number; entryCount: number }>>;
    ReadCrashLog?: (fileName: string) => Promise<Array<{ timestamp: string; source: string; message: string; stack?: string }>>;
    ClearCrashLogs?: () => Promise<{ deletedCount: number }>;
    OpenCrashLogsDir?: () => Promise<{ success: boolean; error?: string }>;
    GetSshDebugLogInfo?: () => Promise<{ enabled: boolean; path: string; exists: boolean; size: number }>;
    OpenSshDebugLogDir?: () => Promise<{ success: boolean; error?: string }>;
    SetSshDebugLogEnabled?: (enabled: boolean) => Promise<{ enabled: boolean; path: string; exists: boolean; size: number }>;
  };
  httpNetworkProxy?: {
    Set?: (settings: { mode: string; url: string; bypass: string }) => Promise<{ success?: boolean; settings: { mode: string; url: string; bypass: string }; error?: string }>;
    Get?: () => Promise<{ settings: { mode: string; url: string; bypass: string } }>;
  };
  sessionLog?: {
    Start?: (sessionID: string, filePath: string, initialLine: string) => Promise<{ success: boolean; started?: boolean; isLogging?: boolean; filePath?: string; error?: string }>;
    Stop?: (sessionID: string) => Promise<{ success: boolean; stopped?: boolean; filePath?: string; error?: string }>;
    Status?: (sessionID: string) => Promise<{ success: boolean; isLogging?: boolean; filePath?: string; error?: string }>;
    OpenDirectory?: (directory: string) => Promise<{ success: boolean; error?: string }>;
    ClearDirectory?: (directory: string) => Promise<{ success: boolean; deletedCount: number; failedCount: number; error?: string }>;
  };
  tray?: {
    SetLanguage?: (language: string) => Promise<boolean>;
    Quit?: () => Promise<void>;
    UpdateTrayMenuData?: (data: unknown) => Promise<{ success: boolean }>;
    CurrentTrayMenuData?: () => Promise<unknown>;
    FocusSession?: (sessionID: string) => Promise<{ success: boolean }>;
    JumpToSessionFromPanel?: (sessionID: string) => Promise<{ success: boolean }>;
    ConnectToHost?: (hostID: string) => Promise<{ success: boolean }>;
    CloseSessionFromPanel?: (sessionID: string) => Promise<{ success: boolean }>;
    TogglePortForward?: (ruleID: string) => Promise<{ success: boolean; running: boolean }>;
    OpenMainWindow?: () => Promise<{ success: boolean }>;
  };
  trayPanel?: {
    Open?: () => Promise<boolean>;
    PaintReady?: () => Promise<boolean>;
    Hide?: () => Promise<boolean>;
  };
  windowLifecycle?: {
    SetCloseToTray?: (enabled: boolean) => Promise<{ success: boolean; enabled: boolean }>;
    IsCloseToTray?: () => Promise<{ success: boolean; enabled: boolean }>;
    SetWindowOpacity?: (opacity: number) => Promise<boolean>;
    ReportDirtyEditorsResult?: (hasDirty: boolean) => Promise<void>;
    RequestClose?: (window: string) => Promise<{ success: boolean; guarded?: boolean }>;
  };
  lemonssh?: {
    Health?: () => Promise<unknown>;
    Version?: () => Promise<{ name: string; version: string; goos: string; goarch: string; goVersion: string }>;
    ResolveWindowRole?: (role: string) => Promise<unknown>;
  };
  update?: {
    CheckForUpdate?: () => Promise<{
      available: boolean;
      supported?: boolean;
      checking?: boolean;
      ready?: boolean;
      downloading?: boolean;
      version?: string;
      releaseNotes?: string;
      releaseDate?: string;
      error?: string;
    }>;
    DownloadUpdate?: () => Promise<{ success: boolean; error?: string }>;
    InstallUpdate?: () => Promise<void>;
    GetUpdateStatus?: () => Promise<{ status: string; percent: number; error: string; version: string; isChecking: boolean }>;
    GetAutoUpdate?: () => Promise<{ enabled: boolean }>;
    SetAutoUpdate?: (enabled: boolean) => Promise<{ success: boolean }>;
  };
  clipboard?: { SetText: (text: string) => Promise<boolean>; Text: () => Promise<string> };
  openDataPlane?: typeof openDataPlaneSession;
  agentservice?: NativeAgentToolBindings & {
    AgentPrepare: (request: AgentPrepareTurnRequest) => Promise<AgentPreparedTurn>;
    AgentStart: (command: AgentTurnCommand) => Promise<void>;
    AgentStop: (turnID: string, reason: string) => Promise<AgentTurnSnapshot>;
    AgentReadEvents: (turnID: string, afterSequence: string, limit: number) => Promise<AgentEventPage>;
    AgentSnapshot: (turnID: string) => Promise<AgentTurnSnapshot>;
    AgentStatus: () => Promise<{ goRuntimeReady: boolean; fixtureDriver: boolean }>;
    // Go interaction router (W13). Unknown/already-resolved ids fail typed.
    AgentPendingInteractions?: () => Promise<Array<{
      interactionId: string;
      capabilityId: string;
      summary?: Record<string, unknown>;
      deadlineMs?: number;
    }>>;
    AgentRespondInteraction?: (interactionID: string, approved: boolean) => Promise<void>;
  };
  agentcli?: NativeAgentCLIBindings;
  externalAgent?: Parameters<typeof createExternalAgentBridge>[0];
  userSkills?: NativeUserSkillsBindings;
}

  const defaultBindings: WailsBindingDeps = {
    provider: providerFetchService as unknown as NativeProviderBindings,
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
    sessionWindow: sessionWindowServiceBinding as unknown as WailsBindingDeps["sessionWindow"],
    shortcuts: shortcutService as unknown as WailsBindingDeps["shortcuts"],
    script: scriptService as unknown as WailsBindingDeps["script"],
    diagnosticLog: diagnosticLogService as unknown as WailsBindingDeps["diagnosticLog"],
    sessionLog: sessionLogServiceBinding as unknown as WailsBindingDeps["sessionLog"],
    httpNetworkProxy: httpNetworkProxyServiceBinding as unknown as WailsBindingDeps["httpNetworkProxy"],
    tray: trayService as unknown as WailsBindingDeps["tray"],
    trayPanel: trayPanelWindowService as unknown as WailsBindingDeps["trayPanel"],
    windowLifecycle: windowLifecycleService as unknown as WailsBindingDeps["windowLifecycle"],
    lemonssh: lemonsshCoreService as unknown as WailsBindingDeps["lemonssh"],
    update: updateServiceBinding as unknown as WailsBindingDeps["update"],
    sync: syncServiceBinding as unknown as WailsBindingDeps["sync"],
    knownHosts: knownHostsServiceBinding as unknown as WailsBindingDeps["knownHosts"],
    agentservice: agentServiceBinding as unknown as WailsBindingDeps["agentservice"],
    agentcli: agentCLIServiceBinding as unknown as NativeAgentCLIBindings,
    externalAgent: externalAgentServiceBinding as unknown as NonNullable<WailsBindingDeps["externalAgent"]>,
    userSkills: userSkillsServiceBinding as unknown as NativeUserSkillsBindings,
  };

type SessionDataCallback = Parameters<LemonSSHBridge["onSessionData"]>[1];
type SessionExitEvent = { sessionId: string; exitCode?: number; reason?: "exited" | "error" | "closed" | "timeout"; error?: string; intentional?: boolean };
type SessionExitCallback = (evt: SessionExitEvent) => void;
type HelperLifecycleCallback = Parameters<NonNullable<LemonSSHBridge["onHelperLifecycle"]>>[1];

/** Go turn runtime methods (W12). The Go side is the single authoritative
 *  state owner; this port only relays the W03 wire DTOs. */
export interface AgentRuntimeStatus {
  goRuntimeReady: boolean;
  fixtureDriver: boolean;
}

export interface AgentRuntimePort {
  agentPrepare(request: AgentPrepareTurnRequest): Promise<AgentPreparedTurn>;
  agentStart(command: AgentTurnCommand): Promise<void>;
  agentStop(turnID: string, reason: string): Promise<AgentTurnSnapshot>;
  agentReadEvents(turnID: string, afterSequence: string, limit: number): Promise<AgentEventPage>;
  agentSnapshot(turnID: string): Promise<AgentTurnSnapshot>;
  agentStatus(): Promise<AgentRuntimeStatus>;
}

export type WailsRuntimeClient = RuntimeClient & { agentRuntime: AgentRuntimePort };

function buildAgentRuntimePort(bindings: WailsBindingDeps): AgentRuntimePort {
  const service = bindings.agentservice;
  if (!service) {
    const unavailable = (): never => {
      throw new Error("agent runtime is unavailable in this shell");
    };
    return {
      agentPrepare: unavailable,
      agentStart: unavailable,
      agentStop: unavailable,
      agentReadEvents: unavailable,
      agentSnapshot: unavailable,
      agentStatus: async () => ({ goRuntimeReady: false, fixtureDriver: false }),
    };
  }
  return {
    agentPrepare: (request) => service.AgentPrepare(request),
    agentStart: (command) => service.AgentStart(command),
    agentStop: (turnID, reason) => service.AgentStop(turnID, reason),
    agentReadEvents: (turnID, afterSequence, limit) => service.AgentReadEvents(turnID, afterSequence, limit),
    agentSnapshot: (turnID) => service.AgentSnapshot(turnID),
    agentStatus: () => service.AgentStatus(),
  };
}

export function createWailsRuntimeClient(bindings: WailsBindingDeps = defaultBindings): WailsRuntimeClient {
  // Field-level credential storage over the Go credential provider. The
  // envelope keeps the Electron enc:v1: sentinel so renderer-side detection
  // (no double encryption, migration) behaves identically.
  const CREDENTIAL_ENC_PREFIX = "enc:v1:";
  const CREDENTIAL_PURPOSE = "cloud-sync-credentials";
  // Subscribes to a Go-side Wails event and unwraps the standard
  // { data: payload } envelope; the listener receives the bare payload.
  const subscribeNativeEvent = (eventName: string, cb: (payload: unknown) => void): () => void => {
    const eventsOn = bindings.events?.On ?? Events.On;
    if (typeof eventsOn !== "function") return () => undefined;
    return eventsOn(eventName, (event) => cb((event as { data?: unknown } | undefined)?.data ?? event));
  };
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
  const sessionAliases = new Map<string, string>();
  const nativeSessionId = (id: string) => sessionAliases.get(id) ?? id;
  // Per-session output decoders for the data plane (terminal encoding switch).
  // The holder indirection lets setSessionEncoding swap the charset without
  // tearing down the live WebSocket; frames decode with whichever decoder is
  // current when they arrive.
  const sessionDecoders = new Map<string, { current: ReturnType<typeof createDataPlaneDecoder> }>();
  const decoderHolderFor = (sessionID: string) => {
    let holder = sessionDecoders.get(sessionID);
    if (!holder) {
      holder = { current: createDataPlaneDecoder("utf-8") };
      sessionDecoders.set(sessionID, holder);
    }
    return holder;
  };
  const monitoring = createMonitoringBridge(bindings.terminal, nativeSessionId);
  // Cloud OAuth facade: maps the generated PascalCase sync bindings onto the
  // camelCase bridge surface the cloud sync adapters and UI already call.
  const cloudOAuth = createCloudOAuthFacade(bindings.sync as unknown as CloudOAuthBindings);
  const cloudSyncSetSessionPassword = (async (password: string) => {
    if (!bindings.sync?.CloudSyncSetSessionPassword) missingBridgeMethod("cloudSyncSetSessionPassword");
    return bindings.sync.CloudSyncSetSessionPassword(password);
  }) as unknown as LemonSSHBridge["cloudSyncSetSessionPassword"];
  const cloudSyncGetSessionPassword = (async () => {
    const result = await bindings.sync?.CloudSyncGetSessionPassword?.();
    return result?.password ?? null;
  }) as unknown as LemonSSHBridge["cloudSyncGetSessionPassword"];
  const cloudSyncClearSessionPassword = (async () => {
    return (await bindings.sync?.CloudSyncClearSessionPassword?.()) ?? { success: false };
  }) as unknown as LemonSSHBridge["cloudSyncClearSessionPassword"];
  const cloudSyncResetEverything = (async () => {
    if (!bindings.sync?.CloudSyncResetEverything) missingBridgeMethod("cloudSyncResetEverything");
    // Wails wraps the []string return as { removedKeys: string[] }.
    const result = await bindings.sync.CloudSyncResetEverything();
    return result?.removedKeys ?? [];
  }) as unknown as LemonSSHBridge["cloudSyncResetEverything"];
  const getVaultBackupCapabilities = (async () => {
    if (!bindings.sync?.GetVaultBackupCapabilities) missingBridgeMethod("getVaultBackupCapabilities");
    const result = await bindings.sync.GetVaultBackupCapabilities();
    return { encryptionAvailable: Boolean(result?.encryptionAvailable) };
  }) as unknown as LemonSSHBridge["getVaultBackupCapabilities"];
  const createVaultBackup = (async (payload) => {
    if (!bindings.sync?.CreateVaultBackup) missingBridgeMethod("createVaultBackup");
    return bindings.sync.CreateVaultBackup(payload);
  }) as unknown as LemonSSHBridge["createVaultBackup"];
  const listVaultBackups = (async () => {
    if (!bindings.sync?.ListVaultBackups) missingBridgeMethod("listVaultBackups");
    const result = await bindings.sync.ListVaultBackups();
    return Array.isArray(result) ? result : result?.backups ?? [];
  }) as unknown as LemonSSHBridge["listVaultBackups"];
  const readVaultBackup = (async (payload) => {
    if (!bindings.sync?.ReadVaultBackup) missingBridgeMethod("readVaultBackup");
    return bindings.sync.ReadVaultBackup(payload);
  }) as unknown as LemonSSHBridge["readVaultBackup"];
  const trimVaultBackups = (async (payload) => {
    if (!bindings.sync?.TrimVaultBackups) missingBridgeMethod("trimVaultBackups");
    return bindings.sync.TrimVaultBackups(payload);
  }) as unknown as LemonSSHBridge["trimVaultBackups"];
  const openVaultBackupDir = (async () => {
    if (!bindings.sync?.OpenVaultBackupDir) missingBridgeMethod("openVaultBackupDir");
    return bindings.sync.OpenVaultBackupDir();
  }) as unknown as LemonSSHBridge["openVaultBackupDir"];
  const openProviderConsole = (async (provider: 'github' | 'google' | 'onedrive') => {
    if (!bindings.sync?.OpenProviderConsole) missingBridgeMethod("openProviderConsole");
    await bindings.sync.OpenProviderConsole(provider);
  }) as unknown as LemonSSHBridge["openProviderConsole"];
  const rememberSession = (uiId: string | undefined, nativeId: string) => {
    if (uiId) sessionAliases.set(uiId, nativeId);
  };
  const uiSessionId = (nativeId: string) => {
    for (const [alias, native] of sessionAliases) {
      if (native === nativeId) return alias;
    }
    return nativeId;
  };
  const getSessionPwd = async (id: string, options?: Parameters<NonNullable<LemonSSHBridge["getSessionPwd"]>>[1]) => {
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

  const showSystemNotification: NonNullable<LemonSSHBridge["showSystemNotification"]> = async payload => {
    try {
      return await bindings.settings?.ShowSystemNotification?.(payload) ?? { shown: false, reason: "Native notifications unavailable" };
    } catch (error) { return { shown: false, reason: String(error) }; }
  };

  // Attach (popup observe) route handoff. Go broadcasts handoff phases; the
  // window that lost the route suspends its reconnect loop and re-arms on the
  // restore phase with the freshly rotated tokens. A window that initiated the
  // rebind/restore RPC never reacts to its own broadcast: the initiator marker
  // is set before the RPC resolves, so the async event cannot precede it.
  const routeHandoffSuspended = new Set<string>();
  const rebindInitiations = new Set<string>();
  const isRouteHandedOffError = (error: unknown): boolean =>
    error instanceof Error && /handed off to another window/i.test(error.message);
  type RouteHandoffPayload = {
    sessionId?: string;
    phase?: string;
    generation?: number;
    route?: { sessionID?: string; generation?: number; dataToken?: string; urgentToken?: string; windowBytes?: number };
  };
  // Generation of the plane this window currently holds per session, so the
  // detached handoff can tell a stale plane (previous owner) from the fresh
  // route the rebind initiator attached moments ago — event and RPC delivery
  // cannot be ordered against each other across the bridge.
  const planeGenerations = new Map<string, number>();
  let routeHandoffSubscribed = false;
  const subscribeRouteHandoff = () => {
    if (routeHandoffSubscribed) return;
    const eventsOn = bindings.events?.On ?? Events.On;
    if (typeof eventsOn !== "function") return;
    routeHandoffSubscribed = true;
    eventsOn("terminal:route-handoff", (event) => {
      const payload = ((event as { data?: unknown })?.data ?? event) as RouteHandoffPayload | null;
      if (!payload || typeof payload.sessionId !== "string" || payload.sessionId === "") return;
      const sessionID = nativeSessionId(payload.sessionId);
      if (payload.phase === "detached") {
        // The rebind initiator attaches its own route right after; only the
        // previous display owner (the home window) suspends here.
        if (rebindInitiations.delete(sessionID)) return;
        // A window already holding a plane at/after the handoff generation is
        // the new owner — never tear its fresh route down.
        const currentGeneration = planeGenerations.get(sessionID);
        if (typeof payload.generation === "number" && typeof currentGeneration === "number" && currentGeneration >= payload.generation) {
          return;
        }
        routeHandoffSuspended.add(sessionID);
        const state = routeStates.get(sessionID);
        if (state) {
          state.closed = true;
          if (state.timer) {
            clearTimeout(state.timer);
            state.timer = undefined;
          }
        }
        planeGenerations.delete(sessionID);
        planes.get(sessionID)?.dispose();
        planes.delete(sessionID);
        return;
      }
      if (payload.phase === "restored") {
        // Only the window that lost the route earlier re-arms; the closing
        // popup must not compete for the restored bootstrap.
        if (!routeHandoffSuspended.delete(sessionID)) return;
        const route = payload.route;
        if (!route || typeof route.generation !== "number" || typeof route.dataToken !== "string" || typeof route.urgentToken !== "string") {
          return;
        }
        // Re-arm the route state the detached phase closed.
        const state = routeStates.get(sessionID);
        if (state) {
          state.closed = false;
          state.retries = 0;
          if (state.timer) {
            clearTimeout(state.timer);
            state.timer = undefined;
          }
        }
        void attachDataPlane(sessionID, false, {
          SessionID: typeof route.sessionID === "string" && route.sessionID !== "" ? route.sessionID : sessionID,
          Generation: route.generation,
          DataToken: route.dataToken,
          UrgentToken: route.urgentToken,
          WindowBytes: typeof route.windowBytes === "number" && route.windowBytes > 0 ? route.windowBytes : 1048576,
        }).catch((error) => {
          console.error("Terminal route restore attach failed", error);
        });
      }
    });
  };

  async function attachDataPlane(sessionID: string, reconnect = false, providedBootstrap?: WailsRouteBootstrap): Promise<void> {
    subscribeRouteHandoff();
    let state = routeStates.get(sessionID);
    if (!state) {
      state = { version: 0, retries: 0, closed: false };
      routeStates.set(sessionID, state);
    }
    const owner = state;
    const version = ++owner.version;
    const current = () => !owner.closed && routeStates.get(sessionID) === owner && owner.version === version;
    planes.get(sessionID)?.dispose();
    let bootstrap: WailsRouteBootstrap;
    let listenAddr: string;
    if (providedBootstrap) {
      // Route handoff: Go already rotated the route and handed us its tokens;
      // rotating again would race the other display owner.
      bootstrap = providedBootstrap;
      listenAddr = await Promise.resolve(bindings.terminal.ListenAddr());
    } else {
      [bootstrap, listenAddr] = await Promise.all([
        reconnect
          ? (bindings.terminal.Reconnect ? bindings.terminal.Reconnect(sessionID) : Promise.reject(new Error('Terminal route reconnect unavailable')))
          : bindings.terminal.Bootstrap(sessionID),
        Promise.resolve(bindings.terminal.ListenAddr()),
      ]);
    }
    if (!current()) return;
    // Sessions may already pin a charset (switched before an attach, popup
    // windows): seed the decoder from the Go state before the first frame.
    // A concurrent setSessionEncoding wins — only seed when still unset.
    if (!sessionDecoders.has(sessionID) && bindings.terminal.GetSessionEncoding) {
      const encoding = await Promise.resolve(bindings.terminal.GetSessionEncoding(sessionID)).catch(() => "");
      if (current() && !sessionDecoders.has(sessionID)) {
        sessionDecoders.set(sessionID, { current: createDataPlaneDecoder(encoding) });
      }
    }
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
          emitData(sessionID, '\r\n[LemonSSH] Terminal connection could not be restored. Please reconnect.\r\n');
          emitExit(sessionID, { sessionId: sessionID, reason: "error", error: "Terminal transport reconnect exhausted" });
          return;
        }
        const delay = Math.min(1000 * 2 ** owner.retries++, 30000);
        owner.timer = setTimeout(() => {
          owner.timer = undefined;
          void attachDataPlane(sessionID, true).catch(error => {
            console.error('Terminal route reconnect failed', error);
            // An attach popup owns the route: suspend this window's reconnect
            // loop instead of spinning (Go refuses the rotation while the
            // handoff is active) — the restored handoff re-arms it.
            if (isRouteHandedOffError(error)) {
              routeHandoffSuspended.add(sessionID);
              return;
            }
            schedule();
          });
        }, delay);
      };
      schedule();
    };
    planeGenerations.set(sessionID, bootstrap.Generation);
    planes.set(sessionID, (bindings.openDataPlane ?? openDataPlaneSession)({
      listenAddr,
      bootstrap,
      // Charset-aware streaming decode (UTF-8 default, GB18030 switchable).
      decoder: { decode: (bytes) => decoderHolderFor(sessionID).current.decode(bytes) },
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

  const testProxy = (async (options: Parameters<NonNullable<LemonSSHBridge["testProxy"]>>[0]) => {
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
  }) as NonNullable<LemonSSHBridge["testProxy"]>;
  const startSSHSession = (options: Parameters<LemonSSHBridge["startSSHSession"]>[0]) => {
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
  // Terminal encoding switch (UTF-8 ↔ GB18030). Output decoding swaps the
  // data-plane decoder immediately; the Go side pins the input charset so
  // keystrokes are encoded back to the same charset. Either side failing
  // still reports through the bridge contract.
  const setSessionEncoding = async (sessionID: string, encoding: string) => {
    const native = nativeSessionId(sessionID);
    decoderHolderFor(native).current = createDataPlaneDecoder(encoding);
    if (!bindings.terminal.SetSessionEncoding) {
      return { ok: false, encoding };
    }
    try {
      return await bindings.terminal.SetSessionEncoding(native, encoding);
    } catch {
      return { ok: false, encoding };
    }
  };
  const resizeSession = (sessionID: string, cols: number, rows: number) =>
    bindings.terminal.Resize(nativeSessionId(sessionID), cols, rows) as unknown as void;
  const interruptSession = (sessionID: string) =>
    bindings.terminal.Signal(nativeSessionId(sessionID), "INT") as unknown as void;
  const closeSession = async (sessionID: string) => {
    sessionID = nativeSessionId(sessionID);
    for (const [alias, nativeId] of sessionAliases) {
      if (nativeId === sessionID) sessionAliases.delete(alias);
    }
    routeHandoffSuspended.delete(sessionID);
    planeGenerations.delete(sessionID);
    sessionDecoders.delete(sessionID);
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

  const openSftp = (options: Parameters<LemonSSHBridge["openSftp"]>[0]) => {
    const args = pickSSHConnectArgs(options);
    return bindings.sftp.Open({ ...args, sudo: options.sudo ?? false });
  };
  // The optional filename-encoding argument (auto / utf-8 / gb18030) flows to
  // the Go service, which transcodes names before they cross the JSON channel
  // — invalid UTF-8 bytes would otherwise be replaced with U+FFFD.
  const listSftp = async (sftpID: string, path: string, encoding?: SftpFilenameEncoding): Promise<RemoteFile[]> => {
    const entries = await bindings.sftp.List(sftpID, path, encoding);
    return entries.map(entryToRemoteFile);
  };
  const mkdirSftp = (sftpID: string, path: string, encoding?: SftpFilenameEncoding) =>
    bindings.sftp.Mkdir(sftpID, path, encoding) as Promise<void>;
  const deleteSftp = (sftpID: string, path: string, encoding?: SftpFilenameEncoding) =>
    bindings.sftp.Remove(sftpID, path, encoding) as Promise<void>;
  const renameSftp = (sftpID: string, oldPath: string, newPath: string, encoding?: SftpFilenameEncoding) =>
    bindings.sftp.Rename(sftpID, oldPath, newPath, encoding) as Promise<void>;
  const statSftp = async (sftpID: string, path: string, encoding?: SftpFilenameEncoding) => {
    try {
      const stat = await bindings.sftp.Stat(sftpID, path, encoding);
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
  const closeSftp = async (sftpID: string) => {
    await bindings.sftp.Close(sftpID);
    return { success: true };
  };
  const readSftp = (sftpID: string, path: string, encoding?: SftpFilenameEncoding) => bindings.sftp.Read?.(sftpID, path, encoding) as Promise<string>;
  const readSftpBinary = async (sftpID: string, path: string, encoding?: SftpFilenameEncoding) => {
    if (!bindings.sftp.ReadBinary) missingBridgeMethod("readSftpBinary");
    return base64ToArrayBuffer(await bindings.sftp.ReadBinary(sftpID, path, encoding));
  };
  const writeSftp = (sftpID: string, path: string, content: string, encoding?: SftpFilenameEncoding) => bindings.sftp.WriteText?.(sftpID, path, content, encoding) as Promise<void>;
  const writeSftpBinary = async (sftpID: string, path: string, content: ArrayBuffer, encoding?: SftpFilenameEncoding) => {
    if (!bindings.sftp.WriteBinary) missingBridgeMethod("writeSftpBinary");
    await bindings.sftp.WriteBinary(sftpID, path, bytesToBase64(new Uint8Array(content)), encoding);
  };
  const realpathSftp = async (sftpID: string, path: string, encoding?: SftpFilenameEncoding) => {
    if (!bindings.sftp.RealPath) missingBridgeMethod("realpathSftp");
    return bindings.sftp.RealPath(sftpID, path, encoding);
  };
  const lstatSftp = async (sftpID: string, path: string, encoding?: SftpFilenameEncoding) => {
    try {
      const stat = bindings.sftp.Lstat
        ? await bindings.sftp.Lstat(sftpID, path, encoding)
        : await bindings.sftp.Stat(sftpID, path, encoding);
      return { ...statToSftpStatResult(stat), type: stat.isSymlink ? "symlink" as const : stat.isDir ? "directory" as const : "file" as const };
    } catch (error) {
      if (error instanceof Error && /does not exist|no such file/i.test(error.message)) return null;
      throw error;
    }
  };
  const retainSftpTransferSession = async (sftpID: string, leaseID: string) => {
    if (!bindings.sftp.RetainTransfer) return { success: false, reason: "unsupported" };
    await bindings.sftp.RetainTransfer(sftpID, leaseID);
    return { success: true };
  };
  const releaseSftpTransferSession = async (sftpID: string, leaseID: string) => {
    if (!bindings.sftp.ReleaseTransfer) return { success: false, reason: "unsupported" };
    await bindings.sftp.ReleaseTransfer(sftpID, leaseID);
    return { success: true };
  };
  const sameHostCopyDirectory = async (sftpID: string, sourcePath: string, targetPath: string, encoding?: SftpFilenameEncoding) => {
    if (!bindings.sftp.CopyDirectory) return { success: false };
    await bindings.sftp.CopyDirectory(sftpID, sourcePath, targetPath, encoding);
    return { success: true };
  };
  const getSftpHomeDir = (sftpID: string) => bindings.sftp.HomeDir?.(sftpID) as Promise<string>;

  const windowMinimize = () => bindings.window?.Minimise();
  const windowMaximize = async () => {
    await bindings.window?.ToggleMaximise();
    return bindings.window?.IsMaximised() ?? false;
  };
  // Dispatch by caller window: the main window routes through the Go-side
  // quit guard (close-to-tray / dirty-editor confirmation); any other window
  // closes itself, keeping its own WindowClosing hook policy (settings hides).
  // Terminal popups keep their legacy hide-self contract — the window stays
  // warm for the next open, and the attach close handshake already ran in the
  // page before close(). Shells without the binding keep the legacy path too.
  const windowClose = () => {
    const hash = typeof window !== 'undefined' ? window.location.hash : '';
    const search = typeof window !== 'undefined' ? window.location.search : '';
    const requestClose = bindings.windowLifecycle?.RequestClose;
    if (!hash.startsWith('#/terminal-popup') && requestClose) {
      // Drop the routing result: windowClose's contract is fire-and-close.
      // Peer session windows resolve to their own window name (URL identity)
      // so closing one never runs the main window's quit guard.
      return requestClose(callerWindowName(hash, search)).then(() => undefined);
    }
    return bindings.window?.Hide?.() ?? bindings.window?.Close();
  };
  const windowIsMaximized = () => bindings.window?.IsMaximised() ?? Promise.resolve(false);
  const windowIsFullscreen = () => bindings.window?.IsFullscreen() ?? Promise.resolve(false);
  // Input-focus recovery (#760/#1714/#1722): refocus the current OS window so
  // keystrokes land in the app after show/hide or foreground transitions.
  const windowFocus = async (): Promise<boolean> => {
    if (!bindings.window?.Focus) return false;
    try {
      await bindings.window.Focus();
      return true;
    } catch {
      return false;
    }
  };
  const openSettingsWindow = () => bindings.settings?.Open() ?? Promise.resolve(false);
  const notifySettingsPainted = () => bindings.settings?.PaintReady?.();
  const closeSettingsWindow = () => bindings.settings?.Close();
  const { selectFile, selectDirectory, showSaveDialog } = nativeFileActions;
  const encodeUtf8 = (value: string) => bytesToBase64(new TextEncoder().encode(value));
  const safeLogName = (label: string, startTime: number, format: 'txt' | 'raw' | 'html') => {
    const base = [...label.trim()].map(character => character.charCodeAt(0) < 32 || '<>:"/\\|?*'.includes(character) ? '-' : character).join('').replace(/\s+/g, ' ').slice(0, 80) || 'session';
    const stamp = new Date(Number.isFinite(startTime) ? startTime : Date.now()).toISOString().replace(/[:.]/g, '-');
    return `${base}-${stamp}.${format}`;
  };
  const formatSessionLog = (terminalData: string, format: 'txt' | 'raw' | 'html', title: string) => {
    if (format === 'raw') return terminalData;
    const normalized = terminalData.replace(/\r\n/g, '\n').replace(/\r/g, '\n');
    if (format === 'txt') return normalized;
    const escaped = normalized.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    const safeTitle = title.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    return `<!doctype html><html><head><meta charset="utf-8"><title>${safeTitle}</title><style>body{background:#101318;color:#e5e7eb;font:14px/1.5 ui-monospace,monospace;padding:24px}pre{white-space:pre-wrap}</style></head><body><pre>${escaped}</pre></body></html>`;
  };
  const joinNativePath = (directory: string, name: string) => `${directory.replace(/[\\/]+$/, '')}${navigator.platform.startsWith('Win') ? '\\' : '/'}${name}`;
  const exportSessionLog: NonNullable<LemonSSHBridge['exportSessionLog']> = async payload => {
    if (!bindings.filesystem?.WriteFile) return { success: false };
    const fileName = safeLogName(payload.hostLabel || payload.hostname, payload.startTime, payload.format);
    const filePath = await showSaveDialog(fileName, [{ name: payload.format.toUpperCase(), extensions: [payload.format] }]);
    if (!filePath) return { success: false, canceled: true };
    await bindings.filesystem.WriteFile(filePath, encodeUtf8(formatSessionLog(payload.terminalData, payload.format, payload.hostLabel || payload.hostname)));
    return { success: true, filePath };
  };
  const autoSaveSessionLog: NonNullable<LemonSSHBridge['autoSaveSessionLog']> = async payload => {
    try {
      if (!bindings.filesystem?.WriteFile) throw new Error('Session log writer unavailable');
      const filePath = joinNativePath(payload.directory, safeLogName(payload.hostLabel || payload.hostname, payload.startTime, payload.format));
      await bindings.filesystem.WriteFile(filePath, encodeUtf8(formatSessionLog(payload.terminalData, payload.format, payload.hostLabel || payload.hostname)));
      return { success: true, filePath };
    } catch (error) {
      return { success: false, error: error instanceof Error ? error.message : String(error) };
    }
  };
  const manualLogSelections = new Map<string, string>();
  const chooseManualSessionLogPath: NonNullable<LemonSSHBridge['chooseManualSessionLogPath']> = async payload => {
    const format = payload.format ?? 'txt';
    const fileName = safeLogName(payload.sessionName || 'session', Date.now(), format);
    const defaultPath = payload.preferredDirectory ? joinNativePath(payload.preferredDirectory, fileName) : fileName;
    const filePath = await showSaveDialog(defaultPath, [{ name: format.toUpperCase(), extensions: [format] }]);
    if (!filePath) return { success: false, canceled: true };
    const selectionToken = crypto.randomUUID();
    manualLogSelections.set(selectionToken, filePath);
    return { success: true, selectionToken, filePath, format };
  };
  const startManualSessionLog: NonNullable<LemonSSHBridge['startManualSessionLog']> = async payload => {
    if (!bindings.sessionLog?.Start) return { success: false, started: false, error: 'Session log service unavailable' };
    const filePath = payload.selectionToken ? manualLogSelections.get(payload.selectionToken) : undefined;
    if (payload.selectionToken) manualLogSelections.delete(payload.selectionToken);
    if (!filePath) return { success: false, started: false, canceled: true, error: 'Session log path was not selected' };
    const result = await bindings.sessionLog.Start(nativeSessionId(payload.sessionId), filePath, payload.initialLine ?? '');
    return { success: result.success, started: Boolean(result.started), error: result.error, filePath: result.filePath };
  };
  const stopManualSessionLog: NonNullable<LemonSSHBridge['stopManualSessionLog']> = async payload => {
    if (!bindings.sessionLog?.Stop) return { success: false, stopped: false, error: 'Session log service unavailable' };
    const result = await bindings.sessionLog.Stop(nativeSessionId(payload.sessionId));
    return { success: result.success, stopped: Boolean(result.stopped), error: result.error, filePath: result.filePath };
  };
  const getManualSessionLogStatus: NonNullable<LemonSSHBridge['getManualSessionLogStatus']> = async payload => {
    if (!bindings.sessionLog?.Status) return { success: false, isLogging: false, error: 'Session log service unavailable' };
    const result = await bindings.sessionLog.Status(nativeSessionId(payload.sessionId));
    return { success: result.success, isLogging: Boolean(result.isLogging), error: result.error };
  };
  const startPortForward = async (options: PortForwardOptions): Promise<PortForwardResult> => {
    if (!bindings.forward?.Start) missingBridgeMethod("startPortForward");
    const request = pickSSHConnectArgs(options as Parameters<LemonSSHBridge["startSSHSession"]>[0]);
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
  // Ordered tunnel-table events pushed by ForwardService.setEventEmitter
  // (cmd/lemonssh/forwardService.go). The runtime feed fans out through one
  // shared native subscription; per-tunnel status callbacks filter the same
  // feed, so both stay strictly ordered by the Go-side revision.
  const portForwardRuntimeListeners = new Set<PortForwardRuntimeEventCallback>();
  let portForwardRuntimeNativeUnsubscribe: (() => void) | undefined;
  const dispatchPortForwardRuntimeEvent = (payload: unknown) => {
    const event = payload as PortForwardRuntimeEvent;
    for (const listener of [...portForwardRuntimeListeners]) {
      try {
        listener(event);
      } catch {
        // One broken listener must not starve the others.
      }
    }
  };
  const ensurePortForwardRuntimeNativeSubscription = () => {
    if (portForwardRuntimeNativeUnsubscribe) return;
    portForwardRuntimeNativeUnsubscribe = subscribeNativeEvent(portForwardRuntimeEvent, dispatchPortForwardRuntimeEvent);
  };
  const onPortForwardRuntime: NonNullable<LemonSSHBridge["onPortForwardRuntime"]> = (cb) => {
    portForwardRuntimeListeners.add(cb);
    ensurePortForwardRuntimeNativeSubscription();
    return () => {
      portForwardRuntimeListeners.delete(cb);
      if (portForwardRuntimeListeners.size === 0) {
        portForwardRuntimeNativeUnsubscribe?.();
        portForwardRuntimeNativeUnsubscribe = undefined;
      }
    };
  };
  const unsubscribePortForwardRuntime: NonNullable<LemonSSHBridge["unsubscribePortForwardRuntime"]> = async () => {
    portForwardRuntimeListeners.clear();
    portForwardRuntimeNativeUnsubscribe?.();
    portForwardRuntimeNativeUnsubscribe = undefined;
    return { success: true };
  };
  // The snapshot call of the runtime subscription protocol: the ordered
  // events plus this snapshot give the renderer gap-free state.
  const subscribePortForwardRuntime: NonNullable<LemonSSHBridge["subscribePortForwardRuntime"]> = async () =>
    getPortForwardSnapshot();
  const subscribePortForward: NonNullable<LemonSSHBridge["subscribePortForward"]> = async (tunnelId) => {
    const status = await getPortForwardStatus(tunnelId);
    return { tunnelId: status.tunnelId, status: status.status, error: status.error };
  };
  const resolvePortForwardPhase = (phase: string): PortForwardStatusResult["status"] => {
    if (phase === "active" || phase === "inactive" || phase === "error") return phase;
    return "connecting";
  };
  const onPortForwardStatus: NonNullable<LemonSSHBridge["onPortForwardStatus"]> = (tunnelId, cb) => {
    return subscribeNativeEvent(portForwardRuntimeEvent, (payload) => {
      const event = payload as PortForwardRuntimeEvent;
      if (event.kind === "upsert") {
        if (event.record?.tunnelId !== tunnelId) return;
        cb(resolvePortForwardPhase(event.record.phase), event.record.error);
        return;
      }
      if (event.tunnelId !== tunnelId) return;
      cb("inactive");
    });
  };
  const statLocalPath = (path: string) =>
    bindings.filesystem?.StatPath?.(path) as Promise<{ name: string; isDir: boolean; size: number }>;
  const readLocalFile: NonNullable<LemonSSHBridge["readLocalFile"]> = async (path, options) => {
    if (!bindings.filesystem?.ReadFile) missingBridgeMethod("readLocalFile");
    return base64ToArrayBuffer(await bindings.filesystem.ReadFile(path, options?.maxBytes ?? 0));
  };
  const writeLocalFile: NonNullable<LemonSSHBridge["writeLocalFile"]> = async (path, content) => {
    if (!bindings.filesystem?.WriteFile) missingBridgeMethod("writeLocalFile");
    await bindings.filesystem.WriteFile(path, bytesToBase64(new Uint8Array(content)));
  };
  const deleteLocalFile: NonNullable<LemonSSHBridge["deleteLocalFile"]> = async (path, expectedType) => {
    if (!bindings.filesystem?.DeletePath) missingBridgeMethod("deleteLocalFile");
    await bindings.filesystem.DeletePath(path, expectedType ?? "");
  };
  const renameLocalFile = async (oldPath: string, newPath: string) => {
    if (!bindings.filesystem?.RenamePath) missingBridgeMethod("renameLocalFile");
    await bindings.filesystem.RenamePath(oldPath, newPath);
  };
  const mkdirLocal = async (path: string) => {
    if (!bindings.filesystem?.Mkdir) missingBridgeMethod("mkdirLocal");
    await bindings.filesystem.Mkdir(path);
  };
  const statLocal = async (path: string) => {
    if (!bindings.filesystem?.Stat) missingBridgeMethod("statLocal");
    return localStatToResult(await bindings.filesystem.Stat(path));
  };
  const lstatLocal = async (path: string) => {
    if (!bindings.filesystem?.Lstat) missingBridgeMethod("lstatLocal");
    return localStatToResult(await bindings.filesystem.Lstat(path));
  };
  const listDrives = async () => {
    if (!bindings.filesystem?.ListDrives) return [];
    return bindings.filesystem.ListDrives();
  };
  const getSystemInfo = async () => {
    if (!bindings.filesystem?.SystemInfo) missingBridgeMethod("getSystemInfo");
    return bindings.filesystem.SystemInfo();
  };
  const getHomeDir = async () => {
    if (!bindings.filesystem?.HomeDir) missingBridgeMethod("getHomeDir");
    return bindings.filesystem.HomeDir();
  };
  const listLocalDir = async (path: string) => {
    if (!bindings.filesystem?.ListDir) missingBridgeMethod("listLocalDir");
    return bindings.filesystem.ListDir(path);
  };
  const localTreeScans = new Map<string, AbortController>();
  const listLocalTree: NonNullable<LemonSSHBridge["listLocalTree"]> = async (path, options = {}) => {
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
    eventsOn("lemonssh:files-dropped", (event) => {
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
  type KeyboardInteractiveCallback = Parameters<NonNullable<LemonSSHBridge["onKeyboardInteractive"]>>[0];
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

  // SSH interactive auth prompts (passphrase + changed host-key). The Go
  // terminal service emits the raw payloads; this adapter normalizes them to
  // the renderer bridge contract and routes the responses back.
  type PassphraseRequestCallback = Parameters<NonNullable<LemonSSHBridge["onPassphraseRequest"]>>[0];
  type PassphraseTimeoutCallback = Parameters<NonNullable<LemonSSHBridge["onPassphraseTimeout"]>>[0];
  type PassphraseCancelledCallback = Parameters<NonNullable<LemonSSHBridge["onPassphraseCancelled"]>>[0];
  type PassphraseAuthFailedCallback = Parameters<NonNullable<LemonSSHBridge["onPassphraseAuthFailed"]>>[0];
  type HostKeyVerificationCallback = Parameters<NonNullable<LemonSSHBridge["onHostKeyVerification"]>>[0];
  const passphraseListeners = new Set<PassphraseRequestCallback>();
  const passphraseTimeoutListeners = new Set<PassphraseTimeoutCallback>();
  const passphraseCancelledListeners = new Set<PassphraseCancelledCallback>();
  const passphraseAuthFailedListeners = new Set<PassphraseAuthFailedCallback>();
  const hostKeyVerificationListeners = new Set<HostKeyVerificationCallback>();
  let sshInteractiveEventsSubscribed = false;
  const subscribeSSHInteractiveEvents = () => {
    if (sshInteractiveEventsSubscribed) return;
    const eventsOn = bindings.events?.On ?? Events.On;
    if (typeof eventsOn !== "function") return;
    sshInteractiveEventsSubscribed = true;
    eventsOn("ssh:passphrase-request", (event) => {
      const payload = (event?.data ?? event) as Record<string, unknown> | null;
      if (!payload || typeof payload.requestId !== "string") return;
      const request = {
        requestId: payload.requestId,
        keyPath: typeof payload.keyPath === "string" ? payload.keyPath : "",
        keyName: typeof payload.keyName === "string" ? payload.keyName : "",
        hostname: typeof payload.hostname === "string" ? payload.hostname : undefined,
        passphraseInvalid: Boolean(payload.passphraseInvalid),
        sessionId: typeof payload.sessionId === "string" ? payload.sessionId : undefined,
        bootEpoch: typeof payload.bootEpoch === "number" && Number.isFinite(payload.bootEpoch) ? payload.bootEpoch : undefined,
      };
      for (const listener of passphraseListeners) listener(request);
    });
    eventsOn("ssh:passphrase-timeout", (event) => {
      const payload = (event?.data ?? event) as { requestId?: string } | null;
      for (const listener of passphraseTimeoutListeners) listener({ requestId: String(payload?.requestId ?? "") });
    });
    eventsOn("ssh:passphrase-cancelled", (event) => {
      const payload = (event?.data ?? event) as { requestId?: string } | null;
      for (const listener of passphraseCancelledListeners) listener({ requestId: String(payload?.requestId ?? "") });
    });
    eventsOn("ssh:passphrase-auth-failed", (event) => {
      const payload = (event?.data ?? event) as { keyPaths?: string[]; keyIds?: string[] } | null;
      for (const listener of passphraseAuthFailedListeners) {
        listener({ keyPaths: Array.isArray(payload?.keyPaths) ? payload.keyPaths : [], keyIds: Array.isArray(payload?.keyIds) ? payload.keyIds : undefined });
      }
    });
    eventsOn("ssh:host-key-verification", (event) => {
      const payload = (event?.data ?? event) as Record<string, unknown> | null;
      if (!payload || typeof payload.requestId !== "string") return;
      const request = {
        requestId: payload.requestId,
        sessionId: typeof payload.sessionId === "string" ? payload.sessionId : "",
        hostname: String(payload.hostname ?? ""),
        port: typeof payload.port === "number" && Number.isFinite(payload.port) ? payload.port : 22,
        status: payload.status === "unknown" ? "unknown" as const : "changed" as const,
        keyType: String(payload.keyType ?? ""),
        fingerprint: String(payload.fingerprint ?? ""),
        publicKey: typeof payload.publicKey === "string" ? payload.publicKey : undefined,
        knownFingerprint: typeof payload.knownFingerprint === "string" ? payload.knownFingerprint : undefined,
        bootEpoch: typeof payload.bootEpoch === "number" && Number.isFinite(payload.bootEpoch) ? payload.bootEpoch : undefined,
      };
      for (const listener of hostKeyVerificationListeners) listener(request);
    });
  };
  const onPassphraseRequest = (cb: PassphraseRequestCallback) => {
    subscribeSSHInteractiveEvents();
    passphraseListeners.add(cb);
    return () => {
      passphraseListeners.delete(cb);
    };
  };
  const respondPassphrase = async (requestId: string, passphrase: string, cancelled?: boolean) => {
    try {
      await bindings.terminal.RespondPassphrase?.(requestId, passphrase, Boolean(cancelled));
      return { success: true };
    } catch (error) {
      return { success: false, error: error instanceof Error ? error.message : String(error) };
    }
  };
  const respondPassphraseSkip = async (requestId: string) => {
    // Skipping abandons the key: without a decrypted key the dial cannot use
    // it, so the Go broker treats this like a cancel of the prompt.
    return respondPassphrase(requestId, "", true);
  };
  const onPassphraseTimeout = (cb: PassphraseTimeoutCallback) => {
    subscribeSSHInteractiveEvents();
    passphraseTimeoutListeners.add(cb);
    return () => {
      passphraseTimeoutListeners.delete(cb);
    };
  };
  const onPassphraseCancelled = (cb: PassphraseCancelledCallback) => {
    subscribeSSHInteractiveEvents();
    passphraseCancelledListeners.add(cb);
    return () => {
      passphraseCancelledListeners.delete(cb);
    };
  };
  const onPassphraseAuthFailed = (cb: PassphraseAuthFailedCallback) => {
    subscribeSSHInteractiveEvents();
    passphraseAuthFailedListeners.add(cb);
    return () => {
      passphraseAuthFailedListeners.delete(cb);
    };
  };
  const onHostKeyVerification = (cb: HostKeyVerificationCallback) => {
    subscribeSSHInteractiveEvents();
    hostKeyVerificationListeners.add(cb);
    return () => {
      hostKeyVerificationListeners.delete(cb);
    };
  };
  const respondHostKeyVerification = async (requestId: string, accept: boolean, addToKnownHosts?: boolean) => {
    try {
      await bindings.terminal.RespondHostKeyVerification?.(requestId, Boolean(accept), Boolean(addToKnownHosts));
      return { success: true };
    } catch (error) {
      return { success: false, error: error instanceof Error ? error.message : String(error) };
    }
  };

  type TelnetEchoCallback = Parameters<NonNullable<LemonSSHBridge["onTelnetEchoMode"]>>[1];
  type TelnetLoginCallback = Parameters<NonNullable<LemonSSHBridge["onTelnetAutoLoginComplete"]>>[1];
  type MoshReadyCallback = Parameters<NonNullable<LemonSSHBridge["onMoshSessionReady"]>>[1];
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
        const payload = (event?.data ?? event) as LemonSSHHelperSessionState | null;
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
  }) as unknown as LemonSSHBridge["onTelnetEchoMode"];
  const onTelnetAutoLoginComplete = ((sessionId: string, cb: TelnetLoginCallback) => {
    subscribeTerminalEvents();
    const set = telnetLoginListeners.get(sessionId) ?? new Set();
    set.add(cb);
    telnetLoginListeners.set(sessionId, set);
    return () => set.delete(cb);
  }) as unknown as LemonSSHBridge["onTelnetAutoLoginComplete"];
  const onTelnetAutoLoginCancelled = ((sessionId: string, cb: TelnetLoginCallback) => {
    subscribeTerminalEvents();
    const set = telnetCancelListeners.get(sessionId) ?? new Set();
    set.add(cb);
    telnetCancelListeners.set(sessionId, set);
    return () => set.delete(cb);
  }) as unknown as LemonSSHBridge["onTelnetAutoLoginCancelled"];
  const onMoshSessionReady = ((sessionId: string, cb: MoshReadyCallback) => {
    subscribeTerminalEvents();
    const set = moshReadyListeners.get(sessionId) ?? new Set();
    set.add(cb);
    moshReadyListeners.set(sessionId, set);
    return () => set.delete(cb);
  }) as unknown as LemonSSHBridge["onMoshSessionReady"];
  const onHelperLifecycle = ((sessionId: string, cb: HelperLifecycleCallback) => {
    subscribeTerminalEvents();
    const set = helperLifecycleListeners.get(sessionId) ?? new Set();
    set.add(cb);
    helperLifecycleListeners.set(sessionId, set);
    return () => {
      set.delete(cb);
      if (set.size === 0) helperLifecycleListeners.delete(sessionId);
    };
  }) as unknown as LemonSSHBridge["onHelperLifecycle"];
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
  }) as unknown as LemonSSHBridge["restartHelperSession"];

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
  }) as unknown as LemonSSHBridge["scriptRecordingAppendStep"];
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
  }) as unknown as LemonSSHBridge["scriptRun"];
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
    form?: unknown;
  }) => void>();
  const scriptRunsUpdatedListeners = new Set<(payload: { runs: unknown[] }) => void>();
  let scriptEventsSubscribed = false;
  const subscribeScriptEvents = () => {
    if (scriptEventsSubscribed) return;
    const eventsOn = bindings.events?.On ?? Events.On;
    if (typeof eventsOn !== "function") return;
    scriptEventsSubscribed = true;
    eventsOn("lemonssh:script:runs-updated", (event) => {
      const payload = ((event as { data?: unknown })?.data ?? event) as { runs?: unknown[] };
      const runs = Array.isArray(payload?.runs) ? payload.runs : [];
      for (const listener of scriptRunsUpdatedListeners) listener({ runs });
    });
    eventsOn("lemonssh:script:dialog-request", (event) => {
      const payload = ((event as { data?: unknown })?.data ?? event) as {
        requestId?: string;
        type?: string;
        message?: string;
        defaultValue?: string;
        sensitive?: boolean;
        form?: unknown;
      };
      if (!payload?.requestId) return;
      const request = {
        requestId: payload.requestId,
        type: payload.type ?? "prompt",
        message: payload.message ?? "",
        defaultValue: payload.defaultValue,
        sensitive: payload.sensitive,
        form: payload.form,
      };
      for (const listener of scriptDialogRequestListeners) listener(request);
    });
  };
  const onScriptRunsUpdated = (cb: Parameters<NonNullable<LemonSSHBridge["onScriptRunsUpdated"]>>[0]) => {
    subscribeScriptEvents();
    scriptRunsUpdatedListeners.add(cb);
    return () => {
      scriptRunsUpdatedListeners.delete(cb);
    };
  };
  const onScriptDialogRequest = (cb: Parameters<NonNullable<LemonSSHBridge["onScriptDialogRequest"]>>[0]) => {
    subscribeScriptEvents();
    scriptDialogRequestListeners.add(cb);
    return () => {
      scriptDialogRequestListeners.delete(cb);
    };
  };
  const scriptDialogResponse = async (requestId: string, value?: unknown, cancelled?: boolean) => {
    if (!bindings.script?.ResolveDialog) missingBridgeMethod("scriptDialogResponse");
    const encoded = typeof value === "string" || value === undefined || value === null
      ? (typeof value === "string" ? value : "")
      : JSON.stringify(value);
    const ok = await bindings.script.ResolveDialog(
      requestId,
      encoded,
      Boolean(cancelled),
    );
    return { ok };
  };

  // Go interaction router (W13): the host blocks a capability dispatch until
  // the renderer responds, the deadline lapses or the turn cancels. Lazy
  // single subscription + Set fan-out, like subscribeScriptEvents, so every
  // mounted approval host (App shell + settings window) sees the prompt.
  type AgentInteractionCallback = Parameters<NonNullable<LemonSSHBridge["onAgentInteraction"]>>[0];
  const agentInteractionListeners = new Set<AgentInteractionCallback>();
  let agentInteractionSubscribed = false;
  const subscribeAgentInteractionEvents = () => {
    if (agentInteractionSubscribed) return;
    const eventsOn = bindings.events?.On ?? Events.On;
    if (typeof eventsOn !== "function") return;
    agentInteractionSubscribed = true;
    eventsOn("agent:interaction", (event) => {
      const payload = ((event as { data?: unknown })?.data ?? event) as Parameters<AgentInteractionCallback>[0];
      for (const listener of agentInteractionListeners) listener(payload);
    });
  };
  const onAgentInteraction = ((cb: AgentInteractionCallback) => {
    subscribeAgentInteractionEvents();
    agentInteractionListeners.add(cb);
    return () => {
      agentInteractionListeners.delete(cb);
    };
  }) as unknown as LemonSSHBridge["onAgentInteraction"];
  const agentPendingInteractions = (async () => {
    if (!bindings.agentservice?.AgentPendingInteractions) missingBridgeMethod("agentPendingInteractions");
    return bindings.agentservice.AgentPendingInteractions();
  }) as unknown as LemonSSHBridge["agentPendingInteractions"];
  // Errors propagate untouched: the caller distinguishes typed
  // "not pending"/"already resolved" outcomes from transport failures.
  const agentRespondInteraction = (async (interactionId: string, approved: boolean) => {
    if (!bindings.agentservice?.AgentRespondInteraction) missingBridgeMethod("agentRespondInteraction");
    await bindings.agentservice.AgentRespondInteraction(interactionId, approved);
  }) as unknown as LemonSSHBridge["agentRespondInteraction"];

  const implementedBridge: Partial<LemonSSHBridge> = {
    ...createAgentToolBridge(bindings.agentservice, bindings.events?.On ?? Events.On, nativeSessionId),
    ...createAgentCliBridge(bindings.agentcli),
    ...createUserSkillsBridge(bindings.userSkills),
    ...createExternalAgentBridge(bindings.externalAgent, bindings.events?.On ?? Events.On),
    ...monitoring,
    ...cloudOAuth,
    ...createProviderBridge(bindings.provider ?? providerFetchService as unknown as NativeProviderBindings, bindings.events?.On ?? Events.On),
    onAgentInteraction,
    agentPendingInteractions,
    agentRespondInteraction,
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
    // Distro probe: reuses the session transport on the Go side (no new SSH
    // connection), mapping the /etc/os-release output the renderer expects.
    getSessionDistroInfo: (async (sessionId: string) => {
      const probe = bindings.terminal.GetSessionDistroInfo;
      if (!probe) return { success: false as const, error: "getSessionDistroInfo unavailable" };
      try {
        return await probe(nativeSessionId(sessionId));
      } catch (error) {
        return { success: false as const, error: error instanceof Error ? error.message : String(error) };
      }
    }) as LemonSSHBridge["getSessionDistroInfo"],
    // Remote shell history for the History side panel: one exec channel on the
    // session's existing transport detects the login shell and tails the files.
    readRemoteHistory: (async (sessionId: string, limit?: number) => {
      const read = bindings.terminal.ReadRemoteHistory;
      if (!read) return { success: false as const, error: "readRemoteHistory unavailable" };
      try {
        return await read(nativeSessionId(sessionId), typeof limit === "number" && limit > 0 ? limit : 1000);
      } catch (error) {
        return { success: false as const, error: error instanceof Error ? error.message : String(error) };
      }
    }) as LemonSSHBridge["readRemoteHistory"],
    // Busy-close confirmation: direct children of a local session's shell.
    ptyGetChildProcesses: (async (sessionId: string) => {
      try {
        return (await bindings.terminal.PtyGetChildProcesses?.(nativeSessionId(sessionId))) ?? [];
      } catch {
        return [];
      }
    }) as unknown as LemonSSHBridge["ptyGetChildProcesses"],
    // The busy-terminal confirm dialog is renderer-side under Wails (the
    // Electron shell used a native main-process dialog); window.confirm keeps
    // the i18n'd title/message and the same boolean contract.
    confirmCloseBusy: (async (payload) => {
      if (typeof window === "undefined" || typeof window.confirm !== "function") return false;
      const title = payload.title ? `${payload.title}\n\n` : "";
      return window.confirm(`${title}${payload.message ?? payload.command}`);
    }) as LemonSSHBridge["confirmCloseBusy"],
    // SSH debug log toggle (settings > system): applies the process-global
    // logger state in Go so ssh-debug.log actually receives events.
    setSshDebugLogsEnabled: (async (enabled: boolean) => {
      if (!bindings.diagnosticLog?.SetSshDebugLogEnabled) {
        return { enabled: Boolean(enabled), path: "", exists: false, size: 0 };
      }
      return bindings.diagnosticLog.SetSshDebugLogEnabled(enabled);
    }) as LemonSSHBridge["setSshDebugLogsEnabled"],
    // --- Attach (popup observe) primitives ---
    setSessionFlowPaused: ((sessionId: string, paused: boolean) => {
      void bindings.terminal.SetSessionFlowPaused?.(nativeSessionId(sessionId), paused);
    }) as unknown as LemonSSHBridge["setSessionFlowPaused"],
    setSessionFlowPausedAndWait: (async (sessionId: string, paused: boolean) => {
      const wait = bindings.terminal.SetSessionFlowPausedAndWait;
      if (!wait) return { success: false as const, error: "Output drain unavailable" };
      try {
        return await wait(nativeSessionId(sessionId), paused);
      } catch (error) {
        return { success: false as const, error: error instanceof Error ? error.message : String(error) };
      }
    }) as unknown as LemonSSHBridge["setSessionFlowPausedAndWait"],
    acquireSessionFlowPauseLease: (async (sessionId: string) => {
      const acquire = bindings.terminal.AcquireSessionFlowPauseLease;
      if (!acquire) return { success: false as const, error: "Terminal flow pause leases unavailable" };
      try {
        return await acquire(nativeSessionId(sessionId));
      } catch (error) {
        return { success: false as const, error: error instanceof Error ? error.message : String(error) };
      }
    }) as unknown as LemonSSHBridge["acquireSessionFlowPauseLease"],
    waitSessionFlowPauseLease: (async (sessionId: string, leaseId: string) => {
      const wait = bindings.terminal.WaitSessionFlowPauseLease;
      if (!wait) return { success: false as const, error: "Output drain unavailable" };
      try {
        return await wait(nativeSessionId(sessionId), leaseId);
      } catch (error) {
        return { success: false as const, error: error instanceof Error ? error.message : String(error) };
      }
    }) as unknown as LemonSSHBridge["waitSessionFlowPauseLease"],
    releaseSessionFlowPauseLease: (async (sessionId: string, leaseId: string, options?: { keepPaused?: boolean }) => {
      const release = bindings.terminal.ReleaseSessionFlowPauseLease;
      if (!release) return { success: false as const, error: "Terminal flow pause leases unavailable" };
      try {
        return await release(nativeSessionId(sessionId), leaseId, options);
      } catch (error) {
        return { success: false as const, error: error instanceof Error ? error.message : String(error) };
      }
    }) as unknown as LemonSSHBridge["releaseSessionFlowPauseLease"],
    requestTerminalSessionSnapshot: (async (sessionId: string, authorization: string) => {
      const request = bindings.terminal.RequestSessionSnapshot;
      if (!request) return { success: false as const, error: "requestTerminalSessionSnapshot unavailable" };
      try {
        return await request(nativeSessionId(sessionId), authorization ?? "");
      } catch (error) {
        return { success: false as const, error: error instanceof Error ? error.message : String(error) };
      }
    }) as unknown as LemonSSHBridge["requestTerminalSessionSnapshot"],
    respondTerminalSessionSnapshot: ((requestId: string, snapshot: string, kittyKeyboardModeState?: LemonSSHKittyKeyboardModeState, kittyKeyboardProtocolEnabled?: boolean, passwordPromptActive?: boolean, cwd?: string | null, title?: string | null) => {
      void bindings.terminal.RespondSessionSnapshot?.(
        requestId,
        snapshot ?? "",
        kittyKeyboardModeState ?? null,
        typeof kittyKeyboardProtocolEnabled === "boolean" ? kittyKeyboardProtocolEnabled : null,
        typeof passwordPromptActive === "boolean" ? passwordPromptActive : null,
        cwd ?? null,
        title ?? null,
      );
    }) as unknown as LemonSSHBridge["respondTerminalSessionSnapshot"],
    onTerminalSessionSnapshotRequest: ((cb: (payload: { sessionId: string; requestId: string }) => void) => {
      return subscribeNativeEvent("terminal:session-snapshot-request", payload => cb(payload as { sessionId: string; requestId: string }));
    }) as unknown as LemonSSHBridge["onTerminalSessionSnapshotRequest"],
    applyTerminalSessionSnapshot: (async (sessionId: string, snapshot: string, context: {
      contextSnapshot: string;
      contextViewportSnapshot: string;
      contextScrollbackSnapshot: string;
      alternateScreen: boolean;
      kittyKeyboardModeState?: LemonSSHKittyKeyboardModeState;
      kittyKeyboardProtocolEnabled?: boolean;
      passwordPromptActive?: boolean;
      cwd?: string | null;
      title?: string | null;
    }, authorization: string) => {
      const apply = bindings.terminal.ApplySessionSnapshot;
      if (!apply) return { success: false as const, error: "applyTerminalSessionSnapshot unavailable" };
      try {
        return await apply(nativeSessionId(sessionId), snapshot, context, authorization ?? "");
      } catch (error) {
        return { success: false as const, error: error instanceof Error ? error.message : String(error) };
      }
    }) as unknown as LemonSSHBridge["applyTerminalSessionSnapshot"],
    onTerminalSessionApplySnapshot: ((cb: (payload: {
      sessionId: string;
      snapshot: string;
      contextSnapshot: string;
      contextViewportSnapshot: string;
      contextScrollbackSnapshot: string;
      alternateScreen: boolean;
      kittyKeyboardModeState?: LemonSSHKittyKeyboardModeState;
      kittyKeyboardProtocolEnabled?: boolean;
      passwordPromptActive?: boolean;
      cwd?: string | null;
      title?: string | null;
      requestId: string;
    }) => boolean | Promise<boolean>) => {
      return subscribeNativeEvent("terminal:session-apply-snapshot", (payload) => {
        const record = (payload ?? {}) as { requestId?: string };
        void Promise.resolve(cb(payload as Parameters<typeof cb>[0])).then((accepted) => {
          if (typeof record.requestId === "string" && record.requestId !== "") {
            bindings.terminal.RespondApplySnapshot?.(record.requestId, accepted === true)?.catch(() => undefined);
          }
        }).catch(() => undefined);
      });
    }) as unknown as LemonSSHBridge["onTerminalSessionApplySnapshot"],
    markAttachPopupClosePrepared: (async (sessionId: string, authorization: string) => {
      const mark = bindings.terminal.MarkAttachPopupClosePrepared;
      if (!mark) return { success: false as const, error: "markAttachPopupClosePrepared unavailable" };
      try {
        return await mark(nativeSessionId(sessionId), authorization ?? "");
      } catch (error) {
        return { success: false as const, error: error instanceof Error ? error.message : String(error) };
      }
    }) as unknown as LemonSSHBridge["markAttachPopupClosePrepared"],
    onTerminalPopupPrepareClose: ((cb: (payload: { sessionId: string; authorization: string }) => void) => {
      return subscribeNativeEvent("terminal:popup-prepare-close", payload => cb(payload as { sessionId: string; authorization: string }));
    }) as unknown as LemonSSHBridge["onTerminalPopupPrepareClose"],
    // Rebind moves the display route to THIS renderer: Go rotates the route and
    // kicks the previous owner's socket; here we attach the new route so live
    // bytes land in this window immediately after the lease is released.
    rebindTerminalSessionOutput: (async (sessionId: string, authorization: string) => {
      const rebind = bindings.terminal.RebindSessionOutput;
      if (!rebind) return { success: false as const, error: "rebindTerminalSessionOutput unavailable" };
      const nativeId = nativeSessionId(sessionId);
      // Flag before the RPC: this window must ignore its own detached broadcast.
      rebindInitiations.add(nativeId);
      try {
        const result = await rebind(nativeId, authorization ?? "");
        if (!result?.success) {
          return { success: false as const, error: result?.error || "Failed to rebind terminal output" };
        }
        const route = result.route;
        if (route) {
          await attachDataPlane(nativeId, false, {
            SessionID: typeof route.sessionID === "string" && route.sessionID !== "" ? route.sessionID : nativeId,
            Generation: route.generation,
            DataToken: route.dataToken,
            UrgentToken: route.urgentToken,
            WindowBytes: typeof route.windowBytes === "number" && route.windowBytes > 0 ? route.windowBytes : 1048576,
          });
        }
        return { success: true as const, previousWebContentsId: null };
      } catch (error) {
        return { success: false as const, error: error instanceof Error ? error.message : String(error) };
      } finally {
        rebindInitiations.delete(nativeId);
      }
    }) as unknown as LemonSSHBridge["rebindTerminalSessionOutput"],
    restoreTerminalSessionOutput: (async (sessionId: string, _webContentsId?: number | null, authorization?: string) => {
      const restore = bindings.terminal.RestoreSessionOutput;
      if (!restore) return { success: false as const, error: "restoreTerminalSessionOutput unavailable" };
      try {
        const result = await restore(nativeSessionId(sessionId), authorization ?? "");
        return result?.success
          ? { success: true as const, restored: true }
          : { success: false as const, error: result?.error || "Failed to restore terminal output" };
      } catch (error) {
        return { success: false as const, error: error instanceof Error ? error.message : String(error) };
      }
    }) as unknown as LemonSSHBridge["restoreTerminalSessionOutput"],
    showSystemNotification,
    writeClipboardText,
    readClipboardText,
    readClipboardImage,
    generateKeyPair: (async options => {
      if (!bindings.terminal.GenerateKeyPair) return { success: false, error: 'generateKeyPair unavailable' };
      return bindings.terminal.GenerateKeyPair(options);
    }) as LemonSSHBridge['generateKeyPair'],
    checkSshAgent: (async options => {
      if (!bindings.terminal.CheckSshAgent) return { running: false, startupType: null, error: 'checkSshAgent unavailable' };
      const result = await bindings.terminal.CheckSshAgent(options ?? {});
      return { running: result.running, startupType: result.startupType ?? null, error: result.error ?? null };
    }) as LemonSSHBridge['checkSshAgent'],
    getDefaultKeys: (async () => bindings.terminal.GetDefaultKeys?.() ?? []) as LemonSSHBridge['getDefaultKeys'],
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
    setSessionEncoding,
    onSessionData: onSessionData as LemonSSHBridge["onSessionData"],
    onSessionExit: onSessionExit as LemonSSHBridge["onSessionExit"],
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
    lstatSftp,
    realpathSftp,
    closeSftp,
    retainSftpTransferSession,
    releaseSftpTransferSession,
    sameHostCopyDirectory,
    readSftp,
    readSftpBinary,
    writeSftp,
    writeSftpBinary,
    // The Go bindings return Wails-native shapes; cast until the shared
    // port types gain Wails-specific variants.
    getSftpHomeDir: ((sftpID: string) =>
      bindings.sftp.HomeDir?.(sftpID).then((homeDir) => ({ success: true, homeDir }))) as unknown as LemonSSHBridge["getSftpHomeDir"],
    getPathForFile: (() => {
      // WebView2 File.path points at the dropped file but os.Open rejects it
      // with PATH_NOT_FOUND on some hosts while os.Stat succeeds. Return
      // undefined so the upload pipeline stages the File content instead.
      return undefined;
    }) as unknown as LemonSSHBridge["getPathForFile"],
    statLocalPath: statLocalPath as unknown as LemonSSHBridge["statLocalPath"],
    getHomeDir,
    listDrives,
    getSystemInfo,
    readLocalFile,
    writeLocalFile,
    deleteLocalFile,
    renameLocalFile,
    mkdirLocal,
    statLocal,
    lstatLocal,
    listLocalDir,
    listLocalTree,
    cancelLocalTreeScan,
    stageFromLocalPath: (async (path: string) => {
      if (!bindings.filesystem?.StageFromLocalPath) throw new Error('Native staging unavailable');
      const result = await bindings.filesystem.StageFromLocalPath(path);
      if (Array.isArray(result)) return { stagedPath: result[0], name: path.split(/[\\/]/).pop() || path, size: result[1] };
      return result;
    }) as LemonSSHBridge["stageFromLocalPath"],
    appendDiagnosticLog: appendDiagnosticLog as unknown as LemonSSHBridge["appendDiagnosticLog"],
    getCrashLogs: (() => bindings.diagnosticLog?.GetCrashLogs?.() ?? Promise.resolve([])) as LemonSSHBridge['getCrashLogs'],
    readCrashLog: ((fileName: string) => bindings.diagnosticLog?.ReadCrashLog?.(fileName) ?? Promise.resolve([])) as LemonSSHBridge['readCrashLog'],
    clearCrashLogs: (() => bindings.diagnosticLog?.ClearCrashLogs?.() ?? Promise.resolve({ deletedCount: 0 })) as LemonSSHBridge['clearCrashLogs'],
    openCrashLogsDir: (() => bindings.diagnosticLog?.OpenCrashLogsDir?.() ?? Promise.resolve({ success: false })) as LemonSSHBridge['openCrashLogsDir'],
    getSshDebugLogInfo: (() => bindings.diagnosticLog?.GetSshDebugLogInfo?.() ?? Promise.resolve({ enabled: false, path: '', exists: false, size: 0 })) as LemonSSHBridge['getSshDebugLogInfo'],
    openSshDebugLogDir: (() => bindings.diagnosticLog?.OpenSshDebugLogDir?.() ?? Promise.resolve({ success: false })) as LemonSSHBridge['openSshDebugLogDir'],
    exportSessionLog,
    autoSaveSessionLog,
    selectSessionLogsDir: (async () => {
      const directory = await selectDirectory('Select session log directory');
      return directory ? { success: true, directory } : { success: false, canceled: true };
    }) as LemonSSHBridge['selectSessionLogsDir'],
    openSessionLogsDir: (async (directory: string) => bindings.sessionLog?.OpenDirectory?.(directory) ?? { success: false, error: 'Session log service unavailable' }) as LemonSSHBridge['openSessionLogsDir'],
    clearSessionLogsDir: (async (directory: string) => bindings.sessionLog?.ClearDirectory?.(directory) ?? { success: false, deletedCount: 0, failedCount: 0, error: 'Session log service unavailable' }) as LemonSSHBridge['clearSessionLogsDir'],
    chooseManualSessionLogPath,
    startManualSessionLog,
    stopManualSessionLog,
    getManualSessionLogStatus,
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
    }) as unknown as LemonSSHBridge["stageUploadFile"],
    ...nativeFileActions,
    // openPath differs from openWithSystemDefault: the SFTP transfer queue
    // reveals a completed download by opening its parent DIRECTORY, which
    // OpenWithSystemDefault (regular-files-only) rejects. The Go OpenPath
    // binding accepts both shapes.
    openPath: (async (path: string) => {
      try {
        if (!bindings.filesystem?.OpenPath) throw new Error("openPath unavailable");
        await bindings.filesystem.OpenPath(path);
        return { success: true };
      } catch (error) {
        return { success: false, error: error instanceof Error ? error.message : String(error) };
      }
    }) as unknown as LemonSSHBridge["openPath"],
    // External-editor auto-sync: maps the renderer's file-watch contract onto
    // the Go FileWatchService (register temp downloads, watch managed-temp
    // saves, stream them back to the remote source, report synced/error).
    startFileWatch: (async (localPath: string, remotePath: string, sftpId: string, encoding?: SftpFilenameEncoding) => {
      if (!bindings.filesystem?.StartFileWatch) missingBridgeMethod("startFileWatch");
      return bindings.filesystem.StartFileWatch(localPath, remotePath, sftpId, encoding ?? "");
    }) as unknown as LemonSSHBridge["startFileWatch"],
    stopFileWatch: (async (watchId: string, cleanupTempFile?: boolean) => {
      if (!bindings.filesystem?.StopFileWatch) missingBridgeMethod("stopFileWatch");
      return bindings.filesystem.StopFileWatch(watchId, Boolean(cleanupTempFile));
    }) as unknown as LemonSSHBridge["stopFileWatch"],
    listFileWatches: (async () => {
      return (await bindings.filesystem?.ListFileWatches?.()) ?? [];
    }) as unknown as LemonSSHBridge["listFileWatches"],
    registerTempFile: (async (sftpId: string, localPath: string) => {
      if (!bindings.filesystem?.RegisterTempFile) missingBridgeMethod("registerTempFile");
      return bindings.filesystem.RegisterTempFile(sftpId, localPath);
    }) as unknown as LemonSSHBridge["registerTempFile"],
    unregisterTempFile: (async (sftpId: string, localPath: string) => {
      if (!bindings.filesystem?.UnregisterTempFile) missingBridgeMethod("unregisterTempFile");
      return bindings.filesystem.UnregisterTempFile(sftpId, localPath);
    }) as unknown as LemonSSHBridge["unregisterTempFile"],
    onFileWatchSynced: ((cb: Parameters<NonNullable<LemonSSHBridge["onFileWatchSynced"]>>[0]) =>
      subscribeNativeEvent(fileWatchSyncedEvent, cb)) as unknown as LemonSSHBridge["onFileWatchSynced"],
    onFileWatchError: ((cb: Parameters<NonNullable<LemonSSHBridge["onFileWatchError"]>>[0]) =>
      subscribeNativeEvent(fileWatchErrorEvent, cb)) as unknown as LemonSSHBridge["onFileWatchError"],
    onFileWatchStopped: ((cb: Parameters<NonNullable<LemonSSHBridge["onFileWatchStopped"]>>[0]) =>
      subscribeNativeEvent(fileWatchStoppedEvent, cb)) as unknown as LemonSSHBridge["onFileWatchStopped"],
    listAutocompleteRemoteDir: async (id: string, directory: string, foldersOnly: boolean, prefix = '', limit = 100) => {
      try { return await bindings.terminal.ListAutocompleteDirectory?.(nativeSessionId(id), directory, foldersOnly, prefix, limit) ?? { success: false, entries: [] }; }
      catch { return { success: false, entries: [] }; }
    },
    listAutocompleteLocalDir: async (directory: string, foldersOnly: boolean, prefix = '', limit = 100) => {
      try { return await bindings.terminal.ListAutocompleteDirectory?.('', directory, foldersOnly, prefix, limit) ?? { success: false, entries: [] }; }
      catch { return { success: false, entries: [] }; }
    },
    chmodSftp: async (sftpID: string, path: string, mode: string, encoding?: SftpFilenameEncoding) => {
      if (!bindings.sftp.Chmod) throw new Error('SFTP permissions unavailable');
      await bindings.sftp.Chmod(sftpID, path, mode, encoding);
    },
    onFilesDropped,
    setLanguage: (async (language: string) => {
      const changed = await bindings.tray?.SetLanguage?.(language);
      return changed ?? false;
    }) as unknown as LemonSSHBridge["setLanguage"],
    quitApp: (async () => {
      await bindings.tray?.Quit?.();
    }) as unknown as LemonSSHBridge["quitApp"],
    // Tray menu content: the renderer is the single content authority; the Go
    // TrayService stores the snapshot, rebuilds the context menu and mirrors
    // it onto the #/tray panel window.
    updateTrayMenuData: (async (data) => {
      const result = await bindings.tray?.UpdateTrayMenuData?.(data);
      return result ?? { success: false };
    }) as unknown as LemonSSHBridge["updateTrayMenuData"],
    onTrayFocusSession: ((cb: Parameters<NonNullable<LemonSSHBridge["onTrayFocusSession"]>>[0]) =>
      subscribeNativeEvent(trayFocusSessionEvent, (payload) => cb(String(payload ?? "")))) as unknown as LemonSSHBridge["onTrayFocusSession"],
    onTrayTogglePortForward: ((cb: Parameters<NonNullable<LemonSSHBridge["onTrayTogglePortForward"]>>[0]) =>
      subscribeNativeEvent(trayToggleForwardEvent, (payload) => {
        const data = (payload ?? {}) as { ruleId?: unknown; ruleID?: unknown; start?: unknown };
        cb(String(data.ruleId ?? data.ruleID ?? ""), data.start !== false);
      })) as unknown as LemonSSHBridge["onTrayTogglePortForward"],
    // Tray panel window: hide/show plus the panel-scoped action twins. The
    // panel's jump lands on TrayService.JumpToSessionFromPanel — the same
    // focus pipeline the tray menu session rows use — and the close-session
    // / connect-host requests ride the main window's existing handlers.
    hideTrayPanel: (async () => {
      // Go TrayPanelWindowService.Hide resolves a boolean.
      const result = await bindings.trayPanel?.Hide?.();
      return { success: result === true };
    }) as unknown as LemonSSHBridge["hideTrayPanel"],
    openMainWindow: (async () => {
      const result = await bindings.tray?.OpenMainWindow?.();
      return result ?? { success: false };
    }) as unknown as LemonSSHBridge["openMainWindow"],
    jumpToSessionFromTrayPanel: (async (sessionId: string) => {
      const result = await bindings.tray?.JumpToSessionFromPanel?.(sessionId);
      return result ?? { success: false };
    }) as unknown as LemonSSHBridge["jumpToSessionFromTrayPanel"],
    connectToHostFromTrayPanel: (async (hostId: string) => {
      const result = await bindings.tray?.ConnectToHost?.(hostId);
      return result ?? { success: false };
    }) as unknown as LemonSSHBridge["connectToHostFromTrayPanel"],
    closeSessionFromTrayPanel: (async (sessionId: string) => {
      const result = await bindings.tray?.CloseSessionFromPanel?.(sessionId);
      return result ?? { success: false };
    }) as unknown as LemonSSHBridge["closeSessionFromTrayPanel"],
    notifyTrayPanelPaintReady: (async () => {
      const result = await bindings.trayPanel?.PaintReady?.();
      return result ?? false;
    }) as unknown as LemonSSHBridge["notifyTrayPanelPaintReady"],
    onTrayPanelJumpToSession: ((cb: Parameters<NonNullable<LemonSSHBridge["onTrayPanelJumpToSession"]>>[0]) =>
      subscribeNativeEvent(trayPanelJumpSessionEvent, (payload) => cb(String(payload ?? "")))) as unknown as LemonSSHBridge["onTrayPanelJumpToSession"],
    onTrayPanelConnectToHost: ((cb: Parameters<NonNullable<LemonSSHBridge["onTrayPanelConnectToHost"]>>[0]) =>
      subscribeNativeEvent(trayPanelConnectHostEvent, (payload) => cb(String(payload ?? "")))) as unknown as LemonSSHBridge["onTrayPanelConnectToHost"],
    onTrayPanelCloseSession: ((cb: Parameters<NonNullable<LemonSSHBridge["onTrayPanelCloseSession"]>>[0]) =>
      subscribeNativeEvent(trayPanelCloseSessionEvent, (payload) => cb(String(payload ?? "")))) as unknown as LemonSSHBridge["onTrayPanelCloseSession"],
    onTrayPanelMenuData: ((cb: Parameters<NonNullable<LemonSSHBridge["onTrayPanelMenuData"]>>[0]) =>
      subscribeNativeEvent(trayPanelMenuDataEvent, (payload) => cb(normalizeTrayPanelMenuData(payload)))) as unknown as LemonSSHBridge["onTrayPanelMenuData"],
    onTrayPanelRefresh: ((cb: Parameters<NonNullable<LemonSSHBridge["onTrayPanelRefresh"]>>[0]) =>
      subscribeNativeEvent(trayPanelRefreshEvent, () => cb())) as unknown as LemonSSHBridge["onTrayPanelRefresh"],
    onTrayPanelCloseRequest: ((cb: Parameters<NonNullable<LemonSSHBridge["onTrayPanelCloseRequest"]>>[0]) =>
      subscribeNativeEvent(trayPanelCloseRequestEvent, () => cb())) as unknown as LemonSSHBridge["onTrayPanelCloseRequest"],
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
    }) as unknown as LemonSSHBridge["extractLocalArchive"],
    drainDeepLinks: (async () => {
      const pending = await bindings.deepLink?.Drain?.() ?? [];
      await bindings.deepLink?.Ready?.();
      return pending;
    }) as unknown as LemonSSHBridge["drainDeepLinks"],
    getOSProtocolStatus: (async () => {
      const result = await bindings.deepLink?.GetOSProtocolStatus?.();
      return result ?? { success: false, registered: false, error: "getOSProtocolStatus unavailable" };
    }) as unknown as LemonSSHBridge["getOSProtocolStatus"],
    setOSProtocol: (async (enabled: boolean) => {
      const result = await bindings.deepLink?.SetOSProtocol?.(enabled);
      return result ?? { success: false, registered: false, error: "setOSProtocol unavailable" };
    }) as unknown as LemonSSHBridge["setOSProtocol"],
    onSshDeepLink: ((cb: (payload: { url?: string }) => void) => {
      const eventsOn = bindings.events?.On ?? Events.On;
      if (typeof eventsOn !== "function") return () => undefined;
      return eventsOn("deeplink:ssh", (event) => {
        const data = (event?.data ?? event) as { url?: string; URL?: string; host?: string; Host?: string; port?: string; Port?: string; username?: string; Username?: string };
        const rawUrl = data.url ?? data.URL;
        if (rawUrl) return cb({ url: rawUrl });
        const host = data.host ?? data.Host;
        if (!host) return;
        const username = data.username ?? data.Username;
        const portValue = data.port ?? data.Port;
        cb({ url: `ssh://${username ? `${username}@` : ""}${host}${portValue ? `:${portValue}` : ""}` });
      });
    }) as unknown as LemonSSHBridge["onSshDeepLink"],
    onTelnetDeepLink: ((cb: (payload: { url?: string }) => void) => {
      const eventsOn = bindings.events?.On ?? Events.On;
      if (typeof eventsOn !== 'function') return () => undefined;
      return eventsOn('deeplink:telnet', event => {
        const data = (event?.data ?? event) as { url?: string; URL?: string; host?: string; Host?: string; port?: string; Port?: string };
        const host = data.host ?? data.Host;
        cb({ url: data.url ?? data.URL ?? (host ? `telnet://${host}${(data.port ?? data.Port) ? `:${data.port ?? data.Port}` : ''}` : '') });
      });
    }) as LemonSSHBridge['onTelnetDeepLink'],
    onJmsDeepLink: ((cb: (payload: { url?: string }) => void) => {
      const eventsOn = bindings.events?.On ?? Events.On;
      if (typeof eventsOn !== 'function') return () => undefined;
      return eventsOn('deeplink:jms', event => {
        const data = (event?.data ?? event) as { url?: string; URL?: string };
        cb({ url: data.url ?? data.URL });
      });
    }) as LemonSSHBridge['onJmsDeepLink'],
    onOpenTerminalPath: ((cb: (payload: { path?: string }) => void) => {
      const eventsOn = bindings.events?.On ?? Events.On;
      if (typeof eventsOn !== 'function') return () => undefined;
      return eventsOn('deeplink:open-terminal', event => {
        const data = (event?.data ?? event) as { path?: string; Path?: string };
        cb({ path: data.path ?? data.Path });
      });
    }) as LemonSSHBridge['onOpenTerminalPath'],
    setSshDeepLinkEnabled: (async enabled => bindings.deepLink?.SetSshDeepLinkEnabled?.(enabled) ?? { success: false, enabled: false }) as LemonSSHBridge['setSshDeepLinkEnabled'],
    getSshDeepLinkEnabled: (async () => bindings.deepLink?.GetSshDeepLinkEnabled?.() ?? false) as LemonSSHBridge['getSshDeepLinkEnabled'],
    setJmsDeepLinkEnabled: (async enabled => bindings.deepLink?.SetJmsDeepLinkEnabled?.(enabled) ?? { success: false, enabled: false }) as LemonSSHBridge['setJmsDeepLinkEnabled'],
    getJmsDeepLinkEnabled: (async () => bindings.deepLink?.GetJmsDeepLinkEnabled?.() ?? false) as LemonSSHBridge['getJmsDeepLinkEnabled'],
    setExplorerContextMenuEnabled: (async enabled => bindings.deepLink?.SetExplorerContextMenuEnabled?.(enabled) ?? { success: false, enabled: false, supported: false }) as LemonSSHBridge['setExplorerContextMenuEnabled'],
    getExplorerContextMenuEnabled: (async () => bindings.deepLink?.GetExplorerContextMenuEnabled?.() ?? { success: false, enabled: false, supported: false }) as LemonSSHBridge['getExplorerContextMenuEnabled'],
    setHttpNetworkProxy: (async settings => {
      const result = await bindings.httpNetworkProxy?.Set?.(settings);
      if (!result) return { success: false, settings };
      if (result.error) throw new Error(result.error);
      return { success: result.success !== false, settings: result.settings as typeof settings };
    }) as LemonSSHBridge['setHttpNetworkProxy'],
    getHttpNetworkProxy: (async () => {
      const result = await bindings.httpNetworkProxy?.Get?.();
      return { settings: (result?.settings ?? { mode: 'system', url: '', bypass: '<local>' }) as { mode: 'system' | 'direct' | 'custom'; url: string; bypass: string } };
    }) as LemonSSHBridge['getHttpNetworkProxy'],
    openTerminalPopup: (async (payload) => {
      if (!bindings.popup?.Open) return { success: false, error: "openTerminalPopup unavailable" };
      return bindings.popup.Open(payload);
    }) as unknown as LemonSSHBridge["openTerminalPopup"],
    onTerminalPopupConfig: ((cb) => subscribePopupConfig(
      typeof window === 'undefined' ? '' : window.location.search,
      bindings.popup,
      config => cb(config as import("../../../domain/systemManager/types").TerminalPopupPayload),
    )) as unknown as LemonSSHBridge["onTerminalPopupConfig"],
    // Peer session windows (#/session-window): Open mints the window in Go
    // (SessionWindowService), and the new window pulls its clone payload
    // through the same lease handshake as the terminal popup. The identity
    // lives only in that window's URL, so the main window's subscription is a
    // no-op (no query params).
    openSessionInNewWindow: (async (payload) => {
      if (!bindings.sessionWindow?.Open) return { success: false, error: "openSessionInNewWindow unavailable" };
      return bindings.sessionWindow.Open(payload);
    }) as unknown as LemonSSHBridge["openSessionInNewWindow"],
    onOpenSessionInNewWindow: ((cb) => subscribePopupConfig(
      typeof window === 'undefined' ? '' : window.location.search,
      bindings.sessionWindow,
      config => cb(config as { title: string; sourceSession: import("../../../types").TerminalSession; localShellType?: import("../../../domain/models").TerminalSession['shellType'] }),
      error => console.error('Session window configuration lease failed', error),
      SESSION_WINDOW_LEASE_PARAMS,
    )) as unknown as LemonSSHBridge["onOpenSessionInNewWindow"],
    onKeyboardInteractive,
    respondKeyboardInteractive,
    onPassphraseRequest,
    respondPassphrase,
    respondPassphraseSkip,
    onPassphraseTimeout,
    onPassphraseCancelled,
    onPassphraseAuthFailed,
    onHostKeyVerification,
    respondHostKeyVerification,
    execCommand: (async (options: Parameters<NonNullable<LemonSSHBridge["execCommand"]>>[0]) => {
      if (!bindings.terminal.ExecCommand) missingBridgeMethod("execCommand");
      const args = pickSSHConnectArgs(options);
      return await bindings.terminal.ExecCommand({
        ...args,
        command: options.command,
        timeoutMs: options.timeout ?? 30000,
      }) as unknown as { stdout: string; stderr: string; code: number | null };
    }) as unknown as LemonSSHBridge["execCommand"],
    readKnownHosts: (async () => {
      const read = bindings.knownHosts?.ReadKnownHosts;
      if (!read) missingBridgeMethod("readKnownHosts");
      // "" from Go means "no known_hosts files" — surface as null (scan found
      // nothing) instead of undefined (scan unavailable).
      const content = await read();
      return content ? content : null;
    }) as unknown as LemonSSHBridge["readKnownHosts"],
    selectFile,
    selectDirectory,
    showSaveDialog,
    startPortForward,
    stopPortForward,
    stopPortForwardByRuleId,
    listPortForwards,
    getPortForwardStatus,
    getPortForwardSnapshot,
    // Port-forward runtime subscription protocol (snapshot + ordered events);
    // usePortForwardingState keeps its 4s heartbeat as the recovery fallback.
    subscribePortForwardRuntime,
    unsubscribePortForwardRuntime,
    subscribePortForward,
    onPortForwardStatus,
    onPortForwardRuntime,
    windowMinimize,
    windowMaximize,
    windowClose,
    windowIsMaximized,
    windowIsFullscreen,
    windowFocus: windowFocus as unknown as LemonSSHBridge["windowFocus"],
    // WindowLifecycleService emits these from the main window's Wails
    // events (show / hide / focus) so the renderer can dismiss transient
    // overlays before a hide and recover input focus afterwards.
    onWindowShown: ((cb: () => void) => subscribeNativeEvent(windowShownEvent, cb)) as unknown as LemonSSHBridge["onWindowShown"],
    onWindowWillHide: ((cb: () => void) => subscribeNativeEvent(windowWillHideEvent, cb)) as unknown as LemonSSHBridge["onWindowWillHide"],
    onWindowFocusRequested: ((cb: () => void) => subscribeNativeEvent(windowFocusRequestedEvent, cb)) as unknown as LemonSSHBridge["onWindowFocusRequested"],
    // Quit guard: the Go close hook emits this, the renderer answers with
    // reportDirtyEditorsResult (see useAppStartupEffects).
    onCheckDirtyEditors: ((cb: () => void) => subscribeNativeEvent(windowCheckDirtyEditorsEvent, cb)) as unknown as LemonSSHBridge["onCheckDirtyEditors"],
    reportDirtyEditorsResult: ((hasDirty: boolean) => {
      void bindings.windowLifecycle?.ReportDirtyEditorsResult?.(hasDirty)?.catch(() => undefined);
    }) as unknown as LemonSSHBridge["reportDirtyEditorsResult"],
    setCloseToTray: (async (enabled: boolean) => {
      const result = await bindings.windowLifecycle?.SetCloseToTray?.(enabled);
      return { success: result?.success ?? false, enabled: result?.enabled ?? enabled };
    }) as unknown as LemonSSHBridge["setCloseToTray"],
    isCloseToTray: (async () => {
      const result = await bindings.windowLifecycle?.IsCloseToTray?.();
      return { enabled: result?.enabled ?? false };
    }) as unknown as LemonSSHBridge["isCloseToTray"],
    // False means "not applied" (unsupported platform / no main window) so
    // callers can treat the apply as skipped instead of silently lying.
    setWindowOpacity: (async (opacity: number) => {
      try {
        return (await bindings.windowLifecycle?.SetWindowOpacity?.(opacity)) ?? false;
      } catch {
        return false;
      }
    }) as unknown as LemonSSHBridge["setWindowOpacity"],
    openSettingsWindow,
    notifySettingsPainted,
    closeSettingsWindow: closeSettingsWindow as unknown as LemonSSHBridge["closeSettingsWindow"],
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
      bindings.appLock?.GetRuntimeState()) as unknown as LemonSSHBridge["getAppLockRuntimeState"],
    reportAppLockActivity: (() =>
      bindings.appLock?.ReportActivity?.()) as unknown as LemonSSHBridge["reportAppLockActivity"],
    setAppLockRuntimeLocked: ((reason: string) =>
      bindings.appLock?.SetRuntimeLocked?.(reason)) as unknown as LemonSSHBridge["setAppLockRuntimeLocked"],
    setAppLockTimeoutMinutes: (async (minutes: number) => {
      if (!bindings.appLock?.SetTimeoutMinutes) throw new Error("App lock timeout setting unavailable");
      return await bindings.appLock.SetTimeoutMinutes(minutes);
    }) as unknown as LemonSSHBridge["setAppLockTimeoutMinutes"],
    onAppLockRuntimeStateChanged: ((cb: (state: unknown) => void) => {
      return subscribeNativeEvent(appLockRuntimeStateChangedEvent, cb);
    }) as unknown as LemonSSHBridge["onAppLockRuntimeStateChanged"],
    onAppLockSettingsChanged: ((cb: (settings: unknown) => void) => {
      return subscribeNativeEvent(appLockSettingsChangedEvent, cb);
    }) as unknown as LemonSSHBridge["onAppLockSettingsChanged"],
    onAppLockReopen: ((listener: () => void) => {
      return subscribeNativeEvent(appLockReopenEvent, listener);
    }) as unknown as LemonSSHBridge["onAppLockReopen"],
    onVaultBackupsChanged: ((handler: () => void) => {
      return subscribeNativeEvent(vaultBackupsChangedEvent, handler);
    }) as unknown as LemonSSHBridge["onVaultBackupsChanged"],
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
      Promise.resolve(bindings.plugins?.List() ?? [])) as unknown as LemonSSHBridge["listPlugins"],
    ...createPluginBridge(bindings.plugins, {
      subscribeEvent: subscribeNativeEvent,
      attachDataPlane,
    }),
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
    }) as unknown as LemonSSHBridge["requestAppLockPasswordChange"],
    requestAppLockUnlock: (async (password: string) => {
      if (!password) return { ok: false, error: "empty" };
      try {
        await bindings.appLock?.Unlock?.(password);
        return { ok: true };
      } catch {
        return { ok: false, error: "incorrect" };
      }
    }) as unknown as LemonSSHBridge["requestAppLockUnlock"],
    requestAppLockDisable: (async (currentPassword: string) => {
      try {
        if (!bindings.appLock?.Disable) throw new Error("App lock unavailable");
        await bindings.appLock.Disable(currentPassword);
        // Report the authoritative settings: the persisted idle timeout
        // survives a disable, so a hardcoded default would lie in the UI.
        if (bindings.appLock.GetSettings) return await bindings.appLock.GetSettings();
        return { enabled: false, timeoutMinutes: 15, systemUnlockEnabled: false, systemUnlockAutoPromptEnabled: false, passwordVerifier: null };
      } catch {
        return { ok: false, error: "incorrect" };
      }
    }) as unknown as LemonSSHBridge["requestAppLockDisable"],
    // Lock-screen recovery: verifies the current password, removes the App
    // Lock verifier plus system-unlock settings and unlocks the app. The Go
    // Reset method answers with the typed mutation codes ("empty-current",
    // "incorrect") this mapping relays to the renderer.
    requestAppLockReset: (async (currentPassword: string) => {
      try {
        if (!bindings.appLock?.Reset) throw new Error("App lock unavailable");
        if (!currentPassword) return { ok: false as const, error: "empty-current" as const };
        await bindings.appLock.Reset(currentPassword);
        if (bindings.appLock.GetSettings) return await bindings.appLock.GetSettings();
        return { enabled: false, timeoutMinutes: 15, systemUnlockEnabled: false, systemUnlockAutoPromptEnabled: false, passwordVerifier: null };
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        const code = (["empty-current", "incorrect"] as const).find((value) => message.includes(value));
        return { ok: false as const, error: code ?? "incorrect" };
      }
    }) as unknown as LemonSSHBridge["requestAppLockReset"],
    // First-time enable: creates the App Lock verifier from a new password
    // (Go derives "enabled" from the verifier, so there is no flag-only
    // enable). First-time setup in the settings UI goes through
    // requestAppLockPasswordChange; this maps the standalone verb.
    requestAppLockEnable: (async (password: string) => {
      if (!password) return { ok: false as const, error: "empty-next" as const };
      try {
        if (!bindings.appLock?.Enable || !bindings.appLock.GetSettings) throw new Error("App lock unavailable");
        await bindings.appLock.Enable(password);
        return await bindings.appLock.GetSettings();
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        const code = (["empty-current", "incorrect"] as const).find((value) => message.includes(value));
        return { ok: false as const, error: code ?? "incorrect" };
      }
    }) as unknown as LemonSSHBridge["requestAppLockEnable"],
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
    }) as unknown as LemonSSHBridge["cancelZmodem"],
    sendSerialYmodem: (async (sessionId: string, filePath: string) => {
      if (!bindings.terminal.SendSerialYmodem) return { success: false, error: "sendSerialYmodem unavailable" };
      try {
        const result = await bindings.terminal.SendSerialYmodem(sessionId, filePath);
        return { success: true, fileName: result.fileName, totalBytes: result.totalBytes, writtenBytes: result.writtenBytes };
      } catch (error) {
        return { success: false, error: error instanceof Error ? error.message : String(error) };
      }
    }) as unknown as LemonSSHBridge["sendSerialYmodem"],
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
    }) as unknown as LemonSSHBridge["receiveSerialYmodem"],
    extractSftpArchive: async (sftpId: string, remotePath: string, encoding?: SftpFilenameEncoding) => {
      if (!bindings.sftp.ExtractArchive) return { success: false };
      await bindings.sftp.ExtractArchive(sftpId, remotePath, encoding);
      return { success: true };
    },
    getTempDirInfo: async () => { if (!bindings.filesystem?.TempInfo) missingBridgeMethod('getTempDirInfo'); return bindings.filesystem.TempInfo(); },
    getTempDirPath: async () => { if (!bindings.filesystem?.TempInfo) missingBridgeMethod('getTempDirPath'); return (await bindings.filesystem.TempInfo()).path; },
    clearTempDir: async () => { if (!bindings.filesystem?.ClearTemp) missingBridgeMethod('clearTempDir'); const result=await bindings.filesystem.ClearTemp(); return { deletedCount:result.deletedCount, failedCount:0 }; },
    // Renderer tool-output spill persistence (managed temp service). These
    // fail closed to "not durable" instead of throwing: the harness
    // ToolOutputStore treats absence as keep-in-memory, never as success.
    getToolOutputPersistenceStatus: (async () => {
      if (!bindings.filesystem?.ToolOutputPersistenceStatus) {
        return { durable: false, reason: "Tool output persistence is unavailable in this shell." };
      }
      return bindings.filesystem.ToolOutputPersistenceStatus();
    }) as unknown as LemonSSHBridge["getToolOutputPersistenceStatus"],
    writeToolOutputTemp: (async (record: unknown, content: string) => {
      if (!bindings.filesystem?.WriteToolOutputTemp) return { ok: false, error: "unavailable" };
      return bindings.filesystem.WriteToolOutputTemp(record, content);
    }) as unknown as LemonSSHBridge["writeToolOutputTemp"],
    restoreToolOutputTemp: (async (handleId: string, chatSessionId: string) => {
      if (!bindings.filesystem?.RestoreToolOutputTemp) return null;
      return bindings.filesystem.RestoreToolOutputTemp(handleId, chatSessionId);
    }) as unknown as LemonSSHBridge["restoreToolOutputTemp"],
    readToolOutputTemp: (async (filePath: string, request?: unknown) => {
      if (!bindings.filesystem?.ReadToolOutputTemp) return null;
      return bindings.filesystem.ReadToolOutputTemp(filePath, request);
    }) as unknown as LemonSSHBridge["readToolOutputTemp"],
    deleteToolOutputTemp: (async (filePath: string) => {
      if (!bindings.filesystem?.DeleteToolOutputTemp) return { ok: false };
      return bindings.filesystem.DeleteToolOutputTemp(filePath);
    }) as unknown as LemonSSHBridge["deleteToolOutputTemp"],
    deleteChatToolOutputsTemp: (async (chatSessionId: string) => {
      if (!bindings.filesystem?.DeleteChatToolOutputsTemp) return { deletedCount: 0 };
      return bindings.filesystem.DeleteChatToolOutputsTemp(chatSessionId);
    }) as unknown as LemonSSHBridge["deleteChatToolOutputsTemp"],
    deleteTerminalToolOutputsTemp: (async (chatSessionId: string, terminalSessionId: string) => {
      if (!bindings.filesystem?.DeleteTerminalToolOutputsTemp) return { deletedCount: 0 };
      return bindings.filesystem.DeleteTerminalToolOutputsTemp(chatSessionId, terminalSessionId);
    }) as unknown as LemonSSHBridge["deleteTerminalToolOutputsTemp"],
    deleteTerminalToolOutputsEverywhereTemp: (async (terminalSessionId: string) => {
      if (!bindings.filesystem?.DeleteTerminalToolOutputsEverywhereTemp) return { deletedCount: 0 };
      return bindings.filesystem.DeleteTerminalToolOutputsEverywhereTemp(terminalSessionId);
    }) as unknown as LemonSSHBridge["deleteTerminalToolOutputsEverywhereTemp"],
    startCompressedUpload: transfers.startCompressedUpload,
    pauseCompressedUpload: transfers.pauseTransfer,
    resumeCompressedUpload: transfers.resumeTransfer,
    cancelCompressedUpload: async (id: string) => { await transfers.cancelTransfer(id); return { success: true }; },
    checkCompressedUploadSupport: (async () => ({ supported: Boolean((bindings.transfer as TransferBindings | undefined)?.StartCompressed), localTar: false, remoteTar: false })) as unknown as LemonSSHBridge["checkCompressedUploadSupport"],
    registerGlobalHotkey: (async (hotkey: string) => {
      if (!bindings.shortcuts?.Register) return { success: false, error: "registerGlobalHotkey unavailable" };
      return bindings.shortcuts.Register(hotkey);
    }) as unknown as LemonSSHBridge["registerGlobalHotkey"],
    unregisterGlobalHotkey: (async () => {
      if (!bindings.shortcuts?.Unregister) return { success: false };
      return bindings.shortcuts.Unregister();
    }) as unknown as LemonSSHBridge["unregisterGlobalHotkey"],
    getGlobalHotkeyStatus: (async () => {
      if (!bindings.shortcuts?.Status) return { enabled: false, hotkey: null };
      return bindings.shortcuts.Status();
    }) as unknown as LemonSSHBridge["getGlobalHotkeyStatus"],
    getAppInfo: (async () => {
      if (!bindings.lemonssh?.Version) missingBridgeMethod("getAppInfo");
      const info = await bindings.lemonssh.Version();
      return { name: info.name, version: info.version, platform: info.goos };
    }) as unknown as LemonSSHBridge["getAppInfo"],
    checkForUpdate: (async () => {
      if (!bindings.update?.CheckForUpdate) {
        return { available: false, supported: false, error: "Update bridge unavailable" };
      }
      const result = await bindings.update.CheckForUpdate();
      return {
        available: result.available,
        supported: result.supported ?? true,
        checking: result.checking,
        ready: result.ready,
        downloading: result.downloading,
        version: result.version,
        releaseNotes: result.releaseNotes,
        releaseDate: result.releaseDate,
        error: result.error,
      };
    }) as unknown as LemonSSHBridge["checkForUpdate"],
    downloadUpdate: (async () => {
      if (!bindings.update?.DownloadUpdate) return { success: false, error: "Update bridge unavailable" };
      return bindings.update.DownloadUpdate();
    }) as unknown as LemonSSHBridge["downloadUpdate"],
    installUpdate: (() => {
      // Fire-and-forget on the Go side: the process swaps its binary and
      // quits; the relaunch is scheduled by the update service.
      void bindings.update?.InstallUpdate?.();
    }) as unknown as LemonSSHBridge["installUpdate"],
    getUpdateStatus: (async () => {
      if (!bindings.update?.GetUpdateStatus) {
        return { status: "idle", percent: 0, error: null, version: null };
      }
      const snapshot = await bindings.update.GetUpdateStatus();
      return {
        status: snapshot.status,
        percent: snapshot.percent,
        error: snapshot.error || null,
        version: snapshot.version || null,
        isChecking: snapshot.isChecking,
      };
    }) as unknown as LemonSSHBridge["getUpdateStatus"],
    getAutoUpdate: (async () => {
      if (!bindings.update?.GetAutoUpdate) return { enabled: true };
      return bindings.update.GetAutoUpdate();
    }) as unknown as LemonSSHBridge["getAutoUpdate"],
    setAutoUpdate: (async (enabled: boolean) => {
      if (!bindings.update?.SetAutoUpdate) return { success: false };
      return bindings.update.SetAutoUpdate(enabled);
    }) as unknown as LemonSSHBridge["setAutoUpdate"],
    onUpdateAvailable: ((cb: (info: { version: string; releaseNotes: string; releaseDate: string | null }) => void) => {
      return subscribeNativeEvent(updateAvailableEvent, cb);
    }) as unknown as LemonSSHBridge["onUpdateAvailable"],
    onUpdateNotAvailable: ((cb: () => void) => {
      return subscribeNativeEvent(updateNotAvailableEvent, cb);
    }) as unknown as LemonSSHBridge["onUpdateNotAvailable"],
    onUpdateDownloadProgress: ((cb: (progress: { percent: number; bytesPerSecond: number; transferred: number; total: number }) => void) => {
      return subscribeNativeEvent(updateDownloadProgressEvent, cb);
    }) as unknown as LemonSSHBridge["onUpdateDownloadProgress"],
    onUpdateDownloaded: ((cb: () => void) => {
      return subscribeNativeEvent(updateDownloadedEvent, cb);
    }) as unknown as LemonSSHBridge["onUpdateDownloaded"],
    onUpdateError: ((cb: (payload: { error: string }) => void) => {
      return subscribeNativeEvent(updateErrorEvent, cb);
    }) as unknown as LemonSSHBridge["onUpdateError"],
    // The Go shell cannot see renderer-side unsaved editors, so it never
    // emits this; the surface stays available for the #1215 contract.
    onUpdateNeedsSave: ((cb: () => void) => subscribeNativeEvent("update:needs-save", cb)) as unknown as LemonSSHBridge["onUpdateNeedsSave"],
    startMoshSession: (async (options: Parameters<NonNullable<LemonSSHBridge["startMoshSession"]>>[0]) => {
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
        sessionId: options.sessionId,
        agentForwarding: options.agentForwarding,
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
    }) as unknown as LemonSSHBridge["startMoshSession"],
    startEtSession: (async (options: Parameters<NonNullable<LemonSSHBridge["startEtSession"]>>[0]) => {
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
        sessionId: options.sessionId,
        agentForwarding: options.agentForwarding,
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
    }) as unknown as LemonSSHBridge["startEtSession"],
    getTelnetEchoMode: (async (sessionId: string) => {
      if (!bindings.terminal.GetTelnetEchoMode) {
        return { success: false, error: "getTelnetEchoMode unavailable" };
      }
      return bindings.terminal.GetTelnetEchoMode(sessionId);
    }) as unknown as LemonSSHBridge["getTelnetEchoMode"],
  };
  const transitionBridge = new Proxy(implementedBridge, {
    get(target, property, receiver) {
      if (property in target) return Reflect.get(target, property, receiver);
      // Optional-chain callers (bridge?.setLanguage?.()) must see undefined,
      // not a throwing function. A throwing stub crashes first paint.
      return undefined;
    },
  }) as LemonSSHBridge;

  const client: WailsRuntimeClient = {
    app: portWith("app", {
      quitApp: async () => {
        await bindings.tray?.Quit?.();
      },
    }),
    agent: portWith("agent", implementedBridge),
    agentRuntime: buildAgentRuntimePort(bindings),
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
      onScriptDialogRequest,
      scriptDialogResponse,
    }),
    terminal: portWith("terminal", {
      getDefaultShell,
      discoverShells,
      validatePath,
      generateKeyPair: implementedBridge.generateKeyPair,
      checkSshAgent: implementedBridge.checkSshAgent,
      getDefaultKeys: implementedBridge.getDefaultKeys,
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
      setSessionEncoding,
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
      readSftpBinary,
      writeSftp,
      writeSftpBinary,
      realpathSftp,
      lstatSftp,
      retainSftpTransferSession,
      releaseSftpTransferSession,
      sameHostCopyDirectory,
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
      }) as unknown as LemonSSHBridge["cloudSyncWebdavInitialize"],
      cloudSyncWebdavUpload: (async (config: unknown, syncedFile: unknown) => {
        if (!bindings.sync?.CloudSyncWebdavUpload) missingBridgeMethod("cloudSyncWebdavUpload");
        return bindings.sync.CloudSyncWebdavUpload(config, syncedFile);
      }) as unknown as LemonSSHBridge["cloudSyncWebdavUpload"],
      cloudSyncWebdavDownload: (async (config: unknown) => {
        if (!bindings.sync?.CloudSyncWebdavDownload) missingBridgeMethod("cloudSyncWebdavDownload");
        return bindings.sync.CloudSyncWebdavDownload(config);
      }) as unknown as LemonSSHBridge["cloudSyncWebdavDownload"],
      cloudSyncWebdavDelete: (async (config: unknown) => {
        if (!bindings.sync?.CloudSyncWebdavDelete) missingBridgeMethod("cloudSyncWebdavDelete");
        return bindings.sync.CloudSyncWebdavDelete(config);
      }) as unknown as LemonSSHBridge["cloudSyncWebdavDelete"],
      cloudSyncS3Initialize: (async (config: unknown) => {
        if (!bindings.sync?.CloudSyncS3Initialize) missingBridgeMethod("cloudSyncS3Initialize");
        return bindings.sync.CloudSyncS3Initialize(config);
      }) as unknown as LemonSSHBridge["cloudSyncS3Initialize"],
      cloudSyncS3Upload: (async (config: unknown, syncedFile: unknown) => {
        if (!bindings.sync?.CloudSyncS3Upload) missingBridgeMethod("cloudSyncS3Upload");
        return bindings.sync.CloudSyncS3Upload(config, syncedFile);
      }) as unknown as LemonSSHBridge["cloudSyncS3Upload"],
      cloudSyncS3Download: (async (config: unknown) => {
        if (!bindings.sync?.CloudSyncS3Download) missingBridgeMethod("cloudSyncS3Download");
        return bindings.sync.CloudSyncS3Download(config);
      }) as unknown as LemonSSHBridge["cloudSyncS3Download"],
      cloudSyncS3Delete: (async (config: unknown) => {
        if (!bindings.sync?.CloudSyncS3Delete) missingBridgeMethod("cloudSyncS3Delete");
        return bindings.sync.CloudSyncS3Delete(config);
      }) as unknown as LemonSSHBridge["cloudSyncS3Delete"],
    }),
    system: portWith("system", monitoring),
    plugin: portWith("plugin", implementedBridge),
    transitionBridge,
  };
  // Keep domain ports aligned with the same native implementations used by
  // aggregate callers; a method must not vanish when a caller narrows its port.
  const complete = <T extends object>(name: string, port: T): T => portWith<T>(name, { ...implementedBridge, ...port });
  return {
    ...client,
    app: complete('app', client.app), agent: complete('agent', client.agent),
    files: complete('files', client.files), script: complete('script', client.script),
    terminal: complete('terminal', client.terminal), sftp: complete('sftp', client.sftp),
    sync: complete('sync', client.sync), system: complete('system', client.system),
    plugin: complete('plugin', client.plugin),
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
      download: (sftpID: string, remotePath: string, localPath: string, encoding?: string) =>
        sftpService.Download(sftpID, remotePath, localPath, encoding ?? ""),
      upload: (sftpID: string, localPath: string, remotePath: string, encoding?: string) =>
        sftpService.Upload(sftpID, localPath, remotePath, encoding ?? ""),
    },
  };
}

export { lemonsshService };

export function installWailsRuntimeClient(): boolean {
  if (!isWailsRuntime()) return false;
  // Go alpha.63 calls this global after resolving native file paths. The npm
  // runtime only installs _wails.handlePlatformFileDrop; reuse its Window here.
  // Remove the alias once Go uses that newer entry point too.
  const host = window as typeof window & { wails?: { Window?: typeof wailsWindow }; lemonssh?: LemonSSHBridge };
  host.wails ??= {};
  host.wails.Window = wailsWindow;
  configureProfileBindings(profileBindings);
  const client = createWailsRuntimeClient();
  // Legacy consumers read window.lemonssh directly (getLemonSSHBridge in
  // aiChatStreamingSupport, AIChatSidePanel, the settings AI tab, ...). The
  // transition bridge is the same fail-closed surface: implemented methods
  // call Go bindings, unimplemented ones stay undefined.
  host.lemonssh ??= client.transitionBridge;
  setActiveRuntimeClient(client);
  return true;
}
