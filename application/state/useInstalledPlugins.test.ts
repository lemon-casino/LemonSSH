import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import { act, create, type ReactTestRenderer } from 'react-test-renderer';

import {
  describeInstalledPlugin,
  useInstalledPlugins,
} from './useInstalledPlugins.ts';

function fixturePlugin(overrides: Partial<LemonSSHInstalledPlugin> = {}): LemonSSHInstalledPlugin {
  return {
    id: 'com.example.demo',
    enabled: true,
    activeVersion: '1.0.0',
    manifest: { displayName: 'Demo Plugin', description: 'fixture' },
    runtime: { status: 'active', kind: null, lastError: null, quarantinedAt: null },
    ...overrides,
  };
}

function fixtureBridge() {
  const calls: string[] = [];
  let installed = [fixturePlugin()];
  let listeners = new Set<() => void>();
  return {
    calls,
    bridge: {
      listPlugins: async () => installed,
      installPluginPackage: async (archivePath: string, options?: { enable?: boolean }) => {
        calls.push(`install:${archivePath}:${String(options?.enable !== false)}`);
        installed = [fixturePlugin({ id: 'com.example.new' })];
        return installed[0];
      },
      setPluginEnabled: async (pluginId: string, enabled: boolean) => {
        calls.push(`setEnabled:${pluginId}:${String(enabled)}`);
        installed = installed.map((plugin) => (
          plugin.id === pluginId ? { ...plugin, enabled } : plugin
        ));
        return installed[0];
      },
      restartPlugin: async (pluginId: string) => {
        calls.push(`restart:${pluginId}`);
        return installed.find((plugin) => plugin.id === pluginId) ?? null;
      },
      uninstallPlugin: async (pluginId: string) => {
        calls.push(`uninstall:${pluginId}`);
        installed = installed.filter((plugin) => plugin.id !== pluginId);
        return true;
      },
      onPluginContributionsChanged: (listener: () => void) => {
        listeners.add(listener);
        return () => { listeners.delete(listener); };
      },
      notifyChange: () => {
        for (const listener of [...listeners]) listener();
      },
    },
  };
}

type ManagerFixture = ReturnType<typeof fixtureBridge>;

async function renderManager(bridge: ManagerFixture['bridge'] | undefined) {  const actEnvironment = globalThis as typeof globalThis & {
    IS_REACT_ACT_ENVIRONMENT?: boolean;
  };
  const previousActEnvironment = actEnvironment.IS_REACT_ACT_ENVIRONMENT;
  actEnvironment.IS_REACT_ACT_ENVIRONMENT = true;
  const originalWindow = globalThis.window;
  globalThis.window = { lemonssh: bridge } as unknown as typeof window;

  let state: ReturnType<typeof useInstalledPlugins> | null = null;
  let renderer: ReactTestRenderer | null = null;
  const Probe = () => {
    state = useInstalledPlugins();
    return null;
  };
  await act(async () => {
    renderer = create(React.createElement(Probe));
  });
  return {
    state: () => state!,
    async act(action: () => Promise<unknown>) {
      await act(async () => { await action(); });
    },
    async cleanup() {
      await act(async () => { renderer?.unmount(); });
      globalThis.window = originalWindow;
      actEnvironment.IS_REACT_ACT_ENVIRONMENT = previousActEnvironment;
    },
  };
}

test('installed plugin drafts surface the declared display name and state', () => {
  const draft = describeInstalledPlugin(fixturePlugin());
  assert.deepEqual(draft, {
    id: 'com.example.demo',
    displayName: 'Demo Plugin',
    version: '1.0.0',
    enabled: true,
    runtimeStatus: 'active',
  });
  assert.equal(describeInstalledPlugin(fixturePlugin({
    manifest: 'not-an-object',
    enabled: false,
    activeVersion: null,
    runtime: { status: 'staged', kind: null, lastError: null, quarantinedAt: null },
  })).displayName, 'com.example.demo');
});

test('manager lists installed plugins and maps actions onto the bridge', async () => {
  const { calls, bridge } = fixtureBridge();
  const harness = await renderManager(bridge as never);
  try {
    assert.equal(harness.state().available, true);
    assert.deepEqual(harness.state().plugins.map((plugin) => plugin.id), ['com.example.demo']);

    await harness.act(() => harness.state().installPackage('/tmp/demo.ncpkg', { enable: true }));
    assert.deepEqual(calls, ['install:/tmp/demo.ncpkg:true']);
    assert.deepEqual(harness.state().plugins.map((plugin) => plugin.id), ['com.example.new']);

    await harness.act(() => harness.state().setEnabled('com.example.new', false));
    await harness.act(() => harness.state().restart('com.example.new'));
    await harness.act(() => harness.state().uninstall('com.example.new'));
    assert.deepEqual(calls.slice(1), [
      'setEnabled:com.example.new:false',
      'restart:com.example.new',
      'uninstall:com.example.new',
    ]);
    assert.deepEqual(harness.state().plugins, []);
    assert.equal(harness.state().error, null);
  } finally {
    await harness.cleanup();
  }
});

test('manager failures stay visible in the error state', async () => {
  const failure = new Error('package rejected');
  const harness = await renderManager({
    listPlugins: async () => [fixturePlugin()],
    installPluginPackage: async () => { throw failure; },
    onPluginContributionsChanged: () => () => {},
  } as never);
  try {
    await harness.act(async () => {
      try {
        await harness.state().installPackage('/tmp/bad.ncpkg');
      } catch {
        // Rejection is expected; the hook must also record it.
      }
    });
    assert.match(harness.state().error?.message ?? '', /package rejected/);
  } finally {
    await harness.cleanup();
  }
});

test('manager is unavailable without a bridge or stays disabled on load failure', async () => {
  const noneHarness = await renderManager(undefined);
  try {
    assert.equal(noneHarness.state().available, false);
    assert.equal(noneHarness.state().plugins.length, 0);
  } finally {
    await noneHarness.cleanup();
  }

  const failingHarness = await renderManager({
    listPlugins: async () => { throw new Error('inventory boom'); },
    onPluginContributionsChanged: () => () => {},
  } as never);
  try {
    assert.equal(failingHarness.state().available, false);
    assert.match(failingHarness.state().error?.message ?? '', /inventory boom/);
  } finally {
    await failingHarness.cleanup();
  }
});
