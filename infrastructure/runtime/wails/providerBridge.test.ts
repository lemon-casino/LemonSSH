import assert from 'node:assert/strict';
import test, { type TestContext } from 'node:test';
import { streamText } from 'ai';
import { createProviderBridge, type NativeProviderBindings } from './providerBridge';
import { createBridgeFetchForSDK, createModelFromConfig } from '../../ai/sdk/providers';
import type { ProviderStyle } from '../../ai/types';

function fixture(t: TestContext) {
  const listeners = new Map<string, Set<(event: { data?: unknown }) => void>>();
  const on = (name: string, callback: (event: { data?: unknown }) => void) => {
    const entries = listeners.get(name) ?? new Set(); entries.add(callback); listeners.set(name, entries);
    return () => { entries.delete(callback); };
  };
  const emit = (name: string, data: unknown) => { for (const callback of listeners.get(name) ?? []) callback({ data }); };
  const native: NativeProviderBindings = {
    AllowlistAddHost: async () => ({ OK: true, Error: '' }),
    SyncProviders: async () => ({ OK: true, Error: '' }),
    Fetch: async () => ({ OK: true, Status: 200, Data: '{}', Error: '' }),
    ChatCancel: async () => true,
    ChatStream: async () => ({ OK: true, StatusCode: 200, StatusText: 'OK', Error: '', Aborted: false }),
  };
  const bridge = createProviderBridge(native, on);
  const host = globalThis as unknown as { window?: unknown };
  const previous = host.window;
  const previousFetch = globalThis.fetch;
  host.window = { _wails: {}, netcatty: bridge };
  globalThis.fetch = async () => { throw new Error('WebView direct fetch must not run'); };
  t.after(() => { host.window = previous; globalThis.fetch = previousFetch; });
  return { native, bridge, emit, listeners };
}

test('provider sync retains encrypted key and endpoint settings', async t => {
  const { native, bridge } = fixture(t);
  native.SyncProviders = async providers => {
    assert.equal(providers[0].APIKey, 'enc:v1:encrypted');
    assert.equal(providers[0].BaseURL, 'https://api.anthropic.com');
    assert.equal(providers[0].Enabled, true);
    return { OK: true, Error: '' };
  };
  await bridge.aiSyncProviders!([{ id: 'p', providerId: 'anthropic', apiKey: 'enc:v1:encrypted', enabled: true }]);
});

test('native stream preserves named multiline SSE and ignores other request IDs', async t => {
  const { native, emit } = fixture(t);
  native.ChatStream = async id => {
    emit('ai:stream-data', { requestId: 'other', data: 'must not leak' });
    emit('ai:stream-data', [{ requestId: id, data: 'line one\nline two', event: 'content_block_delta' }]);
    emit('ai:stream-end', { requestId: id });
    return { OK: true, StatusCode: 200, StatusText: 'OK', Error: '', Aborted: false };
  };
  const response = await createBridgeFetchForSDK('p')('https://fixture.test/messages', { method: 'POST', body: '{"stream":true}' });
  assert.equal(await response.text(), 'event: content_block_delta\ndata: line one\ndata: line two\n\n');
});

