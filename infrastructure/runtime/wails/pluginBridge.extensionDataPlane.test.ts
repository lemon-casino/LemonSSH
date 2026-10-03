import assert from 'node:assert/strict';
import test from 'node:test';

import {
  createPluginBridge,
  pluginBridgeBase64ToBytes,
  pluginBridgeBytesToBase64,
  type NativePluginBindings,
  type NativePluginRecord,
} from './pluginBridge';

function base64Of(text: string): string {
  return pluginBridgeBytesToBase64(new TextEncoder().encode(text));
}

/** Fake Go extension surface: records calls, replays canned results. */
interface ExtensionHarness {
  calls: Array<{ name: string; args: unknown[] }>;
  providers: Array<{ pluginId: string; providerId: string } | null>;
}

function extensionBindings(overrides: Partial<NativePluginBindings> = {}): NativePluginBindings & ExtensionHarness {
  const calls: Array<{ name: string; args: unknown[] }> = [];
  const providers: Array<{ pluginId: string; providerId: string } | null> = [];
  // Every method records its call, then delegates to the override (when one
  // is provided) so tests can assert both the forwarding and the mapping.
  const wire = (name: string, impl: unknown) => async (...args: unknown[]) => {
    calls.push({ name, args });
    if (typeof impl === 'function') {
      return await (impl as (...callArgs: unknown[]) => unknown)(...args);
    }
    return undefined;
  };
  const bindings = {
    calls,
    providers,
    List: async () => [] as NativePluginRecord[],
    SetEnabled: async () => undefined,
    Uninstall: async () => true,
    ExtensionProviders: wire('ExtensionProviders', overrides.ExtensionProviders ?? (async () => providers)),
    InvokePluginExtensionProvider: wire('InvokePluginExtensionProvider', overrides.InvokePluginExtensionProvider),
    CancelPluginExtensionRequest: wire('CancelPluginExtensionRequest', overrides.CancelPluginExtensionRequest),
    PluginSyncConnect: wire('PluginSyncConnect', overrides.PluginSyncConnect),
    PluginSyncDisconnect: wire('PluginSyncDisconnect', overrides.PluginSyncDisconnect),
    PluginSyncGetAccount: wire('PluginSyncGetAccount', overrides.PluginSyncGetAccount),
    PluginSyncGetCapabilities: wire('PluginSyncGetCapabilities', overrides.PluginSyncGetCapabilities),
    PluginSyncReadObject: wire('PluginSyncReadObject', overrides.PluginSyncReadObject),
    PluginSyncReadChunk: wire('PluginSyncReadChunk', overrides.PluginSyncReadChunk),
    PluginSyncWriteObject: wire('PluginSyncWriteObject', overrides.PluginSyncWriteObject),
    PluginSyncWriteBegin: wire('PluginSyncWriteBegin', overrides.PluginSyncWriteBegin),
    PluginSyncWriteChunk: wire('PluginSyncWriteChunk', overrides.PluginSyncWriteChunk),
    PluginSyncWriteCommit: wire('PluginSyncWriteCommit', overrides.PluginSyncWriteCommit),
    PluginSyncDeleteObject: wire('PluginSyncDeleteObject', overrides.PluginSyncDeleteObject),
    PluginSyncPutSecret: wire('PluginSyncPutSecret', overrides.PluginSyncPutSecret),
    PluginSyncDeleteSecrets: wire('PluginSyncDeleteSecrets', overrides.PluginSyncDeleteSecrets),
    PluginSyncRestoreSecrets: wire('PluginSyncRestoreSecrets', overrides.PluginSyncRestoreSecrets),
    StartPluginConnection: wire('StartPluginConnection', overrides.StartPluginConnection),
    WritePluginConnection: wire('WritePluginConnection', overrides.WritePluginConnection),
    ControlPluginConnection: wire('ControlPluginConnection', overrides.ControlPluginConnection),
    DetectPluginImporter: wire('DetectPluginImporter', overrides.DetectPluginImporter),
    SelectPluginImporterFile: wire('SelectPluginImporterFile', overrides.SelectPluginImporterFile),
    ReleasePluginImporterFile: wire('ReleasePluginImporterFile', overrides.ReleasePluginImporterFile),
    ParsePluginImporterFile: wire('ParsePluginImporterFile', overrides.ParsePluginImporterFile),
    RespondPluginAuthenticationChallenge: wire('RespondPluginAuthenticationChallenge', overrides.RespondPluginAuthenticationChallenge),
    SetPluginConnectionDataEvents: wire('SetPluginConnectionDataEvents', overrides.SetPluginConnectionDataEvents),
  } as unknown as NativePluginBindings & ExtensionHarness;
  return bindings;
}

