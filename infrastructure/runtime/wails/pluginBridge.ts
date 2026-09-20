export interface NativePluginRecord {
  pluginId: string;
  version: string;
  manifest: unknown;
  state: string;
  settings?: Record<string, unknown>;
}

export interface NativePluginBindings {
  List: () => Promise<Array<NativePluginRecord | null>>;
  InstallPackage?: (archivePath: string, options: { enable: boolean }) => Promise<NativePluginRecord | null>;
  SetEnabled?: (pluginID: string, enabled: boolean) => Promise<void>;
  Restart?: (pluginID: string) => Promise<NativePluginRecord | null>;
  Uninstall?: (pluginID: string) => Promise<boolean>;
  UISchema?: (pluginID: string) => Promise<{
    settings?: Array<{ id: string; type: string; label: string; default?: string; options?: string[]; required?: boolean; description?: string }>;
    views?: Array<{ id: string; type: string; title: string }>;
  } | null>;
  Settings?: (pluginID: string) => Promise<Record<string, unknown>>;
  SetSetting?: (pluginID: string, settingID: string, valueJSON: string) => Promise<void>;
  ResetSetting?: (pluginID: string, settingID: string) => Promise<void>;
  NativeRunning?: (pluginID: string) => Promise<boolean>;
  CallNative?: (pluginID: string, method: string, paramsJSON: string) => Promise<string>;
}

function parseManifest(value: unknown): Record<string, unknown> {
  if (typeof value === 'string') {
    try { return JSON.parse(value) as Record<string, unknown>; } catch { return {}; }
  }
  if (value && typeof value === 'object') return value as Record<string, unknown>;
  return {};
}

function installedPlugin(record: NativePluginRecord): NetcattyInstalledPlugin {
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

export function createPluginBridge(bindings: NativePluginBindings | undefined): Partial<NetcattyBridge> {
  const required = () => {
    if (!bindings) throw new Error('Native plugin service is unavailable');
    return bindings;
  };
  const contributionListeners = new Set<(event: { reason: string; pluginId: string | null; revision: number }) => void>();
  let revision = 0;
  const notify = (reason: string, pluginId: string | null) => {
    const event = { reason, pluginId, revision: ++revision };
    for (const listener of contributionListeners) listener(event);
  };
  const listRecords = async () => (await required().List()).filter((record): record is NativePluginRecord => Boolean(record));
  const findInstalled = async (pluginId: string) => {
    const record = (await listRecords()).find(candidate => candidate.pluginId === pluginId);
    if (!record) throw new Error(`Plugin ${pluginId} is not installed`);
    return installedPlugin(record);
  };
  return {
    getPluginRuntimeStatus: async () => ({ available: true, experimental: true }),
    pluginHostReady: () => true,
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
      const plugins: NetcattyPluginContributionSnapshot['plugins'][number][] = [];
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
        plugins.push({
          id: record.pluginId,
          version: record.version,
          displayName: typeof manifest.displayName === 'string' ? manifest.displayName : record.pluginId,
          description: typeof manifest.description === 'string' ? manifest.description : '',
          commands: commandContributions.map(item => ({ id: String(item.id), title: String(item.id), enabled: true })),
          keybindings: [],
          menus: [],
          settings,
          views: (schema?.views ?? []).map(view => ({ id: view.id, title: view.title, location: 'settings', entry: '', visible: false })),
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
      if (!required().NativeRunning || !required().CallNative || !await required().NativeRunning(owner.pluginId)) {
        throw new Error(`Plugin command ${command} has no active native command handler`);
      }
      const result = await required().CallNative(owner.pluginId, 'command.execute', JSON.stringify({ command, args, context }));
      return result ? JSON.parse(result) : null;
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
    listPluginTerminalProviders: async () => [],
    providePluginTerminal: async () => [],
    cancelPluginTerminalRequest: async () => false,
    publishPluginTerminalSessionEvent: async () => [],
    listPluginExtensionProviders: async () => [],
  };
}
