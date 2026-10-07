export interface NativePluginRecord {
  pluginId: string;
  version: string;
  manifest: unknown;
  state: string;
  settings?: Record<string, unknown>;
}

/** One declarative view as validated by the Go host (internal/plugin/ui). */
export interface NativePluginUIContributionView {
  id: string;
  type: string;
  title: string;
  location?: string;
  columns?: string[];
  bindings?: string[];
  visible?: boolean;
}

/** One menu entry; `command` is cross-validated against declared contributions. */
export interface NativePluginUIContributionMenu {
  id: string;
  command: string;
  alt?: string;
  location: string;
  title?: string;
  group?: string;
  order?: number;
  visible?: boolean;
}

/** One accelerator bound to a declared plugin command. */
export interface NativePluginUIContributionKeybinding {
  command: string;
  key: string;
  mac?: string;
  linux?: string;
  windows?: string;
  args?: unknown;
  enabled?: boolean;
}

export interface NativePluginUISchema {
  settings?: Array<{ id: string; type: string; label: string; default?: string; options?: string[]; required?: boolean; description?: string }>;
  views?: NativePluginUIContributionView[];
  menus?: NativePluginUIContributionMenu[];
  keybindings?: NativePluginUIContributionKeybinding[];
}

/** One broker-validated provider declaration served by the Go host registry. */
export interface NativePluginProviderDeclaration {
  id: string;
  label: string;
  description?: string;
  kind: string;
  capabilities?: string[];
  configurationSchema?: unknown;
}

/** One plugin's provider contribution as returned by TerminalProviders/ExtensionProviders. */
export interface NativePluginProviderContribution {
  pluginId: string;
  pluginVersion: string;
  pluginDisplayName: string;
  provider: NativePluginProviderDeclaration;
}

export interface NativeTerminalSessionSnapshot {
  sessionId: string;
  hostId?: string;
  workspaceId?: string;
  protocol: string;
  status: string;
  cwd?: string;
  title?: string;
  shellType?: string;
  cols?: number;
  rows?: number;
  alternateScreen?: boolean;
}

export interface NativeTerminalSessionEvent {
  type: string;
  session: NativeTerminalSessionSnapshot;
  exitCode?: number;
}

export interface NativeTerminalProviderRequest {
  requestId: string;
  kind: string;
  operation: string;
  session: NativeTerminalSessionSnapshot;
  payload?: unknown;
  locale?: string;
  preferredProviderIds?: string[];
  deadlineMs?: number;
}

export interface NativeTerminalProviderResult {
  pluginId: string;
  pluginVersion: string;
  providerId: string;
  kind: string;
  requestId: string;
  status: 'ok' | 'cancelled' | 'failed';
  result?: unknown;
  error?: { code: number; message: string; data?: unknown };
}