test('plugin sync data plane maps base64 object reads and streamed chunk pulls', async () => {
  const bindings = extensionBindings({
    PluginSyncReadObject: async () => ({
      found: true,
      key: 'payload.enc',
      data: base64Of('hello-object'),
      byteLength: 12,
      revision: 'r1',
      contentType: 'application/octet-stream',
    }),
    PluginSyncReadChunk: async request => {
      assert.equal(request.transferId, 'transfer-1');
      assert.equal((request as { maxBytes?: number }).maxBytes, 262144);
      return { chunk: base64Of('chunk-bytes'), done: true };
    },
  });
  const bridge = createPluginBridge(bindings);
  const read = await bridge.pluginSyncReadObject!({ requestId: 'r1', providerId: 'ext.sync', key: 'payload.enc' });
  assert.equal(read.found, true);
  assert.equal(new TextDecoder().decode(read.data!), 'hello-object');
  assert.equal(read.revision, 'r1');

  const chunk = await bridge.pluginSyncReadChunk!({ requestId: 'r1', transferId: 'transfer-1', maxBytes: 262144 });
  assert.equal(new TextDecoder().decode(chunk.chunk), 'chunk-bytes');
  assert.equal(chunk.done, true);

  // A null Go answer maps to a typed error; an absent surface to "unavailable".
  await assert.rejects(
    createPluginBridge(extensionBindings()).pluginSyncReadObject!({ requestId: 'r2', providerId: 'ext.sync', key: 'x' }),
    /no result/,
  );
  const bareBridge = createPluginBridge({} as NativePluginBindings);
  await assert.rejects(
    bareBridge.pluginSyncReadObject!({ requestId: 'r2', providerId: 'ext.sync', key: 'x' }),
    /unavailable/,
  );
});

test('plugin sync data plane encodes writes as base64 and maps results', async () => {
  const bindings = extensionBindings({
    PluginSyncWriteObject: async () => ({ created: true, revision: 'w1' }),
    PluginSyncWriteBegin: async () => ({ transferId: 't1', windowBytes: 262144 }),
    PluginSyncWriteChunk: async (request: { chunk: string }) => {
      assert.equal(new TextDecoder().decode(pluginBridgeBase64ToBytes(request.chunk)), 'stream-bytes');
      return { accepted: 11 };
    },
    PluginSyncWriteCommit: async () => ({ created: true, revision: 'w2' }),
    PluginSyncDeleteObject: async () => ({ deleted: true }),
    PluginSyncPutSecret: async () => ({ kind: 'secret', id: 'ext.sync', key: 'token', created: true }),
    PluginSyncDeleteSecrets: async () => ({ deleted: 2 }),
    PluginSyncRestoreSecrets: async () => ({ restored: 1, discarded: 0 }),
  });
  const bridge = createPluginBridge(bindings);
  const write = await bridge.pluginSyncWriteObject!({
    requestId: 'w1', providerId: 'ext.sync', key: 'small.enc', data: new TextEncoder().encode('tiny'),
  });
  assert.deepEqual(write, { created: true, revision: 'w1' });

  const begin = await bridge.pluginSyncWriteBegin!({ requestId: 't1', providerId: 'ext.sync', key: 'big.enc', byteLength: 100 });
  assert.equal(begin.transferId, 't1');
  const accepted = await bridge.pluginSyncWriteChunk!({
    requestId: 't1', transferId: 't1', sequence: 0, chunk: new TextEncoder().encode('stream-bytes'),
  });
  assert.equal(accepted.accepted, 11);
  const commit = await bridge.pluginSyncWriteCommit!({ requestId: 't1', transferId: 't1' });
  assert.equal(commit.revision, 'w2');
  const deleted = await bridge.pluginSyncDeleteObject!({ requestId: 'd1', providerId: 'ext.sync', key: 'old.enc' });
  assert.deepEqual(deleted, { deleted: true });

  const secret = await bridge.pluginSyncPutSecret!({ providerId: 'ext.sync', key: 'token', value: 's3cret' });
  assert.equal(secret.kind, 'secret');
  assert.deepEqual(await bridge.pluginSyncDeleteSecrets!({ providerId: 'ext.sync' }), { deleted: 2 });
  assert.deepEqual(await bridge.pluginSyncRestoreSecrets!({ providerId: 'ext.sync', keys: ['token'] }), { restored: 1, discarded: 0 });

  // The Go surface received the base64 encodings.
  const writeCall = bindings.calls.find(call => call.name === 'PluginSyncWriteObject');
  assert.equal(new TextDecoder().decode(pluginBridgeBase64ToBytes((writeCall!.args[0] as { data: string }).data)), 'tiny');
});