for (const style of ['openai', 'anthropic', 'google'] as ProviderStyle[]) {
  test(`${style} SDK produces text through the Wails streaming bridge`, async t => {
    const { native, emit } = fixture(t);
    native.ChatStream = async (id, request) => {
      if (style === 'openai') assert.equal(request.URL, 'https://fixture.test/v1/chat/completions');
      if (style === 'google') assert.match(request.URL, /streamGenerateContent/);
      const events: Array<Record<string, unknown>> = style === 'anthropic' ? [
        { type: 'message_start', message: { id: 'msg_test', type: 'message', role: 'assistant', model: 'fixture', content: [], stop_reason: null, stop_sequence: null, usage: { input_tokens: 1, output_tokens: 0 } } },
        { type: 'content_block_start', index: 0, content_block: { type: 'text', text: '' } },
        { type: 'content_block_delta', index: 0, delta: { type: 'text_delta', text: 'hello' } },
        { type: 'content_block_stop', index: 0 },
        { type: 'message_delta', delta: { stop_reason: 'end_turn', stop_sequence: null }, usage: { output_tokens: 1 } },
        { type: 'message_stop' },
      ] : style === 'google' ? [
        { candidates: [{ content: { role: 'model', parts: [{ text: 'hello' }] }, finishReason: 'STOP', index: 0 }], usageMetadata: { promptTokenCount: 1, candidatesTokenCount: 1, totalTokenCount: 2 } },
      ] : [
        { id: 'completion_test', object: 'chat.completion.chunk', choices: [{ index: 0, delta: { content: 'hello' }, finish_reason: null }] },
        { id: 'completion_test', object: 'chat.completion.chunk', choices: [{ index: 0, delta: {}, finish_reason: 'stop' }] },
      ];
      for (const event of events) emit('ai:stream-data', { requestId: id, data: JSON.stringify(event), event: style === 'anthropic' ? event.type : '' });
      emit('ai:stream-end', { requestId: id });
      return { OK: true, StatusCode: 200, StatusText: 'OK', Error: '', Aborted: false };
    };
    const model = createModelFromConfig({ id: 'p', providerId: style, style, name: 'fixture', defaultModel: 'fixture', apiKey: 'encrypted-fixture', baseURL: style === 'openai' ? 'https://fixture.test/' : 'https://fixture.test/v1', enabled: true });
    const result = streamText({ model, prompt: 'hello' });
    assert.equal(await result.text, 'hello');
  });
}

test('cancelling a response reader cancels native HTTP and removes listeners', async t => {
  const { native, listeners } = fixture(t);
  let cancelled = false;
  native.ChatCancel = async () => { cancelled = true; return true; };
  const response = await createBridgeFetchForSDK('p')('https://fixture.test/chat', { method: 'POST', body: '{"stream":true}' });
  await response.body!.cancel();
  assert.equal(cancelled, true);
  assert.ok([...listeners.values()].every(entries => entries.size === 0));
});

test('HTTP failure preserves provider status and non-stream transport errors become valid responses', async t => {
  const { native } = fixture(t);
  native.ChatStream = async () => ({ OK: true, StatusCode: 401, StatusText: 'key rejected', Error: '', Aborted: false });
  const response = await createBridgeFetchForSDK('p')('https://fixture.test/chat', { method: 'POST', body: '{"stream":true}' });
  assert.equal(response.status, 401);
  assert.match(await response.text(), /key rejected/);
  native.Fetch = async () => ({ OK: false, Status: 0, Data: '', Error: 'connection refused' });
  const failed = await createBridgeFetchForSDK('p')('https://fixture.test/models');
  assert.equal(failed.status, 502);
  assert.match(await failed.text(), /connection refused/);
});

test('abort during native header wait rejects and tears down subscriptions', async t => {
  const { native, listeners } = fixture(t);
  const controller = new AbortController();
  let cancelled = 0;
  native.ChatCancel = async () => { cancelled++; return true; };
  native.ChatStream = async () => {
    controller.abort();
    return { OK: false, StatusCode: 0, StatusText: '', Error: 'cancelled', Aborted: true };
  };
  await assert.rejects(createBridgeFetchForSDK('p')(new Request('https://fixture.test/chat', { method: 'POST', body: '{"stream":true}', signal: controller.signal }), { headers: { 'Content-Type': 'application/json' } }), { name: 'AbortError' });
  assert.equal(cancelled, 1);
  assert.ok([...listeners.values()].every(entries => entries.size === 0));
});

test('a rejected native start cleans listeners and Wails never falls back to browser fetch', async t => {
  const { native, listeners } = fixture(t);
  native.ChatStream = async () => { throw new Error('native request failed'); };
  await assert.rejects(createBridgeFetchForSDK('p')('https://fixture.test/chat', { method: 'POST', body: '{"stream":true}' }), /native request failed/);
  assert.ok([...listeners.values()].every(entries => entries.size === 0));
  (window as unknown as { netcatty: unknown }).netcatty = { aiFetch: async () => ({}) };
  await assert.rejects(createBridgeFetchForSDK('p')('https://fixture.test/chat', { method: 'POST', body: '{"stream":true}' }), /Native AI transport is unavailable/);
});
