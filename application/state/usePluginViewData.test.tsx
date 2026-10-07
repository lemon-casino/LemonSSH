import assert from 'node:assert/strict';
import test from 'node:test';
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { JSDOM } from 'jsdom';

import {
  collectPluginViewDataRequests,
  serializePluginViewDataRequests,
  usePluginViewData,
  type PluginViewDataRequest,
} from './usePluginViewData.ts';

test('view data requests deduplicate per plugin+view and drop binding-less entries', () => {
  const collected = collectPluginViewDataRequests([
    { pluginId: 'demo', viewId: 'rows', bindings: ['a'] },
    { pluginId: 'demo', viewId: 'rows', bindings: ['a', 'b'] },
    { pluginId: 'demo', viewId: 'empty', bindings: [] },
    { pluginId: 'other', viewId: 'rows', bindings: ['c'] },
  ]);
  assert.deepEqual(collected, [
    { pluginId: 'demo', viewId: 'rows', bindings: ['a'] },
    { pluginId: 'other', viewId: 'rows', bindings: ['c'] },
  ]);
  assert.equal(
    serializePluginViewDataRequests([{ pluginId: 'demo', viewId: 'rows', bindings: ['a'] }]),
    JSON.stringify([{ pluginId: 'demo', viewId: 'rows', bindings: ['a'] }]),
  );
});

function Harness({
  requests,
  onState,
}: {
  requests: PluginViewDataRequest[];
  onState: (state: { loading: boolean; json: string }) => void;
}) {
  const { data, loading } = usePluginViewData(requests);
  onState({ loading, json: JSON.stringify(data) });
  return null;
}

function withDom(): JSDOM {
  const dom = new JSDOM('<!doctype html><html><body></body></html>', { url: 'http://localhost' });
  Object.defineProperty(globalThis, 'window', { configurable: true, value: dom.window });
  Object.defineProperty(globalThis, 'document', { configurable: true, value: dom.window.document });
  Object.defineProperty(globalThis, 'IS_REACT_ACT_ENVIRONMENT', { configurable: true, value: true });
  return dom;
}

test('view data hook merges dispatch results and survives per-view failures', async () => {
  const dom = withDom();
  const dispatched: Array<{ pluginId: string; method: string; payload: string }> = [];
  (dom.window as unknown as { lemonssh: unknown }).lemonssh = {
    getPluginViewData: async (pluginId: string, viewId: string, bindings: string[]) => {
      dispatched.push({ pluginId, method: 'view.data', payload: JSON.stringify({ viewId, bindings }) });
      if (pluginId === 'broken') throw new Error('dispatch trapped');
      if (pluginId === 'fallback-only') {
        // The bridge degrades transport failures to the settings layer.
        return { source: 'settings', data: { greeting: 'from settings' } };
      }
      return {
        source: 'plugin',
        data: { greeting: 'from plugin', extra: 1 },
      };
    },
  };

  const container = document.createElement('div');
  document.body.appendChild(container);
  const root = createRoot(container);
  const states: Array<{ loading: boolean; json: string }> = [];
  const requests: PluginViewDataRequest[] = [
    { pluginId: 'demo', viewId: 'status', bindings: ['greeting'] },
    { pluginId: 'broken', viewId: 'rows', bindings: ['row1'] },
    { pluginId: 'fallback-only', viewId: 'status', bindings: ['greeting'] },
  ];
  await act(async () => {
    root.render(<Harness requests={requests} onState={state => states.push(state)} />);
    await Promise.resolve();
  });

  assert.deepEqual(dispatched.map(entry => entry.pluginId), ['demo', 'broken', 'fallback-only']);
  const finalState = states.at(-1)!;
  assert.equal(finalState.loading, false);
  const snapshot = JSON.parse(finalState.json) as Record<string, Record<string, { source: string; data: Record<string, unknown> }>>;
  // plugin dispatch result wins for the binding it answered…
  assert.deepEqual(snapshot.demo.status, { source: 'plugin', data: { greeting: 'from plugin', extra: 1 } });
  // …failed views are omitted (fail soft) rather than crashing the batch…
  assert.equal(snapshot.broken, undefined);
  // …and transport failures degrade to the settings layer the bridge assembles.
  assert.deepEqual(snapshot['fallback-only']?.status, { source: 'settings', data: { greeting: 'from settings' } });

  await act(async () => root.unmount());
  container.remove();
  dom.window.close();
});