test('startPluginConnection attaches the data plane and maps diagnostics', async () => {
  const attached: string[] = [];
  const bindings = extensionBindings({
    StartPluginConnection: async () => ({
      sessionId: 'session-9',
      providerId: 'ext.conn',
      status: 'connecting',
      diagnostics: [{ severity: 'warning', message: 'legacy option ignored' }],
    }),
  });
  const bridge = createPluginBridge(bindings, {
    attachDataPlane: async sessionId => {
      attached.push(sessionId);
    },
  });
  const opened = await bridge.startPluginConnection!({
    sessionId: 'session-9',
    providerId: 'ext.conn',
    configuration: { host: 'example' },
    columns: 80,
    rows: 24,
  });
  assert.equal(opened.sessionId, 'session-9');
  assert.equal(opened.status, 'connecting');
  assert.equal(opened.diagnostics.length, 1);
  assert.deepEqual(attached, ['session-9']);
});

test('writePluginConnection and controlPluginConnection forward through the native surface', async () => {
  const bindings = extensionBindings({
    ControlPluginConnection: async () => ({ status: 'connected' }),
  });
  const bridge = createPluginBridge(bindings);
  await bridge.writePluginConnection!('session-9', new TextEncoder().encode('ls\r\n'));
  const writeCall = bindings.calls.find(call => call.name === 'WritePluginConnection');
  assert.equal(new TextDecoder().decode(pluginBridgeBase64ToBytes((writeCall!.args[1] as string))), 'ls\r\n');
  const status = await bridge.controlPluginConnection!('session-9', 'getStatus');
  assert.deepEqual(status, { status: 'connected' });
});

test('importer surface converts the staged sample and forwards parse results', async () => {
  const bindings = extensionBindings({
    SelectPluginImporterFile: async () => ({ selectionToken: 'tok-1', fileName: 'hosts.csv', sample: base64Of('host,user') }),
    DetectPluginImporter: async () => ({ confidence: 0.9, format: 'csv' }),
    ParsePluginImporterFile: async () => ({
      providerId: 'ext.import',
      result: { parsed: 2, warnings: 1, errors: 0 },
      records: [{ type: 'draft', draft: { kind: 'host', value: { label: 'a' } } }],
    }),
    ReleasePluginImporterFile: async () => true,
  });
  const bridge = createPluginBridge(bindings);
  const staged = await bridge.selectPluginImporterFile!();
  assert.equal(new TextDecoder().decode(staged!.sample), 'host,user');
  const detect = await bridge.detectPluginImporter!({ providerId: 'ext.import', sample: staged!.sample });
  assert.equal(detect.confidence, 0.9);
  assert.equal(detect.format, 'csv');
  const parsed = await bridge.parsePluginImporterFile!({ providerId: 'ext.import', selectionToken: 'tok-1' });
  assert.equal(parsed.records.length, 1);
  assert.equal(await bridge.releasePluginImporterFile!('tok-1'), true);
});