export interface NativePluginBindings {
  List: () => Promise<Array<NativePluginRecord | null>>;
  InstallPackage?: (archivePath: string, options: { enable: boolean }) => Promise<NativePluginRecord | null>;
  SetEnabled?: (pluginID: string, enabled: boolean) => Promise<void>;
  Restart?: (pluginID: string) => Promise<NativePluginRecord | null>;
  Uninstall?: (pluginID: string) => Promise<boolean>;
  UISchema?: (pluginID: string) => Promise<NativePluginUISchema | null>;
  /** Aggregate declarative views/menus/keybindings across enabled plugins (PluginService.UIContributions). */
  UIContributions?: () => Promise<{
    views?: NativePluginUIContributionView[];
    menus?: NativePluginUIContributionMenu[];
    keybindings?: NativePluginUIContributionKeybinding[];
  } | null>;
  Settings?: (pluginID: string) => Promise<Record<string, unknown>>;
  SetSetting?: (pluginID: string, settingID: string, valueJSON: string) => Promise<void>;
  ResetSetting?: (pluginID: string, settingID: string) => Promise<void>;
  NativeRunning?: (pluginID: string) => Promise<boolean>;
  CallNative?: (pluginID: string, method: string, paramsJSON: string) => Promise<string>;
  /** lemonssh-wasm-abi v1 dispatch into an enabled plugin's WASM entrypoint. */
  CallPlugin?: (pluginID: string, method: string, payloadJSON: string) => Promise<{
    ok: boolean;
    result?: unknown;
    error?: { code: string; message: string; data?: unknown };
  } | null>;
  /** Broker-checked terminal provider enumeration (PluginService.TerminalProviders). */
  TerminalProviders?: (kind: string, locale: string) => Promise<Array<NativePluginProviderContribution | null> | null>;
  /** Fan one terminal provider request out to the matching providers. */
  ProvideTerminal?: (request: NativeTerminalProviderRequest) => Promise<Array<NativeTerminalProviderResult | null> | null>;
  /** Abort one in-flight provider request. */
  CancelTerminalRequest?: (requestID: string) => Promise<boolean>;
  /** Deliver one terminal session lifecycle event to contributing plugins. */
  PublishTerminalSessionEvent?: (event: NativeTerminalSessionEvent) => Promise<Array<{ pluginId: string; delivered: boolean } | null> | null>;
  /** Broker-checked connection/authentication/importer/sync provider enumeration. */
  ExtensionProviders?: (kind: string, locale: string) => Promise<Array<NativePluginProviderContribution | null> | null>;
  // --- Extension provider data plane (PluginService, pluginExtensionService.go).
  /** Generic provider.invoke surface for connection/authentication/importer/sync operations. */
  InvokePluginExtensionProvider?: (request: {
    requestId?: string;
    providerId: string;
    kind: string;
    operation: string;
    payload?: Record<string, unknown>;
    deadlineMs?: number;
  }) => Promise<unknown>;
  /** Abort one in-flight extension request (dispatch, challenge or connection). */
  CancelPluginExtensionRequest?: (requestId: string) => Promise<boolean>;
  /** Sync provider object data plane; binary fields cross as base64 strings. */
  PluginSyncConnect?: (request: NativePluginSyncConnectRequest) => Promise<{ account: NativePluginSyncAccount | null } | null>;
  PluginSyncDisconnect?: (request: NativePluginSyncRequestBase) => Promise<unknown>;
  PluginSyncGetAccount?: (request: NativePluginSyncRequestBase) => Promise<{ account: NativePluginSyncAccount | null } | null>;
  PluginSyncGetCapabilities?: (request: NativePluginSyncRequestBase) => Promise<Record<string, unknown> | null>;
  PluginSyncReadObject?: (request: NativePluginSyncReadObjectRequest) => Promise<NativePluginSyncReadObjectResult | null>;
  PluginSyncReadChunk?: (request: { requestId: string; transferId: string; maxBytes?: number }) => Promise<{ chunk: string; done: boolean } | null>;
  PluginSyncWriteObject?: (request: NativePluginSyncWriteObjectRequest) => Promise<{ created: boolean; revision?: string } | null>;
  PluginSyncWriteBegin?: (request: NativePluginSyncWriteBeginRequest) => Promise<{ transferId: string; windowBytes: number } | null>;
  PluginSyncWriteChunk?: (request: { requestId: string; transferId: string; sequence: number; chunk: string }) => Promise<Record<string, unknown> | null>;
  PluginSyncWriteCommit?: (request: { requestId: string; transferId: string }) => Promise<{ created: boolean; revision?: string } | null>;
  PluginSyncDeleteObject?: (request: NativePluginSyncDeleteObjectRequest) => Promise<Record<string, unknown> | null>;
  PluginSyncPutSecret?: (request: { providerId: string; key: string; value: string }) => Promise<Record<string, unknown> | null>;
  PluginSyncDeleteSecrets?: (request: { providerId: string; keys?: string[] }) => Promise<{ deleted?: number } | null>;
  PluginSyncRestoreSecrets?: (request: { providerId: string; keys: string[]; discard?: boolean }) => Promise<Record<string, unknown> | null>;
  /** Plugin-protocol terminal connections (hosted as ordinary terminal sessions). */
  StartPluginConnection?: (request: {
    requestId?: string;
    sessionId: string;
    protocol?: string;
    hostLabel?: string;
    hostname?: string;
    providerId: string;
    configuration?: Record<string, unknown>;
    columns?: number;
    rows?: number;
    credential?: Record<string, unknown>;
    authenticationProviderId?: string;
    deadlineMs?: number;
  }) => Promise<{ sessionId: string; providerId: string; status: string; diagnostics?: Array<{ path?: string; severity: string; message: string }> } | null>;
  WritePluginConnection?: (sessionId: string, data: string) => Promise<void>;
  ControlPluginConnection?: (sessionId: string, operation: string, payload?: Record<string, unknown>) => Promise<unknown>;
  /** Importer providers; the staged sample cross the bridge as base64. */
  DetectPluginImporter?: (request: { requestId?: string; providerId: string; sample: string; fileName?: string; mediaType?: string; deadlineMs?: number }) => Promise<Record<string, unknown> | null>;
  SelectPluginImporterFile?: () => Promise<{ selectionToken: string; fileName: string; sample: string } | null>;
  ReleasePluginImporterFile?: (selectionToken: string) => Promise<boolean>;
  ParsePluginImporterFile?: (request: { requestId?: string; providerId: string; selectionToken: string; mediaType?: string; options?: Record<string, unknown>; deadlineMs?: number }) => Promise<{ providerId: string; result: { parsed: number; warnings: number; errors: number }; records: unknown[] } | null>;
  /** Authentication challenge round trip (challenges arrive as Wails events). */
  RespondPluginAuthenticationChallenge?: (response: {
    requestId: string;
    challengeRequestId: string;
    challengeId: string;
    response?: string | boolean | string[];
    cancelled?: boolean;
  }) => Promise<void>;
  /** Gates the per-chunk plugin:connection-data event (terminal output always flows through the data plane). */
  SetPluginConnectionDataEvents?: (active: boolean) => Promise<void>;
}

// --- Native sync/importer wire shapes (Go structs in pluginExtensionService.go).

export interface NativePluginSyncAccount {
  id: string;
  email?: string;
  name?: string;
  avatarUrl?: string;
}

export interface NativePluginSyncRequestBase {
  requestId?: string;
  providerId: string;
  deadlineMs?: number;
}

export interface NativePluginSyncConnectRequest extends NativePluginSyncRequestBase {
  configuration?: Record<string, unknown>;
  credential?: Record<string, unknown>;
}

export interface NativePluginSyncReadObjectRequest extends NativePluginSyncRequestBase {
  key: string;
  preferStream?: boolean;
}

export interface NativePluginSyncReadObjectResult {
  found: boolean;
  key: string;
  data?: string;
  streamed?: boolean;
  transferId?: string;
  byteLength?: number;
  revision?: string;
  contentType?: string;
}

export interface NativePluginSyncWriteObjectRequest extends NativePluginSyncRequestBase {
  key: string;
  data: string;
  expectedRevision?: string;
  preferStream?: boolean;
}

export interface NativePluginSyncWriteBeginRequest extends NativePluginSyncRequestBase {
  key: string;
  byteLength: number;
  expectedRevision?: string;
}

export interface NativePluginSyncDeleteObjectRequest extends NativePluginSyncRequestBase {
  key: string;
  expectedRevision?: string;
}

/** Renderer event names emitted by the Go extension host (pluginExtensionService.go). */
export const PLUGIN_CONNECTION_DATA_EVENT = "plugin:connection-data";
export const PLUGIN_CONNECTION_CLOSED_EVENT = "plugin:connection-closed";
export const PLUGIN_AUTHENTICATION_CHALLENGE_EVENT = "plugin:authentication-challenge";
export const PLUGIN_IMPORTER_PROGRESS_EVENT = "plugin:importer-progress";

function parseManifest(value: unknown): Record<string, unknown> {
  if (typeof value === 'string') {
    try { return JSON.parse(value) as Record<string, unknown>; } catch { return {}; }
  }
  if (value && typeof value === 'object') return value as Record<string, unknown>;
  return {};
}

