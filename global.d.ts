/// <reference path="./types/global/lemonssh-bridge-session.d.ts" />
/// <reference path="./types/global/lemonssh-bridge-sftp.d.ts" />
/// <reference path="./types/global/lemonssh-bridge-sync.d.ts" />
/// <reference path="./types/global/lemonssh-bridge-files.d.ts" />
/// <reference path="./types/global/lemonssh-bridge-ai.d.ts" />
/// <reference path="./types/global/lemonssh-bridge-app.d.ts" />
/// <reference path="./types/global/lemonssh-bridge-system.d.ts" />
/// <reference path="./types/global/lemonssh-bridge-script.d.ts" />
declare module "*.cjs" {
  const value: Record<string, unknown>;
  export = value;
}

declare module 'react' {
  interface InputHTMLAttributes<T> extends HTMLAttributes<T> {
    webkitdirectory?: string | boolean;
  }
}

declare global {
  // Proxy configuration for SSH connections
  interface LemonSSHProxyConfig {
    type: 'http' | 'socks5' | 'command';
    host: string;
    port: number;
    command?: string;
    username?: string;
    password?: string;
  }

  // Discovered local shell (e.g. CMD, PowerShell, WSL, Git Bash)
  interface DiscoveredShell {
    id: string;
    name: string;
    command: string;
    args?: string[];
    icon: string;
    isDefault?: boolean;
  }

  // Jump host configuration for SSH tunneling
  interface LemonSSHJumpHost {
    hostname: string;
    hostId?: string;
    port: number;
    username: string;
    authMethod?: import("./domain/models").HostAuthMethod;
    requiresMfa?: boolean;
    password?: string;
    privateKey?: string;
    certificate?: string;
    passphrase?: string;
    publicKey?: string;
    keyId?: string;
    keySource?: 'generated' | 'imported' | 'reference';
    label?: string; // Display label for UI
    proxy?: LemonSSHProxyConfig;
    identityFilePaths?: string[];
    useSshAgent?: boolean;
    agentPublicKeys?: string[];
    identityAgent?: string;
    identitiesOnly?: boolean;
    addKeysToAgent?: string;
    useKeychain?: boolean;
    // ET server port on this hop, used only when ET tunnels through it as a
    // jump host (--jport). Defaults to 2022 in the bridge when omitted.
    etPort?: number;
    // Resolved keepalive for THIS hop (caller has already applied host
    // override / global fallback). interval in seconds, 0 = disabled.
    keepaliveInterval?: number;
    keepaliveCountMax?: number;
    // Per-hop SSH connection timeouts, resolved from the saved host.
    sshTcpConnectTimeoutMs?: number;
    sshAuthReadyTimeoutMs?: number;
    verifyHostKeys?: boolean;
    // Per-hop algorithm settings, mirroring the target-host fields. When
    // omitted the bridge falls back to the target host's settings so a
    // single setting on the leaf still covers the chain (matches the
    // pre-existing behavior of `legacyAlgorithms`).
    legacyAlgorithms?: boolean;
    skipEcdsaHostKey?: boolean;
    algorithmOverrides?: import("./domain/models").HostAlgorithmOverrides;
  }

  // Host key information for verification
  // Reserved for future host key verification UI feature
  interface _LemonSSHHostKeyInfo {
    hostname: string;
    port: number;
    keyType: string;
    fingerprint: string;
    publicKey?: string;
  }

  interface LemonSSHSSHOptions {
    sessionId?: string;
    hostId?: string;
    hostLabel?: string;
    hostname: string;
    username: string;
    authMethod?: import("./domain/models").HostAuthMethod;
    requiresMfa?: boolean;
    port?: number;
    password?: string;
    privateKey?: string;
    // Optional OpenSSH user certificate
    certificate?: string;
    publicKey?: string; // OpenSSH public key line
    keyId?: string;
    keySource?: 'generated' | 'imported' | 'reference';
    agentForwarding?: boolean;
    x11Forwarding?: boolean;
    x11Display?: string;
    cols?: number;
    rows?: number;
    charset?: string;
    extraArgs?: string[];
    startupCommand?: string;
    passphrase?: string;
    knownHosts?: import("./domain/models").KnownHost[];
    verifyHostKeys?: boolean;
    // Environment variables to set in the remote shell
    env?: Record<string, string>;
    // Proxy configuration
    proxy?: LemonSSHProxyConfig;
    // Jump hosts (bastion chain)
    jumpHosts?: LemonSSHJumpHost[];
    // SSH-level keepalive interval in seconds (0 = disabled)
    keepaliveInterval?: number;
    // Unanswered keepalives before ssh2 declares the connection dead
    keepaliveCountMax?: number;
    // Maximum time to establish the TCP connection
    sshTcpConnectTimeoutMs?: number;
    // Maximum time for SSH handshake and authentication
    sshAuthReadyTimeoutMs?: number;
    // Enable legacy SSH algorithms for older network equipment
    legacyAlgorithms?: boolean;
    // Drop ecdsa-sha2-* from offered host-key algorithms (#1027)
    skipEcdsaHostKey?: boolean;
    // Per-category algorithm override lists (advanced, see HostAlgorithmOverrides)
    algorithmOverrides?: import("./domain/models").HostAlgorithmOverrides;
    // Use sudo for SFTP server
    sudo?: boolean;
    // Remote file protocol: auto (SFTP then SCP fallback) | sftp | scp
    fileProtocol?: 'auto' | 'sftp' | 'scp';
    // Saved host password used by background system tools when they need sudo.
    sudoAutofillPassword?: string;
    // Session log configuration for real-time streaming
    sessionLog?: { enabled: boolean; directory: string; format: string; timestampsEnabled?: boolean };
    // SSH connection diagnostics. Does not capture terminal output.
    sshDebugLogEnabled?: boolean;
    // Boot generation for correlating host-key prompts with a terminal start.
    bootEpoch?: number;
    // Local SSH key file paths (from SSH config IdentityFile)
    identityFilePaths?: string[];
    useSshAgent?: boolean;
    agentPublicKeys?: string[];
    identityAgent?: string;
    identitiesOnly?: boolean;
    addKeysToAgent?: string;
    useKeychain?: boolean;
    // When set, reuse the already-authenticated SSH connection of this existing
    // session by opening a new shell channel on it, instead of dialing a fresh
    // connection. Lets a duplicated tab skip a second MFA prompt (issue #1204).
    // The bridge falls back to a fresh connection if the source is gone.
    sourceSessionId?: string;
    // Skip POSIX process discovery when copying a network-device session.
    skipShellPidDiscovery?: boolean;
    /**
     * When false, terminal/SFTP opens must dial a fresh SSH connection and
     * must not borrow a live, parked, or in-flight registry transport. Used by
     * connect-time terminal automation and dedicated bulk transfers.
     * Default true (normal terminal, browse, and MFA-skip reuse).
     */
    reuseTransport?: boolean;
  }

  interface SftpStatResult {
    name: string;
    type: 'file' | 'directory' | 'symlink';
    size: number;
    lastModified: number; // timestamp
    permissions?: string; // e.g., "rwxr-xr-x"
    owner?: string;
    group?: string;
  }

  interface SftpTransferProgress {
    transferId: string;
    bytesTransferred: number;
    totalBytes: number;
    speed: number; // bytes per second
  }

  // Port Forwarding Types
  interface PortForwardOptions {
    ruleId?: string;
    tunnelId: string;
    type: 'local' | 'remote' | 'dynamic';
    localPort: number;
    bindAddress?: string;
    remoteHost?: string;
    remotePort?: number;
    // SSH connection details
    hostname: string;
    hostId?: string;
    port?: number;
    username: string;
    authMethod?: import("./domain/models").HostAuthMethod;
    requiresMfa?: boolean;
    password?: string;
    privateKey?: string;
    certificate?: string;
    keyId?: string;
    passphrase?: string;
    knownHosts?: import("./domain/models").KnownHost[];
    verifyHostKeys?: boolean;
    proxy?: LemonSSHProxyConfig;
    jumpHosts?: LemonSSHJumpHost[];
    identityFilePaths?: string[];
    useSshAgent?: boolean;
    agentPublicKeys?: string[];
    identityAgent?: string;
    identitiesOnly?: boolean;
    addKeysToAgent?: string;
    useKeychain?: boolean;
    legacyAlgorithms?: boolean;
    skipEcdsaHostKey?: boolean;
    algorithmOverrides?: import("./domain/models").HostAlgorithmOverrides;
    // Resolved keepalive for the target connection (caller has already
    // applied host override / global fallback). interval in seconds.
    keepaliveInterval?: number;
    keepaliveCountMax?: number;
    sshTcpConnectTimeoutMs?: number;
    sshAuthReadyTimeoutMs?: number;
  }

  interface PortForwardResult {
    tunnelId: string;
    success: boolean;
    cancelled?: boolean;
    blockedByCleanup?: boolean;
    reused?: boolean;
    status?: 'inactive' | 'connecting' | 'active' | 'error';
    error?: string;
  }

  interface PortForwardStatusResult {
    tunnelId: string;
    status: 'inactive' | 'connecting' | 'active' | 'error';
    type?: 'local' | 'remote' | 'dynamic';
    error?: string;
  }

  type PortForwardRuntimePhase =
    | 'connecting'
    | 'active'
    | 'stopping'
    | 'error'
    | 'inactive';

  interface PortForwardRuntimeRecord {
    ruleId?: string;
    tunnelId: string;
    phase: PortForwardRuntimePhase | string;
    error?: string;
    cleanupRequired?: boolean;
    revision: number;
    updatedAt: number;
  }

  interface PortForwardRuntimeSnapshot {
    epoch: string;
    revision: number;
    records: PortForwardRuntimeRecord[];
  }

  type PortForwardRuntimeEvent =
    | {
        epoch: string;
        revision: number;
        kind: 'upsert';
        record: PortForwardRuntimeRecord;
      }
    | {
        epoch: string;
        revision: number;
        kind: 'remove';
        tunnelId: string;
        ruleId?: string;
      };

  interface LemonSSHWindowsPtyInfo {
    backend: 'conpty' | 'winpty';
    buildNumber?: number;
  }

  type PortForwardStatusCallback = (status: 'inactive' | 'connecting' | 'active' | 'error', error?: string) => void;
  type PortForwardRuntimeEventCallback = (event: PortForwardRuntimeEvent) => void;

  interface LemonSSHPluginRuntimeStatus {
    available: boolean;
    experimental: true;
  }

  interface LemonSSHInstalledPlugin {
    id: string;
    enabled: boolean;
    activeVersion: string | null;
    manifest: unknown;
    runtime: {
      status: string;
      kind: 'browser' | 'utility' | null;
      lastError: string | null;
      quarantinedAt: number | null;
    };
  }

  interface LemonSSHExtensionProviderContribution {
    pluginId: string;
    pluginVersion: string;
    pluginDisplayName?: string;
    provider: import("@lemonssh/plugin-contract").ProviderContribution;
  }

  interface LemonSSHExtensionProviderRequest {
    providerId: string;
    kind: 'connection' | 'authentication' | 'importer';
    operation: string;
    requestId?: string;
    payload?: import("@lemonssh/plugin-contract").JsonValue;
    deadlineMs?: number;
  }

  interface LemonSSHPluginConnectionStartRequest {
    requestId?: string;
    sessionId: string;
    protocol?: string;
    hostLabel?: string;
    hostname?: string;
    providerId: string;
    configuration: import("@lemonssh/plugin-contract").JsonValue;
    columns: number;
    rows: number;
    credential?: import("@lemonssh/plugin-contract").CredentialRef | import("@lemonssh/plugin-contract").SecretRef;
    authenticationProviderId?: string;
    sessionLog?: { enabled: boolean; directory: string; format: string; timestampsEnabled?: boolean };
    deadlineMs?: number;
  }

  interface LemonSSHPluginImporterPreview {
    providerId: string;
    result: import("@lemonssh/plugin-contract").ImporterParseResult;
    records: ReadonlyArray<import("@lemonssh/plugin-contract").ImporterRecord>;
  }

  interface LemonSSHPluginImporterProgressEvent {
    requestId: string;
    providerId: string;
    progress: Extract<import("@lemonssh/plugin-contract").ImporterRecord, { type: "progress" }>;
  }

  interface LemonSSHPluginAuthenticationChallengeOpenEvent {
    requestId: string;
    challengeRequestId: string;
    challenge: import("@lemonssh/plugin-contract").AuthenticationChallenge;
  }

  interface LemonSSHPluginAuthenticationChallengeCancelEvent {
    requestId: string;
    challengeRequestId: string;
    challengeId?: string;
    cancelled: true;
  }

  type LemonSSHPluginAuthenticationChallengeEvent =
    | LemonSSHPluginAuthenticationChallengeOpenEvent
    | LemonSSHPluginAuthenticationChallengeCancelEvent;

  interface LemonSSHBridge {
    getPluginRuntimeStatus?(): Promise<LemonSSHPluginRuntimeStatus>;
    listPlugins?(): Promise<LemonSSHInstalledPlugin[]>;
    installPluginPackage?(archivePath: string, options?: { enable?: boolean }): Promise<LemonSSHInstalledPlugin>;
    setPluginEnabled?(pluginId: string, enabled: boolean): Promise<LemonSSHInstalledPlugin>;
    restartPlugin?(pluginId: string): Promise<LemonSSHInstalledPlugin>;
    uninstallPlugin?(pluginId: string): Promise<boolean>;
    getPluginContributions?(options?: LemonSSHPluginContributionQuery): Promise<LemonSSHPluginContributionSnapshot>;
    getPluginContributionIcon?(pluginId: string, icon: Extract<LemonSSHPluginIconReference, { kind: 'package' }>): Promise<{ light: string; dark?: string }>;
    executePluginCommand?(command: string, args?: unknown, context?: Record<string, unknown>): Promise<unknown>;
    /**
     * Pulls one declarative view's ViewDef.Bindings data from an enabled
     * plugin through the lemonssh-wasm-abi v1 dispatch channel ("view.data");
     * declared non-secret settings values are the fallback and the base layer
     * of the merged result. See docs/plugin-platform/ui-contributions.md.
     */
    getPluginViewData?(pluginId: string, viewId: string, bindings: ReadonlyArray<string>): Promise<{ source: 'plugin' | 'settings'; data: Record<string, unknown> }>;
    updatePluginSetting?(pluginId: string, settingId: string, value: unknown, scopeId?: string): Promise<{ restartRequired: boolean }>;
    resetPluginSetting?(pluginId: string, settingId: string, scopeId?: string): Promise<{ restartRequired: boolean }>;
    setPluginEnvironment?(environment: LemonSSHPluginEnvironment): Promise<void>;
    listPluginTerminalProviders?(options: LemonSSHTerminalProviderQuery): Promise<ReadonlyArray<LemonSSHTerminalProviderContribution>>;
    providePluginTerminal?(request: LemonSSHTerminalProviderRequest): Promise<ReadonlyArray<LemonSSHTerminalProviderResult>>;
    cancelPluginTerminalRequest?(requestId: string): Promise<boolean>;
    publishPluginTerminalSessionEvent?(event: LemonSSHTerminalSessionEvent): Promise<ReadonlyArray<{ pluginId: string; delivered: boolean }>>;
    listPluginExtensionProviders?(options: { kind: 'connection' | 'authentication' | 'importer' | 'sync'; locale?: string }): Promise<ReadonlyArray<LemonSSHExtensionProviderContribution>>;
    updatePluginCredentialCatalog?(entries: ReadonlyArray<{ id: string; ciphertext: string }>): Promise<number>;
    invokePluginExtensionProvider?(request: LemonSSHExtensionProviderRequest): Promise<import("@lemonssh/plugin-contract").JsonValue>;
    cancelPluginExtensionRequest?(requestId: string): Promise<boolean>;
    pluginSyncConnect?(request: {
      requestId?: string;
      providerId: string;
      configuration?: unknown;
      credential?: unknown;
      deadlineMs?: number;
    }): Promise<{ account: { id: string; email?: string; name?: string; avatarUrl?: string } }>;
    pluginSyncDisconnect?(request: { requestId?: string; providerId: string; deadlineMs?: number }): Promise<null>;
    pluginSyncGetAccount?(request: { requestId?: string; providerId: string; deadlineMs?: number }): Promise<{
      account: { id: string; email?: string; name?: string; avatarUrl?: string } | null;
    }>;
    pluginSyncGetCapabilities?(request: { requestId?: string; providerId: string; deadlineMs?: number }): Promise<{
      revisions: boolean;
      conditionalWrites: boolean;
      atomicReplacement: boolean;
      maxObjectBytes?: number;
      maxObjects?: number;
    }>;
    pluginSyncReadObject?(request: {
      requestId?: string;
      providerId: string;
      key: string;
      preferStream?: boolean;
      deadlineMs?: number;
    }): Promise<{
      found: boolean;
      key: string;
      data?: Uint8Array | null;
      streamed?: boolean;
      transferId?: string;
      byteLength?: number;
      revision?: string;
      contentType?: string;
    }>;
    pluginSyncReadChunk?(request: {
      requestId: string;
      transferId: string;
      maxBytes?: number;
    }): Promise<{ chunk: Uint8Array; done: boolean }>;
    pluginSyncWriteObject?(request: {
      requestId?: string;
      providerId: string;
      key: string;
      data: Uint8Array;
      expectedRevision?: string | null;
      preferStream?: boolean;
      deadlineMs?: number;
    }): Promise<{ created: boolean; revision?: string }>;
    pluginSyncWriteBegin?(request: {
      requestId?: string;
      providerId: string;
      key: string;
      byteLength: number;
      expectedRevision?: string | null;
      deadlineMs?: number;
    }): Promise<{ transferId: string; windowBytes: number }>;
    pluginSyncWriteChunk?(request: {
      requestId: string;
      transferId: string;
      sequence: number;
      chunk: Uint8Array;
    }): Promise<{ accepted: number }>;
    pluginSyncWriteCommit?(request: {
      requestId: string;
      transferId: string;
    }): Promise<{ created: boolean; revision?: string }>;
    pluginSyncDeleteObject?(request: {
      requestId?: string;
      providerId: string;
      key: string;
      expectedRevision?: string;
      deadlineMs?: number;
    }): Promise<{ deleted: boolean }>;
    pluginSyncPutSecret?(request: {
      providerId: string;
      key: string;
      value: string;
    }): Promise<{ kind: 'secret'; id: string; key: string; created?: boolean }>;
    pluginSyncDeleteSecrets?(request: {
      providerId: string;
      keys?: string[];
    }): Promise<{ deleted: number }>;
    pluginSyncRestoreSecrets?(request: {
      providerId: string;
      keys: string[];
      discard?: boolean;
    }): Promise<{ restored: number; discarded?: number }>;
    collectPluginSyncSidecars?(): Promise<unknown>;
    applyPluginSyncSidecars?(bundle: unknown): Promise<{ applied: boolean; count?: number; entries?: unknown }>;
    pluginHostReady?(): boolean;
    startPluginConnection?(request: LemonSSHPluginConnectionStartRequest): Promise<{ sessionId: string; providerId: string; status: 'connecting' | 'connected'; diagnostics: ReadonlyArray<import("@lemonssh/plugin-contract").ProviderValidationIssue> }>;
    writePluginConnection?(sessionId: string, data: Uint8Array): Promise<void>;
    controlPluginConnection?(sessionId: string, operation: 'resize' | 'signal' | 'reconnect' | 'close' | 'getStatus', payload?: Record<string, unknown>): Promise<unknown>;
    detectPluginImporter?(request: { providerId: string; sample: Uint8Array; fileName?: string; mediaType?: string; deadlineMs?: number }): Promise<import("@lemonssh/plugin-contract").ImporterDetectResult>;
    selectPluginImporterFile?(): Promise<{ selectionToken: string; fileName: string; sample: Uint8Array } | null>;
    releasePluginImporterFile?(selectionToken: string): Promise<boolean>;
    parsePluginImporterFile?(request: { requestId?: string; providerId: string; selectionToken: string; mediaType?: string; options?: import("@lemonssh/plugin-contract").JsonValue; deadlineMs?: number }): Promise<LemonSSHPluginImporterPreview>;
    onPluginImporterProgress?(callback: (event: LemonSSHPluginImporterProgressEvent) => void): () => void;
    respondPluginAuthenticationChallenge?(response: {
      requestId: string;
      challengeRequestId: string;
      challengeId: string;
      response?: string | boolean | ReadonlyArray<string>;
      cancelled?: boolean;
    }): Promise<void>;
    onPluginAuthenticationChallenge?(callback: (event: LemonSSHPluginAuthenticationChallengeEvent) => void): () => void;
    onPluginConnectionData?(callback: (event: { sessionId: string; data: Uint8Array }) => void): () => void;
    onPluginConnectionClosed?(callback: (event: { sessionId: string; reason: string }) => void): () => void;
    openPluginView?(payload: LemonSSHPluginViewOpenRequest): Promise<{ instanceId: string }>;
    closePluginView?(instanceId: string): Promise<void>;
    setPluginViewBounds?(instanceId: string, bounds: { x: number; y: number; width: number; height: number }): Promise<void>;
    setPluginViewVisibility?(instanceId: string, visible: boolean): Promise<void>;
    postPluginViewMessage?(instanceId: string, message: unknown): Promise<void>;
    onPluginContributionsChanged?(callback: (event: { reason: string; pluginId: string | null; revision: number }) => void): () => void;
    onPluginViewMessage?(callback: (event: { pluginId: string; viewId: string; message: unknown }) => void): () => void;
    onPluginViewClosed?(callback: (event: LemonSSHPluginViewClosedEvent) => void): () => void;
    getPluginScopeCatalog?(): Promise<LemonSSHPluginScopeCatalog>;
    setPluginScopeCatalog?(catalog: LemonSSHPluginScopeCatalog): Promise<void>;
    onPluginScopeCatalogChanged?(callback: (catalog: LemonSSHPluginScopeCatalog) => void): () => void;
  }

  interface LemonSSHPluginContributionQuery {
    locale?: string;
    context?: Record<string, unknown>;
    menuContexts?: Partial<Record<string, Record<string, unknown>>>;
    scopeIds?: Partial<Record<'workspace' | 'host' | 'session' | 'device', string>>;
  }

  interface LemonSSHPluginSettingContribution {
    id: string;
    label: string;
    description?: string;
    placeholder?: string;
    control: string;
    scope: string;
    scopeId: string | null;
    value?: unknown;
    secret?: boolean;
    configured: boolean;
    visible: boolean;
    restartRequired?: boolean;
    required?: boolean;
    options?: ReadonlyArray<{ value: string; label: string; description?: string }>;
    minimum?: number;
    maximum?: number;
    step?: number;
    sortable?: boolean;
    valueSchema?: unknown;
  }

  type LemonSSHPluginIconReference =
    | { kind: 'theme'; name: string }
    | { kind: 'package'; light: string; dark?: string };

  interface LemonSSHPluginContributionSnapshot {
    locale: string;
    plugins: ReadonlyArray<{
      id: string;
      version: string;
      displayName: string;
      description: string;
      commands: ReadonlyArray<{ id: string; title: string; category?: string; description?: string; icon?: LemonSSHPluginIconReference; enabled: boolean }>;
      keybindings: ReadonlyArray<{ command: string; key: string; mac?: string; linux?: string; windows?: string; args?: unknown; enabled: boolean }>;
      menus: ReadonlyArray<{
        id: string;
        command: string;
        alt?: string;
        location: string;
        title: string;
        visible: boolean;
        enabled: boolean;
        checked?: boolean;
        order?: number;
        group?: string;
        shortcut?: string;
        showKeybinding?: boolean;
        icon?: LemonSSHPluginIconReference;
      }>;
      settings: ReadonlyArray<LemonSSHPluginSettingContribution>;
      views: ReadonlyArray<{ id: string; title: string; location: string; entry: string; icon?: LemonSSHPluginIconReference; order?: number; visible: boolean; retainContextWhenHidden?: boolean }>;
    }>;
  }

  interface LemonSSHPluginEnvironment {
    locale: string;
    theme: string;
    reducedMotion: boolean;
    highContrast: boolean;
    themeTokens?: Record<string, string>;
  }

  type LemonSSHTerminalProviderKind =
    | 'terminal.completion'
    | 'terminal.decoration'
    | 'terminal.link'
    | 'terminal.hover'
    | 'terminal.matcher'
    | 'terminal.semantic'
    | 'terminal.prompt'
    | 'terminal.background'
    | 'terminal.theme';

  interface LemonSSHTerminalProviderContribution {
    pluginId: string;
    pluginVersion: string;
    runtimeId?: string;
    pluginDisplayName: string;
    provider: {
      id: string;
      label: string;
      description?: string;
      kind: LemonSSHTerminalProviderKind;
      capabilities?: ReadonlyArray<string>;
      configurationSchema?: unknown;
    };
  }

  interface LemonSSHTerminalProviderQuery {
    kind: LemonSSHTerminalProviderKind;
    locale?: string;
    preferredProviderIds?: ReadonlyArray<string>;
  }

  interface LemonSSHTerminalSessionSnapshot {
    sessionId: string;
    hostId?: string;
    workspaceId?: string;
    protocol: string;
    status: 'connecting' | 'connected' | 'disconnected';
    cwd?: string;
    title?: string;
    shellType?: 'posix' | 'fish' | 'powershell' | 'cmd' | 'unknown';
    cols?: number;
    rows?: number;
    alternateScreen?: boolean;
  }

  interface LemonSSHTerminalSessionEvent {
    type:
      | 'snapshot'
      | 'created'
      | 'connected'
      | 'reconnected'
      | 'cwdChanged'
      | 'titleChanged'
      | 'resized'
      | 'alternateScreenChanged'
      | 'commandSubmitted'
      | 'commandCompleted'
      | 'disconnected'
      | 'disposed';
    session: LemonSSHTerminalSessionSnapshot;
    exitCode?: number;
  }

  interface LemonSSHTerminalProviderRequest {
    requestId: string;
    kind: LemonSSHTerminalProviderKind;
    operation: string;
    session: LemonSSHTerminalSessionSnapshot;
    payload?: unknown;
    locale?: string;
    preferredProviderIds?: ReadonlyArray<string>;
    deadlineMs?: number;
  }

  type LemonSSHTerminalProviderResult = {
    pluginId: string;
    pluginVersion: string;
    runtimeId?: string;
    providerId: string;
    kind: LemonSSHTerminalProviderKind;
    requestId: string;
    status: 'ok';
    result: unknown;
  } | {
    pluginId: string;
    pluginVersion: string;
    runtimeId?: string;
    providerId: string;
    kind: LemonSSHTerminalProviderKind;
    requestId: string;
    status: 'cancelled';
  } | {
    pluginId: string;
    pluginVersion: string;
    providerId: string;
    kind: LemonSSHTerminalProviderKind;
    requestId: string;
    status: 'failed';
    error: { code: number; message: string; data?: unknown };
  };

  interface LemonSSHPluginViewOpenRequest {
    viewId: string;
    instanceId?: string;
    scopeId: string;
    bounds?: { x: number; y: number; width: number; height: number };
    context?: Record<string, unknown>;
  }

  interface LemonSSHPluginViewClosedEvent {
    instanceId: string;
    pluginId: string;
    viewId: string;
    reason: string;
  }

  type LemonSSHPluginSettingScopeKind = 'workspace' | 'host' | 'session' | 'device';

  type LemonSSHPluginScopeCatalog = Record<
    LemonSSHPluginSettingScopeKind,
    ReadonlyArray<{ id: string; label: string }>
  >;

  interface Window {
    lemonssh?: LemonSSHBridge;
  }

}

export { };
