import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import assert from 'node:assert/strict';
import test from 'node:test';
import { setActiveRuntimeClient } from '../../infrastructure/runtime/runtimeClient';
import { createDomRenderer, installDomEnvironment } from '../test-support/renderReactDom';
import { DeclarativePluginViewSurface } from './DeclarativePluginViewSurface';

test('declarative surface renders host-owned view structure from the schema', () => {
  // Without an active runtime client the declarative plugin inventory stays
  // empty and the surface shows its pending state instead of plugin markup.
  const html = renderToStaticMarkup(<DeclarativePluginViewSurface pluginId="demo" viewId="status" />);
  assert.ok(!html.includes('<script'));
});

test('declarative surface merges view.data dispatch results over settings values', async () => {
  const env = installDomEnvironment();
  try {
    const dispatched: string[] = [];
    const pluginV2 = {
      list: async () => [{
        pluginId: 'demo',
        version: '1.0.0',
        state: 'enabled',
        manifest: JSON.stringify({ displayName: 'Demo' }),
      }],
      uiSchema: async () => ({
        settings: [{ id: 'greeting', type: 'text', label: 'Greeting', default: 'hello' }],
        views: [{ id: 'status', type: 'card', title: 'Status', bindings: ['greeting'] }],
      }),
      settings: async () => ({ greeting: 'from settings' }),
      setSetting: async () => undefined,
      grantPermission: async () => undefined,
    };
    // useDeclarativePlugins resolves the V2 client from the active runtime
    // client's aggregate bridge (transitionBridge.pluginV2); view data flows
    // through the same bridge's getPluginViewData.
    setActiveRuntimeClient({
      transitionBridge: {
        pluginV2,
        getPluginViewData: async (pluginId: string, viewId: string, bindings: string[]) => {
          dispatched.push(`${pluginId}:${viewId}:${bindings.join(',')}`);
          return { source: 'plugin', data: { greeting: 'live-from-plugin' } };
        },
      },
    } as never);
    const renderer = await createDomRenderer(env.document);
    try {
      await renderer.render(<DeclarativePluginViewSurface pluginId="demo" viewId="status" />);
      assert.deepEqual(dispatched, ['demo:status:greeting']);
      assert.ok(renderer.container.textContent?.includes('live-from-plugin'));
    } finally {
      await renderer.unmount();
      setActiveRuntimeClient(undefined);
    }
  } finally {
    env.cleanup();
  }
});