function installedPlugin(record: NativePluginRecord): LemonSSHInstalledPlugin {
  const enabled = record.state === 'enabled';
  return {
    id: record.pluginId,
    enabled,
    activeVersion: record.state === 'staged' ? null : record.version,
    manifest: parseManifest(record.manifest),
    runtime: {
      status: enabled ? 'active' : record.state,
      kind: 'browser',
      lastError: null,
      quarantinedAt: null,
    },
  };
}

function defaultValue(field: { type: string; default?: string }): unknown {
  if (field.default == null) return field.type === 'boolean' ? false : field.type === 'number' ? 0 : '';
  if (field.type === 'boolean') return field.default === 'true';
  if (field.type === 'number') return Number(field.default) || 0;
  return field.default;
}

/** Canonical view.data dispatch method (see docs/plugin-platform/ui-contributions.md). */
const VIEW_DATA_DISPATCH_METHOD = 'view.data';

/** Mirrors the Go host's ui.maxViewKeys install-time cap. */
const VIEW_DATA_MAX_BINDINGS = 64;

export interface PluginViewDataResult {
  /** "plugin" when the plugin answered the view.data dispatch, else "settings". */
  source: 'plugin' | 'settings';
  data: Record<string, unknown>;
}

interface PluginViewInstance {
  pluginId: string;
  viewId: string;
  scopeId: string;
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

/** Base64 helpers for the Go extension data plane (binary crosses as base64). */
export function pluginBridgeBytesToBase64(bytes: Uint8Array): string {
  if (typeof Buffer === 'function' && typeof btoa !== 'function') {
    return Buffer.from(bytes.buffer, bytes.byteOffset, bytes.byteLength).toString('base64');
  }
  let binary = '';
  const step = 0x8000;
  for (let index = 0; index < bytes.length; index += step) {
    binary += String.fromCharCode(...bytes.subarray(index, index + step));
  }
  return btoa(binary);
}

export function pluginBridgeBase64ToBytes(value: string): Uint8Array {
  if (typeof atob === 'function') {
    const binary = atob(value);
    const bytes = new Uint8Array(binary.length);
    for (let index = 0; index < binary.length; index += 1) {
      bytes[index] = binary.charCodeAt(index);
    }
    return bytes;
  }
  return Uint8Array.from(Buffer.from(value, 'base64'));
}

/** Optional host seams the bridge needs beyond the plugin service bindings. */
export interface PluginBridgeHost {
  /** Subscribes to a Go-side Wails event; the listener receives the payload. */
  subscribeEvent?: (eventName: string, callback: (payload: unknown) => void) => () => void;
  /** Attaches the loopback data plane for a freshly registered session id. */
  attachDataPlane?: (sessionID: string) => Promise<void>;
}

export function mergePluginViewData(
  settingsValues: Record<string, unknown>,
  keys: ReadonlyArray<string>,
  dispatchResult?: unknown,
): PluginViewDataResult {
  const fallback: Record<string, unknown> = {};
  for (const key of keys) {
    if (key in settingsValues) fallback[key] = settingsValues[key];
  }
  if (!isPlainObject(dispatchResult)) return { source: 'settings', data: fallback };
  return { source: 'plugin', data: { ...fallback, ...dispatchResult } };
}

export function createPluginBridge(
  bindings: NativePluginBindings | undefined,
  host: PluginBridgeHost = {},
): Partial<LemonSSHBridge> {
  const required = () => {
    if (!bindings) throw new Error('Native plugin service is unavailable');
    return bindings;
  };
  const contributionListeners = new Set<(event: { reason: string; pluginId: string | null; revision: number }) => void>();
  const viewClosedListeners = new Set<(event: { instanceId: string; pluginId: string; viewId: string; reason: string }) => void>();
  const viewInstances = new Map<string, PluginViewInstance>();
  // Connection-data event listeners; the Go host gates the per-chunk event
  // on subscriber presence, so first/last flips SetPluginConnectionDataEvents.
  const connectionDataListeners = new Set<(event: { sessionId: string; data: Uint8Array }) => void>();
  const notifyConnectionData = (payload: unknown) => {
    const event = payload as { sessionId?: unknown; data?: unknown } | undefined;
    if (!event || typeof event.sessionId !== 'string' || typeof event.data !== 'string') return;
    const bytes = pluginBridgeBase64ToBytes(event.data);
    for (const listener of [...connectionDataListeners]) {
      listener({ sessionId: event.sessionId, data: bytes });
    }
  };
  const subscribeConnectionData = (callback: (event: { sessionId: string; data: Uint8Array }) => void) => {
    connectionDataListeners.add(callback);
    void bindings?.SetPluginConnectionDataEvents?.(true)?.catch(() => undefined);
    const unsubscribeEvent = host.subscribeEvent
      ? host.subscribeEvent(PLUGIN_CONNECTION_DATA_EVENT, notifyConnectionData)
      : () => undefined;
    return () => {
      connectionDataListeners.delete(callback);
      unsubscribeEvent();
      if (connectionDataListeners.size === 0) {
        void bindings?.SetPluginConnectionDataEvents?.(false)?.catch(() => undefined);
      }
    };
  };
  let revision = 0;
  let instanceCounter = 0;
  const notify = (reason: string, pluginId: string | null) => {
    const event = { reason, pluginId, revision: ++revision };
    for (const listener of contributionListeners) listener(event);
    // The registry snapshot changed: re-probe the sync provider registry so
    // pluginHostReady()/isPluginSyncIpcAvailable() stay truthful.
    void refreshSyncRegistryProbe();
  };
  const listRecords = async () => (await required().List()).filter((record): record is NativePluginRecord => Boolean(record));
  const findInstalled = async (pluginId: string) => {
    const record = (await listRecords()).find(candidate => candidate.pluginId === pluginId);
    if (!record) throw new Error(`Plugin ${pluginId} is not installed`);
    return installedPlugin(record);
  };
  // Declarative views are contributed through the validated ui schema; the
  // first enabled plugin declaring the view id wins (the host rejects
  // duplicate view ids per plugin, and ids are plugin-scoped in the UI).
  const resolveDeclarativeView = async (viewId: string) => {
    for (const record of await listRecords()) {
      if (record.state !== 'enabled') continue;
      const schema = await required().UISchema?.(record.pluginId);
      const view = schema?.views?.find(candidate => candidate.id === viewId);
      if (view) return { pluginId: record.pluginId, view };
    }
    return null;
  };
  // pluginHostReady() must be a synchronous probe, so the registry answer is
  // cached: the bridge asks the Go registry whether any sync provider is
  // currently enumerated (fail-closed until the first probe resolves), and
  // every notify() (install/enable/restart/uninstall) re-probes.
  let syncRegistryReady = false;
  const refreshSyncRegistryProbe = async () => {
    const extensionProviders = bindings?.ExtensionProviders;
    if (typeof extensionProviders !== 'function') {
      syncRegistryReady = false;
      return;
    }
    try {
      const records = await extensionProviders('sync', '');
      syncRegistryReady = (records ?? []).some(record => Boolean(record));
    } catch {
      syncRegistryReady = false;
    }
  };
  void refreshSyncRegistryProbe();
  return {
    getPluginRuntimeStatus: async () => ({ available: true, experimental: true }),
    // True only while the Go registry currently enumerates at least one sync
    // provider (fail-closed until the first probe resolves); the adapter
    // isPluginSyncIpcAvailable() gates the plugin sync cloud path on it.
    pluginHostReady: () => syncRegistryReady,
    listPlugins: async () => (await listRecords()).map(installedPlugin),
    installPluginPackage: async (archivePath, options) => {
      if (!required().InstallPackage) throw new Error('Plugin package installation is unavailable');
      const record = await required().InstallPackage(archivePath, { enable: options?.enable !== false });
      if (!record) throw new Error('Plugin package installation returned no record');
      notify('installed', record.pluginId);
      return installedPlugin(record);
    },
    setPluginEnabled: async (pluginId, enabled) => {
      if (!required().SetEnabled) throw new Error('Plugin lifecycle control is unavailable');
      await required().SetEnabled(pluginId, enabled);
      for (const [instanceId, instance] of [...viewInstances]) {
        if (instance.pluginId !== pluginId) continue;
        viewInstances.delete(instanceId);
        for (const listener of viewClosedListeners) {
          listener({ instanceId, pluginId, viewId: instance.viewId, reason: 'plugin-disabled' });
        }
      }
      notify(enabled ? 'enabled' : 'disabled', pluginId);
      return findInstalled(pluginId);
    },
    restartPlugin: async pluginId => {
      if (!required().Restart) throw new Error('Plugin restart is unavailable');
      const record = await required().Restart(pluginId);
      if (!record) throw new Error(`Plugin ${pluginId} is not installed`);
      notify('restarted', pluginId);
      return installedPlugin(record);
    },
    uninstallPlugin: async pluginId => {
      if (!required().Uninstall) throw new Error('Plugin uninstall is unavailable');
      const removed = await required().Uninstall(pluginId);
      if (removed) notify('uninstalled', pluginId);
      return removed;
    },
    getPluginContributions: async options => {
      const plugins: LemonSSHPluginContributionSnapshot['plugins'][number][] = [];
      for (const record of await listRecords()) {
        if (record.state !== 'enabled') continue;
        const manifest = parseManifest(record.manifest);
        const rawContributions = Array.isArray(manifest.contributions) ? manifest.contributions as Array<Record<string, unknown>> : [];
        const [schema, values] = await Promise.all([
          required().UISchema?.(record.pluginId) ?? Promise.resolve(null),
          required().Settings?.(record.pluginId) ?? Promise.resolve({}),
        ]);
        const settings = (schema?.settings ?? []).map(field => ({
          id: field.id,
          label: field.label,
          description: field.description,
          control: field.type === 'boolean' ? 'switch' : field.type === 'select' ? 'select' : field.type,
          scope: 'application',
          scopeId: null,
          value: field.type === 'password' ? undefined : values[field.id] ?? defaultValue(field),
          secret: field.type === 'password',
          configured: field.type === 'password' ? Boolean(record.settings?.[field.id]) : field.id in values,
          visible: true,
          required: field.required,
          options: field.options?.map(value => ({ value, label: value })),
        }));
        const commandContributions = rawContributions.filter(item => item.type === 'command' && typeof item.id === 'string');
        const commandTitles = new Map(commandContributions.map(item => [String(item.id), String(item.id)]));
        plugins.push({
          id: record.pluginId,
          version: record.version,
          displayName: typeof manifest.displayName === 'string' ? manifest.displayName : record.pluginId,
          description: typeof manifest.description === 'string' ? manifest.description : '',
          commands: commandContributions.map(item => ({ id: String(item.id), title: commandTitles.get(String(item.id)) ?? String(item.id), enabled: true })),
          keybindings: (schema?.keybindings ?? []).map(binding => ({
            command: binding.command,
            key: binding.key,
            ...(binding.mac ? { mac: binding.mac } : {}),
            ...(binding.linux ? { linux: binding.linux } : {}),
            ...(binding.windows ? { windows: binding.windows } : {}),
            ...(binding.args !== undefined ? { args: binding.args } : {}),
            enabled: binding.enabled !== false,
          })),
          menus: (schema?.menus ?? []).map(menu => ({
            id: menu.id,
            command: menu.command,
            ...(menu.alt ? { alt: menu.alt } : {}),
            location: menu.location,
            title: menu.title || commandTitles.get(menu.command) || menu.command,
            visible: menu.visible !== false,
            enabled: true,
            ...(menu.group ? { group: menu.group } : {}),
            ...(menu.order ? { order: menu.order } : {}),
            showKeybinding: true,
          })),
          settings,
          // The Go host only accepts the "settings" view location today; the
          // visible flag honors the manifest declaration (nil means visible).
          views: (schema?.views ?? []).map(view => ({
            id: view.id,
            title: view.title,
            location: view.location || 'settings',
            entry: '',
            visible: view.visible !== false,
          })),
        });
      }
      return { locale: options?.locale ?? 'en', plugins };
    },
    executePluginCommand: async (command, args, context) => {
      const owner = (await listRecords()).find(record => {
        const manifest = parseManifest(record.manifest);
        return record.state === 'enabled' && Array.isArray(manifest.contributions)
          && (manifest.contributions as Array<Record<string, unknown>>).some(item => item.type === 'command' && item.id === command);
      });
      if (!owner) throw new Error(`Plugin command ${command} is unavailable`);
      const api = required();
      if (api.NativeRunning && api.CallNative && await api.NativeRunning(owner.pluginId)) {
        const result = await api.CallNative(owner.pluginId, 'command.execute', JSON.stringify({ command, args, context }));
        return result ? JSON.parse(result) : null;
      }
      // No live companion process: fall through to the WASM dispatch channel
      // so plugins can own their commands without shipping a native binary.
      if (!api.CallPlugin) throw new Error(`Plugin command ${command} has no active native command handler`);
      const response = await api.CallPlugin(owner.pluginId, 'command.execute', JSON.stringify({ command, args, context }));
      if (response?.ok) return response.result ?? null;
      const message = response?.error?.message || 'plugin dispatch error';
      throw new Error(`Plugin command ${command} failed: ${message}`);
    },
    getPluginViewData: async (pluginId, viewId, keys): Promise<PluginViewDataResult> => {
      const record = (await listRecords()).find(candidate => candidate.pluginId === pluginId);
      if (!record) throw new Error(`Plugin ${pluginId} is not installed`);
      if (record.state !== 'enabled') throw new Error(`Plugin ${pluginId} is disabled`);
      const bindingKeys = keys.filter(key => key.length > 0).slice(0, VIEW_DATA_MAX_BINDINGS);
      const settings = await required().Settings?.(pluginId) ?? {};
      if (!required().CallPlugin || bindingKeys.length === 0) {
        return mergePluginViewData(settings, bindingKeys);
      }
      try {
        const response = await required().CallPlugin(pluginId, VIEW_DATA_DISPATCH_METHOD, JSON.stringify({ viewId, bindings: bindingKeys }));
        return mergePluginViewData(settings, bindingKeys, response?.ok ? response.result : undefined);
      } catch {
        // Transport failures degrade to the declared settings values instead
        // of blanking the declarative view.
        return mergePluginViewData(settings, bindingKeys);
      }
    },
    openPluginView: async payload => {
      const resolved = await resolveDeclarativeView(payload.viewId);
      if (!resolved) throw new Error(`Plugin view ${payload.viewId} is not declared by an enabled plugin`);
      const location = resolved.view.location || 'settings';
      if (location !== 'settings') {
        throw new Error(`Plugin view location "${location}" is not supported yet; only settings views render today`);
      }
      const scopeId = payload.scopeId || 'default';
      const existing = payload.instanceId && viewInstances.get(payload.instanceId)
        ? payload.instanceId
        : [...viewInstances.entries()]
            .find(([, instance]) => instance.viewId === payload.viewId && instance.scopeId === scopeId)?.[0];
      const instanceId = existing ?? `view:${++instanceCounter}:${payload.viewId}`;
      viewInstances.set(instanceId, { pluginId: resolved.pluginId, viewId: payload.viewId, scopeId });
      return { instanceId };
    },
    closePluginView: async instanceId => {
      const instance = viewInstances.get(instanceId);
      if (!instance) return;
      viewInstances.delete(instanceId);
      for (const listener of viewClosedListeners) {
        listener({ instanceId, pluginId: instance.pluginId, viewId: instance.viewId, reason: 'host' });
      }
    },
    // Declarative settings views render inside host-owned React surfaces, so
    // bounds/visibility have no native surface to drive; both resolve so the
    // shared view lifecycle can retain and re-show instances.
    setPluginViewBounds: async () => undefined,
    setPluginViewVisibility: async () => undefined,
    onPluginViewClosed: callback => {
      viewClosedListeners.add(callback);
      return () => { viewClosedListeners.delete(callback); };
    },
    updatePluginSetting: async (pluginId, settingId, value) => {
      if (!required().SetSetting) throw new Error('Plugin settings are unavailable');
      await required().SetSetting(pluginId, settingId, JSON.stringify(value));
      notify('setting-updated', pluginId);
      return { restartRequired: false };
    },
    resetPluginSetting: async (pluginId, settingId) => {
      if (!required().ResetSetting) throw new Error('Plugin settings are unavailable');
      await required().ResetSetting(pluginId, settingId);
      notify('setting-reset', pluginId);
      return { restartRequired: false };
    },
    setPluginEnvironment: async () => undefined,
    onPluginContributionsChanged: callback => {
      contributionListeners.add(callback);
      return () => { contributionListeners.delete(callback); };
    },
    // Terminal/extension providers are served by the Go host registry
    // (internal/plugin/providers): declarations are accepted only when the
    // plugin's manifest declares the provider permission and the fail-closed
    // broker holds the grant. Without the native methods the bridge degrades
    // to "no providers" instead of failing the renderer registry.
    listPluginTerminalProviders: async options => {
      if (!bindings?.TerminalProviders) return [];
      const records = await bindings.TerminalProviders(options.kind, options.locale ?? '');
      return (records ?? [])
        .filter((record): record is NativePluginProviderContribution => Boolean(record))
        .map(record => ({
          pluginId: record.pluginId,
          pluginVersion: record.pluginVersion,
          pluginDisplayName: record.pluginDisplayName || record.pluginId,
          provider: record.provider,
        })) as unknown as LemonSSHTerminalProviderContribution[];
    },
    providePluginTerminal: async request => {
      if (!bindings?.ProvideTerminal) return [];
      const results = await bindings.ProvideTerminal({
        requestId: request.requestId,
        kind: request.kind,
        operation: request.operation,
        session: request.session,
        ...(request.payload !== undefined ? { payload: request.payload } : {}),
        ...(request.locale ? { locale: request.locale } : {}),
        ...(request.preferredProviderIds ? { preferredProviderIds: [...request.preferredProviderIds] } : {}),
        ...(request.deadlineMs ? { deadlineMs: request.deadlineMs } : {}),
      });
      return (results ?? [])
        .filter((result): result is NativeTerminalProviderResult => Boolean(result))
        .map(result => ({
          pluginId: result.pluginId,
          pluginVersion: result.pluginVersion,
          providerId: result.providerId,
          kind: result.kind,
          requestId: result.requestId,
          status: result.status,
          ...(result.status === 'ok' ? { result: result.result } : {}),
          ...(result.status === 'failed' && result.error ? { error: result.error } : {}),
        })) as unknown as LemonSSHTerminalProviderResult[];
    },
    cancelPluginTerminalRequest: async requestId => {
      if (!bindings?.CancelTerminalRequest) return false;
      return bindings.CancelTerminalRequest(requestId);
    },
    publishPluginTerminalSessionEvent: async event => {
      if (!bindings?.PublishTerminalSessionEvent) return [];
      const deliveries = await bindings.PublishTerminalSessionEvent({
        type: event.type,
        session: event.session,
        ...(event.exitCode !== undefined ? { exitCode: event.exitCode } : {}),
      });
      return (deliveries ?? [])
        .filter((delivery): delivery is { pluginId: string; delivered: boolean } => Boolean(delivery))
        .map(delivery => ({ pluginId: delivery.pluginId, delivered: delivery.delivered }));
    },
    listPluginExtensionProviders: async options => {
      if (!bindings?.ExtensionProviders) return [];
      const records = await bindings.ExtensionProviders(options.kind, options.locale ?? '');
      return (records ?? [])
        .filter((record): record is NativePluginProviderContribution => Boolean(record))
        .map(record => ({
          pluginId: record.pluginId,
          pluginVersion: record.pluginVersion,
          pluginDisplayName: record.pluginDisplayName || record.pluginId,
          provider: record.provider,
        })) as unknown as LemonSSHExtensionProviderContribution[];
    },
    // --- Extension provider data plane -----------------------------------
    // Generic provider.invoke surface (validate/probe/begin operations from
    // useTerminalBackend, importer detect flows, authentication begins).
    invokePluginExtensionProvider: async request => {
      if (!bindings?.InvokePluginExtensionProvider) {
        throw new Error('Plugin extension providers are unavailable');
      }
      return bindings.InvokePluginExtensionProvider({
        requestId: request.requestId,
        providerId: request.providerId,
        kind: request.kind,
        operation: request.operation,
        ...(request.payload !== undefined ? { payload: asRecord(request.payload) } : {}),
        ...(request.deadlineMs !== undefined ? { deadlineMs: request.deadlineMs } : {}),
      }) as unknown as import("@lemonssh/plugin-contract").JsonValue;
    },
    cancelPluginExtensionRequest: async requestId => {
      if (!bindings?.CancelPluginExtensionRequest) return false;
      return bindings.CancelPluginExtensionRequest(requestId);
    },
    // --- Sync provider data plane ----------------------------------------
    // Binary fields cross the Wails JSON channel as base64 strings; the
    // renderer adapter contract (pluginSyncIpcHost.ts) speaks Uint8Array.
    pluginSyncConnect: async request => {
      if (!bindings?.PluginSyncConnect) throw new Error('Plugin sync host is unavailable');
      const result = await bindings.PluginSyncConnect({
        requestId: request.requestId,
        providerId: request.providerId,
        ...(request.configuration !== undefined ? { configuration: asRecord(request.configuration) } : {}),
        ...(request.credential !== undefined ? { credential: asRecord(request.credential) } : {}),
        ...(request.deadlineMs !== undefined ? { deadlineMs: request.deadlineMs } : {}),
      });
      return { account: result?.account ?? null };
    },
    pluginSyncDisconnect: async request => {
      if (!bindings?.PluginSyncDisconnect) throw new Error('Plugin sync host is unavailable');
      await bindings.PluginSyncDisconnect(nativeSyncRequest(request));
      return null;
    },
    pluginSyncGetAccount: async request => {
      if (!bindings?.PluginSyncGetAccount) throw new Error('Plugin sync host is unavailable');
      const result = await bindings.PluginSyncGetAccount(nativeSyncRequest(request));
      return { account: result?.account ?? null };
    },
    pluginSyncGetCapabilities: async request => {
      if (!bindings?.PluginSyncGetCapabilities) throw new Error('Plugin sync host is unavailable');
      const result = await bindings.PluginSyncGetCapabilities(nativeSyncRequest(request));
      if (!result) throw new Error('Plugin sync provider returned no capabilities');
      return result as {
        revisions: boolean;
        conditionalWrites: boolean;
        atomicReplacement: boolean;
        maxObjectBytes?: number;
        maxObjects?: number;
      };
    },
    pluginSyncReadObject: async request => {
      if (!bindings?.PluginSyncReadObject) throw new Error('Plugin sync host is unavailable');
      const result = await bindings.PluginSyncReadObject({
        ...nativeSyncRequest(request),
        key: request.key,
        ...(request.preferStream !== undefined ? { preferStream: request.preferStream } : {}),
      });
      if (!result) throw new Error('Plugin sync provider returned no result');
      return {
        found: result.found === true,
        key: result.key,
        ...(result.data ? { data: pluginBridgeBase64ToBytes(result.data) } : {}),
        ...(result.streamed === true ? { streamed: true } : {}),
        ...(result.transferId ? { transferId: result.transferId } : {}),
        ...(result.byteLength !== undefined ? { byteLength: result.byteLength } : {}),
        ...(result.revision ? { revision: result.revision } : {}),
        ...(result.contentType ? { contentType: result.contentType } : {}),
      };
    },
    pluginSyncReadChunk: async request => {
      if (!bindings?.PluginSyncReadChunk) throw new Error('Plugin sync streamed read is unavailable');
      const result = await bindings.PluginSyncReadChunk({
        requestId: request.requestId,
        transferId: request.transferId,
        ...(request.maxBytes !== undefined ? { maxBytes: request.maxBytes } : {}),
      });
      if (!result) throw new Error('Plugin sync provider returned no chunk');
      return { chunk: pluginBridgeBase64ToBytes(result.chunk ?? ''), done: result.done === true };
    },
    pluginSyncWriteObject: async request => {
      if (!bindings?.PluginSyncWriteObject) throw new Error('Plugin sync host is unavailable');
      const result = await bindings.PluginSyncWriteObject({
        ...nativeSyncRequest(request),
        key: request.key,
        data: pluginBridgeBytesToBase64(request.data),
        ...(request.expectedRevision !== undefined ? { expectedRevision: request.expectedRevision } : {}),
        ...(request.preferStream !== undefined ? { preferStream: request.preferStream } : {}),
      });
      if (!result) throw new Error('Plugin sync provider returned no result');
      return { created: result.created === true, ...(result.revision ? { revision: result.revision } : {}) };
    },
    pluginSyncWriteBegin: async request => {
      if (!bindings?.PluginSyncWriteBegin) throw new Error('Plugin sync streamed write is unavailable');
      const result = await bindings.PluginSyncWriteBegin({
        ...nativeSyncRequest(request),
        key: request.key,
        byteLength: request.byteLength,
        ...(request.expectedRevision !== undefined ? { expectedRevision: request.expectedRevision } : {}),
      });
      if (!result) throw new Error('Plugin sync provider returned no transfer');
      return { transferId: result.transferId, windowBytes: result.windowBytes };
    },
    pluginSyncWriteChunk: async request => {
      if (!bindings?.PluginSyncWriteChunk) throw new Error('Plugin sync streamed write is unavailable');
      const result = await bindings.PluginSyncWriteChunk({
        requestId: request.requestId,
        transferId: request.transferId,
        sequence: request.sequence,
        chunk: pluginBridgeBytesToBase64(request.chunk),
      });
      if (!result) throw new Error('Plugin sync provider rejected the chunk');
      return { accepted: Number(result.accepted ?? 0) };
    },
    pluginSyncWriteCommit: async request => {
      if (!bindings?.PluginSyncWriteCommit) throw new Error('Plugin sync streamed write is unavailable');
      const result = await bindings.PluginSyncWriteCommit({
        requestId: request.requestId,
        transferId: request.transferId,
      });
      if (!result) throw new Error('Plugin sync provider returned no commit');
      return { created: result.created === true, ...(result.revision ? { revision: result.revision } : {}) };
    },
    pluginSyncDeleteObject: async request => {
      if (!bindings?.PluginSyncDeleteObject) throw new Error('Plugin sync host is unavailable');
      const result = await bindings.PluginSyncDeleteObject({
        ...nativeSyncRequest(request),
        key: request.key,
        ...(request.expectedRevision !== undefined ? { expectedRevision: request.expectedRevision } : {}),
      });
      if (!result) throw new Error('Plugin sync provider returned no result');
      return { deleted: result.deleted === true };
    },
    pluginSyncPutSecret: async request => {
      if (!bindings?.PluginSyncPutSecret) throw new Error('Plugin sync secret storage is unavailable');
      const result = await bindings.PluginSyncPutSecret({
        providerId: request.providerId,
        key: request.key,
        value: request.value,
      });
      if (!result) throw new Error('Plugin sync secret storage returned no result');
      return result as { kind: 'secret'; id: string; key: string; created?: boolean };
    },
    pluginSyncDeleteSecrets: async request => {
      if (!bindings?.PluginSyncDeleteSecrets) throw new Error('Plugin sync secret storage is unavailable');
      const result = await bindings.PluginSyncDeleteSecrets({
        providerId: request.providerId,
        ...(request.keys ? { keys: [...request.keys] } : {}),
      });
      return { deleted: result?.deleted ?? 0 };
    },
    pluginSyncRestoreSecrets: async request => {
      if (!bindings?.PluginSyncRestoreSecrets) throw new Error('Plugin sync secret storage is unavailable');
      const result = await bindings.PluginSyncRestoreSecrets({
        providerId: request.providerId,
        keys: [...request.keys],
        ...(request.discard !== undefined ? { discard: request.discard } : {}),
      });
      return {
        restored: Number(result?.restored ?? 0),
        ...(result && 'discarded' in result ? { discarded: Number(result.discarded ?? 0) } : {}),
      };
    },
    // --- Plugin connections ------------------------------------------------
    // The Go host registers the plugin connection as an ordinary terminal
    // session; the renderer attaches its loopback data plane right after so
    // output/exit flow through the shared session pipeline.
    startPluginConnection: async options => {
      if (!bindings?.StartPluginConnection) throw new Error('startPluginConnection unavailable');
      const opened = await bindings.StartPluginConnection({
        requestId: options.requestId,
        sessionId: options.sessionId,
        ...(options.protocol ? { protocol: options.protocol } : {}),
        ...(options.hostLabel ? { hostLabel: options.hostLabel } : {}),
        ...(options.hostname ? { hostname: options.hostname } : {}),
        providerId: options.providerId,
        ...(options.configuration !== undefined ? { configuration: asRecord(options.configuration) } : {}),
        columns: options.columns,
        rows: options.rows,
        ...(options.credential !== undefined ? { credential: asRecord(options.credential) } : {}),
        ...(options.authenticationProviderId ? { authenticationProviderId: options.authenticationProviderId } : {}),
        ...(options.deadlineMs !== undefined ? { deadlineMs: options.deadlineMs } : {}),
      });
      if (!opened) throw new Error('Plugin connection provider returned no session');
      if (host.attachDataPlane) {
        await host.attachDataPlane(opened.sessionId);
      }
      return {
        sessionId: opened.sessionId,
        providerId: opened.providerId,
        status: opened.status === 'connected' ? 'connected' : 'connecting',
        diagnostics: (opened.diagnostics ?? []).map(issue => ({
          ...(issue.path ? { path: issue.path } : {}),
          severity: issue.severity === 'error' ? 'error' : 'warning',
          message: issue.message,
        })),
      };
    },
    writePluginConnection: async (sessionId, data) => {
      if (!bindings?.WritePluginConnection) throw new Error('Plugin connection write is unavailable');
      await bindings.WritePluginConnection(sessionId, pluginBridgeBytesToBase64(data));
    },
    controlPluginConnection: async (sessionId, operation, payload) => {
      if (!bindings?.ControlPluginConnection) throw new Error('Plugin connection control is unavailable');
      return bindings.ControlPluginConnection(
        sessionId,
        operation,
        payload ? { ...payload } : undefined,
      );
    },
    // --- Importer providers ------------------------------------------------
    detectPluginImporter: async request => {
      if (!bindings?.DetectPluginImporter) throw new Error('Plugin importer detect is unavailable');
      const result = await bindings.DetectPluginImporter({
        providerId: request.providerId,
        sample: pluginBridgeBytesToBase64(request.sample),
        ...(request.fileName ? { fileName: request.fileName } : {}),
        ...(request.mediaType ? { mediaType: request.mediaType } : {}),
        ...(request.deadlineMs !== undefined ? { deadlineMs: request.deadlineMs } : {}),
      });
      if (!result) throw new Error('Plugin importer provider returned no result');
      return {
        confidence: Number(result.confidence ?? 0),
        ...(result.format ? { format: String(result.format) } : {}),
        ...(result.reason ? { reason: String(result.reason) } : {}),
      };
    },
    selectPluginImporterFile: async () => {
      if (!bindings?.SelectPluginImporterFile) throw new Error('Plugin importer file selection is unavailable');
      const staged = await bindings.SelectPluginImporterFile();
      if (!staged) return null;
      return {
        selectionToken: staged.selectionToken,
        fileName: staged.fileName,
        sample: pluginBridgeBase64ToBytes(staged.sample ?? ''),
      };
    },
    releasePluginImporterFile: async selectionToken => {
      if (!bindings?.ReleasePluginImporterFile) return false;
      return bindings.ReleasePluginImporterFile(selectionToken);
    },
    parsePluginImporterFile: async request => {
      if (!bindings?.ParsePluginImporterFile) throw new Error('Plugin importer parse is unavailable');
      const result = await bindings.ParsePluginImporterFile({
        requestId: request.requestId,
        providerId: request.providerId,
        selectionToken: request.selectionToken,
        ...(request.mediaType ? { mediaType: request.mediaType } : {}),
        ...(request.options !== undefined ? { options: request.options as Record<string, unknown> } : {}),
        ...(request.deadlineMs !== undefined ? { deadlineMs: request.deadlineMs } : {}),
      });
      if (!result) throw new Error('Plugin importer provider returned no result');
      return {
        providerId: result.providerId,
        result: result.result,
        records: result.records,
      } as LemonSSHPluginImporterPreview;
    },
    respondPluginAuthenticationChallenge: async response => {
      if (!bindings?.RespondPluginAuthenticationChallenge) {
        throw new Error('Plugin authentication bridge is unavailable');
      }
      await bindings.RespondPluginAuthenticationChallenge({
        requestId: response.requestId,
        challengeRequestId: response.challengeRequestId,
        challengeId: response.challengeId,
        ...(response.response !== undefined ? { response: response.response as string | boolean | string[] } : {}),
        ...(response.cancelled ? { cancelled: true } : {}),
      });
    },
    // --- Renderer events (Go extension host → Wails events) ---------------
    onPluginImporterProgress: callback => {
      if (!host.subscribeEvent) return () => undefined;
      return host.subscribeEvent(PLUGIN_IMPORTER_PROGRESS_EVENT, payload => {
        const event = payload as { requestId?: unknown; providerId?: unknown; progress?: unknown } | undefined;
        if (!event || typeof event.requestId !== 'string' || typeof event.providerId !== 'string' || !event.progress) return;
        callback(event as unknown as LemonSSHPluginImporterProgressEvent);
      });
    },
    onPluginAuthenticationChallenge: callback => {
      if (!host.subscribeEvent) return () => undefined;
      return host.subscribeEvent(PLUGIN_AUTHENTICATION_CHALLENGE_EVENT, payload => {
        const event = payload as { requestId?: unknown; challengeRequestId?: unknown } | undefined;
        if (!event || typeof event.requestId !== 'string' || typeof event.challengeRequestId !== 'string') return;
        if ((event as { cancelled?: unknown }).cancelled === true) {
          callback(event as unknown as LemonSSHPluginAuthenticationChallengeCancelEvent);
          return;
        }
        if ((event as { challenge?: unknown }).challenge) {
          callback(event as unknown as LemonSSHPluginAuthenticationChallengeOpenEvent);
        }
      });
    },
    onPluginConnectionData: callback => subscribeConnectionData(callback),
    onPluginConnectionClosed: callback => {
      if (!host.subscribeEvent) return () => undefined;
      return host.subscribeEvent(PLUGIN_CONNECTION_CLOSED_EVENT, payload => {
        const event = payload as { sessionId?: unknown; reason?: unknown } | undefined;
        if (!event || typeof event.sessionId !== 'string') return;
        callback({ sessionId: event.sessionId, reason: typeof event.reason === 'string' ? event.reason : 'closed' });
      });
    },
  };
}

/** Narrows an unknown JSON-ish value into a plain object record. */
function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : {};
}

/** Normalizes the base sync request (requestId/providerId/deadlineMs). */
function nativeSyncRequest(
  request: { requestId?: string; providerId: string; deadlineMs?: number },
): NativePluginSyncRequestBase {
  return {
    requestId: request.requestId,
    providerId: request.providerId,
    ...(request.deadlineMs !== undefined ? { deadlineMs: request.deadlineMs } : {}),
  };
}