test('authentication challenges subscribe to the Go event and respond through the channel', async () => {
  const listeners = new Map<string, (payload: unknown) => void>();
  const bindings = extensionBindings({
    RespondPluginAuthenticationChallenge: async () => undefined,
  });
  const bridge = createPluginBridge(bindings, {
    subscribeEvent: (name, callback) => {
      listeners.set(name, callback);
      return () => listeners.delete(name);
    },
  });
  const received: unknown[] = [];
  const unsubscribe = bridge.onPluginAuthenticationChallenge!(event => received.push(event));
  assert.ok(listeners.has('plugin:authentication-challenge'));

  listeners.get('plugin:authentication-challenge')!({
    requestId: 'req-1',
    challengeRequestId: 'req-1',
    challenge: { id: 'chal-1', kind: 'password', title: 'Password' },
  });
  listeners.get('plugin:authentication-challenge')!({ requestId: 'req-2', challengeRequestId: 'req-2', cancelled: true });
  assert.equal(received.length, 2);
  unsubscribe();

  await bridge.respondPluginAuthenticationChallenge!({
    requestId: 'req-1', challengeRequestId: 'req-1', challengeId: 'chal-1', response: 'hunter2',
  });
  const respondCall = bindings.calls.find(call => call.name === 'RespondPluginAuthenticationChallenge');
  assert.equal((respondCall!.args[0] as { response?: unknown }).response, 'hunter2');
});

test('plugin connection data events are gated on subscriber presence and decode bytes', async () => {
  const listeners = new Map<string, (payload: unknown) => void>();
  const gates: boolean[] = [];
  const bindings = extensionBindings({
    SetPluginConnectionDataEvents: async active => {
      gates.push(active);
    },
  });
  const bridge = createPluginBridge(bindings, {
    subscribeEvent: (name, callback) => {
      listeners.set(name, callback);
      return () => listeners.delete(name);
    },
  });
  const received: Array<{ sessionId: string; data: Uint8Array }> = [];
  const unsubscribe = bridge.onPluginConnectionData!(event => received.push(event));
  assert.deepEqual(gates, [true]);
  listeners.get('plugin:connection-data')!({ sessionId: 's1', data: base64Of('terminal output') });
  assert.equal(new TextDecoder().decode(received[0].data), 'terminal output');
  assert.equal(received[0].sessionId, 's1');
  unsubscribe();
  assert.deepEqual(gates, [true, false]);
});

test('pluginHostReady reflects the live sync provider registry', async () => {
  const bindings = extensionBindings();
  const bridge = createPluginBridge(bindings);
  // Fail-closed before the async probe resolves.
  assert.equal(bridge.pluginHostReady?.(), false);
  await new Promise(resolve => setTimeout(resolve, 0));
  assert.equal(bridge.pluginHostReady?.(), false); // registry empty

  (bindings.providers as Array<{ pluginId: string; providerId: string } | null>).push({
    pluginId: 'p1', providerId: 'ext.sync',
  });
  await bridge.setPluginEnabled!('noop', true).catch(() => undefined); // notify() re-probes
  await new Promise(resolve => setTimeout(resolve, 0));
  assert.equal(bridge.pluginHostReady?.(), true);
});

test('invokePluginExtensionProvider and cancelPluginExtensionRequest forward through the bridge', async () => {
  const bindings = extensionBindings({
    InvokePluginExtensionProvider: async () => ({ valid: true, issues: [] }),
    CancelPluginExtensionRequest: async () => true,
  });
  const bridge = createPluginBridge(bindings);
  const result = await bridge.invokePluginExtensionProvider!({
    providerId: 'ext.conn', kind: 'connection', operation: 'validateConfiguration', payload: { configuration: {} },
  });
  assert.deepEqual(result, { valid: true, issues: [] });
  assert.equal(await bridge.cancelPluginExtensionRequest!('req-7'), true);
});
