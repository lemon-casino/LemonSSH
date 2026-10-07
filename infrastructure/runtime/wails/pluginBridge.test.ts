import assert from 'node:assert/strict';
import test from 'node:test';

import {
  createPluginBridge,
  mergePluginViewData,
  type NativePluginBindings,
  type NativePluginRecord,
  type NativeTerminalProviderRequest,
} from './pluginBridge';

function fixtureBindings(): NativePluginBindings & { records: NativePluginRecord[]; calls: Array<{ method: string; params: string }>; dispatches: Array<{ pluginId: string; method: string; payload: string }> } {
  const records: NativePluginRecord[] = [{
    pluginId: 'demo-plugin',
    version: '1.2.3',
    state: 'enabled',
    manifest: {
      displayName: 'Demo Plugin',
      description: 'Migration fixture',
      contributions: [{ type: 'command', id: 'demo.run' }],
    },
    settings: { enabled: true },
  }];
  const calls: Array<{ method: string; params: string }> = [];
  const dispatches: Array<{ pluginId: string; method: string; payload: string }> = [];
  return {
    records,
    calls,
    dispatches,
    List: async () => records,
    SetEnabled: async (pluginId, enabled) => {
      const record = records.find(item => item.pluginId === pluginId);
      if (record) record.state = enabled ? 'enabled' : 'disabled';
    },
    Restart: async pluginId => records.find(item => item.pluginId === pluginId) ?? null,
    Uninstall: async pluginId => {
      const index = records.findIndex(item => item.pluginId === pluginId);
      if (index < 0) return false;
      records.splice(index, 1);
      return true;
    },
    UISchema: async () => ({
      settings: [{ id: 'enabled', type: 'boolean', label: 'Enabled' }],
      views: [
        { id: 'status', type: 'card', title: 'Status', bindings: ['enabled'] },
        { id: 'ghost', type: 'card', title: 'Ghost', visible: false },
      ],
      menus: [{ id: 'palette-run', command: 'demo.run', location: 'commandPalette', title: 'Run Demo', group: 'demo', order: 2 }],
      keybindings: [{ command: 'demo.run', key: 'ctrl+alt+d', windows: 'ctrl+shift+d', args: { source: 'test' } }],
    }),
    Settings: async () => ({ enabled: true }),
    SetSetting: async () => undefined,
    ResetSetting: async () => undefined,
    NativeRunning: async () => true,
    CallNative: async (_pluginId, method, params) => {
      calls.push({ method, params });
      return JSON.stringify({ ok: true });
    },
    CallPlugin: async (pluginId, method, payload) => {
      dispatches.push({ pluginId, method, payload });
      if (method === 'view.data') return { ok: true, result: { enabled: 'live-from-plugin' } };
      return { ok: true, result: { via: 'wasm' } };
    },
  };
}

test('Wails plugin bridge exposes lifecycle, settings, views, and native commands', async () => {
  const bindings = fixtureBindings();
  const bridge = createPluginBridge(bindings);

  const installed = await bridge.listPlugins!();
  assert.equal(installed[0]?.id, 'demo-plugin');
  assert.equal(installed[0]?.runtime.status, 'active');

  const contributions = await bridge.getPluginContributions!({ locale: 'zh-CN' });
  assert.equal(contributions.locale, 'zh-CN');
  assert.equal(contributions.plugins[0]?.commands[0]?.id, 'demo.run');
  assert.equal(contributions.plugins[0]?.settings[0]?.value, true);
  assert.equal(contributions.plugins[0]?.views[0]?.title, 'Status');
  assert.equal(contributions.plugins[0]?.views[0]?.location, 'settings');
  assert.equal(contributions.plugins[0]?.views[0]?.visible, true);
  assert.equal(contributions.plugins[0]?.views[1]?.visible, false);

  assert.deepEqual(await bridge.executePluginCommand!('demo.run', { value: 1 }, { source: 'test' }), { ok: true });
  assert.equal(bindings.calls[0]?.method, 'command.execute');

  assert.equal((await bridge.setPluginEnabled!('demo-plugin', false)).enabled, false);
  assert.equal(await bridge.uninstallPlugin!('demo-plugin'), true);
});

test('Wails plugin bridge surfaces schema-declared menus and keybindings', async () => {
  const bridge = createPluginBridge(fixtureBindings());
  const { plugins } = await bridge.getPluginContributions!({});

  assert.deepEqual(plugins[0]?.menus[0], {
    id: 'palette-run',
    command: 'demo.run',
    location: 'commandPalette',
    title: 'Run Demo',
    visible: true,
    enabled: true,
    group: 'demo',
    order: 2,
    showKeybinding: true,
  });
  assert.deepEqual(plugins[0]?.keybindings[0], {
    command: 'demo.run',
    key: 'ctrl+alt+d',
    windows: 'ctrl+shift+d',
    args: { source: 'test' },
    enabled: true,
  });
});

