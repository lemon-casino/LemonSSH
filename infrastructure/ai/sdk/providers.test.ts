import test from 'node:test';
import assert from 'node:assert/strict';
import { createBridgeFetchForSDK } from './providers.ts';

// Regression (WV3-L195 live smoke): the Wails transition bridge is a PARTIAL
// NetcattyBridge — aiFetch exists, the Electron streaming channel does not.
// The old bridge-truthiness guard sent streaming requests into
// bridge.onAiStreamData(...) and crashed with "o.onAiStreamData is not a
// function", surfacing as "No output generated" in the chat.

type FakeBridge = Record<string, unknown>;

function withWindowBridge(bridge: FakeBridge | null, fn: () => Promise<void>): Promise<void> {
  const globalAsAny = globalThis as unknown as { window?: unknown };
  const originalWindow = globalAsAny.window;
  const originalFetch = globalThis.fetch;
  globalAsAny.window = bridge === null ? {} : { netcatty: bridge };
  return (async () => {
    try {
      await fn();
    } finally {
      globalAsAny.window = originalWindow;
      globalThis.fetch = originalFetch;
    }
  })();
}

function stubDirectFetch(calls: Array<{ input: string | URL | Request }>): void {
  globalThis.fetch = (async (input: string | URL | Request) => {
    calls.push({ input });
    return new Response('{}', { status: 200, headers: { 'content-type': 'application/json' } });
  }) as typeof globalThis.fetch;
}

const STREAM_BODY = JSON.stringify({ model: 'm', stream: true, messages: [] });

test('streaming request with a partial bridge falls back to direct fetch instead of crashing', async () => {
  const directCalls: Array<{ input: string | URL | Request }> = [];
  const aiFetchCalls: unknown[] = [];
  const bridge: FakeBridge = {
    // Transition-bridge shape: aiFetch implemented, streaming channel absent.
    aiFetch: async () => {
      aiFetchCalls.push(arguments);
      return { ok: true, status: 200, data: '{}' };
    },
  };
  await withWindowBridge(bridge, async () => {
    stubDirectFetch(directCalls);
    const fetchLike = createBridgeFetchForSDK('prov-1');
    const response = await fetchLike('https://api.example.com/v1/chat/completions', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: STREAM_BODY,
    });
    assert.equal(response.status, 200);
  });
  assert.equal(directCalls.length, 1, 'must fall back to globalThis.fetch');
  assert.equal(aiFetchCalls.length, 0, 'must not touch aiFetch for a streaming request');
});

test('non-streaming request keeps using the bridge aiFetch surface', async () => {
  const directCalls: Array<{ input: string | URL | Request }> = [];
  let aiFetchCalled = 0;
  const bridge: FakeBridge = {
    aiFetch: async () => {
      aiFetchCalled += 1;
      return { ok: true, status: 200, data: '{"models":[]}' };
    },
  };
  await withWindowBridge(bridge, async () => {
    stubDirectFetch(directCalls);
    const fetchLike = createBridgeFetchForSDK('prov-1');
    const response = await fetchLike('https://api.example.com/v1/models', { method: 'GET' });
    assert.equal(response.status, 200);
  });
  assert.equal(aiFetchCalled, 1, 'non-streaming traffic rides the bridge aiFetch surface');
  assert.equal(directCalls.length, 0);
});

test('a full streaming surface still uses the IPC streaming channel', async () => {
  const directCalls: Array<{ input: string | URL | Request }> = [];
  let aiChatStreamCalled = 0;
  const subscribe = () => () => {};
  const bridge: FakeBridge = {
    aiChatStream: async () => {
      aiChatStreamCalled += 1;
      return { ok: true, statusCode: 200, statusText: 'OK' };
    },
    aiChatCancel: async () => true,
    onAiStreamData: subscribe,
    onAiStreamEnd: subscribe,
    onAiStreamError: subscribe,
  };
  await withWindowBridge(bridge, async () => {
    stubDirectFetch(directCalls);
    const fetchLike = createBridgeFetchForSDK('prov-1');
    const response = await fetchLike('https://api.example.com/v1/chat/completions', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: STREAM_BODY,
    });
    assert.equal(response.status, 200);
    assert.equal(response.headers.get('content-type'), 'text/event-stream');
  });
  assert.equal(aiChatStreamCalled, 1, 'full surface keeps the IPC streaming channel');
  assert.equal(directCalls.length, 0);
});

test('a missing bridge keeps the direct fetch fallback', async () => {
  const directCalls: Array<{ input: string | URL | Request }> = [];
  await withWindowBridge(null, async () => {
    stubDirectFetch(directCalls);
    const fetchLike = createBridgeFetchForSDK('prov-1');
    const response = await fetchLike('https://api.example.com/v1/models', { method: 'GET' });
    assert.equal(response.status, 200);
  });
  assert.equal(directCalls.length, 1);
});
