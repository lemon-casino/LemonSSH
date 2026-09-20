import assert from 'node:assert/strict';
import test from 'node:test';

import { createPluginBridge, type NativePluginBindings, type NativePluginRecord } from './pluginBridge';

function fixtureBindings(): NativePluginBindings & { records: NativePluginRecord[]; calls: Array<{ method: string; params: string }> } {
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
  return {
    records,
    calls,
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
      views: [{ id: 'status', type: 'card', title: 'Status' }],
    }),
    Settings: async () => ({ enabled: true }),
    SetSetting: async () => undefined,
    ResetSetting: async () => undefined,
    NativeRunning: async () => true,
    CallNative: async (_pluginId, method, params) => {
      calls.push({ method, params });
      return JSON.stringify({ ok: true });
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

  assert.deepEqual(await bridge.executePluginCommand!('demo.run', { value: 1 }, { source: 'test' }), { ok: true });
  assert.equal(bindings.calls[0]?.method, 'command.execute');

  assert.equal((await bridge.setPluginEnabled!('demo-plugin', false)).enabled, false);
  assert.equal(await bridge.uninstallPlugin!('demo-plugin'), true);
});