test('openPluginView resolves declarative settings views and emits close events', async () => {
  const bindings = fixtureBindings();
  const bridge = createPluginBridge(bindings);
  const closed: string[] = [];
  bridge.onPluginViewClosed!(event => closed.push(`${event.instanceId}:${event.viewId}:${event.reason}`));

  const opened = await bridge.openPluginView!({ viewId: 'status', scopeId: 'window:test' });
  assert.match(opened.instanceId, /^view:\d+:status$/);
  // Opening the same view in the same scope is idempotent.
  const reopened = await bridge.openPluginView!({ viewId: 'status', scopeId: 'window:test' });
  assert.equal(reopened.instanceId, opened.instanceId);
  // Bounds/visibility are host-rendered no-ops that resolve.
  await bridge.setPluginViewBounds!(opened.instanceId, { x: 0, y: 0, width: 10, height: 10 });
  await bridge.setPluginViewVisibility!(opened.instanceId, false);

  await assert.rejects(
    () => bridge.openPluginView!({ viewId: 'missing', scopeId: 'window:test' }),
    /not declared by an enabled plugin/u,
  );

  await bridge.closePluginView!(opened.instanceId);
  assert.deepEqual(closed, [`${opened.instanceId}:status:host`]);
  await bridge.closePluginView!(opened.instanceId);
  assert.equal(closed.length, 1, 'closing an unknown instance must not re-emit');
});

test('openPluginView rejects unsupported view locations', async () => {
  const bindings = fixtureBindings();
  bindings.UISchema = async () => ({
    views: [{ id: 'future', type: 'card', title: 'Future', location: 'aside' }],
  });
  const bridge = createPluginBridge(bindings);
  await assert.rejects(
    () => bridge.openPluginView!({ viewId: 'future', scopeId: 'window:test' }),
    /location "aside" is not supported/u,
  );
});

test('getPluginViewData merges view.data dispatch results over settings values', async () => {
  const bindings = fixtureBindings();
  const bridge = createPluginBridge(bindings);
  bindings.Settings = async () => ({ enabled: true, greeting: 'from settings' });

  const merged = await bridge.getPluginViewData!('demo-plugin', 'status', ['enabled', 'greeting']);
  assert.equal(merged.source, 'plugin');
  assert.equal(merged.data.enabled, 'live-from-plugin');
  // Settings values backfill bindings the plugin did not answer.
  assert.equal(merged.data.greeting, 'from settings');
  assert.equal(bindings.dispatches[0]?.method, 'view.data');
  assert.deepEqual(JSON.parse(bindings.dispatches[0]?.payload ?? '{}'), {
    viewId: 'status',
    bindings: ['enabled', 'greeting'],
  });

  const failing = fixtureBindings();
  failing.CallPlugin = async () => { throw new Error('dispatch trapped'); };
  failing.Settings = async () => ({ enabled: false });
  const fallback = await createPluginBridge(failing).getPluginViewData!('demo-plugin', 'status', ['enabled']);
  assert.deepEqual(fallback, { source: 'settings', data: { enabled: false } });
});

test('executePluginCommand falls back to the WASM dispatch channel without a companion', async () => {
  const bindings = fixtureBindings();
  bindings.NativeRunning = async () => false;
  const bridge = createPluginBridge(bindings);

  assert.deepEqual(await bridge.executePluginCommand!('demo.run', undefined, {}), { via: 'wasm' });
  assert.equal(bindings.dispatches[0]?.method, 'command.execute');

  const failing = fixtureBindings();
  failing.NativeRunning = async () => false;
  failing.CallPlugin = async () => ({ ok: false, error: { code: 'not_found', message: 'unknown method' } });
  await assert.rejects(
    () => createPluginBridge(failing).executePluginCommand!('demo.run'),
    /failed: unknown method/u,
  );
});

test('mergePluginViewData falls back to settings without a plugin result', () => {
  assert.deepEqual(
    mergePluginViewData({ a: 1 }, ['a', 'b']),
    { source: 'settings', data: { a: 1 } },
  );
  assert.deepEqual(
    mergePluginViewData({ a: 1 }, ['a'], { a: 2 }),
    { source: 'plugin', data: { a: 2 } },
  );
  assert.deepEqual(
    mergePluginViewData({ a: 1 }, ['a'], [1, 2]),
    { source: 'settings', data: { a: 1 } },
  );
});

function providerFixtureBindings(): NativePluginBindings {
  return {
    List: async () => [],
    TerminalProviders: async (kind, locale) => {
      if (kind !== 'terminal.theme') return [];
      assert.equal(locale, 'en');
      return [{
        pluginId: 'hello-lemonssh',
        pluginVersion: '0.4.0',
        pluginDisplayName: 'Hello LemonSSH',
        provider: {
          id: 'com.lemonssh.hello.accent',
          label: 'Hello Accent',
          kind: 'terminal.theme',
        },
      }];
    },
    ProvideTerminal: async request => {
      void request;
      return [{
        pluginId: 'hello-lemonssh',
        pluginVersion: '0.4.0',
        providerId: 'com.lemonssh.hello.accent',
        kind: 'terminal.theme',
        requestId: 'terminal-fixture-1',
        status: 'ok',
        result: { colors: { cursor: '#34d399' } },
      }];
    },
    CancelTerminalRequest: async requestID => requestID === 'terminal-fixture-1',
    PublishTerminalSessionEvent: async event => {
      void event;
      return [{ pluginId: 'hello-lemonssh', delivered: true }];
    },
    ExtensionProviders: async kind => {
      if (kind !== 'sync') return [];
      return [{
        pluginId: 'sync-demo',
        pluginVersion: '2.0.0',
        pluginDisplayName: 'Sync Demo',
        provider: { id: 'com.sync.demo', label: 'Sync Demo', kind: 'sync' },
      }];
    },
  };
}

test('plugin bridge serves terminal providers through the Go host registry', async () => {
  const bridge = createPluginBridge(providerFixtureBindings());

  const providers = await bridge.listPluginTerminalProviders!({ kind: 'terminal.theme', locale: 'en' });
  assert.equal(providers.length, 1);
  assert.equal(providers[0]?.pluginId, 'hello-lemonssh');
  assert.equal(providers[0]?.pluginVersion, '0.4.0');
  assert.equal(providers[0]?.pluginDisplayName, 'Hello LemonSSH');
  assert.equal(providers[0]?.provider.id, 'com.lemonssh.hello.accent');
  assert.equal(providers[0]?.provider.kind, 'terminal.theme');

  const results = await bridge.providePluginTerminal!({
    requestId: 'terminal-fixture-1',
    kind: 'terminal.theme',
    operation: 'provideTheme',
    session: { sessionId: 'session-1', protocol: 'ssh', status: 'connected' },
    payload: { reason: 'session-state' },
    deadlineMs: 1500,
  });
  assert.equal(results.length, 1);
  assert.equal(results[0]?.status, 'ok');
  assert.deepEqual((results[0] as { result?: unknown }).result, { colors: { cursor: '#34d399' } });

  assert.equal(await bridge.cancelPluginTerminalRequest!('terminal-fixture-1'), true);
  assert.equal(await bridge.cancelPluginTerminalRequest!('terminal-unknown'), false);

  const deliveries = await bridge.publishPluginTerminalSessionEvent!({
    type: 'connected',
    session: { sessionId: 'session-1', protocol: 'ssh', status: 'connected' },
  });
  assert.deepEqual(deliveries, [{ pluginId: 'hello-lemonssh', delivered: true }]);
});

test('plugin bridge serves extension providers through the same registry', async () => {
  const bridge = createPluginBridge(providerFixtureBindings());
  const providers = await bridge.listPluginExtensionProviders!({ kind: 'sync' });
  assert.equal(providers.length, 1);
  assert.equal(providers[0]?.pluginId, 'sync-demo');
  assert.equal(providers[0]?.provider.kind, 'sync');
  // Terminal kinds are not extension kinds; the bridge passes the kind
  // through and the fixture (like the Go registry) answers with nothing.
  assert.equal((await bridge.listPluginExtensionProviders!({ kind: 'terminal.theme' as 'sync' })).length, 0);
});

test('plugin bridge degrades provider surfaces when native methods are absent', async () => {
  const bridge = createPluginBridge({ List: async () => [] });
  assert.deepEqual(await bridge.listPluginTerminalProviders!({ kind: 'terminal.theme' }), []);
  assert.deepEqual(await bridge.providePluginTerminal!({
    requestId: 'r',
    kind: 'terminal.theme',
    operation: 'provideTheme',
    session: { sessionId: 's', protocol: 'ssh', status: 'connected' },
  }), []);
  assert.equal(await bridge.cancelPluginTerminalRequest!('r'), false);
  assert.deepEqual(await bridge.publishPluginTerminalSessionEvent!({
    type: 'connected',
    session: { sessionId: 's', protocol: 'ssh', status: 'connected' },
  }), []);
  assert.deepEqual(await bridge.listPluginExtensionProviders!({ kind: 'sync' }), []);
});

